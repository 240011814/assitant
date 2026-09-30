package model

import (
	"strings"
	"time"
)

// Agent 类型: 决定该 Agent 能出现在哪里
const (
	// AIAgentTypeChat 对话型 Agent: 只用于 AI 对话/训练页
	AIAgentTypeChat = "chat"
	// AIAgentTypeSubAgent 子Agent: 可被 Agent Studio 编排中的主 Agent 委派调用
	AIAgentTypeSubAgent = "subagent"
)

// DelegatableAgentTypes 可作为编排子Agent 的类型
func DelegatableAgentTypes() []string {
	return []string{AIAgentTypeSubAgent}
}

// AIAgentSubAgentPermission 子Agent 统一权限码: 所有子Agent 共用一个 code。
// 拥有它的用户才能新建/编辑/删除子Agent (agent_type='subagent'),
// 也只有拥有它的用户才能在 Agent Studio 编排里看到并引用子Agent。
const AIAgentSubAgentPermission = "ai:subagent:manage"

// IsSubAgentType 判断请求里的 Agent 类型是否为子Agent (容忍首尾空白)
func IsSubAgentType(t string) bool {
	return strings.TrimSpace(t) == AIAgentTypeSubAgent
}

type AIAgent struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	UserID           uint      `json:"user_id"`
	IsPublic         bool      `json:"is_public"`
	PermissionCode   string    `json:"permission_code"`
	Title            string    `json:"title"`
	Description      string    `json:"description"`
	Code             string    `json:"code"`
	SystemPrompt     string    `json:"system_prompt"`
	Icon             string    `json:"icon"`
	Color            string    `json:"color"`
	InitialMessage   string    `json:"initial_message"`
	InputPlaceholder string    `json:"input_placeholder"`
	SpeechLang       string    `json:"speech_lang"`
	SpeechRate       float64   `json:"speech_rate"`
	IsFavorite       bool      `json:"is_favorite"`
	// AgentType 类型: chat(对话, 默认) / subagent(可作为编排子Agent 被委派)
	AgentType string `json:"agent_type"`
	// DelegationDescription 委派说明: 主 Agent 判断"何时该委派给它"的依据,
	// 编译期会写进主 Agent 的委派指引 (留空则退化为用 Description/Title)
	DelegationDescription string    `json:"delegation_description"`
	CreatedAt             time.Time `json:"created_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

type CreateAIAgentRequest struct {
	Title            string  `json:"title" binding:"required"`
	Description      string  `json:"description"`
	Code             string  `json:"code"`
	SystemPrompt     string  `json:"system_prompt" binding:"required"`
	Icon             string  `json:"icon"`
	Color            string  `json:"color"`
	InitialMessage   string  `json:"initial_message"`
	InputPlaceholder string  `json:"input_placeholder"`
	SpeechLang       string  `json:"speech_lang"`
	SpeechRate       float64 `json:"speech_rate"`
	// AgentType 空=chat
	AgentType string `json:"agent_type"`
	// DelegationDescription 仅子Agent 有意义
	DelegationDescription string `json:"delegation_description"`
}

type UpdateAIAgentRequest struct {
	Title            string  `json:"title"`
	Description      string  `json:"description"`
	SystemPrompt     string  `json:"system_prompt"`
	Icon             string  `json:"icon"`
	Color            string  `json:"color"`
	InitialMessage   string  `json:"initial_message"`
	InputPlaceholder string  `json:"input_placeholder"`
	SpeechLang       string  `json:"speech_lang"`
	SpeechRate       float64 `json:"speech_rate"`
	// 指针: 允许把它们改回空值/chat
	AgentType             *string `json:"agent_type"`
	DelegationDescription *string `json:"delegation_description"`
}
