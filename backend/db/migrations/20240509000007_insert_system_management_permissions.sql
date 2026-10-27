-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
-- 用户管理权限
('system:user:list', '查看用户', '用户管理'),
('system:user:create', '创建用户', '用户管理'),
('system:user:update', '编辑用户', '用户管理'),
('system:user:delete', '删除用户', '用户管理'),
-- 角色管理权限
('system:role:list', '查看角色', '角色管理'),
('system:role:permission:view', '查看角色权限', '角色管理'),
('system:role:permission:update', '配置角色权限', '角色管理'),
-- 权限管理权限
('system:permission:create', '创建权限', '权限管理'),
('system:permission:delete', '删除权限', '权限管理'),
-- AI配置权限
('system:ai-provider:view', '查看AI提供商', 'AI配置'),
('system:ai-provider:create', '创建AI提供商', 'AI配置'),
('system:ai-provider:update', '编辑AI提供商', 'AI配置'),
('system:ai-provider:delete', '删除AI提供商', 'AI配置'),
('system:ai-model:view', '查看AI模型', 'AI配置'),
('system:ai-model:create', '创建AI模型', 'AI配置'),
('system:ai-model:update', '编辑AI模型', 'AI配置'),
('system:ai-model:delete', '删除AI模型', 'AI配置')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN (
  'system:user:list', 'system:user:create', 'system:user:update', 'system:user:delete',
  'system:role:list', 'system:role:permission:view', 'system:role:permission:update',
  'system:permission:create', 'system:permission:delete',
  'system:ai-provider:view', 'system:ai-provider:create', 'system:ai-provider:update', 'system:ai-provider:delete',
  'system:ai-model:view', 'system:ai-model:create', 'system:ai-model:update', 'system:ai-model:delete'
);
-- +goose StatementEnd
