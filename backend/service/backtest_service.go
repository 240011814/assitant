package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"backend/model"
)

// BacktestService 基于 ClickHouse 只读副本的策略回测:
// 每月第一个交易日按筛选条件等权买入, 持有 N 个交易日后卖出, 输出各期收益与汇总统计
// 条件字段限定 CH stock_daily 内可得: price/changePct/turnoverRate/amount/peTtm/pb (市值依赖 stock_info, 暂不支持)
type BacktestService struct{}

func NewBacktestService() *BacktestService {
	return &BacktestService{}
}

// BacktestPeriod 单期回测结果
type BacktestPeriod struct {
	RebalanceDate string   `json:"rebalanceDate"`
	SellDate      string   `json:"sellDate"`
	StockCount    int      `json:"stockCount"`
	Return        *float64 `json:"return"` // 等权平均收益 (小数, 0.05 = 5%)
}

// BacktestSummary 汇总统计
type BacktestSummary struct {
	PeriodCount   int      `json:"periodCount"`
	MeanReturn    *float64 `json:"meanReturn"`   // 平均每期收益 (小数)
	MedianReturn  *float64 `json:"medianReturn"` // 中位数
	WinRate       *float64 `json:"winRate"`      // 正收益期占比 (小数)
	BestReturn    *float64 `json:"bestReturn"`
	WorstReturn   *float64 `json:"worstReturn"`
	CumulativeRet *float64 `json:"cumulativeRet"` // 逐期复利累计
}

// BacktestResponse 回测响应
type BacktestResponse struct {
	Params  map[string]interface{} `json:"params"`
	Periods []BacktestPeriod       `json:"periods"`
	Summary BacktestSummary        `json:"summary"`
	Skipped int                    `json:"skipped"` // 样本不足被跳过的期数
}

// chBacktestField 回测条件的字段白名单 (CH stock_daily 列)
var chBacktestField = map[string]string{
	"price":        "close",
	"changePct":    "change_pct",
	"turnoverRate": "turnover_rate",
	"amount":       "amount",
	"peTtm":        "pe_ttm",
	"pb":           "pb_mrq",
}

// Run 执行回测
func (s *BacktestService) Run(req model.BacktestRequest) (*BacktestResponse, error) {
	if CH == nil {
		return nil, errors.New("ClickHouse 未启用, 回测功能不可用")
	}
	holdDays := req.HoldDays
	if holdDays <= 0 {
		holdDays = 20
	}
	if holdDays > 250 {
		holdDays = 250
	}
	maxStocks := req.MaxStocks
	if maxStocks <= 0 {
		maxStocks = 30
	}
	now := time.Now()
	startYear := req.StartYear
	if startYear <= 0 {
		startYear = now.Year() - 5
	}
	endYear := req.EndYear
	if endYear <= 0 {
		endYear = now.Year()
	}
	startDate := fmt.Sprintf("%d-01-01 00:00:00", startYear)
	endDate := fmt.Sprintf("%d-12-31 23:59:59", endYear)
	if startYear > endYear {
		return nil, errors.New("起始年份不能晚于结束年份")
	}

	conds, args, err := s.buildConds(req.Conditions)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// 1. 每月第一个交易日 (再平衡日)
	rebalanceDays, err := s.queryRebalanceDays(ctx, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("查询再平衡日失败: %w", err)
	}
	if len(rebalanceDays) == 0 {
		return nil, errors.New("回测区间内无行情数据, 请确认 ClickHouse 已同步")
	}

	// 2. 全期交易日日历 (整体日历, 用于定位持有 N 个交易日后的卖出日)
	tradeDays, err := s.queryTradeDayList(ctx, rebalanceDays[0], rebalanceDays[len(rebalanceDays)-1])
	if err != nil {
		return nil, fmt.Errorf("查询交易日历失败: %w", err)
	}
	dayIndex := make(map[string]int, len(tradeDays))
	for i, d := range tradeDays {
		dayIndex[d] = i
	}

	resp := &BacktestResponse{
		Params: map[string]interface{}{
			"startYear":      startYear,
			"endYear":        endYear,
			"holdDays":       holdDays,
			"maxStocks":      maxStocks,
			"conditionCount": len(req.Conditions),
		},
		Periods: []BacktestPeriod{},
	}

	var rets []float64
	skipped := 0
	for _, day := range rebalanceDays {
		idx, ok := dayIndex[day]
		if !ok || idx+holdDays >= len(tradeDays) {
			skipped++ // 卖出日超出日历, 最后一期(或数期)持有不足
			continue
		}
		sellDay := tradeDays[idx+holdDays]

		// 3. 买入日符合条件的股票 (等权, 最多 maxStocks 只)
		buys, err := s.queryStocksAt(ctx, day, conds, args, maxStocks)
		if err != nil {
			return nil, fmt.Errorf("查询买入日数据失败 (%s): %w", day, err)
		}
		if len(buys) == 0 {
			skipped++
			continue
		}
		codes := make([]string, 0, len(buys))
		for code := range buys {
			codes = append(codes, code)
		}

		// 4. 卖出日收盘价
		codeArgs := make([]interface{}, len(codes))
		for i, c := range codes {
			codeArgs[i] = c
		}
		sells, err := s.queryStocksAt(ctx, sellDay, "", codeArgs, 0)
		if err != nil {
			return nil, fmt.Errorf("查询卖出日数据失败 (%s): %w", sellDay, err)
		}

		// 5. 等权收益 (买卖日都有价格才计入)
		sum, count := 0.0, 0
		for code, buyPrice := range buys {
			sellPrice, ok := sells[code]
			if !ok || buyPrice <= 0 || sellPrice <= 0 {
				continue
			}
			sum += (sellPrice - buyPrice) / buyPrice
			count++
		}
		if count < 3 { // 样本过少的期次不可信
			skipped++
			continue
		}
		ret := sum / float64(count)
		rets = append(rets, ret)
		resp.Periods = append(resp.Periods, BacktestPeriod{
			RebalanceDate: day,
			SellDate:      sellDay,
			StockCount:    count,
			Return:        &ret,
		})
	}
	resp.Skipped = skipped
	resp.Summary = summarize(rets)
	return resp, nil
}

func summarize(rets []float64) BacktestSummary {
	summary := BacktestSummary{PeriodCount: len(rets)}
	if len(rets) == 0 {
		return summary
	}
	sorted := append([]float64(nil), rets...)
	sort.Float64s(sorted)

	sum := 0.0
	cum := 1.0
	wins := 0
	for _, r := range rets {
		sum += r
		cum *= 1 + r
		if r > 0 {
			wins++
		}
	}
	mean := sum / float64(len(rets))
	median := sorted[len(sorted)/2]
	if len(sorted)%2 == 0 {
		median = (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
	}
	winRate := float64(wins) / float64(len(rets))
	round := func(v float64) *float64 { f := math.Round(v*10000) / 10000; return &f }
	summary.MeanReturn = round(mean)
	summary.MedianReturn = round(median)
	summary.WinRate = round(winRate)
	summary.BestReturn = round(sorted[len(sorted)-1])
	summary.WorstReturn = round(sorted[0])
	summary.CumulativeRet = round(cum - 1)
	return summary
}

// buildConds 构造 CH WHERE 片段 (字段白名单 + 参数化占位)
func (s *BacktestService) buildConds(conditions []model.FilterCondition) (string, []interface{}, error) {
	var sb strings.Builder
	var args []interface{}
	for _, cond := range conditions {
		col, ok := chBacktestField[cond.Field]
		if !ok {
			return "", nil, fmt.Errorf("回测暂不支持字段: %s (仅支持 price/changePct/turnoverRate/amount/peTtm/pb)", cond.Field)
		}
		switch cond.Operator {
		case "gt":
			sb.WriteString(fmt.Sprintf(" AND %s > ?", col))
			args = append(args, cond.Value)
		case "gte":
			sb.WriteString(fmt.Sprintf(" AND %s >= ?", col))
			args = append(args, cond.Value)
		case "lt":
			sb.WriteString(fmt.Sprintf(" AND %s < ?", col))
			args = append(args, cond.Value)
		case "lte":
			sb.WriteString(fmt.Sprintf(" AND %s <= ?", col))
			args = append(args, cond.Value)
		case "between":
			arr, ok := cond.Value.([]interface{})
			if !ok || len(arr) != 2 {
				return "", nil, errors.New("between 条件需要 [min, max]")
			}
			sb.WriteString(fmt.Sprintf(" AND %s >= ? AND %s <= ?", col, col))
			args = append(args, arr[0], arr[1])
		default:
			return "", nil, fmt.Errorf("回测暂不支持操作符: %s", cond.Operator)
		}
	}
	return sb.String(), args, nil
}

// queryRebalanceDays 每月第一个交易日 (CH, date 字符串 YYYY-MM-DD)
func (s *BacktestService) queryRebalanceDays(ctx context.Context, start, end string) ([]string, error) {
	rows, err := CH.Query(ctx,
		"SELECT toString(toDate(min(trade_date))) FROM stock_daily WHERE frequency = 'daily' AND trade_date >= ? AND trade_date <= ? GROUP BY toYYYYMM(trade_date) ORDER BY 1",
		start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var days []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

// queryTradeDayList 区间内全部交易日 (升序, 去重)
func (s *BacktestService) queryTradeDayList(ctx context.Context, start, end string) ([]string, error) {
	rows, err := CH.Query(ctx,
		"SELECT DISTINCT toString(toDate(trade_date)) FROM stock_daily WHERE frequency = 'daily' AND trade_date >= ? AND trade_date <= ? ORDER BY 1",
		start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var days []string
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		days = append(days, d)
	}
	return days, rows.Err()
}

// queryStocksAt 某交易日满足条件的股票; conds 为空且 limit=0 时按 code 列表取收盘价
func (s *BacktestService) queryStocksAt(ctx context.Context, day, conds string, args []interface{}, limit int) (map[string]float64, error) {
	sql := "SELECT code, close FROM stock_daily WHERE frequency = 'daily' AND trade_date = ?"
	queryArgs := []interface{}{day + " 00:00:00"}
	if conds != "" {
		sql += conds + " AND close > 0"
		queryArgs = append(queryArgs, args...)
	} else {
		sql += " AND code IN ?"
		queryArgs = append(queryArgs, args)
	}
	if limit > 0 {
		sql += fmt.Sprintf(" ORDER BY code LIMIT %d", limit)
	}
	rows, err := CH.Query(ctx, sql, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]float64)
	for rows.Next() {
		var code string
		var close float64
		if err := rows.Scan(&code, &close); err != nil {
			return nil, err
		}
		result[code] = close
	}
	return result, rows.Err()
}
