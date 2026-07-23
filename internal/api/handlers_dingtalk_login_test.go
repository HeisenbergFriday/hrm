package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGenerateAndValidateLoginStateKeepsUnscopedQRLogin(t *testing.T) {
	t.Parallel()

	state := generateLoginState("")
	orgID, ok := validateLoginState(state)
	if !ok {
		t.Fatalf("validateLoginState(%q) returned invalid state", state)
	}
	if orgID != "" {
		t.Fatalf("validateLoginState(%q) orgID = %q, want empty org", state, orgID)
	}
}

func TestGenerateAndValidateLoginStateKeepsOAuthOrgID(t *testing.T) {
	t.Parallel()

	state := generateLoginStateWithOAuthOrgID(" xiaotie ", " default ")
	entry, ok := validateLoginStateEntry(state)
	if !ok {
		t.Fatalf("validateLoginStateEntry(%q) returned invalid state", state)
	}
	if entry.OrgID != "xiaotie" {
		t.Fatalf("validateLoginStateEntry(%q) OrgID = %q, want xiaotie", state, entry.OrgID)
	}
	if entry.OAuthOrgID != "default" {
		t.Fatalf("validateLoginStateEntry(%q) OAuthOrgID = %q, want default", state, entry.OAuthOrgID)
	}
}

func TestUniqueNormalizedOrgIDsSkipsEmptyAndDeduplicates(t *testing.T) {
	t.Parallel()

	got := uniqueNormalizedOrgIDs("", " default ", "default", "muteng")
	if len(got) != 2 {
		t.Fatalf("uniqueNormalizedOrgIDs returned %d items, want 2: %#v", len(got), got)
	}
	if got[0] != "default" || got[1] != "muteng" {
		t.Fatalf("uniqueNormalizedOrgIDs = %#v, want [default muteng]", got)
	}
}

func TestResolveRequestedDingTalkCallbackOrgIDPrefersStateOrg(t *testing.T) {
	t.Parallel()

	got, err := resolveRequestedDingTalkCallbackOrgID(" xiaotie ", "xiaotie")
	if err != nil {
		t.Fatalf("resolveRequestedDingTalkCallbackOrgID returned error: %v", err)
	}
	if got != "xiaotie" {
		t.Fatalf("resolveRequestedDingTalkCallbackOrgID = %q, want xiaotie", got)
	}
}

func TestResolveRequestedDingTalkCallbackOrgIDFallsBackToRequestOrg(t *testing.T) {
	t.Parallel()

	got, err := resolveRequestedDingTalkCallbackOrgID("", " default ")
	if err != nil {
		t.Fatalf("resolveRequestedDingTalkCallbackOrgID returned error: %v", err)
	}
	if got != "default" {
		t.Fatalf("resolveRequestedDingTalkCallbackOrgID = %q, want default", got)
	}
}

func TestResolveRequestedDingTalkCallbackOrgIDRejectsMismatch(t *testing.T) {
	t.Parallel()

	if _, err := resolveRequestedDingTalkCallbackOrgID("xiaotie", "default"); err == nil {
		t.Fatal("resolveRequestedDingTalkCallbackOrgID() error = nil, want mismatch error")
	}
}

func TestValidateDingTalkSelectedOrgIdentityAllowsMatchingCorpID(t *testing.T) {
	t.Setenv("DINGTALK_CORP_ID", "ding123")

	err := validateDingTalkSelectedOrgIdentity(" default ", map[string]interface{}{
		"corpId": "ding123",
	})
	if err != nil {
		t.Fatalf("validateDingTalkSelectedOrgIdentity returned error: %v", err)
	}
}

func TestValidateDingTalkSelectedOrgIdentityRejectsMismatchedCorpID(t *testing.T) {
	t.Setenv("DINGTALK_CORP_ID", "selected-corp")

	err := validateDingTalkSelectedOrgIdentity("default", map[string]interface{}{
		"corpId": "other-corp",
	})
	if !errors.Is(err, errDingTalkSelectedOrgMismatch) {
		t.Fatalf("validateDingTalkSelectedOrgIdentity error = %v, want mismatch", err)
	}
}

func TestValidateDingTalkSelectedOrgIdentityRejectsUnionOnlyCallback(t *testing.T) {
	t.Setenv("DINGTALK_CORP_ID", "selected-corp")

	err := validateDingTalkSelectedOrgIdentity("default", map[string]interface{}{
		"unionId": "union-1",
		"openId":  "open-1",
	})
	if !errors.Is(err, errDingTalkSelectedOrgUnverified) {
		t.Fatalf("validateDingTalkSelectedOrgIdentity error = %v, want unverified", err)
	}
}

func TestValidateDingTalkSelectedOrgIdentityAllowsAssociatedUserID(t *testing.T) {
	err := validateDingTalkSelectedOrgIdentity("default", map[string]interface{}{
		"associated_user_id": "user-1",
	})
	if err != nil {
		t.Fatalf("validateDingTalkSelectedOrgIdentity returned error: %v", err)
	}
}

func TestResolveDingTalkQRStateOrgIDKeepsRequestedOrgForMultiOrgLogin(t *testing.T) {
	t.Parallel()

	got := resolveDingTalkQRStateOrgID("xiaotie", true)
	if got != "xiaotie" {
		t.Fatalf("resolveDingTalkQRStateOrgID() = %q, want xiaotie", got)
	}
}

func TestResolveDingTalkQRStateOrgIDKeepsDirectQRLoginUnscoped(t *testing.T) {
	t.Parallel()

	got := resolveDingTalkQRStateOrgID("", true)
	if got != "" {
		t.Fatalf("resolveDingTalkQRStateOrgID() = %q, want empty org", got)
	}
}

func TestResolveDingTalkQRStateOrgIDUsesConfiguredDefaultForUnscopedLogin(t *testing.T) {
	t.Setenv("DINGTALK_QR_DEFAULT_ORG_ID", " default ")

	got := resolveDingTalkQRStateOrgID("", true)
	if got != "default" {
		t.Fatalf("resolveDingTalkQRStateOrgID() = %q, want default", got)
	}
}

func TestResolveDingTalkQRStateOrgIDKeepsRequestedOrgForSingleOrgLogin(t *testing.T) {
	t.Parallel()

	got := resolveDingTalkQRStateOrgID(" xiaotie ", false)
	if got != "xiaotie" {
		t.Fatalf("resolveDingTalkQRStateOrgID() = %q, want xiaotie", got)
	}
}

func TestResolveDingTalkQROAuthOrgIDKeepsRequestedOrgForMultiOrgLogin(t *testing.T) {
	t.Parallel()

	got := resolveDingTalkQROAuthOrgID(" default ", true)
	if got != "default" {
		t.Fatalf("resolveDingTalkQROAuthOrgID() = %q, want default", got)
	}
}

func TestResolveDingTalkQROAuthOrgIDUsesSharedConfigForUnscopedLogin(t *testing.T) {
	t.Parallel()

	got := resolveDingTalkQROAuthOrgID("", true)
	if got != "" {
		t.Fatalf("resolveDingTalkQROAuthOrgID() = %q, want empty org", got)
	}
}

func TestResolveDingTalkQROAuthOrgIDUsesConfiguredDefaultForUnscopedLogin(t *testing.T) {
	t.Setenv("DINGTALK_QR_DEFAULT_ORG_ID", "default")

	got := resolveDingTalkQROAuthOrgID("", true)
	if got != "default" {
		t.Fatalf("resolveDingTalkQROAuthOrgID() = %q, want default", got)
	}
}

func openEnsureLocalUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:ensure-local-user-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Role{},
		&database.UserRole{},
		&database.EmployeeProfile{},
		&database.MenuPermission{},
		&database.DataPermission{},
		&database.Permission{},
		&database.RolePermission{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func ensureLocalUserGinCtx(t *testing.T, db *gorm.DB, orgID string) *gin.Context {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/dingtalk/in-app", nil)
	ctx := requestmeta.WithRequestInfo(req.Context(), &requestmeta.RequestInfo{OrgID: orgID})
	if strings.TrimSpace(orgID) != "" {
		ctx = requestmeta.WithTenant(ctx, orgID)
	}
	c.Request = req.WithContext(ctx)
	c.Set("orgID", orgID)
	return c
}

func TestEnsureLocalUserForDingTalkLogin_MissingOrgFailsClosed(t *testing.T) {
	db := openEnsureLocalUserDB(t)
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })

	c := ensureLocalUserGinCtx(t, db, "")
	user, err := ensureLocalUserForDingTalkLogin(c, "", dingtalk.UserInfo{
		UserID: "dt-new-1",
		Name:   "New User",
		Active: true,
	}, "d1", "test_missing_org")
	if user != nil {
		t.Fatalf("expected nil user, got %+v", user)
	}
	if err == nil {
		t.Fatal("expected error for missing org")
	}
	if !errors.Is(err, repository.ErrMissingOrgID) &&
		!strings.Contains(strings.ToLower(err.Error()), "orgid required") &&
		!strings.Contains(strings.ToLower(err.Error()), "missing organization") {
		t.Fatalf("err = %v, want orgID required / missing organization", err)
	}

	var total int64
	if err := db.Model(&database.User{}).Count(&total).Error; err != nil {
		t.Fatalf("count users: %v", err)
	}
	if total != 0 {
		t.Fatalf("empty org must not invent users, got %d", total)
	}
}

func TestEnsureLocalUserForDingTalkLogin_WhitespaceOrgFailsClosed(t *testing.T) {
	db := openEnsureLocalUserDB(t)
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })

	c := ensureLocalUserGinCtx(t, db, "   ")
	_, err := ensureLocalUserForDingTalkLogin(c, "   ", dingtalk.UserInfo{
		UserID: "dt-ws",
		Name:   "WS",
		Active: true,
	}, "", "test_ws")
	if err == nil {
		t.Fatal("whitespace org must fail")
	}
	if !errors.Is(err, repository.ErrMissingOrgID) && !strings.Contains(err.Error(), "orgID required") {
		t.Fatalf("err = %v, want ErrMissingOrgID", err)
	}
}

func TestEnsureLocalUserForDingTalkLogin_CreatesUserInOrg(t *testing.T) {
	db := openEnsureLocalUserDB(t)
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })

	const orgID = "muteng"
	c := ensureLocalUserGinCtx(t, db, orgID)
	// RequestDB falls back to database.DB.WithContext when TenantContext is not applied.
	database.DB = db.WithContext(c.Request.Context())

	user, err := ensureLocalUserForDingTalkLogin(c, orgID, dingtalk.UserInfo{
		UserID:   "dt-new-2",
		Name:     "Alice Muteng",
		Email:    "alice@example.com",
		Position: "Engineer",
		Active:   true,
	}, "100", "test_provision")
	if err != nil {
		t.Fatalf("ensureLocalUserForDingTalkLogin: %v", err)
	}
	if user == nil {
		t.Fatal("expected user")
	}
	if user.OrgID != orgID {
		t.Fatalf("user.OrgID = %q, want %q", user.OrgID, orgID)
	}
	if user.DingTalkUserID != "dt-new-2" && !strings.Contains(user.UserID, "dt-new-2") {
		t.Fatalf("unexpected user identity: user_id=%q dingtalk=%q", user.UserID, user.DingTalkUserID)
	}

	var stored database.User
	if err := db.Where("org_id = ?", orgID).First(&stored).Error; err != nil {
		t.Fatalf("load stored user: %v", err)
	}
	if stored.OrgID != orgID {
		t.Fatalf("stored OrgID = %q, want %q", stored.OrgID, orgID)
	}

	var foreign int64
	_ = db.Model(&database.User{}).Where("org_id <> ?", orgID).Count(&foreign)
	if foreign != 0 {
		t.Fatalf("must not write other org users, foreign=%d", foreign)
	}

	var profiles int64
	_ = db.Model(&database.EmployeeProfile{}).Where("org_id = ?", orgID).Count(&profiles)
	if profiles != 1 {
		t.Fatalf("profiles = %d, want 1 in org", profiles)
	}
}
