package service

import (
	"backend/model"
	"errors"

	"gorm.io/gorm"
)

type PromptService struct {
	db           *gorm.DB
	agentService *AIAgentService
}

func NewPromptService(db *gorm.DB, agentService *AIAgentService) *PromptService {
	return &PromptService{db: db, agentService: agentService}
}

func (s *PromptService) GetEffectivePrompt(userID uint, agentID uint) (string, string, int, error) {
	var userPrompt model.UserPrompt
	err := s.db.Where("user_id = ? AND agent_id = ? AND is_active = ?", userID, agentID, true).First(&userPrompt).Error
	if err == nil {
		topK := userPrompt.MemorySearchTopK
		if topK <= 0 {
			topK = 30
		}
		return userPrompt.CustomPrompt, userPrompt.MemorySearchQuery, topK, nil
	}

	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", "", 30, nil
	}

	return "", "", 30, err
}

func (s *PromptService) ListVersions(userID uint, agentID uint) ([]model.UserPrompt, error) {
	var list []model.UserPrompt
	err := s.db.Where("user_id = ? AND agent_id = ?", userID, agentID).Order("version DESC").Find(&list).Error
	return list, err
}

func (s *PromptService) SaveUserPrompt(userID uint, agentID uint, content, remark string) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserPrompt{}).
			Where("user_id = ? AND agent_id = ?", userID, agentID).
			Update("is_active", false).Error; err != nil {
			return err
		}

		var maxVersion int
		tx.Model(&model.UserPrompt{}).
			Where("user_id = ? AND agent_id = ?", userID, agentID).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)

		newPrompt := model.UserPrompt{
			UserID:       userID,
			AgentID:      agentID,
			CustomPrompt: content,
			Version:      maxVersion + 1,
			IsActive:     true,
			Remark:       remark,
		}

		return tx.Create(&newPrompt).Error
	})
	if err == nil {
		s.clearCache(userID, agentID)
	}
	return err
}

func (s *PromptService) SwitchVersion(userID uint, agentID uint, versionID uint) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserPrompt{}).
			Where("user_id = ? AND agent_id = ?", userID, agentID).
			Update("is_active", false).Error; err != nil {
			return err
		}

		// 必须带 agent_id 约束: 防止把其他 agent 的版本切到当前 agent 下造成 active 错乱
		return tx.Model(&model.UserPrompt{}).
			Where("id = ? AND user_id = ? AND agent_id = ?", versionID, userID, agentID).
			Update("is_active", true).Error
	})
	if err == nil {
		s.clearCache(userID, agentID)
	}
	return err
}

func (s *PromptService) ResetUserPrompt(userID uint, agentID uint) error {
	err := s.db.Where("user_id = ? AND agent_id = ?", userID, agentID).Delete(&model.UserPrompt{}).Error
	if err == nil {
		s.clearCache(userID, agentID)
	}
	return err
}

func (s *PromptService) clearCache(userID uint, agentID uint) {
	if s.agentService != nil {
		s.agentService.clearRunnerCache(userID, agentID)
	}
}

func (s *PromptService) DeleteVersion(userID uint, agentID uint, versionID uint) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var prompt model.UserPrompt
		// 必须带 agent_id 约束: 只能删除当前 agent 下的版本
		if err := tx.Where("id = ? AND user_id = ? AND agent_id = ?", versionID, userID, agentID).First(&prompt).Error; err != nil {
			return err
		}

		if err := tx.Delete(&model.UserPrompt{}, versionID).Error; err != nil {
			return err
		}

		if prompt.IsActive {
			var latest model.UserPrompt
			err := tx.Where("user_id = ? AND agent_id = ?", userID, agentID).
				Order("version DESC").First(&latest).Error
			if err == nil {
				return tx.Model(&latest).Update("is_active", true).Error
			}
		}

		return nil
	})
	if err == nil {
		s.clearCache(userID, agentID)
	}
	return err
}
