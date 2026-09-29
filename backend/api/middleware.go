package api

import (
	"errors"
	"backend/service"
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

		hasPermission, err := service.CheckUserPermission(userID.(uint), permissions)
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

func GetUserID(c *gin.Context) uint {
	val, exists := c.Get("userId")
	if !exists {
		return 0
	}
	return val.(uint)
}
