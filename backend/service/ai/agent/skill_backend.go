package agent

import (
	"context"
	"errors"
	"fmt"

	"backend/model"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/skill"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

// dbSkillBackend 实现 Eino skill.Backend, 从数据库动态加载已启用的 Skill
type dbSkillBackend struct{}

func (b *dbSkillBackend) List(_ context.Context) ([]skill.FrontMatter, error) {
	var items []model.AISkill
	if err := DB.Where("enabled = ?", true).Order("id ASC").Find(&items).Error; err != nil {
		return nil, fmt.Errorf("查询 skill 失败: %w", err)
	}
	list := make([]skill.FrontMatter, 0, len(items))
	for _, item := range items {
		list = append(list, toFrontMatter(item))
	}
	return list, nil
}

func (b *dbSkillBackend) Get(_ context.Context, name string) (skill.Skill, error) {
	var item model.AISkill
	if err := DB.Where("name = ? AND enabled = ?", name, true).First(&item).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return skill.Skill{}, fmt.Errorf("skill 不存在或未启用: %s", name)
		}
		return skill.Skill{}, err
	}
	return skill.Skill{
		FrontMatter:   toFrontMatter(item),
		Content:       item.Content,
		BaseDirectory: "db://ai_skills/" + item.Name,
	}, nil
}

func toFrontMatter(item model.AISkill) skill.FrontMatter {
	fm := skill.FrontMatter{
		Name:        item.Name,
		Description: item.Description,
		Agent:       item.Agent,
		Model:       item.Model,
	}
	switch skill.ContextMode(item.Context) {
	case skill.ContextModeFork:
		fm.Context = skill.ContextModeFork
	case skill.ContextModeForkWithContext:
		fm.Context = skill.ContextModeForkWithContext
	}
	return fm
}

// skillModelHub 按名称解析模型, 供 Skill 的 model 字段使用
type skillModelHub struct {
	svc *AIAgentService
}

func (h *skillModelHub) Get(ctx context.Context, name string) (einomodel.BaseModel[*schema.Message], error) {
	if h.svc == nil {
		return nil, errors.New("模型服务不可用")
	}
	return h.svc.getModel(name)
}

// skillAgentHub 为 fork / fork_with_context 模式构建子 Agent
type skillAgentHub struct {
	svc *AIAgentService
}

func (h *skillAgentHub) Get(ctx context.Context, name string, opts *skill.AgentHubOptions) (adk.Agent, error) {
	if h.svc == nil {
		return nil, errors.New("Agent 服务不可用")
	}
	var chatModel einomodel.BaseChatModel
	if opts != nil && opts.Model != nil {
		chatModel = opts.Model
	} else {
		m, err := h.svc.getModel("")
		if err != nil {
			return nil, err
		}
		chatModel = m
	}
	agentName := name
	if agentName == "" {
		agentName = "skill-subagent"
	}
	return adk.NewChatModelAgent(ctx, &adk.ChatModelAgentConfig{
		Name:        agentName,
		Description: "Skill 子 Agent",
		Instruction: "你是一个执行 Skill 的助手, 请严格按用户消息中的 Skill 指令完成任务, 并直接输出结果。",
		Model:       chatModel,
	})
}

// BuildSkillMiddleware 构建基于数据库动态加载的 Eino Skill 中间件
func BuildSkillMiddleware(ctx context.Context, agentService *AIAgentService) (adk.ChatModelAgentMiddleware, error) {
	return skill.NewMiddleware(ctx, &skill.Config{
		Backend:  &dbSkillBackend{},
		AgentHub: &skillAgentHub{svc: agentService},
		ModelHub: &skillModelHub{svc: agentService},
	})
}
