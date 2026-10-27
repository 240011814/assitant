-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('sys:menu:view', '查看系统菜单', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'sys:menu:view'),
('R_ADMIN', 'sys:menu:view')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'sys:menu:view';
DELETE FROM `permissions` WHERE `code` = 'sys:menu:view';
-- +goose StatementEnd
