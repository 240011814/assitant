package service

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"backend/model"

	"gorm.io/gorm"
)

// TokenUsageService 用户 AI Token 用量: 记录 (各会话链路埋点调用) / 限额校验 / 统计查询
type TokenUsageService struct{}

func NewTokenUsageService() *TokenUsageService { return &TokenUsageService{} }

// RecordTokenUsage 记录一次模型调用的用量 (每条明细对应一次模型调用, ReAct 每轮各一条)。
// 输入/输出都为 0 时忽略 (避免无用量空记录); 失败只记日志不阻断调用链路。
func (s *TokenUsageService) Record(userID uint, modelName, source string, prompt, completion int64) {
	if prompt <= 0 && completion <= 0 {
		return
	}
	if DB == nil {
		return
	}
	if modelName == "-" { // 防御: 上游传空模型时不要写成占位符
		modelName = ""
	}
	row := model.AITokenUsage{
		UserID:           userID,
		Model:            modelName,
		Source:           source,
		PromptTokens:     prompt,
		CompletionTokens: completion,
		TotalTokens:      prompt + completion,
	}
	if err := DB.Create(&row).Error; err != nil {
		log.Printf("[token-usage] record failed user=%d model=%s source=%s err=%v", userID, modelName, source, err)
	}
}

// RecordTokenUsage 包级便捷入口 (编排服务/处理器无 TokenUsageService 注入时使用)
func RecordTokenUsage(userID uint, modelName, source string, prompt, completion int64) {
	NewTokenUsageService().Record(userID, modelName, source, prompt, completion)
}

// CheckTokenQuota 用户月度限额校验: 超限返回可读错误, 未配置/查询失败/无用户上下文都不拦截。
// 只在请求入口调用, 运行中不中断 (超量部分记完账, 下一轮生效)
func (s *TokenUsageService) CheckTokenQuota(userID uint) error {
	if userID == 0 || DB == nil {
		return nil
	}
	var user model.User
	if err := DB.Select("id", "token_quota_month").First(&user, userID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		log.Printf("[token-usage] read quota failed user=%d err=%v", userID, err)
		return nil
	}
	if user.TokenQuotaMonth == nil || *user.TokenQuotaMonth <= 0 {
		return nil
	}
	used := s.MonthUsage(userID)
	if used >= int64(*user.TokenQuotaMonth) {
		return fmt.Errorf("本月 Token 用量已达上限 (已用 %d / 限额 %d), 请联系管理员", used, *user.TokenQuotaMonth)
	}
	return nil
}

// MonthUsage 用户本月 (自然月) 已用 token 总量
func (s *TokenUsageService) MonthUsage(userID uint) int64 {
	var used int64
	if err := DB.Model(&model.AITokenUsage{}).
		Select("COALESCE(SUM(total_tokens), 0)").
		Where("user_id = ? AND created_at >= ?", userID, monthStart(time.Now())).
		Scan(&used).Error; err != nil {
		log.Printf("[token-usage] sum month usage failed user=%d err=%v", userID, err)
		return 0
	}
	return used
}

func monthStart(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.Local)
}

// ---------- 统计查询 ----------

// TokenUsageStatsParams 统计参数 (由 handler 从 query 解析)
type TokenUsageStatsParams struct {
	// Granularity 趋势分桶粒度: hour / day / month / year
	Granularity string
	Model       string
	UserID      uint
	StartTime   time.Time
	EndTime     time.Time
}

// TokenUsageStats 统计结果: 汇总 + 趋势 + 模型分布 + 用户排行
type TokenUsageStats struct {
	Summary model.TokenUsageSummary        `json:"summary"`
	Trend   []model.TokenUsageTrendItem    `json:"trend"`
	ByModel []model.TokenUsageModelItem    `json:"by_model"`
	ByUser  []model.TokenUsageUserItem     `json:"by_user"`
}

// granularityFormat MySQL DATE_FORMAT 格式 (分桶键)
var granularityFormat = map[string]string{
	"hour":  "%Y-%m-%d %H:00",
	"day":   "%Y-%m-%d",
	"month": "%Y-%m",
	"year":  "%Y",
}

// GetStats 区间统计: 单项失败只记日志返回空列表 (与 Dashboard 服务一致, 不整体报错)
func (s *TokenUsageService) GetStats(p TokenUsageStatsParams) (*TokenUsageStats, error) {
	format, ok := granularityFormat[p.Granularity]
	if !ok {
		return nil, fmt.Errorf("粒度非法: %s (hour/day/month/year)", p.Granularity)
	}
	if !p.EndTime.IsZero() && !p.StartTime.IsZero() && p.EndTime.Before(p.StartTime) {
		return nil, errors.New("结束时间早于开始时间")
	}
	out := &TokenUsageStats{Trend: []model.TokenUsageTrendItem{}}

	base := s.scope(p)

	// 汇总
	if err := base.Session(&gorm.Session{}).
		Select("COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens, COUNT(*) AS calls, COUNT(DISTINCT user_id) AS users").
		Scan(&out.Summary).Error; err != nil {
		return nil, fmt.Errorf("统计汇总失败: %w", err)
	}

	// 趋势分桶
	type trendRow struct {
		Bucket           string `gorm:"column:bucket"`
		PromptTokens     int64  `gorm:"column:prompt_tokens"`
		CompletionTokens int64  `gorm:"column:completion_tokens"`
		TotalTokens      int64  `gorm:"column:total_tokens"`
		Calls            int64  `gorm:"column:calls"`
	}
	var rows []trendRow
	selectExpr := "DATE_FORMAT(created_at, '" + format + "') AS bucket, " +
		"COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, " +
		"COALESCE(SUM(total_tokens),0) AS total_tokens, COUNT(*) AS calls"
	if err := base.Session(&gorm.Session{}).
		Select(selectExpr).Group("bucket").Order("bucket ASC").
		Scan(&rows).Error; err != nil {
		log.Printf("[token-usage] trend query failed: %v", err)
	}
	byBucket := make(map[string]trendRow, len(rows))
	for _, r := range rows {
		byBucket[r.Bucket] = r
	}
	// 应用侧补齐空桶, 时间轴连续
	for _, bucket := range orchTokenBuckets(p, format) {
		r, ok := byBucket[bucket]
		if !ok {
			r = trendRow{Bucket: bucket}
		}
		out.Trend = append(out.Trend, model.TokenUsageTrendItem{
			Bucket:           r.Bucket,
			PromptTokens:     r.PromptTokens,
			CompletionTokens: r.CompletionTokens,
			TotalTokens:      r.TotalTokens,
			Calls:            r.Calls,
		})
	}

	// 模型分布 (空 model = 默认模型, 原样返回空串由前端显示"默认模型")
	if err := base.Session(&gorm.Session{}).
		Select("model, COALESCE(SUM(prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(completion_tokens),0) AS completion_tokens, COALESCE(SUM(total_tokens),0) AS total_tokens, COUNT(*) AS calls").
		Group("model").Order("total_tokens DESC").
		Scan(&out.ByModel).Error; err != nil {
		log.Printf("[token-usage] by-model query failed: %v", err)
	}
	if out.ByModel == nil {
		out.ByModel = []model.TokenUsageModelItem{}
	}

	// 用户排行 (带用户名与限额, Top 20)
	if err := DB.Table("ai_token_usages tu").
		Select("tu.user_id, COALESCE(NULLIF(u.username, ''), CONCAT('#', tu.user_id)) AS username, u.nickname, u.token_quota_month, "+
			"COALESCE(SUM(tu.prompt_tokens),0) AS prompt_tokens, COALESCE(SUM(tu.completion_tokens),0) AS completion_tokens, "+
			"COALESCE(SUM(tu.total_tokens),0) AS total_tokens, COUNT(*) AS calls").
		Joins("LEFT JOIN users u ON u.id = tu.user_id").
		Where("tu.created_at >= ? AND tu.created_at <= ?", p.StartTime, p.EndTime).
		Group("tu.user_id, u.username, u.nickname, u.token_quota_month").
		Order("total_tokens DESC").Limit(20).
		Scan(&out.ByUser).Error; err != nil {
		log.Printf("[token-usage] by-user query failed: %v", err)
	}
	if out.ByUser == nil {
		out.ByUser = []model.TokenUsageUserItem{}
	}

	return out, nil
}

// MyTokenUsage 用户自查自己的用量 (个人中心展示)
type MyTokenUsage struct {
	// MonthUsed 本月 (自然月) 已用 token 总量
	MonthUsed int64 `json:"month_used"`
	// QuotaMonth 月度限额 (NULL/0=不限)
	QuotaMonth *int                        `json:"quota_month"`
	Trend      []model.TokenUsageTrendItem `json:"trend"`
	ByModel    []model.TokenUsageModelItem `json:"by_model"`
}

// GetMyUsage 当前登录用户自己的用量: 本月已用/限额 + 近 30 天按日趋势与模型分布
func (s *TokenUsageService) GetMyUsage(userID uint) (*MyTokenUsage, error) {
	out := &MyTokenUsage{Trend: []model.TokenUsageTrendItem{}, ByModel: []model.TokenUsageModelItem{}}
	var user model.User
	if err := DB.Select("id", "token_quota_month").First(&user, userID).Error; err == nil {
		out.QuotaMonth = user.TokenQuotaMonth
	}
	out.MonthUsed = s.MonthUsage(userID)
	end := time.Now()
	start := end.AddDate(0, 0, -29)
	stats, err := s.GetStats(TokenUsageStatsParams{
		Granularity: "day",
		UserID:      userID,
		StartTime:   start,
		EndTime:     end,
	})
	if err != nil {
		return nil, err
	}
	out.Trend = stats.Trend
	out.ByModel = stats.ByModel
	return out, nil
}

// ListRecords 明细分页 (带用户名)
func (s *TokenUsageService) ListRecords(p TokenUsageStatsParams, page, pageSize int) ([]map[string]any, int64, error) {
	query := DB.Table("ai_token_usages tu").
		Joins("LEFT JOIN users u ON u.id = tu.user_id").
		Where("tu.created_at >= ? AND tu.created_at <= ?", p.StartTime, p.EndTime)
	if p.Model != "" {
		query = query.Where("tu.model = ?", p.Model)
	}
	if p.UserID > 0 {
		query = query.Where("tu.user_id = ?", p.UserID)
	}
	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	var rows []map[string]any
	err := query.Session(&gorm.Session{}).
		Select("tu.id, tu.user_id, COALESCE(NULLIF(u.username, ''), CONCAT('#', tu.user_id)) AS username, u.nickname, "+
			"tu.model, tu.source, tu.prompt_tokens, tu.completion_tokens, tu.total_tokens, tu.created_at").
		Order("tu.id DESC").Offset((page - 1) * pageSize).Limit(pageSize).
		Scan(&rows).Error
	return rows, total, err
}

// scope 组装区间 + 模型/用户筛选 (Model/Count 复用需先 Session 化, 避免条件串染)
func (s *TokenUsageService) scope(p TokenUsageStatsParams) *gorm.DB {
	query := DB.Model(&model.AITokenUsage{}).
		Where("created_at >= ? AND created_at <= ?", p.StartTime, p.EndTime)
	if p.Model != "" {
		query = query.Where("model = ?", p.Model)
	}
	if p.UserID > 0 {
		query = query.Where("user_id = ?", p.UserID)
	}
	return query
}

// orchTokenBuckets 按粒度生成连续分桶键 (补齐无数据的时间段); 上限 1000 桶防御异常区间
func orchTokenBuckets(p TokenUsageStatsParams, format string) []string {
	loc := time.Local
	step := func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }
	switch p.Granularity {
	case "hour":
		step = func(t time.Time) time.Time { return t.Add(time.Hour) }
	case "month":
		step = func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }
	case "year":
		step = func(t time.Time) time.Time { return t.AddDate(1, 0, 0) }
	}
	start, end := p.StartTime, p.EndTime
	if start.IsZero() || end.IsZero() {
		return nil
	}
	var buckets []string
	// 起点对齐到桶底
	cur := start
	switch p.Granularity {
	case "hour":
		cur = time.Date(start.Year(), start.Month(), start.Day(), start.Hour(), 0, 0, 0, loc)
	case "day":
		cur = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	case "month":
		cur = time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, loc)
	case "year":
		cur = time.Date(start.Year(), 1, 1, 0, 0, 0, 0, loc)
	}
	for i := 0; !cur.After(end) && i < 1000; i++ {
		buckets = append(buckets, cur.Format(mysqlFormatToGo(format)))
		cur = step(cur)
	}
	return buckets
}

// mysqlFormatToGo 把 DATE_FORMAT 格式转为 Go time.Format 布局 (只用到这四种)
func mysqlFormatToGo(format string) string {
	r := strings.NewReplacer(
		"%Y", "2006", "%m", "01", "%d", "02", "%H", "15",
	)
	return r.Replace(format)
}
