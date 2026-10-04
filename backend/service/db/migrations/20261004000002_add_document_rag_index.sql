-- +goose Up
-- +goose StatementBegin
ALTER TABLE `user_documents`
  ADD COLUMN `index_status` varchar(20) NOT NULL DEFAULT 'none' COMMENT '向量索引状态: none未索引/pending排队/indexing索引中/ok完成/failed失败' AFTER `parse_error`,
  ADD COLUMN `index_error` varchar(500) NOT NULL DEFAULT '' COMMENT '索引失败原因' AFTER `index_status`,
  ADD COLUMN `chunk_count` int NOT NULL DEFAULT 0 COMMENT '向量分块数量' AFTER `index_error`;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `ai_tools` (`name`, `display_name`, `description`, `enabled`, `config_json`) VALUES
('search_user_documents', '文档语义检索', '在当前用户上传的文档中做语义检索, 返回与查询最相关的片段 (含文档名/片段序号/相似度)。用户问题可能涉及已上传文档内容、或文档较多较长时优先用本工具定位相关段落, 再配合 read_document 按 (document_id, offset) 精读上下文; 文档列表可用 list_user_documents 获取。', 1, '{}')
ON DUPLICATE KEY UPDATE `display_name` = VALUES(`display_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `ai_tools` WHERE `name` = 'search_user_documents';
-- +goose StatementEnd
-- +goose StatementBegin
ALTER TABLE `user_documents` DROP COLUMN `chunk_count`, DROP COLUMN `index_error`, DROP COLUMN `index_status`;
-- +goose StatementEnd
