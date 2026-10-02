package service

import (
	"strings"
	"sync"
	"time"
)

// LoginThrottle 登录失败限流 (内存版, 适配单实例部署):
// 同一 key (用户名/IP/用户ID) 在 window 窗口内失败 maxFails 次后锁定 lockout 时长。
// 重启后计数清零属可接受降级, 失败明细在审计日志里有完整记录。
type LoginThrottle struct {
	mu       sync.Mutex
	entries  map[string]*loginAttempt
	maxFails int
	window   time.Duration
	lockout  time.Duration
	now      func() time.Time // 可注入时钟, 便于测试
}

type loginAttempt struct {
	failCount   int
	windowStart time.Time
	lockedUntil time.Time
}

func NewLoginThrottle() *LoginThrottle {
	return &LoginThrottle{
		entries:  make(map[string]*loginAttempt),
		maxFails: 5,
		window:   15 * time.Minute,
		lockout:  15 * time.Minute,
		now:      time.Now,
	}
}

// LoginThrottleKey 规范化限流 key (用户名不区分大小写)
func LoginThrottleKey(prefix, value string) string {
	return prefix + ":" + strings.ToLower(strings.TrimSpace(value))
}

// Allowed 该 key 当前是否允许尝试登录 (锁定期间不允许, 且不重复计数)
func (t *LoginThrottle) Allowed(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	a, ok := t.entries[key]
	if !ok {
		return true
	}
	return !t.now().Before(a.lockedUntil)
}

// RecordFailure 记录一次失败; 窗口外的失败重新计数, 达到阈值即锁定
func (t *LoginThrottle) RecordFailure(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	a, ok := t.entries[key]
	if !ok || now.Sub(a.windowStart) > t.window {
		a = &loginAttempt{windowStart: now}
		t.entries[key] = a
	}
	a.failCount++
	if a.failCount >= t.maxFails {
		a.lockedUntil = now.Add(t.lockout)
		// 锁定结束后重新计数, 避免旧计数导致永久锁定
		a.failCount = 0
		a.windowStart = now
	}
}

// RecordSuccess 登录成功清除该 key 的计数
func (t *LoginThrottle) RecordSuccess(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}
