package tasks

import (
	"encoding/json"

	"backend/model"
)

// registerNotifyTasks 通知触达类定时任务
func registerNotifyTasks(d *Deps) {
	if d.Reminder != nil {
		d.JobScheduler.RegisterTask(model.TaskNameReminder, "用户备忘提醒", json.RawMessage(`{"title":"标题","content":"内容"}`), d.Reminder.ReminderTaskHandler)
	}
}
