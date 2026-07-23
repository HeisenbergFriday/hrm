package service

import (
	"errors"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openPermissionPublicFailClosedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:perm-public-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Role{},
		&database.UserRole{},
		&database.Permission{},
		&database.RolePermission{},
		&database.MenuPermission{},
		&database.DataPermission{},
		&database.Department{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed default-org role so a non-fail-closed GetRoles would return rows.
	if err := db.Create(&database.Role{OrgID: "default", Name: "seed-admin", Description: "seed"}).Error; err != nil {
		t.Fatalf("seed role: %v", err)
	}
	if err := db.Create(&database.User{
		OrgID:          "default",
		UserID:         "seed-user",
		DingTalkUserID: "dt-seed",
		Name:           "Seed",
		Status:         "active",
		Email:          "seed@default.test",
		Mobile:         "m-seed",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return db
}

// TestPermissionService_PublicMethodsEmptyOrgFailClosed verifies every public
// tenant method rejects empty organization context with ErrMissingOrgID and
// never returns unscoped role rows.
func TestPermissionService_PublicMethodsEmptyOrgFailClosed(t *testing.T) {
	db := openPermissionPublicFailClosedDB(t)
	svc := NewPermissionServiceWithOrgID(db, "")

	t.Run("GetRoles", func(t *testing.T) {
		roles, total, err := svc.GetRoles()
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("GetRoles err=%v, want ErrMissingOrgID", err)
		}
		if roles != nil || total != 0 {
			t.Fatalf("GetRoles must return empty result on missing org, got roles=%#v total=%d", roles, total)
		}
	})

	t.Run("CreateRole", func(t *testing.T) {
		err := svc.CreateRole(&database.Role{Name: "x"})
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("CreateRole err=%v, want ErrMissingOrgID", err)
		}
		var count int64
		if err := db.Model(&database.Role{}).Where("name = ?", "x").Count(&count).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Fatalf("CreateRole empty org must not write rows, count=%d", count)
		}
	})

	t.Run("UpdateRole", func(t *testing.T) {
		var role database.Role
		if err := db.Where("name = ?", "seed-admin").First(&role).Error; err != nil {
			t.Fatalf("load seed: %v", err)
		}
		role.Description = "mutated"
		err := svc.UpdateRole(&role)
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("UpdateRole err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("GetRolePermissions", func(t *testing.T) {
		var role database.Role
		if err := db.Where("name = ?", "seed-admin").First(&role).Error; err != nil {
			t.Fatalf("load seed: %v", err)
		}
		_, err := svc.GetRolePermissions(role.ID)
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("GetRolePermissions err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("SaveRolePermissions", func(t *testing.T) {
		var role database.Role
		if err := db.Where("name = ?", "seed-admin").First(&role).Error; err != nil {
			t.Fatalf("load seed: %v", err)
		}
		err := svc.SaveRolePermissions(role.ID, []uint{1})
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("SaveRolePermissions err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("GetUserRoles", func(t *testing.T) {
		_, err := svc.GetUserRoles("seed-user")
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("GetUserRoles err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("GetUserPermissions", func(t *testing.T) {
		_, err := svc.GetUserPermissions("seed-user")
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("GetUserPermissions err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("AssignUserRole", func(t *testing.T) {
		var role database.Role
		if err := db.Where("name = ?", "seed-admin").First(&role).Error; err != nil {
			t.Fatalf("load seed: %v", err)
		}
		err := svc.AssignUserRole("seed-user", role.ID)
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("AssignUserRole err=%v, want ErrMissingOrgID", err)
		}
		var count int64
		if err := db.Model(&database.UserRole{}).Count(&count).Error; err != nil {
			t.Fatalf("count: %v", err)
		}
		if count != 0 {
			t.Fatalf("AssignUserRole empty org must not write, count=%d", count)
		}
	})

	t.Run("RemoveUserRole", func(t *testing.T) {
		err := svc.RemoveUserRole("seed-user", 1)
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("RemoveUserRole err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("GetRoleUsers", func(t *testing.T) {
		_, err := svc.GetRoleUsers(1)
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("GetRoleUsers err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("ResolveUserScope", func(t *testing.T) {
		_, err := svc.ResolveUserScope("seed-user")
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("ResolveUserScope err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("GetUserMenuKeys", func(t *testing.T) {
		_, err := svc.GetUserMenuKeys("seed-user")
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("GetUserMenuKeys err=%v, want ErrMissingOrgID", err)
		}
	})

	t.Run("AssignDefaultEmployeeRoleIfUnassigned", func(t *testing.T) {
		_, err := svc.AssignDefaultEmployeeRoleIfUnassigned("seed-user")
		if !errors.Is(err, repository.ErrMissingOrgID) {
			t.Fatalf("AssignDefaultEmployeeRoleIfUnassigned err=%v, want ErrMissingOrgID", err)
		}
	})
}

func TestPermissionService_GetRolesOnlyReturnsBoundOrg(t *testing.T) {
	db := openPermissionPublicFailClosedDB(t)
	if err := db.Create(&database.Role{OrgID: "muteng", Name: "muteng-only"}).Error; err != nil {
		t.Fatalf("seed muteng: %v", err)
	}
	svc := NewPermissionServiceWithOrgID(db, "muteng")
	roles, total, err := svc.GetRoles()
	if err != nil {
		t.Fatalf("GetRoles: %v", err)
	}
	if total != 1 || len(roles) != 1 || roles[0].OrgID != "muteng" {
		t.Fatalf("GetRoles = %#v total=%d, want only muteng role", roles, total)
	}
}
