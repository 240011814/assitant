-- +goose Up
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
('ai_timeout_minutes', '5', 'AI 请求超时时间（分钟）'),
('ai_tls_handshake_timeout', '15', 'AI TLS 握手超时时间（秒）'),
('ai_response_header_timeout', '30', 'AI 响应头超时时间（秒）'),
('http_timeout_seconds', '30', 'HTTP 请求超时时间（秒）')
ON DUPLICATE KEY UPDATE `value` = `value`;

-- +goose Down
DELETE FROM `system_config` WHERE `key` IN ('ai_timeout_minutes', 'ai_tls_handshake_timeout', 'ai_response_header_timeout', 'http_timeout_seconds');
