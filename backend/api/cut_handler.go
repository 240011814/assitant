package api

import (
	"strconv"

	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

type CutHandler struct {
	svc *service.CutService
}

func NewCutHandler(svc *service.CutService) *CutHandler {
	return &CutHandler{svc: svc}
}

// HandleBarCut 一维切割
func (h *CutHandler) HandleBarCut(c *gin.Context) {
	userID := GetUserID(c)
	var req model.BarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	result, err := h.svc.BarCut(userID, req)
	if err != nil {
		SendError(c, "500", "切割失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandlePlaneCut 平面切割
func (h *CutHandler) HandlePlaneCut(c *gin.Context) {
	var req model.BinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	result, err := h.svc.PlaneCut(req)
	if err != nil {
		SendError(c, "500", "切割失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleUpdateScrap 修改库存余料 (数量/名称/备注)
func (h *CutHandler) HandleUpdateScrap(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "余料 ID 不合法")
		return
	}
	var req model.UpdateScrapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	if err := h.svc.UpdateScrap(userID, uint(id), req); err != nil {
		SendError(c, "500", "修改余料失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleListScraps 余料库存列表 (分页 + 名称/长度/类型筛选)
func (h *CutHandler) HandleListScraps(c *gin.Context) {
	userID := GetUserID(c)
	var params model.CutScrapSearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.ListScraps(userID, params)
	if err != nil {
		SendError(c, "500", "获取余料库存失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleAddScraps 余料批量入库 (结果页一键入库)
func (h *CutHandler) HandleAddScraps(c *gin.Context) {
	userID := GetUserID(c)
	var reqs []model.AddCutScrapRequest
	if err := c.ShouldBindJSON(&reqs); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	rows, err := h.svc.AddScraps(userID, reqs)
	if err != nil {
		SendError(c, "500", "余料入库失败: "+err.Error())
		return
	}
	SendSuccess(c, rows)
}

// HandleDeleteScrap 删除余料
func (h *CutHandler) HandleDeleteScrap(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "余料 ID 不合法")
		return
	}
	if err := h.svc.DeleteScrap(userID, uint(id)); err != nil {
		SendError(c, "500", "删除余料失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleBatchDeleteScraps 批量删除库存余料
func (h *CutHandler) HandleBatchDeleteScraps(c *gin.Context) {
	userID := GetUserID(c)
	var req model.BatchDeleteScrapsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	deleted, err := h.svc.BatchDeleteScraps(userID, req.IDs)
	if err != nil {
		SendError(c, "500", "批量删除余料失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"deleted": deleted})
}

// HandleListWindows 窗户单列表 (本人全部)
func (h *CutHandler) HandleListWindows(c *gin.Context) {
	userID := GetUserID(c)
	list, err := h.svc.ListWindows(userID)
	if err != nil {
		SendError(c, "500", "获取窗户单失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
}

// HandleSaveWindow 新增/更新窗户单 (ID=0 新增)
func (h *CutHandler) HandleSaveWindow(c *gin.Context) {
	userID := GetUserID(c)
	var req model.SaveCutWindowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	row, err := h.svc.SaveWindow(userID, req)
	if err != nil {
		SendError(c, "500", "保存窗户单失败: "+err.Error())
		return
	}
	SendSuccess(c, row)
}

// HandleDeleteWindow 删除窗户单
func (h *CutHandler) HandleDeleteWindow(c *gin.Context) {
	userID := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "窗户单 ID 不合法")
		return
	}
	if err := h.svc.DeleteWindow(userID, uint(id)); err != nil {
		SendError(c, "500", "删除窗户单失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleAddRecord 添加切割记录
func (h *CutHandler) HandleAddRecord(c *gin.Context) {
	userID := GetUserID(c)
	var req model.RecordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	record, err := h.svc.SaveCutRecord(userID, req)
	if err != nil {
		SendError(c, "500", "保存失败: "+err.Error())
		return
	}

	SendSuccess(c, record)
}

// HandleListRecords 查询切割记录列表
func (h *CutHandler) HandleListRecords(c *gin.Context) {
	userID := GetUserID(c)
	var params model.CutRecordSearchParams
	if err := c.ShouldBindQuery(&params); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	result, err := h.svc.ListCutRecords(userID, params)
	if err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}

	SendSuccess(c, result)
}

// HandleDeleteRecord 删除切割记录
func (h *CutHandler) HandleDeleteRecord(c *gin.Context) {
	userID := GetUserID(c)
	id := c.Param("id")

	if id == "" {
		SendError(c, "400", "记录ID不能为空")
		return
	}

	if err := h.svc.DeleteCutRecord(userID, id); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}

	SendSuccess(c, true)
}
