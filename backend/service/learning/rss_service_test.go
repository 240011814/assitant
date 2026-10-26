package learning

import (
	"encoding/json"
	"testing"

	"backend/model"
)

// TestRSSItemContentSerialized 正文必须能随详情接口序列化 (曾因 json:"-" 导致
// 抓取成功但接口返回的 content 恒为空); 列表防下发靠 SELECT 不取该列, 不靠 tag
func TestRSSItemContentSerialized(t *testing.T) {
	item := model.RSSItem{
		Title:   "hello",
		Content: "some article text",
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if back["content"] != "some article text" {
		t.Fatalf("content 未序列化: got %v", back["content"])
	}
}
