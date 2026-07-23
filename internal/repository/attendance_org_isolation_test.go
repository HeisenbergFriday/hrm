package repository

import (
	"errors"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openAttendanceIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:att-iso-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&database.Attendance{}, &database.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestAttendanceRepository_FindAllIgnoresFiltersOrgID(t *testing.T) {
	db := openAttendanceIsolationDB(t)
	now := time.Now()
	if err := db.Create(&database.Attendance{OrgID: "org-a", UserID: "u1", CheckTime: now, CheckType: "OnDuty"}).Error; err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := db.Create(&database.Attendance{OrgID: "org-b", UserID: "u1", CheckTime: now.Add(time.Minute), CheckType: "OnDuty"}).Error; err != nil {
		t.Fatalf("seed b: %v", err)
	}

	repo := NewAttendanceRepositoryWithOrgID(db, "org-a")
	// Malicious filter must not switch tenant.
	rows, total, err := repo.FindAll(1, 50, map[string]string{"org_id": "org-b"})
	if err != nil {
		t.Fatalf("FindAll: %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].OrgID != "org-a" {
		t.Fatalf("got total=%d rows=%#v, want only org-a", total, rows)
	}
}

func TestAttendanceRepository_EmptyOrgFailClosed(t *testing.T) {
	db := openAttendanceIsolationDB(t)
	repo := NewAttendanceRepository(db) // no request org context
	err := repo.Upsert(&database.Attendance{
		UserID: "u1", CheckTime: time.Now(), CheckType: "OnDuty", OrgID: "org-a",
	})
	if !isMissingOrgErr(err) {
		t.Fatalf("Upsert err=%v, want missing org", err)
	}
	_, _, err = repo.FindAll(1, 10, map[string]string{"org_id": "org-a"})
	if !isMissingOrgErr(err) {
		t.Fatalf("FindAll err=%v, want missing org", err)
	}
}

func isMissingOrgErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrMissingOrgID) {
		return true
	}
	return strings.Contains(err.Error(), "missing organization") || strings.Contains(err.Error(), "orgID required")
}

func TestAttendanceRepository_RejectsCrossOrgUpsert(t *testing.T) {
	db := openAttendanceIsolationDB(t)
	repo := NewAttendanceRepositoryWithOrgID(db, "org-a")
	err := repo.Upsert(&database.Attendance{
		OrgID: "org-b", UserID: "u1", CheckTime: time.Now(), CheckType: "OnDuty",
	})
	if !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("err=%v, want ErrOrgMismatch", err)
	}
}
