// 后台定时任务的统一注册入口 (tasks_*.go): 全部 job_definitions 可调度任务的
// RegisterTask 调用收拢在本包 (按域一个文件), 各业务 service 只保留任务执行逻辑,
// 不再自带注册代码; main.go 装配后调用 jobs.RegisterAll 一行完成接线。
// agenttask/reminder 依赖本包的 JobScheduler, 因此 Deps 对它们只收 TaskHandler
// (绑方法由 main 传入), 避免 jobs ↔ 业务包循环引用。
package jobs

import (
	"backend/service/ai/usermemory"
	"backend/service/learning"
	"backend/service/stock"
	"backend/service/tool/mihoyo"
)

// Deps 任务执行依赖, 由 main.go 装配后传入。
// 除 TaskHandler 外均为具体 service 实例 (这些包不反向依赖本包, 无环)。
type Deps struct {
	JobScheduler     *JobScheduler
	StockSync        *stock.StockSyncService
	StockCh          *stock.StockChSyncService
	StockAlert       *stock.StockAlertService
	AgentTaskHandler TaskHandler // agenttask.AgentTaskService.AgentTaskHandler
	UserMemory       *usermemory.UserMemoryService
	RSS              *learning.RSSService
	ReminderHandler  TaskHandler // reminder.ReminderService.ReminderTaskHandler
	Mihoyo           *mihoyo.MihoyoService
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
