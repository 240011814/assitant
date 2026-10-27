-- +goose Up

-- +goose StatementBegin
-- 默认创建审计日志清理系统任务: 每日凌晨 4 点清理 90 天前的记录
-- (保留天数可在定时任务管理页通过 params.retentionDays 调整)
INSERT IGNORE INTO `job_definitions` (`name`, `task_name`, `cron_expr`, `params`, `enabled`, `max_retries`, `remark`)
VALUES ('audit-log-cleanup', 'audit.cleanup', '0 4 * * *', '{"retentionDays":90}', 1, 1, '操作审计日志清理(每日凌晨4点, 自动创建)');
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `job_definitions` WHERE `name` = 'audit-log-cleanup' AND `task_name` = 'audit.cleanup';
-- +goose StatementEnd
