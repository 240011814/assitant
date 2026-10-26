package api

import (
	"backend/model"
	"backend/service"
	"backend/service/jobs"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type JobHandler struct {
	scheduler *jobs.JobScheduler
}

func NewJobHandler(scheduler *jobs.JobScheduler) *JobHandler {
	return &JobHandler{scheduler: scheduler}
}

// HandleListTaskRegistry 已注册的可调度任务列表 (不含用户备忘, 备忘走日历页创建)
func (h *JobHandler) HandleListTaskRegistry(c *gin.Context) {
	list := h.scheduler.ListRegisteredTasks()
	filtered := make([]model.TaskMeta, 0, len(list))
	for _, t := range list {
		if t.Name != model.TaskNameReminder {
			filtered = append(filtered, t)
		}
	}
	SendSuccess(c, filtered)
}

// HandleListJobs 定时任务定义列表 (仅系统任务, 用户备忘不展示)
func (h *JobHandler) HandleListJobs(c *gin.Context) {
	var defs []model.JobDefinition
	if err := service.DB.Where("user_id IS NULL").Order("id ASC").Find(&defs).Error; err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}

	type JobDefinitionView struct {
		model.JobDefinition
		NextRunAt *time.Time `json:"nextRunAt"`
	}
	list := make([]JobDefinitionView, 0, len(defs))
	for _, def := range defs {
		list = append(list, JobDefinitionView{JobDefinition: def, NextRunAt: h.scheduler.NextRunAt(def.ID)})
	}

	SendSuccess(c, list)
}

// HandleCreateJob 创建定时任务
func (h *JobHandler) HandleCreateJob(c *gin.Context) {
	var req model.CreateJobDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	if _, ok := h.scheduler.GetTask(req.TaskName); !ok {
		SendError(c, "400", "任务方法未注册: "+req.TaskName)
		return
	}
	if req.TaskName == model.TaskNameReminder {
		SendError(c, "400", "用户备忘请在日历页创建")
		return
	}
	if _, err := jobs.ValidateCronExpr(req.CronExpr); err != nil {
		SendError(c, "400", err.Error())
		return
	}
	if len(req.Params) > 0 && !json.Valid(req.Params) {
		SendError(c, "400", "参数必须是合法 JSON")
		return
	}

	userID := GetUserID(c)
	def := model.JobDefinition{
		Name:         req.Name,
		TaskName:     req.TaskName,
		ScheduleType: model.ScheduleTypeCron,
		CronExpr:     req.CronExpr,
		Params:       req.Params,
		Enabled:      req.Enabled != nil && *req.Enabled,
		MaxRetries:   req.MaxRetries,
		NotifyEmail:  strings.TrimSpace(req.NotifyEmail),
		Remark:       req.Remark,
		CreatedBy:    &userID,
	}
	if err := service.DB.Create(&def).Error; err != nil {
		SendError(c, "500", "创建失败: "+err.Error())
		return
	}
	if err := h.scheduler.ScheduleDefinition(&def); err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, def)
}

// HandleUpdateJob 更新定时任务
func (h *JobHandler) HandleUpdateJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}

	var def model.JobDefinition
	if err := service.DB.First(&def, uint(id)).Error; err != nil {
		SendError(c, "404", "任务不存在")
		return
	}

	var req model.UpdateJobDefinitionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}

	taskName := def.TaskName
	if def.TaskName == model.TaskNameReminder || (req.TaskName != nil && *req.TaskName == model.TaskNameReminder) {
		SendError(c, "400", "用户备忘请在日历页修改")
		return
	}
	if req.TaskName != nil {
		if _, ok := h.scheduler.GetTask(*req.TaskName); !ok {
			SendError(c, "400", "任务方法未注册: "+*req.TaskName)
			return
		}
		taskName = *req.TaskName
	}
	cronExpr := def.CronExpr
	if req.CronExpr != nil {
		if _, err := jobs.ValidateCronExpr(*req.CronExpr); err != nil {
			SendError(c, "400", err.Error())
			return
		}
		cronExpr = *req.CronExpr
	}
	if req.Params != nil && !json.Valid(req.Params) {
		SendError(c, "400", "参数必须是合法 JSON")
		return
	}

	updates := map[string]interface{}{
		"task_name": taskName,
		"cron_expr": cronExpr,
	}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Params != nil {
		updates["params"] = req.Params
	}
	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}
	if req.MaxRetries != nil {
		updates["max_retries"] = *req.MaxRetries
	}
	if req.NotifyEmail != nil {
		updates["notify_email"] = strings.TrimSpace(*req.NotifyEmail)
	}
	if req.Remark != nil {
		updates["remark"] = *req.Remark
	}
	if err := service.DB.Model(&def).Updates(updates).Error; err != nil {
		SendError(c, "500", "更新失败: "+err.Error())
		return
	}
	if err := service.DB.First(&def, uint(id)).Error; err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}
	// 重新注册调度(停用时移除)
	if err := h.scheduler.ScheduleDefinition(&def); err != nil {
		SendError(c, "500", err.Error())
		return
	}

	SendSuccess(c, def)
}

// HandleDeleteJob 删除定时任务
func (h *JobHandler) HandleDeleteJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}

	h.scheduler.UnscheduleDefinition(uint(id))
	if err := service.DB.Delete(&model.JobDefinition{}, uint(id)).Error; err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}
	service.DB.Where("definition_id = ?", uint(id)).Delete(&model.JobRun{})

	SendSuccess(c, true)
}

// HandleRunJob 手动立即执行
func (h *JobHandler) HandleRunJob(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}

	if !h.scheduler.TriggerDefinition(uint(id)) {
		SendError(c, "404", "任务不存在")
		return
	}
	SendSuccess(c, true)
}

// HandleListJobRuns 执行历史
func (h *JobHandler) HandleListJobRuns(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}

	var runs []model.JobRun
	if err := service.DB.Where("definition_id = ?", uint(id)).Order("id DESC").Limit(50).Find(&runs).Error; err != nil {
		SendError(c, "500", "查询失败: "+err.Error())
		return
	}

	SendSuccess(c, runs)
}
