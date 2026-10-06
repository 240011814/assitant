package api

import (
	"backend/service"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

type ToolApprovalRequest struct {
	CheckPointID     string        `json:"checkpoint_id" binding:"required"`
	InterruptID      string        `json:"interrupt_id" binding:"required"`
	Approved         bool          `json:"approved"`
	Reason           string        `json:"reason,omitempty"`
	HistoryID        uint          `json:"history_id"`
	TrainingType     string        `json:"training_type"`
	CustomTrainingID *uint         `json:"custom_training_id"`
	// Model 发起被中断那次对话时使用的模型 (前端随审批带回), 恢复时用同一 runner
	Model            string        `json:"model"`
	Messages         []ChatMessage `json:"messages"`
}

func HandleToolApproval(agentService *service.AIAgentService, historyService *service.HistoryService) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req ToolApprovalRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		userID, ok := currentUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// Parse checkpoint_id format: userID_historyID
		// 必须与当前登录用户匹配, 防止替他人恢复/批准挂起的工具调用 (IDOR)
		parts := strings.SplitN(req.CheckPointID, "_", 2)
		if len(parts) != 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checkpoint_id"})
			return
		}
		ownerID, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil || uint(ownerID) != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "checkpoint 不属于当前用户"})
			return
		}
		hid, err := strconv.ParseUint(parts[1], 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid checkpoint_id"})
			return
		}
		historyID := uint(hid)

		// 与 HandleChatStream 同规则: 只放行 user/assistant, 其余降级为 user
		inputMessages := make([]*schema.Message, len(req.Messages))
		for i, m := range req.Messages {
			role := schema.User
			if m.Role == "assistant" {
				role = schema.Assistant
			}
			inputMessages[i] = &schema.Message{
				Role:    role,
				Content: m.Content,
			}
		}

		iter, cancel, err := agentService.ResumeToolApproval(c.Request.Context(), req.CheckPointID, req.InterruptID, req.Approved, req.Reason, req.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to resume: " + err.Error()})
			return
		}
		defer cancel()

		setupSSE(c)

		var fullAssistantReply string
		var fullThinkingContent string

		c.Stream(func(w io.Writer) bool {
			event, ok := iter.Next()
			if !ok {
				log.Printf("[tool-approval] stream completed user=%d reply_chars=%d", userID, len(fullAssistantReply))
				c.SSEvent("message", "[DONE]")

				go func() {
					if _, saveErr := historyService.SaveConversation(&service.SaveConversationParams{
						UserID:           userID,
						HistoryID:        historyID,
						TrainingType:     req.TrainingType,
						CustomTrainingID: req.CustomTrainingID,
						InputMessages:    inputMessages,
						AssistantReply:   fullAssistantReply,
						ThinkingContent:  fullThinkingContent,
					}); saveErr != nil {
						log.Printf("[tool-approval] 保存对话历史失败 userID=%d historyID=%d: %v", userID, historyID, saveErr)
					}
				}()

				return false
			}

			if event.Err != nil {
				c.SSEvent("error", gin.H{"error": event.Err.Error()})
				return false
			}

			if event.Action != nil && event.Action.Interrupted != nil {
				for _, ictx := range event.Action.Interrupted.InterruptContexts {
					if approvalInfo, ok := ictx.Info.(*service.ToolApprovalInfo); ok {
						c.SSEvent("tool_approval", gin.H{
							"tool_name":     approvalInfo.ToolName,
							"arguments":     approvalInfo.Arguments,
							"checkpoint_id": req.CheckPointID,
							"interrupt_id":  ictx.ID,
						})
						return false
					}
				}
			}

			if event.Output != nil && event.Output.MessageOutput != nil {
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
						if errors.Is(err, io.EOF) {
							break
						}
						if err != nil {
							break
						}
						if msg.ReasoningContent != "" {
							fullThinkingContent += msg.ReasoningContent
							c.SSEvent("message", gin.H{"reasoning_content": msg.ReasoningContent})
						}
						if msg.Content != "" {
							fullAssistantReply += msg.Content
							c.SSEvent("message", gin.H{"content": msg.Content})
						}
					}
				} else if mv.Message != nil {
					msg := mv.Message
					if msg.ReasoningContent != "" {
						fullThinkingContent += msg.ReasoningContent
						c.SSEvent("message", gin.H{"reasoning_content": msg.ReasoningContent})
					}
					if msg.Content != "" {
						fullAssistantReply += msg.Content
						c.SSEvent("message", gin.H{"content": msg.Content})
					}
				}
			}

			return true
		})
	}
}
