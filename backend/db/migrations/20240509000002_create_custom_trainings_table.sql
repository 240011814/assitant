-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied
CREATE TABLE IF NOT EXISTS custom_trainings (
    id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
    user_id BIGINT UNSIGNED NOT NULL,
    title VARCHAR(100) NOT NULL,
    description TEXT,
    system_prompt TEXT NOT NULL,
    icon VARCHAR(50) DEFAULT 'mdi:robot-outline',
    color VARCHAR(20) DEFAULT '#2080f0',
    initial_message TEXT,
    input_placeholder VARCHAR(200) DEFAULT '输入消息... (回车发送，Shift + 回车换行)',
    speech_lang VARCHAR(20) DEFAULT 'zh-CN',
    speech_rate DECIMAL(3,2) DEFAULT 0.95,
    is_favorite BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- +goose Down
-- SQL in section 'Down' is executed when this migration is rolled back
DROP TABLE IF EXISTS custom_trainings;
