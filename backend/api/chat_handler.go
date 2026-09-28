package api

import (
	"backend/service"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/cloudwego/eino/schema"
	"github.com/gin-gonic/gin"
)

type ChatMessage struct {
	Role    string `json:"role" binding:"required"`
	Content string `json:"content" binding:"required"`
}

type ChatRequest struct {
	HistoryID        uint          `json:"history_id"`
	TrainingType     string        `json:"training_type"`
	CustomTrainingID *uint         `json:"custom_training_id"`
	AgentID          uint          `json:"agent_id" binding:"required"`
	Model            string        `json:"model"`
	Messages         []ChatMessage `json:"messages" binding:"required"`
}

func HandleChatStream(agentService *service.AIAgentService, historyService *service.HistoryService) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestStart := time.Now()
		var req ChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
			return
		}

		userID, exists := c.Get("userId")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		inputMessages := make([]*schema.Message, len(req.Messages))
		for i, m := range req.Messages {
			inputMessages[i] = &schema.Message{
				Role:    schema.RoleType(m.Role),
				Content: m.Content,
			}
		}

		iter, err := agentService.ChatStream(userID.(uint), req.AgentID, req.HistoryID, inputMessages, req.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to call AI: " + err.Error()})
			return
		}
		log.Printf("[chat] user=%d agent=%d model=%s messages=%d init_ms=%d", userID.(uint), req.AgentID, req.Model, len(inputMessages), time.Since(requestStart).Milliseconds())
		for _, m := range inputMessages {
			log.Printf("[chat]   [%s] %s", m.Role, m.Content)
		}

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")

		var fullAssistantReply string
		var fullThinkingContent string
		firstTokenLogged := false
		c.Stream(func(w io.Writer) bool {
			event, ok := iter.Next()
			if !ok {
				log.Printf("chat stream completed user=%d reply_chars=%d total_ms=%d", userID.(uint), len(fullAssistantReply), time.Since(requestStart).Milliseconds())
				c.SSEvent("message", "[DONE]")

				saveFunc := func() {
					historyID, saveErr := historyService.SaveConversation(&service.SaveConversationParams{
						UserID:           userID.(uint),
						HistoryID:        req.HistoryID,
						TrainingType:     req.TrainingType,
						CustomTrainingID: req.CustomTrainingID,
						InputMessages:    inputMessages,
						AssistantReply:   fullAssistantReply,
						ThinkingContent:  fullThinkingContent,
					})
					if saveErr == nil && req.HistoryID == 0 {
						c.SSEvent("history_id", gin.H{"history_id": historyID, "title": "AI 训练对话"})
					}
				}

				if req.HistoryID == 0 {
					saveFunc()
				} else {
					go saveFunc()
				}

				return false
			}

			if event.Err != nil {
				c.SSEvent("error", gin.H{"error": event.Err.Error()})
				return false
			}

			if event.Action != nil && event.Action.Interrupted != nil {
				for _, ictx := range event.Action.Interrupted.InterruptContexts {
					if approvalInfo, ok := ictx.Info.(*service.ToolApprovalInfo); ok {
						log.Printf("[chat] tool_approval required user=%d tool=%s id=%s", userID.(uint), approvalInfo.ToolName, ictx.ID)

						savedHistoryID := req.HistoryID
						if savedHistoryID == 0 {
							newID, saveErr := historyService.SaveConversation(&service.SaveConversationParams{
								UserID:        userID.(uint),
								HistoryID:     0,
								TrainingType:  req.TrainingType,
								InputMessages: inputMessages,
							})
							if saveErr != nil {
								log.Printf("[chat] save history failed user=%d err=%v", userID.(uint), saveErr)
							} else {
								savedHistoryID = newID
								c.SSEvent("history_id", gin.H{"history_id": savedHistoryID, "title": "AI 训练对话"})
							}
						}

						c.SSEvent("tool_approval", gin.H{
							"tool_name":     approvalInfo.ToolName,
							"arguments":     approvalInfo.Arguments,
							"checkpoint_id": fmt.Sprintf("%d_%d", userID.(uint), savedHistoryID),
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
							log.Printf("stream recv error: %v", err)
							break
						}
						if !firstTokenLogged {
							firstTokenLogged = true
							log.Printf("chat first token user=%d first_token_ms=%d", userID.(uint), time.Since(requestStart).Milliseconds())
						}
						if msg.ReasoningContent != "" {
							fullThinkingContent += msg.ReasoningContent
							c.SSEvent("message", gin.H{"reasoning_content": msg.ReasoningContent})
						}
						if msg.Content != "" {
							fullAssistantReply += msg.Content
							c.SSEvent("message", gin.H{"content": msg.Content})
						}
						emitUsage(c, msg)
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
					emitUsage(c, msg)
				}
			}

			return true
		})
	}
}

// emitUsage 将模型返回的 token 用量以 SSE 下发给前端
func emitUsage(c *gin.Context, msg *schema.Message) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	if usage.TotalTokens <= 0 {
		return
	}
	c.SSEvent("message", gin.H{
		"usage": gin.H{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		},
	})
}

// HandleListModels 返回所有已启用的模型列表
func HandleListModels(aiAgentService *service.AIAgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		models, err := aiAgentService.ListEnabledModels()
		if err != nil {
			SendError(c, "500", "获取模型列表失败: "+err.Error())
			return
		}
		SendSuccess(c, models)
	}
}
