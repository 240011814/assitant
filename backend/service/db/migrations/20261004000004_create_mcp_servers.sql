-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `mcp_servers` (
  `id` int unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(64) NOT NULL COMMENT '唯一标识 (发现的工具注册为 mcp_<name>_<tool>)',
  `transport` varchar(20) NOT NULL DEFAULT 'http' COMMENT '传输方式: stdio/sse/http(streamable)',
  `url` varchar(512) NOT NULL DEFAULT '' COMMENT 'sse/http 服务地址',
  `command` varchar(255) NOT NULL DEFAULT '' COMMENT 'stdio 可执行命令',
  `args` text COMMENT 'stdio 命令参数 (JSON 数组)',
  `env` text COMMENT 'stdio 环境变量 (JSON 对象)',
  `headers` text COMMENT 'sse/http 附加请求头 (JSON 对象, 如 Authorization)',
  `timeout_seconds` int NOT NULL DEFAULT 30 COMMENT '工具调用超时秒数',
  `enabled` tinyint(1) NOT NULL DEFAULT 1 COMMENT '是否启用 (禁用时自动禁用其工具)',
  `last_status` varchar(20) NOT NULL DEFAULT 'none' COMMENT '最近连接状态: none/ok/failed',
  `last_error` varchar(500) NOT NULL DEFAULT '' COMMENT '最近连接失败原因',
  `tool_count` int NOT NULL DEFAULT 0 COMMENT '发现的工具数量',
  `created_at` datetime DEFAULT NULL,
  `updated_at` datetime DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name` (`name`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='MCP 服务注册 (运行时动态发现工具, 表结构变更走迁移, 工具本身不写迁移)';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:mcp:manage', 'MCP服务管理', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'system:mcp:manage')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'system:mcp:manage';
-- +goose StatementEnd
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'system:mcp:manage';
-- +goose StatementEnd
-- +goose StatementBegin
DROP TABLE IF EXISTS `mcp_servers`;
-- +goose StatementEnd
