package admin

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"backend/model"
	"backend/service/sys/auth"
)

// setupAdminRoleTestDB 纯 Go SQLite 真库, 同时接到 admin 与 auth 包全局 DB
// (角色/权限缓存失效跨包联动), 用完还原并清理。
func setupAdminRoleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	origAdmin, origAuth := DB, auth.DB
	dbFile := t.Name() + ".db"
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserRole{}, &model.Role{}, &model.RolePermission{}, &model.Permission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	auth.DB = db
	auth.InvalidatePermissionCache(0)
	t.Cleanup(func() {
		DB = origAdmin
		auth.DB = origAuth
		auth.InvalidatePermissionCache(0)
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(dbFile)
	})
	return db
}

func seedRole(t *testing.T, code, name string) {
	t.Helper()
	if err := DB.Create(&model.Role{Code: code, Name: name}).Error; err != nil {
		t.Fatalf("seed role %s: %v", code, err)
	}
}

// TestSyncUserRolesRebuild 全删全插: 角色集合被整体替换
func TestSyncUserRolesRebuild(t *testing.T) {
	db := setupAdminRoleTestDB(t)
	user := model.User{Username: "u1", PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := syncUserRoles(DB, user.ID, []string{"R_A", "R_B"}); err != nil {
		t.Fatal(err)
	}
	if _, err := syncUserRoles(DB, user.ID, []string{"R_C"}); err != nil {
		t.Fatal(err)
	}
	var codes []string
	if err := DB.Model(&model.UserRole{}).Where("user_id = ?", user.ID).Order("role_code ASC").Pluck("role_code", &codes).Error; err != nil {
		t.Fatal(err)
	}
	if len(codes) != 1 || codes[0] != "R_C" {
		t.Fatalf("roles should be rebuilt to [R_C], got %v", codes)
	}
}

// TestListUsersRoleFilterExists 角色过滤走 EXISTS: 多角色用户按任一持有角色命中
func TestListUsersRoleFilterExists(t *testing.T) {
	db := setupAdminRoleTestDB(t)
	seedRole(t, "R_A", "A")
	seedRole(t, "R_B", "B")
	u1 := model.User{Username: "alice", PasswordHash: "x"}
	u2 := model.User{Username: "bob", PasswordHash: "x"}
	if err := db.Create(&u1).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&u2).Error; err != nil {
		t.Fatal(err)
	}
	// alice 双角色, bob 单角色
	if err := db.Create(&model.UserRole{UserID: u1.ID, RoleCode: "R_A"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserRole{UserID: u1.ID, RoleCode: "R_B"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserRole{UserID: u2.ID, RoleCode: "R_A"}).Error; err != nil {
		t.Fatal(err)
	}

	s := &AdminService{}
	list, total, err := s.ListUsers("", "R_B", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(list) != 1 || list[0].UserName != "alice" {
		t.Fatalf("R_B filter should match only alice, total=%d list=%v", total, list)
	}
	if len(list[0].Roles) != 2 {
		t.Fatalf("alice should carry 2 roles, got %v", list[0].Roles)
	}
	if list[0].Roles[0] != "R_A" {
		t.Fatalf("roles should be sorted with R_A first, got %v", list[0].Roles)
	}
}

// TestUpdateUserReplacesRolesAndInvalidatesCache 更新角色后缓存立即失效, 新角色集即时生效
func TestUpdateUserReplacesRolesAndInvalidatesCache(t *testing.T) {
	db := setupAdminRoleTestDB(t)
	seedRole(t, "R_A", "A")
	seedRole(t, "R_B", "B")
	user := model.User{Username: "u1", PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserRole{UserID: user.ID, RoleCode: "R_A"}).Error; err != nil {
		t.Fatal(err)
	}
	s := &AdminService{}

	// 预热缓存 (旧角色集)
	if _, err := auth.GetUserRoles(user.ID); err != nil {
		t.Fatal(err)
	}

	if err := s.UpdateUser(user.ID, model.UpdateUserRequest{
		Nickname: "u1",
		Roles:    []string{"R_B"},
	}); err != nil {
		t.Fatal(err)
	}

	roles, err := auth.GetUserRoles(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 1 || roles[0] != "R_B" {
		t.Fatalf("cache should be invalidated and reflect [R_B], got %v", roles)
	}
}

// TestDeleteRoleCleansUserRoles 删角色同步清理用户-角色关联
func TestDeleteRoleCleansUserRoles(t *testing.T) {
	db := setupAdminRoleTestDB(t)
	seedRole(t, "R_X", "X")
	user := model.User{Username: "u1", PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserRole{UserID: user.ID, RoleCode: "R_X"}).Error; err != nil {
		t.Fatal(err)
	}
	s := &AdminService{}
	if err := s.DeleteRole("R_X"); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := DB.Model(&model.UserRole{}).Where("role_code = ?", "R_X").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("user_roles rows for deleted role should be cleaned, got %d", count)
	}
	// 内置角色保护
	if err := s.DeleteRole("R_SUPER"); err == nil {
		t.Fatal("built-in role should be protected")
	}
}

// TestEnsureRolesExistMissing 批量校验: 任一角色缺失即报错
func TestEnsureRolesExistMissing(t *testing.T) {
	setupAdminRoleTestDB(t)
	seedRole(t, "R_A", "A")
	s := &AdminService{}
	if err := s.ensureRolesExist([]string{"R_A", "R_GHOST"}); err == nil {
		t.Fatal("missing role should fail")
	}
	if err := s.ensureRolesExist([]string{"R_A"}); err != nil {
		t.Fatalf("existing role should pass: %v", err)
	}
}
