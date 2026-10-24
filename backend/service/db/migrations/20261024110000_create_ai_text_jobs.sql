-- +goose Up

-- +goose StatementBegin
-- 慢 LLM 生成任务 (AI 写课文等): Cloudflare 对源站响应有 100s 上限, 此类操作必须
-- 异步执行, 任务行供前端轮询
CREATE TABLE IF NOT EXISTS `ai_text_jobs` (
    `id` INT AUTO_INCREMENT PRIMARY KEY,
    `user_id` INT NOT NULL,
    `kind` VARCHAR(30) NOT NULL COMMENT '任务类型 passage',
    `status` VARCHAR(20) NOT NULL DEFAULT 'running' COMMENT 'running/done/failed',
    `result_id` INT NOT NULL DEFAULT 0 COMMENT '完成后的结果 ID (课程包)',
    `error` TEXT,
    `created_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    `updated_at` TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    INDEX `idx_ai_text_job_user` (`user_id`, `created_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `ai_text_jobs`;
-- +goose StatementEnd
