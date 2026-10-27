package api

import (
	"backend/service/sys/userpref"

	"github.com/gin-gonic/gin"
)

type UserPreferenceHandler struct {
	svc *userpref.UserPreferenceService
}

func NewUserPreferenceHandler(svc *userpref.UserPreferenceService) *UserPreferenceHandler {
	return &UserPreferenceHandler{svc: svc}
}

// GetThemePreference 获取当前用户的主题配置
func (h *UserPreferenceHandler) GetThemePreference(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	pref, err := h.svc.GetPreference(userID, "themeSettings")
	if err != nil {
		// 未找到返回空对象，不报错
		SendSuccess(c, nil)
		return
	}

	SendSuccess(c, pref.PrefValue)
}

// SaveThemePreference 保存当前用户的主题配置
func (h *UserPreferenceHandler) SaveThemePreference(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	var body interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.SavePreference(userID, "themeSettings", body); err != nil {
		SendError(c, "500", "保存失败")
		return
	}

	SendSuccess(c, nil)
}

// GetNotificationPreference 获取当前用户的通知渠道配置
func (h *UserPreferenceHandler) GetNotificationPreference(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	pref, err := h.svc.GetPreference(userID, "notification_channels")
	if err != nil {
		// 未找到返回默认值 ["email"]
		SendSuccess(c, []string{"email"})
		return
	}

	SendSuccess(c, pref.PrefValue)
}

// SaveNotificationPreference 保存当前用户的通知渠道配置
func (h *UserPreferenceHandler) SaveNotificationPreference(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	var channels []string
	if err := c.ShouldBindJSON(&channels); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.SavePreference(userID, "notification_channels", channels); err != nil {
		SendError(c, "500", "保存失败")
		return
	}

	SendSuccess(c, nil)
}
