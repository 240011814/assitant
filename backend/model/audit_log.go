package model

import "time"

// OperationAuditLog 后台操作审计日志: 记录变更类请求 (POST/PUT/DELETE/PATCH)
// 由审计中间件统一写入, user_name 是操作时刻的用户名快照 (用户改名/删除不影响历史记录)
type OperationAuditLog struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      *uint     `gorm:"index" json:"userId"`
	UserName    string    `gorm:"size:50;not null;default:''" json:"userName"`
	Method      string    `gorm:"size:10;not null" json:"method"`
	Path        string    `gorm:"size:255;not null" json:"path"`
	StatusCode  string    `gorm:"size:10;not null;default:''" json:"statusCode"`
	Success     bool      `gorm:"not null" json:"success"`
	ErrorMsg    string    `gorm:"size:500;not null;default:''" json:"errorMsg"`
	RequestBody string    `gorm:"type:text" json:"requestBody"`
	IP          string    `gorm:"size:64;not null;default:''" json:"ip"`
	UserAgent   string    `gorm:"size:255;not null;default:''" json:"userAgent"`
	LatencyMs   int64     `json:"latencyMs"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (OperationAuditLog) TableName() string {
	return "operation_audit_logs"
}
