package database

import (
	"reflect"
	"strings"
	"testing"
)

func TestLeaveScheduleBusinessUniqueSpecsCoverPhase2Tables(t *testing.T) {
	want := map[string][]string{
		"employee_shift_configs":     {"org_id", "user_id"},
		"dingtalk_shift_catalogs":    {"org_id", "shift_key"},
		"week_schedule_rules":        {"org_id", "scope_type", "scope_id"},
		"week_schedule_overrides":    {"org_id", "scope_type", "scope_id", "week_start_date"},
		"statutory_holidays":         {"org_id", "date"},
		"annual_leave_eligibilities": {"org_id", "user_id", "year", "quarter"},
		"annual_leave_grants":        {"org_id", "user_id", "year", "quarter", "grant_type"},
		"overtime_rule_configs":      {"org_id", "rule_key"},
		"overtime_match_results":     {"org_id", "user_id", "work_date"},
		"overtime_sync_histories":    {"org_id", "user_id", "work_date"},
		"annual_leave_consume_logs":  {"org_id", "request_ref", "grant_id"},
	}

	got := leaveScheduleBusinessUniqueSpecs()
	if len(got) != len(want) {
		t.Fatalf("spec count = %d, want %d", len(got), len(want))
	}
	seen := map[string]bool{}
	for _, s := range got {
		cols, ok := want[s.Table]
		if !ok {
			t.Fatalf("unexpected table %s", s.Table)
		}
		if !reflect.DeepEqual(s.Columns, cols) {
			t.Fatalf("%s columns = %v, want %v", s.Table, s.Columns, cols)
		}
		if s.Columns[0] != "org_id" {
			t.Fatalf("%s must start with org_id", s.Table)
		}
		if s.NewIndex == "" {
			t.Fatalf("%s missing NewIndex", s.Table)
		}
		seen[s.Table] = true
	}
	for table := range want {
		if !seen[table] {
			t.Fatalf("missing table %s", table)
		}
	}
}

func TestPhase2ModelTagsIncludeOrgCompositeUniques(t *testing.T) {
	cases := []struct {
		name      string
		model     any
		field     string
		wantToken string
	}{
		{"EmployeeShiftConfig.UserID", EmployeeShiftConfig{}, "UserID", "idx_employee_shift_configs_org_user"},
		{"DingTalkShiftCatalog.ShiftKey", DingTalkShiftCatalog{}, "ShiftKey", "idx_dingtalk_shift_catalogs_org_shift_key"},
		{"WeekScheduleRule.ScopeType", WeekScheduleRule{}, "ScopeType", "idx_week_schedule_rules_org_scope"},
		{"WeekScheduleOverride.WeekStartDate", WeekScheduleOverride{}, "WeekStartDate", "idx_week_schedule_overrides_org_scope_date"},
		{"StatutoryHoliday.Date", StatutoryHoliday{}, "Date", "idx_statutory_holidays_org_date"},
		{"AnnualLeaveEligibility.UserID", AnnualLeaveEligibility{}, "UserID", "idx_leave_elig_org_user_year_q"},
		{"AnnualLeaveGrant.GrantType", AnnualLeaveGrant{}, "GrantType", "idx_leave_grant_org_user_year_q_type"},
		{"OvertimeRuleConfig.RuleKey", OvertimeRuleConfig{}, "RuleKey", "idx_overtime_rule_org_key"},
		{"OvertimeMatchResult.WorkDate", OvertimeMatchResult{}, "WorkDate", "idx_overtime_match_org_user_work_date"},
		{"OvertimeSyncHistory.WorkDate", OvertimeSyncHistory{}, "WorkDate", "idx_overtime_sync_org_user_workdate"},
		{"AnnualLeaveConsumeLog.RequestRef", AnnualLeaveConsumeLog{}, "RequestRef", "idx_leave_consume_org_request_grant"},
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
			// Ensure no bare single-column unique on the business field for multi-tenant keys.
			if strings.Contains(tag, "uniqueIndex;") || strings.HasSuffix(tag, "uniqueIndex") {
				if !strings.Contains(tag, "uniqueIndex:") {
					t.Fatalf("field still uses global uniqueIndex: %s", tag)
				}
			}
		})
	}
}

func TestPhase2ModelOrgIDParticipatesInCompositeUnique(t *testing.T) {
	models := []any{
		EmployeeShiftConfig{},
		DingTalkShiftCatalog{},
		WeekScheduleRule{},
		WeekScheduleOverride{},
		StatutoryHoliday{},
		AnnualLeaveEligibility{},
		AnnualLeaveGrant{},
		OvertimeRuleConfig{},
		OvertimeMatchResult{},
		OvertimeSyncHistory{},
		AnnualLeaveConsumeLog{},
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
		if !strings.Contains(tag, "not null") {
			t.Fatalf("%T.OrgID should be not null, tag=%s", m, tag)
		}
	}
}

func TestStringifySQLValue(t *testing.T) {
	if stringifySQLValue(nil) != "<nil>" {
		t.Fatal("nil")
	}
	if stringifySQLValue([]byte("abc")) != "abc" {
		t.Fatal("bytes")
	}
	if stringifySQLValue(3) != "3" {
		t.Fatal("int")
	}
}

// TestCompensatoryLeaveAndSupplementaryHaveNoGlobalBusinessUnique documents
// the phase-2 decision: ledger / supplementary rely on org-scoped app checks
// (ExistsBySourceMatchKey / FindPendingByMatchResultID), not a DB unique that
// would require auto-merging historical empty match_ref rows.
func TestCompensatoryLeaveAndSupplementaryHaveNoGlobalBusinessUnique(t *testing.T) {
	ledgerType := reflect.TypeOf(CompensatoryLeaveLedger{})
	for i := 0; i < ledgerType.NumField(); i++ {
		tag := ledgerType.Field(i).Tag.Get("gorm")
		if strings.Contains(tag, "uniqueIndex") || strings.Contains(tag, "unique;") {
			t.Fatalf("CompensatoryLeaveLedger should not introduce DB unique in phase-2 without data merge plan: %s", tag)
		}
	}
	suppType := reflect.TypeOf(OvertimeSupplementaryRequest{})
	for i := 0; i < suppType.NumField(); i++ {
		tag := suppType.Field(i).Tag.Get("gorm")
		if strings.Contains(tag, "uniqueIndex") || strings.Contains(tag, "unique;") {
			t.Fatalf("OvertimeSupplementaryRequest should not introduce DB unique in phase-2: %s", tag)
		}
	}
}

func TestTwoOrgsSameBusinessKeyIsIntendedByCompositeIndexes(t *testing.T) {
	// Contract: after migration, (orgA, keyX) and (orgB, keyX) must both be allowed.
	// We assert the index columns begin with org_id and include the prior business key.
	for _, s := range leaveScheduleBusinessUniqueSpecs() {
		if s.Columns[0] != "org_id" {
			t.Fatalf("%s does not tenant-scope unique", s.Table)
		}
		if len(s.Columns) < 2 {
			t.Fatalf("%s unique too short", s.Table)
		}
	}
}
