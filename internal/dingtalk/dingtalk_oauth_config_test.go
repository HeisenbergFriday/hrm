package dingtalk

import (
	"strings"
	"testing"

	"peopleops/internal/database"
)

func TestResolveOAuthLoginConfigWithoutDBUsesDefaultConfig(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = originalDB
	})

	t.Setenv("DINGTALK_APP_KEY", "shared-app-key")
	t.Setenv("DINGTALK_APP_SECRET", "shared-app-secret")

	cfg, err := ResolveOAuthLoginConfig("")
	if err != nil {
		t.Fatalf("ResolveOAuthLoginConfig returned error: %v", err)
	}
	if cfg.AppKey != "shared-app-key" {
		t.Fatalf("ResolveOAuthLoginConfig AppKey = %q, want %q", cfg.AppKey, "shared-app-key")
	}
	if cfg.AppSecret != "shared-app-secret" {
		t.Fatalf("ResolveOAuthLoginConfig AppSecret = %q, want %q", cfg.AppSecret, "shared-app-secret")
	}
}

func TestGetQRCodeWithRedirectForConfigUsesProvidedClientID(t *testing.T) {
	t.Parallel()

	loginURL, err := GetQRCodeWithRedirectForConfig(Config{
		OrgID:  "default",
		AppKey: "shared-app-key",
	}, "state-123", "https://peopleops.example.com/callback")
	if err != nil {
		t.Fatalf("GetQRCodeWithRedirectForConfig returned error: %v", err)
	}
	if !strings.Contains(loginURL, "client_id=shared-app-key") {
		t.Fatalf("login url %q does not contain configured client_id", loginURL)
	}
	if !strings.Contains(loginURL, "state=state-123") {
		t.Fatalf("login url %q does not contain state", loginURL)
	}
}

func TestConfigFromOrganizationReadsDingTalkProcessCodes(t *testing.T) {
	cfg := ConfigFromOrganization(database.Organization{
		OrgID:          "muteng",
		DingTalkAppKey: "muteng-key",
		DingTalkSecret: "muteng-secret",
		Extension: map[string]interface{}{
			"dingtalk_process_codes": map[string]interface{}{
				"leave":    "muteng-leave",
				"overtime": "muteng-overtime",
			},
		},
	})

	if cfg.ProcessCodes["leave"] != "muteng-leave" {
		t.Fatalf("leave process code = %q", cfg.ProcessCodes["leave"])
	}
	if cfg.ProcessCodes["overtime"] != "muteng-overtime" {
		t.Fatalf("overtime process code = %q", cfg.ProcessCodes["overtime"])
	}
}

func TestSharedOAuthLoginConfigFromConfigsUsesExplicitSharedOrg(t *testing.T) {
	t.Setenv("DINGTALK_SHARED_OAUTH_ORG_ID", "xiaotie")
	t.Setenv("DINGTALK_APP_KEY", "")
	t.Setenv("DINGTALK_APP_SECRET", "")

	cfg, err := sharedOAuthLoginConfigFromConfigs([]Config{
		{OrgID: "muteng", AppKey: "muteng-key", AppSecret: "muteng-secret"},
		{OrgID: "xiaotie", AppKey: "shared-key", AppSecret: "shared-secret"},
	})
	if err != nil {
		t.Fatalf("sharedOAuthLoginConfigFromConfigs returned error: %v", err)
	}
	if cfg.OrgID != "xiaotie" {
		t.Fatalf("sharedOAuthLoginConfigFromConfigs OrgID = %q, want %q", cfg.OrgID, "xiaotie")
	}
	if cfg.AppKey != "shared-key" || cfg.AppSecret != "shared-secret" {
		t.Fatalf("sharedOAuthLoginConfigFromConfigs returned unexpected credentials: %#v", cfg)
	}
}

func TestSharedOAuthLoginConfigFromConfigsFallsBackToDefaultEnv(t *testing.T) {
	t.Setenv("DINGTALK_SHARED_OAUTH_ORG_ID", "")
	t.Setenv("DINGTALK_APP_KEY", "shared-app-key")
	t.Setenv("DINGTALK_APP_SECRET", "shared-app-secret")

	cfg, err := sharedOAuthLoginConfigFromConfigs([]Config{
		{OrgID: "muteng", AppKey: "muteng-key", AppSecret: "muteng-secret"},
		{OrgID: "xiaotie", AppKey: "xiaotie-key", AppSecret: "xiaotie-secret"},
	})
	if err != nil {
		t.Fatalf("sharedOAuthLoginConfigFromConfigs returned error: %v", err)
	}
	if cfg.AppKey != "shared-app-key" || cfg.AppSecret != "shared-app-secret" {
		t.Fatalf("sharedOAuthLoginConfigFromConfigs returned unexpected default credentials: %#v", cfg)
	}
}

func TestSharedOAuthLoginConfigFromConfigsRequiresSharedConfigWhenOrganizationsDiffer(t *testing.T) {
	t.Setenv("DINGTALK_SHARED_OAUTH_ORG_ID", "")
	t.Setenv("DINGTALK_APP_KEY", "")
	t.Setenv("DINGTALK_APP_SECRET", "")

	_, err := sharedOAuthLoginConfigFromConfigs([]Config{
		{OrgID: "muteng", AppKey: "muteng-key", AppSecret: "muteng-secret"},
		{OrgID: "xiaotie", AppKey: "xiaotie-key", AppSecret: "xiaotie-secret"},
	})
	if err == nil {
		t.Fatal("sharedOAuthLoginConfigFromConfigs returned nil error, want shared config error")
	}
	if !strings.Contains(err.Error(), "shared dingtalk oauth login config is required") {
		t.Fatalf("sharedOAuthLoginConfigFromConfigs error = %q, want shared config hint", err)
	}
}

func TestMergeDingTalkOAuthIdentityFieldsCopiesTokenResponseOrgInfo(t *testing.T) {
	got := mergeDingTalkOAuthIdentityFields(
		map[string]interface{}{
			"nick":    "小铁",
			"unionId": "profile-union",
		},
		map[string]interface{}{
			"corpId":           "ding-corp",
			"associatedUserId": "user-1",
			"unionId":          "token-union",
			"openId":           "open-1",
		},
	)

	if got["corpId"] != "ding-corp" {
		t.Fatalf("merged corpId = %#v, want ding-corp", got["corpId"])
	}
	if got["associatedUserId"] != "user-1" {
		t.Fatalf("merged associatedUserId = %#v, want user-1", got["associatedUserId"])
	}
	if got["unionId"] != "profile-union" {
		t.Fatalf("merged unionId = %#v, want original profile-union", got["unionId"])
	}
	if got["openId"] != "open-1" {
		t.Fatalf("merged openId = %#v, want open-1", got["openId"])
	}
}
