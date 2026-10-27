-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('ai:course:view', '查看课程包', 'AI课程'),
('ai:course:create', '创建课程包', 'AI课程'),
('ai:course:edit', '编辑课程包', 'AI课程'),
('ai:course:delete', '删除课程包', 'AI课程');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`)
SELECT 'R_SUPER', `code` FROM `permissions` WHERE `code` LIKE 'ai:course:%';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'ai:course:%';
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` LIKE 'ai:course:%';
-- +goose StatementEnd
