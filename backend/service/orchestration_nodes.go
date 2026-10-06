package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"text/template"
	"time"

	coremodel "backend/model"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"
)

// 编排节点 lambda 工厂: 各类型节点构建为 Eino compose Lambda / ReAct Agent
// orchMaxNestDepth 子编排最大嵌套层数 (编译链上的编排个数上限)
const orchMaxNestDepth = 5

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

func maxStepOf(cfg OrchAgentConfig) int {
	if cfg.MaxIterations > 0 {
		return cfg.MaxIterations
	}
	return 25
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
		// (userID=0 为校验/单测构造, 不落库); 来源与本次运行一致 (调试/对话);
		// 节点未指定模型时解析成默认模型的真实 code, 避免空模型归属
		if usage != nil && usage.TotalTokens > 0 && c.userID > 0 {
			source := coremodel.TokenSourceOrchDebug
			if c.chatMode {
				source = coremodel.TokenSourceOrchChat
			}
			modelCode := cfg.Model
			if modelCode == "" && c.deps.resolveModelCode != nil {
				modelCode = c.deps.resolveModelCode("")
			}
			RecordTokenUsage(c.userID, modelCode, source, int64(usage.PromptTokens), int64(usage.CompletionTokens))
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
