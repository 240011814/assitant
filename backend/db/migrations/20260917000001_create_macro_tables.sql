-- +goose Up

CREATE TABLE IF NOT EXISTS `macro_reserve_ratio` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `pub_date` date NOT NULL COMMENT '公告日期',
  `effective_date` date NOT NULL COMMENT '生效日期',
  `big_institutions_ratio_pre` decimal(8,4) DEFAULT NULL COMMENT '大型金融机构调整前准备金率(%)',
  `big_institutions_ratio_after` decimal(8,4) DEFAULT NULL COMMENT '大型金融机构调整后准备金率(%)',
  `medium_institutions_ratio_pre` decimal(8,4) DEFAULT NULL COMMENT '中小金融机构调整前准备金率(%)',
  `medium_institutions_ratio_after` decimal(8,4) DEFAULT NULL COMMENT '中小金融机构调整后准备金率(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_rr_pub_eff` (`pub_date`, `effective_date`),
  KEY `idx_effective_date` (`effective_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='存款准备金率';

CREATE TABLE IF NOT EXISTS `macro_money_supply_month` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `stat_year` int NOT NULL COMMENT '统计年度',
  `stat_month` int NOT NULL COMMENT '统计月份',
  `m0_month` decimal(20,2) DEFAULT NULL COMMENT 'M0期末余额(亿元)',
  `m0_yoy` decimal(12,4) DEFAULT NULL COMMENT 'M0同比增长(%)',
  `m0_chain_relative` decimal(12,4) DEFAULT NULL COMMENT 'M0环比增长(%)',
  `m1_month` decimal(20,2) DEFAULT NULL COMMENT 'M1期末余额(亿元)',
  `m1_yoy` decimal(12,4) DEFAULT NULL COMMENT 'M1同比增长(%)',
  `m1_chain_relative` decimal(12,4) DEFAULT NULL COMMENT 'M1环比增长(%)',
  `m2_month` decimal(20,2) DEFAULT NULL COMMENT 'M2期末余额(亿元)',
  `m2_yoy` decimal(12,4) DEFAULT NULL COMMENT 'M2同比增长(%)',
  `m2_chain_relative` decimal(12,4) DEFAULT NULL COMMENT 'M2环比增长(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_msm_year_month` (`stat_year`, `stat_month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='货币供应量月度数据';

CREATE TABLE IF NOT EXISTS `macro_money_supply_year` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `stat_year` int NOT NULL COMMENT '统计年度',
  `m0_year` decimal(20,2) DEFAULT NULL COMMENT 'M0年末余额(亿元)',
  `m0_year_yoy` decimal(12,4) DEFAULT NULL COMMENT 'M0同比增长(%)',
  `m1_year` decimal(20,2) DEFAULT NULL COMMENT 'M1年末余额(亿元)',
  `m1_year_yoy` decimal(12,4) DEFAULT NULL COMMENT 'M1同比增长(%)',
  `m2_year` decimal(20,2) DEFAULT NULL COMMENT 'M2年末余额(亿元)',
  `m2_year_yoy` decimal(12,4) DEFAULT NULL COMMENT 'M2同比增长(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_msy_year` (`stat_year`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='货币供应量年度数据(年底余额)';

CREATE TABLE IF NOT EXISTS `macro_deposit_rate` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `pub_date` date NOT NULL COMMENT '公告日期',
  `demand` decimal(8,4) DEFAULT NULL COMMENT '活期存款利率(%)',
  `fixed_3m` decimal(8,4) DEFAULT NULL COMMENT '整存整取3个月(%)',
  `fixed_6m` decimal(8,4) DEFAULT NULL COMMENT '整存整取6个月(%)',
  `fixed_1y` decimal(8,4) DEFAULT NULL COMMENT '整存整取1年(%)',
  `fixed_2y` decimal(8,4) DEFAULT NULL COMMENT '整存整取2年(%)',
  `fixed_3y` decimal(8,4) DEFAULT NULL COMMENT '整存整取3年(%)',
  `fixed_5y` decimal(8,4) DEFAULT NULL COMMENT '整存整取5年(%)',
  `installment_1y` decimal(8,4) DEFAULT NULL COMMENT '零存整取1年(%)',
  `installment_3y` decimal(8,4) DEFAULT NULL COMMENT '零存整取3年(%)',
  `installment_5y` decimal(8,4) DEFAULT NULL COMMENT '零存整取5年(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_dr_pub` (`pub_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='存款利率';

CREATE TABLE IF NOT EXISTS `macro_loan_rate` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `pub_date` date NOT NULL COMMENT '公告日期',
  `loan_6m` decimal(8,4) DEFAULT NULL COMMENT '贷款利率6个月(%)',
  `loan_6m_1y` decimal(8,4) DEFAULT NULL COMMENT '贷款利率6个月至1年(%)',
  `loan_1y_3y` decimal(8,4) DEFAULT NULL COMMENT '贷款利率1至3年(%)',
  `loan_3y_5y` decimal(8,4) DEFAULT NULL COMMENT '贷款利率3至5年(%)',
  `loan_above_5y` decimal(8,4) DEFAULT NULL COMMENT '贷款利率5年以上(%)',
  `mortgage_below_5y` decimal(8,4) DEFAULT NULL COMMENT '公积金贷款利率5年以下(%)',
  `mortgage_above_5y` decimal(8,4) DEFAULT NULL COMMENT '公积金贷款利率5年以上(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_lr_pub` (`pub_date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='贷款利率';

INSERT IGNORE INTO `permissions` (`code`, `name`, `group_name`) VALUES
('stock:macro:view', '查看宏观经济数据', '股票筛选');

INSERT IGNORE INTO `role_permissions` (`role_code`, `permission_code`)
SELECT 'R_ADMIN', `code` FROM `permissions` WHERE `code` = 'stock:macro:view';

-- +goose Down

DROP TABLE IF EXISTS `macro_loan_rate`;
DROP TABLE IF EXISTS `macro_deposit_rate`;
DROP TABLE IF EXISTS `macro_money_supply_year`;
DROP TABLE IF EXISTS `macro_money_supply_month`;
DROP TABLE IF EXISTS `macro_reserve_ratio`;

DELETE FROM `role_permissions` WHERE `permission_code` = 'stock:macro:view';
DELETE FROM `permissions` WHERE `code` = 'stock:macro:view';