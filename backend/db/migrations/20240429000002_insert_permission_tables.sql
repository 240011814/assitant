-- +goose Up


-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('ai:chat:view', '英语训练', '练习管理'),
('ai:decision:view', '决策训练', '练习管理'),
('ai:social:view', '社交训练', '练习管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd


-- +goose Down


