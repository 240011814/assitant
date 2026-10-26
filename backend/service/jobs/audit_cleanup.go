package service

import (
	"encoding/json"
	"log"
	"time"

	"backend/model"
)

// CleanupAuditLogs 清理超过保留期的操作审计日志 (定时任务 audit.cleanup)。
// params: {"retentionDays": 90}; 分批删除避免一次性大事务锁表。
func CleanupAuditLogs(params json.RawMessage) error {
	retentionDays := 90
	if len(params) > 0 {
		var cfg struct {
			RetentionDays int `json:"retentionDays"`
		}
		if err := json.Unmarshal(params, &cfg); err == nil && cfg.RetentionDays > 0 {
			retentionDays = cfg.RetentionDays
		}
	}
	cutoff := time.Now().AddDate(0, 0, -retentionDays)

	const batchSize = 5000
	var total int64
	for {
		res := DB.Where("created_at < ?", cutoff).Limit(batchSize).Delete(&model.OperationAuditLog{})
		if res.Error != nil {
			return res.Error
		}
		total += res.RowsAffected
		if res.RowsAffected < batchSize {
			break
		}
	}

	if total > 0 {
		log.Printf("[audit.cleanup] 已清理 %d 条 %d 天前的审计日志", total, retentionDays)
	}
	return nil
}
