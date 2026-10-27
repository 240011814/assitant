-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `training_histories` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `training_type` VARCHAR(50) NOT NULL COMMENT '训练类型，如chat, decision, social',
    `title` VARCHAR(255) NOT NULL COMMENT '训练标题或简介',
    `status` VARCHAR(20) NOT NULL DEFAULT 'completed' COMMENT '状态',
    `duration` INT NOT NULL DEFAULT 0 COMMENT '训练时长(秒)',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id),
    INDEX idx_type (training_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `training_histories`;
-- +goose StatementEnd
