package service

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// runMini 编译并运行一段 DSL, 返回图级输出
func runMini(t *testing.T, definition, input string, m *stickyTestModel) string {
	t.Helper()
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	deps := compilerDeps{
		getModel:    func(string) (model.ToolCallingChatModel, error) { return m, nil },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	c := &orchestrationCompiler{
		dsl: dsl, userID: 0,
		stickyDecisions: map[string][]string{}, stickyDefaults: map[string]string{},
		runVars:             map[string]string{},
		stateArtifacts:      map[string]string{},
		parallelSkipTargets: map[string]bool{},
	}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage(input))
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	defer stream.Close()
	var sb strings.Builder
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("recv err: %v", err)
		}
		sb.WriteString(chunk.Content)
	}
	return sb.String()
}

// json 条件: 输入 JSON 按 path 取值精确比较, 取不到值走默认目标
func TestOrchestrationBranchJSONCase(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "in", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "route", "type": "branch", "name": "路由",
	     "config": {"mode": "route", "cases": [{"type": "json", "path": "type", "value": "social", "target": "hit"}], "default_target": "miss"}},
	    {"id": "hit", "type": "template", "name": "命中", "config": {"template": "HIT"}},
	    {"id": "miss", "type": "template", "name": "未中", "config": {"template": "MISS"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "in", "target": "route"},
	    {"source": "route", "target": "hit"},
	    {"source": "route", "target": "miss"},
	    {"source": "hit", "target": "out"},
	    {"source": "miss", "target": "out"}
	  ]
	}`
	m := &stickyTestModel{fixed: ""}
	if got := runMini(t, definition, `{"type":"social","n":1}`, m); got != "HIT" {
		t.Fatalf("json 条件应命中 HIT, got=%q", got)
	}
	if got := runMini(t, definition, `{"type":"other"}`, m); got != "MISS" {
		t.Fatalf("不匹配应走默认 MISS, got=%q", got)
	}
	if got := runMini(t, definition, `not-json`, m); got != "MISS" {
		t.Fatalf("非法 JSON 应走默认 MISS, got=%q", got)
	}
}

// 运行变量: 上游 output_var 捕获的输出, 下游模板经 {{vars.*}} 引用
func TestOrchestrationOutputVarFlow(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "producer", "type": "template", "name": "产出", "output_var": "greeting", "config": {"template": "你好,{{.Input}}"}},
	    {"id": "consumer", "type": "template", "name": "消费", "config": {"template": "变量={{.vars.greeting}} 输入={{.Input}}"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "producer", "target": "consumer"},
	    {"source": "consumer", "target": "out"}
	  ]
	}`
	m := &stickyTestModel{fixed: ""}
	got := runMini(t, definition, "世界", m)
	if !strings.Contains(got, "变量=你好,世界") {
		t.Fatalf("下游模板应读到 output_var, got=%q", got)
	}
}

// 会话存档: save_state 把节点输出捕获进 stateArtifacts (跑后随会话落库)
func TestOrchestrationSaveStateCapture(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "scene", "type": "template", "name": "场景", "save_state": "scene", "config": {"template": "场景卡"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "scene", "target": "out"}
	  ]
	}`
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("validate err: %v", errs)
	}
	deps := compilerDeps{
		getModel:    func(string) (model.ToolCallingChatModel, error) { return &stickyTestModel{}, nil },
		sessionVars: func() map[string]any { return map[string]any{} },
	}
	c := &orchestrationCompiler{
		dsl: dsl, userID: 0,
		stickyDecisions: map[string][]string{}, stickyDefaults: map[string]string{},
		runVars:             map[string]string{},
		stateArtifacts:      map[string]string{},
		parallelSkipTargets: map[string]bool{},
	}
	compiled, err := c.compile(context.Background(), deps)
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("x"))
	if err != nil {
		t.Fatalf("stream err: %v", err)
	}
	for {
		if _, err := stream.Recv(); err == io.EOF {
			break
		} else if err != nil {
			t.Fatalf("recv err: %v", err)
		}
	}
	if compiled.stateArtifacts["scene"] != "场景卡" {
		t.Fatalf("save_state 应捕获场景卡, got=%q", compiled.stateArtifacts["scene"])
	}
}

// foreach: 输入数组逐项渲染提示词并调用模型, 结果按 join 拼接; max_items 截断
func TestOrchestrationForeachNode(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "fe", "type": "foreach", "name": "迭代",
	     "config": {"items_path": "items", "item_template": "第{{.index}}项: {{.item}}", "max_items": 2, "join": " | "}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "fe", "target": "out"}
	  ]
	}`
	m := &stickyTestModel{fixed: "OK"}
	got := runMini(t, definition, `{"items":["甲","乙","丙"]}`, m)
	// 输出是逐项模型结果的拼接 (join)
	if strings.Count(got, "OK") != 2 {
		t.Fatalf("应调用模型 2 次, got=%q", got)
	}
	// max_items=2 截断第 3 项: 输出只有两段
	if strings.Count(got, "|") != 1 {
		t.Fatalf("截断后应只有 2 段 1 个分隔符, got=%q", got)
	}
	// 模型收到的最后一轮输入应含第 1 项的渲染结果 (item/index 模板生效)
	if m.lastInputs == nil {
		t.Fatalf("foreach 应调用模型")
	}
	lastUser := ""
	for _, msg := range m.lastInputs {
		if msg.Role == schema.User {
			lastUser = msg.Content
		}
	}
	if !strings.Contains(lastUser, `第1项: "乙"`) {
		t.Fatalf("item_template 应渲染进模型输入, got=%q", lastUser)
	}
}
