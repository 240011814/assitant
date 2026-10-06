package api

import (
	"sync"

	"github.com/gin-gonic/gin"
)

// setupSSE 统一设置 SSE 流式响应头 (Nginx 反代需要 X-Accel-Buffering: no 才不缓冲)
func setupSSE(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
}

// sseEmitter SSE 写出串行化: 节点事件来自回调 goroutine, delta 来自主循环, 并发直写会交错损坏帧
type sseEmitter struct {
	mu *sync.Mutex
	c  *gin.Context
}

func newSSEEmitter(c *gin.Context) *sseEmitter {
	return &sseEmitter{mu: &sync.Mutex{}, c: c}
}

func (e *sseEmitter) emit(event string, payload any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.c.SSEvent(event, payload)
}
