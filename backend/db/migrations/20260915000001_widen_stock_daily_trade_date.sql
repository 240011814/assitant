-- +goose Up

-- 小时K线需要存储 bar 时间, trade_date 由 date 放宽为 datetime (日/周/月线保持交易日 00:00:00)
-- 注意: date -> datetime 属于需要重建表的类型转换, 大表执行较慢, 建议低峰/停机执行
-- +goose StatementBegin
ALTER TABLE `stock_daily` MODIFY COLUMN `trade_date` datetime NOT NULL COMMENT 'K线时间(日/周/月为交易日00:00:00, 小时线为bar时间)';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_daily` MODIFY COLUMN `trade_date` date NOT NULL COMMENT '交易日期';
-- +goose StatementEnd