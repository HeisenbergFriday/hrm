package api

import (
	"errors"
	"testing"
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
