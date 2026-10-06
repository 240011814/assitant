package service

import (
	"backend/model"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/robfig/cron/v3"
)

// JobScheduler 调度器: 系统任务(cron) + 用户备忘(once/repeat) 统一调度与执行
type JobScheduler struct {
	scheduler gocron.Scheduler
	mu        sync.Mutex
	tasks     map[string]taskEntry  // 已注册的可调度任务
	cronJobs  map[string]gocron.Job // cron-def:{id} -> job
	alertSvc  *JobAlertService      // 任务失败邮件告警 (可为 nil)
}

func NewJobScheduler(alertSvc *JobAlertService) (*JobScheduler, error) {
	s, err := gocron.NewScheduler()
	if err != nil {
		return nil, err
	}

	js := &JobScheduler{
		scheduler: s,
		tasks:     make(map[string]taskEntry),
		cronJobs:  make(map[string]gocron.Job),
		alertSvc:  alertSvc,
	}

	return js, nil
}

func (js *JobScheduler) Start() {
	js.scheduler.Start()
	log.Println("[JobScheduler] 已启动")
}

func (js *JobScheduler) GetScheduler() gocron.Scheduler {
	return js.scheduler
}

func (js *JobScheduler) Shutdown() error {
	return js.scheduler.Shutdown()
}

// ============ 任务注册表 ============
// 后台可调度的任务必须先注册到 registry, 方法名是注册 key 而非反射任意 Go 方法

// TaskHandler 任务执行体: def 提供定义上下文(如备忘的归属用户)
type TaskHandler func(def *model.JobDefinition, params json.RawMessage) error

type taskEntry struct {
	meta    model.TaskMeta
	handler TaskHandler
}

// RegisterTask 注册可调度任务
func (js *JobScheduler) RegisterTask(name, description string, paramsExample json.RawMessage, handler TaskHandler) {
	js.mu.Lock()
	defer js.mu.Unlock()
	js.tasks[name] = taskEntry{
		meta:    model.TaskMeta{Name: name, Description: description, ParamsExample: paramsExample},
		handler: handler,
	}
	log.Printf("[JobScheduler] 注册任务: %s (%s)", name, description)
}

// ListRegisteredTasks 列出所有已注册任务(供后台下拉选择)
func (js *JobScheduler) ListRegisteredTasks() []model.TaskMeta {
	js.mu.Lock()
	defer js.mu.Unlock()
	list := make([]model.TaskMeta, 0, len(js.tasks))
	for _, entry := range js.tasks {
		list = append(list, entry.meta)
	}
	return list
}

func (js *JobScheduler) getTask(taskName string) (TaskHandler, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	entry, ok := js.tasks[taskName]
	return entry.handler, ok
}

// GetTask 查询任务是否已注册
func (js *JobScheduler) GetTask(taskName string) (model.TaskMeta, bool) {
	js.mu.Lock()
	defer js.mu.Unlock()
	entry, ok := js.tasks[taskName]
	return entry.meta, ok
}

// ============ 定义调度 ============

// LoadCronDefinitions 启动时加载启用的任务定义并注册到调度器
func (js *JobScheduler) LoadCronDefinitions() error {
	// 清理上次进程中断遗留的 running 记录
	if err := DB.Model(&model.JobRun{}).Where("status = ?", model.JobRunStatusRunning).Updates(map[string]interface{}{
		"status":      model.JobRunStatusFailed,
		"error":       "服务重启导致任务中断",
		"finished_at": time.Now(),
	}).Error; err != nil {
		log.Printf("[JobScheduler] 清理中断的定时任务记录失败: %v", err)
	}

	var defs []model.JobDefinition
	if err := DB.Where("enabled = ?", true).Find(&defs).Error; err != nil {
		return err
	}
	for i := range defs {
		if err := js.ScheduleDefinition(&defs[i]); err != nil {
			log.Printf("[JobScheduler] 加载任务 %s 失败: %v", defs[i].Name, err)
		}
	}
	log.Printf("[JobScheduler] 已加载 %d 个启用的任务定义", len(defs))
	return nil
}

// ValidateCronExpr 校验 cron 表达式(5段标准格式), 并返回下一次执行时间
func ValidateCronExpr(expr string) (time.Time, error) {
	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)
	schedule, err := parser.Parse(expr)
	if err != nil {
		return time.Time{}, fmt.Errorf("cron 表达式无效: %v", err)
	}
	return schedule.Next(time.Now()), nil
}

// ScheduleDefinition 注册/更新任务定义的调度 (新增或编辑后调用)
func (js *JobScheduler) ScheduleDefinition(def *model.JobDefinition) error {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.scheduleDefinitionLocked(def)
}

func (js *JobScheduler) scheduleDefinitionLocked(def *model.JobDefinition) error {
	key := defKey(def.ID)
	if existing, ok := js.cronJobs[key]; ok {
		_ = js.scheduler.RemoveJob(existing.ID())
		delete(js.cronJobs, key)
	}

	if !def.Enabled {
		return nil
	}

	switch def.ScheduleType {
	case model.ScheduleTypeCron:
		return js.scheduleCron(def, key)
	case model.ScheduleTypeOnce, model.ScheduleTypeRepeat:
		return js.scheduleAtTime(def, key)
	default:
		return fmt.Errorf("未知调度类型: %s", def.ScheduleType)
	}
}

func defKey(defID uint) string {
	return fmt.Sprintf("cron-def:%d", defID)
}

func (js *JobScheduler) scheduleCron(def *model.JobDefinition, key string) error {
	job, err := js.scheduler.NewJob(
		gocron.CronJob(def.CronExpr, false),
		gocron.NewTask(js.cronTick, def.ID),
		gocron.WithName(key),
		gocron.WithTags(key),
	)
	if err != nil {
		return fmt.Errorf("注册 cron 任务失败: %v", err)
	}
	js.cronJobs[key] = job
	log.Printf("[JobScheduler] 已注册定时任务: %s (%s) [%s]", def.Name, def.TaskName, def.CronExpr)
	return nil
}

// scheduleAtTime once/repeat: 按 run_at(减去提前分钟数)注册一次性任务
// repeat 到期后由执行引擎生成下一次的新定义行; 进程重启由 LoadCronDefinitions 恢复
func (js *JobScheduler) scheduleAtTime(def *model.JobDefinition, key string) error {
	if def.RunAt == nil {
		return fmt.Errorf("任务 %s 缺少执行时间", def.Name)
	}
	now := time.Now()
	effectiveAt := def.RunAt.Add(-time.Duration(def.AdvanceMinutes) * time.Minute)

	if effectiveAt.Before(now) {
		// 过期不补发 (旧 skipExpiredJob 语义: 重复任务基于原时间生成下一次, 一次性任务直接过期)
		DB.Model(&model.JobDefinition{}).Where("id = ?", def.ID).Update("enabled", false)
		def.Enabled = false
		recordSkippedRun(def.ID, "执行时间已过期, 跳过执行")
		log.Printf("[JobScheduler] 任务 %s 已过期, 跳过执行", def.Name)

		if def.ScheduleType == model.ScheduleTypeRepeat {
			js.ensureNextOccurrenceLocked(def)
		}
		return nil
	}

	job, err := js.scheduler.NewJob(
		gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(effectiveAt)),
		gocron.NewTask(js.cronTick, def.ID),
		gocron.WithName(key),
		gocron.WithTags(key),
	)
	if err != nil {
		return fmt.Errorf("注册定时任务失败: %v", err)
	}
	js.cronJobs[key] = job
	log.Printf("[JobScheduler] 已注册定时任务: %s (%s), %v 后执行", def.Name, def.TaskName, time.Until(effectiveAt))
	return nil
}

// UnscheduleDefinition 移除任务定义的调度 (停用或删除后调用)
func (js *JobScheduler) UnscheduleDefinition(defID uint) {
	js.mu.Lock()
	defer js.mu.Unlock()

	key := defKey(defID)
	if existing, ok := js.cronJobs[key]; ok {
		_ = js.scheduler.RemoveJob(existing.ID())
		delete(js.cronJobs, key)
	}
}

// NextRunAt 查询定义的下一次执行时间(未启用或未注册返回零值)
func (js *JobScheduler) NextRunAt(defID uint) *time.Time {
	js.mu.Lock()
	defer js.mu.Unlock()

	job, ok := js.cronJobs[defKey(defID)]
	if !ok {
		return nil
	}
	next, err := job.NextRun()
	if err != nil || next.IsZero() {
		return nil
	}
	return &next
}

// cronTick 调度器到点触发
func (js *JobScheduler) cronTick(defID uint) {
	js.runDefinition(defID, model.JobTriggerScheduler, 1)
}

// TriggerDefinition 手动立即执行 (异步, 返回是否成功启动)
func (js *JobScheduler) TriggerDefinition(defID uint) bool {
	var def model.JobDefinition
	if err := DB.First(&def, defID).Error; err != nil {
		log.Printf("[JobScheduler] 手动触发失败: 定义 %d 不存在", defID)
		return false
	}
	go js.runDefinition(defID, model.JobTriggerManual, 1)
	return true
}

// ============ 执行引擎 ============
// 手动触发与调度触发走同一入口, 区分 trigger_type

// cronRunning 正在执行的定义(单实例内存态, 防止 cron/手动重入)
var cronRunning = struct {
	sync.Mutex
	ids map[uint]bool
}{ids: make(map[uint]bool)}

func acquireDefinition(defID uint) bool {
	cronRunning.Lock()
	defer cronRunning.Unlock()
	if cronRunning.ids[defID] {
		return false
	}
	cronRunning.ids[defID] = true
	return true
}

func releaseDefinition(defID uint) {
	cronRunning.Lock()
	defer cronRunning.Unlock()
	delete(cronRunning.ids, defID)
}

func recordSkippedRun(defID uint, reason string) {
	now := time.Now()
	DB.Create(&model.JobRun{
		Definition:  defID,
		Attempt:     1,
		Status:      model.JobRunStatusSkipped,
		TriggerType: model.JobTriggerScheduler,
		StartedAt:   &now,
		FinishedAt:  &now,
		Error:       reason,
	})
}

func (js *JobScheduler) runDefinition(defID uint, triggerType string, attempt int) {
	if !acquireDefinition(defID) {
		if triggerType == model.JobTriggerScheduler {
			// 上一轮还在执行, 记录一次跳过
			recordSkippedRun(defID, "上一轮执行尚未结束, 本次跳过")
			log.Printf("[JobScheduler] 定时任务 %d 上一轮未结束, 跳过本轮", defID)
			// repeat 链不能断: 跳过本次仍需推进到下一次
			var def model.JobDefinition
			if err := DB.First(&def, defID).Error; err == nil {
				js.advanceScheduleAfterRun(&def, triggerType, attempt)
			}
		}
		return
	}
	defer releaseDefinition(defID)

	var def model.JobDefinition
	if err := DB.First(&def, defID).Error; err != nil {
		log.Printf("[JobScheduler] 定时任务 %d 定义不存在", defID)
		return
	}

	handler, ok := js.getTask(def.TaskName)
	now := time.Now()
	run := model.JobRun{
		Definition:  def.ID,
		Attempt:     attempt,
		Status:      model.JobRunStatusRunning,
		TriggerType: triggerType,
		StartedAt:   &now,
	}
	if !ok {
		finishAt := time.Now()
		run.Status = model.JobRunStatusFailed
		run.FinishedAt = &finishAt
		run.Error = fmt.Sprintf("任务方法未注册: %s", def.TaskName)
		DB.Create(&run)
		log.Printf("[JobScheduler] 定时任务 %s 执行失败: %s", def.Name, run.Error)
		js.alertSvc.NotifyJobFailed(&def, run.Error, attempt, triggerType, finishAt)
		return
	}
	DB.Create(&run)

	log.Printf("[JobScheduler] 执行定时任务: %s (%s) 第%d次尝试", def.Name, def.TaskName, attempt)

	// panic 保护, 避免协程崩溃拖垮进程
	err := func() (cbErr error) {
		defer func() {
			if r := recover(); r != nil {
				cbErr = fmt.Errorf("任务执行 panic: %v", r)
			}
		}()
		return handler(&def, def.Params)
	}()

	finishAt := time.Now()
	run.FinishedAt = &finishAt
	if err != nil {
		// 任务互斥冲突 (如另一同步任务正在运行) 属于正常跳过: 记 skipped, 不重试不告警
		if errors.Is(err, ErrSyncBusy) {
			run.Status = model.JobRunStatusSkipped
			run.Error = err.Error()
			DB.Model(&model.JobRun{}).Where("id = ?", run.ID).Updates(map[string]interface{}{
				"status":      run.Status,
				"finished_at": finishAt,
				"error":       run.Error,
			})
			log.Printf("[JobScheduler] 定时任务 %s 跳过: %v", def.Name, err)
			return
		}

		run.Status = model.JobRunStatusFailed
		run.Error = err.Error()
		DB.Model(&model.JobRun{}).Where("id = ?", run.ID).Updates(map[string]interface{}{
			"status":      run.Status,
			"finished_at": finishAt,
			"error":       run.Error,
		})
		log.Printf("[JobScheduler] 定时任务 %s 执行失败: %v", def.Name, err)

		// 失败重试: 延迟 attempt 分钟后单次调度
		willRetry := false
		if attempt <= def.MaxRetries {
			willRetry = true
			retryAt := time.Now().Add(time.Duration(attempt) * time.Minute)
			if _, err := js.scheduler.NewJob(
				gocron.OneTimeJob(gocron.OneTimeJobStartDateTime(retryAt)),
				gocron.NewTask(js.runDefinitionRetry, def.ID, triggerType, attempt+1),
				gocron.WithName(fmt.Sprintf("cron-def-retry:%d:%d", def.ID, attempt)),
				gocron.WithTags(fmt.Sprintf("cron-def-retry:%d", def.ID)),
			); err != nil {
				log.Printf("[JobScheduler] 定时任务 %s 重试调度失败: %v", def.Name, err)
			} else {
				log.Printf("[JobScheduler] 定时任务 %s 将于 %v 重试(第%d次)", def.Name, retryAt, attempt+1)
			}
		}
		// 重试耗尽仍失败: 定义配置了通知邮箱则发送告警 (NotifyJobFailed 内部忽略空邮箱/nil 服务)
		if !willRetry {
			js.alertSvc.NotifyJobFailed(&def, run.Error, attempt, triggerType, finishAt)
		}
		// 只有首次调度触发才推进生命周期, 重试不推进
		if attempt == 1 {
			js.advanceScheduleAfterRun(&def, triggerType, attempt)
		}
		return
	}

	run.Status = model.JobRunStatusSuccess
	DB.Model(&model.JobRun{}).Where("id = ?", run.ID).Updates(map[string]interface{}{
		"status":      run.Status,
		"finished_at": finishAt,
	})
	log.Printf("[JobScheduler] 定时任务 %s 执行成功", def.Name)

	if attempt == 1 {
		js.advanceScheduleAfterRun(&def, triggerType, attempt)
	}
}

// advanceScheduleAfterRun 调度触发执行完后推进 once/repeat 定义的生命周期 (手动触发与重试不推进)
func (js *JobScheduler) advanceScheduleAfterRun(def *model.JobDefinition, triggerType string, attempt int) {
	if triggerType != model.JobTriggerScheduler || attempt != 1 {
		return
	}
	switch def.ScheduleType {
	case model.ScheduleTypeOnce:
		// 一次性任务执行完即结束(无论成败, 失败重试已在上方调度)
		DB.Model(&model.JobDefinition{}).Where("id = ?", def.ID).Update("enabled", false)
		def.Enabled = false
		js.UnscheduleDefinition(def.ID)
	case model.ScheduleTypeRepeat:
		// 当前次执行完成, 生成下一次执行的新任务行 (链式: 每次执行 = 一条任务)
		DB.Model(&model.JobDefinition{}).Where("id = ?", def.ID).Update("enabled", false)
		def.Enabled = false
		js.UnscheduleDefinition(def.ID)
		if next := js.EnsureNextOccurrence(def); next != nil {
			log.Printf("[JobScheduler] 已生成下一次任务: %s, 执行时间: %v", next.Name, *next.RunAt)
		} else {
			log.Printf("[JobScheduler] 任务 %s 重复周期已结束", def.Name)
		}
	}
}

// EnsureNextOccurrence 为 repeat 定义生成下一次执行的新任务行 (同 chain_id)
// 链上已有未来待执行任务时不生成, 防止重复 (旧 createNextRepeat 的防重护栏)
// 返回新任务行, nil 表示无下一次 (非重复/周期结束/已有待执行)
func (js *JobScheduler) EnsureNextOccurrence(def *model.JobDefinition) *model.JobDefinition {
	js.mu.Lock()
	defer js.mu.Unlock()
	return js.ensureNextOccurrenceLocked(def)
}

func (js *JobScheduler) ensureNextOccurrenceLocked(def *model.JobDefinition) *model.JobDefinition {
	if def.RepeatType == "" || def.RepeatType == "none" || def.RunAt == nil {
		return nil
	}

	chainID := def.ChainID
	if chainID == 0 {
		chainID = def.ID
	}

	// 同链已有未来待执行任务时不再创建, 防止重复生成
	var pendingCount int64
	DB.Model(&model.JobDefinition{}).
		Where("chain_id = ? AND enabled = ? AND run_at > ? AND id <> ?", chainID, true, time.Now(), def.ID).
		Count(&pendingCount)
	if pendingCount > 0 {
		return nil
	}

	// 过期不补发: 从当前锚点推进到未来执行点, 只生成未来那一条
	next := calcNextTime(*def.RunAt, def.RepeatType, def.RepeatInterval)
	maxIter := 100
	for i := 0; !next.IsZero() && next.Before(time.Now()) && i < maxIter; i++ {
		next = calcNextTime(next, def.RepeatType, def.RepeatInterval)
	}
	if next.IsZero() || (def.RepeatEndAt != nil && next.After(*def.RepeatEndAt)) {
		return nil
	}

	row := model.JobDefinition{
		ChainID:        chainID,
		UserID:         def.UserID,
		Name:           fmt.Sprintf("%s-%s", truncateRunes(def.Name, 84), next.Format("20060102150405")),
		TaskName:       def.TaskName,
		ScheduleType:   def.ScheduleType,
		RunAt:          &next,
		AdvanceMinutes: def.AdvanceMinutes,
		RepeatType:     def.RepeatType,
		RepeatInterval: def.RepeatInterval,
		RepeatEndAt:    def.RepeatEndAt,
		Params:         def.Params,
		Enabled:        true,
		MaxRetries:     def.MaxRetries,
		NotifyEmail:    def.NotifyEmail,
		Remark:         def.Remark,
		CreatedBy:      def.CreatedBy,
	}
	if err := DB.Create(&row).Error; err != nil {
		log.Printf("[JobScheduler] 生成下一次任务失败: %v", err)
		return nil
	}
	if err := js.scheduleDefinitionLocked(&row); err != nil {
		log.Printf("[JobScheduler] 调度下一次任务失败: %v", err)
	}
	return &row
}

func truncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// runDefinitionRetry 重试入口
func (js *JobScheduler) runDefinitionRetry(defID uint, triggerType string, attempt int) {
	js.runDefinition(defID, triggerType, attempt)
}

func calcNextTime(at time.Time, repeatType string, interval int) time.Time {
	switch repeatType {
	case "daily":
		return at.AddDate(0, 0, interval)
	case "weekly":
		return at.AddDate(0, 0, 7*interval)
	case "monthly":
		return addMonthsSameDay(at, interval)
	case "yearly":
		return addMonthsSameDay(at, 12*interval)
	default:
		return time.Time{}
	}
}

// addMonthsSameDay 按月推进并保持"日"不变; 目标月份没有该日（如 1月31日、2月29日）时跳到下一个月的同一天
func addMonthsSameDay(at time.Time, months int) time.Time {
	next := at.AddDate(0, months, 0)
	// AddDate 会把溢出的日归一化到下个月（如 1月31日+1月=3月2日），此处检测并跳到下一个存在该日的月份
	for i := 0; next.Day() != at.Day() && i < 12; i++ {
		months++
		next = at.AddDate(0, months, 0)
	}
	return next
}
