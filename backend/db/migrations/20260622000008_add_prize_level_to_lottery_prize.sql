-- +goose Up
-- +goose StatementBegin
ALTER TABLE `lottery_prize` ADD COLUMN `prize_level` TINYINT DEFAULT 0 COMMENT '奖品等级: 0-未设置, 1-特等奖, 2-一等奖, 3-二等奖, 4-三等奖' AFTER `prize_type`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `lottery_prize` DROP COLUMN `prize_level`;
-- +goose StatementEnd
