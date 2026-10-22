package api

// 米哈游扫码登录 + 原神每日签到
//
// 流程: POST /api/mihoyo/qr/start 生成二维码 → 米游社 App 扫码确认 →
//       GET /api/mihoyo/qr/status 轮询至 Confirmed (服务端已自动绑定凭据) →
//       每天定时任务自动签到, 或 POST /api/mihoyo/checkin 手动签到。
// 这些接口仅要求登录 (与 Telegram 绑定一致), 不一级一级挂权限。

import (
	"time"

	"backend/service"

	"github.com/gin-gonic/gin"
)

type MihoyoHandler struct {
	svc *service.MihoyoService
}

func NewMihoyoHandler(svc *service.MihoyoService) *MihoyoHandler {
	return &MihoyoHandler{svc: svc}
}

// StartQR 生成扫码登录二维码
func (h *MihoyoHandler) StartQR(c *gin.Context) {
	data, err := h.svc.CreateQRLogin(GetUserID(c))
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, data)
}

// QRStatus 轮询扫码状态 (Confirmed 时服务端已保存登录凭据, 之后不用再轮询)
func (h *MihoyoHandler) QRStatus(c *gin.Context) {
	ticket := c.Query("ticket")
	if ticket == "" {
		SendError(c, "400", "缺少 ticket 参数")
		return
	}
	data, err := h.svc.QRLoginStatus(GetUserID(c), ticket)
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, data)
}

// GetStatus 绑定状态
func (h *MihoyoHandler) GetStatus(c *gin.Context) {
	data, err := h.svc.BindStatus(GetUserID(c))
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, data)
}

// CheckinNow 立即签到一次
func (h *MihoyoHandler) CheckinNow(c *gin.Context) {
	results, err := h.svc.CheckinUser(GetUserID(c))
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, results)
}

// CheckinStatus 签到信息查询 (不触发签到)
func (h *MihoyoHandler) CheckinStatus(c *gin.Context) {
	data, err := h.svc.CheckinStatus(GetUserID(c))
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, data)
}

// Unbind 解绑米游社账号 (删除凭据, 签到记录保留)
func (h *MihoyoHandler) Unbind(c *gin.Context) {
	if err := h.svc.Unbind(GetUserID(c)); err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, gin.H{"unboundAt": time.Now().Format(time.RFC3339)})
}
