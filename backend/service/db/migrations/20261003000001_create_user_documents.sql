-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `user_documents` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `user_id` int unsigned NOT NULL COMMENT '所属用户',
  `filename` varchar(255) NOT NULL COMMENT '原始文件名',
  `ext` varchar(20) NOT NULL DEFAULT '' COMMENT '扩展名 (小写, 含点)',
  `mime_type` varchar(100) NOT NULL DEFAULT '' COMMENT 'MIME 类型',
  `size_bytes` bigint NOT NULL DEFAULT 0 COMMENT '文件大小 (字节)',
  `object_key` varchar(512) NOT NULL COMMENT 'S3 对象键 (原文件)',
  `parse_status` varchar(20) NOT NULL DEFAULT 'none' COMMENT '解析状态: none/ok/failed',
  `parse_error` varchar(500) NOT NULL DEFAULT '' COMMENT '解析失败原因',
  `text_key` varchar(512) NOT NULL DEFAULT '' COMMENT 'S3 对象键 (解析文本)',
  `text_chars` int NOT NULL DEFAULT 0 COMMENT '解析文本字符数',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_user_doc` (`user_id`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户文档 (S3 存储)';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('document:view', '文档查看', '文档管理'),
('document:upload', '文档上传', '文档管理'),
('document:delete', '文档删除', '文档管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'document:view'),
('R_SUPER', 'document:upload'),
('R_SUPER', 'document:delete');
-- +goose StatementEnd



-- +goose Down
-- +goose StatementBegin
DELETE FROM `ai_tools` WHERE `name` IN ('read_document', 'list_user_documents');
-- +goose StatementEnd
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` IN ('document:view', 'document:upload', 'document:delete');
-- +goose StatementEnd
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN ('document:view', 'document:upload', 'document:delete');
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `user_documents`;
-- +goose StatementEnd
