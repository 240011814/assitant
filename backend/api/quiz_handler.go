package api

import (
	"strconv"

	"backend/model"
	"backend/service/learning"
	"github.com/gin-gonic/gin"
)

type QuizHandler struct {
	svc *learning.QuizService
}

func NewQuizHandler(svc *learning.QuizService) *QuizHandler {
	return &QuizHandler{svc: svc}
}

// HandleCreateQuizSession 创建练习会话
func (h *QuizHandler) HandleCreateQuizSession(c *gin.Context) {
	userId := GetUserID(c)
	var req model.CreateQuizSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	session, questions, err := h.svc.CreateSession(userId, req)
	if err != nil {
		SendError(c, "500", "创建练习失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"session": session, "questions": questions})
}

// HandleGetQuizSession 获取会话 (含答题进度)
func (h *QuizHandler) HandleGetQuizSession(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "会话 ID 不合法")
		return
	}
	session, questions, err := h.svc.GetSession(userId, uint(id))
	if err != nil {
		SendError(c, "500", "获取练习会话失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"session": session, "questions": questions})
}

// HandleGetActiveQuizSession 最近一个进行中的会话 (48h 内), 供前端"继续练习"; 无则返回 null
func (h *QuizHandler) HandleGetActiveQuizSession(c *gin.Context) {
	userId := GetUserID(c)
	session, questions, err := h.svc.GetLatestActiveSession(userId)
	if err != nil {
		SendSuccess(c, nil)
		return
	}
	SendSuccess(c, gin.H{"session": session, "questions": questions})
}

// HandleSubmitQuizAnswer 提交单题答案 (服务端判分)
func (h *QuizHandler) HandleSubmitQuizAnswer(c *gin.Context) {
	userId := GetUserID(c)
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "会话 ID 不合法")
		return
	}
	var req model.QuizAnswerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	result, session, questions, err := h.svc.SubmitAnswer(userId, uint(id), req)
	if err != nil {
		SendError(c, "500", "判分失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{"result": result, "session": session, "questions": questions})
}

// HandleSentencePractice 造句批改 (LLM)
func (h *QuizHandler) HandleSentencePractice(c *gin.Context) {
	userId := GetUserID(c)
	var req model.SentencePracticeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误")
		return
	}
	result, err := h.svc.SentencePractice(userId, req)
	if err != nil {
		SendError(c, "500", "批改失败: "+err.Error())
		return
	}
	SendSuccess(c, result)
}
