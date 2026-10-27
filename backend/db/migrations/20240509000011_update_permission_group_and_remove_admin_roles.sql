-- +goose Up
-- +goose StatementBegin
UPDATE `permissions` SET `group_name` = '系统管理' WHERE `code` LIKE 'system:permission:%';
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `role_code` = 'R_ADMIN' AND `permission_code` IN ('system:permission:view', 'system:permission:create', 'system:permission:update', 'system:permission:delete', 'system:role:list', 'system:role:create', 'system:role:delete', 'system:role:permission:view', 'system:role:permission:update');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE `permissions` SET `group_name` = '权限管理' WHERE `code` LIKE 'system:permission:%';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_ADMIN', 'system:permission:view'),
('R_ADMIN', 'system:role:list'),
('R_ADMIN', 'system:role:permission:view')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd
