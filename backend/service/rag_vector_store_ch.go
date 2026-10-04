package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// chVectorStore VectorStore 的 ClickHouse 实现 (document_chunks 表, ch_migrations 建表)。
// 删除走 lightweight DELETE (需要 CH >= 23.3; ALTER DELETE 是异步 mutation,
// 与紧随的重写有竞态, 不用)。与 MySQL->CH 的股票只读复制表角色不同, 本表是 Go 直写主数据
type chVectorStore struct{}

// NewCHVectorStore 基于 CH 全局连接的向量存储 (CH 未启用时 Available=false, 其余方法报错)
func NewCHVectorStore() VectorStore { return &chVectorStore{} }

func (s *chVectorStore) Available() bool { return CH != nil }

func (s *chVectorStore) InsertChunks(ctx context.Context, rows []ChunkRow) error {
	if CH == nil {
		return errors.New("ClickHouse 未启用")
	}
	if len(rows) == 0 {
		return nil
	}
	batch, err := CH.PrepareBatch(ctx, "INSERT INTO document_chunks")
	if err != nil {
		return fmt.Errorf("创建写入批次失败: %w", err)
	}
	for i := range rows {
		if err := batch.Append(
			rows[i].UserID, uint64(rows[i].DocumentID), uint32(rows[i].ChunkIndex),
			rows[i].Content, rows[i].Embedding, rows[i].EmbeddingModel, uint16(rows[i].Dims),
		); err != nil {
			_ = batch.Close()
			return fmt.Errorf("写入向量块失败: %w", err)
		}
	}
	if err := batch.Send(); err != nil {
		_ = batch.Close()
		return fmt.Errorf("写入向量块失败: %w", err)
	}
	return nil
}

func (s *chVectorStore) DeleteDocumentChunks(ctx context.Context, userID uint, documentID uint) error {
	if CH == nil {
		return errors.New("ClickHouse 未启用")
	}
	if err := CH.Exec(ctx, "DELETE FROM document_chunks WHERE user_id = ? AND document_id = ?", userID, documentID); err != nil {
		return fmt.Errorf("删除向量块失败 (lightweight delete 需要 ClickHouse >= 23.3): %w", err)
	}
	return nil
}

func (s *chVectorStore) SearchChunks(ctx context.Context, userID uint, queryVec []float32, embeddingModel string, documentIDs []uint, topK int) ([]VectorHit, error) {
	if CH == nil {
		return nil, errors.New("ClickHouse 未启用")
	}
	// toFloat64 包一层: 距离列类型在不同版本间有 Float32/Float64 差异, 统一按 Float64 扫描
	sql := "SELECT document_id, chunk_index, content, toFloat64(cosineDistance(embedding, ?)) AS dist " +
		"FROM document_chunks WHERE user_id = ? AND embedding_model = ?"
	args := []any{queryVec, userID, embeddingModel}
	if len(documentIDs) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(documentIDs)), ",")
		sql += " AND document_id IN (" + placeholders + ")"
		for _, id := range documentIDs {
			args = append(args, id)
		}
	}
	// topK 已由调用方钳位为整数, 直接拼接 (LIMIT 参数绑定在不同驱动版本行为不一)
	sql += fmt.Sprintf(" ORDER BY dist ASC LIMIT %d", topK)

	rows, err := CH.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("向量检索失败: %w", err)
	}
	defer rows.Close()
	hits := make([]VectorHit, 0, topK)
	for rows.Next() {
		var h VectorHit
		var chunkIdx uint32
		if err := rows.Scan(&h.DocumentID, &chunkIdx, &h.Content, &h.Score); err != nil {
			return nil, fmt.Errorf("读取检索结果失败: %w", err)
		}
		h.ChunkIndex = int(chunkIdx)
		h.Score = 1 - h.Score // cosineDistance 返回距离, 转相似度
		hits = append(hits, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("读取检索结果失败: %w", err)
	}
	return hits, nil
}
