-- +goose Up
-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'cut:menu:view'),
('R_SUPER', 'cut:bar:compute'),
('R_SUPER', 'cut:plane:compute'),
('R_SUPER', 'cut:record:view'),
('R_SUPER', 'cut:record:create'),
('R_SUPER', 'cut:record:delete'),
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
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'cut:%';
-- +goose StatementEnd
