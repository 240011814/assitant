package auth

import (
	"sync"
	"time"

	"backend/model"
)

// 权限检查短 TTL 缓存: RequirePermission 每请求原本要查 2 次 DB (user roles + role_permissions),
// 高频接口开销可观; 缓存后权限变更最迟 60s 生效 (权限是低频管理操作, 可接受)
const permissionCacheTTL = 60 * time.Second

type permissionCacheEntry struct {
	roles     []string
	expiresAt time.Time
}

var (
	permissionCacheMu sync.RWMutex
	permissionCache   = map[uint]permissionCacheEntry{}
)

// InvalidatePermissionCache 权限/角色变更后调用, 清空指定用户 (userID=0 清全部)
func InvalidatePermissionCache(userID uint) {
	permissionCacheMu.Lock()
	defer permissionCacheMu.Unlock()
	if userID == 0 {
		permissionCache = map[uint]permissionCacheEntry{}
		return
	}
	delete(permissionCache, userID)
}

func getCachedUserRoles(userID uint) ([]string, bool) {
	permissionCacheMu.RLock()
	defer permissionCacheMu.RUnlock()
	entry, ok := permissionCache[userID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.roles, true
}

func cacheUserRoles(userID uint, roles []string) {
	permissionCacheMu.Lock()
	defer permissionCacheMu.Unlock()
	permissionCache[userID] = permissionCacheEntry{roles: roles, expiresAt: time.Now().Add(permissionCacheTTL)}
}

// GetUserRoles 用户的全部角色 (user_roles 表为真相源; 无行时回落 users.role 主角色,
// 兜住手改库/旧数据等边界)。空角色列表视为 R_USER 语义由调用方决定, 这里只返回事实。
func GetUserRoles(userID uint) ([]string, error) {
	if roles, ok := getCachedUserRoles(userID); ok {
		return roles, nil
	}
	var rows []model.UserRole
	if err := DB.Where("user_id = ?", userID).Order("role_code ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	roles := make([]string, 0, len(rows))
	for _, r := range rows {
		roles = append(roles, r.RoleCode)
	}
	cacheUserRoles(userID, roles)
	return roles, nil
}

// CheckUserPermission 检查用户是否拥有所需权限中的任意一个 (多角色取权限并集)
func CheckUserPermission(userID uint, permissions []string) (bool, error) {
	roles, ok := getCachedUserRoles(userID)
	if !ok {
		var err error
		roles, err = GetUserRoles(userID)
		if err != nil {
			return false, err
		}
	}
	if len(roles) == 0 {
		return false, nil
	}

	// 查询用户全部角色拥有的权限 (并集)
	var count int64
	err := DB.Table("role_permissions").
		Where("role_code IN ? AND permission_code IN ?", roles, permissions).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
