-- +goose Up

-- 财务同步空报告期区段缓存 (stock_sync_service.SyncFinanceData)
-- 记录各来源曾扫到空的连续报告期区间 [empty_bottom, empty_top] (季度末日期, 双边界内含),
-- 空区间(上市前/未公布)的季度不再每轮重扫, 避免每日调用配额花在重扫上;
-- "连续8个空报告期=自然尽头"断点会把 empty_bottom 预设延伸到 2007Q1, 其下方视为永远为空
CREATE TABLE IF NOT EXISTS `stock_finance_scan` (
  `code` varchar(10) NOT NULL COMMENT '股票代码(sz.000001, 与数据表同口径)',
  `ep` char(1) NOT NULL COMMENT '来源标记: P盈利G成长O营运C现金流B偿债',
  `empty_top` date NOT NULL COMMENT '空报告期区间最浅报告期(季度末)',
  `empty_bottom` date NOT NULL COMMENT '空报告期区间最深报告期(季度末); 自然尽头断点处预设延伸到2007Q1',
  `updated_at` datetime DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP COMMENT '写回时间',
  PRIMARY KEY (`code`, `ep`, `empty_top`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='股票财务空报告期区段缓存(减少每轮重扫占用的调用配额)';

-- +goose Down

DROP TABLE IF EXISTS `stock_finance_scan`;
