package service

import (
	"strings"
	"testing"

	"backend/model"

	"github.com/cloudwego/eino/schema"
)

// TestOrchToDTO 编排行 -> API 视图 (Title/Name 字段互换是历史约定)
func TestOrchToDTO(t *testing.T) {
	a := &model.AIAgent{
		ID:         7,
		Title:      "投票编排",
		Definition: `{"nodes":[]}`,
		Version:    3,
		Enabled:    true,
	}
	dto := orchToDTO(a)
	if dto.ID != 7 || dto.Name != "投票编排" || dto.Version != 3 || !dto.Enabled {
		t.Fatalf("orchToDTO 字段映射错误: %+v", dto)
	}
	if dto.Definition != a.Definition {
		t.Fatal("definition 应原样透传")
	}
}

// TestOrchIssueMessagesAndDTOs 校验结果的两种投影
func TestOrchIssueMessagesAndDTOs(t *testing.T) {
	issues := []orchValidateIssue{
		{NodeID: "a", Message: "缺少入边"},
		{NodeID: "b", Message: "回边未声明"},
	}
	msgs := orchIssueMessages(issues)
	if len(msgs) != 2 || msgs[0] != "缺少入边" {
		t.Fatalf("orchIssueMessages 错误: %v", msgs)
	}
	dtos := orchIssueDTOs(issues)
	if len(dtos) != 2 || dtos[1].NodeID != "b" || dtos[1].Message != "回边未声明" {
		t.Fatalf("orchIssueDTOs 错误: %+v", dtos)
	}
	if orchIssueMessages(nil) == nil || len(orchIssueMessages(nil)) != 0 {
		t.Fatal("空入参应返回空切片而非 nil (前端遍历安全)")
	}
}

// TestOrchHasUpstreamEdge 上游边判定 (合并节点/入口判断依赖)
func TestOrchHasUpstreamEdge(t *testing.T) {
	dsl := &OrchestrationDSL{
		Edges: []OrchestrationEdge{{Source: "a", Target: "b"}},
	}
	if !orchHasUpstreamEdge(dsl, "b") {
		t.Fatal("b 有入边应返回 true")
	}
	if orchHasUpstreamEdge(dsl, "a") {
		t.Fatal("a 无入边应返回 false")
	}
}

// TestOrchNodeKeysOf 事件归属表: 空 id 跳过, 无名节点回退 id
func TestOrchNodeKeysOf(t *testing.T) {
	dsl := &OrchestrationDSL{
		Nodes: []OrchestrationNode{
			{ID: "agent1", Name: "主Agent"},
			{ID: ""},
			{ID: "tool1"},
		},
	}
	keys := orchNodeKeysOf(dsl)
	if len(keys) != 2 {
		t.Fatalf("应跳过空 id, got %v", keys)
	}
	if keys["agent1"] != "主Agent" || keys["tool1"] != "tool1" {
		t.Fatalf("key 映射错误: %v", keys)
	}
}

// TestOrchTokensText 用量格式化 (nil 用量显示占位符)
func TestOrchTokensText(t *testing.T) {
	if got := orchTokensText(nil); got != "-" {
		t.Fatalf("nil 用量应显示 -, got %q", got)
	}
	got := orchTokensText(&schema.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15})
	for _, want := range []string{"prompt=10", "completion=5", "total=15"} {
		if !strings.Contains(got, want) {
			t.Fatalf("用量文本缺 %s: %q", want, got)
		}
	}
}

// TestOrchToolNames 工具轨迹格式化
func TestOrchToolNames(t *testing.T) {
	if orchToolNames(nil) != nil {
		t.Fatal("空轨迹应返回 nil")
	}
	names := orchToolNames([]OrchToolTrace{{Name: "web_search", MS: 120}})
	if len(names) != 1 || names[0] != "web_search(120ms)" {
		t.Fatalf("工具名格式错误: %v", names)
	}
}

// TestOrchErrSuffix 错误后缀
func TestOrchErrSuffix(t *testing.T) {
	if orchErrSuffix("") != "" {
		t.Fatal("无错误应返回空串")
	}
	if got := orchErrSuffix("超时"); got != " 错误=超时" {
		t.Fatalf("错误后缀格式错误: %q", got)
	}
}
