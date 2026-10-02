package service

import (
	"strings"
	"testing"
	"time"
)

func TestValidatePasswordStrength(t *testing.T) {
	cases := []struct {
		name    string
		pwd     string
		wantErr bool
	}{
		{"正常: 字母+数字", "abc12345", false},
		{"正常: 混合大小写与符号", "Abc123!@#", false},
		{"正常: 含中文不影响判定", "abc12345中文", false},
		{"过短", "ab1", true},
		{"空密码", "", true},
		{"纯数字", "12345678", true},
		{"纯字母", "abcdefgh", true},
		{"超72位", strings.Repeat("a1", 40), true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePasswordStrength(tc.pwd)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidatePasswordStrength(%q) error = %v, wantErr = %v", tc.pwd, err, tc.wantErr)
			}
		})
	}
}

func TestLoginThrottleLockout(t *testing.T) {
	th := NewLoginThrottle()
	now := time.Unix(1700000000, 0)
	th.now = func() time.Time { return now }
	key := "user:alice"

	// 未达阈值前一直允许
	for i := 0; i < th.maxFails-1; i++ {
		th.RecordFailure(key)
		if !th.Allowed(key) {
			t.Fatalf("第 %d 次失败不应触发锁定", i+1)
		}
	}

	// 达到阈值即锁定
	th.RecordFailure(key)
	if th.Allowed(key) {
		t.Fatal("达到阈值应锁定")
	}

	// 锁定期间失败不再延长锁定
	th.RecordFailure(key)
	if th.Allowed(key) {
		t.Fatal("锁定期内应仍然拒绝")
	}

	// 锁定期满恢复
	now = now.Add(th.lockout + time.Second)
	if !th.Allowed(key) {
		t.Fatal("锁定期满应恢复允许")
	}
}

func TestLoginThrottleWindowReset(t *testing.T) {
	th := NewLoginThrottle()
	now := time.Unix(1700000000, 0)
	th.now = func() time.Time { return now }
	key := "user:bob"

	// 窗口内 3 次失败
	for i := 0; i < 3; i++ {
		th.RecordFailure(key)
	}
	// 超过窗口后计数应重置, 再失败 3 次不触发锁定
	now = now.Add(th.window + time.Second)
	for i := 0; i < th.maxFails-2; i++ {
		th.RecordFailure(key)
	}
	if !th.Allowed(key) {
		t.Fatal("窗口过期后计数应重置, 不应锁定")
	}
}

func TestLoginThrottleSuccessResets(t *testing.T) {
	th := NewLoginThrottle()
	now := time.Unix(1700000000, 0)
	th.now = func() time.Time { return now }
	key := "user:carol"

	for i := 0; i < th.maxFails-1; i++ {
		th.RecordFailure(key)
	}
	th.RecordSuccess(key)

	// 成功清零后, 需要重新计满阈值才会锁定
	for i := 0; i < th.maxFails-1; i++ {
		th.RecordFailure(key)
		if !th.Allowed(key) {
			t.Fatal("成功清零后未达阈值不应锁定")
		}
	}
	th.RecordFailure(key)
	if th.Allowed(key) {
		t.Fatal("重新计满阈值应锁定")
	}
}

func TestLoginThrottleKeyNormalization(t *testing.T) {
	if LoginThrottleKey("user", " Alice ") != "user:alice" {
		t.Fatalf("key 应小写并去空白, got %q", LoginThrottleKey("user", " Alice "))
	}
}
