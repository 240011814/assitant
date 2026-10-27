-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `cut_record` (
    `id` VARCHAR(100) NOT NULL,
    `type` VARCHAR(20) NULL,
    `request` TEXT NULL,
    `response` TEXT NULL,
    `create_time` DATETIME NULL DEFAULT CURRENT_TIMESTAMP,
    `user_id` BIGINT UNSIGNED NOT NULL,
    `code` VARCHAR(100) NULL,
    `name` VARCHAR(100) NOT NULL,
    PRIMARY KEY (`id`),
    INDEX `idx_create_time` (`create_time`),
    INDEX `idx_type` (`type`),
    INDEX `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `cut_record`;
-- +goose StatementEnd
