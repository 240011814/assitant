package api

import (
	"strconv"

	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

type StockAlertHandler struct {
	svc *service.StockAlertService
}

func NewStockAlertHandler(svc *service.StockAlertService) *StockAlertHandler {
	return &StockAlertHandler{svc: svc}
}

// HandleListAlerts 预警规则列表 (本人)
func (h *StockAlertHandler) HandleListAlerts(c *gin.Context) {
	userID := GetUserID(c)
	list, err := h.svc.ListAlerts(userID)
	if err != nil {
		SendError(c, "500", "获取预警规则失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
}

// HandleCreateAlert 新建预警规则
func (h *StockAlertHandler) HandleCreateAlert(c *gin.Context) {
	userID := GetUserID(c)
	var req model.CreateStockAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	rule, err := h.svc.CreateAlert(userID, req)
	if err != nil {
		SendError(c, "500", "创建预警规则失败: "+err.Error())
		return
	}
	SendSuccess(c, rule)
}

// HandleUpdateAlert 更新预警规则
func (h *StockAlertHandler) HandleUpdateAlert(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "规则 ID 不合法")
		return
	}
	var req model.UpdateStockAlertRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	if err := h.svc.UpdateAlert(userID, uint(id), req); err != nil {
		SendError(c, "500", "更新预警规则失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleDeleteAlert 删除预警规则
func (h *StockAlertHandler) HandleDeleteAlert(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "规则 ID 不合法")
		return
	}
	if err := h.svc.DeleteAlert(userID, uint(id)); err != nil {
		SendError(c, "500", "删除预警规则失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}
