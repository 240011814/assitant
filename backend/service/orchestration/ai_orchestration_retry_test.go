package service

import (
	"context"
	"strings"
	"testing"

	coremodel "github.com/cloudwego/eino/components/model"
)

// 节点 max_retries 应转换为 ark SDK 的 RetryTimes 覆盖:
// >0 时把次数传给 getModelRetry, 0 (未配置) 时传 nil 沿用框架默认 (2 次)
func TestOrchestrationNodeRetryConfig(t *testing.T) {
	build := func(agentConfig string, withRetryDep bool) []int {
		var retried []int // -1 = nil (沿用框架默认)
		deps := orchTestDeps()
		if withRetryDep {
			deps.getModelRetry = func(mo string, rt *int) (coremodel.ToolCallingChatModel, error) {
				if rt == nil {
					retried = append(retried, -1)
				} else {
					retried = append(retried, *rt)
				}
				return deps.getModel(mo)
			}
		}
		definition := `{"version":1,"nodes":[
			{"id":"t","type":"template","name":"t","config":{"template":"{{.Input}}"}},
			{"id":"a","type":"agent","name":"a","config":{"system_prompt":"x"` + agentConfig + `}},
			{"id":"o","type":"end","name":"o","config":{}}],
			"edges":[{"source":"t","target":"a"},{"source":"a","target":"o"}]}`
		dsl, issues := validateOrchestrationDSLDetailed(definition)
		if len(issues) > 0 {
			t.Fatalf("校验失败: %v", orchIssueMessages(issues))
		}
		c := &orchestrationCompiler{dsl: dsl}
		if _, err := c.compile(context.Background(), deps); err != nil {
			t.Fatalf("编译失败: %v", err)
		}
		return retried
	}

	if got := build(`, "max_retries": 3`, true); len(got) != 1 || got[0] != 3 {
		t.Fatalf("max_retries=3 期望重试次数 3 传给模型构建, got %v", got)
	}
	if got := build(``, true); len(got) != 1 || got[0] != -1 {
		t.Fatalf("未配置 max_retries 期望传 nil (框架默认), got %v", got)
	}
	// deps 未提供 getModelRetry (旧装配/单测): 回退 getModel, 编译不受影响
	if got := build(`, "max_retries": 5`, false); len(got) != 0 {
		t.Fatalf("无 getModelRetry 时不应有重试捕获, got %v", got)
	}
}

// 校验: max_retries 越界报带节点定位的错误, 0 与上限内合法
func TestOrchestrationRetryValidation(t *testing.T) {
	build := func(retryJSON string) []string {
		definition := `{"version":1,"nodes":[
			{"id":"a","type":"agent","name":"a","config":{"system_prompt":"x"` + retryJSON + `}},
			{"id":"o","type":"end","name":"o","config":{}}],
			"edges":[{"source":"a","target":"o"}]}`
		_, issues := validateOrchestrationDSLDetailed(definition)
		return orchIssueMessages(issues)
	}
	if errs := build(`, "max_retries": -1`); len(errs) == 0 || !strings.Contains(strings.Join(errs, ";"), "失败重试次数") {
		t.Fatalf("负数重试期望校验失败, got %v", errs)
	}
	if errs := build(`, "max_retries": 11`); len(errs) == 0 || !strings.Contains(strings.Join(errs, ";"), "失败重试次数") {
		t.Fatalf("超上限重试期望校验失败, got %v", errs)
	}
	for _, ok := range []string{"", `, "max_retries": 0`, `, "max_retries": 10`} {
		if errs := build(ok); len(errs) != 0 {
			t.Fatalf("合法重试配置 %q 期望校验通过, got %v", ok, errs)
		}
	}

	// 子Agent 节点同样校验
	sub := `{"version":1,"nodes":[
		{"id":"a","type":"agent","name":"a","config":{"system_prompt":"x"}},
		{"id":"s","type":"subagent","name":"s","config":{"system_prompt":"y","max_retries":-2}},
		{"id":"o","type":"end","name":"o","config":{}}],
		"edges":[{"source":"a","target":"o"},{"source":"a","target":"s"}]}`
	if _, issues := validateOrchestrationDSLDetailed(sub); len(issues) == 0 {
		t.Fatalf("子Agent 负数重试期望校验失败")
	}
}
