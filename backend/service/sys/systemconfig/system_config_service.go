package systemconfig

import (
	"strconv"
	"strings"
	"sync"

	iface "backend/interface"
	"backend/model"
	"backend/service/kernel"

	"gorm.io/gorm"
)

// DB 本包专用数据库句柄, 由 main.go 接线赋值 (systemconfig.DB = db)。
// 子包内部所有 DB. 调用点无需改动。
var DB *gorm.DB

// 编译期断言: 确认本包服务满足 learning 集群所需的窄接口 (原 service/interface_asserts.go)。
var _ iface.ConfigValueGetter = (*SystemConfigService)(nil)

// Mem0Config mem0 服务配置
type Mem0Config struct {
	Enabled bool
	APIKey  string
	BaseURL string
}

// TimeoutConfig 超时配置 (真别名, 与 kernel.TimeoutConfig 是同一类型; 原定义抽到共享层)
type TimeoutConfig = kernel.TimeoutConfig

// TimeoutConfigKeys 超时相关的 system_config 键 (订阅热刷新与 loadTimeoutConfigFromDB 共用)
var TimeoutConfigKeys = []string{
	"ai_timeout_minutes",
	"ai_tls_handshake_timeout",
	"ai_response_header_timeout",
	"http_timeout_seconds",
}

// configSubscription 配置变更订阅: 键命中即触发回调 (回调自身决定同步/异步执行)
type configSubscription struct {
	keys     []string
	onChange func()
}

type SystemConfigService struct {
	mu           sync.RWMutex
	timeoutCache *TimeoutConfig

	subMu sync.RWMutex
	subs  []configSubscription
}

func NewSystemConfigService() *SystemConfigService {
	s := &SystemConfigService{}
	// 自订阅超时键: 配置页保存后立即刷新本服务缓存 (原 api 分发块的 ReloadTimeoutConfig 语义)
	s.Subscribe(TimeoutConfigKeys, func() { s.ReloadTimeoutConfig() })
	return s
}

// Subscribe 注册配置变更订阅: keys 里任一键被保存后触发 onChange。
// 各业务在构造时自注册 (键清单与读取逻辑同源), 新增配置键无需改 api 层。
func (s *SystemConfigService) Subscribe(keys []string, onChange func()) {
	s.subMu.Lock()
	defer s.subMu.Unlock()
	s.subs = append(s.subs, configSubscription{keys: keys, onChange: onChange})
}

// NotifyChanged 配置保存后由调用方 (system-config Update 接口) 逐键触发: 命中的订阅回调执行。
// 回调同步执行, 需要异步的业务在注册时自己包 go/协程 (与原 api 分发的同步/异步语义一致)。
func (s *SystemConfigService) NotifyChanged(keys ...string) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()
	for _, sub := range s.subs {
		for _, want := range sub.keys {
			hit := false
			for _, got := range keys {
				if got == want {
					hit = true
					break
				}
			}
			if hit {
				sub.onChange()
				break
			}
		}
	}
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

// GetByPrefix 批量读取某前缀的全部配置键, 返回 map (键名去掉前缀)。
// 适合一组键整体读取的场景 (如 wol_mqtt_*), 替代逐键 GetValue 的重复模式。
func (s *SystemConfigService) GetByPrefix(prefix string) map[string]string {
	all, err := s.GetAll()
	if err != nil {
		return map[string]string{}
	}
	out := make(map[string]string, len(all))
	for _, row := range all {
		if strings.HasPrefix(row.Key, prefix) {
			out[strings.TrimPrefix(row.Key, prefix)] = row.Value
		}
	}
	return out
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
