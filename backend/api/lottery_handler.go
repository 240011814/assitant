package api

import (
	"strconv"

	"backend/model"
	"backend/service"

	"github.com/gin-gonic/gin"
)

type LotteryHandler struct {
	svc *service.LotteryService
}

func NewLotteryHandler(svc *service.LotteryService) *LotteryHandler {
	return &LotteryHandler{svc: svc}
}

// ==================== 活动管理 ====================

// HandleListActivities 获取抽奖活动列表
func (h *LotteryHandler) HandleListActivities(c *gin.Context) {
	keyword := c.Query("keyword")
	statusStr := c.Query("status")

	var status *int
	if statusStr != "" {
		s, err := strconv.Atoi(statusStr)
		if err == nil {
			status = &s
		}
	}

	activities, err := h.svc.ListActivities(keyword, status)
	if err != nil {
		SendError(c, "500", "获取活动列表失败: "+err.Error())
		return
	}

	SendSuccess(c, activities)
}

// HandleGetActivity 获取抽奖活动详情
func (h *LotteryHandler) HandleGetActivity(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	activity, err := h.svc.GetActivity(uint(id))
	if err != nil {
		SendError(c, "500", "获取活动失败: "+err.Error())
		return
	}

	SendSuccess(c, activity)
}

// HandleCreateActivity 创建抽奖活动
func (h *LotteryHandler) HandleCreateActivity(c *gin.Context) {
	userID := GetUserID(c)
	var req model.CreateLotteryActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	activity, err := h.svc.CreateActivity(userID, req)
	if err != nil {
		SendError(c, "500", "创建活动失败: "+err.Error())
		return
	}

	SendSuccess(c, activity)
}

// HandleUpdateActivity 更新抽奖活动
func (h *LotteryHandler) HandleUpdateActivity(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	var req model.UpdateLotteryActivityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.UpdateActivity(uint(id), req); err != nil {
		SendError(c, "500", "更新活动失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleDeleteActivity 删除抽奖活动
func (h *LotteryHandler) HandleDeleteActivity(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	if err := h.svc.DeleteActivity(uint(id)); err != nil {
		SendError(c, "500", "删除活动失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// ==================== 奖品管理 ====================

// HandleListPrizes 获取奖品列表
func (h *LotteryHandler) HandleListPrizes(c *gin.Context) {
	activityIDStr := c.Param("id")
	activityID, err := strconv.Atoi(activityIDStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	prizes, err := h.svc.ListPrizes(uint(activityID))
	if err != nil {
		SendError(c, "500", "获取奖品列表失败: "+err.Error())
		return
	}

	SendSuccess(c, prizes)
}

// HandleCreatePrize 创建奖品
func (h *LotteryHandler) HandleCreatePrize(c *gin.Context) {
	activityIDStr := c.Param("id")
	activityID, err := strconv.Atoi(activityIDStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	var req model.CreateLotteryPrizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	prize, err := h.svc.CreatePrize(uint(activityID), req)
	if err != nil {
		SendError(c, "500", "创建奖品失败: "+err.Error())
		return
	}

	SendSuccess(c, prize)
}

// HandleUpdatePrize 更新奖品
func (h *LotteryHandler) HandleUpdatePrize(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		SendError(c, "400", "无效的奖品ID")
		return
	}

	var req model.UpdateLotteryPrizeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if err := h.svc.UpdatePrize(uint(id), req); err != nil {
		SendError(c, "500", "更新奖品失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleDeletePrize 删除奖品
func (h *LotteryHandler) HandleDeletePrize(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		SendError(c, "400", "无效的奖品ID")
		return
	}

	if err := h.svc.DeletePrize(uint(id)); err != nil {
		SendError(c, "500", "删除奖品失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// ==================== 抽奖操作 ====================

// HandleDraw 执行抽奖
func (h *LotteryHandler) HandleDraw(c *gin.Context) {
	userID := GetUserID(c)
	activityIDStr := c.Param("activityId")
	activityID, err := strconv.Atoi(activityIDStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	var req model.DrawLotteryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请输入您的姓名")
		return
	}

	prize, err := h.svc.Draw(userID, uint(activityID), req.UserName)
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}

	if prize == nil {
		SendSuccess(c, gin.H{
			"isWinner": false,
			"message":  "很遗憾，未中奖",
		})
	} else {
		SendSuccess(c, gin.H{
			"isWinner": true,
			"prize":    prize,
			"message":  "恭喜中奖！",
		})
	}
}

// ==================== 记录查询 ====================

// HandleDeleteRecord 删除抽奖记录
func (h *LotteryHandler) HandleDeleteRecord(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		SendError(c, "400", "无效的记录ID")
		return
	}

	if err := h.svc.DeleteRecord(uint(id)); err != nil {
		SendError(c, "500", "删除记录失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleDeleteRecordsByActivityID 删除活动的所有抽奖记录
func (h *LotteryHandler) HandleDeleteRecordsByActivityID(c *gin.Context) {
	activityIDStr := c.Param("id")
	activityID, err := strconv.Atoi(activityIDStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	if err := h.svc.DeleteRecordsByActivityID(uint(activityID)); err != nil {
		SendError(c, "500", "删除记录失败: "+err.Error())
		return
	}

	SendSuccess(c, nil)
}

// HandleListRecords 获取抽奖记录列表
func (h *LotteryHandler) HandleListRecords(c *gin.Context) {
	activityIDStr := c.Query("activityId")
	userIDStr := c.Query("userId")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	page = service.NormalizePage(page)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	var activityID *uint
	if activityIDStr != "" {
		id, err := strconv.Atoi(activityIDStr)
		if err == nil {
			uid := uint(id)
			activityID = &uid
		}
	}

	var userID *uint
	if userIDStr != "" {
		id, err := strconv.Atoi(userIDStr)
		if err == nil {
			uid := uint(id)
			userID = &uid
		}
	}

	records, total, err := h.svc.ListRecords(activityID, userID, page, pageSize)
	if err != nil {
		SendError(c, "500", "获取记录失败: "+err.Error())
		return
	}

	SendSuccess(c, gin.H{
		"list":     records,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// HandleListWinners 获取中奖名单
func (h *LotteryHandler) HandleListWinners(c *gin.Context) {
	activityIDStr := c.Query("activityId")
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("pageSize", "10")

	page, _ := strconv.Atoi(pageStr)
	pageSize, _ := strconv.Atoi(pageSizeStr)

	page = service.NormalizePage(page)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	var activityID *uint
	if activityIDStr != "" {
		id, err := strconv.Atoi(activityIDStr)
		if err == nil {
			uid := uint(id)
			activityID = &uid
		}
	}

	winners, total, err := h.svc.ListWinners(activityID, page, pageSize)
	if err != nil {
		SendError(c, "500", "获取中奖名单失败: "+err.Error())
		return
	}

	SendSuccess(c, gin.H{
		"list":     winners,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// HandleGetDrawLimits 获取抽奖次数限制信息
func (h *LotteryHandler) HandleGetDrawLimits(c *gin.Context) {
	activityIDStr := c.Param("id")
	userName := c.Query("userName")

	activityID, err := strconv.Atoi(activityIDStr)
	if err != nil {
		SendError(c, "400", "无效的活动ID")
		return
	}

	// 获取活动信息
	activity, err := h.svc.GetActivity(uint(activityID))
	if err != nil {
		SendError(c, "500", "获取活动失败: "+err.Error())
		return
	}

	result := gin.H{
		"dailyLimit":      activity.DailyLimit,
		"totalLimit":      activity.TotalLimit,
		"userDailyUsed":   0,
		"userTotalUsed":   0,
		"totalDailyUsed":  0,
	}

	// 获取用户今日已用次数
	if userName != "" && activity.DailyLimit > 0 {
		userDailyUsed, err := h.svc.GetUserDailyDrawCountByName(uint(activityID), userName)
		if err == nil {
			result["userDailyUsed"] = userDailyUsed
		}
	}

	// 获取用户总已用次数
	if userName != "" && activity.TotalLimit > 0 {
		userTotalUsed, err := h.svc.GetUserTotalDrawCountByName(uint(activityID), userName)
		if err == nil {
			result["userTotalUsed"] = userTotalUsed
		}
	}

	// 获取今日所有用户总已用次数
	if activity.DailyLimit > 0 {
		totalDailyUsed, err := h.svc.GetTotalDailyDrawCount(uint(activityID))
		if err == nil {
			result["totalDailyUsed"] = totalDailyUsed
		}
	}

	SendSuccess(c, result)
}
