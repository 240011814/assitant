package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"backend/model"
)

// QuizService 自动出题引擎: 词汇选择/例句挖空/易混词辨析/听音拼写/句子翻译 (模板确定性出题)
// + 造句批改 (LLM 判分)
type QuizService struct {
	agentSvc *AIAgentService
}

func NewQuizService(agentSvc *AIAgentService) *QuizService {
	return &QuizService{agentSvc: agentSvc}
}

// quizTypes 全部题型
var quizTypes = map[string]bool{
	"word2def":  true, // 单词 → 释义选择
	"def2word":  true, // 释义 → 单词选择
	"cloze":     true, // 例句挖空选择
	"confusing": true, // 易混词辨析 (confusing_words 字段)
	"spell":     true, // 听音拼写
	"translate": true, // 课程句对中译英
}

// normalizeAnswer 判分归一: 小写、去首尾、压缩空白、去标点
func normalizeAnswer(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '.', ',', '!', '?', ';', ':', '\'', '"', '(', ')', '’', '“', '”':
			continue
		}
		if r == ' ' && b.Len() > 0 && b.String()[b.Len()-1] == ' ' {
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(b.String())
}

// levenshtein 编辑距离
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// textSimilarity 归一化后的相似度 0~1
func textSimilarity(a, b string) float64 {
	na, nb := normalizeAnswer(a), normalizeAnswer(b)
	if na == nb {
		return 1
	}
	maxLen := len([]rune(na))
	if l := len([]rune(nb)); l > maxLen {
		maxLen = l
	}
	if maxLen == 0 {
		return 1
	}
	return 1 - float64(levenshtein(na, nb))/float64(maxLen)
}

// CreateSession 生成一套练习题 (题目与答案持久化到会话, 答题时服务端判分)
func (s *QuizService) CreateSession(userID uint, req model.CreateQuizSessionRequest) (*model.QuizSession, []model.QuizQuestion, error) {
	size := req.Size
	if size <= 0 {
		size = 10
	}
	if size > 50 {
		size = 50
	}
	requested := map[string]bool{}
	for _, t := range req.Types {
		if quizTypes[t] {
			requested[t] = true
		}
	}
	if len(requested) == 0 {
		requested = quizTypes
	}

	// 题目池: 按来源取词
	var pool []model.Vocabulary
	var err error
	switch req.Source {
	case "due":
		now := time.Now()
		err = DB.Where("user_id = ? AND is_mastered = ? AND fsrs_state != ? AND next_review_at <= ?", userID, false, model.FsrsStateNew, now).
			Order("next_review_at ASC, fsrs_lapses DESC").Limit(size * 3).Find(&pool).Error
	case "errorbook":
		var errBook []model.ErrorBook
		err = DB.Where("user_id = ? AND source_type = ?", userID, "vocabulary").
			Order("error_count DESC, updated_at DESC").Limit(size * 3).Find(&errBook).Error
		if err == nil {
			ids := make([]uint, 0, len(errBook))
			for _, e := range errBook {
				if e.SourceID > 0 {
					ids = append(ids, e.SourceID)
				}
			}
			if len(ids) > 0 {
				err = DB.Where("user_id = ? AND id IN ?", userID, ids).Find(&pool).Error
			}
		}
	default:
		err = DB.Where("user_id = ?", userID).Order("RAND()").Limit(size * 3).Find(&pool).Error
	}
	if err != nil {
		return nil, nil, err
	}
	if len(pool) == 0 {
		return nil, nil, errors.New("没有可用的词汇, 请先添加生词")
	}

	// 干扰项池: 随机其他词
	var distractors []model.Vocabulary
	if err := DB.Where("user_id = ?", userID).Order("RAND()").Limit(size*2 + 10).Find(&distractors).Error; err != nil {
		return nil, nil, err
	}

	questions := make([]model.QuizQuestion, 0, size)
	types := make([]string, 0, len(requested))
	for t := range requested {
		types = append(types, t)
	}
	// 打乱题型顺序
	rand.Shuffle(len(types), func(i, j int) { types[i], types[j] = types[j], types[i] })

	usedWords := map[uint]bool{}
	for i := 0; i < size; i++ {
		t := types[i%len(types)]
		word := s.pickWord(pool, usedWords, t)
		if word == nil {
			continue
		}
		usedWords[word.ID] = true

		var q *model.QuizQuestion
		switch t {
		case "word2def":
			q = s.buildWord2Def(word, distractors, i)
		case "def2word":
			q = s.buildDef2Word(word, distractors, i)
		case "cloze":
			q = s.buildCloze(word, distractors, i)
		case "confusing":
			q = s.buildConfusing(word, i)
		case "spell":
			q = &model.QuizQuestion{
				ID: fmt.Sprintf("q%d-%d", time.Now().UnixNano(), i), Type: t,
				WordID: word.ID, Word: word.Word, Prompt: "听音拼写单词", Hint: word.Word,
				Answer: word.Word,
			}
		case "translate":
			q = s.buildTranslate(userID, i)
		}
		if q != nil {
			questions = append(questions, *q)
		}
	}
	if len(questions) == 0 {
		return nil, nil, errors.New("题目生成失败: 词汇数据不足 (选择/挖空题需要释义或例句)")
	}

	raw, err := json.Marshal(questions)
	if err != nil {
		return nil, nil, err
	}
	session := &model.QuizSession{
		UserID:    userID,
		Questions: string(raw),
		Total:     len(questions),
		Status:    "active",
	}
	if err := DB.Create(session).Error; err != nil {
		return nil, nil, err
	}
	// 下发时剥离答案
	for i := range questions {
		questions[i].Answer = ""
	}
	return session, questions, nil
}

// pickWord 按题型挑一个合适的词
func (s *QuizService) pickWord(pool []model.Vocabulary, used map[uint]bool, quizType string) *model.Vocabulary {
	for i := range pool {
		w := &pool[i]
		if used[w.ID] {
			continue
		}
		switch quizType {
		case "word2def", "def2word":
			if w.Definition != "" {
				return w
			}
		case "cloze":
			if w.Example != "" && strings.Contains(strings.ToLower(w.Example), strings.ToLower(w.Word)) {
				return w
			}
		case "confusing":
			if w.ConfusingWords != "" {
				return w
			}
		default:
			return w
		}
	}
	return nil
}

func (s *QuizService) buildWord2Def(word *model.Vocabulary, distractors []model.Vocabulary, idx int) *model.QuizQuestion {
	options := []string{word.Definition}
	for _, d := range distractors {
		if len(options) >= 4 {
			break
		}
		if d.Definition != "" && d.ID != word.ID && !containsStr(options, d.Definition) {
			options = append(options, d.Definition)
		}
	}
	if len(options) < 2 {
		return nil
	}
	shuffleOptions(options)
	return &model.QuizQuestion{
		ID: fmt.Sprintf("q%d-%d", time.Now().UnixNano(), idx), Type: "word2def",
		WordID: word.ID, Word: word.Word, Prompt: word.Word, Options: options,
		Answer: word.Definition,
	}
}

func (s *QuizService) buildDef2Word(word *model.Vocabulary, distractors []model.Vocabulary, idx int) *model.QuizQuestion {
	options := []string{word.Word}
	for _, d := range distractors {
		if len(options) >= 4 {
			break
		}
		if d.Word != "" && d.ID != word.ID && !containsStr(options, d.Word) {
			options = append(options, d.Word)
		}
	}
	if len(options) < 2 {
		return nil
	}
	shuffleOptions(options)
	return &model.QuizQuestion{
		ID: fmt.Sprintf("q%d-%d", time.Now().UnixNano(), idx), Type: "def2word",
		WordID: word.ID, Word: word.Word, Prompt: word.Definition, Options: options,
		Answer: word.Word,
	}
}

// buildCloze 例句挖空: 把例句中的目标词替换为 ____, 干扰项优先取易混词
func (s *QuizService) buildCloze(word *model.Vocabulary, distractors []model.Vocabulary, idx int) *model.QuizQuestion {
	lower := strings.ToLower(word.Example)
	pos := strings.Index(lower, strings.ToLower(word.Word))
	if pos < 0 {
		return nil
	}
	cloze := word.Example[:pos] + "____" + word.Example[pos+len(word.Word):]
	options := []string{word.Word}
	// 干扰项优先: 易混词
	for _, cw := range strings.Split(word.ConfusingWords, ",") {
		cw = strings.TrimSpace(cw)
		if cw != "" && len(options) < 4 && !containsStr(options, cw) {
			options = append(options, cw)
		}
	}
	for _, d := range distractors {
		if len(options) >= 4 {
			break
		}
		if d.Word != "" && d.ID != word.ID && !containsStr(options, d.Word) {
			options = append(options, d.Word)
		}
	}
	if len(options) < 2 {
		return nil
	}
	shuffleOptions(options)
	return &model.QuizQuestion{
		ID: fmt.Sprintf("q%d-%d", time.Now().UnixNano(), idx), Type: "cloze",
		WordID: word.ID, Word: word.Word, Prompt: cloze, Hint: word.Definition, Options: options,
		Answer: word.Word,
	}
}

// buildConfusing 易混词辨析: 例句挖空, 选项就是易混词组
func (s *QuizService) buildConfusing(word *model.Vocabulary, idx int) *model.QuizQuestion {
	if word.Example == "" {
		return nil
	}
	lower := strings.ToLower(word.Example)
	pos := strings.Index(lower, strings.ToLower(word.Word))
	if pos < 0 {
		return nil
	}
	cloze := word.Example[:pos] + "____" + word.Example[pos+len(word.Word):]
	options := []string{word.Word}
	for _, cw := range strings.Split(word.ConfusingWords, ",") {
		cw = strings.TrimSpace(cw)
		if cw != "" && len(options) < 4 && !containsStr(options, cw) {
			options = append(options, cw)
		}
	}
	if len(options) < 2 {
		return nil
	}
	shuffleOptions(options)
	return &model.QuizQuestion{
		ID: fmt.Sprintf("q%d-%d", time.Now().UnixNano(), idx), Type: "confusing",
		WordID: word.ID, Word: word.Word, Prompt: cloze, Hint: word.Definition, Options: options,
		Answer: word.Word,
	}
}

// buildTranslate 课程句对中译英
func (s *QuizService) buildTranslate(userID uint, idx int) *model.QuizQuestion {
	var item model.CourseItem
	err := DB.Joins("JOIN courses ON courses.id = course_items.course_id").
		Where("courses.user_id = ? OR courses.is_public = ?", userID, true).
		Where("course_items.chinese_translation != ''").
		Order("RAND()").
		Select("course_items.*").
		First(&item).Error
	if err != nil {
		return nil
	}
	return &model.QuizQuestion{
		ID: fmt.Sprintf("q%d-%d", time.Now().UnixNano(), idx), Type: "translate",
		CourseID: item.CourseID, Prompt: item.ChineseTranslation,
		Hint: "用英语说出/写出这句话", Answer: item.EnglishSentence,
	}
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func shuffleOptions(options []string) {
	rand.Shuffle(len(options), func(i, j int) { options[i], options[j] = options[j], options[i] })
}

// GetSession 获取会话 (题目剥离答案)
func (s *QuizService) GetSession(userID, sessionID uint) (*model.QuizSession, []model.QuizQuestion, error) {
	session, questions, err := s.loadSession(userID, sessionID)
	if err != nil {
		return nil, nil, err
	}
	return session, questions, nil
}

// loadSession 加载会话并解析题目 (含答案, 内部用)
func (s *QuizService) loadSession(userID, sessionID uint) (*model.QuizSession, []model.QuizQuestion, error) {
	var session model.QuizSession
	if err := DB.Where("id = ? AND user_id = ?", sessionID, userID).First(&session).Error; err != nil {
		return nil, nil, errors.New("练习会话不存在")
	}
	var questions []model.QuizQuestion
	if err := json.Unmarshal([]byte(session.Questions), &questions); err != nil {
		return nil, nil, errors.New("会话数据损坏")
	}
	return &session, questions, nil
}

// SubmitAnswer 判分并记录 (答错自动进错题本)
func (s *QuizService) SubmitAnswer(userID, sessionID uint, req model.QuizAnswerRequest) (*model.QuizAnswerResult, *model.QuizSession, []model.QuizQuestion, error) {
	session, questions, err := s.loadSession(userID, sessionID)
	if err != nil {
		return nil, nil, nil, err
	}
	if session.Status != "active" {
		return nil, nil, nil, errors.New("会话已结束")
	}

	idx := -1
	for i := range questions {
		if questions[i].ID == req.QuestionID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, nil, nil, errors.New("题目不存在")
	}
	if questions[idx].Done {
		return nil, nil, nil, errors.New("该题已作答")
	}
	q := &questions[idx]

	correct := gradeQuestion(q, req.Answer)
	result := &model.QuizAnswerResult{
		QuestionID: req.QuestionID,
		Correct:    correct,
		Answer:     q.Answer,
	}

	// 答错自动进错题本
	if !correct {
		recorded := s.recordToErrorBook(userID, q, sessionID)
		result.Recorded = recorded
	}

	// 更新会话
	q.Done = true
	session.Answered++
	if correct {
		session.Correct++
	}
	if session.Answered >= session.Total {
		session.Status = "finished"
	}
	raw, err := json.Marshal(questions)
	if err != nil {
		return nil, nil, nil, err
	}
	session.Questions = string(raw)
	if err := DB.Save(session).Error; err != nil {
		return nil, nil, nil, err
	}
	for i := range questions {
		questions[i].Answer = ""
	}
	return result, session, questions, nil
}

// gradeQuestion 按题型判分
func gradeQuestion(q *model.QuizQuestion, answer string) bool {
	switch q.Type {
	case "translate":
		// 翻译容忍少量笔误 (相似度 >= 0.9)
		return textSimilarity(answer, q.Answer) >= 0.9
	case "spell":
		return normalizeAnswer(answer) == normalizeAnswer(q.Answer)
	default:
		return normalizeAnswer(answer) == normalizeAnswer(q.Answer)
	}
}

// recordToErrorBook 答错落错题本: 词类错题挂生词, 翻译错题记句子
func (s *QuizService) recordToErrorBook(userID uint, q *model.QuizQuestion, sessionID uint) bool {
	eb := NewErrorBookService()
	var req model.CreateErrorBookRequest
	switch q.Type {
	case "translate":
		req = model.CreateErrorBookRequest{
			ContentType: "sentence", Content: q.Answer,
			Translation: q.Prompt, SourceType: "quiz", SourceID: sessionID,
		}
	default:
		req = model.CreateErrorBookRequest{
			ContentType: "word", Content: q.Word,
			SourceType: "quiz", SourceID: sessionID,
		}
	}
	_, err := eb.AddErrorBook(userID, req)
	return err == nil
}

// SentencePractice 造句批改 (LLM 判分)
func (s *QuizService) SentencePractice(userID uint, req model.SentencePracticeRequest) (*model.SentencePracticeResult, error) {
	if s.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	var word model.Vocabulary
	if err := DB.Where("id = ? AND user_id = ?", req.WordID, userID).First(&word).Error; err != nil {
		return nil, errors.New("生词不存在")
	}

	system := `你是英语写作批改助手。批改用户用指定单词造的句子, 从语法正确性、单词用法、地道程度打分。只输出 JSON:
{"score": <0-100 整数>, "correct": <score>=60 时为 true, 否则 false>, "grammar": ["<语法/用词问题, 无则空数组>"], "improved": "<改进后的句子, 满分时给原句>", "comment": "<一句中文点评>"}`
	user := fmt.Sprintf("目标单词: %s\n释义: %s\n用户造句: %s", word.Word, word.Definition, req.Sentence)
	raw, err := s.agentSvc.GenerateText(userID, "", system, user)
	if err != nil {
		return nil, fmt.Errorf("批改失败: %w", err)
	}

	var result model.SentencePracticeResult
	if err := json.Unmarshal([]byte(extractJSON(raw)), &result); err != nil {
		return nil, errors.New("AI 返回格式异常")
	}
	result.Word = word.Word
	// 答得差的句子进错题本
	if !result.Correct {
		eb := NewErrorBookService()
		_, _ = eb.AddErrorBook(userID, model.CreateErrorBookRequest{
			ContentType: "word", Content: word.Word,
			Translation: req.Sentence, SourceType: "quiz", SourceID: 0,
		})
	}
	return &result, nil
}
