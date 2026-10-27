-- +goose Up
-- 插入 AI 配置管理相关权限
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('sys:ai:config:list', '查询AI配置', '系统管理'),
('sys:ai:config:update', '修改AI配置', '系统管理'),
('sys:ai:config:delete', '删除AI配置', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);

-- 给超级管理员分配权限 (R_SUPER)
-- 注意：根据 main.go 和 api/middleware.go，R_SUPER 默认拥有全部权限
-- 但为了在某些查询中体现，我们还是显式插入
INSERT INTO `role_permissions` (`role_code`, `permission_code`)
VALUES 
('R_SUPER', 'sys:ai:config:list'),
('R_SUPER', 'sys:ai:config:update'),
('R_SUPER', 'sys:ai:config:delete')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);

-- +goose Down
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'sys:ai:config%';
DELETE FROM `permissions` WHERE `code` LIKE 'sys:ai:config%';
