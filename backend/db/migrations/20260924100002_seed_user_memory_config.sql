-- +goose Up
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
('memory_extraction_enabled', 'true', '是否启用用户画像/经历抽取'),
('memory_extraction_model', '', '画像/经历抽取使用的模型代码(空则用默认模型)'),
('memory_session_idle_minutes', '15', '会话静默多少分钟后触发抽取'),
('memory_min_min_user_messages', '6', '纳入抽取的最小用户消息数(大于该值)')
ON DUPLICATE KEY UPDATE `value` = `value`;

-- +goose Down
DELETE FROM `system_config` WHERE `key` IN (
    'memory_extraction_enabled',
    'memory_extraction_model',
    'memory_session_idle_minutes',
    'memory_min_min_user_messages'
);