-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('model_scenario:view', '查看模型场景', '模型和场景'),
('model_scenario:create', '创建模型场景', '模型和场景'),
('model_scenario:update', '更新模型场景', '模型和场景'),
('model_scenario:delete', '删除模型场景', '模型和场景')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `role_permissions` (`role_code`, `permission_code`) VALUES
('R_SUPER', 'model_scenario:view'),
('R_SUPER', 'model_scenario:create'),
('R_SUPER', 'model_scenario:update'),
('R_SUPER', 'model_scenario:delete'),
('R_ADMIN', 'model_scenario:view'),
('R_ADMIN', 'model_scenario:create'),
('R_ADMIN', 'model_scenario:update'),
('R_ADMIN', 'model_scenario:delete')
ON DUPLICATE KEY UPDATE `role_code` = VALUES(`role_code`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'model_scenario:%';
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` LIKE 'model_scenario:%';
-- +goose StatementEnd
