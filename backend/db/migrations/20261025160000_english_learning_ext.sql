-- +goose Up

-- +goose StatementBegin
-- vocabulary: 生词原文回链 + 阅读相遇计数 + 加入时原句 (语境复习方向)
ALTER TABLE vocabulary
    ADD COLUMN source_kind VARCHAR(20) NULL COMMENT '来源: reading=分级阅读 / rss=RSS条目',
    ADD COLUMN source_ref INT NOT NULL DEFAULT 0 COMMENT '来源文档/条目 ID',
    ADD COLUMN encounters INT NOT NULL DEFAULT 0 COMMENT '阅读中相遇次数',
    ADD COLUMN context_sentence TEXT NULL COMMENT '加入生词本时所在的原句';
-- +goose StatementEnd

-- +goose StatementBegin
-- reading_docs: 免 LLM 的个性化难度预估 + 分块标注进度
ALTER TABLE reading_docs
    ADD COLUMN est_unknown_rate DOUBLE NOT NULL DEFAULT 0 COMMENT '导入时预估生词率 (生词本交集+长词启发)',
    ADD COLUMN annotate_progress INT NOT NULL DEFAULT 0 COMMENT '标注进度 0-100 (分块标注时逐块推进)';
-- +goose StatementEnd

-- +goose StatementBegin
-- rss_items: 一键生词提取结果缓存 + 摘要难度预估
ALTER TABLE rss_items
    ADD COLUMN difficult_words MEDIUMTEXT NULL COMMENT 'AI 提取的生词 JSON (一键提取, 与分级阅读同源)',
    ADD COLUMN est_unknown_rate DOUBLE NOT NULL DEFAULT 0;
-- +goose StatementEnd

-- +goose StatementBegin
-- ai_text_jobs: 通用 JSON 结果 (词汇量测试题面 / RSS 生词提取等慢任务)
ALTER TABLE ai_text_jobs
    ADD COLUMN result_json LONGTEXT NULL COMMENT '任务结果 JSON';
-- +goose StatementEnd

-- +goose StatementBegin
-- 阅读行为日统计 (按日聚合, UPSERT 递增)
CREATE TABLE IF NOT EXISTS `reading_daily_stats` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `stat_date` DATE NOT NULL,
    `opens` INT NOT NULL DEFAULT 0 COMMENT '打开文章次数 (RSS+阅读)',
    `lookups` INT NOT NULL DEFAULT 0 COMMENT '点词查词次数',
    `words_added` INT NOT NULL DEFAULT 0 COMMENT '加入生词本次数',
    `read_seconds` INT NOT NULL DEFAULT 0 COMMENT '阅读停留秒数 (前端心跳上报)',
    UNIQUE KEY `uk_rds_user_date` (`user_id`, `stat_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- 词汇量评估记录 (词汇量测试历史)
CREATE TABLE IF NOT EXISTS `vocabulary_assessments` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `cefr` VARCHAR(4) NOT NULL COMMENT 'A1..C2',
    `est_size` INT NOT NULL COMMENT '估算词汇量',
    `detail` TEXT NULL COMMENT '各频段答题明细 JSON',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_va_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE vocabulary DROP COLUMN source_kind, DROP COLUMN source_ref, DROP COLUMN encounters, DROP COLUMN context_sentence;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE reading_docs DROP COLUMN est_unknown_rate, DROP COLUMN annotate_progress;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE rss_items DROP COLUMN difficult_words, DROP COLUMN est_unknown_rate;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE ai_text_jobs DROP COLUMN result_json;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `reading_daily_stats`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `vocabulary_assessments`;
-- +goose StatementEnd
