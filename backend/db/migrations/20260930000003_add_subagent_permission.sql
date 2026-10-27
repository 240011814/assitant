-- +goose Up
-- 子Agent 统一权限码: 所有子Agent 共用一个 code (ai:subagent:manage)。
-- 拥有它的用户才能新建/编辑/删除子Agent (agent_type='subagent'),
-- 也只有拥有它的用户才能在 Agent Studio 编排的「引用 Agent」下拉里看到/引用子Agent。
-- 统一常量定义见 backend/model/ai_agent.go: AIAgentSubAgentPermission。

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('ai:subagent:manage', '子Agent管理', '练习管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
-- 超级管理员默认拥有 (与既有权限迁移保持一致; 其它角色需在角色权限里手动授予)
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'ai:subagent:manage')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'ai:subagent:manage';
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'ai:subagent:manage';
-- +goose StatementEnd