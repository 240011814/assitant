package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
)

// 编排内工具审批: 需人工确认 (ai_tools.confirm_required) 的工具在编排运行时
// 不再静默执行, 而是经 SSE 推送审批请求并阻塞等待, 前端批准后才执行。
// 拒绝/超时把拒绝原因作为工具结果返回给模型, 由模型基于"未获批"继续作答。
//
// 链路: DebugRun 生成 run_id 放入 ctx 并随 start 事件下发 ->
// orchApprovalTool 调用时注册等待并推 approval_request ->
// 前端弹窗, 用户决定后调 /ai-orchestrations/approvals/resolve ->
// ResolveOrchestrationApproval 唤醒等待, 工具继续或返回拒绝文案。

// orchApprovalWaitTimeout 审批等待上限: 超时视为拒绝 (防止 SSE 连接被无限占用)
const orchApprovalWaitTimeout = 2 * time.Minute

type orchApprovalDecision struct {
	approved bool
	reason   string
}

var (
	orchApprovalMu      sync.Mutex
	orchApprovalWaiters = map[string]chan *orchApprovalDecision{}
)

func orchApprovalKey(runID, callID string) string {
	return runID + "|" + callID
}

func registerOrchApproval(runID, callID string) chan *orchApprovalDecision {
	ch := make(chan *orchApprovalDecision, 1)
	orchApprovalMu.Lock()
	orchApprovalWaiters[orchApprovalKey(runID, callID)] = ch
	orchApprovalMu.Unlock()
	return ch
}

func unregisterOrchApproval(runID, callID string) {
	orchApprovalMu.Lock()
	delete(orchApprovalWaiters, orchApprovalKey(runID, callID))
	orchApprovalMu.Unlock()
}

// ResolveOrchestrationApproval 唤醒一次工具审批等待 (由审批端点调用)。
// 返回 false 表示该等待不存在或已超时清理
func ResolveOrchestrationApproval(runID, callID string, approved bool, reason string) bool {
	orchApprovalMu.Lock()
	ch, ok := orchApprovalWaiters[orchApprovalKey(runID, callID)]
	orchApprovalMu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- &orchApprovalDecision{approved: approved, reason: reason}:
	default: // 重复提交, 保留先到的决定
	}
	return true
}

// orchNewRunID 一次编排运行的唯一标识: 审批请求/决定靠它配对
func orchNewRunID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "orch_" + hex.EncodeToString(b)
}

// orchRunIDKey ctx key: 承载当前运行的 run_id
type orchRunIDKey struct{}

func withOrchRunID(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, orchRunIDKey{}, runID)
}

func orchRunIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(orchRunIDKey{}).(string)
	return v
}

// orchApprovalTool 需人工确认工具的编排包装: 阻塞等待前端审批决定
type orchApprovalTool struct {
	name  string
	inner tool.InvokableTool
	seq   atomic.Int64
}

func (t *orchApprovalTool) Info(ctx context.Context) (*schema.ToolInfo, error) {
	return t.inner.Info(ctx)
}

func (t *orchApprovalTool) InvokableRun(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
	h := orchHandlerFromContext(ctx)
	runID := orchRunIDFromContext(ctx)
	// 无审批通道 (非交互运行): 阻断执行, 不允许静默绕过审批
	if h == nil || runID == "" {
		return "", fmt.Errorf("工具 %s 需要人工审批, 当前运行没有审批通道", t.name)
	}
	callID := fmt.Sprintf("%s-%d", t.name, t.seq.Add(1))
	ch := registerOrchApproval(runID, callID)
	defer unregisterOrchApproval(runID, callID)

	orchLog("approval 等待审批 run=%s tool=%s call=%s args=%.80q", runID, t.name, callID, argumentsInJSON)
	h.safeEmit("approval_request", map[string]any{
		"run_id":    runID,
		"call_id":   callID,
		"tool":      t.name,
		"arguments": argumentsInJSON,
	})

	select {
	case d := <-ch:
		if d.approved {
			orchLog("approval 批准 run=%s tool=%s call=%s", runID, t.name, callID)
			h.safeEmit("approval_result", map[string]any{
				"run_id": runID, "call_id": callID, "approved": true,
			})
			return t.inner.InvokableRun(ctx, argumentsInJSON, opts...)
		}
		orchLog("approval 拒绝 run=%s tool=%s call=%s reason=%q", runID, t.name, callID, d.reason)
		h.safeEmit("approval_result", map[string]any{
			"run_id": runID, "call_id": callID, "approved": false, "reason": d.reason,
		})
		return fmt.Sprintf("工具 %s 的执行被用户拒绝。原因: %s。请不要再次调用该工具, 基于已有信息继续完成任务。", t.name, d.reason), nil
	case <-time.After(orchApprovalWaitTimeout):
		orchLog("approval 超时 run=%s tool=%s call=%s", runID, t.name, callID)
		h.safeEmit("approval_result", map[string]any{
			"run_id": runID, "call_id": callID, "approved": false, "reason": "审批等待超时",
		})
		return fmt.Sprintf("工具 %s 的审批等待超时, 未获得批准。请不要再次调用该工具, 基于已有信息继续完成任务。", t.name), nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}
