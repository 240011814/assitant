-- +goose Up
ALTER TABLE `lottery_activity` ADD COLUMN `allow_anonymous` TINYINT DEFAULT 1 COMMENT '是否允许匿名(未登录)抽奖: 0-否, 1-是' AFTER `draw_mode`;
-- +goose Down
ALTER TABLE `lottery_activity` DROP COLUMN `allow_anonymous`;
