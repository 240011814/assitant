-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('ai:custom-training:view', '自定义训练', '练习管理'),
('ai:custom-training:create', '创建自定义训练', '练习管理'),
('ai:custom-training:edit', '编辑自定义训练', '练习管理'),
('ai:custom-training:delete', '删除自定义训练', '练习管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN ('ai:custom-training:view', 'ai:custom-training:create', 'ai:custom-training:edit', 'ai:custom-training:delete');
-- +goose StatementEnd
