-- +goose Up
-- +goose StatementBegin
ALTER TABLE `training_histories` DROP COLUMN `duration`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `training_histories` ADD COLUMN `duration` INT NOT NULL DEFAULT 0 COMMENT '训练时长(秒)';
-- +goose StatementEnd
