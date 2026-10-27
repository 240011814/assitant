-- +goose Up
-- +goose StatementBegin
ALTER TABLE `lottery_activity` ADD COLUMN `draw_mode` TINYINT DEFAULT 0 COMMENT '抽奖模式: 0-转盘, 1-原神抽卡' AFTER `status`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `lottery_activity` DROP COLUMN `draw_mode`;
-- +goose StatementEnd
