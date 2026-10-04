package model

import "time"

// UserDocument 用户上传的文档。
// 原文件与解析文本都存 S3 兼容对象存储 (ObjectKey/TextKey), DB 只存元数据;
// ParseStatus: none 无解析结果(格式不支持) / ok 解析成功 / failed 解析失败
type UserDocument struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	UserID      uint      `gorm:"not null;index:idx_user_doc" json:"user_id"`
	Filename    string    `gorm:"size:255;not null" json:"filename"`
	Ext         string    `gorm:"size:20;not null;default:''" json:"ext"`
	MimeType    string    `gorm:"size:100;not null;default:''" json:"mime_type"`
	SizeBytes   int64     `gorm:"not null;default:0" json:"size_bytes"`
	ObjectKey   string    `gorm:"size:512;not null" json:"-"`
	ParseStatus string    `gorm:"size:20;not null;default:'none'" json:"parse_status"`
	ParseError  string    `gorm:"size:500;not null;default:''" json:"parse_error"`
	// 向量索引 (RAG): 状态机 none未索引/pending排队/indexing索引中/ok完成/failed失败
	IndexStatus string `gorm:"size:20;not null;default:'none'" json:"index_status"`
	IndexError  string `gorm:"size:500;not null;default:''" json:"index_error"`
	ChunkCount  int    `gorm:"not null;default:0" json:"chunk_count"`
	TextKey     string `gorm:"size:512;not null;default:''" json:"-"`
	TextChars   int    `gorm:"not null;default:0" json:"text_chars"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (UserDocument) TableName() string { return "user_documents" }
