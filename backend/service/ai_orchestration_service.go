package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"backend/model"
	"backend/service/tools"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// AIOrchestrationService Agent Studio 编排管理: CRUD + DSL 校验 + 调试运行
type AIOrchestrationService struct {
	agentService  *AIAgentService
	promptService *PromptService
	timeout       time.Duration
}

func NewAIOrchestrationService(agentService *AIAgentService, promptService *PromptService, timeoutMinutes int) *AIOrchestrationService {
	if timeoutMinutes <= 0 {
		timeoutMinutes = 5
	}
	return &AIOrchestrationService{
		agentService:  agentService,
		promptService: promptService,
		timeout:       time.Duration(timeoutMinutes) * time.Minute,
	}
}

// orchToDTO 把 ai_agents 中的编排行映射为 API 视图
func orchToDTO(a *model.AIAgent) model.AIOrchestration {
	return model.AIOrchestration{
		ID:               int(a.ID),
		Name:             a.Title,
		Description:      a.Description,
		Definition:       a.Definition,
		Version:          a.Version,
		Enabled:          a.Enabled,
		LastDebugSummary: a.LastDebugSummary,
		CreatedAt:        a.CreatedAt,
		UpdatedAt:        a.UpdatedAt,
	}
}

func (s *AIOrchestrationService) List() ([]model.AIOrchestration, error) {
	var rows []model.AIAgent
	if err := DB.Where("agent_type = ?", model.AIAgentTypeOrchestration).
		Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]model.AIOrchestration, 0, len(rows))
	for i := range rows {
		list = append(list, orchToDTO(&rows[i]))
	}
	return list, nil
}

// ListEnabled 仅返回已启用的编排, 供训练中心"编排对话"列表使用
func (s *AIOrchestrationService) ListEnabled() ([]model.AIOrchestration, error) {
	var rows []model.AIAgent
	if err := DB.Where("agent_type = ? AND enabled = ?", model.AIAgentTypeOrchestration, true).
		Order("updated_at DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	list := make([]model.AIOrchestration, 0, len(rows))
	for i := range rows {
		list = append(list, orchToDTO(&rows[i]))
	}
	return list, nil
}

func (s *AIOrchestrationService) Get(id uint) (*model.AIOrchestration, error) {
	var agent model.AIAgent
	if err := DB.Where("agent_type = ? AND id = ?", model.AIAgentTypeOrchestration, id).
		First(&agent).Error; err != nil {
		return nil, err
	}
	dto := orchToDTO(&agent)
	return &dto, nil
}

func (s *AIOrchestrationService) Create(req *model.CreateAIOrchestrationRequest) (*model.AIOrchestration, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("名称不能为空")
	}
	if errs := s.validateDefinition(req.Definition, 0); len(errs) > 0 {
		return nil, errors.New("编排定义校验失败: " + strings.Join(errs, "; "))
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	// 编排并入 ai_agents 表, 复用该表的非空/唯一约束列:
	// title=编排名称, code 用内部唯一占位值 (编排不走 code 语义), system_prompt 置空
	agent := model.AIAgent{
		IsPublic:     true,
		Title:        req.Name,
		Description:  req.Description,
		Code:         fmt.Sprintf("orchestration_%d", time.Now().UnixNano()),
		SystemPrompt: "",
		Icon:         "mdi:graph-outline",
		Color:        "#7c3aed",
		AgentType:    model.AIAgentTypeOrchestration,
		Definition:   req.Definition,
		Version:      1,
		Enabled:      enabled,
	}
	if err := DB.Create(&agent).Error; err != nil {
		return nil, err
	}
	dto := orchToDTO(&agent)
	return &dto, nil
}

func (s *AIOrchestrationService) Update(id uint, req *model.UpdateAIOrchestrationRequest) error {
	var agent model.AIAgent
	if err := DB.Where("agent_type = ? AND id = ?", model.AIAgentTypeOrchestration, id).
		First(&agent).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if req.Name != nil && *req.Name != "" {
		updates["title"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Definition != nil {
		if errs := s.validateDefinition(*req.Definition, id); len(errs) > 0 {
			return errors.New("编排定义校验失败: " + strings.Join(errs, "; "))
		}
		updates["definition"] = *req.Definition
		updates["version"] = agent.Version + 1
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Model(&model.AIAgent{}).
		Where("agent_type = ? AND id = ?", model.AIAgentTypeOrchestration, id).
		Updates(updates).Error
}

func (s *AIOrchestrationService) Delete(id uint) error {
	return DB.Where("agent_type = ? AND id = ?", model.AIAgentTypeOrchestration, id).
		Delete(&model.AIAgent{}).Error
}

// OrchestrationValidationIssue 单条校验错误: NodeID 为空表示全局性错误, 非空可定位画布节点
type OrchestrationValidationIssue struct {
	NodeID  string `json:"node_id,omitempty"`
	Message string `json:"message"`
}

// OrchestrationValidationResult 校验结果 (含编译探测到的错误)
type OrchestrationValidationResult struct {
	Valid    bool     `json:"valid"`
	Mode     string   `json:"mode"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	// ErrorItems 结构化错误 (含节点定位): 前端据此高亮/选中画布上的出错节点
	ErrorItems []OrchestrationValidationIssue `json:"error_items,omitempty"`
}

// Validate 校验编排定义: 结构校验 + 完整编译(捕获类型/构建错误)
func (s *AIOrchestrationService) Validate(definition string, userID uint) *OrchestrationValidationResult {
	result := &OrchestrationValidationResult{}
	dsl, issues := validateOrchestrationDSLDetailed(definition)
	if len(issues) > 0 {
		result.Valid = false
		result.Errors = orchIssueMessages(issues)
		result.ErrorItems = orchIssueDTOs(issues)
		return result
	}
	// 结构合法, 尝试完整编译捕获构建期错误
	compiled, err := s.compile(context.Background(), userID, dsl, nil, nil, "", 0, nil, false)
	if err != nil {
		result.Valid = false
		result.Errors = append(result.Errors, err.Error())
		return result
	}
	result.Valid = true
	result.Mode = compiled.mode
	result.Warnings = orchCollectWarnings(dsl)
	return result
}

// orchIssueMessages / orchIssueDTOs 校验错误列表的两种形态: 文案 (兼容旧消费方) 与带节点定位的结构
func orchIssueMessages(issues []orchValidateIssue) []string {
	out := make([]string, 0, len(issues))
	for _, is := range issues {
		out = append(out, is.Message)
	}
	return out
}

func orchIssueDTOs(issues []orchValidateIssue) []OrchestrationValidationIssue {
	out := make([]OrchestrationValidationIssue, 0, len(issues))
	for _, is := range issues {
		out = append(out, OrchestrationValidationIssue{NodeID: is.NodeID, Message: is.Message})
	}
	return out
}

func orchCollectWarnings(dsl *OrchestrationDSL) []string {
	var warnings []string
	for _, n := range dsl.Nodes {
		switch n.Type {
		case OrchNodeAgent:
			var cfg OrchAgentConfig
			if len(n.Config) > 0 && json.Unmarshal(n.Config, &cfg) == nil {
				if strings.TrimSpace(cfg.SystemPrompt) == "" {
					warnings = append(warnings, fmt.Sprintf("Agent 节点 %s 未设置系统提示词", n.ID))
				}
				for _, toolName := range cfg.Tools {
					var dbTool model.AITool
					if err := DB.Where("name = ? AND enabled = ?", toolName, true).First(&dbTool).Error; err == nil && dbTool.ConfirmRequired {
						warnings = append(warnings, fmt.Sprintf("Agent 节点 %s 的工具 %s 需要人工确认, 调试运行不经过工具审批, 将直接执行", n.ID, toolName))
					}
				}
			}
		case OrchNodeSubAgent:
			var cfg OrchSubAgentConfig
			if len(n.Config) > 0 && json.Unmarshal(n.Config, &cfg) != nil {
				continue
			}
			// 委派说明是主 Agent 判断"何时该委派"的唯一依据: 为空时模型几乎不会主动调用该子Agent
			if strings.TrimSpace(cfg.Description) == "" {
				desc := ""
				if cfg.AgentID > 0 {
					var agent model.AIAgent
					if err := DB.Select("description").First(&agent, cfg.AgentID).Error; err == nil {
						desc = strings.TrimSpace(agent.Description)
					}
				}
				if desc == "" {
					warnings = append(warnings, fmt.Sprintf(
						"子Agent 节点 %s 未填写「委派说明」(且被引用 Agent 的简介也为空), 主 Agent 只能凭节点名称判断是否委派, 很可能一直不调用它", n.ID))
				}
			}
		case OrchNodeRouter:
			var cfg OrchRouterConfig
			if len(n.Config) > 0 && json.Unmarshal(n.Config, &cfg) == nil {
				if strings.TrimSpace(cfg.Instructions) == "" {
					warnings = append(warnings, fmt.Sprintf(
						"LLM 路由节点 %s 未填写判定规则, 模型只能凭标签名称分类, 相近意图容易误判", n.ID))
				}
			}
		case OrchNodeTemplate:
			var cfg OrchTemplateConfig
			if len(n.Config) > 0 && json.Unmarshal(n.Config, &cfg) == nil {
				// 入口模板没引用 {{.Input}} 时, 用户输入在入口就被丢弃:
				// 模型只能看到模板里的静态文案, 对话必然答非所问
				if !orchHasUpstreamEdge(dsl, n.ID) && !strings.Contains(cfg.Template, ".Input") {
					warnings = append(warnings, fmt.Sprintf(
						"入口模板节点 %s 未引用 {{.Input}}, 用户的输入不会进入模型 (对话会答非所问), 请在模板中引用 {{.Input}} 透传用户消息", n.ID))
				}
			}
		}
	}
	return warnings
}

// orchHasUpstreamEdge 判断节点是否被任何连线指向 (无入边 = 入口节点)
func orchHasUpstreamEdge(dsl *OrchestrationDSL, nodeID string) bool {
	for _, e := range dsl.Edges {
		if e.Target == nodeID {
			return true
		}
	}
	return false
}

// orchNodeKeysOf 编排节点 key -> 显示名 (含子Agent 节点): 调试事件的归属表,
// 必须在编译前拿到 (委派工具构建时需要事件通道)
func orchNodeKeysOf(dsl *OrchestrationDSL) map[string]string {
	keys := make(map[string]string, len(dsl.Nodes))
	for i := range dsl.Nodes {
		n := &dsl.Nodes[i]
		if n.ID == "" {
			continue
		}
		name := n.Name
		if name == "" {
			name = n.ID
		}
		keys[n.ID] = name
	}
	return keys
}

// validateDefinition 完整校验(结构+编译), 返回错误列表;
// selfID 是被校验编排自己的 id (草稿/新建传 0), 用于保存时检出子编排自引用
func (s *AIOrchestrationService) validateDefinition(definition string, selfID uint) []string {
	dsl, issues := validateOrchestrationDSLDetailed(definition)
	if len(issues) > 0 {
		return orchIssueMessages(issues)
	}
	if _, err := s.compile(context.Background(), 0, dsl, nil, nil, "", selfID, nil, false); err != nil {
		return []string{err.Error()}
	}
	return nil
}

// wrapOrchestrationTool 构建编排运行用的工具实例: 需人工确认 (confirm_required) 的
// 工具包一层审批 (经 SSE 请求人工批准, 拒绝/超时把原因返回给模型), 其余直接复用
func (s *AIOrchestrationService) wrapOrchestrationTool(name string) (tool.BaseTool, error) {
	t, err := s.agentService.BuildToolByName(name)
	if err != nil {
		return nil, err
	}
	if isConfirmRequired(name) {
		if invokable, ok := t.(tool.InvokableTool); ok {
			return &orchApprovalTool{name: name, inner: invokable}, nil
		}
	}
	return t, nil
}

// compile 编译编排 DSL。orchID 是编排自身 id (草稿为 0), chain 是编译链上层的
// 编排 id: 子编排节点引用其他编排时递归编译, 依据二者做循环引用检测。
// chatMode 标记编排对话运行: 入口模板未引用 {{.Input}} 时兜底补入用户输入。
func (s *AIOrchestrationService) compile(ctx context.Context, userID uint, dsl *OrchestrationDSL, trace *orchTraceHandler, history []*schema.Message, chatPreamble string, orchID uint, chain []uint, chatMode bool) (*compiledOrchestration, error) {
	// 编译链含自身: 子编排引用链上的任一编排 (含自己) 都算循环引用
	orchChain := chain
	if orchID > 0 {
		orchChain = append(append([]uint{}, chain...), orchID)
	}
	deps := compilerDeps{
		getModel: s.agentService.GetToolCallingModel,
		// 节点可覆盖 ark SDK 内建的模型调用重试次数 (Agent/子Agent 节点的 max_retries)
		getModelRetry:    s.agentService.GetToolCallingModelWithRetry,
		resolveModelCode: s.agentService.ResolveModelCode,
		buildTool:        s.wrapOrchestrationTool,
		sessionVars: func() map[string]any {
			vars := s.agentService.SessionTemplateVars(userID)
			// 多轮调试的历史经 sessionVars 传给编译器 (模板变量不受影响)
			if len(history) > 0 {
				vars[orchSessionVarsHistoryKey] = history
			}
			return vars
		},
		lookupAgent: func(id uint) (*model.AIAgent, error) {
			var agent model.AIAgent
			if err := DB.First(&agent, id).Error; err != nil {
				return nil, err
			}
			return &agent, nil
		},
		compileNested: func(nestedCtx context.Context, refID uint, nestedChain []uint, keyPrefix string) (*compiledOrchestration, error) {
			return s.compileNested(nestedCtx, userID, refID, trace, nestedChain, keyPrefix)
		},
		nodePrompt: func(nodeKey string) (string, bool) {
			// 编排 Agent 节点的提示词版本: user_prompts 表, 键 = 编排 id + 节点 id,
			// user_id 恒为 0 (编排全局共享, 不区分用户)。草稿 (orchID=0) 无版本可解析,
			// 回退画布内联提示词。经 PromptService 读 (带缓存, 版本变更即失效);
			// promptService 未注入时 (旧装配/单测) 回退直查
			if orchID == 0 {
				return "", false
			}
			if s.promptService != nil {
				return s.promptService.GetOrchNodePrompt(orchID, nodeKey)
			}
			var up model.UserPrompt
			if err := DB.Where("user_id = ? AND agent_id = ? AND node_key = ? AND is_active = ?", 0, orchID, nodeKey, true).
				First(&up).Error; err != nil {
				return "", false
			}
			if strings.TrimSpace(up.CustomPrompt) == "" {
				return "", false
			}
			return up.CustomPrompt, true
		},
	}
	c := &orchestrationCompiler{dsl: dsl, trace: trace, chatPreamble: chatPreamble, chatMode: chatMode, orchChain: orchChain, userID: userID}
	return c.compile(ctx, deps)
}

// compileNested 编译子编排节点引用的已保存编排:
// 查库 -> 校验 -> 节点 id 加前缀隔离节点空间 -> 递归 compile (嵌套编排不注入对话历史与身份前言)
func (s *AIOrchestrationService) compileNested(ctx context.Context, userID uint, refID uint, trace *orchTraceHandler, chain []uint, keyPrefix string) (*compiledOrchestration, error) {
	var agent model.AIAgent
	if err := DB.Where("agent_type = ? AND id = ?", model.AIAgentTypeOrchestration, refID).
		First(&agent).Error; err != nil {
		return nil, fmt.Errorf("引用的编排 %d 不存在", refID)
	}
	if !agent.Enabled {
		return nil, fmt.Errorf("子编排「%s」(%d) 未启用", agent.Title, refID)
	}
	dsl, issues := validateOrchestrationDSLDetailed(agent.Definition)
	if len(issues) > 0 {
		return nil, fmt.Errorf("子编排「%s」(%d) 定义校验失败: %s", agent.Title, refID, strings.Join(orchIssueMessages(issues), "; "))
	}
	return s.compile(ctx, userID, prefixOrchestrationDSL(dsl, keyPrefix), trace, nil, "", refID, chain, false)
}

// Resources 画布可用资源: 工具/模型/Agent/Skill
func (s *AIOrchestrationService) Resources() (map[string]any, error) {
	var dbTools []model.AITool
	if err := DB.Where("enabled = ?", true).Order("name ASC").Find(&dbTools).Error; err != nil {
		return nil, err
	}
	toolsRes := make([]map[string]any, 0, len(dbTools))
	for _, t := range dbTools {
		toolsRes = append(toolsRes, map[string]any{
			"name":             t.Name,
			"display_name":     t.DisplayName,
			"description":      t.Description,
			"confirm_required": t.ConfirmRequired,
		})
	}

	models, err := s.agentService.ListEnabledModels()
	if err != nil {
		return nil, err
	}
	modelsRes := make([]map[string]any, 0, len(models))
	for _, m := range models {
		modelsRes = append(modelsRes, map[string]any{
			"model_code":   m.ModelCode,
			"display_name": m.DisplayName,
			"is_default":   m.IsDefault,
		})
	}

	// 子Agent 下拉只列"允许出现在编排里"的 Agent: agent_type='subagent'
	var agents []model.AIAgent
	if err := DB.Where("agent_type = ?", model.AIAgentTypeSubAgent).
		Order("is_public DESC, created_at DESC").Limit(200).Find(&agents).Error; err != nil {
		return nil, err
	}
	agentsRes := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		agentsRes = append(agentsRes, map[string]any{
			"id":                     a.ID,
			"title":                  a.Title,
			"description":            a.Description,
			"system_prompt":          a.SystemPrompt,
			"agent_type":             a.AgentType,
			"delegation_description": a.DelegationDescription,
		})
	}

	var skills []model.AISkill
	if err := DB.Where("enabled = ?", true).Select("name, description").Order("name ASC").Find(&skills).Error; err != nil {
		return nil, err
	}
	skillsRes := make([]map[string]any, 0, len(skills))
	for _, sk := range skills {
		skillsRes = append(skillsRes, map[string]any{"name": sk.Name, "description": sk.Description})
	}

	// 子编排节点可引用的已保存编排 (含未启用的: 下拉可见, 编译/运行时报未启用)
	var orchs []model.AIAgent
	if err := DB.Where("agent_type = ?", model.AIAgentTypeOrchestration).
		Select("id, title, description, enabled").
		Order("updated_at DESC").Limit(200).Find(&orchs).Error; err != nil {
		return nil, err
	}
	orchsRes := make([]map[string]any, 0, len(orchs))
	for _, o := range orchs {
		orchsRes = append(orchsRes, map[string]any{
			"id":          o.ID,
			"name":        o.Title,
			"description": o.Description,
			"enabled":     o.Enabled,
		})
	}

	return map[string]any{
		"tools":          toolsRes,
		"models":         modelsRes,
		"agents":         agentsRes,
		"skills":         skillsRes,
		"orchestrations": orchsRes,
		"node_types": []map[string]string{
			{"type": OrchNodeAgent, "label": "Agent", "desc": "LLM + 工具 ReAct 执行"},
			{"type": OrchNodeTool, "label": "工具", "desc": "独立调用一个已启用工具"},
			{"type": OrchNodeTemplate, "label": "模板", "desc": "组装/改写上游内容为用户消息"},
			{"type": OrchNodeBranch, "label": "分支", "desc": "按条件路由到不同下游; 从分支连回上游构成受控循环 (虚线回边, 需设置循环上限)"},
			{"type": OrchNodeRouter, "label": "LLM路由", "desc": "由模型对上游内容做意图分类, 按分类路由到不同下游; 同样支持循环回边"},
			{"type": OrchNodeExtract, "label": "字段提取", "desc": "从上游 JSON 内容按字段路径抽取文本"},
			{"type": OrchNodeMerge, "label": "合并", "desc": "汇聚多路上游输出"},
			{"type": OrchNodeEnd, "label": "结束", "desc": "输出最终结果"},
			{"type": OrchNodeSubAgent, "label": "子Agent", "desc": "挂载在主 Agent 下的委派子 Agent (主 Agent -> 子Agent 连线), 运行时由主 Agent 按需调用"},
			{"type": OrchNodeSubOrch, "label": "子编排", "desc": "引用另一个已保存编排作为节点执行"},
		},
	}, nil
}

// DebugRunResult 调试运行的最终摘要
type DebugRunResult struct {
	Mode    string             `json:"mode"`
	Output  string             `json:"output"`
	Nodes   []*OrchNodeTrace   `json:"nodes"`
	TotalMS int64              `json:"total_ms"`
	Tokens  *schema.TokenUsage `json:"tokens,omitempty"`
}

// DebugRun 调试执行一次编排, 通过 emit 推送 SSE 事件:
// start / delta / reasoning / node / summary / error / done
// 多轮: req.History 是之前轮次, req.Input 是本轮输入
func (s *AIOrchestrationService) DebugRun(ctx context.Context, userID uint, req *model.DebugRunRequest, emit func(event string, payload any)) error {
	definition := strings.TrimSpace(req.Definition)
	var orchID uint
	if definition == "" {
		if req.ID == nil || *req.ID == 0 {
			emit("error", map[string]any{"message": "请提供编排 ID 或画布草稿定义"})
			emit("done", map[string]any{})
			return nil
		}
		orch, err := s.Get(uint(*req.ID))
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				emit("error", map[string]any{"message": "编排不存在"})
			} else {
				emit("error", map[string]any{"message": "读取编排失败: " + err.Error()})
			}
			emit("done", map[string]any{})
			return nil
		}
		definition = orch.Definition
		orchID = uint(orch.ID)
	} else if req.ID != nil {
		orchID = uint(*req.ID)
	}

	started := time.Now()
	history := OrchChatTurnsToMessages(req.History)
	runTag := fmt.Sprintf("orch=%d user=%d", orchID, userID)
	orchLog("%s 开始调试运行: 历史轮次=%d 本轮输入=%.60q 定义长度=%d", runTag, len(history), req.Input, len(definition))

	// 月度 Token 限额: 入口前置校验 (画布调试与编排对话都走这里), 超限直接拒绝
	if err := NewTokenUsageService().CheckTokenQuota(userID); err != nil {
		orchLog("%s 限额拦截: %v", runTag, err)
		emit("error", map[string]any{"message": err.Error()})
		emit("done", map[string]any{})
		return nil
	}

	// 编排对话模式: 用编排名称/简介构造身份前言, 注入主 Agent 系统提示词。
	// 这段内容此前只存在于前端欢迎气泡里, 模型从未见过, 首轮对话便"不知道自己是谁"。
	chatPreamble := ""
	if req.ChatMode {
		if orch, err := s.Get(orchID); err == nil {
			chatPreamble = orchChatPreamble(orch.Name, orch.Description)
		}
	}

	dsl, issues := validateOrchestrationDSLDetailed(definition)
	if len(issues) > 0 {
		errs := orchIssueMessages(issues)
		orchLog("%s 校验失败: %v", runTag, errs)
		emit("error", map[string]any{
			"message":     "编排定义校验失败",
			"errors":      errs,
			"error_items": orchIssueDTOs(issues),
		})
		emit("done", map[string]any{})
		return nil
	}

	// 追踪 handler 必须在编译前建立: 委派工具在编译期构建并持有该实例,
	// 事件出口 (emit) 等编译成功后再注入, 避免编译期事件写到已关闭的 SSE
	handler := newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)

	// 本次运行唯一标识: 工具审批的请求/决定靠它配对, 随 start 事件下发
	runID := orchNewRunID()
	runTag = fmt.Sprintf("%s run=%s", runTag, runID)

	compiled, err := s.compile(ctx, userID, dsl, handler, history, chatPreamble, orchID, nil, req.ChatMode)
	if err != nil {
		orchLog("%s 编译失败: %v", runTag, err)
		emit("error", map[string]any{"message": err.Error()})
		emit("done", map[string]any{})
		return nil
	}
	// 子编排节点编译时展开了嵌套编排: 把前缀化的节点 key/子Agent 集合并入归属表,
	// 嵌套节点的事件才能正确进摘要 (在 setEmit 前完成, 事件出口尚未开启)
	handler.mergeCompiled(compiled.nodeKeys, compiled.subNodes)
	orchLog("%s 编译完成: mode=%s 节点=%d 主流节点=%d", runTag, compiled.mode, len(dsl.Nodes), len(compiled.nodeKeys))

	emit("start", map[string]any{
		"mode":       compiled.mode,
		"node_count": len(dsl.Nodes),
		"history":    len(history),
		"run_id":     runID,
	})
	handler.setEmit(emit)

	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	// handler 放进 ctx: 分支判定扫描模型流时要据它把增量文本实时推给前端
	runCtx = withOrchHandler(runCtx, handler)
	// run_id 放进 ctx: 工具审批请求/决定靠它配对
	runCtx = withOrchRunID(runCtx, runID)
	// 用户 ID 放进 ctx: 编排运行没有 ADK 会话, user_info/mem0/reminder 等工具
	// 靠它拿当前用户 (普通对话经 runner.Run(WithSessionValues) 注入, 不走这条)
	runCtx = tools.WithRunUserID(runCtx, userID)

	stream, runErr := compiled.runnable.Stream(runCtx, schema.UserMessage(req.Input), compose.WithCallbacks(handler))
	if runErr != nil {
		orchLog("%s 启动失败: %v", runTag, runErr)
		emit("error", map[string]any{"message": runErr.Error()})
		emit("done", map[string]any{})
		return nil
	}
	defer stream.Close()

	var full strings.Builder
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				orchLog("%s 执行超时 (超时阈值 %s)", runTag, s.timeout)
				emit("error", map[string]any{"message": "执行超时"})
			} else if runCtx.Err() != nil {
				orchLog("%s 执行被取消: %v", runTag, runCtx.Err())
				emit("error", map[string]any{"message": "执行已取消"})
			} else {
				orchLog("%s 执行出错: %v", runTag, err)
				emit("error", map[string]any{"message": err.Error()})
			}
			break
		}
		if chunk.Content == "" {
			continue
		}
		// 模型节点回调已实时推过的内容不再重复下发 (仅影响回放, full 仍保留完整文本)
		content := orchSkipStreamed(handler, chunk.Content)
		full.WriteString(chunk.Content)
		if content == "" {
			continue
		}
		emit("delta", map[string]any{"content": content})
	}

	summary := &DebugRunResult{
		Mode:    compiled.mode,
		Output:  full.String(),
		Nodes:   handler.NodeTraces(),
		TotalMS: time.Since(started).Milliseconds(),
	}
	for _, t := range summary.Nodes {
		if t.Tokens != nil && t.Tokens.TotalTokens > 0 {
			if summary.Tokens == nil {
				cp := *t.Tokens
				summary.Tokens = &cp
			} else {
				summary.Tokens.PromptTokens += t.Tokens.PromptTokens
				summary.Tokens.CompletionTokens += t.Tokens.CompletionTokens
				summary.Tokens.TotalTokens += t.Tokens.TotalTokens
			}
		}
	}
	// token 用量落库: 按各节点配置的模型记录 (含子编排嵌套展开的节点)
	s.recordOrchestrationUsage(userID, req.ChatMode, summary, compiled)

	emit("summary", summary)
	emit("done", map[string]any{})

	// 运行日志: 每个节点的耗时/状态/工具调用/用量逐行落日志, 便于事后复盘
	orchLogRunSummary(runTag, summary, time.Since(started).Milliseconds())
	if summary.Output == "" {
		orchLog("%s 警告: 本次运行没有产出最终文本 (检查入口节点/分支默认目标是否可达)", runTag)
	}

	// 已保存编排: 异步落最近一次调试摘要, 供列表回显
	// (训练中心的编排对话 skip_summary=true, 不污染调试摘要)
	if orchID > 0 && !req.SkipSummary {
		go func(id uint, sres *DebugRunResult) {
			data, mErr := json.Marshal(sres)
			if mErr != nil {
				return
			}
			if err := DB.Model(&model.AIAgent{}).
				Where("agent_type = ? AND id = ?", model.AIAgentTypeOrchestration, id).
				Update("last_debug_summary", string(data)).Error; err != nil {
				log.Printf("[orchestration] save debug summary failed id=%d err=%v", id, err)
			}
		}(orchID, summary)
	}
	return nil
}

// recordOrchestrationUsage 把本次编排运行各节点的用量按节点配置的模型落库:
// 每个节点一条 (一次运行多轮模型调用已在节点 trace 内聚合), 来源区分调试/对话
func (s *AIOrchestrationService) recordOrchestrationUsage(userID uint, chatMode bool, summary *DebugRunResult, compiled *compiledOrchestration) {
	source := model.TokenSourceOrchDebug
	if chatMode {
		source = model.TokenSourceOrchChat
	}
	for _, t := range summary.Nodes {
		if t.Tokens == nil || t.Tokens.TotalTokens <= 0 {
			continue
		}
		modelCode := ""
		if compiled != nil && compiled.nodeModels != nil {
			modelCode = compiled.nodeModels[t.Key]
		}
		// 节点未指定模型 (=默认模型) 时解析出真实模型 code, 避免统计里出现空模型归属
		if modelCode == "" {
			modelCode = s.agentService.ResolveModelCode("")
		}
		RecordTokenUsage(userID, modelCode, source, int64(t.Tokens.PromptTokens), int64(t.Tokens.CompletionTokens))
	}
}

// orchLogRunSummary 输出一次调试运行的完整摘要 (逐节点 + 汇总)
func orchLogRunSummary(runTag string, summary *DebugRunResult, totalMS int64) {
	orchLog("%s 运行结束: mode=%s 总耗时=%dms 节点数=%d 输出长度=%d tokens=%s",
		runTag, summary.Mode, totalMS, len(summary.Nodes), len(summary.Output), orchTokensText(summary.Tokens))
	for _, t := range summary.Nodes {
		if t.Comp == "DelegateTool" {
			// 子Agent 节点: 是否被委派 + 任务 + 结果长度
			orchLog("%s   子Agent %s(%s) status=%s 委派=%v 所属主Agent=%s 耗时=%dms 任务=%.60q 结果长度=%d",
				runTag, t.Name, t.Key, t.Status, t.Delegated, t.Owner, t.MS, t.Task, len(t.Content))
			continue
		}
		orchLog("%s   节点 %s(%s) comp=%s status=%s 耗时=%dms tokens=%s 工具=%v 输出长度=%d%s",
			runTag, t.Name, t.Key, t.Comp, t.Status, t.MS, orchTokensText(t.Tokens), orchToolNames(t.ToolCalls), len(t.Content), orchErrSuffix(t.Error))
	}
}

func orchTokensText(u *schema.TokenUsage) string {
	if u == nil {
		return "-"
	}
	return fmt.Sprintf("prompt=%d completion=%d total=%d", u.PromptTokens, u.CompletionTokens, u.TotalTokens)
}

func orchToolNames(calls []OrchToolTrace) []string {
	if len(calls) == 0 {
		return nil
	}
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, fmt.Sprintf("%s(%dms)", c.Name, c.MS))
	}
	return names
}

func orchErrSuffix(errMsg string) string {
	if errMsg == "" {
		return ""
	}
	return " 错误=" + errMsg
}
