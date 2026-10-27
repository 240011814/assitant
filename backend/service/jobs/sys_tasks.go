package jobs

import (
	"encoding/json"

	"backend/model"
)

// registerSysTasks 系统维护类定时任务
func registerSysTasks(d *Deps) {
	// 操作审计日志清理 (默认种子任务: 每日 4 点, 保留 90 天, 可在任务管理页调整)
	d.JobScheduler.RegisterTask("audit.cleanup", "清理过期的操作审计日志 (参数 retentionDays 为保留天数)", json.RawMessage(`{"retentionDays":90}`), func(_ *model.JobDefinition, params json.RawMessage) error {
		return CleanupAuditLogs(params)
	})
}
