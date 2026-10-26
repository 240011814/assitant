package service

import (
	"backend/model"
	"errors"
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// PromptService 用户提示词: 按 (user_id, agent_id, node_key) 维护版本。
// node_key 为空串即普通 Agent 的用户提示词; 编排 Agent 节点传画布节点 id,
// 与普通提示词共用 user_prompts 表和同一套版本机制
type PromptService struct {
	db           *gorm.DB
	agentService *AIAgentService
	// orchPromptCache 编排节点提示词版本缓存 (键: agentID|nodeKey, user_id 恒为 0):
	// 编译每个 Agent 节点每次运行都要解析一次, 变更时由 Save/Switch/Delete/Reset 失效
	orchPromptCache map[string]orchPromptEntry
	orchPromptMu    sync.RWMutex
}

type orchPromptEntry struct {
	prompt string
	found  bool
}

func NewPromptService(db *gorm.DB, agentService *AIAgentService) *PromptService {
	return &PromptService{db: db, agentService: agentService}
}

// GetOrchNodePrompt 编排 Agent 节点已启用的提示词版本 (编排全局共享, user_id=0)。
// found=false 表示无启用版本 (含版本内容为空), 编译期回退画布内联提示词
func (s *PromptService) GetOrchNodePrompt(agentID uint, nodeKey string) (string, bool) {
	key := fmt.Sprintf("%d|%s", agentID, nodeKey)
	s.orchPromptMu.RLock()
	if e, ok := s.orchPromptCache[key]; ok {
		s.orchPromptMu.RUnlock()
		return e.prompt, e.found
	}
	s.orchPromptMu.RUnlock()
	prompt, found := s.loadOrchNodePrompt(agentID, nodeKey)
	s.orchPromptMu.Lock()
	if s.orchPromptCache == nil {
		s.orchPromptCache = make(map[string]orchPromptEntry)
	}
	s.orchPromptCache[key] = orchPromptEntry{prompt: prompt, found: found}
	s.orchPromptMu.Unlock()
	return prompt, found
}

func (s *PromptService) loadOrchNodePrompt(agentID uint, nodeKey string) (string, bool) {
	var userPrompt model.UserPrompt
	if err := s.db.Where("user_id = ? AND agent_id = ? AND node_key = ? AND is_active = ?", 0, agentID, nodeKey, true).
		First(&userPrompt).Error; err != nil {
		return "", false
	}
	if strings.TrimSpace(userPrompt.CustomPrompt) == "" {
		return "", false
	}
	return userPrompt.CustomPrompt, true
}

// invalidateOrchPromptCache 清空编排节点版本缓存 (提示词任何变更都全清, 量小且保证切版本立刻生效)
func (s *PromptService) invalidateOrchPromptCache() {
	s.orchPromptMu.Lock()
	s.orchPromptCache = make(map[string]orchPromptEntry)
	s.orchPromptMu.Unlock()
}

func (s *PromptService) GetEffectivePrompt(userID uint, agentID uint, nodeKey string) (string, string, int, error) {
	var userPrompt model.UserPrompt
	err := s.db.Where("user_id = ? AND agent_id = ? AND node_key = ? AND is_active = ?", userID, agentID, nodeKey, true).First(&userPrompt).Error
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

func (s *PromptService) ListVersions(userID uint, agentID uint, nodeKey string) ([]model.UserPrompt, error) {
	var list []model.UserPrompt
	err := s.db.Where("user_id = ? AND agent_id = ? AND node_key = ?", userID, agentID, nodeKey).Order("version DESC").Find(&list).Error
	return list, err
}

func (s *PromptService) SaveUserPrompt(userID uint, agentID uint, nodeKey string, content, remark string) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserPrompt{}).
			Where("user_id = ? AND agent_id = ? AND node_key = ?", userID, agentID, nodeKey).
			Update("is_active", false).Error; err != nil {
			return err
		}

		var maxVersion int
		tx.Model(&model.UserPrompt{}).
			Where("user_id = ? AND agent_id = ? AND node_key = ?", userID, agentID, nodeKey).
			Select("COALESCE(MAX(version), 0)").Scan(&maxVersion)

		newPrompt := model.UserPrompt{
			UserID:       userID,
			AgentID:      agentID,
			NodeKey:      nodeKey,
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

func (s *PromptService) SwitchVersion(userID uint, agentID uint, nodeKey string, versionID uint) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.UserPrompt{}).
			Where("user_id = ? AND agent_id = ? AND node_key = ?", userID, agentID, nodeKey).
			Update("is_active", false).Error; err != nil {
			return err
		}

		// 必须带 agent_id/node_key 约束: 防止把其他 agent/节点的版本切过来造成 active 错乱
		return tx.Model(&model.UserPrompt{}).
			Where("id = ? AND user_id = ? AND agent_id = ? AND node_key = ?", versionID, userID, agentID, nodeKey).
			Update("is_active", true).Error
	})
	if err == nil {
		s.clearCache(userID, agentID)
	}
	return err
}

func (s *PromptService) ResetUserPrompt(userID uint, agentID uint, nodeKey string) error {
	err := s.db.Where("user_id = ? AND agent_id = ? AND node_key = ?", userID, agentID, nodeKey).Delete(&model.UserPrompt{}).Error
	if err == nil {
		s.clearCache(userID, agentID)
	}
	return err
}

func (s *PromptService) clearCache(userID uint, agentID uint) {
	s.invalidateOrchPromptCache()
	if s.agentService != nil {
		s.agentService.clearRunnerCache(userID, agentID)
	}
}

func (s *PromptService) DeleteVersion(userID uint, agentID uint, nodeKey string, versionID uint) error {
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var prompt model.UserPrompt
		// 必须带 agent_id/node_key 约束: 只能删除当前 agent/节点下的版本
		if err := tx.Where("id = ? AND user_id = ? AND agent_id = ? AND node_key = ?", versionID, userID, agentID, nodeKey).First(&prompt).Error; err != nil {
			return err
		}

		if err := tx.Delete(&model.UserPrompt{}, versionID).Error; err != nil {
			return err
		}

		if prompt.IsActive {
			var latest model.UserPrompt
			err := tx.Where("user_id = ? AND agent_id = ? AND node_key = ?", userID, agentID, nodeKey).
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
