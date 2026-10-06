package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	coremodel "backend/model"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
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
// branch(条件路由) / router(LLM 意图路由) / merge(多路合并) / extract(JSON 字段提取) /
// suborch(子编排引用) / end(输出)

// compilerDeps 编译期依赖, 由 AIAgentService 提供
type compilerDeps struct {
	// getModel 按 model code 构建 ToolCallingChatModel (空串用默认模型)
	getModel func(modelOverride string) (model.ToolCallingChatModel, error)
	// getModelRetry 同 getModel 但可覆盖 ark SDK 内建的模型调用重试次数
	// (retryTimes=nil 沿用默认)。未注入时 (旧 deps/单测) 回退 getModel, 重试取框架默认
	getModelRetry func(modelOverride string, retryTimes *int) (model.ToolCallingChatModel, error)
	// resolveModelCode 把模型 code 解析成真实 code (空串→当前默认模型的 code),
	// router 节点 token 记账归属用; 未注入时 (单测) 记账保留原值
	resolveModelCode func(modelOverride string) string
	// buildTool 按 ai_tools 表配置构建单个工具
	buildTool func(name string) (tool.BaseTool, error)
	// sessionVars 供模板/系统提示词渲染 (current_time / user_id / user_profile / chat_history)
	sessionVars func() map[string]any
	// lookupAgent 读取子Agent 引用的已配置 Agent; 缺省时回退查库 (便于单测替换)
	lookupAgent func(id uint) (*coremodel.AIAgent, error)
	// compileNested 编译子编排节点引用的已保存编排 (服务层提供: 查库+启用校验+递归编译);
	// chain 是当前编译链 (含正在编译的各编排 id, 不含 refID), 由编译器做循环引用/深度校验;
	// keyPrefix 用于给嵌套编排的节点 key 加前缀 (同一编排被多处引用时按节点区分实例)
	compileNested func(ctx context.Context, refID uint, chain []uint, keyPrefix string) (*compiledOrchestration, error)
	// nodePrompt 解析 Agent 节点的用户提示词版本 (user_prompts 表, 键: 编排 id+节点 id,
	// user_id 恒为 0 即编排全局共享)。存在启用版本时覆盖画布内联提示词,
	// 与普通 Agent 的用户提示词覆盖语义一致; 未命中回退内联
	nodePrompt func(nodeKey string) (string, bool)
}

type orchestrationCompiler struct {
	deps compilerDeps
	dsl  *OrchestrationDSL
	// trace 调试运行的事件通道: 编译期把它注入委派工具, 让子Agent 的委派过程
	// 也能进入调试摘要 (普通校验/生产编译为 nil)
	trace *orchTraceHandler
	// chatPreamble 编排对话模式的身份前言 (编排名称/简介), 追加进主 Agent 系统提示词;
	// 调试/校验运行为空
	chatPreamble string
	// chatMode 编排对话模式: 入口模板未引用 {{.Input}} 时把用户输入补进渲染结果,
	// 保证"用户说了什么"一定到达模型 (画布配置失误不能吞掉对话输入)
	chatMode bool
	// userID 运行发起人: router 节点的分类调用不经 compose 节点 span, token 记账
	// 在 buildRouterCond 里直接落库时归属用; 校验/单测构造为 0 (0 不记账)
	userID uint
	// orchChain 当前编译链上的编排 id (含自身, 草稿为空): 子编排循环引用/深度检测用
	orchChain []uint
	// extraNodeKeys/extraSubNodes 编译子编排节点时发现的嵌套节点 key 与子Agent 集合
	// (子编排 DSL 已加前缀, 不会与主编排冲突), 编译完成后并入 compiledOrchestration
	extraNodeKeys map[string]string
	extraSubNodes map[string]bool
	// extraNodeModels 嵌套编排的节点模型归属 (前缀化 key -> 模型 code)
	extraNodeModels map[string]string

	adj map[string][]string // source -> targets (全部非回边连线, 含子Agent 委派边)
	// loopEdges 循环回边: source (branch/router) -> 回边列表。
	// 回边不参与主流程的拓扑/度计算, 单独做受控环校验 (方向/上限/唯一性)
	loopEdges map[string][]OrchestrationEdge
	flowAdj   map[string][]string // source -> targets (仅主流连线, 不含子Agent)
	flowIn    map[string]int      // target -> 主流入度 (不含子Agent 委派边)
	inDeg     map[string]int
	outDeg    map[string]int
	nodeMap   map[string]*OrchestrationNode
	topo      []string

	// vars/varsResolved sessionVars 每次编译只解析一次。原实现在 buildAgentLambda /
	// buildSubReactAgent / buildTemplateLambda / orchChatHistoryFromVars 各调一次 deps.sessionVars(),
	// 每次都重建用户画像 (多次查库), 是编译期最大的重复开销
	vars         map[string]any
	varsResolved bool
	// agentCache 编译内子Agent 引用查询去重: 同一 Agent 被多个子Agent 节点引用时只查一次
	agentCache map[uint]*coremodel.AIAgent
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

// validateOrchestrationDSLDetailed 校验 DSL, 返回解析结果与带节点定位的错误列表
func validateOrchestrationDSLDetailed(definition string) (*OrchestrationDSL, []orchValidateIssue) {
	dsl, err := parseOrchestrationDSL(definition)
	if err != nil {
		return nil, []orchValidateIssue{{Message: err.Error()}}
	}
	c := &orchestrationCompiler{dsl: dsl}
	return dsl, c.validateIssues()
}

// resolvedSessionVars 返回本次编译的模板/提示词变量 (deps.sessionVars 结果在编译内只解析一次)
func (c *orchestrationCompiler) resolvedSessionVars() map[string]any {
	if !c.varsResolved {
		if c.deps.sessionVars != nil {
			c.vars = c.deps.sessionVars()
		} else {
			c.vars = map[string]any{}
		}
		c.varsResolved = true
	}
	return c.vars
}

// lookupAgent 读取子Agent 引用的已有 Agent (deps 未注入时回退查库);
// 同一编译内按 id 去重, 同一 Agent 被多个子Agent 节点引用时只查一次
func (c *orchestrationCompiler) lookupAgent(id uint) (*coremodel.AIAgent, error) {
	if a, ok := c.agentCache[id]; ok {
		return a, nil
	}
	var (
		agent *coremodel.AIAgent
		err   error
	)
	if c.deps.lookupAgent != nil {
		agent, err = c.deps.lookupAgent(id)
	} else if DB == nil {
		return nil, errors.New("数据库未初始化")
	} else {
		var row coremodel.AIAgent
		if err = DB.First(&row, id).Error; err == nil {
			agent = &row
		}
	}
	if err != nil {
		return nil, err
	}
	if c.agentCache == nil {
		c.agentCache = make(map[uint]*coremodel.AIAgent)
	}
	c.agentCache[id] = agent
	return agent, nil
}

// compiledOrchestration 编译结果: 可运行对象 + 调试事件归属所需的节点名集合
type compiledOrchestration struct {
	runnable compose.Runnable[*schema.Message, *schema.Message]
	mode     string // chain / graph / workflow
	// nodeKeys 参与归属的顶层节点 key -> 显示名 (不含 react 内部子节点;
	// 子编排节点会把嵌套编排的前缀化 key 一并并入)
	nodeKeys map[string]string
	// subNodes 子Agent 节点 id 集合 (含子编排内嵌套的), 调试事件归属用
	subNodes map[string]bool
	// nodeModels 节点 key -> 模型 code (空串=默认模型): token 用量按节点配置的模型落库
	nodeModels map[string]string
	// trace 本次编译注入的调试追踪 handler (委派工具持有同一实例,
	// 因此运行前 setEmit 即可把子Agent 的委派事件推送到 SSE)
	trace *orchTraceHandler
}

func (c *orchestrationCompiler) compile(ctx context.Context, deps compilerDeps) (*compiledOrchestration, error) {
	c.deps = deps
	c.extraNodeKeys = map[string]string{}
	c.extraSubNodes = map[string]bool{}
	c.extraNodeModels = map[string]string{}
	if errs := c.validate(); len(errs) > 0 {
		return nil, errors.New("编排定义校验失败: " + strings.Join(errs, "; "))
	}

	hasBranch, hasRouter, hasMerge := false, false, false
	for _, n := range c.dsl.Nodes {
		if n.Type == OrchNodeBranch {
			hasBranch = true
		}
		if n.Type == OrchNodeRouter {
			hasRouter = true
		}
		if n.Type == OrchNodeMerge {
			hasMerge = true
		}
	}
	// 循环回边只能出现在 graph 形态 (Workflow=AllPredecessor 不支持环, 校验已挡)
	hasLoop := len(c.loopEdges) > 0

	// 循环限步: 主流程节点数 + 每条回边的 (最大轮次 x 环长) + 余量。
	// 超限由 eino 运行时硬性截断; 精确的轮次语义由回边 cond 的强制退出保证
	maxRunSteps := 0
	if hasLoop {
		topoIndex := make(map[string]int, len(c.topo))
		for i, id := range c.topo {
			topoIndex[id] = i
		}
		maxRunSteps = len(c.topo) + 10
		for src, edges := range c.loopEdges {
			maxLoops := c.loopMaxLoops(src)
			for _, le := range edges {
				loopLen := topoIndex[src] - topoIndex[le.Target] + 1
				if loopLen < 1 {
					loopLen = 1
				}
				maxRunSteps += maxLoops * loopLen
			}
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
			c.modeName(hasBranch, hasRouter, hasMerge), id, cfg.Tools, subs, maxStepOf(cfg))
	}

	// 子Agent 集合: 本编排的 + 子编排节点编译时发现的嵌套的 (均按 id 调试事件归属)
	subNodes := orchSubAgentKeysOf(c.dsl)
	for id := range c.extraSubNodes {
		subNodes[id] = true
	}
	// 嵌套编排的前缀化节点 key 并入归属表: 嵌套节点的事件才能在调试摘要里独立成行
	for k, v := range c.extraNodeKeys {
		if _, ok := nodeKeys[k]; !ok {
			nodeKeys[k] = v
		}
	}

	// 节点模型归属: agent/subagent 节点记录各自配置的模型 code, 供用量统计按模型落库
	// (router 节点的分类调用不经 compose 节点 span, 用量无法按节点归属, 不在统计内)
	nodeModels := make(map[string]string, len(c.dsl.Nodes))
	for _, id := range c.topo {
		n := c.nodeByID(id)
		switch n.Type {
		case OrchNodeAgent:
			var cfg OrchAgentConfig
			if len(n.Config) > 0 {
				_ = json.Unmarshal(n.Config, &cfg)
			}
			nodeModels[id] = strings.TrimSpace(cfg.Model)
		}
	}
	for id := range c.nodeMap {
		if c.nodeMap[id].Type != OrchNodeSubAgent {
			continue
		}
		var cfg OrchSubAgentConfig
		if len(c.nodeMap[id].Config) > 0 {
			_ = json.Unmarshal(c.nodeMap[id].Config, &cfg)
		}
		nodeModels[id] = strings.TrimSpace(cfg.Model)
	}
	for k, v := range c.extraNodeModels {
		if _, ok := nodeModels[k]; !ok {
			nodeModels[k] = v
		}
	}

	var res *compiledOrchestration
	var err error
	switch {
	case hasMerge:
		res, err = c.compileWorkflow(lambdas, nodeKeys, subNodes)
	case hasBranch || hasRouter || hasLoop:
		res, err = c.compileGraph(ctx, lambdas, nodeKeys, subNodes, maxRunSteps)
	default:
		res, err = c.compileChain(lambdas, nodeKeys, subNodes)
	}
	if err != nil {
		return nil, err
	}
	res.nodeModels = nodeModels
	return res, nil
}

func (c *orchestrationCompiler) compileChain(lambdas map[string]*compose.Lambda, nodeKeys map[string]string, subNodes map[string]bool) (*compiledOrchestration, error) {
	chain := compose.NewChain[*schema.Message, *schema.Message]()
	for _, id := range c.topo {
		chain.AppendLambda(lambdas[id], compose.WithNodeName(id))
	}
	runnable, err := chain.Compile(context.Background(), compose.WithGraphName("orchestration"))
	if err != nil {
		return nil, fmt.Errorf("Chain 编译失败: %w", err)
	}
	return &compiledOrchestration{runnable: runnable, mode: "chain", nodeKeys: nodeKeys, subNodes: subNodes, trace: c.trace}, nil
}

func (c *orchestrationCompiler) compileGraph(ctx context.Context, lambdas map[string]*compose.Lambda, nodeKeys map[string]string, subNodes map[string]bool, maxRunSteps int) (*compiledOrchestration, error) {
	g := compose.NewGraph[*schema.Message, *schema.Message]()
	for id, lambda := range lambdas {
		if err := g.AddLambdaNode(id, lambda, compose.WithNodeName(id)); err != nil {
			return nil, fmt.Errorf("添加节点 %s 失败: %w", id, err)
		}
	}
	_ = lambdas
	for _, id := range c.topo {
		n := c.nodeByID(id)
		// 入口/出口按主流度判断: 回边不计入 (回边目标若同时是入口, 仍需 START 供首轮进入)
		if c.flowInOf(id) == 0 {
			if err := g.AddEdge(compose.START, id); err != nil {
				return nil, fmt.Errorf("连接入口 %s 失败: %w", id, err)
			}
		}
		if c.flowOutOf(id) == 0 {
			if err := g.AddEdge(id, compose.END); err != nil {
				return nil, fmt.Errorf("连接出口 %s 失败: %w", id, err)
			}
		}
		if n.Type == OrchNodeBranch || n.Type == OrchNodeRouter {
			// 分支/路由节点到目标的路由由 GraphBranch 处理, 不建普通边
			// (回边目标同样由 cond 返回, eino 的 pregel 执行模式支持向上游路由)
			var cond func(_ context.Context, in *schema.Message) (string, error)
			var endNodes map[string]bool
			if n.Type == OrchNodeBranch {
				var cfg OrchBranchConfig
				if err := json.Unmarshal(n.Config, &cfg); err != nil {
					return nil, fmt.Errorf("分支节点 %s 配置解析失败: %w", id, err)
				}
				cases := make([]OrchBranchCase, len(cfg.Cases))
				copy(cases, cfg.Cases)
				defaultTarget := cfg.DefaultTarget
				cond = func(_ context.Context, in *schema.Message) (string, error) {
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
				endNodes = map[string]bool{}
				for _, cs := range cfg.Cases {
					endNodes[cs.Target] = true
				}
				endNodes[defaultTarget] = true
			} else {
				var cfg OrchRouterConfig
				if err := json.Unmarshal(n.Config, &cfg); err != nil {
					return nil, fmt.Errorf("路由节点 %s 配置解析失败: %w", id, err)
				}
				var err error
				if cond, err = c.buildRouterCond(id, cfg); err != nil {
					return nil, err
				}
				endNodes = map[string]bool{}
				for _, cs := range cfg.Cases {
					endNodes[cs.Target] = true
				}
				endNodes[cfg.DefaultTarget] = true
			}
			// 带循环回边的分支: 包装 cond, 命中回边时计数/推事件, 超过上限强制走退出目标
			if len(c.loopEdges[id]) > 0 {
				cond = c.wrapLoopCond(id, n.Name, cond)
			}
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
	compileOpts := make([]compose.GraphCompileOption, 0, 2)
	compileOpts = append(compileOpts, compose.WithGraphName("orchestration"))
	if maxRunSteps > 0 {
		// 循环图必须显式限步, 否则默认"节点数+10"跑不完多轮循环
		compileOpts = append(compileOpts, compose.WithMaxRunSteps(maxRunSteps))
	}
	runnable, err := g.Compile(ctx, compileOpts...)
	if err != nil {
		return nil, fmt.Errorf("Graph 编译失败: %w", err)
	}
	return &compiledOrchestration{runnable: runnable, mode: "graph", nodeKeys: nodeKeys, subNodes: subNodes, trace: c.trace}, nil
}

// wrapLoopCond 包装带循环回边的分支条件: 命中回边目标时计数并推送循环事件;
// 命中次数超过 max_loops 后不再回环, 强制返回退出目标 (评审一直不通过时取当前结果出环)
// 注意: 计数闭包按编译实例生效, 当前服务每次运行都会重新编译, 天然按次隔离
func (c *orchestrationCompiler) wrapLoopCond(id, name string, inner func(_ context.Context, in *schema.Message) (string, error)) func(_ context.Context, in *schema.Message) (string, error) {
	loopTargets := map[string]bool{}
	for _, le := range c.loopEdges[id] {
		loopTargets[le.Target] = true
	}
	exitTarget := c.loopExitTarget(id, loopTargets)
	maxLoops := c.loopMaxLoops(id)
	loops := 0
	return func(ctx context.Context, in *schema.Message) (string, error) {
		target, err := inner(ctx, in)
		if err != nil {
			return "", err
		}
		if !loopTargets[target] {
			return target, nil
		}
		loops++
		if loops > maxLoops {
			orchLog("loop 强制退出 node=%s 第%d次命中回边超过上限%d -> %s", id, loops, maxLoops, exitTarget)
			c.emitLoopEvent(id, name, maxLoops, exitTarget, true)
			return exitTarget, nil
		}
		orchLog("loop 回边 node=%s 第%d/%d次 -> %s", id, loops, maxLoops, target)
		c.emitLoopEvent(id, name, loops, target, false)
		return target, nil
	}
}

// loopMaxLoops 读取分支/路由节点配置的循环上限 (不带回边时为 0)
func (c *orchestrationCompiler) loopMaxLoops(id string) int {
	n := c.nodeByID(id)
	if n == nil {
		return 0
	}
	if n.Type == OrchNodeBranch {
		var cfg OrchBranchConfig
		if len(n.Config) > 0 && json.Unmarshal(n.Config, &cfg) == nil {
			return cfg.MaxLoops
		}
		return 0
	}
	var cfg OrchRouterConfig
	if len(n.Config) > 0 && json.Unmarshal(n.Config, &cfg) == nil {
		return cfg.MaxLoops
	}
	return 0
}

// loopExitTarget 循环的退出目标: 条件目标里第一个非回边目标, 否则非回边的默认目标;
// 校验保证带回边的分支至少有一个退出目标
func (c *orchestrationCompiler) loopExitTarget(id string, loopTargets map[string]bool) string {
	n := c.nodeByID(id)
	if n == nil {
		return ""
	}
	firstCaseTarget := func(target string) bool {
		return target != "" && !loopTargets[target]
	}
	if n.Type == OrchNodeBranch {
		var cfg OrchBranchConfig
		if json.Unmarshal(n.Config, &cfg) == nil {
			for _, cs := range cfg.Cases {
				if firstCaseTarget(cs.Target) {
					return cs.Target
				}
			}
			if firstCaseTarget(cfg.DefaultTarget) {
				return cfg.DefaultTarget
			}
		}
		return ""
	}
	var cfg OrchRouterConfig
	if json.Unmarshal(n.Config, &cfg) == nil {
		for _, cs := range cfg.Cases {
			if firstCaseTarget(cs.Target) {
				return cs.Target
			}
		}
		if firstCaseTarget(cfg.DefaultTarget) {
			return cfg.DefaultTarget
		}
	}
	return ""
}

// emitLoopEvent 推送循环回边的调试事件 (命中回边/强制退出时), 供前端展示循环轮次
func (c *orchestrationCompiler) emitLoopEvent(id, name string, loops int, target string, forced bool) {
	if c.trace == nil {
		return
	}
	targetName := target
	if n := c.nodeByID(target); n != nil && strings.TrimSpace(n.Name) != "" {
		targetName = n.Name
	}
	content := fmt.Sprintf("循环第 %d 次 → %s", loops, targetName)
	if forced {
		content = fmt.Sprintf("已达最大循环次数(%d), 转出循环 → %s", loops, targetName)
	}
	c.trace.emitNodeEvent(map[string]any{
		"kind": "node", "key": id, "name": name, "loop": true, "content": content,
	})
}

// workflowBranchPlan 构造 Workflow 分支的条件与目标集:
// route 模式按条件选单目标 (与 Graph 分支同构); parallel 模式返回全部主流出边目标 (并行分发)
func (c *orchestrationCompiler) workflowBranchPlan(id string) (func(_ context.Context, in *schema.Message) (map[string]bool, error), map[string]bool, error) {
	n := c.nodeByID(id)
	if n == nil {
		return nil, nil, fmt.Errorf("分支节点 %s 不存在", id)
	}
	name := n.Name
	if n.Type == OrchNodeBranch {
		var cfg OrchBranchConfig
		if err := json.Unmarshal(n.Config, &cfg); err != nil {
			return nil, nil, fmt.Errorf("分支节点 %s 配置解析失败: %w", id, err)
		}
		if cfg.Mode == OrchBranchModeParallel {
			targets := append([]string{}, c.flowAdj[id]...)
			sort.Strings(targets)
			endNodes := make(map[string]bool, len(targets))
			for _, t := range targets {
				endNodes[t] = true
			}
			cond := func(_ context.Context, _ *schema.Message) (map[string]bool, error) {
				c.emitParallelEvent(id, name, targets)
				selected := make(map[string]bool, len(targets))
				for _, t := range targets {
					selected[t] = true
				}
				return selected, nil
			}
			return cond, endNodes, nil
		}
		cases := append([]OrchBranchCase(nil), cfg.Cases...)
		defaultTarget := cfg.DefaultTarget
		endNodes := make(map[string]bool, len(cases)+1)
		for _, cs := range cases {
			endNodes[cs.Target] = true
		}
		endNodes[defaultTarget] = true
		cond := func(_ context.Context, in *schema.Message) (map[string]bool, error) {
			content := ""
			if in != nil {
				content = in.Content
			}
			for _, cs := range cases {
				if orchBranchMatch(cs, content) {
					return map[string]bool{cs.Target: true}, nil
				}
			}
			return map[string]bool{defaultTarget: true}, nil
		}
		return cond, endNodes, nil
	}
	// 路由节点 (防御: 校验禁止路由与合并混用, 正常编译不到这里)
	var cfg OrchRouterConfig
	if err := json.Unmarshal(n.Config, &cfg); err != nil {
		return nil, nil, fmt.Errorf("路由节点 %s 配置解析失败: %w", id, err)
	}
	inner, err := c.buildRouterCond(id, cfg)
	if err != nil {
		return nil, nil, err
	}
	endNodes := make(map[string]bool, len(cfg.Cases)+1)
	for _, cs := range cfg.Cases {
		endNodes[cs.Target] = true
	}
	endNodes[cfg.DefaultTarget] = true
	cond := func(ctx context.Context, in *schema.Message) (map[string]bool, error) {
		t, err := inner(ctx, in)
		if err != nil {
			return nil, err
		}
		return map[string]bool{t: true}, nil
	}
	return cond, endNodes, nil
}

// emitParallelEvent 推送并行分发事件 (前端展示本轮分发到几路)
func (c *orchestrationCompiler) emitParallelEvent(id, name string, targets []string) {
	if c.trace == nil {
		return
	}
	names := make([]string, 0, len(targets))
	for _, t := range targets {
		if tn := c.nodeByID(t); tn != nil && strings.TrimSpace(tn.Name) != "" {
			names = append(names, tn.Name)
		} else {
			names = append(names, t)
		}
	}
	c.trace.emitNodeEvent(map[string]any{
		"kind": "node", "key": id, "name": name, "parallel": true,
		"content": fmt.Sprintf("并行分发 %d 路 → %s", len(targets), strings.Join(names, "、")),
	})
}

func (c *orchestrationCompiler) compileWorkflow(lambdas map[string]*compose.Lambda, nodeKeys map[string]string, subNodes map[string]bool) (*compiledOrchestration, error) {
	wf := compose.NewWorkflow[*schema.Message, *schema.Message]()
	wfNodes := make(map[string]*compose.WorkflowNode, len(lambdas))
	for id, lambda := range lambdas {
		wfNodes[id] = wf.AddLambdaNode(id, lambda, compose.WithNodeName(id))
	}
	inEdges := make(map[string][]OrchestrationEdge)
	for _, e := range c.dsl.Edges {
		if e.Kind == OrchEdgeLoop {
			continue // 回边不参与数据流 (校验已禁止循环与合并混用, 这里兜底)
		}
		if n := c.nodeByID(e.Target); n != nil && n.Type == OrchNodeSubAgent {
			continue // 委派边不参与数据流
		}
		inEdges[e.Target] = append(inEdges[e.Target], e)
	}
	for _, id := range c.topo {
		n := c.nodeByID(id)
		// 入口/出口按主流度判断: 回边不计入 (回边目标若同时是入口, 仍需 START 供首轮进入)
		// AddInput 返回链式节点自身无 error, 接线错误统一在 Compile 阶段暴露
		if c.flowInOf(id) == 0 {
			wfNodes[id].AddInput(compose.START)
		}
		if n.Type == OrchNodeBranch || n.Type == OrchNodeRouter {
			// 分支/路由: 自身从上游接收输入, 目标分发由 WorkflowBranch 控制。
			// 未选中的目标子图被运行时整体跳过 (AllPredecessor 的 skip 传播);
			// parallel 模式返回全部目标实现 fan-out, 各路径在合并节点汇聚。
			// 目标节点接收数据靠它自己的 AddInput(分支id) (Workflow 分支不自动透传输入)
			for _, e := range inEdges[id] {
				wfNodes[id].AddInput(e.Source)
			}
			cond, endNodes, err := c.workflowBranchPlan(id)
			if err != nil {
				return nil, err
			}
			wf.AddBranch(id, compose.NewGraphMultiBranch[*schema.Message](cond, endNodes))
			continue
		}
		if n.Type == OrchNodeMerge {
			// 合并节点: 每条入边把来源消息的 Content 映射到 map 的来源 key 上
			for _, e := range inEdges[id] {
				wfNodes[id].AddInput(e.Source, compose.MapFields("Content", e.Source))
			}
			continue
		}
		for _, e := range inEdges[id] {
			wfNodes[id].AddInput(e.Source)
		}
		if c.flowOutOf(id) == 0 {
			wf.End().AddInput(id)
		}
	}
	runnable, err := wf.Compile(context.Background(), compose.WithGraphName("orchestration"))
	if err != nil {
		return nil, fmt.Errorf("Workflow 编译失败: %w", err)
	}
	return &compiledOrchestration{runnable: runnable, mode: "workflow", nodeKeys: nodeKeys, subNodes: subNodes, trace: c.trace}, nil
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

// modeName 编译形态名 (日志用)
func (c *orchestrationCompiler) modeName(hasBranch, hasRouter, hasMerge bool) string {
	switch {
	case hasMerge:
		return "workflow"
	case hasBranch || hasRouter:
		return "graph"
	default:
		return "chain"
	}
}

// orchChatHistoryFromVars 取出多轮调试传入的历史 (schema 消息序列), 无历史时返回 nil
func (c *orchestrationCompiler) orchChatHistoryFromVars() []*schema.Message {
	vars := c.resolvedSessionVars()
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

// nodeModelFn 按节点配置构建模型: maxRetries>0 时覆盖 ark SDK 内建重试次数,
// 0 表示沿用框架默认; deps 未提供 getModelRetry 时 (旧 deps/单测) 回退 getModel
func (c *orchestrationCompiler) nodeModelFn(modelOverride string, maxRetries int, nodeID string) (model.ToolCallingChatModel, error) {
	if c.deps.getModelRetry != nil {
		var retryTimes *int
		if maxRetries > 0 {
			rt := maxRetries
			retryTimes = &rt
		}
		return c.deps.getModelRetry(modelOverride, retryTimes)
	}
	if maxRetries > 0 {
		orchLog("node model retry 被忽略 node=%s retries=%d (deps 未提供 getModelRetry)", nodeID, maxRetries)
	}
	return c.deps.getModel(modelOverride)
}
