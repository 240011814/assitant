-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('ai:history:view', '历史记录', '练习管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
-- +goose StatementEnd
