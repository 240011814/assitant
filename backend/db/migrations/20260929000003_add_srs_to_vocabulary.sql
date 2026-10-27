-- +goose Up

-- +goose StatementBegin
-- SRS 间隔重复: 盒子 0~5, 已知升盒, 遗忘回盒 0
ALTER TABLE vocabulary
    ADD COLUMN srs_box TINYINT NOT NULL DEFAULT 0 COMMENT 'SRS 盒子 0~5';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN next_review_at DATETIME NULL COMMENT '下次复习时间 (NULL=未进入复习流程)';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN last_reviewed_at DATETIME NULL COMMENT '最近复习时间';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD INDEX idx_vocab_review (user_id, next_review_at);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE vocabulary DROP INDEX idx_vocab_review;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN last_reviewed_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN next_review_at;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN srs_box;
-- +goose StatementEnd
