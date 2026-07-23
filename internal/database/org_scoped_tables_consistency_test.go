package database

import (
	"reflect"
	"strings"
	"testing"

	"gorm.io/gorm/schema"
)

// intentionalGlobalModels have no OrgID by design (platform/global catalogs).
var intentionalGlobalModels = map[string]struct{}{
	"permissions":      {},
	"role_permissions": {},
	"organizations":    {}, // tenant root, not org-scoped rows of itself
}

func TestOrganizationScopedTablesIncludeAllOrgIDModels(t *testing.T) {
	// Models that carry OrgID and must be in the organization-scoped allowlist.
	orgModels := []interface{}{
		&User{},
		&Department{},
		&DepartmentChangeLog{},
		&Attendance{},
		&Approval{},
		&ApprovalTemplate{},
		&Role{},
		&UserRole{},
		&MenuPermission{},
		&DataPermission{},
		&OperationLog{},
		&SyncStatus{},
		&IdempotencyRecord{},
		&DingTalkBinding{},
		&UserSession{},
		&LoginLog{},
		&AttendanceExport{},
		&EmployeeProfile{},
		&EmployeeTransfer{},
		&EmployeeResignation{},
		&EmployeeOnboarding{},
		&TalentAnalysis{},
		&EmployeeShiftConfig{},
		&DingTalkShiftCatalog{},
		&WeekScheduleRule{},
		&WeekScheduleOverride{},
		&WeekScheduleSyncLog{},
		&StatutoryHoliday{},
		&LeaveRuleConfig{},
		&AnnualLeaveEligibility{},
		&AnnualLeaveGrant{},
		&OvertimeRuleConfig{},
		&OvertimeMatchResult{},
		&OvertimeSyncHistory{},
		&OvertimeSupplementaryRequest{},
		&CompensatoryLeaveLedger{},
		&AnnualLeaveConsumeLog{},
		&PerformanceTemplate{},
		&PerformanceTemplateSection{},
		&PerformanceTemplateItem{},
		&PerformanceLevelRule{},
		&PerformanceLevelRuleItem{},
		&PerformanceActivity{},
		&PerformanceDistributionRule{},
		&PerformanceDistributionException{},
		&PerformanceReminderLog{},
		&PerformanceInterviewRecord{},
		&PerformanceAppealRecord{},
		&PerformanceParticipant{},
		&PerformanceReview{},
		&PerformanceReviewVersion{},
		&PerformanceRelationshipChangeLog{},
		&PerformanceGoalRecord{},
		&PerformanceGoalApprovalLog{},
		&PerformanceImportBatch{},
		&PerformanceCompanyFinance{},
		&PerformanceIndicatorLibrary{},
		&PerformanceIndicatorItem{},
		&UploadedFile{},
		&OrganizationUser{},
		&DingTalkEventLog{},
		&ExternalAttendanceRaw{},
		&ExternalAttendanceApproveLink{},
		&ExternalUserDepartmentRaw{},
		&UserDepartmentRelation{},
		&ExternalSyncCursor{},
		&ExternalSyncJob{},
		&ExternalSyncLock{},
	}

	// Explicit global allowlist markers.
	globalModels := []interface{}{
		&Permission{},
		&RolePermission{},
	}
	for _, m := range globalModels {
		name := tableNameOf(t, m)
		if _, ok := intentionalGlobalModels[name]; !ok {
			t.Fatalf("global model %s missing from intentionalGlobalModels", name)
		}
		if _, hasOrg := reflect.TypeOf(m).Elem().FieldByName("OrgID"); hasOrg {
			t.Fatalf("global model %s unexpectedly has OrgID", name)
		}
		if isOrganizationScopedTable(name) {
			t.Fatalf("global table %s must not be organization-scoped", name)
		}
	}

	set := organizationScopedTableNameSet
	fromFn := map[string]struct{}{}
	for _, name := range organizationScopedTables() {
		fromFn[name] = struct{}{}
	}

	for _, m := range orgModels {
		typ := reflect.TypeOf(m).Elem()
		if _, ok := typ.FieldByName("OrgID"); !ok {
			t.Fatalf("model %s expected OrgID field", typ.Name())
		}
		name := tableNameOf(t, m)
		if _, ok := set[name]; !ok {
			t.Fatalf("organizationScopedTableNameSet missing %s (%s)", name, typ.Name())
		}
		if _, ok := fromFn[name]; !ok {
			t.Fatalf("organizationScopedTables() missing %s (%s)", name, typ.Name())
		}
	}

	// Required tables from security checklist.
	required := []string{
		"organization_users",
		"ding_talk_event_logs", // GORM default for DingTalkEventLog
		"performance_import_batches",
		"external_attendance_raw",
		"external_attendance_approve_links",
		"external_user_department_raw",
		"user_department_relations",
		"external_sync_cursors",
		"external_sync_jobs",
		"external_sync_locks",
	}
	for _, name := range required {
		if _, ok := set[name]; !ok {
			t.Fatalf("required org table missing from set: %s", name)
		}
		if _, ok := fromFn[name]; !ok {
			t.Fatalf("required org table missing from organizationScopedTables(): %s", name)
		}
	}
}

func tableNameOf(t *testing.T, model interface{}) string {
	t.Helper()
	if tn, ok := model.(interface{ TableName() string }); ok {
		if name := strings.TrimSpace(tn.TableName()); name != "" {
			return name
		}
	}
	// Prefer GORM pluralization of the type name when TableName() is absent.
	typ := reflect.TypeOf(model)
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	// Default GORM naming: snake_case plural via schema.NamingStrategy.
	ns := schema.NamingStrategy{}
	return ns.TableName(typ.Name())
}
