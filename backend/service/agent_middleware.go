package service

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"sync"

	"backend/model"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/middlewares/patchtoolcalls"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func init() {
	gob.Register(&ToolApprovalInfo{})
	gob.Register(&ToolApprovalResult{})
}

type ToolApprovalInfo struct {
	ToolName  string `json:"tool_name"`
	Arguments string `json:"arguments"`
	CallID    string `json:"call_id"`
}

type ToolApprovalResult struct {
	Approved bool   `json:"approved"`
	Reason   string `json:"reason,omitempty"`
}

// confirmRequiredCache caches tool confirm_required status
var (
	confirmRequiredCache = make(map[string]bool)
	confirmRequiredMu    sync.RWMutex
)

// InvalidateConfirmRequiredCache clears the cache for a specific tool or all tools
func InvalidateConfirmRequiredCache(toolName string) {
	confirmRequiredMu.Lock()
	defer confirmRequiredMu.Unlock()
	if toolName == "" {
		confirmRequiredCache = make(map[string]bool)
	} else {
		delete(confirmRequiredCache, toolName)
	}
}

// IsConfirmRequired checks if a tool requires confirmation, with caching
func isConfirmRequired(toolName string) bool {
	confirmRequiredMu.RLock()
	if cached, ok := confirmRequiredCache[toolName]; ok {
		confirmRequiredMu.RUnlock()
		return cached
	}
	confirmRequiredMu.RUnlock()

	var dbTool model.AITool
	if err := DB.Where("name = ? AND enabled = ?", toolName, true).First(&dbTool).Error; err != nil {
		return false
	}

	confirmRequiredMu.Lock()
	confirmRequiredCache[toolName] = dbTool.ConfirmRequired
	confirmRequiredMu.Unlock()

	return dbTool.ConfirmRequired
}

type approvalMiddleware struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.Message]
}

func (m *approvalMiddleware) WrapInvokableToolCall(_ context.Context, endpoint adk.InvokableToolCallEndpoint, tCtx *adk.ToolContext) (adk.InvokableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (string, error) {
		if !isConfirmRequired(tCtx.Name) {
			return endpoint(ctx, argumentsInJSON, opts...)
		}

		wasInterrupted, _, storedArgs := compose.GetInterruptState[string](ctx)

		if !wasInterrupted {
			return "", compose.StatefulInterrupt(ctx, &ToolApprovalInfo{
				ToolName:  tCtx.Name,
				Arguments: argumentsInJSON,
				CallID:    tCtx.CallID,
			}, argumentsInJSON)
		}

		isTarget, hasData, data := compose.GetResumeContext[*ToolApprovalResult](ctx)
		if isTarget && hasData {
			if data.Approved {
				return endpoint(ctx, storedArgs, opts...)
			}
			reason := "操作已被用户拒绝"
			if data.Reason != "" {
				reason = fmt.Sprintf("操作已被用户拒绝: %s", data.Reason)
			}
			return reason, nil
		}

		return "", compose.StatefulInterrupt(ctx, &ToolApprovalInfo{
			ToolName:  tCtx.Name,
			Arguments: storedArgs,
			CallID:    tCtx.CallID,
		}, storedArgs)
	}, nil
}

func (m *approvalMiddleware) WrapStreamableToolCall(_ context.Context, endpoint adk.StreamableToolCallEndpoint, tCtx *adk.ToolContext) (adk.StreamableToolCallEndpoint, error) {
	return func(ctx context.Context, argumentsInJSON string, opts ...tool.Option) (*schema.StreamReader[string], error) {
		if !isConfirmRequired(tCtx.Name) {
			return endpoint(ctx, argumentsInJSON, opts...)
		}

		wasInterrupted, _, storedArgs := compose.GetInterruptState[string](ctx)

		if !wasInterrupted {
			return nil, compose.StatefulInterrupt(ctx, &ToolApprovalInfo{
				ToolName:  tCtx.Name,
				Arguments: argumentsInJSON,
				CallID:    tCtx.CallID,
			}, argumentsInJSON)
		}

		isTarget, hasData, data := compose.GetResumeContext[*ToolApprovalResult](ctx)
		if isTarget && hasData {
			if data.Approved {
				return endpoint(ctx, storedArgs, opts...)
			}
			reason := "操作已被用户拒绝"
			if data.Reason != "" {
				reason = fmt.Sprintf("操作已被用户拒绝: %s", data.Reason)
			}
			return schema.StreamReaderFromArray([]string{reason}), nil
		}

		return nil, compose.StatefulInterrupt(ctx, &ToolApprovalInfo{
			ToolName:  tCtx.Name,
			Arguments: storedArgs,
			CallID:    tCtx.CallID,
		}, storedArgs)
	}, nil
}

func NewApprovalMiddleware() adk.ChatModelAgentMiddleware {
	return &approvalMiddleware{}
}

type loggingMiddleware struct {
	adk.TypedBaseChatModelAgentMiddleware[*schema.Message]
}

func (m *loggingMiddleware) BeforeModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.Message], _ *adk.TypedModelContext[*schema.Message]) (context.Context, *adk.TypedChatModelAgentState[*schema.Message], error) {
	log.Printf("[eino] chat model start: %d messages", len(state.Messages))
	// 对话正文不落日志 (隐私), 仅记录角色与内容长度
	for _, msg := range state.Messages {
		log.Printf("[eino]   [%s] len=%d", msg.Role, len(msg.Content))
	}
	return ctx, state, nil
}

func (m *loggingMiddleware) AfterModelRewriteState(ctx context.Context, state *adk.TypedChatModelAgentState[*schema.Message], _ *adk.TypedModelContext[*schema.Message]) (context.Context, *adk.TypedChatModelAgentState[*schema.Message], error) {
	for _, msg := range state.Messages {
		if msg.Role == schema.Assistant {
			if msg.ResponseMeta != nil && msg.ResponseMeta.Usage != nil && msg.ResponseMeta.Usage.TotalTokens > 0 {
				log.Printf("[eino] chat model end: prompt_tokens=%d completion_tokens=%d total_tokens=%d",
					msg.ResponseMeta.Usage.PromptTokens,
					msg.ResponseMeta.Usage.CompletionTokens,
					msg.ResponseMeta.Usage.TotalTokens)
			}
			if msg.Content != "" {
				log.Printf("[eino] chat model end: reply_len=%d", len(msg.Content))
			}
		}
	}
	return ctx, state, nil
}

func NewLoggingMiddleware() adk.ChatModelAgentMiddleware {
	return &loggingMiddleware{}
}

// BuildPatchToolCallsMiddleware 修补会话历史中「悬空的工具调用」
// (assistant 消息有 tool_calls 但缺少对应的 tool result), 保证发往模型的输入合法
func BuildPatchToolCallsMiddleware(ctx context.Context) (adk.ChatModelAgentMiddleware, error) {
	return patchtoolcalls.New(ctx, nil)
}
