-- +goose Up
CREATE TABLE IF NOT EXISTS `system_config` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `key` VARCHAR(100) NOT NULL UNIQUE,
    `value` VARCHAR(500) NOT NULL DEFAULT '',
    `remark` VARCHAR(255) DEFAULT '',
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 默认关闭注册
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES ('register_enabled', 'false', '注册功能开关');

-- +goose Down
DROP TABLE IF EXISTS `system_config`;
