-- +goose Up

-- ai_text_jobs 升级为通用异步任务表: 请求触发的慢 LLM 任务统一走这里,
-- 前端轮询端点同步迁移到 /api/async-jobs/:id (与 jobs 包的时间驱动调度体系互补, 不合并)

-- +goose StatementBegin
RENAME TABLE `ai_text_jobs` TO `async_jobs`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `async_jobs`
    ADD COLUMN `params_json` LONGTEXT NULL COMMENT '创建参数快照 JSON (排查/重跑依据)' AFTER `kind`,
    ADD COLUMN `progress` VARCHAR(100) NULL COMMENT '进度文本 (轮询可见, 如 12/30 段)' AFTER `result_json`,
    ADD COLUMN `started_at` DATETIME NULL AFTER `progress`,
    ADD COLUMN `finished_at` DATETIME NULL AFTER `started_at`,
    ADD INDEX `idx_async_job_kind` (`kind`, `status`);
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `async_jobs`
    DROP INDEX `idx_async_job_kind`,
    DROP COLUMN `finished_at`,
    DROP COLUMN `started_at`,
    DROP COLUMN `progress`,
    DROP COLUMN `params_json`;
-- +goose StatementEnd

-- +goose StatementBegin
RENAME TABLE `async_jobs` TO `ai_text_jobs`;
-- +goose StatementEnd
