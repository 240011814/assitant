package service

import (
	"sync"
	"time"

	"backend/model"
)

// 权限检查短 TTL 缓存: RequirePermission 每请求原本要查 2 次 DB (user + role_permissions),
// 高频接口开销可观; 缓存后权限变更最迟 60s 生效 (权限是低频管理操作, 可接受)
const permissionCacheTTL = 60 * time.Second

type permissionCacheEntry struct {
	role      string
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

func getCachedUserRole(userID uint) (string, bool) {
	permissionCacheMu.RLock()
	defer permissionCacheMu.RUnlock()
	entry, ok := permissionCache[userID]
	if !ok || time.Now().After(entry.expiresAt) {
		return "", false
	}
	return entry.role, true
}

func cacheUserRole(userID uint, role string) {
	permissionCacheMu.Lock()
	defer permissionCacheMu.Unlock()
	permissionCache[userID] = permissionCacheEntry{role: role, expiresAt: time.Now().Add(permissionCacheTTL)}
}

// CheckUserPermission 检查用户是否拥有所需权限中的任意一个
func CheckUserPermission(userID uint, permissions []string) (bool, error) {
	role, ok := getCachedUserRole(userID)
	if !ok {
		var user model.User
		if err := DB.First(&user, userID).Error; err != nil {
			return false, err
		}
		role = user.Role
		cacheUserRole(userID, role)
	}

	// 查询用户角色拥有的权限
	var count int64
	err := DB.Table("role_permissions").
		Where("role_code = ? AND permission_code IN ?", role, permissions).
		Count(&count).Error
	if err != nil {
		return false, err
	}

	return count > 0, nil
}
