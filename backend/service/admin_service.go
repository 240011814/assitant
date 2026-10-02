package service

import (
	"errors"

	"backend/model"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type AdminService struct{}

func NewAdminService() *AdminService {
	return &AdminService{}
}

func (s *AdminService) ListUsers(keyword, role string, page, pageSize int) ([]model.UserListItem, int64, error) {
	query := DB.Model(&model.User{})
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR nickname LIKE ?", like, like)
	}
	if role != "" {
		query = query.Where("role = ?", role)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var users []model.User
	if err := query.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	list := make([]model.UserListItem, 0, len(users))
	for _, user := range users {
		list = append(list, model.UserListItem{
			UserId:    user.ID,
			UserName:  user.Username,
			Nickname:  user.Nickname,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
		})
	}
	return list, total, nil
}

func (s *AdminService) CreateUser(req model.CreateUserRequest) error {
	if err := ValidatePasswordStrength(req.Password); err != nil {
		return err
	}
	if err := s.ensureRoleExists(req.Role); err != nil {
		return err
	}

	var count int64
	if err := DB.Model(&model.User{}).Where("username = ?", req.UserName).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("用户名已存在")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user := model.User{
		Username:     req.UserName,
		PasswordHash: string(hash),
		Nickname:     req.Nickname,
		Role:         req.Role,
	}
	return DB.Create(&user).Error
}

func (s *AdminService) UpdateUser(id uint, req model.UpdateUserRequest) error {
	if err := s.ensureRoleExists(req.Role); err != nil {
		return err
	}

	updates := map[string]interface{}{
		"nickname": req.Nickname,
		"role":     req.Role,
	}
	if req.Password != "" {
		if err := ValidatePasswordStrength(req.Password); err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		updates["password_hash"] = string(hash)
	}

	result := DB.Model(&model.User{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("用户不存在")
	}
	// 用户角色可能被修改, 失效该用户权限缓存
	InvalidatePermissionCache(id)
	return nil
}

func (s *AdminService) DeleteUser(id, currentUserID uint) error {
	if id == currentUserID {
		return errors.New("不能删除当前登录用户")
	}
	result := DB.Delete(&model.User{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("用户不存在")
	}
	return nil
}

func (s *AdminService) ListRoles() ([]model.Role, error) {
	var roles []model.Role
	err := DB.Order("id ASC").Find(&roles).Error
	return roles, err
}

func (s *AdminService) CreateRole(role model.Role) error {
	var count int64
	if err := DB.Model(&model.Role{}).Where("code = ?", role.Code).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("角色编码已存在")
	}
	return DB.Create(&role).Error
}

func (s *AdminService) DeleteRole(roleCode string) error {
	if roleCode == "R_SUPER" || roleCode == "R_ADMIN" || roleCode == "R_USER" {
		return errors.New("内置角色不可删除")
	}

	return DB.Transaction(func(tx *gorm.DB) error {
		// 删除角色权限关联
		if err := tx.Where("role_code = ?", roleCode).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		// 删除角色
		result := tx.Where("code = ?", roleCode).Delete(&model.Role{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("角色不存在")
		}
		return nil
	})
}

func (s *AdminService) ListPermissions() ([]model.Permission, error) {
	var permissions []model.Permission
	err := DB.Order("group_name ASC, id ASC").Find(&permissions).Error
	return permissions, err
}

func (s *AdminService) CreatePermission(p model.Permission) error {
	var count int64
	if err := DB.Model(&model.Permission{}).Where("code = ?", p.Code).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("权限编码已存在")
	}
	return DB.Create(&p).Error
}

func (s *AdminService) UpdatePermission(id uint, p model.Permission) error {
	updates := map[string]interface{}{
		"name":       p.Name,
		"group_name": p.GroupName,
	}
	// Note: Usually we don't allow updating the code as it's used as a key in many places
	result := DB.Model(&model.Permission{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("权限点不存在")
	}
	return nil
}

func (s *AdminService) DeletePermission(id uint) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var p model.Permission
		if err := tx.First(&p, id).Error; err != nil {
			return err
		}

		// 删除关联
		if err := tx.Where("permission_code = ?", p.Code).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		// 删除权限点
		return tx.Delete(&model.Permission{}, id).Error
	})
}

func (s *AdminService) GetRolePermissions(roleCode string) ([]string, error) {
	if err := s.ensureRoleExists(roleCode); err != nil {
		return nil, err
	}

	var rows []model.RolePermission
	if err := DB.Where("role_code = ?", roleCode).Find(&rows).Error; err != nil {
		return nil, err
	}

	permissions := make([]string, 0, len(rows))
	for _, row := range rows {
		permissions = append(permissions, row.PermissionCode)
	}
	return permissions, nil
}

func (s *AdminService) UpdateRolePermissions(roleCode string, permissions []string) error {
	if roleCode == "R_SUPER" {
		return errors.New("超级管理员默认拥有全部权限，无需配置")
	}
	if err := s.ensureRoleExists(roleCode); err != nil {
		return err
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("role_code = ?", roleCode).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}

		for _, permission := range permissions {
			row := model.RolePermission{RoleCode: roleCode, PermissionCode: permission}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		// 权限变更后失效短 TTL 缓存, 立即生效
		InvalidatePermissionCache(0)
	}
	return err
}

func (s *AdminService) ensureRoleExists(roleCode string) error {
	var role model.Role
	if err := DB.Where("code = ?", roleCode).First(&role).Error; err != nil {
		return errors.New("角色不存在")
	}
	return nil
}

// AI Config Management

func (s *AdminService) ListAIProviders() ([]model.AIProvider, error) {
	var providers []model.AIProvider
	err := DB.Preload("Models").Find(&providers).Error
	if err != nil {
		return nil, err
	}
	// API Key 明文不出后端: 列表只回掩码, 更新时留空表示不修改
	for i := range providers {
		providers[i].MaskedKey = maskAPIKey(providers[i].APIKey)
		providers[i].APIKey = ""
	}
	return providers, nil
}

// maskAPIKey 生成掩码 (保留前 4 后 4), 过短时整体打码
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 12 {
		return "****"
	}
	return key[:4] + "****" + key[len(key)-4:]
}

func (s *AdminService) CreateAIProvider(provider model.AIProvider) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if provider.IsActive {
			// 如果当前要启用，则将其他所有提供商设为禁用
			if err := tx.Model(&model.AIProvider{}).Where("id > 0").Update("is_active", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&provider).Error
	})
}

func (s *AdminService) UpdateAIProvider(id int, provider model.AIProvider) error {
	// api_key 为空表示不修改 (列表已不再返回明文, 前端编辑留空即可保留原值)
	cols := []string{"name", "base_url", "is_active"}
	if provider.APIKey != "" {
		cols = append(cols, "api_key")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if provider.IsActive {
			// 如果当前设为启用，则将其他所有提供商设为禁用
			if err := tx.Model(&model.AIProvider{}).Where("id != ?", id).Update("is_active", false).Error; err != nil {
				return err
			}
		}
		// 使用 Select 强制更新 is_active 字段，解决 GORM 忽略 false 的问题
		return tx.Model(&model.AIProvider{}).Where("id = ?", id).
			Select(cols).
			Updates(provider).Error
	})
}

func (s *AdminService) DeleteAIProvider(id int) error {
	return DB.Delete(&model.AIProvider{}, id).Error
}

func (s *AdminService) ListAIModels() ([]model.AIModel, error) {
	var models []model.AIModel
	err := DB.Find(&models).Error
	return models, err
}

func (s *AdminService) CreateAIModel(m model.AIModel) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if m.IsDefault {
			// 将同一 provider 下的其他模型设为非默认
			if err := tx.Model(&model.AIModel{}).Where("provider_id = ?", m.ProviderID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&m).Error
	})
}

func (s *AdminService) UpdateAIModel(id int, m model.AIModel) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if m.IsDefault {
			if err := tx.Model(&model.AIModel{}).Where("provider_id = ? AND id != ?", m.ProviderID, id).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		// 显式指定字段以包含 bool 类型的 false
		return tx.Model(&model.AIModel{}).Where("id = ?", id).
			Select("model_code", "display_name", "is_default", "config_json").
			Updates(m).Error
	})
}

func (s *AdminService) DeleteAIModel(id int) error {
	return DB.Delete(&model.AIModel{}, id).Error
}
