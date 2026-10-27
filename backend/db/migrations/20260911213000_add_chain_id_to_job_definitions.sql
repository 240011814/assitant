-- +goose Up

-- 链式重复: 每次执行 = 一条定义, 执行完生成下一条; chain_id 分组同一条重复备忘
-- +goose StatementBegin
ALTER TABLE `job_definitions`
  ADD COLUMN `chain_id` bigint unsigned NOT NULL DEFAULT 0 COMMENT '重复链标识(首条定义为自身ID, 0=非链式)' AFTER `id`,
  ADD KEY `idx_chain` (`chain_id`);
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE `job_definitions` SET `chain_id` = `id` WHERE `chain_id` = 0;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `job_definitions`
  DROP KEY `idx_chain`,
  DROP COLUMN `chain_id`;
-- +goose StatementEnd
