-- +goose Up
-- +goose StatementBegin
INSERT INTO system_config (`key`, `value`, `remark`, `created_at`, `updated_at`) VALUES
('smtp_enabled', 'false', 'SMTP邮件服务开关', NOW(), NOW()),
('smtp_host', '', 'SMTP服务器地址', NOW(), NOW()),
('smtp_port', '587', 'SMTP服务器端口', NOW(), NOW()),
('smtp_encryption', 'ssl', 'SMTP加密方式(none/ssl/starttls)', NOW(), NOW()),
('smtp_user', '', 'SMTP用户名', NOW(), NOW()),
('smtp_password', '', 'SMTP密码', NOW(), NOW()),
('smtp_from', '', '发件人邮箱地址', NOW(), NOW()),
('smtp_from_name', '', '发件人显示名称', NOW(), NOW())
ON DUPLICATE KEY UPDATE `updated_at` = NOW();
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM system_config WHERE `key` IN ('smtp_enabled', 'smtp_host', 'smtp_port', 'smtp_encryption', 'smtp_user', 'smtp_password', 'smtp_from', 'smtp_from_name');
-- +goose StatementEnd
