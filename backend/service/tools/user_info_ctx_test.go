package tools

import (
	"context"
	"testing"
)

// 编排等非 ADK 运行时没有会话值: 工具应能取到 WithRunUserID 注入的用户 ID,
// 什么都没有时保持"无法获取用户 ID"报错 (普通对话经 ADK 会话注入, 不受影响)。
// ADK 会话路径无法在单测里构造 (runCtxKey 未导出), 由线上普通对话覆盖。
func TestUserIDFromSession(t *testing.T) {
	uid, err := userIDFromSession(WithRunUserID(context.Background(), 42))
	if err != nil || uid != 42 {
		t.Fatalf("WithRunUserID 注入的用户 ID 应可用: uid=%d err=%v", uid, err)
	}

	if _, err := userIDFromSession(context.Background()); err == nil {
		t.Fatalf("无任何来源时应报无法获取用户 ID")
	}

	if _, err := userIDFromSession(WithRunUserID(context.Background(), 0)); err == nil {
		t.Fatalf("用户 ID=0 不应被接受")
	}
}
