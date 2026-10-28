package api

import (
	"strconv"

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
