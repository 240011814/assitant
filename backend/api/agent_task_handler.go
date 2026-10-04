package api

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/model"
	"backend/service"
)

// AgentTaskHandler 定时 Agent 任务: 用户侧 CRUD + 立即运行 (仅本人任务)
type AgentTaskHandler struct {
	svc *service.AgentTaskService
}

func NewAgentTaskHandler(svc *service.AgentTaskService) *AgentTaskHandler {
	return &AgentTaskHandler{svc: svc}
}

// HandleList 任务列表 (含下次执行时间与参数)
func (h *AgentTaskHandler) HandleList(c *gin.Context) {
	userID := GetUserID(c)
	items, err := h.svc.List(userID)
	if err != nil {
		SendError(c, "500", "获取任务列表失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"items": items})
}

func (h *AgentTaskHandler) HandleCreate(c *gin.Context) {
	userID := GetUserID(c)
	var req model.CreateAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	def, err := h.svc.Create(userID, req)
	if err != nil {
		SendError(c, "500", "创建任务失败: "+err.Error())
		return
	}
	SendSuccess(c, def)
}

func (h *AgentTaskHandler) HandleUpdate(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的任务 ID")
		return
	}
	var req model.UpdateAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	def, err := h.svc.Update(userID, uint(id), req)
	if err != nil {
		SendError(c, "500", "更新任务失败: "+err.Error())
		return
	}
	SendSuccess(c, def)
}

func (h *AgentTaskHandler) HandleDelete(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的任务 ID")
		return
	}
	if err := h.svc.Delete(userID, uint(id)); err != nil {
		SendError(c, "500", "删除任务失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleRunNow 手动立即执行一次 (结果照常落训练历史/通知)
func (h *AgentTaskHandler) HandleRunNow(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的任务 ID")
		return
	}
	if err := h.svc.RunNow(userID, uint(id)); err != nil {
		SendError(c, "500", "触发执行失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"message": "已触发执行, 结果将写入训练历史"})
}
