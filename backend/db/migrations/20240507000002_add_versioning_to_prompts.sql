-- +goose Up
-- Update user_prompts table to support versioning
ALTER TABLE user_prompts DROP INDEX idx_user_module;

ALTER TABLE user_prompts ADD COLUMN version INT NOT NULL DEFAULT 1;
ALTER TABLE user_prompts ADD COLUMN is_active TINYINT(1) DEFAULT 1;
ALTER TABLE user_prompts ADD COLUMN remark VARCHAR(255);

-- 添加新的复合索引
CREATE INDEX idx_user_module_active ON user_prompts (user_id, module_key, is_active);

-- +goose Down
DROP INDEX idx_user_module_active ON user_prompts;
ALTER TABLE user_prompts DROP COLUMN version;
ALTER TABLE user_prompts DROP COLUMN is_active;
ALTER TABLE user_prompts DROP COLUMN remark;
ALTER TABLE user_prompts ADD CONSTRAINT idx_user_module UNIQUE (user_id, module_key);
