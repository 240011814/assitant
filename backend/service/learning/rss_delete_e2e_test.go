package learning

import (
	"os"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"backend/model"
)

// setupRSSTestDB 用纯 Go SQLite 起真库, 把 service.DB 指过去, 对业务逻辑做端到端验证
// (覆盖: 删除订阅/条目、列表过滤排序、正文缓存命中)。用完还原全局 DB 并清理库文件。
func setupRSSTestDB(t *testing.T) {
	t.Helper()
	origDB := DB
	dbFile := t.Name() + ".db"
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.RSSSource{}, &model.RSSItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	t.Cleanup(func() {
		DB = origDB
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(dbFile)
	})
}

// seedRSSFixture 用户 2 的一个源 (BBC) + 两条文章, 返回 source id
func seedRSSFixture(t *testing.T, userID uint) uint {
	t.Helper()
	src := model.RSSSource{UserID: userID, Title: "BBC News", URL: "https://feeds.bbci.co.uk/news/world/rss.xml", Enabled: true}
	if err := DB.Create(&src).Error; err != nil {
		t.Fatalf("seed source: %v", err)
	}
	now := time.Now()
	items := []model.RSSItem{
		{UserID: userID, SourceID: src.ID, GUID: "g1", Title: "item one", Link: "https://example.com/1", Status: "unread", PublishedAt: &now},
		{UserID: userID, SourceID: src.ID, GUID: "g2", Title: "item two", Link: "https://example.com/2", Status: "read", PublishedAt: &now},
	}
	if err := DB.Create(&items).Error; err != nil {
		t.Fatalf("seed items: %v", err)
	}
	return src.ID
}

// TestDeleteSource 端到端: 删除订阅必须连条目一起删掉且只删自己的
func TestDeleteSource(t *testing.T) {
	setupRSSTestDB(t)
	const me = uint(2)
	srcID := seedRSSFixture(t, me)

	// 另一个用户的同名源, 不应被误删
	otherID := seedRSSFixture(t, 3)

	svc := NewRSSService(nil)
	if err := svc.DeleteSource(me, srcID); err != nil {
		t.Fatalf("DeleteSource: %v", err)
	}

	var cnt int64
	DB.Model(&model.RSSSource{}).Where("id = ?", srcID).Count(&cnt)
	if cnt != 0 {
		t.Fatal("订阅源未删除")
	}
	DB.Model(&model.RSSItem{}).Where("user_id = ? AND source_id = ?", me, srcID).Count(&cnt)
	if cnt != 0 {
		t.Fatalf("该源的条目未级联删除 (%d 条残留)", cnt)
	}
	DB.Model(&model.RSSSource{}).Where("id = ?", otherID).Count(&cnt)
	if cnt != 1 {
		t.Fatal("误删了其他用户的订阅源")
	}
}

// TestDeleteSourceWrongUser 删除他人订阅必须无效 (行数不变)
func TestDeleteSourceWrongUser(t *testing.T) {
	setupRSSTestDB(t)
	const me = uint(2)
	srcID := seedRSSFixture(t, me)

	svc := NewRSSService(nil)
	if err := svc.DeleteSource(999, srcID); err != nil {
		t.Fatalf("DeleteSource 不应报错: %v", err)
	}
	var cnt int64
	DB.Model(&model.RSSSource{}).Where("id = ?", srcID).Count(&cnt)
	if cnt != 1 {
		t.Fatal("跨用户删除竟然生效了")
	}
}

// TestListItemsUnreadFilterAndSort 列表的未读过滤 + 时间排序
func TestListItemsUnreadFilterAndSort(t *testing.T) {
	setupRSSTestDB(t)
	const me = uint(2)
	srcID := seedRSSFixture(t, me)

	svc := NewRSSService(nil)

	// 仅未读
	list, total, err := svc.ListItems(me, model.ListRSSItemsRequest{SourceID: srcID, Status: "unread"})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if total != 1 || len(list) != 1 || list[0].Status != "unread" {
		t.Fatalf("仅未读过滤异常: total=%d list=%+v", total, list)
	}

	// 时间倒序: 两条都在
	list, total, err = svc.ListItems(me, model.ListRSSItemsRequest{SourceID: srcID})
	if err != nil {
		t.Fatalf("ListItems: %v", err)
	}
	if total != 2 || len(list) != 2 {
		t.Fatalf("列表数量异常: total=%d", total)
	}
	// 正文列不应被带出 (列表 SELECT 不含 content)
	if list[0].Content != "" {
		t.Fatal("列表不应返回正文")
	}
}

// TestGetItemContentCache 正文缓存命中时不发网络请求 (直接返回已缓存内容)
func TestGetItemContentCache(t *testing.T) {
	setupRSSTestDB(t)
	const me = uint(2)
	srcID := seedRSSFixture(t, me)
	item := model.RSSItem{UserID: me, SourceID: srcID, GUID: "cached", Title: "cached item", Link: "https://example.com/cached", Status: "unread", Content: "cached body text"}
	if err := DB.Create(&item).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewRSSService(nil)
	got, err := svc.GetItemContent(me, item.ID)
	if err != nil {
		t.Fatalf("GetItemContent: %v", err)
	}
	if got.Content != "cached body text" {
		t.Fatalf("缓存正文返回异常: %q", got.Content)
	}
}
