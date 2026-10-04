package api

import (
	"context"
	"fmt"
	"time"

	iface "backend/interface"
	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

type SystemConfigHandler struct {
	configSvc       *service.SystemConfigService
	telegramService *service.TelegramService
	aiAgentSvc      *service.AIAgentService
	emailNotifier   *service.EmailNotifier
	docSvc          *service.UserDocumentService
	ragSvc          *service.RagService
}

func NewSystemConfigHandler(configSvc *service.SystemConfigService, telegramService *service.TelegramService, emailNotifier *service.EmailNotifier, aiAgentSvc ...*service.AIAgentService) *SystemConfigHandler {
	h := &SystemConfigHandler{
		configSvc:       configSvc,
		telegramService: telegramService,
		emailNotifier:   emailNotifier,
	}
	if len(aiAgentSvc) > 0 {
		h.aiAgentSvc = aiAgentSvc[0]
	}
	return h
}

// SetUserDocumentService 注入用户文档服务 (S3 存储配置变更后热刷新用)
func (h *SystemConfigHandler) SetUserDocumentService(docSvc *service.UserDocumentService) {
	h.docSvc = docSvc
}

// SetRAGService 注入 RAG 服务 (rag_* 配置变更后热刷新 + 嵌入测试连接用)
func (h *SystemConfigHandler) SetRAGService(ragSvc *service.RagService) {
	h.ragSvc = ragSvc
}

func (h *SystemConfigHandler) GetAll(c *gin.Context) {
	configs, err := h.configSvc.GetAll()
	if err != nil {
		SendError(c, "500", "获取配置失败: "+err.Error())
		return
	}
	SendSuccess(c, configs)
}

func (h *SystemConfigHandler) Update(c *gin.Context) {
	var req struct {
		Key    string `json:"key" binding:"required"`
		Value  string `json:"value"`
		Remark string `json:"remark"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.configSvc.SetValue(req.Key, req.Value, req.Remark); err != nil {
		SendError(c, "500", "更新配置失败: "+err.Error())
		return
	}

	// 当关闭 2FA 时，清除所有用户的 TOTP 密钥，确保重新开启时需要重新绑定
	if req.Key == "admin_2fa_enabled" && req.Value == "false" {
		service.DB.Model(&model.User{}).Where("totp_secret IS NOT NULL").Update("totp_secret", nil)
	}

	// Telegram Bot Token 变更后重启 Bot
	if req.Key == "telegram_bot_token" || req.Key == "telegram_enabled" || req.Key == "telegram_webhook_url" {
		if h.telegramService != nil {
			go h.telegramService.RestartBot()
		}
	}

	// 超时配置变更后重新加载配置缓存和 AI 服务
	timeoutKeys := []string{"ai_timeout_minutes", "ai_tls_handshake_timeout", "ai_response_header_timeout", "http_timeout_seconds"}
	for _, key := range timeoutKeys {
		if req.Key == key {
			h.configSvc.ReloadTimeoutConfig()
			if h.aiAgentSvc != nil {
				go h.aiAgentSvc.ReloadConfig()
			}
			break
		}
	}

	// SMTP 配置变更后刷新缓存
	smtpKeys := []string{"smtp_host", "smtp_port", "smtp_encryption", "smtp_user", "smtp_password", "smtp_from", "smtp_from_name"}
	for _, key := range smtpKeys {
		if req.Key == key {
			h.emailNotifier.RefreshConfig()
			break
		}
	}

	// S3 存储配置变更后热刷新用户文档存储连接
	s3Keys := []string{"s3_enabled", "s3_endpoint", "s3_region", "s3_bucket", "s3_access_key", "s3_secret_key", "s3_secure", "s3_use_path_style", "s3_max_upload_mb"}
	for _, key := range s3Keys {
		if req.Key == key {
			if h.docSvc != nil {
				go h.docSvc.RefreshStorageConfig()
			}
			break
		}
	}

	// RAG (文档语义检索) 配置变更后热刷新
	ragKeys := []string{"rag_enabled", "rag_embedding_base_url", "rag_embedding_model", "rag_embedding_api_key", "rag_chunk_size", "rag_chunk_overlap", "rag_top_k"}
	for _, key := range ragKeys {
		if req.Key == key {
			if h.ragSvc != nil {
				go h.ragSvc.RefreshConfig()
			}
			break
		}
	}

	SendSuccess(c, nil)
}

// GetRegisterStatus 公开接口，供登录页检查注册是否开启
func (h *SystemConfigHandler) GetRegisterStatus(c *gin.Context) {
	val, err := h.configSvc.GetValue("register_enabled")
	if err != nil {
		SendSuccess(c, gin.H{"enabled": true})
		return
	}
	SendSuccess(c, gin.H{"enabled": val == "true"})
}

// HandleTestEmbedding 测试嵌入模型连通性 (OpenAI 兼容 /embeddings, 如本地 Ollama), 返回向量维度
func (h *SystemConfigHandler) HandleTestEmbedding(c *gin.Context) {
	var req struct {
		BaseURL string `json:"base_url"`
		Model   string `json:"model"`
		APIKey  string `json:"api_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	if h.ragSvc == nil {
		SendError(c, "500", "RAG 服务未初始化")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()
	dims, err := h.ragSvc.TestEmbedding(ctx, req.BaseURL, req.Model, req.APIKey)
	if err != nil {
		SendError(c, "500", "嵌入测试失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"dims": dims, "message": fmt.Sprintf("连接成功, 向量维度 %d", dims)})
}

// SendTestEmail 发送测试邮件
func (h *SystemConfigHandler) SendTestEmail(c *gin.Context) {
	var req struct {
		Email   string `json:"email" binding:"required,email"`
		Subject string `json:"subject"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请输入有效的邮箱地址")
		return
	}

	subject := req.Subject
	if subject == "" {
		subject = "测试邮件"
	}
	content := req.Content
	if content == "" {
		content = "这是一封 SMTP 邮件服务的测试邮件。\n\n如果您收到此邮件，说明 SMTP 配置正确。"
	}

	msg := iface.NotifyMessage{
		Subject: subject,
		Body:    content,
	}

	if err := h.emailNotifier.Send(req.Email, msg); err != nil {
		SendError(c, "500", "发送测试邮件失败: "+err.Error())
		return
	}

	SendSuccess(c, gin.H{"message": "测试邮件已发送"})
}
