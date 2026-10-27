-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `ai_tools` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL COMMENT '工具唯一标识',
  `display_name` varchar(100) NOT NULL COMMENT '显示名称',
  `description` text COMMENT '工具描述',
  `enabled` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否启用',
  `config_json` text COMMENT '工具配置JSON',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI 工具配置表';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:ai-tool:view', '查看AI工具', 'AI工具管理'),
('system:ai-tool:create', '创建AI工具', 'AI工具管理'),
('system:ai-tool:update', '更新AI工具', 'AI工具管理'),
('system:ai-tool:delete', '删除AI工具', 'AI工具管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'system:ai-tool:view'),
('R_SUPER', 'system:ai-tool:create'),
('R_SUPER', 'system:ai-tool:update'),
('R_SUPER', 'system:ai-tool:delete')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'system:ai-tool:%';
DELETE FROM `permissions` WHERE `code` LIKE 'system:ai-tool:%';
DROP TABLE IF EXISTS `ai_tools`;
-- +goose StatementEnd
