package service

import (
	"context"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/requestmeta"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openWeekIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:week-isolation-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Organization{},
		&database.WeekScheduleRule{},
		&database.WeekScheduleSyncLog{},
		&database.StatutoryHoliday{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func tenantWeekService(t *testing.T, root *gorm.DB, orgID string) *WeekScheduleService {
	t.Helper()
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: orgID})
	ctx = requestmeta.WithTenant(ctx, orgID)
	return NewWeekScheduleService(root.Session(&gorm.Session{NewDB: true}).WithContext(ctx))
}

func TestSyncToDingTalkFailClosedWithoutOrgContext(t *testing.T) {
	db := openWeekIsolationDB(t)
	svc := NewWeekScheduleService(db)
	_, err := svc.SyncToDingTalk(1)
	if err == nil || !strings.Contains(err.Error(), "missing organization context") {
		t.Fatalf("err = %v, want missing organization context", err)
	}
}

func TestSyncFromDingTalkFailClosedWithoutAdminForNonDefaultOrg(t *testing.T) {
	originalDB := database.DB
	t.Cleanup(func() { database.DB = originalDB })

	db := openWeekIsolationDB(t)
	database.DB = db

	if err := db.Create(&database.Organization{
		OrgID:          "org-b",
		Name:           "Org B",
		CorpID:         "corp-b-from-" + t.Name(),
		DingTalkAppKey: "b-key",
		DingTalkSecret: "b-secret",
		Status:         "active",
	}).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if err := db.Create(&database.User{
		OrgID:          "org-b",
		UserID:         "u-b-1",
		DingTalkUserID: "dt-org-b-u-b-1",
		Name:           "B User",
		Status:         "active",
		Email:          "u-b-1@org-b.test",
		Mobile:         "m-org-b-u-b-1",
		DepartmentID:   "dept-1",
	}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	// Global env must not authorize non-default org sync.
	t.Setenv("DINGTALK_ADMIN_USER_ID", "default-admin")

	svc := tenantWeekService(t, db, "org-b")
	_, err := svc.SyncFromDingTalk()
	if err == nil {
		t.Fatal("expected missing admin error before any write")
	}
	if !strings.Contains(err.Error(), "org-b") && !strings.Contains(err.Error(), "admin") {
		t.Fatalf("err = %v, want org-scoped admin error", err)
	}

	var logCount int64
	if err := db.Model(&database.WeekScheduleSyncLog{}).Count(&logCount).Error; err != nil {
		t.Fatalf("count sync logs: %v", err)
	}
	if logCount != 0 {
		t.Fatalf("sync log count = %d, want 0 (no DB write on missing config)", logCount)
	}

	var ruleCount int64
	if err := db.Model(&database.WeekScheduleRule{}).Count(&ruleCount).Error; err != nil {
		t.Fatalf("count rules: %v", err)
	}
	if ruleCount != 0 {
		t.Fatalf("rule count = %d, want 0", ruleCount)
	}
}

func TestSyncToDingTalkFailClosedWithoutAdminForNonDefaultOrg(t *testing.T) {
	originalDB := database.DB
	t.Cleanup(func() { database.DB = originalDB })

	db := openWeekIsolationDB(t)
	database.DB = db

	if err := db.Create(&database.Organization{
		OrgID:          "org-b",
		Name:           "Org B",
		CorpID:         "corp-b-to-" + t.Name(),
		DingTalkAppKey: "b-key",
		DingTalkSecret: "b-secret",
		Status:         "active",
	}).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}
	t.Setenv("DINGTALK_ADMIN_USER_ID", "default-admin")

	svc := tenantWeekService(t, db, "org-b")
	_, err := svc.SyncToDingTalk(1)
	if err == nil {
		t.Fatal("expected missing admin error")
	}
	if !strings.Contains(err.Error(), "org-b") && !strings.Contains(err.Error(), "admin") {
		t.Fatalf("err = %v, want org-scoped admin error", err)
	}
}

func TestPushPersonalScheduleImageFailClosedWithoutOrg(t *testing.T) {
	db := openWeekIsolationDB(t)
	svc := NewWeekScheduleService(db)
	_, err := svc.PushPersonalScheduleImage([]string{"u1"}, "title", "content", []byte{1, 2, 3}, "x.png")
	if err == nil || !strings.Contains(err.Error(), "missing organization context") {
		t.Fatalf("err = %v, want missing organization context", err)
	}
}

func TestPushPersonalScheduleImageRejectsEmptyImageAndUsers(t *testing.T) {
	db := openWeekIsolationDB(t)
	svc := NewWeekScheduleServiceWithOrgID(db, "org-a")

	if _, err := svc.PushPersonalScheduleImage([]string{"u1"}, "t", "c", nil, "x.png"); err == nil || !strings.Contains(err.Error(), "image is empty") {
		t.Fatalf("empty image err = %v", err)
	}
	if _, err := svc.PushPersonalScheduleImage(nil, "t", "c", []byte{1}, "x.png"); err == nil || !strings.Contains(err.Error(), "user_ids is empty") {
		t.Fatalf("empty users err = %v", err)
	}
}
