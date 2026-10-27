-- +goose Up
-- +goose StatementBegin
ALTER TABLE `ai_tools` ADD COLUMN `confirm_required` BOOLEAN DEFAULT FALSE AFTER `enabled`;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `ai_tools` DROP COLUMN `confirm_required`;
-- +goose StatementEnd
