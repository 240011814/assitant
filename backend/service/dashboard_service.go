package service

import (
	"backend/model"
	"log"
	"time"
)

type DashboardService struct{}

func NewDashboardService() *DashboardService {
	return &DashboardService{}
}

func (s *DashboardService) GetStats(userID uint) (*model.DashboardStats, error) {
	stats := &model.DashboardStats{}

	// 今日对话消息数量
	today := time.Now().Format("2006-01-02")
	if err := DB.Model(&model.TrainingMessage{}).
		Joins("JOIN training_histories ON training_histories.id = training_messages.history_id").
		Where("training_histories.user_id = ? AND DATE(training_messages.created_at) = ?", userID, today).
		Count(&stats.TodayMessages).Error; err != nil {
		log.Printf("[Dashboard] 统计今日消息失败 user=%d: %v", userID, err)
	}

	// 累计对话消息数量
	if err := DB.Model(&model.TrainingMessage{}).
		Joins("JOIN training_histories ON training_histories.id = training_messages.history_id").
		Where("training_histories.user_id = ?", userID).
		Count(&stats.TotalMessages).Error; err != nil {
		log.Printf("[Dashboard] 统计累计消息失败 user=%d: %v", userID, err)
	}

	// 生词本词汇量
	if err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ?", userID).
		Count(&stats.TotalVocabulary).Error; err != nil {
		log.Printf("[Dashboard] 统计词汇量失败 user=%d: %v", userID, err)
	}

	// 笔记数量
	if err := DB.Model(&model.Note{}).
		Where("user_id = ?", userID).
		Count(&stats.TotalNotes).Error; err != nil {
		log.Printf("[Dashboard] 统计笔记失败 user=%d: %v", userID, err)
	}

	// 收藏数量
	if err := DB.Model(&model.TrainingHistory{}).
		Where("user_id = ? AND is_favorite = ?", userID, true).
		Count(&stats.TotalFavorites).Error; err != nil {
		log.Printf("[Dashboard] 统计收藏失败 user=%d: %v", userID, err)
	}

	// 近7天消息趋势: 一条 GROUP BY 聚合 (原循环 7 次单日 Count), 应用侧补齐无数据的日期
	type trendRow struct {
		Day   string `gorm:"column:day"`
		Count int64  `gorm:"column:cnt"`
	}
	var trendRows []trendRow
	if err := DB.Model(&model.TrainingMessage{}).
		Joins("JOIN training_histories ON training_histories.id = training_messages.history_id").
		Where("training_histories.user_id = ? AND DATE(training_messages.created_at) >= ?", userID, time.Now().AddDate(0, 0, -6).Format("2006-01-02")).
		Select("DATE(training_messages.created_at) as day, COUNT(*) as cnt").
		Group("day").
		Scan(&trendRows).Error; err != nil {
		log.Printf("[Dashboard] 统计趋势失败 user=%d: %v", userID, err)
	}
	trendMap := make(map[string]int64, len(trendRows))
	for _, r := range trendRows {
		trendMap[r.Day] = r.Count
	}
	for i := 6; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		stats.TrainingTrend = append(stats.TrainingTrend, model.TrendItem{
			Date:  date,
			Count: trendMap[date],
		})
	}

	// 训练类型分布
	type TypeResult struct {
		TrainingType string `gorm:"column:training_type"`
		Count        int64  `gorm:"column:count"`
	}
	var typeResults []TypeResult
	if err := DB.Model(&model.TrainingHistory{}).
		Select("training_type, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("training_type").
		Scan(&typeResults).Error; err != nil {
		log.Printf("[Dashboard] 统计训练类型失败 user=%d: %v", userID, err)
	}

	typeNameMap := map[string]string{
		"ai_chat":      "英语训练",
		"ai_decision":  "决策训练",
		"ai_social":    "社交训练",
		"ai_emergency": "应急训练",
	}

	for _, r := range typeResults {
		name := typeNameMap[r.TrainingType]
		if name == "" {
			name = r.TrainingType
		}
		stats.TrainingTypeStats = append(stats.TrainingTypeStats, model.TypeStatItem{
			Type:  name,
			Count: r.Count,
		})
	}

	return stats, nil
}
