package api

import (
	"backend/model"
	"backend/service/reminder"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type ReminderHandler struct {
	reminderSvc *reminder.ReminderService
}

func NewReminderHandler(reminderSvc *reminder.ReminderService) *ReminderHandler {
	return &ReminderHandler{reminderSvc: reminderSvc}
}

func (h *ReminderHandler) List(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	year, _ := strconv.Atoi(c.DefaultQuery("year", ""))
	month, _ := strconv.Atoi(c.DefaultQuery("month", ""))

	now := time.Now()
	if year == 0 {
		year = now.Year()
	}
	if month == 0 {
		month = int(now.Month())
	}

	jobs, err := h.reminderSvc.ListByMonth(userID, year, time.Month(month))
	if err != nil {
		SendError(c, "500", "查询备忘失败: "+err.Error())
		return
	}

	SendSuccess(c, jobs)
}

func (h *ReminderHandler) Create(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	var req model.CreateReminderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}

	job, err := h.reminderSvc.Create(userID, req)
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, job)
}

func (h *ReminderHandler) Update(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的备忘ID")
		return
	}

	var req model.UpdateReminderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}

	job, err := h.reminderSvc.Update(userID, uint(id), req)
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, job)
}

func (h *ReminderHandler) Delete(c *gin.Context) {
	userID := GetUserID(c)
	if userID == 0 {
		SendError(c, "401", "未登录")
		return
	}

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		SendError(c, "400", "无效的备忘ID")
		return
	}

	// scope: this 仅删除当前条目（默认），all 删除整条重复链
	scope := c.DefaultQuery("scope", "this")
	if scope != "this" && scope != "all" {
		scope = "this"
	}

	if err := h.reminderSvc.Delete(userID, id, scope); err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, nil)
}
