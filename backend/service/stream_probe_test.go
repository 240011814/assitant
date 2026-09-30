package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	coremodel "backend/model"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	einoagent "github.com/cloudwego/eino/flow/agent"
	"github.com/cloudwego/eino/schema"
)

// timedModel 流式回复固定文本, 每片间隔固定时长, 用于观测"编排流是否逐片流出"
type timedModel struct {
	pieces []string
	// reasoningPieces 先于正文流式输出的思考内容 (GLM/DeepSeek 的 reasoning 增量)
	reasoningPieces []string
	delay           time.Duration
}

func (f *timedModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: strings.Join(f.pieces, "")}, nil
}

func (f *timedModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](len(f.pieces) + len(f.reasoningPieces) + 1)
	go func() {
		defer sw.Close()
		for _, p := range f.reasoningPieces {
			time.Sleep(f.delay)
			sw.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: p}, nil)
		}
		for _, p := range f.pieces {
			time.Sleep(f.delay)
			sw.Send(&schema.Message{Role: schema.Assistant, Content: p}, nil)
		}
	}()
	return sr, nil
}

func (f *timedModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) { return f, nil }

// 观测: 主 Agent 的回答应当"边生成边流出", 而不是等整段生成完才一次性到达
func TestOrchestrationStreamsAnswerIncrementally(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "tpl", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "你是主管"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "tpl", "target": "boss"},
	    {"source": "boss", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) {
			return &timedModel{
				reasoningPieces: []string{"先想一下…", "再权衡一下…"},
				pieces:          []string{"第一", "第二", "第三"},
				delay:           300 * time.Millisecond,
			}, nil
		},
		buildTool:   func(string) (tool.BaseTool, error) { return nil, errors.New("no tool") },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	start := time.Now()
	// DebugRun 会做同样的事: handler 放进 ctx + 注册回调
	handler := newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)
	var deltas, reasonings []string
	var text strings.Builder
	handler.setEmit(func(event string, payload any) {
		m, ok := payload.(map[string]any)
		if !ok {
			return
		}
		switch event {
		case "delta":
			deltas = append(deltas, fmt.Sprintf("%q@%dms", m["content"], time.Since(start).Milliseconds()))
			text.WriteString(probeStr(m["content"]))
		case "reasoning":
			reasonings = append(reasonings, fmt.Sprintf("%q@%dms", m["content"], time.Since(start).Milliseconds()))
		}
	})
	runCtx := withOrchHandler(context.Background(), handler)
	stream, err := compiled.runnable.Stream(runCtx, schema.UserMessage("开始"), compose.WithCallbacks(handler))
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
	t.Logf("实时正文增量: %v", deltas)
	t.Logf("实时思考增量: %v", reasonings)
	if len(deltas) < 3 {
		t.Fatalf("正文没有实时推送(仍被整体缓冲): %v", deltas)
	}
	if len(reasonings) < 2 {
		t.Fatalf("思考内容没有实时推送: %v", reasonings)
	}
	// 去重: 图级输出到达后不能再重复下发已推过的正文
	if got := text.String(); got != "第一第二第三" {
		t.Fatalf("正文重复下发或缺失, 累计=%q", got)
	}
}

// 多轮对话回归: 历史消息要拼在本轮输入之前 (每轮按"历史+本轮"重建, 不能重复累加)
func TestOrchestrationChatHistoryInjected(t *testing.T) {
	history := OrchChatTurnsToMessages([]coremodel.ChatTurn{
		{Role: "user", Content: "我叫老王"},
		{Role: "assistant", Content: "记住了, 老王"},
		{Role: "system", Content: "这条应被忽略"},
		{Role: "user", Content: "   "}, // 空内容忽略
	})
	if len(history) != 2 {
		t.Fatalf("历史应只保留 user/assistant 两条, got %d", len(history))
	}
	if history[0].Role != schema.User || history[0].Content != "我叫老王" {
		t.Fatalf("历史首条异常: %+v", history[0])
	}
	if history[1].Role != schema.Assistant || history[1].Content != "记住了, 老王" {
		t.Fatalf("历史次条异常: %+v", history[1])
	}

	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "tpl", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "你是主管"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "tpl", "target": "boss"},
	    {"source": "boss", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	captured := &capturingModel{}
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) { return captured, nil },
		buildTool: func(string) (tool.BaseTool, error) {
			return nil, errors.New("no tool")
		},
		sessionVars: func() map[string]any {
			return map[string]any{orchSessionVarsHistoryKey: history}
		},
	}
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("我叫什么"))
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
	if len(captured.received) == 0 {
		t.Fatalf("模型未被调用")
	}
	msgs := captured.received[0]
	if len(msgs) < 4 {
		t.Fatalf("消息序列过短, 历史未拼入: %d", len(msgs))
	}
	// 期望: [system, user(老王), assistant(记住了), user(本轮)]
	if msgs[0].Role != schema.System {
		t.Fatalf("首条应为 system, got %s", msgs[0].Role)
	}
	if msgs[1].Role != schema.User || msgs[1].Content != "我叫老王" {
		t.Fatalf("第 2 条应为历史 user, got %s %q", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Role != schema.Assistant || msgs[2].Content != "记住了, 老王" {
		t.Fatalf("第 3 条应为历史 assistant, got %s %q", msgs[2].Role, msgs[2].Content)
	}
	last := msgs[len(msgs)-1]
	if last.Role != schema.User || last.Content != "我叫什么" {
		t.Fatalf("末条应为本轮输入, got %s %q", last.Role, last.Content)
	}
}

// 子Agent 增量回归: 子Agent 的模型被 orchSubAgentProgress 包装(实时透出 + 心跳),
// 同时又会被图回调 OnEndWithStreamOutput 看到 (comp=ChatModel)。两条路径若都 emit,
// 前端同一段思考/正文会被交错追加两次 (线上症状: "用户用户现在现在…" 的叠字)。
// 期望: 思考与正文各恰好下发一次。
func TestOrchestrationSubAgentDeltasNotDuplicated(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "boss", "type": "agent", "name": "主管", "config": {"system_prompt": "你是主管"}},
	    {"id": "sub", "type": "subagent", "name": "子Agent", "config": {"system_prompt": "你是子Agent"}},
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
	trace := newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)
	mm := &timedModel{
		reasoningPieces: []string{"子想一", "子想二"},
		pieces:          []string{"子答一", "子答二"},
		delay:           10 * time.Millisecond,
	}
	deps := compilerDeps{
		getModel:    func(string) (model.ToolCallingChatModel, error) { return mm, nil },
		buildTool:   func(string) (tool.BaseTool, error) { return nil, errors.New("no tool") },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	c := &orchestrationCompiler{dsl: dsl, deps: deps, trace: trace}
	c.nodeMap = map[string]*OrchestrationNode{}
	for i := range dsl.Nodes {
		c.nodeMap[dsl.Nodes[i].ID] = &dsl.Nodes[i]
	}
	sub, err := c.buildSubReactAgent(context.Background(), "sub", "boss", OrchSubAgentConfig{SystemPrompt: "你是子Agent"})
	if err != nil {
		t.Fatalf("build subagent err: %v", err)
	}
	var reasons, deltas []string
	trace.setEmit(func(event string, payload any) {
		m, _ := payload.(map[string]any)
		switch event {
		case "reasoning":
			reasons = append(reasons, probeStr(m["content"]))
		case "delta":
			deltas = append(deltas, probeStr(m["content"]))
		}
	})
	runCtx := withOrchHandler(context.Background(), trace)
	stream, err := sub.Stream(runCtx, []*schema.Message{schema.UserMessage("干活")},
		einoagent.WithComposeOptions(compose.WithCallbacks(trace)))
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
	t.Logf("子Agent 思考增量: %v", reasons)
	t.Logf("子Agent 正文增量: %v", deltas)
	if len(reasons) != 2 {
		t.Fatalf("子Agent 思考应恰好 2 片, 实际 %d: %v", len(reasons), reasons)
	}
	if got := strings.Join(deltas, ""); got != "子答一子答二" {
		t.Fatalf("子Agent 正文重复或缺失, 累计=%q", got)
	}
}

// 复刻线上编排形态 (模板→主Agent(带子Agent)→结束, 主Agent 真的委派子Agent) 统计事件:
// 定位"整段回复重复 3 遍"到底来自哪条路径。
func TestOrchestrationDelegationEmissionCount(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "tpl", "type": "template", "name": "模板", "config": {"template": "请处理以下内容\n{{.Input}}"}},
	    {"id": "boss", "type": "agent", "name": "Agent", "config": {"system_prompt": "你是我的助手"}},
	    {"id": "sub", "type": "subagent", "name": "子Agent", "config": {"system_prompt": "你是搜索助手", "description": "支持搜索"}},
	    {"id": "out", "type": "end", "name": "结束", "config": {}}
	  ],
	  "edges": [
	    {"source": "tpl", "target": "boss"},
	    {"source": "boss", "target": "out"},
	    {"source": "boss", "target": "sub"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	mainModel := &scriptedToolModel{toolName: "subagent_1", args: `{"task":"查天气"}`, finalText: "最终答案"}
	subModel := &timedModel{reasoningPieces: []string{"子想"}, pieces: []string{"天气问候语"}, delay: 0}
	getN := 0
	deps := compilerDeps{
		getModel: func(string) (model.ToolCallingChatModel, error) {
			getN++
			if getN == 1 {
				return mainModel, nil
			}
			return subModel, nil
		},
		buildTool:   func(string) (tool.BaseTool, error) { return nil, errors.New("no tool") },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	trace := newOrchTraceHandler(orchNodeKeysOf(dsl), orchSubAgentKeysOf(dsl), nil)
	var reasonings, deltas []string
	trace.setEmit(func(event string, payload any) {
		m, _ := payload.(map[string]any)
		switch event {
		case "reasoning":
			reasonings = append(reasonings, probeStr(m["content"]))
		case "delta":
			deltas = append(deltas, probeStr(m["content"]))
		}
	})
	c := &orchestrationCompiler{dsl: dsl, trace: trace}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	runCtx := withOrchHandler(context.Background(), trace)
	stream, err := compiled.runnable.Stream(runCtx, schema.UserMessage("帮我查天气"), compose.WithCallbacks(trace))
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	var graphOut strings.Builder
	var loopDeltas []string
	for {
		chunk, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("recv err: %v", err)
		}
		if chunk == nil || chunk.Content == "" {
			continue
		}
		graphOut.WriteString(chunk.Content)
		// 复刻 DebugRun 的图级去重
		if rest := orchSkipStreamed(trace, chunk.Content); rest != "" {
			loopDeltas = append(loopDeltas, rest)
		}
	}
	t.Logf("reasoning 事件: %v", reasonings)
	t.Logf("回调 delta 事件: %v", deltas)
	t.Logf("图级补发 delta: %v", loopDeltas)
	t.Logf("图级输出: %q", graphOut.String())

	all := strings.Join(append(append([]string{}, deltas...), loopDeltas...), "")
	if len(reasonings) != 1 {
		t.Fatalf("子Agent 思考应恰好 1 条, 实际 %d: %v", len(reasonings), reasonings)
	}
	if n := strings.Count(all, "天气问候语"); n != 1 {
		t.Fatalf("子Agent 正文应恰好出现 1 次, 实际 %d 次 (delta=%v)", n, deltas)
	}
	if n := strings.Count(all, "最终答案"); n != 1 {
		t.Fatalf("主 Agent 最终答案应恰好出现 1 次, 实际 %d 次 (delta=%v 补发=%v)", n, deltas, loopDeltas)
	}
}

// scriptedToolModel 第一次 Stream 返回工具调用, 之后返回最终文本
type scriptedToolModel struct {
	calls     int
	toolName  string
	args      string
	finalText string
}

func (m *scriptedToolModel) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	sr, err := m.Stream(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	return schema.ConcatMessageStream(sr)
}

func (m *scriptedToolModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	m.calls++
	if m.calls == 1 {
		msg := &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
			ID:       "call_1",
			Function: schema.FunctionCall{Name: m.toolName, Arguments: m.args},
		}}}
		return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
	}
	runes := []rune(m.finalText)
	chunks := make([]*schema.Message, 0, len(runes))
	for _, r := range runes {
		chunks = append(chunks, &schema.Message{Role: schema.Assistant, Content: string(r)})
	}
	return schema.StreamReaderFromArray(chunks), nil
}

func (m *scriptedToolModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return m, nil
}

func probeStr(v any) string {
	s, _ := v.(string)
	return s
}

// Agent 类型规范化: 只认 subagent, 其余一律 chat (空值/拼错都退回 chat, 避免脏数据)
func TestNormalizeAgentType(t *testing.T) {
	cases := map[string]string{
		"":            coremodel.AIAgentTypeChat,
		"chat":        coremodel.AIAgentTypeChat,
		"subagent":    coremodel.AIAgentTypeSubAgent,
		" subagent ":  coremodel.AIAgentTypeSubAgent,
		"subAgent":    coremodel.AIAgentTypeChat,
		"orchestrate": coremodel.AIAgentTypeChat,
	}
	for in, want := range cases {
		if got := normalizeAgentType(in); got != want {
			t.Fatalf("normalizeAgentType(%q) = %q, want %q", in, got, want)
		}
	}
}

// 去重回归: 链式多个模型节点各自实时下发后, 图级输出不能把前一个节点的长度算进来,
// 否则第二个节点的最终文本会被整段跳过 (回答凭空消失)
func TestOrchestrationStreamDedupeAcrossModelNodes(t *testing.T) {
	handler := newOrchTraceHandler(map[string]string{"a1": "一", "a2": "二"}, nil, nil)
	handler.setEmit(func(string, any) {})

	// 节点1 流式下发 "AAAA", 节点2 流式下发 "BBBB" (每次模型调用重新计数)
	handler.markStreamed(4, true)
	if got := orchSkipStreamed(handler, "AAAA"); got != "" {
		t.Fatalf("节点1 图级输出应被完全去重, got %q", got)
	}
	handler.markStreamed(4, true)
	// 节点2 的图级输出是 "BBBB": 不能被节点1 的计数吃掉
	if got := orchSkipStreamed(handler, "BBBB"); got != "" {
		t.Fatalf("节点2 图级输出应被去重, got %q", got)
	}
	// 再来一轮: 计数已耗尽, 新内容必须完整下发
	if got := orchSkipStreamed(handler, "CCCC"); got != "CCCC" {
		t.Fatalf("计数耗尽后应完整下发, got %q", got)
	}
	// 部分重叠: 已下发 2 字节, 图级输出 4 字节 -> 只补后 2 字节
	handler.markStreamed(2, true)
	if got := orchSkipStreamed(handler, "DDDD"); got != "DD" {
		t.Fatalf("应只补推未下发部分, got %q", got)
	}
}
