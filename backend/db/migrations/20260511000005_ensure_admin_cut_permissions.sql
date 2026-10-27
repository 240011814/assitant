-- +goose Up
-- +goose StatementBegin
INSERT INTO `roles` (`code`, `name`, `description`) VALUES
('R_ADMIN', '管理员', '可管理业务数据')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_ADMIN', 'cut:menu:view'),
('R_ADMIN', 'cut:bar:compute'),
('R_ADMIN', 'cut:plane:compute'),
('R_ADMIN', 'cut:record:view'),
('R_ADMIN', 'cut:record:create'),
('R_ADMIN', 'cut:record:delete')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `role_code` = 'R_ADMIN' AND `permission_code` LIKE 'cut:%';
-- +goose StatementEnd
