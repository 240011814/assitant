package api

import (
	"strconv"

	"backend/model"
	"backend/service/learning"
	"github.com/gin-gonic/gin"
)

type ErrorBookHandler struct {
	svc *learning.ErrorBookService
}

func NewErrorBookHandler(svc *learning.ErrorBookService) *ErrorBookHandler {
	return &ErrorBookHandler{svc: svc}
}

// HandleAddErrorBook 添加错题
func (h *ErrorBookHandler) HandleAddErrorBook(c *gin.Context) {
	userId := GetUserID(c)
	var req model.CreateErrorBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	res, err := h.svc.AddErrorBook(userId, req)
	if err != nil {
		SendError(c, "500", "保存失败: "+err.Error())
		return
	}

	SendSuccess(c, res)
}

// HandleListErrorBooks 获取错题本列表
func (h *ErrorBookHandler) HandleListErrorBooks(c *gin.Context) {
	userId := GetUserID(c)
	sourceType := c.Query("sourceType")
	keyword := c.Query("keyword")
	isMasteredStr := c.Query("isMastered")

	var isMastered *bool
	if isMasteredStr != "" {
		b, err := strconv.ParseBool(isMasteredStr)
		if err == nil {
			isMastered = &b
		}
	}

	list, err := h.svc.GetErrorBookList(userId, sourceType, isMastered, keyword)
	if err != nil {
		SendError(c, "500", "获取列表失败: "+err.Error())
		return
	}

	SendSuccess(c, list)
}

// HandleGetErrorBookForPractice 获取错题练习数据
func (h *ErrorBookHandler) HandleGetErrorBookForPractice(c *gin.Context) {
	userId := GetUserID(c)
	contentType := c.Query("contentType")

	list, err := h.svc.GetErrorBookForPractice(userId, contentType)
	if err != nil {
		SendError(c, "500", "获取练习数据失败: "+err.Error())
		return
	}

	SendSuccess(c, list)
}

// HandleGetRandomErrorBooks 随机获取指定数量的错题
func (h *ErrorBookHandler) HandleGetRandomErrorBooks(c *gin.Context) {
	userId := GetUserID(c)
	countStr := c.DefaultQuery("count", "10")
	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		count = 10
	}
	if count > 100 {
		count = 100
	}

	contentType := c.Query("contentType")

	list, err := h.svc.GetRandomErrorBooks(userId, count, contentType)
	if err != nil {
		SendError(c, "500", "获取随机错题失败: "+err.Error())
		return
	}

	SendSuccess(c, list)
}

// HandleUpdateErrorBook 更新错题状态
func (h *ErrorBookHandler) HandleUpdateErrorBook(c *gin.Context) {
	userId := GetUserID(c)
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	var req model.UpdateErrorBookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.UpdateErrorBook(userId, uint(id), req); err != nil {
		SendError(c, "500", "更新失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleDeleteErrorBook 删除错题
func (h *ErrorBookHandler) HandleDeleteErrorBook(c *gin.Context) {
	userId := GetUserID(c)
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)

	if err := h.svc.DeleteErrorBook(userId, uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleGetErrorBookStats 获取错题统计
func (h *ErrorBookHandler) HandleGetErrorBookStats(c *gin.Context) {
	userId := GetUserID(c)

	stats, err := h.svc.GetErrorBookStats(userId)
	if err != nil {
		SendError(c, "500", "获取统计失败: "+err.Error())
		return
	}

	SendSuccess(c, stats)
}
