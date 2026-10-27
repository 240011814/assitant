package jobs

import (
	"encoding/json"

	"backend/model"
	"backend/service/stock"
)

// registerStockTasks 股票数据同步与预警任务。
// baostock 同步必须增量+串行 (每日调用限额), 业务侧用 RunExclusive 互斥防重入。
func registerStockTasks(d *Deps) {
	sync := d.StockSync
	ch := d.StockCh

	d.JobScheduler.RegisterTask("stock.sync_stock_list", "同步股票列表", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
		return sync.RunExclusive("股票列表(定时)", sync.SyncStockList)
	})

	d.JobScheduler.RegisterTask("stock.sync_daily_quotes", "同步行情数据(日/周/月K线, 增量)", json.RawMessage(`{"force": false}`), func(_ *model.JobDefinition, params json.RawMessage) error {
		force := false
		if len(params) > 0 {
			var p struct {
				Force bool `json:"force"`
			}
			if err := json.Unmarshal(params, &p); err == nil {
				force = p.Force
			}
		}
		task := "行情数据(定时)"
		if force {
			task = "行情数据全量(定时)"
		}
		return sync.RunExclusive(task, func() error { return sync.SyncDailyQuotes(force) })
	})

	d.JobScheduler.RegisterTask("stock.sync_finance_all", "同步全部股票财务数据(增量)", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
		return sync.RunExclusive("全部财务数据(定时)", sync.SyncAllFinance)
	})

	d.JobScheduler.RegisterTask("stock.sync_macro", "同步宏观经济数据(存款/贷款利率/准备金率/货币供应量)", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
		return sync.RunExclusive("宏观经济数据(定时)", func() error { return sync.SyncMacroData(false) })
	})

	d.JobScheduler.RegisterTask("stock.sync_clickhouse", "同步行情/财务到ClickHouse(增量)", json.RawMessage(`{"full": false}`), func(_ *model.JobDefinition, params json.RawMessage) error {
		full := false
		if len(params) > 0 {
			var p struct {
				Full bool `json:"full"`
			}
			if err := json.Unmarshal(params, &p); err == nil {
				full = p.Full
			}
		}
		task := "ClickHouse同步(定时)"
		if full {
			task = "ClickHouse全量重建(定时)"
		}
		// CH 复制不消耗 baostock 配额, 用独立互斥: 卡住/全量重建期间不阻塞行情/财务同步
		return ch.RunExclusive(task, func() error {
			if !stock.ChEnabled() {
				return nil
			}
			if full {
				return ch.SyncFull()
			}
			return ch.SyncIncremental()
		})
	})

	// 自选股预警评估 (建议每个交易日收盘后)
	d.JobScheduler.RegisterTask("stock.evaluate_alerts", "评估自选股预警规则并推送 (建议每个交易日收盘后)", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
		_, err := d.StockAlert.EvaluateAll()
		return err
	})
}
