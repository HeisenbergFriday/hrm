package repository

import (
	"errors"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openUserDepartmentMembershipDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:user-department-membership-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Department{}, &database.UserDepartmentMembership{}, &database.EmployeeProfile{}); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	return db
}

func TestDeactivateUsersMissingFromDingTalkRequiresNonEmptySourceAndStaysInTenant(t *testing.T) {
	db := openUserDepartmentMembershipDB(t)
	seedMembershipDepartment(t, db, "org-a", "dept")
	seedMembershipDepartment(t, db, "org-b", "dept")
	seedMembershipUser(t, db, "org-a", "current", "dept")
	seedMembershipUser(t, db, "org-a", "stale", "dept")
	seedMembershipUser(t, db, "org-b", "stale", "dept")
	for _, profile := range []database.EmployeeProfile{
		{OrgID: "org-a", UserID: "stale", EmployeeID: "org-a-stale", ProfileStatus: "active"},
		{OrgID: "org-b", UserID: "stale", EmployeeID: "org-b-stale", ProfileStatus: "active"},
	} {
		if err := db.Create(&profile).Error; err != nil {
			t.Fatalf("seed profile: %v", err)
		}
	}

	repo := NewUserRepositoryWithOrgID(db, "org-a")
	if _, err := repo.DeactivateUsersMissingFromDingTalk(nil); !errors.Is(err, gorm.ErrInvalidData) {
		t.Fatalf("empty source err=%v, want invalid data", err)
	}
	deactivated, err := repo.DeactivateUsersMissingFromDingTalk([]string{"org-a-current"})
	if err != nil || len(deactivated) != 1 || deactivated[0] != "stale" {
		t.Fatalf("deactivated=%#v err=%v, want org-a stale", deactivated, err)
	}

	for _, test := range []struct {
		orgID, userID, wantStatus string
	}{
		{orgID: "org-a", userID: "current", wantStatus: "active"},
		{orgID: "org-a", userID: "stale", wantStatus: "inactive"},
		{orgID: "org-b", userID: "stale", wantStatus: "active"},
	} {
		var user database.User
		if err := db.Where("org_id = ? AND user_id = ?", test.orgID, test.userID).First(&user).Error; err != nil {
			t.Fatalf("load %s/%s: %v", test.orgID, test.userID, err)
		}
		if user.Status != test.wantStatus {
			t.Fatalf("%s/%s status=%q want=%q", test.orgID, test.userID, user.Status, test.wantStatus)
		}
	}
	var orgAProfile, orgBProfile database.EmployeeProfile
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "stale").First(&orgAProfile).Error; err != nil {
		t.Fatalf("load org-a profile: %v", err)
	}
	if err := db.Where("org_id = ? AND user_id = ?", "org-b", "stale").First(&orgBProfile).Error; err != nil {
		t.Fatalf("load org-b profile: %v", err)
	}
	if orgAProfile.ProfileStatus != "inactive" || orgBProfile.ProfileStatus != "active" {
		t.Fatalf("profile statuses crossed tenant boundary: org-a=%q org-b=%q", orgAProfile.ProfileStatus, orgBProfile.ProfileStatus)
	}
}

func seedMembershipDepartment(t *testing.T, db *gorm.DB, orgID, departmentID string) {
	t.Helper()
	department := &database.Department{
		OrgID:                orgID,
		DepartmentID:         departmentID,
		DingTalkDepartmentID: orgID + "-" + departmentID,
		Name:                 orgID + " " + departmentID,
	}
	if err := db.Create(department).Error; err != nil {
		t.Fatalf("seed department %s/%s: %v", orgID, departmentID, err)
	}
}

func seedMembershipUser(t *testing.T, db *gorm.DB, orgID, userID, departmentID string) {
	t.Helper()
	user := &database.User{
		OrgID:          orgID,
		UserID:         userID,
		DingTalkUserID: orgID + "-" + userID,
		Name:           orgID + " " + userID,
		Email:          orgID + "-" + userID + "@example.com",
		Mobile:         orgID + "-" + userID,
		DepartmentID:   departmentID,
		Status:         "active",
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("seed user %s/%s: %v", orgID, userID, err)
	}
}

func TestUserRepositoryDepartmentMembershipsReplaceAndQueryWithTenantIsolation(t *testing.T) {
	db := openUserDepartmentMembershipDB(t)
	for _, departmentID := range []string{"dept-primary", "dept-secondary", "dept-legacy", "dept-stale"} {
		seedMembershipDepartment(t, db, "org-a", departmentID)
	}
	for _, departmentID := range []string{"dept-primary", "dept-secondary", "org-b-only"} {
		seedMembershipDepartment(t, db, "org-b", departmentID)
	}

	seedMembershipUser(t, db, "org-a", "single-user", "dept-primary")
	seedMembershipUser(t, db, "org-a", "multi-user", "dept-stale")
	seedMembershipUser(t, db, "org-a", "legacy-user", "dept-legacy")
	seedMembershipUser(t, db, "org-a", "shared-user", "dept-primary")
	seedMembershipUser(t, db, "org-b", "shared-user", "dept-secondary")

	orgA := NewUserRepositoryWithOrgID(db, "org-a")
	orgB := NewUserRepositoryWithOrgID(db, "org-b")
	if err := orgA.ReplaceDepartmentMemberships("single-user", []string{"dept-primary"}); err != nil {
		t.Fatalf("replace single membership: %v", err)
	}
	if err := orgA.ReplaceDepartmentMemberships("multi-user", []string{"dept-primary", "dept-secondary", "dept-primary"}); err != nil {
		t.Fatalf("replace multi memberships: %v", err)
	}
	if err := orgA.ReplaceDepartmentMemberships("shared-user", []string{"dept-primary"}); err != nil {
		t.Fatalf("replace org-a shared membership: %v", err)
	}
	if err := orgB.ReplaceDepartmentMemberships("shared-user", []string{"dept-secondary"}); err != nil {
		t.Fatalf("replace org-b shared membership: %v", err)
	}

	var multiMemberships []database.UserDepartmentMembership
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "multi-user").Order("id ASC").Find(&multiMemberships).Error; err != nil {
		t.Fatalf("load multi memberships: %v", err)
	}
	if len(multiMemberships) != 2 || !multiMemberships[0].IsPrimary || multiMemberships[1].IsPrimary {
		t.Fatalf("multi memberships = %#v, want two rows with only first primary", multiMemberships)
	}
	var initialSingleMemberships []database.UserDepartmentMembership
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "single-user").Find(&initialSingleMemberships).Error; err != nil {
		t.Fatalf("load single membership: %v", err)
	}
	if len(initialSingleMemberships) != 1 || !initialSingleMemberships[0].IsPrimary || initialSingleMemberships[0].DepartmentID != "dept-primary" {
		t.Fatalf("single memberships = %#v, want one primary row", initialSingleMemberships)
	}

	secondaryUsers, total, err := orgA.FindByDepartment("dept-secondary", 1, 20)
	if err != nil {
		t.Fatalf("find secondary department: %v", err)
	}
	if total != 1 || len(secondaryUsers) != 1 || secondaryUsers[0].UserID != "multi-user" {
		t.Fatalf("secondary users = %#v total=%d, want multi-user", secondaryUsers, total)
	}

	legacyUsers, total, err := orgA.FindByDepartment("dept-legacy", 1, 20)
	if err != nil {
		t.Fatalf("find legacy department: %v", err)
	}
	if total != 1 || len(legacyUsers) != 1 || legacyUsers[0].UserID != "legacy-user" {
		t.Fatalf("legacy users = %#v total=%d, want fallback legacy-user", legacyUsers, total)
	}

	staleUsers, total, err := orgA.FindByDepartment("dept-stale", 1, 20)
	if err != nil {
		t.Fatalf("find stale department: %v", err)
	}
	if total != 0 || len(staleUsers) != 0 {
		t.Fatalf("stale primary fallback remained active after memberships were generated: %#v total=%d", staleUsers, total)
	}

	orgBUsers, total, err := orgB.FindByDepartment("dept-secondary", 1, 20)
	if err != nil {
		t.Fatalf("find org-b department: %v", err)
	}
	if total != 1 || len(orgBUsers) != 1 || orgBUsers[0].OrgID != "org-b" || orgBUsers[0].UserID != "shared-user" {
		t.Fatalf("org-b users = %#v total=%d, want only org-b shared-user", orgBUsers, total)
	}

	if err := orgA.ReplaceDepartmentMemberships("single-user", []string{"org-b-only"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-org department replacement err=%v, want record not found", err)
	}
	var singleMemberships []database.UserDepartmentMembership
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "single-user").Find(&singleMemberships).Error; err != nil {
		t.Fatalf("load single memberships after rejected replacement: %v", err)
	}
	if len(singleMemberships) != 1 || singleMemberships[0].DepartmentID != "dept-primary" {
		t.Fatalf("rejected replacement changed prior memberships: %#v", singleMemberships)
	}

	var orgBMemberships int64
	if err := db.Model(&database.UserDepartmentMembership{}).
		Where("org_id = ? AND user_id = ? AND department_id = ?", "org-b", "shared-user", "dept-secondary").
		Count(&orgBMemberships).Error; err != nil {
		t.Fatalf("count org-b memberships: %v", err)
	}
	if orgBMemberships != 1 {
		t.Fatalf("org-a replacement affected org-b memberships: %d", orgBMemberships)
	}
}
