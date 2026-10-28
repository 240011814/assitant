-- +goose Up

-- 前端接口请求超时后台配置: 默认值 + 按路由覆盖项 (JSON, 只存非默认项)。
-- 覆盖项种子 = 迁移自前端原有硬编码 (cut 90s / skill 发现 600s / 文档上传 120s),
-- 并给划词翻译配 30s (LLM 同步调用可能超过包默认的 10s)。
-- 全量路由清单不落库, 由后端 gin 路由枚举实时生成 (GET /api/admin/system-config/api-timeouts)。

-- +goose StatementBegin
INSERT INTO `system_config` (`key`, `value`, `remark`) VALUES
    ('api_timeout_default', '10000', '前端接口默认请求超时 (毫秒)'),
    ('api_timeout_overrides', '[{"method":"POST","path":"/api/cut/plane","timeoutMs":90000},{"method":"POST","path":"/api/skills/discover/github","timeoutMs":600000},{"method":"POST","path":"/api/documents/upload","timeoutMs":120000},{"method":"POST","path":"/api/reading/translate","timeoutMs":30000}]', '前端接口超时覆盖项 (method+path → 毫秒, 只存非默认项)')
ON DUPLICATE KEY UPDATE `key` = `key`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DELETE FROM `system_config` WHERE `key` IN ('api_timeout_default', 'api_timeout_overrides');
-- +goose StatementEnd
