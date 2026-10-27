-- +goose Up

-- CH 增量同步需要 updated_at 作为水位 (GORM 写入路径不变: 插入由 DB 默认值填充, 覆盖更新由 ON UPDATE 维护)
-- +goose StatementBegin
ALTER TABLE `stock_daily`
  ADD COLUMN `updated_at` datetime NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '最后变更时间(ClickHouse增量同步水位用)' AFTER `created_at`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_daily` DROP COLUMN `updated_at`;
-- +goose StatementEnd