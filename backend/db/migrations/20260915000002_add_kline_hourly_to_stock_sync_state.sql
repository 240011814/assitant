-- +goose Up

-- 小时K线同步水位 (逐股增量, 记录已同步到的最新 bar 时间)
-- +goose StatementBegin
ALTER TABLE `stock_sync_state` ADD COLUMN `kline_hourly_to` datetime DEFAULT NULL COMMENT '小时K线已同步到的最新bar时间' AFTER `kline_monthly_to`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_sync_state` DROP COLUMN `kline_hourly_to`;
-- +goose StatementEnd