package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"

	"backend/model"
	"backend/service/tools"
)

// RSSService RSS 订阅: gofeed 解析 feed, 条目按 GUID 去重入库, 正文按需抓取缓存
type RSSService struct {
	readingSvc *ReadingService
}

func NewRSSService(readingSvc *ReadingService) *RSSService {
	return &RSSService{readingSvc: readingSvc}
}

// rssFetchLimits 单次抓取的体积与条目上限
const (
	rssFetchMaxBytes = 2 << 20
	rssFetchMaxItems = 100
	rssSummaryLimit  = 1000

	// rssRefreshConcurrency 定时刷新全部源的并发上限
	rssRefreshConcurrency = 4
)

var rssTagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// AddSource 添加订阅: 先本地查重再验证 feed 可解析 (重复添加不必先付一次网络往返), 成功后入库并抓首批条目
func (s *RSSService) AddSource(userID uint, req model.AddRSSSourceRequest) (*model.RSSSource, error) {
	feedURL := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(feedURL, "http://") && !strings.HasPrefix(feedURL, "https://") {
		return nil, errors.New("订阅地址必须是 http(s) URL")
	}
	feedURL = truncateRunes(feedURL, 500)

	// 同一用户重复订阅同一 feed: 直接拒绝, 避免产生没有条目的空壳源
	// (并发双击的漏网由 uk_rss_user_url 唯一索引兜底)
	var dupCount int64
	DB.Model(&model.RSSSource{}).Where("user_id = ? AND url = ?", userID, feedURL).Count(&dupCount)
	if dupCount > 0 {
		return nil, errors.New("该 feed 已订阅过, 请勿重复添加")
	}

	feed, err := s.fetchFeed(feedURL)
	if err != nil {
		return nil, fmt.Errorf("订阅源无法解析: %w", err)
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = feed.Title
	}
	if title == "" {
		title = feedURL
	}
	title = strings.TrimSpace(truncateRunes(title, 100))

	source := &model.RSSSource{
		UserID:  userID,
		Title:   title,
		URL:     feedURL,
		Enabled: true,
	}
	if err := DB.Create(source).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, errors.New("该 feed 已订阅过, 请勿重复添加")
		}
		return nil, err
	}
	inserted := s.upsertItems(userID, source.ID, feed)
	_ = inserted
	now := time.Now()
	DB.Model(source).Updates(map[string]interface{}{"last_fetched_at": now})
	source.LastFetchedAt = &now
	return source, nil
}

// fetchFeed 按当前私网策略抓取并解析 feed
func (s *RSSService) fetchFeed(feedURL string) (*gofeed.Feed, error) {
	return fetchRSSFeed(feedURL, s.privateNetAllowed())
}

// privateNetAllowed 是否允许环回/私网 feed 地址 (与阅读抓取共用 system_config 开关)
func (s *RSSService) privateNetAllowed() bool {
	if s.readingSvc == nil {
		return true
	}
	return s.readingSvc.outboundAllowPrivate()
}

// fetchRSSFeed 抓取并解析 feed (浏览器 UA, 15s 超时, 2MB 上限, 重定向逐跳复检目标地址)
func fetchRSSFeed(feedURL string, allowPrivate bool) (*gofeed.Feed, error) {
	if err := tools.ValidateOutboundURL(feedURL, allowPrivate); err != nil {
		return nil, err
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("重定向次数过多")
			}
			return tools.ValidateOutboundURL(req.URL.String(), allowPrivate)
		},
	}
	req, err := http.NewRequest(http.MethodGet, feedURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml, text/html, */*")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// 403/429 多为反爬盾 (典型: rsshub.app 公共实例拦程序抓取), 单改请求头无解
		if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusTooManyRequests {
			return nil, fmt.Errorf("HTTP %d: 站点有反爬盾拦截程序抓取; rsshub.app 等公共实例会这样, 请自部署 RSSHub (docker run diygod/rsshub) 后用其内网地址, 或改用带 RSS 的源", resp.StatusCode)
		}
		// 5xx: 自部署 RSSHub 常见于路由的上游站抓取失败, 去看 RSSHub 容器日志
		if resp.StatusCode >= 500 {
			return nil, fmt.Errorf("HTTP %d: 源服务器内部错误; 若是自部署 RSSHub, 多为该路由的上游站抓取失败, 查看 RSSHub 容器日志定位", resp.StatusCode)
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, rssFetchMaxBytes))
	if err != nil {
		return nil, err
	}
	parser := gofeed.NewParser()
	return parser.ParseString(string(body))
}

// upsertItems 按 (user, guid) 去重插入条目, 返回新增条数
func (s *RSSService) upsertItems(userID, sourceID uint, feed *gofeed.Feed) int {
	items := feed.Items
	if len(items) > rssFetchMaxItems {
		items = items[:rssFetchMaxItems]
	}

	// 预载该源已有 GUID, 批量判重
	var existing []string
	DB.Model(&model.RSSItem{}).Where("user_id = ? AND source_id = ?", userID, sourceID).Pluck("guid", &existing)
	seen := make(map[string]bool, len(existing))
	for _, g := range existing {
		seen[g] = true
	}

	inserted := 0

	// 批量难度预估共用一次生词本集合查询
	unknownSet, err := loadUnmasteredSet(userID)
	if err != nil {
		unknownSet = map[string]bool{}
	}
	for _, item := range items {
		guid := item.GUID
		if guid == "" {
			guid = item.Link
		}
		guid = truncateRunes(guid, 255)
		if guid == "" || seen[guid] {
			continue
		}
		seen[guid] = true

		title := item.Title
		if title == "" {
			title = item.Link
		}
		summary := strings.TrimSpace(rssTagRe.ReplaceAllString(item.Description, " "))
		summary = truncateRunes(strings.Join(strings.Fields(summary), " "), rssSummaryLimit)

		newItem := model.RSSItem{
			UserID:         userID,
			SourceID:       sourceID,
			GUID:           guid,
			Title:          truncateRunes(title, 500),
			Link:           truncateRunes(item.Link, 500),
			Summary:        summary,
			PublishedAt:    item.PublishedParsed,
			EstUnknownRate: estimateUnknownRateFromSet(summary+"\n"+title, unknownSet),
		}
		if newItem.Link == "" {
			continue
		}
		if err := DB.Create(&newItem).Error; err != nil {
			// 唯一键冲突 (并发抓取同源) 静默跳过; 其余错误记日志, 避免条目悄悄丢失无迹可查
			msg := strings.ToLower(err.Error())
			if !strings.Contains(msg, "unique") && !strings.Contains(msg, "duplicate") {
				log.Printf("rss insert item source=%d guid=%s: %v", sourceID, guid, err)
			}
			continue
		}
		inserted++
	}
	return inserted
}

// ListSources 订阅源列表
func (s *RSSService) ListSources(userID uint) ([]model.RSSSource, error) {
	var list []model.RSSSource
	err := DB.Where("user_id = ?", userID).Order("created_at ASC").Find(&list).Error
	return list, err
}

// RSSSourceWithUnread 订阅源 + 未读数 (侧栏展示)
type RSSSourceWithUnread struct {
	model.RSSSource
	Unread int64 `json:"unread"`
}

// ListSourcesWithUnread 订阅源列表, 附各源未读数
func (s *RSSService) ListSourcesWithUnread(userID uint) ([]RSSSourceWithUnread, error) {
	sources, err := s.ListSources(userID)
	if err != nil {
		return nil, err
	}
	type cntRow struct {
		SourceID uint  `gorm:"column:source_id"`
		Cnt      int64 `gorm:"column:cnt"`
	}
	var rows []cntRow
	if err := DB.Model(&model.RSSItem{}).
		Select("source_id, COUNT(*) as cnt").
		Where("user_id = ? AND status = 'unread'", userID).
		Group("source_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	unread := make(map[uint]int64, len(rows))
	for _, r := range rows {
		unread[r.SourceID] = r.Cnt
	}
	out := make([]RSSSourceWithUnread, 0, len(sources))
	for _, src := range sources {
		out = append(out, RSSSourceWithUnread{RSSSource: src, Unread: unread[src.ID]})
	}
	return out, nil
}

// UpdateSource 编辑订阅 (改名 / 换 feed 地址)。空字段保持原值;
// URL 变更时重新验证 http(s) 前缀、查重 (排除自身) 且 feed 可解析, 已抓条目保留。
func (s *RSSService) UpdateSource(userID, id uint, req model.UpdateRSSSourceRequest) (*model.RSSSource, error) {
	var source model.RSSSource
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&source).Error; err != nil {
		return nil, errors.New("订阅源不存在")
	}

	updates := map[string]interface{}{}
	newURL := strings.TrimSpace(req.URL)
	if newURL != "" && newURL != source.URL {
		if !strings.HasPrefix(newURL, "http://") && !strings.HasPrefix(newURL, "https://") {
			return nil, errors.New("订阅地址必须是 http(s) URL")
		}
		newURL = truncateRunes(newURL, 500)
		var dupCount int64
		DB.Model(&model.RSSSource{}).Where("user_id = ? AND url = ? AND id <> ?", userID, newURL, id).Count(&dupCount)
		if dupCount > 0 {
			return nil, errors.New("该 feed 已订阅过, 请勿重复添加")
		}
		if _, err := s.fetchFeed(newURL); err != nil {
			return nil, fmt.Errorf("订阅源无法解析: %w", err)
		}
		updates["url"] = newURL
	}
	if title := strings.TrimSpace(req.Title); title != "" {
		updates["title"] = truncateRunes(title, 100)
	}

	if len(updates) > 0 {
		if err := DB.Model(&source).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	var fresh model.RSSSource
	if err := DB.Where("id = ?", id).First(&fresh).Error; err != nil {
		return nil, err
	}
	return &fresh, nil
}

// DeleteSource 删除订阅源及其条目
func (s *RSSService) DeleteSource(userID, id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&model.RSSSource{}).Error; err != nil {
			return err
		}
		return tx.Where("user_id = ? AND source_id = ?", userID, id).Delete(&model.RSSItem{}).Error
	})
}

// FetchSource 手动抓取单个源, 返回新增条数
func (s *RSSService) FetchSource(userID, id uint) (int, error) {
	var source model.RSSSource
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&source).Error; err != nil {
		return 0, errors.New("订阅源不存在")
	}
	if !source.Enabled {
		return 0, errors.New("订阅源已停用")
	}

	feed, err := s.fetchFeed(source.URL)
	if err != nil {
		DB.Model(&source).Updates(map[string]interface{}{
			"last_fetched_at": time.Now(),
			"last_error":      truncateRunes(err.Error(), 500),
		})
		return 0, fmt.Errorf("抓取失败: %w", err)
	}
	inserted := s.upsertItems(userID, source.ID, feed)
	DB.Model(&source).Updates(map[string]interface{}{
		"last_fetched_at": time.Now(),
		"last_error":      "",
	})
	return inserted, nil
}

// RefreshAllSources 定时任务入口: 抓取全部用户的全部启用订阅源。
// 复用 FetchSource 的单源逻辑 (错误落到 last_error, 条目按 GUID 去重),
// 并发限流 rssRefreshConcurrency 避免瞬时打满出网; 单源失败不影响其余源。
// 返回 (源数, 新增条目数, 失败源数)。
func (s *RSSService) RefreshAllSources() (int, int, int) {
	// 字段必须导出, 否则 GORM Scan 映射不到列
	type sourceRef struct {
		UserID uint
		ID     uint
	}
	var refs []sourceRef
	if err := DB.Model(&model.RSSSource{}).
		Where("enabled = ?", true).
		Order("user_id, id").
		Select("user_id, id").
		Scan(&refs).Error; err != nil {
		log.Printf("[rss.refresh] 读取订阅源列表失败: %v", err)
		return 0, 0, 0
	}

	var (
		mu        sync.Mutex
		inserted  int
		failed    int
		wg        sync.WaitGroup
		semaphore = make(chan struct{}, rssRefreshConcurrency)
	)
	for _, ref := range refs {
		wg.Add(1)
		go func(ref sourceRef) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			n, err := s.FetchSource(ref.UserID, ref.ID)
			mu.Lock()
			defer mu.Unlock()
			inserted += n
			if err != nil {
				failed += 1
				log.Printf("[rss.refresh] 源 %d (用户 %d) 抓取失败: %v", ref.ID, ref.UserID, err)
			}
		}(ref)
	}
	wg.Wait()
	log.Printf("[rss.refresh] 完成: %d 个源, 新增 %d 条, 失败 %d 个", len(refs), inserted, failed)
	return len(refs), inserted, failed
}

// ListItems 文章列表 (分页, 可按源/未读过滤)
func (s *RSSService) ListItems(userID uint, req model.ListRSSItemsRequest) ([]model.RSSItem, int64, error) {
	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 || req.PageSize > 100 {
		req.PageSize = 20
	}

	query := DB.Model(&model.RSSItem{}).Where("user_id = ?", userID)
	if req.SourceID > 0 {
		query = query.Where("source_id = ?", req.SourceID)
	}
	// 已读过滤: 已导入的条目 (status=imported) 导入前必然读过, 也算已读
	switch req.Status {
	case "unread":
		query = query.Where("status = ?", "unread")
	case "read":
		query = query.Where("status <> ?", "unread")
	}
	if req.EasyOnly {
		// 适合当前水平: 已算难度且预估生词率 <= 8% (est=0 是未算的历史数据, 不算简单)
		query = query.Where("est_unknown_rate > 0 AND est_unknown_rate <= 0.08")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 排序: 按发布时间(缺失用创建时间)升降序, 纯时间序不掺未读优先
	order := "COALESCE(published_at, created_at) DESC"
	if req.Sort == "time_asc" {
		order = "COALESCE(published_at, created_at) ASC"
	}

	var list []model.RSSItem
	// 列表不取正文 (mediumtext, 体积大且列表用不到), 详情接口按需抓取后返回
	err := query.Select("id, user_id, source_id, guid, title, link, summary, est_unknown_rate, published_at, status, imported_doc_id, created_at").
		Order(order).
		Offset((req.Page - 1) * req.PageSize).Limit(req.PageSize).
		Find(&list).Error
	return list, total, err
}

// MarkItemRead 标记已读
func (s *RSSService) MarkItemRead(userID, id uint) error {
	return DB.Model(&model.RSSItem{}).
		Where("id = ? AND user_id = ? AND status = 'unread'", id, userID).
		Update("status", "read").Error
}

// rssItemContentLocks 按条目 ID 串行化正文抓取, 防并发缓存击穿 (同篇被重复抓取+双写)
var rssItemContentLocks sync.Map

// GetItemContent 取条目正文: 双格式 (纯文本 + 净化 HTML) 缓存优先, 未命中则抓原文页。
// 旧缓存只有纯文本时, 阅读视图回退用 feed 摘要的净化 HTML
func (s *RSSService) GetItemContent(userID, id uint) (*model.RSSItem, error) {
	var item model.RSSItem
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		return nil, errors.New("文章不存在")
	}
	LogReadingEvent(userID, "open", 1)
	if item.Content == "" && item.ContentHTML == "" {
		muAny, _ := rssItemContentLocks.LoadOrStore(item.ID, &sync.Mutex{})
		mu := muAny.(*sync.Mutex)
		mu.Lock()
		err := s.fillItemContent(&item)
		mu.Unlock()
		if err != nil {
			return nil, err
		}
	}
	// 缓存无 HTML (旧数据) 或抓取失败时, 用 feed 摘要的净化 HTML 兜底 (不落库)
	if item.ContentHTML == "" && strings.TrimSpace(item.Summary) != "" {
		item.ContentHTML = sanitizeArticleHTML(item.Link, item.Summary)
	}
	return &item, nil
}

// fillItemContent 抓取原文页并写缓存 (调用方需持有该条目的抓取锁)
func (s *RSSService) fillItemContent(item *model.RSSItem) error {
	// 双检: 等锁期间缓存可能已被并发请求填充
	var fresh model.RSSItem
	if err := DB.Where("id = ?", item.ID).First(&fresh).Error; err == nil &&
		(fresh.Content != "" || fresh.ContentHTML != "") {
		*item = fresh
		return nil
	}
	// 直抓被盾且配置了 FlareSolverr 时自动回退 (readingSvc 在 main 中始终注入)
	text, _, htmlOut, err := s.readingSvc.fetchArticle(item.Link)
	if err != nil {
		return fmt.Errorf("抓取原文失败: %w", err)
	}
	if len(text) > 100_000 {
		// 按 rune 截断: 字节截断会切断多字节字符, 导致 utf8mb4 写入失败
		text = truncateRunes(text, 100_000)
	}
	if err := DB.Model(item).Updates(map[string]interface{}{"content": text, "content_html": htmlOut}).Error; err != nil {
		return err
	}
	item.Content = text
	item.ContentHTML = htmlOut
	return nil
}

// ImportToReading 把条目导入分级阅读 (正文已缓存则直接复用), 返回阅读文档
func (s *RSSService) ImportToReading(userID, id uint) (*model.ReadingDoc, error) {
	if s.readingSvc == nil {
		return nil, errors.New("阅读服务未启用")
	}
	item, err := s.GetItemContent(userID, id)
	if err != nil {
		return nil, err
	}
	if s.readingSvc.HasDocByURL(userID, item.Link) {
		return nil, errors.New("该文章已导入过")
	}
	doc, err := s.readingSvc.ImportDoc(userID, model.ReadingImportRequest{
		Title:      item.Title,
		SourceType: "text",
		SourceURL:  item.Link,
		Content:    item.Content,
	})
	if err != nil {
		return nil, err
	}
	DB.Model(&model.RSSItem{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
		"status":          "imported",
		"imported_doc_id": doc.ID,
	})
	item.Status = "imported"
	item.ImportedDocID = doc.ID
	return doc, nil
}

// StartItemAnnotate 一键提取本文生词 (异步 job): 结果缓存在条目上, 与分级阅读标注同源。
// 正文未缓存时先抓正文; 都没有用摘要兜底。
func (s *RSSService) StartItemAnnotate(userID, id uint) (*model.AITextJob, error) {
	var item model.RSSItem
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		return nil, errors.New("文章不存在")
	}
	if item.DifficultWords != "" {
		// 已提取过: 返回一个即时完成的 job, 前端直接刷新详情
		return &model.AITextJob{UserID: userID, Kind: "rss_annotate", Status: "done", ResultID: item.ID}, nil
	}
	// 确保正文可用 (利用 GetItemContent 的缓存/抓取/摘要兜底链路)
	full, err := s.GetItemContent(userID, id)
	if err != nil {
		return nil, err
	}
	text := full.Content
	if strings.TrimSpace(text) == "" {
		text = full.Summary
	}
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("本文没有可标注的文本")
	}
	if len(text) > 15000 {
		text = truncateRunes(text, 15000)
	}
	job := &model.AITextJob{UserID: userID, Kind: "rss_annotate", Status: "running", ResultID: item.ID}
	if err := DB.Create(job).Error; err != nil {
		return nil, err
	}
	SafeGo("rss:annotate-item", func() {
		updates := map[string]interface{}{}
		flagged, err := s.readingSvc.AnnotateWordsForText(userID, text)
		if err != nil {
			updates["status"] = "failed"
			updates["error"] = err.Error()
		} else {
			// 标记已在生词本中的词
			var known []string
			if e := DB.Model(&model.Vocabulary{}).Where("user_id = ?", userID).Pluck("word", &known).Error; e == nil {
				knownSet := make(map[string]bool, len(known))
				for _, w := range known {
					knownSet[strings.ToLower(w)] = true
				}
				for i := range flagged {
					flagged[i].Known = knownSet[strings.ToLower(flagged[i].Word)]
				}
			}
			raw, _ := json.Marshal(flagged)
			if e := DB.Model(&model.RSSItem{}).Where("id = ?", item.ID).
				Update("difficult_words", string(raw)).Error; e != nil {
				updates["status"] = "failed"
				updates["error"] = e.Error()
			} else {
				updates["status"] = "done"
			}
		}
		if err := DB.Model(&model.AITextJob{}).Where("id = ?", job.ID).Updates(updates).Error; err != nil {
			log.Printf("rss annotate job #%d update failed: %v", job.ID, err)
		}
	})
	return job, nil
}
