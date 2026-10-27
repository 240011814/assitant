-- +goose Up
ALTER TABLE user_prompts ADD COLUMN agent_id BIGINT UNSIGNED NOT NULL DEFAULT 0 AFTER user_id;
DROP INDEX idx_user_module_active ON user_prompts;
ALTER TABLE user_prompts DROP COLUMN module_key;
CREATE INDEX idx_user_agent ON user_prompts (user_id, agent_id);
CREATE INDEX idx_user_agent_active ON user_prompts (user_id, agent_id, is_active);

-- +goose Down
ALTER TABLE user_prompts ADD COLUMN module_key VARCHAR(50) NOT NULL DEFAULT '' AFTER user_id;
DROP INDEX idx_user_agent ON user_prompts;
DROP INDEX idx_user_agent_active ON user_prompts;
ALTER TABLE user_prompts DROP COLUMN agent_id;
CREATE INDEX idx_user_module_active ON user_prompts (user_id, module_key, is_active);
