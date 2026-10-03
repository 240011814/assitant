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
	"strconv"
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
// branch(条件路由) / router(LLM 意图路由) / merge(多路合并) / extract(JSON 字段提取) /
// suborch(子编排引用) / end(输出)

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
	// OrchNodeRouter LLM 路由: 用模型把上游内容分类成 N 个标签之一, 按标签路由
	// (与 branch 一样编译为 GraphBranch, 只是路由决策来自模型而非文本匹配)
	OrchNodeRouter = "router"
	// OrchNodeExtract 字段提取: 上游内容为 JSON 时按字段路径抽取文本
	OrchNodeExtract = "extract"
	// OrchNodeSubOrch 子编排: 引用另一个已保存编排作为节点执行 (递归编译, 禁止循环引用)
	OrchNodeSubOrch = "suborch"
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
	// Kind 连线类型: 空/"flow" 普通主流边; "loop" 循环回边 (仅 branch/router 可发出,
	// 指向拓扑序更早的节点, 运行时构成受控循环, 由分支的 max_loops 限次)
	Kind string `json:"kind,omitempty"`
}

// 连线类型
const (
	OrchEdgeFlow = "flow"
	OrchEdgeLoop = "loop"
)

type OrchAgentConfig struct {
	Model        string   `json:"model"`
	SystemPrompt string   `json:"system_prompt"`
	Tools        []string `json:"tools"`
	// MaxIterations 映射 react.MaxStep (模型↔工具往返轮次上限)
	MaxIterations int `json:"max_iterations"`
	// MaxRetries 模型调用失败重试次数: 覆盖 ark SDK 内建重试 (HTTP 层, 指数退避,
	// 仅对 5xx/429/网络错误等可重试错误生效; 流式仅覆盖建流失败)。
	// 0 = 使用框架默认 (2 次), 1-10 = 指定次数; 重试只发生在模型调用层, 工具副作用不会重复执行
	MaxRetries int `json:"max_retries"`
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

// orchMaxRetriesCap 单节点失败重试次数上限 (与画布校验一致)
const orchMaxRetriesCap = 10

// 分支节点的分发模式
const (
	// OrchBranchModeRoute 互斥路由 (默认): 按条件选中一个目标执行
	OrchBranchModeRoute = "route"
	// OrchBranchModeParallel 并行分发: 所有目标同时执行, 各路径汇入合并节点
	// (编译为 Workflow 多目标分支, 未选中的互斥路径由 eino AllPredecessor 运行时跳过)
	OrchBranchModeParallel = "parallel"
)

type OrchBranchConfig struct {
	// Mode 分发模式: route (默认, 互斥路由) / parallel (并行分发全部目标)
	Mode          string           `json:"mode"`
	Cases         []OrchBranchCase `json:"cases"`
	DefaultTarget string           `json:"default_target"`
	// MaxLoops 循环回边的最大执行次数 (该分支带 loop 边时必填):
	// 回边命中超过该次数后强制走退出目标, 防止评审一直不通过导致死循环
	MaxLoops int `json:"max_loops"`
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
	// MaxIterations 子 Agent 的 ReAct 最大轮次 (模型↔工具往返上限), 0 = 默认 15
	MaxIterations int `json:"max_iterations"`
	// TimeoutSeconds 单次委派的执行超时 (秒), 0 = 不单独限时 (跟随编排整体超时)
	TimeoutSeconds int `json:"timeout_seconds"`
	// MaxRetries 子 Agent 模型调用失败重试次数: 0 = 使用框架默认 (2 次), 1-10 = 指定次数
	MaxRetries int `json:"max_retries"`
}

type OrchMergeConfig struct {
	// Separator 合并多个来源内容时的分隔符, 默认 "\n\n"
	Separator string `json:"separator"`
}

// OrchRouterCase LLM 路由分支: Label 是模型应输出的分类标签
type OrchRouterCase struct {
	// Label 分类标签 (模型按它输出, 精确匹配优先, 其次包含匹配)
	Label string `json:"label"`
	// Description 该分类的判定说明, 帮助模型区分相近意图
	Description string `json:"description"`
	Target      string `json:"target"`
}

type OrchRouterConfig struct {
	// Model 分类用的模型 code, 空=默认模型
	Model string `json:"model"`
	// Instructions 分类任务说明 (判定规则/边界描述), 可为空
	Instructions string           `json:"instructions"`
	Cases        []OrchRouterCase `json:"cases"`
	// DefaultTarget 无标签命中 (含模型输出不可解析) 时的兜底目标
	DefaultTarget string `json:"default_target"`
	// MaxLoops 循环回边的最大执行次数 (该分支带 loop 边时必填), 语义同 OrchBranchConfig
	MaxLoops int `json:"max_loops"`
}

type OrchExtractConfig struct {
	// Field 字段路径, 点号分隔; 数组段用数字下标 (如 data.items.0.name)
	Field string `json:"field"`
	// Fallback 提取失败 (非 JSON/路径不存在) 时的兜底输出; 为空则原样透传上游内容
	Fallback string `json:"fallback"`
}

type OrchSubOrchConfig struct {
	// OrchestrationID 引用已有编排 (ai_agents 表 agent_type='orchestration', 需已启用)
	OrchestrationID int `json:"orchestration_id"`
}

// compilerDeps 编译期依赖, 由 AIAgentService 提供
type compilerDeps struct {
	// getModel 按 model code 构建 ToolCallingChatModel (空串用默认模型)
	getModel func(modelOverride string) (model.ToolCallingChatModel, error)
	// getModelRetry 同 getModel 但可覆盖 ark SDK 内建的模型调用重试次数
	// (retryTimes=nil 沿用默认)。未注入时 (旧 deps/单测) 回退 getModel, 重试取框架默认
	getModelRetry func(modelOverride string, retryTimes *int) (model.ToolCallingChatModel, error)
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

// orchMaxNestDepth 子编排最大嵌套层数 (编译链上的编排个数上限)
const orchMaxNestDepth = 5

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

// validateOrchestrationDSLDetailed 校验 DSL, 返回解析结果与带节点定位的错误列表
func validateOrchestrationDSLDetailed(definition string) (*OrchestrationDSL, []orchValidateIssue) {
	dsl, err := parseOrchestrationDSL(definition)
	if err != nil {
		return nil, []orchValidateIssue{{Message: err.Error()}}
	}
	c := &orchestrationCompiler{dsl: dsl}
	return dsl, c.validateIssues()
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

// orchValidateIssue 单条校验错误: NodeID 为空表示全局性错误 (无法定位到单个节点)
type orchValidateIssue struct {
	NodeID  string
	Message string
}

// validate 返回错误文案列表 (兼容旧调用方: validateOrchestrationDSL/compile/测试)
func (c *orchestrationCompiler) validate() []string {
	issues := c.validateIssues()
	errs := make([]string, len(issues))
	for i := range issues {
		errs[i] = issues[i].Message
	}
	return errs
}

// validateIssues 带节点定位的校验: NodeID 供前端高亮/选中出错节点, Message 与原 validate() 文案一致
func (c *orchestrationCompiler) validateIssues() []orchValidateIssue {
	var issues []orchValidateIssue
	add := func(nodeID, format string, args ...any) {
		issues = append(issues, orchValidateIssue{NodeID: nodeID, Message: fmt.Sprintf(format, args...)})
	}

	c.nodeMap = make(map[string]*OrchestrationNode, len(c.dsl.Nodes))
	c.adj = make(map[string][]string)
	c.loopEdges = make(map[string][]OrchestrationEdge)
	c.inDeg = make(map[string]int)
	c.outDeg = make(map[string]int)

	if len(c.dsl.Nodes) == 0 {
		return []orchValidateIssue{{Message: "编排至少需要一个节点"}}
	}

	hasBranch, hasRouter, hasMerge, hasLoop := false, false, false, false
	parallelBranches := map[string]bool{} // 并行分发分支节点 id
	for i := range c.dsl.Nodes {
		n := &c.dsl.Nodes[i]
		if n.ID == "" {
			add("", "第 %d 个节点缺少 id", i+1)
			continue
		}
		if _, dup := c.nodeMap[n.ID]; dup {
			add(n.ID, "节点 id 重复: %s", n.ID)
			continue
		}
		c.nodeMap[n.ID] = n
		if n.Name == "" {
			n.Name = n.ID
		}
		switch n.Type {
		case OrchNodeAgent, OrchNodeTool, OrchNodeTemplate, OrchNodeBranch, OrchNodeMerge, OrchNodeEnd, OrchNodeSubAgent,
			OrchNodeRouter, OrchNodeExtract, OrchNodeSubOrch:
		default:
			add(n.ID, "节点 %s 类型非法: %s", n.ID, n.Type)
		}
		if n.Type == OrchNodeBranch {
			hasBranch = true
			var cfg OrchBranchConfig
			if len(n.Config) > 0 && json.Unmarshal(n.Config, &cfg) == nil && cfg.Mode == OrchBranchModeParallel {
				parallelBranches[n.ID] = true
			}
		}
		if n.Type == OrchNodeRouter {
			hasRouter = true
		}
		if n.Type == OrchNodeMerge {
			hasMerge = true
		}
	}
	if len(issues) > 0 {
		return issues
	}

	for i, e := range c.dsl.Edges {
		if c.nodeByID(e.Source) == nil {
			add("", "第 %d 条连线来源不存在: %s", i+1, e.Source)
			continue
		}
		if c.nodeByID(e.Target) == nil {
			add(e.Source, "第 %d 条连线目标不存在: %s", i+1, e.Target)
			continue
		}
		if e.Source == e.Target {
			add(e.Source, "连线不允许自环: %s", e.Source)
			continue
		}
		if e.Kind == OrchEdgeLoop {
			src := c.nodeByID(e.Source)
			if src.Type != OrchNodeBranch && src.Type != OrchNodeRouter {
				add(e.Source, "循环回边 %s -> %s 只能从分支/LLM路由节点发出", e.Source, e.Target)
				continue
			}
			if len(c.loopEdges[e.Source]) > 0 {
				add(e.Source, "节点 %s 只允许一条循环回边", e.Source)
				continue
			}
			if c.nodeByID(e.Target).Type == OrchNodeSubAgent {
				add(e.Source, "循环回边 %s -> %s 不能指向子Agent 节点", e.Source, e.Target)
				continue
			}
			c.loopEdges[e.Source] = append(c.loopEdges[e.Source], e)
			continue
		}
		if e.Kind != "" && e.Kind != OrchEdgeFlow {
			add(e.Source, "第 %d 条连线类型非法: %s (flow/loop)", i+1, e.Kind)
			continue
		}
		c.adj[e.Source] = append(c.adj[e.Source], e.Target)
		c.inDeg[e.Target]++
		c.outDeg[e.Source]++
	}

	hasLoop = len(c.loopEdges) > 0

	// 拓扑排序 + 环检测 (Kahn): 子Agent 节点不属于主流程 (仅作为主 Agent 的委派挂载),
	// 主流程的度按"目标非子Agent"的连线计算; 循环回边已在上一步剔除, 不参与拓扑
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
		add("", "编排存在循环连线, 不允许成环 (循环请使用分支/LLM路由节点的循环回边; Agent 节点内部的工具循环由 ReAct 自动处理)")
		return issues
	}

	// 受控环校验: 回边已从主流剔除, 这里按局部规则逐条检查。
	// target 必须在剔除后的拓扑序中位于 source 之前 (保证真的成环, 而非绕过单出边的第二出边)
	topoIndex := make(map[string]int, len(c.topo))
	for i, id := range c.topo {
		topoIndex[id] = i
	}
	if hasLoop && hasMerge {
		add("", "当前版本暂不支持循环与合并混用, 请拆分为多个编排")
	}
	for src, edges := range c.loopEdges {
		for _, le := range edges {
			if ti, ok := topoIndex[le.Target]; !ok || ti >= topoIndex[src] {
				add(le.Source, "循环回边 %s -> %s 的目标必须是主流程中位于该节点之前的节点 (构成循环)", le.Source, le.Target)
				continue
			}
			var maxLoops int
			if c.nodeByID(src).Type == OrchNodeBranch {
				var cfg OrchBranchConfig
				if json.Unmarshal(c.nodeByID(src).Config, &cfg) == nil {
					maxLoops = cfg.MaxLoops
				}
			} else {
				var cfg OrchRouterConfig
				if json.Unmarshal(c.nodeByID(src).Config, &cfg) == nil {
					maxLoops = cfg.MaxLoops
				}
			}
			if maxLoops < 1 {
				add(src, "节点 %s 带循环回边, 必须设置循环上限 (max_loops >= 1)", src)
			}
			// 退出目标: 条件/默认目标中至少一个不是回边目标, 循环才有出口
			exitExists := false
			if c.nodeByID(src).Type == OrchNodeBranch {
				var cfg OrchBranchConfig
				if json.Unmarshal(c.nodeByID(src).Config, &cfg) == nil {
					if cfg.DefaultTarget != le.Target && cfg.DefaultTarget != "" {
						exitExists = true
					}
					for _, cs := range cfg.Cases {
						if cs.Target != le.Target && c.nodeByID(cs.Target) != nil {
							exitExists = true
						}
					}
				}
			} else {
				var cfg OrchRouterConfig
				if json.Unmarshal(c.nodeByID(src).Config, &cfg) == nil {
					if cfg.DefaultTarget != le.Target && cfg.DefaultTarget != "" {
						exitExists = true
					}
					for _, cs := range cfg.Cases {
						if cs.Target != le.Target && c.nodeByID(cs.Target) != nil {
							exitExists = true
						}
					}
				}
			}
			if !exitExists {
				add(src, "节点 %s 的所有目标都是循环回边, 缺少退出目标", src)
			}
		}
	}

	// 预解析分支/路由目标: target node id -> branch/router node id
	// (用于"同一分支的多个目标可以汇聚"的一致性判定)
	branchTargetOf := make(map[string]string)
	for _, id := range c.topo {
		n := c.nodeByID(id)
		if n.Type != OrchNodeBranch && n.Type != OrchNodeRouter {
			continue
		}
		recordTarget := func(target string) {
			if target != "" && target != id {
				branchTargetOf[target] = id
			}
		}
		if n.Type == OrchNodeBranch {
			var cfg OrchBranchConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				continue
			}
			for _, cs := range cfg.Cases {
				recordTarget(cs.Target)
			}
			recordTarget(cfg.DefaultTarget)
			continue
		}
		var cfg OrchRouterConfig
		if err := json.Unmarshal(n.Config, &cfg); err != nil {
			continue
		}
		for _, cs := range cfg.Cases {
			recordTarget(cs.Target)
		}
		recordTarget(cfg.DefaultTarget)
	}

	inputCount, outputCount := 0, 0
	for _, id := range c.topo {
		n := c.nodeByID(id)
		inDeg, outDeg := c.flowInOf(id), c.flowOutOf(id)
		switch n.Type {
		case OrchNodeMerge:
			if inDeg < 2 {
				add(id, "合并节点 %s 至少需要两条入边", id)
			}
			if outDeg != 1 {
				add(id, "合并节点 %s 必须恰好一条出边", id)
			}
		case OrchNodeBranch:
			if inDeg != 1 {
				add(id, "分支节点 %s 必须恰好一条入边", id)
			}
			var cfg OrchBranchConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				add(id, "分支节点 %s 配置解析失败: %v", id, err)
				continue
			}
			if cfg.Mode == OrchBranchModeParallel {
				// 并行分发: 目标即画布全部主流出边 (条件不生效), 专项校验在下方
				if outDeg+len(c.loopEdges[id]) < 2 {
					add(id, "并行分支 %s 至少需要两条分发连线", id)
				}
			} else {
				if len(cfg.Cases) == 0 {
					add(id, "分支节点 %s 至少需要一个分支条件", id)
				}
				targets := map[string]bool{}
				for j, cs := range cfg.Cases {
					if cs.Value == "" {
						add(id, "分支节点 %s 第 %d 个条件缺少匹配值", id, j+1)
					}
					if cs.Target == "" || c.nodeByID(cs.Target) == nil {
						add(id, "分支节点 %s 第 %d 个条件目标无效: %s", id, j+1, cs.Target)
						continue
					}
					if cs.Target == id {
						add(id, "分支节点 %s 条件不能指向自身", id)
						continue
					}
					targets[cs.Target] = true
					switch cs.Type {
					case "contains", "equals":
					case "regex":
						if _, err := regexp.Compile(cs.Value); err != nil {
							add(id, "分支节点 %s 第 %d 个条件正则非法: %v", id, j+1, err)
						}
					default:
						add(id, "分支节点 %s 第 %d 个条件类型非法: %s (contains/equals/regex)", id, j+1, cs.Type)
					}
				}
				if cfg.DefaultTarget == "" || c.nodeByID(cfg.DefaultTarget) == nil {
					add(id, "分支节点 %s 缺少有效的默认分支目标", id)
				} else if cfg.DefaultTarget == id {
					add(id, "分支节点 %s 默认分支不能指向自身", id)
				} else {
					targets[cfg.DefaultTarget] = true
				}
				if outDeg+len(c.loopEdges[id]) != len(targets) {
					add(id, "分支节点 %s 的画布连线(%d 条, 含回边)与条件目标(%d 个)不一致, 分支节点必须连接到所有条件目标", id, outDeg+len(c.loopEdges[id]), len(targets))
				}
				for _, t := range c.adj[id] {
					if !targets[t] {
						add(id, "分支节点 %s 连线目标 %s 未出现在分支条件中", id, t)
					}
				}
				for _, le := range c.loopEdges[id] {
					if !targets[le.Target] {
						add(id, "分支节点 %s 回边目标 %s 未出现在分支条件中", id, le.Target)
					}
				}
			}
		case OrchNodeRouter:
			if inDeg != 1 {
				add(id, "路由节点 %s 必须恰好一条入边", id)
			}
			var cfg OrchRouterConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				add(id, "路由节点 %s 配置解析失败: %v", id, err)
				continue
			}
			if len(cfg.Cases) == 0 {
				add(id, "路由节点 %s 至少需要一个分类标签", id)
			}
			targets := map[string]bool{}
			for j, cs := range cfg.Cases {
				if strings.TrimSpace(cs.Label) == "" {
					add(id, "路由节点 %s 第 %d 个分类缺少标签", id, j+1)
				}
				if cs.Target == "" || c.nodeByID(cs.Target) == nil {
					add(id, "路由节点 %s 第 %d 个分类目标无效: %s", id, j+1, cs.Target)
					continue
				}
				if cs.Target == id {
					add(id, "路由节点 %s 分类不能指向自身", id)
					continue
				}
				targets[cs.Target] = true
			}
			if cfg.DefaultTarget == "" || c.nodeByID(cfg.DefaultTarget) == nil {
				add(id, "路由节点 %s 缺少有效的默认目标", id)
			} else if cfg.DefaultTarget == id {
				add(id, "路由节点 %s 默认目标不能指向自身", id)
			} else {
				targets[cfg.DefaultTarget] = true
			}
			if outDeg+len(c.loopEdges[id]) != len(targets) {
				add(id, "路由节点 %s 的画布连线(%d 条, 含回边)与分类目标(%d 个)不一致, 路由节点必须连接到所有分类目标", id, outDeg+len(c.loopEdges[id]), len(targets))
			}
			for _, t := range c.adj[id] {
				if !targets[t] {
					add(id, "路由节点 %s 连线目标 %s 未出现在分类中", id, t)
				}
			}
			for _, le := range c.loopEdges[id] {
				if !targets[le.Target] {
					add(id, "路由节点 %s 回边目标 %s 未出现在分类中", id, le.Target)
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
					add(id, "节点 %s 只允许一条入边 (多路合并请使用合并节点; 分支汇合必须来自同一分支节点的目标)", id)
				}
			}
			if outDeg > 1 {
				add(id, "节点 %s 只允许一条出边 (多路分发请使用分支节点)", id)
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
			add(id, "子Agent 节点 %s 必须恰好有一条来自主 Agent 的入边", id)
		} else {
			for _, e := range c.dsl.Edges {
				if e.Target != id {
					continue
				}
				if src := c.nodeByID(e.Source); src == nil || src.Type != OrchNodeAgent {
					add(id, "子Agent 节点 %s 的入边必须来自 Agent 节点 (委派方向: 主 Agent -> 子Agent)", id)
				}
				break
			}
		}
		if c.outDeg[id] > 0 {
			add(id, "子Agent 节点 %s 不允许向外连线", id)
		}
		var cfg OrchSubAgentConfig
		if len(n.Config) > 0 {
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				add(id, "子Agent 节点 %s 配置解析失败: %v", id, err)
				continue
			}
		}
		if cfg.AgentID == 0 && strings.TrimSpace(cfg.SystemPrompt) == "" {
			add(id, "子Agent 节点 %s 需要引用已有 Agent 或填写内联系统提示词", id)
		}
		if cfg.MaxIterations < 0 {
			add(id, "子Agent 节点 %s 最大轮次不能为负数", id)
		}
		if cfg.TimeoutSeconds < 0 || cfg.TimeoutSeconds > 3600 {
			add(id, "子Agent 节点 %s 超时须在 0-3600 秒之间 (0 表示跟随编排整体超时)", id)
		}
		if cfg.MaxRetries < 0 || cfg.MaxRetries > orchMaxRetriesCap {
			add(id, "子Agent 节点 %s 失败重试次数须在 0-%d 之间 (0 表示使用框架默认 2 次)", id, orchMaxRetriesCap)
		}
	}

	if hasMerge {
		if inputCount < 1 {
			add("", "编排至少需要一个入口节点(无入边)")
		}
	} else if inputCount != 1 {
		add("", "编排必须恰好一个入口节点(无入边), 当前 %d 个", inputCount)
	}
	if outputCount != 1 {
		add("", "编排必须恰好一个出口节点(无出边), 当前 %d 个", outputCount)
	}
	if hasBranch && hasRouter {
		add("", "当前版本暂不支持分支与 LLM 路由混用, 请拆分为多个编排")
	}
	if hasRouter && hasMerge {
		add("", "当前版本暂不支持 LLM 路由与合并混用, 请拆分为多个编排")
	}
	// 分支与合并混用已支持: 分支按节点配置的 mode 执行 (route 互斥路由 / parallel 并行
	// 分发), 编译为 Workflow, 未选中的互斥路径由运行时整体跳过, 各路径在合并节点汇聚。
	// 并行分支的专项约束见下方校验 (≥2 目标/不支持回边/必须汇入合并节点)

	// 并行分支专项校验: 不能带回边, 且每条分发路径必须汇入合并节点
	for _, id := range c.topo {
		if !parallelBranches[id] {
			continue
		}
		if len(c.loopEdges[id]) > 0 {
			add(id, "并行分支 %s 不支持循环回边, 请改用互斥路由分支", id)
			continue
		}
		if !c.parallelBranchConverges(id) {
			add(id, "并行分支 %s 的每条分发路径都必须汇入合并节点 (并行结果需要汇聚后输出)", id)
		}
	}

	// 节点配置校验
	for _, id := range c.topo {
		n := c.nodeByID(id)
		switch n.Type {
		case OrchNodeAgent:
			var cfg OrchAgentConfig
			if len(n.Config) > 0 {
				if err := json.Unmarshal(n.Config, &cfg); err != nil {
					add(id, "Agent 节点 %s 配置解析失败: %v", id, err)
				}
			}
			if cfg.MaxRetries < 0 || cfg.MaxRetries > orchMaxRetriesCap {
				add(id, "Agent 节点 %s 失败重试次数须在 0-%d 之间 (0 表示使用框架默认 2 次)", id, orchMaxRetriesCap)
			}
		case OrchNodeTool:
			var cfg OrchToolConfig
			if len(n.Config) == 0 || json.Unmarshal(n.Config, &cfg) != nil || cfg.Tool == "" {
				add(id, "工具节点 %s 缺少工具配置", id)
			}
		case OrchNodeTemplate:
			var cfg OrchTemplateConfig
			if err := json.Unmarshal(n.Config, &cfg); err != nil {
				add(id, "模板节点 %s 配置解析失败: %v", id, err)
			} else if strings.TrimSpace(cfg.Template) == "" {
				add(id, "模板节点 %s 模板内容不能为空", id)
			} else if _, err := template.New(id).Parse(cfg.Template); err != nil {
				add(id, "模板节点 %s 模板语法错误: %v", id, err)
			}
		case OrchNodeExtract:
			var cfg OrchExtractConfig
			if len(n.Config) == 0 || json.Unmarshal(n.Config, &cfg) != nil || strings.TrimSpace(cfg.Field) == "" {
				add(id, "提取节点 %s 缺少字段路径 (如 result.content)", id)
			}
		case OrchNodeSubOrch:
			var cfg OrchSubOrchConfig
			if len(n.Config) == 0 || json.Unmarshal(n.Config, &cfg) != nil || cfg.OrchestrationID <= 0 {
				add(id, "子编排节点 %s 缺少引用的编排", id)
			}
		}
	}
	return issues
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

// parallelBranchConverges 检查并行分支的每条分发路径是否都汇入合并节点:
// 从各目标沿主流出边遍历, 遇合并节点视为已汇入 (不再向后),
// 走到非合并节点的出口 (END 方向) 视为未汇聚
func (c *orchestrationCompiler) parallelBranchConverges(branchID string) bool {
	visited := map[string]bool{}
	stack := append([]string{}, c.flowAdj[branchID]...)
	for len(stack) > 0 {
		id := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[id] {
			continue
		}
		visited[id] = true
		if n := c.nodeByID(id); n != nil && n.Type == OrchNodeMerge {
			continue
		}
		if c.flowOutOf(id) == 0 {
			return false
		}
		stack = append(stack, c.flowAdj[id]...)
	}
	return true
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
		if c.flowInOf(id) == 0 {
			_ = wfNodes[id].AddInput(compose.START)
		}
		if n.Type == OrchNodeBranch || n.Type == OrchNodeRouter {
			// 分支/路由: 自身从上游接收输入, 目标分发由 WorkflowBranch 控制。
			// 未选中的目标子图被运行时整体跳过 (AllPredecessor 的 skip 传播);
			// parallel 模式返回全部目标实现 fan-out, 各路径在合并节点汇聚。
			// 目标节点接收数据靠它自己的 AddInput(分支id) (Workflow 分支不自动透传输入)
			for _, e := range inEdges[id] {
				_ = wfNodes[id].AddInput(e.Source)
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
				_ = wfNodes[id].AddInput(e.Source, compose.MapFields("Content", e.Source))
			}
			continue
		}
		for _, e := range inEdges[id] {
			_ = wfNodes[id].AddInput(e.Source)
		}
		if c.flowOutOf(id) == 0 {
			_ = wf.End().AddInput(id)
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

// orchChatPreamble 编排对话的身份前言 (由编排名称/简介构造):
// 这段内容此前只存在于前端欢迎气泡里, 模型从未见过, 首轮对话便"不知道自己是谁"。
func orchChatPreamble(name, description string) string {
	var sb strings.Builder
	sb.WriteString("你是「" + strings.TrimSpace(name) + "」编排助手。")
	if d := strings.TrimSpace(description); d != "" {
		sb.WriteString("你的职责: " + d + "。")
	}
	sb.WriteString("请始终以此身份与用户对话。")
	return sb.String()
}

// orchAppendPromptSection 追加一段提示词章节 (已有内容时空一行分隔, 空提示词直接返回章节)
func orchAppendPromptSection(prompt, section string) string {
	if strings.TrimSpace(section) == "" {
		return prompt
	}
	if strings.TrimSpace(prompt) == "" {
		return section
	}
	return prompt + "\n\n" + section
}

// orchHistoryGuide 主 Agent 多轮对话提示。
// 历史消息已随消息序列给出, 但模型(尤其面对"省略/指代式追问")常常忽略上文并反问,
// 例如上一轮在问天气、这一轮只说"用子agent搜索", 模型就回复"不清楚要搜索什么"。
// 这里显式提醒它结合历史推断意图, 并把当前会话的主题列出来, 降低"上下文丢失"的错觉。
func orchHistoryGuide() string {
	return "\n\n【多轮对话】本次会话的历史消息已随消息序列提供。用户可能用省略或指代的方式延续上一轮的话题(例如上一轮在问天气, 这一轮只说「用子agent搜索」)。请结合历史推断其真实意图并直接执行, 不要因为这一句没有重复主题就反问; 只有在历史里确实找不到指代对象时才追问。"
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

// buildNodeLambda 构建单个节点的 Lambda (agent/tool/template/extract/suborch/merge/end;
// branch/router 是路由点: lambda 为直通, 路由由 GraphBranch 承担)
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
	case OrchNodeExtract:
		return c.buildExtractLambda(n)
	case OrchNodeSubOrch:
		return c.buildSubOrchLambda(ctx, n)
	default: // end/branch/router 及其他: 直通
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
	// 提示词版本覆盖 (编排全局共享, user_id=0): 优先级高于被引用 Agent 的提示词与内联提示词
	if c.deps.nodePrompt != nil {
		if v, ok := c.deps.nodePrompt(id); ok && strings.TrimSpace(v) != "" {
			orchLog("subagent node prompt override node=%s -> 版本长度=%d", id, len(v))
			instruction = v
		}
	}
	if instruction == "" {
		return nil, fmt.Errorf("子Agent 节点 %s 缺少可用的系统提示词", id)
	}
	vars := c.resolvedSessionVars()
	instruction, err := orchRenderTemplate(id, instruction, vars)
	if err != nil {
		return nil, fmt.Errorf("子Agent 节点 %s 系统提示词渲染失败: %w", id, err)
	}
	instruction = orchInjectRuntimeContext(instruction, vars)
	chatModel, err := c.nodeModelFn(cfg.Model, cfg.MaxRetries, id)
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
	// ReAct 轮次: 节点可配置, 未配置时沿用历史默认值 15
	maxStep := cfg.MaxIterations
	if maxStep <= 0 {
		maxStep = 15
	}
	agent, err := orchNewReactAgent(ctx, id, instruction, chatModel, subTools, maxStep)
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
	// timeoutSeconds 单次委派的执行超时 (秒), 0 = 不单独限时 (跟随编排整体超时)
	timeoutSeconds int
}

func newOrchDelegateTool(name, subID, subName, parentID, desc string, agent *react.Agent, trace *orchTraceHandler, timeoutSeconds int) *orchDelegateTool {
	return &orchDelegateTool{name: name, subID: subID, subName: subName, parentID: parentID, desc: desc, agent: agent, trace: trace, timeoutSeconds: timeoutSeconds}
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

// IsCallbacksEnabled 向 eino 声明本组件自带回调转发 (components.Checker)。
// 不声明时框架会再包一层 OnStart/OnEnd, 且这层包装会把节点 ctx 的 RunInfo 清空,
// 迫使内层模型(ark)自建一个**空名字**的回调 span——它的实时增量曾与包装器的结尾
// 补发叠加成重复输出。声明后 ark 的回调直接挂在 "<subID>.model" 名下, 与主 Agent
// 完全同一条实时路径, 全程只有一处 emit。
func (p *orchSubAgentProgress) IsCallbacksEnabled() bool { return true }

func (p *orchSubAgentProgress) Generate(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	stop := p.startHeartbeat(ctx)
	defer stop()
	msg, err := p.model.Generate(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// Stream 直通内层模型: 子Agent 的思考/正文增量统一由模型的流式回调
// (orchTraceHandler.OnEndWithStreamOutput 的 directStream 分支) 实时透出。
// 这里不要再包一层 emit——包装器只有在 ReAct 图消费到流时才触发(整段生成完才到),
// 曾经与模型回调的实时增量叠成"回答先流式一遍、结束再整段一遍"的重复。
func (p *orchSubAgentProgress) Stream(ctx context.Context, in []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return p.model.Stream(ctx, in, opts...)
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
	orchLog("delegate 开始 tool=%s 子Agent=%s(%s) 超时=%ds 任务=%.80q", t.name, t.subID, t.subName, t.timeoutSeconds, task)
	// 调试事件: 子Agent 节点本身不参与主流, 没有自己的 compose 节点 span,
	// 由委派工具在上游 Agent 的 span 内推送 "已委派 + 任务内容"
	t.emitEvent(map[string]any{
		"kind": "node", "key": t.subID, "name": t.subName, "comp": "DelegateTool",
		"owner": t.parentID, "status": "running", "delegated": true, "task": task,
	})
	runCtx := ctx
	if t.timeoutSeconds > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(t.timeoutSeconds)*time.Second)
		defer cancel()
	}
	// 走流式: 子Agent 的思考/正文可以实时透出 (Generate 不会产生任何增量)
	sr, err := t.agent.Stream(runCtx, []*schema.Message{schema.UserMessage(task)})
	if err != nil {
		orchLog("delegate 失败 tool=%s 子Agent=%s 耗时=%dms err=%v", t.name, t.subID, time.Since(started).Milliseconds(), err)
		t.emitEvent(map[string]any{
			"kind": "node", "key": t.subID, "name": t.subName, "comp": "DelegateTool",
			"owner": t.parentID, "status": "error", "delegated": true, "error": t.runError(runCtx, err).Error(),
		})
		return "", t.runError(runCtx, err)
	}
	defer sr.Close()
	msg, err := schema.ConcatMessageStream(sr)
	if err != nil {
		orchLog("delegate 失败 tool=%s 子Agent=%s 耗时=%dms err=%v", t.name, t.subID, time.Since(started).Milliseconds(), err)
		t.emitEvent(map[string]any{
			"kind": "node", "key": t.subID, "name": t.subName, "comp": "DelegateTool",
			"owner": t.parentID, "status": "error", "delegated": true, "error": t.runError(runCtx, err).Error(),
		})
		return "", t.runError(runCtx, err)
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

// runError 把委派执行的错误映射为可读文案: 配置了节点超时且确因超时中断时给出明确提示,
// 其余保持原有包装 (内层错误可能是被包装过的 deadline, 需同时看 runCtx)
func (t *orchDelegateTool) runError(runCtx context.Context, err error) error {
	if t.timeoutSeconds > 0 &&
		(errors.Is(err, context.DeadlineExceeded) || (runCtx.Err() != nil && errors.Is(runCtx.Err(), context.DeadlineExceeded))) {
		return fmt.Errorf("子Agent「%s」执行超时 (%d 秒)", t.subName, t.timeoutSeconds)
	}
	return fmt.Errorf("子Agent「%s」执行失败: %w", t.subName, err)
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
	chatModel, err := c.nodeModelFn(cfg.Model, cfg.MaxRetries, n.ID)
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
		agentTools = append(agentTools, newOrchDelegateTool(toolName, subID, subNode.Name, n.ID, desc, subAgent, c.trace, subCfg.TimeoutSeconds))
	}

	vars := c.resolvedSessionVars()
	// 提示词版本: 该编排节点存在启用的提示词版本 (编排全局共享) 时, 覆盖画布内联提示词;
	// 子编排嵌套时按嵌套编排自己的 id 解析
	systemPrompt := cfg.SystemPrompt
	if c.deps.nodePrompt != nil {
		if v, ok := c.deps.nodePrompt(n.ID); ok && strings.TrimSpace(v) != "" {
			orchLog("node prompt override node=%s 内联长度=%d -> 版本长度=%d", n.ID, len(cfg.SystemPrompt), len(v))
			systemPrompt = v
		}
	}
	if systemPrompt != "" {
		var err error
		systemPrompt, err = orchRenderTemplate(n.ID, systemPrompt, vars)
		if err != nil {
			return nil, fmt.Errorf("Agent 节点 %s 系统提示词渲染失败: %w", n.ID, err)
		}
	}
	// 编排对话: 身份前言放在画布提示词之后, 模型从第一轮就知道自己是谁、职责是什么
	systemPrompt = orchAppendPromptSection(systemPrompt, c.chatPreamble)
	// 主管模式: 把挂载的子Agent 及其职责写进系统提示词, 否则模型常常完全不去委派
	if len(subIDs) > 0 {
		systemPrompt += orchDelegationGuide(subIDs, c.orchSubAgentName, c.orchSubAgentTitle, c.orchSubAgentDesc)
	}
	// 运行时上下文 (当前时间/用户ID): 提示词没引用 {{.current_time}} 时也要让模型知道当前时间
	systemPrompt = orchInjectRuntimeContext(systemPrompt, vars)

	// 多轮调试: 把历史消息拼在本轮输入之前, 让主 Agent 记得之前说过什么
	history := c.orchChatHistoryFromVars()
	// 历史已在消息序列里, 但模型对"省略/指代式追问"常忽略上文并反问, 显式提醒一句
	if len(history) > 0 {
		systemPrompt += orchHistoryGuide()
	}
	maxStep := maxStepOf(cfg)
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
	// 编排对话兜底: 入口模板若未引用 {{.Input}}, 渲染结果里没有用户消息, 模型只能看到
	// 静态文案 (线上症状: 模型把模板里的身份文案当成用户消息, 真实问题从未到达)。
	// 对话模式下把本轮用户输入补到渲染结果之后; 模板已引用 .Input 或调试/校验运行不加。
	appendUserInput := c.chatMode && c.flowInOf(n.ID) == 0 && !strings.Contains(cfg.Template, ".Input")
	vars := c.resolvedSessionVars()
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
		content := sb.String()
		if appendUserInput && strings.TrimSpace(input) != "" {
			content += "\n\n【用户消息】" + input
		}
		return &schema.Message{Role: schema.User, Content: content}, nil
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

// ---------- LLM 路由节点 ----------

// buildRouterCond 构建路由节点的分类条件: 调用模型把上游内容归入唯一标签,
// 按标签选目标; 模型输出不可解析时走默认目标, 模型调用失败则整个运行报错
func (c *orchestrationCompiler) buildRouterCond(id string, cfg OrchRouterConfig) (func(_ context.Context, in *schema.Message) (string, error), error) {
	if c.deps.getModel == nil {
		return nil, fmt.Errorf("路由节点 %s 编译依赖缺失", id)
	}
	chatModel, err := c.deps.getModel(cfg.Model)
	if err != nil {
		return nil, fmt.Errorf("路由节点 %s 获取模型失败: %w", id, err)
	}
	cases := append([]OrchRouterCase(nil), cfg.Cases...)
	defaultTarget := cfg.DefaultTarget
	instructions := strings.TrimSpace(cfg.Instructions)
	name := id
	if n := c.nodeByID(id); n != nil && strings.TrimSpace(n.Name) != "" {
		name = n.Name
	}
	return func(ctx context.Context, in *schema.Message) (string, error) {
		content := ""
		if in != nil {
			content = in.Content
		}
		started := time.Now()
		c.emitRouterEvent(id, name, "running", "", 0, "")
		label, usage, err := orchRouterClassify(ctx, chatModel, instructions, cases, content)
		if err != nil {
			ms := time.Since(started).Milliseconds()
			orchLog("router 失败 node=%s 耗时=%dms err=%v", id, ms, err)
			c.emitRouterEvent(id, name, "error", "", ms, err.Error())
			return "", fmt.Errorf("路由节点 %s 分类失败: %w", id, err)
		}
		// router 的分类调用不经 compose 节点 span, 摘要聚合不到, 这里直接记账
		// (userID=0 为校验/单测构造, 不落库); 来源与本次运行一致 (调试/对话)
		if usage != nil && usage.TotalTokens > 0 && c.userID > 0 {
			source := coremodel.TokenSourceOrchDebug
			if c.chatMode {
				source = coremodel.TokenSourceOrchChat
			}
			RecordTokenUsage(c.userID, cfg.Model, source, int64(usage.PromptTokens), int64(usage.CompletionTokens))
		}
		target, matched := orchRouterMatchLabel(label, cases)
		if !matched {
			target = defaultTarget
		}
		ms := time.Since(started).Milliseconds()
		orchLog("router 完成 node=%s 耗时=%dms label=%q 命中=%v target=%s", id, ms, label, matched, target)
		c.emitRouterEvent(id, name, "success", label, ms, "")
		return target, nil
	}, nil
}

// emitRouterEvent 推送路由决策的调试事件 (路由节点不是 compose 数据节点,
// 没有自己的模型/工具 span, 决策过程由这里显式推送)
func (c *orchestrationCompiler) emitRouterEvent(id, name, status, label string, ms int64, errMsg string) {
	if c.trace == nil {
		return
	}
	payload := map[string]any{
		"kind": "node", "key": id, "name": name, "comp": "Router", "status": status, "ms": ms,
	}
	if label != "" {
		payload["content"] = label
	}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	c.trace.emitNodeEvent(payload)
}

// orchRouterClassify 调用模型对内容做单标签分类, 返回模型原始输出 (期望就是标签
// 文本) 与本次调用的 token 用量 (无 ResponseMeta 时为 nil)
func orchRouterClassify(ctx context.Context, chatModel model.ToolCallingChatModel, instructions string, cases []OrchRouterCase, content string) (string, *schema.TokenUsage, error) {
	var sb strings.Builder
	sb.WriteString("你是意图路由决策器。根据用户内容, 从下列分类中选出唯一一个标签。\n")
	if instructions != "" {
		sb.WriteString("判定规则: " + instructions + "\n")
	}
	sb.WriteString("可选标签:\n")
	for _, cs := range cases {
		line := "- " + strings.TrimSpace(cs.Label)
		if d := strings.TrimSpace(cs.Description); d != "" {
			line += ": " + d
		}
		sb.WriteString(line + "\n")
	}
	sb.WriteString("只输出标签本身, 不要输出任何其他内容。")
	msg, err := chatModel.Generate(ctx, []*schema.Message{
		{Role: schema.System, Content: sb.String()},
		schema.UserMessage(content),
	})
	if err != nil {
		return "", nil, err
	}
	if msg == nil {
		return "", nil, errors.New("模型没有返回分类结果")
	}
	var usage *schema.TokenUsage
	if msg.ResponseMeta != nil {
		usage = msg.ResponseMeta.Usage
	}
	return strings.TrimSpace(msg.Content), usage, nil
}

// orchRouterMatchLabel 把模型输出映射到分类目标: 先整段精确匹配 (忽略大小写与首尾
// 空白/引号), 再包含匹配; 都不中返回 false, 由调用方走默认目标
func orchRouterMatchLabel(response string, cases []OrchRouterCase) (string, bool) {
	resp := strings.Trim(strings.TrimSpace(response), "\"'`「」")
	if resp == "" {
		return "", false
	}
	for _, cs := range cases {
		if strings.EqualFold(resp, strings.TrimSpace(cs.Label)) {
			return cs.Target, true
		}
	}
	lower := strings.ToLower(resp)
	for _, cs := range cases {
		if label := strings.TrimSpace(cs.Label); label != "" && strings.Contains(lower, strings.ToLower(label)) {
			return cs.Target, true
		}
	}
	return "", false
}

// ---------- 字段提取节点 ----------

// buildExtractLambda 字段提取节点: 上游内容为 JSON 时按字段路径抽取文本,
// 失败时用 fallback (为空则原样透传), 保证流水线不因脏输出中断
func (c *orchestrationCompiler) buildExtractLambda(n *OrchestrationNode) (*compose.Lambda, error) {
	var cfg OrchExtractConfig
	if len(n.Config) == 0 || json.Unmarshal(n.Config, &cfg) != nil || strings.TrimSpace(cfg.Field) == "" {
		return nil, fmt.Errorf("提取节点 %s 缺少字段路径", n.ID)
	}
	field := strings.TrimSpace(cfg.Field)
	run := func(in *schema.Message) (*schema.Message, error) {
		content := ""
		if in != nil {
			content = in.Content
		}
		out, ok := orchExtractField(content, field)
		if !ok {
			orchLog("extract 未命中 node=%s field=%s 原文长度=%d", n.ID, field, len(content))
			if cfg.Fallback != "" {
				out = cfg.Fallback
			} else {
				out = content
			}
		}
		return &schema.Message{Role: schema.User, Content: out}, nil
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

// orchExtractField 按 a.b.0.c 形式的点号路径从 JSON 内容里抽取值:
// 对象段按键名, 数组段按十进制下标; 抽到字符串原样返回, 其他值 JSON 编码
func orchExtractField(content, path string) (string, bool) {
	raw := orchStripCodeFence(strings.TrimSpace(content))
	if raw == "" || path == "" {
		return "", false
	}
	var root any
	if err := json.Unmarshal([]byte(raw), &root); err != nil {
		return "", false
	}
	cur := root
	for _, seg := range strings.Split(path, ".") {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			return "", false
		}
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return "", false
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return "", false
			}
			cur = node[idx]
		default:
			return "", false
		}
	}
	switch out := cur.(type) {
	case string:
		return out, true
	case nil:
		return "", false
	default:
		b, err := json.Marshal(out)
		if err != nil {
			return "", false
		}
		return string(b), true
	}
}

// orchStripCodeFence 剥掉 markdown 代码围栏 (```json ... ```), LLM 输出常带
func orchStripCodeFence(s string) string {
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 2 || strings.TrimSpace(lines[len(lines)-1]) != "```" {
		return s
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

// ---------- 子编排节点 ----------

// buildSubOrchLambda 子编排节点: 引用另一个已保存编排, 递归编译后作为普通节点执行。
// 嵌套编排的节点 id 由服务层按 "so_<子编排节点id>_" 加前缀 (同一编排被多处引用时
// 各实例不冲突), 调试事件归属与主编排及其他子编排隔离
func (c *orchestrationCompiler) buildSubOrchLambda(ctx context.Context, n *OrchestrationNode) (*compose.Lambda, error) {
	var cfg OrchSubOrchConfig
	if len(n.Config) == 0 || json.Unmarshal(n.Config, &cfg) != nil {
		return nil, fmt.Errorf("子编排节点 %s 配置解析失败", n.ID)
	}
	if cfg.OrchestrationID <= 0 {
		return nil, fmt.Errorf("子编排节点 %s 缺少引用的编排", n.ID)
	}
	refID := uint(cfg.OrchestrationID)
	for _, id := range c.orchChain {
		if id == refID {
			return nil, fmt.Errorf("子编排节点 %s 引用了编排 %d, 存在循环引用", n.ID, refID)
		}
	}
	if len(c.orchChain) >= orchMaxNestDepth {
		return nil, fmt.Errorf("子编排嵌套层级超过 %d 层", orchMaxNestDepth)
	}
	if c.deps.compileNested == nil {
		return nil, fmt.Errorf("子编排节点 %s 编译依赖缺失", n.ID)
	}
	// chain 不含 refID (递归的 compile 会把 refID 追加进去); 编译链已含自身,
	// 这里把 refID 交给服务层查库编译, 循环引用由下一层的编译链检出
	nested, err := c.deps.compileNested(ctx, refID, c.orchChain, fmt.Sprintf("so_%s_", n.ID))
	if err != nil {
		return nil, fmt.Errorf("子编排节点 %s: %w", n.ID, err)
	}
	for k, v := range nested.nodeKeys {
		c.extraNodeKeys[k] = v
	}
	for k := range nested.subNodes {
		c.extraSubNodes[k] = true
	}
	for k, v := range nested.nodeModels {
		c.extraNodeModels[k] = v
	}
	runnable := nested.runnable
	lambda, err := compose.AnyLambda(
		func(ctx context.Context, in *schema.Message, _ ...any) (*schema.Message, error) {
			return runnable.Invoke(ctx, in)
		},
		func(ctx context.Context, in *schema.Message, _ ...any) (*schema.StreamReader[*schema.Message], error) {
			return runnable.Stream(ctx, in)
		},
		nil, nil,
	)
	if err != nil {
		return nil, fmt.Errorf("子编排节点 %s 构建失败: %w", n.ID, err)
	}
	return lambda, nil
}

// prefixOrchestrationDSL 复制 DSL 并给所有节点 id 加前缀 (子编排编译用):
// 节点 key、连线与 branch/router 配置里的目标一并改名, 与主编排及其他子编排
// 的节点空间隔离; 节点显示名保持不变
func prefixOrchestrationDSL(dsl *OrchestrationDSL, prefix string) *OrchestrationDSL {
	out := &OrchestrationDSL{
		Version: dsl.Version,
		Nodes:   make([]OrchestrationNode, 0, len(dsl.Nodes)),
		Edges:   make([]OrchestrationEdge, 0, len(dsl.Edges)),
	}
	remap := make(map[string]string, len(dsl.Nodes))
	for i := range dsl.Nodes {
		if dsl.Nodes[i].ID == "" {
			continue
		}
		remap[dsl.Nodes[i].ID] = prefix + dsl.Nodes[i].ID
	}
	for i := range dsl.Nodes {
		n := &dsl.Nodes[i]
		if n.ID == "" {
			continue
		}
		node := OrchestrationNode{ID: remap[n.ID], Type: n.Type, Name: n.Name, Config: n.Config}
		if n.Type == OrchNodeBranch || n.Type == OrchNodeRouter {
			node.Config = prefixRouteTargets(n.Type, n.Config, remap)
		}
		out.Nodes = append(out.Nodes, node)
	}
	for _, e := range dsl.Edges {
		src, okS := remap[e.Source]
		dst, okT := remap[e.Target]
		if !okS || !okT {
			continue // 悬挂连线交给校验报错
		}
		out.Edges = append(out.Edges, OrchestrationEdge{Source: src, Target: dst, Label: e.Label, Kind: e.Kind})
	}
	return out
}

// prefixRouteTargets 改写 branch/router 配置里的目标节点 id (DefaultTarget/Cases.Target)
func prefixRouteTargets(typ string, raw json.RawMessage, remap map[string]string) json.RawMessage {
	remapTarget := func(t string) string {
		if v, ok := remap[t]; ok {
			return v
		}
		return t
	}
	switch typ {
	case OrchNodeBranch:
		var cfg OrchBranchConfig
		if json.Unmarshal(raw, &cfg) != nil {
			return raw
		}
		cfg.DefaultTarget = remapTarget(cfg.DefaultTarget)
		for i := range cfg.Cases {
			cfg.Cases[i].Target = remapTarget(cfg.Cases[i].Target)
		}
		out, err := json.Marshal(cfg)
		if err != nil {
			return raw
		}
		return out
	default: // router
		var cfg OrchRouterConfig
		if json.Unmarshal(raw, &cfg) != nil {
			return raw
		}
		cfg.DefaultTarget = remapTarget(cfg.DefaultTarget)
		for i := range cfg.Cases {
			cfg.Cases[i].Target = remapTarget(cfg.Cases[i].Target)
		}
		out, err := json.Marshal(cfg)
		if err != nil {
			return raw
		}
		return out
	}
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
	// streamedText 已被实时下发的模型文本 (按当前模型调用计, 图级输出据此去重);
	// 记文本而不是长度: 图级输出与流式内容不是同一段文本时 (如模型后接模板/提取节点)
	// 按长度跳过会误伤, 前缀比对只在真正重复时生效
	streamedText string
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

// mergeCompiled 把编译结果里发现的嵌套节点 key/子Agent 集合并入归属表
// (子编排节点在编译期展开, 其内部节点的 key 不在主编排 DSL 里)
func (h *orchTraceHandler) mergeCompiled(nodeKeys map[string]string, subNodes map[string]bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for k, v := range nodeKeys {
		if _, ok := h.nodeKeys[k]; !ok {
			h.nodeKeys[k] = v
		}
	}
	for k := range subNodes {
		h.subNodes[k] = true
	}
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

// markStreamed 设定/追加"已实时下发"的模型文本 (图级输出据此去重)
func (h *orchTraceHandler) markStreamed(content string, replace bool) {
	if content == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if replace {
		h.streamedText = content
		return
	}
	h.streamedText += content
}

// takeStreamed 取出并清零"已实时下发"的文本
func (h *orchTraceHandler) takeStreamed() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.streamedText
	h.streamedText = ""
	return s
}

// orchSkipStreamed 按"已实时下发"的文本前缀比对去重并递减:
// 同一段文本既走了模型节点回调(已实时推)又走了图级输出(此处)时, 保证只下发一次;
// 图级输出与已下发文本对不上 (模型后接了模板/提取等改写节点) 时原样放行
func orchSkipStreamed(h *orchTraceHandler, content string) string {
	streamed := h.takeStreamed()
	if streamed == "" {
		return content
	}
	if strings.HasPrefix(content, streamed) {
		return content[len(streamed):]
	}
	if strings.HasPrefix(streamed, content) {
		// 分块边界不同: 本块是已下发文本的前缀, 剩余部分留待后续块抵扣
		h.markStreamed(streamed[len(content):], true)
		return ""
	}
	return content
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
	// 主 Agent 与子Agent 的模型都走这条路径: 子Agent 的模型包装器已声明 IsCallbacksEnabled,
	// ark 的回调同样挂在 "<subID>.model" 名下, 全程只有这一处 emit (不会与包装器叠加)。
	directStream := span.comp == "ChatModel"
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
					h.markStreamed(m.Message.Content, firstDelta)
					firstDelta = false
				}
			}
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

// emitNodeEvent 推送一条自定义 node 事件 (emit 未注入时只更新摘要, 不推 SSE)
func (h *orchTraceHandler) emitNodeEvent(payload map[string]any) {
	orchDebugTrace("custom", payload)
	// 推送与摘要更新读取同一份 payload, 持同一把锁串行化
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.emit != nil {
		h.emit("node", payload)
	}
	h.recordCustomLocked(payload)
}

// recordCustomLocked 把自定义事件并入节点调试摘要: 委派工具事件 (delegated=true)
// 记为子Agent 被委派, 路由决策等其余事件按 comp 归档; 调用方需持 h.mu
func (h *orchTraceHandler) recordCustomLocked(payload map[string]any) {
	key, _ := payload["key"].(string)
	if key == "" {
		return
	}
	name, _ := payload["name"].(string)
	owner, _ := payload["owner"].(string)
	status, _ := payload["status"].(string)
	comp, _ := payload["comp"].(string)
	t, ok := h.owners[key]
	if !ok {
		t = &OrchNodeTrace{Key: key, Name: name, Comp: comp}
		h.owners[key] = t
		h.order = append(h.order, key)
	}
	if delegated, _ := payload["delegated"].(bool); delegated {
		t.Delegated = true
	}
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
	if ms, _ := payload["ms"].(int64); ms > 0 {
		t.MS = ms
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
