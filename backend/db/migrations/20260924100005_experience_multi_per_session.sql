-- +goose Up

-- +goose StatementBegin
-- 一段对话可对应多条经历: 去掉 history_id 唯一约束, 改为普通索引
ALTER TABLE user_experiences DROP INDEX uk_history;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences ADD KEY idx_history (history_id);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE user_experiences DROP INDEX idx_history;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences ADD UNIQUE KEY uk_history (history_id);
-- +goose StatementEnd