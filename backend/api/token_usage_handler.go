package api

import (
	"strconv"
	"time"

	"backend/service"

	"github.com/gin-gonic/gin"
)

// parseTokenUsageParams 解析 Token 用量查询参数 (统计与明细共用):
// granularity=hour|day|month|year (默认 day), model, userId, startTime, endTime (默认近7天)
func parseTokenUsageParams(c *gin.Context) (*service.TokenUsageStatsParams, bool) {
	params := &service.TokenUsageStatsParams{
		Granularity: c.DefaultQuery("granularity", "day"),
		Model:       c.Query("model"),
	}
	switch params.Granularity {
	case "hour", "day", "month", "year":
	default:
		SendError(c, "400", "粒度非法: "+params.Granularity+" (hour/day/month/year)")
		return nil, false
	}
	if v := c.Query("userId"); v != "" {
		if uid, err := strconv.ParseUint(v, 10, 64); err == nil {
			params.UserID = uint(uid)
		}
	}
	end := time.Now()
	start := end.AddDate(0, 0, -7)
	if v := c.Query("startTime"); v != "" {
		if t, ok := parseAuditTime(v); ok {
			start = t
		}
	}
	if v := c.Query("endTime"); v != "" {
		if t, ok := parseAuditTime(v); ok {
			end = t
		}
	}
	// 小时粒度区间太宽会产生海量分桶, 直接拒绝
	if params.Granularity == "hour" && end.Sub(start) > 31*24*time.Hour {
		SendError(c, "400", "小时粒度最多查询 31 天")
		return nil, false
	}
	params.StartTime = start
	params.EndTime = end
	return params, true
}

// HandleTokenUsageStats GET /api/admin/token-usages/stats
// 汇总 + 趋势 (按粒度分桶) + 模型分布 + 用户排行 (带限额)
func HandleTokenUsageStats(c *gin.Context) {
	params, ok := parseTokenUsageParams(c)
	if !ok {
		return
	}
	stats, err := service.NewTokenUsageService().GetStats(*params)
	if err != nil {
		SendError(c, "400", "统计失败: "+err.Error())
		return
	}
	SendSuccess(c, stats)
}

// HandleTokenUsageList GET /api/admin/token-usages 明细分页 (带用户名)
func HandleTokenUsageList(c *gin.Context) {
	params, ok := parseTokenUsageParams(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	rows, total, err := service.NewTokenUsageService().ListRecords(*params, page, pageSize)
	if err != nil {
		SendError(c, "500", "查询 Token 用量明细失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{
		"list":      rows,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
