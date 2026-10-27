-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:permission:view', '查看权限', '系统管理'),
('system:permission:update', '配置权限', '系统管理'),
('system:role:create', '新增角色', '系统管理'),
('system:role:delete', '删除角色', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN ('system:permission:view', 'system:permission:update', 'system:role:create', 'system:role:delete');
-- +goose StatementEnd
