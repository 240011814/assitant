-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `training_messages` (
    `id` BIGINT AUTO_INCREMENT PRIMARY KEY,
    `history_id` INT NOT NULL,
    `role` VARCHAR(20) NOT NULL COMMENT 'system/user/assistant',
    `content` LONGTEXT NOT NULL,
    `sort_order` INT NOT NULL DEFAULT 0 COMMENT '消息顺序',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_history_id (history_id),
    FOREIGN KEY (history_id) REFERENCES training_histories(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `training_messages`;
-- +goose StatementEnd
