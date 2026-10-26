package service

import (
	"backend/model"
)

// ModelScenarioService 模型和场景服务
type ModelScenarioService struct{}

func NewModelScenarioService() *ModelScenarioService {
	return &ModelScenarioService{}
}

// List 获取列表
func (s *ModelScenarioService) List(typ string) ([]model.ModelScenario, error) {
	var items []model.ModelScenario
	query := DB.Order("sort_order ASC, id ASC")
	if typ != "" {
		query = query.Where("type = ?", typ)
	}
	err := query.Find(&items).Error
	return items, err
}

// Get 获取单个
func (s *ModelScenarioService) Get(id uint) (*model.ModelScenario, error) {
	var item model.ModelScenario
	err := DB.First(&item, id).Error
	return &item, err
}

// Create 创建
func (s *ModelScenarioService) Create(req *model.CreateModelScenarioRequest) (*model.ModelScenario, error) {
	item := &model.ModelScenario{
		Type:        req.Type,
		Name:        req.Name,
		Summary:     req.Summary,
		Description: req.Description,
		Detail:    req.Detail,
		Category:  req.Category,
		SortOrder: req.SortOrder,
	}
	if err := DB.Create(item).Error; err != nil {
		return nil, err
	}
	return item, nil
}

// Update 更新
func (s *ModelScenarioService) Update(id uint, req *model.UpdateModelScenarioRequest) error {
	updates := map[string]interface{}{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Summary != nil {
		updates["summary"] = *req.Summary
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Detail != nil {
		updates["detail"] = *req.Detail
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.SortOrder != nil {
		updates["sort_order"] = *req.SortOrder
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Model(&model.ModelScenario{}).Where("id = ?", id).Updates(updates).Error
}

// Delete 删除
func (s *ModelScenarioService) Delete(id uint) error {
	return DB.Delete(&model.ModelScenario{}, id).Error
}
