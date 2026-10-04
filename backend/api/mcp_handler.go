package api

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"backend/model"
	"backend/service"
)

// MCPHandler MCP 服务管理: 配置动态注册 (增删改/手动重连), 工具运行时发现注册
type MCPHandler struct {
	svc *service.MCPService
}

func NewMCPHandler(svc *service.MCPService) *MCPHandler {
	return &MCPHandler{svc: svc}
}

type mcpServerRequest struct {
	Name           string `json:"name"`
	Transport      string `json:"transport"`
	URL            string `json:"url"`
	Command        string `json:"command"`
	Args           string `json:"args"`
	Env            string `json:"env"`
	Headers        string `json:"headers"`
	TimeoutSeconds int    `json:"timeout_seconds"`
	Enabled        bool   `json:"enabled"`
}

// HandleList 服务列表 (含当前已注册的工具名)
func (h *MCPHandler) HandleList(c *gin.Context) {
	var servers []model.MCPServer
	if err := service.DB.Order("id ASC").Find(&servers).Error; err != nil {
		SendError(c, "500", "获取服务列表失败: "+err.Error())
		return
	}
	items := make([]gin.H, 0, len(servers))
	for i := range servers {
		s := servers[i]
		items = append(items, gin.H{
			"id": s.ID, "name": s.Name, "transport": s.Transport, "url": s.URL,
			"command": s.Command, "args": s.Args, "env": s.Env, "headers": s.Headers,
			"timeout_seconds": s.TimeoutSeconds, "enabled": s.Enabled,
			"last_status": s.LastStatus, "last_error": s.LastError, "tool_count": s.ToolCount,
			"registered_tools": h.svc.RegisteredToolNames(s.Name),
			"created_at":       s.CreatedAt, "updated_at": s.UpdatedAt,
		})
	}
	SendSuccess(c, gin.H{"items": items})
}

// HandleCreate 新增服务 (启用时自动连接并注册工具)
func (h *MCPHandler) HandleCreate(c *gin.Context) {
	var req mcpServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	srv := &model.MCPServer{
		Name: req.Name, Transport: req.Transport, URL: req.URL,
		Command: req.Command, Args: req.Args, Env: req.Env, Headers: req.Headers,
		TimeoutSeconds: req.TimeoutSeconds, Enabled: req.Enabled,
	}
	if err := h.svc.Create(srv); err != nil {
		SendError(c, "500", "创建失败: "+err.Error())
		return
	}
	SendSuccess(c, srv)
}

// HandleUpdate 修改服务 (断开旧连接; 禁用时自动禁用其工具行; 启用时重新连接)
func (h *MCPHandler) HandleUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的服务 ID")
		return
	}
	var req mcpServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	srv := &model.MCPServer{
		Name: req.Name, Transport: req.Transport, URL: req.URL,
		Command: req.Command, Args: req.Args, Env: req.Env, Headers: req.Headers,
		TimeoutSeconds: req.TimeoutSeconds, Enabled: req.Enabled,
	}
	if err := h.svc.Update(uint(id), srv); err != nil {
		SendError(c, "500", "更新失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleConnect 手动重连 (重新发现并注册工具)
func (h *MCPHandler) HandleConnect(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的服务 ID")
		return
	}
	if err := h.svc.ConnectNow(uint(id)); err != nil {
		SendError(c, "500", "重连失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"message": "已开始连接, 稍后刷新查看状态"})
}

// HandleDelete 删除服务 (自动注销工具并删除其 ai_tools 行)
func (h *MCPHandler) HandleDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil || id == 0 {
		SendError(c, "400", "无效的服务 ID")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}
