-- +goose Up

-- +goose StatementBegin
ALTER TABLE `stock_daily`
  ADD COLUMN `preclose` decimal(12,4) DEFAULT NULL COMMENT '前收盘价' AFTER `close`,
  ADD COLUMN `trade_status` tinyint DEFAULT NULL COMMENT '交易状态 1正常交易 0停牌' AFTER `volume`,
  ADD COLUMN `pe_ttm` decimal(14,6) DEFAULT NULL COMMENT '滚动市盈率' AFTER `change_pct`,
  ADD COLUMN `pb_mrq` decimal(14,6) DEFAULT NULL COMMENT '市净率' AFTER `pe_ttm`,
  ADD COLUMN `ps_ttm` decimal(14,6) DEFAULT NULL COMMENT '滚动市销率' AFTER `pb_mrq`,
  ADD COLUMN `pcf_ncf_ttm` decimal(14,6) DEFAULT NULL COMMENT '滚动市现率' AFTER `ps_ttm`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_daily`
  DROP COLUMN `pcf_ncf_ttm`,
  DROP COLUMN `ps_ttm`,
  DROP COLUMN `pb_mrq`,
  DROP COLUMN `pe_ttm`,
  DROP COLUMN `trade_status`,
  DROP COLUMN `preclose`;
-- +goose StatementEnd
