-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
ALTER TABLE training_histories 
DROP COLUMN record_type,
ADD COLUMN is_favorite BOOLEAN NOT NULL DEFAULT FALSE AFTER title;

-- +goose Down
-- SQL in section 'Down' is executed when this migration is rolled back
ALTER TABLE training_histories 
DROP COLUMN is_favorite,
ADD COLUMN record_type VARCHAR(20) NOT NULL DEFAULT 'auto' AFTER title;
