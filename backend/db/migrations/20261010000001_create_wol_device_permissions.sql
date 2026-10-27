-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('tool:device:view', '查看设备管理', '设备管理'),
('tool:device:manage', '管理设备 (连接/唤醒/地址簿)', '设备管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT IGNORE INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_ADMIN', 'tool:device:view'),
('R_ADMIN', 'tool:device:manage');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` IN ('tool:device:view', 'tool:device:manage');
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN ('tool:device:view', 'tool:device:manage');
-- +goose StatementEnd