-- +goose Up

-- +goose StatementBegin
-- 用户画像(每用户一条), 由聊天会话抽取/合并而来
CREATE TABLE IF NOT EXISTS user_portraits (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    summary TEXT COMMENT '整体画像文本, 用于注入对话',
    dimensions JSON COMMENT '结构化维度, 如 {"职业":"","目标":""}',
    tags JSON COMMENT '兴趣/特征标签',
    confidence DECIMAL(4,3) NOT NULL DEFAULT 0 COMMENT '整体置信度 0~1',
    is_user_edited TINYINT(1) NOT NULL DEFAULT 0 COMMENT '用户是否手动编辑过',
    extraction_enabled TINYINT(1) NOT NULL DEFAULT 1 COMMENT '是否参与自动抽取',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_user (user_id),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- 用户经历(一个会话对应一条)
CREATE TABLE IF NOT EXISTS user_experiences (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    history_id BIGINT UNSIGNED NULL COMMENT '来源会话ID, 手动新增为空',
    category VARCHAR(32) NOT NULL DEFAULT '' COMMENT 'work/project/study/achievement/challenge/other',
    title VARCHAR(255) NOT NULL DEFAULT '',
    content TEXT,
    occurred_at DATETIME NULL COMMENT '发生时间, 默认取会话创建时间',
    tags JSON,
    confidence DECIMAL(4,3) NOT NULL DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'active' COMMENT 'active/archived',
    is_user_edited TINYINT(1) NOT NULL DEFAULT 0,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_history (history_id),
    KEY idx_user_category (user_id, category),
    KEY idx_user_occurred (user_id, occurred_at),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose StatementBegin
-- 会话级抽取进度(取代不合理的全局水位)
CREATE TABLE IF NOT EXISTS memory_extraction_states (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    history_id BIGINT UNSIGNED NOT NULL,
    user_id INT NOT NULL,
    last_sort_order INT NOT NULL DEFAULT 0 COMMENT '已抽取到会话内第几条消息',
    status VARCHAR(20) NOT NULL DEFAULT 'pending' COMMENT 'pending/processing/done/failed/skipped',
    error TEXT,
    extracted_at DATETIME NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    UNIQUE KEY uk_history (history_id),
    KEY idx_user_status (user_id, status),
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS memory_extraction_states;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS user_experiences;
-- +goose StatementEnd

-- +goose StatementBegin
DROP TABLE IF EXISTS user_portraits;
-- +goose StatementEnd