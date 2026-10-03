package model

import "time"

// Token 用量来源 (ai_token_usages.source)
const (
	// TokenSourceChat AI 对话 (训练页/聊天页)
	TokenSourceChat = "chat"
	// TokenSourceOrchDebug 编排画布调试运行
	TokenSourceOrchDebug = "orchestration_debug"
	// TokenSourceOrchChat 编排对话 (训练中心)
	TokenSourceOrchChat = "orchestration_chat"
	// TokenSourceExtraction 用户画像/经历抽取 (后台任务)
	TokenSourceExtraction = "extraction"
)

// AITokenUsage 用户 AI Token 用量明细: 每次模型调用一条 (ReAct 多轮工具调用的每轮各一条),
// 统计聚合在查询端完成
type AITokenUsage struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           uint      `json:"user_id"`
	// Model 模型 code; 空串 = 默认模型
	Model            string    `json:"model" gorm:"size:100"`
	// Source 来源: chat / orchestration_debug / orchestration_chat
	Source           string    `json:"source" gorm:"size:30"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	TotalTokens      int64     `json:"total_tokens"`
	CreatedAt        time.Time `json:"created_at"`
}

func (AITokenUsage) TableName() string { return "ai_token_usages" }

// TokenUsageSummary 统计区间汇总
type TokenUsageSummary struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	// Calls 模型调用次数 (明细行数)
	Calls int64 `json:"calls"`
	// Users 该区间内有用量的用户数
	Users int64 `json:"users"`
}

// TokenUsageTrendItem 时间趋势分桶项
type TokenUsageTrendItem struct {
	// Bucket 分桶键: 小时 "2026-10-02 14:00" / 日 "2026-10-02" / 月 "2026-10" / 年 "2026"
	Bucket           string `json:"bucket"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	Calls            int64  `json:"calls"`
}

// TokenUsageModelItem 按模型聚合项
type TokenUsageModelItem struct {
	Model            string `json:"model"`
	PromptTokens     int64  `json:"prompt_tokens"`
	CompletionTokens int64  `json:"completion_tokens"`
	TotalTokens      int64  `json:"total_tokens"`
	Calls            int64  `json:"calls"`
}

// TokenUsageUserItem 按用户聚合项 (带限额, 管理页展示使用比例)
type TokenUsageUserItem struct {
	UserID           uint    `json:"user_id"`
	Username         string  `json:"username"`
	Nickname         string  `json:"nickname"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Calls            int64   `json:"calls"`
	// QuotaMonth 月度限额 (NULL/0=不限); 仅 granularity 查询区间为"本月"时有意义, 原样返回
	QuotaMonth       *int    `json:"quota_month" gorm:"column:token_quota_month"`
}
