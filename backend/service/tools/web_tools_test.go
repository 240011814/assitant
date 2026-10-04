package tools

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	interfaces "backend/interface"
)

// ============ fetch_url: SSRF 防护 ============

func TestValidateFetchURLBlocksPrivateTargets(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/x",
		"http://[::1]/x",
		"http://192.168.1.1/x",
		"http://10.0.0.1/x",
		"http://172.16.0.1/x",
		"http://169.254.169.254/latest/meta-data", // 云元数据
		"http://0.0.0.0/x",
		"ftp://example.com/x",
		"file:///etc/passwd",
		"localhost", // 解析到环回
	}
	for _, raw := range blocked {
		if err := validateFetchURL(raw, false); err == nil {
			t.Fatalf("应拒绝 %q", raw)
		}
	}
	// 公网 IP 字面量 (跳过 DNS, 离线可测)
	if err := validateFetchURL("https://93.184.216.34/page", false); err != nil {
		t.Fatalf("公网 IP 不应被拒: %v", err)
	}
}

// ============ fetch_url: 抓取/解析/分页 (allowLoopback 放行 httptest) ============

func invokeFetch(t *testing.T, tool *fetchURLTool, args string) (string, error) {
	t.Helper()
	return tool.InvokableRun(context.Background(), args)
}

func TestFetchURLHTMLPagination(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var body strings.Builder
		body.WriteString("<html><head><title>测试页</title></head><body>")
		for i := 0; i < 50; i++ {
			fmt.Fprintf(&body, "<p>第%d段内容一些文字用于撑起长度测试文本块。</p>", i)
		}
		body.WriteString("</body></html>")
		_, _ = w.Write([]byte(body.String()))
	}))
	defer srv.Close()

	tool := &fetchURLTool{allowLoopback: true}
	out, err := invokeFetch(t, tool, fmt.Sprintf(`{"url": %q, "length": 500}`, srv.URL))
	if err != nil {
		t.Fatalf("抓取失败: %v", err)
	}
	if !strings.Contains(out, "未读完") || !strings.Contains(out, "继续读取请传 offset=") {
		t.Fatalf("长页面应提示翻页: %s", out[:120])
	}

	// 越界偏移: 返回到结尾提示
	out, err = invokeFetch(t, tool, fmt.Sprintf(`{"url": %q, "offset": 9999999}`, srv.URL))
	if err != nil {
		t.Fatalf("越界读取失败: %v", err)
	}
	if !strings.Contains(out, "已到结尾") {
		t.Fatalf("越界偏移应提示已到结尾: %s", out[:120])
	}
}

func TestFetchURLPlainTextAndErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plain":
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte("纯文本内容第一行\n第二行"))
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		case "/binary":
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte{0x00, 0x01})
		default:
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte("<html><body>跳转页</body></html>"))
		}
	}))
	defer srv.Close()

	tool := &fetchURLTool{allowLoopback: true}

	out, err := invokeFetch(t, tool, fmt.Sprintf(`{"url": %q}`, srv.URL+"/plain"))
	if err != nil {
		t.Fatalf("纯文本抓取失败: %v", err)
	}
	if !strings.Contains(out, "第二行") {
		t.Fatalf("纯文本应原样返回: %s", out)
	}

	if _, err := invokeFetch(t, tool, fmt.Sprintf(`{"url": %q}`, srv.URL+"/missing")); err == nil {
		t.Fatal("404 应报错")
	}
	if _, err := invokeFetch(t, tool, fmt.Sprintf(`{"url": %q}`, srv.URL+"/binary")); err == nil {
		t.Fatal("二进制类型应报不支持")
	}
}

func TestFetchURLFollowsRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/target", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("重定向后内容"))
	}))
	defer srv.Close()

	tool := &fetchURLTool{allowLoopback: true}
	out, err := invokeFetch(t, tool, fmt.Sprintf(`{"url": %q}`, srv.URL+"/start"))
	if err != nil {
		t.Fatalf("重定向抓取失败: %v", err)
	}
	if !strings.Contains(out, "重定向后内容") {
		t.Fatalf("应返回重定向目标内容: %s", out)
	}
}

// ============ send_email ============

type fakeMail struct {
	to, subject, body string
}

type fakeNotifier struct {
	sent   []fakeMail
	failOn map[string]error
}

func (f *fakeNotifier) Send(to string, msg interfaces.NotifyMessage) error {
	if err, ok := f.failOn[to]; ok {
		return err
	}
	f.sent = append(f.sent, fakeMail{to: to, subject: msg.Subject, body: msg.Body})
	return nil
}
func (f *fakeNotifier) Name() string { return "fake" }

func withFakeNotifier(t *testing.T, n interfaces.Notifier) {
	t.Helper()
	old := emailNotifier
	emailNotifier = n
	t.Cleanup(func() { emailNotifier = old })
}

func invokeEmail(t *testing.T, args string) (string, error) {
	t.Helper()
	tool := &sendEmailTool{}
	return tool.InvokableRun(context.Background(), args)
}

func TestSendEmailValidation(t *testing.T) {
	withFakeNotifier(t, &fakeNotifier{})
	cases := []struct {
		name string
		args string
	}{
		{"缺收件人", `{"subject":"s","content":"c"}`},
		{"邮箱格式错", `{"to":"abc","subject":"s","content":"c"}`},
		{"缺主题", `{"to":"a@b.com","content":"c"}`},
		{"缺正文", `{"to":"a@b.com","subject":"s"}`},
		{"收件人超限", fmt.Sprintf(`{"to":"%s","subject":"s","content":"c"}`, func() string {
			addrs := make([]string, 12)
			for i := range addrs {
				addrs[i] = fmt.Sprintf("u%d@b.com", i)
			}
			return strings.Join(addrs, ",")
		}())},
	}
	for _, tc := range cases {
		if _, err := invokeEmail(t, tc.args); err == nil {
			t.Fatalf("%s 应报错", tc.name)
		}
	}
	// 未注入通知器 (显式置空, 清理后恢复)
	withFakeNotifier(t, nil)
	if _, err := invokeEmail(t, `{"to":"a@b.com","subject":"s","content":"c"}`); err == nil {
		t.Fatal("未注入 Notifier 应报未启用")
	}
}

func TestSendEmailSuccessAndPartialFailure(t *testing.T) {
	n := &fakeNotifier{failOn: map[string]error{"bad@x.com": fmt.Errorf("SMTP 拒绝")}}
	withFakeNotifier(t, n)

	out, err := invokeEmail(t, `{"to":"a@b.com, bad@x.com ,a@b.com","subject":" 报告 ","content":"正文内容"}`)
	if err != nil {
		t.Fatalf("发送失败: %v", err)
	}
	if !strings.Contains(out, "a@b.com") || !strings.Contains(out, "bad@x.com") || !strings.Contains(out, "SMTP 拒绝") {
		t.Fatalf("输出应包含成功与失败详情: %s", out)
	}
	// 去重后只发 1 封给 a@b.com, bad@x.com 不入成功列表
	if len(n.sent) != 1 || n.sent[0].to != "a@b.com" {
		t.Fatalf("成功投递应为 1 封 a@b.com, got %+v", n.sent)
	}
	if n.sent[0].subject != "报告" || n.sent[0].body != "正文内容" {
		t.Fatalf("主题/正文透传不符: %+v", n.sent[0])
	}
}
