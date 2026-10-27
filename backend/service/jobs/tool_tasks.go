package tasks

import (
	"encoding/json"

	"backend/model"
)

// registerToolTasks 工具箱类定时任务
func registerToolTasks(d *Deps) {
	// 原神每日签到 (job_definitions 里可改 cron/参数)
	if d.Mihoyo != nil {
		d.JobScheduler.RegisterTask(model.TaskNameMihoyoGenshinCheckin,
			"原神每日签到 (所有已扫码绑定的账号, 失败可邮件告警)",
			json.RawMessage(`{}`),
			func(_ *model.JobDefinition, _ json.RawMessage) error {
				return d.Mihoyo.CheckinAll()
			})
	}
}
