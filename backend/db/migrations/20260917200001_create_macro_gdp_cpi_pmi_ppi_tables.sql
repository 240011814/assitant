-- +goose Up

CREATE TABLE IF NOT EXISTS `macro_gdp` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `quarter` varchar(20) NOT NULL COMMENT '季度(如2024Q1)',
  `gdp_yoy` decimal(12,4) DEFAULT NULL COMMENT 'GDP同比增长(%)',
  `gdp_cumulative` decimal(20,2) DEFAULT NULL COMMENT 'GDP累计值(亿元)',
  `gdp_cumulative_yoy` decimal(12,4) DEFAULT NULL COMMENT 'GDP累计同比增长(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_gdp_quarter` (`quarter`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='GDP数据';

CREATE TABLE IF NOT EXISTS `macro_cpi` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `month` varchar(20) NOT NULL COMMENT '月份(如2024-01)',
  `cpi_yoy` decimal(12,4) DEFAULT NULL COMMENT 'CPI同比增长(%)',
  `cpi_mom` decimal(12,4) DEFAULT NULL COMMENT 'CPI环比增长(%)',
  `cpi_cumulative_yoy` decimal(12,4) DEFAULT NULL COMMENT 'CPI累计同比增长(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_cpi_month` (`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='CPI数据';

CREATE TABLE IF NOT EXISTS `macro_pmi` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `month` varchar(20) NOT NULL COMMENT '月份(如2024-01)',
  `pmi_manufacturing` decimal(12,4) DEFAULT NULL COMMENT '制造业PMI指数',
  `pmi_manufacturing_yoy` decimal(12,4) DEFAULT NULL COMMENT '制造业PMI同比增长(%)',
  `pmi_non_manufacturing` decimal(12,4) DEFAULT NULL COMMENT '非制造业PMI指数',
  `pmi_non_manufacturing_yoy` decimal(12,4) DEFAULT NULL COMMENT '非制造业PMI同比增长(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_pmi_month` (`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='PMI数据';

CREATE TABLE IF NOT EXISTS `macro_ppi` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `month` varchar(20) NOT NULL COMMENT '月份(如2024-01)',
  `ppi_yoy` decimal(12,4) DEFAULT NULL COMMENT 'PPI同比增长(%)',
  `ppi_mom` decimal(12,4) DEFAULT NULL COMMENT 'PPI环比增长(%)',
  `ppi_cumulative_yoy` decimal(12,4) DEFAULT NULL COMMENT 'PPI累计同比增长(%)',
  `created_at` datetime DEFAULT CURRENT_TIMESTAMP,
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_ppi_month` (`month`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='PPI数据';

-- +goose Down

DROP TABLE IF EXISTS `macro_ppi`;
DROP TABLE IF EXISTS `macro_pmi`;
DROP TABLE IF EXISTS `macro_cpi`;
DROP TABLE IF EXISTS `macro_gdp`;
