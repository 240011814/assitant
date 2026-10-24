package api

import (
	"log"
	"strconv"

	"github.com/gin-gonic/gin"
)

// ---------- 运行历史 ----------

// HandleListRuns 编排的运行历史 (新→旧, 供调试面板回看)
func (h *AIOrchestrationHandler) HandleListRuns(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的编排 ID")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	runs, err := h.svc.ListRuns(uint(id), limit)
	if err != nil {
		log.Printf("[orchestration] list runs failed orch=%d err=%v", id, err)
		SendError(c, "500", "获取运行历史失败")
		return
	}
	SendSuccess(c, runs)
}

// ---------- 单节点试跑 ----------

type OrchestrationDebugNodeRequest struct {
	Definition string            `json:"definition" binding:"required"`
	NodeID     string            `json:"node_id" binding:"required"`
	Input      string            `json:"input" binding:"required"`
	Params     map[string]string `json:"params"`
}

// HandleDebugNode 单节点试跑: 只运行目标节点 (含其子Agent 委派), 用于验证单个节点的提示词/配置
func (h *AIOrchestrationHandler) HandleDebugNode(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	var req OrchestrationDebugNodeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.DebugNode(c.Request.Context(), userID, req.Definition, req.NodeID, req.Input, req.Params)
	if err != nil {
		SendError(c, "500", "节点试跑失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// ---------- 自定义模板 ----------

type OrchestrationTemplateSaveRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Definition  string `json:"definition" binding:"required"`
}

// HandleListMyTemplates 当前用户的自定义编排模板
func (h *AIOrchestrationHandler) HandleListMyTemplates(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	rows, err := h.svc.ListMyTemplates(userID)
	if err != nil {
		log.Printf("[orchestration] list templates failed user=%d err=%v", userID, err)
		SendError(c, "500", "获取模板列表失败")
		return
	}
	SendSuccess(c, rows)
}

// HandleSaveMyTemplate 把画布定义存为自定义模板
func (h *AIOrchestrationHandler) HandleSaveMyTemplate(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	var req OrchestrationTemplateSaveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	row, err := h.svc.SaveMyTemplate(userID, req.Name, req.Description, req.Definition)
	if err != nil {
		SendError(c, "500", "保存模板失败: "+err.Error())
		return
	}
	SendSuccess(c, row)
}

// HandleDeleteMyTemplate 删除自定义模板
func (h *AIOrchestrationHandler) HandleDeleteMyTemplate(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的模板 ID")
		return
	}
	if err := h.svc.DeleteMyTemplate(userID, uint(id)); err != nil {
		SendError(c, "500", "删除模板失败: "+err.Error())
		return
	}
	SendSuccess(c, true)
}
