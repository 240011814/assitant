-- +goose Up

-- +goose StatementBegin
-- RSS 订阅源
CREATE TABLE IF NOT EXISTS `rss_sources` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `title` VARCHAR(100) NOT NULL,
    `url` VARCHAR(500) NOT NULL,
    `enabled` TINYINT NOT NULL DEFAULT 1,
    `last_fetched_at` DATETIME NULL,
    `last_error` VARCHAR(500),
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX `idx_rss_source_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- RSS 文章条目 (正文按需抓取缓存)
CREATE TABLE IF NOT EXISTS `rss_items` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `source_id` INT NOT NULL,
    `guid` VARCHAR(255) NOT NULL,
    `title` VARCHAR(500) NOT NULL,
    `link` VARCHAR(500) NOT NULL,
    `summary` TEXT,
    `content` MEDIUMTEXT,
    `published_at` DATETIME NULL,
    `status` VARCHAR(20) NOT NULL DEFAULT 'unread' COMMENT 'unread/read/imported',
    `imported_doc_id` INT NOT NULL DEFAULT 0,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY `uk_rss_user_guid` (`user_id`, `guid`),
    INDEX `idx_rss_item_user` (`user_id`, `created_at`),
    INDEX `idx_rss_item_source` (`source_id`, `status`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `rss_items`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `rss_sources`;
-- +goose StatementEnd
