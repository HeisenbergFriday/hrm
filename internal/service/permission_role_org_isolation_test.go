package service

import (
	"errors"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openPermissionIsolationSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:perm-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Role{},
		&database.UserRole{},
		&database.Permission{},
		&database.RolePermission{},
		&database.MenuPermission{},
		&database.DataPermission{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func otherPermissionOrg(org string) string {
	switch org {
	case "default":
		return "muteng"
	case "muteng":
		return "xiaotie"
	default:
		return "default"
	}
}

func seedTestUser(t *testing.T, db *gorm.DB, org, userID, name string) {
	t.Helper()
	// SQLite treats empty string as a value for unique indexes; populate org-unique fields.
	u := &database.User{
		OrgID:          org,
		UserID:         userID,
		DingTalkUserID: "dt-" + org + "-" + userID,
		Name:           name,
		Status:         "active",
		Email:          userID + "@" + org + ".test",
		Mobile:         "m-" + org + "-" + userID,
	}
	if err := db.Create(u).Error; err != nil {
		t.Fatalf("seed user %s/%s: %v", org, userID, err)
	}
}

func seedPermissionOrgFixture(t *testing.T, db *gorm.DB, org string) (userID string, localRoleID, foreignRoleID uint) {
	t.Helper()
	userID = "emp-" + org
	foreign := otherPermissionOrg(org)
	seedTestUser(t, db, org, userID, "Emp "+org)
	// A second user in foreign org with the same user_id string — must never be used.
	seedTestUser(t, db, foreign, userID, "Foreign twin")
	local := database.Role{OrgID: org, Name: "admin-" + org, Description: "local admin"}
	if err := db.Create(&local).Error; err != nil {
		t.Fatalf("seed local role: %v", err)
	}
	foreignRole := database.Role{OrgID: foreign, Name: "super-" + foreign, Description: "foreign super"}
	if err := db.Create(&foreignRole).Error; err != nil {
		t.Fatalf("seed foreign role: %v", err)
	}
	return userID, local.ID, foreignRole.ID
}

// TestAssignUserRoleInOrg_OrgIsolation covers the required cases across three orgs:
// success same-org, fail cross-org role (no write), fail missing role, query isolation,
// default employee role scoped to current org.
func TestAssignUserRoleInOrg_OrgIsolation(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openPermissionIsolationSQLite(t)
			userID, localRoleID, foreignRoleID := seedPermissionOrgFixture(t, db, org)
			svc := NewPermissionServiceWithOrgID(db, org)

			// 1) current org user + current org role → success
			if err := svc.AssignUserRoleInOrg(org, userID, localRoleID); err != nil {
				t.Fatalf("same-org assign: %v", err)
			}
			roles, err := svc.GetUserRolesInOrg(org, userID)
			if err != nil {
				t.Fatalf("GetUserRolesInOrg: %v", err)
			}
			if len(roles) != 1 || roles[0].ID != localRoleID || roles[0].OrgID != org {
				t.Fatalf("roles after assign = %#v", roles)
			}

			// 2) current org user + other org role → fail and no write
			err = svc.AssignUserRoleInOrg(org, userID, foreignRoleID)
			if !errors.Is(err, ErrRoleNotInOrg) {
				t.Fatalf("cross-org role err = %v, want ErrRoleNotInOrg", err)
			}
			var foreignBindings int64
			if err := db.Model(&database.UserRole{}).
				Where("org_id = ? AND user_id = ? AND role_id = ?", org, userID, foreignRoleID).
				Count(&foreignBindings).Error; err != nil {
				t.Fatalf("count foreign binding: %v", err)
			}
			if foreignBindings != 0 {
				t.Fatalf("cross-org role was written into user_roles")
			}
			// Previous local assignment must remain.
			roles, err = svc.GetUserRolesInOrg(org, userID)
			if err != nil {
				t.Fatalf("GetUserRolesInOrg after fail: %v", err)
			}
			if len(roles) != 1 || roles[0].ID != localRoleID {
				t.Fatalf("local assignment lost after failed cross-org assign: %#v", roles)
			}

			// 3) missing role → fail
			err = svc.AssignUserRoleInOrg(org, userID, 999999)
			if !errors.Is(err, ErrRoleNotInOrg) {
				t.Fatalf("missing role err = %v, want ErrRoleNotInOrg", err)
			}

			// 4) query must not return other org roles even if poisoned binding exists
			poisonUser := "poison-" + org
			seedTestUser(t, db, org, poisonUser, "Poison")
			if err := db.Create(&database.UserRole{OrgID: org, UserID: poisonUser, RoleID: foreignRoleID}).Error; err != nil {
				t.Fatalf("seed poison binding: %v", err)
			}
			poisonRoles, err := svc.GetUserRolesInOrg(org, poisonUser)
			if err != nil {
				t.Fatalf("GetUserRolesInOrg poison: %v", err)
			}
			if len(poisonRoles) != 0 {
				t.Fatalf("poisoned cross-org role leaked: %#v", poisonRoles)
			}
		})
	}
}

func TestAssignDefaultEmployeeRole_UsesCurrentOrgRole(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openPermissionIsolationSQLite(t)
			foreign := otherPermissionOrg(org)

			// Pre-create a foreign-org default employee role with a different ID.
			foreignDefault := database.Role{OrgID: foreign, Name: DefaultEmployeeRoleName, Description: "foreign default"}
			if err := db.Create(&foreignDefault).Error; err != nil {
				t.Fatalf("seed foreign default role: %v", err)
			}

			userID := "newcomer-" + org
			seedTestUser(t, db, org, userID, "Newcomer")

			svc := NewPermissionServiceWithOrgID(db, org)
			assigned, err := svc.AssignDefaultEmployeeRoleIfUnassignedInOrg(org, userID)
			if err != nil {
				t.Fatalf("AssignDefaultEmployeeRoleIfUnassignedInOrg: %v", err)
			}
			if !assigned {
				t.Fatalf("expected default role to be assigned")
			}

			roles, err := svc.GetUserRolesInOrg(org, userID)
			if err != nil {
				t.Fatalf("GetUserRolesInOrg: %v", err)
			}
			if len(roles) != 1 {
				t.Fatalf("roles = %#v, want 1", roles)
			}
			if roles[0].Name != DefaultEmployeeRoleName {
				t.Fatalf("role name = %q, want %q", roles[0].Name, DefaultEmployeeRoleName)
			}
			if roles[0].OrgID != org {
				t.Fatalf("role org = %q, want %q", roles[0].OrgID, org)
			}
			if roles[0].ID == foreignDefault.ID {
				t.Fatalf("default employee role reused foreign org role id=%d", foreignDefault.ID)
			}

			// Ensure the created role row is also org-scoped.
			var stored database.Role
			if err := db.First(&stored, roles[0].ID).Error; err != nil {
				t.Fatalf("load stored role: %v", err)
			}
			if stored.OrgID != org {
				t.Fatalf("stored role org = %q, want %q", stored.OrgID, org)
			}
		})
	}
}

func TestAssignUserRoleInOrg_RejectsUserOutsideOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openPermissionIsolationSQLite(t)
			foreign := otherPermissionOrg(org)
			// User only exists in foreign org.
			seedTestUser(t, db, foreign, "only-foreign", "X")
			local := database.Role{OrgID: org, Name: "r-" + org}
			if err := db.Create(&local).Error; err != nil {
				t.Fatalf("seed role: %v", err)
			}

			svc := NewPermissionServiceWithOrgID(db, org)
			err := svc.AssignUserRoleInOrg(org, "only-foreign", local.ID)
			if !errors.Is(err, ErrUserNotInOrg) {
				t.Fatalf("err = %v, want ErrUserNotInOrg", err)
			}
			var count int64
			_ = db.Model(&database.UserRole{}).Where("user_id = ?", "only-foreign").Count(&count)
			if count != 0 {
				t.Fatalf("user_roles written for out-of-org user, count=%d", count)
			}
		})
	}
}
