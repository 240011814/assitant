-- +goose Up

-- +goose StatementBegin
-- 默认创建 RSS 订阅自动刷新任务: 每 30 分钟抓取全部用户的启用订阅源
-- (cron 可在定时任务管理页调整, 手动"执行一次"也可用)
INSERT IGNORE INTO `job_definitions` (`name`, `task_name`, `cron_expr`, `params`, `enabled`, `max_retries`, `remark`)
VALUES ('rss-auto-refresh', 'rss.refresh_all', '*/30 * * * *', '{}', 1, 0, 'RSS 订阅自动刷新(每30分钟, 自动创建)');
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `job_definitions` WHERE `name` = 'rss-auto-refresh' AND `task_name` = 'rss.refresh_all';
-- +goose StatementEnd
