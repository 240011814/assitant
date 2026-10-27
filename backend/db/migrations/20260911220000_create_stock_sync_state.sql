-- +goose Up

CREATE TABLE IF NOT EXISTS `stock_sync_state` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(10) NOT NULL COMMENT '股票代码',
  `kline_daily_to` date DEFAULT NULL COMMENT '日K已同步到的最新交易日',
  `kline_weekly_to` date DEFAULT NULL COMMENT '周K已同步到的最新交易日',
  `kline_monthly_to` date DEFAULT NULL COMMENT '月K已同步到的最新交易日',
  `kline_status` varchar(20) NOT NULL DEFAULT 'pending' COMMENT 'K线同步状态 pending/ok/failed',
  `kline_error` varchar(255) DEFAULT NULL COMMENT 'K线同步失败原因',
  `kline_synced_at` datetime DEFAULT NULL COMMENT 'K线最后同步时间',
  `finance_to` date DEFAULT NULL COMMENT '财务已同步到的最新报告期',
  `finance_status` varchar(20) NOT NULL DEFAULT 'pending' COMMENT '财务同步状态 pending/ok/failed/skipped',
  `finance_error` varchar(255) DEFAULT NULL COMMENT '财务同步失败原因',
  `finance_synced_at` datetime DEFAULT NULL COMMENT '财务最后同步时间',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='股票数据同步状态(数据表是真相, 此表为断点与观测)';

-- 存量数据回填: 从数据表派生各股票的同步水位, 已有数据标记为 ok
INSERT INTO `stock_sync_state` (`code`, `kline_daily_to`, `kline_weekly_to`, `kline_monthly_to`, `kline_status`, `kline_synced_at`, `finance_to`, `finance_status`, `finance_synced_at`)
SELECT si.`code`,
  k.`daily_to`, k.`weekly_to`, k.`monthly_to`,
  CASE WHEN k.`daily_to` IS NOT NULL OR k.`weekly_to` IS NOT NULL OR k.`monthly_to` IS NOT NULL THEN 'ok' ELSE 'pending' END,
  CASE WHEN k.`daily_to` IS NOT NULL THEN NOW() ELSE NULL END,
  f.`finance_to`,
  CASE WHEN f.`finance_to` IS NOT NULL THEN 'ok' ELSE 'pending' END,
  CASE WHEN f.`finance_to` IS NOT NULL THEN NOW() ELSE NULL END
FROM `stock_info` si
LEFT JOIN (
  SELECT `code`,
    MAX(CASE WHEN `frequency` = 'daily' THEN `trade_date` END) AS `daily_to`,
    MAX(CASE WHEN `frequency` = 'weekly' THEN `trade_date` END) AS `weekly_to`,
    MAX(CASE WHEN `frequency` = 'monthly' THEN `trade_date` END) AS `monthly_to`
  FROM `stock_daily` GROUP BY `code`
) k ON k.`code` = si.`code`
LEFT JOIN (
  SELECT `code`, MAX(`report_date`) AS `finance_to`
  FROM `stock_finance` GROUP BY `code`
) f ON f.`code` = si.`code`;

-- +goose Down

DROP TABLE IF EXISTS `stock_sync_state`;
