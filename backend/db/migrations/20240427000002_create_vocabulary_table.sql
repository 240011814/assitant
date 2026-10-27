-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `vocabulary` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `word` VARCHAR(100) NOT NULL,
    `phonetic` VARCHAR(100),
    `definition` TEXT,
    `example` TEXT,
    `source_context` TEXT,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_user_id` (`user_id`),
    INDEX `idx_word` (`word`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `vocabulary`;
-- +goose StatementEnd
