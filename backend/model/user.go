package model

import (
	"time"
)

// User 用户实体类 (GORM Model)
type User struct {
	ID                      uint       `gorm:"primaryKey;autoIncrement" json:"userId"`
	Username                string     `gorm:"size:50;not null;unique" json:"userName"`
	PasswordHash            string     `gorm:"size:255;not null" json:"-"`
	Nickname                string     `gorm:"size:100" json:"nickname"`
	Email                   string     `gorm:"size:255;default:''" json:"email"`
	Role                    string     `gorm:"size:20;default:'R_USER'" json:"role"`
	// TokenQuotaMonth 月度 Token 限额 (NULL/0=不限), 按自然月统计 ai_token_usages
	TokenQuotaMonth         *int       `gorm:"column:token_quota_month" json:"token_quota_month"`
	TotpSecret              *string    `gorm:"column:totp_secret;size:64" json:"-"` // TOTP secret, nil = not set up
	TelegramChatID          *int64     `gorm:"column:telegram_chat_id;uniqueIndex" json:"-"`
	TelegramUsername         *string    `gorm:"column:telegram_username;size:100" json:"telegramUsername"`
	TelegramBindCode        *string    `gorm:"column:telegram_bind_code;size:10" json:"-"`
	TelegramBindCodeExpiresAt *time.Time `gorm:"column:telegram_bind_code_expires_at" json:"-"`
	LastLoginAt             *time.Time `json:"lastLoginAt"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

// TableName 指定表名
func (User) TableName() string {
	return "users"
}

// LoginRequest 登录请求
type LoginRequest struct {
	UserName string `json:"userName" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse 登录成功的响应数据
type LoginResponseData struct {
	Token        string `json:"token"`
	RefreshToken string `json:"refreshToken"`
}

// UserInfoResponse 用户信息响应数据
type UserInfoResponseData struct {
	UserId      string   `json:"userId"`
	UserName    string   `json:"userName"`
	Nickname    string   `json:"nickname"`
	Roles       []string `json:"roles"`
	Buttons     []string `json:"buttons"`
	Permissions []string `json:"permissions"`
}

type UserListItem struct {
	UserId    uint      `json:"userId"`
	UserName  string    `json:"userName"`
	Nickname  string    `json:"nickname"`
	Role      string    `json:"role"`
	// TokenQuotaMonth 月度 Token 限额 (NULL/0=不限)
	TokenQuotaMonth *int   `json:"token_quota_month"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateUserRequest struct {
	UserName string `json:"userName" binding:"required"`
	Password string `json:"password" binding:"required"`
	Nickname string `json:"nickname"`
	Role     string `json:"role" binding:"required"`
}

type UpdateUserRequest struct {
	Password string `json:"password"`
	Nickname string `json:"nickname"`
	Role     string `json:"role" binding:"required"`
	// TokenQuotaMonth 月度 Token 限额: nil=不修改, 0=不限, 正数=每月 token 上限
	TokenQuotaMonth *int `json:"token_quota_month"`
}

// UserProfileResponse 用户详细信息响应
type UserProfileResponse struct {
	UserId      uint       `json:"userId"`
	UserName    string     `json:"userName"`
	Nickname    string     `json:"nickname"`
	Email       string     `json:"email"`
	Role        string     `json:"role"`
	LastLoginAt *time.Time `json:"lastLoginAt"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
}

// UpdateProfileRequest 更新当前用户信息请求
type UpdateProfileRequest struct {
	Nickname string `json:"nickname" binding:"required"`
	Email    string `json:"email" binding:"omitempty,email"` // 邮箱会用于系统通知收件人, 必须是合法格式
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"oldPassword" binding:"required"`
	NewPassword string `json:"newPassword" binding:"required,min=6"`
}

// TwoFactorLoginResponse 2FA required response after password login
type TwoFactorLoginResponse struct {
	Need2FA   bool   `json:"need2fa"`
	TempToken string `json:"tempToken,omitempty"`
	NeedSetup bool   `json:"needSetup,omitempty"`
}

// TwoFactorSetupResponse contains QR code URL for first-time TOTP setup
type TwoFactorSetupResponse struct {
	QRCodeURL string `json:"qrCodeUrl"`
	Secret    string `json:"secret"`
}

// TwoFactorVerifyRequest request body for 2FA verification
type TwoFactorVerifyRequest struct {
	Code string `json:"code" binding:"required"`
}

// TelegramBindCodeResponse 绑定码响应
type TelegramBindCodeResponse struct {
	BindCode  string `json:"bindCode"`
	ExpiresAt int64  `json:"expiresAt"` // Unix timestamp
	BotName   string `json:"botName"`
}

// TelegramStatusResponse Telegram 绑定状态响应
type TelegramStatusResponse struct {
	IsBound        bool   `json:"isBound"`
	TelegramUsername string `json:"telegramUsername,omitempty"`
}
