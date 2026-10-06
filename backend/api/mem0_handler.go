package api

import (
	interfaces "backend/interface"
	"strconv"

	"github.com/gin-gonic/gin"
)

type Mem0Handler struct {
	mem0Service interfaces.Mem0Service
}

func NewMem0Handler(mem0Service interfaces.Mem0Service) *Mem0Handler {
	return &Mem0Handler{
		mem0Service: mem0Service,
	}
}

func (h *Mem0Handler) HandleStatus(c *gin.Context) {
	SendSuccess(c, gin.H{"enabled": h.mem0Service.GetEnabled()})
}

func (h *Mem0Handler) HandleAddMemory(c *gin.Context) {
	if !h.mem0Service.IsConfigured() {
		SendError(c, "503", "记忆服务未配置")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	var req struct {
		Messages []interfaces.Mem0Message `json:"messages" binding:"required"`
		Metadata map[string]any     `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", err.Error())
		return
	}

	result, err := h.mem0Service.AddMemory(userID, req.Messages, req.Metadata)
	if err != nil {
		SendError(c, "500", "添加记忆失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

func (h *Mem0Handler) HandleSearchMemories(c *gin.Context) {
	if !h.mem0Service.IsConfigured() {
		SendError(c, "503", "记忆服务未配置")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	var req struct {
		Query string `json:"query" binding:"required"`
		TopK  int    `json:"top_k"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", err.Error())
		return
	}

	memories, err := h.mem0Service.SearchMemories(userID, req.Query, req.TopK)
	if err != nil {
		SendError(c, "500", "搜索记忆失败: "+err.Error())
		return
	}

	SendSuccess(c, memories)
}

func (h *Mem0Handler) HandleListMemories(c *gin.Context) {
	if !h.mem0Service.IsConfigured() {
		SendError(c, "503", "记忆服务未配置")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))

	result, err := h.mem0Service.ListMemories(userID, page, pageSize)
	if err != nil {
		SendError(c, "500", "获取记忆列表失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

func (h *Mem0Handler) HandleDeleteMemory(c *gin.Context) {
	if !h.mem0Service.IsConfigured() {
		SendError(c, "503", "记忆服务未配置")
		return
	}

	memoryID := c.Param("id")
	if memoryID == "" {
		SendError(c, "400", "记忆ID不能为空")
		return
	}

	if err := h.mem0Service.DeleteMemory(memoryID); err != nil {
		SendError(c, "500", "删除记忆失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}
