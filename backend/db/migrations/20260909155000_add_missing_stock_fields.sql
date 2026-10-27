-- +goose Up

ALTER TABLE `stock_info` ADD COLUMN `total_market_cap` decimal(20,2) DEFAULT NULL COMMENT '总市值(元)' AFTER `float_share`;
ALTER TABLE `stock_info` ADD COLUMN `float_market_cap` decimal(20,2) DEFAULT NULL COMMENT '流通市值(元)' AFTER `total_market_cap`;

ALTER TABLE `stock_finance` ADD COLUMN `quick_ratio` decimal(8,4) DEFAULT NULL COMMENT '速动比率' AFTER `current_ratio`;
ALTER TABLE `stock_finance` ADD COLUMN `eps_deducted` decimal(8,4) DEFAULT NULL COMMENT '扣除每股收益' AFTER `eps`;

-- +goose Down

ALTER TABLE `stock_info` DROP COLUMN `float_market_cap`;
ALTER TABLE `stock_info` DROP COLUMN `total_market_cap`;

ALTER TABLE `stock_finance` DROP COLUMN `eps_deducted`;
ALTER TABLE `stock_finance` DROP COLUMN `quick_ratio`;