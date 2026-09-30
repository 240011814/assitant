package model

import "time"

// AIOrchestration 存储可视化编排定义 (Agent Studio)
// definition 为编排 DSL JSON: {version, nodes[], edges[]}, 由后端校验并编译为
// Eino compose 的 Chain/Graph/Workflow 运行
type AIOrchestration struct {
	ID          int       `json:"id" gorm:"primaryKey"`
	Name        string    `json:"name" gorm:"uniqueIndex;size:100"`
	Description string    `json:"description" gorm:"type:text"`
	// Definition 编排 DSL JSON 原文
	Definition string `json:"definition" gorm:"type:longtext"`
	// Version 每次保存递增, 便于排查线上行为对应的定义版本
	Version int  `json:"version"`
	Enabled bool `json:"enabled"`
	// LastDebugSummary 最近一次调试运行的节点级摘要 JSON (可选回显)
	LastDebugSummary string    `json:"last_debug_summary" gorm:"type:text"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (AIOrchestration) TableName() string {
	return "ai_orchestrations"
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

// DebugRunRequest 调试执行请求
// id 为空时按 definition 草稿直接执行 (画布未保存也能调试)
type DebugRunRequest struct {
	ID         *int   `json:"id"`
	Definition string `json:"definition"`
	Input      string `json:"input" binding:"required"`
}
