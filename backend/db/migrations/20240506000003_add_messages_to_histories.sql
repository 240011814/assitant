-- +goose Up
-- +goose StatementBegin
ALTER TABLE `training_histories` ADD COLUMN `messages` LONGTEXT;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `training_histories` DROP COLUMN `messages`;
-- +goose StatementEnd
