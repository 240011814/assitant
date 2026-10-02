package api

import (
	"net/http"

	"backend/model"

	"github.com/gin-gonic/gin"
)

// RecordLoginAudit 记录一次登录/注册/2FA 尝试 (成功与失败都记, 失败记录是防爆破审计的关键)。
// 复用操作审计的异步写入队列, 落入同一张 operation_audit_logs 表。
// username 为尝试输入的用户名 (即使不存在也照实记录); userID 已知时传入, 否则由 worker 落库时回填用户名快照。
func RecordLoginAudit(c *gin.Context, userID *uint, username, path string, success bool, errMsg string) {
	if auditQueue == nil || c == nil || c.Request == nil {
		return
	}

	entry := &model.OperationAuditLog{
		UserID:    userID,
		UserName:  auditTruncateRunes(username, 50),
		Method:    http.MethodPost,
		Path:      path,
		IP:        auditTruncateRunes(c.ClientIP(), 64),
		UserAgent: auditTruncateRunes(c.Request.UserAgent(), 255),
		Success:   success,
	}
	if success {
		entry.StatusCode = "0000"
	} else {
		entry.StatusCode = "1001"
		entry.ErrorMsg = auditTruncateRunes(errMsg, 500)
	}

	select {
	case auditQueue <- entry:
	default:
	}
}
