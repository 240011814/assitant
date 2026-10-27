-- +goose Up
ALTER TABLE training_histories ADD COLUMN share_token VARCHAR(32) DEFAULT NULL;
CREATE UNIQUE INDEX idx_share_token ON training_histories(share_token);

-- +goose Down
DROP INDEX idx_share_token ON training_histories;
ALTER TABLE training_histories DROP COLUMN share_token;
