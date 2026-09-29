package service

import (
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	iface "backend/interface"
	"backend/model"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type TelegramService struct {
	sysCfgService *SystemConfigService
	bot           *tgbotapi.BotAPI
	stopCh        chan struct{}
	mu            sync.RWMutex // 保护 bot/running/webhookURL
	startMu       sync.Mutex
	webhookURL    string
	running       bool
}

func NewTelegramService(sysCfgService *SystemConfigService) *TelegramService {
	s := &TelegramService{
		sysCfgService: sysCfgService,
		stopCh:        make(chan struct{}),
	}
	return s
}

// Name 实现 Notifier 接口
func (s *TelegramService) Name() string {
	return "telegram"
}

// ErrBotNotStarted 表示 Telegram bot 尚未启动，调用方不应视为发送失败
var ErrBotNotStarted = errors.New("Telegram bot 未启动")

// getBot 在读锁内取 bot 指针快照, 供各处安全使用 (指针本身指向并发安全的 BotAPI)
func (s *TelegramService) getBot() *tgbotapi.BotAPI {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.bot
}

// Send 实现 Notifier 接口，发送 Telegram 消息
// to 参数为用户的 telegram_chat_id（字符串形式）
func (s *TelegramService) Send(to string, msg iface.NotifyMessage) error {
	bot := s.getBot()
	if bot == nil {
		return ErrBotNotStarted
	}

	var chatID int64
	if _, err := fmt.Sscanf(to, "%d", &chatID); err != nil {
		return fmt.Errorf("无效的 Telegram Chat ID: %s", to)
	}

	text := fmt.Sprintf("*%s*\n\n%s", msg.Subject, msg.Body)
	tgMsg := tgbotapi.NewMessage(chatID, text)
	tgMsg.ParseMode = "Markdown"

	_, err := bot.Send(tgMsg)
	if err != nil {
		// Markdown 解析失败，尝试纯文本
		tgMsg.ParseMode = ""
		_, err = bot.Send(tgMsg)
	}
	return err
}

// StartBot 启动 Telegram Bot
func (s *TelegramService) StartBot() error {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		log.Println("[Telegram] Bot is already running, skipping")
		return nil
	}
	s.running = true
	s.mu.Unlock()

	if !s.sysCfgService.IsTelegramEnabled() {
		log.Println("[Telegram] Bot is disabled in system config, skipping")
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
		return nil
	}

	token := s.sysCfgService.GetTelegramBotToken()
	if token == "" {
		log.Println("[Telegram] Bot Token not configured, skipping bot start")
		return nil
	}

	log.Printf("[Telegram] Starting bot (token length=%d)", len(token))

	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Printf("[Telegram] Failed to create bot: %v", err)
		return fmt.Errorf("failed to create bot: %w", err)
	}

	s.mu.Lock()
	s.bot = bot
	s.mu.Unlock()
	bot.Debug = true
	log.Printf("[Telegram] Bot authorized as @%s (ID: %d)", bot.Self.UserName, bot.Self.ID)

	webhookURL := s.sysCfgService.GetTelegramWebhookURL()
	if webhookURL != "" {
		webhookURL = strings.TrimRight(webhookURL, "/") + "/api/telegram/webhook"
		log.Printf("[Telegram] Using webhook mode, URL: %s", webhookURL)
		if err := s.setupWebhook(webhookURL); err != nil {
			log.Printf("[Telegram] Failed to setup webhook: %v, falling back to long polling", err)
			s.webhookURL = ""
			s.startLongPolling()
		} else {
			s.webhookURL = webhookURL
		}
	} else {
		log.Println("[Telegram] Using long polling mode")
		s.webhookURL = ""
		s.startLongPolling()
	}

	s.setBotCommands()
	return nil
}

// setBotCommands 设置 Bot 命令菜单
func (s *TelegramService) setBotCommands() {
	bot := s.getBot()
	if bot == nil {
		return
	}

	commands := []tgbotapi.BotCommand{
		{Command: "bind", Description: "绑定账号"},
		{Command: "unbind", Description: "解绑账号"},
		{Command: "status", Description: "查看绑定状态"},
		{Command: "help", Description: "帮助信息"},
	}

	_, err := bot.Request(tgbotapi.NewSetMyCommands(commands...))
	if err != nil {
		log.Printf("[Telegram] Failed to set bot commands: %v", err)
	} else {
		log.Println("[Telegram] Bot commands set successfully")
	}
}

// StopBot 停止 Bot
func (s *TelegramService) StopBot() {
	s.startMu.Lock()
	defer s.startMu.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}

	if s.bot != nil {
		if s.webhookURL != "" {
			log.Println("[Telegram] Deleting webhook...")
			_, err := s.bot.Request(tgbotapi.DeleteWebhookConfig{})
			if err != nil {
				log.Printf("[Telegram] Failed to delete webhook: %v", err)
			}
		} else {
			s.bot.StopReceivingUpdates()
		}
		s.bot = nil
		s.webhookURL = ""
	}

	s.running = false
}

// RestartBot 重启 Bot（配置更新后调用）
func (s *TelegramService) RestartBot() error {
	s.StopBot()

	s.mu.Lock()
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	return s.StartBot()
}

// setupWebhook 设置 Webhook
func (s *TelegramService) setupWebhook(webhookURL string) error {
	bot := s.getBot()
	if bot == nil {
		return fmt.Errorf("bot is nil")
	}
	_, err := bot.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		log.Printf("[Telegram] Failed to delete old webhook: %v", err)
	}

	wh, err := tgbotapi.NewWebhook(webhookURL)
	if err != nil {
		return fmt.Errorf("failed to create webhook config: %w", err)
	}

	_, err = bot.Request(wh)
	if err != nil {
		return fmt.Errorf("failed to set webhook: %w", err)
	}

	info, err := bot.GetWebhookInfo()
	if err != nil {
		return fmt.Errorf("failed to get webhook info: %w", err)
	}

	if info.LastErrorDate != 0 {
		log.Printf("[Telegram] Webhook has error: %s", info.LastErrorMessage)
	}

	log.Printf("[Telegram] Webhook set successfully, pending updates: %d", info.PendingUpdateCount)
	return nil
}

// startLongPolling 启动 Long Polling 模式
func (s *TelegramService) startLongPolling() {
	bot := s.getBot()
	if bot == nil {
		log.Println("[Telegram] Bot is nil, skip long polling setup")
		return
	}
	log.Println("[Telegram] Deleting webhook...")
	_, err := bot.Request(tgbotapi.DeleteWebhookConfig{})
	if err != nil {
		log.Printf("[Telegram] Failed to delete webhook: %v", err)
	} else {
		log.Println("[Telegram] Webhook deleted successfully")
	}

	go s.listenUpdates(bot)
}

// GetBot 获取 Bot 实例（供 webhook handler 使用）
func (s *TelegramService) GetBot() *tgbotapi.BotAPI {
	return s.getBot()
}

// HandleWebhookUpdate 处理 Webhook 回调的 update
func (s *TelegramService) HandleWebhookUpdate(update tgbotapi.Update) {
	if update.Message != nil {
		log.Printf("[Telegram] [Webhook] Received message from %s: %s", update.Message.From.UserName, update.Message.Text)
		s.handleMessage(update.Message)
	}
}

// listenUpdates 监听消息更新
func (s *TelegramService) listenUpdates(bot *tgbotapi.BotAPI) {
	if bot == nil {
		log.Println("[Telegram] Bot is nil, cannot listen for updates")
		return
	}

	log.Println("[Telegram] Starting to listen for updates...")

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)
	log.Println("[Telegram] Update channel created, waiting for messages...")

	for {
		select {
		case <-s.stopCh:
			log.Println("[Telegram] Stop signal received")
			return
		case update := <-updates:
			if update.Message != nil {
				log.Printf("[Telegram] Received message from %s: %s", update.Message.From.UserName, update.Message.Text)
				s.handleMessage(update.Message)
			}
		}
	}
}

// handleMessage 处理用户消息
func (s *TelegramService) handleMessage(msg *tgbotapi.Message) {
	log.Printf("[Telegram] Handling message: %s (IsCommand: %v)", msg.Text, msg.IsCommand())

	if !msg.IsCommand() {
		s.reply(msg.Chat.ID, "发送 /help 查看可用命令。")
		return
	}

	switch msg.Command() {
	case "start":
		s.handleStart(msg)
	case "bind":
		s.handleBind(msg)
	case "unbind":
		s.handleUnbind(msg)
	case "status":
		s.handleStatus(msg)
	case "help":
		s.handleHelp(msg)
	default:
		s.reply(msg.Chat.ID, "未知命令。发送 /help 查看可用命令。")
	}
}

// handleStart 处理 /start 命令
func (s *TelegramService) handleStart(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	user, _ := s.getUserByChatID(chatID)

	if user != nil {
		s.reply(chatID, fmt.Sprintf("你好 %s！你的账号已绑定。", user.Nickname))
	} else {
		s.reply(chatID, "你好！请先在网页端生成绑定码，然后使用 /bind <code> 绑定你的账号。")
	}
}

// handleBind 处理 /bind 命令
func (s *TelegramService) handleBind(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID
	args := msg.CommandArguments()

	if args == "" {
		s.reply(chatID, "请提供绑定码。用法：/bind <6位数字码>")
		return
	}

	existingUser, _ := s.getUserByChatID(chatID)
	if existingUser != nil {
		s.reply(chatID, "你的 Telegram 已绑定账号："+existingUser.Username)
		return
	}

	var user model.User
	now := time.Now()
	result := DB.Where("telegram_bind_code = ? AND telegram_bind_code_expires_at > ?", args, now).First(&user)
	if result.Error != nil {
		s.reply(chatID, "绑定码无效或已过期，请重新生成。")
		return
	}

	telegramUsername := msg.From.UserName
	if telegramUsername == "" {
		telegramUsername = msg.From.FirstName
	}

	updates := map[string]interface{}{
		"telegram_chat_id":            chatID,
		"telegram_username":           telegramUsername,
		"telegram_bind_code":          nil,
		"telegram_bind_code_expires_at": nil,
	}

	if err := DB.Model(&user).Updates(updates).Error; err != nil {
		s.reply(chatID, "绑定失败，请稍后重试。")
		return
	}

	s.reply(chatID, fmt.Sprintf("绑定成功！你好 %s，现在可以接收通知了。", user.Nickname))
}

// handleUnbind 处理 /unbind 命令
func (s *TelegramService) handleUnbind(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	user, _ := s.getUserByChatID(chatID)
	if user == nil {
		s.reply(chatID, "你的 Telegram 尚未绑定任何账号。")
		return
	}

	updates := map[string]interface{}{
		"telegram_chat_id":  nil,
		"telegram_username": nil,
	}

	if err := DB.Model(&model.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		s.reply(chatID, "解绑失败，请稍后重试。")
		return
	}

	s.reply(chatID, "已成功解绑。")
}

// handleStatus 处理 /status 命令
func (s *TelegramService) handleStatus(msg *tgbotapi.Message) {
	chatID := msg.Chat.ID

	user, _ := s.getUserByChatID(chatID)
	if user == nil {
		s.reply(chatID, "你的 Telegram 尚未绑定任何账号。请使用 /bind <code> 绑定。")
		return
	}

	s.reply(chatID, fmt.Sprintf("已绑定账号：%s (%s)", user.Username, user.Nickname))
}

// handleHelp 处理 /help 命令
func (s *TelegramService) handleHelp(msg *tgbotapi.Message) {
	helpText := `可用命令：

/bind <code> - 绑定账号（在网页端生成绑定码）
/unbind - 解绑账号
/status - 查看绑定状态
/help - 显示此帮助信息

绑定步骤：
1. 登录网页端，进入个人中心
2. 点击 Telegram 绑定，生成绑定码
3. 在这里发送 /bind <绑定码>`

	s.reply(msg.Chat.ID, helpText)
}

// reply 发送回复消息
func (s *TelegramService) reply(chatID int64, text string) {
	bot := s.getBot()
	if bot == nil {
		log.Println("[Telegram] Bot is nil, cannot send reply")
		return
	}
	log.Printf("[Telegram] Sending reply to chat %d: %s", chatID, truncateString(text, 100))
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "Markdown"
	_, err := bot.Send(msg)
	if err != nil {
		log.Printf("[Telegram] Markdown parse failed, retrying as plain text: %v", err)
		msg.ParseMode = ""
		_, err = bot.Send(msg)
		if err != nil {
			log.Printf("[Telegram] Failed to send message: %v", err)
		} else {
			log.Printf("[Telegram] Message sent successfully (plain text)")
		}
	} else {
		log.Printf("[Telegram] Message sent successfully")
	}
}

// truncateString 截断字符串
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// getUserByChatID 通过 Telegram Chat ID 获取用户
func (s *TelegramService) getUserByChatID(chatID int64) (*model.User, error) {
	var user model.User
	result := DB.Where("telegram_chat_id = ?", chatID).First(&user)
	if result.Error != nil {
		return nil, result.Error
	}
	return &user, nil
}

// GenerateBindCode 为用户生成绑定码
func (s *TelegramService) GenerateBindCode(userID uint) (*model.TelegramBindCodeResponse, error) {
	var user model.User
	if err := DB.First(&user, userID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	if user.TelegramChatID != nil {
		return nil, errors.New("Telegram 已绑定，请先解绑")
	}

	code, err := generateRandomCode(6)
	if err != nil {
		return nil, errors.New("生成绑定码失败")
	}

	expiresAt := time.Now().Add(10 * time.Minute)

	updates := map[string]interface{}{
		"telegram_bind_code":           code,
		"telegram_bind_code_expires_at": expiresAt,
	}

	if err := DB.Model(&user).Updates(updates).Error; err != nil {
		return nil, errors.New("保存绑定码失败")
	}

	botName := ""
	if bot := s.getBot(); bot != nil {
		botName = bot.Self.UserName
	}

	return &model.TelegramBindCodeResponse{
		BindCode:  code,
		ExpiresAt: expiresAt.Unix(),
		BotName:   botName,
	}, nil
}

// GetTelegramStatus 获取用户的 Telegram 绑定状态
func (s *TelegramService) GetTelegramStatus(userID uint) (*model.TelegramStatusResponse, error) {
	var user model.User
	if err := DB.First(&user, userID).Error; err != nil {
		return nil, errors.New("用户不存在")
	}

	if user.TelegramChatID == nil {
		return &model.TelegramStatusResponse{IsBound: false}, nil
	}

	tgUsername := ""
	if user.TelegramUsername != nil {
		tgUsername = *user.TelegramUsername
	}

	return &model.TelegramStatusResponse{
		IsBound:          true,
		TelegramUsername: tgUsername,
	}, nil
}

// VerifyWebhookSecret 校验 Telegram webhook 的 X-Telegram-Bot-Api-Secret-Token 请求头。
// 未配置 telegram_webhook_secret 时返回 true (兼容未设置 secret 的存量部署), 配置后则强制校验
func (s *TelegramService) VerifyWebhookSecret(received string) bool {
	secret, err := s.sysCfgService.GetValue("telegram_webhook_secret")
	if err != nil || secret == "" {
		return true
	}
	if received == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(received), []byte(secret)) == 1
}

// UnbindTelegram 解绑用户的 Telegram
func (s *TelegramService) UnbindTelegram(userID uint) error {
	var user model.User
	if err := DB.First(&user, userID).Error; err != nil {
		return errors.New("用户不存在")
	}

	if user.TelegramChatID == nil {
		return errors.New("Telegram 未绑定")
	}

	updates := map[string]interface{}{
		"telegram_chat_id":            nil,
		"telegram_username":           nil,
		"telegram_bind_code":          nil,
		"telegram_bind_code_expires_at": nil,
	}

	return DB.Model(&user).Updates(updates).Error
}

// generateRandomCode 生成随机数字码 (crypto/rand, 不可预测)
func generateRandomCode(length int) (string, error) {
	code := make([]byte, length)
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range code {
		code[i] = byte('0' + buf[i]%10)
	}
	return string(code), nil
}
