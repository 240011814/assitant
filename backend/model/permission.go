package model

import "time"

type Role struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Code        string    `gorm:"size:50;not null;unique" json:"code"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	Description string    `gorm:"size:255" json:"description"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (Role) TableName() string {
	return "roles"
}

type Permission struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Code      string    `gorm:"size:100;not null;unique" json:"code"`
	Name      string    `gorm:"size:100;not null" json:"name"`
	GroupName string    `gorm:"size:100;not null" json:"groupName"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

func (Permission) TableName() string {
	return "permissions"
}

type RolePermission struct {
	ID             uint      `gorm:"primaryKey;autoIncrement"`
	RoleCode       string    `gorm:"size:50;not null;index"`
	PermissionCode string    `gorm:"size:100;not null;index"`
	CreatedAt      time.Time `json:"createdAt"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}

type RolePermissionRequest struct {
	Permissions []string `json:"permissions"`
}

// UpdateRoleRequest 编辑角色请求 (内置角色编码不可改, 自定义角色可改码并级联更新关联)
type UpdateRoleRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UserRole 用户-角色关联 (user_roles 表): 用户可拥有多个角色, 角色归属的唯一真相源。
// users.role 列保留为"主角色"兼容字段 (始终与第一项同步)。
type UserRole struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;uniqueIndex:uk_user_role,priority:1" json:"userId"`
	RoleCode  string    `gorm:"size:50;not null;uniqueIndex:uk_user_role,priority:2;index" json:"roleCode"`
	CreatedAt time.Time `json:"createdAt"`
}

func (UserRole) TableName() string {
	return "user_roles"
}
