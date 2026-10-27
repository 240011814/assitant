-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('lottery:record:delete', '删除抽奖记录', '抽奖管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'lottery:record:delete'),
('R_ADMIN', 'lottery:record:delete')
ON DUPLICATE KEY UPDATE `role_code` = VALUES(`role_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` = 'lottery:record:delete';
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'lottery:record:delete';
-- +goose StatementEnd
