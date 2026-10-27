package tasks

import (
	"encoding/json"
	"fmt"
	"log"

	"backend/model"
)

// registerAITasks AI 相关定时任务: 定时 Agent/编排任务、用户画像抽取、RSS 订阅刷新
func registerAITasks(d *Deps) {
	if d.AgentTask != nil {
		d.JobScheduler.RegisterTask(model.TaskNameAgentTask, "定时运行 Agent/编排 (参数: agent_id/agent_type/input/notify_email/notify_telegram)",
			json.RawMessage(jsonExampleAgentTask), d.AgentTask.AgentTaskHandler)
	}

	// 会话增量抽取用户画像与经历 (依赖 AI, 记账走 tokenusage extraction 来源)
	if d.UserMemory != nil {
		d.JobScheduler.RegisterTask("memory.extract_sessions", "抽取用户画像与经历(会话增量)", json.RawMessage(`{}`),
			func(_ *model.JobDefinition, _ json.RawMessage) error {
				return d.UserMemory.RunExtraction()
			})
	}

	// RSS 订阅自动刷新 (种子任务: 每 30 分钟, 可在任务管理页调整 cron)
	if d.RSS != nil {
		d.JobScheduler.RegisterTask("rss.refresh_all", "刷新全部 RSS 订阅源 (抓取新文章, 并发限流)", json.RawMessage(`{}`), func(_ *model.JobDefinition, _ json.RawMessage) error {
			sources, inserted, failed := d.RSS.RefreshAllSources()
			if sources > 0 && failed == sources {
				return fmt.Errorf("全部 %d 个订阅源抓取失败", sources)
			}
			log.Printf("[rss.refresh_all] 源 %d, 新增 %d 条, 失败 %d", sources, inserted, failed)
			return nil
		})
	}
}

// jsonExampleAgentTask 任务管理页展示的参数示例
const jsonExampleAgentTask = `{"agent_id":1,"agent_type":"chat","input":"总结今天的待办","notify_email":true}`
