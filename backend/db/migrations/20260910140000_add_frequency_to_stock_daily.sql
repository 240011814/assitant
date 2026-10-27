-- +goose Up

-- +goose StatementBegin
ALTER TABLE `stock_daily`
  ADD COLUMN `frequency` varchar(10) NOT NULL DEFAULT 'daily' COMMENT 'K线周期: daily/weekly/monthly' AFTER `code`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `stock_daily`
  DROP INDEX `uk_code_date`,
  ADD UNIQUE KEY `uk_code_freq_date` (`code`, `frequency`, `trade_date`);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `stock_daily`
  DROP INDEX `idx_code_date_desc`,
  ADD INDEX `idx_code_freq_date_desc` (`code`, `frequency`, `trade_date` DESC);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_daily`
  DROP INDEX `uk_code_freq_date`,
  ADD UNIQUE KEY `uk_code_date` (`code`, `trade_date`);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `stock_daily`
  DROP INDEX `idx_code_freq_date_desc`,
  ADD INDEX `idx_code_date_desc` (`code`, `trade_date` DESC);
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `stock_daily` DROP COLUMN `frequency`;
-- +goose StatementEnd
