package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
)

// 编排 DSL (Agent Studio): 节点/连线类型定义、JSON 解析与画布校验
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
