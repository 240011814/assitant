-- +goose Up
ALTER TABLE user_prompts ADD COLUMN memory_search_query VARCHAR(500) DEFAULT '' AFTER custom_prompt;

-- +goose Down
ALTER TABLE user_prompts DROP COLUMN memory_search_query;
