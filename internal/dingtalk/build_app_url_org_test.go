package dingtalk

import (
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestBuildAppURLForOrg(t *testing.T) {
	if got := BuildAppURLForOrg("", "/performance"); got != "" {
		t.Fatalf("empty org should fail closed, got %q", got)
	}

	// Non-default org without DB config must not use global env home.
	t.Setenv("DINGTALK_APP_HOME_URL", "https://global.example")
	if got := BuildAppURLForOrg("tenant-x", "/path"); got != "" {
		t.Fatalf("unconfigured tenant-x must not reuse global home, got %q", got)
	}

	// Explicit default may use process-wide env fallbacks.
	want := "https://global.example/path"
	if got := BuildAppURLForOrg(database.DefaultOrganizationID, "/path"); got != want {
		t.Fatalf("default org = %q, want %q", got, want)
	}

	// Absolute URLs pass through once org is present.
	if got := BuildAppURLForOrg(database.DefaultOrganizationID, "https://abs.example/x"); got != "https://abs.example/x" {
		t.Fatalf("absolute = %q", got)
	}
}

func TestGetConfiguredAppHomeURLForOrgDualTenant(t *testing.T) {
	originalDB := database.DB
	t.Cleanup(func() { database.DB = originalDB })

	db, err := gorm.Open(sqlite.Open("file:ding-home-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Organization{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&database.Organization{
		OrgID: "org-a", Name: "A", CorpID: "corp-a", Status: "active",
		DingTalkAppKey: "key-a", DingTalkSecret: "sec-a", DingTalkAgentID: "1001",
		AppHomeURL: "https://a.example/app",
	}).Error; err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := db.Create(&database.Organization{
		OrgID: "org-b", Name: "B", CorpID: "corp-b", Status: "active",
		DingTalkAppKey: "key-b", DingTalkSecret: "sec-b", DingTalkAgentID: "2002",
		AppHomeURL: "https://b.example/app",
	}).Error; err != nil {
		t.Fatalf("seed b: %v", err)
	}
	database.DB = db

	t.Setenv("DINGTALK_APP_HOME_URL", "https://default.example/app")
	if got := GetConfiguredAppHomeURLForOrg("org-a"); got != "https://a.example/app" {
		t.Fatalf("org-a home = %q", got)
	}
	if got := GetConfiguredAppHomeURLForOrg("org-b"); got != "https://b.example/app" {
		t.Fatalf("org-b home = %q", got)
	}
	if got := BuildAppURLForOrg("org-b", "/performance-self-eval/1/2"); got != "https://b.example/app/performance-self-eval/1/2" {
		t.Fatalf("org-b url = %q", got)
	}
	// org-b must not reuse default/global home.
	if strings.Contains(BuildAppURLForOrg("org-b", "/x"), "default.example") {
		t.Fatalf("org-b leaked default home")
	}
	if got := GetConfiguredAppHomeURLForOrg(""); got != "" {
		t.Fatalf("empty org home must fail closed, got %q", got)
	}
}

func TestConfigForOrgIDSelectsTenantCredentials(t *testing.T) {
	originalDB := database.DB
	t.Cleanup(func() { database.DB = originalDB })

	db, err := gorm.Open(sqlite.Open("file:ding-cfg-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Organization{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Setenv("DINGTALK_ADMIN_USER_ID", "env-default-admin")
	if err := db.Create(&database.Organization{
		OrgID: "org-a", Name: "A", CorpID: "corp-a", Status: "active",
		DingTalkAppKey: "key-a", DingTalkSecret: "sec-a", DingTalkAgentID: "1001",
		DingTalkAdminUserID: "admin-a",
		AppHomeURL:          "https://a.example/app",
	}).Error; err != nil {
		t.Fatalf("seed a: %v", err)
	}
	if err := db.Create(&database.Organization{
		OrgID: "org-b", Name: "B", CorpID: "corp-b", Status: "active",
		DingTalkAppKey: "key-b", DingTalkSecret: "sec-b", DingTalkAgentID: "2002",
		DingTalkAdminUserID: "admin-b",
		AppHomeURL:          "https://b.example/app",
	}).Error; err != nil {
		t.Fatalf("seed b: %v", err)
	}
	database.DB = db

	cfgA, err := ConfigForOrgID("org-a")
	if err != nil {
		t.Fatalf("cfg a: %v", err)
	}
	cfgB, err := ConfigForOrgID("org-b")
	if err != nil {
		t.Fatalf("cfg b: %v", err)
	}
	if cfgA.AppKey != "key-a" || cfgA.AgentID != "1001" || cfgA.AdminUserID != "admin-a" {
		t.Fatalf("cfgA = %#v", cfgA)
	}
	if cfgB.AppKey != "key-b" || cfgB.AgentID != "2002" || cfgB.AdminUserID != "admin-b" {
		t.Fatalf("cfgB = %#v", cfgB)
	}
	if cfgB.AppKey == cfgA.AppKey {
		t.Fatal("org-b must not reuse org-a AppKey")
	}
	if cfgA.AdminUserID == "env-default-admin" || cfgB.AdminUserID == "env-default-admin" {
		t.Fatal("non-default org must not use global DINGTALK_ADMIN_USER_ID")
	}
	if cfgA.AdminUserID == cfgB.AdminUserID {
		t.Fatal("org-a and org-b must keep distinct AdminUserID")
	}
}

func TestSendCorpActionCardToUserForOrgFailClosed(t *testing.T) {
	err := SendCorpActionCardToUserForOrg("", "user-1", "t", "c", "a", "https://example/x")
	if err == nil || !strings.Contains(err.Error(), "orgID is empty") {
		t.Fatalf("empty org err = %v, want orgID is empty", err)
	}
	// Unconfigured non-default must not fall back to default credentials.
	err = SendCorpActionCardToUserForOrg("missing-org", "user-1", "t", "c", "a", "https://example/x")
	if err == nil {
		t.Fatal("missing org config should fail")
	}
}
