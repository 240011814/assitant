package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
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
	// sessionVars 供模板/系统提示词渲染 (current_time / user_id / user_profile / chat_history)
	sessionVars func() map[string]any
	// lookupAgent 读取子Agent 引用的已配置 Agent; 缺省时回退查库 (便于单测替换)
	lookupAgent func(id uint) (*coremodel.AIAgent, error)
}

type orchestrationCompiler struct {
	deps compilerDeps
	dsl  *OrchestrationDSL
	// trace 调试运行的事件通道: 编译期把它注入委派工具, 让子Agent 的委派过程
	// 也能进入调试摘要 (普通校验/生产编译为 nil)
	trace *orchTraceHandler

	adj     map[string][]string // source -> targets (全部连线, 含子Agent 委派边)
	flowAdj map[string][]string // source -> targets (仅主流连线, 不含子Agent)
	flowIn  map[string]int      // target -> 主流入度 (不含子Agent 委派边)
	inDeg   map[string]int
	outDeg  map[string]int
	nodeMap map[string]*OrchestrationNode
	topo    []string
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

// flowInOf / flowOutOf 主流入度/出度: 不含子Agent 委派边与子Agent 节点自身
// (子Agent 只是主 Agent 的委派挂载, 不参与主流程的入口/出口判定)
func (c *orchestrationCompiler) flowInOf(id string) int {
	return c.flowIn[id]
}

func (c *orchestrationCompiler) flowOutOf(id string) int {
	return len(c.flowAdj[id])
}

// lookupAgent 读取子Agent 引用的已有 Agent (deps 未注入时回退查库)
func (c *orchestrationCompiler) lookupAgent(id uint) (*coremodel.AIAgent, error) {
	if c.deps.lookupAgent != nil {
		return c.deps.lookupAgent(id)
	}
	if DB == nil {
		return nil, errors.New("数据库未初始化")
	}
	var agent coremodel.AIAgent
	if err := DB.First(&agent, id).Error; err != nil {
		return nil, err
	}
	return &agent, nil
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
	c.flowIn = make(map[string]int)
	for src, ts := range c.adj {
		// 子Agent 节点及其委派边完全脱离主流: 既不计入也不传递 (子Agent 无出边,
		// 但违规出边时不能让它把下游节点带进拓扑, 否则报错信息会变成"成环")
		if isSub[src] {
			continue
		}
		for _, t := range ts {
			if isSub[t] {
				continue
			}
			c.flowAdj[src] = append(c.flowAdj[src], t)
			c.flowIn[t]++
		}
	}
	c.topo = make([]string, 0, flowNodeCount)
	deg := make(map[string]int, flowNodeCount)
	for id := range c.nodeMap {
		if isSub[id] {
			continue
		}
		deg[id] = c.flowIn[id]
	}
	queue := make([]string, 0, len(deg))
	for id, d := range deg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	// Kahn 的初始队列来自 map 迭代, 排序后拓扑序才稳定 (编译期构建顺序影响
	// 委派工具命名与调试事件的先后, 不能随机)
	sort.Strings(queue)
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
		inDeg, outDeg := c.flowInOf(id), c.flowOutOf(id)
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
	// trace 本次编译注入的调试追踪 handler (委派工具持有同一实例,
	// 因此运行前 setEmit 即可把子Agent 的委派事件推送到 SSE)
	trace *orchTraceHandler
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

	// 子Agent 节点不参与主流, 但需要在调试摘要里出现 (委派事件由委派工具推送),
	// 因此把它们的 key/名称一并登记
	for id, n := range c.nodeMap {
		if n.Type == OrchNodeSubAgent {
			nodeKeys[id] = n.Name
		}
	}

	// 编译摘要日志: 一眼看出每个 Agent 挂了几个委派子Agent、系统提示词里是否含委派指引
	for _, id := range c.topo {
		n := c.nodeByID(id)
		if n.Type != OrchNodeAgent {
			continue
		}
		var cfg OrchAgentConfig
		if len(n.Config) > 0 {
			_ = json.Unmarshal(n.Config, &cfg)
		}
		subs := c.subAgentIDsOf(id)
		orchLog("compile mode=%s node=%s tools=%v subagents=%v max_step=%d",
			c.modeName(hasBranch, hasMerge), id, cfg.Tools, subs, maxStepOf(cfg))
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
	return &compiledOrchestration{runnable: runnable, mode: "chain", nodeKeys: nodeKeys, trace: c.trace}, nil
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
	return &compiledOrchestration{runnable: runnable, mode: "graph", nodeKeys: nodeKeys, trace: c.trace}, nil
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
	return &compiledOrchestration{runnable: runnable, mode: "workflow", nodeKeys: nodeKeys, trace: c.trace}, nil
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

// orchDelegationGuide 生成主 Agent 系统提示词里的"子Agent 委派"章节。
// 委派工具只在工具表里出现 (描述为空时更是只看到工具名), 模型往往压根不调用;
// 把子Agent 及其职责显式写进系统提示词是主管模式的必要一环。
// nameOf/titleOf/descOf 按节点 id 取显示名、被引用 Agent 标题与职责说明。
func orchDelegationGuide(subIDs []string, nameOf, titleOf, descOf func(string) string) string {
	if len(subIDs) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString("\n\n【子Agent 委派】\n你可以把子任务委派给以下子Agent, 由它们独立完成后把结果交回给你。")
	sb.WriteString("当任务属于某个子Agent 的职责(或用户明确点名该角色)时, 先用对应的子Agent 完成该部分, 再基于返回结果作答; 不需要时不要调用。\n")
	for i, id := range subIDs {
		name := strings.TrimSpace(nameOf(id))
		title := strings.TrimSpace(titleOf(id))
		// 节点名多为默认的「子Agent」, 用被引用 Agent 的标题补足可辨识度
		if title != "" && title != name {
			name = fmt.Sprintf("%s(%s)", name, title)
		}
		if name == "" {
			name = id
		}
		desc := strings.TrimSpace(descOf(id))
		if desc == "" {
			desc = "职责见其名称与系统提示词"
		}
		sb.WriteString(fmt.Sprintf("- 子Agent「%s」: %s (在本轮直接调用工具 subagent_%d, 参数 task 写清要它完成的具体任务; 不要只说已委派却没有调用工具)\n",
			name, desc, i+1))
	}
	return sb.String()
}

// orchSubAgentDesc 解析子Agent 节点用于提示词的职责说明 (委派指引里的职责描述):
// 优先节点「委派说明」, 其次被引用 Agent 的委派说明, 再次 Agent 简介/标题, 最后节点 id。
// 同名子Agent 靠它区分, 否则三个都叫「子Agent」时模型无法选择。
func (c *orchestrationCompiler) orchSubAgentDesc(id string) string {
	n := c.nodeByID(id)
	if n == nil {
		return ""
	}
	var cfg OrchSubAgentConfig
	if len(n.Config) > 0 {
		_ = json.Unmarshal(n.Config, &cfg)
	}
	desc := strings.TrimSpace(cfg.Description)
	if cfg.AgentID > 0 {
		if agent, err := c.lookupAgent(uint(cfg.AgentID)); err == nil {
			if desc == "" {
				desc = strings.TrimSpace(agent.DelegationDescription)
			}
			if desc == "" {
				desc = strings.TrimSpace(agent.Description)
			}
			if desc == "" {
				desc = strings.TrimSpace(agent.Title)
			}
		}
	}
	if desc == "" {
		if name := strings.TrimSpace(n.Name); name != "" && name != n.ID {
			desc = name
		} else {
			desc = n.ID
		}
	}
	return desc
}

// orchSubAgentTitle 被引用 Agent 的标题 (子Agent 节点名常是默认的「子Agent」, 需要它来区分)
func (c *orchestrationCompiler) orchSubAgentTitle(id string) string {
	n := c.nodeByID(id)
	if n == nil {
		return ""
	}
	var cfg OrchSubAgentConfig
	if len(n.Config) > 0 {
		_ = json.Unmarshal(n.Config, &cfg)
	}
	if cfg.AgentID == 0 {
		return ""
	}
	agent, err := c.lookupAgent(uint(cfg.AgentID))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(agent.Title)
}

// orchSubAgentName 子Agent 节点显示名 (提示词里用)
func (c *orchestrationCompiler) orchSubAgentName(id string) string {
	if n := c.nodeByID(id); n != nil {
		if name := strings.TrimSpace(n.Name); name != "" {
			return name
		}
	}
	return id
}

// orchSubAgentKeysOf 返回子Agent 节点 id 集合: 调试事件的归属要用它区分
// "子Agent 内部事件(归子Agent)" 与 "委派工具事件(归主 Agent)"
func orchSubAgentKeysOf(dsl *OrchestrationDSL) map[string]bool {
	keys := make(map[string]bool)
	for i := range dsl.Nodes {
		if dsl.Nodes[i].Type == OrchNodeSubAgent && dsl.Nodes[i].ID != "" {
			keys[dsl.Nodes[i].ID] = true
		}
	}
	return keys
}

func maxStepOf(cfg OrchAgentConfig) int {
	if cfg.MaxIterations > 0 {
		return cfg.MaxIterations
	}
	return 25
}

// modeName 编译形态名 (日志用)
func (c *orchestrationCompiler) modeName(hasBranch, hasMerge bool) string {
	switch {
	case hasMerge:
		return "workflow"
	case hasBranch:
		return "graph"
	default:
		return "chain"
	}
}

// orchChatHistoryFromVars 取出多轮调试传入的历史 (schema 消息序列), 无历史时返回 nil
func (c *orchestrationCompiler) orchChatHistoryFromVars() []*schema.Message {
	if c.deps.sessionVars == nil {
		return nil
	}
	vars := c.deps.sessionVars()
	if vars == nil {
		return nil
	}
	hist, _ := vars[orchSessionVarsHistoryKey].([]*schema.Message)
	return hist
}

// orchSessionVarsHistoryKey sessionVars 里多轮历史的键 (避免与模板变量混用)
const orchSessionVarsHistoryKey = "chat_history"

// OrchChatTurnsToMessages 把调试请求里的历史转为模型消息序列:
// 只保留 user/assistant 文本, 丢掉空内容与非对话角色, 保证序列合法 (Ark 严格校验)
func OrchChatTurnsToMessages(turns []coremodel.ChatTurn) []*schema.Message {
	out := make([]*schema.Message, 0, len(turns))
	for _, t := range turns {
		content := strings.TrimSpace(t.Content)
		if content == "" {
			continue
		}
		switch t.Role {
		case "user":
			out = append(out, schema.UserMessage(content))
		case "assistant":
			out = append(out, schema.AssistantMessage(content, nil))
		}
	}
	return out
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

// subAgentIDsOf 返回挂载在主 Agent 下的子Agent 节点 id (按名称+id 排序, 保证委派工具命名稳定)
func (c *orchestrationCompiler) subAgentIDsOf(parentID string) []string {
	var ids []string
	for _, t := range c.adj[parentID] {
		if n := c.nodeByID(t); n != nil && n.Type == OrchNodeSubAgent {
			ids = append(ids, t)
		}
	}
	// 同名节点必须再用 id 兜底: sort.Slice 不稳定, 只比名称会让同名子Agent 顺序随机
	sort.Slice(ids, func(i, j int) bool {
		ni, nj := c.nodeByID(ids[i]), c.nodeByID(ids[j])
		if ni.Name != nj.Name {
			return ni.Name < nj.Name
		}
		return ids[i] < ids[j]
	})
	return ids
}

// orchStreamToolCallCheckerHook 供单测替换 (置 nil 可复现 eino 默认 checker 的漏判行为)
var orchStreamToolCallCheckerHook = orchStreamToolCallChecker

// orchStreamToolCallChecker 判断流式输出里是否包含工具调用。
// eino 默认的 firstChunkStreamToolCallChecker 只看第一个分片: 第一块是文本就直接判定
// "没有工具调用", 而 GLM/Claude 这类模型是"先输出文本, 再给 tool_calls",
// 结果委派工具永远不被执行, ReAct 一轮就结束 (线上症状: 子Agent 从不触发, 只回一句开场白)。
// 因此这里扫描完整流 (纯判定, 不做转发: 分支拿到的流是"缓冲后"的, 见下)。
func orchStreamToolCallChecker(_ context.Context, sr *schema.StreamReader[*schema.Message]) (bool, error) {
	defer sr.Close()
	for {
		msg, err := sr.Recv()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if msg == nil {
			continue
		}
		if len(msg.ToolCalls) > 0 {
			return true, nil
		}
	}
}

// orchHandlerKey ctx key: 承载编排调试 handler
type orchHandlerKey struct{}

// withOrchHandler 把 handler 放进 ctx (DebugRun 使用)
func withOrchHandler(ctx context.Context, h *orchTraceHandler) context.Context {
	return context.WithValue(ctx, orchHandlerKey{}, h)
}

// orchHandlerFromContext 从 ctx 取回 handler (没有调试运行时为 nil)
func orchHandlerFromContext(ctx context.Context) *orchTraceHandler {
	if ctx == nil {
		return nil
	}
	h, _ := ctx.Value(orchHandlerKey{}).(*orchTraceHandler)
	return h
}

// orchNewReactAgent 主 Agent 与子Agent 共用的 ReAct 构建入口
func orchNewReactAgent(ctx context.Context, key, systemPrompt string, chatModel model.ToolCallingChatModel, tools []tool.BaseTool, maxStep int) (*react.Agent, error) {
	return react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		// 必须自定义: 默认实现只看首个分片, 对"先文本后 tool_calls"的模型会漏掉工具调用
		StreamToolCallChecker: orchStreamToolCallCheckerHook,
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
// parentID 是委派它的主 Agent 节点 id (进度事件的 owner)
func (c *orchestrationCompiler) buildSubReactAgent(ctx context.Context, id, parentID string, cfg OrchSubAgentConfig) (*react.Agent, error) {
	instruction := strings.TrimSpace(cfg.SystemPrompt)
	desc := strings.TrimSpace(cfg.Description)
	if cfg.AgentID > 0 {
		// 经 lookupAgent 取被引用 Agent (不直接用全局 DB, 便于单测替换与失败定位)
		dbAgent, err := c.lookupAgent(uint(cfg.AgentID))
		if err != nil {
			return nil, fmt.Errorf("子Agent 节点 %s 引用的 Agent %d 读取失败: %w", id, cfg.AgentID, err)
		}
		instruction = dbAgent.SystemPrompt
		if desc == "" {
			desc = dbAgent.Description
		}
	}
	if instruction == "" {
		return nil, fmt.Errorf("子Agent 节点 %s 缺少可用的系统提示词", id)
	}
	vars := c.deps.sessionVars()
	instruction, err := orchRenderTemplate(id, instruction, vars)
	if err != nil {
		return nil, fmt.Errorf("子Agent 节点 %s 系统提示词渲染失败: %w", id, err)
	}
	instruction = orchInjectRuntimeContext(instruction, vars)
	chatModel, err := c.deps.getModel(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("子Agent 节点 %s 获取模型失败: %w", id, err)
	}
	// 进度心跳: 子Agent 执行期间(实测可达 60s+)持续向前端推事件, 避免界面像卡死
	chatModel = newOrchSubAgentProgress(id, c.orchSubAgentName(id), parentID, c.trace, chatModel)
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
	name     string
	subID    string // 子Agent 节点 id, 用于调试事件归属
	subName  string
	desc     string
	parentID string // 挂载它的主 Agent 节点 id (调试摘要里的 owner)
	agent    *react.Agent
	trace    *orchTraceHandler // 调试追踪 handler (非调试运行时为 nil)
}

func newOrchDelegateTool(name, subID, subName, parentID, desc string, agent *react.Agent, trace *orchTraceHandler) *orchDelegateTool {
	return &orchDelegateTool{name: name, subID: subID, subName: subName, parentID: parentID, desc: desc, agent: agent, trace: trace}
}

// orchSubAgentProgress 子Agent 的进度通道: 子Agent 的 ReAct 内部事件现在能正确归属,
// 但模型调用是同步 Generate (几十秒无任何分片), 所以这里额外推"心跳"事件,
// 前端在被委派期间能看到子Agent 节点正在运行 + 心跳耗时, 不会误以为卡死。
type orchSubAgentProgress struct {
	subID   string
	subName string
	owner   string
	trace   *orchTraceHandler
	model   model.ToolCallingChatModel
}

func newOrchSubAgentProgress(subID, subName, owner string, trace *orchTraceHandler, inner model.ToolCallingChatModel) *orchSubAgentProgress {
	return &orchSubAgentProgress{subID: subID, subName: subName, owner: owner, trace: trace, model: inner}
}

func (p *orchSubAgentProgress) emit(status string, extra map[string]any) {
	if p.trace == nil {
		return
	}
	payload := map[string]any{
		"kind": "node", "key": p.subID, "name": p.subName, "comp": "DelegateTool",
		"owner": p.owner, "delegated": true, "status": status,
	}
	for k, v := range extra {
		payload[k] = v
	}
	p.trace.emitNodeEvent(payload)
}

// startHeartbeat 执行期间按固定间隔推 status=running 的心跳 (前端可显示已运行秒数)
func (p *orchSubAgentProgress) startHeartbeat(ctx context.Context) func() {
	if p.trace == nil {
		return func() {}
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		start := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(start).Milliseconds()
				orchLog("delegate heartbeat subagent=%s elapsed=%dms", p.subID, elapsed)
				p.emit("running", map[string]any{"ms": elapsed})
			}
		}
	}()
	return cancel
}

func (p *orchSubAgentProgress) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	stop := p.startHeartbeat(ctx)
	defer stop()
	msg, err := p.model.Generate(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

func (p *orchSubAgentProgress) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, err := p.model.Stream(ctx, in, opts...)
	if err != nil || p.trace == nil {
		return sr, err
	}
	// 实时透出子Agent 的思考与正文增量 (子Agent 走流式时用户即可看到它在写什么)
	first := true
	return schema.StreamReaderWithConvert(sr, func(m *schema.Message) (*schema.Message, error) {
		if m != nil {
			if m.ReasoningContent != "" {
				p.trace.emitReasoningDelta(m.ReasoningContent)
			}
			if m.Content != "" {
				p.trace.emitDelta(m.Content)
				p.trace.markStreamed(len(m.Content), first)
				first = false
			}
		}
		return m, nil
	}), nil
}

func (p *orchSubAgentProgress) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	inner, err := p.model.WithTools(tools)
	if err != nil {
		return nil, err
	}
	return &orchSubAgentProgress{subID: p.subID, subName: p.subName, owner: p.owner, trace: p.trace, model: inner}, nil
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
	started := time.Now()
	orchLog("delegate 开始 tool=%s 子Agent=%s(%s) 任务=%.80q", t.name, t.subID, t.subName, task)
	// 调试事件: 子Agent 节点本身不参与主流, 没有自己的 compose 节点 span,
	// 由委派工具在上游 Agent 的 span 内推送 "已委派 + 任务内容"
	t.emitEvent(map[string]any{
		"kind": "node", "key": t.subID, "name": t.subName, "comp": "DelegateTool",
		"owner": t.parentID, "status": "running", "delegated": true, "task": task,
	})
	runCtx := ctx
	// 走流式: 子Agent 的思考/正文可以实时透出 (Generate 不会产生任何增量)
	sr, err := t.agent.Stream(runCtx, []*schema.Message{schema.UserMessage(task)})
	if err != nil {
		orchLog("delegate 失败 tool=%s 子Agent=%s 耗时=%dms err=%v", t.name, t.subID, time.Since(started).Milliseconds(), err)
		t.emitEvent(map[string]any{
			"kind": "node", "key": t.subID, "name": t.subName, "comp": "DelegateTool",
			"owner": t.parentID, "status": "error", "delegated": true, "error": err.Error(),
		})
		return "", fmt.Errorf("子Agent「%s」执行失败: %w", t.subName, err)
	}
	defer sr.Close()
	msg, err := schema.ConcatMessageStream(sr)
	if err != nil {
		orchLog("delegate 失败 tool=%s 子Agent=%s 耗时=%dms err=%v", t.name, t.subID, time.Since(started).Milliseconds(), err)
		t.emitEvent(map[string]any{
			"kind": "node", "key": t.subID, "name": t.subName, "comp": "DelegateTool",
			"owner": t.parentID, "status": "error", "delegated": true, "error": err.Error(),
		})
		return "", fmt.Errorf("子Agent「%s」执行失败: %w", t.subName, err)
	}
	resultLen := 0
	if msg != nil {
		resultLen = len(msg.Content)
	}
	orchLog("delegate 完成 tool=%s 子Agent=%s 耗时=%dms 结果长度=%d", t.name, t.subID, time.Since(started).Milliseconds(), resultLen)
	t.emitEvent(map[string]any{
		"kind": "node", "key": t.subID, "name": t.subName, "comp": "DelegateTool",
		"owner": t.parentID, "status": "success", "delegated": true, "content": msg.Content,
	})
	return msg.Content, nil
}

// emitEvent 把委派过程并入调试事件流 (emit 由运行期注入, 未注入时只更新摘要表)
func (t *orchDelegateTool) emitEvent(payload map[string]any) {
	if t.trace == nil {
		return
	}
	t.trace.emitNodeEvent(payload)
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
	// 挂在主 Agent 下的子Agent 编译为委派工具: 主 Agent 的 ReAct 循环按需调用,
	// 工具名按节点名称排序后的序号生成 (subagent_1, subagent_2 ...), 编译期稳定
	subIDs := c.subAgentIDsOf(n.ID)
	for i, subID := range subIDs {
		subNode := c.nodeByID(subID)
		var subCfg OrchSubAgentConfig
		if len(subNode.Config) > 0 {
			if err := json.Unmarshal(subNode.Config, &subCfg); err != nil {
				return nil, fmt.Errorf("子Agent 节点 %s 配置解析失败: %w", subID, err)
			}
		}
		subAgent, err := c.buildSubReactAgent(ctx, subID, n.ID, subCfg)
		if err != nil {
			return nil, err
		}
		toolName := fmt.Sprintf("subagent_%d", i+1)
		desc := strings.TrimSpace(subCfg.Description)
		if desc == "" {
			desc = c.orchSubAgentDesc(subID)
		}
		agentTools = append(agentTools, newOrchDelegateTool(toolName, subID, subNode.Name, n.ID, desc, subAgent, c.trace))
	}

	vars := c.deps.sessionVars()
	systemPrompt := cfg.SystemPrompt
	if systemPrompt != "" {
		systemPrompt, err = orchRenderTemplate(n.ID, systemPrompt, vars)
		if err != nil {
			return nil, fmt.Errorf("Agent 节点 %s 系统提示词渲染失败: %w", n.ID, err)
		}
	}
	// 主管模式: 把挂载的子Agent 及其职责写进系统提示词, 否则模型常常完全不去委派
	if len(subIDs) > 0 {
		systemPrompt += orchDelegationGuide(subIDs, c.orchSubAgentName, c.orchSubAgentTitle, c.orchSubAgentDesc)
	}
	// 运行时上下文 (当前时间/用户ID): 提示词没引用 {{.current_time}} 时也要让模型知道当前时间
	systemPrompt = orchInjectRuntimeContext(systemPrompt, vars)
	maxStep := maxStepOf(cfg)

	// 多轮调试: 把历史消息拼在本轮输入之前, 让主 Agent 记得之前说过什么
	history := c.orchChatHistoryFromVars()
	agent, err := react.NewAgent(ctx, &react.AgentConfig{
		ToolCallingModel: chatModel,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: agentTools,
		},
		// 与子Agent 一致: 默认 checker 只看首个分片, 会漏掉"先文本后 tool_calls"的模型
		StreamToolCallChecker: orchStreamToolCallCheckerHook,
		MessageModifier: func(_ context.Context, input []*schema.Message) []*schema.Message {
			msgs := orchNormalizeModelInput(input)
			if len(history) > 0 {
				// 历史在前, 本轮在后; 每轮都按"历史+本轮"重建, 避免 ReAct 循环里重复累加
				msgs = append(append([]*schema.Message{}, history...), msgs...)
			}
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

// orchInjectRuntimeContext 给系统提示词补充运行时上下文 (用户画像/当前时间/用户ID)。
// 普通 Agent 对话的 Instruction 里本来就带「{user_profile}」和「当前时间: {current_time}」,
// 但编排节点的系统提示词只有显式写了 {{.user_profile}}/{{.current_time}} 才会渲染出来,
// 导致「你是我的助手」这类提示词下模型既不知道当前时间、也拿不到用户画像。
// 与普通对话保持一致: 无论提示词有没有引用, 都自动补上; 已在提示词里出现的内容不重复追加。
func orchInjectRuntimeContext(prompt string, vars map[string]any) string {
	var blocks []string

	// 用户画像: 与普通 Agent 对话一致, 有就带上 (提示词已引用则不重复)
	if profile, _ := vars["user_profile"].(string); strings.TrimSpace(profile) != "" {
		profile = strings.TrimSpace(profile)
		if !strings.Contains(prompt, profile) {
			blocks = append(blocks, profile)
		}
	}

	// 当前时间 / 用户ID: 提示词已渲染出当前时间时, 视为已自带运行时上下文, 不再追加
	if now, _ := vars["current_time"].(string); now != "" && !strings.Contains(prompt, now) {
		runtime := []string{"当前时间: " + now}
		if uid, ok := vars["user_id"]; ok {
			runtime = append(runtime, fmt.Sprintf("当前用户ID: %v", uid))
		}
		blocks = append(blocks, strings.Join(runtime, "\n"))
	}

	if len(blocks) == 0 {
		return prompt
	}
	block := strings.Join(blocks, "\n\n")
	if strings.TrimSpace(prompt) == "" {
		return block
	}
	return prompt + "\n\n" + block
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
	// Delegated 子Agent 节点是否被主 Agent 委派过 (仅子Agent 节点有意义)
	Delegated bool `json:"delegated,omitempty"`
	// Owner 子Agent 节点所属的主 Agent 节点 key
	Owner string `json:"owner,omitempty"`
	// Task 主 Agent 委派给子Agent 的任务原文
	Task string `json:"task,omitempty"`
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
	// subNodes 子Agent 节点 id 集合: 子Agent 内部事件归它自己 (而不是外层主 Agent),
	// 这样前端在被委派期间就能看到该节点在运行、以及它自己的耗时/用量
	subNodes map[string]bool
	owners   map[string]*OrchNodeTrace
	order    []string
	mu       sync.Mutex
	emit     func(event string, payload any)
	started  time.Time
	// streamed 已被实时下发的文本长度 (模型节点流式回调直推), 图级输出据此去重
	streamed int
}

// orchStreamProbeStart 探针基准时间 (仅 ORCH_DEBUG 下使用)
var orchStreamProbeStart = time.Now()

// orchLog 编排调试日志: 设 ORCH_LOG=1 (或 ORCH_DEBUG=1) 后打到 stderr,
// ORCH_LOG_FILE 可同时落文件; 用于排查"事件推了/没推、归属对不对"
func orchLog(format string, args ...any) {
	if os.Getenv("ORCH_LOG") == "" && os.Getenv("ORCH_DEBUG") == "" {
		return
	}
	line := fmt.Sprintf("[orch] "+format, args...)
	fmt.Fprintln(os.Stderr, line)
	if path := os.Getenv("ORCH_LOG_FILE"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			fmt.Fprintln(f, line)
			_ = f.Close()
		}
	}
}

// orchDebugTrace 打出每条事件的归属判定, 便于确认 owner 是否正确
func orchDebugTrace(event string, payload map[string]any) {
	if os.Getenv("ORCH_LOG") == "" && os.Getenv("ORCH_DEBUG") == "" {
		return
	}
	orchLog("event=%s kind=%v key=%v owner=%v comp=%v status=%v task=%.40q content_len=%d",
		event, payload["kind"], payload["key"], payload["owner"], payload["comp"],
		payload["status"], toStr(payload["task"]), len(toStr(payload["content"])))
}

func toStr(v any) string {
	s, _ := v.(string)
	return s
}

func newOrchTraceHandler(nodeKeys map[string]string, subNodes map[string]bool, emit func(event string, payload any)) *orchTraceHandler {
	if subNodes == nil {
		subNodes = map[string]bool{}
	}
	return &orchTraceHandler{
		nodeKeys: nodeKeys,
		subNodes: subNodes,
		owners:   make(map[string]*OrchNodeTrace),
		emit:     emit,
		started:  time.Now(),
	}
}

// setEmit 运行前注入事件出口 (编译期先建 handler 以注入委派工具, emit 在编译成功后才有意义)
func (h *orchTraceHandler) setEmit(emit func(event string, payload any)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.emit = emit
}

// emitDelta 实时推送模型增量文本 (模型节点流式回调里调用)
func (h *orchTraceHandler) emitDelta(content string) {
	if content == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.emit != nil {
		h.emit("delta", map[string]any{"content": content})
	}
}

// emitReasoningDelta 推送模型思考(reasoning)增量: 前端折叠展示"思考过程"
func (h *orchTraceHandler) emitReasoningDelta(content string) {
	if content == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.emit != nil {
		h.emit("reasoning", map[string]any{"content": content})
	}
}

// markStreamed 设定/追加"已实时下发"的文本长度 (按当前模型调用计, 图级输出据此去重)
func (h *orchTraceHandler) markStreamed(n int, replace bool) {
	if n <= 0 {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if replace {
		h.streamed = n
		return
	}
	h.streamed += n
}

// TakeStreamed 取出并清零"已实时下发"的长度
func (h *orchTraceHandler) TakeStreamed() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := h.streamed
	h.streamed = 0
	return n
}

// orchSkipStreamed 按"已实时下发"的长度跳过重复内容并递减计数:
// 同一段文本既走了模型节点回调(已实时推)又走了图级输出(此处)时, 保证只下发一次
func orchSkipStreamed(h *orchTraceHandler, content string) string {
	skip := h.TakeStreamed()
	if skip <= 0 {
		return content
	}
	if skip >= len(content) {
		h.markStreamed(skip-len(content), true)
		return ""
	}
	h.markStreamed(0, true)
	return content[skip:]
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

// ownerKeyFor 解析 span 归属的编排节点:
//  1. 自身是顶层节点 -> 自身 (子Agent 的 <id>.react span 归子Agent 自己)
//  2. 自身是子Agent 的 react 内部节点 (如 <subID>.model) -> 该子Agent 节点
//  3. 否则取剩余栈中最近的顶层节点 (react 内部子节点/工具调用归所在 Agent 节点);
//     但子Agent 的委派工具 span (subagent_N) 要归委派它的主 Agent, 不能被子Agent 抢走
func ownerKeyFor(span *orchSpan, rest []*orchSpan, nodeKeys map[string]string, subNodes map[string]bool) string {
	if _, ok := nodeKeys[span.key]; ok {
		return span.key
	}
	// 子Agent 的 react/model/tools 子节点名形如 "<subID>.model"
	if i := strings.LastIndex(span.key, "."); i > 0 {
		if prefix := span.key[:i]; subNodes[prefix] {
			return prefix
		}
	}
	for i := len(rest) - 1; i >= 0; i-- {
		if _, ok := nodeKeys[rest[i].key]; ok {
			return rest[i].key
		}
	}
	return ""
}

// isSubAgentInternal 判断 span key 是否属于某个子Agent 的内部子节点 (形如 "<subID>.model")。
// 这些模型调用由 orchSubAgentProgress 负责实时透出, 图回调不再重复推送, 避免叠字。
// subNodes 在 handler 构造后只读, 无需加锁。
func (h *orchTraceHandler) isSubAgentInternal(key string) bool {
	if i := strings.LastIndex(key, "."); i > 0 {
		return h.subNodes[key[:i]]
	}
	return false
}

func (h *orchTraceHandler) OnStart(ctx context.Context, info *callbacks.RunInfo, _ callbacks.CallbackInput) context.Context {
	if info == nil {
		return ctx
	}
	span := &orchSpan{key: info.Name, comp: string(info.Component), start: time.Now()}
	nextCtx := pushSpan(ctx, span)
	_, isTop := h.nodeKeys[info.Name]
	ownerKey := ownerKeyFor(span, stackOf(nextCtx), h.nodeKeys, h.subNodes)
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
	// 模型节点的回调流是"真实时"的 (图级节点拿到的是缓冲后的副本), 在这里把增量直推前端,
	// 用户才能在模型生成期间看到内容; 图级输出稍后到达时由 orchSkipStreamed 去重。
	// 子Agent 的模型增量已由其进度包装 (orchSubAgentProgress, 兼心跳) 实时透出,
	// 这里若再推一次会与包装器的增量交错重复 (症状: "用户用户现在现在…" 叠字), 故跳过。
	directStream := span.comp == "ChatModel" && !h.isSubAgentInternal(span.key)
	firstDelta := true
	for {
		chunk, err := output.Recv()
		if err != nil {
			break
		}
		if m := model.ConvCallbackOutput(chunk); m != nil {
			if directStream && m.Message != nil {
				// 思考(reasoning)与正文分开推: 前端可以折叠展示"思考过程"
				if m.Message.ReasoningContent != "" {
					h.emitReasoningDelta(m.Message.ReasoningContent)
				}
				if m.Message.Content != "" {
					h.emitDelta(m.Message.Content)
					// 计数按"本次模型调用"重置, 避免多个模型节点累计导致图级输出去重过度
					h.markStreamed(len(m.Message.Content), firstDelta)
					firstDelta = false
				}
			}
			mo = m
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
	ownerKey := ownerKeyFor(span, rest, h.nodeKeys, h.subNodes)
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
	orchDebugTrace("span-end", payload)
	// 运行日志: 工具调用与顶层节点结束这两类事件最有用, 逐条落日志
	if span.comp == "Tool" {
		orchLog("工具调用完成: %s 归属=%s 耗时=%dms%s", span.key, ownerKey, ms, orchErrSuffix(errMsg))
	} else if isTop {
		orchLog("节点完成: %s(%s) comp=%s 耗时=%dms tokens=%s 输出长度=%d%s",
			owner.Name, owner.Key, span.comp, ms, orchTokensText(owner.Tokens), len(owner.Content), orchErrSuffix(errMsg))
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

// ---------- 调试事件通道: 委派工具把子Agent 事件回推到本次运行的追踪 handler ----------

// emitNodeEvent 推送一条自定义 node 事件并记入摘要 (emit 未注入时静默跳过 SSE)
func (h *orchTraceHandler) emitNodeEvent(payload map[string]any) {
	orchDebugTrace("custom", payload)
	// 推送与摘要更新读取同一份 payload, 持同一把锁串行化
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.emit != nil {
		h.emit("node", payload)
	}
	h.recordDelegateLocked(payload)
}

// recordDelegateLocked 把委派事件并入子Agent 节点的调试摘要
// (子Agent 不是 compose 节点, 其摘要只能由委派工具按事件填充); 调用方需持 h.mu
func (h *orchTraceHandler) recordDelegateLocked(payload map[string]any) {
	key, _ := payload["key"].(string)
	if key == "" {
		return
	}
	name, _ := payload["name"].(string)
	owner, _ := payload["owner"].(string)
	status, _ := payload["status"].(string)
	t, ok := h.owners[key]
	if !ok {
		t = &OrchNodeTrace{Key: key, Name: name, Comp: "DelegateTool"}
		h.owners[key] = t
		h.order = append(h.order, key)
	}
	t.Delegated = true
	if owner != "" {
		t.Owner = owner
	}
	if content, _ := payload["content"].(string); content != "" {
		t.Content = content
	}
	if errMsg, _ := payload["error"].(string); errMsg != "" {
		t.Error = errMsg
	}
	if task, _ := payload["task"].(string); task != "" {
		t.Task = task
	}
	if status != "" {
		t.Status = status
	}
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
