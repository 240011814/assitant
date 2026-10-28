package api

import (
	"strconv"

	"backend/model"
	"backend/service/learning"
	"github.com/gin-gonic/gin"
)

type RSSHandler struct {
	svc *learning.RSSService
}

func NewRSSHandler(svc *learning.RSSService) *RSSHandler {
	return &RSSHandler{svc: svc}
}

// HandleAddRSSSource 添加订阅 (先验证 feed 可解析)
func (h *RSSHandler) HandleAddRSSSource(c *gin.Context) {
	userId := GetUserID(c)
	var req model.AddRSSSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	source, err := h.svc.AddSource(userId, req)
	if err != nil {
		SendError(c, "500", "添加订阅失败: "+err.Error())
		return
	}
	SendSuccess(c, source)
}

// HandleListRSSSources 订阅源列表 (含未读数)
func (h *RSSHandler) HandleListRSSSources(c *gin.Context) {
	userId := GetUserID(c)
	list, err := h.svc.ListSourcesWithUnread(userId)
	if err != nil {
		SendError(c, "500", "获取订阅列表失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
}

// HandleUpdateRSSSource 编辑订阅 (改名 / 换 feed 地址, 空字段保持原值)
func (h *RSSHandler) HandleUpdateRSSSource(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "订阅 ID 不合法")
		return
	}
	var req model.UpdateRSSSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	source, err := h.svc.UpdateSource(userId, uint(id), req)
	if err != nil {
		SendError(c, "500", "更新失败: "+err.Error())
		return
	}
	SendSuccess(c, source)
}

// HandleDeleteRSSSource 删除订阅源及其条目
func (h *RSSHandler) HandleDeleteRSSSource(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "订阅 ID 不合法")
		return
	}
	if err := h.svc.DeleteSource(userId, uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleFetchRSSSource 手动抓取单个源
func (h *RSSHandler) HandleFetchRSSSource(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "订阅 ID 不合法")
		return
	}
	inserted, err := h.svc.FetchSource(userId, uint(id))
	if err != nil {
		SendError(c, "500", "抓取失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"inserted": inserted})
}

// HandleListRSSItems 文章列表 (分页 + 源/未读过滤)
func (h *RSSHandler) HandleListRSSItems(c *gin.Context) {
	userId := GetUserID(c)
	var req model.ListRSSItemsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	list, total, err := h.svc.ListItems(userId, req)
	if err != nil {
		SendError(c, "500", "获取文章列表失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"list": list, "total": total})
}

// HandleMarkRSSItemRead 标记已读
func (h *RSSHandler) HandleMarkRSSItemRead(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文章 ID 不合法")
		return
	}
	if err := h.svc.MarkItemRead(userId, uint(id)); err != nil {
		SendError(c, "500", "标记失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleGetRSSItemContent 取条目正文 (按需抓取原文页并缓存)
func (h *RSSHandler) HandleGetRSSItemContent(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文章 ID 不合法")
		return
	}
	item, err := h.svc.GetItemContent(userId, uint(id))
	if err != nil {
		SendError(c, "500", "获取正文失败: "+err.Error())
		return
	}
	SendSuccess(c, item)
}

// HandleImportRSSItem 把条目导入分级阅读
func (h *RSSHandler) HandleImportRSSItem(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文章 ID 不合法")
		return
	}
	doc, err := h.svc.ImportToReading(userId, uint(id))
	if err != nil {
		SendError(c, "500", "导入失败: "+err.Error())
		return
	}
	SendSuccess(c, doc)
}

// HandleAnnotateRSSItem 一键提取本文生词 (异步任务, 前端轮询 /api/async-jobs/:id)
func (h *RSSHandler) HandleAnnotateRSSItem(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文章 ID 不合法")
		return
	}
	job, err := h.svc.StartItemAnnotate(userId, uint(id))
	if err != nil {
		SendError(c, "500", "启动提取失败: "+err.Error())
		return
	}
	SendSuccess(c, job)
}

// HandleTranslateRSSItem 生成中英对照 (段落级异步翻译任务, 前端轮询 /api/async-jobs/:id)
func (h *RSSHandler) HandleTranslateRSSItem(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "文章 ID 不合法")
		return
	}
	job, err := h.svc.StartItemTranslate(userId, uint(id))
	if err != nil {
		SendError(c, "500", "启动翻译失败: "+err.Error())
		return
	}
	SendSuccess(c, job)
}
