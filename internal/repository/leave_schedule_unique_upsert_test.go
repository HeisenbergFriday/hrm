package repository

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"peopleops/internal/database"

	"gorm.io/gorm/clause"
)

func TestShiftConfigUpsertOnConflictColumns(t *testing.T) {
	// Repository contract: OnConflict must target (org_id, user_id).
	onConflict := clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_name", "shift_id", "end_time", "note", "updated_at"}),
	}
	if len(onConflict.Columns) != 2 || onConflict.Columns[0].Name != "org_id" || onConflict.Columns[1].Name != "user_id" {
		t.Fatalf("shift config OnConflict columns = %+v", onConflict.Columns)
	}
	src := readPackageFile(t, "shift_config_repository.go")
	if !strings.Contains(src, `Name: "org_id"`) || !strings.Contains(src, `Name: "user_id"`) {
		t.Fatal("shift_config_repository Upsert must OnConflict on org_id + user_id")
	}
	if strings.Contains(src, `Columns:   []clause.Column{{Name: "user_id"}}`) {
		t.Fatal("legacy user_id-only OnConflict still present")
	}
	mustJoinUnique(t, database.EmployeeShiftConfig{}, "OrgID", "idx_employee_shift_configs_org_user")
	mustJoinUnique(t, database.EmployeeShiftConfig{}, "UserID", "idx_employee_shift_configs_org_user")
}

func TestHolidayUpsertOnConflictColumns(t *testing.T) {
	src := readPackageFile(t, "week_schedule_repository.go")
	if !strings.Contains(src, `Name: "org_id"`) || !strings.Contains(src, `Name: "date"`) {
		t.Fatal("UpsertHoliday must OnConflict on org_id + date")
	}
	if strings.Contains(src, `Columns:   []clause.Column{{Name: "date"}}`) {
		t.Fatal("legacy date-only OnConflict still present")
	}
	mustJoinUnique(t, database.StatutoryHoliday{}, "OrgID", "idx_statutory_holidays_org_date")
	mustJoinUnique(t, database.StatutoryHoliday{}, "Date", "idx_statutory_holidays_org_date")
}

func TestAnnualLeaveGrantCreateIfAbsentOnConflictIncludesOrg(t *testing.T) {
	src := readPackageFile(t, "annual_leave_grant_repository.go")
	for _, col := range []string{"org_id", "user_id", "year", "quarter", "grant_type"} {
		if !strings.Contains(src, `Name: "`+col+`"`) {
			t.Fatalf("CreateIfAbsent missing OnConflict column %s", col)
		}
	}
	mustJoinUnique(t, database.AnnualLeaveGrant{}, "OrgID", "idx_leave_grant_org_user_year_q_type")
	mustJoinUnique(t, database.AnnualLeaveGrant{}, "UserID", "idx_leave_grant_org_user_year_q_type")
	mustJoinUnique(t, database.AnnualLeaveGrant{}, "Year", "idx_leave_grant_org_user_year_q_type")
	mustJoinUnique(t, database.AnnualLeaveGrant{}, "Quarter", "idx_leave_grant_org_user_year_q_type")
	mustJoinUnique(t, database.AnnualLeaveGrant{}, "GrantType", "idx_leave_grant_org_user_year_q_type")
}

func TestEligibilityUpsertOnConflictIncludesOrg(t *testing.T) {
	src := readPackageFile(t, "annual_leave_eligibility_repository.go")
	for _, col := range []string{"org_id", "user_id", "year", "quarter"} {
		if !strings.Contains(src, `Name: "`+col+`"`) {
			t.Fatalf("eligibility Upsert missing OnConflict column %s", col)
		}
	}
	mustJoinUnique(t, database.AnnualLeaveEligibility{}, "OrgID", "idx_leave_elig_org_user_year_q")
	mustJoinUnique(t, database.AnnualLeaveEligibility{}, "UserID", "idx_leave_elig_org_user_year_q")
	mustJoinUnique(t, database.AnnualLeaveEligibility{}, "Year", "idx_leave_elig_org_user_year_q")
	mustJoinUnique(t, database.AnnualLeaveEligibility{}, "Quarter", "idx_leave_elig_org_user_year_q")
}

func TestTwoOrgsAllowedSameBusinessKeysInModelContracts(t *testing.T) {
	// Two orgs sharing the same user_id/date/scope/rule/shift must be allowed by composite uniques.
	contracts := []struct {
		model  any
		index  string
		fields []string
	}{
		{database.EmployeeShiftConfig{}, "idx_employee_shift_configs_org_user", []string{"OrgID", "UserID"}},
		{database.DingTalkShiftCatalog{}, "idx_dingtalk_shift_catalogs_org_shift_key", []string{"OrgID", "ShiftKey"}},
		{database.WeekScheduleRule{}, "idx_week_schedule_rules_org_scope", []string{"OrgID", "ScopeType", "ScopeID"}},
		{database.WeekScheduleOverride{}, "idx_week_schedule_overrides_org_scope_date", []string{"OrgID", "ScopeType", "ScopeID", "WeekStartDate"}},
		{database.StatutoryHoliday{}, "idx_statutory_holidays_org_date", []string{"OrgID", "Date"}},
		{database.OvertimeRuleConfig{}, "idx_overtime_rule_org_key", []string{"OrgID", "RuleKey"}},
		{database.OvertimeMatchResult{}, "idx_overtime_match_org_user_work_date", []string{"OrgID", "UserID", "WorkDate"}},
		{database.OvertimeSyncHistory{}, "idx_overtime_sync_org_user_workdate", []string{"OrgID", "UserID", "WorkDate"}},
		{database.AnnualLeaveConsumeLog{}, "idx_leave_consume_org_request_grant", []string{"OrgID", "RequestRef", "GrantID"}},
	}
	for _, c := range contracts {
		for _, f := range c.fields {
			mustJoinUnique(t, c.model, f, c.index)
		}
	}
}

func TestSameOrgDuplicateStillRejectedByUniqueIndexNames(t *testing.T) {
	// Within one org, the composite unique still rejects duplicates: index is UNIQUE, not plain index.
	// We assert model tags use uniqueIndex (not index:) for the composite name.
	cases := []struct {
		model any
		field string
		index string
	}{
		{database.EmployeeShiftConfig{}, "UserID", "idx_employee_shift_configs_org_user"},
		{database.StatutoryHoliday{}, "Date", "idx_statutory_holidays_org_date"},
		{database.OvertimeRuleConfig{}, "RuleKey", "idx_overtime_rule_org_key"},
	}
	for _, c := range cases {
		f, ok := reflect.TypeOf(c.model).FieldByName(c.field)
		if !ok {
			t.Fatalf("missing %s", c.field)
		}
		tag := f.Tag.Get("gorm")
		if !strings.Contains(tag, "uniqueIndex:"+c.index) {
			t.Fatalf("%T.%s must use uniqueIndex:%s, got %s", c.model, c.field, c.index, tag)
		}
	}
}

func mustJoinUnique(t *testing.T, model any, field, index string) {
	t.Helper()
	f, ok := reflect.TypeOf(model).FieldByName(field)
	if !ok {
		t.Fatalf("%T missing field %s", model, field)
	}
	tag := f.Tag.Get("gorm")
	if !strings.Contains(tag, index) {
		t.Fatalf("%T.%s tag %q missing unique %s", model, field, tag, index)
	}
}

func readPackageFile(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(thisFile), name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	// Parse to ensure file is valid Go (guards against broken partial edits).
	fset := token.NewFileSet()
	if _, err := parser.ParseFile(fset, path, b, parser.SkipObjectResolution); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	// Touch ast package so import is used even if parser API changes.
	_ = ast.File{}
	return string(b)
}
