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

// HandleConsumeScraps 批量扣减库存余料 (自动导入计算确认后调用)
func (h *CutHandler) HandleConsumeScraps(c *gin.Context) {
	userID := GetUserID(c)
	var req model.ConsumeScrapsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	list, err := h.svc.ConsumeScraps(userID, req.IDs)
	if err != nil {
		SendError(c, "500", "扣减余料失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
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

// HandleListScraps 余料库存列表
func (h *CutHandler) HandleListScraps(c *gin.Context) {
	userID := GetUserID(c)
	scrapType, _ := strconv.Atoi(c.DefaultQuery("scrapType", "0"))
	list, err := h.svc.ListScraps(userID, scrapType)
	if err != nil {
		SendError(c, "500", "获取余料库存失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
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
