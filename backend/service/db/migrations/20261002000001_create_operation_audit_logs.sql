-- +goose Up

-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS operation_audit_logs (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NULL COMMENT '操作用户ID',
    user_name VARCHAR(50) NOT NULL DEFAULT '' COMMENT '操作用户名快照',
    method VARCHAR(10) NOT NULL COMMENT 'HTTP 方法',
    path VARCHAR(255) NOT NULL COMMENT '请求路径',
    status_code VARCHAR(10) NOT NULL DEFAULT '' COMMENT '业务响应码 (0000=成功)',
    success TINYINT(1) NOT NULL DEFAULT 1 COMMENT '操作是否成功',
    error_msg VARCHAR(500) NOT NULL DEFAULT '' COMMENT '失败原因 (业务msg)',
    request_body TEXT NULL COMMENT '请求参数 (脱敏/截断)',
    ip VARCHAR(64) NOT NULL DEFAULT '' COMMENT '客户端 IP',
    user_agent VARCHAR(255) NOT NULL DEFAULT '' COMMENT 'User-Agent',
    latency_ms INT NOT NULL DEFAULT 0 COMMENT '处理耗时(毫秒)',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    KEY idx_audit_user (user_id),
    KEY idx_audit_created_at (created_at),
    KEY idx_audit_path (path)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='后台操作审计日志 (变更类请求)';
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO `permissions` (`code`, `name`, `group_name`) VALUES
('system:audit:view', '查看审计日志', '系统管理')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `permissions` WHERE `code` = 'system:audit:view';
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS operation_audit_logs;
-- +goose StatementEnd
