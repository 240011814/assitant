package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/cloudwego/eino/callbacks"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// 调试运行追踪与流式处理: compose 回调事件归属、流式去重与调试日志
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

// orchLog 编排调试日志: 设 ORCH_LOG=1 (或 ORCH_DEBUG=1) 后打到 stderr,
// ORCH_LOG_FILE 可同时落文件; 用于排查"事件推了/没推、归属对不对"
func orchLog(format string, args ...any) {
	if os.Getenv("ORCH_LOG") == "" && os.Getenv("ORCH_DEBUG") == "" {
		return
	}
	line := fmt.Sprintf("[orch] "+format, args...)
	_, _ = fmt.Fprintln(os.Stderr, line)
	if path := os.Getenv("ORCH_LOG_FILE"); path != "" {
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
			_, _ = fmt.Fprintln(f, line)
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
