-- +goose Up

ALTER TABLE `stock_info` ADD COLUMN `total_market_cap` decimal(20,2) DEFAULT NULL COMMENT '总市值(元)' AFTER `float_share`;
ALTER TABLE `stock_info` ADD COLUMN `float_market_cap` decimal(20,2) DEFAULT NULL COMMENT '流通市值(元)' AFTER `total_market_cap`;

-- +goose Down

ALTER TABLE `stock_info` DROP COLUMN `total_market_cap`;
ALTER TABLE `stock_info` DROP COLUMN `float_market_cap`;
