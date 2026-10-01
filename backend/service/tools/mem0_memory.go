package tools

import (
	interfaces "backend/interface"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

var mem0Svc interfaces.Mem0Service

// SetMem0Service 注册 mem0 服务实例供工具使用
func SetMem0Service(svc interfaces.Mem0Service) { mem0Svc = svc }

type Mem0MemoryConfig struct {
	APIKey  string `json:"api_key" description:"Mem0 API Key" required:"true"`
	BaseURL string `json:"base_url" description:"Mem0 API 地址"`
}

type mem0MemoryTool struct{}

type mem0ToolRequest struct {
	Action string `json:"action" jsonschema:"description=操作类型: search 搜索记忆, add 添加记忆"`
	Query  string `json:"query,omitempty" jsonschema:"description=搜索关键词（search 时必填）"`
	Memory string `json:"memory,omitempty" jsonschema:"description=要添加的记忆内容（add 时必填）"`
}

func init() {
	Register("mem0_memory", "用户记忆管理",
		"管理用户的长期记忆。可以搜索用户之前的学习记录、偏好和进度，也可以添加新的记忆。当需要了解用户的训练历史、学习偏好或之前的表现时使用搜索；当需要记住用户的重要信息时使用添加。",
		Mem0MemoryConfig{},
		func(config map[string]any) (tool.BaseTool, error) {
			return &mem0MemoryTool{}, nil
		})
}

func (t *mem0MemoryTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "mem0_memory",
		Desc: "管理用户的长期记忆。可以搜索用户之前的学习记录、偏好和进度，也可以添加新的记忆。当需要了解用户的训练历史、学习偏好或之前的表现时使用搜索；当需要记住用户的重要信息时使用添加。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     "操作类型: search 搜索记忆, add 添加记忆",
				Required: true,
			},
			"query": {
				Type:     schema.String,
				Desc:     "搜索关键词（search 时必填）",
				Required: false,
			},
			"memory": {
				Type:     schema.String,
				Desc:     "要添加的记忆内容（add 时必填）",
				Required: false,
			},
		}),
	}, nil
}

func (t *mem0MemoryTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	var req mem0ToolRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	if mem0Svc == nil || !mem0Svc.IsConfigured() {
		return "", fmt.Errorf("mem0 服务未配置")
	}

	userID, err := userIDFromSession(ctx)
	if err != nil {
		return "", err
	}

	switch req.Action {
	case "search":
		if req.Query == "" {
			return "", fmt.Errorf("搜索关键词不能为空")
		}
		memories, err := mem0Svc.SearchMemories(userID, req.Query, 10)
		if err != nil {
			return "", fmt.Errorf("搜索记忆失败: %w", err)
		}
		if len(memories) == 0 {
			return "未找到相关记忆", nil
		}
		result, _ := json.Marshal(memories)
		return string(result), nil		

	case "add":
		if req.Memory == "" {
			return "", fmt.Errorf("记忆内容不能为空")
		}
		messages := []interfaces.Mem0Message{
			{Role: "user", Content: req.Memory},
		}
		resp, err := mem0Svc.AddMemory(userID, messages, nil)
		if err != nil {
			return "", fmt.Errorf("添加记忆失败: %w", err)
		}
		return fmt.Sprintf("记忆已保存，状态: %s，事件 ID: %s", resp.Status, resp.EventID), nil

	default:
		return "", fmt.Errorf("不支持的操作: %s，支持的操作: search, add", req.Action)
	}
}
