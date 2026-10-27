-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN telegram_chat_id BIGINT NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users ADD COLUMN telegram_username VARCHAR(100) NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users ADD COLUMN telegram_bind_code VARCHAR(10) NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users ADD COLUMN telegram_bind_code_expires_at DATETIME NULL;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE UNIQUE INDEX idx_users_telegram_chat_id ON users(telegram_chat_id);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_users_telegram_chat_id ON users;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users DROP COLUMN telegram_bind_code_expires_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users DROP COLUMN telegram_bind_code;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users DROP COLUMN telegram_username;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE users DROP COLUMN telegram_chat_id;
-- +goose StatementEnd
