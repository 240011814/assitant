// Package kernel 跨业务共享的底层工具: 无 DB、无业务依赖, 任何业务子包都可安全引用。
// 设计约束: 本包不得 import backend/service 下的任何业务包, 以免形成循环。
package kernel

import (
	"crypto/tls"
	"log"
	"math"
	"net"
	"net/http"
	"runtime/debug"
	"time"
)

// SafeGo 后台协程统一封装: recover panic 防止拖垮整个进程 (HTTP 层有 gin Recovery 兜底,
// 手工启动的后台协程没有), 并带上调用点名与堆栈留痕定位。
func SafeGo(name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[SafeGo] %s panic: %v\n%s", name, r, debug.Stack())
			}
		}()
		fn()
	}()
}

// NormalizePage 规范化页码: <1 回 1
func NormalizePage(page int) int {
	if page < 1 {
		return 1
	}
	return page
}

// TruncateRunes 按 rune 截断字符串, 避免字节截断切断多字节字符 (utf8mb4 写入失败)
func TruncateRunes(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

// Round2 保留两位小数
func Round2(f float64) float64 {
	return math.Round(f*100) / 100
}

// TimeoutConfig 超时配置 (由 system_config 的超时键加载)
type TimeoutConfig struct {
	AIRequestTimeout        int // AI 请求超时时间（分钟）
	AITLSHandshakeTimeout   int // AI TLS 握手超时时间（秒）
	AIResponseHeaderTimeout int // AI 响应头超时时间（秒）
	HTTPTimeout             int // HTTP 请求超时时间（秒），用于 AI 连接、Mem0、Telegram
}

// NewHTTPClient 构造带统一超时与 TLS 策略的 HTTP 客户端
func NewHTTPClient(timeout time.Duration, config ...TimeoutConfig) *http.Client {
	dialTimeout := 30 * time.Second
	tlsHandshakeTimeout := 15 * time.Second
	responseHeaderTimeout := 30 * time.Second

	if len(config) > 0 {
		cfg := config[0]
		if cfg.HTTPTimeout > 0 {
			dialTimeout = time.Duration(cfg.HTTPTimeout) * time.Second
		}
		if cfg.AITLSHandshakeTimeout > 0 {
			tlsHandshakeTimeout = time.Duration(cfg.AITLSHandshakeTimeout) * time.Second
		}
		if cfg.AIResponseHeaderTimeout > 0 {
			responseHeaderTimeout = time.Duration(cfg.AIResponseHeaderTimeout) * time.Second
		}
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout:   dialTimeout,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   tlsHandshakeTimeout,
			ResponseHeaderTimeout: responseHeaderTimeout,
			IdleConnTimeout:       90 * time.Second,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
		},
	}
}
