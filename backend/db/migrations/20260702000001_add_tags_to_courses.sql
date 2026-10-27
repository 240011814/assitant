-- +goose Up
-- +goose StatementBegin
ALTER TABLE `courses` ADD COLUMN `tags` VARCHAR(500) DEFAULT '' AFTER `description`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `courses` ADD INDEX `idx_courses_tags` (`tags`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `courses` DROP INDEX `idx_courses_tags`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `courses` DROP COLUMN `tags`;
-- +goose StatementEnd
