-- +goose Up

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `job_definitions` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `name` varchar(100) NOT NULL COMMENT '任务名称(唯一)',
  `task_name` varchar(100) NOT NULL COMMENT '已注册的任务方法标识',
  `cron_expr` varchar(64) NOT NULL COMMENT 'cron表达式(5段, 分 时 日 月 周)',
  `params` json DEFAULT NULL COMMENT '任务参数(JSON)',
  `enabled` tinyint(1) DEFAULT 0 COMMENT '是否启用',
  `max_retries` int DEFAULT 0 COMMENT '失败重试次数',
  `remark` varchar(255) DEFAULT NULL COMMENT '备注',
  `created_by` bigint unsigned DEFAULT NULL COMMENT '创建人',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_name` (`name`),
  KEY `idx_enabled` (`enabled`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='定时任务定义';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `job_runs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `definition_id` bigint unsigned NOT NULL COMMENT '任务定义ID',
  `attempt` int DEFAULT 1 COMMENT '第几次尝试(重试递增)',
  `status` varchar(20) DEFAULT 'running' COMMENT 'running/success/failed/skipped',
  `trigger_type` varchar(20) DEFAULT 'scheduler' COMMENT 'scheduler/manual',
  `started_at` datetime DEFAULT NULL,
  `finished_at` datetime DEFAULT NULL,
  `error` text COMMENT '失败原因',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_definition` (`definition_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='定时任务执行历史';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('job:manage', '定时任务管理', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_ADMIN', 'job:manage')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- 预置常用同步任务, 默认停用, 由管理员在后台按需启用
-- +goose StatementBegin
INSERT INTO `job_definitions` (`name`, `task_name`, `cron_expr`, `enabled`, `remark`) VALUES
('股票列表每日同步', 'stock.sync_stock_list', '30 17 * * 1-5', 0, '工作日收盘后同步股票列表'),
('行情数据每日同步', 'stock.sync_daily_quotes', '40 17 * * 1-5', 0, '工作日收盘后增量同步日/周/月K线'),
('财务数据每日同步', 'stock.sync_finance_all', '0 8 * * 1-5', 0, '工作日早上增量同步财务数据')
ON DUPLICATE KEY UPDATE `remark` = VALUES(`remark`);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'job:manage';
DELETE FROM `permissions` WHERE `code` = 'job:manage';
DROP TABLE IF EXISTS `job_runs`;
DROP TABLE IF EXISTS `job_definitions`;
-- +goose StatementEnd
