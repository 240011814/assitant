-- +goose Up
ALTER TABLE notes ADD COLUMN title VARCHAR(255) NOT NULL DEFAULT '' AFTER user_id;
CREATE INDEX idx_notes_title ON notes(title);

-- +goose Down
DROP INDEX idx_notes_title ON notes;
ALTER TABLE notes DROP COLUMN title;
