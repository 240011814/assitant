package learning

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"

	"backend/model"
	"backend/service/ai/prompt"
	"backend/service/kernel"
	"backend/service/sys/asynctask"
)

// ===== RSS 中英对照翻译 (段落级, 异步任务) =====
// 管线: content_html 按块级元素切段 → 分批 LLM 翻译 (编号 JSON 对齐) → 服务端把译文插回
// 原文树组装双语 HTML (原文块 + <div class="rss-zh">译文块) 缓存到 rss_items.bilingual_html。
// 前端三态切换 (对照/仅译文/仅原文) 靠容器 class + CSS, 渲染路径与现有净化管线复用。

const (
	// rssTranslateMaxRunes 送翻的 HTML 源码上限 (正文缓存 rune 上限 100k, HTML 放大一倍足够)
	rssTranslateMaxRunes = 200_000
	// rssTranslateMaxUnits 单篇段落上限 (超出直接拒绝, 不做静默截断)
	rssTranslateMaxUnits = 120
	// rssTranslateBatchRunes 每批送 LLM 的源文 rune 预算
	rssTranslateBatchRunes = 2400
	// rssTranslateMaxBatches 批数上限 (双保险, 与 MaxUnits 联动)
	rssTranslateMaxBatches = 60
	// rssTranslateMinUnitRunes 短于该长度的块不翻译 (纯符号/编号/孤词)
	rssTranslateMinUnitRunes = 2
)

// rssTranslateUnit 一个待翻译块: 文档顺序 + 块元素节点 + 内部纯文本
type rssTranslateUnit struct {
	node *xhtml.Node
	text string
	tag  string
}

// rssUnitTags 可翻译的块级单元 (content_html 已过白名单净化, 只可能是这些标签)
var rssUnitTags = map[string]bool{
	"p": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"li": true, "blockquote": true, "pre": true, "td": true, "th": true, "caption": true,
}

// rssUnitInnerInsert 父元素为这些标签时, 译文 div 必须插进单元内部 (末子节点) 而非兄弟位置:
// div 直接放在 ul/ol/tr/table 下是非法 HTML, 浏览器解析时会被挪出列表/表格破坏布局
var rssUnitInnerInsert = map[string]bool{"ul": true, "ol": true, "tr": true, "table": true}

// rssContentHash 正文指纹: 翻译时记录, 原文重新抓取后内容变化即视为译文过期
func rssContentHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// findHTMLBody 在解析树中找 <body> (净化产物存的是 body 子树片段, Parse 后都会包一层 html/body)
func findHTMLBody(root *xhtml.Node) *xhtml.Node {
	if root == nil {
		return nil
	}
	if root.Type == xhtml.ElementNode && root.Data == "body" {
		return root
	}
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if b := findHTMLBody(c); b != nil {
			return b
		}
	}
	return nil
}

// collectRSSUnits 收集可翻译块 (最内层优先: blockquote>p 只收 p, 整块引用作为一个单元)
func collectRSSUnits(body *xhtml.Node) []*rssTranslateUnit {
	var units []*rssTranslateUnit
	var walk func(n *xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type != xhtml.ElementNode {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			return
		}
		if rssUnitTags[n.Data] {
			text := nodeRawText(n)
			if n.Data != "pre" {
				// 空白折叠 (pre 保留原始换行/缩进)
				text = strings.Join(strings.Fields(text), " ")
			}
			text = strings.TrimSpace(text)
			if len([]rune(text)) >= rssTranslateMinUnitRunes {
				units = append(units, &rssTranslateUnit{node: n, text: text, tag: n.Data})
			}
			return // 不再下探: 块内嵌套块整块作为一个翻译单元
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(body)
	return units
}

// nodeRawText 收集节点子树纯文本 (跳过 img, 它的 alt 是给读屏器的标注不是正文)
func nodeRawText(n *xhtml.Node) string {
	var sb strings.Builder
	var walk func(c *xhtml.Node)
	walk = func(c *xhtml.Node) {
		if c.Type == xhtml.TextNode {
			sb.WriteString(c.Data)
			return
		}
		if c.Type == xhtml.ElementNode && c.Data == "img" {
			return
		}
		for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
			walk(cc)
		}
	}
	walk(n)
	return sb.String()
}

// buildTranslateBatches 按源文 rune 预算把单元下标分批 (批内互不跨单元, 译文按单元下标对齐)
func buildTranslateBatches(units []*rssTranslateUnit) [][]int {
	var batches [][]int
	var cur []int
	curRunes := 0
	for i, u := range units {
		n := len([]rune(u.text))
		if len(cur) > 0 && curRunes+n > rssTranslateBatchRunes {
			batches = append(batches, cur)
			cur, curRunes = nil, 0
		}
		cur = append(cur, i)
		curRunes += n
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}
	return batches
}

// rssTranslateSeg LLM 返回的编号译文
type rssTranslateSeg struct {
	I  int    `json:"i"`
	Zh string `json:"zh"`
}

// translateBatch 翻译一批段落: 标题 + 上一段结尾作上下文, 编号 JSON 输出保证对齐。
// 返回与 segments 等长的译文切片 (校验失败返回错误, 调用方决定重试)。
func (s *RSSService) translateBatch(userID uint, title string, segments []string, prevTail string) ([]string, error) {
	system := prompt.GetPrompt(prompt.PromptKeyRSSTranslate)
	var b strings.Builder
	b.WriteString("文章标题: " + title + "\n")
	if prevTail != "" {
		b.WriteString("上一段结尾 (供行文衔接参考): …" + prevTail + "\n")
	}
	b.WriteString("请翻译以下段落:\n")
	for i, seg := range segments {
		fmt.Fprintf(&b, "[%d] %s\n", i+1, seg)
	}
	raw, err := s.readingSvc.agentSvc.GenerateTextWithTimeout(userID, "", system, b.String(), slowGenTimeout)
	if err != nil {
		return nil, err
	}
	var segs []rssTranslateSeg
	if err := kernel.DecodeLLMArray(raw, &segs); err != nil {
		return nil, errors.New("AI 返回格式异常 (详见后端日志)")
	}
	out := make([]string, len(segments))
	seen := make(map[int]bool, len(segments))
	for _, sg := range segs {
		if sg.I >= 1 && sg.I <= len(segments) && !seen[sg.I-1] {
			out[sg.I-1] = strings.TrimSpace(sg.Zh)
			seen[sg.I-1] = true
		}
	}
	for i := range segments {
		if !seen[i] {
			return nil, fmt.Errorf("AI 返回缺少第 %d 段的译文", i+1)
		}
	}
	return out, nil
}

// prepareRSSBatches 公共骨架 (翻译/改写共用): 解析净化 HTML → 块级切段 → 按 rune 预算分批。
// 返回 body 节点 (组装时需要回插/序列化)、翻译单元与分批下标。
func prepareRSSBatches(htmlSrc string) (body *xhtml.Node, units []*rssTranslateUnit, batches [][]int, err error) {
	root, perr := xhtml.Parse(strings.NewReader(htmlSrc))
	if perr != nil {
		return nil, nil, nil, fmt.Errorf("正文 HTML 解析失败: %w", perr)
	}
	body = findHTMLBody(root)
	if body == nil {
		return nil, nil, nil, errors.New("正文 HTML 无 body 节点")
	}
	units = collectRSSUnits(body)
	if len(units) > rssTranslateMaxUnits {
		return nil, nil, nil, fmt.Errorf("文章段落过多 (%d, 上限 %d), 暂不支持处理", len(units), rssTranslateMaxUnits)
	}
	if len(units) == 0 {
		return nil, nil, nil, errors.New("没有可处理的段落")
	}
	batches = buildTranslateBatches(units)
	if len(batches) > rssTranslateMaxBatches {
		return nil, nil, nil, fmt.Errorf("文章过长 (%d 批, 上限 %d), 暂不支持处理", len(batches), rssTranslateMaxBatches)
	}
	return body, units, batches, nil
}

// translateItemSync 对照翻译主体 (后台协程执行): 切段 → 分批翻译 → 组装双语 HTML
func (s *RSSService) translateItemSync(userID uint, title, htmlSrc string, report func(string)) (string, int, error) {
	body, units, batches, err := prepareRSSBatches(htmlSrc)
	if err != nil {
		return "", 0, err
	}

	zh := make([]string, len(units))
	for bi, batch := range batches {
		report(fmt.Sprintf("翻译中 %d/%d 批", bi+1, len(batches)))
		segs := make([]string, len(batch))
		for i, idx := range batch {
			segs[i] = units[idx].text
		}
		// 行文衔接: 上一批最后一个单元的结尾 (源文, 100 rune)
		prevTail := ""
		if bi > 0 {
			prev := units[batch[0]-1].text
			if r := []rune(prev); len(r) > 100 {
				prev = string(r[len(r)-100:])
			}
			prevTail = prev
		}
		// 对齐校验失败重试一次 (编号遗漏/格式抖动常见于长批)
		var out []string
		for attempt := 0; attempt < 2; attempt++ {
			out, err = s.translateBatch(userID, title, segs, prevTail)
			if err == nil {
				break
			}
		}
		if err != nil {
			return "", 0, fmt.Errorf("第 %d/%d 批翻译失败: %w", bi+1, len(batches), err)
		}
		for i, idx := range batch {
			zh[idx] = out[i]
		}
	}
	return assembleBilingualHTML(body, units, zh), len(units), nil
}

// markUnitSrc 给已翻译的原文块打标记类 (前端"仅译文"模式用它隐藏原文块)
func markUnitSrc(n *xhtml.Node) {
	for i, a := range n.Attr {
		if a.Key == "class" {
			if !strings.Contains(a.Val, "rss-src") {
				n.Attr[i].Val = strings.TrimSpace(a.Val + " rss-src")
			}
			return
		}
	}
	n.Attr = append(n.Attr, xhtml.Attribute{Key: "class", Val: "rss-src"})
}

// assembleBilingualHTML 把译文插回原文树 (就地改树): 块后插入 <div class="rss-zh">译文</div>,
// 已翻译的原文块加 class="rss-src" 标记; 父元素是 ul/ol/tr/table 时译文插进单元内部;
// 返回 body 子树序列化结果 (与 content_html 同构的片段)
func assembleBilingualHTML(body *xhtml.Node, units []*rssTranslateUnit, zh []string) string {
	for i, u := range units {
		text := strings.TrimSpace(zh[i])
		if text == "" {
			continue
		}
		markUnitSrc(u.node)
		zhNode := &xhtml.Node{
			Type: xhtml.ElementNode,
			Data: "div",
			Attr: []xhtml.Attribute{{Key: "class", Val: "rss-zh"}},
		}
		if u.tag == "pre" {
			zhNode.Attr[0].Val = "rss-zh rss-zh-pre"
		}
		zhNode.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: text})
		if u.node.Parent != nil && rssUnitInnerInsert[u.node.Parent.Data] {
			u.node.AppendChild(zhNode)
		} else if u.node.Parent != nil {
			if next := u.node.NextSibling; next != nil {
				u.node.Parent.InsertBefore(zhNode, next)
			} else {
				u.node.Parent.AppendChild(zhNode)
			}
		}
	}
	var sb strings.Builder
	for c := body.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(renderNode(c))
	}
	return sb.String()
}

// StartItemTranslate 生成中英对照 (异步任务): 段落级分批翻译, 结果缓存到条目。
// 已有未过期翻译直接返回即时 done; 同一条目进行中复用任务, 不重复烧 token。
func (s *RSSService) StartItemTranslate(userID, id uint) (*model.AsyncJob, error) {
	if s.readingSvc == nil || s.readingSvc.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	var item model.RSSItem
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		return nil, errors.New("文章不存在")
	}
	if running, err := asynctask.FindRunning(asynctask.KindRSSTranslate, userID, item.ID); err == nil && running != nil {
		return running, nil
	}
	// 确保正文可用 (复用 GetItemContent 的缓存/抓取/摘要兜底链路)
	full, err := s.GetItemContent(userID, id)
	if err != nil {
		return nil, err
	}
	htmlSrc := strings.TrimSpace(full.ContentHTML)
	if htmlSrc == "" {
		return nil, errors.New("本文没有可翻译的正文")
	}
	if len(htmlSrc) > rssTranslateMaxRunes {
		return nil, errors.New("正文过长, 暂不支持对照翻译")
	}
	hash := rssContentHash(htmlSrc)
	if item.BilingualHTML != "" && item.TranslationHash == hash {
		// 已有未过期翻译: 即时完成, 前端直接切对照视图
		return asynctask.InstantDone(asynctask.KindRSSTranslate, userID, item.ID), nil
	}
	itemID := item.ID
	title := full.Title
	return asynctask.Start(asynctask.KindRSSTranslate, userID, itemID, "", "rss:translate-item",
		func(report func(string)) (uint, string, error) {
			bilingual, _, err := s.translateItemSync(userID, title, htmlSrc, report)
			if err != nil {
				return 0, "", err
			}
			updates := map[string]interface{}{
				"bilingual_html":   bilingual,
				"translation_hash": hash,
				"translated_at":    time.Now(),
			}
			if e := DB.Model(&model.RSSItem{}).Where("id = ?", itemID).Updates(updates).Error; e != nil {
				return 0, "", e
			}
			return itemID, "", nil
		})
}
