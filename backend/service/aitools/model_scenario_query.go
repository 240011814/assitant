package aitools

import (
	"backend/model"
	"context"
	"encoding/json"
	"fmt"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

type ModelScenarioQueryConfig struct{}

type modelScenarioQueryTool struct{}

type modelScenarioQueryRequest struct {
	Action   string `json:"action" jsonschema:"description=操作类型: list(列表查询) 或 get(详情查询)"`
	Type     string `json:"type,omitempty" jsonschema:"description=类型过滤: model(思维模型) 或 scenario(场景), 仅list时有效"`
	Category string `json:"category,omitempty" jsonschema:"description=分类过滤, 仅list时有效"`
	Keyword  string `json:"keyword,omitempty" jsonschema:"description=按名称模糊搜索, 仅list时有效"`
	ID       uint   `json:"id,omitempty" jsonschema:"description=记录ID, 仅get时有效"`
}

func NewModelScenarioQueryTool() (tool.InvokableTool, error) {
	return &modelScenarioQueryTool{}, nil
}

func init() {
	Register("model_scenario_query", "思维模型和场景应对方法查询", "查询思维模型和生活场景的列表与详情。可用于帮助用户了解有哪些思维模型可用、按分类浏览模型和场景、或查看某个具体模型/场景的详细内容。", ModelScenarioQueryConfig{}, func(config map[string]any) (tool.BaseTool, error) {
		return NewModelScenarioQueryTool()
	})
}

func (t *modelScenarioQueryTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: "model_scenario_query",
		Desc: "查询思维模型和生活场景应对方法的列表与详情。支持按类型(model/scenario)、分类、名称关键词筛选列表，或根据ID查看详细信息。",
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"action": {
				Type:     schema.String,
				Desc:     "操作类型: list(查询列表) 或 get(查询详情)",
				Required: true,
			},
			"type": {
				Type:     schema.String,
				Desc:     "类型过滤: model(思维模型) 或 scenario(场景), 仅list时有效",
				Required: false,
			},
			"category": {
				Type:     schema.String,
				Desc:     "分类名称过滤, 仅list时有效",
				Required: false,
			},
			"keyword": {
				Type:     schema.String,
				Desc:     "按名称模糊搜索关键词, 仅list时有效",
				Required: false,
			},
			"id": {
				Type:     schema.Integer,
				Desc:     "记录ID, 仅get时使用",
				Required: false,
			},
		}),
	}, nil
}

func (t *modelScenarioQueryTool) InvokableRun(ctx context.Context, arguments string, _ ...tool.Option) (string, error) {
	var req modelScenarioQueryRequest
	if err := json.Unmarshal([]byte(arguments), &req); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	if GetDB() == nil {
		return "", fmt.Errorf("数据库未初始化")
	}

	switch req.Action {
	case "list":
		return t.handleList(req)
	case "get":
		return t.handleGet(req)
	default:
		return "", fmt.Errorf("不支持的操作: %s, 请使用 list 或 get", req.Action)
	}
}

func (t *modelScenarioQueryTool) handleList(req modelScenarioQueryRequest) (string, error) {
	query := GetDB().Model(&model.ModelScenario{})

	if req.Type != "" {
		query = query.Where("type = ?", req.Type)
	}
	if req.Category != "" {
		query = query.Where("category = ?", req.Category)
	}
	if req.Keyword != "" {
		query = query.Where("name LIKE ?", "%"+req.Keyword+"%")
	}

	var items []model.ModelScenario
	if err := query.Order("sort_order ASC, id ASC").Find(&items).Error; err != nil {
		return "", fmt.Errorf("查询失败: %w", err)
	}

	output := map[string]any{
		"total": len(items),
		"items": items,
	}
	data, _ := json.Marshal(output)
	return string(data), nil
}

func (t *modelScenarioQueryTool) handleGet(req modelScenarioQueryRequest) (string, error) {
	if req.ID == 0 {
		return "", fmt.Errorf("请提供要查询的记录ID")
	}

	var item model.ModelScenario
	if err := GetDB().Where("id = ?", req.ID).First(&item).Error; err != nil {
		return "", fmt.Errorf("未找到ID为 %d 的记录: %w", req.ID, err)
	}

	data, _ := json.Marshal(item)
	return string(data), nil
}
