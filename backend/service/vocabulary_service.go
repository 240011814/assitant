package service

import (
	"errors"
	"fmt"
	"time"

	"backend/model"
)

type VocabularyService struct{}

func NewVocabularyService() *VocabularyService {
	return &VocabularyService{}
}

// AddWord 添加生词
func (s *VocabularyService) AddWord(userID uint, req model.CreateVocabularyRequest) (*model.Vocabulary, error) {
	// 重复检查：同一用户下单词不重复（忽略大小写）
	var count int64
	if err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ? AND LOWER(word) = LOWER(?)", userID, req.Word).
		Count(&count).Error; err != nil {
		return nil, err
	}
	if count > 0 {
		return nil, errors.New("该单词已存在 ")
	}

	word := model.Vocabulary{
		UserID:         userID,
		Word:           req.Word,
		Phonetic:       req.Phonetic,
		Definition:     req.Definition,
		Example:        req.Example,
		SourceContext:  req.SourceContext,
		ConfusingWords: req.ConfusingWords,
	}

	if err := DB.Create(&word).Error; err != nil {
		return nil, err
	}
	return &word, nil
}

// GetUserVocabulary 获取用户生词列表
func (s *VocabularyService) GetUserVocabulary(userID uint, keyword string, isMastered *bool) ([]model.Vocabulary, error) {
	var list []model.Vocabulary
	query := DB.Where("user_id = ?", userID)
	if keyword != "" {
		query = query.Where("word LIKE ?", "%"+keyword+"%")
	}
	if isMastered != nil {
		query = query.Where("is_mastered = ?", *isMastered)
	}
	if err := query.Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// GetVocabularyByIDs 按 id 列表查询用户的生词 (错题按词练习等场景, 避免为过滤少量词拉全量)
func (s *VocabularyService) GetVocabularyByIDs(userID uint, ids []uint) ([]model.Vocabulary, error) {
	if len(ids) == 0 {
		return []model.Vocabulary{}, nil
	}
	var list []model.Vocabulary
	if err := DB.Where("user_id = ? AND id IN ?", userID, ids).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// ===== SRS 间隔重复 (Leitner 盒子) =====

// srsIntervals 各盒子的下次复习间隔: 盒 0 当天 10 分钟, 之后 1/2/4/7/15 天
var srsIntervals = [6]time.Duration{
	0: 10 * time.Minute,
	1: 24 * time.Hour,
	2: 2 * 24 * time.Hour,
	3: 4 * 24 * time.Hour,
	4: 7 * 24 * time.Hour,
	5: 15 * 24 * time.Hour,
}

const srsMaxBox = 5

// GetDueWords 今日到期词 (next_review_at <= now, 含从未复习过的新词), 按到期时间升序;
// 已掌握 (is_mastered) 的词不进入复习队列
func (s *VocabularyService) GetDueWords(userID uint, limit int) ([]model.Vocabulary, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	now := time.Now()
	var list []model.Vocabulary
	err := DB.Where("user_id = ? AND is_mastered = ? AND (next_review_at IS NULL OR next_review_at <= ?)", userID, false, now).
		Order("next_review_at ASC, created_at ASC").
		Limit(limit).
		Find(&list).Error
	return list, err
}

// GetReviewStats 复习概况: 今日到期数 / 复习中的词数 / 各盒分布
// GetReviewStats 复习概况: 今日到期数 / 复习中的词数 / 各盒分布
// 口径与 GetDueWords 严格一致: 均排除已掌握 (is_mastered) 的词, 否则角标数与实际可复习数对不上
func (s *VocabularyService) GetReviewStats(userID uint) (map[string]interface{}, error) {
	now := time.Now()
	var dueCount, learningCount int64
	if err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ? AND is_mastered = ? AND (next_review_at IS NULL OR next_review_at <= ?)", userID, false, now).
		Count(&dueCount).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ? AND is_mastered = ? AND next_review_at IS NOT NULL", userID, false).
		Count(&learningCount).Error; err != nil {
		return nil, err
	}

	type boxRow struct {
		Box   int   `gorm:"column:box"`
		Count int64 `gorm:"column:cnt"`
	}
	var boxRows []boxRow
	if err := DB.Model(&model.Vocabulary{}).
		Select("srs_box as box, COUNT(*) as cnt").
		Where("user_id = ? AND is_mastered = ? AND next_review_at IS NOT NULL", userID, false).
		Group("srs_box").Scan(&boxRows).Error; err != nil {
		return nil, err
	}
	boxDist := map[string]int64{}
	for _, r := range boxRows {
		boxDist[fmt.Sprintf("%d", r.Box)] = r.Count
	}

	return map[string]interface{}{
		"dueCount":      dueCount,
		"learningCount": learningCount,
		"boxDist":       boxDist,
	}, nil
}

// SubmitReview 提交单个词的复习结果: 认识升盒, 遗忘回盒 0
func (s *VocabularyService) SubmitReview(userID, id uint, known bool) (*model.Vocabulary, error) {
	var word model.Vocabulary
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&word).Error; err != nil {
		return nil, errors.New("生词不存在")
	}

	box := word.SrsBox
	if known {
		box++
		if box > srsMaxBox {
			box = srsMaxBox
		}
	} else {
		box = 0
	}
	now := time.Now()
	next := now.Add(srsIntervals[box])

	updates := map[string]interface{}{
		"srs_box":          box,
		"next_review_at":   next,
		"last_reviewed_at": now,
	}
	// 升到最高盒视为已掌握; 遗忘回炉则取消已掌握标记 (重新进入复习队列)
	if box == srsMaxBox {
		updates["is_mastered"] = true
	} else if !known {
		updates["is_mastered"] = false
	}
	if err := DB.Model(&word).Updates(updates).Error; err != nil {
		return nil, err
	}
	DB.First(&word, word.ID)
	return &word, nil
}

// DeleteWord 删除生词
func (s *VocabularyService) DeleteWord(userID, id uint) error {
	return DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.Vocabulary{}).Error
}

// GetRandomWords 随机获取指定数量的生词
func (s *VocabularyService) GetRandomWords(userID uint, count int, isMastered *bool) ([]model.Vocabulary, error) {
	var list []model.Vocabulary
	query := DB.Where("user_id = ? AND example != '' AND example IS NOT NULL", userID)
	if isMastered != nil {
		query = query.Where("is_mastered = ?", *isMastered)
	}
	if err := query.Order("RAND()").Limit(count).Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

// UpdateWord 更新生词
func (s *VocabularyService) UpdateWord(userID, id uint, req model.UpdateVocabularyRequest) error {
	updates := map[string]interface{}{}
	if req.Phonetic != "" {
		updates["phonetic"] = req.Phonetic
	}
	if req.Definition != "" {
		updates["definition"] = req.Definition
	}
	if req.Example != "" {
		updates["example"] = req.Example
	}
	if req.ConfusingWords != "" {
		updates["confusing_words"] = req.ConfusingWords
	}
	if req.IsMastered != nil {
		updates["is_mastered"] = *req.IsMastered
	}

	return DB.Model(&model.Vocabulary{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(updates).Error
}
