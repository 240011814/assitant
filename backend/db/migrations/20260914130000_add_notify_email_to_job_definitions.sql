-- +goose Up
-- +goose StatementBegin
ALTER TABLE job_definitions
    ADD COLUMN notify_email VARCHAR(255) NOT NULL DEFAULT '' COMMENT '失败告警通知邮箱(空=不发)';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE job_definitions DROP COLUMN notify_email;
-- +goose StatementEnd
