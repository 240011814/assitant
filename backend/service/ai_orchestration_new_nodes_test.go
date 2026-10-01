package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// ---------- LLM 路由节点 ----------

// routerLabelModel 分类模型桩: 固定返回一个标签
type routerLabelModel struct{ label string }

func (f *routerLabelModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: f.label}, nil
}

func (f *routerLabelModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := f.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (f *routerLabelModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

const orchRouterDSL = `{
  "version": 1,
  "nodes": [
    {"id": "in", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
    {"id": "rt", "type": "router", "name": "意图路由", "config": {"instructions": "按用户意图分类", "cases": [{"label": "天气", "description": "问天气", "target": "tplA"}, {"label": "时间", "description": "问时间", "target": "tplB"}], "default_target": "tplB"}},
    {"id": "tplA", "type": "template", "name": "A路径", "config": {"template": "路径A: {{.Input}}"}},
    {"id": "tplB", "type": "template", "name": "B路径", "config": {"template": "路径B: {{.Input}}"}},
    {"id": "out", "type": "end", "name": "输出", "config": {}}
  ],
  "edges": [
    {"source": "in", "target": "rt"},
    {"source": "rt", "target": "tplA"},
    {"source": "rt", "target": "tplB"},
    {"source": "tplA", "target": "out"},
    {"source": "tplB", "target": "out"}
  ]
}`

func orchRouterDeps(label string) compilerDeps {
	deps := orchTestDeps()
	deps.getModel = func(string) (model.ToolCallingChatModel, error) { return &routerLabelModel{label: label}, nil }
	return deps
}

func TestOrchestrationRouterRun(t *testing.T) {
	cases := []struct {
		label string
		want  string
	}{
		{"天气", "路径A"},       // 精确命中
		{"  时间  ", "路径B"},   // 带空白精确命中
		{"我判断这是天气类", "路径A"}, // 包含匹配
		{"无法分类", "路径B"},     // 未命中 -> 默认目标
	}
	for _, tc := range cases {
		mode, output, err := runOrchDSLWithDeps(t, orchRouterDSL, "今天怎么样", orchRouterDeps(tc.label))
		if err != nil {
			t.Fatalf("label=%q run err: %v", tc.label, err)
		}
		if mode != "graph" {
			t.Fatalf("label=%q mode = %s, want graph", tc.label, mode)
		}
		if !strings.Contains(output, tc.want) {
			t.Fatalf("label=%q output = %q, want contains %q", tc.label, output, tc.want)
		}
	}
}

func TestOrchRouterMatchLabel(t *testing.T) {
	cases := []OrchRouterCase{
		{Label: "售前", Target: "a"},
		{Label: "售后", Target: "b"},
	}
	if target, ok := orchRouterMatchLabel("售前", cases); !ok || target != "a" {
		t.Fatalf("精确匹配失败: %q %v", target, ok)
	}
	if target, ok := orchRouterMatchLabel(`"售后"`, cases); !ok || target != "b" {
		t.Fatalf("带引号匹配失败: %q %v", target, ok)
	}
	if _, ok := orchRouterMatchLabel("其他", cases); ok {
		t.Fatal("不相关输出不应命中")
	}
	if _, ok := orchRouterMatchLabel("", cases); ok {
		t.Fatal("空输出不应命中")
	}
}

func TestOrchestrationRouterValidation(t *testing.T) {
	cases := []struct {
		name string
		dsl  string
		want string
	}{
		{"连线与分类不一致", `{"nodes":[{"id":"rt","type":"router","name":"rt","config":{"cases":[{"label":"A","target":"o"}],"default_target":"o"}},{"id":"o","type":"end","name":"o","config":{}},{"id":"x","type":"end","name":"x","config":{}}],"edges":[{"source":"rt","target":"o"},{"source":"rt","target":"x"}]}`, "不一致"},
		{"缺默认目标", `{"nodes":[{"id":"rt","type":"router","name":"rt","config":{"cases":[{"label":"A","target":"o"}]}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"rt","target":"o"}]}`, "默认目标"},
		{"缺标签", `{"nodes":[{"id":"rt","type":"router","name":"rt","config":{"cases":[{"description":"x","target":"o"}],"default_target":"o"}},{"id":"o","type":"end","name":"o","config":{}}],"edges":[{"source":"rt","target":"o"}]}`, "缺少标签"},
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

// ---------- 字段提取节点 ----------

const orchExtractDSL = `{
  "version": 1,
  "nodes": [
    {"id": "ex", "type": "extract", "name": "提取", "config": {"field": "%s", "fallback": "%s"}},
    {"id": "out", "type": "end", "name": "输出", "config": {}}
  ],
  "edges": [{"source": "ex", "target": "out"}]
}`

func TestOrchestrationExtractRun(t *testing.T) {
	cases := []struct {
		name     string
		field    string
		fallback string
		input    string
		want     string
	}{
		{"点号路径", "data.items.0.name", "", `{"data":{"items":[{"name":"你好"}]}}`, "你好"},
		{"剥代码围栏", "a", "", "```json\n{\"a\": \"X\"}\n```", "X"},
		{"对象值JSON编码", "data", "", `{"data":{"k":1}}`, `{"k":1}`},
		{"未命中走兜底", "missing", "兜底内容", `{"a":1}`, "兜底内容"},
		{"未命中无兜底透传", "missing", "", `{"a":1}`, `{"a":1}`},
		{"非JSON透传", "a", "", "纯文本内容", "纯文本内容"},
	}
	for _, tc := range cases {
		dsl := fmt.Sprintf(orchExtractDSL, tc.field, tc.fallback)
		mode, output, err := runOrchDSL(t, dsl, tc.input)
		if err != nil {
			t.Fatalf("%s run err: %v", tc.name, err)
		}
		if mode != "chain" {
			t.Fatalf("%s mode = %s, want chain", tc.name, mode)
		}
		if strings.TrimSpace(output) != tc.want {
			t.Fatalf("%s output = %q, want %q", tc.name, output, tc.want)
		}
	}
}

// ---------- 子编排节点 ----------

const orchInnerDSL = `{
  "version": 1,
  "nodes": [
    {"id": "tpl", "type": "template", "name": "内部模板", "config": {"template": "子编排收到: {{.Input}}"}},
    {"id": "out", "type": "end", "name": "输出", "config": {}}
  ],
  "edges": [{"source": "tpl", "target": "out"}]
}`

func orchSubOrchDeps() compilerDeps {
	deps := orchTestDeps()
	deps.compileNested = func(ctx context.Context, refID uint, chain []uint, keyPrefix string) (*compiledOrchestration, error) {
		if refID != 7 {
			return nil, fmt.Errorf("意外引用 %d", refID)
		}
		dsl, errs := validateOrchestrationDSL(orchInnerDSL)
		if len(errs) > 0 {
			return nil, errors.New(strings.Join(errs, "; "))
		}
		sub := &orchestrationCompiler{dsl: prefixOrchestrationDSL(dsl, keyPrefix)}
		return sub.compile(ctx, orchTestDeps())
	}
	return deps
}

func TestOrchestrationSubOrchRun(t *testing.T) {
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "pre", "type": "template", "name": "前置", "config": {"template": "前置"}},
	    {"id": "sub", "type": "suborch", "name": "嵌套", "config": {"orchestration_id": 7}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "pre", "target": "sub"},
	    {"source": "sub", "target": "out"}
	  ]
	}`
	mode, output, err := runOrchDSLWithDeps(t, definition, "hello", orchSubOrchDeps())
	if err != nil {
		t.Fatalf("suborch run err: %v", err)
	}
	if mode != "chain" {
		t.Fatalf("mode = %s, want chain", mode)
	}
	if !strings.Contains(output, "子编排收到: 前置") {
		t.Fatalf("output = %q, want contains 子编排收到: 前置", output)
	}

	// 嵌套编排的前缀化节点 key 应并入编译结果的归属表, 调试摘要才能独立成行
	dsl := mustParseOrchestrationDSL(t, definition)
	c := &orchestrationCompiler{dsl: dsl}
	compiled, err := c.compile(context.Background(), orchSubOrchDeps())
	if err != nil {
		t.Fatalf("compile err: %v", err)
	}
	if _, ok := compiled.nodeKeys["so_sub_tpl"]; !ok {
		t.Fatalf("嵌套节点 key 未并入归属表: %v", compiled.nodeKeys)
	}
}

func TestOrchestrationSubOrchCycleAndDepth(t *testing.T) {
	definition := `{"nodes":[{"id":"sub","type":"suborch","name":"嵌套","config":{"orchestration_id":7}},{"id":"out","type":"end","name":"输出","config":{}}],"edges":[{"source":"sub","target":"out"}]}`

	// 循环引用: 编译链上已有编排 7, 再引用 7 应直接报错且不触发查库
	cycleCalled := false
	deps := orchTestDeps()
	deps.compileNested = func(context.Context, uint, []uint, string) (*compiledOrchestration, error) {
		cycleCalled = true
		return nil, errors.New("不应被调用")
	}
	c := &orchestrationCompiler{dsl: mustParseOrchestrationDSL(t, definition), orchChain: []uint{7}}
	if _, err := c.compile(context.Background(), deps); err == nil {
		t.Fatal("循环引用期望编译失败")
	} else if !strings.Contains(err.Error(), "循环引用") {
		t.Fatalf("错误 %v 不含 循环引用", err)
	} else if cycleCalled {
		t.Fatal("循环引用不应触发查库编译")
	}

	// 超过最大嵌套层数
	c = &orchestrationCompiler{dsl: mustParseOrchestrationDSL(t, definition), orchChain: []uint{1, 2, 3, 4, 5}}
	if _, err := c.compile(context.Background(), deps); err == nil {
		t.Fatal("超深层级期望编译失败")
	} else if !strings.Contains(err.Error(), "嵌套层级") {
		t.Fatalf("错误 %v 不含 嵌套层级", err)
	}
}

func TestPrefixOrchestrationDSL(t *testing.T) {
	dsl := mustParseOrchestrationDSL(t, `{
	  "version": 1,
	  "nodes": [
	    {"id": "in", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "br", "type": "branch", "name": "分支", "config": {"cases": [{"type": "contains", "value": "a", "target": "x"}], "default_target": "y"}},
	    {"id": "x", "type": "agent", "name": "X", "config": {"system_prompt": "x"}},
	    {"id": "y", "type": "agent", "name": "Y", "config": {"system_prompt": "y"}},
	    {"id": "out", "type": "end", "name": "O", "config": {}}
	  ],
	  "edges": [
	    {"source": "in", "target": "br"},
	    {"source": "br", "target": "x"},
	    {"source": "br", "target": "y"},
	    {"source": "x", "target": "out"},
	    {"source": "y", "target": "out"}
	  ]
	}`)
	prefixed := prefixOrchestrationDSL(dsl, "so_n1_")
	if len(prefixed.Nodes) != 5 {
		t.Fatalf("节点数 = %d, want 5", len(prefixed.Nodes))
	}
	ids := map[string]string{}
	for _, n := range prefixed.Nodes {
		ids[n.ID] = n.Type
		if !strings.HasPrefix(n.ID, "so_n1_") {
			t.Fatalf("节点 id 未加前缀: %s", n.ID)
		}
		if n.Name == n.ID {
			t.Fatalf("显示名不应被改写: %s", n.Name)
		}
	}
	var branchCfg OrchBranchConfig
	for _, n := range prefixed.Nodes {
		if n.Type == "branch" {
			if err := json.Unmarshal(n.Config, &branchCfg); err != nil {
				t.Fatalf("分支配置解析失败: %v", err)
			}
		}
	}
	if branchCfg.Cases[0].Target != "so_n1_x" || branchCfg.DefaultTarget != "so_n1_y" {
		t.Fatalf("分支目标未重写: %+v", branchCfg)
	}
	for _, e := range prefixed.Edges {
		if !strings.HasPrefix(e.Source, "so_n1_") || !strings.HasPrefix(e.Target, "so_n1_") {
			t.Fatalf("连线未重写: %+v", e)
		}
	}
}

func mustParseOrchestrationDSL(t *testing.T, definition string) *OrchestrationDSL {
	t.Helper()
	dsl, errs := validateOrchestrationDSL(definition)
	if len(errs) > 0 {
		t.Fatalf("DSL 校验失败: %v", errs)
	}
	return dsl
}
