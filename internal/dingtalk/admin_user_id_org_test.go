package dingtalk

import (
	"strings"
	"testing"

	"peopleops/internal/database"
)

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
	cfgA := ConfigFromOrganization(database.Organization{OrgID: "org-a", DingTalkAdminUserID: "admin-a"})
	cfgB := ConfigFromOrganization(database.Organization{OrgID: "org-b", DingTalkAdminUserID: "admin-b"})
	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-default-admin")

	idA, err := ResolveAdminUserIDFromConfig(cfgA)
	if err != nil {
		t.Fatalf("org-a: %v", err)
	}
	idB, err := ResolveAdminUserIDFromConfig(cfgB)
	if err != nil {
		t.Fatalf("org-b: %v", err)
	}
	if idA != "admin-a" || idB != "admin-b" {
		t.Fatalf("ids A=%q B=%q, want admin-a/admin-b (not env)", idA, idB)
	}
}

func TestResolveAdminUserID_NonDefaultMissingAdminFailsClosed(t *testing.T) {
	cfg := ConfigFromOrganization(database.Organization{OrgID: "org-c"})
	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-default-admin")

	_, err := ResolveAdminUserIDFromConfig(cfg)
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

	cfgDefault := ConfigFromOrganization(database.Organization{OrgID: database.DefaultOrganizationID})
	if cfgDefault.AdminUserID != "env-admin" {
		t.Fatalf("default AdminUserID = %q, want env-admin", cfgDefault.AdminUserID)
	}

	cfgOther := ConfigFromOrganization(database.Organization{OrgID: "muteng"})
	if cfgOther.AdminUserID != "" {
		t.Fatalf("non-default empty AdminUserID = %q, want empty (no env fallback)", cfgOther.AdminUserID)
	}

	cfgFilled := ConfigFromOrganization(database.Organization{OrgID: "muteng", DingTalkAdminUserID: "muteng-admin"})
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

func TestGetScheduleListBatchByDay_DefaultOnlyEntry(t *testing.T) {
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
