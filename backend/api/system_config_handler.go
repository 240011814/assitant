package api

import (
	"backend/service/ai/prompt"
	"backend/service/ai/rag"
	"backend/service/ai/userdoc"
	"backend/service/notify/notifier"
	"backend/service/sys/systemconfig"
	"context"
	"fmt"
	"strings"
	"time"

	iface "backend/interface"

	"github.com/gin-gonic/gin"
)

// SystemConfigHandler 系统配置管理端点。配置变更的热刷新走 systemconfig 键订阅
// (各业务构造时自注册), 本 handler 不再持有各业务服务, 只留测试连通性端点需要的
// emailNotifier/ragSvc。
type SystemConfigHandler struct {
	configSvc     *systemconfig.SystemConfigService
	emailNotifier *notifier.EmailNotifier
	ragSvc        *rag.RagService
}

func NewSystemConfigHandler(configSvc *systemconfig.SystemConfigService, emailNotifier *notifier.EmailNotifier) *SystemConfigHandler {
	return &SystemConfigHandler{configSvc: configSvc, emailNotifier: emailNotifier}
}

// SetRAGService 注入 RAG 服务 (嵌入测试连接端点用)
func (h *SystemConfigHandler) SetRAGService(ragSvc *rag.RagService) {
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

	// 关闭 2FA 时同事务清除所有用户的 TOTP 密钥, 确保重新开启时需要重新绑定
	if req.Key == "admin_2fa_enabled" && req.Value == "false" {
		if err := h.configSvc.Disable2FAAndResetTOTP(req.Key, req.Value, req.Remark); err != nil {
			SendError(c, "500", "更新配置失败: "+err.Error())
			return
		}
	} else if err := h.configSvc.SetValue(req.Key, req.Value, req.Remark); err != nil {
		SendError(c, "500", "更新配置失败: "+err.Error())
		return
	}

	// prompt_* 内置提示词配置变更: 重建提示词缓存, 保存后所有 LLM 调用立即用新提示词
	if strings.HasPrefix(req.Key, "prompt_") {
		prompt.RefreshPromptCache()
	}

	// 其余热刷新走键订阅: telegram_/超时/smtp_/s3_/rag_/tts_* 各业务在构造时向
	// systemconfig 注册了回调, 这里统一分发 (键清单由各业务自持, 新增键不用改这里)
	h.configSvc.NotifyChanged(req.Key)

	SendSuccess(c, nil)
}

// HandleListPrompts 内置 AI 提示词列表 (当前生效值 + 内置默认值, 供配置页管理)
func (h *SystemConfigHandler) HandleListPrompts(c *gin.Context) {
	SendSuccess(c, prompt.ListPrompts())
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

// HandleTestS3 测试 S3/MinIO 连接 (使用表单当前值, 保存前即可验证)
func (h *SystemConfigHandler) HandleTestS3(c *gin.Context) {
	var req struct {
		Endpoint     string `json:"endpoint"`
		Region       string `json:"region"`
		Bucket       string `json:"bucket"`
		AccessKey    string `json:"access_key"`
		SecretKey    string `json:"secret_key"`
		Secure       bool   `json:"secure"`
		UsePathStyle bool   `json:"use_path_style"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	if req.Endpoint == "" || req.Bucket == "" || req.AccessKey == "" || req.SecretKey == "" {
		SendError(c, "400", "请先填写 endpoint / bucket / access_key / secret_key")
		return
	}

	endpoint, secure := userdoc.NormalizeS3Endpoint(req.Endpoint, req.Secure)
	cfg := userdoc.S3StorageConfig{
		Enabled:      true,
		Endpoint:     endpoint,
		Region:       req.Region,
		Bucket:       req.Bucket,
		AccessKey:    req.AccessKey,
		SecretKey:    req.SecretKey,
		Secure:       secure,
		UsePathStyle: req.UsePathStyle,
	}
	if err := userdoc.TestS3Connection(c.Request.Context(), cfg); err != nil {
		SendError(c, "500", "连接失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"message": "连接成功, 存储可用"})
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
		content = "这是一封 SMTP 邮件服务的测试邮件。如果您收到此邮件，说明 SMTP 配置正确。"
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
