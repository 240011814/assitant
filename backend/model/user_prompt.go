package model

import "time"

// UserPrompt 用户自定义提示词模型
// 编排 Agent 节点的提示词版本也存本表: AgentID=编排 id, NodeKey=画布节点 id
// (普通用户提示词 NodeKey 恒为空串)
type UserPrompt struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	UserID            uint      `gorm:"not null;index:idx_user_agent" json:"user_id"`
	AgentID           uint      `gorm:"not null;index:idx_user_agent" json:"agent_id"`
	NodeKey           string    `gorm:"size:64;not null;default:''" json:"node_key"`
	CustomPrompt      string    `gorm:"type:text;not null" json:"custom_prompt"`
	MemorySearchQuery string    `gorm:"size:500;default:''" json:"memory_search_query"`
	MemorySearchTopK  int       `gorm:"default:30" json:"memory_search_top_k"`
	Version           int       `gorm:"not null;default:1" json:"version"`
	IsActive          bool      `gorm:"default:false" json:"is_active"`
	Remark            string    `gorm:"size:255" json:"remark"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}
