package main

import (
	"errors"
	"strings"
	"testing"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

type fakeStreamOrgSource struct {
	byOrgID  map[string]*database.Organization
	byAppKey map[string][]database.Organization
}

func (f fakeStreamOrgSource) GetActiveOrg(orgID string) (*database.Organization, error) {
	org, ok := f.byOrgID[orgID]
	if !ok || org == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return org, nil
}

func (f fakeStreamOrgSource) ListActiveByAppKey(appKey string) ([]database.Organization, error) {
	return f.byAppKey[appKey], nil
}

func TestResolveStreamOrgID_ExplicitMatchSuccess(t *testing.T) {
	src := fakeStreamOrgSource{
		byOrgID: map[string]*database.Organization{
			"muteng": {OrgID: "muteng", DingTalkAppKey: "dingappkey12345", Status: "active"},
		},
	}
	got, err := resolveStreamOrgIDWithSource(src, "muteng", "dingappkey12345")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "muteng" {
		t.Fatalf("got=%s", got)
	}
}

func TestResolveStreamOrgID_ExplicitAppKeyMismatch(t *testing.T) {
	src := fakeStreamOrgSource{
		byOrgID: map[string]*database.Organization{
			"muteng": {OrgID: "muteng", DingTalkAppKey: "correct-key-aaaa", Status: "active"},
		},
	}
	_, err := resolveStreamOrgIDWithSource(src, "muteng", "wrong-key-bbbb")
	if err == nil {
		t.Fatal("expected mismatch error")
	}
	if !strings.Contains(err.Error(), "app key mismatch") {
		t.Fatalf("err=%v", err)
	}
	// 错误信息不得包含完整 clientID。
	if strings.Contains(err.Error(), "wrong-key-bbbb") {
		t.Fatalf("error leaked full app key: %v", err)
	}
	if !strings.Contains(err.Error(), maskAppKey("wrong-key-bbbb")) {
		t.Fatalf("expected masked key in error: %v", err)
	}
}

func TestResolveStreamOrgID_AutoMatchOneSuccess(t *testing.T) {
	src := fakeStreamOrgSource{
		byAppKey: map[string][]database.Organization{
			"only-one-key-xx": {{OrgID: "org-a", DingTalkAppKey: "only-one-key-xx", Status: "active"}},
		},
	}
	got, err := resolveStreamOrgIDWithSource(src, "", "only-one-key-xx")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "org-a" {
		t.Fatalf("got=%s", got)
	}
}

func TestResolveStreamOrgID_AutoMatchZeroFails(t *testing.T) {
	src := fakeStreamOrgSource{byAppKey: map[string][]database.Organization{}}
	_, err := resolveStreamOrgIDWithSource(src, "", "missing-key-yy")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no active organization matched") {
		t.Fatalf("err=%v", err)
	}
	if strings.Contains(err.Error(), "missing-key-yy") {
		t.Fatalf("error leaked full app key: %v", err)
	}
}

func TestResolveStreamOrgID_AutoMatchManyFails(t *testing.T) {
	src := fakeStreamOrgSource{
		byAppKey: map[string][]database.Organization{
			"shared-key-zz": {
				{OrgID: "org-a", DingTalkAppKey: "shared-key-zz"},
				{OrgID: "org-b", DingTalkAppKey: "shared-key-zz"},
			},
		},
	}
	_, err := resolveStreamOrgIDWithSource(src, "", "shared-key-zz")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "matches 2 active organizations") {
		t.Fatalf("err=%v", err)
	}
}

func TestResolveStreamOrgID_ExplicitMissingOrg(t *testing.T) {
	src := fakeStreamOrgSource{byOrgID: map[string]*database.Organization{}}
	_, err := resolveStreamOrgIDWithSource(src, "ghost", "any-key-123456")
	if err == nil {
		t.Fatal("expected not found")
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) && !strings.Contains(err.Error(), "not found or inactive") {
		t.Fatalf("err=%v", err)
	}
}

func TestMaskAppKey(t *testing.T) {
	if got := maskAppKey("abcdefghij"); got == "abcdefghij" {
		t.Fatalf("should mask full key, got %s", got)
	}
	if got := maskAppKey("ab"); !strings.Contains(got, "***") {
		t.Fatalf("got %s", got)
	}
}

func TestTruthyEnv(t *testing.T) {
	t.Setenv("DINGTALK_STREAM_TEST_FLAG", "true")
	if !truthyEnv("DINGTALK_STREAM_TEST_FLAG") {
		t.Fatal("expected true value")
	}
	t.Setenv("DINGTALK_STREAM_TEST_FLAG", "false")
	if truthyEnv("DINGTALK_STREAM_TEST_FLAG") {
		t.Fatal("expected false value")
	}
}
