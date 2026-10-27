-- +goose Up
ALTER TABLE vocabulary ADD COLUMN confusing_words TEXT;

-- +goose Down
ALTER TABLE vocabulary DROP COLUMN confusing_words;
