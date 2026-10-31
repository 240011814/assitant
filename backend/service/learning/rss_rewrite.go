package learning

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"backend/model"
	"backend/service/ai/prompt"
	"backend/service/kernel"
	"backend/service/sys/asynctask"
)

// ===== RSS 分级改写 (按用户 CEFR 档位, 异步任务) =====
// 复用中英对照翻译的切段/分批骨架 (rss_translate.go): content_html 块级切段 →
// 编号 JSON 分批改写 → 组装 <p>/<pre> 段落序列缓存到 rss_items.rewrite_html。
// 改写后用 word_bank 做轻量后置校验: 已覆盖词中的超纲率 (档位 > 目标+1) 写进进度与日志。
// 校验是弱信号 (词库只覆盖常用词), 只提示不阻断。

// rssRewriteOutOfBandRate 已覆盖词超纲率的告警阈值 (超过则在进度里提示)
const rssRewriteOutOfBandRate = 0.15

// rssRewriteSeg 改写的编号段落返回
type rssRewriteSeg struct {
	I    int    `json:"i"`
	Text string `json:"text"`
}

// currentVocabLevel 用户当前目标档位: 词汇量测试定级优先, 行为校准兜底, 默认 B1
func (s *RSSService) currentVocabLevel(userID uint) string {
	if a, err := GetLatestVocabTest(userID); err == nil && a.CEFR != "" && bandIndexOf(a.CEFR) >= 0 {
		return a.CEFR
	}
	if est, err := s.readingSvc.VocabBehaviorEstimate(userID); err == nil && est.EstCEFR != "" && bandIndexOf(est.EstCEFR) >= 0 {
		return est.EstCEFR
	}
	return "B1"
}

// StartItemRewrite 生成中英档位改写 (异步任务): 结果缓存到条目, 原文变化即过期。
// 同一条目进行中复用任务; 已有未过期改写直接返回即时 done。
func (s *RSSService) StartItemRewrite(userID, id uint) (*model.AsyncJob, error) {
	if s.readingSvc == nil || s.readingSvc.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	var item model.RSSItem
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		return nil, errors.New("文章不存在")
	}
	if running, err := asynctask.FindRunning(asynctask.KindRSSRewrite, userID, item.ID); err == nil && running != nil {
		return running, nil
	}
	// 确保正文可用 (复用 GetItemContent 的缓存/抓取/摘要兜底链路)
	full, err := s.GetItemContent(userID, id)
	if err != nil {
		return nil, err
	}
	htmlSrc := strings.TrimSpace(full.ContentHTML)
	if htmlSrc == "" {
		return nil, errors.New("本文没有可改写的正文")
	}
	if len(htmlSrc) > rssTranslateMaxRunes {
		return nil, errors.New("正文过长, 暂不支持改写")
	}
	hash := rssContentHash(htmlSrc)
	level := s.currentVocabLevel(userID)
	if item.RewriteHTML != "" && item.RewriteHash == hash {
		// 已有未过期改写: 即时完成, 前端直接切改写视图
		return asynctask.InstantDone(asynctask.KindRSSRewrite, userID, item.ID), nil
	}
	itemID := item.ID
	title := full.Title
	return asynctask.Start(asynctask.KindRSSRewrite, userID, itemID,
		fmt.Sprintf(`{"level":"%s"}`, level), "rss:rewrite-item",
		func(report func(string)) (uint, string, error) {
			rewritten, rate, err := s.rewriteItemSync(userID, level, title, htmlSrc, report)
			if err != nil {
				return 0, "", err
			}
			updates := map[string]interface{}{
				"rewrite_html":  rewritten,
				"rewrite_level": level,
				"rewrite_hash":  hash,
			}
			if e := DB.Model(&model.RSSItem{}).Where("id = ?", itemID).Updates(updates).Error; e != nil {
				return 0, "", e
			}
			note := fmt.Sprintf("改写完成 (目标 %s, 已覆盖词超纲率 %.0f%%)", level, rate*100)
			if rate > rssRewriteOutOfBandRate {
				note += ", 超纲词偏多, 改写仅供参考"
			}
			report(note)
			log.Printf("[rss:rewrite] item=%d user=%d %s", itemID, userID, note)
			return itemID, "", nil
		})
}

// rewriteItemSync 改写主体 (后台协程执行): 切段 → 分批改写 → 组装 p/pre 序列 + 超纲率校验
func (s *RSSService) rewriteItemSync(userID uint, level, title, htmlSrc string, report func(string)) (string, float64, error) {
	_, units, batches, err := prepareRSSBatches(htmlSrc)
	if err != nil {
		return "", 0, err
	}
	rewritten := make([]string, len(units))
	for bi, batch := range batches {
		report(fmt.Sprintf("改写中 %d/%d 批", bi+1, len(batches)))
		segs := make([]string, len(batch))
		for i, idx := range batch {
			segs[i] = units[idx].text
		}
		var out []string
		for attempt := 0; attempt < 2; attempt++ {
			out, err = s.rewriteBatch(userID, level, title, segs)
			if err == nil {
				break
			}
		}
		if err != nil {
			return "", 0, fmt.Errorf("第 %d/%d 批改写失败: %w", bi+1, len(batches), err)
		}
		for i, idx := range batch {
			rewritten[idx] = out[i]
		}
	}
	// 组装: 段落级 <p>, 代码块 <pre> (阅读视图对两者的样式均已具备)
	var sb strings.Builder
	for i, u := range units {
		text := strings.TrimSpace(rewritten[i])
		if text == "" {
			continue
		}
		tag := "p"
		cls := ""
		if u.tag == "pre" {
			tag, cls = "pre", ` class="rss-rw-pre"`
		}
		sb.WriteString("<" + tag + cls + ">" + escapeHTMLText(text) + "</" + tag + ">")
	}
	out := strings.TrimSpace(sb.String())
	if out == "" {
		return "", 0, errors.New("改写结果为空")
	}
	rate := rewriteOutOfBandRate(out, level)
	return out, rate, nil
}

// rewriteBatch 一批段落的改写: 标题做上下文, 编号 JSON 输出保证对齐
func (s *RSSService) rewriteBatch(userID uint, level, title string, segments []string) ([]string, error) {
	system := strings.ReplaceAll(prompt.GetPrompt(prompt.PromptKeyRSSRewrite), "{level}", level)
	var b strings.Builder
	b.WriteString("文章标题: " + title + "\n")
	b.WriteString("请改写以下段落:\n")
	for i, seg := range segments {
		fmt.Fprintf(&b, "[%d] %s\n", i+1, seg)
	}
	raw, err := s.readingSvc.agentSvc.GenerateTextWithTimeout(userID, "", system, b.String(), slowGenTimeout)
	if err != nil {
		return nil, err
	}
	var segs []rssRewriteSeg
	if err := kernel.DecodeLLMArray(raw, &segs); err != nil {
		return nil, errors.New("AI 返回格式异常 (详见后端日志)")
	}
	out := make([]string, len(segments))
	seen := make(map[int]bool, len(segments))
	for _, sg := range segs {
		if sg.I >= 1 && sg.I <= len(segments) && !seen[sg.I-1] {
			out[sg.I-1] = strings.TrimSpace(sg.Text)
			seen[sg.I-1] = true
		}
	}
	for i := range segments {
		if !seen[i] {
			return nil, fmt.Errorf("AI 返回缺少第 %d 段的改写", i+1)
		}
	}
	return out, nil
}

// rewriteOutOfBandRate 轻量后置校验: 词库已覆盖词中, 档位高于目标+1 的占比。
// COM 行 (常用词) 不算超纲; 未被词库覆盖的词不计入分母 (弱信号, 只提示不阻断)。
func rewriteOutOfBandRate(text, level string) float64 {
	targetIdx := bandIndexOf(level)
	if targetIdx < 0 {
		return 0
	}
	words := wordTokenRe.FindAllString(text, -1)
	seen := make(map[string]bool, len(words))
	var uniq []string
	for _, w := range words {
		w = strings.ToLower(w)
		if w != "" && !seen[w] {
			seen[w] = true
			uniq = append(uniq, w)
		}
	}
	if len(uniq) == 0 {
		return 0
	}
	var bands []string
	if err := DB.Model(&model.WordBank{}).Where("word IN ?", uniq).Pluck("cefr", &bands).Error; err != nil {
		return 0
	}
	covered := 0
	outOfBand := 0
	for _, band := range bands {
		bi := bandIndexOf(band)
		if bi < 0 {
			continue
		}
		covered++
		if bi > targetIdx+1 {
			outOfBand++
		}
	}
	if covered == 0 {
		return 0
	}
	return float64(outOfBand) / float64(covered)
}

// prepareRSSBatches 公共骨架在 rss_translate.go (与翻译共用切段/分批)

var escapeHTMLText = func(s string) string {
	// 与净化器同口径: 文本一律 HTML 转义
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;").Replace(s)
}
