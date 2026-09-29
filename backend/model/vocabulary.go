package model

import (
	"time"
)

// Vocabulary 生词实体
type Vocabulary struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         uint       `gorm:"not null;index" json:"userId"`
	Word           string     `gorm:"size:100;not null;index" json:"word"`
	Phonetic       string     `gorm:"size:100" json:"phonetic"`
	Definition     string     `gorm:"type:text" json:"definition"`
	Example        string     `gorm:"type:text" json:"example"`
	SourceContext  string     `gorm:"type:text" json:"sourceContext"`
	ConfusingWords string     `gorm:"type:text" json:"confusingWords"`
	IsMastered     bool       `gorm:"default:false" json:"isMastered"`
	SrsBox         int        `gorm:"not null;default:0" json:"srsBox"` // SRS 盒子 0~5
	NextReviewAt   *time.Time `json:"nextReviewAt"`                     // 下次复习时间 (NULL=未进入复习流程)
	LastReviewedAt *time.Time `json:"lastReviewedAt"`                   // 最近复习时间
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// TableName 指定表名
func (Vocabulary) TableName() string {
	return "vocabulary"
}

// CreateVocabularyRequest 创建生词请求
type CreateVocabularyRequest struct {
	Word           string `json:"word" binding:"required"`
	Phonetic       string `json:"phonetic"`
	Definition     string `json:"definition"`
	Example        string `json:"example"`
	SourceContext  string `json:"sourceContext"`
	ConfusingWords string `json:"confusingWords"`
}

// UpdateVocabularyRequest 更新生词请求
type UpdateVocabularyRequest struct {
	Phonetic       string `json:"phonetic"`
	Definition     string `json:"definition"`
	Example        string `json:"example"`
	ConfusingWords string `json:"confusingWords"`
	IsMastered     *bool  `json:"isMastered"`
}
