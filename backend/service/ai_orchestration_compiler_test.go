package service

import (
	"context"
	"errors"
	"fmt"
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

// capturingModel 记录每次收到的消息序列, 用于断言发给模型的内容
type capturingModel struct {
	received [][]*schema.Message
}

func (f *capturingModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.received = append(f.received, input)
	return &schema.Message{Role: schema.Assistant, Content: "chain step answer"}, nil
}

func (f *capturingModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if _, err := f.Generate(ctx, input, opts...); err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: "chain step answer"}}), nil
}

func (f *capturingModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return f, nil }

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

// 链式 Agent: 第二个 Agent 收到的上游是 assistant 消息,
// 发往模型的序列必须被规范化为 user 开头 (Ark 1214 场景)
func TestOrchestrationAgentRoleNormalization(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "a1", "type": "agent", "name": "一", "config": {"system_prompt": "第一跳", "tools": ["orch_echo"]}},
	    {"id": "a2", "type": "agent", "name": "二", "config": {"system_prompt": "第二跳", "tools": ["orch_echo"]}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "a1", "target": "a2"},
	    {"source": "a2", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	second := &capturingModel{}
	modelBuilds := 0
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) {
			// getModel 在编译期为每个 agent 节点调用: 第 1 个节点用默认假模型, 第 2 个用捕获模型
			modelBuilds++
			if modelBuilds >= 2 {
				return second, nil
			}
			return &fakeOrchModel{}, nil
		},
		buildTool:   func(string) (tool.BaseTool, error) { return &fakeOrchTool{}, nil },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("开始"))
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
	if len(second.received) == 0 {
		t.Fatalf("第二个 Agent 未被调用")
	}
	msgs := second.received[0]
	if len(msgs) < 2 {
		t.Fatalf("第二个 Agent 收到消息序列过短: %d", len(msgs))
	}
	// 序列: [system(第二跳), user(上游 assistant 内容)] — 不能以 assistant 开头
	if msgs[0].Role != schema.System {
		t.Fatalf("首条应为 system, got %s", msgs[0].Role)
	}
	if msgs[1].Role != schema.User {
		t.Fatalf("第二跳的输入应被规范化为 user 角色, got %s: %q", msgs[1].Role, msgs[1].Content)
	}
	if msgs[1].Content != "agent final answer" {
		t.Fatalf("user 消息应携带上游内容, got %q", msgs[1].Content)
	}
	if len(msgs[1].ToolCalls) != 0 {
		t.Fatalf("转换为 user 的消息不应携带 ToolCalls")
	}
}

// ---------- 主 Agent 委派子Agent ----------

// scriptedToolCallModel 第一轮固定调用指定工具, 第二轮给出最终回复
type scriptedToolCallModel struct {
	toolName string
	args     string
	calls    int
}

func (f *scriptedToolCallModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	if f.calls == 1 {
		return &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_1",
			Function: schema.FunctionCall{Name: f.toolName, Arguments: f.args},
		}}}, nil
	}
	return &schema.Message{Role: schema.Assistant, Content: "主管总结完成"}, nil
}

func (f *scriptedToolCallModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if _, err := f.Generate(ctx, input, opts...); err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: "主管总结完成"}}), nil
}

func (f *scriptedToolCallModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return f, nil }

// 主 Agent 挂两个子Agent: 主 Agent 调用 subagent_1 委派任务,
// 子Agent 以 user 消息收到任务文本并把结果回传给主 Agent
func TestOrchestrationSubAgentDelegation(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "你是主管, 善于委派"}},
	    {"id": "sub1", "type": "subagent", "name": "调研员", "config": {"system_prompt": "你负责调研", "description": "负责资料调研"}},
	    {"id": "sub2", "type": "subagent", "name": "撰写员", "config": {"system_prompt": "你负责撰写", "description": "负责文案撰写"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "boss", "target": "sub1"},
	    {"source": "boss", "target": "sub2"},
	    {"source": "boss", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	sub1 := &capturingModel{received: make([][]*schema.Message, 0)}
	sub2 := &capturingModel{received: make([][]*schema.Message, 0)}
	parent := &scriptedToolCallModel{toolName: "subagent_1", args: `{"task":"调研上海"}`}
	builds := 0
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) {
			// 编译期调用顺序: 主管 -> 子Agent(按名称排序: 撰写员 -> 调研员? 名称排序: "撰写员" < "调研员" 按字典序不确定)
			// 因此按 build 序号: 第 1 个 build 为主管, 之后为子Agent; 子Agent 用捕获模型区分任务是否送达
			builds++
			switch builds {
			case 1:
				return parent, nil
			case 2:
				return sub1, nil
			default:
				return sub2, nil
			}
		},
		buildTool:   func(string) (tool.BaseTool, error) { return &fakeOrchTool{}, nil },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	if compiled.mode != "chain" {
		t.Fatalf("子Agent 不参与主流, 纯主管链应为 chain 模式, got %s", compiled.mode)
	}
	var events []string
	handler := newOrchTraceHandler(compiled.nodeKeys, func(event string, payload any) {
		if event == "node" {
			if m, ok := payload.(map[string]any); ok {
				events = append(events, fmt.Sprintf("%v %v %v owner=%v", m["kind"], m["key"], m["comp"], m["owner"]))
			}
		}
	})
	invokeMode := true
	var output strings.Builder
	if invokeMode {
		msg, err := compiled.runnable.Invoke(context.Background(), schema.UserMessage("开始"), []compose.Option{compose.WithCallbacks(handler)}...)
		if err != nil {
			t.Fatalf("invoke err: %v", err)
		}
		output.WriteString(msg.Content)
	} else {
		stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("开始"), []compose.Option{compose.WithCallbacks(handler)}...)
		if err != nil {
			t.Fatalf("stream err: %v", err)
		}
		for {
			chunk, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				break
			}
			if err != nil {
				t.Fatalf("recv err: %v", err)
			}
			output.WriteString(chunk.Content)
		}
	}
	if !strings.Contains(output.String(), "主管总结完成") {
		t.Fatalf("期望主管给出最终回复, got %q", output.String())
	}
	t.Logf("parent_calls=%d sub1=%d sub2=%d events=%v", parent.calls, len(sub1.received), len(sub2.received), events)
	// subagent_1 的任务应以 user 消息送达某个子Agent
	got := false
	for _, m := range append(sub1.received, sub2.received...) {
		for _, msg := range m {
			if msg.Role == schema.User && strings.Contains(msg.Content, "调研上海") {
				got = true
			}
		}
	}
	if !got {
		t.Fatalf("委派任务未送达子Agent")
	}
}

func TestOrchestrationSubAgentValidation(t *testing.T) {
	cases := []struct {
		name string
		dsl  string
		want string
	}{
		{"子Agent 缺主 Agent 入边", `{"nodes":[{"id":"s","type":"subagent","name":"s","config":{"system_prompt":"x"}},{"id":"a","type":"agent","name":"a","config":{}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"a","target":"o"}]}`, "必须恰好有一条来自主 Agent 的入边"},
		{"子Agent 出边", `{"nodes":[{"id":"a","type":"agent","name":"a","config":{}},{"id":"s","type":"subagent","name":"s","config":{"system_prompt":"x"}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"a","target":"s"},{"source":"s","target":"o"},{"source":"a","target":"o"}]}`, "不允许向外连线"},
		{"子Agent 空配置", `{"nodes":[{"id":"a","type":"agent","name":"a","config":{}},{"id":"s","type":"subagent","name":"s","config":{}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"a","target":"s"},{"source":"a","target":"o"}]}`, "需要引用已有 Agent 或填写内联系统提示词"},
		{"非 Agent 委派", `{"nodes":[{"id":"t","type":"template","name":"t","config":{"template":"{{.Input}}"}},{"id":"s","type":"subagent","name":"s","config":{"system_prompt":"x"}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"t","target":"s"},{"source":"t","target":"o"}]}`, "入边必须来自 Agent 节点"},
		{"主 Agent 仅委派也算出口", `{"nodes":[{"id":"a","type":"agent","name":"a","config":{"system_prompt":"x"}},{"id":"s","type":"subagent","name":"s","config":{"system_prompt":"y"}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"a","target":"s"}]}`, "出口节点"},
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
