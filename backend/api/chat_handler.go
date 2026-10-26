package api

import (
	"backend/model"
	"backend/service/agent"
	"backend/service/history"
	"backend/service/kernel"
	"backend/service/tokenusage"
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

func HandleChatStream(agentService *agent.AIAgentService, historyService *history.HistoryService) gin.HandlerFunc {
	return func(c *gin.Context) {
		requestStart := time.Now()
		var req ChatRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format: " + err.Error()})
			return
		}

		userID, ok := currentUserID(c)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// 只放行 user/assistant: 客户端可发任意 role, 原样透传 system 等于允许注入
		// 系统提示; 其余角色一律降级为 user (保序保量, 不影响历史按条数差追加)
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

		// 月度 Token 限额: 入口前置校验, 超限直接拒绝 (请求中不中断, 下一轮生效)
		if err := tokenusage.NewTokenUsageService().CheckTokenQuota(userID); err != nil {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}

		// 请求 ctx: 客户端断开即取消底层运行; cancel 在流结束后由 defer 释放
		iter, cancel, err := agentService.ChatStream(c.Request.Context(), userID, req.AgentID, req.HistoryID, inputMessages, req.Model)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to call AI: " + err.Error()})
			return
		}
		defer cancel()
		log.Printf("[chat] user=%d agent=%d model=%s messages=%d init_ms=%d", userID, req.AgentID, req.Model, len(inputMessages), time.Since(requestStart).Milliseconds())
		// 用户对话内容不落日志 (隐私), 仅记录角色与内容长度
		for _, m := range inputMessages {
			log.Printf("[chat]   [%s] len=%d", m.Role, len(m.Content))
		}

		setupSSE(c)

		state := &agentStreamState{}
		firstTokenLogged := false
		// token 用量落库: 每次模型调用一条 (req.Model 为空 = 默认模型, 记账时解析成真实 code)
		recordUsage := func(u *schema.TokenUsage) {
			if u == nil || u.TotalTokens <= 0 {
				return
			}
			tokenusage.RecordTokenUsage(userID, agentService.ResolveModelCode(req.Model), model.TokenSourceChat, int64(u.PromptTokens), int64(u.CompletionTokens))
		}
		onMessage := func(msg *schema.Message) {
			if !firstTokenLogged {
				firstTokenLogged = true
				log.Printf("chat first token user=%d first_token_ms=%d", userID, time.Since(requestStart).Milliseconds())
			}
			emitUsage(c, msg, recordUsage)
		}
		c.Stream(func(w io.Writer) bool {
			event, ok := iter.Next()
			if !ok {
				log.Printf("chat stream completed user=%d reply_chars=%d total_ms=%d", userID, len(state.assistantReply), time.Since(requestStart).Milliseconds())
				c.SSEvent("message", "[DONE]")

				saveFunc := func() {
					historyID, saveErr := historyService.SaveConversation(&history.SaveConversationParams{
						UserID:           userID,
						HistoryID:        req.HistoryID,
						TrainingType:     req.TrainingType,
						CustomTrainingID: req.CustomTrainingID,
						InputMessages:    inputMessages,
						AssistantReply:   state.assistantReply,
						ThinkingContent:  state.thinking,
					})
					if saveErr == nil && req.HistoryID == 0 {
						c.SSEvent("history_id", gin.H{"history_id": historyID, "title": "AI 训练对话"})
					}
				}

				if req.HistoryID == 0 {
					saveFunc()
				} else {
					kernel.SafeGo("chat:save-conversation", saveFunc)
				}

				return false
			}

			if event.Err != nil {
				c.SSEvent("error", gin.H{"error": event.Err.Error()})
				return false
			}

			if event.Action != nil && event.Action.Interrupted != nil {
				for _, ictx := range event.Action.Interrupted.InterruptContexts {
					if approvalInfo, ok := ictx.Info.(*agent.ToolApprovalInfo); ok {
						log.Printf("[chat] tool_approval required user=%d tool=%s id=%s", userID, approvalInfo.ToolName, ictx.ID)

						savedHistoryID := req.HistoryID
						if savedHistoryID == 0 {
							newID, saveErr := historyService.SaveConversation(&history.SaveConversationParams{
								UserID:        userID,
								HistoryID:     0,
								TrainingType:  req.TrainingType,
								InputMessages: inputMessages,
							})
							if saveErr != nil {
								log.Printf("[chat] save history failed user=%d err=%v", userID, saveErr)
							} else {
								savedHistoryID = newID
								c.SSEvent("history_id", gin.H{"history_id": savedHistoryID, "title": "AI 训练对话"})
							}
						}

						c.SSEvent("tool_approval", gin.H{
							"tool_name":     approvalInfo.ToolName,
							"arguments":     approvalInfo.Arguments,
							"checkpoint_id": fmt.Sprintf("%d_%d", userID, savedHistoryID),
							"interrupt_id":  ictx.ID,
						})
						return false
					}
				}
			}

			return handleAgentStreamEvent(c, event, state, onMessage)
		})
	}
}

// emitUsage 将模型返回的 token 用量以 SSE 下发给前端, 并经 record 落库
func emitUsage(c *gin.Context, msg *schema.Message, record func(*schema.TokenUsage)) {
	if msg == nil || msg.ResponseMeta == nil || msg.ResponseMeta.Usage == nil {
		return
	}
	usage := msg.ResponseMeta.Usage
	if usage.TotalTokens <= 0 {
		return
	}
	record(usage)
	c.SSEvent("message", gin.H{
		"usage": gin.H{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		},
	})
}

// HandleListModels 返回所有已启用的模型列表
func HandleListModels(aiAgentService *agent.AIAgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		models, err := aiAgentService.ListEnabledModels()
		if err != nil {
			SendError(c, "500", "获取模型列表失败: "+err.Error())
			return
		}
		SendSuccess(c, models)
	}
}
