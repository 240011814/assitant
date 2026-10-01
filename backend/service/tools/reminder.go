package tools

import (
	"backend/model"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ReminderService interface {
	Create(userID uint, req model.CreateReminderRequest) (*model.Job, error)
	ListByMonth(userID uint, year int, month time.Month) ([]model.Job, error)
	Delete(userID uint, id int64, scope string) error
}

var reminderSvc ReminderService

func SetReminderService(svc ReminderService) { reminderSvc = svc }

type reminderRequest struct {
	Action         string `json:"action" jsonschema:"description=操作类型: create 创建备忘, list 查询备忘, delete 删除备忘"`
	Title          string `json:"title,omitempty" jsonschema:"description=备忘标题（create 时必填）"`
	Content        string `json:"content,omitempty" jsonschema:"description=备忘内容"`
	RemindAt       string `json:"remind_at,omitempty" jsonschema:"description=提醒时间，格式: 2006-01-02 15:04:05（create 时必填）"`
	RepeatType     string `json:"repeat_type,omitempty" jsonschema:"description=重复类型: none 不重复, daily 每天, weekly 每周, monthly 每月, yearly 每年"`
	RepeatInterval int    `json:"repeat_interval,omitempty" jsonschema:"description=重复间隔数，默认1"`
	RemindID       string `json:"reminder_id,omitempty" jsonschema:"description=备忘ID（delete 时必填）"`
	ListDate       string `json:"list_date,omitempty" jsonschema:"description=查询日期，格式: 2006-01-02（list 时使用，默认当天）"`
}

type ReminderConfig struct{}

func init() {
	Register("reminder", "备忘提醒",
		"管理用户的备忘提醒。可以创建、查询和删除备忘。支持设置提醒时间和重复提醒。",
		ReminderConfig{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &reminderTool{}, nil
		})
}

type reminderTool struct{}

func (t *reminderTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "reminder",
		Desc: "管理用户的备忘提醒。可以创建、查询和删除备忘。支持设置提醒时间和重复提醒。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action":          {Type: schema.String, Desc: "操作类型: create 创建备忘, list 查询备忘, delete 删除备忘", Required: true},
			"title":           {Type: schema.String, Desc: "备忘标题（create 时必填）", Required: false},
			"content":         {Type: schema.String, Desc: "备忘内容", Required: false},
			"remind_at":       {Type: schema.String, Desc: "提醒时间，格式: 2006-01-02 15:04:05（create 时必填）", Required: false},
			"repeat_type":     {Type: schema.String, Desc: "重复类型: none 不重复, daily 每天, weekly 每周, monthly 每月, yearly 每年", Required: false},
			"repeat_interval": {Type: schema.Integer, Desc: "重复间隔数，默认1", Required: false},
			"reminder_id":     {Type: schema.String, Desc: "备忘ID（delete 时必填）", Required: false},
			"list_date":       {Type: schema.String, Desc: "查询日期，格式: 2006-01-02（list 时使用，默认当天）", Required: false},
		}),
	}, nil
}

func (t *reminderTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	var req reminderRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	userID, err := userIDFromSession(ctx)
	if err != nil {
		return "", err
	}

	switch req.Action {
	case "create":
		if req.Title == "" {
			return "", fmt.Errorf("标题不能为空")
		}
		if req.RemindAt == "" {
			return "", fmt.Errorf("提醒时间不能为空")
		}
		remindAt, err := time.ParseInLocation("2006-01-02 15:04:05", req.RemindAt, time.Local)
		if err != nil {
			return "", fmt.Errorf("提醒时间格式错误，应为: 2006-01-02 15:04:05")
		}
		repeatType := req.RepeatType
		if repeatType == "" {
			repeatType = "none"
		}
		repeatInterval := req.RepeatInterval
		if repeatInterval <= 0 {
			repeatInterval = 1
		}
		job, err := reminderSvc.Create(userID, model.CreateReminderRequest{
			Title: req.Title, Content: req.Content, RemindAt: remindAt,
			RepeatType: repeatType, RepeatInterval: repeatInterval,
		})
		if err != nil {
			return "", err
		}
		data, _ := json.Marshal(job)
		return string(data), nil

	case "list":
		now := time.Now()
		jobs, _ := reminderSvc.ListByMonth(userID, now.Year(), now.Month())
		var filtered []model.Job
		if req.ListDate != "" {
			d, err := time.ParseInLocation("2006-01-02", req.ListDate, time.Local)
			if err != nil {
				return "", fmt.Errorf("日期格式错误，应为: 2006-01-02")
			}
			for _, j := range jobs {
				if j.ScheduledAt.Year() == d.Year() && j.ScheduledAt.Month() == d.Month() && j.ScheduledAt.Day() == d.Day() {
					filtered = append(filtered, j)
				}
			}
		} else {
			for _, j := range jobs {
				if j.ScheduledAt.Year() == now.Year() && j.ScheduledAt.Month() == now.Month() && j.ScheduledAt.Day() == now.Day() {
					filtered = append(filtered, j)
				}
			}
		}
		data, _ := json.Marshal(filtered)
		return string(data), nil

	case "delete":
		if req.RemindID == "" {
			return "", fmt.Errorf("备忘ID不能为空")
		}
		id, err := strconv.ParseInt(req.RemindID, 10, 64)
		if err != nil {
			return "", fmt.Errorf("备忘ID格式错误")
		}
		if err := reminderSvc.Delete(userID, id, "all"); err != nil {
			return "", err
		}
		return fmt.Sprintf(`{"id":%d,"message":"备忘已删除"}`, id), nil

	default:
		return "", fmt.Errorf("未知操作: %s，支持的操作: create, list, delete", req.Action)
	}
}