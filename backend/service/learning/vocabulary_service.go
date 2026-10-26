package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"backend/model"
)

type VocabularyService struct {
	agentSvc *AIAgentService
	docSvc   *UserDocumentService
	prefSvc  *UserPreferenceService
}

// NewVocabularyService agentSvc/docSvc/prefSvc 用于遗忘干预、文档提词与每日新词限额
func NewVocabularyService(agentSvc *AIAgentService, docSvc *UserDocumentService, prefSvc *UserPreferenceService) *VocabularyService {
	return &VocabularyService{agentSvc: agentSvc, docSvc: docSvc, prefSvc: prefSvc}
}

// reviewDirections 双向卡方向轮换: 再认 → 主动回忆 → 听音辨词 → 原文语境 (有原句时)
var reviewDirections = []string{
	model.ReviewDirectionEn2Zh,
	model.ReviewDirectionZh2En,
	model.ReviewDirectionListen,
	model.ReviewDirectionContext,
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
		return nil, errors.New("该单词已存在")
	}

	word := model.Vocabulary{
		UserID:          userID,
		Word:            req.Word,
		Phonetic:        req.Phonetic,
		Definition:      req.Definition,
		Example:         req.Example,
		SourceContext:   req.SourceContext,
		ConfusingWords:  req.ConfusingWords,
		SourceKind:      req.SourceKind,
		SourceRef:       req.SourceRef,
		ContextSentence: req.ContextSentence,
	}

	if err := DB.Create(&word).Error; err != nil {
		return nil, err
	}
	LogReadingEvent(userID, "add_word", 1)
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

// getWordByID 取用户自己的单个生词
func (s *VocabularyService) getWordByID(userID, id uint) (*model.Vocabulary, error) {
	var word model.Vocabulary
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&word).Error; err != nil {
		return nil, errors.New("生词不存在")
	}
	return &word, nil
}

// ===== FSRS 间隔重复 =====

// getDailyNewLimit 每日新词限额 (用户偏好 english_daily_new_limit, 默认 20)
func (s *VocabularyService) getDailyNewLimit(userID uint) int {
	limit := 20
	if s.prefSvc != nil {
		if pref, err := s.prefSvc.GetPreference(userID, "english_daily_new_limit"); err == nil && pref != nil {
			var v int
			b, _ := json.Marshal(pref.PrefValue)
			if json.Unmarshal(b, &v) == nil && v >= 0 {
				limit = v
			}
		}
	}
	if limit > 200 {
		limit = 200
	}
	return limit
}

// dayStart 本地零点
func dayStart(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

// countNewStartedToday 今日已开始学习的新词数 (复习日志中 state_before=New 的条数)
func (s *VocabularyService) countNewStartedToday(userID uint) (int64, error) {
	var count int64
	err := DB.Model(&model.VocabularyReviewLog{}).
		Where("user_id = ? AND state_before = ? AND created_at >= ?", userID, model.FsrsStateNew, dayStart(time.Now())).
		Count(&count).Error
	return count, err
}

// buildCards 组装复习卡并轮换分配方向
func buildCards(words []model.Vocabulary) []model.ReviewCardItem {
	cards := make([]model.ReviewCardItem, 0, len(words))
	for i, w := range words {
		dir := reviewDirections[i%len(reviewDirections)]
		// 语境方向必须有原句, 没有就退回再认方向
		if dir == model.ReviewDirectionContext && strings.TrimSpace(w.ContextSentence) == "" {
			dir = model.ReviewDirectionEn2Zh
		}
		cards = append(cards, model.ReviewCardItem{Direction: dir, Word: w})
	}
	return cards
}

// GetDueWords 今日到期词 (旧接口: 含未排期新词), 按到期时间升序; 已掌握的词不进入队列
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

// GetReviewSession 复习会话: 到期复习词 + 受每日限额约束的新词
func (s *VocabularyService) GetReviewSession(userID uint, reviewLimit int) (*model.ReviewSessionResponse, error) {
	if reviewLimit <= 0 || reviewLimit > 100 {
		reviewLimit = 20
	}
	now := time.Now()

	var dueWords []model.Vocabulary
	if err := DB.Where("user_id = ? AND is_mastered = ? AND fsrs_state != ? AND next_review_at <= ?", userID, false, model.FsrsStateNew, now).
		Order("next_review_at ASC, fsrs_lapses DESC").
		Limit(reviewLimit).
		Find(&dueWords).Error; err != nil {
		return nil, err
	}

	newLimit := s.getDailyNewLimit(userID)
	startedToday, err := s.countNewStartedToday(userID)
	if err != nil {
		return nil, err
	}

	var newWords []model.Vocabulary
	if remaining := newLimit - int(startedToday); remaining > 0 {
		if err := DB.Where("user_id = ? AND is_mastered = ? AND fsrs_state = ?", userID, false, model.FsrsStateNew).
			Order("created_at ASC").
			Limit(remaining).
			Find(&newWords).Error; err != nil {
			return nil, err
		}
	}

	reviews := buildCards(dueWords)
	newCards := buildCards(newWords)
	estimated := (len(reviews) + len(newCards) + 3) / 4 // 每卡约 15 秒
	return &model.ReviewSessionResponse{
		Reviews:         reviews,
		NewWords:        newCards,
		NewLimit:        newLimit,
		NewStartedToday: int(startedToday),
		EstimatedMin:    estimated,
	}, nil
}

// GetQuickSession 碎片时间模式: 按遗忘次数 × 到期紧急度挑性价比最高的词
func (s *VocabularyService) GetQuickSession(userID uint, minutes int) (*model.ReviewSessionResponse, error) {
	if minutes <= 0 {
		minutes = 5
	}
	if minutes > 30 {
		minutes = 30
	}
	n := minutes * 3
	if n > 40 {
		n = 40
	}
	now := time.Now()

	var dueWords []model.Vocabulary
	if err := DB.Where("user_id = ? AND is_mastered = ? AND fsrs_state != ? AND next_review_at <= ?", userID, false, model.FsrsStateNew, now).
		Order("fsrs_lapses DESC, encounters ASC, next_review_at ASC").
		Limit(n).
		Find(&dueWords).Error; err != nil {
		return nil, err
	}

	newLimit := s.getDailyNewLimit(userID)
	startedToday, err := s.countNewStartedToday(userID)
	if err != nil {
		return nil, err
	}
	var newWords []model.Vocabulary
	if remaining := newLimit - int(startedToday); remaining > 0 {
		if fill := n - len(dueWords); fill > 0 {
			if err := DB.Where("user_id = ? AND is_mastered = ? AND fsrs_state = ?", userID, false, model.FsrsStateNew).
				Order("created_at ASC").
				Limit(fill).
				Find(&newWords).Error; err != nil {
				return nil, err
			}
		}
	}

	return &model.ReviewSessionResponse{
		Reviews:         buildCards(dueWords),
		NewWords:        buildCards(newWords),
		NewLimit:        newLimit,
		NewStartedToday: int(startedToday),
		EstimatedMin:    minutes,
	}, nil
}

// SubmitReview 提交单个词的复习结果 (旧接口: 认识/不认识 映射为 Good/Again)
func (s *VocabularyService) SubmitReview(userID, id uint, known bool) (*model.Vocabulary, error) {
	rating := 3
	if !known {
		rating = 1
	}
	return s.SubmitReviewFSRS(userID, id, rating, model.ReviewDirectionEn2Zh, 0)
}

// SubmitReviewFSRS 按 FSRS 调度提交复习结果, 记录复习日志 (事务)
func (s *VocabularyService) SubmitReviewFSRS(userID, id uint, rating int, direction string, durationMs int) (*model.Vocabulary, error) {
	word, err := s.getWordByID(userID, id)
	if err != nil {
		return nil, err
	}
	if direction == "" {
		direction = model.ReviewDirectionEn2Zh
	}

	now := time.Now()
	next := applyFsrsReview(word, rating, now)
	stateBefore := word.FsrsState
	mastered := fsrsCardIsMastered(next, rating)

	err = DB.Transaction(func(tx *gorm.DB) error {
		due := next.Due
		if err := tx.Model(&model.Vocabulary{}).Where("id = ?", word.ID).Updates(map[string]interface{}{
			"next_review_at":   due,
			"last_reviewed_at": now,
			"fsrs_stability":   next.Stability,
			"fsrs_difficulty":  next.Difficulty,
			"fsrs_state":       int(next.State),
			"fsrs_reps":        int(next.Reps),
			"fsrs_lapses":      int(next.Lapses),
			"is_mastered":      mastered,
		}).Error; err != nil {
			return err
		}
		return tx.Create(&model.VocabularyReviewLog{
			UserID:       userID,
			VocabularyID: word.ID,
			Direction:    direction,
			Rating:       rating,
			StateBefore:  stateBefore,
			Stability:    next.Stability,
			Difficulty:   next.Difficulty,
			Due:          &due,
			DurationMs:   durationMs,
		}).Error
	})
	if err != nil {
		return nil, err
	}
	return s.getWordByID(userID, id)
}

// GetReviewStats 复习概况: 今日到期数 / 复习中的词数 / FSRS 状态分布
// 口径与 GetReviewSession 一致: 均排除已掌握 (is_mastered) 的词
func (s *VocabularyService) GetReviewStats(userID uint) (map[string]interface{}, error) {
	now := time.Now()
	var dueCount, learningCount int64
	if err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ? AND is_mastered = ? AND fsrs_state != ? AND next_review_at <= ?", userID, false, model.FsrsStateNew, now).
		Count(&dueCount).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ? AND is_mastered = ? AND fsrs_state != ?", userID, false, model.FsrsStateNew).
		Count(&learningCount).Error; err != nil {
		return nil, err
	}

	type stateRow struct {
		State int   `gorm:"column:state"`
		Count int64 `gorm:"column:cnt"`
	}
	var stateRows []stateRow
	if err := DB.Model(&model.Vocabulary{}).
		Select("fsrs_state as state, COUNT(*) as cnt").
		Where("user_id = ? AND is_mastered = ? AND fsrs_state != ?", userID, false, model.FsrsStateNew).
		Group("fsrs_state").Scan(&stateRows).Error; err != nil {
		return nil, err
	}
	stateDist := map[string]int64{}
	for _, r := range stateRows {
		stateDist[fmt.Sprintf("%d", r.State)] = r.Count
	}

	return map[string]interface{}{
		"dueCount":      dueCount,
		"learningCount": learningCount,
		"stateDist":     stateDist,
	}, nil
}

// GetDueForecast 未来 N 天到期预测 (含未排期新词数)
func (s *VocabularyService) GetDueForecast(userID uint, days int) (map[string]interface{}, error) {
	if days <= 0 || days > 90 {
		days = 30
	}
	now := time.Now()
	end := now.AddDate(0, 0, days)

	type dateRow struct {
		Date string `gorm:"column:date"`
		Count int64 `gorm:"column:cnt"`
	}
	var rows []dateRow
	if err := DB.Model(&model.Vocabulary{}).
		Select("DATE(next_review_at) as date, COUNT(*) as cnt").
		Where("user_id = ? AND is_mastered = ? AND next_review_at IS NOT NULL AND next_review_at BETWEEN ? AND ?", userID, false, now, end).
		Group("DATE(next_review_at)").
		Order("date ASC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}
	forecast := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		forecast = append(forecast, map[string]interface{}{"date": r.Date, "count": r.Count})
	}

	var newCount int64
	if err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ? AND is_mastered = ? AND fsrs_state = ?", userID, false, model.FsrsStateNew).
		Count(&newCount).Error; err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"days":     days,
		"forecast": forecast,
		"newWords": newCount,
	}, nil
}

// GetLearningStats 学习看板: 总量/热力图/保持率/连续天数/易错词
func (s *VocabularyService) GetLearningStats(userID uint) (map[string]interface{}, error) {
	now := time.Now()
	stats := map[string]interface{}{}

	var total, mastered, newCount int64
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ? AND is_mastered = ?", userID, true).Count(&mastered).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ? AND is_mastered = ? AND fsrs_state = ?", userID, false, model.FsrsStateNew).Count(&newCount).Error; err != nil {
		return nil, err
	}

	// 近 90 天复习热力图
	type dayRow struct {
		Date string `gorm:"column:date"`
		Count int64 `gorm:"column:cnt"`
	}
	var heatRows []dayRow
	if err := DB.Model(&model.VocabularyReviewLog{}).
		Select("DATE(created_at) as date, COUNT(*) as cnt").
		Where("user_id = ? AND created_at >= ?", userID, now.AddDate(0, 0, -90)).
		Group("DATE(created_at)").Order("date ASC").Scan(&heatRows).Error; err != nil {
		return nil, err
	}
	heatmap := make([]map[string]interface{}, 0, len(heatRows))
	reviewDates := map[string]bool{}
	for _, r := range heatRows {
		heatmap = append(heatmap, map[string]interface{}{"date": r.Date, "count": r.Count})
		reviewDates[r.Date] = true
	}

	// 近 30 天保持率: 评分为 Good 及以上 (rating >= 3) 的比例
	var total30, good30 int64
	if err := DB.Model(&model.VocabularyReviewLog{}).
		Where("user_id = ? AND created_at >= ?", userID, now.AddDate(0, 0, -30)).
		Count(&total30).Error; err != nil {
		return nil, err
	}
	if err := DB.Model(&model.VocabularyReviewLog{}).
		Where("user_id = ? AND created_at >= ? AND rating >= 3", userID, now.AddDate(0, 0, -30)).
		Count(&good30).Error; err != nil {
		return nil, err
	}
	retention := 0.0
	if total30 > 0 {
		retention = float64(good30) / float64(total30)
	}

	// 连续复习天数: 从今天 (或昨天) 往回数连续有复习记录的天数
	streak := 0
	cursor := now
	if !reviewDates[cursor.Format("2006-01-02")] {
		cursor = cursor.AddDate(0, 0, -1)
	}
	for reviewDates[cursor.Format("2006-01-02")] {
		streak++
		cursor = cursor.AddDate(0, 0, -1)
	}

	// 易错词 Top10 (按遗忘次数; 相同遗忘次数优先"阅读中从未再遇到"的词——语境最弱最易忘)
	var troubleWords []model.Vocabulary
	if err := DB.Where("user_id = ? AND is_mastered = ? AND fsrs_lapses >= 1", userID, false).
		Order("fsrs_lapses DESC, encounters ASC, next_review_at ASC").
		Limit(10).
		Find(&troubleWords).Error; err != nil {
		return nil, err
	}

	// 每掌握一个词的平均复习次数
	var totalReviews int64
	if err := DB.Model(&model.VocabularyReviewLog{}).Where("user_id = ?", userID).Count(&totalReviews).Error; err != nil {
		return nil, err
	}
	avgPerMastered := 0.0
	if mastered > 0 {
		avgPerMastered = float64(totalReviews) / float64(mastered)
	}

	stats["total"] = total
	stats["mastered"] = mastered
	stats["newWords"] = newCount
	stats["heatmap"] = heatmap
	stats["retention30"] = retention
	stats["reviewCount30"] = total30
	stats["streak"] = streak
	stats["troubleWords"] = troubleWords
	stats["totalReviews"] = totalReviews
	stats["avgReviewsPerMastered"] = avgPerMastered
	// 阅读维度 (打开/查词/加词/阅读分钟, 近90天): 阅读事件埋点聚合
	if reading, err := GetReadingStats(userID); err == nil {
		stats["reading"] = reading
	}
	// 练习维度 (近30天): 题数/正确数/正确率/会话数
	var quizAgg struct {
		Sessions int64 `gorm:"column:sessions"`
		Answered int64 `gorm:"column:answered"`
		Correct  int64 `gorm:"column:correct"`
	}
	if err := DB.Model(&model.QuizSession{}).
		Select("COUNT(*) as sessions, COALESCE(SUM(answered),0) as answered, COALESCE(SUM(correct),0) as correct").
		Where("user_id = ? AND created_at >= ?", userID, now.AddDate(0, 0, -30)).
		Scan(&quizAgg).Error; err == nil {
		accuracy := 0.0
		if quizAgg.Answered > 0 {
			accuracy = float64(quizAgg.Correct) / float64(quizAgg.Answered)
		}
		stats["quiz"] = map[string]interface{}{
			"sessions": quizAgg.Sessions,
			"answered": quizAgg.Answered,
			"correct":  quizAgg.Correct,
			"accuracy": accuracy,
		}
	}
	return stats, nil
}

// ===== 遗忘干预 =====

// AssistForgetfulWord 为反复遗忘的词生成新例句与助记提示 (结果缓存到词记录上)
func (s *VocabularyService) AssistForgetfulWord(userID, id uint) (*model.Vocabulary, error) {
	word, err := s.getWordByID(userID, id)
	if err != nil {
		return nil, err
	}
	if word.AssistExample != "" && word.AssistMnemonic != "" {
		return word, nil
	}
	if s.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}

	system := GetPrompt(PromptKeyVocabAssist)
	user := fmt.Sprintf("单词: %s\n释义: %s\n已有例句(请避开类似场景): %s",
		word.Word, word.Definition, word.Example)
	raw, err := s.agentSvc.GenerateText(userID, "", system, user)
	if err != nil {
		return nil, fmt.Errorf("生成记忆辅助失败: %w", err)
	}

	var assist struct {
		Example  string `json:"example"`
		Mnemonic string `json:"mnemonic"`
	}
	if err := decodeLLMObject(raw, &assist); err != nil || assist.Example == "" {
		return nil, errors.New("AI 返回格式异常 (详见后端日志)")
	}
	if err := DB.Model(&model.Vocabulary{}).Where("id = ?", word.ID).Updates(map[string]interface{}{
		"assist_example":  assist.Example,
		"assist_mnemonic": assist.Mnemonic,
	}).Error; err != nil {
		return nil, err
	}
	return s.getWordByID(userID, id)
}

// ===== 词书批量导入 =====

// ImportWords 批量导入生词 (词书 CSV/TXT 解析后的结构化条目), 已存在的跳过
func (s *VocabularyService) ImportWords(userID uint, items []model.ImportWordItem, sourceTag string) (*model.ImportWordsResult, error) {
	result := &model.ImportWordsResult{Failed: []string{}}
	if len(items) == 0 {
		return result, nil
	}
	if len(items) > 2000 {
		items = items[:2000]
	}

	var existing []string
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Pluck("word", &existing).Error; err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(existing))
	for _, w := range existing {
		seen[strings.ToLower(w)] = true
	}

	batch := make([]model.Vocabulary, 0, 100)
	for _, item := range items {
		word := strings.TrimSpace(item.Word)
		if word == "" || len(word) > 100 {
			result.Failed = append(result.Failed, item.Word)
			continue
		}
		key := strings.ToLower(word)
		if seen[key] {
			result.Skipped++
			continue
		}
		seen[key] = true
		batch = append(batch, model.Vocabulary{
			UserID:        userID,
			Word:          word,
			Phonetic:      item.Phonetic,
			Definition:    item.Definition,
			Example:       item.Example,
			SourceContext: sourceTag,
		})
		result.Added++
	}

	if len(batch) > 0 {
		if err := DB.CreateInBatches(&batch, 100).Error; err != nil {
			return nil, err
		}
	}
	return result, nil
}

// ExtractWordsFromDocument 从知识库文档提取超纲生词并入库 (LLM 提取, 已有词自动跳过)
func (s *VocabularyService) ExtractWordsFromDocument(userID, documentID uint, limit int) (*model.ImportWordsResult, error) {
	if s.docSvc == nil || s.agentSvc == nil {
		return nil, errors.New("文档服务或 AI 服务未启用")
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}

	chunk, _, _, err := s.docSvc.ReadDocumentChunk(userID, documentID, 0, 20000)
	if err != nil {
		return nil, fmt.Errorf("读取文档失败: %w", err)
	}

	var existing []string
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Pluck("word", &existing).Error; err != nil {
		return nil, err
	}
	knownList := strings.Join(existing, ", ")
	if len(knownList) > 6000 {
		knownList = knownList[:6000]
	}

	system := GetPrompt(PromptKeyVocabExtract)
	user := fmt.Sprintf("文章:\n%s\n\n用户已有的生词本单词 (这些不要重复提取):\n%s\n\n最多提取 %d 个生词。",
		chunk, knownList, limit)
	raw, err := s.agentSvc.GenerateText(userID, "", system, user)
	if err != nil {
		return nil, fmt.Errorf("提取生词失败: %w", err)
	}

	var items []model.ImportWordItem
	if err := decodeLLMArray(raw, &items); err != nil {
		return nil, errors.New("提取生词失败: AI 返回格式异常 (详见后端日志)")
	}
	return s.ImportWords(userID, items, "文档提取")
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
