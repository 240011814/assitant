package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// 子Agent 委派: 委派工具、子Agent 执行体构建与进度心跳
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
		fmt.Fprintf(&sb, "- 子Agent「%s」: %s (在本轮直接调用工具 subagent_%d, 参数 task 写清要它完成的具体任务; 不要只说已委派却没有调用工具)\n",
			name, desc, i+1)
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

// buildSubReactAgent 构建子Agent 执行体: 引用已有 AI Agent 或内联提示词
// parentID 是委派它的主 Agent 节点 id (进度事件的 owner)
func (c *orchestrationCompiler) buildSubReactAgent(ctx context.Context, id, parentID string, cfg OrchSubAgentConfig) (*react.Agent, error) {
	instruction := strings.TrimSpace(cfg.SystemPrompt)
	if cfg.AgentID > 0 {
		// 经 lookupAgent 取被引用 Agent (不直接用全局 DB, 便于单测替换与失败定位)
		dbAgent, err := c.lookupAgent(uint(cfg.AgentID))
		if err != nil {
			return nil, fmt.Errorf("子Agent 节点 %s 引用的 Agent %d 读取失败: %w", id, cfg.AgentID, err)
		}
		instruction = dbAgent.SystemPrompt
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
