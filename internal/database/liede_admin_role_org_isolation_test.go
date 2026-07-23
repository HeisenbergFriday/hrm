package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openLiedeRoleSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:liede-role-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&User{},
		&Role{},
		&UserRole{},
		&Permission{},
		&RolePermission{},
		&MenuPermission{},
		&DataPermission{},
		&OrganizationUser{},
	); err != nil {
		// OrganizationUser may not exist in all branches; retry without it.
		if err2 := db.AutoMigrate(
			&User{},
			&Role{},
			&UserRole{},
			&Permission{},
			&RolePermission{},
			&MenuPermission{},
			&DataPermission{},
		); err2 != nil {
			t.Fatalf("automigrate: %v / %v", err, err2)
		}
	}
	return db
}

func withTempDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	prev := DB
	DB = db
	t.Cleanup(func() { DB = prev })
}

func seedBasicPermissions(t *testing.T, db *gorm.DB) {
	t.Helper()
	codes := []Permission{
		{Name: "用户管理", Code: "user_manage", Description: "用户管理权限"},
		{Name: "权限管理", Code: "permission_manage", Description: "权限管理权限"},
		{Name: "组织数据只读", Code: "org:read", Description: "查看组织架构"},
		{Name: "绩效结果查看", Code: "performance:result:view", Description: "查看绩效结果"},
	}
	for _, p := range codes {
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("seed permission %s: %v", p.Code, err)
		}
	}
}

func TestEnsureRolePresetInOrg_DoesNotReuseOtherOrgRole(t *testing.T) {
	db := openLiedeRoleSQLite(t)
	withTempDB(t, db)

	defaultAdmin := Role{OrgID: DefaultOrganizationID, Name: "管理员", Description: "系统管理员"}
	if err := db.Create(&defaultAdmin).Error; err != nil {
		t.Fatalf("seed default admin: %v", err)
	}

	mutengAdmin, err := ensureRolePresetInOrg("muteng", "管理员", "系统管理员")
	if err != nil {
		t.Fatalf("ensureRolePresetInOrg muteng: %v", err)
	}
	if mutengAdmin.ID == 0 {
		t.Fatal("expected muteng admin role id")
	}
	if mutengAdmin.ID == defaultAdmin.ID {
		t.Fatalf("muteng admin reused default role id=%d", defaultAdmin.ID)
	}
	if mutengAdmin.OrgID != "muteng" {
		t.Fatalf("muteng admin org=%q", mutengAdmin.OrgID)
	}

	// Second call is idempotent.
	again, err := ensureRolePresetInOrg("muteng", "管理员", "系统管理员")
	if err != nil {
		t.Fatalf("ensureRolePresetInOrg again: %v", err)
	}
	if again.ID != mutengAdmin.ID {
		t.Fatalf("idempotent id mismatch %d vs %d", again.ID, mutengAdmin.ID)
	}
}

func TestEnsureUserRoleInOrg_RejectsCrossOrgRole(t *testing.T) {
	db := openLiedeRoleSQLite(t)
	withTempDB(t, db)

	defaultAdmin := Role{OrgID: DefaultOrganizationID, Name: "管理员", Description: "系统管理员"}
	if err := db.Create(&defaultAdmin).Error; err != nil {
		t.Fatalf("seed default admin: %v", err)
	}
	if err := db.Create(&User{OrgID: "muteng", UserID: "u-muteng", Name: "测试列德管理员", Status: "active"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	err := ensureUserRoleInOrg("muteng", "u-muteng", defaultAdmin.ID)
	if err == nil {
		t.Fatal("expected cross-org role assign to fail")
	}
	var count int64
	if err := db.Model(&UserRole{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("cross-org assign must not write user_roles, count=%d", count)
	}
}

func TestRemapCrossOrgUserRoleBindings_RepairsPoisonedRows(t *testing.T) {
	db := openLiedeRoleSQLite(t)
	withTempDB(t, db)
	seedBasicPermissions(t, db)

	defaultAdmin := Role{OrgID: DefaultOrganizationID, Name: "管理员", Description: "系统管理员"}
	if err := db.Create(&defaultAdmin).Error; err != nil {
		t.Fatalf("seed default admin: %v", err)
	}
	// Intentionally poison: muteng user bound to default admin role id.
	if err := db.Create(&User{OrgID: "muteng", UserID: "test-admin", Name: "测试列德管理员", Status: "active"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&UserRole{OrgID: "muteng", UserID: "test-admin", RoleID: defaultAdmin.ID}).Error; err != nil {
		t.Fatalf("seed poisoned user_role: %v", err)
	}

	if err := remapCrossOrgUserRoleBindings(); err != nil {
		t.Fatalf("remap: %v", err)
	}

	var binding UserRole
	if err := db.Where("org_id = ? AND user_id = ?", "muteng", "test-admin").First(&binding).Error; err != nil {
		t.Fatalf("load binding: %v", err)
	}
	if binding.RoleID == defaultAdmin.ID {
		t.Fatalf("role_id still points to default admin %d", defaultAdmin.ID)
	}
	var target Role
	if err := db.Where("id = ?", binding.RoleID).First(&target).Error; err != nil {
		t.Fatalf("load target role: %v", err)
	}
	if target.OrgID != "muteng" || target.Name != "管理员" {
		t.Fatalf("target role = %#v, want muteng/管理员", target)
	}

	// Permission join used by production code must now return rows.
	var permCount int64
	if err := db.Table("permissions").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id AND role_permissions.deleted_at IS NULL").
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id AND user_roles.deleted_at IS NULL").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.org_id = user_roles.org_id AND roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND roles.org_id = ? AND permissions.deleted_at IS NULL",
			"muteng", "test-admin", "muteng").
		Distinct("permissions.id").
		Count(&permCount).Error; err != nil {
		t.Fatalf("perm count: %v", err)
	}
	if permCount == 0 {
		t.Fatal("expected non-zero permissions after remap + admin full access")
	}
}

func TestMigrateLiedeOrganizationAdminRoles_BindsLocalOrgAdmin(t *testing.T) {
	db := openLiedeRoleSQLite(t)
	withTempDB(t, db)
	seedBasicPermissions(t, db)

	// Pre-existing default admin role (the historic bug source).
	defaultAdmin := Role{OrgID: DefaultOrganizationID, Name: "管理员", Description: "系统管理员"}
	if err := db.Create(&defaultAdmin).Error; err != nil {
		t.Fatalf("seed default admin: %v", err)
	}

	users := []User{
		{OrgID: DefaultOrganizationID, UserID: "u-default", Name: "测试列德管理员", Status: "active"},
		{OrgID: "xiaotie", UserID: "xiaotie:test-admin", Name: "测试列德管理员", Status: "active"},
		{OrgID: "muteng", UserID: "test-admin", Name: "测试列德管理员", Status: "active"},
	}
	for _, u := range users {
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed user %s: %v", u.UserID, err)
		}
	}

	// Simulate previous poison on muteng/xiaotie.
	for _, orgUser := range []struct{ org, uid string }{
		{"muteng", "test-admin"},
		{"xiaotie", "xiaotie:test-admin"},
	} {
		if err := db.Create(&UserRole{OrgID: orgUser.org, UserID: orgUser.uid, RoleID: defaultAdmin.ID}).Error; err != nil {
			t.Fatalf("seed poison %s: %v", orgUser.org, err)
		}
	}

	migrateLiedeOrganizationAdminRoles()
	if err := remapCrossOrgUserRoleBindings(); err != nil {
		t.Fatalf("remap after migrate: %v", err)
	}

	for _, orgUser := range []struct{ org, uid string }{
		{DefaultOrganizationID, "u-default"},
		{"muteng", "test-admin"},
		{"xiaotie", "xiaotie:test-admin"},
	} {
		var binding UserRole
		if err := db.Where("org_id = ? AND user_id = ?", orgUser.org, orgUser.uid).First(&binding).Error; err != nil {
			t.Fatalf("binding %s/%s: %v", orgUser.org, orgUser.uid, err)
		}
		var role Role
		if err := db.Where("id = ?", binding.RoleID).First(&role).Error; err != nil {
			t.Fatalf("role for %s: %v", orgUser.org, err)
		}
		if role.OrgID != orgUser.org {
			t.Fatalf("%s user bound to role org=%s role_id=%d (defaultAdmin=%d)",
				orgUser.org, role.OrgID, role.ID, defaultAdmin.ID)
		}
		if role.Name != "管理员" {
			t.Fatalf("%s role name=%q", orgUser.org, role.Name)
		}

		var permCount int64
		if err := db.Table("permissions").
			Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id AND role_permissions.deleted_at IS NULL").
			Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id AND user_roles.deleted_at IS NULL").
			Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.org_id = user_roles.org_id AND roles.deleted_at IS NULL").
			Where("user_roles.org_id = ? AND user_roles.user_id = ? AND roles.org_id = ? AND permissions.deleted_at IS NULL",
				orgUser.org, orgUser.uid, orgUser.org).
			Distinct("permissions.id").
			Count(&permCount).Error; err != nil {
			t.Fatalf("perm count %s: %v", orgUser.org, err)
		}
		if permCount == 0 {
			t.Fatalf("%s still has zero permissions after migration", orgUser.org)
		}
	}
}
