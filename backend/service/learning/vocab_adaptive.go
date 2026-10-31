package learning

import (
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"backend/model"
	"backend/service/ai/prompt"
	"backend/service/kernel"
	"backend/service/sys/asynctask"
)

// ===== 自适应词汇量测试 v2 (锚定词库 + 二分探针) =====
// 与旧版 (LLM 按频段现出 24 题 + ≥50% 频段定级) 的区别:
// 1. 考词由 word_bank 分级锚定词库按档随机抽样——词的难度是真实数据, 不由 LLM 判断;
// 2. LLM 只生成正确释义+干扰项, 落 vocab_test_items 缓存按词全局复用 (题库可沉淀);
// 3. 出题自适应: 从 B1 档二分探针, 对了上探错了下探, 约 3~4 轮收敛到"会/不会"分界档,
//    每一题都在测量分界线附近 (旧版一半题目零信息量);
// 4. 计分输出区间 [est_low, est_high] + 中点, 不再给假精度数字; CEFR 输出保持兼容
//    (AI 写课文的 {level} 校准继续读 vocabulary_assessments)。

const (
	vocabProbeSize = 4 // 每轮探针题数
	vocabMaxRounds = 4 // 二分轮次上限 (7 个区间值收敛 ≤3 探针, 留裕量)
	vocabPassRate  = 0.5
	// vocabSuspiciousMsPerQ 作答过快阈值: 平均每题低于该毫秒数视为疑似乱选
	vocabSuspiciousMsPerQ = 1500
)

// vocabBandOrder 档位从易到难; 锚点沿用旧版语义 (词汇量量级锚)
var vocabBandOrder = []string{"A1", "A2", "B1", "B2", "C1", "C2"}
var vocabBandAnchors = []int{500, 1500, 3000, 5000, 8000, 15000}

// AdaptiveQuestionOut 下发版题目 (剥离正确答案)
type AdaptiveQuestionOut struct {
	ID      string   `json:"id"`
	Band    string   `json:"band"`
	Word    string   `json:"word"`
	Options []string `json:"options"`
}

// adaptivePool 出题 job 的 result_json (含答案, 仅服务端; 下发前经 redactor 剥离)
type adaptivePool struct {
	Questions []model.VocabTestQuestion `json:"questions"`
}

// RedactAdaptiveVocabResult 自适应测试结果脱敏器: 剥离正确答案 (main.go 注册到 asynctask)
func RedactAdaptiveVocabResult(raw string) string {
	var pool adaptivePool
	if err := json.Unmarshal([]byte(raw), &pool); err != nil {
		return raw
	}
	out := make([]AdaptiveQuestionOut, 0, len(pool.Questions))
	for _, q := range pool.Questions {
		out = append(out, AdaptiveQuestionOut{ID: q.ID, Band: q.Band, Word: q.Word, Options: q.Options})
	}
	b, _ := json.Marshal(map[string]any{"questions": out})
	return string(b)
}

// StartAdaptiveVocabTest 启动自适应测试: 异步 job 抽样 + 生成/复用题目池, 前端轮询通用任务端点
func (s *ReadingService) StartAdaptiveVocabTest(userID uint) (*model.AsyncJob, error) {
	if s.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	if running, err := asynctask.FindRunning(asynctask.KindVocabAdaptive, userID, 0); err == nil && running != nil {
		return running, nil
	}
	return asynctask.Start(asynctask.KindVocabAdaptive, userID, 0, "", "vocab:adaptive",
		func(report func(string)) (uint, string, error) {
			report("抽样考词")
			// 每档随机抽 vocabProbeSize 个词 (word_bank 分级锚定)
			pool := make([]model.VocabTestQuestion, 0, len(vocabBandOrder)*vocabProbeSize)
			for _, band := range vocabBandOrder {
				var words []model.WordBank
				if err := DB.Model(&model.WordBank{}).
					Where("cefr = ? AND enabled = ?", band, true).
					Order("RAND()").Limit(vocabProbeSize).
					Find(&words).Error; err != nil {
					return 0, "", fmt.Errorf("抽样 %s 档考词失败: %w", band, err)
				}
				if len(words) < vocabProbeSize {
					return 0, "", fmt.Errorf("词库 %s 档不足 %d 个词, 请先扩充 word_bank", band, vocabProbeSize)
				}
				for _, w := range words {
					pool = append(pool, model.VocabTestQuestion{ID: w.Word, Band: w.CEFR, Word: w.Word})
				}
			}

			// 题目缓存优先, 缺的批量让 LLM 生成干扰释义 (落 vocab_test_items 永久复用)
			if err := s.ensureVocabTestItems(userID, pool, report); err != nil {
				return 0, "", err
			}
			// 回填选项与答案
			words := make([]string, 0, len(pool))
			for _, q := range pool {
				words = append(words, q.Word)
			}
			var items []model.VocabTestItem
			if err := DB.Where("word IN ?", words).Find(&items).Error; err != nil {
				return 0, "", err
			}
			byWord := make(map[string]model.VocabTestItem, len(items))
			for _, it := range items {
				byWord[it.Word] = it
			}
			for i := range pool {
				it, ok := byWord[pool[i].Word]
				if !ok {
					return 0, "", fmt.Errorf("题目缓存缺失: %s", pool[i].Word)
				}
				var options []string
				if err := json.Unmarshal([]byte(it.OptionsJSON), &options); err != nil {
					return 0, "", fmt.Errorf("题目缓存损坏: %s", pool[i].Word)
				}
				pool[i].Options = options
				pool[i].Answer = it.Answer
			}

			raw, _ := json.Marshal(adaptivePool{Questions: pool})
			return 0, string(raw), nil
		})
}

// ensureVocabTestItems 保证词表中每词都有缓存考题: 缺失的分批让 LLM 生成干扰释义并落库
func (s *ReadingService) ensureVocabTestItems(userID uint, pool []model.VocabTestQuestion, report func(string)) error {
	words := make([]string, 0, len(pool))
	for _, q := range pool {
		words = append(words, q.Word)
	}
	var cached []model.VocabTestItem
	if err := DB.Where("word IN ?", words).Find(&cached).Error; err != nil {
		return err
	}
	has := make(map[string]bool, len(cached))
	for _, it := range cached {
		has[it.Word] = true
	}
	var missing []model.VocabTestQuestion
	for _, q := range pool {
		if !has[q.Word] {
			missing = append(missing, q)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	const chunkSize = 8
	total := len(missing)
	for start := 0; start < total; start += chunkSize {
		end := start + chunkSize
		if end > total {
			end = total
		}
		chunk := missing[start:end]
		report(fmt.Sprintf("生成题目 %d/%d", end, total))
		var err error
		for attempt := 0; attempt < 2; attempt++ {
			err = s.generateVocabTestItems(userID, chunk)
			if err == nil {
				break
			}
		}
		if err != nil {
			return fmt.Errorf("生成干扰释义失败: %w", err)
		}
	}
	return nil
}

// generateVocabTestItems 一批词的干扰释义生成 + 落缓存 (词唯一键, 并发重复创建静默忽略)
func (s *ReadingService) generateVocabTestItems(userID uint, chunk []model.VocabTestQuestion) error {
	var b strings.Builder
	b.WriteString("单词列表:\n")
	for i, q := range chunk {
		fmt.Fprintf(&b, "%d. %s\n", i+1, q.Word)
	}
	system := prompt.GetPrompt(prompt.PromptKeyVocabItem)
	raw, err := s.agentSvc.GenerateTextWithTimeout(userID, "", system, b.String(), slowGenTimeout)
	if err != nil {
		return err
	}
	var entries []struct {
		Word        string   `json:"word"`
		Definition  string   `json:"definition"`
		Distractors []string `json:"distractors"`
	}
	if err := kernel.DecodeLLMArray(raw, &entries); err != nil {
		return errors.New("AI 返回格式异常 (详见后端日志)")
	}
	byWord := make(map[string]struct {
		Definition  string
		Distractors []string
	}, len(entries))
	for _, e := range entries {
		key := strings.ToLower(strings.TrimSpace(e.Word))
		byWord[key] = struct {
			Definition  string
			Distractors []string
		}{strings.TrimSpace(e.Definition), e.Distractors}
	}

	for _, q := range chunk {
		entry, ok := byWord[strings.ToLower(q.Word)]
		if !ok || entry.Definition == "" || len(entry.Distractors) != 3 {
			return fmt.Errorf("AI 未给出 %s 的完整释义", q.Word)
		}
		def := kernel.TruncateRunes(entry.Definition, 255)
		seen := map[string]bool{strings.ToLower(def): true}
		clean := make([]string, 0, 3)
		for _, d := range entry.Distractors {
			d = kernel.TruncateRunes(strings.TrimSpace(d), 255)
			if d == "" || seen[strings.ToLower(d)] {
				return fmt.Errorf("AI 给出的 %s 干扰项重复或为空", q.Word)
			}
			seen[strings.ToLower(d)] = true
			clean = append(clean, d)
		}
		// 选项打乱后落库 (生成一次永久复用)
		options := append([]string{def}, clean...)
		rand.Shuffle(len(options), func(i, j int) { options[i], options[j] = options[j], options[i] })
		answer := 0
		for i, o := range options {
			if o == def {
				answer = i
				break
			}
		}
		optsJSON, _ := json.Marshal(options)
		distJSON, _ := json.Marshal(clean)
		item := model.VocabTestItem{
			Word: q.Word, CEFR: q.Band,
			CorrectDefinition: def, Distractors: string(distJSON),
			OptionsJSON: string(optsJSON), Answer: answer,
		}
		if err := DB.Create(&item).Error; err != nil {
			// 并发下另一测试先生成: 唯一键冲突直接复用已有缓存
			if !strings.Contains(strings.ToLower(err.Error()), "duplicate") && !strings.Contains(strings.ToLower(err.Error()), "unique") {
				return err
			}
		}
	}
	return nil
}

// ===== 逐轮作答 (无状态二分: 客户端每轮回传全部历史, 服务端重放定位) =====

// AdaptiveRoundAnswer 单题作答
type AdaptiveRoundAnswer struct {
	QuestionID string `json:"questionId"`
	Choice     int    `json:"choice"`
}

// AdaptiveRoundRequest 逐轮请求: rounds 为历史各轮作答 (含本轮), 服务端重放二分路径
type AdaptiveRoundRequest struct {
	JobID      uint                    `json:"jobId" binding:"required"`
	DurationMs int64                   `json:"durationMs"`
	Rounds     [][]AdaptiveRoundAnswer `json:"rounds" binding:"required,min=1,max=8"`
}

// VocabBandStat 单档作答明细
type VocabBandStat struct {
	Asked   int `json:"asked"`
	Correct int `json:"correct"`
}

// VocabTestResult 自适应测试结果 (区间 + 中点, 不再给假精度)
type VocabTestResult struct {
	CEFR       string                   `json:"cefr"`
	EstSize    int                      `json:"estSize"`
	EstLow     int                      `json:"estLow"`
	EstHigh    int                      `json:"estHigh"`
	Bands      map[string]VocabBandStat `json:"bands"`
	Rounds     int                      `json:"rounds"`
	Suspicious bool                     `json:"suspicious"`
}

// AdaptiveRoundResponse 轮次响应: 未收敛给下一轮题目, 收敛给最终结果
type AdaptiveRoundResponse struct {
	Finished  bool                  `json:"finished"`
	Round     int                   `json:"round"`
	ProbeBand string                `json:"probeBand"`
	Questions []AdaptiveQuestionOut `json:"questions,omitempty"`
	Result    *VocabTestResult      `json:"result,omitempty"`
}

// SubmitAdaptiveRound 处理一轮作答: 重放历史 → 收敛判定 → 出下一轮 / 出结果并落评估
func (s *ReadingService) SubmitAdaptiveRound(userID uint, req AdaptiveRoundRequest) (*AdaptiveRoundResponse, error) {
	var job model.AsyncJob
	if err := DB.Where("id = ? AND user_id = ? AND kind = ?", req.JobID, userID, asynctask.KindVocabAdaptive).
		First(&job).Error; err != nil {
		return nil, errors.New("测试会话不存在")
	}
	if job.Status != asynctask.StatusDone || job.ResultJSON == "" {
		return nil, errors.New("题目尚未生成完成")
	}
	var pool adaptivePool
	if err := json.Unmarshal([]byte(job.ResultJSON), &pool); err != nil {
		return nil, errors.New("题目数据损坏")
	}

	// 重放二分路径: 通过档抬 lo, 失败档压 hi
	lo, hi, bands, totalQ, err := vocabReplayPath(&pool, req.Rounds)
	if err != nil {
		return nil, err
	}
	used := make(map[string]bool)
	for _, round := range req.Rounds {
		for _, a := range round {
			used[a.QuestionID] = true
		}
	}
	if lo >= hi {
		return nil, errors.New("作答路径异常, 请重新开始测试")
	}

	resp := &AdaptiveRoundResponse{}
	finished := hi-lo <= 1 || len(req.Rounds) >= vocabMaxRounds
	if !finished {
		// 下一探针 = 当前 [lo, hi] 区间的中点档
		mid := (lo + hi) / 2
		probe := vocabBandOrder[mid]
		resp.Round = len(req.Rounds) + 1
		resp.ProbeBand = probe
		resp.Questions = s.sampleProbeQuestions(&pool, probe, used)
		if len(resp.Questions) == 0 {
			return nil, fmt.Errorf("词库 %s 档可用考词不足, 请扩充 word_bank 后重试", probe)
		}
		return resp, nil
	}

	// 收敛: 区间计分
	resp.Finished = true
	estLow := 200
	if lo >= 0 {
		estLow = vocabBandAnchors[lo]
	}
	estHigh := 20000
	if hi < len(vocabBandOrder) {
		estHigh = vocabBandAnchors[hi]
	}
	cefr := "A1"
	if lo >= 0 {
		cefr = vocabBandOrder[lo]
	}
	result := &VocabTestResult{
		CEFR: cefr, EstLow: estLow, EstHigh: estHigh,
		EstSize: (estLow + estHigh) / 2,
		Bands:   bands, Rounds: len(req.Rounds),
	}
	// 作答过快: 平均每题耗时低于阈值, 结果标注疑似乱选
	if totalQ > 0 && req.DurationMs > 0 {
		result.Suspicious = req.DurationMs < int64(totalQ)*vocabSuspiciousMsPerQ
	}
	resp.Result = result

	detail, _ := json.Marshal(map[string]any{
		"low": estLow, "high": estHigh, "bands": bands,
		"rounds": len(req.Rounds), "durationMs": req.DurationMs, "suspicious": result.Suspicious,
	})
	if err := DB.Create(&model.VocabularyAssessment{
		UserID: userID, CEFR: cefr, EstSize: result.EstSize, Detail: string(detail),
	}).Error; err != nil {
		return nil, err
	}
	return resp, nil
}

// sampleProbeQuestions 从题池取该档未用过的考词 (每档恰有 vocabProbeSize 个, 单轮即耗尽该档)
func (s *ReadingService) sampleProbeQuestions(pool *adaptivePool, band string, used map[string]bool) []AdaptiveQuestionOut {
	out := make([]AdaptiveQuestionOut, 0, vocabProbeSize)
	for _, q := range pool.Questions {
		if q.Band == band && !used[q.ID] {
			out = append(out, AdaptiveQuestionOut{ID: q.ID, Band: q.Band, Word: q.Word, Options: q.Options})
		}
	}
	return out
}

func bandIndexOf(band string) int {
	for i, b := range vocabBandOrder {
		if b == band {
			return i
		}
	}
	return -1
}

// vocabReplayPath 无状态重放二分路径: 逐轮判分, 通过档抬 lo / 失败档压 hi。
// 返回收敛后的 [lo, hi) 区间边界、各档作答明细与总题数。纯函数, 表驱动可测。
func vocabReplayPath(pool *adaptivePool, rounds [][]AdaptiveRoundAnswer) (lo, hi int, bands map[string]VocabBandStat, totalQ int, err error) {
	lo, hi = -1, len(vocabBandOrder)
	bands = make(map[string]VocabBandStat)
	byID := make(map[string]model.VocabTestQuestion, len(pool.Questions))
	for _, q := range pool.Questions {
		byID[q.ID] = q
	}
	for _, round := range rounds {
		if len(round) == 0 {
			continue
		}
		bi := -1
		correct := 0
		for _, a := range round {
			q, ok := byID[a.QuestionID]
			if !ok {
				return 0, 0, nil, 0, fmt.Errorf("题目 %s 不在本题池", a.QuestionID)
			}
			if a.Choice < 0 || a.Choice >= len(q.Options) {
				return 0, 0, nil, 0, fmt.Errorf("题目 %s 选项不合法", a.QuestionID)
			}
			if bi == -1 {
				bi = bandIndexOf(q.Band)
			}
			st := bands[q.Band]
			st.Asked++
			if a.Choice == q.Answer {
				st.Correct++
				correct++
			}
			bands[q.Band] = st
			totalQ++
		}
		if float64(correct)/float64(len(round)) >= vocabPassRate {
			if bi > lo {
				lo = bi
			}
		} else if bi < hi {
			hi = bi
		}
	}
	return lo, hi, bands, totalQ, nil
}

// ===== 行为数据滚动校准 (方案 B): 零操作持续推断, 与测试结果互相印证 =====

// VocabBehaviorEstimate 行为校准结果
type VocabBehaviorEstimate struct {
	FrontierBand string         `json:"frontierBand"` // 生词本主要分布的最高档 (学习边界)
	EstCEFR      string         `json:"estCefr"`      // 推断水平 = 边界档往下修正一档
	Confidence   string         `json:"confidence"`   // low/mid/high (样本量)
	TaggedCount  int            `json:"taggedCount"`  // 生词本中可锚定的词数
	BandCounts   map[string]int `json:"bandCounts"`   // 生词本词的档位分布
	Trend        string         `json:"trend"`        // up/down/flat (近30天查词密度变化), 空=数据不足
	Note         string         `json:"note"`
}

// VocabBehaviorEstimate 用生词本档位分布 + 阅读查词密度推断水平:
// 生词本装的是"不认识的词", 其最高密集档即学习边界 → 当前水平约低一档。
// 冷启动 (锚定词 < 10) 时 confidence=low, 前端提示以测试为准。
func (s *ReadingService) VocabBehaviorEstimate(userID uint) (*VocabBehaviorEstimate, error) {
	var words []string
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Pluck("word", &words).Error; err != nil {
		return nil, err
	}
	normalized := make([]string, 0, len(words))
	seen := make(map[string]bool, len(words))
	for _, w := range words {
		w = strings.ToLower(strings.TrimSpace(w))
		if w != "" && !seen[w] {
			seen[w] = true
			normalized = append(normalized, w)
		}
	}
	out := &VocabBehaviorEstimate{BandCounts: map[string]int{}, Confidence: "low", Note: ""}
	if len(normalized) == 0 {
		out.Note = "生词本为空, 暂无法从阅读行为推断"
		return out, nil
	}
	var bank []model.WordBank
	if err := DB.Where("word IN ? AND enabled = ?", normalized, true).Find(&bank).Error; err != nil {
		return nil, err
	}
	for _, b := range bank {
		// COM 行 (Oxford 3000 常用词标记) 不计入档位画像: 生词本含常用词是正常现象,
		// 只有带 A1..C2 档位的词才表达"这个难度的词不认识"
		if bandIndexOf(b.CEFR) < 0 {
			continue
		}
		out.BandCounts[b.CEFR]++
		out.TaggedCount++
	}
	if out.TaggedCount == 0 {
		out.Note = "生词本暂无可锚定的词, 数据积累后自动更新"
		return out, nil
	}
	// 学习边界 = 生词本分布的最高密集档 (≥3 个)
	frontier := -1
	for i := len(vocabBandOrder) - 1; i >= 0; i-- {
		if out.BandCounts[vocabBandOrder[i]] >= 3 {
			frontier = i
			break
		}
	}
	if frontier < 0 {
		out.Note = "生词样本还太少, 再积累一些阅读生词后自动更新"
		return out, nil
	}
	out.FrontierBand = vocabBandOrder[frontier]
	// 推断水平 = 边界往下修正一档 (正在攻克的档还没掌握)
	estIdx := frontier - 1
	if estIdx < 0 {
		estIdx = 0
	}
	out.EstCEFR = vocabBandOrder[estIdx]
	switch {
	case out.TaggedCount >= 30:
		out.Confidence = "high"
	case out.TaggedCount >= 10:
		out.Confidence = "mid"
	}

	// 近 30 天查词密度趋势: 后 15 天 vs 前 15 天 (每 open 的查词数)
	var stats []model.ReadingDailyStat
	since := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	if err := DB.Where("user_id = ? AND stat_date >= ?", userID, since).
		Order("stat_date ASC").Find(&stats).Error; err == nil && len(stats) > 4 {
		half := len(stats) / 2
		rate := func(rows []model.ReadingDailyStat) float64 {
			opens, lookups := 0, 0
			for _, r := range rows {
				opens += r.Opens
				lookups += r.Lookups
			}
			if opens == 0 {
				return 0
			}
			return float64(lookups) / float64(opens)
		}
		prev, recent := rate(stats[:half]), rate(stats[half:])
		switch {
		case prev == 0 && recent == 0:
			out.Trend = ""
		case recent < prev*0.8:
			out.Trend = "down" // 查词变少 = 阅读更轻松
		case recent > prev*1.25:
			out.Trend = "up"
		default:
			out.Trend = "flat"
		}
	}
	if out.Note == "" {
		out.Note = "基于生词本档位分布与阅读查词行为的持续推断, 随学习自动更新"
	}
	return out, nil
}
