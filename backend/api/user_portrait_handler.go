package api

import (
	"backend/model"
	"backend/service"
	"encoding/json"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type UserPortraitHandler struct {
	svc *service.UserMemoryService
}

func NewUserPortraitHandler(svc *service.UserMemoryService) *UserPortraitHandler {
	return &UserPortraitHandler{svc: svc}
}

type portraitView struct {
	Summary           string            `json:"summary"`
	Dimensions        map[string]string `json:"dimensions"`
	Tags              []string          `json:"tags"`
	Confidence        float64           `json:"confidence"`
	IsUserEdited      bool              `json:"is_user_edited"`
	ExtractionEnabled bool              `json:"extraction_enabled"`
	UpdatedAt         *time.Time        `json:"updated_at"`
}

type experienceView struct {
	ID           uint       `json:"id"`
	HistoryID    *uint      `json:"history_id"`
	Domain       string     `json:"domain"`
	EventType    string     `json:"event_type"`
	Title        string     `json:"title"`
	Content      string     `json:"content"`
	TimeRange    string     `json:"time_range"`
	Tags         []string   `json:"tags"`
	OccurredAt   *time.Time `json:"occurred_at"`
	Confidence   float64    `json:"confidence"`
	Importance   int        `json:"importance"`
	MemoryLevel  string     `json:"memory_level"`
	Evidence     string     `json:"evidence"`
	Status       string     `json:"status"`
	IsUserEdited bool       `json:"is_user_edited"`
	CreatedAt    time.Time  `json:"created_at"`
}

func toPortraitView(p *model.UserPortrait) *portraitView {
	if p == nil {
		return nil
	}
	v := &portraitView{
		Summary:           p.Summary,
		Dimensions:        map[string]string{},
		Tags:              []string{},
		Confidence:        p.Confidence,
		IsUserEdited:      p.IsUserEdited,
		ExtractionEnabled: p.ExtractionEnabled,
		UpdatedAt:         &p.UpdatedAt,
	}
	_ = json.Unmarshal([]byte(p.Dimensions), &v.Dimensions)
	_ = json.Unmarshal([]byte(p.Tags), &v.Tags)
	if v.Dimensions == nil {
		v.Dimensions = map[string]string{}
	}
	if v.Tags == nil {
		v.Tags = []string{}
	}
	return v
}

func toExperienceViews(list []model.UserExperience) []experienceView {
	views := make([]experienceView, 0, len(list))
	for _, e := range list {
		var tags []string
		_ = json.Unmarshal([]byte(e.Tags), &tags)
		if tags == nil {
			tags = []string{}
		}
		views = append(views, experienceView{
			ID:           e.ID,
			HistoryID:    e.HistoryID,
			Domain:       e.Domain,
			EventType:    e.EventType,
			Title:        e.Title,
			Content:      e.Content,
			TimeRange:    e.TimeRange,
			Tags:         tags,
			OccurredAt:   e.OccurredAt,
			Confidence:   e.Confidence,
			Importance:   e.Importance,
			MemoryLevel:  e.MemoryLevel,
			Evidence:     e.Evidence,
			Status:       e.Status,
			IsUserEdited: e.IsUserEdited,
			CreatedAt:    e.CreatedAt,
		})
	}
	return views
}

func userIDFromContext(c *gin.Context) (uint, bool) {
	val, exists := c.Get("userId")
	if !exists {
		return 0, false
	}
	id, ok := val.(uint)
	return id, ok
}

// GetPortrait 获取画像 + 经历列表(首屏)
func (h *UserPortraitHandler) GetPortrait(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	portrait, err := h.svc.GetPortrait(userID)
	if err != nil {
		SendError(c, "500", "获取画像失败: "+err.Error())
		return
	}
	_, experiences, err := h.svc.ListExperiences(userID, 1, 100, "", "")
	if err != nil {
		SendError(c, "500", "获取经历失败: "+err.Error())
		return
	}

	SendSuccess(c, gin.H{
		"portrait":    toPortraitView(portrait),
		"experiences": toExperienceViews(experiences),
	})
}

func (h *UserPortraitHandler) UpdatePortrait(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	var req model.UpdateUserPortraitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdatePortrait(userID, req); err != nil {
		SendError(c, "500", "保存画像失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *UserPortraitHandler) ListExperiences(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	domain := c.Query("domain")
	keyword := c.Query("keyword")

	total, list, err := h.svc.ListExperiences(userID, page, pageSize, domain, keyword)
	if err != nil {
		SendError(c, "500", "获取经历失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{
		"total": total,
		"items": toExperienceViews(list),
	})
}

func (h *UserPortraitHandler) CreateExperience(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}

	var req model.CreateUserExperienceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	exp, err := h.svc.CreateExperience(userID, req)
	if err != nil {
		SendError(c, "500", "新增经历失败: "+err.Error())
		return
	}
	SendSuccess(c, toExperienceViews([]model.UserExperience{*exp})[0])
}

func (h *UserPortraitHandler) UpdateExperience(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的经历ID")
		return
	}

	var req model.UpdateUserExperienceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateExperience(userID, uint(id), req); err != nil {
		SendError(c, "500", "更新经历失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *UserPortraitHandler) DeleteExperience(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的经历ID")
		return
	}
	if err := h.svc.DeleteExperience(userID, uint(id)); err != nil {
		SendError(c, "500", "删除经历失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// TriggerExtract 手动触发当前用户的画像/经历抽取
func (h *UserPortraitHandler) TriggerExtract(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		SendError(c, "401", "Unauthorized")
		return
	}
	count, err := h.svc.ExtractUserSessions(userID)
	if err != nil {
		SendError(c, "500", "触发抽取失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"extracted": count})
}
