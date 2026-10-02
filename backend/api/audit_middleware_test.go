package api

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMaskSensitiveValue(t *testing.T) {
	in := map[string]any{
		"userName": "alice",
		"password": "secret123",
		"Profile": map[string]any{
			"oldPassword": "a",
			"apiKey":      "k",
			"nickname":    "n",
		},
		"tokens": []any{"x", map[string]any{"token": "t"}},
	}
	out := maskSensitiveValue(in).(map[string]any)
	if out["userName"] != "alice" {
		t.Fatal("非敏感字段不应被替换")
	}
	if out["password"] != "***" {
		t.Fatalf("password 应脱敏, got %v", out["password"])
	}
	profile := out["Profile"].(map[string]any)
	if profile["oldPassword"] != "***" || profile["apiKey"] != "***" {
		t.Fatal("嵌套对象内的敏感字段应脱敏")
	}
	if profile["nickname"] != "n" {
		t.Fatal("嵌套对象内的普通字段不应被替换")
	}
	tokens := out["tokens"].([]any)
	if tokens[1].(map[string]any)["token"] != "***" {
		t.Fatal("数组元素内的敏感字段应脱敏")
	}
}

func TestAuditRequestBodyString(t *testing.T) {
	// JSON: 先脱敏再截断
	body := auditRequestBodyString([]byte(`{"userName":"alice","password":"secret123"}`))
	if strings.Contains(body, "secret123") {
		t.Fatalf("JSON 请求体应脱敏, got %s", body)
	}
	if !strings.Contains(body, `"password":"***"`) {
		t.Fatalf("脱敏后应保留字段结构, got %s", body)
	}

	// 非 JSON: 原样截断
	raw := `{"broken` + strings.Repeat("x", 5000)
	out := auditRequestBodyString([]byte(raw))
	if int(len([]rune(out))) > auditRequestBodyLimit {
		t.Fatalf("应按 rune 截断到 %d, got %d", auditRequestBodyLimit, len([]rune(out)))
	}

	// 空体
	if auditRequestBodyString(nil) != "" {
		t.Fatal("空请求体应返回空串")
	}
}

func TestAuditTruncateRunesKeepsValidUTF8(t *testing.T) {
	s := strings.Repeat("中", 200) // 多字节字符, 字节截断会产生非法 utf8 导致 MySQL 写入失败
	out := auditTruncateRunes(s, 100)
	if len([]rune(out)) != 100 {
		t.Fatalf("应截断到 100 个 rune, got %d", len([]rune(out)))
	}
}

func newAuditTestContext(method, path string) *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(method, path, nil)
	return c
}

func TestShouldAuditRequest(t *testing.T) {
	cases := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/api/vocabulary", false},
		{"POST", "/api/vocabulary", true},
		{"PUT", "/api/notes/3", true},
		{"DELETE", "/api/admin/users/1", true},
		{"PATCH", "/api/stock/alerts/2", true},
		{"POST", "/api/chat", false},                                  // SSE 流式对话
		{"POST", "/api/chat/tool-approval", true},                     // 工具审批保留审计
		{"POST", "/api/ai-orchestrations/12/chat", false},             // 编排对话流
		{"PUT", "/api/ai-orchestrations/12", true},                    // 编排定义更新仍审计
		{"POST", "/api/agent-studio/debug", false},                    // 调试运行
		{"POST", "/api/agent-studio/orchestrations/validate", false},  // dry-run
		{"POST", "/api/agent-studio/orchestrations", true},            // 编排创建审计
		{"PUT", "/api/user/preferences/theme", false},                 // 主题切换噪音
		{"POST", "/api/user/preferences/notification", true},          // 通知偏好审计
	}

	for _, tc := range cases {
		c := newAuditTestContext(tc.method, tc.path)
		if got := shouldAuditRequest(c); got != tc.want {
			t.Fatalf("shouldAuditRequest(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestParseAuditTime(t *testing.T) {
	if _, ok := parseAuditTime("2026-10-02 12:30:00"); !ok {
		t.Fatal("应支持 yyyy-MM-dd HH:mm:ss")
	}
	if _, ok := parseAuditTime("2026-10-02T12:30:00Z"); !ok {
		t.Fatal("应支持 RFC3339")
	}
	if _, ok := parseAuditTime("2026-10-02"); !ok {
		t.Fatal("应支持日期")
	}
	if _, ok := parseAuditTime("not-a-date"); ok {
		t.Fatal("非法时间应返回 false")
	}
	if _, ok := parseAuditTime(""); ok {
		t.Fatal("空串应返回 false")
	}
}
