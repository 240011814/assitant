package api

import (
	"testing"

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
