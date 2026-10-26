package learning

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"backend/model"
	"backend/service/kernel"
)

// ===== 智能练习扩展: 判分升级 / FSRS 回流 / 错题消错 / 文章翻译题 / 相近干扰项 / 会话管理 =====

// spellTolerantEqual 拼写容错: 精确相等 / 去尾部屈折变化(s/es/ies/ed)后相等 / 相似度 >= 0.85
func spellTolerantEqual(answer, truth string) bool {
	na, nt := normalizeAnswer(answer), normalizeAnswer(truth)
	if na == nt {
		return true
	}
	stripSuffix := func(s string) string {
		for _, suf := range []string{"ies", "es", "s", "ed"} {
			if strings.HasSuffix(s, suf) && len(s) > len(suf)+1 {
				return strings.TrimSuffix(s, suf)
			}
		}
		return s
	}
	if stripSuffix(na) == stripSuffix(nt) {
		return true
	}
	return textSimilarity(na, nt) >= 0.85
}

// jaccardWordOverlap 词级重叠率 (translate 判分: 换序/少量漏词不受惩罚)
func jaccardWordOverlap(a, b string) float64 {
	wa := strings.Fields(normalizeAnswer(a))
	wb := strings.Fields(normalizeAnswer(b))
	if len(wa) == 0 && len(wb) == 0 {
		return 1
	}
	sa := make(map[string]bool, len(wa))
	for _, w := range wa {
		sa[w] = true
	}
	inter := 0
	for _, w := range wb {
		if sa[w] {
			inter++
		}
	}
	union := len(sa) + len(wb) - inter
	if union == 0 {
		return 1
	}
	return float64(inter) / float64(union)
}

// gradeTranslateV2 翻译判分: 词级重叠 >= 0.7 或编辑距离相似度 >= 0.88 (双通道)
func gradeTranslateV2(answer, truth string) bool {
	return jaccardWordOverlap(answer, truth) >= 0.7 || textSimilarity(answer, truth) >= 0.88
}

// DecayError 答对消错: 错题 error_count-1, 归零标记已消灭 (is_mastered)
func (s *ErrorBookService) DecayError(userID uint, contentType, content string) bool {
	var eb model.ErrorBook
	if err := DB.Where("user_id = ? AND content_type = ? AND content = ? AND is_mastered = ?",
		userID, contentType, content, false).First(&eb).Error; err != nil {
		return false
	}
	eb.ErrorCount--
	if eb.ErrorCount <= 0 {
		eb.ErrorCount = 0
		eb.IsMastered = true
	}
	return DB.Save(&eb).Error == nil
}

// quizFSRSFeedbackEnabled 练习成绩回流 FSRS 的开关 (system_config quiz_fsrs_feedback, 默认开)
func (s *QuizService) quizFSRSFeedbackEnabled() bool {
	if s.systemCfg == nil {
		return true
	}
	if v, err := s.systemCfg.GetValue("quiz_fsrs_feedback"); err == nil {
		return strings.TrimSpace(v) != "0"
	}
	return true
}

// applyFSRSFeedback 把答题结果映射为一次 FSRS 复习: 答错=Again / 秒答对=Easy / 答对=Good
func (s *QuizService) applyFSRSFeedback(userID uint, q *model.QuizQuestion, correct bool, durationMs int) {
	if !s.quizFSRSFeedbackEnabled() {
		return
	}
	if s.vocabSvc == nil || q.WordID == 0 {
		return
	}
	rating := 3
	if !correct {
		rating = 1
	} else if durationMs > 0 && durationMs < 5000 {
		rating = 4
	}
	_, _ = s.vocabSvc.SubmitReviewFSRS(userID, q.WordID, rating, "quiz", durationMs)
}

// explanationFor 答错讲解: 词义 + 例句 + 易混对比 (数据齐全时组装一段)
func explanationFor(q *model.QuizQuestion) string {
	if q.WordID == 0 || q.Word == "" {
		if q.Type == "translate" {
			return "参考答案见上; 可以回看课程/文章原句对比语序与用词。"
		}
		return ""
	}
	var w model.Vocabulary
	if err := DB.Where("id = ?", q.WordID).First(&w).Error; err != nil {
		return ""
	}
	parts := []string{}
	if w.Definition != "" {
		parts = append(parts, fmt.Sprintf("「%s」: %s", w.Word, w.Definition))
	}
	if w.Example != "" {
		parts = append(parts, "例句: "+w.Example)
	}
	if w.ConfusingWords != "" && q.Type != "confusing" {
		parts = append(parts, "易混词: "+w.ConfusingWords)
	}
	return strings.Join(parts, "\n")
}

// sortDistractorsBySimilarity 把干扰项按与目标词的形态相近度排序 (首字母相同/长度接近优先),
// 保持 (词, 释义) 配对不变, 调用方按此顺序取干扰项即得"更有区分度"的选项
func sortDistractorsBySimilarity(target string, distractors []model.Vocabulary) []model.Vocabulary {
	score := func(w model.Vocabulary) int {
		d := len(w.Word) - len(target)
		if d < 0 {
			d = -d
		}
		s := 0
		if len(w.Word) > 0 && len(target) > 0 && w.Word[0] == target[0] {
			s -= 2 // 首字母相同优先
		}
		if d <= 3 {
			s -= 1 // 长度接近优先
		}
		return s
	}
	sorted := append([]model.Vocabulary(nil), distractors...)
	sort.SliceStable(sorted, func(i, j int) bool { return score(sorted[i]) < score(sorted[j]) })
	return sorted
}

// ===== 文章翻译题: 从用户阅读文档抽英文句, LLM 译中做题面 (quiz_sentence_zh 全局缓存) =====

// pickDocSentences 从用户阅读文档随机抽英文句子 (8~25 词, 以句号结尾)
func pickDocSentences(userID uint, n int) []string {
	var docs []model.ReadingDoc
	if err := DB.Select("content").Where("user_id = ?", userID).
		Order("created_at DESC").Limit(20).Find(&docs).Error; err != nil {
		return nil
	}
	sentences := []string{}
	seen := map[string]bool{}
	for _, doc := range docs {
		for _, para := range strings.Split(doc.Content, "\n") {
			for _, sent := range strings.Split(para, ". ") {
				sent = strings.TrimSpace(sent)
				if !strings.HasSuffix(sent, ".") {
					sent += "."
				}
				words := strings.Fields(sent)
				if len(words) < 8 || len(words) > 25 || seen[sent] {
					continue
				}
				if !isMostlyEnglish(sent) {
					continue
				}
				seen[sent] = true
				sentences = append(sentences, sent)
				if len(sentences) >= n {
					return sentences
				}
			}
		}
	}
	return sentences
}

func isMostlyEnglish(s string) bool {
	letters := 0
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			letters++
		}
	}
	return letters*2 > len([]rune(s))
}

// translateSentenceZH 单句翻译 (缓存优先, 未命中 LLM)
func (s *QuizService) translateSentenceZH(userID uint, sentence string) (string, error) {
	var cached model.QuizSentenceZH
	if err := DB.Where("sentence = ?", sentence).First(&cached).Error; err == nil {
		return cached.ZH, nil
	}
	if s.agentSvc == nil {
		return "", errors.New("AI 服务未启用")
	}
	raw, err := s.agentSvc.GenerateText(userID, "", "把这句英语翻译成简体中文, 只输出译文本身。", sentence)
	if err != nil {
		return "", err
	}
	zh := strings.TrimSpace(kernel.TruncateRunes(raw, 1000))
	if zh == "" {
		return "", errors.New("翻译为空")
	}
	cache := model.QuizSentenceZH{Sentence: kernel.TruncateRunes(sentence, 1000), ZH: zh}
	if err := DB.Create(&cache).Error; err != nil {
		// 并发写冲突 (唯一键) 时重读
		if err2 := DB.Where("sentence = ?", sentence).First(&cached).Error; err2 == nil {
			return cached.ZH, nil
		}
	}
	return zh, nil
}

// buildTranslateFromDocs 文章翻译题: 并发翻译 1~3 句 (总超时 12s), 全部失败返回 nil
func (s *QuizService) buildTranslateFromDocs(userID uint, idxBase int, usedSentences map[string]bool) []*model.QuizQuestion {
	candidates := pickDocSentences(userID, 6)
	usable := []string{}
	for _, sent := range candidates {
		if !usedSentences[sent] {
			usable = append(usable, sent)
		}
	}
	if len(usable) == 0 {
		return nil
	}
	if len(usable) > 3 {
		usable = usable[:3]
	}
	type result struct {
		sentence string
		zh       string
		ok       bool
	}
	results := make([]result, len(usable))
	var wg sync.WaitGroup
	done := make(chan struct{})
	for i, sent := range usable {
		wg.Add(1)
		go func(i int, sent string) {
			defer wg.Done()
			zh, err := s.translateSentenceZH(userID, sent)
			results[i] = result{sentence: sent, zh: zh, ok: err == nil}
		}(i, sent)
	}
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(12 * time.Second):
		return nil // 超时: 本批放弃, 其他题型补足
	}
	questions := []*model.QuizQuestion{}
	for i, r := range results {
		if !r.ok || r.zh == "" {
			continue
		}
		usedSentences[r.sentence] = true
		questions = append(questions, &model.QuizQuestion{
			ID: fmt.Sprintf("q%d-doc-%d", time.Now().UnixNano(), idxBase+i), Type: "translate",
			Prompt: r.zh, Hint: "用英语写出这句话 (出自你读过的文章)",
			Answer: r.sentence,
		})
	}
	return questions
}

// GetLatestActiveSession 最近一个未过期 (48h) 的进行中会话, 供前端"继续练习"
func (s *QuizService) GetLatestActiveSession(userID uint) (*model.QuizSession, []model.QuizQuestion, error) {
	var session model.QuizSession
	err := DB.Where("id = (SELECT MAX(id) FROM quiz_sessions WHERE user_id = ? AND status = 'active' AND updated_at > ?)",
		userID, time.Now().Add(-48*time.Hour)).First(&session).Error
	if err != nil {
		return nil, nil, errors.New("没有进行中的练习")
	}
	var questions []model.QuizQuestion
	if err := json.Unmarshal([]byte(session.Questions), &questions); err != nil {
		return nil, nil, errors.New("会话数据损坏")
	}
	stripAnswers(questions)
	return &session, questions, nil
}

// abandonActiveSessions 新建会话前把旧 active 会话收口 (一个用户同时只保留一个 active)
func abandonActiveSessions(userID uint) {
	DB.Model(&model.QuizSession{}).
		Where("user_id = ? AND status = 'active'", userID).
		Update("status", "abandoned")
}

func stripAnswers(questions []model.QuizQuestion) {
	for i := range questions {
		questions[i].Answer = ""
	}
}
