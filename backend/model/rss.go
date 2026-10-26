package model

import (
	"time"
)

// RSSSource RSS 订阅源
type RSSSource struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID        uint       `gorm:"not null;index:idx_rss_source_user,priority:1;uniqueIndex:uk_rss_user_url,priority:1" json:"userId"`
	Title         string     `gorm:"size:100;not null" json:"title"`
	URL           string     `gorm:"size:500;not null;uniqueIndex:uk_rss_user_url,priority:2" json:"url"`
	Enabled       bool       `gorm:"not null;default:true" json:"enabled"`
	LastFetchedAt *time.Time `json:"lastFetchedAt"`
	LastError     string     `gorm:"size:500" json:"lastError"`
	CreatedAt     time.Time  `gorm:"index:idx_rss_source_user,priority:2" json:"createdAt"`
}

// TableName 指定表名
func (RSSSource) TableName() string {
	return "rss_sources"
}

// RSSItem RSS 文章条目 (正文按需抓取缓存)
type RSSItem struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID         uint       `gorm:"not null;index:idx_rss_item_user,priority:1;uniqueIndex:uk_rss_user_guid,priority:1" json:"userId"`
	SourceID       uint       `gorm:"not null;index:idx_rss_item_source,priority:1" json:"sourceId"`
	GUID           string     `gorm:"size:255;not null;uniqueIndex:uk_rss_user_guid,priority:2" json:"-"`
	Title          string     `gorm:"size:500;not null" json:"title"`
	Link           string     `gorm:"size:500;not null" json:"link"`
	Summary        string     `gorm:"type:text" json:"summary"`
	Content        string     `gorm:"type:mediumtext" json:"content,omitempty"`        // 纯文本正文: 列表查询不 SELECT 此列, 详情接口返回; 供导入/标注/听写
	ContentHTML    string     `gorm:"type:mediumtext" json:"contentHtml,omitempty"`    // 净化后的 HTML 正文: 阅读视图渲染
	DifficultWords string     `gorm:"type:mediumtext" json:"difficultWords,omitempty"` // AI 提取的生词 JSON (一键提取, 与分级阅读同源)
	EstUnknownRate float64    `gorm:"not null;default:0" json:"estUnknownRate"`        // 摘要预估生词率 (生词本交集+长词启发)
	PublishedAt    *time.Time `json:"publishedAt"`
	Status         string     `gorm:"size:20;not null;default:'unread';index:idx_rss_item_source,priority:2" json:"status"` // unread/read/imported
	ImportedDocID  uint       `gorm:"not null;default:0" json:"importedDocId"`
	CreatedAt      time.Time  `gorm:"index:idx_rss_item_user,priority:2" json:"createdAt"`
}

// TableName 指定表名
func (RSSItem) TableName() string {
	return "rss_items"
}

// AddRSSSourceRequest 添加订阅
type AddRSSSourceRequest struct {
	Title string `json:"title"`
	URL   string `json:"url" binding:"required"`
}

// UpdateRSSSourceRequest 编辑订阅 (空字段保持原值; url 变更时重新验证 feed)
type UpdateRSSSourceRequest struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// ListRSSItemsRequest 文章列表查询
type ListRSSItemsRequest struct {
	SourceID   uint   `form:"sourceId"`
	Status     string `form:"status"`     // unread / read (含 imported) / 空=全部
	Sort       string `form:"sort"`       // time_desc(默认)/time_asc, 按发布时间(缺失用创建时间)排序
	EasyOnly   bool   `form:"easyOnly"`   // 只看预估适合当前水平的 (est_unknown_rate <= 0.08)
	Page       int    `form:"page"`
	PageSize   int    `form:"page_size"`
}
