package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"backend/model"
)

// ===== 阅读↔记忆闭环 / 难度预估 / 划词翻译 / 词汇量测试 / 阅读埋点 =====

var estWordTokenRe = regexp.MustCompile(`[A-Za-z][A-Za-z'-]*`)

// estimateUnknownRate 免 LLM 的个性化难度预估:
// 优先口径 = 文章独特词 ∩ 用户未掌握生词本 的比例 (真正"你大概率不认识的词");
// 生词本为空时回退长词启发 (长度>=8 的独特词占比 × 0.5)。返回 [0,1]。
func estimateUnknownRate(userID uint, text string) float64 {
	set, err := loadUnmasteredSet(userID)
	if err != nil {
		return 0
	}
	return estimateUnknownRateFromSet(text, set)
}

// loadUnmasteredSet 用户未掌握生词集合 (小写)
func loadUnmasteredSet(userID uint) (map[string]bool, error) {
	var words []string
	err := DB.Model(&model.Vocabulary{}).
		Where("user_id = ? AND is_mastered = ?", userID, false).
		Pluck("LOWER(word)", &words).Error
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set, nil
}

// estimateUnknownRateFromSet 与 loadUnmasteredSet 配套 (批量场景查一次集合复用)
func estimateUnknownRateFromSet(text string, unknownSet map[string]bool) float64 {
	tokens := estWordTokenRe.FindAllString(strings.ToLower(text), -1)
	if len(tokens) < 20 {
		return 0 // 太短没有统计意义
	}
	distinct := make(map[string]bool, len(tokens))
	for _, t := range tokens {
		distinct[t] = true
	}
	hits, longWords := 0, 0
	for w := range distinct {
		if unknownSet[w] {
			hits++
		}
		if len(w) >= 8 {
			longWords++
		}
	}
	d := float64(len(distinct))
	if len(unknownSet) > 0 {
		r := float64(hits) / d * 1.2 // distinct 命中略放大逼近"出现次数"口径
		if r > 1 {
			r = 1
		}
		return r
	}
	r := float64(longWords) / d * 0.5
	if r > 1 {
		r = 1
	}
	return r
}

// ===== 阅读行为埋点 (按日聚合, gorm OnConflict 兼容 MySQL/SQLite) =====

// LogReadingEvent 记阅读行为: kind = open / lookup / add_word / read_seconds(n)
func LogReadingEvent(userID uint, kind string, n int) {
	if n <= 0 {
		n = 1
	}
	stat := model.ReadingDailyStat{UserID: userID, StatDate: time.Now().Format("2006-01-02")}
	col := ""
	switch kind {
	case "open":
		col, stat.Opens = "opens", n
	case "lookup":
		col, stat.Lookups = "lookups", n
	case "add_word":
		col, stat.WordsAdded = "words_added", n
	case "read_seconds":
		col, stat.ReadSeconds = "read_seconds", n
	default:
		return
	}
	if err := DB.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "stat_date"}},
		DoUpdates: clause.Assignments(map[string]interface{}{col: gorm.Expr(col + " + ?", n)}),
	}).Create(&stat).Error; err != nil {
		log.Printf("reading stat %s user=%d: %v", kind, userID, err)
	}
}

// ReadingStatsSummary 阅读维度统计 (学习看板用)
type ReadingStatsSummary struct {
	TotalOpens      int64                     `json:"totalOpens"`
	TotalLookups    int64                     `json:"totalLookups"`
	TotalWordsAdded int64                     `json:"totalWordsAdded"`
	TotalReadMinutes int64                    `json:"totalReadMinutes"`
	Daily           []map[string]interface{} `json:"daily"` // 近90天按日
}

// GetReadingStats 近 90 天阅读行为聚合
func GetReadingStats(userID uint) (*ReadingStatsSummary, error) {
	out := &ReadingStatsSummary{Daily: []map[string]interface{}{}}
	since := time.Now().AddDate(0, 0, -90).Format("2006-01-02")
	var rows []model.ReadingDailyStat
	if err := DB.Where("user_id = ? AND stat_date >= ?", userID, since).
		Order("stat_date ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		out.TotalOpens += int64(r.Opens)
		out.TotalLookups += int64(r.Lookups)
		out.TotalWordsAdded += int64(r.WordsAdded)
		out.TotalReadMinutes += int64(r.ReadSeconds) / 60
		out.Daily = append(out.Daily, map[string]interface{}{
			"date": r.StatDate, "opens": r.Opens, "lookups": r.Lookups,
			"wordsAdded": r.WordsAdded, "readMinutes": r.ReadSeconds / 60,
		})
	}
	return out, nil
}

// ===== LookupWord 扩展: 返回是否已在生词本 + 阅读相遇计数 =====

// LookupWordResponse 查词响应: 词条 + 用户维度状态
type LookupWordResponse struct {
	model.DictionaryCache
	InVocabulary bool `json:"inVocabulary"`
	Encounters   int  `json:"encounters"`
}

// attachVocabularyState 查词命中生词本时计一次阅读相遇 (阅读中再次见到)
func attachVocabularyState(userID uint, resp *LookupWordResponse, word string) {
	var v model.Vocabulary
	if err := DB.Where("user_id = ? AND LOWER(word) = LOWER(?)", userID, word).First(&v).Error; err != nil {
		return
	}
	v.Encounters += 1
	if err := DB.Model(&model.Vocabulary{}).Where("id = ?", v.ID).
		UpdateColumn("encounters", v.Encounters).Error; err != nil {
		return
	}
	resp.InVocabulary = true
	resp.Encounters = v.Encounters
}

// ===== 免费词典优先 (查词提速): dictionaryapi.dev 命中则 LLM 只补中文释义 =====

type freeDictEntry struct {
	Phonetic string `json:"phonetic"`
	Meanings []struct {
		Definitions []struct {
			Definition string `json:"definition"`
			Example    string `json:"example"`
		} `json:"definitions"`
	} `json:"meanings"`
}

// lookupFreeDictionary 免费词典查询 (仅限单词, 5s 超时; 固定公网域名无 SSRF 面)
func lookupFreeDictionary(word string) (phonetic, definition, example string, ok bool) {
	if strings.ContainsAny(word, " -'") || len(word) > 30 || len(word) < 2 {
		return "", "", "", false
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://api.dictionaryapi.dev/api/v2/entries/en/" + url.PathEscape(word))
	if err != nil {
		return "", "", "", false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", "", false
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", "", "", false
	}
	var entries []freeDictEntry
	if err := json.Unmarshal(body, &entries); err != nil || len(entries) == 0 {
		return "", "", "", false
	}
	e := entries[0]
	if e.Phonetic != "" {
		phonetic = e.Phonetic
	}
	for _, m := range e.Meanings {
		if len(m.Definitions) > 0 {
			definition = m.Definitions[0].Definition
			example = m.Definitions[0].Example
			break
		}
	}
	if definition == "" {
		return "", "", "", false
	}
	return phonetic, definition, example, true
}

// ===== 划词翻译 (ReaderView 选中文本) =====

// TranslateText 双向翻译: 中文→英文, 其他→中文。短文本同步调用 (数秒, CF 100s 内安全)
func (s *ReadingService) TranslateText(userID uint, text string) (string, error) {
	if s.agentSvc == nil {
		return "", errors.New("AI 服务未启用")
	}
	system := GetPrompt(PromptKeyTranslate)
	raw, err := s.agentSvc.GenerateText(userID, "", system, text)
	if err != nil {
		return "", fmt.Errorf("翻译失败: %w", err)
	}
	return strings.TrimSpace(raw), nil
}

// ===== 词汇量测试 =====

var vocabTestBands = []string{"A1", "A2", "B1", "B2", "C1", "C2"}
var vocabTestAnchors = map[string]int{"A1": 500, "A2": 1500, "B1": 3000, "B2": 5000, "C1": 8000, "C2": 15000}

// StartVocabTest 异步生成一套分层词汇量测试题 (6 频段 × 4 题)
func (s *ReadingService) StartVocabTest(userID uint) (*model.AITextJob, error) {
	if s.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	job := &model.AITextJob{UserID: userID, Kind: "vocab_test", Status: "running"}
	if err := DB.Create(job).Error; err != nil {
		return nil, err
	}
	SafeGo("reading:vocab-test", func() {
		updates := map[string]interface{}{}
		questions, err := s.generateVocabTest(userID)
		if err != nil {
			updates["status"] = "failed"
			updates["error"] = err.Error()
		} else {
			raw, _ := json.Marshal(questions)
			updates["status"] = "done"
			updates["result_json"] = string(raw)
		}
		if err := DB.Model(&model.AITextJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
			log.Printf("vocab test job #%d update failed: %v", job.ID, err)
		}
	})
	return job, nil
}

func (s *ReadingService) generateVocabTest(userID uint) ([]model.VocabTestQuestion, error) {
	system := GetPrompt(PromptKeyVocabTest)
	raw, err := s.agentSvc.GenerateTextWithTimeout(userID, "", system, "请出题。", slowGenTimeout)
	if err != nil {
		return nil, err
	}
	var questions []model.VocabTestQuestion
	if err := decodeLLMArray(raw, &questions); err != nil {
		return nil, errors.New("出题格式异常 (详见后端日志)")
	}
	if len(questions) < 12 {
		return nil, fmt.Errorf("出题数量不足: %d", len(questions))
	}
	for i := range questions {
		if questions[i].ID == "" {
			questions[i].ID = fmt.Sprintf("q-%d", i)
		}
	}
	return questions, nil
}

// VocabTestResult 测试结果
type VocabTestResult struct {
	CEFR    string            `json:"cefr"`
	EstSize int               `json:"estSize"`
	Bands   map[string]string `json:"bands"` // band -> "正确数/总数"
}

// SubmitVocabTest 提交答卷: 按频段正确率取"最高达标频段"定级, 锚点插值估算词汇量
func (s *ReadingService) SubmitVocabTest(userID uint, req model.VocabTestSubmitRequest) (*VocabTestResult, error) {
	var job model.AITextJob
	if err := DB.Where("id = ? AND user_id = ? AND kind = ?", req.JobID, userID, "vocab_test").First(&job).Error; err != nil {
		return nil, errors.New("测试不存在")
	}
	if job.Status != "done" || job.ResultJSON == "" {
		return nil, errors.New("测试题尚未生成完成")
	}
	var questions []model.VocabTestQuestion
	if err := json.Unmarshal([]byte(job.ResultJSON), &questions); err != nil {
		return nil, errors.New("测试数据损坏")
	}
	bandCorrect := map[string]int{}
	bandTotal := map[string]int{}
	for _, q := range questions {
		bandTotal[q.Band]++
		if ans, ok := req.Answers[q.ID]; ok && ans == q.Answer {
			bandCorrect[q.Band]++
		}
	}
	// 从高到低找第一个正确率 >= 50% 的频段
	cefr := "A1"
	acc := 0.0
	for i := len(vocabTestBands) - 1; i >= 0; i-- {
		b := vocabTestBands[i]
		if bandTotal[b] == 0 {
			continue
		}
		rate := float64(bandCorrect[b]) / float64(bandTotal[b])
		if rate >= 0.5 {
			cefr = b
			acc = rate
			break
		}
	}
	anchor := vocabTestAnchors[cefr]
	est := int(float64(anchor) * (0.7 + 0.3*acc))
	if cefr == "A1" && acc < 0.5 {
		est = int(float64(anchor) * 0.6)
	}

	bands := map[string]string{}
	for _, b := range vocabTestBands {
		if bandTotal[b] > 0 {
			bands[b] = fmt.Sprintf("%d/%d", bandCorrect[b], bandTotal[b])
		}
	}
	detail, _ := json.Marshal(bands)
	if err := DB.Create(&model.VocabularyAssessment{
		UserID: userID, CEFR: cefr, EstSize: est, Detail: string(detail),
	}).Error; err != nil {
		return nil, err
	}
	return &VocabTestResult{CEFR: cefr, EstSize: est, Bands: bands}, nil
}

// GetLatestVocabTest 最近一次词汇量评估 (课文生成难度校准也用它)
func GetLatestVocabTest(userID uint) (*model.VocabularyAssessment, error) {
	var a model.VocabularyAssessment
	if err := DB.Where("user_id = ?", userID).Order("created_at DESC").First(&a).Error; err != nil {
		return nil, errors.New("还没有测试记录")
	}
	return &a, nil
}

// stripVocabTestAnswers 下发题面前剥离正确答案 (选项顺序保持与 answer 下标一致, 不重排)
func stripVocabTestQuestions(resultJSON string) []map[string]interface{} {
	var questions []model.VocabTestQuestion
	if err := json.Unmarshal([]byte(resultJSON), &questions); err != nil {
		return nil
	}
	out := make([]map[string]interface{}, 0, len(questions))
	for _, q := range questions {
		options := append([]string(nil), q.Options...)
		out = append(out, map[string]interface{}{
			"id": q.ID, "band": q.Band, "word": q.Word, "options": options,
		})
	}
	return out
}
