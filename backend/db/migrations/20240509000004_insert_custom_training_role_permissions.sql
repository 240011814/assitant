-- +goose Up
-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'ai:custom-training:view'),
('R_SUPER', 'ai:custom-training:create'),
('R_SUPER', 'ai:custom-training:edit'),
('R_SUPER', 'ai:custom-training:delete'),
('R_ADMIN', 'ai:custom-training:view'),
('R_ADMIN', 'ai:custom-training:create'),
('R_ADMIN', 'ai:custom-training:edit'),
('R_ADMIN', 'ai:custom-training:delete')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'ai:custom-training:%';
-- +goose StatementEnd
