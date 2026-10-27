-- +goose Up
ALTER TABLE `users` ADD COLUMN `totp_secret` VARCHAR(64) DEFAULT NULL AFTER `role`;

INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
('admin_2fa_enabled', 'false', '管理员二次验证(TOTP)开关')
ON DUPLICATE KEY UPDATE `value` = `value`;

-- +goose Down
ALTER TABLE `users` DROP COLUMN `totp_secret`;
DELETE FROM `system_config` WHERE `key` = 'admin_2fa_enabled';
