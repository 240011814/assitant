package model

import (
	"time"
)

// ModelScenario 模型和场景配置
type ModelScenario struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Type        string    `gorm:"size:20;not null;index" json:"type"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	Summary     string    `gorm:"size:500" json:"summary"`
	Description string    `gorm:"type:text" json:"description"`
	Detail      string    `gorm:"type:text" json:"detail"`
	Category    string    `gorm:"size:50" json:"category"`
	SortOrder   int       `gorm:"default:0" json:"sortOrder"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (ModelScenario) TableName() string {
	return "model_scenario"
}

// CreateModelScenarioRequest 创建请求
type CreateModelScenarioRequest struct {
	Type        string `json:"type" binding:"required"`
	Name        string `json:"name" binding:"required"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Detail      string `json:"detail"`
	Category    string `json:"category"`
	SortOrder   int    `json:"sortOrder"`
}

// UpdateModelScenarioRequest 更新请求
type UpdateModelScenarioRequest struct {
	Name        *string `json:"name"`
	Summary     *string `json:"summary"`
	Description *string `json:"description"`
	Detail      *string `json:"detail"`
	Category    *string `json:"category"`
	SortOrder   *int    `json:"sortOrder"`
}
