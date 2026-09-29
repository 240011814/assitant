package api

import (
	"backend/model"
	"backend/service"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// stockCodePattern 完整证券代码白名单 (sh.600000 / sz.000003 / bj.430047),
// 防止任意字符拼进 baostock 代理 URL 与落库
var stockCodePattern = regexp.MustCompile(`^(sh|sz|bj)\.\d{6}$`)

type StockHandler struct {
	svc         *service.StockService
	syncService *service.StockSyncService
	chService   *service.StockChSyncService
}

func NewStockHandler(svc *service.StockService, baostockURL string) *StockHandler {
	return &StockHandler{
		svc:         svc,
		syncService: service.NewStockSyncService(baostockURL),
		chService:   service.NewStockChSyncService(),
	}
}

// SyncService 暴露同步服务(供定时任务注册使用)
func (h *StockHandler) SyncService() *service.StockSyncService {
	return h.syncService
}

// RegisterCronTasks 注册股票相关的后台定时任务
func (h *StockHandler) RegisterCronTasks(js *service.JobScheduler) {
	sync := h.syncService
	ch := h.chService

	js.RegisterTask("stock.sync_stock_list", "同步股票列表", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
		return sync.RunExclusive("股票列表(定时)", sync.SyncStockList)
	})

	js.RegisterTask("stock.sync_daily_quotes", "同步行情数据(日/周/月K线, 增量)", json.RawMessage(`{"force": false}`), func(_ *model.JobDefinition, params json.RawMessage) error {
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

	js.RegisterTask("stock.sync_finance_all", "同步全部股票财务数据(增量)", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
		return sync.RunExclusive("全部财务数据(定时)", sync.SyncAllFinance)
	})

	js.RegisterTask("stock.sync_macro", "同步宏观经济数据(存款/贷款利率/准备金率/货币供应量)", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
		return sync.RunExclusive("宏观经济数据(定时)", func() error { return sync.SyncMacroData(false) })
	})

	js.RegisterTask("stock.sync_clickhouse", "同步行情/财务到ClickHouse(增量)", json.RawMessage(`{"full": false}`), func(_ *model.JobDefinition, params json.RawMessage) error {
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
			if !service.ChEnabled() {
				return nil
			}
			if full {
				return ch.SyncFull()
			}
			return ch.SyncIncremental()
		})
	})
}

// HandleScreen 股票筛选
func (h *StockHandler) HandleScreen(c *gin.Context) {
	var req model.StockScreenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	result, err := h.svc.Screen(req)
	if err != nil {
		SendError(c, "500", "筛选失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleGetDetail 个股详情
func (h *StockHandler) HandleGetDetail(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		SendError(c, "400", "股票代码不能为空")
		return
	}

	result, err := h.svc.GetDetail(code)
	if err != nil {
		SendError(c, "404", err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleGetSyncState 个股数据同步状态
func (h *StockHandler) HandleGetSyncState(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		SendError(c, "400", "股票代码不能为空")
		return
	}

	result, err := h.svc.GetSyncState(code)
	if err != nil {
		SendError(c, "500", "获取同步状态失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleGetFinanceHistory 历史财务数据
func (h *StockHandler) HandleGetFinanceHistory(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		SendError(c, "400", "股票代码不能为空")
		return
	}

	limitStr := c.DefaultQuery("limit", "8")
	limit, _ := strconv.Atoi(limitStr)

	result, err := h.svc.GetFinanceHistory(code, limit)
	if err != nil {
		SendError(c, "500", "获取财务历史失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleGetKline K线数据
func (h *StockHandler) HandleGetKline(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		SendError(c, "400", "股票代码不能为空")
		return
	}

	period := c.DefaultQuery("period", "daily")
	countStr := c.DefaultQuery("count", "120")
	count, _ := strconv.Atoi(countStr)

	result, err := h.svc.GetKline(code, period, count)
	if err != nil {
		SendError(c, "500", "获取K线失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleGetIndustries 行业列表
func (h *StockHandler) HandleGetIndustries(c *gin.Context) {
	result, err := h.svc.GetIndustries()
	if err != nil {
		SendError(c, "500", "获取行业列表失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleGetConcepts 概念列表
func (h *StockHandler) HandleGetConcepts(c *gin.Context) {
	result, err := h.svc.GetConcepts()
	if err != nil {
		SendError(c, "500", "获取概念列表失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleSaveFilterCondition 保存筛选条件
func (h *StockHandler) HandleSaveFilterCondition(c *gin.Context) {
	userID := GetUserID(c)
	var req model.StockFilterConditionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.SaveFilterCondition(userID, req); err != nil {
		SendError(c, "500", "保存失败: "+err.Error())
		return
	}

	SendSuccess(c, true)
}

// HandleListFilterConditions 获取筛选条件列表
func (h *StockHandler) HandleListFilterConditions(c *gin.Context) {
	userID := GetUserID(c)
	result, err := h.svc.ListFilterConditions(userID)
	if err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleDeleteFilterCondition 删除筛选条件
func (h *StockHandler) HandleDeleteFilterCondition(c *gin.Context) {
	userID := GetUserID(c)
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}

	if err := h.svc.DeleteFilterCondition(userID, uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}

	SendSuccess(c, true)
}

// HandleAddWatchlist 添加自选股
func (h *StockHandler) HandleAddWatchlist(c *gin.Context) {
	userID := GetUserID(c)
	var req model.StockWatchlistRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.AddWatchlist(userID, req); err != nil {
		SendError(c, "500", "添加失败: "+err.Error())
		return
	}

	SendSuccess(c, true)
}

// HandleListWatchlist 获取自选股列表
func (h *StockHandler) HandleListWatchlist(c *gin.Context) {
	userID := GetUserID(c)
	result, err := h.svc.ListWatchlist(userID)
	if err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleListWatchlistGroups 获取自选股分组
func (h *StockHandler) HandleListWatchlistGroups(c *gin.Context) {
	userID := GetUserID(c)
	result, err := h.svc.ListWatchlistGroups(userID)
	if err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleListWatchlistCodes 获取已自选股票代码
func (h *StockHandler) HandleListWatchlistCodes(c *gin.Context) {
	userID := GetUserID(c)
	result, err := h.svc.ListWatchlistCodes(userID)
	if err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleDeleteWatchlist 删除自选股
func (h *StockHandler) HandleDeleteWatchlist(c *gin.Context) {
	userID := GetUserID(c)
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}

	if err := h.svc.DeleteWatchlist(userID, uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}

	SendSuccess(c, true)
}

// HandleSyncStockList 同步股票列表
func (h *StockHandler) HandleSyncStockList(c *gin.Context) {
	if !h.syncService.StartTask("股票列表", h.syncService.SyncStockList) {
		SendError(c, "409", "已有同步任务在运行中，请稍后再试")
		return
	}
	SendSuccess(c, true)
}

// HandleSyncDailyQuotes 同步行情数据(日/周/月K线, 增量; ?force=1 全量)
func (h *StockHandler) HandleSyncDailyQuotes(c *gin.Context) {
	force := c.Query("force") == "1"
	task := "行情数据"
	if force {
		task = "行情数据(全量)"
	}
	if !h.syncService.StartTask(task, func() error {
		return h.syncService.SyncDailyQuotes(force)
	}) {
		SendError(c, "409", "已有同步任务在运行中，请稍后再试")
		return
	}
	SendSuccess(c, true)
}

// HandleSyncSingleStock 同步单只股票行情+财务
func (h *StockHandler) HandleSyncSingleStock(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		SendError(c, "400", "股票代码不能为空")
		return
	}
	if !stockCodePattern.MatchString(code) {
		SendError(c, "400", "股票代码格式错误, 应为 sh.XXXXXX / sz.XXXXXX / bj.XXXXXX")
		return
	}
	market := c.Query("market")
	force := c.Query("force") == "1"

	task := fmt.Sprintf("单股同步(%s)", code)
	fn := func() error {
		starts := map[string]time.Time{}
		var latestTradeDay time.Time
		if !force {
			starts = h.syncService.GetStoredKlineStarts(code)
			latestTradeDay = h.syncService.LatestTradeDay()
		}
		// 小时线目标日独立于 force: 盘中回落到上一交易日, 不拉当日未完成的bar
		hourlyDay := h.syncService.LatestCompletedTradeDay()
		kRows, kErr := h.syncService.SyncSingleStockDaily(code, market, starts, latestTradeDay, hourlyDay, service.KlineFrequencies())
		fRows, fErr := h.syncService.SyncFinanceData(code, market)

		var errs []string
		if kErr != nil {
			errs = append(errs, "行情: "+kErr.Error())
		}
		if fErr != nil {
			errs = append(errs, "财务: "+fErr.Error())
		}
		if len(errs) > 0 {
			return fmt.Errorf("%s", strings.Join(errs, "; "))
		}
		if kRows == 0 && fRows == 0 {
			// 库里已有数据说明只是无新增(已是最新), 库里完全没有才是异常
			if !h.syncService.HasStoredStockData(code) {
				return fmt.Errorf("未获取到 %s 的任何数据, 该股票可能已退市或长期停牌", code)
			}
		}
		return nil
	}
	if !h.syncService.StartTask(task, fn) {
		SendError(c, "409", "已有同步任务在运行中，请稍后再试")
		return
	}
	SendSuccess(c, true)
}

// HandleSyncFinanceData 同步单只股票财务数据
func (h *StockHandler) HandleSyncFinanceData(c *gin.Context) {
	code := c.Query("code")
	if code == "" {
		SendError(c, "400", "股票代码不能为空")
		return
	}

	task := fmt.Sprintf("财务同步(%s)", code)
	if !h.syncService.StartTask(task, func() error {
		_, err := h.syncService.SyncFinanceData(code, c.Query("market"))
		return err
	}) {
		SendError(c, "409", "已有同步任务在运行中，请稍后再试")
		return
	}
	SendSuccess(c, true)
}

// HandleSyncAllFinance 同步全部股票财务数据
func (h *StockHandler) HandleSyncAllFinance(c *gin.Context) {
	if !h.syncService.StartTask("全部财务数据", h.syncService.SyncAllFinance) {
		SendError(c, "409", "已有同步任务在运行中，请稍后再试")
		return
	}
	SendSuccess(c, true)
}

// HandleSyncStatus 获取同步任务状态
func (h *StockHandler) HandleSyncStatus(c *gin.Context) {
	SendSuccess(c, h.syncService.GetStatus())
}

// HandleGetMacroReserveRatios 存款准备金率历史
func (h *StockHandler) HandleGetMacroReserveRatios(c *gin.Context) {
	result, err := h.svc.GetReserveRatios()
	if err != nil {
		SendError(c, "500", "获取存款准备金率失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroMoneySupplyMonth 货币供应量月度历史
func (h *StockHandler) HandleGetMacroMoneySupplyMonth(c *gin.Context) {
	result, err := h.svc.GetMoneySupplyMonth()
	if err != nil {
		SendError(c, "500", "获取货币供应量(月度)失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroMoneySupplyYear 货币供应量年度历史(年底余额)
func (h *StockHandler) HandleGetMacroMoneySupplyYear(c *gin.Context) {
	result, err := h.svc.GetMoneySupplyYear()
	if err != nil {
		SendError(c, "500", "获取货币供应量(年度)失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroDepositRates 存款利率历史
func (h *StockHandler) HandleGetMacroDepositRates(c *gin.Context) {
	result, err := h.svc.GetDepositRates()
	if err != nil {
		SendError(c, "500", "获取存款利率失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroLoanRates 贷款利率历史
func (h *StockHandler) HandleGetMacroLoanRates(c *gin.Context) {
	result, err := h.svc.GetLoanRates()
	if err != nil {
		SendError(c, "500", "获取贷款利率失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroLPR 贷款市场报价利率 LPR 历史
func (h *StockHandler) HandleGetMacroLPR(c *gin.Context) {
	result, err := h.svc.GetLPR()
	if err != nil {
		SendError(c, "500", "获取LPR失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroGDP GDP数据
func (h *StockHandler) HandleGetMacroGDP(c *gin.Context) {
	result, err := h.svc.GetGDP()
	if err != nil {
		SendError(c, "500", "获取GDP失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroCPI CPI数据
func (h *StockHandler) HandleGetMacroCPI(c *gin.Context) {
	result, err := h.svc.GetCPI()
	if err != nil {
		SendError(c, "500", "获取CPI失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroPMI PMI数据
func (h *StockHandler) HandleGetMacroPMI(c *gin.Context) {
	result, err := h.svc.GetPMI()
	if err != nil {
		SendError(c, "500", "获取PMI失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetMacroPPI PPI数据
func (h *StockHandler) HandleGetMacroPPI(c *gin.Context) {
	result, err := h.svc.GetPPI()
	if err != nil {
		SendError(c, "500", "获取PPI失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleSyncMacro 同步宏观经济数据(存款准备金率/货币供应量)
func (h *StockHandler) HandleSyncMacro(c *gin.Context) {
	if !h.syncService.StartTask("宏观经济数据", func() error {
		return h.syncService.SyncMacroData(false)
	}) {
		SendError(c, "409", "已有同步任务在运行中，请稍后再试")
		return
	}
	SendSuccess(c, true)
}
