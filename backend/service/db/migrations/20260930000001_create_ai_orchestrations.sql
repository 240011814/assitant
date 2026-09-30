-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `ai_orchestrations` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL COMMENT '编排名称(唯一)',
  `description` text COMMENT '编排描述',
  `definition` longtext NOT NULL COMMENT '编排 DSL JSON: {version, nodes, edges}',
  `version` int NOT NULL DEFAULT 1 COMMENT '定义版本, 保存递增',
  `enabled` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否启用',
  `last_debug_summary` text COMMENT '最近一次调试运行的节点级摘要 JSON',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI 可视化编排定义表(Agent Studio)';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:orchestration:view', '查看编排', 'Agent编排'),
('system:orchestration:create', '创建编排', 'Agent编排'),
('system:orchestration:update', '更新编排', 'Agent编排'),
('system:orchestration:delete', '删除编排', 'Agent编排'),
('system:orchestration:debug', '调试编排', 'Agent编排')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'system:orchestration:view'),
('R_SUPER', 'system:orchestration:create'),
('R_SUPER', 'system:orchestration:update'),
('R_SUPER', 'system:orchestration:delete'),
('R_SUPER', 'system:orchestration:debug')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'system:orchestration:%';
DELETE FROM `permissions` WHERE `code` LIKE 'system:orchestration:%';
DROP TABLE IF EXISTS `ai_orchestrations`;
-- +goose StatementEnd
