-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
-- AI 相关权限
('ai:model:view', '查看AI模型', 'AI训练'),
('ai:chat:send', '发送聊天消息', 'AI训练'),
('ai:prompt:view', '查看提示词', 'AI训练'),
('ai:prompt:save', '保存提示词', 'AI训练'),
('ai:prompt:switch', '切换提示词版本', 'AI训练'),
('ai:prompt:delete', '删除提示词版本', 'AI训练'),
('ai:prompt:reset', '重置提示词', 'AI训练'),
('ai:vocabulary:view', '查看生词', '生词本'),
('ai:vocabulary:add', '添加生词', '生词本'),
('ai:vocabulary:edit', '编辑生词', '生词本'),
('ai:vocabulary:delete', '删除生词', '生词本'),
('ai:note:view', '查看笔记', '笔记本'),
('ai:note:create', '创建笔记', '笔记本'),
('ai:note:edit', '编辑笔记', '笔记本'),
('ai:note:delete', '删除笔记', '笔记本'),
('ai:history:view', '查看历史记录', '历史记录'),
('ai:history:favorite', '收藏历史记录', '历史记录'),
('ai:history:edit', '编辑历史记录', '历史记录'),
('ai:history:delete', '删除历史记录', '历史记录'),
-- 系统管理权限
('system:role:list', '查看角色', '角色管理'),
('system:role:create', '创建角色', '角色管理'),
('system:role:delete', '删除角色', '角色管理'),
('system:role:permission:view', '查看角色权限', '角色管理'),
('system:role:permission:update', '配置角色权限', '角色管理'),
('system:ai-provider:view', '查看AI提供商', 'AI配置'),
('system:ai-provider:create', '创建AI提供商', 'AI配置'),
('system:ai-provider:update', '编辑AI提供商', 'AI配置'),
('system:ai-provider:delete', '删除AI提供商', 'AI配置'),
('system:ai-model:view', '查看AI模型配置', 'AI配置'),
('system:ai-model:create', '创建AI模型配置', 'AI配置'),
('system:ai-model:update', '编辑AI模型配置', 'AI配置'),
('system:ai-model:delete', '删除AI模型配置', 'AI配置')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN (
  'ai:model:view', 'ai:chat:send',
  'ai:prompt:view', 'ai:prompt:save', 'ai:prompt:switch', 'ai:prompt:delete', 'ai:prompt:reset',
  'ai:vocabulary:view', 'ai:vocabulary:add', 'ai:vocabulary:edit', 'ai:vocabulary:delete',
  'ai:note:view', 'ai:note:create', 'ai:note:edit', 'ai:note:delete',
  'ai:history:view', 'ai:history:favorite', 'ai:history:edit', 'ai:history:delete',
  'system:role:list', 'system:role:create', 'system:role:delete',
  'system:role:permission:view', 'system:role:permission:update',
  'system:ai-provider:view', 'system:ai-provider:create', 'system:ai-provider:update', 'system:ai-provider:delete',
  'system:ai-model:view', 'system:ai-model:create', 'system:ai-model:update', 'system:ai-model:delete'
);
-- +goose StatementEnd
