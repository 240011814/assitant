-- +goose Up
ALTER TABLE user_prompts ADD COLUMN memory_search_top_k INT DEFAULT 30 AFTER memory_search_query;

-- +goose Down
ALTER TABLE user_prompts DROP COLUMN memory_search_top_k;
