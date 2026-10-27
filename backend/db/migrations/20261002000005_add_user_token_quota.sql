-- +goose Up

-- +goose StatementBegin
ALTER TABLE users
    ADD COLUMN token_quota_month INT NULL COMMENT '月度 Token 限额 (NULL/0=不限), 按自然月统计 ai_token_usages' AFTER role;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE users DROP COLUMN token_quota_month;
-- +goose StatementEnd
