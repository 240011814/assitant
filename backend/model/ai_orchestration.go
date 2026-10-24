package model

import "time"

// AIOrchestration 编排视图。
// 存储已合并进 ai_agents 表 (agent_type='orchestration'), 本结构仅用于 API 出入参,
// 不再映射独立数据表。
type AIOrchestration struct {
	ID               int       `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Definition       string    `json:"definition"`
	Version          int       `json:"version"`
	Enabled          bool      `json:"enabled"`
	LastDebugSummary string    `json:"last_debug_summary"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CreateAIOrchestrationRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Definition  string `json:"definition" binding:"required"`
	Enabled     *bool  `json:"enabled"`
}

type UpdateAIOrchestrationRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Definition  *string `json:"definition"`
	Enabled     *bool   `json:"enabled"`
}

// ChatTurn 一轮对话 (多轮调试用)
type ChatTurn struct {
	// Role user / assistant
	Role    string `json:"role"`
	Content string `json:"content"`
}

// DebugRunRequest 调试执行请求
// id 为空时按 definition 草稿直接执行 (画布未保存也能调试);
// history 为之前轮次 (不含本轮 input), 用于多轮对话式调试
type DebugRunRequest struct {
	ID         *int       `json:"id"`
	Definition string     `json:"definition"`
	Input      string     `json:"input" binding:"required"`
	History    []ChatTurn `json:"history"`
	// HistoryID 编排对话已落库的会话 ID (首轮为 0); chat_first_turn 会话状态按它存取
	HistoryID uint `json:"history_id"`
	// Params 编排入参值 (按 DSL 声明的 params 名传值), 模板以 {{vars.<名>}} 引用
	Params map[string]string `json:"params,omitempty"`
	// SkipSummary 为 true 时不回写 last_debug_summary。
	// 训练中心的"编排对话"复用调试运行时置 true, 避免覆盖 Agent Studio 的最近调试摘要。
	SkipSummary bool `json:"skip_summary"`
	// ChatMode 标记本次运行来自训练中心「编排对话」: 把编排名称/简介注入主 Agent 系统提示词,
	// 让模型从第一轮就知道自己的身份与职责 (此前这段内容只在前端欢迎气泡里, 模型从未见过)。
	// Agent Studio 的调试运行不置位, 保持画布定义的原样测试。
	ChatMode bool `json:"-"`
}

// OrchChatState 编排对话的会话级状态 (chat_first_turn 机制):
// 会话"激活"后, 标记 chat_first_turn 的节点在后续轮次跳过执行、粘性分支回放记录的路由,
// 避免训练型编排每轮重跑场景生成/意图解析。按 history_id 一行。
type OrchChatState struct {
	HistoryID uint `gorm:"primaryKey" json:"history_id"`
	OrchID    uint `gorm:"index" json:"orch_id"`
	Active    bool `json:"active"`
	// Routes 粘性分支 (chat_first_turn 的 branch/router) 本轮实际选择的目标: 节点ID -> target
	Routes string `gorm:"type:text" json:"routes"`
	// Artifacts 会话存档 (节点 save_state 的跨轮产物): JSON map 名称 -> 内容,
	// 下一轮编译种子进 vars ({{vars.<名>}} 引用), 典型如场景事实卡
	Artifacts string    `gorm:"type:text" json:"artifacts"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (OrchChatState) TableName() string { return "orch_chat_states" }

// OrchRun 编排运行历史: 调试/对话/单节点试跑每次运行一行, 供事后复盘与用量统计
type OrchRun struct {
	ID     uint `gorm:"primaryKey" json:"id"`
	OrchID uint `gorm:"index" json:"orch_id"`
	UserID uint `gorm:"index" json:"user_id"`
	// Source 触发来源: debug / chat / node
	Source  string `json:"source"`
	Status  string `json:"status"` // success / error
	Mode    string `json:"mode"`   // chain / graph / workflow
	TotalMS int64  `json:"total_ms"`
	Error   string `json:"error,omitempty"`
	// Summary 运行摘要 JSON (DebugRunResult: 节点 trace/输出/token), 供回看
	Summary   string    `gorm:"type:mediumtext" json:"summary"`
	CreatedAt time.Time `gorm:"index" json:"created_at"`
}

func (OrchRun) TableName() string { return "orch_runs" }

// OrchRun 来源与状态常量
const (
	OrchRunSourceDebug = "debug"
	OrchRunSourceChat  = "chat"
	OrchRunSourceNode  = "node"

	OrchRunStatusSuccess = "success"
	OrchRunStatusError   = "error"
)

// OrchTemplate 用户自定义编排模板: 从当前画布"存为模板", 新建时复用
type OrchTemplate struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"index" json:"user_id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Definition  string    `gorm:"type:mediumtext" json:"definition"`
	CreatedAt   time.Time `json:"created_at"`
}

func (OrchTemplate) TableName() string { return "orch_templates" }
