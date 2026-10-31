// Package asynctask 请求触发的通用异步任务 (async_jobs 表, 原 ai_text_jobs):
// "HTTP 请求进来 → 后台协程执行 → 前端拿 job id 轮询结果" 的占位行服务。
// 与 jobs 包的 job_definitions/job_runs (时间驱动调度: cron/once/repeat + 重试告警)
// 职责不同, 互不合并。本包只依赖 DB/model/kernel, 业务包单向引用。
package asynctask

import (
	"errors"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"backend/model"
	"backend/service/kernel"
)

// DB 本包专用数据库句柄, 由 main.go 接线赋值。
var DB *gorm.DB

// 任务类型 (跨包共用, 业务侧 Kind 值必须从这里取)
const (
	KindPassage       = "passage"        // AI 写课文 (结果 = 课程包 ID)
	KindVocabTest     = "vocab_test"     // 词汇量测试出题 (旧版固定 24 题, 已被 vocab_adaptive 取代)
	KindVocabAdaptive = "vocab_adaptive" // 自适应词汇量测试出题 (锚定词库抽样 + 干扰释义生成)
	KindRSSAnnotate   = "rss_annotate"   // RSS 一键生词提取 (结果写回 rss_items)
	KindRSSTranslate  = "rss_translate"  // RSS 中英对照翻译 (结果写回 rss_items)
)

// 任务状态
const (
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"
)

// Runner 任务主体: 返回 (结果ID, 结果JSON, 错误); report 上报进度文本 (轮询可见, 如 "12/30 段")。
// panic 会被捕获并置任务 failed, 不会泄漏协程。
type Runner func(report func(progress string)) (resultID uint, resultJSON string, err error)

// Start 创建任务行 (status=running) 并立即用后台协程执行, 返回任务行供前端轮询。
// targetID 为业务目标 ID (如 RSS 条目), 与 kind/userID 一起用于运行中去重; paramsJSON
// 建议存请求参数快照, 便于排查与人工重跑。
func Start(kind string, userID, targetID uint, paramsJSON, goName string, run Runner) (*model.AsyncJob, error) {
	now := time.Now()
	job := &model.AsyncJob{
		UserID: userID, Kind: kind, Status: StatusRunning,
		ResultID: targetID, ParamsJSON: paramsJSON, StartedAt: &now,
	}
	if err := DB.Create(job).Error; err != nil {
		return nil, err
	}
	kernel.SafeGo("asynctask:"+goName, func() { runJob(job.ID, run) })
	return job, nil
}

// runJob 执行任务主体并回写终态 (自带 panic 保护: SafeGo 只防进程, 这里还要保证任务行不卡 running)
func runJob(id uint, run Runner) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[asynctask] job #%d panic: %v", id, r)
			finishJob(id, 0, "", errors.New("任务执行异常 (panic, 详见后端日志)"))
		}
	}()
	resultID, resultJSON, err := run(func(progress string) {
		if e := DB.Model(&model.AsyncJob{}).Where("id = ?", id).
			Update("progress", kernel.TruncateRunes(progress, 100)).Error; e != nil {
			log.Printf("[asynctask] job #%d 上报进度失败: %v", id, e)
		}
	})
	finishJob(id, resultID, resultJSON, err)
}

func finishJob(id uint, resultID uint, resultJSON string, err error) {
	now := time.Now()
	updates := map[string]interface{}{"finished_at": now}
	if err != nil {
		updates["status"] = StatusFailed
		updates["error"] = kernel.TruncateRunes(err.Error(), 2000)
	} else {
		updates["status"] = StatusDone
		updates["result_id"] = resultID
		updates["result_json"] = resultJSON
		updates["error"] = ""
	}
	if e := DB.Model(&model.AsyncJob{}).Where("id = ?", id).Updates(updates).Error; e != nil {
		log.Printf("[asynctask] job #%d 回写终态失败: %v", id, e)
	}
}

// FindRunning 查同 (user, kind, target) 的进行中任务: 有则前端直接复用轮询, 不重复起任务烧 token。
func FindRunning(kind string, userID, targetID uint) (*model.AsyncJob, error) {
	var job model.AsyncJob
	err := DB.Where("kind = ? AND user_id = ? AND result_id = ? AND status = ?", kind, userID, targetID, StatusRunning).
		Order("id DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// Get 查本人的任务 (owner 校验), 结果按下发脱敏器处理后返回。
func Get(userID, id uint) (*model.AsyncJob, error) {
	var job model.AsyncJob
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&job).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	if fn := lookupRedactor(job.Kind); fn != nil && job.ResultJSON != "" {
		job.ResultJSON = fn(job.ResultJSON)
	}
	return &job, nil
}

// InstantDone 构造一个不落库的即时完成 job (业务侧已有缓存结果时返回, 前端看到 done 就不轮询)。
func InstantDone(kind string, userID, targetID uint) *model.AsyncJob {
	now := time.Now()
	return &model.AsyncJob{
		UserID: userID, Kind: kind, Status: StatusDone, ResultID: targetID,
		StartedAt: &now, FinishedAt: &now, CreatedAt: now,
	}
}

// SweepInterrupted 启动清扫: 进程重启后旧进程的 running 任务永远不会有结果, 统一置 failed。
// 参照 job_scheduler 对 job_runs 的重启清理模式。
func SweepInterrupted() {
	if DB == nil {
		return
	}
	res := DB.Model(&model.AsyncJob{}).Where("status = ?", StatusRunning).Updates(map[string]interface{}{
		"status":      StatusFailed,
		"error":       "服务重启中断, 请重新触发",
		"finished_at": time.Now(),
	})
	if res.Error != nil {
		log.Printf("[asynctask] 重启清扫失败: %v", res.Error)
		return
	}
	if res.RowsAffected > 0 {
		log.Printf("[asynctask] 重启清扫: %d 个中断任务已置 failed", res.RowsAffected)
	}
}

// ===== 结果下发脱敏 =====
// 某些任务的 result_json 含敏感内容 (如 vocab_test 的正确答案), 下发前按 kind 脱敏。
// 脱敏器是业务逻辑, 由 main.go 接线注册, 本包不反向 import 业务包。

var redactorMu sync.RWMutex
var redactors = map[string]func(string) string{}

// RegisterRedactor 注册某 kind 的结果脱敏器 (main.go 装配时调用)
func RegisterRedactor(kind string, fn func(string) string) {
	redactorMu.Lock()
	defer redactorMu.Unlock()
	redactors[kind] = fn
}

func lookupRedactor(kind string) func(string) string {
	redactorMu.RLock()
	defer redactorMu.RUnlock()
	return redactors[kind]
}
