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

// openDeactivateMissingUsersDB 打开一个隔离的 SQLite 库并迁移用户/档案/部门表，供历史员工停用测试复用。
func openDeactivateMissingUsersDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:deactivate-missing-"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.EmployeeProfile{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

// seedDeactivateUser 写入一名员工及其档案，DingTalkUserID 为空时表示本地手工账号。
func seedDeactivateUser(t *testing.T, db *gorm.DB, orgID, userID, dingTalkUserID string, status string) {
	t.Helper()
	user := &database.User{
		OrgID:          orgID,
		UserID:         userID,
		DingTalkUserID: dingTalkUserID,
		Name:           orgID + " " + userID,
		Email:          orgID + "-" + userID + "@example.com",
		Mobile:         orgID + "-" + userID,
		DepartmentID:   "1",
		Status:         status,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user %s/%s: %v", orgID, userID, err)
	}
	profile := &database.EmployeeProfile{
		OrgID:         orgID,
		UserID:        userID,
		EmployeeID:    userID,
		ProfileStatus: status,
	}
	if err := db.Create(profile).Error; err != nil {
		t.Fatalf("seed profile %s/%s: %v", orgID, userID, err)
	}
}

// TestDeactivateUsersMissingFromDingTalk_DeactivatesHistoricalEmployees 验证：
// 本次钉钉源数据中已不存在的历史 active 员工被标为 inactive，档案状态同步更新。
func TestDeactivateUsersMissingFromDingTalk_DeactivatesHistoricalEmployees(t *testing.T) {
	db := openDeactivateMissingUsersDB(t)
	seedDeactivateUser(t, db, "org-a", "active-still", "dt-active-still", "active")
	seedDeactivateUser(t, db, "org-a", "active-stale", "dt-active-stale", "active")
	seedDeactivateUser(t, db, "org-a", "already-inactive", "dt-already-inactive", "inactive")

	repo := NewUserRepositoryWithOrgID(db, "org-a")
	deactivated, err := repo.DeactivateUsersMissingFromDingTalk([]string{"dt-active-still"})
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if len(deactivated) != 1 || deactivated[0] != "active-stale" {
		t.Fatalf("deactivated = %#v, want [active-stale]", deactivated)
	}

	var stillActive database.User
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "active-still").First(&stillActive).Error; err != nil {
		t.Fatalf("reload still active: %v", err)
	}
	if stillActive.Status != "active" {
		t.Fatalf("still active user status = %q, want active", stillActive.Status)
	}

	var stale database.User
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "active-stale").First(&stale).Error; err != nil {
		t.Fatalf("reload stale: %v", err)
	}
	if stale.Status != "inactive" {
		t.Fatalf("stale user status = %q, want inactive", stale.Status)
	}

	var alreadyInactive database.User
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "already-inactive").First(&alreadyInactive).Error; err != nil {
		t.Fatalf("reload already inactive: %v", err)
	}
	if alreadyInactive.Status != "inactive" {
		t.Fatalf("already inactive user status = %q, want inactive", alreadyInactive.Status)
	}

	var staleProfile database.EmployeeProfile
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "active-stale").First(&staleProfile).Error; err != nil {
		t.Fatalf("reload stale profile: %v", err)
	}
	if staleProfile.ProfileStatus != "inactive" {
		t.Fatalf("stale profile status = %q, want inactive", staleProfile.ProfileStatus)
	}

	var stillActiveProfile database.EmployeeProfile
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "active-still").First(&stillActiveProfile).Error; err != nil {
		t.Fatalf("reload still active profile: %v", err)
	}
	if stillActiveProfile.ProfileStatus != "active" {
		t.Fatalf("still active profile status = %q, want active", stillActiveProfile.ProfileStatus)
	}
}

// TestDeactivateUsersMissingFromDingTalk_EmptySourceFailsClosed 验证空源列表 fail-closed：
// 钉钉异常返回空数据时不得停用任何员工。
func TestDeactivateUsersMissingFromDingTalk_EmptySourceFailsClosed(t *testing.T) {
	db := openDeactivateMissingUsersDB(t)
	seedDeactivateUser(t, db, "org-a", "active-stale", "dt-active-stale", "active")

	repo := NewUserRepositoryWithOrgID(db, "org-a")
	deactivated, err := repo.DeactivateUsersMissingFromDingTalk(nil)
	if !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("empty source err = %v, want ErrInvalidData", err)
	}
	if deactivated != nil {
		t.Fatalf("deactivated = %#v, want nil", deactivated)
	}

	deactivated, err = repo.DeactivateUsersMissingFromDingTalk([]string{"  ", ""})
	if !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("whitespace source err = %v, want ErrInvalidData", err)
	}
	if deactivated != nil {
		t.Fatalf("whitespace deactivated = %#v, want nil", deactivated)
	}

	var user database.User
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "active-stale").First(&user).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.Status != "active" {
		t.Fatalf("empty source deactivated user: status=%q", user.Status)
	}
}

// TestDeactivateUsersMissingFromDingTalk_ManualAccountsUntouched 验证：
// admin、本地手工账号（无稳定钉钉用户 ID）与已经 inactive 的账号都不会被停用。
func TestDeactivateUsersMissingFromDingTalk_ManualAccountsUntouched(t *testing.T) {
	db := openDeactivateMissingUsersDB(t)
	// admin 账号：钉钉同步管理员本地行，DingTalkUserID 与源一致但应保留。
	seedDeactivateUser(t, db, "org-a", "admin", "dt-admin", "active")
	// 本地手工账号：没有稳定钉钉用户 ID。
	seedDeactivateUser(t, db, "org-a", "manual-local", "", "active")
	// 钉钉同步员工：源中不存在，应被停用。
	seedDeactivateUser(t, db, "org-a", "synced-stale", "dt-synced-stale", "active")

	repo := NewUserRepositoryWithOrgID(db, "org-a")
	deactivated, err := repo.DeactivateUsersMissingFromDingTalk([]string{"dt-admin"})
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if len(deactivated) != 1 || deactivated[0] != "synced-stale" {
		t.Fatalf("deactivated = %#v, want [synced-stale]", deactivated)
	}

	for _, userID := range []string{"admin", "manual-local"} {
		var user database.User
		if err := db.Where("org_id = ? AND user_id = ?", "org-a", userID).First(&user).Error; err != nil {
			t.Fatalf("reload %s: %v", userID, err)
		}
		if user.Status != "active" {
			t.Fatalf("%s status = %q, want active", userID, user.Status)
		}
		var profile database.EmployeeProfile
		if err := db.Where("org_id = ? AND user_id = ?", "org-a", userID).First(&profile).Error; err != nil {
			t.Fatalf("reload %s profile: %v", userID, err)
		}
		if profile.ProfileStatus != "active" {
			t.Fatalf("%s profile status = %q, want active", userID, profile.ProfileStatus)
		}
	}
}

// TestDeactivateUsersMissingFromDingTalk_TenantIsolation 验证：
// 停用仅作用于当前 org_id，跨租户同名/同钉钉 ID 的员工不受影响。
func TestDeactivateUsersMissingFromDingTalk_TenantIsolation(t *testing.T) {
	db := openDeactivateMissingUsersDB(t)
	// org-a 与 org-b 各有一名使用相同 DingTalkUserID 的员工。
	seedDeactivateUser(t, db, "org-a", "user-stale", "dt-shared", "active")
	seedDeactivateUser(t, db, "org-b", "user-stale", "dt-shared", "active")

	repo := NewUserRepositoryWithOrgID(db, "org-a")
	// org-a 同步源为空集（不含 dt-shared），应停用 org-a 的员工。
	deactivated, err := repo.DeactivateUsersMissingFromDingTalk([]string{"dt-other"})
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if len(deactivated) != 1 || deactivated[0] != "user-stale" {
		t.Fatalf("deactivated = %#v, want [user-stale]", deactivated)
	}

	var orgAUser database.User
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "user-stale").First(&orgAUser).Error; err != nil {
		t.Fatalf("reload org-a: %v", err)
	}
	if orgAUser.Status != "inactive" {
		t.Fatalf("org-a user status = %q, want inactive", orgAUser.Status)
	}

	var orgBUser database.User
	if err := db.Where("org_id = ? AND user_id = ?", "org-b", "user-stale").First(&orgBUser).Error; err != nil {
		t.Fatalf("reload org-b: %v", err)
	}
	if orgBUser.Status != "active" {
		t.Fatalf("org-b user status = %q, want active (tenant isolation)", orgBUser.Status)
	}
}

// TestDeactivateUsersMissingFromDingTalk_EmptyOrgFailClosed 验证未绑定 org 时 fail-closed。
func TestDeactivateUsersMissingFromDingTalk_EmptyOrgFailClosed(t *testing.T) {
	db := openDeactivateMissingUsersDB(t)
	seedDeactivateUser(t, db, "org-a", "user-stale", "dt-shared", "active")

	repo := NewUserRepositoryWithOrgID(db, "")
	deactivated, err := repo.DeactivateUsersMissingFromDingTalk([]string{"dt-other"})
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("empty org err = %v, want ErrMissingOrgID", err)
	}
	if deactivated != nil {
		t.Fatalf("empty org deactivated = %#v, want nil", deactivated)
	}

	var user database.User
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "user-stale").First(&user).Error; err != nil {
		t.Fatalf("reload user: %v", err)
	}
	if user.Status != "active" {
		t.Fatalf("empty org deactivated user: status=%q", user.Status)
	}
}

// TestDeactivateUsersMissingFromDingTalk_NoCandidatesReturnsEmpty 验证：
// 当本组织无符合条件的 active 历史员工时，返回空列表且不报错。
func TestDeactivateUsersMissingFromDingTalk_NoCandidatesReturnsEmpty(t *testing.T) {
	db := openDeactivateMissingUsersDB(t)
	seedDeactivateUser(t, db, "org-a", "already-inactive", "dt-already-inactive", "inactive")
	seedDeactivateUser(t, db, "org-a", "manual-local", "", "active")

	repo := NewUserRepositoryWithOrgID(db, "org-a")
	deactivated, err := repo.DeactivateUsersMissingFromDingTalk([]string{"dt-other"})
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if len(deactivated) != 0 {
		t.Fatalf("deactivated = %#v, want empty", deactivated)
	}
}
