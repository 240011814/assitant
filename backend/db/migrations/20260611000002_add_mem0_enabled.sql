-- +goose Up
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
('mem0_enabled', 'true', 'Mem0 记忆服务开关')
ON DUPLICATE KEY UPDATE `value` = `value`;

-- +goose Down
DELETE FROM `system_config` WHERE `key` = 'mem0_enabled';
