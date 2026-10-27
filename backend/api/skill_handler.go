package api

import (
	"backend/model"
	"backend/service/ai/skill"
	"strconv"

	"github.com/gin-gonic/gin"
)

type SkillHandler struct {
	svc *skill.AISkillService
}

func NewSkillHandler(svc *skill.AISkillService) *SkillHandler {
	return &SkillHandler{svc: svc}
}

// HandleList 获取 Skill 列表
func (h *SkillHandler) HandleList(c *gin.Context) {
	items, err := h.svc.List()
	if err != nil {
		SendError(c, "500", "获取Skill列表失败")
		return
	}
	SendSuccess(c, items)
}

// DiscoverSkillRequest 从 GitHub 仓库扫描 Skill 的请求
type DiscoverSkillRequest struct {
	Repo string `json:"repo" binding:"required"`
	Ref  string `json:"ref"`
	Path string `json:"path"`
}

// HandleDiscoverGitHub 扫描 GitHub 仓库中的 Skill (不落库, 供前端预填新增表单)
func (h *SkillHandler) HandleDiscoverGitHub(c *gin.Context) {
	var req DiscoverSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	result, err := h.svc.DiscoverFromGitHub(c.Request.Context(), req.Repo, req.Ref, req.Path)
	if err != nil {
		SendError(c, "500", err.Error())
		return
	}
	SendSuccess(c, result)
}

// HandleGetCachedDiscovery 读取某仓库上次扫描成功的内存缓存 (不触发扫描)
func (h *SkillHandler) HandleGetCachedDiscovery(c *gin.Context) {
	repo := c.Query("repo")
	if repo == "" {
		SendSuccess(c, nil)
		return
	}
	SendSuccess(c, h.svc.GetCachedDiscovery(repo, c.Query("ref"), c.Query("path")))
}

// HandleCreate 创建 Skill
func (h *SkillHandler) HandleCreate(c *gin.Context) {
	var req model.CreateAISkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	item, err := h.svc.Create(&req)
	if err != nil {
		SendError(c, "500", "创建失败: "+err.Error())
		return
	}
	SendSuccess(c, item)
}

// HandleUpdate 更新 Skill
func (h *SkillHandler) HandleUpdate(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}
	var req model.UpdateAISkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.Update(uint(id), &req); err != nil {
		SendError(c, "500", "更新失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// HandleDelete 删除 Skill
func (h *SkillHandler) HandleDelete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		SendError(c, "400", "无效的ID")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		SendError(c, "500", "删除失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}
