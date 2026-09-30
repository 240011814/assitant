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

	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// AIOrchestrationService Agent Studio 编排管理: CRUD + DSL 校验 + 调试运行
type AIOrchestrationService struct {
	agentService *AIAgentService
	timeout      time.Duration
}

func NewAIOrchestrationService(agentService *AIAgentService, timeoutMinutes int) *AIOrchestrationService {
	if timeoutMinutes <= 0 {
		timeoutMinutes = 5
	}
	return &AIOrchestrationService{
		agentService: agentService,
		timeout:      time.Duration(timeoutMinutes) * time.Minute,
	}
}

func (s *AIOrchestrationService) List() ([]model.AIOrchestration, error) {
	var list []model.AIOrchestration
	if err := DB.Order("updated_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *AIOrchestrationService) Get(id uint) (*model.AIOrchestration, error) {
	var item model.AIOrchestration
	if err := DB.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *AIOrchestrationService) Create(req *model.CreateAIOrchestrationRequest) (*model.AIOrchestration, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("名称不能为空")
	}
	if errs := s.validateDefinition(req.Definition); len(errs) > 0 {
		return nil, errors.New("编排定义校验失败: " + strings.Join(errs, "; "))
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item := model.AIOrchestration{
		Name:        req.Name,
		Description: req.Description,
		Definition:  req.Definition,
		Version:     1,
		Enabled:     enabled,
	}
	if err := DB.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *AIOrchestrationService) Update(id uint, req *model.UpdateAIOrchestrationRequest) error {
	var item model.AIOrchestration
	if err := DB.First(&item, id).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if req.Name != nil && *req.Name != "" {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.Definition != nil {
		if errs := s.validateDefinition(*req.Definition); len(errs) > 0 {
			return errors.New("编排定义校验失败: " + strings.Join(errs, "; "))
		}
		updates["definition"] = *req.Definition
		updates["version"] = item.Version + 1
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Model(&model.AIOrchestration{}).Where("id = ?", id).Updates(updates).Error
}

func (s *AIOrchestrationService) Delete(id uint) error {
	return DB.Delete(&model.AIOrchestration{}, id).Error
}

// OrchestrationValidationResult 校验结果 (含编译探测到的错误)
type OrchestrationValidationResult struct {
	Valid    bool     `json:"valid"`
	Mode     string   `json:"mode"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Validate 校验编排定义: 结构校验 + 完整编译(捕获类型/构建错误)
func (s *AIOrchestrationService) Validate(definition string, userID uint) *OrchestrationValidationResult {
	result := &OrchestrationValidationResult{}
	dsl, errs := validateOrchestrationDSL(definition)
	result.Errors = append(result.Errors, errs...)
	if len(errs) > 0 {
		result.Valid = false
		return result
	}
	// 结构合法, 尝试完整编译捕获构建期错误
	compiled, err := s.compile(context.Background(), userID, dsl)
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
		}
	}
	return warnings
}

// validateDefinition 完整校验(结构+编译), 返回错误列表
func (s *AIOrchestrationService) validateDefinition(definition string) []string {
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		return errs
	}
	if _, err := s.compile(context.Background(), 0, dsl); err != nil {
		return []string{err.Error()}
	}
	return nil
}

func (s *AIOrchestrationService) compile(ctx context.Context, userID uint, dsl *OrchestrationDSL) (*compiledOrchestration, error) {
	deps := compilerDeps{
		getModel:    s.agentService.GetToolCallingModel,
		buildTool:   s.agentService.BuildToolByName,
		sessionVars: func() map[string]any { return s.agentService.SessionTemplateVars(userID) },
	}
	c := &orchestrationCompiler{dsl: dsl}
	return c.compile(ctx, deps)
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

	var agents []model.AIAgent
	if err := DB.Select("id, title, description, system_prompt").
		Order("is_public DESC, created_at DESC").Limit(200).Find(&agents).Error; err != nil {
		return nil, err
	}
	agentsRes := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		agentsRes = append(agentsRes, map[string]any{
			"id":            a.ID,
			"title":         a.Title,
			"description":   a.Description,
			"system_prompt": a.SystemPrompt,
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

	return map[string]any{
		"tools":   toolsRes,
		"models":  modelsRes,
		"agents":  agentsRes,
		"skills":  skillsRes,
		"node_types": []map[string]string{
			{"type": OrchNodeAgent, "label": "Agent", "desc": "LLM + 工具 ReAct 执行"},
			{"type": OrchNodeTool, "label": "工具", "desc": "独立调用一个已启用工具"},
			{"type": OrchNodeTemplate, "label": "模板", "desc": "组装/改写上游内容为用户消息"},
			{"type": OrchNodeBranch, "label": "分支", "desc": "按条件路由到不同下游"},
			{"type": OrchNodeMerge, "label": "合并", "desc": "汇聚多路上游输出"},
			{"type": OrchNodeEnd, "label": "结束", "desc": "输出最终结果"},
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
// start / delta / node / summary / error / done
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

	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		emit("error", map[string]any{"message": "编排定义校验失败", "errors": errs})
		emit("done", map[string]any{})
		return nil
	}

	compiled, err := s.compile(ctx, userID, dsl)
	if err != nil {
		emit("error", map[string]any{"message": err.Error()})
		emit("done", map[string]any{})
		return nil
	}

	emit("start", map[string]any{
		"mode":       compiled.mode,
		"node_count": len(dsl.Nodes),
	})

	handler := newOrchTraceHandler(compiled.nodeKeys, emit)
	runCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	started := time.Now()
	stream, runErr := compiled.runnable.Stream(runCtx, schema.UserMessage(req.Input), compose.WithCallbacks(handler))
	if runErr != nil {
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
				emit("error", map[string]any{"message": "执行超时"})
			} else if runCtx.Err() != nil {
				emit("error", map[string]any{"message": "执行已取消"})
			} else {
				emit("error", map[string]any{"message": err.Error()})
			}
			break
		}
		if chunk.Content != "" {
			full.WriteString(chunk.Content)
			emit("delta", map[string]any{"content": chunk.Content})
		}
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
	emit("summary", summary)
	emit("done", map[string]any{})

	// 已保存编排: 异步落最近一次调试摘要, 供列表回显
	if orchID > 0 {
		go func(id uint, sres *DebugRunResult) {
			data, mErr := json.Marshal(sres)
			if mErr != nil {
				return
			}
			if err := DB.Model(&model.AIOrchestration{}).Where("id = ?", id).
				Update("last_debug_summary", string(data)).Error; err != nil {
				log.Printf("[orchestration] save debug summary failed id=%d err=%v", id, err)
			}
		}(orchID, summary)
	}
	return nil
}
