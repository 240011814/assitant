package model

import "time"

type SystemConfig struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:100;not null;unique" json:"key"`
	Value     string    `gorm:"type:text;not null;default:''" json:"value"` // TEXT: prompt_* 键存数千字符的内置提示词
	Remark    string    `gorm:"size:255" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (SystemConfig) TableName() string {
	return "system_config"
}
