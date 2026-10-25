-- +goose Up

-- +goose StatementBegin
-- 文档句子中文翻译缓存 (文章翻译题题面, 全局共享)
CREATE TABLE IF NOT EXISTS `quiz_sentence_zh` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `sentence` VARCHAR(1000) NOT NULL,
    `zh` VARCHAR(1000) NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY `uk_qsz_sentence` (`sentence`(500))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `quiz_sentence_zh`;
-- +goose StatementEnd
