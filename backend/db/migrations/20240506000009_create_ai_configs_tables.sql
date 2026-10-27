-- +goose Up
-- AI Provider 密钥管理表
CREATE TABLE IF NOT EXISTS ai_providers (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(50) NOT NULL COMMENT '提供商名称如 DeepSeek, Gemini',
    api_key VARCHAR(255) NOT NULL COMMENT 'API 密钥',
    base_url VARCHAR(255) DEFAULT '' COMMENT 'API 基准地址',
    is_active TINYINT(1) DEFAULT 0 COMMENT '是否启用',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- AI 模型管理表
CREATE TABLE IF NOT EXISTS ai_models (
    id INT AUTO_INCREMENT PRIMARY KEY,
    provider_id INT NOT NULL COMMENT '关联提供商ID',
    model_code VARCHAR(100) NOT NULL COMMENT '模型标识如 deepseek-chat',
    display_name VARCHAR(100) NOT NULL COMMENT '显示名称',
    is_default TINYINT(1) DEFAULT 0 COMMENT '是否为默认模型',
    config_json TEXT COMMENT '运行参数 JSON (temperature, max_tokens等)',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    FOREIGN KEY (provider_id) REFERENCES ai_providers(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- +goose Down
DROP TABLE IF EXISTS ai_models;
DROP TABLE IF EXISTS ai_providers;
