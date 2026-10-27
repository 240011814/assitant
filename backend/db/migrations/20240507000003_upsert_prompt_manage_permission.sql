-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`)
VALUES ('ai:prompt:manage', '提示词设置', 'AI训练')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
UPDATE `permissions`
SET `name` = '用户提示词管理', `group_name` = 'AI训练'
WHERE `code` = 'ai:prompt:manage';
-- +goose StatementEnd
