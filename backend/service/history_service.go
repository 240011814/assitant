package service

import (
	interfaces "backend/interface"
	"backend/model"
	"crypto/rand"
	"encoding/hex"
	"log"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/cloudwego/eino/schema"
	"gorm.io/gorm"
)

var (
	htmlTagRegex      = regexp.MustCompile(`<[^>]*>`)
	markdownCharRegex = regexp.MustCompile("[#*`~_]")
)

// truncateUTF8 按 UTF-8 字符边界安全截断，不会切碎多字节字符
func truncateUTF8(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}

type HistoryService struct {
	mem0Service interfaces.Mem0Service
}

func NewHistoryService(mem0Service interfaces.Mem0Service) *HistoryService {
	return &HistoryService{mem0Service: mem0Service}
}

func (s *HistoryService) ListHistory(userID uint, page, pageSize int, title string, isFavorite *bool) ([]model.TrainingHistory, int64, error) {
	var histories []model.TrainingHistory
	var total int64

	query := DB.Model(&model.TrainingHistory{}).Where("user_id = ?", userID)

	if title != "" {
		fuzzy := "%" + title + "%"
		query = query.Where("title LIKE ?", fuzzy)
	}
	if isFavorite != nil {
		query = query.Where("is_favorite = ?", *isFavorite)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Order("created_at DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&histories).Error; err != nil {
		return nil, 0, err
	}

	return histories, total, nil
}

func (s *HistoryService) GetHistoryByID(userID uint, historyID uint) (*model.TrainingHistory, error) {
	var history model.TrainingHistory
	if err := DB.Where("id = ? AND user_id = ?", historyID, userID).First(&history).Error; err != nil {
		return nil, err
	}

	var messages []model.TrainingMessage
	if err := DB.Where("history_id = ?", historyID).Order("sort_order ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	history.Messages = messages

	return &history, nil
}

func (s *HistoryService) UpdateFavorite(userID uint, historyID uint, isFavorite bool) error {
	return DB.Model(&model.TrainingHistory{}).Where("id = ? AND user_id = ?", historyID, userID).Update("is_favorite", isFavorite).Error
}

func (s *HistoryService) UpdateTitle(userID uint, historyID uint, title string) error {
	return DB.Model(&model.TrainingHistory{}).Where("id = ? AND user_id = ?", historyID, userID).Update("title", title).Error
}

func (s *HistoryService) DeleteHistory(userID uint, historyID uint) error {
	// 事务: 先校验归属, 连带清理会话消息, 避免留下孤儿 training_messages
	return DB.Transaction(func(tx *gorm.DB) error {
		var history model.TrainingHistory
		if err := tx.Where("id = ? AND user_id = ?", historyID, userID).First(&history).Error; err != nil {
			return err
		}
		if err := tx.Where("history_id = ?", historyID).Delete(&model.TrainingMessage{}).Error; err != nil {
			return err
		}
		return tx.Delete(&history).Error
	})
}

func extractLastMessage(messages []model.OpenAIMessage) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "system" && messages[i].Content != "" {
			content := messages[i].Content
			content = htmlTagRegex.ReplaceAllString(content, "")
			content = markdownCharRegex.ReplaceAllString(content, "")
			content = strings.Join(strings.Fields(content), " ")
			if len(content) > 200 {
				content = truncateUTF8(content, 200)
			}
			return content
		}
	}
	return ""
}

func (s *HistoryService) SaveHistory(userID uint, historyID uint, trainingType string, customTrainingID *uint, title string, messages []model.OpenAIMessage, isFavorite bool) (uint, error) {
	lastMessage := extractLastMessage(messages)

	if historyID > 0 {
		var history model.TrainingHistory
		if err := DB.First(&history, historyID).Error; err != nil {
			return 0, err
		}
		if history.UserID != userID {
			return 0, nil
		}
		history.Title = title
		history.IsFavorite = isFavorite
		history.LastMessage = lastMessage
		if err := DB.Save(&history).Error; err != nil {
			return 0, err
		}

		// 查询已有消息数量，只追加新消息
		var existingCount int64
		DB.Model(&model.TrainingMessage{}).Where("history_id = ?", historyID).Count(&existingCount)

		if len(messages) > int(existingCount) {
			newMsgs := make([]model.TrainingMessage, 0, len(messages)-int(existingCount))
			for i := int(existingCount); i < len(messages); i++ {
				m := messages[i]
				newMsgs = append(newMsgs, model.TrainingMessage{
					HistoryID:       historyID,
					Role:            m.Role,
					Content:         m.Content,
					ThinkingContent: m.ThinkingContent,
					SortOrder:       i,
				})
			}
			if len(newMsgs) > 0 {
				if err := DB.Create(&newMsgs).Error; err != nil {
					return 0, err
				}
			}
		}

		return historyID, nil
	}

	history := model.TrainingHistory{
		UserID:           userID,
		TrainingType:     trainingType,
		CustomTrainingID: customTrainingID,
		Title:            title,
		IsFavorite:       isFavorite,
		LastMessage:      lastMessage,
	}
	if err := DB.Create(&history).Error; err != nil {
		return 0, err
	}

	msgs := make([]model.TrainingMessage, len(messages))
	for i, m := range messages {
		msgs[i] = model.TrainingMessage{
			HistoryID:       history.ID,
			Role:            m.Role,
			Content:         m.Content,
			ThinkingContent: m.ThinkingContent,
			SortOrder:       i,
		}
	}
	if len(msgs) > 0 {
		if err := DB.Create(&msgs).Error; err != nil {
			return 0, err
		}
	}

	return history.ID, nil
}

func (s *HistoryService) GenerateShareToken(userID uint, historyID uint) (string, error) {
	var history model.TrainingHistory
	if err := DB.Where("id = ? AND user_id = ?", historyID, userID).First(&history).Error; err != nil {
		return "", err
	}

	// 如果已有 token，直接返回
	if history.ShareToken != nil {
		return *history.ShareToken, nil
	}

	// 生成 16 字节随机 token
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	token := hex.EncodeToString(bytes)

	if err := DB.Model(&history).Update("share_token", token).Error; err != nil {
		return "", err
	}

	return token, nil
}

func (s *HistoryService) RevokeShareToken(userID uint, historyID uint) error {
	return DB.Model(&model.TrainingHistory{}).Where("id = ? AND user_id = ?", historyID, userID).Update("share_token", nil).Error
}

func (s *HistoryService) GetSharedHistory(shareToken string) (*model.TrainingHistory, error) {
	var history model.TrainingHistory
	if err := DB.Where("share_token = ?", shareToken).First(&history).Error; err != nil {
		return nil, err
	}

	var messages []model.TrainingMessage
	if err := DB.Where("history_id = ?", history.ID).Order("sort_order ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	history.Messages = messages

	return &history, nil
}

type SaveConversationParams struct {
	UserID           uint
	HistoryID        uint
	TrainingType     string
	CustomTrainingID *uint
	InputMessages    []*schema.Message
	AssistantReply   string
	ThinkingContent  string
}

func (s *HistoryService) SaveConversation(params *SaveConversationParams) (uint, error) {
	allMessages := make([]*schema.Message, 0, len(params.InputMessages)+1)
	allMessages = append(allMessages, params.InputMessages...)
	if params.AssistantReply != "" {
		allMessages = append(allMessages, &schema.Message{
			Role:    schema.Assistant,
			Content: params.AssistantReply,
		})
	}

	msgs := make([]model.OpenAIMessage, len(allMessages))
	for i, m := range allMessages {
		msgs[i] = model.OpenAIMessage{Role: string(m.Role), Content: m.Content}
	}
	if params.ThinkingContent != "" && len(msgs) > 0 {
		msgs[len(msgs)-1].ThinkingContent = params.ThinkingContent
	}

	title := "AI 训练对话"
	for _, m := range params.InputMessages {
		if m.Role == schema.User && m.Content != "" {
			title = m.Content
			runes := []rune(title)
			if len(runes) > 20 {
				title = string(runes[:20]) + "..."
			}
			break
		}
	}

	historyID, err := s.SaveHistory(params.UserID, params.HistoryID, params.TrainingType, params.CustomTrainingID, title, msgs, false)
	if err != nil {
		return 0, err
	}

	if s.mem0Service != nil && s.mem0Service.IsConfigured() {
		memMessages := make([]interfaces.Mem0Message, 0, len(allMessages))
		for _, m := range allMessages {
			if m.Role == schema.System {
				continue
			}
			memMessages = append(memMessages, interfaces.Mem0Message{
				Role:    string(m.Role),
				Content: m.Content,
			})
		}
		if len(memMessages) > 0 {
			if _, err := s.mem0Service.AddMemory(params.UserID, memMessages, nil); err != nil {
				log.Printf("mem0 save memory failed user=%d err=%v", params.UserID, err)
			}
		}
	}

	return historyID, nil
}
