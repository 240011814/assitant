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
	SrsBox         int        `gorm:"not null;default:0" json:"srsBox"` // 旧 Leitner 盒子, 保留兼容, FSRS 后不再更新
	NextReviewAt   *time.Time `json:"nextReviewAt"`                     // 下次复习时间 (NULL=未进入复习流程)
	LastReviewedAt *time.Time `json:"lastReviewedAt"`                   // 最近复习时间
	FsrsStability  float64    `gorm:"not null;default:0" json:"fsrsStability"`
	FsrsDifficulty float64    `gorm:"not null;default:0" json:"fsrsDifficulty"`
	FsrsState      int        `gorm:"not null;default:0" json:"fsrsState"` // 0=New 1=Learning 2=Review 3=Relearning
	FsrsReps       int        `gorm:"not null;default:0" json:"fsrsReps"`
	FsrsLapses     int        `gorm:"not null;default:0" json:"fsrsLapses"`
	AssistExample  string     `gorm:"type:text" json:"assistExample"`  // 遗忘干预: AI 新例句
	AssistMnemonic string     `gorm:"type:text" json:"assistMnemonic"` // 遗忘干预: AI 助记提示
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

// FSRS 状态常量 (与 go-fsrs State 对齐)
const (
	FsrsStateNew        = 0
	FsrsStateLearning   = 1
	FsrsStateReview     = 2
	FsrsStateRelearning = 3
)

// ReviewDirection 复习方向: 双向卡 (en2zh 再认 / zh2en 主动回忆 / listen 听音辨词)
const (
	ReviewDirectionEn2Zh = "en2zh"
	ReviewDirectionZh2En = "zh2en"
	ReviewDirectionListen = "listen"
)

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
