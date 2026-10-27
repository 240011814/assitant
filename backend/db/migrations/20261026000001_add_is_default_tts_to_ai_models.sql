-- +goose Up

-- +goose StatementBegin
-- 默认 TTS 模型: 模型管理里标记, 供 OpenAI 兼容朗读引擎选择语音合成模型
ALTER TABLE ai_models
    ADD COLUMN is_default_tts TINYINT(1) NOT NULL DEFAULT 0 COMMENT '是否为默认 TTS 模型';
-- +goose StatementEnd

-- +goose StatementBegin
CREATE INDEX idx_ai_model_provider_default_tts ON ai_models(provider_id, is_default_tts);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP INDEX idx_ai_model_provider_default_tts ON ai_models;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE ai_models DROP COLUMN is_default_tts;
-- +goose StatementEnd
