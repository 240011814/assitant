-- +goose Up
-- +goose StatementBegin
ALTER TABLE jobs ADD COLUMN advance_minutes INT NOT NULL DEFAULT 0 COMMENT '提前通知分钟数';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE jobs DROP COLUMN advance_minutes;
-- +goose StatementEnd
