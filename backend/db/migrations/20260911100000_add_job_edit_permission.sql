-- +goose Up

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('job:edit', '定时任务编辑', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_ADMIN', 'job:edit')
ON DUPLICATE KEY UPDATE `permission_code` = VALUES(`permission_code`);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'job:edit';
DELETE FROM `permissions` WHERE `code` = 'job:edit';
-- +goose StatementEnd
