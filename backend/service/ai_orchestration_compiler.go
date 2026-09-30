package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	coremodel "backend/model"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// 编排 DSL (Agent Studio)
//
// 设计参考 EinoDev 可视化编排插件的思路: 画布上的节点/连线序列化为 JSON DSL,
// 后端校验后按拓扑形态编译为 Eino compose 的三种编排原语:
//   - 线性流水线      -> compose.Chain
//   - 含分支(无合流)  -> compose.Graph + GraphBranch
//   - 含合流(多入边)  -> compose.Workflow + 字段映射合并
//
// 节点类型: agent(LLM+工具 ReAct) / tool(独立工具调用) / template(提示词模板) /
// branch(条件路由) / merge(多路合并) / end(输出)

const (
	OrchNodeAgent    = "agent"
	OrchNodeTool     = "tool"
	OrchNodeTemplate = "template"
	OrchNodeBranch   = "branch"
	OrchNodeMerge    = "merge"
	OrchNodeEnd      = "end"
	// OrchNodeSubAgent 子Agent: 挂在主 Agent 下 (连线 主Agent->子Agent),
	// 编译为委派工具, 由主 Agent 的 ReAct 循环按需调用
	OrchNodeSubAgent = "subagent"
)

type OrchestrationDSL struct {
	Version int                 `json:"version"`
	Nodes   []OrchestrationNode `json:"nodes"`
	Edges   []OrchestrationEdge `json:"edges"`
}

type OrchestrationNode struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
	// Config 各类型节点的配置, 按类型反序列化
	Config json.RawMessage `json:"config"`
}

type OrchestrationEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	// Label 画布连线上的标签 (branch 分支名等, 仅展示用)
	Label string `json:"label"`
}

type OrchAgentConfig struct {
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
	// MaxIterations 映射 react.MaxStep (模型↔工具往返轮次上限)
	MaxIterations int `json:"max_iterations"`
}

type OrchToolConfig struct {
	Tool string `json:"tool"`
}

type OrchTemplateConfig struct {
	Template string `json:"template"`
}

type OrchBranchCase struct {
	Type   string `json:"type"` // contains / equals / regex
	Value  string `json:"value"`
	Target string `json:"target"`
}

type OrchBranchConfig struct {
	Cases         []OrchBranchCase `json:"cases"`
	DefaultTarget string           `json:"default_target"`
}

type OrchSubAgentConfig struct {
	// AgentID 引用已有 AI Agent (ai_agents 表), 设置后以其系统提示词为子 Agent 指令
	AgentID int `json:"agent_id"`
	// Model 子 Agent 模型 code, 空=默认模型
	Model string `json:"model"`
	// SystemPrompt 内联系统提示词 (AgentID 为空时生效)
	SystemPrompt string `json:"system_prompt"`
	// Description 委派说明: 写入委派工具描述, 供主 Agent 判断何时委派
	Description string `json:"description"`
	// Tools 子 Agent 可调用的工具
	Tools []string `json:"tools"`
}

type OrchMergeConfig struct {
	// Separator 合并多个来源内容时的分隔符, 默认 "\n\n"
	Separator string `json:"separator"`
}

// compilerDeps 编译期依赖, 由 AIAgentService 提供
type compilerDeps struct {
	// getModel 按 model code 构建 ToolCallingChatModel (空串用默认模型)
	getModel func(modelOverride string) (model.ToolCallingChatModel, error)
	// buildTool 按 ai_tools 表配置构建单个工具
	buildTool func(name string) (tool.BaseTool, error)
	// sessionVars 供模板/系统提示词渲染 (current_time / user_id / user_profile)
	sessionVars func() map[string]any
}

type orchestrationCompiler struct {
	deps compilerDeps
	dsl  *OrchestrationDSL

	adj      map[string][]string // source -> targets (全部连线, 含子Agent 委派边)
	flowAdj  map[string][]string // source -> targets (仅主流连线, 不含子Agent)
	inDeg    map[string]int
	outDeg   map[string]int
	nodeMap  map[string]*OrchestrationNode
	topo     []string
}

func parseOrchestrationDSL(definition string) (*OrchestrationDSL, error) {
	if strings.TrimSpace(definition) == "" {
		return nil, errors.New("编排定义不能为空")
	}
	var dsl OrchestrationDSL
	if err := json.Unmarshal([]byte(definition), &dsl); err != nil {
		return nil, fmt.Errorf("编排定义 JSON 解析失败: %w", err)
	}
	if dsl.Version == 0 {
		dsl.Version = 1
	}
	return &dsl, nil
}

// validateOrchestrationDSL 校验 DSL, 返回解析结果与错误列表
func validateOrchestrationDSL(definition string) (*OrchestrationDSL, []string) {
	dsl, err := parseOrchestrationDSL(definition)
	if err != nil {
		return nil, []string{err.Error()}
	}
	c := &orchestrationCompiler{dsl: dsl}
	if errs := c.validate(); len(errs) > 0 {
		return dsl, errs
	}
	return dsl, nil
}

func (c *orchestrationCompiler) nodeByID(id string) *OrchestrationNode {
	return c.nodeMap[id]
}

func (c *orchestrationCompiler) validate() []string {
	var errs []string
	add := func(format string, args ...any) {
		errs = append(errs, fmt.Sprintf(format, args...))
	}

	c.nodeMap = make(map[string]*OrchestrationNode, len(c.dsl.Nodes))
	c.adj = make(map[string][]string)
	c.inDeg = make(map[string]int)
	c.outDeg = make(map[string]int)

	if len(c.dsl.Nodes) == 0 {
		return []string{"编排至少需要一个节点"}
	}

	hasBranch, hasMerge := false, false
	for i := range c.dsl.Nodes {
		n := &c.dsl.Nodes[i]
		if n.ID == "" {
			add("第 %d 个节点缺少 id", i+1)
			continue
		}
		if _, dup := c.nodeMap[n.ID]; dup {
			add("节点 id 重复: %s", n.ID)
			continue
		}
		c.nodeMap[n.ID] = n
		if n.Name == "" {
			n.Name = n.ID
		}
		switch n.Type {
		case OrchNodeAgent, OrchNodeTool, OrchNodeTemplate, OrchNodeBranch, OrchNodeMerge, OrchNodeEnd, OrchNodeSubAgent:
		default:
			add("节点 %s 类型非法: %s", n.ID, n.Type)
		}
		if n.Type == OrchNodeBranch {
			hasBranch = true
		}
		if n.Type == OrchNodeMerge {
			hasMerge = true
		}
	}
	if len(errs) > 0 {
		return errs
	}

	for i, e := range c.dsl.Edges {
		if c.nodeByID(e.Source) == nil {
			add("第 %d 条连线来源不存在: %s", i+1, e.Source)
			continue
		}
		if c.nodeByID(e.Target) == nil {
			add("第 %d 条连线目标不存在: %s", i+1, e.Target)
			continue
		}
		if e.Source == e.Target {
			add("连线不允许自环: %s", e.Source)
			continue
		}
		c.adj[e.Source] = append(c.adj[e.Source], e.Target)
		c.inDeg[e.Target]++
		c.outDeg[e.Source]++
	}

	// 拓扑排序 + 环检测 (Kahn): 子Agent 节点不属于主流程 (仅作为主 Agent 的委派挂载),
	// 主流程的度按"目标非子Agent"的连线计算
	isSub := map[string]bool{}
	flowNodeCount := 0
	for id := range c.nodeMap {
		if c.nodeMap[id].Type == OrchNodeSubAgent {
			isSub[id] = true
			continue
		}
		flowNodeCount++
	}
	c.flowAdj = make(map[string][]string)
	flowIn := make(map[string]int)
	for src, ts := range c.adj {
		for _, t := range ts {
			if isSub[t] {
				continue
			}
			c.flowAdj[src] = append(c.flowAdj[src], t)
			flowIn[t]++
		}
	}
	c.topo = make([]string, 0, flowNodeCount)
	deg := make(map[string]int, flowNodeCount)
	for id := range c.nodeMap {
		if isSub[id] {
			continue
		}
		deg[id] = flowIn[id]
	}
	queue := make([]string, 0, len(deg))
	for id, d := range deg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		c.topo = append(c.topo, id)
		for _, t := range c.flowAdj[id] {
			deg[t]--
			if deg[t] == 0 {
				queue = append(queue, t)
			}
		}
	}
	if len(c.topo) != flowNodeCount {
		add("编排存在循环连线, 不允许成环 (Agent 节点内部的工具循环由 ReAct 自动处理)")
		return errs
	}

	// 预解析分支目标: target node id -> branch node id
	branchTargetOf := make(map[string]string)
	for _, id := range c.topo {
		n := c.nodeByID(id)
		if n.Type != OrchNodeBranch {
			continue
		}
		var cfg OrchBranchConfig
		if err := json.Unmarshal(n.Config, &cfg); err != nil {
			continue
		}
		for _, cs := range cfg.Cases {
			if cs.Target != "" && cs.Target != id {
				branchTargetOf[cs.Target] = id
			}
		}
		if cfg.DefaultTarget != "" && cfg.DefaultTarget != id {
			branchTargetOf[cfg.DefaultTarget] = id
		}
	}

	inputCount, outputCount := 0, 0
	for _, id := range c.topo {
		n := c.nodeByID(id)
		inDeg, outDeg := flowIn[id], len(c.flowAdj[id])
		switch n.Type {
		case OrchNodeMerge:
			if inDeg < 2 {
				add("合并节点 %s 至少需要两条入边", id)
			}
			if outDeg != 1 {
				add("合并节点 %s 必须恰好一条出边", id)
			}
		case OrchNodeBranch:
			if inDeg != 1 {
				add("分支节点 %s 必须恰好一条入边", id)
			}
			var cfg OrchBranchConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				add("分支节点 %s 配置解析失败: %v", id, err)
				continue
			}
			if len(cfg.Cases) == 0 {
				add("分支节点 %s 至少需要一个分支条件", id)
			}
			targets := map[string]bool{}
			for j, cs := range cfg.Cases {
				if cs.Value == "" {
					add("分支节点 %s 第 %d 个条件缺少匹配值", id, j+1)
				}
				if cs.Target == "" || c.nodeByID(cs.Target) == nil {
					add("分支节点 %s 第 %d 个条件目标无效: %s", id, j+1, cs.Target)
					continue
				}
				if cs.Target == id {
					add("分支节点 %s 条件不能指向自身", id)
					continue
				}
				targets[cs.Target] = true
				switch cs.Type {
				case "contains", "equals":
				case "regex":
					if _, err := regexp.Compile(cs.Value); err != nil {
						add("分支节点 %s 第 %d 个条件正则非法: %v", id, j+1, err)
					}
				default:
					add("分支节点 %s 第 %d 个条件类型非法: %s (contains/equals/regex)", id, j+1, cs.Type)
				}
			}
			if cfg.DefaultTarget == "" || c.nodeByID(cfg.DefaultTarget) == nil {
				add("分支节点 %s 缺少有效的默认分支目标", id)
			} else if cfg.DefaultTarget == id {
				add("分支节点 %s 默认分支不能指向自身", id)
			} else {
				targets[cfg.DefaultTarget] = true
			}
			if outDeg != len(targets) {
				add("分支节点 %s 的画布连线(%d 条)与条件目标(%d 个)不一致, 分支节点必须连接到所有条件目标", id, outDeg, len(targets))
			}
			for _, t := range c.adj[id] {
				if !targets[t] {
					add("分支节点 %s 连线目标 %s 未出现在分支条件中", id, t)
				}
			}
		default:
			if inDeg > 1 {
				// 允许同一分支的多个目标汇聚 (运行时只会有一条路径执行)
				branches := map[string]bool{}
				consistent := true
				for _, e := range c.dsl.Edges {
					if e.Target != id {
						continue
					}
					br, ok := branchTargetOf[e.Source]
					if !ok {
						consistent = false
						break
					}
					branches[br] = true
				}
				if !consistent || len(branches) != 1 {
					add("节点 %s 只允许一条入边 (多路合并请使用合并节点; 分支汇合必须来自同一分支节点的目标)", id)
				}
			}
			if outDeg > 1 {
				add("节点 %s 只允许一条出边 (多路分发请使用分支节点)", id)
			}
		}
		if inDeg == 0 {
			inputCount++
		}
		if outDeg == 0 {
			outputCount++
		}
	}
	// 子Agent 节点规则: 恰好一条来自 Agent 节点的入边 (委派), 不允许出边
	for id := range c.nodeMap {
		n := c.nodeByID(id)
		if n.Type != OrchNodeSubAgent {
			continue
		}
		if c.inDeg[id] != 1 {
			add("子Agent 节点 %s 必须恰好有一条来自主 Agent 的入边", id)
		} else {
			for _, e := range c.dsl.Edges {
				if e.Target != id {
					continue
				}
				if src := c.nodeByID(e.Source); src == nil || src.Type != OrchNodeAgent {
					add("子Agent 节点 %s 的入边必须来自 Agent 节点 (委派方向: 主 Agent -> 子Agent)", id)
				}
				break
			}
		}
		if c.outDeg[id] > 0 {
			add("子Agent 节点 %s 不允许向外连线", id)
		}
		var cfg OrchSubAgentConfig
		if len(n.Config) > 0 {
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				add("子Agent 节点 %s 配置解析失败: %v", id, err)
				continue
			}
		}
		if cfg.AgentID == 0 && strings.TrimSpace(cfg.SystemPrompt) == "" {
			add("子Agent 节点 %s 需要引用已有 Agent 或填写内联系统提示词", id)
		}
	}

	if hasMerge {
		if inputCount < 1 {
			add("编排至少需要一个入口节点(无入边)")
		}
	} else if inputCount != 1 {
		add("编排必须恰好一个入口节点(无入边), 当前 %d 个", inputCount)
	}
	if outputCount != 1 {
		add("编排必须恰好一个出口节点(无出边), 当前 %d 个", outputCount)
	}
	if hasBranch && hasMerge {
		add("当前版本暂不支持分支与合并混用, 请拆分为多个编排")
	}

	// 节点配置校验
	for _, id := range c.topo {
		n := c.nodeByID(id)
		switch n.Type {
		case OrchNodeAgent:
			var cfg OrchAgentConfig
			if len(n.Config) > 0 {
				if err := json.Unmarshal(n.Config, &cfg); err != nil {
					add("Agent 节点 %s 配置解析失败: %v", id, err)
				}
			}
		case OrchNodeTool:
			var cfg OrchToolConfig
			if len(n.Config) == 0 || json.Unmarshal(n.Config, &cfg) != nil || cfg.Tool == "" {
				add("工具节点 %s 缺少工具配置", id)
			}
		case OrchNodeTemplate:
			var cfg OrchTemplateConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				add("模板节点 %s 配置解析失败: %v", id, err)
			} else if strings.TrimSpace(cfg.Template) == "" {
				add("模板节点 %s 模板内容不能为空", id)
			} else if _, err := template.New(id).Parse(cfg.Template); err != nil {
				add("模板节点 %s 模板语法错误: %v", id, err)
			}
		}
	}
	return errs
}

// compiledOrchestration 编译结果: 可运行对象 + 调试事件归属所需的节点名集合
type compiledOrchestration struct {
	runnable compose.Runnable[*schema.Message, *schema.Message]
	mode     string // chain / graph / workflow
	// nodeKeys 参与归属的顶层节点 key -> 显示名 (不含 react 内部子节点)
	nodeKeys map[string]string
}

func (c *orchestrationCompiler) compile(ctx context.Context, deps compilerDeps) (*compiledOrchestration, error) {
	c.deps = deps
	if errs := c.validate(); len(errs) > 0 {
		return nil, errors.New("编排定义校验失败: " + strings.Join(errs, "; "))
	}

	hasBranch, hasMerge := false, false
	for _, n := range c.dsl.Nodes {
		if n.Type == OrchNodeBranch {
			hasBranch = true
		}
		if n.Type == OrchNodeMerge {
			hasMerge = true
		}
	}

	// 预构建各节点 lambda; branch 节点是路由点, 编译时用 GraphBranch 实现
	nodeKeys := make(map[string]string, len(c.dsl.Nodes))
	lambdas := make(map[string]*compose.Lambda, len(c.dsl.Nodes))
	for _, id := range c.topo {
		n := c.nodeByID(id)
		nodeKeys[id] = n.Name
		lambda, err := c.buildNodeLambda(ctx, n)
		if err != nil {
			return nil, err
		}
		lambdas[id] = lambda
	}

	switch {
	case hasMerge:
		return c.compileWorkflow(lambdas, nodeKeys)
	case hasBranch:
		return c.compileGraph(ctx, lambdas, nodeKeys)
	default:
		return c.compileChain(lambdas, nodeKeys)
	}
}

func (c *orchestrationCompiler) compileChain(lambdas map[string]*compose.Lambda, nodeKeys map[string]string) (*compiledOrchestration, error) {
	chain := compose.NewChain[*schema.Message, *schema.Message]()
	for _, id := range c.topo {
		chain.AppendLambda(lambdas[id], compose.WithNodeName(id))
	}
	runnable, err := chain.Compile(context.Background(), compose.WithGraphName("orchestration"))
	if err != nil {
		return nil, fmt.Errorf("Chain 编译失败: %w", err)
	}
	return &compiledOrchestration{runnable: runnable, mode: "chain", nodeKeys: nodeKeys}, nil
}

func (c *orchestrationCompiler) compileGraph(ctx context.Context, lambdas map[string]*compose.Lambda, nodeKeys map[string]string) (*compiledOrchestration, error) {
	g := compose.NewGraph[*schema.Message, *schema.Message]()
	for id, lambda := range lambdas {
		if err := g.AddLambdaNode(id, lambda, compose.WithNodeName(id)); err != nil {
			return nil, fmt.Errorf("添加节点 %s 失败: %w", id, err)
		}
	}
	_ = lambdas
	for _, id := range c.topo {
		n := c.nodeByID(id)
		if c.inDeg[id] == 0 {
			if err := g.AddEdge(compose.START, id); err != nil {
				return nil, fmt.Errorf("连接入口 %s 失败: %w", id, err)
			}
		}
		if c.outDeg[id] == 0 {
			if err := g.AddEdge(id, compose.END); err != nil {
				return nil, fmt.Errorf("连接出口 %s 失败: %w", id, err)
			}
		}
		if n.Type == OrchNodeBranch {
			// 分支节点到目标的路由由 GraphBranch 处理, 不建普通边
			var cfg OrchBranchConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				return nil, fmt.Errorf("分支节点 %s 配置解析失败: %w", id, err)
			}
			cases := make([]OrchBranchCase, len(cfg.Cases))
			copy(cases, cfg.Cases)
			defaultTarget := cfg.DefaultTarget
			cond := func(_ context.Context, in *schema.Message) (string, error) {
				content := ""
				if in != nil {
					content = in.Content
				}
				for _, cs := range cases {
					if orchBranchMatch(cs, content) {
						return cs.Target, nil
					}
				}
				return defaultTarget, nil
			}
			endNodes := map[string]bool{}
			for _, cs := range cfg.Cases {
				endNodes[cs.Target] = true
			}
			endNodes[defaultTarget] = true
			if err := g.AddBranch(id, compose.NewGraphBranch[*schema.Message](cond, endNodes)); err != nil {
				return nil, fmt.Errorf("构建分支 %s 失败: %w", id, err)
			}
			continue
		}
		for _, t := range c.flowAdj[id] {
			if err := g.AddEdge(id, t); err != nil {
				return nil, fmt.Errorf("连接 %s -> %s 失败: %w", id, t, err)
			}
		}
	}
	runnable, err := g.Compile(ctx, compose.WithGraphName("orchestration"))
	if err != nil {
		return nil, fmt.Errorf("Graph 编译失败: %w", err)
	}
	return &compiledOrchestration{runnable: runnable, mode: "graph", nodeKeys: nodeKeys}, nil
}

func (c *orchestrationCompiler) compileWorkflow(lambdas map[string]*compose.Lambda, nodeKeys map[string]string) (*compiledOrchestration, error) {
	wf := compose.NewWorkflow[*schema.Message, *schema.Message]()
	wfNodes := make(map[string]*compose.WorkflowNode, len(lambdas))
	for id, lambda := range lambdas {
		wfNodes[id] = wf.AddLambdaNode(id, lambda, compose.WithNodeName(id))
	}
	inEdges := make(map[string][]OrchestrationEdge)
	for _, e := range c.dsl.Edges {
		if n := c.nodeByID(e.Target); n != nil && n.Type == OrchNodeSubAgent {
			continue // 委派边不参与数据流
		}
		inEdges[e.Target] = append(inEdges[e.Target], e)
	}
	for _, id := range c.topo {
		if c.inDeg[id] == 0 {
			_ = wfNodes[id].AddInput(compose.START)
		}
		if c.nodeByID(id).Type == OrchNodeMerge {
			// 合并节点: 每条入边把来源消息的 Content 映射到 map 的来源 key 上
			for _, e := range inEdges[id] {
				_ = wfNodes[id].AddInput(e.Source, compose.MapFields("Content", e.Source))
			}
			continue
		}
		for _, e := range inEdges[id] {
			_ = wfNodes[id].AddInput(e.Source)
		}
		if c.outDeg[id] == 0 {
			_ = wf.End().AddInput(id)
		}
	}
	runnable, err := wf.Compile(context.Background(), compose.WithGraphName("orchestration"))
	if err != nil {
		return nil, fmt.Errorf("Workflow 编译失败: %w", err)
	}
	return &compiledOrchestration{runnable: runnable, mode: "workflow", nodeKeys: nodeKeys}, nil
}

func orchBranchMatch(cs OrchBranchCase, content string) bool {
	switch cs.Type {
	case "equals":
		return strings.TrimSpace(content) == strings.TrimSpace(cs.Value)
	case "regex":
		re, err := regexp.Compile(cs.Value)
		if err != nil {
			return false
		}
		return re.MatchString(content)
	default: // contains
		return strings.Contains(content, cs.Value)
	}
}

// buildNodeLambda 构建单个节点的 Lambda (agent/tool/template/merge/end)
func (c *orchestrationCompiler) buildNodeLambda(ctx context.Context, n *OrchestrationNode) (*compose.Lambda, error) {
	switch n.Type {
	case OrchNodeAgent:
		return c.buildAgentLambda(ctx, n)
	case OrchNodeTool:
		return c.buildToolLambda(n)
	case OrchNodeTemplate:
		return c.buildTemplateLambda(n)
	case OrchNodeMerge:
		return c.buildMergeLambda(n)
	default: // end 及其他: 直通
		return orchPassthroughLambda(), nil
	}
}

func orchPassthroughLambda() *compose.Lambda {
	lambda, _ := compose.AnyLambda(
		func(_ context.Context, in *schema.Message, _ ...any) (*schema.Message, error) { return in, nil },
		func(_ context.Context, in *schema.Message, _ ...any) (*schema.StreamReader[*schema.Message], error) {
			return schema.StreamReaderFromArray([]*schema.Message{in}), nil
		},
		nil, nil,
	)
	return lambda
}

// subAgentIDsOf 返回挂载在主 Agent 下的子Agent 节点 id (按名称排序, 保证委派工具命名稳定)
func (c *orchestrationCompiler) subAgentIDsOf(parentID string) []string {
	var ids []string
	for _, t := range c.adj[parentID] {
		if n := c.nodeByID(t); n != nil && n.Type == OrchNodeSubAgent {
			ids = append(ids, t)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		return c.nodeByID(ids[i]).Name < c.nodeByID(ids[j]).Name
	})
	return ids
}

// orchNewReactAgent 主 Agent 与子Agent 共用的 ReAct 构建入口
func orchNewReactAgent(ctx context.Context, key, systemPrompt string, chatModel model.ToolCallingChatModel, tools []tool.BaseTool, maxStep int) (*react.Agent, error) {
	return react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MessageModifier: func(_ context.Context, input []*schema.Message) []*schema.Message {
			msgs := orchNormalizeModelInput(input)
			if systemPrompt == "" {
				return msgs
			}
			return append([]*schema.Message{{Role: schema.System, Content: systemPrompt}}, msgs...)
		},
		// 子图名与编排节点 key 隔离, 避免图级 end 事件与节点自身 end 事件混淆
		GraphName:     key + ".react",
		ModelNodeName: key + ".model",
		ToolsNodeName: key + ".tools",
		MaxStep:       maxStep,
	})
}

// buildSubReactAgent 构建子Agent 执行体: 引用已有 AI Agent 或内联提示词
func (c *orchestrationCompiler) buildSubReactAgent(ctx context.Context, id string, cfg OrchSubAgentConfig) (*react.Agent, error) {
	instruction := strings.TrimSpace(cfg.SystemPrompt)
	desc := strings.TrimSpace(cfg.Description)
	if cfg.AgentID > 0 {
		var dbAgent coremodel.AIAgent
		if err := DB.First(&dbAgent, cfg.AgentID).Error; err != nil {
			return nil, fmt.Errorf("子Agent 节点 %s 引用的 Agent %d 不存在", id, cfg.AgentID)
		}
		instruction = dbAgent.SystemPrompt
		if desc == "" {
			desc = dbAgent.Description
		}
	}
	if instruction == "" {
		return nil, fmt.Errorf("子Agent 节点 %s 缺少可用的系统提示词", id)
	}
	instruction, err := orchRenderTemplate(id, instruction, c.deps.sessionVars())
	if err != nil {
		return nil, fmt.Errorf("子Agent 节点 %s 系统提示词渲染失败: %w", id, err)
	}
	chatModel, err := c.deps.getModel(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("子Agent 节点 %s 获取模型失败: %w", id, err)
	}
	subTools := make([]tool.BaseTool, 0, len(cfg.Tools))
	for _, name := range cfg.Tools {
		t, err := c.deps.buildTool(name)
		if err != nil {
			return nil, fmt.Errorf("子Agent 节点 %s 构建工具 %s 失败: %w", id, name, err)
		}
		subTools = append(subTools, t)
	}
	agent, err := orchNewReactAgent(ctx, id, instruction, chatModel, subTools, 15)
	if err != nil {
		return nil, fmt.Errorf("子Agent 节点 %s 构建失败: %w", id, err)
	}
	return agent, nil
}

// orchDelegateTool 把子Agent 包装成主 Agent 可调用的委派工具:
// 主 Agent 以 {"task": "..."} 传任务, 子Agent 独立 ReAct 执行后返回文本结果
type orchDelegateTool struct {
	name    string
	subName string
	desc    string
	agent   *react.Agent
}

func newOrchDelegateTool(name, subName, desc string, agent *react.Agent) *orchDelegateTool {
	return &orchDelegateTool{name: name, subName: subName, desc: desc, agent: agent}
}

func (t *orchDelegateTool) Info(_ context.Context) (*schema.ToolInfo, error) {
	return &schema.ToolInfo{
		Name: t.name,
		Desc: fmt.Sprintf("委派给子Agent「%s」处理: %s", t.subName, t.desc),
		ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task": {Type: schema.String, Desc: "委派给该子Agent 的任务描述或问题", Required: true},
		}),
	}, nil
}

func (t *orchDelegateTool) InvokableRun(ctx context.Context, argumentsInJSON string, _ ...tool.Option) (string, error) {
	task := strings.TrimSpace(argumentsInJSON)
	var args struct {
		Task string `json:"task"`
	}
	if json.Unmarshal([]byte(task), &args) == nil && strings.TrimSpace(args.Task) != "" {
		task = strings.TrimSpace(args.Task)
	}
	msg, err := t.agent.Generate(ctx, []*schema.Message{schema.UserMessage(task)})
	if err != nil {
		return "", fmt.Errorf("子Agent「%s」执行失败: %w", t.subName, err)
	}
	return msg.Content, nil
}

func (c *orchestrationCompiler) buildAgentLambda(ctx context.Context, n *OrchestrationNode) (*compose.Lambda, error) {
	var cfg OrchAgentConfig
	if len(n.Config) > 0 {
		if err := json.Unmarshal(n.Config, &cfg); err != nil {
			return nil, fmt.Errorf("Agent 节点 %s 配置解析失败: %w", n.ID, err)
		}
	}
	chatModel, err := c.deps.getModel(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("Agent 节点 %s 获取模型失败: %w", n.ID, err)
	}
	agentTools := make([]tool.BaseTool, 0, len(cfg.Tools))
	for _, name := range cfg.Tools {
		t, err := c.deps.buildTool(name)
		if err != nil {
			return nil, fmt.Errorf("Agent 节点 %s 构建工具 %s 失败: %w", n.ID, name, err)
		}
		agentTools = append(agentTools, t)
	}

	vars := c.deps.sessionVars()
	systemPrompt := cfg.SystemPrompt
	if systemPrompt != "" {
		systemPrompt, err = orchRenderTemplate(n.ID, systemPrompt, vars)
		if err != nil {
			return nil, fmt.Errorf("Agent 节点 %s 系统提示词渲染失败: %w", n.ID, err)
		}
	}
	maxStep := cfg.MaxIterations
	if maxStep <= 0 {
		maxStep = 25
	}

	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: agentTools,
		},
		MessageModifier: func(_ context.Context, input []*schema.Message) []*schema.Message {
			msgs := orchNormalizeModelInput(input)
			if systemPrompt == "" {
				return msgs
			}
			return append([]*schema.Message{{Role: schema.System, Content: systemPrompt}}, msgs...)
		},
		// 子图名与编排节点 key 隔离, 避免图级 end 事件与节点自身 end 事件混淆
		GraphName:     n.ID + ".react",
		ModelNodeName: n.ID + ".model",
		ToolsNodeName: n.ID + ".tools",
		MaxStep:       maxStep,
	})
	if err != nil {
		return nil, fmt.Errorf("Agent 节点 %s 构建失败: %w", n.ID, err)
	}
	return compose.AnyLambda(
		func(ctx context.Context, in *schema.Message, _ ...any) (*schema.Message, error) {
			return agent.Generate(ctx, []*schema.Message{in})
		},
		func(ctx context.Context, in *schema.Message, _ ...any) (*schema.StreamReader[*schema.Message], error) {
			return agent.Stream(ctx, []*schema.Message{in})
		},
		nil, nil,
	)
}

func (c *orchestrationCompiler) buildToolLambda(n *OrchestrationNode) (*compose.Lambda, error) {
	var cfg OrchToolConfig
	if len(n.Config) == 0 || json.Unmarshal(n.Config, &cfg) != nil || cfg.Tool == "" {
		return nil, fmt.Errorf("工具节点 %s 缺少工具配置", n.ID)
	}
	toolName := cfg.Tool
	baseTool, err := c.deps.buildTool(toolName)
	if err != nil {
		return nil, fmt.Errorf("工具节点 %s 构建工具 %s 失败: %w", n.ID, toolName, err)
	}
	invokable, ok := baseTool.(tool.InvokableTool)
	if !ok {
		return nil, fmt.Errorf("工具节点 %s: 工具 %s 不支持同步调用", n.ID, toolName)
	}
	run := func(ctx context.Context, in *schema.Message) (*schema.Message, error) {
		content := ""
		if in != nil {
			content = in.Content
		}
		args := strings.TrimSpace(content)
		if args == "" || !orchIsJSONObject(args) {
			// 上游不是 JSON 时包装为 {"input": ...}, 保证工具参数合法
			wrapped, _ := json.Marshal(map[string]string{"input": content})
			args = string(wrapped)
		}
		result, err := invokable.InvokableRun(ctx, args)
		if err != nil {
			return nil, fmt.Errorf("工具 %s 执行失败: %w", toolName, err)
		}
		return &schema.Message{Role: schema.Tool, Content: result, ToolName: toolName}, nil
	}
	return compose.AnyLambda(
		func(ctx context.Context, in *schema.Message, _ ...any) (*schema.Message, error) { return run(ctx, in) },
		func(ctx context.Context, in *schema.Message, _ ...any) (*schema.StreamReader[*schema.Message], error) {
			out, err := run(ctx, in)
			if err != nil {
				return nil, err
			}
			return schema.StreamReaderFromArray([]*schema.Message{out}), nil
		},
		nil, nil,
	)
}

func (c *orchestrationCompiler) buildTemplateLambda(n *OrchestrationNode) (*compose.Lambda, error) {
	var cfg OrchTemplateConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		return nil, fmt.Errorf("模板节点 %s 配置解析失败: %w", n.ID, err)
	}
	tpl, err := template.New(n.ID).Parse(cfg.Template)
	if err != nil {
		return nil, fmt.Errorf("模板节点 %s 模板语法错误: %w", n.ID, err)
	}
	vars := c.deps.sessionVars()
	run := func(in *schema.Message) (*schema.Message, error) {
		input := ""
		if in != nil {
			input = in.Content
		}
		data := map[string]any{"Input": input}
		for k, v := range vars {
			data[k] = v
		}
		var sb strings.Builder
		if err := tpl.Execute(&sb, data); err != nil {
			return nil, fmt.Errorf("模板 %s 渲染失败: %w", n.ID, err)
		}
		return &schema.Message{Role: schema.User, Content: sb.String()}, nil
	}
	return compose.AnyLambda(
		func(_ context.Context, in *schema.Message, _ ...any) (*schema.Message, error) { return run(in) },
		func(_ context.Context, in *schema.Message, _ ...any) (*schema.StreamReader[*schema.Message], error) {
			out, err := run(in)
			if err != nil {
				return nil, err
			}
			return schema.StreamReaderFromArray([]*schema.Message{out}), nil
		},
		nil, nil,
	)
}

// buildMergeLambda 合并节点: Workflow 字段映射把各来源的 Content 注入 map[来源key]any,
// 按 DSL 连线顺序拼接
func (c *orchestrationCompiler) buildMergeLambda(n *OrchestrationNode) (*compose.Lambda, error) {
	var cfg OrchMergeConfig
	if len(n.Config) > 0 {
		_ = json.Unmarshal(n.Config, &cfg)
	}
	sep := cfg.Separator
	if sep == "" {
		sep = "\n\n"
	}
	var sourceOrder []string
	for _, e := range c.dsl.Edges {
		if e.Target == n.ID {
			sourceOrder = append(sourceOrder, e.Source)
		}
	}
	run := func(in map[string]any) (*schema.Message, error) {
		parts := make([]string, 0, len(sourceOrder))
		for _, src := range sourceOrder {
			v, ok := in[src]
			if !ok {
				continue
			}
			switch msg := v.(type) {
			case *schema.Message:
				if msg != nil && msg.Content != "" {
					parts = append(parts, msg.Content)
				}
			case string:
				if msg != "" {
					parts = append(parts, msg)
				}
			}
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("合并节点 %s 未收到任何上游内容", n.ID)
		}
		return &schema.Message{Role: schema.User, Content: strings.Join(parts, sep)}, nil
	}
	return compose.AnyLambda(
		func(_ context.Context, in map[string]any, _ ...any) (*schema.Message, error) { return run(in) },
		func(_ context.Context, in map[string]any, _ ...any) (*schema.StreamReader[*schema.Message], error) {
			out, err := run(in)
			if err != nil {
				return nil, err
			}
			return schema.StreamReaderFromArray([]*schema.Message{out}), nil
		},
		nil, nil,
	)
}

// orchNormalizeModelInput 规范化发往模型的消息序列:
// Ark 等模型接口要求序列以 system/user 开头且必须含 user 消息, 而编排的上游
// 输出可能是 assistant/tool 角色 (链式 Agent/工具节点), 无 user 时把首条消息
// 复制并转为 user 角色 (不改写共享的原消息, 遵循外部只读原则)
func orchNormalizeModelInput(input []*schema.Message) []*schema.Message {
	hasUser := false
	for _, m := range input {
		if m != nil && m.Role == schema.User {
			hasUser = true
			break
		}
	}
	if hasUser || len(input) == 0 {
		return input
	}
	msgs := make([]*schema.Message, 0, len(input))
	for i, m := range input {
		if m == nil {
			continue
		}
		if i == 0 && m.Content != "" && m.Role != schema.User {
			cp := *m
			cp.Role = schema.User
			cp.ToolCalls = nil
			msgs = append(msgs, &cp)
			continue
		}
		msgs = append(msgs, m)
	}
	return msgs
}

func orchIsJSONObject(s string) bool {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") || !strings.HasSuffix(s, "}") {
		return false
	}
	var v map[string]any
	return json.Unmarshal([]byte(s), &v) == nil
}

func orchRenderTemplate(name, tpl string, vars map[string]any) (string, error) {
	t, err := template.New(name).Parse(tpl)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	if err := t.Execute(&sb, vars); err != nil {
		return "", err
	}
	return sb.String(), nil
}

// ---------- 调试运行追踪: 把 compose 回调事件归属到编排节点 ----------

// OrchToolTrace 工具调用记录
type OrchToolTrace struct {
	Name string `json:"name"`
	MS   int64  `json:"ms"`
}

// OrchNodeTrace 单个编排节点的调试摘要
type OrchNodeTrace struct {
	Key    string             `json:"key"`
	Name   string             `json:"name"`
	Comp   string             `json:"comp"`
	Status string             `json:"status"` // running / success / error
	MS     int64              `json:"ms"`
	Tokens *schema.TokenUsage `json:"tokens,omitempty"`
	// Content 节点最终输出内容
	Content string `json:"content,omitempty"`
	// ToolCalls agent 节点内部的工具调用记录
	ToolCalls []OrchToolTrace `json:"tool_calls,omitempty"`
	Error     string          `json:"error,omitempty"`
}

// orchSpanKey ctx key, 携带当前 goroutine 的回调调用栈
type orchSpanKey struct{}

// orchSpan 一次回调调用的区间, 经 ctx 在 OnStart/OnEnd 间传递
type orchSpan struct {
	key   string // RunInfo.Name (节点 key 或 react 内部子节点名)
	comp  string
	start time.Time
	// chunks 输出分片; 最终内容用 schema.ConcatMessages 合并,
	// 与编排框架对流的拼接语义一致 (正确处理用量块携带完整消息的情况)
	chunks []*schema.Message
}

// orchTraceHandler 调试回调: ctx 调用栈保证并发/嵌套时归属正确;
// 顶层节点事件记入摘要表, 内部事件(model/tools/tool)归属到所在编排节点
type orchTraceHandler struct {
	nodeKeys map[string]string
	owners   map[string]*OrchNodeTrace
	order    []string
	mu       sync.Mutex
	emit     func(event string, payload any)
	started  time.Time
}

func newOrchTraceHandler(nodeKeys map[string]string, emit func(event string, payload any)) *orchTraceHandler {
	return &orchTraceHandler{
		nodeKeys: nodeKeys,
		owners:   make(map[string]*OrchNodeTrace),
		emit:     emit,
		started:  time.Now(),
	}
}

func (h *orchTraceHandler) Needed(_ context.Context, _ *callbacks.RunInfo, timing callbacks.CallbackTiming) bool {
	switch timing {
	case callbacks.TimingOnStart, callbacks.TimingOnEnd, callbacks.TimingOnEndWithStreamOutput, callbacks.TimingOnError:
		return true
	}
	return false
}

func (h *orchTraceHandler) getOwner(key, name, comp string) *OrchNodeTrace {
	h.mu.Lock()
	defer h.mu.Unlock()
	t, ok := h.owners[key]
	if !ok {
		t = &OrchNodeTrace{Key: key, Name: name, Comp: string(comp), Status: "running"}
		h.owners[key] = t
		h.order = append(h.order, key)
	}
	return t
}

// popSpan 从 ctx 栈弹出当前 span, 返回 span 与剩余栈
func popSpan(ctx context.Context) (*orchSpan, []*orchSpan, context.Context) {
	stack, _ := ctx.Value(orchSpanKey{}).([]*orchSpan)
	if len(stack) == 0 {
		return nil, nil, ctx
	}
	span := stack[len(stack)-1]
	rest := stack[:len(stack)-1]
	return span, rest, context.WithValue(ctx, orchSpanKey{}, rest)
}

func pushSpan(ctx context.Context, span *orchSpan) context.Context {
	stack, _ := ctx.Value(orchSpanKey{}).([]*orchSpan)
	return context.WithValue(ctx, orchSpanKey{}, append(append([]*orchSpan{}, stack...), span))
}

// ownerKeyFor 解析 span 归属的编排节点: 自身是顶层节点则用自身,
// 否则取剩余栈中最近的顶层节点 (react 内部子节点/工具调用都归属所在 Agent 节点)
func ownerKeyFor(span *orchSpan, rest []*orchSpan, nodeKeys map[string]string) string {
	if _, ok := nodeKeys[span.key]; ok {
		return span.key
	}
	for i := len(rest) - 1; i >= 0; i-- {
		if _, ok := nodeKeys[rest[i].key]; ok {
			return rest[i].key
		}
	}
	return ""
}

func (h *orchTraceHandler) OnStart(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
	if info == nil {
		return ctx
	}
	span := &orchSpan{key: info.Name, comp: string(info.Component), start: time.Now()}
	nextCtx := pushSpan(ctx, span)
	_, isTop := h.nodeKeys[info.Name]
	ownerKey := ownerKeyFor(span, stackOf(nextCtx), h.nodeKeys)
	payload := map[string]any{
		"kind": "start", "key": info.Name, "comp": string(info.Component),
		"owner": ownerKey,
	}
	if isTop {
		payload["name"] = h.nodeKeys[info.Name]
		h.getOwner(info.Name, h.nodeKeys[info.Name], string(info.Component))
	}
	h.safeEmit("node", payload)
	return nextCtx
}

func stackOf(ctx context.Context) []*orchSpan {
	stack, _ := ctx.Value(orchSpanKey{}).([]*orchSpan)
	return stack
}

// OnStartWithStreamInput 流式输入场景: 关闭复制的流并放行
func (h *orchTraceHandler) OnStartWithStreamInput(ctx context.Context, info *callbacks.RunInfo, input *schema.StreamReader[callbacks.CallbackInput]) context.Context {
	input.Close()
	return ctx
}

func (h *orchTraceHandler) OnEnd(ctx context.Context, info *callbacks.RunInfo, output callbacks.CallbackOutput) context.Context {
	if info == nil {
		return ctx
	}
	span, rest, _ := popSpan(ctx)
	if span == nil {
		return ctx
	}
	rawMsg, _ := output.(*schema.Message)
	h.finishSpan(span, rest, model.ConvCallbackOutput(output), rawMsg, "")
	return ctx
}

func (h *orchTraceHandler) OnEndWithStreamOutput(ctx context.Context, info *callbacks.RunInfo, output *schema.StreamReader[callbacks.CallbackOutput]) context.Context {
	if info == nil {
		output.Close()
		return ctx
	}
	span, rest, _ := popSpan(ctx)
	if span == nil {
		output.Close()
		return ctx
	}
	defer output.Close()
	var mo *model.CallbackOutput
	var rawMsg *schema.Message
	for {
		chunk, err := output.Recv()
		if err != nil {
			break
		}
		if m := model.ConvCallbackOutput(chunk); m != nil {
			mo = m
			if m.Message != nil {
				span.chunks = append(span.chunks, m.Message)
			}
			// 节点外部包装的回调: TokenUsage 为空, 用量在原始消息的 ResponseMeta 里
			if m.TokenUsage == nil && m.Message != nil && m.Message.ResponseMeta != nil && m.Message.ResponseMeta.Usage != nil {
				rawMsg = m.Message
			}
			continue
		}
		// 组件未内置回调切面时, 框架上抛的是原始输出
		if msg, ok := chunk.(*schema.Message); ok {
			rawMsg = msg
			span.chunks = append(span.chunks, msg)
		}
	}
	h.finishSpan(span, rest, mo, rawMsg, "")
	return ctx
}

func (h *orchTraceHandler) OnError(ctx context.Context, info *callbacks.RunInfo, err error) context.Context {
	if info == nil {
		return ctx
	}
	span, rest, _ := popSpan(ctx)
	if span == nil {
		return ctx
	}
	h.finishSpan(span, rest, nil, nil, err.Error())
	return ctx
}

func (h *orchTraceHandler) finishSpan(span *orchSpan, rest []*orchSpan, mo *model.CallbackOutput, rawMsg *schema.Message, errMsg string) {
	ms := time.Since(span.start).Milliseconds()
	ownerKey := ownerKeyFor(span, rest, h.nodeKeys)
	if ownerKey == "" {
		return
	}
	isTop := ownerKey == span.key
	owner := h.getOwner(ownerKey, h.nodeKeys[ownerKey], "")

	// token 用量只在与模型相关的 span 上合并, 避免下游透传节点继承上游用量:
	// 内置回调切面的组件给 model.CallbackOutput, 外部包装的组件给原始 *schema.Message
	if span.comp == "ChatModel" {
		if mo != nil && mo.TokenUsage != nil && mo.TokenUsage.TotalTokens > 0 {
			owner.Tokens = orchMergeTokenUsage(owner.Tokens, &schema.TokenUsage{
				PromptTokens:     mo.TokenUsage.PromptTokens,
				CompletionTokens: mo.TokenUsage.CompletionTokens,
				TotalTokens:      mo.TokenUsage.TotalTokens,
			})
		} else if rawMsg != nil && rawMsg.ResponseMeta != nil && rawMsg.ResponseMeta.Usage != nil && rawMsg.ResponseMeta.Usage.TotalTokens > 0 {
			owner.Tokens = orchMergeTokenUsage(owner.Tokens, rawMsg.ResponseMeta.Usage)
		}
	}

	payload := map[string]any{
		"kind": "end", "key": span.key, "comp": span.comp, "ms": ms, "owner": ownerKey,
	}
	switch {
	case span.comp == "Tool":
		payload["tool"] = span.key
		owner.ToolCalls = append(owner.ToolCalls, OrchToolTrace{Name: span.key, MS: ms})
	case isTop:
		owner.MS = ms
		owner.Status = "success"
		if errMsg != "" {
			owner.Status = "error"
			owner.Error = errMsg
		} else if len(span.chunks) > 0 {
			owner.Content = orchSpanContent(span.chunks)
		}
		payload["name"] = owner.Name
		if owner.Content != "" {
			payload["content"] = owner.Content
		}
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	h.safeEmit("node", payload)
}

func (h *orchTraceHandler) safeEmit(event string, payload any) {
	if h.emit == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emit(event, payload)
}

// NodeTraces 按首次出现顺序返回节点摘要
func (h *orchTraceHandler) NodeTraces() []*OrchNodeTrace {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]*OrchNodeTrace, 0, len(h.order))
	for _, key := range h.order {
		out = append(out, h.owners[key])
	}
	return out
}

// orchSpanContent 按框架拼接语义合并分片消息
func orchSpanContent(chunks []*schema.Message) string {
	merged := ""
	if len(chunks) == 1 {
		merged = chunks[0].Content
	} else if m, err := schema.ConcatMessages(chunks); err == nil {
		merged = m.Content
	} else {
		var sb strings.Builder
		for _, c := range chunks {
			sb.WriteString(c.Content)
		}
		merged = sb.String()
	}
	return orchDedupeDoubledContent(merged)
}

// orchDedupeDoubledContent 部分模型流式的用量分片会携带完整消息副本,
// 拼接结果呈 X+X 形态; 精确重复时取单份
func orchDedupeDoubledContent(content string) string {
	n := len(content)
	if n%2 != 0 {
		return content
	}
	if content[:n/2] == content[n/2:] {
		return content[:n/2]
	}
	return content
}

func orchMergeTokenUsage(a, b *schema.TokenUsage) *schema.TokenUsage {
	if a == nil {
		cp := *b
		return &cp
	}
	a.PromptTokens += b.PromptTokens
	a.CompletionTokens += b.CompletionTokens
	a.TotalTokens += b.TotalTokens
	return a
}
