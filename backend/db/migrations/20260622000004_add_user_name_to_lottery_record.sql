-- +goose Up
-- +goose StatementBegin
ALTER TABLE `lottery_record` ADD COLUMN `user_name` VARCHAR(100) COMMENT '用户姓名' AFTER `user_id`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `lottery_record` DROP COLUMN `user_name`;
-- +goose StatementEnd
