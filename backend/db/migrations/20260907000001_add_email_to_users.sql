-- +goose Up
-- +goose StatementBegin
ALTER TABLE users ADD COLUMN email VARCHAR(255) DEFAULT '' COMMENT '邮箱';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE users DROP COLUMN email;
-- +goose StatementEnd
