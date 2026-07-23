package repository

import (
	"errors"
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openRoleIsolationSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
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
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func seedOrgUserAndRoles(t *testing.T, db *gorm.DB, org string) (userID string, localRoleID, foreignRoleID uint) {
	t.Helper()
	userID = "user-" + org
	foreign := otherOrg(org)
	// SQLite treats empty string as a value for unique indexes; populate org-unique fields.
	if err := db.Create(&database.User{
		OrgID:          org,
		UserID:         userID,
		DingTalkUserID: "dt-" + org + "-" + userID,
		Name:           "User " + org,
		Status:         "active",
		Email:          userID + "@" + org + ".test",
		Mobile:         "m-" + org + "-" + userID,
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	local := database.Role{OrgID: org, Name: "local-role-" + org, Description: "local"}
	if err := db.Create(&local).Error; err != nil {
		t.Fatalf("seed local role: %v", err)
	}
	foreignRole := database.Role{OrgID: foreign, Name: "foreign-role-" + foreign, Description: "foreign"}
	if err := db.Create(&foreignRole).Error; err != nil {
		t.Fatalf("seed foreign role: %v", err)
	}
	return userID, local.ID, foreignRole.ID
}

// TestUserRoleRepository_AssignEnforcesRoleOrg covers:
// - same-org assign succeeds
// - cross-org role_id fails and does not write
// - missing role fails
// for default / xiaotie / muteng.
func TestUserRoleRepository_AssignEnforcesRoleOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openRoleIsolationSQLite(t)
			userID, localRoleID, foreignRoleID := seedOrgUserAndRoles(t, db, org)
			repo := NewUserRoleRepository(db)

			// 1) current org user + current org role → success
			if err := repo.Assign(org, userID, localRoleID); err != nil {
				t.Fatalf("same-org assign: %v", err)
			}
			var count int64
			if err := db.Model(&database.UserRole{}).Where("org_id = ? AND user_id = ? AND role_id = ?", org, userID, localRoleID).Count(&count).Error; err != nil {
				t.Fatalf("count after success: %v", err)
			}
			if count != 1 {
				t.Fatalf("user_roles count = %d, want 1 after same-org assign", count)
			}

			// 2) current org user + other org role → fail, no write of foreign role
			err := repo.Assign(org, userID, foreignRoleID)
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("cross-org assign err = %v, want ErrRecordNotFound", err)
			}
			var foreignCount int64
			if err := db.Model(&database.UserRole{}).Where("org_id = ? AND user_id = ? AND role_id = ?", org, userID, foreignRoleID).Count(&foreignCount).Error; err != nil {
				t.Fatalf("count foreign: %v", err)
			}
			if foreignCount != 0 {
				t.Fatalf("foreign role was written: count=%d", foreignCount)
			}
			// Existing same-org assignment must remain (transaction rolled back before delete).
			if err := db.Model(&database.UserRole{}).Where("org_id = ? AND user_id = ? AND role_id = ?", org, userID, localRoleID).Count(&count).Error; err != nil {
				t.Fatalf("count after cross-org fail: %v", err)
			}
			if count != 1 {
				t.Fatalf("existing assignment must survive failed cross-org assign, count=%d", count)
			}

			// 3) missing role → fail
			err = repo.Assign(org, userID, 999999)
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("missing role err = %v, want ErrRecordNotFound", err)
			}
		})
	}
}

// TestUserRoleRepository_FindByUserIDFiltersCrossOrgRoles ensures poisoned
// user_roles rows that point at another org's role are not returned.
func TestUserRoleRepository_FindByUserIDFiltersCrossOrgRoles(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openRoleIsolationSQLite(t)
			userID, localRoleID, foreignRoleID := seedOrgUserAndRoles(t, db, org)
			// Legitimate local binding.
			if err := db.Create(&database.UserRole{OrgID: org, UserID: userID, RoleID: localRoleID}).Error; err != nil {
				t.Fatalf("seed local binding: %v", err)
			}
			// Poisoned row: same org user_roles.org_id but role belongs elsewhere.
			// Bypass Assign validation by direct insert (simulates legacy / attack data).
			if err := db.Create(&database.UserRole{OrgID: org, UserID: userID + "-poison", RoleID: foreignRoleID}).Error; err != nil {
				t.Fatalf("seed poison binding: %v", err)
			}

			repo := NewUserRoleRepository(db)
			roles, err := repo.FindByUserID(org, userID)
			if err != nil {
				t.Fatalf("FindByUserID: %v", err)
			}
			if len(roles) != 1 || roles[0].ID != localRoleID || roles[0].OrgID != org {
				t.Fatalf("FindByUserID roles = %#v, want single local role", roles)
			}

			poisoned, err := repo.FindByUserID(org, userID+"-poison")
			if err != nil {
				t.Fatalf("FindByUserID poison: %v", err)
			}
			if len(poisoned) != 0 {
				t.Fatalf("poisoned cross-org role leaked: %#v", poisoned)
			}
		})
	}
}

// TestUserRoleRepository_QuerySQLRequiresRolesOrgConsistency asserts generated
// SQL includes roles.org_id consistency for the three tenants (DryRun).
func TestUserRoleRepository_QuerySQLRequiresRolesOrgConsistency(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := newDryRunGORM(t)

			// FindByUserID
			{
				capSQL := captureUserRoleSQL(t, db, func(r *UserRoleRepository) {
					_, _ = r.FindByUserID(org, "alice")
				})
				if !strings.Contains(strings.ToLower(capSQL), "roles") {
					t.Fatalf("FindByUserID SQL missing roles join: %s", capSQL)
				}
				if !strings.Contains(strings.ToLower(capSQL), "org_id") {
					t.Fatalf("FindByUserID SQL missing org_id: %s", capSQL)
				}
			}

			// HasRole
			{
				capSQL := captureUserRoleSQL(t, db, func(r *UserRoleRepository) {
					_, _ = r.HasRole(org, "alice", "admin")
				})
				lower := strings.ToLower(capSQL)
				if !strings.Contains(lower, "roles") || !strings.Contains(lower, "org_id") {
					t.Fatalf("HasRole SQL must constrain roles.org_id: %s", capSQL)
				}
			}

			// FindByRoleID
			{
				capSQL := captureUserRoleSQL(t, db, func(r *UserRoleRepository) {
					_, _ = r.FindByRoleID(org, 1)
				})
				lower := strings.ToLower(capSQL)
				if !strings.Contains(lower, "roles") || !strings.Contains(lower, "org_id") {
					t.Fatalf("FindByRoleID SQL must join roles with org: %s", capSQL)
				}
			}

			// FindByUserRole (permissions path)
			{
				capSQL := captureRolePermSQL(t, db, func(r *RolePermissionRepository) {
					_, _ = r.FindByUserRole(org, "alice")
				})
				lower := strings.ToLower(capSQL)
				if !strings.Contains(lower, "roles") || !strings.Contains(lower, "org_id") {
					t.Fatalf("FindByUserRole SQL must join roles with org: %s", capSQL)
				}
			}
		})
	}
}

func captureUserRoleSQL(t *testing.T, db *gorm.DB, do func(*UserRoleRepository)) string {
	t.Helper()
	session := db.Session(&gorm.Session{DryRun: true, NewDB: false})
	var sql string
	name := "user-role-sql-" + t.Name()
	_ = session.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		sql = tx.Statement.SQL.String()
	})
	t.Cleanup(func() {
		_ = session.Callback().Query().Remove(name)
	})
	do(NewUserRoleRepository(session))
	return sql
}

func captureRolePermSQL(t *testing.T, db *gorm.DB, do func(*RolePermissionRepository)) string {
	t.Helper()
	session := db.Session(&gorm.Session{DryRun: true, NewDB: false})
	var sql string
	name := "role-perm-sql-" + t.Name()
	_ = session.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		sql = tx.Statement.SQL.String()
	})
	t.Cleanup(func() {
		_ = session.Callback().Query().Remove(name)
	})
	do(NewRolePermissionRepository(session))
	return sql
}

// TestRoleRepository_FindByIDAndOrg isolates role lookups by org.
func TestRoleRepository_FindByIDAndOrg(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openRoleIsolationSQLite(t)
			_, localRoleID, foreignRoleID := seedOrgUserAndRoles(t, db, org)
			repo := NewRoleRepository(db)

			got, err := repo.FindByIDAndOrg(localRoleID, org)
			if err != nil {
				t.Fatalf("FindByIDAndOrg local: %v", err)
			}
			if got.ID != localRoleID || got.OrgID != org {
				t.Fatalf("got = %#v", got)
			}

			_, err = repo.FindByIDAndOrg(foreignRoleID, org)
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("foreign role err = %v, want not found", err)
			}

			_, err = repo.FindByIDAndOrg(999999, org)
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("missing role err = %v, want not found", err)
			}
		})
	}
}

// TestRoleRepository_FindByIDFailClosed ensures unscoped role ID lookups are forbidden.
func TestRoleRepository_FindByIDFailClosed(t *testing.T) {
	db := openRoleIsolationSQLite(t)
	_, localRoleID, _ := seedOrgUserAndRoles(t, db, "muteng")
	repo := NewRoleRepository(db)

	_, err := repo.FindByID(localRoleID)
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindByID err = %v, want ErrMissingOrgID", err)
	}

	_, err = repo.FindByIDAndOrg(localRoleID, "")
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindByIDAndOrg empty org err = %v, want ErrMissingOrgID", err)
	}
}

// TestRoleRepository_UpdateRequiresOrgAndRejectsCrossOrg covers empty-org and cross-org writes.
func TestRoleRepository_UpdateRequiresOrgAndRejectsCrossOrg(t *testing.T) {
	db := openRoleIsolationSQLite(t)
	_, localRoleID, foreignRoleID := seedOrgUserAndRoles(t, db, "muteng")
	repo := NewRoleRepository(db)

	err := repo.Update(&database.Role{ID: localRoleID, OrgID: "", Name: "hijacked", Description: "x"})
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("Update empty org err = %v, want ErrMissingOrgID", err)
	}

	err = repo.Update(&database.Role{ID: foreignRoleID, OrgID: "muteng", Name: "stolen", Description: "x"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-org Update err = %v, want not found", err)
	}
	var foreign database.Role
	if err := db.First(&foreign, foreignRoleID).Error; err != nil {
		t.Fatalf("reload foreign: %v", err)
	}
	if foreign.Name == "stolen" {
		t.Fatalf("foreign role was modified by cross-org Update")
	}

	if err := repo.UpdateInOrg("muteng", &database.Role{ID: localRoleID, OrgID: "xiaotie", Name: "local-ok", Description: "d"}); err != nil {
		t.Fatalf("UpdateInOrg local: %v", err)
	}
	var local database.Role
	if err := db.First(&local, localRoleID).Error; err != nil {
		t.Fatalf("reload local: %v", err)
	}
	if local.Name != "local-ok" || local.OrgID != "muteng" {
		t.Fatalf("local after UpdateInOrg = %#v", local)
	}

	err = repo.UpdateInOrg("muteng", &database.Role{ID: foreignRoleID, Name: "nope", Description: "d"})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("UpdateInOrg foreign err = %v, want not found", err)
	}

	err = repo.UpdateInOrg("", &database.Role{ID: localRoleID, Name: "x", Description: "d"})
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("UpdateInOrg empty err = %v, want ErrMissingOrgID", err)
	}
}

// TestRoleRepository_EmptyOrgWriteOpsDoNotChangeDB covers Create/Assign empty-org fail-closed.
func TestRoleRepository_EmptyOrgWriteOpsDoNotChangeDB(t *testing.T) {
	db := openRoleIsolationSQLite(t)
	userID, localRoleID, _ := seedOrgUserAndRoles(t, db, "muteng")
	roleRepo := NewRoleRepository(db)
	userRoleRepo := NewUserRoleRepository(db)

	var beforeRoles, beforeUserRoles int64
	if err := db.Model(&database.Role{}).Count(&beforeRoles).Error; err != nil {
		t.Fatalf("count roles: %v", err)
	}
	if err := db.Model(&database.UserRole{}).Count(&beforeUserRoles).Error; err != nil {
		t.Fatalf("count user_roles: %v", err)
	}

	if err := roleRepo.Create(&database.Role{Name: "empty-org-role", Description: "x"}); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("Create empty org err = %v, want ErrMissingOrgID", err)
	}
	if err := userRoleRepo.Assign("", userID, localRoleID); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("Assign empty org err = %v, want ErrMissingOrgID", err)
	}
	if err := userRoleRepo.Remove("", userID, localRoleID); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("Remove empty org err = %v, want ErrMissingOrgID", err)
	}

	var afterRoles, afterUserRoles int64
	_ = db.Model(&database.Role{}).Count(&afterRoles)
	_ = db.Model(&database.UserRole{}).Count(&afterUserRoles)
	if afterRoles != beforeRoles || afterUserRoles != beforeUserRoles {
		t.Fatalf("empty-org writes changed DB: roles %d→%d user_roles %d→%d", beforeRoles, afterRoles, beforeUserRoles, afterUserRoles)
	}
}
