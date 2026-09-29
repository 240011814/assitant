-- +goose Up
-- 公共 agent 访问权限码: 非空时前端按该权限码控制公共 agent 的可见性/可访问性,
-- 空字符串表示对所有登录用户可见; 仅约束内置公共 agent (is_public=1), 个人 agent 不受影响
ALTER TABLE ai_agents ADD COLUMN permission_code VARCHAR(100) NOT NULL DEFAULT '' AFTER is_public;

-- +goose StatementBegin
-- 确保 4 个内置公共 agent 对应的权限码存在 (参考 20240429000002 / 20240506000007)
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('ai:chat:view', '英语训练', '练习管理'),
('ai:decision:view', '决策训练', '练习管理'),
('ai:social:view', '社交训练', '练习管理'),
('ai:emergency:view', '应急训练', '练习管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
-- 超级管理员默认拥有全部权限 (与 20240506000007 保持一致)
INSERT INTO `role_permissions` (`role_code`, `permission_code`)
SELECT 'R_SUPER', `code` FROM `permissions`
WHERE `code` IN ('ai:chat:view', 'ai:decision:view', 'ai:social:view', 'ai:emergency:view')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose StatementBegin
-- 为 4 个内置公共 agent 绑定访问权限码
UPDATE `ai_agents` SET `permission_code` = 'ai:chat:view' WHERE `code` = 'chat';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `ai_agents` SET `permission_code` = 'ai:decision:view' WHERE `code` = 'decision';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `ai_agents` SET `permission_code` = 'ai:social:view' WHERE `code` = 'social';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `ai_agents` SET `permission_code` = 'ai:emergency:view' WHERE `code` = 'emergency';
-- +goose StatementEnd

-- +goose Down
-- 仅回滚新增列; 4 个权限码为既有数据, 不在本迁移中删除
ALTER TABLE ai_agents DROP COLUMN permission_code;