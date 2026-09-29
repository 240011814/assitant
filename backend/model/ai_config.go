package model

import "time"

type AIProvider struct {
	ID        int    `json:"id" gorm:"primaryKey"`
	Name      string `json:"name"`
	APIKey    string `json:"api_key"`                        // 列表接口返回空串, 明文不外泄
	MaskedKey string `json:"masked_api_key" gorm:"-"`        // 掩码展示用, 仅列表接口填充
	BaseURL   string `json:"base_url"`
	IsActive  bool   `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Models    []AIModel `json:"models" gorm:"foreignKey:ProviderID"`
}

type AIModel struct {
	ID          int       `json:"id" gorm:"primaryKey"`
	ProviderID  int       `json:"provider_id"`
	ModelCode   string    `json:"model_code"`
	DisplayName string    `json:"display_name"`
	IsDefault   bool      `json:"is_default"`
	ConfigJSON  string    `json:"config_json"` // JSON string for parameters
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (AIProvider) TableName() string {
	return "ai_providers"
}

func (AIModel) TableName() string {
	return "ai_models"
}
