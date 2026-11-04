package api

import "github.com/gin-gonic/gin"

// currentUserID 读取 AuthMiddleware 写入的认证用户 ID (middleware.go c.Set("userId", uint(...)));
// 未认证或类型异常返回 ok=false, 让调用方显式拒绝, 而不是断言失败静默得 0
func currentUserID(c *gin.Context) (uint, bool) {
	v, exists := c.Get("userId")
	if !exists {
		return 0, false
	}
	uid, ok := v.(uint)
	return uid, ok
}

// currentUserName 读取认证中间件写入的账号用户名 (OptionalAuthMiddleware c.Set("accountName", ...));
// 未认证时返回空串
func currentUserName(c *gin.Context) string {
	v, exists := c.Get("accountName")
	if !exists {
		return ""
	}
	name, _ := v.(string)
	return name
}
