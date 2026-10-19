package model

import (
	"time"
)

// StockInfo 股票基础信息
type StockInfo struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Code           string     `gorm:"size:16;not null;uniqueIndex" json:"code"`
	Name           string     `gorm:"size:50;not null" json:"name"`
	Market         string     `gorm:"size:10;default:SZ" json:"market"`
	Type           int        `gorm:"default:1" json:"type"` // 证券类型: 1股票 2指数 5ETF
	Industry       string     `gorm:"size:50" json:"industry"`
	Area           string     `gorm:"size:50" json:"area"`
	ListDate       *time.Time `json:"listDate"`
	IsST           bool       `gorm:"default:false" json:"isSt"`
	IsActive       bool       `json:"isActive"`
	TotalShare     *float64   `json:"totalShare"`
	FloatShare     *float64   `json:"floatShare"`
	TotalMarketCap *float64   `json:"totalMarketCap"`
	FloatMarketCap *float64   `json:"floatMarketCap"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

func (StockInfo) TableName() string {
	return "stock_info"
}

// StockDaily 股票K线 (frequency: daily/weekly/monthly)
type StockDaily struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Code         string    `gorm:"size:16;not null" json:"code"`
	Frequency    string    `gorm:"size:10;not null;default:daily" json:"frequency"`
	TradeDate    time.Time `gorm:"not null" json:"tradeDate"`
	Open         *float64  `json:"open"`
	High         *float64  `json:"high"`
	Low          *float64  `json:"low"`
	Close        *float64  `json:"close"`
	Preclose     *float64  `json:"preclose"`
	Volume       *float64  `json:"volume"`
	TradeStatus  *int8     `json:"tradeStatus"`
	Amount       *float64  `json:"amount"`
	TurnoverRate *float64  `json:"turnoverRate"`
	ChangePct    *float64  `json:"changePct"`
	PeTtm        *float64  `json:"peTtm"`
	PbMrq        *float64  `json:"pbMrq"`
	PsTtm        *float64  `json:"psTtm"`
	PcfNcfTtm    *float64  `json:"pcfNcfTtm"`
	Amplitude    *float64  `json:"amplitude"`
	CreatedAt    time.Time `json:"createdAt"`
}

func (StockDaily) TableName() string {
	return "stock_daily"
}

// StockSyncState 股票数据同步状态 (每只股票一行)
// 数据表(stock_daily/stock_finance)是真相, 此表是同步水位缓存+观测: 用于断点续跑、失败记录与状态展示
type StockSyncState struct {
	ID              uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Code            string     `gorm:"size:16;not null;uniqueIndex" json:"code"`
	KlineDailyTo    *time.Time `json:"klineDailyTo"`
	KlineWeeklyTo   *time.Time `json:"klineWeeklyTo"`
	KlineMonthlyTo  *time.Time `json:"klineMonthlyTo"`
	KlineHourlyTo   *time.Time `json:"klineHourlyTo"`
	KlineStatus     string     `gorm:"size:20;not null;default:pending" json:"klineStatus"`
	KlineError      string     `gorm:"size:255" json:"klineError"`
	KlineSyncedAt   *time.Time `json:"klineSyncedAt"`
	FinanceTo       *time.Time `json:"financeTo"`
	FinanceStatus   string     `gorm:"size:20;not null;default:pending" json:"financeStatus"`
	FinanceError    string     `gorm:"size:255" json:"financeError"`
	FinanceSyncedAt *time.Time `json:"financeSyncedAt"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

func (StockSyncState) TableName() string {
	return "stock_sync_state"
}

// SyncWatermark 批量同步水位断点 (按日全量等批量同步, 只推进到完整成功的一天)
type SyncWatermark struct {
	Name      string     `gorm:"size:50;primaryKey" json:"name"`
	LastDate  *time.Time `json:"lastDate"`
	LastTime  *time.Time `json:"lastTime"`
	Status    string     `gorm:"size:20;not null;default:pending" json:"status"`
	Error     string     `gorm:"size:255" json:"error"`
	SyncedAt  *time.Time `json:"syncedAt"`
	CreatedAt time.Time  `json:"createdAt"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

func (SyncWatermark) TableName() string {
	return "sync_watermark"
}

// StockFinance 股票财务数据
type StockFinance struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Code           string    `gorm:"size:16;not null" json:"code"`
	ReportDate     time.Time `gorm:"not null" json:"reportDate"`
	ReportType     string    `gorm:"size:20" json:"reportType"`
	PeTtm          *float64  `json:"peTtm"`
	Pb             *float64  `json:"pb"`
	PsTtm          *float64  `json:"psTtm"`
	Roe            *float64  `json:"roe"`
	Roa            *float64  `json:"roa"`
	GrossMargin    *float64  `json:"grossMargin"`
	NetMargin      *float64  `json:"netMargin"`
	Revenue        *float64  `json:"revenue"`
	RevenueYoy     *float64  `json:"revenueYoy"`
	NetProfit      *float64  `json:"netProfit"`
	NetProfitYoy   *float64  `json:"netProfitYoy"`
	DebtRatio      *float64  `json:"debtRatio"`
	CurrentRatio   *float64  `json:"currentRatio"`
	QuickRatio     *float64  `json:"quickRatio"`
	CashRatio      *float64  `json:"cashRatio"`
	NrTurnRatio    *float64  `json:"nrTurnRatio"`
	InvTurnRatio   *float64  `json:"invTurnRatio"`
	CaTurnRatio    *float64  `json:"caTurnRatio"`
	AssetTurnRatio *float64  `json:"assetTurnRatio"`
	YoyEquity      *float64  `json:"yoyEquity"`
	YoyAsset       *float64  `json:"yoyAsset"`
	YoyEps         *float64  `json:"yoyEps"`
	CfoToOr        *float64  `json:"cfoToOr"`
	CfoToNp        *float64  `json:"cfoToNp"`
	Eps            *float64  `json:"eps"`
	EpsDeducted    *float64  `json:"epsDeducted"`
	Bps            *float64  `json:"bps"`
	OcfPerShare    *float64  `json:"ocfPerShare"`
	FinanceSources string    `gorm:"size:8;not null;default:''" json:"financeSources"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

func (StockFinance) TableName() string {
	return "stock_finance"
}

// StockFinanceScan 财务同步空报告期区段缓存 (每股票每来源若干行)
// 记录各来源曾扫到空的连续报告期区间 [empty_bottom, empty_top] (季度末日期, 双边界内含);
// 与 stock_finance 数据表解耦, 不产生占位数据行, 历史数据/前端展示均不受影响;
// empty_bottom 在"连续8空=自然尽头"断点处预设延伸到 2007Q1, 其下方视为上市前/无数据永远为空
type StockFinanceScan struct {
	Code        string    `gorm:"size:10;not null;primaryKey" json:"code"`
	Ep          string    `gorm:"size:1;not null;primaryKey" json:"ep"`
	EmptyTop    time.Time `gorm:"not null;primaryKey" json:"emptyTop"`
	EmptyBottom time.Time `gorm:"not null" json:"emptyBottom"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (StockFinanceScan) TableName() string {
	return "stock_finance_scan"
}

// StockConcept 股票概念关联
type StockConcept struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Code        string    `gorm:"size:16;not null" json:"code"`
	ConceptName string    `gorm:"size:50;not null" json:"conceptName"`
	ConceptCode string    `gorm:"size:20" json:"conceptCode"`
	ConceptType string    `gorm:"size:20;default:industry" json:"conceptType"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (StockConcept) TableName() string {
	return "stock_concept"
}

// StockFilterCondition 用户保存的筛选条件
type StockFilterCondition struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID      uint      `gorm:"not null;index" json:"userId"`
	Name        string    `gorm:"size:100;not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Conditions  string    `gorm:"type:json;not null" json:"conditions"`
	ResultCount int       `json:"resultCount"`
	IsPinned    bool      `gorm:"default:false" json:"isPinned"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

func (StockFilterCondition) TableName() string {
	return "stock_filter_condition"
}

// StockWatchlist 自选股
type StockWatchlist struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID    uint      `gorm:"not null;index" json:"userId"`
	Code      string    `gorm:"size:16;not null" json:"code"`
	Name      string    `gorm:"size:50" json:"name"`
	GroupName string    `gorm:"size:50;default:默认" json:"groupName"`
	Note      string    `gorm:"size:200" json:"note"`
	CreatedAt time.Time `json:"createdAt"`
}

func (StockWatchlist) TableName() string {
	return "stock_watchlist"
}

// StockWatchlistItem 自选股展示项(联表补齐行情/财务, 字段与筛选页对齐)
type StockWatchlistItem struct {
	ID             uint     `json:"id"`
	Code           string   `json:"code"`
	GroupName      string   `json:"groupName"`
	Name           string   `json:"name"`
	Market         string   `json:"market"`
	Type           int      `json:"type"`
	Industry       string   `json:"industry"`
	IsSt           bool     `json:"isSt"`
	Price          *float64 `json:"price"`
	ChangePct      *float64 `json:"changePct"`
	TurnoverRate   *float64 `json:"turnoverRate"`
	Amount         *float64 `json:"amount"`
	MarketCap      *float64 `json:"marketCap"`
	FloatMarketCap *float64 `json:"floatMarketCap"`
	PeTtm          *float64 `json:"peTtm"`
	Pb             *float64 `json:"pb"`
	Roe            *float64 `json:"roe"`
	RevenueYoy     *float64 `json:"revenueYoy"`
	NetProfitYoy   *float64 `json:"netProfitYoy"`
	GrossMargin    *float64 `json:"grossMargin"`
	NetMargin      *float64 `json:"netMargin"`
	DebtRatio      *float64 `json:"debtRatio"`
	CurrentRatio   *float64 `json:"currentRatio"`
	QuickRatio     *float64 `json:"quickRatio"`
	Concepts       []string `json:"concepts"`
}

// === 筛选请求/响应结构 ===

// FilterCondition 单个筛选条件
type FilterCondition struct {
	Field    string `json:"field"`
	Operator string `json:"operator"`
	Value    any    `json:"value"`
}

// StockScreenRequest 筛选请求
type StockScreenRequest struct {
	Conditions    []FilterCondition `json:"conditions"`
	SortBy        string            `json:"sortBy"`
	SortOrder     string            `json:"sortOrder"`
	Page          int               `json:"page"`
	PageSize      int               `json:"pageSize"`
	ConceptNames  []string          `json:"conceptNames"`
	Industries    []string          `json:"industries"`
	Markets       []string          `json:"markets"`
	SecurityTypes []int             `json:"securityTypes"`
	ExcludeST     bool              `json:"excludeSt"`
	Keyword       string            `json:"keyword"`
}

// StockScreenResult 筛选结果项
type StockScreenResult struct {
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	Market         string   `json:"market"`
	Type           int      `json:"type"`
	Industry       string   `json:"industry"`
	Price          *float64 `json:"price"`
	ChangePct      *float64 `json:"changePct"`
	TurnoverRate   *float64 `json:"turnoverRate"`
	Amount         *float64 `json:"amount"`
	MarketCap      *float64 `json:"marketCap"`
	FloatMarketCap *float64 `json:"floatMarketCap"`
	PeTtm          *float64 `json:"peTtm"`
	Pb             *float64 `json:"pb"`
	Roe            *float64 `json:"roe"`
	RevenueYoy     *float64 `json:"revenueYoy"`
	NetProfitYoy   *float64 `json:"netProfitYoy"`
	GrossMargin    *float64 `json:"grossMargin"`
	NetMargin      *float64 `json:"netMargin"`
	DebtRatio      *float64 `json:"debtRatio"`
	CurrentRatio   *float64 `json:"currentRatio"`
	QuickRatio     *float64 `json:"quickRatio"`
	CashRatio      *float64 `json:"cashRatio"`
	NrTurnRatio    *float64 `json:"nrTurnRatio"`
	InvTurnRatio   *float64 `json:"invTurnRatio"`
	YoyEquity      *float64 `json:"yoyEquity"`
	YoyAsset       *float64 `json:"yoyAsset"`
	CfoToOr        *float64 `json:"cfoToOr"`
	Change5d       *float64 `json:"change5d"`
	Change20d      *float64 `json:"change20d"`
	Ma5            *float64 `json:"ma5"`
	Ma10           *float64 `json:"ma10"`
	Ma20           *float64 `json:"ma20"`
	Ma60           *float64 `json:"ma60"`
	Rsi6           *float64 `json:"rsi6"`
	Rsi12          *float64 `json:"rsi12"`
	Rsi24          *float64 `json:"rsi24"`
	Concepts       []string `json:"concepts"`
}

// StockScreenResponse 筛选响应
type StockScreenResponse struct {
	List       []StockScreenResult `json:"list"`
	Total      int64               `json:"total"`
	Page       int                 `json:"page"`
	PageSize   int                 `json:"pageSize"`
	TotalPages int                 `json:"totalPages"`
}

// StockFilterConditionRequest 保存筛选条件请求
type StockFilterConditionRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
	Conditions  string `json:"conditions" binding:"required"`
}

// StockWatchlistRequest 自选股请求
type StockWatchlistRequest struct {
	Code      string `json:"code" binding:"required"`
	Name      string `json:"name"`
	GroupName string `json:"groupName"`
	Note      string `json:"note"`
}

// StockKlineRequest K线请求
type StockKlineRequest struct {
	Period string `form:"period" json:"period"`
	Count  int    `form:"count" json:"count"`
}
