package service

import (
	"os"
	"testing"
)

// TestRSSBBCRealFetch 真实网络自测: BBC feed 解析 + 条目正文抓取 (Go 直连, 与服务端同代码路径)。
// 默认跳过 (避免常规测试依赖外网); RSS_REAL_NET_TEST=1 时启用。
// 网络不通时打印原因跳过, 不算失败; 抓到了但正文为空才算 bug。
func TestRSSBBCRealFetch(t *testing.T) {
	if os.Getenv("RSS_REAL_NET_TEST") == "" {
		t.Skip("真实网络自测默认关闭, 设 RSS_REAL_NET_TEST=1 启用")
	}
	feed, err := fetchRSSFeed("https://feeds.bbci.co.uk/news/world/rss.xml")
	if err != nil {
		t.Skipf("无法访问 BBC feed: %v", err)
	}
	t.Logf("feed title=%q items=%d", feed.Title, len(feed.Items))
	if len(feed.Items) == 0 {
		t.Fatal("feed 解析成功但 0 条")
	}
	item := feed.Items[0]
	t.Logf("item[0] title=%q link=%q summary_len=%d", item.Title, item.Link, len(item.Description))
	if item.Link == "" {
		t.Fatal("条目 link 为空")
	}

	text, title, err := fetchURLText(item.Link)
	if err != nil {
		t.Skipf("无法访问 BBC 文章页: %v", err)
	}
	t.Logf("article title=%q text_len=%d preview=%.200s", title, len([]rune(text)), text)
	if len([]rune(text)) < 200 {
		t.Fatalf("抓到了响应但正文过短 (%d runes), 提取器有问题", len([]rune(text)))
	}
}

// TestRSSReachableSourceRealFetch 用国内可达的源验证 HTML→正文提取链路本身
func TestRSSReachableSourceRealFetch(t *testing.T) {
	if os.Getenv("RSS_REAL_NET_TEST") == "" {
		t.Skip("真实网络自测默认关闭, 设 RSS_REAL_NET_TEST=1 启用")
	}
	feed, err := fetchRSSFeed("https://www.newsinlevels.com/feed/")
	if err != nil {
		t.Skipf("无法访问 newsinlevels: %v", err)
	}
	if len(feed.Items) == 0 || feed.Items[0].Link == "" {
		t.Fatal("feed 无条目或无 link")
	}
	item := feed.Items[0]
	t.Logf("item[0] title=%q link=%q", item.Title, item.Link)

	text, title, err := fetchURLText(item.Link)
	if err != nil {
		t.Fatalf("可达源的文章页抓取失败 (提取器问题): %v", err)
	}
	t.Logf("article title=%q text_len=%d preview=%.200s", title, len([]rune(text)), text)
	if len([]rune(text)) < 200 {
		t.Fatalf("抓到了响应但正文过短 (%d runes), 提取器有问题", len([]rune(text)))
	}
}
