-- +goose Up
-- SQL in section 'Up' is executed when this migration is applied

-- training_histories 表：优化 Dashboard 趋势查询和收藏筛选
CREATE INDEX idx_training_user_created ON training_histories(user_id, created_at);
CREATE INDEX idx_training_user_favorite ON training_histories(user_id, is_favorite);

-- vocabulary 表：优化掌握状态筛选和列表排序
CREATE INDEX idx_vocab_user_mastered ON vocabulary(user_id, is_mastered);
CREATE INDEX idx_vocab_user_created ON vocabulary(user_id, created_at);

-- notes 表：优化列表排序
CREATE INDEX idx_notes_user_created ON notes(user_id, created_at);

-- custom_trainings 表：优化列表排序和收藏筛选
CREATE INDEX idx_custom_training_user_created ON custom_trainings(user_id, created_at);
CREATE INDEX idx_custom_training_user_favorite ON custom_trainings(user_id, is_favorite);

-- ai_providers 表：优化启用状态筛选
CREATE INDEX idx_ai_provider_active ON ai_providers(is_active);

-- ai_models 表：优化默认模型查询
CREATE INDEX idx_ai_model_provider_default ON ai_models(provider_id, is_default);

-- +goose Down
-- SQL in section 'Down' is executed when this migration is rolled back

DROP INDEX idx_training_user_created ON training_histories;
DROP INDEX idx_training_user_favorite ON training_histories;
DROP INDEX idx_vocab_user_mastered ON vocabulary;
DROP INDEX idx_vocab_user_created ON vocabulary;
DROP INDEX idx_notes_user_created ON notes;
DROP INDEX idx_custom_training_user_created ON custom_trainings;
DROP INDEX idx_custom_training_user_favorite ON custom_trainings;
DROP INDEX idx_ai_provider_active ON ai_providers;
DROP INDEX idx_ai_model_provider_default ON ai_models;
