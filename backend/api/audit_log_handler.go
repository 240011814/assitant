package api

import (
	"strconv"
	"strings"
	"time"

	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

// HandleListAuditLogs 操作审计日志列表 (分页 + 筛选)
// 筛选: userId, method, path (模糊), success, startTime, endTime
func HandleListAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page = service.NormalizePage(page)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := service.DB.Model(&model.OperationAuditLog{})
	if v := c.Query("userId"); v != "" {
		if uid, err := strconv.ParseUint(v, 10, 64); err == nil {
			query = query.Where("user_id = ?", uid)
		}
	}
	if v := c.Query("method"); v != "" {
		query = query.Where("method = ?", strings.ToUpper(v))
	}
	if v := c.Query("path"); v != "" {
		query = query.Where("path LIKE ?", "%"+v+"%")
	}
	if v := c.Query("success"); v != "" {
		query = query.Where("success = ?", v == "true" || v == "1")
	}
	if v := c.Query("startTime"); v != "" {
		if t, ok := parseAuditTime(v); ok {
			query = query.Where("created_at >= ?", t)
		}
	}
	if v := c.Query("endTime"); v != "" {
		if t, ok := parseAuditTime(v); ok {
			query = query.Where("created_at <= ?", t)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		SendError(c, "500", "查询审计日志失败: "+err.Error())
		return
	}

	var logs []model.OperationAuditLog
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&logs).Error; err != nil {
		SendError(c, "500", "查询审计日志失败: "+err.Error())
		return
	}

	SendSuccess(c, gin.H{
		"list":      logs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func parseAuditTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
