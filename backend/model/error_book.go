package model

import (
	"time"
)

// ErrorBook 错题本实体
type ErrorBook struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint      `gorm:"not null;index" json:"userId"`
	ContentType string    `gorm:"size:20;not null" json:"contentType"` // word 或 sentence
	Content     string    `gorm:"type:text;not null" json:"content"`   // 单词或句子内容
	Translation string    `gorm:"type:text" json:"translation"`        // 中文翻译
	SourceType  string    `gorm:"size:20" json:"sourceType"`           // vocabulary 或 course
	SourceID    uint      `gorm:"index" json:"sourceId"`               // 原始记录ID
	ErrorCount  int       `gorm:"default:1" json:"errorCount"`         // 错误次数
	IsMastered  bool      `gorm:"default:false" json:"isMastered"`     // 是否已掌握
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// TableName 指定表名
func (ErrorBook) TableName() string {
	return "error_book"
}

// CreateErrorBookRequest 创建错题请求
type CreateErrorBookRequest struct {
	ContentType string `json:"contentType" binding:"required"`
	Content     string `json:"content" binding:"required"`
	Translation string `json:"translation"`
	SourceType  string `json:"sourceType"`
	SourceID    uint   `json:"sourceId"`
}

// UpdateErrorBookRequest 更新错题请求
type UpdateErrorBookRequest struct {
	IsMastered *bool `json:"isMastered"`
}
