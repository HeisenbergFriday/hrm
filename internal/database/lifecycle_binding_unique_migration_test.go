package database

import (
	"reflect"
	"strings"
	"testing"
)

func TestLifecycleBindingBusinessUniqueSpecsCoverPhase3Tables(t *testing.T) {
	// Multiple specs may share a table (e.g. onboardings has two uniques).
	want := []struct {
		table   string
		index   string
		columns []string
	}{
		{"employee_transfers", "idx_employee_transfers_org_transfer", []string{"org_id", "transfer_id"}},
		{"employee_resignations", "idx_employee_resignations_org_resignation", []string{"org_id", "resignation_id"}},
		{"employee_onboardings", "idx_employee_onboardings_org_onboarding", []string{"org_id", "onboarding_id"}},
		{"employee_onboardings", "idx_employee_onboardings_org_employee", []string{"org_id", "employee_id"}},
		{"talent_analyses", "idx_talent_analyses_org_user", []string{"org_id", "user_id"}},
		{"ding_talk_bindings", "idx_dingtalk_bindings_org_user", []string{"org_id", "user_id"}},
		{"ding_talk_bindings", "idx_dingtalk_bindings_org_ding", []string{"org_id", "ding_talk_user_id"}},
		{"ding_talk_bindings", "idx_dingtalk_bindings_org_union", []string{"org_id", "union_id"}},
		{"ding_talk_bindings", "idx_dingtalk_bindings_org_open", []string{"org_id", "open_id"}},
		{"idempotency_records", "idx_idempotency_org_digest", []string{"org_id", "digest"}},
		{"users", "idx_users_org_mobile", []string{"org_id", "mobile"}},
	}

	got := lifecycleBindingBusinessUniqueSpecs()
	if len(got) != len(want) {
		t.Fatalf("spec count = %d, want %d", len(got), len(want))
	}
	seen := map[string]bool{}
	for i, s := range got {
		w := want[i]
		if s.Table != w.table || s.NewIndex != w.index {
			t.Fatalf("spec[%d] table/index = %s/%s, want %s/%s", i, s.Table, s.NewIndex, w.table, w.index)
		}
		if !reflect.DeepEqual(s.Columns, w.columns) {
			t.Fatalf("%s columns = %v, want %v", s.NewIndex, s.Columns, w.columns)
		}
		if s.Columns[0] != "org_id" {
			t.Fatalf("%s must start with org_id", s.NewIndex)
		}
		seen[s.NewIndex] = true
	}
	for _, w := range want {
		if !seen[w.index] {
			t.Fatalf("missing index %s", w.index)
		}
	}
}

func TestPhase3ModelTagsIncludeOrgCompositeUniques(t *testing.T) {
	cases := []struct {
		name      string
		model     any
		field     string
		wantToken string
		// bareUniqueForbidden: field must not keep single-column unique/uniqueIndex
		bareUniqueForbidden bool
	}{
		{"EmployeeTransfer.TransferID", EmployeeTransfer{}, "TransferID", "idx_employee_transfers_org_transfer", true},
		{"EmployeeResignation.ResignationID", EmployeeResignation{}, "ResignationID", "idx_employee_resignations_org_resignation", true},
		{"EmployeeOnboarding.OnboardingID", EmployeeOnboarding{}, "OnboardingID", "idx_employee_onboardings_org_onboarding", true},
		{"EmployeeOnboarding.EmployeeID", EmployeeOnboarding{}, "EmployeeID", "idx_employee_onboardings_org_employee", true},
		{"TalentAnalysis.UserID", TalentAnalysis{}, "UserID", "idx_talent_analyses_org_user", true},
		{"DingTalkBinding.UserID", DingTalkBinding{}, "UserID", "idx_dingtalk_bindings_org_user", true},
		{"DingTalkBinding.DingTalkUserID", DingTalkBinding{}, "DingTalkUserID", "idx_dingtalk_bindings_org_ding", true},
		{"DingTalkBinding.UnionID", DingTalkBinding{}, "UnionID", "idx_dingtalk_bindings_org_union", true},
		{"DingTalkBinding.OpenID", DingTalkBinding{}, "OpenID", "idx_dingtalk_bindings_org_open", true},
		{"IdempotencyRecord.Digest", IdempotencyRecord{}, "Digest", "idx_idempotency_org_digest", true},
		{"User.Mobile", User{}, "Mobile", "idx_users_org_mobile", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, ok := reflect.TypeOf(tc.model).FieldByName(tc.field)
			if !ok {
				t.Fatalf("field %s missing", tc.field)
			}
			tag := f.Tag.Get("gorm")
			if !strings.Contains(tag, tc.wantToken) {
				t.Fatalf("gorm tag %q missing %s", tag, tc.wantToken)
			}
			if tc.bareUniqueForbidden {
				// bare "unique;" or trailing "unique" without composite name is forbidden.
				if strings.Contains(tag, "unique;") || strings.HasSuffix(tag, ";unique") || tag == "unique" {
					// Allow uniqueIndex:name form only.
					if !strings.Contains(tag, "uniqueIndex:") {
						t.Fatalf("field still uses global unique: %s", tag)
					}
				}
				if strings.Contains(tag, "uniqueIndex;") || strings.HasSuffix(tag, "uniqueIndex") {
					if !strings.Contains(tag, "uniqueIndex:") {
						t.Fatalf("field still uses global uniqueIndex: %s", tag)
					}
				}
			}
		})
	}
}

func TestPhase3ModelOrgIDParticipatesInCompositeUnique(t *testing.T) {
	models := []any{
		EmployeeTransfer{},
		EmployeeResignation{},
		EmployeeOnboarding{},
		TalentAnalysis{},
		DingTalkBinding{},
		IdempotencyRecord{},
		User{},
	}
	for _, m := range models {
		f, ok := reflect.TypeOf(m).FieldByName("OrgID")
		if !ok {
			t.Fatalf("%T missing OrgID", m)
		}
		tag := f.Tag.Get("gorm")
		if !strings.Contains(tag, "uniqueIndex:") {
			t.Fatalf("%T.OrgID should join composite unique, tag=%s", m, tag)
		}
	}
}

func TestPhase3DingTalkBindingUnionOpenAreOrgScopedNotGlobal(t *testing.T) {
	// Documented decision: union_id/open_id are org-scoped unique keys, not platform-global.
	// Rationale: bindings map enterprise-login; same natural person may bind in multiple orgs.
	for _, field := range []string{"UnionID", "OpenID"} {
		f, ok := reflect.TypeOf(DingTalkBinding{}).FieldByName(field)
		if !ok {
			t.Fatalf("missing %s", field)
		}
		tag := f.Tag.Get("gorm")
		if strings.Contains(tag, "unique;") || (strings.Contains(tag, "unique") && !strings.Contains(tag, "uniqueIndex:")) {
			t.Fatalf("%s must not be globally unique: %s", field, tag)
		}
		if !strings.Contains(tag, "uniqueIndex:idx_dingtalk_bindings_org_") {
			t.Fatalf("%s should use org composite unique: %s", field, tag)
		}
	}
}

func TestPhase3UsersMobileNotGloballyUnique(t *testing.T) {
	f, ok := reflect.TypeOf(User{}).FieldByName("Mobile")
	if !ok {
		t.Fatal("User.Mobile missing")
	}
	tag := f.Tag.Get("gorm")
	if strings.Contains(tag, "unique;") {
		t.Fatalf("Mobile must not be global unique: %s", tag)
	}
	if !strings.Contains(tag, "idx_users_org_mobile") {
		t.Fatalf("Mobile should join idx_users_org_mobile: %s", tag)
	}
}

// TestPhase3CrossOrgSameBusinessKeysWouldBeAllowed documents the intended multi-tenant
// uniqueness: same transfer_id / employee_id / user_id / mobile / digest may exist across
// orgs; only within one org they collide. This is a structural/tag test (no live DB).
func TestPhase3CrossOrgSameBusinessKeysWouldBeAllowed(t *testing.T) {
	// Composite unique starts with org_id ⇒ cross-org duplicates of the business column are allowed.
	for _, s := range lifecycleBindingBusinessUniqueSpecs() {
		if len(s.Columns) < 2 || s.Columns[0] != "org_id" {
			t.Fatalf("%s must be composite unique starting with org_id, got %v", s.NewIndex, s.Columns)
		}
	}
}
