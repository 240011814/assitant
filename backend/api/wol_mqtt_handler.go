package api

import (
	"backend/service"

	"github.com/gin-gonic/gin"
)

// WolMqttHandler 设备管理 (WOL MQTT) HTTP 接口
type WolMqttHandler struct {
	svc *service.WolMqttService
}

// NewWolMqttHandler 构造设备管理处理器
func NewWolMqttHandler(svc *service.WolMqttService) *WolMqttHandler {
	return &WolMqttHandler{svc: svc}
}

// GetConfig 读取 MQTT 连接配置
func (h *WolMqttHandler) GetConfig(c *gin.Context) {
	SendSuccess(c, h.svc.LoadConfig())
}

// UpdateConfig 保存 MQTT 连接配置到 system_config
func (h *WolMqttHandler) UpdateConfig(c *gin.Context) {
	var req struct {
		Host     string `json:"host" binding:"required"`
		Port     int    `json:"port"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	cfg := service.WolMqttConfig{Host: req.Host, Port: req.Port, Username: req.Username}
	if err := h.svc.SaveConfig(cfg, req.Password); err != nil {
		SendError(c, "400", err.Error())
		return
	}
	SendSuccess(c, h.svc.LoadConfig())
}

// Connect 建立 MQTT 连接
func (h *WolMqttHandler) Connect(c *gin.Context) {
	if err := h.svc.Connect(); err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, h.svc.Status())
}

// Disconnect 断开 MQTT 连接
func (h *WolMqttHandler) Disconnect(c *gin.Context) {
	h.svc.Disconnect()
	SendSuccess(c, h.svc.Status())
}

// Status 查询连接状态与最近消息
func (h *WolMqttHandler) Status(c *gin.Context) {
	SendSuccess(c, h.svc.Status())
}

// ListDevices 发现并返回设备 (discover=false 时仅返回缓存)
func (h *WolMqttHandler) ListDevices(c *gin.Context) {
	if c.DefaultQuery("discover", "true") == "false" {
		SendSuccess(c, h.svc.ListDevices())
		return
	}
	devices, err := h.svc.Discover()
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, devices)
}

// GetMacList 获取指定设备的地址簿列表
func (h *WolMqttHandler) GetMacList(c *gin.Context) {
	macs, err := h.svc.GetMacList(c.Param("id"))
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, macs)
}

// UpdateMac 地址簿增删改 (action: set/del/clear)
func (h *WolMqttHandler) UpdateMac(c *gin.Context) {
	var req struct {
		Action string `json:"action" binding:"required"`
		MAC    string `json:"mac"`
		IP     string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	if err := h.svc.UpdateMac(c.Param("id"), req.Action, req.MAC, req.IP); err != nil {
		SendError(c, "400", err.Error())
		return
	}
	SendSuccess(c, nil)
}

// Wake 发送唤醒命令
func (h *WolMqttHandler) Wake(c *gin.Context) {
	var req struct {
		MAC string `json:"mac" binding:"required"`
		IP  string `json:"ip"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请填写 MAC 地址")
		return
	}
	if err := h.svc.Wake(c.Param("id"), req.MAC, req.IP); err != nil {
		SendError(c, "400", err.Error())
		return
	}
	SendSuccess(c, nil)
}
