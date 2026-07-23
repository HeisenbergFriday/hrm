package repository

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openEmployeeIsolationSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Department{},
		&database.EmployeeProfile{},
		&database.EmployeeTransfer{},
		&database.EmployeeResignation{},
		&database.EmployeeOnboarding{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

// seedTwinOrgEmployees creates org A/B with identical user_id + department_id.
// This is the core cross-tenant collision case for department filters.
func seedTwinOrgEmployees(t *testing.T, db *gorm.DB) (profileA, profileB *database.EmployeeProfile) {
	t.Helper()
	const sharedUserID = "same-user"
	const sharedDeptID = "same-dept"

	for _, org := range []string{"org-a", "org-b"} {
		user := &database.User{
			OrgID:          org,
			UserID:         sharedUserID,
			DingTalkUserID: "dt-" + org,
			Name:           "User " + org,
			Email:          sharedUserID + "@" + org + ".test",
			Mobile:         "m-" + org,
			DepartmentID:   sharedDeptID,
			Status:         "active",
		}
		if err := db.Create(user).Error; err != nil {
			t.Fatalf("seed user %s: %v", org, err)
		}
		if err := db.Create(&database.Department{
			OrgID:        org,
			DepartmentID: sharedDeptID,
			Name:         "Dept " + org,
		}).Error; err != nil {
			t.Fatalf("seed dept %s: %v", org, err)
		}
		profile := &database.EmployeeProfile{
			OrgID:         org,
			UserID:        sharedUserID,
			EmployeeID:    "emp-" + org,
			ProfileStatus: "active",
		}
		if err := db.Create(profile).Error; err != nil {
			t.Fatalf("seed profile %s: %v", org, err)
		}
		if org == "org-a" {
			profileA = profile
		} else {
			profileB = profile
		}
	}
	return profileA, profileB
}

func TestEmployeeRepository_FindAllProfilesDepartmentFilterCrossOrg(t *testing.T) {
	db := openEmployeeIsolationSQLite(t)
	profileA, profileB := seedTwinOrgEmployees(t, db)

	// Org A department filter must only see org-a profile, never org-b.
	repoA := NewEmployeeRepositoryWithOrgID(db, "org-a")
	items, total, err := repoA.FindAllProfiles(1, 20, map[string]string{
		"department_id": "same-dept",
	})
	if err != nil {
		t.Fatalf("org-a dept filter: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("org-a total=%d len=%d, want 1", total, len(items))
	}
	if items[0].ID != profileA.ID || items[0].OrgID != "org-a" {
		t.Fatalf("org-a result = %+v, want profileA id=%d", items[0], profileA.ID)
	}

	// Org B same department_id must not pull org-a.
	repoB := NewEmployeeRepositoryWithOrgID(db, "org-b")
	itemsB, totalB, err := repoB.FindAllProfiles(1, 20, map[string]string{
		"department_id": "same-dept",
	})
	if err != nil {
		t.Fatalf("org-b dept filter: %v", err)
	}
	if totalB != 1 || len(itemsB) != 1 {
		t.Fatalf("org-b total=%d len=%d, want 1", totalB, len(itemsB))
	}
	if itemsB[0].ID != profileB.ID || itemsB[0].OrgID != "org-b" {
		t.Fatalf("org-b result = %+v, want profileB id=%d", itemsB[0], profileB.ID)
	}

	// Multi-department filter with shared department_id.
	itemsMulti, totalMulti, err := repoA.FindAllProfiles(1, 20, map[string]string{
		"department_ids": "same-dept,other-dept",
	})
	if err != nil {
		t.Fatalf("org-a multi dept: %v", err)
	}
	if totalMulti != 1 || len(itemsMulti) != 1 || itemsMulti[0].OrgID != "org-a" {
		t.Fatalf("multi dept leaked cross-org: total=%d items=%+v", totalMulti, itemsMulti)
	}
}

func TestEmployeeRepository_FindProfileByIDCrossOrg(t *testing.T) {
	db := openEmployeeIsolationSQLite(t)
	profileA, profileB := seedTwinOrgEmployees(t, db)

	repoA := NewEmployeeRepositoryWithOrgID(db, "org-a")
	got, err := repoA.FindProfileByID(fmt.Sprintf("%d", profileA.ID))
	if err != nil {
		t.Fatalf("same-org by id: %v", err)
	}
	if got.ID != profileA.ID {
		t.Fatalf("got id=%d want %d", got.ID, profileA.ID)
	}

	// Cross-org primary key must not be readable (404 / record not found).
	_, err = repoA.FindProfileByID(fmt.Sprintf("%d", profileB.ID))
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-org by id err=%v, want ErrRecordNotFound", err)
	}
}

func TestEmployeeRepository_FindProfileByUserIDCrossOrg(t *testing.T) {
	db := openEmployeeIsolationSQLite(t)
	profileA, _ := seedTwinOrgEmployees(t, db)

	repoA := NewEmployeeRepositoryWithOrgID(db, "org-a")
	got, err := repoA.FindProfileByUserID("same-user")
	if err != nil {
		t.Fatalf("same-org by user_id: %v", err)
	}
	if got.ID != profileA.ID || got.OrgID != "org-a" {
		t.Fatalf("got %+v, want org-a profile", got)
	}

	// Filter by user_id in list must stay within org.
	items, total, err := repoA.FindAllProfiles(1, 20, map[string]string{
		"user_id": "same-user",
	})
	if err != nil {
		t.Fatalf("list by user_id: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].OrgID != "org-a" {
		t.Fatalf("user_id filter leaked: total=%d items=%+v", total, items)
	}
}

func TestEmployeeRepository_MissingOrgFailClosed(t *testing.T) {
	db := openEmployeeIsolationSQLite(t)
	_, _ = seedTwinOrgEmployees(t, db)

	// Legacy empty constructor: reads and writes must fail closed.
	legacy := NewEmployeeRepository(db)

	_, total, err := legacy.FindAllProfiles(1, 20, map[string]string{"department_id": "same-dept"})
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindAllProfiles empty org err=%v, want ErrMissingOrgID", err)
	}
	if total != 0 {
		t.Fatalf("empty org total=%d, want 0", total)
	}

	_, err = legacy.FindProfileByID("1")
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindProfileByID empty org err=%v, want ErrMissingOrgID", err)
	}

	_, err = legacy.FindProfileByUserID("same-user")
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindProfileByUserID empty org err=%v, want ErrMissingOrgID", err)
	}

	err = legacy.CreateProfile(&database.EmployeeProfile{UserID: "x", EmployeeID: "x"})
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("CreateProfile empty org err=%v, want ErrMissingOrgID", err)
	}

	_, total, err = legacy.FindAllTransfers(1, 20, nil)
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindAllTransfers empty org err=%v, want ErrMissingOrgID", err)
	}
	if total != 0 {
		t.Fatalf("transfers total=%d", total)
	}

	_, total, err = legacy.FindAllResignations(1, 20, nil)
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindAllResignations empty org err=%v, want ErrMissingOrgID", err)
	}
	if total != 0 {
		t.Fatalf("resignations total=%d", total)
	}

	_, total, err = legacy.FindAllOnboardings(1, 20, nil)
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindAllOnboardings empty org err=%v, want ErrMissingOrgID", err)
	}
	if total != 0 {
		t.Fatalf("onboardings total=%d", total)
	}

	_, total, err = legacy.FindLifecycleLedger(1, 20, nil)
	if !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindLifecycleLedger empty org err=%v, want ErrMissingOrgID", err)
	}
	if total != 0 {
		t.Fatalf("ledger total=%d", total)
	}
}

func TestEmployeeRepository_FindAllProfilesSubqueryCarriesOrg(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewEmployeeRepositoryWithOrgID(db, "org-a")

	session := db.Session(&gorm.Session{DryRun: true, NewDB: false})
	var sql string
	var vars []interface{}
	name := "employee-profile-sql-" + t.Name()
	_ = session.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		sql = tx.Statement.SQL.String()
		vars = append([]interface{}{}, tx.Statement.Vars...)
	})
	t.Cleanup(func() {
		_ = session.Callback().Query().Remove(name)
	})

	scoped := NewEmployeeRepositoryWithOrgID(session, "org-a")
	_, _, _ = scoped.FindAllProfiles(1, 10, map[string]string{"department_id": "dept-1"})

	lower := strings.ToLower(sql)
	if !strings.Contains(lower, "select user_id from users where org_id") &&
		!strings.Contains(lower, "select user_id from `users` where org_id") &&
		!strings.Contains(lower, "org_id = ?") {
		t.Fatalf("expected users subquery to carry org_id, sql=%s", sql)
	}
	foundOrg := false
	foundDept := false
	for _, v := range vars {
		if s, ok := v.(string); ok {
			if s == "org-a" {
				foundOrg = true
			}
			if s == "dept-1" {
				foundDept = true
			}
		}
	}
	if !foundOrg || !foundDept {
		t.Fatalf("vars missing org/dept: %#v sql=%s", vars, sql)
	}
	// Ensure subquery still requires deleted_at IS NULL.
	if !strings.Contains(lower, "deleted_at is null") {
		t.Fatalf("expected deleted_at IS NULL in users subquery, sql=%s", sql)
	}
	_ = repo // silence if unused in some toolchains
}
