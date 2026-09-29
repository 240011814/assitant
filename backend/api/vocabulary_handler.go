package api

import (
	"strconv"
	"strings"

	"backend/model"
	"backend/service"
	"github.com/gin-gonic/gin"
)

type VocabularyHandler struct {
	svc *service.VocabularyService
}

func NewVocabularyHandler(svc *service.VocabularyService) *VocabularyHandler {
	return &VocabularyHandler{svc: svc}
}

// HandleAddWord 添加生词
func (h *VocabularyHandler) HandleAddWord(c *gin.Context) {
	userId := GetUserID(c)
	var req model.CreateVocabularyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	res, err := h.svc.AddWord(userId, req)
	if err != nil {
		SendError(c, "500", "保存失败: "+err.Error())
		return
	}

	SendSuccess(c, res)
}

// HandleListWords 获取生词列表 (支持 ids=1,2,3 按主键过滤)
func (h *VocabularyHandler) HandleListWords(c *gin.Context) {
	userId := GetUserID(c)

	// 按 id 列表查询: 错题按词练习等场景只取指定词, 避免拉全量
	if idsStr := c.Query("ids"); idsStr != "" {
		var ids []uint
		for _, part := range strings.Split(idsStr, ",") {
			if id, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32); err == nil {
				ids = append(ids, uint(id))
			}
		}
		list, err := h.svc.GetVocabularyByIDs(userId, ids)
		if err != nil {
			SendError(c, "500", "获取列表失败: "+err.Error())
			return
		}
		SendSuccess(c, list)
		return
	}

	keyword := c.Query("keyword")
	isMasteredStr := c.Query("isMastered")

	var isMastered *bool
	if isMasteredStr != "" {
		b, err := strconv.ParseBool(isMasteredStr)
		if err == nil {
			isMastered = &b
		}
	}

	list, err := h.svc.GetUserVocabulary(userId, keyword, isMastered)
	if err != nil {
		SendError(c, "500", "获取列表失败: "+err.Error())
		return
	}

	SendSuccess(c, list)
}

// HandleDeleteWord 删除生词
func (h *VocabularyHandler) HandleDeleteWord(c *gin.Context) {
	userId := GetUserID(c)
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	if err := h.svc.DeleteWord(userId, uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleGetDueWords 今日到期复习词 (SRS)
func (h *VocabularyHandler) HandleGetDueWords(c *gin.Context) {
	userId := GetUserID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	list, err := h.svc.GetDueWords(userId, limit)
	if err != nil {
		SendError(c, "500", "获取复习词失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
}

// HandleGetReviewStats 复习概况 (到期数/学习中/盒子分布)
func (h *VocabularyHandler) HandleGetReviewStats(c *gin.Context) {
	userId := GetUserID(c)
	stats, err := h.svc.GetReviewStats(userId)
	if err != nil {
		SendError(c, "500", "获取复习统计失败: "+err.Error())
		return
	}
	SendSuccess(c, stats)
}

// HandleSubmitReview 提交复习结果 {known: bool}
func (h *VocabularyHandler) HandleSubmitReview(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "生词 ID 不合法")
		return
	}
	var req struct {
		Known bool `json:"known"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	word, err := h.svc.SubmitReview(userId, uint(id), req.Known)
	if err != nil {
		SendError(c, "500", "提交复习结果失败: "+err.Error())
		return
	}
	SendSuccess(c, word)
}

// HandleGetRandomWords 随机获取指定数量的生词
func (h *VocabularyHandler) HandleGetRandomWords(c *gin.Context) {
	userId := GetUserID(c)
	countStr := c.DefaultQuery("count", "10")
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		count = 10
	}
	if count > 100 {
		count = 100
	}

	isMasteredStr := c.Query("isMastered")
	var isMastered *bool
	if isMasteredStr != "" {
		b, err := strconv.ParseBool(isMasteredStr)
		if err == nil {
			isMastered = &b
		}
	}

	list, err := h.svc.GetRandomWords(userId, count, isMastered)
	if err != nil {
		SendError(c, "500", "获取随机词汇失败: "+err.Error())
		return
	}

	SendSuccess(c, list)
}

// HandleUpdateWord 更新生词
func (h *VocabularyHandler) HandleUpdateWord(c *gin.Context) {
	userId := GetUserID(c)
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	var req model.UpdateVocabularyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.UpdateWord(userId, uint(id), req); err != nil {
		SendError(c, "500", "更新失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}
