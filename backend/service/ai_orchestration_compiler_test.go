package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

// ---------- 测试桩 ----------

type fakeOrchModel struct{ calls int }

func (f *fakeOrchModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	if f.calls == 1 {
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_1",
			Function: schema.FunctionCall{Name: "orch_echo", Arguments: `{"input":"hi"}`},
		}}}, nil
	}
	return &schema.Message{
		Role:         schema.Assistant,
		Content:      "agent final answer",
		ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}},
	}, nil
}

func (f *fakeOrchModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := f.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	if len(msg.ToolCalls) > 0 {
		return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
	}
	var chunks []*schema.Message
	for _, w := range strings.Split(msg.Content, " ") {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, Content: w + " "})
	}
	chunks[len(chunks)-1].Content = strings.TrimSpace(chunks[len(chunks)-1].Content)
	chunks[len(chunks)-1].ResponseMeta = msg.ResponseMeta
	return schema.StreamReaderFromArray(chunks), nil
}

func (f *fakeOrchModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return f, nil }

type fakeOrchTool struct{}

func (t *fakeOrchTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{Name: "orch_echo", Desc: "echo"}, nil
}
func (t *fakeOrchTool) InvokableRun(_ context.Context, args string, _ ...tool.Option) (string, error) {
	return "tool:" + args, nil
}

func orchTestDeps() compilerDeps {
	return compilerDeps{
		getModel:  func(string) (model.ToolCallingChatModel, error) { return &fakeOrchModel{}, nil },
		buildTool: func(string) (tool.BaseTool, error) { return &fakeOrchTool{}, nil },
		sessionVars: func() map[string]any {
			return map[string]any{"current_time": "2026-09-30", "user_id": 1}
		},
	}
}

func runOrchDSL(t *testing.T, definition, input string) (mode string, output string, err error) {
	t.Helper()
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		return "", "", errors.New(strings.Join(errs, "; "))
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), orchTestDeps())
	if err != nil {
		return "", "", err
	}
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

// ---------- 编排定义 ----------

const orchChainDSL = `{
  "version": 1,
  "nodes": [
    {"id": "tpl", "type": "template", "name": "改写", "config": {"template": "请处理: {{.Input}} ({{.current_time}})"}},
    {"id": "agent", "type": "agent", "name": "助手", "config": {"system_prompt": "你是测试助手", "tools": ["orch_echo"]}},
    {"id": "out", "type": "end", "name": "输出", "config": {}}
  ],
  "edges": [
    {"source": "tpl", "target": "agent"},
    {"source": "agent", "target": "out"}
  ]
}`

const orchBranchDSL = `{
  "version": 1,
  "nodes": [
    {"id": "tpl", "type": "template", "name": "改写", "config": {"template": "choose A: {{.Input}}"}},
    {"id": "br", "type": "branch", "name": "路由", "config": {"cases": [{"type": "contains", "value": "A", "target": "agentA"}, {"type": "contains", "value": "B", "target": "agentB"}], "default_target": "agentB"}},
    {"id": "agentA", "type": "agent", "name": "A助手", "config": {"system_prompt": "A", "tools": ["orch_echo"]}},
    {"id": "agentB", "type": "agent", "name": "B助手", "config": {"system_prompt": "B", "tools": ["orch_echo"]}},
    {"id": "out", "type": "end", "name": "输出", "config": {}}
  ],
  "edges": [
    {"source": "tpl", "target": "br"},
    {"source": "br", "target": "agentA", "label": "A"},
    {"source": "br", "target": "agentB", "label": "B"},
    {"source": "agentA", "target": "out"},
    {"source": "agentB", "target": "out"}
  ]
}`

const orchMergeDSL = `{
  "version": 1,
  "nodes": [
    {"id": "agent1", "type": "agent", "name": "左", "config": {"system_prompt": "左", "tools": ["orch_echo"]}},
    {"id": "agent2", "type": "agent", "name": "右", "config": {"system_prompt": "右", "tools": ["orch_echo"]}},
    {"id": "merge", "type": "merge", "name": "合并", "config": {"separator": " | "}},
    {"id": "out", "type": "end", "name": "输出", "config": {}}
  ],
  "edges": [
    {"source": "agent1", "target": "merge"},
    {"source": "agent2", "target": "merge"},
    {"source": "merge", "target": "out"}
  ]
}`

// ---------- 用例 ----------

func TestOrchestrationChainRun(t *testing.T) {
	mode, output, err := runOrchDSL(t, orchChainDSL, "hello")
	if err != nil {
		t.Fatalf("chain run err: %v", err)
	}
	if mode != "chain" {
		t.Fatalf("mode = %s, want chain", mode)
	}
	if !strings.Contains(output, "agent final answer") {
		t.Fatalf("output = %q, want contains agent final answer", output)
	}
}

func TestOrchestrationBranchRun(t *testing.T) {
	mode, output, err := runOrchDSL(t, orchBranchDSL, "hello")
	if err != nil {
		t.Fatalf("branch run err: %v", err)
	}
	if mode != "graph" {
		t.Fatalf("mode = %s, want graph", mode)
	}
	if !strings.Contains(output, "agent final answer") {
		t.Fatalf("output = %q, want contains agent final answer", output)
	}
}

func TestOrchestrationMergeRun(t *testing.T) {
	mode, output, err := runOrchDSL(t, orchMergeDSL, "hello")
	if err != nil {
		t.Fatalf("merge run err: %v", err)
	}
	if mode != "workflow" {
		t.Fatalf("mode = %s, want workflow", mode)
	}
	if !strings.Contains(output, "agent final answer") {
		t.Fatalf("output = %q, want contains agent final answer", output)
	}
}

func TestOrchestrationValidation(t *testing.T) {
	cases := []struct {
		name string
		dsl  string
		want string
	}{
		{"环", `{"nodes":[{"id":"a","type":"agent","name":"a","config":{}},{"id":"b","type":"end","name":"b","config":{}}],"edges":[{"source":"a","target":"b"},{"source":"b","target":"a"}]}`, "循环连线"},
		{"多入边", `{"nodes":[{"id":"a","type":"agent","name":"a","config":{}},{"id":"b","type":"agent","name":"b","config":{}},{"id":"c","type":"end","name":"c","config":{}}],"edges":[{"source":"a","target":"c"},{"source":"b","target":"c"}]}`, "只允许一条入边"},
		{"分支合并混用", `{"nodes":[{"id":"t","type":"template","name":"t","config":{"template":"{{.Input}}"}},{"id":"br","type":"branch","name":"br","config":{"cases":[{"type":"contains","value":"A","target":"a"}],"default_target":"a"}},{"id":"a","type":"agent","name":"a","config":{"system_prompt":"x"}},{"id":"m","type":"merge","name":"m","config":{}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"t","target":"br"},{"source":"br","target":"a","label":"A"},{"source":"a","target":"m"},{"source":"t","target":"m"},{"source":"m","target":"o"}]}`, "暂不支持"},
	}
	for _, tc := range cases {
		_, errs := validateOrchestrationDSL(tc.dsl)
		if len(errs) == 0 {
			t.Fatalf("%s: 期望校验失败", tc.name)
		}
		if !strings.Contains(strings.Join(errs, ";"), tc.want) {
			t.Fatalf("%s: 错误 %v 不含 %q", tc.name, errs, tc.want)
		}
	}
}

func TestOrchestrationTraceAttribution(t *testing.T) {
	dsl, errs := validateOrchestrationDSL(orchChainDSL)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), orchTestDeps())
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	var events []map[string]any
	handler := newOrchTraceHandler(compiled.nodeKeys, func(event string, payload any) {
		if event == "node" {
			if m, ok := payload.(map[string]any); ok {
				events = append(events, m)
			}
		}
	})
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("hi"), []compose.Option{compose.WithCallbacks(handler)}...)
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	for {
		_, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("recv err: %v", err)
		}
	}
	traces := handler.NodeTraces()
	if len(traces) == 0 {
		t.Fatalf("期望产生节点摘要")
	}
	var agentTrace *OrchNodeTrace
	for _, tr := range traces {
		if tr.Key == "agent" {
			agentTrace = tr
		}
	}
	if agentTrace == nil {
		t.Fatalf("期望 agent 节点摘要, got %v", traces)
	}
	if agentTrace.Status != "success" || !strings.Contains(agentTrace.Content, "agent final answer") {
		t.Fatalf("agent trace 状态/内容异常: %+v", agentTrace)
	}
	if agentTrace.Tokens == nil || agentTrace.Tokens.TotalTokens <= 0 {
		t.Fatalf("期望聚合 token 用量, got %+v; events=%+v traces=%+v", agentTrace.Tokens, events, traces)
	}
	if len(agentTrace.ToolCalls) == 0 {
		t.Fatalf("期望记录工具调用, got %+v", agentTrace.ToolCalls)
	}
	_ = events
}
