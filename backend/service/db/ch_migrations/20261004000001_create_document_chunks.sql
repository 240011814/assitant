-- ClickHouse RAG 向量块表: Go 后端直写的主数据 (角色与 MySQL->CH 只读复制的股票表不同, 不参与水位复制)
-- 检索: WHERE user_id = ? [AND document_id IN (...)] AND embedding_model = 当前嵌入模型
--       ORDER BY toFloat64(cosineDistance(embedding, 查询向量)) ASC LIMIT k
-- 删除/重建: DELETE FROM document_chunks WHERE user_id = ? AND document_id = ? (lightweight delete, 需要 CH >= 23.3)
-- 换嵌入模型后需按文档重建索引 (embedding_model 过滤保证查询向量与库内向量同源, 避免维度不匹配报错)
-- 注意: 语句以分号分隔; 表结构变更请新增迁移文件, 不要改历史文件

CREATE TABLE IF NOT EXISTS document_chunks (
	user_id UInt32,
	document_id UInt64,
	chunk_index UInt32,
	content String,
	embedding Array(Float32),
	embedding_model LowCardinality(String),
	dims UInt16,
	created_at DateTime('Asia/Shanghai') DEFAULT now()
) ENGINE = MergeTree
ORDER BY (user_id, document_id, chunk_index);
