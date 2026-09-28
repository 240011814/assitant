package interfaces

import "backend/model"

// UserMemoryService 用户画像/经历服务, 供 user_info 工具使用
type UserMemoryService interface {
	BuildProfilePrompt(userID uint) string
	SearchExperiences(userID uint, query string, page, pageSize int) (int64, []model.UserExperience, error)
	AddExperience(userID uint, exp *model.UserExperience) (*model.UserExperience, error)
	MergeProfileFacts(userID uint, facts []model.ProfileFact) error
}
