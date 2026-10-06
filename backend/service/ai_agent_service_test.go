package service

import (
	"testing"
	"time"

	"backend/model"
)

// TestResolveModelCode 空串=默认模型 的解析规则 (token 记账归属依赖它)
func TestResolveModelCode(t *testing.T) {
	svc := &AIAgentService{
		activeProvider: &model.AIProvider{ID: 1, Name: "p"},
		activeModel:    &model.AIModel{ModelCode: "ep-20260101"},
	}

	if got := svc.ResolveModelCode("ep-custom"); got != "ep-custom" {
		t.Fatalf("显式 override 应原样返回, got %q", got)
	}
	if got := svc.ResolveModelCode(""); got != "ep-20260101" {
		t.Fatalf("空串应解析为默认模型 code, got %q", got)
	}

	// 未配置任何模型时空串解析为空串 (调用方据此记为默认模型桶)
	empty := &AIAgentService{}
	if got := empty.ResolveModelCode(""); got != "" {
		t.Fatalf("无默认模型时应返回空串, got %q", got)
	}
}

// TestHasActiveModel 后台任务前置检查 (锁内快照读)
func TestHasActiveModel(t *testing.T) {
	empty := &AIAgentService{}
	if empty.HasActiveModel() {
		t.Fatal("未配置 provider/model 时应返回 false")
	}
	onlyProvider := &AIAgentService{activeProvider: &model.AIProvider{ID: 1}}
	if onlyProvider.HasActiveModel() {
		t.Fatal("缺 model 时应返回 false")
	}
	full := &AIAgentService{
		activeProvider: &model.AIProvider{ID: 1},
		activeModel:    &model.AIModel{ModelCode: "ep-1"},
	}
	if !full.HasActiveModel() {
		t.Fatal("provider+model 齐备时应返回 true")
	}
}

// TestAIAgentServiceGetTimeout 超时快照读
func TestAIAgentServiceGetTimeout(t *testing.T) {
	svc := &AIAgentService{timeout: 3 * time.Minute}
	if got := svc.getTimeout(); got != 3*time.Minute {
		t.Fatalf("getTimeout = %v, want 3m", got)
	}
}
