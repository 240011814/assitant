package api

import (
	"backend/model"
	"backend/service"
	"strconv"

	"github.com/gin-gonic/gin"
)

type HistoryHandler struct {
	historyService *service.HistoryService
}

func NewHistoryHandler(historyService *service.HistoryService) *HistoryHandler {
	return &HistoryHandler{
		historyService: historyService,
	}
}

func (h *HistoryHandler) GetHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid history ID")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	history, err := h.historyService.GetHistoryByID(userID, uint(id))
	if err != nil {
		SendError(c, "404", "History not found")
		return
	}

	SendSuccess(c, history)
}

func (h *HistoryHandler) ListHistory(c *gin.Context) {
	var req model.ListHistoryRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		SendError(c, "400", "Invalid request parameters: "+err.Error())
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	histories, total, err := h.historyService.ListHistory(userID, req.Page, req.PageSize, req.Title, req.IsFavorite)
	if err != nil {
		SendError(c, "500", "Failed to fetch histories")
		return
	}

	SendSuccess(c, model.ListHistoryResponse{
		Total: total,
		Items: histories,
	})
}

func (h *HistoryHandler) UpdateFavorite(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid history ID")
		return
	}

	var req model.UpdateFavoriteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "Invalid request body: "+err.Error())
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	if err := h.historyService.UpdateFavorite(userID, uint(id), req.IsFavorite); err != nil {
		SendError(c, "500", "Failed to update favorite status")
		return
	}

	SendSuccess(c, nil)
}

func (h *HistoryHandler) UpdateTitle(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid history ID")
		return
	}

	var req model.UpdateTitleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "Invalid request body: "+err.Error())
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	if err := h.historyService.UpdateTitle(userID, uint(id), req.Title); err != nil {
		SendError(c, "500", "Failed to update title")
		return
	}

	SendSuccess(c, nil)
}

func (h *HistoryHandler) DeleteHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid history ID")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	if err := h.historyService.DeleteHistory(userID, uint(id)); err != nil {
		SendError(c, "500", "Failed to delete history")
		return
	}

	SendSuccess(c, nil)
}

func (h *HistoryHandler) GenerateShare(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid history ID")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	token, err := h.historyService.GenerateShareToken(userID, uint(id))
	if err != nil {
		SendError(c, "500", "Failed to generate share token")
		return
	}

	SendSuccess(c, gin.H{"share_token": token})
}

func (h *HistoryHandler) RevokeShare(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "Invalid history ID")
		return
	}

	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	if err := h.historyService.RevokeShareToken(userID, uint(id)); err != nil {
		SendError(c, "500", "Failed to revoke share token")
		return
	}

	SendSuccess(c, nil)
}

func HandleGetSharedHistory(historyService *service.HistoryService) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.Param("token")
		if token == "" {
			SendError(c, "400", "Share token is required")
			return
		}

		history, err := historyService.GetSharedHistory(token)
		if err != nil {
			SendError(c, "404", "Shared conversation not found")
			return
		}

		SendSuccess(c, history)
	}
}
