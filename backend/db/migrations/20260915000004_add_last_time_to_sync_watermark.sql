-- +goose Up

-- sync_watermark 增加时刻级水位 (last_date 只有天粒度, MySQL->ClickHouse 复制按 updated_at 时刻增量)
-- +goose StatementBegin
ALTER TABLE `sync_watermark`
  ADD COLUMN `last_time` datetime DEFAULT NULL COMMENT '已完整同步到的时刻(ClickHouse增量同步水位用)' AFTER `last_date`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `sync_watermark` DROP COLUMN `last_time`;
-- +goose StatementEnd