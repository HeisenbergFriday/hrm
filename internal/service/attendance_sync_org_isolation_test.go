package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"peopleops/internal/requestmeta"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openAttendanceSyncIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:att-sync-iso-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Attendance{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func attendanceServiceWithOrg(t *testing.T, db *gorm.DB, orgID string) *AttendanceService {
	t.Helper()
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: orgID})
	ctx = requestmeta.WithTenant(ctx, orgID)
	tenantDB := db.WithContext(ctx)
	return NewAttendanceService(tenantDB)
}

func TestAttendanceService_SyncRecordsEmptyOrgFailsClosed(t *testing.T) {
	db := openAttendanceSyncIsolationDB(t)
	svc := NewAttendanceService(db)

	n, err := svc.SyncRecords("", []dingtalk.AttendanceRecord{{
		UserID:        "u1",
		UserCheckTime: time.Now().Format("2006-01-02 15:04:05"),
		CheckType:     "OnDuty",
	}}, map[string]string{"u1": "Alice"})
	if !errors.Is(err, repository.ErrMissingOrgID) {
		t.Fatalf("empty org err = %v, want ErrMissingOrgID", err)
	}
	if n != 0 {
		t.Fatalf("count = %d, want 0", n)
	}

	var total int64
	if err := db.Model(&database.Attendance{}).Count(&total).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 0 {
		t.Fatalf("empty org wrote %d attendance rows (must not invent default)", total)
	}
}

func TestAttendanceService_SyncRecordsWritesOnlyTargetOrg(t *testing.T) {
	db := openAttendanceSyncIsolationDB(t)
	// Seed a foreign row that must remain untouched.
	foreignTime := time.Date(2026, 1, 1, 9, 0, 0, 0, time.FixedZone("CST", 8*3600))
	if err := db.Create(&database.Attendance{
		OrgID: "org-b", UserID: "u1", UserName: "Bob",
		CheckTime: foreignTime, CheckType: "上班",
	}).Error; err != nil {
		t.Fatalf("seed foreign: %v", err)
	}

	svc := attendanceServiceWithOrg(t, db, "org-a")
	checkTime := "2026-01-02 09:00:00"
	n, err := svc.SyncRecords("org-a", []dingtalk.AttendanceRecord{{
		UserID:         "u1",
		UserCheckTime:  checkTime,
		CheckType:      "OnDuty",
		LocationResult: "Normal",
	}}, map[string]string{"u1": "Alice"})
	if err != nil {
		t.Fatalf("SyncRecords org-a: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}

	var aRows, bRows int64
	_ = db.Model(&database.Attendance{}).Where("org_id = ?", "org-a").Count(&aRows)
	_ = db.Model(&database.Attendance{}).Where("org_id = ?", "org-b").Count(&bRows)
	if aRows != 1 {
		t.Fatalf("org-a rows = %d, want 1", aRows)
	}
	if bRows != 1 {
		t.Fatalf("org-b rows = %d, want 1 (foreign must remain)", bRows)
	}

	// Sync for org-a must not rewrite org-b even with same user/time payload.
	var foreign database.Attendance
	if err := db.Where("org_id = ?", "org-b").First(&foreign).Error; err != nil {
		t.Fatalf("reload foreign: %v", err)
	}
	if foreign.UserName != "Bob" {
		t.Fatalf("foreign row mutated: %#v", foreign)
	}
}

func TestAttendanceService_SyncRecordsRejectsWritingIntoUnboundServiceDefault(t *testing.T) {
	// Explicit non-empty org is required; "default" is only valid when the caller
	// intentionally passes it — empty must never become default.
	db := openAttendanceSyncIsolationDB(t)
	svc := NewAttendanceService(db)
	_, err := svc.SyncRecords("   ", nil, nil)
	if !errors.Is(err, repository.ErrMissingOrgID) {
		t.Fatalf("whitespace org err = %v, want ErrMissingOrgID", err)
	}
	var total int64
	_ = db.Model(&database.Attendance{}).Where("org_id = ?", database.DefaultOrganizationID).Count(&total)
	if total != 0 {
		t.Fatalf("unexpected default-org rows: %d", total)
	}
}

func TestAttendanceLoadUsersFailClosedWithoutOrg(t *testing.T) {
	db := openAttendanceSyncIsolationDB(t)
	if err := db.AutoMigrate(&database.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	if err := db.Create(&database.User{OrgID: "org-a", UserID: "u1", Name: "Alice", Status: "active"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := NewAttendanceService(db) // no org binding
	users, err := svc.loadUsers(map[string]string{})
	if err == nil {
		t.Fatalf("expected ErrMissingOrgID, got users=%d", len(users))
	}
	if !errors.Is(err, repository.ErrMissingOrgID) {
		t.Fatalf("err = %v, want ErrMissingOrgID", err)
	}
}

func TestAttendanceLoadUsersScopesByOrg(t *testing.T) {
	db := openAttendanceSyncIsolationDB(t)
	if err := db.AutoMigrate(&database.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	if err := db.Create(&database.User{OrgID: "org-a", UserID: "u-a", Name: "Alice", Status: "active"}).Error; err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := db.Create(&database.User{OrgID: "org-b", UserID: "u-b", Name: "Bob", Status: "active"}).Error; err != nil {
		t.Fatalf("seed b: %v", err)
	}
	svc := NewAttendanceServiceWithOrgID(db, "org-a")
	users, err := svc.loadUsers(map[string]string{})
	if err != nil {
		t.Fatalf("loadUsers: %v", err)
	}
	if len(users) != 1 || users[0].UserID != "u-a" {
		t.Fatalf("users = %#v, want only u-a", users)
	}
}

func TestAttendanceLoadDepartmentNamesFailClosedWithoutOrg(t *testing.T) {
	db := openAttendanceSyncIsolationDB(t)
	if err := db.AutoMigrate(&database.Department{}); err != nil {
		t.Fatalf("migrate depts: %v", err)
	}
	svc := NewAttendanceService(db)
	_, err := svc.loadDepartmentNames()
	if !errors.Is(err, repository.ErrMissingOrgID) {
		t.Fatalf("err = %v, want ErrMissingOrgID", err)
	}
}
