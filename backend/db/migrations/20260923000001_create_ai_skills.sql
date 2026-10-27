-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `ai_skills` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL COMMENT 'Skill 唯一标识(frontmatter.name)',
  `description` text COMMENT 'Skill 描述',
  `context` varchar(32) NOT NULL DEFAULT 'inline' COMMENT '执行模式: inline/fork/fork_with_context',
  `agent` varchar(100) DEFAULT '' COMMENT 'fork 模式使用的子 Agent',
  `model` varchar(100) DEFAULT '' COMMENT 'Skill 指定模型',
  `content` longtext COMMENT 'SKILL.md 正文',
  `enabled` tinyint(1) NOT NULL DEFAULT 0 COMMENT '是否启用(参与动态加载)',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='AI Skill 配置表';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:skill:view', '查看Skill', 'Skill管理'),
('system:skill:create', '创建Skill', 'Skill管理'),
('system:skill:update', '更新Skill', 'Skill管理'),
('system:skill:delete', '删除Skill', 'Skill管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'system:skill:view'),
('R_SUPER', 'system:skill:create'),
('R_SUPER', 'system:skill:update'),
('R_SUPER', 'system:skill:delete')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'system:skill:%';
DELETE FROM `permissions` WHERE `code` LIKE 'system:skill:%';
DROP TABLE IF EXISTS `ai_skills`;
-- +goose StatementEnd