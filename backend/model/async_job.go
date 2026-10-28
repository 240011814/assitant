package model

import (
	"time"
)

// AsyncJob 通用异步任务 (原 ai_text_jobs): 请求触发的慢 LLM 任务的占位行,
// 前端拿 job id 轮询 /api/async-jobs/:id 取结果。存在原因是 Cloudflare 对源站
// 响应有 100s 上限, 分钟级的 LLM 生成不能同步等。与 jobs 包的 job_definitions/
// job_runs (时间驱动调度) 职责不同, 不合并。
type AsyncJob struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID     uint       `gorm:"not null;index:idx_async_job_user,priority:1" json:"userId"`
	Kind       string     `gorm:"size:50;not null;index:idx_async_job_kind,priority:1" json:"kind"`                     // passage / vocab_test / rss_annotate / rss_translate
	Status     string     `gorm:"size:20;not null;default:'running';index:idx_async_job_kind,priority:2" json:"status"` // running/done/failed
	ParamsJSON string     `gorm:"type:longtext" json:"-"`                                                               // 创建参数快照 (排查/重跑依据)
	ResultID   uint       `gorm:"not null;default:0" json:"resultId"`                                                   // 完成后的结果 ID (课程包/RSS 条目)
	ResultJSON string     `gorm:"type:longtext" json:"resultJson,omitempty"`                                            // 任务结果 JSON (词汇量测试题面等)
	Progress   string     `gorm:"size:100" json:"progress,omitempty"`                                                   // 进度文本 (如 "12/30 段")
	Error      string     `gorm:"type:text" json:"error,omitempty"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
	CreatedAt  time.Time  `gorm:"index:idx_async_job_user,priority:2" json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// TableName 指定表名
func (AsyncJob) TableName() string {
	return "async_jobs"
}
