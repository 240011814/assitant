package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// TestRolesFromClaims JWT 角色声明解析: 新版 roles 数组优先, 旧令牌回落单一 role
func TestRolesFromClaims(t *testing.T) {
	cases := []struct {
		name   string
		claims jwt.MapClaims
		want   []string
	}{
		{
			name:   "新版 roles 数组",
			claims: jwt.MapClaims{"roles": []any{"R_ADMIN", "R_USER"}, "role": "R_ADMIN"},
			want:   []string{"R_ADMIN", "R_USER"},
		},
		{
			name:   "旧令牌仅单一 role",
			claims: jwt.MapClaims{"role": "R_USER"},
			want:   []string{"R_USER"},
		},
		{
			name:   "roles 数组为空时回落 role",
			claims: jwt.MapClaims{"roles": []any{}, "role": "R_ADMIN"},
			want:   []string{"R_ADMIN"},
		},
		{
			name:   "roles 含非字符串项时跳过",
			claims: jwt.MapClaims{"roles": []any{"R_ADMIN", 123, nil, "R_USER"}},
			want:   []string{"R_ADMIN", "R_USER"},
		},
		{
			name:   "两者都无 → 空",
			claims: jwt.MapClaims{},
			want:   nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := rolesFromClaims(tc.claims)
			if len(got) != len(tc.want) {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("want %v, got %v", tc.want, got)
				}
			}
		})
	}
}

// TestHasAnyRole 多角色任一匹配
func TestHasAnyRole(t *testing.T) {
	if !hasAnyRole([]string{"R_USER", "R_ADMIN"}, "R_ADMIN") {
		t.Fatal("R_ADMIN 应命中")
	}
	if hasAnyRole([]string{"R_USER"}, "R_ADMIN", "R_SUPER") {
		t.Fatal("无交集不应命中")
	}
	if hasAnyRole(nil, "R_USER") {
		t.Fatal("空角色列表不应命中")
	}
}

// makeTestToken 签发测试访问令牌
func makeTestToken(t *testing.T, secret string, claims jwt.MapClaims) string {
	t.Helper()
	if claims["exp"] == nil {
		claims["exp"] = time.Now().Add(time.Hour).Unix()
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("签发测试令牌失败: %v", err)
	}
	return signed
}

// TestParseAccessClaims 访问令牌解析: 错误码语义承载前端刷新流程 (9999 换发, 401 仅提示)
func TestParseAccessClaims(t *testing.T) {
	const secret = "test-secret"
	valid := makeTestToken(t, secret, jwt.MapClaims{
		"userId": float64(7), "userName": "alice", "roles": []any{"R_USER"}, "typ": "access",
	})
	refresh := makeTestToken(t, secret, jwt.MapClaims{
		"userId": float64(7), "userName": "alice", "typ": "refresh",
	})

	cases := []struct {
		name       string
		authHeader string
		wantOK     bool
		wantCode   string // !wantOK 时校验错误码
	}{
		{name: "有效访问令牌", authHeader: "Bearer " + valid, wantOK: true},
		{name: "无 Authorization 头按 401 未登录", authHeader: "", wantOK: false, wantCode: "401"},
		{name: "非 Bearer 前缀按 401", authHeader: "Basic abc", wantOK: false, wantCode: "401"},
		{name: "签名错误按 9999 (触发前端换发)", authHeader: "Bearer " + makeTestToken(t, "other-secret", jwt.MapClaims{"userId": float64(7)}), wantOK: false, wantCode: "9999"},
		{name: "垃圾串按 9999", authHeader: "Bearer not-a-jwt", wantOK: false, wantCode: "9999"},
		{name: "刷新令牌按 401 令牌类型错误", authHeader: "Bearer " + refresh, wantOK: false, wantCode: "401"},
		{name: "缺 userId 按 401", authHeader: "Bearer " + makeTestToken(t, secret, jwt.MapClaims{"userName": "alice"}), wantOK: false, wantCode: "401"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			claims, authErr := parseAccessClaims(secret, tc.authHeader)
			if tc.wantOK {
				if authErr != nil {
					t.Fatalf("应解析成功, 实际失败: %+v", authErr)
				}
				if claims["userName"] != "alice" {
					t.Fatalf("claims.userName 应为 alice, 实际 %v", claims["userName"])
				}
				return
			}
			if authErr == nil {
				t.Fatal("应解析失败")
			}
			if authErr.code != tc.wantCode {
				t.Fatalf("错误码应为 %s, 实际 %s", tc.wantCode, authErr.code)
			}
		})
	}
}

// TestOptionalAuthMiddleware 可选认证: 无头/无效令牌静默放行 (不注入身份), 有效令牌注入 userId/roles/accountName
func TestOptionalAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const secret = "test-secret"

	runMiddleware := func(authHeader string) (*gin.Context, *httptest.ResponseRecorder) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/draw/1", nil)
		if authHeader != "" {
			c.Request.Header.Set("Authorization", authHeader)
		}
		OptionalAuthMiddleware(secret)(c)
		return c, w
	}

	assertNoIdentity := func(t *testing.T, c *gin.Context, scene string) {
		t.Helper()
		if _, exists := c.Get("userId"); exists {
			t.Fatalf("%s: 不应注入 userId", scene)
		}
		if c.IsAborted() {
			t.Fatalf("%s: 可选认证不应中断请求", scene)
		}
	}

	t.Run("无头静默放行", func(t *testing.T) {
		c, _ := runMiddleware("")
		assertNoIdentity(t, c, "无头")
	})
	t.Run("无效令牌静默放行", func(t *testing.T) {
		c, _ := runMiddleware("Bearer not-a-jwt")
		assertNoIdentity(t, c, "无效令牌")
	})
	t.Run("刷新令牌静默放行", func(t *testing.T) {
		refresh := makeTestToken(t, secret, jwt.MapClaims{"userId": float64(7), "typ": "refresh"})
		c, _ := runMiddleware("Bearer " + refresh)
		assertNoIdentity(t, c, "刷新令牌")
	})
	t.Run("有效令牌注入身份", func(t *testing.T) {
		valid := makeTestToken(t, secret, jwt.MapClaims{
			"userId": float64(7), "userName": "alice", "roles": []any{"R_USER"}, "typ": "access",
		})
		c, _ := runMiddleware("Bearer " + valid)
		if uid := GetUserID(c); uid != 7 {
			t.Fatalf("userId 应为 7, 实际 %d", uid)
		}
		if name := currentUserName(c); name != "alice" {
			t.Fatalf("accountName 应为 alice, 实际 %q", name)
		}
		if roles := c.GetStringSlice("roles"); len(roles) != 1 || roles[0] != "R_USER" {
			t.Fatalf("roles 应为 [R_USER], 实际 %v", roles)
		}
	})
}
