-- +goose Up

-- +goose StatementBegin
-- 每股最新日K快照 (Screen 大查询专用: 用主键 JOIN 替代每行 MAX(trade_date) 相关子查询)
-- 维护方: 日K按日全量与逐股补拉写入时 upsert (仅当新交易日 >= 已存交易日才覆盖, 允许乱序回补历史)
CREATE TABLE IF NOT EXISTS `stock_daily_latest` (
  `code` varchar(16) NOT NULL COMMENT '完整证券代码',
  `trade_date` datetime NOT NULL COMMENT '最新日K交易日',
  `close` decimal(12,4) DEFAULT NULL,
  `change_pct` decimal(12,4) DEFAULT NULL,
  `turnover_rate` decimal(12,4) DEFAULT NULL,
  `amount` decimal(24,4) DEFAULT NULL,
  `pe_ttm` decimal(12,4) DEFAULT NULL,
  `pb_mrq` decimal(12,4) DEFAULT NULL,
  PRIMARY KEY (`code`),
  KEY `idx_sdl_trade_date` (`trade_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='每股最新日K快照(筛选页专用, 由同步链路维护)';
-- +goose StatementEnd

-- +goose StatementBegin
-- 存量回填: 每只证券 frequency=daily 的最新一行 (一次性大查询, 大表需低峰执行)
INSERT INTO `stock_daily_latest` (`code`, `trade_date`, `close`, `change_pct`, `turnover_rate`, `amount`, `pe_ttm`, `pb_mrq`)
SELECT sd.code, sd.trade_date, sd.close, sd.change_pct, sd.turnover_rate, sd.amount, sd.pe_ttm, sd.pb_mrq
FROM stock_daily sd
JOIN (
    SELECT code, MAX(trade_date) AS md
    FROM stock_daily
    WHERE frequency = 'daily'
    GROUP BY code
) t ON t.code = sd.code AND t.md = sd.trade_date
WHERE sd.frequency = 'daily';
-- +goose StatementEnd

-- +goose Down

-- +goose StatementBegin
DROP TABLE IF EXISTS `stock_daily_latest`;
-- +goose StatementEnd
