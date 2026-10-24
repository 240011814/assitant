package api

import (
	"strconv"

	"backend/model"
	"backend/service"
	"github.com/gin-gonic/gin"
)

type ReadingHandler struct {
	svc *service.ReadingService
}

func NewReadingHandler(svc *service.ReadingService) *ReadingHandler {
	return &ReadingHandler{svc: svc}
}

// HandleImportReading 导入阅读材料 (URL 或文本)
func (h *ReadingHandler) HandleImportReading(c *gin.Context) {
	userId := GetUserID(c)
	var req model.ReadingImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	doc, err := h.svc.ImportDoc(userId, req)
	if err != nil {
		SendError(c, "500", "导入失败: "+err.Error())
		return
	}
	SendSuccess(c, doc)
}

// HandleListReadingDocs 文档列表 (keyword 模糊匹配标题与正文)
func (h *ReadingHandler) HandleListReadingDocs(c *gin.Context) {
	userId := GetUserID(c)
	list, err := h.svc.ListDocs(userId, c.Query("keyword"))
	if err != nil {
		SendError(c, "500", "获取列表失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
}

// HandleGetReadingDoc 文档详情 (含标注)
func (h *ReadingHandler) HandleGetReadingDoc(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文档 ID 不合法")
		return
	}
	doc, words, err := h.svc.GetDoc(userId, uint(id))
	if err != nil {
		SendError(c, "500", "获取文档失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"doc": doc, "difficultWords": words})
}

// HandleDeleteReadingDoc 删除文档
func (h *ReadingHandler) HandleDeleteReadingDoc(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文档 ID 不合法")
		return
	}
	if err := h.svc.DeleteDoc(userId, uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleAnnotateReadingDoc 异步启动 LLM 标注 (立即返回 annotating, 前端轮询文档状态)
func (h *ReadingHandler) HandleAnnotateReadingDoc(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文档 ID 不合法")
		return
	}
	doc, err := h.svc.StartAnnotate(userId, uint(id))
	if err != nil {
		SendError(c, "500", "启动标注失败: "+err.Error())
		return
	}
	SendSuccess(c, doc)
}

// HandleGetGenerationJob 查询慢生成任务状态 (AI 写课文等, 前端轮询)
func (h *ReadingHandler) HandleGetGenerationJob(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "任务 ID 不合法")
		return
	}
	job, err := h.svc.GetGenerationJob(userId, uint(id))
	if err != nil {
		SendError(c, "500", "查询任务失败: "+err.Error())
		return
	}
	SendSuccess(c, job)
}

// HandleLookupWord 点词即查 (词典缓存)
func (h *ReadingHandler) HandleLookupWord(c *gin.Context) {
	userId := GetUserID(c)
	word := c.Query("word")
	if word == "" {
		SendError(c, "400", "缺少 word 参数")
		return
	}
	entry, err := h.svc.LookupWord(userId, word)
	if err != nil {
		SendError(c, "500", "查词失败: "+err.Error())
		return
	}
	SendSuccess(c, entry)
}

// HandleGeneratePassage 异步启动 AI 写课文 (返回任务, 前端轮询)
func (h *ReadingHandler) HandleGeneratePassage(c *gin.Context) {
	userId := GetUserID(c)
	var req model.GeneratePassageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	job, err := h.svc.StartGeneratePassage(userId, req)
	if err != nil {
		SendError(c, "500", "启动生成失败: "+err.Error())
		return
	}
	SendSuccess(c, job)
}
