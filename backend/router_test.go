package main

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestSetupRouterNoPanic 全量注册路由: gin 的路径树冲突 (静态段与参数段同级等) 在注册时 panic,
// 这里用零值依赖只验证注册本身, 不触发任何 handler
func TestSetupRouterNoPanic(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("路由注册 panic: %v", rec)
		}
	}()
	setupRouter(r, &appDeps{jwtSecret: "test-secret"})
}
