-- +goose Up
-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('tool:mihoyo:view', '查看米游社绑定与签到状态', '米游社签到'),
('tool:mihoyo:manage', '米游社扫码绑定/手动签到/解绑', '米游社签到')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd



-- +goose Down
-- +goose StatementBegin
DELETE FROM `role_permissions` WHERE `permission_code` IN ('tool:mihoyo:view', 'tool:mihoyo:manage');
-- +goose StatementEnd

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` IN ('tool:mihoyo:view', 'tool:mihoyo:manage');
-- +goose StatementEnd
