package api

import (
	"backend/service"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramHandler struct {
	telegramService *service.TelegramService
	configService   *service.SystemConfigService
}

func NewTelegramHandler(telegramService *service.TelegramService, configService *service.SystemConfigService) *TelegramHandler {
	return &TelegramHandler{
		telegramService: telegramService,
		configService:   configService,
	}
}

// HandleGetTelegramConfig 获取 Telegram 配置状态（是否配置了 Bot Token 且启用）
func (h *TelegramHandler) HandleGetTelegramConfig(c *gin.Context) {
	enabled := h.configService.IsTelegramEnabled()
	token := h.configService.GetTelegramBotToken()
	SendSuccess(c, gin.H{"configured": enabled && token != ""})
}

// HandleGenerateBindCode 生成 Telegram 绑定码
func (h *TelegramHandler) HandleGenerateBindCode(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	result, err := h.telegramService.GenerateBindCode(userID)
	if err != nil {
		SendError(c, "1001", err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleGetTelegramStatus 获取 Telegram 绑定状态
func (h *TelegramHandler) HandleGetTelegramStatus(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	result, err := h.telegramService.GetTelegramStatus(userID)
	if err != nil {
		SendError(c, "1001", err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleUnbindTelegram 解绑 Telegram
func (h *TelegramHandler) HandleUnbindTelegram(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	if err := h.telegramService.UnbindTelegram(userID); err != nil {
		SendError(c, "1001", err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleWebhook 处理 Telegram Webhook 回调
func (h *TelegramHandler) HandleWebhook(c *gin.Context) {
	// 校验 Telegram 官方 webhook secret (系统配置 telegram_webhook_secret), 防止伪造 update
	if !h.telegramService.VerifyWebhookSecret(c.GetHeader("X-Telegram-Bot-Api-Secret-Token")) {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid secret token"})
		return
	}

	bot := h.telegramService.GetBot()
	if bot == nil {
		log.Println("[Telegram Webhook] Bot is not initialized")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "bot not initialized"})
		return
	}

	var update tgbotapi.Update
	if err := c.ShouldBindJSON(&update); err != nil {
		log.Printf("[Telegram Webhook] Failed to parse update: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update"})
		return
	}

	h.telegramService.HandleWebhookUpdate(update)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
