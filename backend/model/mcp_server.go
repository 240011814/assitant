package model

import "time"

// MCPServer MCP 服务注册 (动态发现工具)。
// 配置由管理页动态增删改, 运行时连接并注册工具 (mcp_<name>_<tool>); 不写迁移种子。
// ai_tools 里的 MCP 工具行由用户手动创建/启用; server 删除时自动删行、禁用时自动禁行
type MCPServer struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	Name           string    `gorm:"size:64;not null;uniqueIndex:uk_name" json:"name"`
	Transport      string    `gorm:"size:20;not null;default:'http'" json:"transport"` // stdio/sse/http
	URL            string    `gorm:"size:512;not null;default:''" json:"url"`
	Command        string    `gorm:"size:255;not null;default:''" json:"command"`
	Args           string    `gorm:"type:text" json:"args"`   // JSON 数组
	Env            string    `gorm:"type:text" json:"env"`    // JSON 对象
	Headers        string    `gorm:"type:text" json:"headers"` // JSON 对象
	TimeoutSeconds int       `gorm:"not null;default:30" json:"timeout_seconds"`
	Enabled        bool      `json:"enabled"`
	LastStatus     string    `gorm:"size:20;not null;default:'none'" json:"last_status"` // none/ok/failed
	LastError      string    `gorm:"size:500;not null;default:''" json:"last_error"`
	ToolCount      int       `gorm:"not null;default:0" json:"tool_count"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (MCPServer) TableName() string { return "mcp_servers" }
