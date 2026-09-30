package api

import (
	"backend/model"
	"backend/service"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"
)

type AIOrchestrationHandler struct {
	svc *service.AIOrchestrationService
}

func NewAIOrchestrationHandler(svc *service.AIOrchestrationService) *AIOrchestrationHandler {
	return &AIOrchestrationHandler{svc: svc}
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
	userID, _ := c.Get("userId")
	uid, _ := userID.(uint)
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
	userID, exists := c.Get("userId")
	if !exists {
		SendError(c, "401", "Unauthorized")
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	// SSE 写串行化: 节点事件来自回调 goroutine, delta 来自主循环
	var mu sync.Mutex
	emit := func(event string, payload any) {
		mu.Lock()
		defer mu.Unlock()
		c.SSEvent(event, payload)
	}

	_ = h.svc.DebugRun(c.Request.Context(), userID.(uint), &req, emit)
}
