-- +goose Up
-- +goose StatementBegin
ALTER TABLE stock_info
    ADD COLUMN `type` TINYINT NOT NULL DEFAULT 1 COMMENT '证券类型: 1股票 2指数' AFTER market;
-- +goose StatementEnd

-- +goose StatementBegin
UPDATE stock_info SET `type` = 2 WHERE (market = 'SH' AND code LIKE '000%') OR (market = 'SZ' AND code LIKE '399%');
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
ALTER TABLE stock_info DROP COLUMN `type`;
-- +goose StatementEnd
