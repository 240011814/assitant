package service

import (
	"backend/model"
	"backend/service/tools"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino-ext/components/model/ark"
	"github.com/cloudwego/eino/adk"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

type AIAgentService struct {
	ctx            context.Context
	activeProvider *model.AIProvider
	activeModel    *model.AIModel
	enabledModels  []model.AIModel
	// providerByID 启用中的 Provider (按 id 索引): 指定模型 code 时用它解析模型
	// 归属的 Provider, 构建 ChatModel 必须用该 Provider 的 key/baseURL
	providerByID  map[int]*model.AIProvider
	timeout       time.Duration
	timeoutConfig TimeoutConfig
	runnerCache   map[string]*adk.Runner
	promptCache   map[string]string
	// modelCache 模型实例缓存 (按 model code): 编排每个节点/每次运行都构建模型,
	// ark.ChatModel 是无状态配置+HTTP client, 可复用; WithTools 返回副本不改接收者
	modelCache map[string]*ark.ChatModel
	// toolCache 工具实例缓存 (按工具名): 编译每个工具节点/Agent 工具都要查库构建,
	// 工具实现无状态 (config + http.Client), 可复用
	toolCache       map[string]tool.BaseTool
	cacheMu         sync.RWMutex // 保护 runnerCache/promptCache/modelCache/toolCache 及 active* 字段, gin 请求与 ReloadConfig 并发访问
	sysCfgService   *SystemConfigService
	checkpointStore compose.CheckPointStore
	memoryService   *UserMemoryService
}

func NewAIAgentService(timeoutMinutes int, sysCfgService *SystemConfigService) (*AIAgentService, error) {
	if timeoutMinutes <= 0 {
		timeoutMinutes = 5
	}
	s := &AIAgentService{
		ctx:             context.Background(),
		timeout:         time.Duration(timeoutMinutes) * time.Minute,
		runnerCache:     make(map[string]*adk.Runner),
		promptCache:     make(map[string]string),
		modelCache:      make(map[string]*ark.ChatModel),
		toolCache:       make(map[string]tool.BaseTool),
		sysCfgService:   sysCfgService,
		checkpointStore: NewInMemoryCheckPointStore(),
	}
	// 从数据库读取超时配置
	if sysCfgService != nil {
		s.timeoutConfig = sysCfgService.GetTimeoutConfig()
		// 如果数据库中有AI请求超时配置，使用数据库值覆盖
		if s.timeoutConfig.AIRequestTimeout > 0 {
			s.timeout = time.Duration(s.timeoutConfig.AIRequestTimeout) * time.Minute
		}
	}
	if err := s.ReloadConfig(); err != nil {
		// 初始加载失败不阻塞启动，但记录日志
		return s, nil
	}
	return s, nil
}

// SetMemoryService 注入用户画像服务(用于对话时注入画像)
func (s *AIAgentService) SetMemoryService(m *UserMemoryService) {
	s.memoryService = m
}

func (s *AIAgentService) ListAvailableAgents(userID uint) ([]model.AIAgent, error) {
	var agents []model.AIAgent
	// 排编排 (agent_type='orchestration'): 它们由 Agent Studio / 训练中心单独接口提供
	if err := DB.Where("(is_public = ? OR user_id = ?) AND agent_type <> ?", true, userID, model.AIAgentTypeOrchestration).
		Order("is_public DESC, created_at DESC").
		Find(&agents).Error; err != nil {
		return nil, err
	}
	return agents, nil
}

func (s *AIAgentService) GetAIAgentByID(userID uint, agentID uint) (*model.AIAgent, error) {
	var agent model.AIAgent
	if err := DB.Where("(is_public = ? OR user_id = ?) AND id = ?", true, userID, agentID).
		First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *AIAgentService) CreateAIAgent(userID uint, req model.CreateAIAgentRequest) (*model.AIAgent, error) {
	agent := model.AIAgent{
		UserID:                userID,
		IsPublic:              false,
		Title:                 req.Title,
		Description:           req.Description,
		Code:                  req.Code,
		SystemPrompt:          req.SystemPrompt,
		Icon:                  req.Icon,
		Color:                 req.Color,
		InitialMessage:        req.InitialMessage,
		InputPlaceholder:      req.InputPlaceholder,
		SpeechLang:            req.SpeechLang,
		SpeechRate:            req.SpeechRate,
		AgentType:             normalizeAgentType(req.AgentType),
		DelegationDescription: strings.TrimSpace(req.DelegationDescription),
		// 普通 Agent 恒为启用 (enabled 列仅供编排语义使用)
		Enabled: true,
	}

	if agent.Icon == "" {
		agent.Icon = "mdi:robot-outline"
	}
	if agent.Color == "" {
		agent.Color = "#2080f0"
	}
	if agent.InputPlaceholder == "" {
		agent.InputPlaceholder = "输入消息... (回车发送，Shift + 回车换行)"
	}
	if agent.SpeechLang == "" {
		agent.SpeechLang = "zh-CN"
	}
	if agent.SpeechRate == 0 {
		agent.SpeechRate = 0.95
	}

	if err := DB.Create(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func (s *AIAgentService) UpdateAIAgent(userID uint, agentID uint, req model.UpdateAIAgentRequest) error {
	updates := map[string]interface{}{}
	if req.Title != "" {
		updates["title"] = req.Title
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.SystemPrompt != "" {
		updates["system_prompt"] = req.SystemPrompt
	}
	if req.Icon != "" {
		updates["icon"] = req.Icon
	}
	if req.Color != "" {
		updates["color"] = req.Color
	}
	if req.InitialMessage != "" {
		updates["initial_message"] = req.InitialMessage
	}
	if req.InputPlaceholder != "" {
		updates["input_placeholder"] = req.InputPlaceholder
	}
	if req.SpeechLang != "" {
		updates["speech_lang"] = req.SpeechLang
	}
	if req.SpeechRate != 0 {
		updates["speech_rate"] = req.SpeechRate
	}
	// 指针字段: 允许显式改回 chat / 清空委派说明
	if req.AgentType != nil {
		updates["agent_type"] = normalizeAgentType(*req.AgentType)
	}
	if req.DelegationDescription != nil {
		updates["delegation_description"] = strings.TrimSpace(*req.DelegationDescription)
	}

	err := DB.Model(&model.AIAgent{}).Where("id = ? AND user_id = ?", agentID, userID).Updates(updates).Error
	if err == nil {
		s.clearRunnerCache(userID, agentID)
	}
	return err
}

// normalizeAgentType 规范化 Agent 类型: 只认 subagent, 其余一律 chat
func normalizeAgentType(t string) string {
	if model.IsSubAgentType(t) {
		return model.AIAgentTypeSubAgent
	}
	return model.AIAgentTypeChat
}

func (s *AIAgentService) DeleteAIAgent(userID uint, agentID uint) error {
	err := DB.Where("id = ? AND user_id = ? AND is_public = ?", agentID, userID, false).
		Delete(&model.AIAgent{}).Error
	if err == nil {
		s.clearRunnerCache(userID, agentID)
	}
	return err
}

// clearCachesLocked 清空全部实例缓存 (调用方需持 cacheMu)。
// 模型/工具实例与 runner 同生命周期: 工具配置、Agent 提示词、模型配置变更的
// 失效路径都已汇聚到 clearRunnerCache/ClearRunnerCache/ReloadConfig
func (s *AIAgentService) clearCachesLocked() {
	s.runnerCache = make(map[string]*adk.Runner)
	s.modelCache = make(map[string]*ark.ChatModel)
	s.toolCache = make(map[string]tool.BaseTool)
}

func (s *AIAgentService) clearRunnerCache(userID uint, agentID uint) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.clearCachesLocked()
	delete(s.promptCache, fmt.Sprintf("%d_%d", userID, agentID))
}

func (s *AIAgentService) ClearRunnerCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.clearCachesLocked()
}

func (s *AIAgentService) ReloadConfig() error {
	// 超时配置一并重读: 配置页保存后 SystemConfigHandler 会触发 ReloadConfig,
	// 不重读的话改超时必须重启才生效 (查库不持锁, 结果在下方锁内合并)
	var newTimeout time.Duration
	if s.sysCfgService != nil {
		if tc := s.sysCfgService.GetTimeoutConfig(); tc.AIRequestTimeout > 0 {
			newTimeout = time.Duration(tc.AIRequestTimeout) * time.Minute
		}
	}

	var provider model.AIProvider
	if err := DB.Where("is_active = ?", true).First(&provider).Error; err != nil {
		s.cacheMu.Lock()
		s.activeProvider = nil
		s.activeModel = nil
		s.enabledModels = nil
		s.providerByID = nil
		s.clearCachesLocked()
		s.cacheMu.Unlock()
		return err
	}

	// 所有启用 Provider: 模型按 code 解析归属, 构建时用各自 Provider 的 key/baseURL
	var providers []model.AIProvider
	if err := DB.Where("is_active = ?", true).Find(&providers).Error; err != nil || len(providers) == 0 {
		providers = []model.AIProvider{provider}
	}
	providerByID := make(map[int]*model.AIProvider, len(providers))
	for i := range providers {
		providerByID[providers[i].ID] = &providers[i]
	}

	s.cacheMu.Lock()
	s.activeProvider = &provider

	var m model.AIModel
	if err := DB.Where("provider_id = ? AND is_default = ?", provider.ID, true).First(&m).Error; err != nil {
		if err := DB.Where("provider_id = ?", provider.ID).First(&m).Error; err != nil {
			s.activeModel = nil
		} else {
			s.activeModel = &m
		}
	} else {
		s.activeModel = &m
	}

	// 加载所有启用 Provider 的模型列表
	var models []model.AIModel
	providerIDs := make([]int, 0, len(providers))
	for _, p := range providers {
		providerIDs = append(providerIDs, p.ID)
	}
	if err := DB.Where("provider_id IN ?", providerIDs).Order("is_default DESC, id ASC").Find(&models).Error; err == nil {
		s.enabledModels = models
	} else {
		s.enabledModels = nil
	}
	s.providerByID = providerByID
	if newTimeout > 0 {
		s.timeout = newTimeout
	}

	s.clearCachesLocked()
	s.cacheMu.Unlock()
	return nil
}

func (s *AIAgentService) ListEnabledModels() ([]model.AIModel, error) {
	var providers []model.AIProvider
	if err := DB.Where("is_active = ?", true).Find(&providers).Error; err != nil {
		return nil, err
	}
	if len(providers) == 0 {
		return []model.AIModel{}, nil
	}
	providerIDs := make([]int, 0, len(providers))
	for _, p := range providers {
		providerIDs = append(providerIDs, p.ID)
	}
	var models []model.AIModel
	if err := DB.Where("provider_id IN ?", providerIDs).Order("is_default DESC, id ASC").Find(&models).Error; err != nil {
		return nil, err
	}
	return models, nil
}

func (s *AIAgentService) TestConnection(apiKey, baseURL, modelCode string) error {
	if apiKey == "" {
		return errors.New("API Key 不能为空")
	}
	testModel := modelCode
	if testModel == "" {
		testModel = "gpt-3.5-turbo"
	}
	chatModel, err := ark.NewChatModel(s.ctx, &ark.ChatModelConfig{
		Model:   testModel,
		APIKey:  apiKey,
		BaseURL: baseURL,
	})
	if err != nil {
		return err
	}
	testAgent, err := adk.NewChatModelAgent(s.ctx, &adk.ChatModelAgentConfig{
		Name:        "test-connection",
		Description: "test connection agent",
		Instruction: "You are a test bot. Reply OK.",
		Model:       chatModel,
	})
	if err != nil {
		return err
	}
	runner := adk.NewRunner(s.ctx, adk.RunnerConfig{
		Agent:           testAgent,
		EnableStreaming: false,
	})
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()
	iter := runner.Run(ctx, []*schema.Message{
		{Role: schema.User, Content: "Hello"},
	})
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return fmt.Errorf("模型返回错误: %w", event.Err)
		}
	}
	return nil
}

func (s *AIAgentService) buildTools() []tool.BaseTool {
	var toolsList []tool.BaseTool

	var dbTools []model.AITool
	if err := DB.Where("enabled = ?", true).Find(&dbTools).Error; err != nil {
		log.Printf("Failed to load tools from database: %v", err)
		return toolsList
	}

	for _, dbTool := range dbTools {
		t, err := tools.CreateTool(dbTool.Name, dbTool.ConfigJSON)
		if err != nil {
			log.Printf("Failed to create tool %s: %v", dbTool.Name, err)
			continue
		}
		toolsList = append(toolsList, t)
	}

	return toolsList
}

func (s *AIAgentService) getOrCreateRunner(modelOverride string) (*adk.Runner, error) {
	cacheKey := modelOverride
	s.cacheMu.RLock()
	runner, ok := s.runnerCache[cacheKey]
	s.cacheMu.RUnlock()
	if ok {
		return runner, nil
	}

	chatModel, err := s.getModel(modelOverride)
	if err != nil {
		log.Printf("Failed to get model: %v", err)
		return nil, err
	}

	handlers := []adk.ChatModelAgentMiddleware{
		NewLoggingMiddleware(),
		NewApprovalMiddleware(),
	}
	// Eino Skill Middleware: 从数据库动态加载 Skill
	if skillMW, skillErr := BuildSkillMiddleware(s.ctx, s); skillErr != nil {
		log.Printf("Failed to build skill middleware: %v", skillErr)
	} else {
		handlers = append(handlers, skillMW)
	}
	// 修补悬空 tool call(有 tool_calls 但缺 tool result 的历史)
	if patchMW, patchErr := BuildPatchToolCallsMiddleware(s.ctx); patchErr != nil {
		log.Printf("Failed to build patchtoolcalls middleware: %v", patchErr)
	} else {
		handlers = append(handlers, patchMW)
	}

	agent, err := adk.NewChatModelAgent(s.ctx, &adk.ChatModelAgentConfig{
		Name:        "default",
		Description: "AI Assistant",
		Instruction: "{custom_prompt}\n\n{user_profile}\n\n当前时间: {current_time}\n当前用户ID: {user_id}",
		Model:       chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools: s.buildTools(),
			},
		},
		Handlers: handlers,
	})
	if err != nil {
		return nil, fmt.Errorf("构建 AI Agent 失败: %w", err)
	}

	runner = adk.NewRunner(s.ctx, adk.RunnerConfig{
		Agent:           agent,
		EnableStreaming: true,
		CheckPointStore: s.checkpointStore,
	})
	s.cacheMu.Lock()
	s.runnerCache[cacheKey] = runner
	s.cacheMu.Unlock()
	return runner, nil
}

func (s *AIAgentService) getCustomPrompt(userID uint, agentID uint) string {
	cacheKey := fmt.Sprintf("%d_%d", userID, agentID)
	s.cacheMu.RLock()
	prompt, ok := s.promptCache[cacheKey]
	s.cacheMu.RUnlock()
	if ok {
		return prompt
	}

	userAgent, err := s.GetAIAgentByID(userID, agentID)
	if err != nil {
		return ""
	}
	var userPrompt model.UserPrompt
	err = DB.Where("user_id = ? AND agent_id = ? AND is_active = ?", userID, agentID, true).First(&userPrompt).Error
	if err == nil {
		prompt = userPrompt.CustomPrompt
	} else {
		prompt = userAgent.SystemPrompt
	}

	s.cacheMu.Lock()
	s.promptCache[cacheKey] = prompt
	s.cacheMu.Unlock()
	return prompt
}

func (s *AIAgentService) getModel(modelOverride string) (*ark.ChatModel, error) {
	return s.getModelWithRetry(modelOverride, nil)
}

// getModelWithRetry 构建模型实例; retryTimes 非 nil 时覆盖 ark SDK 内建的
// 模型调用重试次数 (HTTP 层, 指数退避, 仅对 5xx/429/网络错误等可重试错误生效)
func (s *AIAgentService) getModelWithRetry(modelOverride string, retryTimes *int) (*ark.ChatModel, error) {
	// 在锁内对 active*/enabled* 做快照, 防止与 ReloadConfig 并发时的 nil/竞态问题
	s.cacheMu.RLock()
	activeProvider := s.activeProvider
	activeModel := s.activeModel
	enabledModels := s.enabledModels
	providerByID := s.providerByID
	s.cacheMu.RUnlock()

	if activeProvider == nil || activeModel == nil {
		return nil, errors.New("AI 模型未配置, 请先在系统管理中启用 AI Provider")
	}

	// 指定模型时按 ai_models 的归属解析 Provider: key/baseURL 必须来自该模型
	// 所属的 Provider, 否则多 Provider 下会用默认 Provider 的 key 去调别家模型。
	// 不在启用列表中的模型直接拒绝, 不透传任意 model code
	cfgProvider := activeProvider
	cfgModel := activeModel
	modelCode := activeModel.ModelCode
	if modelOverride != "" {
		var matched *model.AIModel
		for i := range enabledModels {
			if enabledModels[i].ModelCode == modelOverride {
				matched = &enabledModels[i]
				break
			}
		}
		if matched == nil {
			return nil, fmt.Errorf("模型不存在或未启用: %s", modelOverride)
		}
		p := providerByID[matched.ProviderID]
		if p == nil {
			return nil, fmt.Errorf("模型 %s 所属的 Provider 未启用", modelOverride)
		}
		cfgProvider = p
		cfgModel = matched
		modelCode = modelOverride
	}

	// 模型实例缓存: 同一 (provider, model code, 重试次数) 复用 (构建含 HTTP client, 每节点/每次运行新建是编译期主要开销之一)
	cacheKey := fmt.Sprintf("%d|%s", cfgProvider.ID, modelCode)
	if retryTimes != nil {
		cacheKey = fmt.Sprintf("%s|retry%d", cacheKey, *retryTimes)
	}
	s.cacheMu.RLock()
	if cm, ok := s.modelCache[cacheKey]; ok {
		s.cacheMu.RUnlock()
		return cm, nil
	}
	s.cacheMu.RUnlock()

	chatConfig := &ark.ChatModelConfig{
		Model:   modelCode,
		APIKey:  cfgProvider.APIKey,
		BaseURL: cfgProvider.BaseURL,
	}
	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(cfgModel.ConfigJSON), &configMap); err == nil {
		if t, ok := configMap["temperature"].(float64); ok {
			temperature := float32(t)
			chatConfig.Temperature = &temperature
		}
		if topP, ok := configMap["top_p"].(float64); ok {
			topP := float32(topP)
			chatConfig.TopP = &topP
		}
		if maxTokens, ok := configMap["max_tokens"].(float64); ok {
			maxTokens := int(maxTokens)
			chatConfig.MaxTokens = &maxTokens
		}
		if frequencyPenalty, ok := configMap["frequency_penalty"].(float64); ok {
			frequencyPenalty := float32(frequencyPenalty)
			chatConfig.FrequencyPenalty = &frequencyPenalty
		}
		if presencePenalty, ok := configMap["presence_penalty"].(float64); ok {
			presencePenalty := float32(presencePenalty)
			chatConfig.PresencePenalty = &presencePenalty
		}
	} else {
		log.Printf("AI model config_json parse failed model=%s config_json=%s err=%v", modelCode, cfgModel.ConfigJSON, err)
	}
	if retryTimes != nil {
		rt := *retryTimes
		chatConfig.RetryTimes = &rt
	}
	chatModel, err := ark.NewChatModel(s.ctx, chatConfig)
	if err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	if s.modelCache == nil {
		s.modelCache = make(map[string]*ark.ChatModel)
	}
	s.modelCache[cacheKey] = chatModel
	s.cacheMu.Unlock()
	return chatModel, nil
}

// GetToolCallingModel 按 model code 构建可工具调用的模型 (编排/技能等复用), 空串用默认模型
func (s *AIAgentService) GetToolCallingModel(modelOverride string) (einomodel.ToolCallingChatModel, error) {
	return s.getModel(modelOverride)
}

// GetToolCallingModelWithRetry 同 GetToolCallingModel, 但可覆盖 ark SDK 内建重试次数
// (retryTimes=nil 沿用默认 2 次; 0 表示不重试, 正数为指定重试次数)
func (s *AIAgentService) GetToolCallingModelWithRetry(modelOverride string, retryTimes *int) (einomodel.ToolCallingChatModel, error) {
	return s.getModelWithRetry(modelOverride, retryTimes)
}

// BuildToolByName 按 ai_tools 表配置构建单个工具实例 (带缓存: 工具实现无状态可复用,
// 配置/启用状态变更经 ClearRunnerCache 失效)
func (s *AIAgentService) BuildToolByName(name string) (tool.BaseTool, error) {
	s.cacheMu.RLock()
	if t, ok := s.toolCache[name]; ok {
		s.cacheMu.RUnlock()
		return t, nil
	}
	s.cacheMu.RUnlock()
	var dbTool model.AITool
	if err := DB.Where("name = ? AND enabled = ?", name, true).First(&dbTool).Error; err != nil {
		return nil, fmt.Errorf("工具不存在或未启用: %s", name)
	}
	t, err := tools.CreateTool(dbTool.Name, dbTool.ConfigJSON)
	if err != nil {
		return nil, err
	}
	s.cacheMu.Lock()
	if s.toolCache == nil {
		s.toolCache = make(map[string]tool.BaseTool)
	}
	s.toolCache[name] = t
	s.cacheMu.Unlock()
	return t, nil
}

// SessionTemplateVars 模板/系统提示词可用的变量
func (s *AIAgentService) SessionTemplateVars(userID uint) map[string]any {
	vars := map[string]any{
		"current_time": time.Now().Format("2006-01-02 15:04:05"),
		"user_id":      userID,
	}
	if s.memoryService != nil {
		vars["user_profile"] = s.memoryService.BuildProfilePrompt(userID)
	}
	return vars
}

// HasActiveModel 是否已配置默认模型 (锁内快照读, 供后台任务做前置检查)
func (s *AIAgentService) HasActiveModel() bool {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return s.activeProvider != nil && s.activeModel != nil
}

// ResolveModelCode 返回 modelOverride 实际对应的模型 code: override 非空返回自身,
// 空串解析为当前默认模型的 code (即"空串=默认模型"的真实身份)。
// 与 getModelWithRetry 同一套解析规则; 供 token 记账等需要真实模型归属的场合使用
func (s *AIAgentService) ResolveModelCode(modelOverride string) string {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	if modelOverride != "" {
		return modelOverride
	}
	if s.activeModel != nil {
		return s.activeModel.ModelCode
	}
	return ""
}

// getTimeout 锁内快照读当前 AI 请求超时
func (s *AIAgentService) getTimeout() time.Duration {
	s.cacheMu.RLock()
	defer s.cacheMu.RUnlock()
	return s.timeout
}

// GenerateText 非流式生成, 供画像/经历抽取等后台任务复用当前模型配置。
// userID>0 时把本次调用的 token 用量记账到该用户 (此前抽取链路漏记, 限额统计偏低)
func (s *AIAgentService) GenerateText(userID uint, modelOverride, systemPrompt, userPrompt string) (string, error) {
	chatModel, err := s.getModel(modelOverride)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(s.ctx, s.getTimeout())
	defer cancel()
	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	})
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("模型没有返回结果")
	}
	if u := resp.ResponseMeta; u != nil && u.Usage != nil && u.Usage.TotalTokens > 0 && userID > 0 {
		RecordTokenUsage(userID, s.ResolveModelCode(modelOverride), model.TokenSourceExtraction, int64(u.Usage.PromptTokens), int64(u.Usage.CompletionTokens))
	}
	return resp.Content, nil
}

// ChatStream 流式对话。ctx 必须传请求的 ctx: 客户端断开即取消底层运行, 不再白烧
// token; 整次运行受 AI 请求超时约束 (与编排 DebugRun 一致)。返回的 cancel 须在
// SSE 流结束后调用 (handler defer)
func (s *AIAgentService) ChatStream(ctx context.Context, userID uint, agentID uint, historyID uint, messages []*schema.Message, modelOverride string) (*adk.AsyncIterator[*adk.AgentEvent], context.CancelFunc, error) {
	runner, err := s.getOrCreateRunner(modelOverride)
	if err != nil {
		return nil, nil, err
	}

	customPrompt := s.getCustomPrompt(userID, agentID)

	userProfile := ""
	if s.memoryService != nil {
		userProfile = s.memoryService.BuildProfilePrompt(userID)
	}

	checkPointID := fmt.Sprintf("%d_%d", userID, historyID)
	runCtx, cancel := context.WithTimeout(ctx, s.getTimeout())
	iter := runner.Run(runCtx, messages, adk.WithCheckPointID(checkPointID), adk.WithSessionValues(map[string]any{
		"custom_prompt": customPrompt,
		"user_profile":  userProfile,
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"user_id":       userID,
	}))
	return iter, cancel, nil
}

// ResumeToolApproval 恢复被工具审批中断的对话。modelOverride 传发起那次对话时
// 使用的模型 (前端随审批请求带回), 否则会落到默认 runner, 非默认模型对话恢复后
// 模型错乱
func (s *AIAgentService) ResumeToolApproval(ctx context.Context, checkPointID string, interruptID string, approved bool, reason string, modelOverride string) (*adk.AsyncIterator[*adk.AgentEvent], context.CancelFunc, error) {
	runner, err := s.getOrCreateRunner(modelOverride)
	if err != nil {
		return nil, nil, err
	}

	result := &ToolApprovalResult{
		Approved: approved,
		Reason:   reason,
	}

	runCtx, cancel := context.WithTimeout(ctx, s.getTimeout())
	iter, err := runner.ResumeWithParams(runCtx, checkPointID, &adk.ResumeParams{
		Targets: map[string]any{
			interruptID: result,
		},
	})
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return iter, cancel, nil
}
