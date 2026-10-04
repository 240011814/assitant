package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	iface "backend/interface"
	"backend/model"
)

// 定时 Agent 任务: job_definitions 里 task_name=agent_task.run 的定义到点执行,
// 让指定 Agent(对话) 或 编排 带着输入文本完整跑一遍 (含工具调用), 结果落训练历史
// (training_type=agent_task) 并可选邮件/Telegram 通知。调度复用 once/cron,
// 不用 repeat 链 (每次执行不累积定义行); 无人值守下触发审批的工具直接中止并说明
const agentTaskRunTimeout = 30 * time.Minute

type AgentTaskService struct {
	jobScheduler *JobScheduler
	agentSvc     *AIAgentService
	orchSvc      *AIOrchestrationService
	historySvc   *HistoryService
	notifiers    []iface.Notifier
}

// NewAgentTaskService 构造并注册任务执行器
func NewAgentTaskService(jobScheduler *JobScheduler, agentSvc *AIAgentService, orchSvc *AIOrchestrationService, historySvc *HistoryService, notifiers ...iface.Notifier) *AgentTaskService {
	s := &AgentTaskService{
		jobScheduler: jobScheduler,
		agentSvc:     agentSvc,
		orchSvc:      orchSvc,
		historySvc:   historySvc,
		notifiers:    notifiers,
	}
	jobScheduler.RegisterTask(model.TaskNameAgentTask, "定时运行 Agent/编排 (参数: agent_id/agent_type/input/notify_email/notify_telegram)",
		json.RawMessage(`{"agent_id":1,"agent_type":"chat","input":"总结今天的待办","notify_email":true}`), s.agentTaskHandler)
	return s
}

// ============ 执行 ============

func (s *AgentTaskService) agentTaskHandler(def *model.JobDefinition, paramsRaw json.RawMessage) error {
	var params model.AgentTaskParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return fmt.Errorf("解析参数失败: %v", err)
	}
	if def.UserID == nil {
		return errors.New("任务缺少所属用户")
	}
	userID := *def.UserID
	log.Printf("[AgentTask] 执行任务 def=%d agent=%d(%s) user=%d", def.ID, params.AgentID, params.AgentType, userID)

	ctx, cancel := context.WithTimeout(context.Background(), agentTaskRunTimeout)
	defer cancel()
	output, thinking, err := s.runAgent(ctx, userID, params)
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) == "" {
		return errors.New("Agent 没有产出任何文本输出")
	}

	// 结果落训练历史 (历史列表可见, 继续训练按类型路由不认识该类型则仅展示)
	if s.historySvc != nil {
		agentID := params.AgentID
		inputMsgs := []*schema.Message{{Role: schema.User, Content: params.Input}}
		if _, err := s.historySvc.SaveConversation(&SaveConversationParams{
			UserID:           userID,
			TrainingType:     "agent_task",
			CustomTrainingID: &agentID,
			InputMessages:    inputMsgs,
			AssistantReply:   output,
			ThinkingContent:  thinking,
		}); err != nil {
			log.Printf("[AgentTask] 保存运行结果失败 def=%d: %v", def.ID, err)
		}
	}

	s.notify(def, userID, params, output)
	return nil
}

// runAgent 按类型分流: chat 走完整 Agent 运行时 (含工具), orchestration 走编排运行
func (s *AgentTaskService) runAgent(ctx context.Context, userID uint, params model.AgentTaskParams) (string, string, error) {
	switch params.AgentType {
	case model.AIAgentTypeChat:
		return s.runChatAgent(ctx, userID, params)
	case model.AIAgentTypeOrchestration:
		return s.runOrchestration(ctx, userID, params)
	default:
		return "", "", fmt.Errorf("不支持的 Agent 类型: %s", params.AgentType)
	}
}

func (s *AgentTaskService) runChatAgent(ctx context.Context, userID uint, params model.AgentTaskParams) (string, string, error) {
	if s.agentSvc == nil {
		return "", "", errors.New("Agent 服务未初始化")
	}
	iter, cancel, err := s.agentSvc.ChatStream(ctx, userID, params.AgentID, 0,
		[]*schema.Message{{Role: schema.User, Content: params.Input}}, "")
	if err != nil {
		return "", "", err
	}
	defer cancel()

	var reply, thinking strings.Builder
	for {
		event, ok := iter.Next()
		if !ok {
			break
		}
		if event.Err != nil {
			return "", "", fmt.Errorf("Agent 运行失败: %w", event.Err)
		}
		if event.Action != nil && event.Action.Interrupted != nil {
			return "", "", errors.New("任务触发了需要人工确认的工具 (如发送邮件), 无人值守运行已中止; 请调整该 Agent 的工具确认配置或改用交互对话")
		}
		if event.Output == nil || event.Output.MessageOutput == nil {
			continue
		}
		mv := event.Output.MessageOutput
		if mv.Role == schema.Tool {
			continue
		}
		if mv.IsStreaming && mv.MessageStream != nil {
			for {
				msg, err := mv.MessageStream.Recv()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return "", "", fmt.Errorf("读取 Agent 输出流失败: %w", err)
				}
				thinking.WriteString(msg.ReasoningContent)
				reply.WriteString(msg.Content)
			}
		} else if mv.Message != nil {
			thinking.WriteString(mv.Message.ReasoningContent)
			reply.WriteString(mv.Message.Content)
		}
	}
	return reply.String(), thinking.String(), nil
}

func (s *AgentTaskService) runOrchestration(ctx context.Context, userID uint, params model.AgentTaskParams) (string, string, error) {
	if s.orchSvc == nil {
		return "", "", errors.New("编排服务未初始化")
	}
	var output string
	var runErr error
	// DebugRun 的事件回调里, summary 携带图级最终输出 (与训练页/调试面板同源), error 携带失败信息
	collect := func(event string, payload any) {
		switch event {
		case "summary":
			if m, ok := payload.(map[string]any); ok {
				if o, ok2 := m["output"].(string); ok2 {
					output = o
				}
			}
		case "error":
			if m, ok := payload.(map[string]any); ok {
				if msg, ok2 := m["message"].(string); ok2 {
					runErr = errors.New(msg)
				}
			}
		}
	}
	orchID := int(params.AgentID)
	req := &model.DebugRunRequest{ID: &orchID, Input: params.Input, ChatMode: true, SkipSummary: true}
	if err := s.orchSvc.DebugRun(ctx, userID, req, collect); err != nil {
		return "", "", fmt.Errorf("编排运行失败: %w", err)
	}
	if runErr != nil {
		return "", "", fmt.Errorf("编排运行失败: %w", runErr)
	}
	return output, "", nil
}

// notify 结果通知 (best effort: 失败只记日志, 不触发任务重试以免重复烧 token)
func (s *AgentTaskService) notify(def *model.JobDefinition, userID uint, params model.AgentTaskParams, output string) {
	if !params.NotifyEmail && !params.NotifyTelegram || len(s.notifiers) == 0 {
		return
	}
	var user model.User
	if err := DB.First(&user, userID).Error; err != nil {
		log.Printf("[AgentTask] 通知时找不到用户 %d: %v", userID, err)
		return
	}
	body := output
	if n := len([]rune(body)); n > 4000 {
		body = string([]rune(body)[:4000]) + "\n\n…(内容过长已截断, 完整结果见训练历史)"
	}
	msg := iface.NotifyMessage{Subject: "Agent 任务完成: " + def.Name, Body: body}
	for _, n := range s.notifiers {
		name := strings.ToLower(n.Name())
		if params.NotifyTelegram && strings.Contains(name, "telegram") && user.TelegramChatID != nil {
			if err := n.Send(strconv.FormatInt(*user.TelegramChatID, 10), msg); err != nil {
				log.Printf("[AgentTask] Telegram 通知失败 user=%d: %v", userID, err)
			}
		}
		if params.NotifyEmail && strings.Contains(name, "email") && user.Email != "" {
			if err := n.Send(user.Email, msg); err != nil {
				log.Printf("[AgentTask] 邮件通知失败 user=%d: %v", userID, err)
			}
		}
	}
}

// ============ CRUD ============

// checkAgentAccess 校验 agent 存在、可用且类型匹配 (本人或公开)
func (s *AgentTaskService) checkAgentAccess(userID uint, params model.AgentTaskParams) error {
	var ag model.AIAgent
	if err := DB.First(&ag, params.AgentID).Error; err != nil {
		return errors.New("Agent 不存在")
	}
	if ag.UserID != userID && !ag.IsPublic {
		return errors.New("无权使用该 Agent")
	}
	switch params.AgentType {
	case model.AIAgentTypeChat:
		if ag.AgentType == model.AIAgentTypeOrchestration {
			return errors.New("该条目是编排, 请选择对话 Agent 或把任务类型改为编排")
		}
	case model.AIAgentTypeOrchestration:
		if ag.AgentType != model.AIAgentTypeOrchestration {
			return errors.New("该条目不是编排")
		}
	default:
		return fmt.Errorf("不支持的 Agent 类型: %s", params.AgentType)
	}
	return nil
}

func agentTaskDefName(input string) string {
	cut := strings.TrimSpace(strings.ReplaceAll(input, "\n", " "))
	if len([]rune(cut)) > 24 {
		cut = string([]rune(cut)[:24])
	}
	if cut == "" {
		cut = "任务"
	}
	return fmt.Sprintf("Agent任务-%s-%s", cut, time.Now().Format("20060102150405.000"))
}

// validateAgentTaskSchedule 校验调度配置 (once 需未来时间, cron 需合法表达式)
func validateAgentTaskSchedule(scheduleType string, runAt *time.Time, cronExpr string) error {
	switch scheduleType {
	case model.ScheduleTypeOnce:
		if runAt == nil {
			return errors.New("单次执行需要指定执行时间")
		}
		if runAt.Before(time.Now()) {
			return errors.New("执行时间必须晚于当前时间")
		}
	case model.ScheduleTypeCron:
		if _, err := ValidateCronExpr(cronExpr); err != nil {
			return fmt.Errorf("cron 表达式无效: %w", err)
		}
	default:
		return fmt.Errorf("不支持的调度类型: %s (once/cron)", scheduleType)
	}
	return nil
}

// Create 新增定时 Agent 任务并调度
func (s *AgentTaskService) Create(userID uint, req model.CreateAgentTaskRequest) (*model.JobDefinition, error) {
	params := model.AgentTaskParams{
		AgentID:        req.AgentID,
		AgentType:      req.AgentType,
		Input:          strings.TrimSpace(req.Input),
		NotifyEmail:    req.NotifyEmail,
		NotifyTelegram: req.NotifyTelegram,
	}
	if params.Input == "" {
		return nil, errors.New("任务输入不能为空")
	}
	if err := s.checkAgentAccess(userID, params); err != nil {
		return nil, err
	}
	if err := validateAgentTaskSchedule(req.ScheduleType, req.RunAt, req.CronExpr); err != nil {
		return nil, err
	}
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	def := model.JobDefinition{
		UserID:       &userID,
		Name:         agentTaskDefName(params.Input),
		TaskName:     model.TaskNameAgentTask,
		ScheduleType: req.ScheduleType,
		RunAt:        req.RunAt,
		CronExpr:     req.CronExpr,
		Params:       paramsJSON,
		Enabled:      enabled,
		MaxRetries:   1, // LLM 运行有成本, 失败只重试一次
	}
	if err := DB.Create(&def).Error; err != nil {
		return nil, errors.New("创建任务失败: " + err.Error())
	}
	if def.Enabled {
		if err := s.jobScheduler.ScheduleDefinition(&def); err != nil {
			log.Printf("[AgentTask] 调度任务 %d 失败: %v", def.ID, err)
			return nil, errors.New("任务已创建但调度失败: " + err.Error())
		}
	}
	return &def, nil
}

// Update 修改任务 (重新调度; 调度类型可切换)
func (s *AgentTaskService) Update(userID uint, id uint, req model.UpdateAgentTaskRequest) (*model.JobDefinition, error) {
	var def model.JobDefinition
	if err := DB.Where("id = ? AND user_id = ? AND task_name = ?", id, userID, model.TaskNameAgentTask).First(&def).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	var params model.AgentTaskParams
	if err := json.Unmarshal(def.Params, &params); err != nil {
		return nil, fmt.Errorf("解析原参数失败: %v", err)
	}
	if req.Input != nil {
		params.Input = strings.TrimSpace(*req.Input)
		if params.Input == "" {
			return nil, errors.New("任务输入不能为空")
		}
	}
	if req.NotifyEmail != nil {
		params.NotifyEmail = *req.NotifyEmail
	}
	if req.NotifyTelegram != nil {
		params.NotifyTelegram = *req.NotifyTelegram
	}

	scheduleType := def.ScheduleType
	if req.ScheduleType != nil {
		scheduleType = *req.ScheduleType
	}
	runAt := def.RunAt
	if req.RunAt != nil {
		runAt = req.RunAt
	}
	cronExpr := def.CronExpr
	if req.CronExpr != nil {
		cronExpr = *req.CronExpr
	}
	if err := validateAgentTaskSchedule(scheduleType, runAt, cronExpr); err != nil {
		return nil, err
	}
	// 输入/通知变更即校验 agent 可用性 (防止 agent 被删后任务空跑)
	if err := s.checkAgentAccess(userID, params); err != nil {
		return nil, err
	}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	enabled := def.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	updates := map[string]any{
		"params": paramsJSON, "schedule_type": scheduleType, "run_at": runAt,
		"cron_expr": cronExpr, "enabled": enabled,
	}
	if err := DB.Model(&model.JobDefinition{}).Where("id = ?", def.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	s.jobScheduler.UnscheduleDefinition(def.ID)
	def.ScheduleType, def.RunAt, def.CronExpr, def.Enabled = scheduleType, runAt, cronExpr, enabled
	if def.Enabled {
		if err := s.jobScheduler.ScheduleDefinition(&def); err != nil {
			return nil, errors.New("任务已更新但调度失败: " + err.Error())
		}
	}
	return &def, nil
}

// Delete 删除任务 (取消调度; 执行历史保留)
func (s *AgentTaskService) Delete(userID uint, id uint) error {
	var def model.JobDefinition
	if err := DB.Where("id = ? AND user_id = ? AND task_name = ?", id, userID, model.TaskNameAgentTask).First(&def).Error; err != nil {
		return errors.New("任务不存在")
	}
	s.jobScheduler.UnscheduleDefinition(def.ID)
	return DB.Delete(&model.JobDefinition{}, def.ID).Error
}

// TaskItem 任务列表条目 (定义 + 调度信息 + 最近一次执行)
type TaskItem struct {
	model.JobDefinition
	NextRunAt  *time.Time `json:"next_run_at"`
	ParamsData model.AgentTaskParams `json:"params_data"`
}

// List 用户的定时 Agent 任务 (含下次执行时间)
func (s *AgentTaskService) List(userID uint) ([]TaskItem, error) {
	var defs []model.JobDefinition
	if err := DB.Where("user_id = ? AND task_name = ?", userID, model.TaskNameAgentTask).
		Order("id DESC").Find(&defs).Error; err != nil {
		return nil, err
	}
	items := make([]TaskItem, 0, len(defs))
	for i := range defs {
		def := defs[i]
		item := TaskItem{JobDefinition: def}
		_ = json.Unmarshal(def.Params, &item.ParamsData)
		if def.Enabled {
			item.NextRunAt = s.jobScheduler.NextRunAt(def.ID)
		}
		items = append(items, item)
	}
	return items, nil
}

// RunNow 手动立即执行一次 (不改调度)
func (s *AgentTaskService) RunNow(userID uint, id uint) error {
	var def model.JobDefinition
	if err := DB.Where("id = ? AND user_id = ? AND task_name = ?", id, userID, model.TaskNameAgentTask).First(&def).Error; err != nil {
		return errors.New("任务不存在")
	}
	if !s.jobScheduler.TriggerDefinition(def.ID) {
		return errors.New("触发执行失败")
	}
	return nil
}
