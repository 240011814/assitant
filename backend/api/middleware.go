package api

import (
	"backend/service/auth"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func AuthMiddleware(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			SendError(c, "401", "未登录")
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
			// 固定 HMAC 签名算法, 防止算法混淆
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, errors.New("unexpected signing method")
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			SendError(c, "9999", "无效的 Token")
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			SendError(c, "401", "无效的 Token Claims")
			c.Abort()
			return
		}

		// 拒绝非访问令牌: 2FA 临时令牌 (purpose=2fa) 与刷新令牌 (typ=refresh)
		// 不得直接调用业务接口; 旧版签发的访问令牌无 typ 字段, 保持兼容放行
		if typ, _ := claims["typ"].(string); typ != "" && typ != "access" {
			SendError(c, "401", "令牌类型错误")
			c.Abort()
			return
		}
		if purpose, _ := claims["purpose"].(string); purpose != "" {
			SendError(c, "401", "令牌类型错误")
			c.Abort()
			return
		}

		userID, ok := claims["userId"].(float64)
		if !ok {
			SendError(c, "401", "无效的用户信息")
			c.Abort()
			return
		}

		c.Set("userId", uint(userID))
		if role, ok := claims["role"].(string); ok {
			c.Set("role", role)
		}
		c.Next()
	}
}

func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleValue, _ := role.(string)
		if _, ok := allowed[roleValue]; !ok {
			SendError(c, "403", "无权限访问")
			c.Abort()
			return
		}
		c.Next()
	}
}

func RequirePermission(permissions ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleValue, _ := role.(string)

		// 超级管理员拥有所有权限
		if roleValue == "R_SUPER" {
			c.Next()
			return
		}

		userID, exists := c.Get("userId")
		if !exists {
			SendError(c, "401", "未登录")
			c.Abort()
			return
		}

		hasPermission, err := auth.CheckUserPermission(userID.(uint), permissions)
		if err != nil {
			SendError(c, "500", "权限检查失败")
			c.Abort()
			return
		}

		if !hasPermission {
			SendError(c, "403", "无权限访问")
			c.Abort()
			return
		}

		c.Next()
	}
}

// HasPermission 判断当前请求用户是否拥有指定权限中的任意一个。
// 超级管理员始终拥有; 未登录/查询失败一律视为无权限。
// 供"字段级/资源级"权限判断复用 (RequirePermission 是整条路由的拦截)。
func HasPermission(c *gin.Context, permissions ...string) bool {
	role, _ := c.Get("role")
	if roleValue, _ := role.(string); roleValue == "R_SUPER" {
		return true
	}

	userID, exists := c.Get("userId")
	if !exists {
		return false
	}

	hasPermission, err := auth.CheckUserPermission(userID.(uint), permissions)
	if err != nil {
		return false
	}
	return hasPermission
}

// GetUserID 读取 AuthMiddleware 写入的认证用户 ID。
// 未认证返回 0; 类型异常 (非 uint) 也返回 0 而不是 panic —— 该函数在几十个 handler 中使用,
// 任何一处被非标准中间件链调用都不应拖垮请求。
func GetUserID(c *gin.Context) uint {
	uid, ok := currentUserID(c)
	if !ok {
		return 0
	}
	return uid
}
