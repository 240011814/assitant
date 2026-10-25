package service

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mmcdole/gofeed"
	"gorm.io/gorm"

	"backend/model"
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
)

var rssTagRe = regexp.MustCompile(`(?s)<[^>]*>`)

// AddSource 添加订阅: 先验证 feed 可解析, 成功后入库并抓首批条目
func (s *RSSService) AddSource(userID uint, req model.AddRSSSourceRequest) (*model.RSSSource, error) {
	feedURL := strings.TrimSpace(req.URL)
	if !strings.HasPrefix(feedURL, "http://") && !strings.HasPrefix(feedURL, "https://") {
		return nil, errors.New("订阅地址必须是 http(s) URL")
	}
	feed, err := fetchRSSFeed(feedURL)
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
		URL:     truncateRunes(feedURL, 500),
		Enabled: true,
	}
	if err := DB.Create(source).Error; err != nil {
		return nil, err
	}
	inserted := s.upsertItems(userID, source.ID, feed)
	_ = inserted
	now := time.Now()
	DB.Model(source).Updates(map[string]interface{}{"last_fetched_at": now})
	source.LastFetchedAt = &now
	return source, nil
}

// fetchRSSFeed 抓取并解析 feed (浏览器 UA, 15s 超时, 2MB 上限)
func fetchRSSFeed(feedURL string) (*gofeed.Feed, error) {
	client := &http.Client{Timeout: 15 * time.Second}
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
			UserID:      userID,
			SourceID:    sourceID,
			GUID:        guid,
			Title:       truncateRunes(title, 500),
			Link:        truncateRunes(item.Link, 500),
			Summary:     summary,
			PublishedAt: item.PublishedParsed,
		}
		if newItem.Link == "" {
			continue
		}
		if err := DB.Create(&newItem).Error; err == nil {
			inserted++
		}
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
		SourceID uint `gorm:"column:source_id"`
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

	feed, err := fetchRSSFeed(source.URL)
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
	if req.UnreadOnly {
		query = query.Where("status = ?", "unread")
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
	err := query.Select("id, user_id, source_id, guid, title, link, summary, published_at, status, imported_doc_id, created_at").
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

// GetItemContent 取条目正文: 缓存优先, 未命中则抓原文页并缓存
func (s *RSSService) GetItemContent(userID, id uint) (*model.RSSItem, error) {
	var item model.RSSItem
	if err := DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		return nil, errors.New("文章不存在")
	}
	if item.Content != "" {
		return &item, nil
	}

	// 直抓被盾且配置了 FlareSolverr 时自动回退 (readingSvc 在 main 中始终注入)
	text, _, err := s.readingSvc.fetchArticleText(item.Link)
	if err != nil {
		return nil, fmt.Errorf("抓取原文失败: %w", err)
	}
	if len(text) > 100_000 {
		// 按 rune 截断: 字节截断会切断多字节字符, 导致 utf8mb4 写入失败
		text = truncateRunes(text, 100_000)
	}
	if err := DB.Model(&item).Update("content", text).Error; err != nil {
		return nil, err
	}
	item.Content = text
	return &item, nil
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
