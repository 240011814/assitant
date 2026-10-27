-- +goose Up

-- 同比增速/比率类字段在基数为 0 附近时会出现超大值(如净利润同比 10868%),
-- decimal(8,4) 上限 9999.9999 不够用, 统一放宽为 decimal(12,4)
-- +goose StatementBegin
ALTER TABLE `stock_finance`
  MODIFY COLUMN `roe` decimal(12,4) DEFAULT NULL COMMENT 'ROE(%)',
  MODIFY COLUMN `roa` decimal(12,4) DEFAULT NULL COMMENT 'ROA(%)',
  MODIFY COLUMN `gross_margin` decimal(12,4) DEFAULT NULL COMMENT '毛利率(%)',
  MODIFY COLUMN `net_margin` decimal(12,4) DEFAULT NULL COMMENT '净利率(%)',
  MODIFY COLUMN `revenue_yoy` decimal(12,4) DEFAULT NULL COMMENT '营收同比增长(%)',
  MODIFY COLUMN `net_profit_yoy` decimal(12,4) DEFAULT NULL COMMENT '净利润同比增长(%)',
  MODIFY COLUMN `debt_ratio` decimal(12,4) DEFAULT NULL COMMENT '资产负债率(%)',
  MODIFY COLUMN `current_ratio` decimal(12,4) DEFAULT NULL COMMENT '流动比率',
  MODIFY COLUMN `quick_ratio` decimal(12,4) DEFAULT NULL COMMENT '速动比率',
  MODIFY COLUMN `cash_ratio` decimal(12,4) DEFAULT NULL COMMENT '现金比率',
  MODIFY COLUMN `nr_turn_ratio` decimal(12,4) DEFAULT NULL COMMENT '应收账款周转率',
  MODIFY COLUMN `inv_turn_ratio` decimal(12,4) DEFAULT NULL COMMENT '存货周转率',
  MODIFY COLUMN `ca_turn_ratio` decimal(12,4) DEFAULT NULL COMMENT '流动资产周转率',
  MODIFY COLUMN `asset_turn_ratio` decimal(12,4) DEFAULT NULL COMMENT '总资产周转率',
  MODIFY COLUMN `yoy_equity` decimal(12,4) DEFAULT NULL COMMENT '净资产同比增长(%)',
  MODIFY COLUMN `yoy_asset` decimal(12,4) DEFAULT NULL COMMENT '总资产同比增长(%)',
  MODIFY COLUMN `yoy_eps` decimal(12,4) DEFAULT NULL COMMENT '每股收益同比增长(%)',
  MODIFY COLUMN `cfo_to_or` decimal(12,4) DEFAULT NULL COMMENT '经营现金流/营业收入',
  MODIFY COLUMN `cfo_to_np` decimal(12,4) DEFAULT NULL COMMENT '经营现金流/净利润';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
ALTER TABLE `stock_finance`
  MODIFY COLUMN `roe` decimal(8,4) DEFAULT NULL COMMENT 'ROE(%)',
  MODIFY COLUMN `roa` decimal(8,4) DEFAULT NULL COMMENT 'ROA(%)',
  MODIFY COLUMN `gross_margin` decimal(8,4) DEFAULT NULL COMMENT '毛利率(%)',
  MODIFY COLUMN `net_margin` decimal(8,4) DEFAULT NULL COMMENT '净利率(%)',
  MODIFY COLUMN `revenue_yoy` decimal(8,4) DEFAULT NULL COMMENT '营收同比增长(%)',
  MODIFY COLUMN `net_profit_yoy` decimal(8,4) DEFAULT NULL COMMENT '净利润同比增长(%)',
  MODIFY COLUMN `debt_ratio` decimal(8,4) DEFAULT NULL COMMENT '资产负债率(%)',
  MODIFY COLUMN `current_ratio` decimal(8,4) DEFAULT NULL COMMENT '流动比率',
  MODIFY COLUMN `quick_ratio` decimal(8,4) DEFAULT NULL COMMENT '速动比率',
  MODIFY COLUMN `cash_ratio` decimal(8,4) DEFAULT NULL COMMENT '现金比率',
  MODIFY COLUMN `nr_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '应收账款周转率',
  MODIFY COLUMN `inv_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '存货周转率',
  MODIFY COLUMN `ca_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '流动资产周转率',
  MODIFY COLUMN `asset_turn_ratio` decimal(8,4) DEFAULT NULL COMMENT '总资产周转率',
  MODIFY COLUMN `yoy_equity` decimal(8,4) DEFAULT NULL COMMENT '净资产同比增长(%)',
  MODIFY COLUMN `yoy_asset` decimal(8,4) DEFAULT NULL COMMENT '总资产同比增长(%)',
  MODIFY COLUMN `yoy_eps` decimal(8,4) DEFAULT NULL COMMENT '每股收益同比增长(%)',
  MODIFY COLUMN `cfo_to_or` decimal(8,4) DEFAULT NULL COMMENT '经营现金流/营业收入',
  MODIFY COLUMN `cfo_to_np` decimal(8,4) DEFAULT NULL COMMENT '经营现金流/净利润';
-- +goose StatementEnd
