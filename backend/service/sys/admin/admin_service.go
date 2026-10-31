package admin

import (
	"backend/service/sys/auth"
	crand "crypto/rand"
	"errors"
	"math/big"
	"strings"

	"backend/model"
	"backend/service/kernel"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// DB 本包专用数据库句柄, 由 main.go 接线赋值 (admin.DB = db)。
var DB *gorm.DB

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
		// 多角色: 按任一持有角色过滤
		query = query.Where("EXISTS (SELECT 1 FROM user_roles ur WHERE ur.user_id = users.id AND ur.role_code = ?)", role)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	page = kernel.NormalizePage(page)
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	var users []model.User
	if err := query.Order("id ASC").Offset((page - 1) * pageSize).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	// 批量加载角色 (按页一次 IN 查询)
	ids := make([]uint, 0, len(users))
	for _, user := range users {
		ids = append(ids, user.ID)
	}
	rolesByUser := make(map[uint][]string, len(users))
	if len(ids) > 0 {
		var rows []model.UserRole
		if err := DB.Where("user_id IN ?", ids).Order("role_code ASC").Find(&rows).Error; err == nil {
			for _, r := range rows {
				rolesByUser[r.UserID] = append(rolesByUser[r.UserID], r.RoleCode)
			}
		}
	}

	list := make([]model.UserListItem, 0, len(users))
	for _, user := range users {
		roles := rolesByUser[user.ID]
		if len(roles) == 0 {
			roles = []string{}
		}
		list = append(list, model.UserListItem{
			UserId:          user.ID,
			UserName:        user.Username,
			Nickname:        user.Nickname,
			Roles:           roles,
			TokenQuotaMonth: user.TokenQuotaMonth,
			TwoFAEnabled:    user.TotpSecret != nil && *user.TotpSecret != "",
			CreatedAt:       user.CreatedAt,
			UpdatedAt:       user.UpdatedAt,
		})
	}
	return list, total, nil
}

// syncUserRoles 重建用户的角色集合 (事务内调用): 全删全插。
// users.role 列已删除, 角色归属只存在于 user_roles。返回排序后的角色列表。
func syncUserRoles(tx *gorm.DB, userID uint, roles []string) ([]string, error) {
	if err := tx.Where("user_id = ?", userID).Delete(&model.UserRole{}).Error; err != nil {
		return nil, err
	}
	for _, role := range roles {
		if err := tx.Create(&model.UserRole{UserID: userID, RoleCode: role}).Error; err != nil {
			return nil, err
		}
	}
	return roles, nil
}

func (s *AdminService) CreateUser(req model.CreateUserRequest) error {
	if err := auth.ValidatePasswordStrength(req.Password); err != nil {
		return err
	}
	roles := req.Roles
	if err := s.ensureRolesExist(roles); err != nil {
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
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		_, err := syncUserRoles(tx, user.ID, roles)
		return err
	})
}

func (s *AdminService) UpdateUser(id uint, req model.UpdateUserRequest) error {
	// 角色未提供时保持原角色不动; 提供则全量替换
	roles := req.Roles
	if len(roles) > 0 {
		if err := s.ensureRolesExist(roles); err != nil {
			return err
		}
	}

	updates := map[string]interface{}{
		"nickname": req.Nickname,
	}
	// 月度 Token 限额: nil=不修改, 0=不限, 正数=每月上限
	if req.TokenQuotaMonth != nil {
		if *req.TokenQuotaMonth < 0 {
			return errors.New("月度 Token 限额不能为负数")
		}
		updates["token_quota_month"] = *req.TokenQuotaMonth
	}
	if req.Password != "" {
		if err := auth.ValidatePasswordStrength(req.Password); err != nil {
			return err
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		updates["password_hash"] = string(hash)
	}

	err := DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.User{}).Where("id = ?", id).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("用户不存在")
		}
		if len(roles) > 0 {
			if _, err := syncUserRoles(tx, id, roles); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 用户角色可能被修改, 失效该用户权限缓存
	auth.InvalidatePermissionCache(id)
	return nil
}

// ResetUserPassword 管理员重置用户密码; password 为空时生成随机密码并返回明文(仅此一次), 指定密码时返回空串;
// clear2FA 为 true 时同时清空该用户的两步验证密钥 (TOTP)
func (s *AdminService) ResetUserPassword(id uint, password string, clear2FA bool) (string, error) {
	generated := ""
	if password == "" {
		var err error
		generated, err = GenerateRandomPassword(12)
		if err != nil {
			return "", err
		}
		password = generated
	} else if err := auth.ValidatePasswordStrength(password); err != nil {
		return "", err
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	updates := map[string]interface{}{"password_hash": string(hash)}
	if clear2FA {
		updates["totp_secret"] = nil
	}
	result := DB.Model(&model.User{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return "", result.Error
	}
	if result.RowsAffected == 0 {
		return "", errors.New("用户不存在")
	}
	return generated, nil
}

// 随机密码字符集: 去掉易混淆的 0/O/o/1/l/I
const randomPasswordCharset = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

// GenerateRandomPassword 用 crypto/rand 生成长度为 n 的随机密码, 保证同时包含字母和数字
func GenerateRandomPassword(n int) (string, error) {
	if n < 8 {
		return "", errors.New("随机密码长度不足")
	}
	buf := make([]byte, n)
	for i := range buf {
		idx, err := randInt(len(randomPasswordCharset))
		if err != nil {
			return "", err
		}
		buf[i] = randomPasswordCharset[idx]
	}
	// 保证满足密码策略(字母+数字): 不满足时回填两类字符
	if !hasLetterAndDigit(buf) {
		buf[0] = 'a'
		idx, err := randInt(10)
		if err != nil {
			return "", err
		}
		buf[1] = byte('2' + idx%8)
	}
	// Fisher-Yates 洗牌, 打散回填的固定字符
	for i := len(buf) - 1; i > 0; i-- {
		j, err := randInt(i + 1)
		if err != nil {
			return "", err
		}
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf), nil
}

func hasLetterAndDigit(buf []byte) bool {
	var hasLetter, hasDigit bool
	for _, b := range buf {
		switch {
		case (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z'):
			hasLetter = true
		case b >= '0' && b <= '9':
			hasDigit = true
		}
	}
	return hasLetter && hasDigit
}

func randInt(max int) (int, error) {
	n, err := crand.Int(crand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(n.Int64()), nil
}

func (s *AdminService) DeleteUser(id, currentUserID uint) error {
	if id == currentUserID {
		return errors.New("不能删除当前登录用户")
	}
	return DB.Transaction(func(tx *gorm.DB) error {
		// 先删角色关联再删用户 (外键语义靠应用层维护)
		if err := tx.Where("user_id = ?", id).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		result := tx.Delete(&model.User{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return errors.New("用户不存在")
		}
		return nil
	})
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

	err := DB.Transaction(func(tx *gorm.DB) error {
		// 删除角色权限关联
		if err := tx.Where("role_code = ?", roleCode).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		// 删除用户-角色关联 (持有该角色的用户失去它)
		if err := tx.Where("role_code = ?", roleCode).Delete(&model.UserRole{}).Error; err != nil {
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
	if err == nil {
		// 持有该角色的用户权限集合已变化, 清全部权限缓存
		auth.InvalidatePermissionCache(0)
	}
	return err
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
		auth.InvalidatePermissionCache(0)
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

// ensureRolesExist 校验全部角色存在 (多角色创建/更新前的防手滑检查)
func (s *AdminService) ensureRolesExist(roleCodes []string) error {
	var count int64
	if err := DB.Model(&model.Role{}).Where("code IN ?", roleCodes).Count(&count).Error; err != nil {
		return err
	}
	if int(count) != len(roleCodes) {
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

// GetAIProvider 读取 Provider 完整配置 (含明文 API Key), 仅供服务端内部使用
func (s *AdminService) GetAIProvider(id int) (*model.AIProvider, error) {
	var provider model.AIProvider
	if err := DB.First(&provider, id).Error; err != nil {
		return nil, err
	}
	return &provider, nil
}

// GetAIModel 读取模型配置
func (s *AdminService) GetAIModel(id int) (*model.AIModel, error) {
	var m model.AIModel
	if err := DB.First(&m, id).Error; err != nil {
		return nil, err
	}
	return &m, nil
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

func normalizeAIModelType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case model.AIModelTypeTTS:
		return model.AIModelTypeTTS
	case model.AIModelTypeASR:
		return model.AIModelTypeASR
	case model.AIModelTypeEmbedding:
		return model.AIModelTypeEmbedding
	default:
		return model.AIModelTypeChat
	}
}

func (s *AdminService) CreateAIModel(m model.AIModel) error {
	m.ModelType = normalizeAIModelType(m.ModelType)
	return DB.Transaction(func(tx *gorm.DB) error {
		if m.IsDefault {
			// 将同一 provider 下的其他模型设为非默认
			if err := tx.Model(&model.AIModel{}).Where("provider_id = ?", m.ProviderID).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if m.IsDefaultTTS {
			// 同一 provider 只保留一个默认 TTS 模型
			if err := tx.Model(&model.AIModel{}).Where("provider_id = ?", m.ProviderID).Update("is_default_tts", false).Error; err != nil {
				return err
			}
		}
		return tx.Create(&m).Error
	})
}

func (s *AdminService) UpdateAIModel(id int, m model.AIModel) error {
	m.ModelType = normalizeAIModelType(m.ModelType)
	return DB.Transaction(func(tx *gorm.DB) error {
		if m.IsDefault {
			if err := tx.Model(&model.AIModel{}).Where("provider_id = ? AND id != ?", m.ProviderID, id).Update("is_default", false).Error; err != nil {
				return err
			}
		}
		if m.IsDefaultTTS {
			if err := tx.Model(&model.AIModel{}).Where("provider_id = ? AND id != ?", m.ProviderID, id).Update("is_default_tts", false).Error; err != nil {
				return err
			}
		}
		// 显式指定字段以包含 bool 类型的 false
		return tx.Model(&model.AIModel{}).Where("id = ?", id).
			Select("model_code", "display_name", "model_type", "is_default", "is_default_tts", "config_json").
			Updates(m).Error
	})
}

func (s *AdminService) DeleteAIModel(id int) error {
	return DB.Delete(&model.AIModel{}, id).Error
}
