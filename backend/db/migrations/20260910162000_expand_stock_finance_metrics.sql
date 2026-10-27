-- +goose Up

-- +goose StatementBegin
ALTER TABLE `stock_finance`
  ADD COLUMN `cash_ratio` decimal(8,4) DEFAULT NULL COMMENT '现金比率' AFTER `quick_ratio`,
  ADD COLUMN `nr_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '应收账款周转率' AFTER `cash_ratio`,
  ADD COLUMN `inv_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '存货周转率' AFTER `nr_turn_ratio`,
  ADD COLUMN `ca_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '流动资产周转率' AFTER `inv_turn_ratio`,
  ADD COLUMN `asset_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '总资产周转率' AFTER `ca_turn_ratio`,
  ADD COLUMN `yoy_equity` decimal(8,4) DEFAULT NULL COMMENT '净资产同比增长(%)' AFTER `asset_turn_ratio`,
  ADD COLUMN `yoy_asset` decimal(8,4) DEFAULT NULL COMMENT '总资产同比增长(%)' AFTER `yoy_equity`,
  ADD COLUMN `yoy_eps` decimal(8,4) DEFAULT NULL COMMENT '每股收益同比增长(%)' AFTER `yoy_asset`,
  ADD COLUMN `cfo_to_or` decimal(8,4) DEFAULT NULL COMMENT '经营现金流/营业收入' AFTER `yoy_eps`,
  ADD COLUMN `cfo_to_np` decimal(8,4) DEFAULT NULL COMMENT '经营现金流/净利润' AFTER `cfo_to_or`;
-- +goose StatementEnd

-- +goose StatementBegin
ALTER TABLE `stock_finance`
  ADD COLUMN `finance_sources` varchar(8) NOT NULL DEFAULT '' COMMENT '已同步数据来源 P盈利G成长O营运C现金流B偿债' AFTER `cfo_to_np`;
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_finance`
  DROP COLUMN `finance_sources`,
  DROP COLUMN `cfo_to_np`,
  DROP COLUMN `cfo_to_or`,
  DROP COLUMN `yoy_eps`,
  DROP COLUMN `yoy_asset`,
  DROP COLUMN `yoy_equity`,
  DROP COLUMN `asset_turn_ratio`,
  DROP COLUMN `ca_turn_ratio`,
  DROP COLUMN `inv_turn_ratio`,
  DROP COLUMN `nr_turn_ratio`,
  DROP COLUMN `cash_ratio`;
-- +goose StatementEnd
