package skill

import (
	"errors"

	"backend/service/ai/agent"

	"backend/model"

	"github.com/cloudwego/eino/adk/middlewares/skill"
	"gorm.io/gorm"
)

// DB 本包专用数据库句柄, 由 main.go 接线赋值。
var DB *gorm.DB

// AISkillService Skill 配置的增删改查
type AISkillService struct {
	agentService *agent.AIAgentService
}

func NewAISkillService(agentService *agent.AIAgentService) *AISkillService {
	return &AISkillService{agentService: agentService}
}

func (s *AISkillService) List() ([]model.AISkill, error) {
	var list []model.AISkill
	if err := DB.Order("id ASC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (s *AISkillService) Get(id uint) (*model.AISkill, error) {
	var item model.AISkill
	if err := DB.First(&item, id).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *AISkillService) Create(req *model.CreateAISkillRequest) (*model.AISkill, error) {
	if req.Name == "" {
		return nil, errors.New("name 不能为空")
	}
	ctx := normalizeSkillContext(req.Context)
	enabled := false
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item := model.AISkill{
		Name:        req.Name,
		Description: req.Description,
		Context:     ctx,
		Agent:       req.Agent,
		Model:       req.Model,
		Content:     req.Content,
		Enabled:     enabled,
	}
	if err := DB.Create(&item).Error; err != nil {
		return nil, err
	}
	s.invalidate()
	return &item, nil
}

func (s *AISkillService) Update(id uint, req *model.UpdateAISkillRequest) error {
	var item model.AISkill
	if err := DB.First(&item, id).Error; err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Context != nil {
		updates["context"] = normalizeSkillContext(*req.Context)
	}
	if req.Agent != nil {
		updates["agent"] = *req.Agent
	}
	if req.Model != nil {
		updates["model"] = *req.Model
	}
	if req.Content != nil {
		updates["content"] = *req.Content
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if len(updates) == 0 {
		return nil
	}
	if err := DB.Model(&model.AISkill{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return err
	}
	s.invalidate()
	return nil
}

func (s *AISkillService) Delete(id uint) error {
	if err := DB.Delete(&model.AISkill{}, id).Error; err != nil {
		return err
	}
	s.invalidate()
	return nil
}

// invalidate 清理 runner 缓存, 使 Skill 变更后的工具描述及时生效
func (s *AISkillService) invalidate() {
	if s.agentService != nil {
		s.agentService.ClearRunnerCache()
	}
}

func normalizeSkillContext(ctx string) string {
	switch skill.ContextMode(ctx) {
	case skill.ContextModeFork, skill.ContextModeForkWithContext:
		return ctx
	default:
		return "inline"
	}
}
