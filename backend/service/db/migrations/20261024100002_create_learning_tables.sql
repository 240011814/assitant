-- +goose Up

-- +goose StatementBegin
-- 词汇复习日志: 支撑看板 (热力图/保持率/连续天数) 与每日新词限额计数
CREATE TABLE IF NOT EXISTS `vocabulary_review_logs` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `vocabulary_id` INT NOT NULL,
    `direction` VARCHAR(20) NOT NULL DEFAULT 'en2zh' COMMENT '复习方向 en2zh/zh2en/listen',
    `rating` TINYINT NOT NULL COMMENT 'FSRS 评分 1=Again 2=Hard 3=Good 4=Easy',
    `state_before` TINYINT NOT NULL DEFAULT 0 COMMENT '复习前 FSRS 状态',
    `stability` DOUBLE NOT NULL DEFAULT 0 COMMENT '复习后稳定性',
    `difficulty` DOUBLE NOT NULL DEFAULT 0 COMMENT '复习后难度',
    `due` DATETIME NULL COMMENT '复习后下次到期时间',
    `duration_ms` INT NOT NULL DEFAULT 0 COMMENT '本次复习用时',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_vrl_user_date` (`user_id`, `created_at`),
    INDEX `idx_vrl_vocab` (`vocabulary_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- 出题练习会话: 题目与答案以 JSON 持久化, 答题时服务端判分
CREATE TABLE IF NOT EXISTS `quiz_sessions` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `questions` LONGTEXT NOT NULL COMMENT '题目 JSON (含正确答案)',
    `total` INT NOT NULL DEFAULT 0,
    `answered` INT NOT NULL DEFAULT 0,
    `correct` INT NOT NULL DEFAULT 0,
    `status` VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT 'active/finished',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_quiz_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- 分级阅读文档
CREATE TABLE IF NOT EXISTS `reading_docs` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `title` VARCHAR(200) NOT NULL,
    `source_type` VARCHAR(20) NOT NULL DEFAULT 'text' COMMENT 'url/text',
    `source_url` VARCHAR(500),
    `content` MEDIUMTEXT NOT NULL COMMENT '纯文本正文 (段落以空行分隔)',
    `word_count` INT NOT NULL DEFAULT 0,
    `difficult_words` MEDIUMTEXT COMMENT 'LLM 标注生词 JSON [{word,meaning,reason,known}]',
    `unknown_rate` DOUBLE NOT NULL DEFAULT 0 COMMENT '生词率 (标注生词/总词数)',
    `status` VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT 'pending/annotating/annotated/failed',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_reading_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- 单词词典缓存 (点词即查: LLM 查询结果全局复用)
CREATE TABLE IF NOT EXISTS `dictionary_cache` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `word` VARCHAR(100) NOT NULL UNIQUE,
    `phonetic` VARCHAR(100),
    `definition` TEXT COMMENT '英文释义',
    `translation` TEXT COMMENT '中文释义',
    `example` TEXT COMMENT '例句',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `dictionary_cache`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `reading_docs`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `quiz_sessions`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `vocabulary_review_logs`;
-- +goose StatementEnd
