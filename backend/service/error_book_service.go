package service

import (
	"backend/model"
	"log"
)

type ErrorBookService struct{}

func NewErrorBookService() *ErrorBookService {
	return &ErrorBookService{}
}

// AddErrorBook 添加错题（如果已存在则增加错误次数）
func (s *ErrorBookService) AddErrorBook(userID uint, req model.CreateErrorBookRequest) (*model.ErrorBook, error) {
	// 检查是否已存在（同一用户 + 同一内容）
	var existing model.ErrorBook
	err := DB.Where("user_id = ? AND content = ? AND content_type = ?", userID, req.Content, req.ContentType).
		First(&existing).Error

	if err == nil {
		// 已存在，增加错误次数
		existing.ErrorCount++
		if err := DB.Save(&existing).Error; err != nil {
			return nil, err
		}
		return &existing, nil
	}

	// 不存在，创建新记录
	item := model.ErrorBook{
		UserID:      userID,
		ContentType: req.ContentType,
		Content:     req.Content,
		Translation: req.Translation,
		SourceType:  req.SourceType,
		SourceID:    req.SourceID,
		ErrorCount:  1,
		IsMastered:  false,
	}

	if err := DB.Create(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

// GetErrorBookList 获取错题本列表
func (s *ErrorBookService) GetErrorBookList(userID uint, sourceType string, isMastered *bool, keyword string) ([]model.ErrorBook, error) {
	var list []model.ErrorBook
	query := DB.Where("user_id = ?", userID)

	if sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if isMastered != nil {
		query = query.Where("is_mastered = ?", *isMastered)
	}
	if keyword != "" {
		query = query.Where("content LIKE ?", "%"+keyword+"%")
	}

	if err := query.Order("error_count DESC, created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetErrorBookForPractice 获取错题本练习数据（未掌握的）
func (s *ErrorBookService) GetErrorBookForPractice(userID uint, contentType string) ([]model.ErrorBook, error) {
	var list []model.ErrorBook
	query := DB.Where("user_id = ? AND is_mastered = ?", userID, false)

	if contentType != "" {
		query = query.Where("content_type = ?", contentType)
	}

	if err := query.Order("error_count DESC, created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetRandomErrorBooks 随机获取指定数量的错题
func (s *ErrorBookService) GetRandomErrorBooks(userID uint, count int, contentType string) ([]model.ErrorBook, error) {
	var list []model.ErrorBook
	query := DB.Where("user_id = ? AND is_mastered = ?", userID, false)
	if contentType != "" {
		query = query.Where("content_type = ?", contentType)
	}
	if err := query.Order("RAND()").Limit(count).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateErrorBook 更新错题状态
func (s *ErrorBookService) UpdateErrorBook(userID, id uint, req model.UpdateErrorBookRequest) error {
	updates := map[string]interface{}{}
	if req.IsMastered != nil {
		updates["is_mastered"] = *req.IsMastered
	}

	return DB.Model(&model.ErrorBook{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates).Error
}

// DeleteErrorBook 删除错题
func (s *ErrorBookService) DeleteErrorBook(userID, id uint) error {
	return DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.ErrorBook{}).Error
}

// GetErrorBookStats 获取错题统计
func (s *ErrorBookService) GetErrorBookStats(userID uint) (map[string]interface{}, error) {
	var total int64
	var mastered int64
	var wordCount int64
	var sentenceCount int64

	if err := DB.Model(&model.ErrorBook{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		log.Printf("[error_book] 统计 total 失败 userID=%d: %v", userID, err)
	}
	if err := DB.Model(&model.ErrorBook{}).Where("user_id = ? AND is_mastered = ?", userID, true).Count(&mastered).Error; err != nil {
		log.Printf("[error_book] 统计 mastered 失败 userID=%d: %v", userID, err)
	}
	if err := DB.Model(&model.ErrorBook{}).Where("user_id = ? AND source_type = ?", userID, "vocabulary").Count(&wordCount).Error; err != nil {
		log.Printf("[error_book] 统计 wordCount 失败 userID=%d: %v", userID, err)
	}
	if err := DB.Model(&model.ErrorBook{}).Where("user_id = ? AND source_type = ?", userID, "course").Count(&sentenceCount).Error; err != nil {
		log.Printf("[error_book] 统计 sentenceCount 失败 userID=%d: %v", userID, err)
	}

	return map[string]interface{}{
		"total":         total,
		"mastered":      mastered,
		"unmastered":    total - mastered,
		"wordCount":     wordCount,
		"sentenceCount": sentenceCount,
	}, nil
}
