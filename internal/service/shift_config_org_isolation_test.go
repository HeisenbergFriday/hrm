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

func openShiftIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:shift-isolation-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.DingTalkShiftCatalog{}, &database.EmployeeShiftConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func tenantShiftService(t *testing.T, root *gorm.DB, orgID string) *ShiftConfigService {
	t.Helper()
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: orgID})
	ctx = requestmeta.WithTenant(ctx, orgID)
	return NewShiftConfigService(root.Session(&gorm.Session{NewDB: true}).WithContext(ctx))
}

func TestShiftIDCacheIsolatesIdenticalShiftKeyAcrossOrgs(t *testing.T) {
	ClearShiftIDCacheForTest()
	t.Cleanup(ClearShiftIDCacheForTest)

	const shiftKey = "晚班|09:00|18:30"
	cacheShiftID("org-a", shiftKey, 1001)
	cacheShiftID("org-b", shiftKey, 2002)

	gotA, okA := getCachedShiftID("org-a", shiftKey)
	gotB, okB := getCachedShiftID("org-b", shiftKey)
	if !okA || !okB {
		t.Fatalf("cache hit missing: okA=%v okB=%v", okA, okB)
	}
	if gotA != 1001 {
		t.Fatalf("org-a shift id = %d, want 1001", gotA)
	}
	if gotB != 2002 {
		t.Fatalf("org-b shift id = %d, want 2002", gotB)
	}
	// Legacy bare key must not hit either org entry.
	if _, ok := getCachedShiftID("", shiftKey); ok {
		t.Fatalf("empty org must not resolve bare shiftKey cache entry")
	}
}

func TestShiftIDCacheClearForTestPreventsCrossTestContamination(t *testing.T) {
	ClearShiftIDCacheForTest()
	cacheShiftID("org-a", "k|09:00|18:00", 11)
	if _, ok := getCachedShiftID("org-a", "k|09:00|18:00"); !ok {
		t.Fatal("expected cache hit before clear")
	}
	ClearShiftIDCacheForTest()
	if _, ok := getCachedShiftID("org-a", "k|09:00|18:00"); ok {
		t.Fatal("expected empty cache after ClearShiftIDCacheForTest")
	}
}

func TestGetOrCreateShiftFailClosedWithoutOrgContext(t *testing.T) {
	ClearShiftIDCacheForTest()
	t.Cleanup(ClearShiftIDCacheForTest)

	db := openShiftIsolationDB(t)
	svc := NewShiftConfigService(db) // no RequestInfo / Tenant on context

	_, err := svc.GetOrCreateShift("晚班", "09:00", "18:30")
	if err == nil {
		t.Fatal("expected missing organization context error")
	}
	if !strings.Contains(err.Error(), "missing organization context") {
		t.Fatalf("err = %v, want missing organization context", err)
	}
}

func TestGetOrCreateShiftFailClosedWithoutAdminUserIDForNonDefaultOrg(t *testing.T) {
	ClearShiftIDCacheForTest()
	t.Cleanup(ClearShiftIDCacheForTest)

	originalDB := database.DB
	t.Cleanup(func() { database.DB = originalDB })

	db := openShiftIsolationDB(t)
	if err := db.AutoMigrate(&database.Organization{}); err != nil {
		t.Fatalf("migrate org: %v", err)
	}
	database.DB = db

	// Active non-default org without DingTalkAdminUserID.
	if err := db.Create(&database.Organization{
		OrgID:          "org-b",
		Name:           "Org B",
		CorpID:         "corp-b-" + t.Name(),
		DingTalkAppKey: "b-key",
		DingTalkSecret: "b-secret",
		Status:         "active",
	}).Error; err != nil {
		t.Fatalf("seed org: %v", err)
	}

	// Pollution trap: global env must not be used for non-default org.
	t.Setenv("DINGTALK_ADMIN_USER_ID", "default-admin-should-not-leak")

	svc := tenantShiftService(t, db, "org-b")
	_, err := svc.GetOrCreateShift("晚班", "09:00", "18:30")
	if err == nil {
		t.Fatal("expected missing admin user id error for org-b")
	}
	if !strings.Contains(err.Error(), "org-b") && !strings.Contains(err.Error(), "admin user id") {
		t.Fatalf("err = %v, want org-scoped admin missing message", err)
	}
}

func TestPersistShiftIDCatalogIsolatesByOrg(t *testing.T) {
	ClearShiftIDCacheForTest()
	t.Cleanup(ClearShiftIDCacheForTest)

	db := openShiftIsolationDB(t)
	svcA := tenantShiftService(t, db, "org-a")
	svcB := tenantShiftService(t, db, "org-b")

	const shiftKey = "标准班|09:00|18:30"
	if err := svcA.persistShiftID("org-a", "标准班", shiftKey, 111, "09:00", "18:30"); err != nil {
		t.Fatalf("persist A: %v", err)
	}
	if err := svcB.persistShiftID("org-b", "标准班", shiftKey, 222, "09:00", "18:30"); err != nil {
		t.Fatalf("persist B: %v", err)
	}

	idA, err := svcA.getPersistedShiftID("org-a", shiftKey)
	if err != nil {
		t.Fatalf("get A: %v", err)
	}
	idB, err := svcB.getPersistedShiftID("org-b", shiftKey)
	if err != nil {
		t.Fatalf("get B: %v", err)
	}
	if idA != 111 || idB != 222 {
		t.Fatalf("persisted ids A=%d B=%d, want 111/222", idA, idB)
	}
}

func TestSetConfigsFailClosedWithoutOrgContext(t *testing.T) {
	db := openShiftIsolationDB(t)
	svc := NewShiftConfigService(db)
	_, err := svc.SetConfigs(&SetShiftConfigInput{
		UserIDs: []string{"u1"},
		ShiftID: 1,
		EndTime: "18:30",
	})
	if err == nil || !strings.Contains(err.Error(), "missing organization context") {
		t.Fatalf("err = %v, want missing organization context", err)
	}
}

func TestGetAllWithUsersFailClosedWithoutOrgContext(t *testing.T) {
	db := openShiftIsolationDB(t)
	if err := db.AutoMigrate(&database.User{}, &database.Department{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Seed a row that must never be returned without org binding.
	if err := db.Create(&database.User{OrgID: "org-a", UserID: "u1", Name: "Alice", Status: "active"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	svc := NewShiftConfigService(db)
	items, err := svc.GetAllWithUsers()
	if err == nil {
		t.Fatalf("expected missing org error, got items=%d", len(items))
	}
	if !strings.Contains(err.Error(), "missing organization context") && !strings.Contains(err.Error(), "orgID required") {
		t.Fatalf("err = %v, want missing organization context", err)
	}
}

func TestGetAllWithUsersScopesByOrg(t *testing.T) {
	db := openShiftIsolationDB(t)
	if err := db.AutoMigrate(&database.User{}, &database.Department{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&database.User{OrgID: "org-a", UserID: "u-a", Name: "Alice", Status: "active"}).Error; err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := db.Create(&database.User{OrgID: "org-b", UserID: "u-b", Name: "Bob", Status: "active"}).Error; err != nil {
		t.Fatalf("seed b: %v", err)
	}
	svc := NewShiftConfigServiceWithOrgID(db, "org-a")
	items, err := svc.GetAllWithUsers()
	if err != nil {
		t.Fatalf("GetAllWithUsers: %v", err)
	}
	if len(items) != 1 || items[0].UserID != "u-a" {
		t.Fatalf("items = %#v, want only u-a", items)
	}
}

func TestListShiftCatalogsFailClosedWithoutOrg(t *testing.T) {
	db := openShiftIsolationDB(t)
	svc := NewShiftConfigService(db)
	_, err := svc.ListShiftCatalogs()
	if err == nil {
		t.Fatal("expected missing org error")
	}
}
