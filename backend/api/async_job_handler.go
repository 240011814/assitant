package api

import (
	"strconv"

	"backend/model"
	"backend/service"
	"backend/service/kernel"
	"backend/service/sys/asynctask"
	"github.com/gin-gonic/gin"
)

// AsyncJobHandler 通用异步任务轮询 (写课文/词汇测试/RSS 生词提取/RSS 对照翻译等)
type AsyncJobHandler struct{}

func NewAsyncJobHandler() *AsyncJobHandler {
	return &AsyncJobHandler{}
}

// HandleGet 查询本人异步任务状态 (仅需登录, service 内做 owner 校验;
// 含按 kind 的结果脱敏, 如词汇量测试剥离正确答案)
func (h *AsyncJobHandler) HandleGet(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "任务 ID 不合法")
		return
	}
	job, err := asynctask.Get(userId, uint(id))
	if err != nil {
		SendError(c, "500", "查询任务失败: "+err.Error())
		return
	}
	SendSuccess(c, job)
}

// AdminAsyncJobRow 审计页异步任务行: 任务 + 用户名 + 输入快照 (仅管理员可见)
type AdminAsyncJobRow struct {
	model.AsyncJob
	UserName   string `json:"userName"`
	ParamsJSON string `json:"paramsJson,omitempty"` // 内嵌 AsyncJob 的同名字段 json:"-", 此处显式下发
}

// HandleAdminList 全量异步任务列表 (审计日志页「异步任务」Tab, 与操作日志共用 system:audit:view)
// 筛选: userId, kind, status
func (h *AsyncJobHandler) HandleAdminList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	page = kernel.NormalizePage(page)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	query := service.DB.Model(&model.AsyncJob{})
	if v := c.Query("userId"); v != "" {
		if uid, err := strconv.ParseUint(v, 10, 64); err == nil {
			query = query.Where("user_id = ?", uid)
		}
	}
	if v := c.Query("kind"); v != "" {
		query = query.Where("kind = ?", v)
	}
	if v := c.Query("status"); v != "" {
		query = query.Where("status = ?", v)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		SendError(c, "500", "查询异步任务失败: "+err.Error())
		return
	}

	var jobs []model.AsyncJob
	if err := query.Order("id DESC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&jobs).Error; err != nil {
		SendError(c, "500", "查询异步任务失败: "+err.Error())
		return
	}

	// 批量补用户名 (job 行不冗余存, 与操作日志的落库冗余不同源)
	userIDs := make([]uint, 0, len(jobs))
	seen := make(map[uint]bool, len(jobs))
	for _, j := range jobs {
		if j.UserID > 0 && !seen[j.UserID] {
			seen[j.UserID] = true
			userIDs = append(userIDs, j.UserID)
		}
	}
	userNames := make(map[uint]string, len(userIDs))
	if len(userIDs) > 0 {
		var users []model.User
		if err := service.DB.Model(&model.User{}).Where("id IN ?", userIDs).Select("id, username").Find(&users).Error; err == nil {
			for _, u := range users {
				userNames[u.ID] = u.Username
			}
		}
	}

	rows := make([]AdminAsyncJobRow, 0, len(jobs))
	for _, j := range jobs {
		rows = append(rows, AdminAsyncJobRow{AsyncJob: j, UserName: userNames[j.UserID], ParamsJSON: j.ParamsJSON})
	}

	SendSuccess(c, gin.H{
		"list":      rows,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}
