-- +goose Up
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'system:tokenusage:view';
-- +goose StatementEnd
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'system:tokenusage:view';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:tokenusage:view', 'Token 用量统计', '系统管理');
-- +goose StatementEnd
-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'system:tokenusage:view');
-- +goose StatementEnd
