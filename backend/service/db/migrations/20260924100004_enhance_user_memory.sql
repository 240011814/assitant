-- +goose Up

-- +goose StatementBegin
-- 经历新增: 时间段 / 记忆等级 / 用户原话证据
ALTER TABLE user_experiences
    ADD COLUMN time_range VARCHAR(64) NOT NULL DEFAULT '' COMMENT '经历时间段, 如 2026 / 2026-03~2026-05' AFTER content,
    ADD COLUMN memory_level VARCHAR(20) NOT NULL DEFAULT 'long_term' COMMENT '记忆等级: core/long_term/temporary' AFTER confidence,
    ADD COLUMN evidence TEXT COMMENT '用户原话摘要, 便于回溯';
-- +goose StatementEnd

-- +goose StatementBegin
-- status 语义调整: active/archived -> 经历进度 ongoing/completed/abandoned/unknown
UPDATE user_experiences SET status = 'unknown' WHERE status = 'active' OR status = '' OR status IS NULL;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences
    MODIFY COLUMN status VARCHAR(20) NOT NULL DEFAULT 'unknown' COMMENT '进度状态: ongoing/completed/abandoned/unknown';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE user_experiences
    MODIFY COLUMN status VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT 'active/archived';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE user_experiences
    DROP COLUMN evidence,
    DROP COLUMN memory_level,
    DROP COLUMN time_range;
-- +goose StatementEnd