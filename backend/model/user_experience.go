package model

import (
	"encoding/json"
	"strings"
	"time"
)

// 记忆等级
const (
	MemoryLevelCore      = "core"      // 长期稳定, 基本不变
	MemoryLevelLongTerm  = "long_term" // 预计保留半年以上
	MemoryLevelTemporary = "temporary" // 临时信息, 不进入画像
)

// 经历进度状态
const (
	ExperienceStatusPlanned   = "planned"
	ExperienceStatusOngoing   = "ongoing"
	ExperienceStatusCompleted = "completed"
	ExperienceStatusAbandoned = "abandoned"
	ExperienceStatusPaused    = "paused"
	ExperienceStatusUnknown   = "unknown"
)

// ProfileFact 画像事实(抽取出的一条稳定信息)
type ProfileFact struct {
	Content     string  `json:"content"`
	MemoryLevel string  `json:"memory_level,omitempty"`
	Confidence  float64 `json:"confidence,omitempty"`
	Evidence    string  `json:"evidence,omitempty"`
}

// UnmarshalJSON 兼容 LLM 输出字符串或对象两种形态
func (f *ProfileFact) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, `"`) {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		f.Content = s
		return nil
	}
	type plain ProfileFact
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	*f = ProfileFact(p)
	return nil
}

// UserExperience 用户经历, 一段会话可对应多条(按 history_id + title 去重)
type UserExperience struct {
	ID           uint       `json:"id" gorm:"primaryKey"`
	UserID       uint       `json:"user_id" gorm:"index"`
	HistoryID    *uint      `json:"history_id" gorm:"index"`
	Domain       string     `json:"domain" gorm:"size:32"`
	EventType    string     `json:"event_type" gorm:"size:32"`
	Title        string     `json:"title" gorm:"size:255"`
	Content      string     `json:"content" gorm:"type:text"`
	TimeRange    string     `json:"time_range" gorm:"size:64"`
	OccurredAt   *time.Time `json:"occurred_at"`
	Tags         string     `json:"tags" gorm:"type:json"` // JSON array
	Confidence   float64    `json:"confidence"`
	Importance   int        `json:"importance" gorm:"default:3"`
	MemoryLevel  string     `json:"memory_level" gorm:"size:20;default:long_term"`
	Evidence     string     `json:"evidence" gorm:"type:text"`
	Status       string     `json:"status" gorm:"size:20;default:unknown"`
	IsUserEdited bool       `json:"is_user_edited"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (UserExperience) TableName() string {
	return "user_experiences"
}

type CreateUserExperienceRequest struct {
	Domain      string     `json:"domain"`
	EventType   string     `json:"event_type"`
	Title       string     `json:"title" binding:"required"`
	Content     string     `json:"content"`
	TimeRange   string     `json:"time_range"`
	OccurredAt  *time.Time `json:"occurred_at"`
	Tags        []string   `json:"tags"`
	MemoryLevel string     `json:"memory_level"`
	Evidence    string     `json:"evidence"`
	Status      string     `json:"status"`
	Importance  int        `json:"importance"`
}

type UpdateUserExperienceRequest struct {
	Domain      *string    `json:"domain"`
	EventType   *string    `json:"event_type"`
	Title       *string    `json:"title"`
	Content     *string    `json:"content"`
	TimeRange   *string    `json:"time_range"`
	OccurredAt  *time.Time `json:"occurred_at"`
	Tags        *[]string  `json:"tags"`
	MemoryLevel *string    `json:"memory_level"`
	Evidence    *string    `json:"evidence"`
	Status      *string    `json:"status"`
	Importance  *int       `json:"importance"`
}
