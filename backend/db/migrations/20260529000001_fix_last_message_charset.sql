-- +goose Up
ALTER TABLE training_histories MODIFY COLUMN last_message VARCHAR(500) CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci DEFAULT '';

-- +goose Down
ALTER TABLE training_histories MODIFY COLUMN last_message VARCHAR(500) DEFAULT '';
