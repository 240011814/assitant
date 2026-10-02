-- +goose Up

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:dashboard:view', '查看系统概览', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'system:dashboard:view';
-- +goose StatementEnd
