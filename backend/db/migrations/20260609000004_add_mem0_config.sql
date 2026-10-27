-- +goose Up
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
('mem0_api_key', '', 'Mem0 API 密钥'),
('mem0_base_url', 'https://api.mem0.ai/v1', 'Mem0 API 地址')
ON DUPLICATE KEY UPDATE `value` = `value`;

-- +goose Down
DELETE FROM `system_config` WHERE `key` IN ('mem0_api_key', 'mem0_base_url');
