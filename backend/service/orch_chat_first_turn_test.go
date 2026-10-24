package service

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	coremodel "backend/model"
)

// stickyTestModel 输出固定文本并统计调用次数: 检验首跑节点是否被跳过 (次数为 0)
type stickyTestModel struct {
	fixed    string
	received int
}

func (f *stickyTestModel) Generate(_ context.Context, input []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	f.received++
	return &schema.Message{Role: schema.Assistant, Content: f.fixed}, nil
}

func (f *stickyTestModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	msg, err := f.Generate(ctx, input, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (f *stickyTestModel) WithTools([]*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	return f, nil
}

// 编排对话 chat_first_turn 机制回归:
// 1) 会话未激活时首跑节点正常执行, 粘性分支的选择被记录且命中非默认目标时判定激活
// 2) 会话激活后, chat_first_turn 节点编译为直通 (不再调用模型), 粘性分支回放记录的路由
func TestOrchestrationChatFirstTurnLifecycle(t *testing.T) {
	// tpl -> parser(agent) -> route(branch, 首跑命中 social 而非默认) -> social/clarify(agent) -> end
	definition := `{
	  "version": 1,
	  "nodes": [
	    {"id": "tpl", "type": "template", "name": "入口", "config": {"template": "{{.Input}}"}},
	    {"id": "parser", "type": "agent", "name": "解析器", "chat_first_turn": true, "config": {"system_prompt": "解析"}},
	    {"id": "route", "type": "branch", "name": "路由", "chat_first_turn": true,
	     "config": {"mode": "route", "cases": [{"type": "contains", "value": "social", "target": "social"}], "default_target": "clarify"}},
	    {"id": "social", "type": "agent", "name": "社交训练", "config": {"system_prompt": "训练"}},
	    {"id": "clarify", "type": "agent", "name": "澄清", "config": {"system_prompt": "澄清"}},
	    {"id": "out", "type": "end", "name": "输出", "config": {}}
	  ],
	  "edges": [
	    {"source": "tpl", "target": "parser"},
	    {"source": "parser", "target": "route"},
	    {"source": "route", "target": "social"},
	    {"source": "route", "target": "clarify"},
	    {"source": "social", "target": "out"},
	    {"source": "clarify", "target": "out"}
	  ]
	}`

	buildCompiler := func(t *testing.T, chatMode bool, m *stickyTestModel, state *coremodel.OrchChatState) (*orchestrationCompiler, *stickyTestModel) {
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
			dsl: dsl, chatMode: chatMode, userID: 0,
			stickyDecisions: map[string][]string{}, stickyDefaults: map[string]string{},
		}
		if state != nil && state.Active {
			c.chatSkipActive = true
			if strings.TrimSpace(state.Routes) != "" {
				var routes map[string][]string
				if err := json.Unmarshal([]byte(state.Routes), &routes); err == nil {
					c.chatStickyRoutes = routes
				}
			}
		}
		compiled, err := c.compile(context.Background(), deps)
		if err != nil {
			t.Fatalf("compile err: %v", err)
		}
		stream, err := compiled.runnable.Stream(context.Background(), schema.UserMessage("判定: social"))
		if err != nil {
			t.Fatalf("stream err: %v", err)
		}
		defer stream.Close()
		for {
			_, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("recv err: %v", err)
			}
		}
		return c, m
	}

	// 首轮: 未激活, 首跑节点真实执行; 解析器输出含 social, 路由记录 social 并判定激活
	c1, m1 := buildCompiler(t, true, &stickyTestModel{fixed: "类型 social"}, nil)
	if !c1.builtFirstTurnReal {
		t.Fatalf("首轮应真实构建首跑节点")
	}
	if got := c1.stickyDecisions["route"]; len(got) == 0 || got[0] != "social" {
		t.Fatalf("首轮粘性路由应记录 social, got=%v", got)
	}
	if !orchStickyActivated(c1.stickyDecisions, c1.stickyDefaults) {
		t.Fatalf("命中非默认目标 social 应判定激活")
	}
	if m1.received == 0 {
		t.Fatalf("首轮解析器应调用模型")
	}

	// 只走默认目标的轮次 (如进澄清) 不激活
	if orchStickyActivated(map[string][]string{"route": {"clarify"}}, map[string]string{"route": "clarify"}) {
		t.Fatalf("命中默认目标不应激活")
	}

	// 次轮: 激活状态 + 回放 social, parser 跳过不调模型, 仅 social 训练 Agent 真实执行 (1 次调用;
	// 未跳过时会是 parser+social 两次)。收到输入含 social 证明路由确实回放而非重新分类
	_, m2 := buildCompiler(t, true, &stickyTestModel{fixed: "类型 social"}, &coremodel.OrchChatState{
		Active: true,
		Routes: `{"route":["social"]}`,
	})
	if m2.received != 1 {
		t.Fatalf("激活后应只执行非首跑节点 (1 次模型调用), got=%d", m2.received)
	}
}
