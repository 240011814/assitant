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

// ChunkHit 语义检索命中的文档片段
type ChunkHit struct {
	DocumentID uint    `json:"document_id"`
	Filename   string  `json:"filename"`
	ChunkIndex int     `json:"chunk_index"`
	Score      float64 `json:"score"` // 余弦相似度 (1-距离), 越大越相关
	Content    string  `json:"content"`
}

// DocumentSearcher 文档语义检索能力 (由 service.UserDocumentService 实现, 注入给 AI 工具)。
// 检索范围始终限定在当前用户自己的文档内; 功能未启用时返回明确错误
type DocumentSearcher interface {
	// SearchDocuments 在用户文档的向量索引中检索与 query 最相关的片段 (topK<=0 用默认值;
	// documentIDs 非空时限定在这些文档内检索)
	SearchDocuments(userID uint, query string, documentIDs []uint, topK int) ([]ChunkHit, error)
}
