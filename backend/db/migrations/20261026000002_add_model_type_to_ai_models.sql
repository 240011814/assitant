-- +goose Up

-- +goose StatementBegin
-- AI 模型能力类型: chat / tts / asr / embedding
ALTER TABLE ai_models
    ADD COLUMN model_type VARCHAR(20) NOT NULL DEFAULT 'chat' COMMENT '模型类型: chat/tts/asr/embedding';
-- +goose StatementEnd

-- +goose StatementBegin
-- 历史数据按常见模型命名归类, 避免语音模型被当成对话模型测试
UPDATE ai_models SET model_type = 'tts' WHERE LOWER(model_code) LIKE '%tts%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE ai_models SET model_type = 'asr' WHERE LOWER(model_code) LIKE '%asr%';
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE ai_models SET model_type = 'embedding' WHERE LOWER(model_code) LIKE '%embedding%' OR LOWER(model_code) LIKE '%embed-%';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_ai_model_provider_type ON ai_models(provider_id, model_type);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP INDEX idx_ai_model_provider_type ON ai_models;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE ai_models DROP COLUMN model_type;
-- +goose StatementEnd
