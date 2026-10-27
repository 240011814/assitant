-- +goose Up
-- 编排 Agent 节点提示词版本控制: 与用户提示词共用本表,
-- node_key 为画布节点 id (普通用户提示词恒为空串, 行为不变)
ALTER TABLE user_prompts ADD COLUMN node_key VARCHAR(64) NOT NULL DEFAULT '' AFTER agent_id;
CREATE INDEX idx_user_agent_node ON user_prompts (user_id, agent_id, node_key, is_active);

-- +goose Down
DROP INDEX idx_user_agent_node ON user_prompts;
ALTER TABLE user_prompts DROP COLUMN node_key;
