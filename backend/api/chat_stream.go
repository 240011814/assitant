package api

import (
	"io"
	"log"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

// agentStreamState 聚合一轮 Agent 流式运行的累计正文/思考内容 (落历史用)
type agentStreamState struct {
	assistantReply string
	thinking      string
}

// onAgentMessage 每收到一条模型消息 (流式分片或完整消息) 时回调; chat 记账用它, 审批恢复可传 nil
type onAgentMessage func(msg *schema.Message)

// handleAgentStreamEvent 处理一条 ADK 迭代事件的输出部分 (chat 与 tool-approval 恢复共用):
// 工具调用提示 / 流式分片 / 完整消息, 增量经 SSE 下发并累计进 state。
// 返回 true 表示继续迭代。流结束与工具审批中断两个分支两边语义不同 (保存时机/checkpoint 来源), 留在调用方。
func handleAgentStreamEvent(c *gin.Context, event *adk.AgentEvent, state *agentStreamState, onMessage onAgentMessage) bool {
	if event.Output == nil || event.Output.MessageOutput == nil {
		return true
	}
	mv := event.Output.MessageOutput
	if mv.Role == schema.Tool {
		toolName := ""
		if mv.Message != nil {
			toolName = mv.Message.ToolName
		}
		thinking := "正在调用工具..."
		if toolName != "" {
			thinking = "正在调用工具：" + toolName
		}
		c.SSEvent("message", gin.H{"thinking": thinking})
		return true
	}
	if mv.IsStreaming && mv.MessageStream != nil {
		for {
			msg, err := mv.MessageStream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				log.Printf("stream recv error: %v", err)
				break
			}
			emitAgentMessage(c, state, msg, onMessage)
		}
		return true
	}
	if mv.Message != nil {
		emitAgentMessage(c, state, mv.Message, onMessage)
	}
	return true
}

// emitAgentMessage 下发单条消息的思考/正文增量并累计
func emitAgentMessage(c *gin.Context, state *agentStreamState, msg *schema.Message, onMessage onAgentMessage) {
	if msg.ReasoningContent != "" {
		state.thinking += msg.ReasoningContent
		c.SSEvent("message", gin.H{"reasoning_content": msg.ReasoningContent})
	}
	if msg.Content != "" {
		state.assistantReply += msg.Content
		c.SSEvent("message", gin.H{"content": msg.Content})
	}
	if onMessage != nil {
		onMessage(msg)
	}
}
