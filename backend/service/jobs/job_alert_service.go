package service

import (
	iface "backend/interface"
	"backend/model"
	"fmt"
	"log"
	"strings"
	"time"
)

// JobAlertService 定时任务失败邮件告警: 任务定义配置了通知邮箱, 重试耗尽仍失败时发送
type JobAlertService struct {
	email *EmailNotifier
}

// NewJobAlertService 创建任务失败告警服务
func NewJobAlertService(email *EmailNotifier) *JobAlertService {
	return &JobAlertService{email: email}
}

// NotifyJobFailed 发送任务失败告警邮件 (异步, 不阻塞调度流程; 未配置通知邮箱时忽略)
func (s *JobAlertService) NotifyJobFailed(def *model.JobDefinition, errMsg string, attempt int, triggerType string, failedAt time.Time) {
	if s == nil || s.email == nil {
		return
	}

	to := strings.TrimSpace(def.NotifyEmail)
	if to == "" {
		return
	}

	trigger := "调度触发"
	if triggerType == model.JobTriggerManual {
		trigger = "手动触发"
	}

	msg := iface.NotifyMessage{
		Subject: fmt.Sprintf("【任务告警】定时任务执行失败: %s", def.Name),
		Body: fmt.Sprintf(`定时任务重试后仍未成功，请及时处理。

任务名称: %s
任务方法: %s
任务定义 ID: %d
触发方式: %s
失败时间: %s
尝试次数: 第 %d 次 (最大重试 %d 次)

失败原因:
%s

此邮件由系统自动发送，请勿直接回复。`,
			def.Name, def.TaskName, def.ID, trigger,
			failedAt.Format("2006-01-02 15:04:05"),
			attempt, def.MaxRetries, errMsg),
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[JobAlert] 发送告警邮件 panic: %v", r)
			}
		}()
		if err := s.email.Send(to, msg); err != nil {
			log.Printf("[JobAlert] 告警邮件发送失败 -> %s: %v", to, err)
		} else {
			log.Printf("[JobAlert] 告警邮件已发送 -> %s (任务: %s)", to, def.Name)
		}
	}()
}
