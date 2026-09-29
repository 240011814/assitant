package api

import (
	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

type BacktestHandler struct {
	svc *service.BacktestService
}

func NewBacktestHandler(svc *service.BacktestService) *BacktestHandler {
	return &BacktestHandler{svc: svc}
}

// HandleRun 执行策略回测 (ClickHouse 只读)
func (h *BacktestHandler) HandleRun(c *gin.Context) {
	var req model.BacktestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.Run(req)
	if err != nil {
		SendError(c, "500", "回测失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}
