-- +goose Up
ALTER TABLE `lottery_record` ADD COLUMN `redeemed` TINYINT DEFAULT 0 COMMENT '奖品核销状态: 0-未核销, 1-已核销' AFTER `is_winner`;
-- +goose Down
ALTER TABLE `lottery_record` DROP COLUMN `redeemed`;
