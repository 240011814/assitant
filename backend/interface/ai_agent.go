package interfaces

import "time"

// TextGenerator AI 文本生成能力 (由 service.AIAgentService 实现)。
// 依赖此窄接口而非具体 *AIAgentService, 可切断 user/learning ↔ ai 的相互 import。
// user 记忆抽取/画像合并用 HasActiveModel+GenerateText; learning (标注/查词/批改/出题) 还需 GenerateTextWithTimeout。
type TextGenerator interface {
	// HasActiveModel 是否已配置可用的 AI 模型
	HasActiveModel() bool
	// GenerateText 用指定系统提示词与用户输入生成文本 (modelOverride 为空用默认模型)
	GenerateText(userID uint, modelOverride, systemPrompt, userPrompt string) (string, error)
	// GenerateTextWithTimeout 同 GenerateText 但指定单次调用超时 (慢任务用)
	GenerateTextWithTimeout(userID uint, modelOverride, systemPrompt, userPrompt string, timeout time.Duration) (string, error)
}

// ProfilePromptBuilder 用户画像提示词构造能力 (由 service.UserMemoryService 实现)。
// ai 对话注入画像只需要 BuildProfilePrompt 一个方法, 依赖此窄接口而非具体
// *UserMemoryService (更不依赖 4 方法的 UserMemoryService 全接口), 切断 ai→user 的 import。
type ProfilePromptBuilder interface {
	// BuildProfilePrompt 生成用于注入对话上下文的用户画像提示词
	BuildProfilePrompt(userID uint) string
}
