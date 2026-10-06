package api

import (
	"backend/service"

	"github.com/gin-gonic/gin"
)

type DashboardHandler struct {
	dashboardService *service.DashboardService
}

func NewDashboardHandler(dashboardService *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{
		dashboardService: dashboardService,
	}
}

func (h *DashboardHandler) GetStats(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	stats, err := h.dashboardService.GetStats(userID)
	if err != nil {
		SendError(c, "500", "Failed to fetch dashboard stats")
		return
	}

	SendSuccess(c, stats)
}

// GetAdminStats 系统概览 (管理员): 用户 / AI 用量 / 任务健康 / 操作审计
func (h *DashboardHandler) GetAdminStats(c *gin.Context) {
	stats, err := h.dashboardService.GetAdminStats()
	if err != nil {
		SendError(c, "500", "获取系统概览失败: "+err.Error())
		return
	}

	SendSuccess(c, stats)
}
