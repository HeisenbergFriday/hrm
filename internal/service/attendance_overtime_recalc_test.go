package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

func seedOvertimeRule(t *testing.T, db *gorm.DB, orgID string) {
	t.Helper()
	rule := database.OvertimeRuleConfig{
		OrgID:         orgID,
		RuleKey:       "overtime.min_threshold_minutes",
		RuleName:      "最低阈值",
		RuleValueJSON: `{"minutes": 30}`,
		Status:        "active",
	}
	if err := db.Create(&rule).Error; err != nil {
		t.Fatalf("seed rule: %v", err)
	}
}

func migrateRecalcTables(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(
		&database.Approval{},
		&database.Attendance{},
		&database.OvertimeRuleConfig{},
		&database.OvertimeMatchResult{},
		&database.CompensatoryLeaveLedger{},
		&database.OvertimeSyncHistory{},
		&database.OvertimeSupplementaryRequest{},
		&database.User{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

func seedRetryableOvertimeApproval(t *testing.T, db *gorm.DB, orgID, processID, userID string) database.Approval {
	t.Helper()
	overtimeStart := time.Date(2026, 8, 5, 18, 0, 0, 0, dingtalk.ApprovalBusinessLocation())
	approval := database.Approval{
		OrgID: orgID, ProcessID: processID, Title: "加班审批",
		ApplicantID: userID, ApplicantName: "测试员工",
		Status: "COMPLETED", CreateTime: overtimeStart.Add(-time.Hour), FinishTime: overtimeStart,
		Content: map[string]interface{}{
			"加班开始时间": "2026-08-05 18:00:00",
			"加班结束时间": "2026-08-05 21:00:00",
		},
		Extension: map[string]interface{}{"result": "agree", "process_code": "PROC-OVERTIME"},
	}
	if err := db.Create(&approval).Error; err != nil {
		t.Fatalf("create approval: %v", err)
	}
	svc := NewOvertimeMatchingServiceWithOrgID(db, orgID)
	if err := svc.MatchApproval(approval.ID); err != nil {
		t.Fatalf("initial match: %v", err)
	}
	match, err := svc.matchRepo.FindByApprovalID(approval.ID)
	if err != nil {
		t.Fatalf("find initial match: %v", err)
	}
	if match.MatchStatus != "no_clock_record" {
		t.Fatalf("initial match status = %q, want no_clock_record", match.MatchStatus)
	}
	return approval
}

func syncRetryableOvertimeApproval(t *testing.T, db *gorm.DB, orgID, processID, userID string) database.Approval {
	t.Helper()
	instance := dingtalk.ApprovalInstance{
		ProcessInstanceID: processID,
		OriginatorUserID:  userID,
		Title:             "加班审批",
		Status:            "COMPLETED",
		Result:            "agree",
		CreateTime:        "2026-08-05 17:00:00",
		FinishTime:        "2026-08-05 18:00:00",
		FormValues: []map[string]interface{}{
			{"name": "加班开始时间", "value": "2026-08-05 18:00:00"},
			{"name": "加班结束时间", "value": "2026-08-05 21:00:00"},
		},
	}
	syncSvc := NewApprovalSyncService(db, orgID)
	syncSvc.resolveName = nil
	syncSvc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{instance}}, nil
	}
	result := syncSvc.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"PROC-OVERTIME"}}, "attendance-recalc")
	if result.Status != ApprovalSyncStatusPartial ||
		result.ReconcileSkippedCount != 0 ||
		result.ReconcileFailCount != 0 ||
		result.ReconciledCount != 0 ||
		result.ReconcileRetryableCount != 1 {
		t.Fatalf("approval sync reconciliation result = %#v", result)
	}
	var approval database.Approval
	if err := db.Where("org_id = ? AND process_id = ?", orgID, processID).First(&approval).Error; err != nil {
		t.Fatalf("load synchronized approval: %v", err)
	}
	match, err := NewOvertimeMatchingServiceWithOrgID(db, orgID).matchRepo.FindByApprovalID(approval.ID)
	if err != nil || match.MatchStatus != "no_clock_record" {
		t.Fatalf("synchronized approval retryable match status=%q err=%v", match.MatchStatus, err)
	}
	return approval
}

func attendanceRecords(userID string) []dingtalk.AttendanceRecord {
	return []dingtalk.AttendanceRecord{
		{UserID: userID, UserCheckTime: "2026-08-05 18:00:00", CheckType: "OnDuty"},
		{UserID: userID, UserCheckTime: "2026-08-05 21:00:00", CheckType: "OffDuty"},
	}
}

func countLedgerType(t *testing.T, db *gorm.DB, orgID, userID, ledgerType string) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&database.CompensatoryLeaveLedger{}).
		Where("org_id = ? AND user_id = ? AND ledger_type = ?", orgID, userID, ledgerType).
		Count(&count).Error; err != nil {
		t.Fatalf("count %s ledgers: %v", ledgerType, err)
	}
	return count
}

func TestAttendanceSyncAutomaticallyRecalculatesRetryableOvertime(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	seedOvertimeRule(t, db, "org-a")
	approval := syncRetryableOvertimeApproval(t, db, "org-a", "ot-auto", "user-1")

	attendanceSvc := NewAttendanceServiceWithOrgID(db, "org-a")
	records := attendanceRecords("user-1")
	count, err := attendanceSvc.SyncRecords("org-a", records, map[string]string{"user-1": "测试员工"})
	if err != nil {
		t.Fatalf("sync attendance: %v", err)
	}
	if count != 2 {
		t.Fatalf("attendance count = %d, want 2", count)
	}

	match, err := NewOvertimeMatchingServiceWithOrgID(db, "org-a").matchRepo.FindByApprovalID(approval.ID)
	if err != nil {
		t.Fatalf("find recalculated match: %v", err)
	}
	if isOvertimeRetryableMatchStatus(match.MatchStatus) || match.EffectiveOvertimeMinutes != 180 {
		t.Fatalf("recalculated match status=%q minutes=%d", match.MatchStatus, match.EffectiveOvertimeMinutes)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "credit"); got != 1 {
		t.Fatalf("credit count = %d, want 1", got)
	}

	if _, err := attendanceSvc.SyncRecords("org-a", records, map[string]string{"user-1": "测试员工"}); err != nil {
		t.Fatalf("repeat attendance sync: %v", err)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "credit"); got != 1 {
		t.Fatalf("credit count after duplicate sync = %d, want 1", got)
	}
}

func TestAttendanceSyncRecalculationIsTenantScopedAndSkipsTerminal(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	for _, orgID := range []string{"org-a", "org-b"} {
		seedOvertimeRule(t, db, orgID)
		seedRetryableOvertimeApproval(t, db, orgID, "ot-"+orgID, "shared-user")
	}
	terminal := database.OvertimeMatchResult{
		OrgID: "org-a", UserID: "terminal-user", WorkDate: "2026-08-05",
		MatchRef: "terminal:1", ApprovalID: 999, EffectiveOvertimeMinutes: 60,
		MatchStatus: "synced", LocalBalanceStatus: "success", DingtalkSyncStatus: "success",
	}
	if err := db.Create(&terminal).Error; err != nil {
		t.Fatalf("create terminal match: %v", err)
	}

	records := append(attendanceRecords("shared-user"), attendanceRecords("terminal-user")...)
	if _, err := NewAttendanceServiceWithOrgID(db, "org-a").SyncRecords("org-a", records, nil); err != nil {
		t.Fatalf("sync org-a attendance: %v", err)
	}

	orgAMatch, err := NewOvertimeMatchingServiceWithOrgID(db, "org-a").matchRepo.FindByUserAndWorkDate("shared-user", "2026-08-05")
	if err != nil || isOvertimeRetryableMatchStatus(orgAMatch.MatchStatus) {
		t.Fatalf("org-a match not recalculated: status=%q err=%v", orgAMatch.MatchStatus, err)
	}
	orgBMatch, err := NewOvertimeMatchingServiceWithOrgID(db, "org-b").matchRepo.FindByUserAndWorkDate("shared-user", "2026-08-05")
	if err != nil || orgBMatch.MatchStatus != "no_clock_record" {
		t.Fatalf("org-b match crossed tenant: status=%q err=%v", orgBMatch.MatchStatus, err)
	}
	var terminalAfter database.OvertimeMatchResult
	if err := db.Where("org_id = ? AND id = ?", "org-a", terminal.ID).First(&terminalAfter).Error; err != nil {
		t.Fatalf("reload terminal match: %v", err)
	}
	if terminalAfter.MatchStatus != "synced" || countLedgerType(t, db, "org-a", "terminal-user", "credit") != 0 {
		t.Fatalf("terminal match was reprocessed: status=%q", terminalAfter.MatchStatus)
	}
}

func TestAttendanceRecalculationFailurePreservesAttendanceAndCompensates(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	seedOvertimeRule(t, db, "org-a")
	approval := seedRetryableOvertimeApproval(t, db, "org-a", "ot-retry", "user-1")

	attendanceSvc := NewAttendanceServiceWithOrgID(db, "org-a")
	attendanceSvc.retryableOvertimeRecalculator = func([]repository.UserDatePair) (int, error) {
		return 0, errors.New("temporary recalc failure token=must-not-log")
	}
	if _, err := attendanceSvc.SyncRecords("org-a", attendanceRecords("user-1"), nil); err != nil {
		t.Fatalf("attendance sync must survive recalc failure: %v", err)
	}
	var attendanceCount int64
	if err := db.Model(&database.Attendance{}).Where("org_id = ? AND user_id = ?", "org-a", "user-1").Count(&attendanceCount).Error; err != nil {
		t.Fatalf("count attendance: %v", err)
	}
	if attendanceCount != 2 {
		t.Fatalf("attendance count = %d, want 2", attendanceCount)
	}
	match, _ := NewOvertimeMatchingServiceWithOrgID(db, "org-a").matchRepo.FindByApprovalID(approval.ID)
	if !isOvertimeRetryableMatchStatus(match.MatchStatus) {
		t.Fatalf("match status = %q, want retryable after failed trigger", match.MatchStatus)
	}

	recalculated, err := NewOvertimeMatchingServiceWithOrgID(db, "org-a").RunRetryableMatchCompensation("2026-08-01", "2026-08-10")
	if err != nil || recalculated != 1 {
		t.Fatalf("compensation recalculated=%d err=%v", recalculated, err)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "credit"); got != 1 {
		t.Fatalf("credit count after compensation = %d, want 1", got)
	}
}

func TestAttendanceFailedBatchDoesNotStartRecalculation(t *testing.T) {
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	attendanceSvc := NewAttendanceServiceWithOrgID(db, "org-a")
	recalcCalls := 0
	attendanceSvc.retryableOvertimeRecalculator = func([]repository.UserDatePair) (int, error) {
		recalcCalls++
		return 0, nil
	}
	records := attendanceRecords("user-1")
	records = append(records, dingtalk.AttendanceRecord{UserID: "user-1", UserCheckTime: "invalid", CheckType: "OffDuty"})
	if _, err := attendanceSvc.SyncRecords("org-a", records, nil); err == nil {
		t.Fatal("invalid attendance batch should fail")
	}
	if recalcCalls != 0 {
		t.Fatalf("recalculation calls = %d, want 0", recalcCalls)
	}
}

func TestAttendanceSyncEarlyMorningAffectsPreviousWorkDateThroughSix(t *testing.T) {
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	attendanceSvc := NewAttendanceServiceWithOrgID(db, "org-a")

	var captured []repository.UserDatePair
	attendanceSvc.retryableOvertimeRecalculator = func(pairs []repository.UserDatePair) (int, error) {
		captured = append(captured, pairs...)
		return 0, nil
	}
	_, err := attendanceSvc.SyncRecords("org-a", []dingtalk.AttendanceRecord{{
		UserID: "night-user", UserCheckTime: "2026-08-06 01:00:00", CheckType: "OffDuty",
	}}, nil)
	if err != nil {
		t.Fatalf("sync early-morning attendance: %v", err)
	}
	assertUserDatePairs(t, captured, "night-user", "2026-08-05", "2026-08-06")

	location := dingtalk.ApprovalBusinessLocation()
	atCutoff := attendanceAffectedUserDatePairs("night-user", time.Date(2026, 8, 6, 6, 0, 0, 0, location))
	assertUserDatePairs(t, atCutoff, "night-user", "2026-08-05", "2026-08-06")
	afterCutoff := attendanceAffectedUserDatePairs("night-user", time.Date(2026, 8, 6, 6, 0, 1, 0, location))
	assertUserDatePairs(t, afterCutoff, "night-user", "2026-08-06")
}

func assertUserDatePairs(t *testing.T, pairs []repository.UserDatePair, userID string, workDates ...string) {
	t.Helper()
	got := make(map[string]struct{}, len(pairs))
	for _, pair := range pairs {
		if pair.UserID != userID {
			t.Fatalf("pair user_id = %q, want %q", pair.UserID, userID)
		}
		got[pair.WorkDate] = struct{}{}
	}
	if len(got) != len(workDates) {
		t.Fatalf("affected pairs = %#v, want dates %v", pairs, workDates)
	}
	for _, workDate := range workDates {
		if _, ok := got[workDate]; !ok {
			t.Fatalf("affected pairs = %#v, missing date %s", pairs, workDate)
		}
	}
}

func TestRetryableCompensationSelectionRotatesOldestAttempts(t *testing.T) {
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	repo := repository.NewOvertimeMatchResultRepositoryWithOrgID(db, "org-a")
	base := time.Now().Add(-3 * time.Hour)
	matches := make([]database.OvertimeMatchResult, 3)
	for i := range matches {
		matches[i] = database.OvertimeMatchResult{
			OrgID: "org-a", UserID: fmt.Sprintf("queue-user-%d", i), WorkDate: "2026-08-05",
			MatchRef: fmt.Sprintf("queue:%d", i), ApprovalID: uint(700 + i), MatchStatus: "no_clock_record",
			CreatedAt: base.Add(time.Duration(i) * time.Minute), UpdatedAt: base.Add(time.Duration(i) * time.Minute),
		}
		if err := db.Create(&matches[i]).Error; err != nil {
			t.Fatalf("create retryable match %d: %v", i, err)
		}
	}

	first, err := repo.FindRetryableInDateRange("2026-08-01", "2026-08-10", 2)
	if err != nil {
		t.Fatalf("first compensation selection: %v", err)
	}
	if len(first) != 2 || first[0].ID != matches[0].ID || first[1].ID != matches[1].ID {
		t.Fatalf("first compensation selection = %#v", first)
	}
	if err := repo.TouchRetryAttempts([]uint{matches[0].ID, matches[1].ID}); err != nil {
		t.Fatalf("mark first batch attempted: %v", err)
	}

	second, err := repo.FindRetryableInDateRange("2026-08-01", "2026-08-10", 2)
	if err != nil {
		t.Fatalf("second compensation selection: %v", err)
	}
	if len(second) != 2 || second[0].ID != matches[2].ID {
		t.Fatalf("second compensation selection = %#v, want previously starved row first", second)
	}
}

func TestRetryableCompensationTouchesRowsSkippedWithoutApprovalID(t *testing.T) {
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	oldAttempt := time.Now().Add(-time.Hour)
	match := database.OvertimeMatchResult{
		OrgID: "org-a", UserID: "missing-approval-user", WorkDate: "2026-08-05",
		MatchRef: "missing-approval", MatchStatus: "no_clock_record",
		CreatedAt: oldAttempt, UpdatedAt: oldAttempt,
	}
	if err := db.Create(&match).Error; err != nil {
		t.Fatalf("create retryable match without approval: %v", err)
	}

	processed, err := NewOvertimeMatchingServiceWithOrgID(db, "org-a").RunRetryableMatchCompensation("2026-08-01", "2026-08-10")
	if err != nil || processed != 0 {
		t.Fatalf("compensation processed=%d err=%v", processed, err)
	}
	var after database.OvertimeMatchResult
	if err := db.First(&after, match.ID).Error; err != nil {
		t.Fatalf("reload skipped retryable match: %v", err)
	}
	if !after.UpdatedAt.After(oldAttempt) {
		t.Fatalf("retry attempt was not refreshed: before=%s after=%s", oldAttempt, after.UpdatedAt)
	}
}

type absoluteSyncCall struct {
	OrgID        string
	UserID       string
	Year         int
	TotalMinutes int
}

func seedSettledOvertimeMatch(t *testing.T, db *gorm.DB, orgID, userID string, approvalID uint) database.OvertimeMatchResult {
	t.Helper()
	match := database.OvertimeMatchResult{
		OrgID: orgID, UserID: userID, WorkDate: "2026-08-05",
		MatchRef: "settled:" + userID, ApprovalID: approvalID, ApprovalProcessID: "proc-settled",
		EffectiveOvertimeMinutes: 180, MatchStatus: "synced",
		LocalBalanceStatus: "success", DingtalkSyncStatus: "success",
	}
	if err := db.Create(&match).Error; err != nil {
		t.Fatalf("create settled match: %v", err)
	}
	credit := database.CompensatoryLeaveLedger{
		OrgID: orgID, UserID: userID, SourceType: "overtime",
		SourceMatchID: match.ID, SourceMatchRef: match.MatchRef,
		CreditMinutes: 180, BalanceMinutes: 180, LedgerType: "credit", EffectiveDate: match.WorkDate,
	}
	if err := db.Create(&credit).Error; err != nil {
		t.Fatalf("create credit: %v", err)
	}
	now := time.Now()
	history := database.OvertimeSyncHistory{
		OrgID: orgID, UserID: userID, WorkDate: match.WorkDate,
		ApprovalID: approvalID, EffectiveOvertimeMinutes: 180,
		SyncRequestID: "forward", SyncMode: "incremental", SyncedAt: &now,
	}
	if err := db.Create(&history).Error; err != nil {
		t.Fatalf("create sync history: %v", err)
	}
	return match
}

func configureAbsoluteSync(svc *OvertimeMatchingService, calls *[]absoluteSyncCall, results ...error) {
	index := 0
	svc.setAbsoluteCompTimeQuotaFunc = func(orgID, userID string, year, totalMinutes int, _ string) error {
		*calls = append(*calls, absoluteSyncCall{OrgID: orgID, UserID: userID, Year: year, TotalMinutes: totalMinutes})
		if index >= len(results) {
			return nil
		}
		err := results[index]
		index++
		return err
	}
}

func TestRollbackDingTalkSuccessAndDuplicateWithdrawalIsIdempotent(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "true")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	seedSettledOvertimeMatch(t, db, "org-a", "user-1", 200)

	svc := NewOvertimeMatchingServiceWithOrgID(db, "org-a")
	var calls []absoluteSyncCall
	svc.setAbsoluteCompTimeQuotaFunc = func(orgID, userID string, year, totalMinutes int, _ string) error {
		calls = append(calls, absoluteSyncCall{OrgID: orgID, UserID: userID, Year: year, TotalMinutes: totalMinutes})
		pending, err := svc.matchRepo.FindByApprovalID(200)
		if err != nil {
			t.Fatalf("load pending rollback: %v", err)
		}
		if pending.MatchStatus != "rollback_pending" ||
			pending.DingtalkSyncStatus != "rollback_pending" ||
			pending.RollbackDingtalkSyncStatus != "rollback_pending" {
			t.Fatalf("pre-call rollback states: %#v", pending)
		}
		return nil
	}
	if err := svc.RollbackApprovalMatch(200); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := svc.RollbackApprovalMatch(200); err != nil {
		t.Fatalf("duplicate rollback: %v", err)
	}
	if len(calls) != 1 || calls[0] != (absoluteSyncCall{OrgID: "org-a", UserID: "user-1", Year: 2026, TotalMinutes: 0}) {
		t.Fatalf("absolute sync calls = %#v", calls)
	}
	match, _ := svc.matchRepo.FindByApprovalID(200)
	if match.MatchStatus != "rolled_back" || match.DingtalkSyncStatus != "rollback_success" || match.RollbackDingtalkSyncStatus != "rollback_success" {
		t.Fatalf("rollback statuses match=%q sync=%q rollback=%q", match.MatchStatus, match.DingtalkSyncStatus, match.RollbackDingtalkSyncStatus)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "rollback"); got != 1 {
		t.Fatalf("rollback ledger count = %d, want 1", got)
	}
	var history database.OvertimeSyncHistory
	if err := db.Where("org_id = ? AND user_id = ? AND work_date = ?", "org-a", "user-1", "2026-08-05").First(&history).Error; err != nil {
		t.Fatalf("load history: %v", err)
	}
	if history.SyncMode != "rollback" || history.EffectiveOvertimeMinutes != 0 {
		t.Fatalf("rollback history mode=%q minutes=%d", history.SyncMode, history.EffectiveOvertimeMinutes)
	}
}

func TestRollbackDingTalkFailureRetriesWithAbsoluteBalance(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "true")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	seedSettledOvertimeMatch(t, db, "org-a", "user-1", 300)

	svc := NewOvertimeMatchingServiceWithOrgID(db, "org-a")
	var calls []absoluteSyncCall
	configureAbsoluteSync(svc, &calls, errors.New("remote rejected https://example.invalid?token=secret"), nil)
	if err := svc.RollbackApprovalMatch(300); err == nil {
		t.Fatal("first rollback should report DingTalk failure")
	}
	failed, _ := svc.matchRepo.FindByApprovalID(300)
	if failed.MatchStatus != "rollback_failed" || failed.DingtalkSyncStatus != "rollback_failed" || failed.RollbackDingtalkSyncStatus != "rollback_failed" {
		t.Fatalf("failed rollback statuses: %#v", failed)
	}
	if strings.Contains(failed.RollbackDingtalkSyncError, "secret") || strings.Contains(failed.RollbackDingtalkSyncError, "example.invalid") {
		t.Fatalf("unsafe persisted error: %q", failed.RollbackDingtalkSyncError)
	}
	if err := svc.RollbackApprovalMatch(300); err != nil {
		t.Fatalf("retry rollback: %v", err)
	}
	if len(calls) != 2 || calls[0].TotalMinutes != 0 || calls[1].TotalMinutes != 0 {
		t.Fatalf("absolute retry calls = %#v", calls)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "rollback"); got != 1 {
		t.Fatalf("rollback ledger count = %d, want 1", got)
	}
	var history database.OvertimeSyncHistory
	_ = db.Where("org_id = ? AND user_id = ?", "org-a", "user-1").First(&history).Error
	if history.SyncMode != "retry" {
		t.Fatalf("retry history mode = %q", history.SyncMode)
	}
}

func TestRollbackTimeoutCanBeSafelyRetried(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "true")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	seedSettledOvertimeMatch(t, db, "org-a", "user-timeout", 400)

	svc := NewOvertimeMatchingServiceWithOrgID(db, "org-a")
	var calls []absoluteSyncCall
	configureAbsoluteSync(svc, &calls, context.DeadlineExceeded, nil)
	if err := svc.RollbackApprovalMatch(400); err == nil {
		t.Fatal("timeout rollback should remain uncertain")
	}
	if err := svc.RollbackApprovalMatch(400); err != nil {
		t.Fatalf("timeout retry: %v", err)
	}
	if len(calls) != 2 || calls[0].TotalMinutes != calls[1].TotalMinutes {
		t.Fatalf("timeout retry was not absolute and stable: %#v", calls)
	}
}

func TestRollbackFailureThenImmediateReactivationRecalibratesOnce(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	seedOvertimeRule(t, db, "org-a")
	approval := seedRetryableOvertimeApproval(t, db, "org-a", "ot-reactivate", "user-1")
	if _, err := NewAttendanceServiceWithOrgID(db, "org-a").SyncRecords("org-a", attendanceRecords("user-1"), nil); err != nil {
		t.Fatalf("sync initial attendance: %v", err)
	}
	svc := NewOvertimeMatchingServiceWithOrgID(db, "org-a")
	match, _ := svc.matchRepo.FindByApprovalID(approval.ID)
	if err := svc.matchRepo.UpdateSyncStatus(match.ID, "success", "forward", ""); err != nil {
		t.Fatalf("mark forward synced: %v", err)
	}
	if err := svc.matchRepo.UpdateStatus(match.ID, "synced", "forward synced"); err != nil {
		t.Fatalf("mark match synced: %v", err)
	}
	now := time.Now()
	if err := db.Create(&database.OvertimeSyncHistory{
		OrgID: "org-a", UserID: "user-1", WorkDate: "2026-08-05", ApprovalID: approval.ID,
		EffectiveOvertimeMinutes: 180, SyncRequestID: "forward", SyncMode: "incremental", SyncedAt: &now,
	}).Error; err != nil {
		t.Fatalf("seed forward history: %v", err)
	}

	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "true")
	var calls []absoluteSyncCall
	configureAbsoluteSync(svc, &calls, errors.New("temporary rollback failure"), nil)
	if err := svc.RollbackApprovalMatch(approval.ID); err == nil {
		t.Fatal("rollback should fail externally")
	}
	if err := svc.MatchApproval(approval.ID); err != nil {
		t.Fatalf("reactivation absolute recalibration: %v", err)
	}
	if err := svc.MatchApproval(approval.ID); err != nil {
		t.Fatalf("duplicate reactivation: %v", err)
	}
	if len(calls) != 2 || calls[0].TotalMinutes != 0 || calls[1].TotalMinutes != 180 {
		t.Fatalf("rollback/reactivation calls = %#v", calls)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "credit"); got != 2 {
		t.Fatalf("credit count = %d, want original + one reactivation", got)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "rollback"); got != 1 {
		t.Fatalf("rollback count = %d, want 1", got)
	}
	var history database.OvertimeSyncHistory
	_ = db.Where("org_id = ? AND user_id = ?", "org-a", "user-1").First(&history).Error
	if history.SyncMode != "reactivation" || history.EffectiveOvertimeMinutes != 180 {
		t.Fatalf("reactivation history mode=%q minutes=%d", history.SyncMode, history.EffectiveOvertimeMinutes)
	}
}

func TestAbsoluteRecalibrationFailureDoesNotPolluteSameYearMatches(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "true")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	target := database.OvertimeMatchResult{
		OrgID: "org-a", UserID: "user-1", WorkDate: "2026-08-05", MatchRef: "failed-target",
		ApprovalID: 801, EffectiveOvertimeMinutes: 60, MatchStatus: "rollback_failed",
		LocalBalanceStatus: "success", DingtalkSyncStatus: "rollback_failed", RollbackDingtalkSyncStatus: "rollback_failed",
	}
	peer := database.OvertimeMatchResult{
		OrgID: "org-a", UserID: "user-1", WorkDate: "2026-07-05", MatchRef: "successful-peer",
		ApprovalID: 802, EffectiveOvertimeMinutes: 120, MatchStatus: "synced",
		LocalBalanceStatus: "success", DingtalkSyncStatus: "success",
	}
	if err := db.Create(&target).Error; err != nil {
		t.Fatalf("create target match: %v", err)
	}
	if err := db.Create(&peer).Error; err != nil {
		t.Fatalf("create peer match: %v", err)
	}

	svc := NewOvertimeMatchingServiceWithOrgID(db, "org-a")
	var calls []absoluteSyncCall
	configureAbsoluteSync(svc, &calls, errors.New("temporary absolute sync failure"))
	if err := svc.forceAbsoluteBalanceRecalibration(&target, "retry"); err == nil {
		t.Fatal("absolute recalibration should fail")
	}

	var targetAfter, peerAfter database.OvertimeMatchResult
	if err := db.First(&targetAfter, target.ID).Error; err != nil {
		t.Fatalf("reload target match: %v", err)
	}
	if err := db.First(&peerAfter, peer.ID).Error; err != nil {
		t.Fatalf("reload peer match: %v", err)
	}
	if targetAfter.MatchStatus != "rollback_failed" || targetAfter.DingtalkSyncStatus != "rollback_failed" {
		t.Fatalf("target failure states = %#v", targetAfter)
	}
	if peerAfter.MatchStatus != "synced" || peerAfter.DingtalkSyncStatus != "success" || peerAfter.DingtalkSyncError != "" {
		t.Fatalf("same-year successful peer was polluted: %#v", peerAfter)
	}
}

func TestLocalOnlyRollbackReactivationSucceedsWhenDingTalkSyncDisabled(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := openLeaveJobsDB(t)
	migrateRecalcTables(t, db)
	seedOvertimeRule(t, db, "org-a")
	approval := seedRetryableOvertimeApproval(t, db, "org-a", "ot-local-reactivate", "user-1")
	if _, err := NewAttendanceServiceWithOrgID(db, "org-a").SyncRecords("org-a", attendanceRecords("user-1"), nil); err != nil {
		t.Fatalf("sync initial attendance: %v", err)
	}

	svc := NewOvertimeMatchingServiceWithOrgID(db, "org-a")
	if err := svc.RollbackApprovalMatch(approval.ID); err != nil {
		t.Fatalf("local-only rollback: %v", err)
	}
	if err := svc.MatchApproval(approval.ID); err != nil {
		t.Fatalf("local-only reactivation: %v", err)
	}
	if err := svc.MatchApproval(approval.ID); err != nil {
		t.Fatalf("duplicate local-only reactivation: %v", err)
	}

	match, err := svc.matchRepo.FindByApprovalID(approval.ID)
	if err != nil {
		t.Fatalf("reload local-only reactivation: %v", err)
	}
	if match.MatchStatus == "rollback_failed" || match.LocalBalanceStatus != "success" || match.DingtalkSyncStatus != "skipped" || match.RollbackDingtalkSyncStatus != "" {
		t.Fatalf("local-only reactivation states = %#v", match)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "credit"); got != 2 {
		t.Fatalf("credit count = %d, want original + one reactivation", got)
	}
	if got := countLedgerType(t, db, "org-a", "user-1", "rollback"); got != 1 {
		t.Fatalf("rollback count = %d, want 1", got)
	}
	var historyCount int64
	if err := db.Model(&database.OvertimeSyncHistory{}).Where("org_id = ? AND user_id = ?", "org-a", "user-1").Count(&historyCount).Error; err != nil {
		t.Fatalf("count sync history: %v", err)
	}
	if historyCount != 0 {
		t.Fatalf("DingTalk sync history count = %d, want 0", historyCount)
	}
}
