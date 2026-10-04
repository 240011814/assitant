package service

import "context"

// VectorHit 向量存储层检索命中 (未富化文件名, 文件名由 service 层批量补齐)
type VectorHit struct {
	DocumentID uint64
	ChunkIndex int
	Score      float64 // 余弦相似度, 越大越相关
	Content    string
}

// ChunkRow 写入向量存储的一条块
type ChunkRow struct {
	UserID         uint
	DocumentID     uint
	ChunkIndex     int
	Content        string
	Embedding      []float32
	EmbeddingModel string
	Dims           int
}

// VectorStore RAG 向量存储抽象 (当前实现: ClickHouse, 见 rag_vector_store_ch.go;
// 将来换 Qdrant/Milvus/MySQL 等只需新增实现并在 main.go 换构造, 索引管线与检索代码不动)。
// 实现约定:
//   - 数据按 (user, document) 圈定; 重建语义 = 先 DeleteDocumentChunks 再 InsertChunks
//   - SearchChunks 只在同 embeddingModel 的向量中检索 (换嵌入模型后旧向量自动失效,
//     也避免维度不匹配导致后端报错)
//   - Score 为余弦相似度 (越大越相关), 返回按相似度降序
//   - Available 为 false 时其余方法返回明确错误, 不得 panic
type VectorStore interface {
	// Available 存储后端当前是否可用 (如 ClickHouse 未启用)
	Available() bool
	// InsertChunks 批量写入向量块
	InsertChunks(ctx context.Context, rows []ChunkRow) error
	// DeleteDocumentChunks 删除某文档的全部向量块 (重建/删除文档时调用)
	DeleteDocumentChunks(ctx context.Context, userID uint, documentID uint) error
	// SearchChunks 检索与 queryVec 最相关的片段; documentIDs 非空时限定范围; topK 已由调用方钳位
	SearchChunks(ctx context.Context, userID uint, queryVec []float32, embeddingModel string, documentIDs []uint, topK int) ([]VectorHit, error)
}
