package auth

import (
	"os"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"backend/config"
	"backend/model"
)

// setupMultiRoleTestDB 纯 Go SQLite 真库, 接到 auth 包全局 DB, 用完还原并清理。
func setupMultiRoleTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	origDB := DB
	dbFile := t.Name() + ".db"
	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.User{}, &model.UserRole{}, &model.Role{}, &model.RolePermission{}, &model.Permission{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	DB = db
	InvalidatePermissionCache(0)
	t.Cleanup(func() {
		DB = origDB
		InvalidatePermissionCache(0)
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(dbFile)
	})
	return db
}

func seedRoleWithPermission(t *testing.T, roleCode, permCode string) {
	t.Helper()
	if err := DB.Create(&model.RolePermission{RoleCode: roleCode, PermissionCode: permCode}).Error; err != nil {
		t.Fatalf("seed role permission: %v", err)
	}
}

// TestGetUserRolesMultiple 用户持有多个角色时全部返回 (按 role_code 排序)
func TestGetUserRolesMultiple(t *testing.T) {
	db := setupMultiRoleTestDB(t)
	user := model.User{Username: "u1", PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"R_USER", "R_ADMIN"} {
		if err := db.Create(&model.UserRole{UserID: user.ID, RoleCode: code}).Error; err != nil {
			t.Fatal(err)
		}
	}

	roles, err := GetUserRoles(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles) != 2 || roles[0] != "R_ADMIN" || roles[1] != "R_USER" {
		t.Fatalf("want [R_ADMIN R_USER] (sorted), got %v", roles)
	}

	// 无角色用户: 空列表 (角色归属只在 user_roles)
	user2 := model.User{Username: "u2", PasswordHash: "x"}
	if err := db.Create(&user2).Error; err != nil {
		t.Fatal(err)
	}
	roles2, err := GetUserRoles(user2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(roles2) != 0 {
		t.Fatalf("user without roles should return empty, got %v", roles2)
	}
}

// TestCheckUserPermissionUnion 权限并集: 权限分散在用户的多个角色里, 任一命中即通过
func TestCheckUserPermissionUnion(t *testing.T) {
	db := setupMultiRoleTestDB(t)
	user := model.User{Username: "u1", PasswordHash: "x"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserRole{UserID: user.ID, RoleCode: "R_A"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserRole{UserID: user.ID, RoleCode: "R_B"}).Error; err != nil {
		t.Fatal(err)
	}
	seedRoleWithPermission(t, "R_A", "doc.read")
	seedRoleWithPermission(t, "R_B", "doc.delete")

	if ok, err := CheckUserPermission(user.ID, []string{"doc.read"}); err != nil || !ok {
		t.Fatalf("doc.read should pass via R_A, ok=%v err=%v", ok, err)
	}
	if ok, err := CheckUserPermission(user.ID, []string{"doc.delete"}); err != nil || !ok {
		t.Fatalf("doc.delete should pass via R_B, ok=%v err=%v", ok, err)
	}
	if ok, _ := CheckUserPermission(user.ID, []string{"doc.none"}); ok {
		t.Fatal("doc.none should not pass")
	}
	// 缓存路径: 第二次查询走缓存, 结果一致
	if ok, _ := CheckUserPermission(user.ID, []string{"doc.read"}); !ok {
		t.Fatal("cached check should still pass")
	}
	InvalidatePermissionCache(user.ID)
	if ok, err := CheckUserPermission(user.ID, []string{"doc.read"}); err != nil || !ok {
		t.Fatalf("after invalidation should re-read and pass, ok=%v err=%v", ok, err)
	}
}

// TestGetPermissionsByRolesSuper 任一角色为 R_SUPER 即拥有全部权限
func TestGetPermissionsByRolesSuper(t *testing.T) {
	setupMultiRoleTestDB(t)
	if err := DB.Create(&model.Permission{Code: "p1", Name: "P1", GroupName: "g"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := DB.Create(&model.Permission{Code: "p2", Name: "P2", GroupName: "g"}).Error; err != nil {
		t.Fatal(err)
	}
	s := NewAuthService(&config.Config{Auth: config.AuthConfig{JWTSecret: "test-secret"}})

	got, err := s.getPermissionsByRoles([]string{"R_A", "R_SUPER"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("R_SUPER should grant all permissions, got %v", got)
	}

	seedRoleWithPermission(t, "R_A", "p1")
	got, err = s.getPermissionsByRoles([]string{"R_A"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != "p1" {
		t.Fatalf("single role should return its permission only, got %v", got)
	}
}

// TestRegisterCreatesDefaultRole 注册事务: 用户行 + 默认 R_USER 角色行同时落库
func TestRegisterCreatesDefaultRole(t *testing.T) {
	setupMultiRoleTestDB(t)
	s := NewAuthService(&config.Config{Auth: config.AuthConfig{JWTSecret: "test-secret"}})

	resp, err := s.Register("newuser", "Passw0rd1")
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if resp.Token == "" {
		t.Fatal("register should return token")
	}
	var count int64
	if err := DB.Model(&model.UserRole{}).Where("role_code = ?", "R_USER").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("register should create default R_USER role row, got %d", count)
	}
}
