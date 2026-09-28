package tools

import (
	interfaces "backend/interface"
	"backend/model"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var userMemorySvc interfaces.UserMemoryService

// SetUserMemoryService 注册用户画像/经历服务实例供工具使用
func SetUserMemoryService(svc interfaces.UserMemoryService) { userMemorySvc = svc }

const (
	userInfoQueryDesc = "查询用户画像与个人经历。get_profile 获取用户画像; search_experience 分页搜索用户过去的经历(返回 total/page/page_size/items)。当需要了解用户是什么样的人、有什么目标/偏好，或回顾用户过往经历时使用。"
	userInfoEditDesc  = "更新用户画像与个人经历。add_experience 记录一段用户经历; update_profile 更新用户画像事实。当用户希望你记住某段经历、或根据对话更新对用户的认识时使用。"
)

type UserInfoQueryConfig struct{}

type UserInfoEditConfig struct{}

func init() {
	Register("user_info_query", "用户画像查询", userInfoQueryDesc, UserInfoQueryConfig{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &userInfoQueryTool{}, nil
		})
	Register("user_info_edit", "用户画像编辑", userInfoEditDesc, UserInfoEditConfig{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &userInfoEditTool{}, nil
		})
}

func userIDFromSession(ctx context.Context) (uint, error) {
	sessionValues := adk.GetSessionValues(ctx)
	userIDVal, ok := sessionValues["user_id"]
	if !ok {
		return 0, fmt.Errorf("无法获取用户 ID")
	}
	userID, ok := userIDVal.(uint)
	if !ok {
		return 0, fmt.Errorf("用户 ID 类型错误")
	}
	return userID, nil
}

// ============ 查询工具 ============

type userInfoQueryTool struct{}

type userInfoQueryRequest struct {
	Action   string `json:"action" jsonschema:"description=操作类型: get_profile 获取画像, search_experience 搜索经历"`
	Query    string `json:"query,omitempty" jsonschema:"description=搜索关键词（search_experience 时使用）"`
	Page     int    `json:"page,omitempty" jsonschema:"description=页码（search_experience 时使用, 默认1）"`
	PageSize int    `json:"page_size,omitempty" jsonschema:"description=每页数量（search_experience 时使用, 默认10, 最大50）"`
}

func (t *userInfoQueryTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "user_info_query",
		Desc: userInfoQueryDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     "操作类型: get_profile 获取画像, search_experience 搜索经历",
				Required: true,
			},
			"query": {
				Type: schema.String,
				Desc: "搜索关键词（search_experience 时使用）",
			},
			"page": {
				Type: schema.Integer,
				Desc: "页码（search_experience 时使用, 默认1）",
			},
			"page_size": {
				Type: schema.Integer,
				Desc: "每页数量（search_experience 时使用, 默认10, 最大50）",
			},
		}),
	}, nil
}

func (t *userInfoQueryTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	var req userInfoQueryRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}
	if userMemorySvc == nil {
		return "", fmt.Errorf("用户画像服务未初始化")
	}
	userID, err := userIDFromSession(ctx)
	if err != nil {
		return "", err
	}

	switch req.Action {
	case "get_profile":
		profile := userMemorySvc.BuildProfilePrompt(userID)
		if profile == "" {
			return "暂无用户画像", nil
		}
		return profile, nil

	case "search_experience":
		total, list, err := userMemorySvc.SearchExperiences(userID, req.Query, req.Page, req.PageSize)
		if err != nil {
			return "", fmt.Errorf("搜索经历失败: %w", err)
		}
		if total == 0 {
			return "未找到相关经历", nil
		}
		page := req.Page
		if page < 1 {
			page = 1
		}
		pageSize := req.PageSize
		if pageSize <= 0 {
			pageSize = 10
		}
		result, _ := json.Marshal(map[string]any{
			"total":     total,
			"page":      page,
			"page_size": pageSize,
			"items":     toExperienceViews(list),
		})
		return string(result), nil

	default:
		return "", fmt.Errorf("不支持的操作: %s，支持的操作: get_profile, search_experience", req.Action)
	}
}

// ============ 编辑工具 ============

type userInfoEditTool struct{}

type userInfoEditRequest struct {
	Action      string              `json:"action" jsonschema:"description=操作类型: add_experience 记录经历, update_profile 更新画像"`
	Category    string              `json:"category,omitempty" jsonschema:"description=经历分类: identity/goal/project/skill/preference/habit/experience/challenge/decision"`
	Title       string              `json:"title,omitempty" jsonschema:"description=经历标题（add_experience 时必填）"`
	Content     string              `json:"content,omitempty" jsonschema:"description=经历内容（add_experience 时使用）"`
	TimeRange   string              `json:"time_range,omitempty" jsonschema:"description=经历时间段（add_experience 时使用, 如 2026）"`
	Status      string              `json:"status,omitempty" jsonschema:"description=进度状态: ongoing/completed/abandoned/unknown"`
	MemoryLevel string              `json:"memory_level,omitempty" jsonschema:"description=记忆等级: core/long_term/temporary"`
	Evidence    string              `json:"evidence,omitempty" jsonschema:"description=用户原话摘要"`
	Tags        []string            `json:"tags,omitempty" jsonschema:"description=经历标签"`
	Facts       []model.ProfileFact `json:"facts,omitempty" jsonschema:"description=要合并进画像的新事实（update_profile 时必填）, 每项含 content/memory_level/confidence/evidence"`
}

func (t *userInfoEditTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "user_info_edit",
		Desc: userInfoEditDesc,
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     "操作类型: add_experience 记录经历, update_profile 更新画像",
				Required: true,
			},
			"category": {
				Type: schema.String,
				Desc: "经历分类: identity/goal/project/skill/preference/habit/experience/challenge/decision",
			},
			"title": {
				Type: schema.String,
				Desc: "经历标题（add_experience 时必填）",
			},
			"content": {
				Type: schema.String,
				Desc: "经历内容（add_experience 时使用）",
			},
			"time_range": {
				Type: schema.String,
				Desc: "经历时间段（add_experience 时使用, 如 2026）",
			},
			"status": {
				Type: schema.String,
				Desc: "进度状态: ongoing/completed/abandoned/unknown",
			},
			"memory_level": {
				Type: schema.String,
				Desc: "记忆等级: core/long_term/temporary",
			},
			"evidence": {
				Type: schema.String,
				Desc: "用户原话摘要",
			},
			"tags": {
				Type: schema.Array,
				Desc: "经历标签",
			},
			"facts": {
				Type: schema.Array,
				Desc: "要合并进画像的新事实（update_profile 时必填）, 每项含 content/memory_level/confidence/evidence",
			},
		}),
	}, nil
}

func (t *userInfoEditTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	var req userInfoEditRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}
	if userMemorySvc == nil {
		return "", fmt.Errorf("用户画像服务未初始化")
	}
	userID, err := userIDFromSession(ctx)
	if err != nil {
		return "", err
	}

	switch req.Action {
	case "add_experience":
		if req.Title == "" {
			return "", fmt.Errorf("经历标题不能为空")
		}
		tags, _ := json.Marshal(req.Tags)
		exp, err := userMemorySvc.AddExperience(userID, &model.UserExperience{
			Category:    req.Category,
			Title:       req.Title,
			Content:     req.Content,
			TimeRange:   req.TimeRange,
			Status:      req.Status,
			MemoryLevel: req.MemoryLevel,
			Evidence:    req.Evidence,
			Tags:        string(tags),
		})
		if err != nil {
			return "", fmt.Errorf("记录经历失败: %w", err)
		}
		return fmt.Sprintf("经历已记录, ID: %d", exp.ID), nil

	case "update_profile":
		if len(req.Facts) == 0 {
			return "", fmt.Errorf("facts 不能为空")
		}
		if err := userMemorySvc.MergeProfileFacts(userID, req.Facts); err != nil {
			return "", fmt.Errorf("更新画像失败: %w", err)
		}
		return "用户画像已更新", nil

	default:
		return "", fmt.Errorf("不支持的操作: %s，支持的操作: add_experience, update_profile", req.Action)
	}
}

// ============ 辅助 ============

type experienceView struct {
	ID          uint        `json:"id"`
	Category    string      `json:"category"`
	Title       string      `json:"title"`
	Content     string      `json:"content"`
	TimeRange   string      `json:"time_range"`
	Status      string      `json:"status"`
	MemoryLevel string      `json:"memory_level"`
	Tags        []string    `json:"tags"`
	OccurredAt  interface{} `json:"occurred_at"`
}

func toExperienceViews(list []model.UserExperience) []experienceView {
	views := make([]experienceView, 0, len(list))
	for _, e := range list {
		var tags []string
		_ = json.Unmarshal([]byte(e.Tags), &tags)
		views = append(views, experienceView{
			ID:          e.ID,
			Category:    e.Category,
			Title:       e.Title,
			Content:     e.Content,
			TimeRange:   e.TimeRange,
			Status:      e.Status,
			MemoryLevel: e.MemoryLevel,
			Tags:        tags,
			OccurredAt:  e.OccurredAt,
		})
	}
	return views
}
