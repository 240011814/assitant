package model

import (
	"encoding/json"
	"time"
)

// Job 备忘视图 DTO: 由 job_definitions/job_runs 映射而来, 供日历与 AI 工具使用 (不对应数据库表)
// ID 为有符号: 正数=定义, 负数=执行历史(job_runs)
type Job struct {
	ID             int64           `json:"id"`
	JobType        string          `json:"jobType"`
	JobID          int64           `json:"jobId"`
	UserID         *uint           `json:"userId"`
	ScheduledAt    time.Time       `json:"scheduledAt"`
	AdvanceMinutes int             `json:"advanceMinutes"`
	Status         string          `json:"status"`
	RetryCount     int             `json:"retryCount"`
	MaxRetries     int             `json:"maxRetries"`
	LastError      string          `json:"lastError"`
	Params         json.RawMessage `json:"params"`
	RepeatType     string          `json:"repeatType"`
	RepeatInterval int             `json:"repeatInterval"`
	RepeatEndAt    *time.Time      `json:"repeatEndAt"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

const (
	JobStatusPending   = "pending"
	JobStatusRunning   = "running"
	JobStatusCompleted = "completed"
	JobStatusFailed    = "failed"
)

type ReminderParams struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

type CreateReminderRequest struct {
	Title          string     `json:"title" binding:"required"`
	Content        string     `json:"content"`
	RemindAt       time.Time  `json:"remindAt" binding:"required"`
	AdvanceMinutes int        `json:"advanceMinutes"`
	RepeatType     string     `json:"repeatType"`
	RepeatInterval int        `json:"repeatInterval"`
	RepeatEndAt    *time.Time `json:"repeatEndAt"`
}

type UpdateReminderRequest struct {
	Title          string     `json:"title"`
	Content        string     `json:"content"`
	RemindAt       time.Time  `json:"remindAt"`
	AdvanceMinutes *int       `json:"advanceMinutes"`
	RepeatType     string     `json:"repeatType"`
	RepeatInterval int        `json:"repeatInterval"`
	RepeatEndAt    *time.Time `json:"repeatEndAt"`
	// RepeatEndAtClear 为 true 时显式清除重复结束时间。
	// JSON 里"字段缺失"与"传 null"反序列化后都是 nil, 无法区分;
	// 不带该标记的请求 (如仅改标题) 不会清空已有的 repeat_end_at
	RepeatEndAtClear bool `json:"repeatEndAtClear"`
}
