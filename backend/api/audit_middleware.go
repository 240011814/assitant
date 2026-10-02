package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

// 请求体最多记录 4000 字符 (超出截断), 响应体最多取 2KB 用于解析业务码
const (
	auditRequestBodyLimit  = 4000
	auditResponseBodyLimit = 2 << 10
	auditQueueSize         = 1024
)

var auditQueue chan *model.OperationAuditLog
var auditDone chan struct{}

// InitAuditLogger 启动审计日志异步写入协程 (main 中 DB 初始化后调用一次)
func InitAuditLogger() {
	auditQueue = make(chan *model.OperationAuditLog, auditQueueSize)
	auditDone = make(chan struct{})
	go func() {
		defer close(auditDone)
		for entry := range auditQueue {
			// 用户名快照: 记录操作时刻的用户名, 用户改名/删除不影响历史记录
			if entry.UserID != nil && entry.UserName == "" {
				var user model.User
				if err := service.DB.Select("username").First(&user, *entry.UserID).Error; err == nil {
					entry.UserName = user.Username
				}
			}
			if err := service.DB.Create(entry).Error; err != nil {
				log.Printf("审计日志写入失败: %v (method=%s path=%s)", err, entry.Method, entry.Path)
			}
		}
	}()
}

// ShutdownAuditLogger 关闭队列并等待剩余日志全部落库 (优雅停机时在 HTTP 服务停止后调用)
func ShutdownAuditLogger() {
	if auditQueue == nil {
		return
	}
	close(auditQueue)
	if auditDone != nil {
		<-auditDone
	}
	auditQueue = nil
}

// 高频/流式/无状态变更的路径不记录 (前缀匹配)
var auditSkipPathPrefixes = []string{
	"/api/user/preferences/theme", // 主题切换是纯 UI 状态, 高频噪音
	"/api/agent-studio/debug",     // 编排调试运行, 高频试错
}

func shouldAuditRequest(c *gin.Context) bool {
	switch c.Request.Method {
	case http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch:
	default:
		return false
	}

	path := c.Request.URL.Path
	// SSE 流式 AI 对话 (请求体是完整消息历史, 噪音大); 工具审批 /api/chat/tool-approval 保留审计
	if path == "/api/chat" {
		return false
	}
	// 编排对话同为流式接口, 路径带会话 id, 按前后缀匹配
	if strings.HasPrefix(path, "/api/ai-orchestrations/") && strings.HasSuffix(path, "/chat") {
		return false
	}
	// 编排校验是 dry-run, 无状态变更
	if path == "/api/agent-studio/orchestrations/validate" {
		return false
	}
	for _, p := range auditSkipPathPrefixes {
		if strings.HasPrefix(path, p) {
			return false
		}
	}
	return true
}

var auditSensitiveKeys = map[string]struct{}{
	"password": {}, "oldpassword": {}, "newpassword": {}, "confirmpassword": {},
	"token": {}, "secret": {}, "apikey": {}, "authorization": {}, "totpsecret": {},
}

// maskSensitiveValue 递归脱敏: 敏感字段的值替换为 "***"
func maskSensitiveValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		for k, item := range val {
			if _, ok := auditSensitiveKeys[strings.ToLower(k)]; ok {
				val[k] = "***"
			} else {
				val[k] = maskSensitiveValue(item)
			}
		}
		return val
	case []any:
		for i, item := range val {
			val[i] = maskSensitiveValue(item)
		}
		return val
	default:
		return v
	}
}

// auditRequestBodyString 请求体先脱敏再按 rune 截断 (字节截断会切断多字节字符, 导致 utf8mb4 写入失败)
func auditRequestBodyString(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	masked := raw
	var v any
	if err := json.Unmarshal(raw, &v); err == nil {
		if out, err := json.Marshal(maskSensitiveValue(v)); err == nil {
			masked = out
		}
	}
	runes := []rune(string(masked))
	if len(runes) > auditRequestBodyLimit {
		runes = runes[:auditRequestBodyLimit]
	}
	return string(runes)
}

func auditTruncateRunes(s string, n int) string {
	runes := []rune(s)
	if len(runes) > n {
		return string(runes[:n])
	}
	return s
}

// auditResponseWriter 包装响应写入器, 采样响应体用于解析业务响应码
type auditResponseWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *auditResponseWriter) Write(b []byte) (int, error) {
	if w.body.Len() < auditResponseBodyLimit {
		if remain := auditResponseBodyLimit - w.body.Len(); remain < len(b) {
			w.body.Write(b[:remain])
		} else {
			w.body.Write(b)
		}
	}
	return w.ResponseWriter.Write(b)
}

// AuditMiddleware 记录变更类操作 (POST/PUT/DELETE/PATCH) 的审计日志。
// 必须注册在 AuthMiddleware 之后 (依赖 userId), 异步落库不影响请求。
func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldAuditRequest(c) {
			c.Next()
			return
		}

		var requestBody string
		if strings.HasPrefix(c.GetHeader("Content-Type"), "multipart/form-data") {
			requestBody = "[multipart/form-data]"
		} else if c.Request.Body != nil {
			raw, err := io.ReadAll(c.Request.Body)
			// 复原请求体给下游 handler (读取失败时给空体, handler 会返回参数错误)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))
			if err == nil {
				requestBody = auditRequestBodyString(raw)
			}
		}

		start := time.Now()
		respWriter := &auditResponseWriter{ResponseWriter: c.Writer}
		c.Writer = respWriter
		c.Next()

		entry := &model.OperationAuditLog{
			Method:      c.Request.Method,
			Path:        auditTruncateRunes(c.Request.URL.Path, 255),
			RequestBody: requestBody,
			IP:          auditTruncateRunes(c.ClientIP(), 64),
			UserAgent:   auditTruncateRunes(c.Request.UserAgent(), 255),
			LatencyMs:   time.Since(start).Milliseconds(),
			Success:     true,
		}
		if uid, exists := c.Get("userId"); exists {
			if id, ok := uid.(uint); ok {
				entry.UserID = &id
			}
		}

		// 统一响应结构按业务码判断成败; 非统一结构 (下载等) 按 HTTP 状态码兜底
		var resp struct {
			Code string `json:"code"`
			Msg  string `json:"msg"`
		}
		if err := json.Unmarshal(respWriter.body.Bytes(), &resp); err == nil && resp.Code != "" {
			entry.StatusCode = resp.Code
			if resp.Code != "0000" {
				entry.Success = false
				entry.ErrorMsg = auditTruncateRunes(resp.Msg, 500)
			}
		} else if respWriter.Status() >= http.StatusBadRequest {
			entry.Success = false
		}

		if auditQueue == nil {
			return
		}
		select {
		case auditQueue <- entry:
		default:
			log.Printf("审计日志队列已满, 丢弃记录 (method=%s path=%s)", entry.Method, entry.Path)
		}
	}
}
