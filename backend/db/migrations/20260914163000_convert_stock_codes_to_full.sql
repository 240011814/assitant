-- +goose Up
-- +goose StatementBegin
DELETE FROM stock_daily WHERE code LIKE '000%';
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM stock_sync_state WHERE code LIKE '000%';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_info MODIFY COLUMN code varchar(16) NOT NULL COMMENT '完整证券代码 sz.000003/sh.600000/bj.430047';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_daily MODIFY COLUMN code varchar(16) NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_finance MODIFY COLUMN code varchar(16) NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_sync_state MODIFY COLUMN code varchar(16) NOT NULL COMMENT '完整证券代码';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_concept MODIFY COLUMN code varchar(16) NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_watchlist MODIFY COLUMN code varchar(16) NOT NULL;
-- +goose StatementEnd

-- stock_info 按自身 market 列补前缀 (行级身份唯一, 无冲突)
-- +goose StatementBegin
UPDATE stock_info SET code = CONCAT(LOWER(market), '.', code) WHERE code NOT LIKE '%.%';
-- +goose StatementEnd

-- 其余表无 stock_info 的 market 可参照, 按首数字推断: 6=沪 0/3=深 4/8/9=北
-- (000% 歧义行已删除; stock_finance/stock_concept/stock_watchlist 只存股票, 推断无歧义)
-- +goose StatementBegin
UPDATE stock_daily SET code = CONCAT('sh.', code) WHERE code NOT LIKE '%.%' AND code LIKE '6%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_daily SET code = CONCAT('sz.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '0%' OR code LIKE '3%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_daily SET code = CONCAT('bj.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '4%' OR code LIKE '8%' OR code LIKE '9%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_finance SET code = CONCAT('sh.', code) WHERE code NOT LIKE '%.%' AND code LIKE '6%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_finance SET code = CONCAT('sz.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '0%' OR code LIKE '3%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_finance SET code = CONCAT('bj.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '4%' OR code LIKE '8%' OR code LIKE '9%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_sync_state SET code = CONCAT('sh.', code) WHERE code NOT LIKE '%.%' AND code LIKE '6%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_sync_state SET code = CONCAT('sz.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '0%' OR code LIKE '3%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_sync_state SET code = CONCAT('bj.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '4%' OR code LIKE '8%' OR code LIKE '9%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_concept SET code = CONCAT('sh.', code) WHERE code NOT LIKE '%.%' AND code LIKE '6%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_concept SET code = CONCAT('sz.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '0%' OR code LIKE '3%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_concept SET code = CONCAT('bj.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '4%' OR code LIKE '8%' OR code LIKE '9%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_watchlist SET code = CONCAT('sh.', code) WHERE code NOT LIKE '%.%' AND code LIKE '6%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_watchlist SET code = CONCAT('sz.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '0%' OR code LIKE '3%');
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_watchlist SET code = CONCAT('bj.', code) WHERE code NOT LIKE '%.%' AND (code LIKE '4%' OR code LIKE '8%' OR code LIKE '9%');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE stock_info SET code = SUBSTRING_INDEX(code, '.', -1) WHERE code LIKE '%.%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_daily SET code = SUBSTRING_INDEX(code, '.', -1) WHERE code LIKE '%.%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_finance SET code = SUBSTRING_INDEX(code, '.', -1) WHERE code LIKE '%.%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_sync_state SET code = SUBSTRING_INDEX(code, '.', -1) WHERE code LIKE '%.%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_concept SET code = SUBSTRING_INDEX(code, '.', -1) WHERE code LIKE '%.%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_watchlist SET code = SUBSTRING_INDEX(code, '.', -1) WHERE code LIKE '%.%';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_info MODIFY COLUMN code varchar(10) NOT NULL COMMENT '股票代码';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_daily MODIFY COLUMN code varchar(10) NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_finance MODIFY COLUMN code varchar(10) NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_sync_state MODIFY COLUMN code varchar(10) NOT NULL COMMENT '股票代码';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_concept MODIFY COLUMN code varchar(10) NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE stock_watchlist MODIFY COLUMN code varchar(10) NOT NULL;
-- +goose StatementEnd
