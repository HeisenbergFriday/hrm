package registry

import (
	"reflect"
	"testing"

	"peopleops/internal/database"
)

func TestHasOrgIDMatchesDatabaseModels(t *testing.T) {
	models := map[string]any{
		"users":                                database.User{},
		"departments":                          database.Department{},
		"department_change_logs":               database.DepartmentChangeLog{},
		"attendances":                          database.Attendance{},
		"approvals":                            database.Approval{},
		"approval_templates":                   database.ApprovalTemplate{},
		"user_roles":                           database.UserRole{},
		"operation_logs":                       database.OperationLog{},
		"sync_statuses":                        database.SyncStatus{},
		"dingtalk_bindings":                    database.DingTalkBinding{},
		"user_sessions":                        database.UserSession{},
		"login_logs":                           database.LoginLog{},
		"attendance_exports":                   database.AttendanceExport{},
		"employee_profiles":                    database.EmployeeProfile{},
		"employee_transfers":                   database.EmployeeTransfer{},
		"employee_resignations":                database.EmployeeResignation{},
		"employee_onboardings":                 database.EmployeeOnboarding{},
		"talent_analyses":                      database.TalentAnalysis{},
		"employee_shift_configs":               database.EmployeeShiftConfig{},
		"dingtalk_shift_catalogs":              database.DingTalkShiftCatalog{},
		"week_schedule_rules":                  database.WeekScheduleRule{},
		"week_schedule_overrides":              database.WeekScheduleOverride{},
		"week_schedule_sync_logs":              database.WeekScheduleSyncLog{},
		"statutory_holidays":                   database.StatutoryHoliday{},
		"leave_rule_configs":                   database.LeaveRuleConfig{},
		"annual_leave_eligibilities":           database.AnnualLeaveEligibility{},
		"annual_leave_grants":                  database.AnnualLeaveGrant{},
		"annual_leave_consume_logs":            database.AnnualLeaveConsumeLog{},
		"overtime_rule_configs":                database.OvertimeRuleConfig{},
		"overtime_match_results":               database.OvertimeMatchResult{},
		"overtime_sync_histories":              database.OvertimeSyncHistory{},
		"overtime_supplementary_requests":      database.OvertimeSupplementaryRequest{},
		"compensatory_leave_ledgers":           database.CompensatoryLeaveLedger{},
		"performance_templates":                database.PerformanceTemplate{},
		"performance_template_sections":        database.PerformanceTemplateSection{},
		"performance_template_items":           database.PerformanceTemplateItem{},
		"performance_level_rules":              database.PerformanceLevelRule{},
		"performance_level_rule_items":         database.PerformanceLevelRuleItem{},
		"performance_activities":               database.PerformanceActivity{},
		"performance_distribution_rules":       database.PerformanceDistributionRule{},
		"performance_distribution_exceptions":  database.PerformanceDistributionException{},
		"performance_reminder_logs":            database.PerformanceReminderLog{},
		"performance_participants":             database.PerformanceParticipant{},
		"performance_reviews":                  database.PerformanceReview{},
		"performance_review_versions":          database.PerformanceReviewVersion{},
		"performance_relationship_change_logs": database.PerformanceRelationshipChangeLog{},
		"performance_goal_records":             database.PerformanceGoalRecord{},
		"performance_goal_approval_logs":       database.PerformanceGoalApprovalLog{},
		"performance_company_finances":         database.PerformanceCompanyFinance{},
		"performance_indicator_libraries":      database.PerformanceIndicatorLibrary{},
		"performance_indicator_items":          database.PerformanceIndicatorItem{},
		"organization_users":                   database.OrganizationUser{},
	}
	for _, tbl := range All() {
		if tbl.Kind == KindPlatform {
			continue
		}
		model, ok := models[tbl.Name]
		if !ok {
			t.Fatalf("missing model mapping for %s", tbl.Name)
		}
		_, hasOrgID := reflect.TypeOf(model).FieldByName("OrgID")
		if tbl.HasOrgID != hasOrgID {
			t.Fatalf("%s HasOrgID=%v, model has OrgID=%v", tbl.Name, tbl.HasOrgID, hasOrgID)
		}
	}
}
