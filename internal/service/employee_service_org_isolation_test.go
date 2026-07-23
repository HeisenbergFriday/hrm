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

func openEmployeeServiceIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:svc-emp-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Department{},
		&database.EmployeeProfile{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	for _, org := range []string{"org-a", "org-b"} {
		if err := db.Create(&database.User{
			OrgID:          org,
			UserID:         "same-user",
			DingTalkUserID: "dt-" + org,
			Name:           "User " + org,
			Email:          "same-user@" + org + ".test",
			Mobile:         "m-" + org,
			DepartmentID:   "same-dept",
			Status:         "active",
		}).Error; err != nil {
			t.Fatalf("seed user: %v", err)
		}
		if err := db.Create(&database.EmployeeProfile{
			OrgID:         org,
			UserID:        "same-user",
			EmployeeID:    "emp-" + org,
			ProfileStatus: "active",
		}).Error; err != nil {
			t.Fatalf("seed profile: %v", err)
		}
	}
	return db
}

func TestEmployeeService_WithOrgIDDepartmentFilterIsolation(t *testing.T) {
	db := openEmployeeServiceIsolationDB(t)

	svcA := NewEmployeeServiceWithOrgID(db, "org-a")
	items, total, err := svcA.GetProfiles(1, 20, map[string]string{"department_id": "same-dept"})
	if err != nil {
		t.Fatalf("GetProfiles: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].OrgID != "org-a" {
		t.Fatalf("org-a isolation failed: total=%d items=%+v", total, items)
	}

	svcB := NewEmployeeServiceWithOrgID(db, "org-b")
	itemsB, totalB, err := svcB.GetProfiles(1, 20, map[string]string{"department_id": "same-dept"})
	if err != nil {
		t.Fatalf("GetProfiles B: %v", err)
	}
	if totalB != 1 || len(itemsB) != 1 || itemsB[0].OrgID != "org-b" {
		t.Fatalf("org-b isolation failed: total=%d items=%+v", totalB, itemsB)
	}
}

func TestEmployeeService_LegacyEmptyOrgFailClosed(t *testing.T) {
	db := openEmployeeServiceIsolationDB(t)
	svc := NewEmployeeService(db)

	_, total, err := svc.GetProfiles(1, 20, map[string]string{"department_id": "same-dept"})
	if !errors.Is(err, repository.ErrMissingOrgID) {
		t.Fatalf("legacy GetProfiles err=%v, want ErrMissingOrgID", err)
	}
	if total != 0 {
		t.Fatalf("legacy total=%d", total)
	}

	_, err = svc.GetProfileByUserID("same-user")
	if !errors.Is(err, repository.ErrMissingOrgID) {
		t.Fatalf("legacy GetProfileByUserID err=%v, want ErrMissingOrgID", err)
	}
}
