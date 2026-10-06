package api

import (
	"backend/model"
	"backend/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

type AIAgentHandler struct {
	aiAgentService *service.AIAgentService
}

func NewAIAgentHandler(aiAgentService *service.AIAgentService) *AIAgentHandler {
	return &AIAgentHandler{
		aiAgentService: aiAgentService,
	}
}

func (h *AIAgentHandler) ListAvailableAgents(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	agents, err := h.aiAgentService.ListAvailableAgents(userID)
	if err != nil {
		SendError(c, "500", "Failed to fetch agents")
		return
	}

	SendSuccess(c, agents)
}

func (h *AIAgentHandler) GetAIAgent(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid agent ID")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	agent, err := h.aiAgentService.GetAIAgentByID(userID, uint(id))
	if err != nil {
		SendError(c, "404", "Agent not found")
		return
	}

	SendSuccess(c, agent)
}

func (h *AIAgentHandler) CreateAIAgent(c *gin.Context) {
	var req model.CreateAIAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "Invalid request body: "+err.Error())
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	// 子Agent 统一权限码: 只有拥有 ai:subagent:manage 的用户才能新建子Agent
	if model.IsSubAgentType(req.AgentType) && !HasPermission(c, model.AIAgentSubAgentPermission) {
		SendError(c, "403", "无权限创建子Agent")
		return
	}

	agent, err := h.aiAgentService.CreateAIAgent(userID, req)
	if err != nil {
		SendError(c, "500", "Failed to create agent")
		return
	}

	SendSuccess(c, agent)
}

func (h *AIAgentHandler) UpdateAIAgent(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid agent ID")
		return
	}

	var req model.UpdateAIAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "Invalid request body: "+err.Error())
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	uid := userID
	// 子Agent 统一权限码: 编辑子Agent (含把它改成子Agent) 需要该权限
	becomesSubAgent := req.AgentType != nil && model.IsSubAgentType(*req.AgentType)
	isSubAgent := false
	if existing, err := h.aiAgentService.GetAIAgentByID(uid, uint(id)); err == nil && existing != nil {
		isSubAgent = existing.AgentType == model.AIAgentTypeSubAgent
	}
	if (becomesSubAgent || isSubAgent) && !HasPermission(c, model.AIAgentSubAgentPermission) {
		SendError(c, "403", "无权限编辑子Agent")
		return
	}

	if err := h.aiAgentService.UpdateAIAgent(uid, uint(id), req); err != nil {
		SendError(c, "500", "Failed to update agent")
		return
	}

	SendSuccess(c, nil)
}

func (h *AIAgentHandler) DeleteAIAgent(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid agent ID")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	uid := userID
	// 子Agent 统一权限码: 删除子Agent 需要该权限
	if existing, err := h.aiAgentService.GetAIAgentByID(uid, uint(id)); err == nil && existing != nil && existing.AgentType == model.AIAgentTypeSubAgent {
		if !HasPermission(c, model.AIAgentSubAgentPermission) {
			SendError(c, "403", "无权限删除子Agent")
			return
		}
	}

	if err := h.aiAgentService.DeleteAIAgent(uid, uint(id)); err != nil {
		SendError(c, "500", "Failed to delete agent")
		return
	}

	SendSuccess(c, nil)
}
