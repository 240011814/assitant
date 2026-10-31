-- +goose Up

-- 角色编辑: 新增 system:role:edit 权限 (名称/描述可编辑, 编码是稳定标识不可改)。
-- 授予 R_SUPER, 与角色创建/删除的授予口径一致。

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
    ('system:role:edit', '编辑角色', '角色管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
    ('R_SUPER', 'system:role:edit')
ON DUPLICATE KEY UPDATE `role_code` = `role_code`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'system:role:edit';
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'system:role:edit';
-- +goose StatementEnd
