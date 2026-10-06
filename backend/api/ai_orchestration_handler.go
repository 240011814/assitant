package api

import (
	"backend/model"
	"backend/service"
	"log"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

type AIOrchestrationHandler struct {
	svc *service.AIOrchestrationService
	// historyService 用于「编排对话」的多轮持久化 (落 training_histories)
	historyService *service.HistoryService
}

func NewAIOrchestrationHandler(svc *service.AIOrchestrationService, historyService *service.HistoryService) *AIOrchestrationHandler {
	return &AIOrchestrationHandler{svc: svc, historyService: historyService}
}

// HandleList 编排列表
func (h *AIOrchestrationHandler) HandleList(c *gin.Context) {
	items, err := h.svc.List()
	if err != nil {
		SendError(c, "500", "获取编排列表失败")
		return
	}
	SendSuccess(c, items)
}

// HandleGet 编排详情
func (h *AIOrchestrationHandler) HandleGet(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的编排 ID")
		return
	}
	item, err := h.svc.Get(uint(id))
	if err != nil {
		SendError(c, "404", "编排不存在")
		return
	}
	SendSuccess(c, item)
}

// HandleCreate 创建编排
func (h *AIOrchestrationHandler) HandleCreate(c *gin.Context) {
	var req model.CreateAIOrchestrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	item, err := h.svc.Create(&req)
	if err != nil {
		SendError(c, "400", err.Error())
		return
	}
	SendSuccess(c, item)
}

// HandleUpdate 更新编排
func (h *AIOrchestrationHandler) HandleUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的编排 ID")
		return
	}
	var req model.UpdateAIOrchestrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.Update(uint(id), &req); err != nil {
		SendError(c, "400", err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleDelete 删除编排
func (h *AIOrchestrationHandler) HandleDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的编排 ID")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		SendError(c, "500", "删除编排失败")
		return
	}
	SendSuccess(c, nil)
}

// HandleValidate 校验编排定义 (画布草稿可直接校验)
type OrchestrationValidateRequest struct {
	Definition string `json:"definition" binding:"required"`
}

func (h *AIOrchestrationHandler) HandleValidate(c *gin.Context) {
	var req OrchestrationValidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	uid, _ := currentUserID(c)
	SendSuccess(c, h.svc.Validate(req.Definition, uid))
}

// HandleResources 画布可用资源: 工具/模型/Agent/Skill
func (h *AIOrchestrationHandler) HandleResources(c *gin.Context) {
	resources, err := h.svc.Resources()
	if err != nil {
		SendError(c, "500", "获取资源失败: "+err.Error())
		return
	}
	SendSuccess(c, resources)
}

// HandleDebugRun 调试执行编排 (SSE 流式返回节点级事件)
func (h *AIOrchestrationHandler) HandleDebugRun(c *gin.Context) {
	var req model.DebugRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	setupSSE(c)

	// SSE 写串行化: 节点事件来自回调 goroutine, delta 来自主循环
	emit := newSSEEmitter(c).emit

	if err := h.svc.DebugRun(c.Request.Context(), userID, &req, emit); err != nil {
		log.Printf("[orchestration] debug run error: id=%v err=%v", req.ID, err)
	}
}

// HandleResolveApproval 处理一次编排工具审批决定 (调试/编排对话共用)。
// run_id 随 start 事件下发, 只有能看到本次运行 SSE 的用户持有它, 因此仅需登录
type OrchestrationApprovalResolveRequest struct {
	RunID    string `json:"run_id" binding:"required"`
	CallID   string `json:"call_id" binding:"required"`
	Approved bool   `json:"approved"`
	Reason   string `json:"reason"`
}

func (h *AIOrchestrationHandler) HandleResolveApproval(c *gin.Context) {
	var req OrchestrationApprovalResolveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if _, ok := currentUserID(c); !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	if !service.ResolveOrchestrationApproval(req.RunID, req.CallID, req.Approved, req.Reason) {
		SendError(c, "404", "审批请求不存在或已处理")
		return
	}
	SendSuccess(c, nil)
}

// HandleChatList 训练中心「编排对话」列表: 仅返回已启用编排的精简信息
// (不暴露 definition 等画布细节, 权限仅要求登录)
func (h *AIOrchestrationHandler) HandleChatList(c *gin.Context) {
	items, err := h.svc.ListEnabled()
	if err != nil {
		SendError(c, "500", "获取编排列表失败")
		return
	}
	type chatItem struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	out := make([]chatItem, 0, len(items))
	for _, it := range items {
		out = append(out, chatItem{ID: it.ID, Name: it.Name, Description: it.Description})
	}
	SendSuccess(c, out)
}

// HandleChatGet 训练中心「编排对话」详情: 精简信息
func (h *AIOrchestrationHandler) HandleChatGet(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的编排 ID")
		return
	}
	item, err := h.svc.Get(uint(id))
	if err != nil {
		SendError(c, "404", "编排不存在")
		return
	}
	SendSuccess(c, map[string]any{
		"id":          item.ID,
		"name":        item.Name,
		"description": item.Description,
	})
}

// HandleChatRun 训练中心「编排对话」: 复用调试运行时以 SSE 流式返回,
// 但只要求 ai:chat:send (普通训练用户可对话), 且不回写 last_debug_summary
func (h *AIOrchestrationHandler) HandleChatRun(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的编排 ID")
		return
	}
	var body struct {
		Input     string           `json:"input" binding:"required"`
		History   []model.ChatTurn `json:"history"`
		HistoryID uint             `json:"history_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	uid := userID

	orchID := int(id)
	req := &model.DebugRunRequest{
		ID:          &orchID,
		Input:       body.Input,
		History:     body.History,
		SkipSummary: true,
		// 编排对话: 注入编排名称/简介作为身份前言 (模型此前只在前端欢迎气泡"见过"自己)
		ChatMode: true,
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// 捕获最终答案与思考内容, 运行结束后落库 (供刷新/重开后继续与历史列表回放)
	var finalOutput string
	var thinking strings.Builder

	// SSE 写串行化: 节点事件来自回调 goroutine, delta 来自主循环; 顺带捕获最终答案与思考供落库
	em := newSSEEmitter(c)
	emit := func(event string, payload any) {
		switch event {
		case "summary":
			if res, ok := payload.(*service.DebugRunResult); ok {
				finalOutput = res.Output
			}
		case "reasoning":
			if m, ok := payload.(map[string]any); ok {
				if s, _ := m["content"].(string); s != "" {
					thinking.WriteString(s)
				}
			}
		}
		em.emit(event, payload)
	}

	if err := h.svc.DebugRun(c.Request.Context(), uid, req, emit); err != nil {
		log.Printf("[orchestration] chat run error: orchID=%d userID=%d err=%v", orchID, uid, err)
	}

	// 「编排对话」持久化: 把本轮 user + 图级最终答案写入 training_histories。
	// 无输出 (编译/启动失败) 时不落库, 避免留下空的半截会话。
	if h.historyService != nil && strings.TrimSpace(finalOutput) != "" {
		inputMessages := service.OrchChatTurnsToMessages(body.History)
		inputMessages = append(inputMessages, schema.UserMessage(body.Input))
		customID := uint(orchID)
		historyID, saveErr := h.historyService.SaveConversation(&service.SaveConversationParams{
			UserID:           uid,
			HistoryID:        body.HistoryID,
			TrainingType:     model.TrainingTypeOrchestration,
			CustomTrainingID: &customID,
			InputMessages:    inputMessages,
			AssistantReply:   finalOutput,
			ThinkingContent:  thinking.String(),
		})
		if saveErr != nil {
			log.Printf("[orchestration] save chat history failed user=%d orch=%d err=%v", uid, orchID, saveErr)
		} else if body.HistoryID == 0 {
			em.emit("history_id", gin.H{"history_id": historyID, "title": "编排对话"})
		}
	}
}
