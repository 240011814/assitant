-- +goose Up
-- +goose StatementBegin
ALTER TABLE `cut_record` MODIFY COLUMN `user_id` BIGINT UNSIGNED NOT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `cut_record` SET `user_id` = CAST(`user_id` AS UNSIGNED) WHERE `user_id` REGEXP '^[0-9]+$';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `cut_record` MODIFY COLUMN `user_id` VARCHAR(100) NOT NULL;
-- +goose StatementEnd
