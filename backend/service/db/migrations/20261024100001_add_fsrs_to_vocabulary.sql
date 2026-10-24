-- +goose Up

-- +goose StatementBegin
-- FSRS 调度字段: 替代 Leitner 盒子 (srs_box 保留仅为历史兼容, 不再更新)
ALTER TABLE vocabulary
    ADD COLUMN fsrs_stability DOUBLE NOT NULL DEFAULT 0 COMMENT 'FSRS 稳定性';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN fsrs_difficulty DOUBLE NOT NULL DEFAULT 0 COMMENT 'FSRS 难度';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN fsrs_state TINYINT NOT NULL DEFAULT 0 COMMENT 'FSRS 状态 0=New 1=Learning 2=Review 3=Relearning';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN fsrs_reps INT NOT NULL DEFAULT 0 COMMENT 'FSRS 累计复习次数';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN fsrs_lapses INT NOT NULL DEFAULT 0 COMMENT 'FSRS 遗忘次数';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN assist_example TEXT NULL COMMENT '遗忘干预: AI 新例句';
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary
    ADD COLUMN assist_mnemonic TEXT NULL COMMENT '遗忘干预: AI 助记提示';
-- +goose StatementEnd

-- +goose StatementBegin
-- 历史数据迁移: 已进入复习流程的词按 Leitner 盒子映射到近似 FSRS 参数 (state=Review),
-- 未复习过的词保持 New
UPDATE vocabulary
SET fsrs_state = 2,
    fsrs_stability = CASE srs_box WHEN 1 THEN 1 WHEN 2 THEN 2 WHEN 3 THEN 4 WHEN 4 THEN 7 WHEN 5 THEN 15 ELSE 1 END,
    fsrs_difficulty = 5,
    fsrs_reps = 1
WHERE next_review_at IS NOT NULL;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN assist_mnemonic;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN assist_example;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN fsrs_lapses;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN fsrs_reps;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN fsrs_state;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN fsrs_difficulty;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN fsrs_stability;
-- +goose StatementEnd
