-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('ai:prompt:manage', '管理提示词', 'AI训练')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'ai:prompt:manage'),
('R_ADMIN', 'ai:prompt:manage'),
('R_USER', 'ai:prompt:manage')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'ai:prompt:manage';
DELETE FROM `permissions` WHERE `code` = 'ai:prompt:manage';
-- +goose StatementEnd
