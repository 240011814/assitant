-- +goose Up
-- Create user_prompts table
CREATE TABLE IF NOT EXISTS user_prompts (
    id INT AUTO_INCREMENT PRIMARY KEY,
    user_id INT NOT NULL,
    module_key VARCHAR(50) NOT NULL,
    custom_prompt LONGTEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    CONSTRAINT idx_user_module UNIQUE (user_id, module_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- Add permission for user prompt management
INSERT INTO permissions (`code`, `name`, `group_name`) 
VALUES ('ai:prompt:manage', '用户提示词管理', 'AI训练')
ON DUPLICATE KEY UPDATE `name` = VALUES(`name`), `group_name` = VALUES(`group_name`);

-- +goose Down
DROP TABLE IF EXISTS user_prompts;
DELETE FROM permissions WHERE code = 'ai:prompt:manage';
