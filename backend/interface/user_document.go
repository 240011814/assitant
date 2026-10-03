package interfaces

// DocumentInfo AI 工具可见的文档元数据 (不含对象存储路径等内部字段)
type DocumentInfo struct {
	ID          uint   `json:"id"`
	Filename    string `json:"filename"`
	Ext         string `json:"ext"`
	SizeBytes   int64  `json:"size_bytes"`
	ParseStatus string `json:"parse_status"`
	TextChars   int    `json:"text_chars"`
	CreatedAt   string `json:"created_at"`
}

// UserDocumentStore 用户文档存取能力 (由 service.UserDocumentService 实现, 注入给 AI 工具)。
// 所有方法都以 userID 圈定范围, 工具侧只能访问当前用户自己的文档
type UserDocumentStore interface {
	// ListDocuments 列出用户的文档元数据 (按创建时间倒序)
	ListDocuments(userID uint) ([]DocumentInfo, error)
	// ReadDocumentChunk 按字符区间读取文档解析文本 (chunk 为空且 err 为 nil 表示区间越界)
	ReadDocumentChunk(userID uint, documentID uint, offset, length int) (chunk string, totalChars int, filename string, err error)
}
