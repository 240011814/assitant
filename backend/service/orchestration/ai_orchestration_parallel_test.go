package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

// runOrchDSLParallel 按服务层语义运行: 结构校验通过后再编译运行 (并行分支由节点配置决定)
func runOrchDSLParallel(t *testing.T, definition, input string) (mode string, output string, err error) {
	t.Helper()
	deps := orchTestDeps()
	dsl, issues := validateOrchestrationDSLDetailed(definition)
	if len(issues) > 0 {
		return "", "", errors.New(strings.Join(orchIssueMessages(issues), "; "))
	}
	c := &orchestrationCompiler{dsl: dsl, trace: newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		return "", "", err
	}
	c.trace.mergeCompiled(compiled.nodeKeys, compiled.subNodes)
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage(input))
	if err != nil {
		return compiled.mode, "", err
	}
	defer stream.Close()
	var sb strings.Builder
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return compiled.mode, sb.String(), err
		}
		sb.WriteString(chunk.Content)
	}
	return compiled.mode, sb.String(), nil
}

// 并行分支: 两个目标同时执行, 结果在合并节点汇聚
func TestOrchestrationParallelBranchRun(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "t", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "br", "type": "branch", "name": "并行分支", "config": {"mode": "parallel"}},
	    {"id": "p1", "type": "template", "name": "路线A", "config": {"template": "A:{{.Input}}"}},
	    {"id": "p2", "type": "template", "name": "路线B", "config": {"template": "B:{{.Input}}"}},
	    {"id": "m", "type": "merge", "name": "合并", "config": {}},
	    {"id": "o", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "t", "target": "br"},
	    {"source": "br", "target": "p1"},
	    {"source": "br", "target": "p2"},
	    {"source": "p1", "target": "m"},
	    {"source": "p2", "target": "m"},
	    {"source": "m", "target": "o"}
	  ]
	}`
	_, output, err := runOrchDSLParallel(t, definition, "hi")
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	if !strings.Contains(output, "A:hi") || !strings.Contains(output, "B:hi") {
		t.Fatalf("期望两路并行结果都出现在合并输出, got %q", output)
	}
}

// 互斥路由 + 合并: 未选中的路径被整体跳过, 合并节点只收到选中路径的结果
func TestOrchestrationRouteBranchMergeRun(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "t", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "br", "type": "branch", "name": "路由", "config": {"cases": [{"type": "contains", "value": "A", "target": "pa"}], "default_target": "pb"}},
	    {"id": "pa", "type": "template", "name": "路线A", "config": {"template": "A:{{.Input}}"}},
	    {"id": "pb", "type": "template", "name": "路线B", "config": {"template": "B:{{.Input}}"}},
	    {"id": "m", "type": "merge", "name": "合并", "config": {}},
	    {"id": "o", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "t", "target": "br"},
	    {"source": "br", "target": "pa"},
	    {"source": "br", "target": "pb"},
	    {"source": "pa", "target": "m"},
	    {"source": "pb", "target": "m"},
	    {"source": "m", "target": "o"}
	  ]
	}`
	_, output, err := runOrchDSLParallel(t, definition, "AAA")
	if err != nil {
		t.Fatalf("运行失败: %v", err)
	}
	if !strings.Contains(output, "A:AAA") {
		t.Fatalf("期望选中路径结果出现, got %q", output)
	}
	if strings.Contains(output, "B:") {
		t.Fatalf("未选中路径不应被执行, got %q", output)
	}
}

// 校验: 分支与合并混用由节点配置直接决定 (parallel/route 均放行, 无全局开关)
func TestOrchestrationParallelGate(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "t", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "br", "type": "branch", "name": "分支", "config": {"mode": "parallel"}},
	    {"id": "p1", "type": "template", "name": "路线A", "config": {"template": "A"}},
	    {"id": "p2", "type": "template", "name": "路线B", "config": {"template": "B"}},
	    {"id": "m", "type": "merge", "name": "合并", "config": {}},
	    {"id": "o", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "t", "target": "br"},
	    {"source": "br", "target": "p1"},
	    {"source": "br", "target": "p2"},
	    {"source": "p1", "target": "m"},
	    {"source": "p2", "target": "m"},
	    {"source": "m", "target": "o"}
	  ]
	}`
	if _, errs := validateOrchestrationDSLDetailed(definition); len(errs) != 0 {
		t.Fatalf("并行分支与合并混用期望校验通过, got %v", errs)
	}

	// 对照: 互斥路由分支 (mode=route) 与合并混用同样放行 (skip 语义, 由节点 mode 决定)
	route := strings.Replace(definition, `{"id": "br", "type": "branch", "name": "并行分支", "config": {"mode": "parallel"}}`,
		`{"id": "br", "type": "branch", "name": "分支", "config": {"cases": [{"type": "contains", "value": "A", "target": "p1"}], "default_target": "p2"}}`, 1)
	if _, errs := validateOrchestrationDSLDetailed(route); len(errs) != 0 {
		t.Fatalf("互斥分支与合并混用期望校验通过, got %v", errs)
	}
}

// 校验: 并行分支的每条路径必须汇入合并节点; 至少两条分发连线
func TestOrchestrationParallelValidation(t *testing.T) {
	// 路径直达 END, 不经合并
	noConverge := `{
	  "version": 1,
	  "nodes": [
	    {"id": "br", "type": "branch", "name": "并行分支", "config": {"mode": "parallel"}},
	    {"id": "p1", "type": "template", "name": "路线A", "config": {"template": "A"}},
	    {"id": "o", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "br", "target": "p1"},
	    {"source": "p1", "target": "o"}
	  ]
	}`
	_, errs := validateOrchestrationDSLDetailed(noConverge)
	joined := strings.Join(orchIssueMessages(errs), ";")
	if len(errs) == 0 || !strings.Contains(joined, "汇入合并节点") && !strings.Contains(joined, "至少需要两条分发连线") {
		t.Fatalf("期望并行分支汇聚/连线校验失败, got %v", errs)
	}

	// 只有一条分发连线
	single := `{
	  "version": 1,
	  "nodes": [
	    {"id": "br", "type": "branch", "name": "并行分支", "config": {"mode": "parallel"}},
	    {"id": "p1", "type": "template", "name": "路线A", "config": {"template": "A"}},
	    {"id": "m", "type": "merge", "name": "合并", "config": {}},
	    {"id": "o", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "br", "target": "p1"},
	    {"source": "p1", "target": "m"},
	    {"source": "m", "target": "o"}
	  ]
	}`
	_, errs = validateOrchestrationDSLDetailed(single)
	if len(errs) == 0 || !strings.Contains(strings.Join(orchIssueMessages(errs), ";"), "至少需要两条分发连线") {
		t.Fatalf("期望单路并行分支校验失败, got %v", errs)
	}
}

// 工具审批: 批准后执行原工具, 拒绝时把原因作为工具结果返回
func TestOrchestrationApprovalTool(t *testing.T) {
	inner := &fakeOrchTool{}
	tw := &orchApprovalTool{name: "orch_gate", inner: inner}
	handler := newOrchTraceHandler(map[string]string{"a": "a"}, nil, nil)
	ctx := withOrchHandler(withOrchRunID(context.Background(), "run_test"), handler)

	waitApproval := func(callID string, approved bool, reason string) {
		go func() {
			for i := 0; i < 200; i++ {
				if ResolveOrchestrationApproval("run_test", callID, approved, reason) {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
		}()
	}

	// 批准: 执行原工具
	waitApproval("orch_gate-1", true, "")
	res, err := tw.InvokableRun(ctx, `{"input":"x"}`)
	if err != nil {
		t.Fatalf("批准后执行失败: %v", err)
	}
	if !strings.Contains(res, `tool:{`) {
		t.Fatalf("批准后应返回原工具结果, got %q", res)
	}

	// 拒绝: 原因作为工具结果返回给模型, 不报错
	waitApproval("orch_gate-2", false, "涉密内容不允许导出")
	res, err = tw.InvokableRun(ctx, `{"input":"y"}`)
	if err != nil {
		t.Fatalf("拒绝时不应返回错误 (原因要交给模型), got %v", err)
	}
	if !strings.Contains(res, "被用户拒绝") || !strings.Contains(res, "涉密内容不允许导出") {
		t.Fatalf("拒绝原因应出现在工具结果里, got %q", res)
	}
}
