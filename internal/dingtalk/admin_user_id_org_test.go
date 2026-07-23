package dingtalk

import (
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openAdminOrgDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:admin-org-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Organization{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestResolveAdminUserID_DefaultMayUseEnv(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = originalDB })

	t.Setenv("DINGTALK_ADMIN_USER_ID", "default-admin")
	id, err := ResolveAdminUserID(database.DefaultOrganizationID)
	if err != nil {
		t.Fatalf("ResolveAdminUserID default: %v", err)
	}
	if id != "default-admin" {
		t.Fatalf("id = %q, want default-admin", id)
	}
}

func TestResolveAdminUserID_NonDefaultUsesOrgFieldNotGlobalEnv(t *testing.T) {
	originalDB := database.DB
	t.Cleanup(func() { database.DB = originalDB })

	db := openAdminOrgDB(t)
	database.DB = db

	if err := db.Create(&database.Organization{
		OrgID:               "org-a",
		Name:                "Org A",
		CorpID:              "corp-a",
		DingTalkAppKey:      "a-key",
		DingTalkSecret:      "a-secret",
		DingTalkAdminUserID: "admin-a",
		Status:              "active",
	}).Error; err != nil {
		t.Fatalf("seed org-a: %v", err)
	}
	if err := db.Create(&database.Organization{
		OrgID:               "org-b",
		Name:                "Org B",
		CorpID:              "corp-b",
		DingTalkAppKey:      "b-key",
		DingTalkSecret:      "b-secret",
		DingTalkAdminUserID: "admin-b",
		Status:              "active",
	}).Error; err != nil {
		t.Fatalf("seed org-b: %v", err)
	}

	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-default-admin")

	idA, err := ResolveAdminUserID("org-a")
	if err != nil {
		t.Fatalf("org-a: %v", err)
	}
	idB, err := ResolveAdminUserID("org-b")
	if err != nil {
		t.Fatalf("org-b: %v", err)
	}
	if idA != "admin-a" || idB != "admin-b" {
		t.Fatalf("ids A=%q B=%q, want admin-a/admin-b (not env)", idA, idB)
	}
}

func TestResolveAdminUserID_NonDefaultMissingAdminFailsClosed(t *testing.T) {
	originalDB := database.DB
	t.Cleanup(func() { database.DB = originalDB })

	db := openAdminOrgDB(t)
	database.DB = db

	if err := db.Create(&database.Organization{
		OrgID:          "org-c",
		Name:           "Org C",
		CorpID:         "corp-c",
		DingTalkAppKey: "c-key",
		DingTalkSecret: "c-secret",
		Status:         "active",
	}).Error; err != nil {
		t.Fatalf("seed org-c: %v", err)
	}

	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-default-admin")

	_, err := ResolveAdminUserID("org-c")
	if err == nil {
		t.Fatal("expected error for missing enterprise admin")
	}
	if !strings.Contains(err.Error(), "org-c") {
		t.Fatalf("err = %v, want org-c in message", err)
	}
	if strings.Contains(err.Error(), "env-default-admin") {
		t.Fatalf("err unexpectedly mentions env admin: %v", err)
	}
}

func TestResolveAdminUserID_EmptyOrgFailsClosed(t *testing.T) {
	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-default-admin")

	_, err := ResolveAdminUserIDFromConfig(Config{})
	if err == nil {
		t.Fatal("expected empty org to fail closed")
	}
	if !strings.Contains(err.Error(), "organization is required") {
		t.Fatalf("err = %v, want missing organization error", err)
	}
}

func TestAdminRequiredOperations_NonDefaultNeverUseGlobalEnv(t *testing.T) {
	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-default-admin")
	cfg := Config{OrgID: "org-without-admin"}

	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "add compensatory quota",
			run: func() error {
				return updateCompensatoryLeaveQuotaWithConfig(cfg, "u1", 60, "2026-07-18", "security test")
			},
		},
		{
			name: "set compensatory quota",
			run: func() error {
				return setCompensatoryLeaveQuotaWithConfig(cfg, "u1", 2026, 60, "security test")
			},
		},
		{
			name: "initialize vacation quota",
			run: func() error {
				return initVacationQuotaWithConfig(cfg, "u1", "leave-code", 2026, 0, 0, "security test")
			},
		},
		{
			name: "read schedules in batch",
			run: func() error {
				_, err := getScheduleListBatchByDayWithConfig(cfg, []string{"u1"}, "2026-07-18")
				return err
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected missing organization admin error")
			}
			if !strings.Contains(err.Error(), "org-without-admin") {
				t.Fatalf("err = %v, want organization id", err)
			}
		})
	}
}

func TestConfigFromOrganization_AdminUserIDAndDefaultEnvFallback(t *testing.T) {
	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-admin")

	cfgDefault := ConfigFromOrganization(database.Organization{
		OrgID: database.DefaultOrganizationID,
		Name:  "Default",
	})
	if cfgDefault.AdminUserID != "env-admin" {
		t.Fatalf("default AdminUserID = %q, want env-admin", cfgDefault.AdminUserID)
	}

	cfgOther := ConfigFromOrganization(database.Organization{
		OrgID: "muteng",
		Name:  "沐腾",
	})
	if cfgOther.AdminUserID != "" {
		t.Fatalf("non-default empty AdminUserID = %q, want empty (no env fallback)", cfgOther.AdminUserID)
	}

	cfgFilled := ConfigFromOrganization(database.Organization{
		OrgID:               "muteng",
		DingTalkAdminUserID: "muteng-admin",
	})
	if cfgFilled.AdminUserID != "muteng-admin" {
		t.Fatalf("filled AdminUserID = %q", cfgFilled.AdminUserID)
	}
}

func TestAppConfigRoundTripPreservesOrganizationAdmin(t *testing.T) {
	original := Config{OrgID: "org-a", AppKey: "key-a", AdminUserID: "admin-a"}
	appCfg := appConfigFromConfig(original)
	if appCfg.AdminUserID != "admin-a" {
		t.Fatalf("AppConfig AdminUserID = %q, want admin-a", appCfg.AdminUserID)
	}

	roundTripped := configFromAppConfig(appCfg)
	if roundTripped.OrgID != "org-a" || roundTripped.AdminUserID != "admin-a" {
		t.Fatalf("round trip config = %#v", roundTripped)
	}
}

func TestGetScheduleListBatchByDayForOrg_MissingConfigFailsWithoutWrite(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = originalDB })

	_, err := GetScheduleListBatchByDayForOrg("org-missing", []string{"u1"}, "2026-07-18")
	if err == nil {
		t.Fatal("expected config error")
	}
	if !strings.Contains(err.Error(), "org-missing") && !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v, want missing org config", err)
	}
}

func TestGetScheduleListBatchByDay_DefaultOnlyEntry(t *testing.T) {
	// Compatibility wrapper must pin default org; non-default callers use ForOrg.
	// With no DB/config this still fails closed on default credentials rather than
	// silently using another enterprise.
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = originalDB })
	t.Setenv("DINGTALK_ADMIN_USER_ID", "")
	t.Setenv("DINGTALK_APP_KEY", "")
	t.Setenv("DINGTALK_APP_SECRET", "")

	_, err := GetScheduleListBatchByDay([]string{"u1"}, "2026-07-18")
	if err == nil {
		t.Fatal("expected error without default credentials/admin")
	}
}
