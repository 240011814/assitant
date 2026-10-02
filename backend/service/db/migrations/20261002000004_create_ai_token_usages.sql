-- +goose Up

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS ai_token_usages (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL DEFAULT 0 COMMENT '用户ID (0=系统/无用户上下文)',
    model VARCHAR(100) NOT NULL DEFAULT '' COMMENT '模型 code (空=默认模型)',
    source VARCHAR(30) NOT NULL DEFAULT '' COMMENT '来源: chat=对话, orchestration_debug=编排调试, orchestration_chat=编排对话',
    prompt_tokens INT NOT NULL DEFAULT 0 COMMENT '输入 token 数',
    completion_tokens INT NOT NULL DEFAULT 0 COMMENT '输出 token 数',
    total_tokens INT NOT NULL DEFAULT 0 COMMENT '总 token 数',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_token_usage_user_time (user_id, created_at),
    KEY idx_token_usage_model_time (model, created_at),
    KEY idx_token_usage_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户 AI Token 用量记录 (每次模型调用一条)';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:tokenusage:view', 'Token 用量统计', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'system:tokenusage:view';
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS ai_token_usages;
-- +goose StatementEnd
