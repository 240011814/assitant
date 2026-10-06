package service

import (
	"fmt"
	"strings"

	"backend/model"
)

// snapshotCols 快照表列 (与 model.StockDaily 对应字段的映射)
var snapshotCols = []string{"trade_date", "close", "change_pct", "turnover_rate", "amount", "pe_ttm", "pb_mrq"}

// refreshLatestSnapshot 把新写入的日K行 upsert 到"每股最新日K快照表"(stock_daily_latest),
// 供筛选页用主键 JOIN 替代每股 MAX(trade_date) 相关子查询。
// 仅当新行的交易日 >= 已存交易日才覆盖 (回补历史/乱序写入不会把快照倒退); 周/月/小时K行跳过。
// 用 VALUES() 写法, 兼容 MySQL 5.7 与 8.x。
func refreshLatestSnapshot(rows []model.StockDaily) error {
	daily := make([]model.StockDaily, 0, len(rows))
	for i := range rows {
		if strings.EqualFold(rows[i].Frequency, "daily") && rows[i].Code != "" {
			daily = append(daily, rows[i])
		}
	}
	if len(daily) == 0 {
		return nil
	}

	var sb strings.Builder
	args := make([]interface{}, 0, len(daily)*8)
	sb.WriteString("INSERT INTO `stock_daily_latest` (`code`, `trade_date`, `close`, `change_pct`, `turnover_rate`, `amount`, `pe_ttm`, `pb_mrq`) VALUES ")
	for i := range daily {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString("(?,?,?,?,?,?,?,?)")
		args = append(args,
			daily[i].Code, daily[i].TradeDate, daily[i].Close, daily[i].ChangePct,
			daily[i].TurnoverRate, daily[i].Amount, daily[i].PeTtm, daily[i].PbMrq)
	}

	sb.WriteString(" ON DUPLICATE KEY UPDATE ")
	for i, col := range snapshotCols {
		if i > 0 {
			sb.WriteString(",")
		}
		// 新交易日 >= 已存交易日才采用新值, 否则保持快照不倒退
		fmt.Fprintf(&sb, "`%s`=IF(VALUES(`trade_date`) >= `stock_daily_latest`.`trade_date`, VALUES(`%s`), `stock_daily_latest`.`%s`)", col, col, col)
	}
	return DB.Exec(sb.String(), args...).Error
}
