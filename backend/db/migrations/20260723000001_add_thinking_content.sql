-- +goose Up
-- +goose StatementBegin
ALTER TABLE `training_messages` ADD COLUMN `thinking_content` TEXT AFTER `content`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `training_messages` DROP COLUMN `thinking_content`;
-- +goose StatementEnd
