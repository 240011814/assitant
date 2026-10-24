package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
}

func NewReadingService(agentSvc *AIAgentService, courseSvc *CourseService) *ReadingService {
	return &ReadingService{agentSvc: agentSvc, courseSvc: courseSvc}
}

var wordTokenRe = regexp.MustCompile(`[A-Za-z]+(?:'[a-zA-Z]+)?`)

// maxFetchBytes URL 抓取大小上限
const maxFetchBytes = 2 << 20

// ImportDoc 导入阅读材料 (URL 自动抓取正文, 或直接用文本)
func (s *ReadingService) ImportDoc(userID uint, req model.ReadingImportRequest) (*model.ReadingDoc, error) {
	content := strings.TrimSpace(req.Content)
	title := strings.TrimSpace(req.Title)

	if req.SourceType == "url" {
		if req.SourceURL == "" {
			return nil, errors.New("缺少 URL")
		}
		text, pageTitle, err := fetchURLText(req.SourceURL)
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
		content = content[:maxFetchBytes]
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

// fetchURLText 抓取网页并提取正文纯文本
func fetchURLText(rawURL string) (string, string, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return "", "", err
	}

	root, err := html.Parse(strings.NewReader(string(body)))
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

// ListDocs 文档列表 (不含正文)
func (s *ReadingService) ListDocs(userID uint) ([]model.ReadingDoc, error) {
	var list []model.ReadingDoc
	err := DB.Select("id, user_id, title, source_type, source_url, word_count, unknown_rate, status, created_at, updated_at").
		Where("user_id = ?", userID).Order("created_at DESC").Find(&list).Error
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

// AnnotateDoc LLM 标注生词: 产出超纲词清单 + 生词率 (同步调用, 已有词自动标记 known)
func (s *ReadingService) AnnotateDoc(userID, id uint) (*model.ReadingDoc, []model.DifficultWord, error) {
	if s.agentSvc == nil {
		return nil, nil, errors.New("AI 服务未启用")
	}
	var doc model.ReadingDoc
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&doc).Error; err != nil {
		return nil, nil, errors.New("文档不存在")
	}
	if doc.Status == "annotating" {
		return nil, nil, errors.New("正在标注中, 请稍候")
	}
	DB.Model(&doc).Update("status", "annotating")

	text := doc.Content
	if len(text) > 15000 {
		text = text[:15000]
	}
	system := `你是英语分级阅读标注助手。找出文中对学习者构成障碍的超纲生词 (低频词、习语、专有表达; 排除最常用 3000 词和普通专有名词), 给出中文释义。只输出 JSON 数组:
[{"word": "<原文中的词或短语>", "meaning": "<中文释义>", "reason": "<低频/习语/术语>"}]`
	user := fmt.Sprintf("文章:\n%s\n\n最多标注 40 个。", text)
	raw, err := s.agentSvc.GenerateText(userID, "", system, user)
	if err != nil {
		DB.Model(&doc).Update("status", "failed")
		return nil, nil, fmt.Errorf("标注失败: %w", err)
	}

	var flagged []model.DifficultWord
	if err := json.Unmarshal([]byte(extractJSON(raw)), &flagged); err != nil || len(flagged) == 0 {
		DB.Model(&doc).Update("status", "failed")
		return nil, nil, errors.New("AI 返回格式异常")
	}

	// 标记已在生词本中的词
	var known []string
	if err := DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Pluck("word", &known).Error; err == nil {
		knownSet := make(map[string]bool, len(known))
		for _, w := range known {
			knownSet[strings.ToLower(w)] = true
		}
		unknown := 0
		for i := range flagged {
			flagged[i].Known = knownSet[strings.ToLower(flagged[i].Word)]
			if !flagged[i].Known {
				unknown++
			}
		}
	}
	if doc.WordCount > 0 {
		unknownCount := 0
		for _, f := range flagged {
			if !f.Known {
				unknownCount++
			}
		}
		doc.UnknownRate = float64(unknownCount) / float64(doc.WordCount)
	}
	rawWords, _ := json.Marshal(flagged)
	updates := map[string]interface{}{
		"difficult_words": string(rawWords),
		"unknown_rate":    doc.UnknownRate,
		"status":          "annotated",
	}
	if err := DB.Model(&doc).Updates(updates).Error; err != nil {
		return nil, nil, err
	}
	return &doc, flagged, nil
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
	if err := json.Unmarshal([]byte(extractJSON(raw)), &entry); err != nil {
		return nil, errors.New("AI 返回格式异常")
	}
	entry.Word = word
	if err := DB.Where("word = ?", word).FirstOrCreate(&entry).Error; err != nil {
		// 缓存写失败不影响查询结果
		return &entry, nil
	}
	return &entry, nil
}

// GeneratePassage AI 生成个性化课文: 把指定新词自然织入短文, 落成课程包 (可接着做听写/翻译练习)
func (s *ReadingService) GeneratePassage(userID uint, req model.GeneratePassageRequest) (*model.Course, []string, error) {
	if s.agentSvc == nil || s.courseSvc == nil {
		return nil, nil, errors.New("AI 服务未启用")
	}

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
	raw, err := s.agentSvc.GenerateText(userID, "", system, user)
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
	if err := json.Unmarshal([]byte(extractJSON(raw)), &passage); err != nil || len(passage.Sentences) == 0 {
		return nil, nil, errors.New("AI 返回格式异常")
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
