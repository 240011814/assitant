-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS error_book (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    content_type VARCHAR(20) NOT NULL COMMENT 'word 或 sentence',
    content TEXT NOT NULL COMMENT '单词或句子内容',
    translation TEXT COMMENT '中文翻译',
    source_type VARCHAR(20) COMMENT 'vocabulary 或 course',
    source_id BIGINT UNSIGNED COMMENT '原始记录ID',
    error_count INT DEFAULT 1 COMMENT '错误次数',
    is_mastered TINYINT(1) DEFAULT 0 COMMENT '是否已掌握',
    created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    INDEX idx_user_id (user_id),
    INDEX idx_source_id (source_id),
    INDEX idx_content_type (content_type),
    INDEX idx_is_mastered (is_mastered)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO permissions (code, name, group_name) VALUES
('ai:error-book:view', '错题本查看', 'AI 错题本'),
('ai:error-book:add', '错题本添加', 'AI 错题本'),
('ai:error-book:edit', '错题本编辑', 'AI 错题本'),
('ai:error-book:delete', '错题本删除', 'AI 错题本'),
('ai:error-book:practice', '错题本练习', 'AI 错题本');
-- +goose StatementEnd

-- +goose StatementBegin
INSERT INTO role_permissions (role_code, permission_code) VALUES
('R_SUPER', 'ai:error-book:view'),
('R_SUPER', 'ai:error-book:add'),
('R_SUPER', 'ai:error-book:edit'),
('R_SUPER', 'ai:error-book:delete'),
('R_SUPER', 'ai:error-book:practice'),
('R_ADMIN', 'ai:error-book:view'),
('R_ADMIN', 'ai:error-book:add'),
('R_ADMIN', 'ai:error-book:edit'),
('R_ADMIN', 'ai:error-book:delete'),
('R_ADMIN', 'ai:error-book:practice'),
('R_USER', 'ai:error-book:view'),
('R_USER', 'ai:error-book:add'),
('R_USER', 'ai:error-book:edit'),
('R_USER', 'ai:error-book:delete'),
('R_USER', 'ai:error-book:practice');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DELETE FROM role_permissions WHERE permission_code LIKE 'ai:error-book:%';
DELETE FROM permissions WHERE code LIKE 'ai:error-book:%';
DROP TABLE IF EXISTS error_book;
-- +goose StatementEnd
