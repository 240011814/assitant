-- +goose Up
-- +goose StatementBegin
ALTER TABLE `user_documents`
  ADD COLUMN `index_status` varchar(20) NOT NULL DEFAULT 'none' COMMENT '向量索引状态: none未索引/pending排队/indexing索引中/ok完成/failed失败' AFTER `parse_error`,
  ADD COLUMN `index_error` varchar(500) NOT NULL DEFAULT '' COMMENT '索引失败原因' AFTER `index_status`,
  ADD COLUMN `chunk_count` int NOT NULL DEFAULT 0 COMMENT '向量分块数量' AFTER `index_error`;
-- +goose StatementEnd

-- 工具不写迁移文件: search_user_documents 等新工具在「AI 工具管理」页从注册表元数据新增并启用

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `user_documents` DROP COLUMN `chunk_count`, DROP COLUMN `index_error`, DROP COLUMN `index_status`;
-- +goose StatementEnd
