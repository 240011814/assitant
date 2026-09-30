-- +goose Up
-- 把编排并入 ai_agents 表: 编排行用 agent_type='orchestration' 区分,
-- 定义/版本/启用/最近调试摘要作为附加列存放在同表。
-- 不考虑旧数据, 独立编排表直接丢弃。

-- +goose StatementBegin
ALTER TABLE `ai_agents`
  ADD COLUMN `definition` LONGTEXT NULL COMMENT '编排 DSL JSON: {version, nodes, edges} (仅 agent_type=orchestration)',
  ADD COLUMN `version` INT NOT NULL DEFAULT 1 COMMENT '编排定义版本, 保存递增 (仅编排)',
  ADD COLUMN `enabled` TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否启用 (仅编排语义)',
  ADD COLUMN `last_debug_summary` TEXT NULL COMMENT '最近一次调试运行的节点级摘要 JSON (仅编排)';
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `ai_orchestrations`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `ai_agents`
  DROP COLUMN `definition`,
  DROP COLUMN `version`,
  DROP COLUMN `enabled`,
  DROP COLUMN `last_debug_summary`;
-- +goose StatementEnd

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