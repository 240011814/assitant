package service

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

	"backend/model"
)

// ReadingService 分级阅读: 导入文章 → LLM 标注生词 → 点词即查即加 → AI 生成个性化课文
type ReadingService struct {
	agentSvc  *AIAgentService
	courseSvc *CourseService
	systemCfg *SystemConfigService
}

func NewReadingService(agentSvc *AIAgentService, courseSvc *CourseService, systemCfg *SystemConfigService) *ReadingService {
	return &ReadingService{agentSvc: agentSvc, courseSvc: courseSvc, systemCfg: systemCfg}
}

var wordTokenRe = regexp.MustCompile(`[A-Za-z]+(?:'[a-zA-Z]+)?`)

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
	if title == "" {
		title = "未命名文章"
	}
	if len(content) > maxFetchBytes {
		// 按 rune 截断: 字节截断会切断多字节字符, 导致 utf8mb4 写入失败
		content = truncateRunes(content, maxFetchBytes/4)
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
	if err := DB.Create(doc).Error; err != nil {
		return nil, err
	}
	return doc, nil
}

// httpStatusError 非成功响应 (供上层识别 403/429 等触发绕盾回退)
type httpStatusError struct{ Code int }

func (e *httpStatusError) Error() string { return fmt.Sprintf("HTTP %d", e.Code) }

// fetchPageRaw 拉取页面原始 HTML (浏览器 UA, 15s 超时, 2MB 上限)
func fetchPageRaw(rawURL string) (string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
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
func fetchURLText(rawURL string) (string, string, error) {
	body, err := fetchPageRaw(rawURL)
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
	return strings.TrimSpace(text), title, nil
}

// fetchArticle 抓文章页: 返回纯文本 + 净化 HTML 双输出。
// 直抓被盾 (403/429) 且配置了 flaresolverr_url 时, 走 FlareSolverr 无头浏览器回退
// (仅用于订阅自己有权访问的公开内容)
func (s *ReadingService) fetchArticle(pageURL string) (text, title, htmlOut string, err error) {
	body, err := fetchPageRaw(pageURL)
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
		Status  string `json:"status"`
		Message string `json:"message"`
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
	SafeGo("reading:annotate", func() {
		if err := s.annotateDocSync(userID, id); err != nil {
			log.Printf("reading annotate #%d failed: %v", id, err)
		}
	})
	doc.Status = "annotating"
	return &doc, nil
}

// annotateDocSync 标注主体 (后台协程执行): 产出超纲词清单 + 生词率, 失败置 failed 供重试
func (s *ReadingService) annotateDocSync(userID, id uint) error {
	var doc model.ReadingDoc
	if err := DB.Where("id = ?", id).First(&doc).Error; err != nil {
		return err
	}

	text := doc.Content
	if len(text) > 15000 {
		text = text[:15000]
	}
	system := `你是英语分级阅读标注助手。找出文中对学习者构成障碍的超纲生词 (低频词、习语、专有表达; 排除最常用 3000 词和普通专有名词), 给出中文释义。只输出 JSON 数组:
[{"word": "<原文中的词或短语>", "meaning": "<中文释义>", "reason": "<低频/习语/术语>"}]`
	user := fmt.Sprintf("文章:\n%s\n\n最多标注 40 个。", text)
	raw, err := s.agentSvc.GenerateTextWithTimeout(userID, "", system, user, slowGenTimeout)
	if err != nil {
		DB.Model(&doc).Update("status", "failed")
		return fmt.Errorf("标注失败: %w", err)
	}

	var flagged []model.DifficultWord
	if err := decodeLLMArray(raw, &flagged); err != nil {
		DB.Model(&doc).Update("status", "failed")
		return errors.New("标注失败: AI 返回格式异常 (详见后端日志)")
	}
	// 解析成功但 0 个生词是合法结果 (文章全在掌握范围内), 正常落 annotated
	if len(flagged) == 0 {
		log.Printf("reading annotate #%d 标注结果为空 (模型未找出超纲词)", id)
	}

	// 标记已在生词本中的词
	var known []string
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Pluck("word", &known).Error; err == nil {
		knownSet := make(map[string]bool, len(known))
		for _, w := range known {
			knownSet[strings.ToLower(w)] = true
		}
		for i := range flagged {
			flagged[i].Known = knownSet[strings.ToLower(flagged[i].Word)]
		}
	}
	unknownCount := 0
	for _, f := range flagged {
		if !f.Known {
			unknownCount++
		}
	}
	unknownRate := 0.0
	if doc.WordCount > 0 {
		unknownRate = float64(unknownCount) / float64(doc.WordCount)
	}
	rawWords, _ := json.Marshal(flagged)
	return DB.Model(&doc).Updates(map[string]interface{}{
		"difficult_words": string(rawWords),
		"unknown_rate":    unknownRate,
		"status":          "annotated",
	}).Error
}

// LookupWord 点词即查: 词典缓存优先, 未命中走 LLM 并写入缓存
func (s *ReadingService) LookupWord(userID uint, word string) (*model.DictionaryCache, error) {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" || len(word) > 100 {
		return nil, errors.New("单词不合法")
	}

	var cached model.DictionaryCache
	if err := DB.Where("word = ?", word).First(&cached).Error; err == nil {
		return &cached, nil
	}
	if s.agentSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}

	system := `你是英语词典助手。查一个词并给出词典式解释。只输出 JSON:
{"phonetic": "<音标, 可空>", "definition": "<英文释义, 一句>", "translation": "<中文释义>", "example": "<一个自然例句>"}`
	raw, err := s.agentSvc.GenerateText(userID, "", system, "单词: "+word)
	if err != nil {
		return nil, fmt.Errorf("查词失败: %w", err)
	}

	var entry model.DictionaryCache
	if err := decodeLLMObject(raw, &entry); err != nil {
		return nil, errors.New("查词失败: AI 返回格式异常 (详见后端日志)")
	}
	entry.Word = word
	if err := DB.Where("word = ?", word).FirstOrCreate(&entry).Error; err != nil {
		// 缓存写失败不影响查询结果
		return &entry, nil
	}
	return &entry, nil
}

// StartGeneratePassage 异步启动 AI 写课文: 建 ai_text_jobs 任务行后立即返回,
// LLM 在后台执行, 前端轮询任务状态 (Cloudflare 100s 源站响应上限, 不能同步等)
func (s *ReadingService) StartGeneratePassage(userID uint, req model.GeneratePassageRequest) (*model.AITextJob, error) {
	if s.agentSvc == nil || s.courseSvc == nil {
		return nil, errors.New("AI 服务未启用")
	}
	job := &model.AITextJob{UserID: userID, Kind: "passage", Status: "running"}
	if err := DB.Create(job).Error; err != nil {
		return nil, err
	}
	SafeGo("reading:generate-passage", func() {
		updates := map[string]interface{}{}
		course, _, err := s.generatePassageSync(userID, req)
		if err != nil {
			updates["status"] = "failed"
			updates["error"] = err.Error()
		} else {
			updates["status"] = "done"
			updates["result_id"] = course.ID
		}
		if err := DB.Model(&model.AITextJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
			log.Printf("reading passage job #%d update failed: %v", job.ID, err)
		}
	})
	return job, nil
}

// GetGenerationJob 查询慢生成任务状态 (前端轮询)
func (s *ReadingService) GetGenerationJob(userID, jobID uint) (*model.AITextJob, error) {
	var job model.AITextJob
	if err := DB.Where("id = ? AND user_id = ?", jobID, userID).First(&job).Error; err != nil {
		return nil, errors.New("任务不存在")
	}
	return &job, nil
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
	system := `你是英语课文编写助手。写一篇短文, 把指定的每个单词都自然地用到至少一个句子里 (不要生硬堆砌), 难度约 CEFR B1。只输出 JSON:
{"title": "<英文标题>", "sentences": [{"en": "<英文句子>", "zh": "<中文翻译>"}]}`
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
	if err := decodeLLMObject(raw, &passage); err != nil || len(passage.Sentences) == 0 {
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
