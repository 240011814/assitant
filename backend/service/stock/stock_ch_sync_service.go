package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"backend/model"

	"gorm.io/gorm"
)

// StockChSyncService MySQL -> ClickHouse 行情/财务数据复制
// MySQL 为真相源, ClickHouse 为下游只读副本(回测/分析用)
// 增量策略: 按 updated_at 时刻水位, 幂等重放(ReplacingMergeTree 以 updated_at 为版本), 不消耗 baostock 配额
// 覆盖场景: 按日全量重放/单股同步/手工修数 都会触发 updated_at 变化, 从而被增量捕获
type StockChSyncService struct {
	mu      sync.Mutex
	running bool
	task    string
}

func NewStockChSyncService() *StockChSyncService {
	return &StockChSyncService{}
}

const (
	chWatermarkDaily   = "clickhouse_stock_daily"   // CH stock_daily 复制水位(updated_at)
	chWatermarkFinance = "clickhouse_stock_finance" // CH stock_finance 复制水位(updated_at)
	chReadBatch        = 20000                      // MySQL 每批读取条数

	chIncrementalTimeout = 30 * time.Minute  // 增量复制整体超时
	chFullTimeout        = 6 * time.Hour     // 全量重建整体超时 (千万级行)
)

// RunExclusive CH 复制任务的独立互斥: 不占用 baostock 同步的全局锁,
// CH 卡住/全量重建期间不会饿死行情/财务同步; 冲突返回 ErrSyncBusy 哨兵 (调度器记 skipped)
func (s *StockChSyncService) RunExclusive(task string, fn func() error) error {
	s.mu.Lock()
	if s.running {
		current := s.task
		s.mu.Unlock()
		return fmt.Errorf("同步任务 %s 正在运行中: %w", current, ErrSyncBusy)
	}
	s.running = true
	s.task = task
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()
	return fn()
}

// ChEnabled ClickHouse 是否可用
func ChEnabled() bool {
	return CH != nil
}

// SyncIncremental 增量复制(日常): 首次调用(无水位)等价于全量
func (s *StockChSyncService) SyncIncremental() error {
	if CH == nil {
		return errors.New("ClickHouse 未启用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), chIncrementalTimeout)
	defer cancel()
	if err := s.syncDaily(ctx, false); err != nil {
		return err
	}
	return s.syncFinance(ctx, false)
}

// SyncFull 全量重建(首次初始化/数据修复): 清空 CH 两张表后全量复制
func (s *StockChSyncService) SyncFull() error {
	if CH == nil {
		return errors.New("ClickHouse 未启用")
	}
	ctx, cancel := context.WithTimeout(context.Background(), chFullTimeout)
	defer cancel()
	for _, table := range []string{"stock_daily", "stock_finance"} {
		if err := CH.Exec(ctx, "TRUNCATE TABLE "+table); err != nil {
			return fmt.Errorf("清空 ClickHouse 表 %s 失败: %v", table, err)
		}
	}
	if err := s.syncDaily(ctx, true); err != nil {
		return err
	}
	return s.syncFinance(ctx, true)
}

// chDailyRow stock_daily 复制行(模型未定义 updated_at, 这里显式声明)
type chDailyRow struct {
	ID           uint      `gorm:"column:id"`
	Code         string    `gorm:"column:code"`
	Frequency    string    `gorm:"column:frequency"`
	TradeDate    time.Time `gorm:"column:trade_date"`
	Open         *float64  `gorm:"column:open"`
	High         *float64  `gorm:"column:high"`
	Low          *float64  `gorm:"column:low"`
	Close        *float64  `gorm:"column:close"`
	Preclose     *float64  `gorm:"column:preclose"`
	Volume       *float64  `gorm:"column:volume"`
	Amount       *float64  `gorm:"column:amount"`
	TurnoverRate *float64  `gorm:"column:turnover_rate"`
	ChangePct    *float64  `gorm:"column:change_pct"`
	TradeStatus  *int8     `gorm:"column:trade_status"`
	PeTtm        *float64  `gorm:"column:pe_ttm"`
	PbMrq        *float64  `gorm:"column:pb_mrq"`
	PsTtm        *float64  `gorm:"column:ps_ttm"`
	PcfNcfTtm    *float64  `gorm:"column:pcf_ncf_ttm"`
	Amplitude    *float64  `gorm:"column:amplitude"`
	UpdatedAt    time.Time `gorm:"column:updated_at"`
}

const chDailyCols = "id, code, frequency, trade_date, open, high, low, close, preclose, volume, amount, turnover_rate, change_pct, trade_status, pe_ttm, pb_mrq, ps_ttm, pcf_ncf_ttm, amplitude, updated_at"

// syncDaily 复制 stock_daily: full=true 忽略水位全量; 否则按 updated_at >= 水位
func (s *StockChSyncService) syncDaily(ctx context.Context, full bool) error {
	wm := time.Time{}
	if !full {
		wm = s.getWatermarkTime(chWatermarkDaily)
	}
	lastID := uint(0)
	maxUpdated := wm
	total := 0
	for {
		var rows []chDailyRow
		q := DB.Table("stock_daily").Select(chDailyCols)
		if !full && !wm.IsZero() {
			q = q.Where("updated_at >= ?", wm)
		}
		if lastID > 0 {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Order("id ASC").Limit(chReadBatch).Scan(&rows).Error; err != nil {
			return fmt.Errorf("读取 stock_daily 失败: %v", err)
		}
		if len(rows) == 0 {
			break
		}
		if err := s.insertDailyBatch(ctx, rows); err != nil {
			return fmt.Errorf("写入 ClickHouse stock_daily 失败: %v", err)
		}
		total += len(rows)
		lastID = rows[len(rows)-1].ID
		for i := range rows {
			if rows[i].UpdatedAt.After(maxUpdated) {
				maxUpdated = rows[i].UpdatedAt
			}
		}
		if len(rows) < chReadBatch {
			break
		}
	}
	s.saveWatermarkTime(chWatermarkDaily, maxUpdated, "ok", "")
	log.Printf("[ChSync] stock_daily -> ClickHouse: %d 条, 水位=%s", total, maxUpdated.Format("2006-01-02 15:04:05"))
	return nil
}

func (s *StockChSyncService) insertDailyBatch(ctx context.Context, rows []chDailyRow) error {
	batch, err := CH.PrepareBatch(ctx, "INSERT INTO stock_daily")
	if err != nil {
		return err
	}
	for i := range rows {
		r := rows[i]
		if err := batch.Append(
			r.Code, r.Frequency, r.TradeDate,
			r.Open, r.High, r.Low, r.Close, r.Preclose,
			r.Volume, r.Amount, r.TurnoverRate, r.ChangePct,
			r.TradeStatus, r.PeTtm, r.PbMrq, r.PsTtm, r.PcfNcfTtm, r.Amplitude,
			r.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

// syncFinance 复制 stock_finance
func (s *StockChSyncService) syncFinance(ctx context.Context, full bool) error {
	wm := time.Time{}
	if !full {
		wm = s.getWatermarkTime(chWatermarkFinance)
	}
	lastID := uint(0)
	maxUpdated := wm
	total := 0
	for {
		var rows []model.StockFinance
		q := DB.Table("stock_finance").Select(
			"id, code, report_date, report_type, pe_ttm, pb, ps_ttm, roe, roa, gross_margin, net_margin, revenue, revenue_yoy, net_profit, net_profit_yoy, debt_ratio, current_ratio, quick_ratio, cash_ratio, nr_turn_ratio, inv_turn_ratio, ca_turn_ratio, asset_turn_ratio, yoy_equity, yoy_asset, yoy_eps, cfo_to_or, cfo_to_np, eps, eps_deducted, bps, ocf_per_share, finance_sources, updated_at")
		if !full && !wm.IsZero() {
			q = q.Where("updated_at >= ?", wm)
		}
		if lastID > 0 {
			q = q.Where("id > ?", lastID)
		}
		if err := q.Order("id ASC").Limit(chReadBatch).Scan(&rows).Error; err != nil {
			return fmt.Errorf("读取 stock_finance 失败: %v", err)
		}
		if len(rows) == 0 {
			break
		}
		if err := s.insertFinanceBatch(ctx, rows); err != nil {
			return fmt.Errorf("写入 ClickHouse stock_finance 失败: %v", err)
		}
		total += len(rows)
		lastID = rows[len(rows)-1].ID
		for i := range rows {
			if rows[i].UpdatedAt.After(maxUpdated) {
				maxUpdated = rows[i].UpdatedAt
			}
		}
		if len(rows) < chReadBatch {
			break
		}
	}
	s.saveWatermarkTime(chWatermarkFinance, maxUpdated, "ok", "")
	log.Printf("[ChSync] stock_finance -> ClickHouse: %d 条, 水位=%s", total, maxUpdated.Format("2006-01-02 15:04:05"))
	return nil
}

func (s *StockChSyncService) insertFinanceBatch(ctx context.Context, rows []model.StockFinance) error {
	batch, err := CH.PrepareBatch(ctx, "INSERT INTO stock_finance")
	if err != nil {
		return err
	}
	for i := range rows {
		r := rows[i]
		if err := batch.Append(
			r.Code, r.ReportDate, r.ReportType,
			r.PeTtm, r.Pb, r.PsTtm,
			r.Roe, r.Roa, r.GrossMargin, r.NetMargin,
			r.Revenue, r.RevenueYoy, r.NetProfit, r.NetProfitYoy,
			r.DebtRatio, r.CurrentRatio, r.QuickRatio, r.CashRatio,
			r.NrTurnRatio, r.InvTurnRatio, r.CaTurnRatio, r.AssetTurnRatio,
			r.YoyEquity, r.YoyAsset, r.YoyEps,
			r.CfoToOr, r.CfoToNp,
			r.Eps, r.EpsDeducted, r.Bps, r.OcfPerShare,
			r.FinanceSources, r.UpdatedAt,
		); err != nil {
			return err
		}
	}
	return batch.Send()
}

// getWatermarkTime 读取时刻水位 (零值=无水位)
func (s *StockChSyncService) getWatermarkTime(name string) time.Time {
	var wm model.SyncWatermark
	if err := DB.Where("name = ?", name).First(&wm).Error; err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[ChSync] 读取水位 %s 失败: %v", name, err)
		}
		return time.Time{}
	}
	if wm.LastTime == nil {
		return time.Time{}
	}
	return *wm.LastTime
}

// saveWatermarkTime 保存时刻水位
func (s *StockChSyncService) saveWatermarkTime(name string, t time.Time, status, errMsg string) {
	var wm model.SyncWatermark
	err := DB.Where("name = ?", name).First(&wm).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[ChSync] 读取水位 %s 失败: %v", name, err)
			return
		}
		wm = model.SyncWatermark{Name: name}
	}
	now := time.Now()
	if !t.IsZero() {
		wm.LastTime = &t
	}
	wm.Status = status
	wm.Error = truncateText(errMsg, 250)
	wm.SyncedAt = &now
	if err := DB.Save(&wm).Error; err != nil {
		log.Printf("[ChSync] 写入水位 %s 失败: %v", name, err)
	}
}
