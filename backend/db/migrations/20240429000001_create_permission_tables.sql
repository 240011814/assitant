-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `roles` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `code` VARCHAR(50) NOT NULL UNIQUE,
    `name` VARCHAR(100) NOT NULL,
    `description` VARCHAR(255),
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `permissions` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `code` VARCHAR(100) NOT NULL UNIQUE,
    `name` VARCHAR(100) NOT NULL,
    `group_name` VARCHAR(100) NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS `role_permissions` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `role_code` VARCHAR(50) NOT NULL,
    `permission_code` VARCHAR(100) NOT NULL,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE KEY `uk_role_permission` (`role_code`, `permission_code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `roles` (`code`, `name`, `description`) VALUES
('R_SUPER', '超级管理员', '拥有全部系统权限'),
('R_ADMIN', '管理员', '可管理业务数据'),
('R_USER', '普通用户', '基础使用权限')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `description` = VALUES(`description`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:user:list', '查看用户', '用户管理'),
('system:user:create', '新增用户', '用户管理'),
('system:user:update', '编辑用户', '用户管理'),
('system:user:delete', '删除用户', '用户管理'),
('system:permission:view', '查看权限', '权限管理'),
('system:permission:update', '配置权限', '权限管理'),
('vocabulary:list', '查看生词', '生词本'),
('vocabulary:update', '更新生词', '生词本'),
('vocabulary:delete', '删除生词', '生词本')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_ADMIN', 'vocabulary:list'),
('R_ADMIN', 'vocabulary:update'),
('R_ADMIN', 'vocabulary:delete'),
('R_USER', 'vocabulary:list'),
('R_USER', 'vocabulary:update'),
('R_USER', 'vocabulary:delete')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE IF EXISTS `role_permissions`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `permissions`;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS `roles`;
-- +goose StatementEnd
