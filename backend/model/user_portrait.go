package model

import "time"

// UserPortrait 用户画像(每用户一条), 由聊天会话抽取/合并而来
type UserPortrait struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	UserID            uint      `json:"user_id" gorm:"uniqueIndex"`
	Summary           string    `json:"summary" gorm:"type:text"`
	Dimensions        string    `json:"dimensions" gorm:"type:json"` // JSON object
	Tags              string    `json:"tags" gorm:"type:json"`       // JSON array
	Confidence        float64   `json:"confidence"`
	IsUserEdited      bool      `json:"is_user_edited"`
	ExtractionEnabled bool      `json:"extraction_enabled"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (UserPortrait) TableName() string {
	return "user_portraits"
}

type UpdateUserPortraitRequest struct {
	Summary           *string            `json:"summary"`
	Dimensions        *map[string]string `json:"dimensions"`
	Tags              *[]string          `json:"tags"`
	ExtractionEnabled *bool              `json:"extraction_enabled"`
}
