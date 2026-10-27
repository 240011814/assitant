-- +goose Up

-- +goose StatementBegin
-- 默认创建预警评估系统任务: 每个交易日 18:30 运行 (行情同步完成后),
-- 幂等: 规则仅在新交易日行情晚于上次触发日时评估, 节假日空转无害
INSERT IGNORE INTO `job_definitions` (`name`, `task_name`, `cron_expr`, `params`, `enabled`, `max_retries`, `remark`)
VALUES ('stock-alert-evaluate', 'stock.evaluate_alerts', '30 18 * * 1-5', '{}', 1, 1, '自选股预警评估(每个交易日收盘后, 自动创建)');
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `job_definitions` WHERE `name` = 'stock-alert-evaluate' AND `task_name` = 'stock.evaluate_alerts';
-- +goose StatementEnd
