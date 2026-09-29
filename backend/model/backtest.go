package model

// BacktestRequest 策略回测请求
// 条件字段限定 ClickHouse stock_daily 内可得: price/changePct/turnoverRate/amount/peTtm/pb
type BacktestRequest struct {
	Conditions []FilterCondition `json:"conditions"`
	StartYear  int               `json:"startYear"` // 默认 当前年-5
	EndYear    int               `json:"endYear"`   // 默认 当前年
	HoldDays   int               `json:"holdDays"`  // 持有交易日数, 默认 20
	MaxStocks  int               `json:"maxStocks"` // 每期最多持仓数 (按代码排序取前 N 等权), 默认 30
}
