package learning

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"

	iface "backend/interface"
	"backend/model"
	"backend/service/ai/prompt"
	"backend/service/aitools"
	"backend/service/kernel"
	"backend/service/sys/asynctask"
)

// ReadingService 分级阅读: 导入文章 → LLM 标注生词 → 点词即查即加 → AI 生成个性化课文
type ReadingService struct {
	agentSvc  iface.TextGenerator
	courseSvc *CourseService
	systemCfg iface.ConfigValueGetter
}

func NewReadingService(agentSvc iface.TextGenerator, courseSvc *CourseService, systemCfg iface.ConfigValueGetter) *ReadingService {
	return &ReadingService{agentSvc: agentSvc, courseSvc: courseSvc, systemCfg: systemCfg}
}

var wordTokenRe = regexp.MustCompile(`[A-Za-z]+(?:'[a-zA-Z]+)?`)

// htmlTagRe 检测 text 导入中粘贴的 HTML 标签 (<p>/<br>/带属性的标签)
var htmlTagRe = regexp.MustCompile(`</?(p|br|div|span|h[1-6]|li|ul|ol|blockquote)\b[^>]*>`)

// maxFetchBytes URL 抓取大小上限
const maxFetchBytes = 2 << 20

// slowGenTimeout 课文生成/全文标注等慢 LLM 任务的专用超时, 不随全局 AI 超时收紧;
// 留 60s 余量给 nginx /api/ 的 proxy_read_timeout (300s)
const slowGenTimeout = 4 * time.Minute

// ImportDoc 导入阅读材料 (URL 自动抓取正文, 或直接用文本)
func (s *ReadingService) ImportDoc(userID uint, req model.ReadingImportRequest) (*model.ReadingDoc, error) {
	content := strings.TrimSpace(req.Content)
	title := strings.TrimSpace(req.Title)

	if req.SourceType == "url" {
		if req.SourceURL == "" {
			return nil, errors.New("缺少 URL")
		}
		text, pageTitle, _, err := s.fetchArticle(req.SourceURL)
		if err != nil {
			return nil, fmt.Errorf("抓取网页失败: %w", err)
		}
		content = text
		if title == "" {
			title = pageTitle
		}
	} else if content == "" {
		return nil, errors.New("正文不能为空")
	}
	// text 来源粘贴 HTML 时 (带 <p>/<br> 等标签) 转为纯文本分段,
	// 否则标签会被当正文渲染/朗读, 且整篇退化成单一段落
	if req.SourceType == "text" && htmlTagRe.MatchString(content) {
		text, _, err := htmlToText(content)
		if err == nil && strings.TrimSpace(text) != "" {
			content = text
		}
	}
	if title == "" {
		title = "未命名文章"
	}
	if len(content) > maxFetchBytes {
		// 按 rune 截断: 字节截断会切断多字节字符, 导致 utf8mb4 写入失败
		content = kernel.TruncateRunes(content, maxFetchBytes/4)
	}

	doc := &model.ReadingDoc{
		UserID:     userID,
		Title:      title,
		SourceType: req.SourceType,
		SourceURL:  req.SourceURL,
		Content:    content,
		WordCount:  len(wordTokenRe.FindAllString(content, -1)),
		Status:     "pending",
	}
	// 导入即算个性化难度预估 (免 LLM): 生词本交集 + 长词启发
	doc.EstUnknownRate = estimateUnknownRate(userID, content)
	if err := DB.Create(doc).Error; err != nil {
		return nil, err
	}
	return doc, nil
}

// httpStatusError 非成功响应 (供上层识别 403/429 等触发绕盾回退)
type httpStatusError struct{ Code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.Code) }

// fetchPageRaw 拉取页面原始 HTML (浏览器 UA, 15s 超时, 2MB 上限)
// outboundAllowPrivate 是否允许抓取环回/私网地址 (system_config rss_allow_private_network, 默认允许:
// 自部署内网 RSSHub 是文档化用法)。链路本地/云元数据地址 (169.254.0.0/16) 无论开关始终拒绝。
func (s *ReadingService) outboundAllowPrivate() bool {
	if s.systemCfg == nil {
		return true
	}
	if v, err := s.systemCfg.GetValue("rss_allow_private_network"); err == nil {
		return strings.TrimSpace(v) != "0"
	}
	return true
}

// fetchPageRaw 拉取页面原始 HTML (浏览器 UA, 15s 超时, 2MB 上限, 重定向逐跳复检目标地址)
func fetchPageRaw(rawURL string, allowPrivate bool) (string, error) {
	if err := aitools.ValidateOutboundURL(rawURL, allowPrivate); err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("重定向次数过多")
			}
			return aitools.ValidateOutboundURL(req.URL.String(), allowPrivate)
		},
	}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", &httpStatusError{Code: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// fetchURLText 抓取网页并提取正文纯文本 (fetchPageRaw + htmlToText)
func fetchURLText(rawURL string, allowPrivate bool) (string, string, error) {
	body, err := fetchPageRaw(rawURL, allowPrivate)
	if err != nil {
		return "", "", err
	}
	return htmlToText(body)
}

// htmlToText HTML → 纯文本 (段落以空行分隔), 附带 <title>
func htmlToText(body string) (string, string, error) {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "", "", err
	}
	var title, text string
	var extract func(n *html.Node)
	extract = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if n.FirstChild != nil {
					title = strings.TrimSpace(n.FirstChild.Data)
				}
				return
			case "script", "style", "noscript":
				return
			// BBC 等站的图片说明 (<figcaption>: "Image source, Getty Images"/"Image caption, ...")
			// 是给读屏器的无障碍标注, 对阅读与朗读均为噪音, 连内容一起跳过
			case "figcaption":
				return
			}
		}
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				text += t + " "
			}
		}
		isBlock := false
		if n.Type == html.ElementNode {
			switch n.Data {
			case "p", "div", "br", "li", "h1", "h2", "h3", "h4", "h5", "h6", "tr", "section", "article":
				isBlock = true
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			extract(c)
		}
		if isBlock {
			text += "\n\n"
		}
	}
	extract(root)
	// 压缩空白
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	return cleanImageCreditLines(text), title, nil
}

// narrowToArticle 正文定位:
// 1) 页面有 <article> 且内容够长 -> 只保留该子树;
// 2) 否则按 <p> 文本密度找最紧的正文容器 (导航/菜单是 ul>li>a 结构, 没有 p, 会被剔掉);
// 3) 都不奏效则原样返回整页。
func narrowToArticle(body string) string {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return body
	}
	var bodyNode *html.Node
	var findBody func(n *html.Node)
	findBody = func(n *html.Node) {
		if bodyNode != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "body" {
			bodyNode = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findBody(c)
		}
	}
	findBody(root)
	if bodyNode == nil {
		return body
	}

	var article *html.Node
	var findArticle func(n *html.Node)
	findArticle = func(n *html.Node) {
		if article != nil || n.Type != html.ElementNode {
			return
		}
		if n.Data == "article" {
			article = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findArticle(c)
		}
	}
	findArticle(bodyNode)
	if article != nil {
		var sb strings.Builder
		if err := html.Render(&sb, article); err == nil {
			out := sb.String()
			if len([]rune(stripTagsRE.ReplaceAllString(out, ""))) >= 200 {
				return out
			}
		}
	}

	pTextOf := map[*html.Node]int{}
	depthOf := map[*html.Node]int{}
	var walk func(n *html.Node, depth int) int
	walk = func(n *html.Node, depth int) int {
		sum := 0
		if n.Type == html.ElementNode {
			depthOf[n] = depth
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "p" {
				t := len([]rune(stripTagsRE.ReplaceAllString(renderNode(c), "")))
				sum += t
				continue
			}
			sum += walk(c, depth+1)
		}
		if n.Type == html.ElementNode {
			pTextOf[n] = sum
		}
		return sum
	}
	totalP := walk(bodyNode, 0)
	if totalP < 300 {
		return body
	}
	best := bodyNode
	bestDepth := -1
	for n, t := range pTextOf {
		if t*10 >= totalP*9 && t >= 300 && depthOf[n] > bestDepth {
			best = n
			bestDepth = depthOf[n]
		}
	}
	if best == bodyNode {
		return body
	}
	return renderNode(best)
}

// renderNode 序列化单个节点子树
func renderNode(n *html.Node) string {
	var sb strings.Builder
	if err := html.Render(&sb, n); err != nil {
		return ""
	}
	return sb.String()
}

// stripMenuLists 剔除菜单列表: 链接数 >=4 且链接文本占列表总文本 80% 以上的 ul/ol
// (站点导航的典型形态; 正文列表极少长这样)
func stripMenuLists(body string) string {
	root, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return body
	}
	var remove []*html.Node
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && (n.Data == "ul" || n.Data == "ol") {
			links, textLen := 0, 0
			var count func(c *html.Node)
			count = func(c *html.Node) {
				if c.Type == html.TextNode {
					textLen += len(strings.TrimSpace(c.Data))
					return
				}
				if c.Type == html.ElementNode {
					if c.Data == "a" {
						links++
						for t := c.FirstChild; t != nil; t = t.NextSibling {
							if t.Type == html.TextNode {
								textLen += len(strings.TrimSpace(t.Data))
							}
						}
						return
					}
					if c.Data == "p" {
						return
					}
				}
				for cc := c.FirstChild; cc != nil; cc = cc.NextSibling {
					count(cc)
				}
			}
			count(n)
			if links >= 4 && links*5 >= textLen {
				remove = append(remove, n)
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	for _, n := range remove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	var sb strings.Builder
	if err := html.Render(&sb, root); err != nil {
		return body
	}
	return sb.String()
}

var stripTagsRE = regexp.MustCompile(`(?s)<[^>]*>`)

// fetchArticle 抓文章页: 返回纯文本 + 净化 HTML 双输出。
// 直抓被盾 (403/429) 且配置了 flaresolverr_url 时, 走 FlareSolverr 无头浏览器回退
// (仅用于订阅自己有权访问的公开内容)
func (s *ReadingService) fetchArticle(pageURL string) (text, title, htmlOut string, err error) {
	body, err := fetchPageRaw(pageURL, s.outboundAllowPrivate())
	if err != nil {
		var statusErr *httpStatusError
		if !errors.As(err, &statusErr) || (statusErr.Code != http.StatusForbidden && statusErr.Code != http.StatusTooManyRequests) {
			return "", "", "", err
		}
		if s.systemCfg == nil {
			return "", "", "", err
		}
		fsURL := ""
		if v, e := s.systemCfg.GetValue("flaresolverr_url"); e == nil {
			fsURL = strings.TrimSpace(v)
		}
		if fsURL == "" {
			return "", "", "", err
		}
		body, err = fetchViaFlareSolverrHTML(fsURL, pageURL)
		if err != nil {
			return "", "", "", fmt.Errorf("直抓 %w; FlareSolverr 回退失败: %v", &httpStatusError{Code: statusErr.Code}, err)
		}
	}
	body = narrowToArticle(body)
	body = stripMenuLists(body)
	text, title, err = htmlToText(body)
	if err != nil {
		return "", "", "", err
	}
	htmlOut = sanitizeArticleHTML(pageURL, body)
	return text, title, htmlOut, nil
}

// fetchViaFlareSolverrHTML 通过 FlareSolverr 服务 (无头浏览器) 抓取被盾页面, 返回原始 HTML
func fetchViaFlareSolverrHTML(fsURL, pageURL string) (string, error) {
	client := &http.Client{Timeout: 90 * time.Second}
	payload, err := json.Marshal(map[string]interface{}{
		"cmd":        "request.get",
		"url":        pageURL,
		"maxTimeout": 60000,
	})
	if err != nil {
		return "", err
	}
	resp, err := client.Post(strings.TrimRight(fsURL, "/")+"/v1", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var out struct {
		Status   string `json:"status"`
		Message  string `json:"message"`
		Solution struct {
			Response string `json:"response"`
		} `json:"solution"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 10<<20)).Decode(&out); err != nil {
		return "", err
	}
	if out.Status != "ok" || out.Solution.Response == "" {
		return "", fmt.Errorf("%s: %s", out.Status, out.Message)
	}
	return out.Solution.Response, nil
}

// ListDocs 文档列表 (不含正文, keyword 模糊匹配标题与正文)
func (s *ReadingService) ListDocs(userID uint, keyword string) ([]model.ReadingDoc, error) {
	var list []model.ReadingDoc
	query := DB.Select("id, user_id, title, source_type, source_url, word_count, unknown_rate, status, created_at, updated_at").
		Where("user_id = ?", userID)
	if kw := strings.TrimSpace(keyword); kw != "" {
		like := "%" + kw + "%"
		query = query.Where("(title LIKE ? OR content LIKE ?)", like, like)
	}
	err := query.Order("created_at DESC").Find(&list).Error
	return list, err
}

// GetDoc 文档详情 (含正文与标注)
func (s *ReadingService) GetDoc(userID, id uint) (*model.ReadingDoc, []model.DifficultWord, error) {
	var doc model.ReadingDoc
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&doc).Error; err != nil {
		return nil, nil, errors.New("文档不存在")
	}
	LogReadingEvent(userID, "open", 1)
	// figcaption/图源标注清理上线前导入的旧文档, 读路径兜底清理展示噪音
	doc.Content = cleanImageCreditLines(doc.Content)
	var words []model.DifficultWord
	if doc.DifficultWords != "" {
		_ = json.Unmarshal([]byte(doc.DifficultWords), &words)
	}
	return &doc, words, nil
}

// DeleteDoc 删除文档
func (s *ReadingService) DeleteDoc(userID, id uint) error {
	return DB.Where("id = ? AND user_id = ?", id, userID).Delete(&model.ReadingDoc{}).Error
}

// HasDocByURL 判断是否已导入过同一原文链接的文章 (RSS 条目防重复导入)
func (s *ReadingService) HasDocByURL(userID uint, sourceURL string) bool {
	if strings.TrimSpace(sourceURL) == "" {
		return false
	}
	var count int64
	DB.Model(&model.ReadingDoc{}).Where("user_id = ? AND source_url = ?", userID, sourceURL).Count(&count)
	return count > 0
}

// StartAnnotate 异步启动生词标注: 立即置 annotating 并返回, LLM 在后台执行。
// Cloudflare 对源站响应有 100s 上限, 同步等 LLM 必 504, 故一律异步 + 前端轮询文档状态
func (s *ReadingService) StartAnnotate(userID, id uint) (*model.ReadingDoc, error) {
	if s.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	var doc model.ReadingDoc
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&doc).Error; err != nil {
		return nil, errors.New("文档不存在")
	}
	if doc.Status == "annotating" {
		// 进程在标注中途重启会把状态卡死在 annotating: 超过 10 分钟视为陈旧, 允许重新触发
		if time.Since(doc.UpdatedAt) < 10*time.Minute {
			return nil, errors.New("正在标注中, 请稍候")
		}
		log.Printf("reading annotate #%d 状态卡在 annotating 超过 10 分钟, 允许重试", id)
	}
	if err := DB.Model(&doc).Update("status", "annotating").Error; err != nil {
		return nil, err
	}
	kernel.SafeGo("reading:annotate", func() {
		if err := s.annotateDocSync(userID, id); err != nil {
			log.Printf("reading annotate #%d failed: %v", id, err)
		}
	})
	doc.Status = "annotating"
	return &doc, nil
}

// annotateDocSync 标注主体 (后台协程执行): 产出超纲词清单 + 生词率, 失败置 failed 供重试。
// 长文分块标注, 每块完成即落部分结果 (annotate_progress 逐块推进), 前端轮询可见进度与部分生词
func (s *ReadingService) annotateDocSync(userID, id uint) error {
	var doc model.ReadingDoc
	if err := DB.Where("id = ?", id).First(&doc).Error; err != nil {
		return err
	}

	// 兼顾旧文档: 过滤图片标注行后再送 LLM, 避免 "Getty Images" 等噪音被标成生词
	text := cleanImageCreditLines(doc.Content)
	if len(text) > 15000 {
		// 按 rune 截断: 字节截断会切断多字节字符, 中文文章会产生非法 UTF-8 发给 LLM
		text = kernel.TruncateRunes(text, 15000)
	}
	chunks := splitAnnotationChunks(text, 3)

	all := make([]model.DifficultWord, 0, 40)
	var known []string
	_ = DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Pluck("word", &known).Error
	knownSet := make(map[string]bool, len(known))
	for _, w := range known {
		knownSet[strings.ToLower(w)] = true
	}

	for i, chunk := range chunks {
		flagged, err := s.AnnotateWordsForText(userID, chunk)
		if err != nil {
			DB.Model(&doc).Updates(map[string]interface{}{"status": "failed"})
			return err
		}
		// 标记已在生词本中的词
		for j := range flagged {
			flagged[j].Known = knownSet[strings.ToLower(flagged[j].Word)]
		}
		all = append(all, flagged...)
		// 部分结果先落库 (status 保持 annotating, 前端轮询可见生词逐块出现)
		rawWords, _ := json.Marshal(all)
		progress := (i + 1) * 100 / len(chunks)
		if err := DB.Model(&doc).Updates(map[string]interface{}{
			"difficult_words":   string(rawWords),
			"annotate_progress": progress,
		}).Error; err != nil {
			return err
		}
	}
	// 解析成功但 0 个生词是合法结果 (文章全在掌握范围内), 正常落 annotated
	if len(all) == 0 {
		log.Printf("reading annotate #%d 标注结果为空 (模型未找出超纲词)", id)
	}

	unknownCount := 0
	for _, f := range all {
		if !f.Known {
			unknownCount++
		}
	}
	// 分母 = 实际送去标注的文本词数 (截断/分块后的口径), 与分子同口径
	annotatedWordCount := len(wordTokenRe.FindAllString(text, -1))
	unknownRate := 0.0
	if annotatedWordCount > 0 {
		unknownRate = float64(unknownCount) / float64(annotatedWordCount)
	}
	rawWords, _ := json.Marshal(all)
	return DB.Model(&doc).Updates(map[string]interface{}{
		"difficult_words":   string(rawWords),
		"unknown_rate":      unknownRate,
		"status":            "annotated",
		"annotate_progress": 100,
	}).Error
}

// AnnotateWordsForText 对一段文本做 LLM 生词标注 (分级阅读与 RSS 一键提取共用)
func (s *ReadingService) AnnotateWordsForText(userID uint, text string) ([]model.DifficultWord, error) {
	if s.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	system := prompt.GetPrompt(prompt.PromptKeyReadingAnnotate)
	user := fmt.Sprintf("文章:\n%s\n\n最多标注 40 个。", text)
	raw, err := s.agentSvc.GenerateTextWithTimeout(userID, "", system, user, slowGenTimeout)
	if err != nil {
		return nil, fmt.Errorf("标注失败: %w", err)
	}
	var flagged []model.DifficultWord
	if err := kernel.DecodeLLMArray(raw, &flagged); err != nil {
		return nil, errors.New("标注失败: AI 返回格式异常 (详见后端日志)")
	}
	return flagged, nil
}

// splitAnnotationChunks 把文本按段落边界近似均分成 n 块 (不足 n 段时返回实际块数)
func splitAnnotationChunks(text string, n int) []string {
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) <= n {
		return paragraphs
	}
	chunks := make([]string, n)
	per := len(paragraphs) / n
	extra := len(paragraphs) % n
	idx := 0
	for i := 0; i < n; i++ {
		take := per
		if i < extra {
			take++
		}
		chunks[i] = strings.Join(paragraphs[idx:idx+take], "\n\n")
		idx += take
	}
	return chunks
}

// LookupWord 点词即查: 词典缓存优先; 未命中先打免费词典 (dictionaryapi.dev) 只让 LLM 补中文,
// 免费词典也没有再走 LLM 全量; 命中生词本顺带计一次阅读相遇。结果写缓存。
func (s *ReadingService) LookupWord(userID uint, word string) (*LookupWordResponse, error) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" || len(word) > 100 {
		return nil, errors.New("单词不合法")
	}
	LogReadingEvent(userID, "lookup", 1)

	var entry model.DictionaryCache
	var cached model.DictionaryCache
	if err := DB.Where("word = ?", word).First(&cached).Error; err == nil {
		entry = cached
	} else {
		found := false
		if phonetic, definition, example, ok := lookupFreeDictionary(word); ok {
			// 免费词典命中: LLM 只补一句中文释义 (短调用, 秒级)
			translation := ""
			if s.agentSvc != nil {
				if raw, err := s.agentSvc.GenerateText(userID, "", prompt.GetPrompt(prompt.PromptKeyDictTranslate), definition); err == nil {
					translation = strings.TrimSpace(raw)
				}
			}
			entry = model.DictionaryCache{
				Word: word, Phonetic: phonetic, Definition: definition,
				Translation: translation, Example: example,
			}
			found = translation != ""
		}
		if !found {
			if s.agentSvc == nil {
				return nil, errors.New("AI 服务未启用")
			}
			system := prompt.GetPrompt(prompt.PromptKeyDictLLM)
			raw, err := s.agentSvc.GenerateText(userID, "", system, "单词: "+word)
			if err != nil {
				return nil, fmt.Errorf("查词失败: %w", err)
			}
			if err := kernel.DecodeLLMObject(raw, &entry); err != nil {
				return nil, errors.New("查词失败: AI 返回格式异常 (详见后端日志)")
			}
			entry.Word = word
		}
		// 缓存写失败不影响查询结果
		_ = DB.Where("word = ?", word).FirstOrCreate(&entry).Error
	}
	resp := &LookupWordResponse{DictionaryCache: entry}
	attachVocabularyState(userID, resp, word)
	return resp, nil
}

// StartGeneratePassage 异步启动 AI 写课文: 建异步任务行后立即返回,
// LLM 在后台执行, 前端轮询 /api/async-jobs/:id (Cloudflare 100s 源站响应上限, 不能同步等)
func (s *ReadingService) StartGeneratePassage(userID uint, req model.GeneratePassageRequest) (*model.AsyncJob, error) {
	if s.agentSvc == nil || s.courseSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	raw, _ := json.Marshal(req)
	return asynctask.Start(asynctask.KindPassage, userID, 0, string(raw), "reading:generate-passage",
		func(report func(string)) (uint, string, error) {
			course, _, err := s.generatePassageSync(userID, req)
			if err != nil {
				return 0, "", err
			}
			return course.ID, "", nil
		})
}

// generatePassageSync 课文生成主体 (后台协程执行): 把指定新词自然织入短文, 落成课程包
func (s *ReadingService) generatePassageSync(userID uint, req model.GeneratePassageRequest) (*model.Course, []string, error) {
	var words []model.Vocabulary
	if len(req.WordIDs) > 0 {
		if err := DB.Where("user_id = ? AND id IN ?", userID, req.WordIDs).Find(&words).Error; err != nil {
			return nil, nil, err
		}
	} else {
		if err := DB.Where("user_id = ? AND is_mastered = ? AND fsrs_state = ?", userID, false, model.FsrsStateNew).
			Order("created_at ASC").Limit(8).Find(&words).Error; err != nil {
			return nil, nil, err
		}
	}
	if len(words) == 0 {
		return nil, nil, errors.New("没有可用的新词, 请先添加生词")
	}
	if len(words) > 12 {
		words = words[:12]
	}

	count := req.SentenceCount
	if count <= 0 || count > 20 {
		count = 8
	}

	wordList := make([]string, 0, len(words))
	for _, w := range words {
		wordList = append(wordList, w.Word)
	}
	// 难度按最近一次词汇量测试校准 (没测过默认 B1)
	level := "B1"
	if a, err := GetLatestVocabTest(userID); err == nil && a.CEFR != "" {
		level = a.CEFR
	}
	// 难度占位符 {level} 替换 (不用 fmt.Sprintf: 提示词可配置, 用户文本里的 % 会被 Sprintf 破坏)
	system := strings.ReplaceAll(prompt.GetPrompt(prompt.PromptKeyReadingPassage), "{level}", level)
	user := fmt.Sprintf("必须使用的单词: %s\n句子数量: %d 句左右。", strings.Join(wordList, ", "), count)
	raw, err := s.agentSvc.GenerateTextWithTimeout(userID, "", system, user, slowGenTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("生成课文失败: %w", err)
	}

	var passage struct {
		Title     string `json:"title"`
		Sentences []struct {
			En string `json:"en"`
			Zh string `json:"zh"`
		} `json:"sentences"`
	}
	if err := kernel.DecodeLLMObject(raw, &passage); err != nil || len(passage.Sentences) == 0 {
		return nil, nil, errors.New("AI 返回格式异常 (详见后端日志)")
	}

	items := make([]model.CreateCourseItemRequest, 0, len(passage.Sentences))
	for i, s := range passage.Sentences {
		items = append(items, model.CreateCourseItemRequest{
			EnglishSentence:    s.En,
			ChineseTranslation: s.Zh,
			SortOrder:          i,
		})
	}
	course, err := s.courseSvc.CreateCourse(userID, model.CreateCourseRequest{
		Title:       passage.Title,
		Description: "AI 个性化课文: " + strings.Join(wordList, ", "),
		Tags:        []string{"AI课文"},
	})
	if err != nil {
		return nil, nil, err
	}
	if _, _, err := s.courseSvc.BatchCreateCourseItems(userID, course.ID, items); err != nil {
		return nil, nil, err
	}
	return course, wordList, nil
}
