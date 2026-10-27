-- +goose Up
-- +goose StatementBegin
ALTER TABLE `training_histories` CHANGE COLUMN `status` `record_type` VARCHAR(20) NOT NULL DEFAULT 'auto' COMMENT '记录类型: auto-自动, manual-手动';
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE `training_histories` CHANGE COLUMN `record_type` `status` VARCHAR(20) NOT NULL DEFAULT 'completed';
-- +goose StatementEnd
