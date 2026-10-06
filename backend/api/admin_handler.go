package api

import (
	"log"
	"strconv"

	interfaces "backend/interface"
	"backend/model"
	"backend/service"
	"backend/service/tools"

	"github.com/gin-gonic/gin"
)

type AdminHandler struct {
	svc          *service.AdminService
	aiAgentSvc   *service.AIAgentService
	authSvc      *service.AuthService
	mem0Service  interfaces.Mem0Service
}

func NewAdminHandler(svc *service.AdminService, aiAgentSvc *service.AIAgentService, authSvc *service.AuthService, mem0Service interfaces.Mem0Service) *AdminHandler {
	return &AdminHandler{svc: svc, aiAgentSvc: aiAgentSvc, authSvc: authSvc, mem0Service: mem0Service}
}

func (h *AdminHandler) HandleListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	list, total, err := h.svc.ListUsers(c.Query("keyword"), c.Query("role"), page, pageSize)
	if err != nil {
		SendError(c, "500", "获取用户列表失败: "+err.Error())
		return
	}
	SendSuccess(c, gin.H{
		"list":      list,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

func (h *AdminHandler) HandleCreateUser(c *gin.Context) {
	var req model.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.CreateUser(req); err != nil {
		SendError(c, "500", "创建用户失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleUpdateUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "用户 ID 不合法")
		return
	}

	var req model.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateUser(uint(id), req); err != nil {
		SendError(c, "500", "更新用户失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleDeleteUser(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "用户 ID 不合法")
		return
	}
	if err := h.svc.DeleteUser(uint(id), GetUserID(c)); err != nil {
		SendError(c, "500", "删除用户失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleProxyLogin(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "用户 ID 不合法")
		return
	}
	res, err := h.authSvc.ProxyLogin(uint(id))
	if err != nil {
		SendError(c, "500", "代理登录失败: "+err.Error())
		return
	}
	SendSuccess(c, res)
}

func (h *AdminHandler) HandleListRoles(c *gin.Context) {
	roles, err := h.svc.ListRoles()
	if err != nil {
		SendError(c, "500", "获取角色列表失败: "+err.Error())
		return
	}
	SendSuccess(c, roles)
}

func (h *AdminHandler) HandleCreateRole(c *gin.Context) {
	var role model.Role
	if err := c.ShouldBindJSON(&role); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.CreateRole(role); err != nil {
		SendError(c, "500", "创建角色失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleDeleteRole(c *gin.Context) {
	roleCode := c.Param("roleCode")
	if err := h.svc.DeleteRole(roleCode); err != nil {
		SendError(c, "500", "删除角色失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleListPermissions(c *gin.Context) {
	permissions, err := h.svc.ListPermissions()
	if err != nil {
		SendError(c, "500", "获取权限列表失败: "+err.Error())
		return
	}
	SendSuccess(c, permissions)
}

func (h *AdminHandler) HandleCreatePermission(c *gin.Context) {
	var p model.Permission
	if err := c.ShouldBindJSON(&p); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.CreatePermission(p); err != nil {
		SendError(c, "500", "创建权限点失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleUpdatePermission(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "权限 ID 不合法")
		return
	}

	var p model.Permission
	if err := c.ShouldBindJSON(&p); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdatePermission(uint(id), p); err != nil {
		SendError(c, "500", "更新权限点失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleDeletePermission(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "权限 ID 不合法")
		return
	}
	if err := h.svc.DeletePermission(uint(id)); err != nil {
		SendError(c, "500", "删除权限点失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleGetRolePermissions(c *gin.Context) {
	permissions, err := h.svc.GetRolePermissions(c.Param("roleCode"))
	if err != nil {
		SendError(c, "500", "获取角色权限失败: "+err.Error())
		return
	}
	SendSuccess(c, permissions)
}

func (h *AdminHandler) HandleUpdateRolePermissions(c *gin.Context) {
	var req model.RolePermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateRolePermissions(c.Param("roleCode"), req.Permissions); err != nil {
		SendError(c, "500", "更新角色权限失败: "+err.Error())
		return
	}
	SendSuccess(c, nil)
}

// AI Config Handlers

func (h *AdminHandler) HandleListAIProviders(c *gin.Context) {
	list, err := h.svc.ListAIProviders()
	if err != nil {
		SendError(c, "500", "获取 AI 提供商列表失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
}

func (h *AdminHandler) HandleCreateAIProvider(c *gin.Context) {
	var req model.AIProvider
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.CreateAIProvider(req); err != nil {
		SendError(c, "500", "创建 AI 提供商失败: "+err.Error())
		return
	}
	if err := h.aiAgentSvc.ReloadConfig(); err != nil {
		log.Printf("[admin] 重载 AI 配置失败 (provider 创建后): %v", err)
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleUpdateAIProvider(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req model.AIProvider
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateAIProvider(id, req); err != nil {
		SendError(c, "500", "更新 AI 提供商失败: "+err.Error())
		return
	}
	if err := h.aiAgentSvc.ReloadConfig(); err != nil {
		log.Printf("[admin] 重载 AI 配置失败 (provider 更新后): %v", err)
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleDeleteAIProvider(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DeleteAIProvider(id); err != nil {
		SendError(c, "500", "删除 AI 提供商失败: "+err.Error())
		return
	}
	if err := h.aiAgentSvc.ReloadConfig(); err != nil {
		log.Printf("[admin] 重载 AI 配置失败 (provider 删除后): %v", err)
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleListAIModels(c *gin.Context) {
	list, err := h.svc.ListAIModels()
	if err != nil {
		SendError(c, "500", "获取 AI 模型列表失败: "+err.Error())
		return
	}
	SendSuccess(c, list)
}

func (h *AdminHandler) HandleCreateAIModel(c *gin.Context) {
	var req model.AIModel
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.CreateAIModel(req); err != nil {
		SendError(c, "500", "创建 AI 模型失败: "+err.Error())
		return
	}
	if err := h.aiAgentSvc.ReloadConfig(); err != nil {
		log.Printf("[admin] 重载 AI 配置失败 (model 创建后): %v", err)
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleUpdateAIModel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var req model.AIModel
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.svc.UpdateAIModel(id, req); err != nil {
		SendError(c, "500", "更新 AI 模型失败: "+err.Error())
		return
	}
	if err := h.aiAgentSvc.ReloadConfig(); err != nil {
		log.Printf("[admin] 重载 AI 配置失败 (model 更新后): %v", err)
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleDeleteAIModel(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.svc.DeleteAIModel(id); err != nil {
		SendError(c, "500", "删除 AI 模型失败: "+err.Error())
		return
	}
	if err := h.aiAgentSvc.ReloadConfig(); err != nil {
		log.Printf("[admin] 重载 AI 配置失败 (model 删除后): %v", err)
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleTestAIConnection(c *gin.Context) {
	var req struct {
		APIKey   string `json:"api_key"`
		BaseURL  string `json:"base_url"`
		ModelCode string `json:"model_code"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := h.aiAgentSvc.TestConnection(req.APIKey, req.BaseURL, req.ModelCode); err != nil {
		SendError(c, "500", "连接测试失败: "+err.Error())
		return
	}
	SendSuccess(c, "连接测试成功")
}

// AI Tool Handlers

func (h *AdminHandler) HandleListAITools(c *gin.Context) {
	var tools []model.AITool
	if err := service.DB.Find(&tools).Error; err != nil {
		SendError(c, "500", "获取工具列表失败: "+err.Error())
		return
	}
	SendSuccess(c, tools)
}

func (h *AdminHandler) HandleGetAITool(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "工具 ID 不合法")
		return
	}
	var tool model.AITool
	if err := service.DB.First(&tool, id).Error; err != nil {
		SendError(c, "404", "工具不存在")
		return
	}
	SendSuccess(c, tool)
}

func (h *AdminHandler) HandleCreateAITool(c *gin.Context) {
	var tool model.AITool
	if err := c.ShouldBindJSON(&tool); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	if err := service.DB.Create(&tool).Error; err != nil {
		SendError(c, "500", "创建工具失败: "+err.Error())
		return
	}
	service.InvalidateConfirmRequiredCache(tool.Name)
	h.aiAgentSvc.ClearRunnerCache()
	if tool.Name == "mem0_memory" {
		h.mem0Service.LoadFromTool()
	}
	SendSuccess(c, tool)
}

func (h *AdminHandler) HandleUpdateAITool(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "工具 ID 不合法")
		return
	}
	var req model.AITool
	if err := c.ShouldBindJSON(&req); err != nil {
		SendError(c, "400", "请求参数错误: "+err.Error())
		return
	}
	// 先读库取原有 name: 请求体可能不回传 name, 用 req.Name 判断缓存失效会落空
	var existing model.AITool
	if err := service.DB.First(&existing, id).Error; err != nil {
		SendError(c, "500", "工具不存在: "+err.Error())
		return
	}
	if err := service.DB.Model(&model.AITool{}).Where("id = ?", id).
		Select("display_name", "description", "enabled", "confirm_required", "config_json").
		Updates(req).Error; err != nil {
		SendError(c, "500", "更新工具失败: "+err.Error())
		return
	}
	service.InvalidateConfirmRequiredCache(existing.Name)
	h.aiAgentSvc.ClearRunnerCache()
	if existing.Name == "mem0_memory" {
		h.mem0Service.LoadFromTool()
	}
	SendSuccess(c, nil)
}

func (h *AdminHandler) HandleDeleteAITool(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		SendError(c, "400", "工具 ID 不合法")
		return
	}
	// Get tool name before delete for cache invalidation
	var tool model.AITool
	service.DB.First(&tool, id)
	if err := service.DB.Delete(&model.AITool{}, id).Error; err != nil {
		SendError(c, "500", "删除工具失败: "+err.Error())
		return
	}
	service.InvalidateConfirmRequiredCache(tool.Name)
	h.aiAgentSvc.ClearRunnerCache()
	if tool.Name == "mem0_memory" {
		h.mem0Service.LoadFromTool()
	}
	SendSuccess(c, nil)
}
func (h *AdminHandler) HandleListAIToolMetas(c *gin.Context) {
	metas := tools.GetAllToolMeta()
	SendSuccess(c, metas)
}
