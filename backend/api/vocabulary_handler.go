package api

import (
	"io"
	"strconv"
	"strings"

	"backend/model"
	"backend/service/learning"
	"github.com/gin-gonic/gin"
)

type VocabularyHandler struct {
	svc *learning.VocabularyService
}

func NewVocabularyHandler(svc *learning.VocabularyService) *VocabularyHandler {
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

// HandleSubmitReview 提交复习结果: 兼容 {known: bool}, 推荐 {rating: 1~4, direction, durationMs} (FSRS)
func (h *VocabularyHandler) HandleSubmitReview(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "生词 ID 不合法")
		return
	}
	var req struct {
		Known      *bool  `json:"known"`
		Rating     int    `json:"rating"`
		Direction  string `json:"direction"`
		DurationMs int    `json:"durationMs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	var word *model.Vocabulary
	if req.Rating >= 1 && req.Rating <= 4 {
		word, err = h.svc.SubmitReviewFSRS(userId, uint(id), req.Rating, req.Direction, req.DurationMs)
	} else if req.Known != nil {
		word, err = h.svc.SubmitReview(userId, uint(id), *req.Known)
	} else {
		SendError(c, "400", "请求参数错误")
		return
	}
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

// HandleGetReviewSession 复习会话: 到期复习 + 新词 (每日限额)
func (h *VocabularyHandler) HandleGetReviewSession(c *gin.Context) {
	userId := GetUserID(c)
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	res, err := h.svc.GetReviewSession(userId, limit)
	if err != nil {
		SendError(c, "500", "获取复习会话失败: "+err.Error())
		return
	}
	SendSuccess(c, res)
}

// HandleGetQuickSession 碎片时间模式 (?minutes=10)
func (h *VocabularyHandler) HandleGetQuickSession(c *gin.Context) {
	userId := GetUserID(c)
	minutes, _ := strconv.Atoi(c.DefaultQuery("minutes", "5"))
	res, err := h.svc.GetQuickSession(userId, minutes)
	if err != nil {
		SendError(c, "500", "获取碎片复习失败: "+err.Error())
		return
	}
	SendSuccess(c, res)
}

// HandleGetReviewForecast 未来 N 天到期预测
func (h *VocabularyHandler) HandleGetReviewForecast(c *gin.Context) {
	userId := GetUserID(c)
	days, _ := strconv.Atoi(c.DefaultQuery("days", "30"))
	res, err := h.svc.GetDueForecast(userId, days)
	if err != nil {
		SendError(c, "500", "获取到期预测失败: "+err.Error())
		return
	}
	SendSuccess(c, res)
}

// HandleGetLearningStats 学习看板
func (h *VocabularyHandler) HandleGetLearningStats(c *gin.Context) {
	userId := GetUserID(c)
	res, err := h.svc.GetLearningStats(userId)
	if err != nil {
		SendError(c, "500", "获取学习统计失败: "+err.Error())
		return
	}
	SendSuccess(c, res)
}

// HandleAssistWord 遗忘干预: 生成新例句与助记提示
func (h *VocabularyHandler) HandleAssistWord(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "生词 ID 不合法")
		return
	}
	word, err := h.svc.AssistForgetfulWord(userId, uint(id))
	if err != nil {
		SendError(c, "500", "生成记忆辅助失败: "+err.Error())
		return
	}
	SendSuccess(c, word)
}

// HandleImportWords 词书批量导入: JSON {words:[...]} 或 multipart 文件 (txt/csv)
func (h *VocabularyHandler) HandleImportWords(c *gin.Context) {
	userId := GetUserID(c)

	var items []model.ImportWordItem
	sourceTag := "词书导入"

	if file, err := c.FormFile("file"); err == nil {
		f, err := file.Open()
		if err != nil {
			SendError(c, "400", "读取文件失败")
			return
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, 5<<20))
		if err != nil {
			SendError(c, "400", "读取文件失败")
			return
		}
		if tag := c.PostForm("sourceTag"); tag != "" {
			sourceTag = tag
		}
		items = parseWordbookFile(data)
	} else {
		var req struct {
			Words     []model.ImportWordItem `json:"words" binding:"required,min=1"`
			SourceTag string                 `json:"sourceTag"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			SendError(c, "400", "请求参数错误")
			return
		}
		items = req.Words
		if req.SourceTag != "" {
			sourceTag = req.SourceTag
		}
	}

	res, err := h.svc.ImportWords(userId, items, sourceTag)
	if err != nil {
		SendError(c, "500", "导入失败: "+err.Error())
		return
	}
	SendSuccess(c, res)
}

// parseWordbookFile 解析词书文件: 每行一词, 制表符或逗号分隔 [word, phonetic, definition, example]
func parseWordbookFile(data []byte) []model.ImportWordItem {
	text := strings.TrimPrefix(string(data), "\xEF\xBB\xBF")
	lines := strings.Split(text, "\n")
	items := make([]model.ImportWordItem, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var parts []string
		if strings.Contains(line, "\t") {
			parts = strings.Split(line, "\t")
		} else {
			parts = strings.SplitN(line, ",", 4)
		}
		for i := range parts {
			parts[i] = strings.TrimSpace(strings.Trim(parts[i], `"`))
		}
		if len(parts) == 0 || parts[0] == "" {
			continue
		}
		if strings.EqualFold(parts[0], "word") { // 表头
			continue
		}
		item := model.ImportWordItem{Word: parts[0]}
		if len(parts) > 1 {
			item.Phonetic = parts[1]
		}
		if len(parts) > 2 {
			item.Definition = parts[2]
		}
		if len(parts) > 3 {
			item.Example = parts[3]
		}
		items = append(items, item)
	}
	return items
}

// HandleExtractFromDocument 知识库文档提词入库
func (h *VocabularyHandler) HandleExtractFromDocument(c *gin.Context) {
	userId := GetUserID(c)
	var req struct {
		DocumentID uint `json:"documentId" binding:"required"`
		Limit      int  `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	res, err := h.svc.ExtractWordsFromDocument(userId, req.DocumentID, req.Limit)
	if err != nil {
		SendError(c, "500", "提取生词失败: "+err.Error())
		return
	}
	SendSuccess(c, res)
}
