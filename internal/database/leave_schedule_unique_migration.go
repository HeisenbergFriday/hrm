package database

import (
	"fmt"

	"gorm.io/gorm"
)

// leaveScheduleBusinessUniqueSpec describes one phase-2 composite unique
// for shift / week-schedule / leave / overtime tables.
// Phase 4 migrates these via OrgCompositeUniqueSpec / MigrateOrgCompositeUniqueIndexes.
type leaveScheduleBusinessUniqueSpec struct {
	Table        string
	NewIndex     string
	Columns      []string
	SoftDelete   bool
	OldIndexes   []string // known legacy unique index names to drop after audit
	OldSingleCol string   // if set, also drop any single-column UNIQUE on this column
}

func leaveScheduleBusinessUniqueSpecs() []leaveScheduleBusinessUniqueSpec {
	return []leaveScheduleBusinessUniqueSpec{
		{
			Table:        "employee_shift_configs",
			NewIndex:     "idx_employee_shift_configs_org_user",
			Columns:      []string{"org_id", "user_id"},
			SoftDelete:   true,
			OldIndexes:   []string{"idx_employee_shift_configs_user_id", "uni_employee_shift_configs_user_id", "user_id"},
			OldSingleCol: "user_id",
		},
		{
			Table:        "dingtalk_shift_catalogs",
			NewIndex:     "idx_dingtalk_shift_catalogs_org_shift_key",
			Columns:      []string{"org_id", "shift_key"},
			SoftDelete:   false,
			OldIndexes:   []string{"idx_dingtalk_shift_catalogs_shift_key", "uni_dingtalk_shift_catalogs_shift_key", "shift_key"},
			OldSingleCol: "shift_key",
		},
		{
			Table:      "week_schedule_rules",
			NewIndex:   "idx_week_schedule_rules_org_scope",
			Columns:    []string{"org_id", "scope_type", "scope_id"},
			SoftDelete: true,
			OldIndexes: []string{"idx_scope", "uni_week_schedule_rules_scope"},
		},
		{
			Table:      "week_schedule_overrides",
			NewIndex:   "idx_week_schedule_overrides_org_scope_date",
			Columns:    []string{"org_id", "scope_type", "scope_id", "week_start_date"},
			SoftDelete: false,
			OldIndexes: []string{"idx_scope_date", "uni_week_schedule_overrides_scope_date"},
		},
		{
			Table:        "statutory_holidays",
			NewIndex:     "idx_statutory_holidays_org_date",
			Columns:      []string{"org_id", "date"},
			SoftDelete:   false,
			OldIndexes:   []string{"uni_statutory_holidays_date", "idx_statutory_holidays_date", "date"},
			OldSingleCol: "date",
		},
		{
			Table:      "annual_leave_eligibilities",
			NewIndex:   "idx_leave_elig_org_user_year_q",
			Columns:    []string{"org_id", "user_id", "year", "quarter"},
			SoftDelete: false,
			OldIndexes: []string{"idx_leave_elig_user_year_q"},
		},
		{
			Table:      "annual_leave_grants",
			NewIndex:   "idx_leave_grant_org_user_year_q_type",
			Columns:    []string{"org_id", "user_id", "year", "quarter", "grant_type"},
			SoftDelete: false,
			OldIndexes: []string{"idx_leave_grant_user_year_q_type"},
		},
		{
			Table:        "overtime_rule_configs",
			NewIndex:     "idx_overtime_rule_org_key",
			Columns:      []string{"org_id", "rule_key"},
			SoftDelete:   false,
			OldIndexes:   []string{"uni_overtime_rule_configs_rule_key", "idx_overtime_rule_configs_rule_key", "rule_key"},
			OldSingleCol: "rule_key",
		},
		{
			Table:      "overtime_match_results",
			NewIndex:   "idx_overtime_match_org_user_work_date",
			Columns:    []string{"org_id", "user_id", "work_date"},
			SoftDelete: true,
			OldIndexes: []string{"idx_user_work_date"},
		},
		{
			Table:      "overtime_sync_histories",
			NewIndex:   "idx_overtime_sync_org_user_workdate",
			Columns:    []string{"org_id", "user_id", "work_date"},
			SoftDelete: false,
			OldIndexes: []string{"idx_overtime_sync_user_workdate"},
		},
		{
			Table:      "annual_leave_consume_logs",
			NewIndex:   "idx_leave_consume_org_request_grant",
			Columns:    []string{"org_id", "request_ref", "grant_id"},
			SoftDelete: false,
			OldIndexes: []string{
				"idx_leave_consume_request_grant",
				"idx_leave_consume_approval_ref",
				"idx_annual_leave_consume_logs_approval_ref",
				"uni_annual_leave_consume_logs_approval_ref",
				"approval_ref",
			},
			OldSingleCol: "approval_ref",
		},
	}
}

func overtimeRuleConfigOrgUniqueSpec() (OrgCompositeUniqueSpec, error) {
	for _, spec := range leaveScheduleSpecsAsOrgSpecs() {
		if spec.Table == "overtime_rule_configs" {
			return spec, nil
		}
	}
	return OrgCompositeUniqueSpec{}, fmt.Errorf("overtime_rule_configs org unique spec is not configured")
}

// prepareOvertimeRuleConfigOrgUniqueIndex runs before OvertimeRuleConfig AutoMigrate.
// It upgrades an existing table without touching unrelated, not-yet-aligned phase-4 tables.
func prepareOvertimeRuleConfigOrgUniqueIndex(db *gorm.DB) error {
	spec, err := overtimeRuleConfigOrgUniqueSpec()
	if err != nil {
		return err
	}
	return migrateOneOrgCompositeUniqueWithOptions(db, spec, orgUniqueMigrationOptions{
		addMissingOrgColumn: true,
	})
}

// migrateOvertimeRuleConfigOrgUniqueIndex verifies the table created or updated by AutoMigrate.
func migrateOvertimeRuleConfigOrgUniqueIndex(db *gorm.DB) error {
	spec, err := overtimeRuleConfigOrgUniqueSpec()
	if err != nil {
		return err
	}
	return migrateOneOrgCompositeUnique(db, spec)
}
