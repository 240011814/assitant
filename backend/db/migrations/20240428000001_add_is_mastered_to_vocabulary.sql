-- +goose Up
ALTER TABLE vocabulary ADD COLUMN is_mastered BOOLEAN DEFAULT FALSE;

-- +goose Down
ALTER TABLE vocabulary DROP COLUMN is_mastered;
