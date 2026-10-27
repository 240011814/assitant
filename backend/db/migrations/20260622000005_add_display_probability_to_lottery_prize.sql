-- +goose Up
-- +goose StatementBegin
ALTER TABLE `lottery_prize` ADD COLUMN `display_probability` DECIMAL(5,4) DEFAULT 0 COMMENT '展示概率 (0-1), 用于前端显示' AFTER `probability`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `lottery_prize` DROP COLUMN `display_probability`;
-- +goose StatementEnd
