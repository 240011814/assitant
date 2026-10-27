package interfaces

import "backend/model"

// 本文件定义 learning 集群 (分级阅读/RSS/词汇/测验/课程/错题本/FSRS) 对集群外服务
// 所需的最小能力面。learning 只 import model + 已下沉的共享包 (kernel/llmjson/prompt),
// 通过这些接口解耦对 ai/user/system-config 具体服务的依赖, 从而可独立成包且不成环。

// PreferenceGetter 用户偏好读取能力 (由 service.UserPreferenceService 实现)。
// learning 目前只用来读 english_daily_new_limit (每日新增单词上限)。
type PreferenceGetter interface {
	// GetPreference 读取用户指定 key 的偏好项 (不存在时返回 nil, nil)
	GetPreference(userID uint, key string) (*model.UserPreference, error)
}

// ConfigValueGetter 系统配置读取能力 (由 service.SystemConfigService 实现)。
// learning 用来读 rss_allow_private_network (私网抓取开关) 与 flaresolverr_url (反爬回退)。
type ConfigValueGetter interface {
	// GetValue 读取 system_config 指定 key 的值 (不存在时返回 error)
	GetValue(key string) (string, error)
}
