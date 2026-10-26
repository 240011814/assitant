package service

import (
	iface "backend/interface"
)

// 编译期断言: 确认具体服务满足 learning 集群所需的各种窄接口。
// 拆包时若某方法签名变更, 此处会立即编译失败, 避免运行期才发现不匹配。
var (
	_ iface.TextGenerator        = (*AIAgentService)(nil)
	_ iface.UserDocumentStore    = (*UserDocumentService)(nil)
	_ iface.PreferenceGetter     = (*UserPreferenceService)(nil)
	_ iface.ConfigValueGetter    = (*SystemConfigService)(nil)
	_ iface.ProfilePromptBuilder = (*UserMemoryService)(nil)
)
