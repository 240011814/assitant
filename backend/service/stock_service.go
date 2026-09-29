package service

import (
	"backend/model"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

type StockService struct{}

func NewStockService() *StockService {
	return &StockService{}
}

// Screen 股票筛选核心逻辑
func (s *StockService) Screen(req model.StockScreenRequest) (*model.StockScreenResponse, error) {
	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 || req.PageSize > 100 {
		req.PageSize = 20
	}

	// 构建基础查询：关联 stock_info + 最新行情 + 最新财务
	query := DB.Table("stock_info AS si").
		Select(`si.code, si.name, si.market, si.type, si.industry,
			sd.close AS price, sd.change_pct AS change_pct, sd.turnover_rate, sd.amount,
			CASE WHEN si.total_share > 0 THEN ROUND(sd.close * si.total_share / 10000, 2)
			     WHEN si.total_market_cap > 0 THEN ROUND(si.total_market_cap / 100000000, 2)
			     ELSE NULL END AS market_cap,
			CASE WHEN si.float_share > 0 THEN ROUND(sd.close * si.float_share / 10000, 2)
			     WHEN si.float_market_cap > 0 THEN ROUND(si.float_market_cap / 100000000, 2)
			     ELSE NULL END AS float_market_cap,
			CASE WHEN sf.eps != 0 THEN ROUND(sd.close / sf.eps, 2) ELSE NULL END AS pe_ttm,
			sd.pb_mrq AS pb,
			sf.roe, sf.revenue_yoy, sf.net_profit_yoy,
			sf.gross_margin, sf.net_margin, sf.debt_ratio, sf.current_ratio, sf.quick_ratio`).
		Joins(`LEFT JOIN stock_daily AS sd ON sd.code = si.code AND sd.frequency = 'daily' AND sd.trade_date = (
			SELECT MAX(trade_date) FROM stock_daily WHERE code = si.code AND frequency = 'daily'
		)`).
		Joins(`LEFT JOIN stock_finance AS sf ON sf.code = si.code AND sf.report_date = (
			SELECT MAX(report_date) FROM stock_finance WHERE code = si.code
		)`)

	// 基础过滤: 仅活跃证券(排除退市股); 证券类型不传=全部(股票+指数)
	query = query.Where("si.is_active = ?", true)
	if len(req.SecurityTypes) > 0 {
		query = query.Where("si.type IN ?", req.SecurityTypes)
	}

	// 名称/代码搜索
	if req.Keyword != "" {
		query = query.Where("(si.code LIKE ? OR si.name LIKE ?)", "%"+req.Keyword+"%", "%"+req.Keyword+"%")
	}

	// 排除ST
	if req.ExcludeST {
		query = query.Where("si.is_st = ?", false)
	}

	// 行业筛选
	if len(req.Industries) > 0 {
		query = query.Where("si.industry IN ?", req.Industries)
	}

	// 市场筛选
	if len(req.Markets) > 0 {
		query = query.Where("si.market IN ?", req.Markets)
	}

	// 概念板块筛选
	if len(req.ConceptNames) > 0 {
		query = query.Where("si.code IN (SELECT code FROM stock_concept WHERE concept_name IN ?)", req.ConceptNames)
	}

	// 应用筛选条件
	for _, cond := range req.Conditions {
		query = s.applyCondition(query, cond)
	}

	// 统计总数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}

	// 排序
	allowedSorts := map[string]bool{
		"code": true, "name": true, "price": true, "change_pct": true,
		"turnover_rate": true, "amount": true, "market_cap": true,
		"pe_ttm": true, "pb": true, "roe": true, "revenue_yoy": true, "net_profit_yoy": true,
	}
	if req.SortBy != "" && allowedSorts[req.SortBy] {
		order := "ASC"
		if strings.EqualFold(req.SortOrder, "desc") {
			order = "DESC"
		}
		query = query.Order(fmt.Sprintf("%s %s", req.SortBy, order))
	} else {
		query = query.Order("si.code ASC")
	}

	// 分页
	offset := (req.Page - 1) * req.PageSize
	var results []struct {
		Code           string   `gorm:"column:code"`
		Name           string   `gorm:"column:name"`
		Market         string   `gorm:"column:market"`
		Type           int      `gorm:"column:type"`
		Industry       string   `gorm:"column:industry"`
		Price          *float64 `gorm:"column:price"`
		ChangePct      *float64 `gorm:"column:change_pct"`
		TurnoverRate   *float64 `gorm:"column:turnover_rate"`
		Amount         *float64 `gorm:"column:amount"`
		MarketCap      *float64 `gorm:"column:market_cap"`
		FloatMarketCap *float64 `gorm:"column:float_market_cap"`
		PeTtm          *float64 `gorm:"column:pe_ttm"`
		Pb             *float64 `gorm:"column:pb"`
		Roe            *float64 `gorm:"column:roe"`
		RevenueYoy     *float64 `gorm:"column:revenue_yoy"`
		NetProfitYoy   *float64 `gorm:"column:net_profit_yoy"`
		GrossMargin    *float64 `gorm:"column:gross_margin"`
		NetMargin      *float64 `gorm:"column:net_margin"`
		DebtRatio      *float64 `gorm:"column:debt_ratio"`
		CurrentRatio   *float64 `gorm:"column:current_ratio"`
		QuickRatio     *float64 `gorm:"column:quick_ratio"`
	}

	if err := query.Offset(offset).Limit(req.PageSize).Find(&results).Error; err != nil {
		return nil, err
	}

	// 转换结果 + 补充技术指标和概念
	list := make([]model.StockScreenResult, 0, len(results))
	codes := make([]string, 0, len(results))
	for _, r := range results {
		codes = append(codes, r.Code)
		list = append(list, model.StockScreenResult{
			Code:           r.Code,
			Name:           r.Name,
			Market:         r.Market,
			Type:           r.Type,
			Industry:       r.Industry,
			Price:          r.Price,
			ChangePct:      r.ChangePct,
			TurnoverRate:   r.TurnoverRate,
			Amount:         r.Amount,
			MarketCap:      r.MarketCap,
			FloatMarketCap: r.FloatMarketCap,
			PeTtm:          r.PeTtm,
			Pb:             r.Pb,
			Roe:            r.Roe,
			RevenueYoy:     r.RevenueYoy,
			NetProfitYoy:   r.NetProfitYoy,
			GrossMargin:    r.GrossMargin,
			NetMargin:      r.NetMargin,
			DebtRatio:      r.DebtRatio,
			CurrentRatio:   r.CurrentRatio,
			QuickRatio:     r.QuickRatio,
		})
	}

	// 批量补充技术指标和概念
	if len(codes) > 0 {
		senrichTechIndicators(list, codes)
		senrichConcepts(list, codes)
	}

	totalPages := int(total) / req.PageSize
	if int(total)%req.PageSize > 0 {
		totalPages++
	}

	return &model.StockScreenResponse{
		List:       list,
		Total:      total,
		Page:       req.Page,
		PageSize:   req.PageSize,
		TotalPages: totalPages,
	}, nil
}

// applyCondition 应用单个筛选条件
func (s *StockService) applyCondition(query *gorm.DB, cond model.FilterCondition) *gorm.DB {
	// 注意: market_cap/pe_ttm/pb 是 SELECT 里的计算别名, MySQL 不允许 WHERE 引用 SELECT 别名,
	// 因此这里必须内联等价原始表达式 (与 SELECT 口径一致), 否则直接 SQL 1054 报错或过滤恒不生效
	fieldMap := map[string]string{
		"peTtm":        "CASE WHEN sf.eps != 0 THEN ROUND(sd.close / sf.eps, 2) ELSE NULL END",
		"pb":           "sd.pb_mrq",
		"roe":          "sf.roe",
		"revenueYoy":   "sf.revenue_yoy",
		"netProfitYoy": "sf.net_profit_yoy",
		"price":        "sd.close",
		"changePct":    "sd.change_pct",
		"turnoverRate": "sd.turnover_rate",
		"amount":       "sd.amount",
		"marketCap": `CASE WHEN si.total_share > 0 THEN ROUND(sd.close * si.total_share / 10000, 2)
			WHEN si.total_market_cap > 0 THEN ROUND(si.total_market_cap / 100000000, 2)
			ELSE NULL END`,
		"floatMarketCap": `CASE WHEN si.float_share > 0 THEN ROUND(sd.close * si.float_share / 10000, 2)
			WHEN si.float_market_cap > 0 THEN ROUND(si.float_market_cap / 100000000, 2)
			ELSE NULL END`,
	}

	dbField, ok := fieldMap[cond.Field]
	if !ok {
		return query
	}

	switch cond.Operator {
	case "gt":
		if v, ok := cond.Value.(float64); ok {
			query = query.Where(fmt.Sprintf("%s > ?", dbField), v)
		}
	case "lt":
		if v, ok := cond.Value.(float64); ok {
			query = query.Where(fmt.Sprintf("%s < ?", dbField), v)
		}
	case "gte":
		if v, ok := cond.Value.(float64); ok {
			query = query.Where(fmt.Sprintf("%s >= ?", dbField), v)
		}
	case "lte":
		if v, ok := cond.Value.(float64); ok {
			query = query.Where(fmt.Sprintf("%s <= ?", dbField), v)
		}
	case "between":
		if arr, ok := cond.Value.([]interface{}); ok && len(arr) == 2 {
			minVal, _ := arr[0].(float64)
			maxVal, _ := arr[1].(float64)
			query = query.Where(fmt.Sprintf("%s >= ? AND %s <= ?", dbField, dbField), minVal, maxVal)
		}
	case "eq":
		if v, ok := cond.Value.(float64); ok {
			query = query.Where(fmt.Sprintf("%s = ?", dbField), v)
		}
	case "in":
		if arr, ok := cond.Value.([]interface{}); ok {
			query = query.Where(fmt.Sprintf("%s IN ?", dbField), arr)
		}
	}

	return query
}

// senrichTechIndicators 批量补充技术指标
// 每只股票取最近 60 根日K (UNION ALL 一次往返, 兼容 MySQL 5.7/8.0), 在应用侧计算 MA 与 RSI;
// 旧实现的四个 MA 子查询完全相同 (都取最新一根收盘价), RSI 从未赋值, 属于假数据
func senrichTechIndicators(list []model.StockScreenResult, codes []string) {
	if len(codes) == 0 {
		return
	}

	const barsNeeded = 60 // MA60 需要 60 根, RSI24 需要 25 根

	var sb strings.Builder
	args := make([]interface{}, 0, len(codes)*2)
	for i, code := range codes {
		if i > 0 {
			sb.WriteString(" UNION ALL ")
		}
		sb.WriteString("(SELECT code, trade_date, close FROM stock_daily WHERE code = ? AND frequency = 'daily' ORDER BY trade_date DESC LIMIT ?)")
		args = append(args, code, barsNeeded)
	}

	var rows []struct {
		Code      string    `gorm:"column:code"`
		TradeDate time.Time `gorm:"column:trade_date"`
		Close     *float64  `gorm:"column:close"`
	}
	if err := DB.Raw(sb.String(), args...).Find(&rows).Error; err != nil {
		return
	}

	// 按代码分组并按日期倒序 (最新在前), 不依赖 SQL 返回顺序
	type bar struct {
		date  time.Time
		close float64
	}
	closesByCode := make(map[string][]bar, len(codes))
	for _, r := range rows {
		if r.Close == nil {
			continue
		}
		closesByCode[r.Code] = append(closesByCode[r.Code], bar{date: r.TradeDate, close: *r.Close})
	}
	for code := range closesByCode {
		bars := closesByCode[code]
		sort.Slice(bars, func(i, j int) bool { return bars[i].date.After(bars[j].date) })
		closesByCode[code] = bars
	}

	for i := range list {
		bars := closesByCode[list[i].Code]
		if len(bars) == 0 {
			continue
		}
		closes := make([]float64, len(bars))
		for j, b := range bars {
			closes[j] = b.close
		}
		list[i].Ma5 = smaOf(closes, 5)
		list[i].Ma10 = smaOf(closes, 10)
		list[i].Ma20 = smaOf(closes, 20)
		list[i].Ma60 = smaOf(closes, 60)
		list[i].Rsi6 = rsiOf(closes, 6)
		list[i].Rsi12 = rsiOf(closes, 12)
		list[i].Rsi24 = rsiOf(closes, 24)
	}
}

// smaOf 计算简单移动平均 (closes 为最新在前), 样本不足时返回 nil
func smaOf(closes []float64, n int) *float64 {
	if len(closes) < n {
		return nil
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += closes[i]
	}
	v := round2(sum / float64(n))
	return &v
}

// rsiOf 计算简单 RSI (100 * 平均涨幅 / (平均涨幅 + 平均跌幅)), 样本不足时返回 nil
func rsiOf(closes []float64, n int) *float64 {
	if len(closes) < n+1 {
		return nil
	}
	gain, loss := 0.0, 0.0
	for i := 0; i < n; i++ {
		diff := closes[i] - closes[i+1]
		if diff > 0 {
			gain += diff
		} else {
			loss -= diff
		}
	}
	if gain+loss == 0 {
		return nil
	}
	v := round2(100 * gain / (gain + loss))
	return &v
}

// enrichConcepts 批量补充概念板块
func senrichConcepts(list []model.StockScreenResult, codes []string) {
	if len(codes) == 0 {
		return
	}

	type conceptResult struct {
		Code        string `gorm:"column:code"`
		ConceptName string `gorm:"column:concept_name"`
	}

	var conceptResults []conceptResult
	DB.Raw(`
		SELECT code, concept_name
		FROM stock_concept
		WHERE code IN ?
	`, codes).Find(&conceptResults)

	conceptMap := make(map[string][]string)
	for _, c := range conceptResults {
		conceptMap[c.Code] = append(conceptMap[c.Code], c.ConceptName)
	}

	for i := range list {
		list[i].Concepts = conceptMap[list[i].Code]
	}
}

// GetDetail 获取个股详情
func (s *StockService) GetDetail(code string) (*model.StockScreenResult, error) {
	var result struct {
		Code           string   `gorm:"column:code"`
		Name           string   `gorm:"column:name"`
		Market         string   `gorm:"column:market"`
		Type           int      `gorm:"column:type"`
		Industry       string   `gorm:"column:industry"`
		Price          *float64 `gorm:"column:price"`
		ChangePct      *float64 `gorm:"column:change_pct"`
		TurnoverRate   *float64 `gorm:"column:turnover_rate"`
		Amount         *float64 `gorm:"column:amount"`
		MarketCap      *float64 `gorm:"column:market_cap"`
		FloatMarketCap *float64 `gorm:"column:float_market_cap"`
		PeTtm          *float64 `gorm:"column:pe_ttm"`
		Pb             *float64 `gorm:"column:pb"`
		Roe            *float64 `gorm:"column:roe"`
		RevenueYoy     *float64 `gorm:"column:revenue_yoy"`
		NetProfitYoy   *float64 `gorm:"column:net_profit_yoy"`
		GrossMargin    *float64 `gorm:"column:gross_margin"`
		NetMargin      *float64 `gorm:"column:net_margin"`
		DebtRatio      *float64 `gorm:"column:debt_ratio"`
		CurrentRatio   *float64 `gorm:"column:current_ratio"`
		QuickRatio     *float64 `gorm:"column:quick_ratio"`
		CashRatio      *float64 `gorm:"column:cash_ratio"`
		NrTurnRatio    *float64 `gorm:"column:nr_turn_ratio"`
		InvTurnRatio   *float64 `gorm:"column:inv_turn_ratio"`
		YoyEquity      *float64 `gorm:"column:yoy_equity"`
		YoyAsset       *float64 `gorm:"column:yoy_asset"`
		CfoToOr        *float64 `gorm:"column:cfo_to_or"`
	}

	err := DB.Table("stock_info AS si").
		Select(`si.code, si.name, si.market, si.type, si.industry,
			sd.close AS price, sd.change_pct, sd.turnover_rate, sd.amount,
			CASE WHEN si.total_share > 0 THEN ROUND(sd.close * si.total_share / 10000, 2)
			     WHEN si.total_market_cap > 0 THEN ROUND(si.total_market_cap / 100000000, 2)
			     ELSE NULL END AS market_cap,
			CASE WHEN si.float_share > 0 THEN ROUND(sd.close * si.float_share / 10000, 2)
			     WHEN si.float_market_cap > 0 THEN ROUND(si.float_market_cap / 100000000, 2)
			     ELSE NULL END AS float_market_cap,
			CASE WHEN sf.eps != 0 THEN ROUND(sd.close / sf.eps, 2) ELSE NULL END AS pe_ttm,
			sd.pb_mrq AS pb,
			sf.roe, sf.revenue_yoy, sf.net_profit_yoy,
			sf.gross_margin, sf.net_margin, sf.debt_ratio, sf.current_ratio, sf.quick_ratio,
			sf.cash_ratio, sf.nr_turn_ratio, sf.inv_turn_ratio, sf.yoy_equity, sf.yoy_asset, sf.cfo_to_or`).
		Joins(`LEFT JOIN stock_daily AS sd ON sd.code = si.code AND sd.frequency = 'daily' AND sd.trade_date = (
			SELECT MAX(trade_date) FROM stock_daily WHERE code = si.code AND frequency = 'daily'
		)`).
		Joins(`LEFT JOIN stock_finance AS sf ON sf.code = si.code AND sf.report_date = (
			SELECT MAX(report_date) FROM stock_finance WHERE code = si.code
		)`).
		Where("si.code = ?", code).
		First(&result).Error

	if err != nil {
		return nil, errors.New("股票不存在")
	}

	detail := &model.StockScreenResult{
		Code:           result.Code,
		Name:           result.Name,
		Market:         result.Market,
		Type:           result.Type,
		Industry:       result.Industry,
		Price:          result.Price,
		ChangePct:      result.ChangePct,
		TurnoverRate:   result.TurnoverRate,
		Amount:         result.Amount,
		MarketCap:      result.MarketCap,
		FloatMarketCap: result.FloatMarketCap,
		PeTtm:          result.PeTtm,
		Pb:             result.Pb,
		Roe:            result.Roe,
		RevenueYoy:     result.RevenueYoy,
		NetProfitYoy:   result.NetProfitYoy,
		GrossMargin:    result.GrossMargin,
		NetMargin:      result.NetMargin,
		DebtRatio:      result.DebtRatio,
		CurrentRatio:   result.CurrentRatio,
		QuickRatio:     result.QuickRatio,
		CashRatio:      result.CashRatio,
		NrTurnRatio:    result.NrTurnRatio,
		InvTurnRatio:   result.InvTurnRatio,
		YoyEquity:      result.YoyEquity,
YoyAsset:      result.YoyAsset,
		CfoToOr:        result.CfoToOr,
	}

	// 补充概念
	var concepts []string
	DB.Raw("SELECT concept_name FROM stock_concept WHERE code = ?", code).Scan(&concepts)
	detail.Concepts = concepts

	return detail, nil
}

// GetSyncState 获取个股同步状态 (从未同步时返回 nil, 由前端显示未同步)
func (s *StockService) GetSyncState(code string) (*model.StockSyncState, error) {
	var st model.StockSyncState
	if err := DB.Where("code = ?", code).First(&st).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &st, nil
}

// GetFinanceHistory 获取历史财务数据 (limit<=0 返回全部报告期)
func (s *StockService) GetFinanceHistory(code string, limit int) ([]model.StockFinance, error) {
	if limit < 0 || limit > 200 {
		limit = 200
	}

	var finances []model.StockFinance
	query := DB.Where("code = ?", code).
		Order("report_date DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	err := query.Find(&finances).Error
	if err != nil {
		return nil, err
	}

	return finances, nil
}

// GetKline 获取K线数据 (period: daily/weekly/monthly/60, 60=小时线)
func (s *StockService) GetKline(code string, period string, count int) ([]map[string]interface{}, error) {
	frequency := strings.ToLower(period)
	switch frequency {
	case "weekly", "monthly":
	case "60", "hourly", "hour":
		frequency = "60"
	default:
		frequency = "daily"
	}

	// 小时线粒度更细, 允许取更多根
	maxCount := 500
	if frequency == "60" {
		maxCount = 2000
	}
	if count <= 0 || count > maxCount {
		count = 120
	}

	var dailies []model.StockDaily
	err := DB.Where("code = ? AND frequency = ?", code, frequency).
		Order("trade_date DESC").
		Limit(count).
		Find(&dailies).Error
	if err != nil {
		return nil, err
	}

	// 小时线带时分, 日/周/月线只到日
	dateLayout := "2006-01-02"
	if frequency == "60" {
		dateLayout = "2006-01-02 15:04"
	}

	// 计算MA
	result := make([]map[string]interface{}, 0, len(dailies))
	for i := len(dailies) - 1; i >= 0; i-- {
		d := dailies[i]
		item := map[string]interface{}{
			"date":         d.TradeDate.Format(dateLayout),
			"open":         d.Open,
			"high":         d.High,
			"low":          d.Low,
			"close":        d.Close,
			"preclose":     d.Preclose,
			"volume":       d.Volume,
			"amount":       d.Amount,
			"turnoverRate": d.TurnoverRate,
			"tradeStatus":  d.TradeStatus,
			"changePct":    d.ChangePct,
			"peTtm":        d.PeTtm,
			"pbMrq":        d.PbMrq,
			"psTtm":        d.PsTtm,
			"pcfNcfTtm":    d.PcfNcfTtm,
		}

		// 计算MA5/10/20/60
		if i+4 < len(dailies) {
			sum := 0.0
			for j := i; j <= i+4; j++ {
				if dailies[j].Close != nil {
					sum += *dailies[j].Close
				}
			}
			ma5 := sum / 5
			item["ma5"] = math.Round(ma5*100) / 100
		}
		if i+9 < len(dailies) {
			sum := 0.0
			for j := i; j <= i+9; j++ {
				if dailies[j].Close != nil {
					sum += *dailies[j].Close
				}
			}
			ma10 := sum / 10
			item["ma10"] = math.Round(ma10*100) / 100
		}
		if i+19 < len(dailies) {
			sum := 0.0
			for j := i; j <= i+19; j++ {
				if dailies[j].Close != nil {
					sum += *dailies[j].Close
				}
			}
			ma20 := sum / 20
			item["ma20"] = math.Round(ma20*100) / 100
		}

		result = append(result, item)
	}

	return result, nil
}

// GetIndustries 获取行业列表
func (s *StockService) GetIndustries() ([]string, error) {
	var industries []string
	err := DB.Model(&model.StockInfo{}).
		Distinct("industry").
		Where("industry IS NOT NULL AND industry != ''").
		Order("industry ASC").
		Pluck("industry", &industries).Error
	return industries, err
}

// GetConcepts 获取概念列表
func (s *StockService) GetConcepts() ([]map[string]string, error) {
	var concepts []struct {
		ConceptName string `gorm:"column:concept_name"`
		ConceptCode string `gorm:"column:concept_code"`
	}
	err := DB.Model(&model.StockConcept{}).
		Select("DISTINCT concept_name, concept_code").
		Where("concept_type = 'concept'").
		Order("concept_name ASC").
		Find(&concepts).Error
	if err != nil {
		return nil, err
	}

	result := make([]map[string]string, 0, len(concepts))
	for _, c := range concepts {
		result = append(result, map[string]string{
			"name": c.ConceptName,
			"code": c.ConceptCode,
		})
	}
	return result, nil
}

// SaveFilterCondition 保存筛选条件
func (s *StockService) SaveFilterCondition(userID uint, req model.StockFilterConditionRequest) error {
	condition := model.StockFilterCondition{
		UserID:      userID,
		Name:        req.Name,
		Description: req.Description,
		Conditions:  req.Conditions,
	}
	return DB.Create(&condition).Error
}

// ListFilterConditions 获取用户保存的筛选条件
func (s *StockService) ListFilterConditions(userID uint) ([]model.StockFilterCondition, error) {
	var conditions []model.StockFilterCondition
	err := DB.Where("user_id = ?", userID).
		Order("is_pinned DESC, updated_at DESC").
		Find(&conditions).Error
	return conditions, err
}

// DeleteFilterCondition 删除筛选条件
func (s *StockService) DeleteFilterCondition(userID uint, id uint) error {
	result := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.StockFilterCondition{})
	if result.RowsAffected == 0 {
		return errors.New("筛选条件不存在")
	}
	return result.Error
}

// AddWatchlist 添加自选股
func (s *StockService) AddWatchlist(userID uint, req model.StockWatchlistRequest) error {
	groupName := req.GroupName
	if groupName == "" {
		groupName = "默认"
	}

	// 查询股票名称
	var name string
	DB.Model(&model.StockInfo{}).Where("code = ?", req.Code).Pluck("name", &name)

	watchlist := model.StockWatchlist{
		UserID:    userID,
		Code:      req.Code,
		Name:      name,
		GroupName: groupName,
		Note:      req.Note,
	}
	return DB.Create(&watchlist).Error
}

// ListWatchlist 获取自选股列表(联表补齐行情/财务, 字段与筛选页对齐)
func (s *StockService) ListWatchlist(userID uint) ([]model.StockWatchlistItem, error) {
	var list []model.StockWatchlistItem
	err := DB.Table("stock_watchlist AS w").
		Select(`w.id, w.code, w.group_name,
			COALESCE(si.name, '') AS name, COALESCE(si.market, '') AS market,
			COALESCE(si.type, 0) AS type, COALESCE(si.industry, '') AS industry,
			COALESCE(si.is_st, 0) AS is_st,
			sd.close AS price, sd.change_pct, sd.turnover_rate, sd.amount,
			CASE WHEN si.total_share > 0 THEN ROUND(sd.close * si.total_share / 10000, 2)
			     WHEN si.total_market_cap > 0 THEN ROUND(si.total_market_cap / 100000000, 2)
			     ELSE NULL END AS market_cap,
			CASE WHEN si.float_share > 0 THEN ROUND(sd.close * si.float_share / 10000, 2)
			     WHEN si.float_market_cap > 0 THEN ROUND(si.float_market_cap / 100000000, 2)
			     ELSE NULL END AS float_market_cap,
			CASE WHEN sf.eps != 0 THEN ROUND(sd.close / sf.eps, 2) ELSE NULL END AS pe_ttm,
			sd.pb_mrq AS pb,
			sf.roe, sf.revenue_yoy, sf.net_profit_yoy, sf.gross_margin, sf.net_margin,
			sf.debt_ratio, sf.current_ratio, sf.quick_ratio`).
		Joins(`LEFT JOIN stock_info AS si ON si.code = w.code`).
		Joins(`LEFT JOIN stock_daily AS sd ON sd.code = w.code AND sd.frequency = 'daily' AND sd.trade_date = (
			SELECT MAX(trade_date) FROM stock_daily WHERE code = w.code AND frequency = 'daily'
		)`).
		Joins(`LEFT JOIN stock_finance AS sf ON sf.code = w.code AND sf.report_date = (
			SELECT MAX(report_date) FROM stock_finance WHERE code = w.code
		)`).
		Where("w.user_id = ?", userID).
		Order("w.created_at DESC").
		Scan(&list).Error
	if err != nil {
		return nil, err
	}

	codes := make([]string, 0, len(list))
	for i := range list {
		codes = append(codes, list[i].Code)
	}
	enrichWatchlistConcepts(list, codes)

	return list, nil
}

// enrichWatchlistConcepts 补充自选股概念板块
func enrichWatchlistConcepts(list []model.StockWatchlistItem, codes []string) {
	if len(codes) == 0 {
		return
	}

	type conceptResult struct {
		Code        string `gorm:"column:code"`
		ConceptName string `gorm:"column:concept_name"`
	}

	var conceptResults []conceptResult
	DB.Raw(`
		SELECT code, concept_name
		FROM stock_concept
		WHERE code IN ?
	`, codes).Find(&conceptResults)

	conceptMap := make(map[string][]string)
	for _, c := range conceptResults {
		conceptMap[c.Code] = append(conceptMap[c.Code], c.ConceptName)
	}

	for i := range list {
		list[i].Concepts = conceptMap[list[i].Code]
	}
}

// ListWatchlistGroups 获取用户的自选股分组
func (s *StockService) ListWatchlistGroups(userID uint) ([]string, error) {
	groups := make([]string, 0)
	err := DB.Model(&model.StockWatchlist{}).
		Where("user_id = ?", userID).
		Distinct().
		Order("group_name").
		Pluck("group_name", &groups).Error
	if err != nil {
		return nil, err
	}
	return groups, nil
}

// ListWatchlistCodes 获取用户已自选的股票代码
func (s *StockService) ListWatchlistCodes(userID uint) ([]string, error) {
	codes := make([]string, 0)
	err := DB.Model(&model.StockWatchlist{}).
		Where("user_id = ?", userID).
		Distinct().
		Pluck("code", &codes).Error
	if err != nil {
		return nil, err
	}
	return codes, nil
}

// DeleteWatchlist 删除自选股
func (s *StockService) DeleteWatchlist(userID uint, id uint) error {
	result := DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.StockWatchlist{})
	if result.RowsAffected == 0 {
		return errors.New("自选股不存在")
	}
	return result.Error
}
