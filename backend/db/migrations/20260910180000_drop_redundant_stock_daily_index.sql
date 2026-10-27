-- +goose Up

-- 冗余索引: 与 uk_code_freq_date 同列仅方向不同, ASC 索引可反向扫描, 删除节省空间
-- +goose StatementBegin
ALTER TABLE `stock_daily` DROP INDEX `idx_code_freq_date_desc`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_daily` ADD INDEX `idx_code_freq_date_desc` (`code`, `frequency`, `trade_date` DESC);
-- +goose StatementEnd
