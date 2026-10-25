package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"backend/service/tools"
)

// TestValidateOutboundURLPolicy 校验共享 SSRF 防护的三档行为:
// 宽松 (默认, 内网 RSSHub 场景) 放行环回/私网但始终拦链路本地/云元数据; 严格拦全部内网。
func TestValidateOutboundURLPolicy(t *testing.T) {
	cases := []struct {
		name        string
		url         string
		allowPriv   bool
		wantBlocked bool
	}{
		{"宽松-环回放行", "http://127.0.0.1:1200/some/route", true, false},
		{"宽松-私网放行", "http://192.168.1.10:1200", true, false},
		{"宽松-链路本地拦截", "http://169.254.169.254/latest/meta-data", true, true},
		{"严格-环回拦截", "http://127.0.0.1:8080/api", false, true},
		{"严格-私网拦截", "http://10.1.6.156:3306", false, true},
		{"非http协议拦截", "ftp://example.com/feed", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := tools.ValidateOutboundURL(c.url, c.allowPriv)
			blocked := err != nil
			if blocked != c.wantBlocked {
				t.Fatalf("ValidateOutboundURL(%q, allowPrivate=%v) err=%v, 期望 blocked=%v", c.url, c.allowPriv, err, c.wantBlocked)
			}
		})
	}
}

// TestFetchPageRawStrictBlocksLoopback 严格模式下环回地址在发起请求前即被拒 (宽超时也不会耗掉)
func TestFetchPageRawStrictBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	_, err := fetchPageRaw(srv.URL, false)
	if err == nil || !strings.Contains(err.Error(), "内网") {
		t.Fatalf("严格模式应拒绝环回地址, got err=%v", err)
	}
	if _, _, err := fetchURLText(srv.URL, true); err != nil {
		t.Fatalf("宽松模式应放行环回地址 (httptest), got err=%v", err)
	}
}
