// Package tasks 后台定时任务的统一注册入口: 全部 job_definitions 可调度任务的
// RegisterTask 调用收拢在本包 (按域一个文件), 各业务 service 只保留任务执行逻辑,
// 不再自带注册代码; main.go 装配后调用 tasks.RegisterAll 一行完成接线。
package tasks

import (
	"backend/service/ai/agenttask"
	"backend/service/ai/usermemory"
	"backend/service/jobs"
	"backend/service/learning"
	"backend/service/notify/reminder"
	"backend/service/stock"
	"backend/service/tool/mihoyo"
)

// Deps 任务执行依赖的具体 service 实例, 由 main.go 装配后传入
type Deps struct {
	JobScheduler *jobs.JobScheduler
	StockSync    *stock.StockSyncService
	StockCh      *stock.StockChSyncService
	StockAlert   *stock.StockAlertService
	AgentTask    *agenttask.AgentTaskService
	UserMemory   *usermemory.UserMemoryService
	RSS          *learning.RSSService
	Reminder     *reminder.ReminderService
	Mihoyo       *mihoyo.MihoyoService
}

// RegisterAll 注册全部定时任务。JobScheduler 为 nil 时跳过 (调度器创建失败仅告警不阻断启动)。
func RegisterAll(d *Deps) {
	if d == nil || d.JobScheduler == nil {
		return
	}
	registerStockTasks(d)
	registerAITasks(d)
	registerNotifyTasks(d)
	registerToolTasks(d)
	registerSysTasks(d)
}
