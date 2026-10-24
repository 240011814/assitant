package service

import (
	"backend/model"
	"strconv"
	"sync"

	"gorm.io/gorm"
)

// Mem0Config mem0 服务配置
type Mem0Config struct {
	Enabled bool
	APIKey  string
	BaseURL string
}

// TimeoutConfig 超时配置
type TimeoutConfig struct {
	AIRequestTimeout        int // AI 请求超时时间（分钟）
	AITLSHandshakeTimeout   int // AI TLS 握手超时时间（秒）
	AIResponseHeaderTimeout int // AI 响应头超时时间（秒）
	HTTPTimeout             int // HTTP 请求超时时间（秒），用于 AI 连接、Mem0、Telegram
}

type SystemConfigService struct {
	mu           sync.RWMutex
	timeoutCache *TimeoutConfig
}

func NewSystemConfigService() *SystemConfigService {
	return &SystemConfigService{}
}

func (s *SystemConfigService) GetAll() ([]model.SystemConfig, error) {
	var configs []model.SystemConfig
	err := DB.Find(&configs).Error
	return configs, err
}

func (s *SystemConfigService) GetValue(key string) (string, error) {
	var config model.SystemConfig
	err := DB.Where("`key` = ?", key).First(&config).Error
	if err != nil {
		return "", err
	}
	return config.Value, nil
}

func (s *SystemConfigService) SetValue(key, value, remark string) error {
	var cfg model.SystemConfig
	err := DB.Where("`key` = ?", key).First(&cfg).Error
	if err != nil {
		cfg = model.SystemConfig{Key: key, Value: value, Remark: remark}
		return DB.Create(&cfg).Error
	}
	cfg.Value = value
	if remark != "" {
		cfg.Remark = remark
	}
	return DB.Save(&cfg).Error
}

// Disable2FAAndResetTOTP 关闭 2FA 并在同一事务里清除全部用户的 TOTP 密钥:
// 两步分开执行时, 中途失败会留下"开关已关但旧密钥仍在"的不一致状态 (重新开启时旧密钥可被复用)
func (s *SystemConfigService) Disable2FAAndResetTOTP(key, value, remark string) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var cfg model.SystemConfig
		if err := tx.Where("`key` = ?", key).First(&cfg).Error; err != nil {
			cfg = model.SystemConfig{Key: key, Value: value, Remark: remark}
			if err := tx.Create(&cfg).Error; err != nil {
				return err
			}
		} else {
			cfg.Value = value
			if remark != "" {
				cfg.Remark = remark
			}
			if err := tx.Save(&cfg).Error; err != nil {
				return err
			}
		}
		return tx.Model(&model.User{}).Where("totp_secret IS NOT NULL").Update("totp_secret", nil).Error
	})
}

// GetMem0Config 从数据库读取 mem0 配置
func (s *SystemConfigService) GetMem0Config() Mem0Config {
	enabledStr, _ := s.GetValue("mem0_enabled")
	enabled := enabledStr != "false" // 默认开启
	apiKey, _ := s.GetValue("mem0_api_key")
	baseURL, _ := s.GetValue("mem0_base_url")
	if baseURL == "" {
		baseURL = "https://api.mem0.ai/v1"
	}
	return Mem0Config{
		Enabled: enabled,
		APIKey:  apiKey,
		BaseURL: baseURL,
	}
}

// GetTelegramBotToken 获取 Telegram Bot Token
func (s *SystemConfigService) GetTelegramBotToken() string {
	token, _ := s.GetValue("telegram_bot_token")
	return token
}

// IsTelegramEnabled 检查 Telegram Bot 是否启用
func (s *SystemConfigService) IsTelegramEnabled() bool {
	enabled, _ := s.GetValue("telegram_enabled")
	return enabled != "false" // 默认启用
}

// GetTelegramWebhookURL 获取 Telegram Webhook URL
func (s *SystemConfigService) GetTelegramWebhookURL() string {
	url, _ := s.GetValue("telegram_webhook_url")
	return url
}

// GetTimeoutConfig 获取超时配置（优先从缓存读取）
func (s *SystemConfigService) GetTimeoutConfig() TimeoutConfig {
	s.mu.RLock()
	if s.timeoutCache != nil {
		defer s.mu.RUnlock()
		return *s.timeoutCache
	}
	s.mu.RUnlock()

	// 缓存未命中，从数据库加载
	config := s.loadTimeoutConfigFromDB()

	s.mu.Lock()
	s.timeoutCache = &config
	s.mu.Unlock()

	return config
}

// ReloadTimeoutConfig 强制刷新超时配置缓存
func (s *SystemConfigService) ReloadTimeoutConfig() TimeoutConfig {
	config := s.loadTimeoutConfigFromDB()

	s.mu.Lock()
	s.timeoutCache = &config
	s.mu.Unlock()

	return config
}

// loadTimeoutConfigFromDB 从数据库读取超时配置
func (s *SystemConfigService) loadTimeoutConfigFromDB() TimeoutConfig {
	config := TimeoutConfig{
		AIRequestTimeout:        5,  // 默认5分钟
		AITLSHandshakeTimeout:   15, // 默认15秒
		AIResponseHeaderTimeout: 30, // 默认30秒
		HTTPTimeout:             30, // 默认30秒
	}

	if val, err := s.GetValue("ai_timeout_minutes"); err == nil {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			config.AIRequestTimeout = n
		}
	}
	if val, err := s.GetValue("ai_tls_handshake_timeout"); err == nil {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			config.AITLSHandshakeTimeout = n
		}
	}
	if val, err := s.GetValue("ai_response_header_timeout"); err == nil {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			config.AIResponseHeaderTimeout = n
		}
	}
	if val, err := s.GetValue("http_timeout_seconds"); err == nil {
		if n, err := strconv.Atoi(val); err == nil && n > 0 {
			config.HTTPTimeout = n
		}
	}

	return config
}
