package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"backend/model"
	"backend/service/learning"
)

// setupRSSTestRouter 真 HTTP 端到端: gin 引擎 + 真路由 + 真 handler + SQLite 真库,
// 认证中间件用桩替换 (直接写 userId)。用完还原全局 DB。
func setupRSSTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	origDB := learning.DB
	db, err := gorm.Open(sqlite.Open(t.Name()+".db"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.RSSSource{}, &model.RSSItem{}, &model.ReadingDoc{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	learning.DB = db
	t.Cleanup(func() {
		learning.DB = origDB
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(t.Name() + ".db")
	})

	rssSvc := learning.NewRSSService(learning.NewReadingService(nil, nil, nil))
	readingSvc := learning.NewReadingService(nil, nil, nil)

	r := gin.New()
	r.Use(func(c *gin.Context) {
		uid := uint(2)
		c.Set("userId", uid)
		c.Next()
	})
	apiGroup := r.Group("/api")
	{
		rssGroup := apiGroup.Group("/rss")
		{
			rssGroup.POST("/sources", NewRSSHandler(rssSvc).HandleAddRSSSource)
			rssGroup.GET("/sources", NewRSSHandler(rssSvc).HandleListRSSSources)
			rssGroup.DELETE("/sources/:id", NewRSSHandler(rssSvc).HandleDeleteRSSSource)
			rssGroup.POST("/sources/:id/fetch", NewRSSHandler(rssSvc).HandleFetchRSSSource)
			rssGroup.GET("/items", NewRSSHandler(rssSvc).HandleListRSSItems)
			rssGroup.POST("/items/:id/read", NewRSSHandler(rssSvc).HandleMarkRSSItemRead)
			rssGroup.GET("/items/:id/content", NewRSSHandler(rssSvc).HandleGetRSSItemContent)
			rssGroup.POST("/items/:id/import", NewRSSHandler(rssSvc).HandleImportRSSItem)
		}
		readingGroup := apiGroup.Group("/reading")
		{
			readingGroup.GET("/docs/:id", NewReadingHandler(readingSvc).HandleGetReadingDoc)
		}
	}
	return r
}

func doJSON(t *testing.T, r *gin.Engine, method, path string, body any) (int, map[string]any) {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// TestRSSHTTPFlowE2E 端到端 (需外网, RSS_REAL_NET_TEST=1 启用):
// 添加订阅 → 抓条目 → 列表 → 正文 (断言净化 HTML 进了响应) → 导入 → 删除
func TestRSSHTTPFlowE2E(t *testing.T) {
	if os.Getenv("RSS_REAL_NET_TEST") == "" {
		t.Skip("真实网络自测默认关闭, 设 RSS_REAL_NET_TEST=1 启用")
	}
	r := setupRSSTestRouter(t)

	// 1. 添加订阅 (真实抓取 newsinlevels feed)
	code, resp := doJSON(t, r, http.MethodPost, "/api/rss/sources", map[string]string{
		"url": "https://www.newsinlevels.com/feed/",
	})
	if code != http.StatusOK || resp["code"] != "0000" {
		t.Fatalf("添加订阅失败: code=%d resp=%v", code, resp)
	}
	srcData, _ := resp["data"].(map[string]any)
	if srcData == nil || srcData["id"] == nil {
		t.Fatalf("响应缺少订阅源: %v", resp)
	}

	// 2. 订阅源列表
	code, resp = doJSON(t, r, http.MethodGet, "/api/rss/sources", nil)
	list, _ := resp["data"].([]any)
	if code != http.StatusOK || len(list) != 1 {
		t.Fatalf("订阅列表异常: code=%d len=%d", code, len(list))
	}

	// 3. 文章列表 (不应包含正文列)
	code, resp = doJSON(t, r, http.MethodGet, "/api/rss/items?sourceId=1", nil)
	itemsData, _ := resp["data"].(map[string]any)
	items, _ := itemsData["list"].([]any)
	if code != http.StatusOK || len(items) == 0 {
		t.Fatalf("文章列表为空: code=%d count=%v", code, itemsData)
	}
	firstItem, _ := items[0].(map[string]any)
	if v, ok := firstItem["contentHtml"]; ok && v != nil && v != "" {
		t.Fatal("列表不应下发 contentHtml")
	}
	itemID := int(firstItem["id"].(float64))

	// 4. 正文详情: contentHtml 必须出现在响应里且已净化
	code, resp = doJSON(t, r, http.MethodGet, "/api/rss/items/1/content", nil)
	_ = itemID
	if code != http.StatusOK || resp["code"] != "0000" {
		t.Fatalf("获取正文失败: code=%d resp=%v", code, resp)
	}
	content, _ := resp["data"].(map[string]any)
	html, _ := content["contentHtml"].(string)
	if strings.TrimSpace(html) == "" {
		t.Fatalf("contentHtml 为空 (json 序列化或净化链路断了): %v", resp["data"])
	}
	if strings.Contains(strings.ToLower(html), "<script") {
		t.Fatal("contentHtml 含脚本, 净化失效")
	}
	if !strings.Contains(html, "<p") {
		t.Fatalf("contentHtml 不含段落标签, 提取异常: %.200s", html)
	}

	// 5. 导入分级阅读 (真库落 reading_docs)
	code, resp = doJSON(t, r, http.MethodPost, "/api/rss/items/1/import", nil)
	if code != http.StatusOK || resp["code"] != "0000" {
		t.Fatalf("导入失败: code=%d resp=%v", code, resp)
	}

	// 6. 删除订阅 (级联条目)
	code, resp = doJSON(t, r, http.MethodDelete, "/api/rss/sources/1", nil)
	if code != http.StatusOK || resp["code"] != "0000" {
		t.Fatalf("删除订阅失败: code=%d resp=%v", code, resp)
	}
	_, resp = doJSON(t, r, http.MethodGet, "/api/rss/sources", nil)
	list, _ = resp["data"].([]any)
	if len(list) != 0 {
		t.Fatalf("删除后订阅源仍存在: %v", list)
	}
}
