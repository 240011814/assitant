package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	coremodel "backend/model"

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

func (f *fakeOrchModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

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

func (f *capturingModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
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
	return runOrchDSLWithDeps(t, definition, input, orchTestDeps())
}

func runOrchDSLWithDeps(t *testing.T, definition, input string, deps compilerDeps) (mode string, output string, err error) {
	t.Helper()
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		return "", "", errors.New(strings.Join(errs, "; "))
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
	handler := newOrchTraceHandler(compiled.nodeKeys, orchSubAgentKeysOf(dsl), func(event string, payload any) {
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

// 编排节点的系统提示词必须自动带上当前时间与用户画像: 普通 Agent 对话的 Instruction 里有,
// 但编排先前只有显式写 {{.current_time}}/{{.user_profile}} 才有, 导致"你是我的助手"这类提示词下模型拿不到。
func TestOrchestrationAgentInjectsRuntimeContext(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "agent", "type": "agent", "name": "助手", "config": {"system_prompt": "你是我的助手", "tools": []}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "agent", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	capModel := &capturingModel{}
	deps := compilerDeps{
		getModel:  func(string) (model.ToolCallingChatModel, error) { return capModel, nil },
		buildTool: func(string) (tool.BaseTool, error) { return &fakeOrchTool{}, nil },
		sessionVars: func() map[string]any {
			return map[string]any{
				"current_time": "2026-09-30 10:00:00",
				"user_id":      7,
				"user_profile": "## 用户画像（供参考，不要直接复述）\n喜欢简洁回答",
			}
		},
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("现在几点"))
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
	if len(capModel.received) == 0 {
		t.Fatalf("Agent 未被调用")
	}
	msgs := capModel.received[0]
	if len(msgs) == 0 || msgs[0].Role != schema.System {
		t.Fatalf("首条应为 system, got %v", msgs)
	}
	if !strings.Contains(msgs[0].Content, "你是我的助手") {
		t.Fatalf("系统提示词丢失: %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "当前时间: 2026-09-30 10:00:00") {
		t.Fatalf("未向 Agent 注入当前时间: %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[0].Content, "喜欢简洁回答") {
		t.Fatalf("未向 Agent 注入用户画像: %q", msgs[0].Content)
	}
	// 首轮(无历史)不应出现多轮对话提示
	if strings.Contains(msgs[0].Content, "【多轮对话】") {
		t.Fatalf("无历史时不应注入多轮对话提示: %q", msgs[0].Content)
	}
}

func TestOrchInjectRuntimeContext(t *testing.T) {
	vars := map[string]any{"current_time": "2026-09-30 10:00:00", "user_id": 7}

	got := orchInjectRuntimeContext("你是我的助手", vars)
	if !strings.Contains(got, "当前时间: 2026-09-30 10:00:00") {
		t.Fatalf("未注入当前时间: %q", got)
	}
	if !strings.Contains(got, "你是我的助手") || !strings.Contains(got, "当前用户ID: 7") {
		t.Fatalf("提示词/用户ID 注入异常: %q", got)
	}

	// 用户画像自动注入
	withProfile := map[string]any{
		"current_time": "2026-09-30 10:00:00",
		"user_id":      7,
		"user_profile": "## 用户画像（供参考，不要直接复述）\n喜欢简洁回答",
	}
	got = orchInjectRuntimeContext("你是我的助手", withProfile)
	if !strings.Contains(got, "喜欢简洁回答") {
		t.Fatalf("未注入用户画像: %q", got)
	}
	// 提示词已引用用户画像时不重复追加
	profileOnly := "## 用户画像（供参考，不要直接复述）\n喜欢简洁回答"
	if got := orchInjectRuntimeContext(profileOnly, withProfile); strings.Count(got, "喜欢简洁回答") != 1 {
		t.Fatalf("用户画像不应重复注入: %q", got)
	}

	// 提示词已渲染出当前时间时不重复追加
	rendered := "当前时间: 2026-09-30 10:00:00"
	if got := orchInjectRuntimeContext(rendered, vars); got != rendered {
		t.Fatalf("已包含时间时不应重复注入: %q", got)
	}

	// 空提示词只给运行时上下文, 不以空行开头
	if got := orchInjectRuntimeContext("  ", vars); !strings.HasPrefix(got, "当前时间: ") {
		t.Fatalf("空提示词注入应以时间开头: %q", got)
	}
}

// ---------- 主 Agent 委派子Agent ----------

// scriptedToolCallModel 第一轮固定调用指定工具, 第二轮给出最终回复
type scriptedToolCallModel struct {
	toolName string
	args     string
	calls    int
	// providedTools 模型实际收到的工具名 (由 WithTools 注入) —— 用于断言委派工具已注册
	providedTools []string
	// received 每次调用收到的消息序列 (用于断言系统提示词里的子Agent 委派指引)
	received [][]*schema.Message
}

func (f *scriptedToolCallModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	f.received = append(f.received, input)
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

func (f *scriptedToolCallModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	f.providedTools = f.providedTools[:0]
	for _, t := range tools {
		f.providedTools = append(f.providedTools, t.Name)
	}
	return f, nil
}

// 实测复现: 线上"编排 id=3"的定义 (1 主 Agent + 3 个同名子Agent, 全部引用已有 Agent)。
// 断言主 Agent 的 ReAct 工具表里真的出现 3 个委派工具, 且主 Agent 调用委派工具时
// 子Agent 会执行并把结果回传——这是"新建编排后子Agent 未被触发"的直接回归防线。
// 子Agent 同名, 序号按 id 字典序: n_munqwe85_657=subagent_1, n_munqwjk2_882=subagent_2, n_munqwmfa_341=subagent_3
const orchLiveSubAgentDSL = `{
  "version": 1,
  "nodes": [
    {"id": "n_munqw8ao_307", "type": "template", "name": "模板", "config": {"template": "请处理以下内容:\n{{.Input}}"}},
    {"id": "n_munqw8ao_726", "type": "agent", "name": "Agent", "config": {"max_iterations": 0, "model": "", "system_prompt": "你是ai助手", "tools": []}},
    {"id": "n_munqw8ao_261", "type": "end", "name": "结束", "config": {}},
    {"id": "n_munqwe85_657", "type": "subagent", "name": "子Agent", "config": {"agent_id": 3, "description": "", "model": "", "system_prompt": "", "tools": []}},
    {"id": "n_munqwjk2_882", "type": "subagent", "name": "子Agent", "config": {"agent_id": 5, "description": "", "model": "", "system_prompt": "", "tools": []}},
    {"id": "n_munqwmfa_341", "type": "subagent", "name": "子Agent", "config": {"agent_id": 4, "description": "", "model": "", "system_prompt": "", "tools": []}}
  ],
  "edges": [
    {"source": "n_munqw8ao_307", "target": "n_munqw8ao_726"},
    {"source": "n_munqw8ao_726", "target": "n_munqwe85_657"},
    {"source": "n_munqw8ao_726", "target": "n_munqwjk2_882"},
    {"source": "n_munqw8ao_726", "target": "n_munqwmfa_341"},
    {"source": "n_munqw8ao_726", "target": "n_munqw8ao_261"}
  ]
}`

func TestOrchestrationLiveSubAgentDef(t *testing.T) {
	dsl, errs := validateOrchestrationDSL(orchLiveSubAgentDSL)
	if len(errs) > 0 {
		t.Fatalf("线上定义校验失败: %v", errs)
	}
	parent := &scriptedToolCallModel{toolName: "subagent_1", args: `{"task":"用英语练一段点咖啡"}`}
	subs := []*capturingModel{{}, {}, {}}
	builds := 0
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) {
			builds++
			if builds == 1 {
				return parent, nil
			}
			idx := builds - 2
			if idx >= len(subs) {
				idx = len(subs) - 1
			}
			return subs[idx], nil
		},
		buildTool:   func(string) (tool.BaseTool, error) { return &fakeOrchTool{}, nil },
		sessionVars: func() map[string]any { return map[string]any{} },
		// 被引用的三个已有 Agent (对应线上 ai_agents 表 id=3/4/5)
		lookupAgent: func(id uint) (*coremodel.AIAgent, error) {
			switch id {
			case 3:
				return &coremodel.AIAgent{ID: 3, Title: "英语情景对话", Description: "通过模拟真实生活场景练习地道英语口语表达", SystemPrompt: "You are an English teacher."}, nil
			case 4:
				return &coremodel.AIAgent{ID: 4, Title: "决策训练", Description: "学习60+决策模型", SystemPrompt: "你是决策教练"}, nil
			case 5:
				return &coremodel.AIAgent{ID: 5, Title: "社交训练", Description: "练习聊天破冰", SystemPrompt: "你是社交教练"}, nil
			}
			return nil, fmt.Errorf("agent %d not found", id)
		},
	}
	handler := newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)
	c := &orchestrationCompiler{dsl: dsl, trace: handler}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("线上定义编译失败: %v", err)
	}
	// 3 个子Agent 都必须变成主 Agent 的委派工具 (此前"改到一半"时这里一个都没有)
	if got := c.subAgentIDsOf("n_munqw8ao_726"); len(got) != 3 || got[0] != "n_munqwe85_657" {
		t.Fatalf("委派工具序号依据异常: %v", got)
	}
	msg, err := compiled.runnable.Invoke(context.Background(), schema.UserMessage("你好"))
	if err != nil {
		t.Fatalf("线上定义运行失败: %v", err)
	}
	if !strings.Contains(msg.Content, "主管总结完成") {
		t.Fatalf("主 Agent 未给出最终回复: %q", msg.Content)
	}
	// 关键回归: 3 个委派工具必须真的进到模型的工具表里 (否则模型永远不可能委派)
	want := map[string]bool{"subagent_1": false, "subagent_2": false, "subagent_3": false}
	for _, name := range parent.providedTools {
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, ok := range want {
		if !ok {
			t.Fatalf("委派工具 %s 未注册到模型工具表, got %v", name, parent.providedTools)
		}
	}
	// 系统提示词里必须出现子Agent 委派指引 (线上实测: 不写进提示词模型就一直不委派)
	if len(parent.received) == 0 {
		t.Fatalf("主 Agent 未被调用")
	}
	sysPrompt := ""
	for _, m := range parent.received[0] {
		if m.Role == schema.System {
			sysPrompt += m.Content
		}
	}
	for _, needle := range []string{"【子Agent 委派】", "英语情景对话", "subagent_1", "决策训练", "社交训练"} {
		if !strings.Contains(sysPrompt, needle) {
			t.Fatalf("系统提示词缺少委派指引 %q:\n%s", needle, sysPrompt)
		}
	}
	var delegated *OrchNodeTrace
	for _, tr := range handler.NodeTraces() {
		if tr.Key == "n_munqwe85_657" {
			delegated = tr
		}
	}
	if delegated == nil || !delegated.Delegated || delegated.Owner != "n_munqw8ao_726" {
		t.Fatalf("subagent_1 委派事件缺失: %+v", handler.NodeTraces())
	}
	if !strings.Contains(delegated.Task, "点咖啡") {
		t.Fatalf("委派任务未记录: %+v", delegated)
	}
	// 被委派的子Agent 必须以 user 消息收到任务
	gotTask := false
	for _, m := range subs[0].received {
		for _, msg := range m {
			if msg.Role == schema.User && strings.Contains(msg.Content, "点咖啡") {
				gotTask = true
			}
		}
	}
	if !gotTask {
		t.Fatalf("subagent_1 未收到任务")
	}
}

// textThenToolCallModel 复现 GLM/Claude 的流式行为: 先吐文本, 再吐 tool_calls。
// eino 默认 firstChunkStreamToolCallChecker 看到首块文本就判定"无工具调用", 于是
// 委派工具永远不执行 (线上症状: 主 Agent 只回一句开场白, 子Agent 从不触发)
type textThenToolCallModel struct {
	toolName string
	args     string
	text     string
	calls    int
}

func (f *textThenToolCallModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.calls++
	if f.calls == 1 {
		return &schema.Message{Role: schema.Assistant, Content: f.text, ToolCalls: []schema.ToolCall{{
			ID:       "call_1",
			Function: schema.FunctionCall{Name: f.toolName, Arguments: f.args},
		}}}, nil
	}
	return &schema.Message{Role: schema.Assistant, Content: "主管总结完成"}, nil
}

func (f *textThenToolCallModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	f.calls++
	if f.calls == 1 {
		// 分片 1: 纯文本开场白 (无 tool_calls); 分片 2: 携带 tool_calls
		return schema.StreamReaderFromArray([]*schema.Message{
			{Role: schema.Assistant, Content: f.text},
			{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
				ID:       "call_1",
				Function: schema.FunctionCall{Name: f.toolName, Arguments: f.args},
			}}},
		}), nil
	}
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.Assistant, Content: "主管总结完成"}}), nil
}

func (f *textThenToolCallModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

// 流式委派回归: 模型"先文本后 tool_calls"时, 委派工具仍必须被执行
func TestOrchestrationSubAgentDelegationStreamingTextFirst(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "你是主管"}},
	    {"id": "sub", "type": "subagent", "name": "调研员", "config": {"system_prompt": "你负责调研", "description": "负责资料调研"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "boss", "target": "sub"},
	    {"source": "boss", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	// 子Agent 的模型用自带回调的假模型 (模拟 ark): 包装器声明 IsCallbacksEnabled 后,
	// 增量/事件都来自模型自身的回调切面, 挂在 "<subID>.model" 下
	sub := &selfCallbackModel{reasoningPieces: []string{"子想"}, pieces: []string{"调研完成"}}
	parent := &textThenToolCallModel{toolName: "subagent_1", args: `{"task":"调研上海"}`, text: "让我用调研方法帮你梳理思路。"}
	builds := 0
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) {
			builds++
			if builds == 1 {
				return parent, nil
			}
			return sub, nil
		},
		buildTool:   func(string) (tool.BaseTool, error) { return &fakeOrchTool{}, nil },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	handler := newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)
	c := &orchestrationCompiler{dsl: dsl, trace: handler}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	// 子Agent 的 ReAct 内部事件必须归到子Agent 自己 (而不是外层主 Agent):
	// 否则前端在被委派期间看不到"子Agent 正在运行", 长任务看起来像卡死
	var subEvents []string
	handler.setEmit(func(event string, payload any) {
		m, ok := payload.(map[string]any)
		if !ok || event != "node" {
			return
		}
		if owner, _ := m["owner"].(string); owner == "sub" {
			subEvents = append(subEvents, fmt.Sprintf("%v %v %v", m["kind"], m["owner"], m["key"]))
		}
	})
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("开始"), compose.WithCallbacks(handler))
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	var out strings.Builder
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("recv err: %v", err)
		}
		out.WriteString(chunk.Content)
	}
	if !strings.Contains(out.String(), "主管总结完成") {
		t.Fatalf("主管未给出最终回复: %q", out.String())
	}
	if len(sub.received) == 0 {
		t.Fatalf("模型先文本后 tool_calls 时委派工具被漏掉 (StreamToolCallChecker 未生效)")
	}
	var delegated *OrchNodeTrace
	for _, tr := range handler.NodeTraces() {
		if tr.Key == "sub" {
			delegated = tr
		}
	}
	if delegated == nil || !delegated.Delegated {
		t.Fatalf("子Agent 委派事件缺失: %+v", handler.NodeTraces())
	}
	if len(subEvents) == 0 {
		t.Fatalf("子Agent 内部 ReAct 事件未回推 (被委派期间界面会一直无反馈)")
	}
	if delegated.Status != "success" {
		t.Fatalf("子Agent 未以 success 结束: %+v (subEvents=%v)", delegated, subEvents)
	}
}

// 反证: 把 StreamToolCallChecker 换回 eino 默认行为 (nil) 时, 同一个用例必须失败,
// 证明"先文本后 tool_calls"场景下默认 checker 确实会漏掉工具调用
func TestOrchestrationDefaultCheckerDropsTextFirstToolCall(t *testing.T) {
	saved := orchStreamToolCallCheckerHook
	orchStreamToolCallCheckerHook = nil // eino 默认 firstChunkStreamToolCallChecker
	defer func() { orchStreamToolCallCheckerHook = saved }()

	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "你是主管"}},
	    {"id": "sub", "type": "subagent", "name": "调研员", "config": {"system_prompt": "你负责调研", "description": "负责资料调研"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "boss", "target": "sub"},
	    {"source": "boss", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	sub := &capturingModel{}
	parent := &textThenToolCallModel{toolName: "subagent_1", args: `{"task":"调研上海"}`, text: "让我用调研方法帮你梳理思路。"}
	builds := 0
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) {
			builds++
			if builds == 1 {
				return parent, nil
			}
			return sub, nil
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
	if len(sub.received) != 0 {
		t.Fatalf("默认 checker 竟然调用了工具? 该用例的前提已变, 请复核")
	}
}

// 主 Agent 挂两个子Agent: 主 Agent 委派任务给其中一个,
// 子Agent 以 user 消息收到任务文本并把结果回传给主 Agent。
// 委派工具名 = "subagent_<序号>", 序号按子Agent 名称(同名再按 id)排序后的位次得出:
// 本用例名称 UTF-8 字典序 "撰写员" < "调研员", 故 sub2=subagent_1, sub1=subagent_2
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
	parent := &scriptedToolCallModel{toolName: "subagent_2", args: `{"task":"调研上海"}`}
	builds := 0
	deps := compilerDeps{
		// 编译期调用顺序: 主 Agent 先构建, 再按序号构建子Agent
		// (subagent_1 = 撰写员/sub2, subagent_2 = 调研员/sub1), 用捕获模型区分任务送达对象
		getModel: func(string) (model.ToolCallingChatModel, error) {
			builds++
			switch builds {
			case 1:
				return parent, nil
			case 2:
				return sub2, nil
			default:
				return sub1, nil
			}
		},
		buildTool:   func(string) (tool.BaseTool, error) { return &fakeOrchTool{}, nil },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	// 事件出口在编译后才确定 (与 DebugRun 一致): 先建 handler 注入委派工具, 再 setEmit
	var events []string
	handler := newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)
	c := &orchestrationCompiler{dsl: dsl, trace: handler}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	if got := c.subAgentIDsOf("boss"); len(got) != 2 || got[0] != "sub2" {
		t.Fatalf("委派工具命名依据 (名称字典序) 变化, subIDs=%v", got)
	}
	if compiled.mode != "chain" {
		t.Fatalf("子Agent 不参与主流, 纯主管链应为 chain 模式, got %s", compiled.mode)
	}
	handler.setEmit(func(event string, payload any) {
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
	// 子Agent 节点不参与主流, 但调试摘要里必须能看到"被委派"的记录 (含所属主 Agent 与任务)
	var delegated *OrchNodeTrace
	for _, tr := range handler.NodeTraces() {
		if tr.Key == "sub1" {
			delegated = tr
		}
	}
	if delegated == nil {
		t.Fatalf("期望子Agent 节点出现在调试摘要中, got %+v", handler.NodeTraces())
	}
	if !delegated.Delegated || delegated.Owner != "boss" || !strings.Contains(delegated.Task, "调研上海") {
		t.Fatalf("子Agent 委派摘要异常: %+v", delegated)
	}
	if delegated.Status != "success" || delegated.Content == "" {
		t.Fatalf("子Agent 委派结果未回填: %+v", delegated)
	}
}

// 子Agent 委派边不能影响主流形态判定:
// 主 Agent 同时是分支目标/合并来源时, 委派边既不该让它被判成"有主流入边",
// 也不该让它被判成"有主流出边"(否则 START/END 漏接或 Workflow 入口丢失)
func TestOrchestrationSubAgentWithBranchAndMerge(t *testing.T) {
	// 分支图: 主 Agent 挂在分支后的目标上, 同时委派子Agent
	branchDSL := `{
	  "version": 1,
	  "nodes": [
	    {"id": "tpl", "type": "template", "name": "入口", "config": {"template": "choose A: {{.Input}}"}},
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "你是主管", "tools": ["orch_echo"]}},
	    {"id": "sub", "type": "subagent", "name": "助手", "config": {"system_prompt": "协助"}},
	    {"id": "br", "type": "branch", "name": "路由", "config": {"cases": [{"type": "contains", "value": "A", "target": "boss"}], "default_target": "boss"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "tpl", "target": "br"},
	    {"source": "br", "target": "boss", "label": "A"},
	    {"source": "boss", "target": "sub"},
	    {"source": "boss", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(branchDSL)
	if len(errs) > 0 {
		t.Fatalf("分支+子Agent 校验失败: %v", errs)
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), orchTestDeps())
	if err != nil {
		t.Fatalf("分支+子Agent 编译失败: %v", err)
	}
	if compiled.mode != "graph" {
		t.Fatalf("mode = %s, want graph", compiled.mode)
	}
	// 主管只有"到 sub(委派)+到 out(主流)"两条出边, 出口必须仍接到 END
	if c.flowOutOf("boss") != 1 {
		t.Fatalf("主管主流出度应为 1, got %d", c.flowOutOf("boss"))
	}
	msg, err := compiled.runnable.Invoke(context.Background(), schema.UserMessage("A"))
	if err != nil {
		t.Fatalf("分支+子Agent 运行失败: %v", err)
	}
	if !strings.Contains(msg.Content, "agent final answer") {
		t.Fatalf("输出异常: %q", msg.Content)
	}

	// 合并工作流: 主 Agent 作为合并来源之一, 同时委派子Agent
	mergeDSL := `{
	  "version": 1,
	  "nodes": [
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "左", "tools": ["orch_echo"]}},
	    {"id": "sub", "type": "subagent", "name": "助手", "config": {"system_prompt": "协助"}},
	    {"id": "tpl", "type": "template", "name": "右", "config": {"template": "{{.Input}}"}},
	    {"id": "merge", "type": "merge", "name": "合并", "config": {"separator": " | "}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "boss", "target": "sub"},
	    {"source": "boss", "target": "merge"},
	    {"source": "tpl", "target": "merge"},
	    {"source": "merge", "target": "out"}
	  ]
	}`
	dsl2, errs2 := validateOrchestrationDSL(mergeDSL)
	if len(errs2) > 0 {
		t.Fatalf("合并+子Agent 校验失败: %v", errs2)
	}
	c2 := &orchestrationCompiler{dsl: dsl2}
	compiled2, err := c2.compile(context.Background(), orchTestDeps())
	if err != nil {
		t.Fatalf("合并+子Agent 编译失败: %v", err)
	}
	if compiled2.mode != "workflow" {
		t.Fatalf("mode = %s, want workflow", compiled2.mode)
	}
	if c2.flowOutOf("boss") != 1 || c2.flowOutOf("tpl") != 1 {
		t.Fatalf("合并来源主流出度应为 1: boss=%d tpl=%d", c2.flowOutOf("boss"), c2.flowOutOf("tpl"))
	}
	out, err := compiled2.runnable.Invoke(context.Background(), schema.UserMessage("开始"))
	if err != nil {
		t.Fatalf("合并+子Agent 运行失败: %v", err)
	}
	if !strings.Contains(out.Content, "agent final answer") {
		t.Fatalf("合并输出异常: %q", out.Content)
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
