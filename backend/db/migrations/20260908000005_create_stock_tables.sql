-- +goose Up

CREATE TABLE IF NOT EXISTS `stock_info` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(10) NOT NULL COMMENT '股票代码',
  `name` varchar(50) NOT NULL COMMENT '股票名称',
  `market` varchar(10) NOT NULL DEFAULT 'SZ' COMMENT '市场 SH/SZ/BJ',
  `industry` varchar(50) DEFAULT NULL COMMENT '所属行业',
  `area` varchar(50) DEFAULT NULL COMMENT '所属地域',
  `list_date` date DEFAULT NULL COMMENT '上市日期',
  `is_st` tinyint(1) DEFAULT 0 COMMENT '是否ST',
  `is_active` tinyint(1) DEFAULT 1 COMMENT '是否活跃',
  `total_share` decimal(20,2) DEFAULT NULL COMMENT '总股本(万股)',
  `float_share` decimal(20,2) DEFAULT NULL COMMENT '流通股本(万股)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code` (`code`),
  KEY `idx_industry` (`industry`),
  KEY `idx_market` (`market`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='股票基础信息';

CREATE TABLE IF NOT EXISTS `stock_daily` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(10) NOT NULL,
  `trade_date` date NOT NULL COMMENT '交易日期',
  `open` decimal(12,4) DEFAULT NULL,
  `high` decimal(12,4) DEFAULT NULL,
  `low` decimal(12,4) DEFAULT NULL,
  `close` decimal(12,4) DEFAULT NULL,
  `volume` decimal(20,2) DEFAULT NULL COMMENT '成交量(手)',
  `amount` decimal(20,2) DEFAULT NULL COMMENT '成交额(万元)',
  `turnover_rate` decimal(8,4) DEFAULT NULL COMMENT '换手率(%)',
  `change_pct` decimal(8,4) DEFAULT NULL COMMENT '涨跌幅(%)',
  `amplitude` decimal(8,4) DEFAULT NULL COMMENT '振幅(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code_date` (`code`, `trade_date`),
  KEY `idx_trade_date` (`trade_date`),
  KEY `idx_code_date_desc` (`code`, `trade_date` DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='股票日行情';

CREATE TABLE IF NOT EXISTS `stock_finance` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(10) NOT NULL,
  `report_date` date NOT NULL COMMENT '报告期',
  `report_type` varchar(20) DEFAULT NULL COMMENT '一季报/中报/三季报/年报',
  `pe_ttm` decimal(12,4) DEFAULT NULL COMMENT 'PE(TTM)',
  `pb` decimal(12,4) DEFAULT NULL COMMENT 'PB',
  `ps_ttm` decimal(12,4) DEFAULT NULL COMMENT 'PS(TTM)',
  `roe` decimal(8,4) DEFAULT NULL COMMENT 'ROE(%)',
  `roa` decimal(8,4) DEFAULT NULL COMMENT 'ROA(%)',
  `gross_margin` decimal(8,4) DEFAULT NULL COMMENT '毛利率(%)',
  `net_margin` decimal(8,4) DEFAULT NULL COMMENT '净利率(%)',
  `revenue` decimal(20,2) DEFAULT NULL COMMENT '营业收入(万元)',
  `revenue_yoy` decimal(8,4) DEFAULT NULL COMMENT '营收同比增长(%)',
  `net_profit` decimal(20,2) DEFAULT NULL COMMENT '净利润(万元)',
  `net_profit_yoy` decimal(8,4) DEFAULT NULL COMMENT '净利润同比增长(%)',
  `debt_ratio` decimal(8,4) DEFAULT NULL COMMENT '资产负债率(%)',
  `current_ratio` decimal(8,4) DEFAULT NULL COMMENT '流动比率',
  `eps` decimal(8,4) DEFAULT NULL COMMENT '每股收益',
  `bps` decimal(8,4) DEFAULT NULL COMMENT '每股净资产',
  `ocf_per_share` decimal(8,4) DEFAULT NULL COMMENT '每股经营现金流',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_code_report` (`code`, `report_date`),
  KEY `idx_report_date` (`report_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='股票财务数据';

CREATE TABLE IF NOT EXISTS `stock_concept` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `code` varchar(10) NOT NULL,
  `concept_name` varchar(50) NOT NULL COMMENT '概念名称',
  `concept_code` varchar(20) DEFAULT NULL COMMENT '概念代码',
  `concept_type` varchar(20) NOT NULL DEFAULT 'industry' COMMENT 'industry/concept/area',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_code` (`code`),
  KEY `idx_concept_name` (`concept_name`),
  UNIQUE KEY `uk_code_concept` (`code`, `concept_name`, `concept_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='股票概念板块关联';

CREATE TABLE IF NOT EXISTS `stock_filter_condition` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `name` varchar(100) NOT NULL COMMENT '筛选条件名称',
  `description` text DEFAULT NULL COMMENT '描述',
  `conditions` json NOT NULL COMMENT '筛选条件JSON',
  `result_count` int DEFAULT 0 COMMENT '上次筛选结果数',
  `is_pinned` tinyint(1) DEFAULT 0 COMMENT '是否置顶',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  KEY `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户自定义筛选条件';

CREATE TABLE IF NOT EXISTS `stock_watchlist` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `user_id` bigint unsigned NOT NULL,
  `code` varchar(10) NOT NULL,
  `name` varchar(50) DEFAULT NULL,
  `group_name` varchar(50) DEFAULT '默认' COMMENT '分组名称',
  `note` varchar(200) DEFAULT NULL COMMENT '备注',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_code_group` (`user_id`, `code`, `group_name`),
  KEY `idx_user_id` (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='用户自选股';

INSERT IGNORE INTO `permissions` (`code`, `name`, `group_name`) VALUES
('stock:menu:view', '查看股票模块', '股票筛选'),
('stock:screen:view', '使用筛选功能', '股票筛选'),
('stock:screen:save', '保存筛选条件', '股票筛选'),
('stock:watchlist:view', '查看自选股', '股票筛选'),
('stock:watchlist:edit', '管理自选股', '股票筛选'),
('stock:sync:execute', '数据同步', '股票筛选');

INSERT IGNORE INTO `role_permissions` (`role_code`, `permission_code`)
SELECT 'R_ADMIN', `code` FROM `permissions` WHERE `code` LIKE 'stock:%';

-- +goose Down

DROP TABLE IF EXISTS `stock_watchlist`;
DROP TABLE IF EXISTS `stock_filter_condition`;
DROP TABLE IF EXISTS `stock_concept`;
DROP TABLE IF EXISTS `stock_finance`;
DROP TABLE IF EXISTS `stock_daily`;
DROP TABLE IF EXISTS `stock_info`;

DELETE FROM `role_permissions` WHERE `permission_code` LIKE 'stock:%';
DELETE FROM `permissions` WHERE `code` LIKE 'stock:%';
