-- +goose Up
-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
-- R_SUPER 拥有所有系统管理权限
('R_SUPER', 'system:user:list'),
('R_SUPER', 'system:user:create'),
('R_SUPER', 'system:user:update'),
('R_SUPER', 'system:user:delete'),
('R_SUPER', 'system:role:list'),
('R_SUPER', 'system:role:permission:view'),
('R_SUPER', 'system:role:permission:update'),
('R_SUPER', 'system:permission:view'),
('R_SUPER', 'system:permission:update'),
('R_SUPER', 'system:permission:create'),
('R_SUPER', 'system:permission:delete'),
('R_SUPER', 'system:ai-provider:view'),
('R_SUPER', 'system:ai-provider:create'),
('R_SUPER', 'system:ai-provider:update'),
('R_SUPER', 'system:ai-provider:delete'),
('R_SUPER', 'system:ai-model:view'),
('R_SUPER', 'system:ai-model:create'),
('R_SUPER', 'system:ai-model:update'),
('R_SUPER', 'system:ai-model:delete'),
-- R_ADMIN 拥有部分系统管理权限
('R_ADMIN', 'system:user:list'),
('R_ADMIN', 'system:user:create'),
('R_ADMIN', 'system:user:update'),
('R_ADMIN', 'system:role:list'),
('R_ADMIN', 'system:role:permission:view'),
('R_ADMIN', 'system:permission:view'),
('R_ADMIN', 'system:ai-provider:view'),
('R_ADMIN', 'system:ai-provider:create'),
('R_ADMIN', 'system:ai-provider:update'),
('R_ADMIN', 'system:ai-model:view'),
('R_ADMIN', 'system:ai-model:create'),
('R_ADMIN', 'system:ai-model:update')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` IN (
  'system:user:list', 'system:user:create', 'system:user:update', 'system:user:delete',
  'system:role:list', 'system:role:permission:view', 'system:role:permission:update',
  'system:permission:view', 'system:permission:update', 'system:permission:create', 'system:permission:delete',
  'system:ai-provider:view', 'system:ai-provider:create', 'system:ai-provider:update', 'system:ai-provider:delete',
  'system:ai-model:view', 'system:ai-model:create', 'system:ai-model:update', 'system:ai-model:delete'
);
-- +goose StatementEnd
