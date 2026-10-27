-- +goose Up

-- job_definitions 扩展: 支持一次性/重复调度(备忘迁移), cron 任务保持不变
-- +goose StatementBegin
ALTER TABLE `job_definitions`
  ADD COLUMN `user_id` bigint unsigned DEFAULT NULL COMMENT '归属用户(用户备忘), 系统任务为 NULL' AFTER `id`,
  ADD COLUMN `schedule_type` varchar(20) NOT NULL DEFAULT 'cron' COMMENT '调度类型: cron/once/repeat' AFTER `task_name`,
  ADD COLUMN `run_at` datetime DEFAULT NULL COMMENT 'once/repeat 的执行时间点' AFTER `cron_expr`,
  ADD COLUMN `advance_minutes` int DEFAULT 0 COMMENT '提前提醒分钟数(备忘)' AFTER `run_at`,
  ADD COLUMN `repeat_type` varchar(20) DEFAULT NULL COMMENT '重复类型: daily/weekly/monthly/yearly' AFTER `advance_minutes`,
  ADD COLUMN `repeat_interval` int DEFAULT 1 COMMENT '重复间隔数' AFTER `repeat_type`,
  ADD COLUMN `repeat_end_at` datetime DEFAULT NULL COMMENT '重复终止时间' AFTER `repeat_interval`,
  ADD KEY `idx_user` (`user_id`);
-- +goose StatementEnd

-- 旧任务表(备忘)整体废弃, 旧数据不迁移
-- +goose StatementBegin
DROP TABLE IF EXISTS `jobs`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `job_definitions`
  DROP KEY `idx_user`,
  DROP COLUMN `repeat_end_at`,
  DROP COLUMN `repeat_interval`,
  DROP COLUMN `repeat_type`,
  DROP COLUMN `advance_minutes`,
  DROP COLUMN `run_at`,
  DROP COLUMN `schedule_type`,
  DROP COLUMN `user_id`;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `jobs` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `job_type` varchar(50) NOT NULL,
  `job_id` bigint unsigned NOT NULL DEFAULT 0,
  `user_id` bigint unsigned DEFAULT NULL,
  `scheduled_at` datetime NOT NULL,
  `advance_minutes` int DEFAULT 0,
  `status` varchar(20) DEFAULT 'pending',
  `retry_count` int DEFAULT 0,
  `max_retries` int DEFAULT 3,
  `last_error` text,
  `params` json DEFAULT NULL,
  `repeat_type` varchar(20) DEFAULT 'none',
  `repeat_interval` int DEFAULT 1,
  `repeat_end_at` datetime DEFAULT NULL,
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_job_type` (`job_type`),
  KEY `idx_job_id` (`job_id`),
  KEY `idx_user_id` (`user_id`),
  KEY `idx_scheduled_at` (`scheduled_at`),
  KEY `idx_status` (`status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd
