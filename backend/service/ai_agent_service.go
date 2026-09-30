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
	ctx             context.Context
	activeProvider  *model.AIProvider
	activeModel     *model.AIModel
	enabledModels   []model.AIModel
	timeout         time.Duration
	timeoutConfig   TimeoutConfig
	runnerCache     map[string]*adk.Runner
	promptCache     map[string]string
	cacheMu         sync.RWMutex // 保护 runnerCache/promptCache 及 active* 字段, gin 请求与 ReloadConfig 并发访问
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
	if err := DB.Where("is_public = ? OR user_id = ?", true, userID).
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

func (s *AIAgentService) clearRunnerCache(userID uint, agentID uint) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.runnerCache = make(map[string]*adk.Runner)
	delete(s.promptCache, fmt.Sprintf("%d_%d", userID, agentID))
}

func (s *AIAgentService) ClearRunnerCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.runnerCache = make(map[string]*adk.Runner)
}

func (s *AIAgentService) ReloadConfig() error {
	var provider model.AIProvider
	if err := DB.Where("is_active = ?", true).First(&provider).Error; err != nil {
		s.cacheMu.Lock()
		s.activeProvider = nil
		s.activeModel = nil
		s.enabledModels = nil
		s.cacheMu.Unlock()
		return err
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
	var providers []model.AIProvider
	if err := DB.Where("is_active = ?", true).Find(&providers).Error; err == nil {
		providerIDs := make([]int, 0, len(providers))
		for _, p := range providers {
			providerIDs = append(providerIDs, p.ID)
		}
		var models []model.AIModel
		if err := DB.Where("provider_id IN ?", providerIDs).Order("is_default DESC, id ASC").Find(&models).Error; err == nil {
			s.enabledModels = models
		}
	}

	s.runnerCache = make(map[string]*adk.Runner)
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
	// 在锁内对 active* 做快照, 防止与 ReloadConfig 并发时的 nil/竞态问题
	s.cacheMu.RLock()
	activeProvider := s.activeProvider
	activeModel := s.activeModel
	s.cacheMu.RUnlock()

	if activeProvider == nil || activeModel == nil {
		return nil, errors.New("AI 模型未配置, 请先在系统管理中启用 AI Provider")
	}

	modelCode := activeModel.ModelCode
	if modelOverride != "" {
		modelCode = modelOverride
	}
	chatConfig := &ark.ChatModelConfig{
		Model:   modelCode,
		APIKey:  activeProvider.APIKey,
		BaseURL: activeProvider.BaseURL,
	}
	var configMap map[string]interface{}
	if err := json.Unmarshal([]byte(activeModel.ConfigJSON), &configMap); err == nil {
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
		log.Printf("AI model config_json parse failed model=%s config_json=%s err=%v", activeModel.ModelCode, activeModel.ConfigJSON, err)
	}
	return ark.NewChatModel(s.ctx, chatConfig)
}

// GetToolCallingModel 按 model code 构建可工具调用的模型 (编排/技能等复用), 空串用默认模型
func (s *AIAgentService) GetToolCallingModel(modelOverride string) (einomodel.ToolCallingChatModel, error) {
	return s.getModel(modelOverride)
}

// BuildToolByName 按 ai_tools 表配置构建单个工具实例
func (s *AIAgentService) BuildToolByName(name string) (tool.BaseTool, error) {
	var dbTool model.AITool
	if err := DB.Where("name = ? AND enabled = ?", name, true).First(&dbTool).Error; err != nil {
		return nil, fmt.Errorf("工具不存在或未启用: %s", name)
	}
	return tools.CreateTool(dbTool.Name, dbTool.ConfigJSON)
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

// GenerateText 非流式生成, 供画像/经历抽取等后台任务复用当前模型配置
func (s *AIAgentService) GenerateText(modelOverride, systemPrompt, userPrompt string) (string, error) {
	if s.activeProvider == nil || s.activeModel == nil {
		return "", errors.New("AI 模型未配置")
	}
	chatModel, err := s.getModel(modelOverride)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(s.ctx, s.timeout)
	defer cancel()
	resp, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: systemPrompt},
		{Role: schema.User, Content: userPrompt},
	})
	if err != nil {
		return "", err
	}
	return resp.Content, nil
}

func (s *AIAgentService) ChatStream(userID uint, agentID uint, historyID uint, messages []*schema.Message, modelOverride string) (*adk.AsyncIterator[*adk.AgentEvent], error) {
	runner, err := s.getOrCreateRunner(modelOverride)
	if err != nil {
		return nil, err
	}

	customPrompt := s.getCustomPrompt(userID, agentID)

	userProfile := ""
	if s.memoryService != nil {
		userProfile = s.memoryService.BuildProfilePrompt(userID)
	}

	checkPointID := fmt.Sprintf("%d_%d", userID, historyID)
	return runner.Run(s.ctx, messages, adk.WithCheckPointID(checkPointID), adk.WithSessionValues(map[string]any{
		"custom_prompt": customPrompt,
		"user_profile":  userProfile,
		"current_time":  time.Now().Format("2006-01-02 15:04:05"),
		"user_id":       userID,
	})), nil
}

func (s *AIAgentService) ResumeToolApproval(checkPointID string, interruptID string, approved bool, reason string) (*adk.AsyncIterator[*adk.AgentEvent], error) {
	runner, err := s.getOrCreateRunner("")
	if err != nil {
		return nil, err
	}

	result := &ToolApprovalResult{
		Approved: approved,
		Reason:   reason,
	}

	return runner.ResumeWithParams(s.ctx, checkPointID, &adk.ResumeParams{
		Targets: map[string]any{
			interruptID: result,
		},
	})
}
