package service

import (
	"backend/model"
	iface "backend/interface"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"
)

// ReminderService 用户备忘
// 存储于 job_definitions (task_name=reminder.notify, user_id 归属)
// 链式重复: 每条定义 = 一次执行, 重复任务执行完后由调度器生成下一条 (chain_id 分组), 与旧 jobs 表逻辑一致
type ReminderService struct {
	jobScheduler *JobScheduler
	notifiers    []iface.Notifier
}

func NewReminderService(jobScheduler *JobScheduler, notifiers ...iface.Notifier) *ReminderService {
	s := &ReminderService{
		jobScheduler: jobScheduler,
		notifiers:    notifiers,
	}
	// jobScheduler 可能为 nil (调度器创建失败时仅告警不阻断启动), 此时跳过任务注册
	if jobScheduler != nil {
		jobScheduler.RegisterTask(model.TaskNameReminder, "用户备忘提醒", json.RawMessage(`{"title":"标题","content":"内容"}`), s.reminderTaskHandler)
	}
	return s
}

// defToJob 定义 -> 备忘视图 (enabled=false 即已执行的 completed)
func defToJob(def *model.JobDefinition) *model.Job {
	scheduledAt := time.Time{}
	if def.RunAt != nil {
		scheduledAt = *def.RunAt
	}
	repeatType := def.RepeatType
	if repeatType == "" {
		repeatType = "none"
	}
	status := model.JobStatusPending
	if !def.Enabled {
		status = model.JobStatusCompleted
	}
	return &model.Job{
		ID:             int64(def.ID),
		JobType:        "reminder",
		JobID:          int64(def.ChainID),
		UserID:         def.UserID,
		ScheduledAt:    scheduledAt,
		AdvanceMinutes: def.AdvanceMinutes,
		Status:         status,
		MaxRetries:     def.MaxRetries,
		Params:         def.Params,
		RepeatType:     repeatType,
		RepeatInterval: def.RepeatInterval,
		RepeatEndAt:    def.RepeatEndAt,
		CreatedAt:      def.CreatedAt,
		UpdatedAt:      def.UpdatedAt,
	}
}

func reminderScheduleType(repeatType string) string {
	if repeatType == "" || repeatType == "none" {
		return model.ScheduleTypeOnce
	}
	return model.ScheduleTypeRepeat
}

func reminderDefName(title string) string {
	cut := title
	if len([]rune(cut)) > 30 {
		cut = string([]rune(cut)[:30])
	}
	return fmt.Sprintf("备忘-%s-%s", cut, time.Now().Format("20060102150405.000"))
}

// ListByMonth 月内全部备忘条目 (每条定义 = 一次执行, 已执行的为 completed)
func (s *ReminderService) ListByMonth(userID uint, year int, month time.Month) ([]model.Job, error) {
	start := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
	end := start.AddDate(0, 1, 0)

	var defs []model.JobDefinition
	if err := DB.Where("user_id = ? AND task_name = ? AND run_at >= ? AND run_at < ?",
		userID, model.TaskNameReminder, start, end).
		Order("run_at ASC").
		Find(&defs).Error; err != nil {
		return nil, err
	}

	result := make([]model.Job, 0, len(defs))
	for i := range defs {
		result = append(result, *defToJob(&defs[i]))
	}
	return result, nil
}

func (s *ReminderService) GetByID(userID, id uint) (*model.Job, error) {
	var def model.JobDefinition
	err := DB.Where("user_id = ? AND id = ? AND task_name = ?", userID, id, model.TaskNameReminder).First(&def).Error
	if err != nil {
		return nil, errors.New("备忘不存在")
	}
	return defToJob(&def), nil
}

func (s *ReminderService) Create(userID uint, req model.CreateReminderRequest) (*model.Job, error) {
	repeatType := req.RepeatType
	if repeatType == "" {
		repeatType = "none"
	}
	repeatInterval := req.RepeatInterval
	if repeatInterval <= 0 {
		repeatInterval = 1
	}
	advanceMinutes := req.AdvanceMinutes
	if advanceMinutes < 0 {
		advanceMinutes = 0
	}

	params := model.ReminderParams{
		Title:   req.Title,
		Content: req.Content,
	}
	paramsJSON, _ := json.Marshal(params)

	def := model.JobDefinition{
		UserID:         &userID,
		Name:           reminderDefName(req.Title),
		TaskName:       model.TaskNameReminder,
		ScheduleType:   reminderScheduleType(repeatType),
		RunAt:          &req.RemindAt,
		AdvanceMinutes: advanceMinutes,
		RepeatType:     repeatType,
		RepeatInterval: repeatInterval,
		RepeatEndAt:    req.RepeatEndAt,
		Params:         paramsJSON,
		Enabled:        true,
		MaxRetries:     3,
	}

	if err := DB.Create(&def).Error; err != nil {
		return nil, errors.New("创建备忘失败: " + err.Error())
	}
	// 链标识 = 首条定义自身 ID
	if err := DB.Model(&def).Update("chain_id", def.ID).Error; err != nil {
		return nil, errors.New("创建备忘失败: " + err.Error())
	}
	def.ChainID = def.ID

	if err := s.jobScheduler.ScheduleDefinition(&def); err != nil {
		log.Printf("[ReminderService] 调度备忘 %d 失败: %v", def.ID, err)
	}

	return defToJob(&def), nil
}

func (s *ReminderService) Update(userID, id uint, req model.UpdateReminderRequest) (*model.Job, error) {
	var def model.JobDefinition
	if err := DB.Where("user_id = ? AND id = ? AND task_name = ?", userID, id, model.TaskNameReminder).First(&def).Error; err != nil {
		return nil, errors.New("备忘不存在")
	}

	var params model.ReminderParams
	if err := json.Unmarshal(def.Params, &params); err != nil {
		return nil, errors.New("解析参数失败")
	}
	if req.Title != "" {
		params.Title = req.Title
	}
	if req.Content != "" {
		params.Content = req.Content
	}
	paramsJSON, _ := json.Marshal(params)

	updates := map[string]interface{}{
		"params": paramsJSON,
	}
	if !req.RemindAt.IsZero() {
		updates["run_at"] = req.RemindAt
	}
	if req.AdvanceMinutes != nil {
		advanceMinutes := *req.AdvanceMinutes
		if advanceMinutes < 0 {
			advanceMinutes = 0
		}
		updates["advance_minutes"] = advanceMinutes
	}
	if req.RepeatType != "" {
		updates["repeat_type"] = req.RepeatType
		updates["schedule_type"] = reminderScheduleType(req.RepeatType)
	}
	if req.RepeatInterval > 0 {
		updates["repeat_interval"] = req.RepeatInterval
	}
	// 仅显式传入/显式清除时才更新, 防止只改标题的请求把 repeat_end_at 清成 NULL (重复任务变无限重复)
	if req.RepeatEndAt != nil {
		updates["repeat_end_at"] = req.RepeatEndAt
	} else if req.RepeatEndAtClear {
		updates["repeat_end_at"] = nil
	}
	// 状态不重置 (旧逻辑: 已通知的任务不重置状态, 避免编辑后重复通知)

	if err := DB.Model(&def).Updates(updates).Error; err != nil {
		return nil, errors.New("更新备忘失败: " + err.Error())
	}
	DB.First(&def, def.ID)

	// 已执行的任务: 仅同步内容到链上待执行任务, 不重新调度;
	// 若链上已无未来待执行任务且配置了重复, 生成下一次 (等价旧 skipExpiredJob 语义)
	if !def.Enabled {
		s.syncChainJobs(userID, &def, paramsJSON, req)
		s.jobScheduler.EnsureNextOccurrence(&def)
		return defToJob(&def), nil
	}

	s.jobScheduler.UnscheduleDefinition(def.ID)

	// 编辑为过期时间: 先清理链上其它待执行任务, 本次不补发, 基于新时间生成下一次
	if def.RunAt != nil && def.RunAt.Before(time.Now()) {
		s.deletePendingChainJobs(&def)
		s.jobScheduler.EnsureNextOccurrence(&def)
		return defToJob(&def), nil
	}

	// 同步更新到重复链上的其它待执行任务, 并重新调度本条
	s.syncChainJobs(userID, &def, paramsJSON, req)
	if err := s.jobScheduler.ScheduleDefinition(&def); err != nil {
		log.Printf("[ReminderService] 重新调度备忘 %d 失败: %v", def.ID, err)
	}

	return defToJob(&def), nil
}

// deletePendingChainJobs 删除同链上除本条外的所有待执行任务
func (s *ReminderService) deletePendingChainJobs(def *model.JobDefinition) {
	if def.ChainID == 0 {
		return
	}
	var rows []model.JobDefinition
	if err := DB.Where("user_id = ? AND task_name = ? AND chain_id = ? AND enabled = ? AND id <> ?",
		def.UserID, model.TaskNameReminder, def.ChainID, true, def.ID).Find(&rows).Error; err != nil {
		return
	}
	for i := range rows {
		s.jobScheduler.UnscheduleDefinition(rows[i].ID)
		DB.Delete(&rows[i])
		DB.Where("definition_id = ?", rows[i].ID).Delete(&model.JobRun{})
	}
}

// syncChainJobs 将更新同步到同链的其它待执行任务:
// 标题/内容/重复配置全量同步; 若修改了提醒时间, 其它任务保留各自日期、仅同步时刻
func (s *ReminderService) syncChainJobs(userID uint, def *model.JobDefinition, paramsJSON []byte, req model.UpdateReminderRequest) {
	if def.ChainID == 0 {
		return
	}
	var rows []model.JobDefinition
	if err := DB.Where("user_id = ? AND task_name = ? AND chain_id = ? AND enabled = ? AND id <> ?",
		userID, model.TaskNameReminder, def.ChainID, true, def.ID).Find(&rows).Error; err != nil {
		return
	}

	for i := range rows {
		row := &rows[i]
		updates := map[string]interface{}{
			"params": paramsJSON,
		}
		if req.AdvanceMinutes != nil {
			advanceMinutes := *req.AdvanceMinutes
			if advanceMinutes < 0 {
				advanceMinutes = 0
			}
			updates["advance_minutes"] = advanceMinutes
		}
		if req.RepeatType != "" {
			updates["repeat_type"] = req.RepeatType
		}
		if req.RepeatInterval > 0 {
			updates["repeat_interval"] = req.RepeatInterval
		}
		// 与本条更新语义一致: 未传不清空, 显式清除才置 NULL
		if req.RepeatEndAt != nil {
			updates["repeat_end_at"] = req.RepeatEndAt
		} else if req.RepeatEndAtClear {
			updates["repeat_end_at"] = nil
		}

		if !req.RemindAt.IsZero() && row.RunAt != nil {
			t := req.RemindAt.In(row.RunAt.Location())
			updates["run_at"] = time.Date(
				row.RunAt.Year(), row.RunAt.Month(), row.RunAt.Day(),
				t.Hour(), t.Minute(), t.Second(), 0, row.RunAt.Location(),
			)
		}

		if err := DB.Model(row).Updates(updates).Error; err != nil {
			log.Printf("[ReminderService] 同步重复链任务 %d 失败: %v", row.ID, err)
			continue
		}
		DB.First(row, row.ID)

		s.jobScheduler.UnscheduleDefinition(row.ID)

		// 同步后超出重复结束时间的任务直接删除
		if row.RepeatEndAt != nil && row.RunAt != nil && row.RunAt.After(*row.RepeatEndAt) {
			DB.Delete(row)
			DB.Where("definition_id = ?", row.ID).Delete(&model.JobRun{})
			continue
		}

		if err := s.jobScheduler.ScheduleDefinition(row); err != nil {
			log.Printf("[ReminderService] 重新调度重复链任务 %d 失败: %v", row.ID, err)
		}
	}
}

// Delete 删除备忘
// scope: "this" 仅删除当前条目; "all" 删除同链(chain_id)全部条目
func (s *ReminderService) Delete(userID uint, id int64, scope string) error {
	var def model.JobDefinition
	if err := DB.Where("user_id = ? AND id = ? AND task_name = ?", userID, id, model.TaskNameReminder).First(&def).Error; err != nil {
		return errors.New("备忘不存在")
	}

	if scope == "all" && def.ChainID != 0 {
		var rows []model.JobDefinition
		if err := DB.Where("user_id = ? AND task_name = ? AND chain_id = ?", userID, model.TaskNameReminder, def.ChainID).Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			if err := s.deleteDefinition(rows[i].ID); err != nil {
				return err
			}
		}
		return nil
	}

	return s.deleteDefinition(def.ID)
}

func (s *ReminderService) deleteDefinition(defID uint) error {
	s.jobScheduler.UnscheduleDefinition(defID)
	if err := DB.Delete(&model.JobDefinition{}, defID).Error; err != nil {
		return err
	}
	DB.Where("definition_id = ?", defID).Delete(&model.JobRun{})
	return nil
}

// reminderTaskHandler 备忘任务执行体: 按用户通知偏好发送
func (s *ReminderService) reminderTaskHandler(def *model.JobDefinition, paramsRaw json.RawMessage) error {
	var params model.ReminderParams
	if err := json.Unmarshal(paramsRaw, &params); err != nil {
		return fmt.Errorf("解析参数失败: %v", err)
	}

	log.Printf("[ReminderService] 执行备忘任务: %s (def=%d)", params.Title, def.ID)

	if def.UserID == nil {
		return fmt.Errorf("用户ID为空")
	}

	var user model.User
	if err := DB.First(&user, *def.UserID).Error; err != nil {
		return fmt.Errorf("用户不存在: %v", err)
	}

	// 获取用户通知偏好
	channels := s.getUserNotificationChannels(user.ID)

	msg := iface.NotifyMessage{
		Subject: params.Title,
		Body:    params.Content,
	}

	var sendErrs []error
	sentCount := 0
	for _, notifier := range s.notifiers {
		// 检查该通知渠道是否启用
		if !s.isChannelEnabled(channels, notifier.Name()) {
			continue
		}

		to := s.getRecipientForNotifier(notifier.Name(), &user)
		if to == "" {
			log.Printf("[ReminderService] 用户 %d 没有 %s 渠道的接收地址，跳过", user.ID, notifier.Name())
			continue
		}

		if err := notifier.Send(to, msg); err != nil {
			// bot 未启动不作为发送失败，避免任务重试导致重复发送
			if errors.Is(err, ErrBotNotStarted) {
				log.Printf("[ReminderService] %s 未启动，跳过发送: %s", notifier.Name(), params.Title)
				continue
			}
			log.Printf("[ReminderService] 通过 %s 发送失败: %v", notifier.Name(), err)
			sendErrs = append(sendErrs, fmt.Errorf("%s: %w", notifier.Name(), err))
		} else {
			sentCount++
			log.Printf("[ReminderService] 已通过 %s 发送: %s -> %s", notifier.Name(), params.Title, to)
		}
	}

	// 只有存在发送尝试且全部渠道都失败时，任务才判定为失败
	if len(sendErrs) > 0 && sentCount == 0 {
		return errors.Join(sendErrs...)
	}
	return nil
}

// getUserNotificationChannels 获取用户的通知渠道偏好
func (s *ReminderService) getUserNotificationChannels(userID uint) []string {
	var pref model.UserPreference
	err := DB.Where("user_id = ? AND pref_key = ?", userID, "notification_channels").First(&pref).Error
	if err != nil {
		// 默认使用邮件
		return []string{"email"}
	}

	var channels []string
	if err := json.Unmarshal(pref.PrefValue, &channels); err != nil {
		return []string{"email"}
	}

	if len(channels) == 0 {
		return []string{"email"}
	}

	return channels
}

// isChannelEnabled 检查渠道是否在启用列表中
func (s *ReminderService) isChannelEnabled(channels []string, channelName string) bool {
	for _, ch := range channels {
		if ch == channelName {
			return true
		}
	}
	return false
}

// getRecipientForNotifier 获取通知的接收地址
func (s *ReminderService) getRecipientForNotifier(notifierName string, user *model.User) string {
	switch notifierName {
	case "email":
		return user.Email
	case "telegram":
		if user.TelegramChatID != nil {
			return fmt.Sprintf("%d", *user.TelegramChatID)
		}
		return ""
	default:
		return ""
	}
}
