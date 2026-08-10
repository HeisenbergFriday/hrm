package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
)

func TestApprovalSyncRunReconcilesHistoricalLeaveAndOvertimeIdempotently(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(
		&database.Approval{},
		&database.Attendance{},
		&database.AnnualLeaveGrant{},
		&database.AnnualLeaveConsumeLog{},
		&database.AnnualLeaveConsumeRequest{},
		&database.OvertimeRuleConfig{},
		&database.OvertimeMatchResult{},
		&database.CompensatoryLeaveLedger{},
		&database.OvertimeSyncHistory{},
		&database.OvertimeSupplementaryRequest{},
	); err != nil {
		t.Fatalf("migrate approval reconciliation tables: %v", err)
	}

	grants := []database.AnnualLeaveGrant{
		{OrgID: "org-a", UserID: "user-leave", Year: 2025, Quarter: 1, GrantedDays: 5, RemainingDays: 5, GrantType: "normal"},
		{OrgID: "org-b", UserID: "user-leave", Year: 2025, Quarter: 1, GrantedDays: 8, RemainingDays: 8, GrantType: "normal"},
	}
	if err := db.Create(&grants).Error; err != nil {
		t.Fatalf("create grants: %v", err)
	}
	attendances := []database.Attendance{
		{OrgID: "org-a", UserID: "user-overtime", UserName: "员工甲", CheckTime: time.Date(2025, 1, 15, 18, 0, 0, 0, time.Local), CheckType: "OnDuty"},
		{OrgID: "org-a", UserID: "user-overtime", UserName: "员工甲", CheckTime: time.Date(2025, 1, 15, 21, 0, 0, 0, time.Local), CheckType: "OffDuty"},
		{OrgID: "org-b", UserID: "user-overtime", UserName: "外组织员工", CheckTime: time.Date(2025, 1, 15, 17, 0, 0, 0, time.Local), CheckType: "OnDuty"},
		{OrgID: "org-b", UserID: "user-overtime", UserName: "外组织员工", CheckTime: time.Date(2025, 1, 15, 23, 0, 0, 0, time.Local), CheckType: "OffDuty"},
	}
	if err := db.Create(&attendances).Error; err != nil {
		t.Fatalf("create attendances: %v", err)
	}
	foreignApproval := database.Approval{
		OrgID: "org-b", ProcessID: "historical-leave", Title: "外组织审批", ApplicantID: "foreign-user",
		Status: "RUNNING", CreateTime: time.Date(2025, 1, 9, 9, 0, 0, 0, time.Local),
	}
	if err := db.Omit("FinishTime").Create(&foreignApproval).Error; err != nil {
		t.Fatalf("create foreign approval: %v", err)
	}

	instances := []dingtalk.ApprovalInstance{
		{
			ProcessInstanceID: "historical-leave", OriginatorUserID: "user-leave", Title: "年假审批",
			Status: "COMPLETED", Result: "agree", CreateTime: "2025-01-10 09:00:00", FinishTime: "2025-01-11 10:00:00",
			FormValues: []map[string]interface{}{{"name": "天数", "value": "1.5"}, {"name": "开始日期", "value": "2025-01-20"}, {"name": "结束日期", "value": "2025-01-20"}},
		},
		{
			ProcessInstanceID: "historical-overtime", OriginatorUserID: "user-overtime", Title: "加班审批",
			Status: "COMPLETED", Result: "agree", CreateTime: "2025-01-16 09:00:00", FinishTime: "2025-01-16 10:00:00",
			FormValues: []map[string]interface{}{
				{"name": "加班开始时间", "value": "2025-01-15 18:00:00"},
				{"name": "加班结束时间", "value": "2025-01-15 21:00:00"},
			},
		},
	}
	svc := NewApprovalSyncService(db, "org-a")
	svc.resolveName = nil
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: instances}, nil
	}
	plan := ApprovalSyncPlan{ProcessCodes: []string{"PROC-HISTORY"}, StartDate: "2025-01-01", EndDate: "2025-01-31"}
	for run := 1; run <= 2; run++ {
		result := svc.Run(context.Background(), plan, "historical-run")
		wantReconciled := 2
		wantSkipped := 0
		if run == 2 {
			wantReconciled = 0
			wantSkipped = 2
		}
		if result.Status != ApprovalSyncStatusSuccess || result.SuccessCount != 2 || result.ReconciledCount != wantReconciled || result.ReconcileSkippedCount != wantSkipped || result.ReconcileFailCount != 0 {
			t.Fatalf("run %d result = %#v", run, result)
		}
	}

	var orgAGrant database.AnnualLeaveGrant
	if err := db.Where("org_id = ? AND user_id = ?", "org-a", "user-leave").First(&orgAGrant).Error; err != nil {
		t.Fatalf("load org-a grant: %v", err)
	}
	if orgAGrant.UsedDays != 1.5 || orgAGrant.RemainingDays != 3.5 {
		t.Fatalf("org-a grant used=%v remaining=%v, want 1.5/3.5", orgAGrant.UsedDays, orgAGrant.RemainingDays)
	}
	var orgBGrant database.AnnualLeaveGrant
	if err := db.Where("org_id = ? AND user_id = ?", "org-b", "user-leave").First(&orgBGrant).Error; err != nil {
		t.Fatalf("load org-b grant: %v", err)
	}
	if orgBGrant.UsedDays != 0 || orgBGrant.RemainingDays != 8 {
		t.Fatalf("foreign grant changed: %#v", orgBGrant)
	}

	var consumeLogs []database.AnnualLeaveConsumeLog
	if err := db.Where("org_id = ?", "org-a").Find(&consumeLogs).Error; err != nil {
		t.Fatalf("load consume logs: %v", err)
	}
	if len(consumeLogs) != 1 || consumeLogs[0].ApprovalRef != "approval:historical-leave" || consumeLogs[0].RequestRef != "approval:historical-leave" {
		t.Fatalf("consume logs = %#v", consumeLogs)
	}

	var matches []database.OvertimeMatchResult
	if err := db.Where("org_id = ?", "org-a").Find(&matches).Error; err != nil {
		t.Fatalf("load matches: %v", err)
	}
	if len(matches) != 1 || matches[0].WorkDate != "2025-01-15" || matches[0].EffectiveOvertimeMinutes != 180 {
		t.Fatalf("matches = %#v", matches)
	}
	var ledgers []database.CompensatoryLeaveLedger
	if err := db.Where("org_id = ? AND ledger_type = ?", "org-a", "credit").Find(&ledgers).Error; err != nil {
		t.Fatalf("load ledgers: %v", err)
	}
	if len(ledgers) != 1 || ledgers[0].CreditMinutes != 180 || ledgers[0].EffectiveDate != "2025-01-15" {
		t.Fatalf("ledgers = %#v", ledgers)
	}
	var foreignMatches, foreignLedgers int64
	if err := db.Model(&database.OvertimeMatchResult{}).Where("org_id = ?", "org-b").Count(&foreignMatches).Error; err != nil {
		t.Fatalf("count foreign matches: %v", err)
	}
	if err := db.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ?", "org-b").Count(&foreignLedgers).Error; err != nil {
		t.Fatalf("count foreign ledgers: %v", err)
	}
	if foreignMatches != 0 || foreignLedgers != 0 {
		t.Fatalf("foreign side effects matches=%d ledgers=%d", foreignMatches, foreignLedgers)
	}
	var savedForeignApproval database.Approval
	if err := db.Where("org_id = ? AND process_id = ?", "org-b", "historical-leave").First(&savedForeignApproval).Error; err != nil {
		t.Fatalf("load foreign approval: %v", err)
	}
	if savedForeignApproval.Status != "RUNNING" || savedForeignApproval.Title != "外组织审批" {
		t.Fatalf("foreign approval changed: %#v", savedForeignApproval)
	}
}

func TestApprovalSyncRunSkipsNonEffectiveApprovalStates(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.Approval{}, &database.AnnualLeaveGrant{}, &database.AnnualLeaveConsumeLog{}, &database.AnnualLeaveConsumeRequest{}, &database.OvertimeRuleConfig{}, &database.OvertimeMatchResult{}, &database.CompensatoryLeaveLedger{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	grant := database.AnnualLeaveGrant{OrgID: "org-a", UserID: "user-1", Year: 2025, Quarter: 1, GrantedDays: 10, RemainingDays: 10, GrantType: "normal"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("create grant: %v", err)
	}
	instances := []dingtalk.ApprovalInstance{
		{ProcessInstanceID: "refused", OriginatorUserID: "user-1", Title: "年假审批", Status: "COMPLETED", Result: "refuse", FormValues: []map[string]interface{}{{"name": "天数", "value": "1"}}},
		{ProcessInstanceID: "terminated", OriginatorUserID: "user-1", Title: "年假审批", Status: "TERMINATED", Result: "agree", FormValues: []map[string]interface{}{{"name": "天数", "value": "1"}}},
		{ProcessInstanceID: "canceled", OriginatorUserID: "user-1", Title: "加班审批", Status: "CANCELED", Result: "agree", FormValues: []map[string]interface{}{{"name": "加班开始时间", "value": "2025-01-15 18:00:00"}, {"name": "加班结束时间", "value": "2025-01-15 21:00:00"}}},
		{ProcessInstanceID: "running", OriginatorUserID: "user-1", Title: "加班审批", Status: "RUNNING", Result: "agree", FormValues: []map[string]interface{}{{"name": "加班开始时间", "value": "2025-01-15 18:00:00"}, {"name": "加班结束时间", "value": "2025-01-15 21:00:00"}}},
	}
	svc := NewApprovalSyncService(db, "org-a")
	svc.resolveName = nil
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: instances}, nil
	}
	result := svc.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"PROC-STATES"}}, "state-run")
	if result.Status != ApprovalSyncStatusSuccess || result.ReconcileSkippedCount != 4 || result.ReconciledCount != 0 || result.ReconcileFailCount != 0 {
		t.Fatalf("result = %#v", result)
	}
	var logs, matches, ledgers int64
	if err := db.Model(&database.AnnualLeaveConsumeLog{}).Count(&logs).Error; err != nil {
		t.Fatalf("count consume logs: %v", err)
	}
	if err := db.Model(&database.OvertimeMatchResult{}).Count(&matches).Error; err != nil {
		t.Fatalf("count matches: %v", err)
	}
	if err := db.Model(&database.CompensatoryLeaveLedger{}).Count(&ledgers).Error; err != nil {
		t.Fatalf("count ledgers: %v", err)
	}
	if logs != 0 || matches != 0 || ledgers != 0 {
		t.Fatalf("unexpected side effects logs=%d matches=%d ledgers=%d", logs, matches, ledgers)
	}
}

type selectiveApprovalReconciler struct {
	failProcessID string
}

func (r *selectiveApprovalReconciler) Reconcile(_ context.Context, approval *database.Approval) (ApprovalBusinessReconcileResult, error) {
	if approval.ProcessID == r.failProcessID {
		return ApprovalBusinessReconcileResult{}, errors.New("database failed secret=must-not-leak")
	}
	return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusApplied}, nil
}

func TestApprovalSyncRunContinuesAfterReconciliationFailureAndReturnsPartial(t *testing.T) {
	svc, store := newApprovalSyncServiceStub("org-a")
	svc.reconciler = &selectiveApprovalReconciler{failProcessID: "reconcile-fail"}
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{
			{ProcessInstanceID: "reconcile-fail", Status: "COMPLETED", Result: "agree"},
			{ProcessInstanceID: "reconcile-ok", Status: "COMPLETED", Result: "agree"},
		}}, nil
	}
	result := svc.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"PROC"}}, "partial-reconcile")
	if result.Status != ApprovalSyncStatusPartial || result.SuccessCount != 2 || result.ReconciledCount != 1 || result.ReconcileFailCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	if len(store.records) != 2 {
		t.Fatalf("persisted approvals = %d, want 2", len(store.records))
	}
	process := result.Processes[0]
	if process.ErrorCode != ApprovalSyncErrorReconcile || strings.Contains(process.Error, "secret") {
		t.Fatalf("unsafe process result = %#v", process)
	}
}

func TestApprovalSyncRunRetriesFailedAnnualLeaveReconciliationSafely(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.Approval{}, &database.AnnualLeaveGrant{}, &database.AnnualLeaveConsumeLog{}, &database.AnnualLeaveConsumeRequest{}, &database.OvertimeRuleConfig{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	instance := dingtalk.ApprovalInstance{
		ProcessInstanceID: "leave-retry", OriginatorUserID: "user-1", Title: "年假审批", Status: "COMPLETED", Result: "agree",
		FormValues: []map[string]interface{}{{"name": "天数", "value": "2"}, {"name": "开始日期", "value": "2025-02-10"}, {"name": "结束日期", "value": "2025-02-10"}},
	}
	svc := NewApprovalSyncService(db, "org-a")
	svc.resolveName = nil
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{instance}}, nil
	}
	plan := ApprovalSyncPlan{ProcessCodes: []string{"PROC-LEAVE"}}
	first := svc.Run(context.Background(), plan, "retry-1")
	if first.Status != ApprovalSyncStatusPartial || first.SuccessCount != 1 || first.ReconcileFailCount != 1 {
		t.Fatalf("first result = %#v", first)
	}
	var approvals int64
	if err := db.Model(&database.Approval{}).Where("org_id = ?", "org-a").Count(&approvals).Error; err != nil || approvals != 1 {
		t.Fatalf("approval persisted count=%d err=%v", approvals, err)
	}
	grant := database.AnnualLeaveGrant{OrgID: "org-a", UserID: "user-1", Year: 2025, Quarter: 1, GrantedDays: 5, RemainingDays: 5, GrantType: "normal"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatalf("create retry grant: %v", err)
	}
	for _, requestID := range []string{"retry-2", "retry-3"} {
		result := svc.Run(context.Background(), plan, requestID)
		wantReconciled := 1
		wantSkipped := 0
		if requestID == "retry-3" {
			wantReconciled = 0
			wantSkipped = 1
		}
		if result.Status != ApprovalSyncStatusSuccess || result.ReconciledCount != wantReconciled || result.ReconcileSkippedCount != wantSkipped || result.ReconcileFailCount != 0 {
			t.Fatalf("%s result = %#v", requestID, result)
		}
	}
	var saved database.AnnualLeaveGrant
	if err := db.First(&saved, grant.ID).Error; err != nil {
		t.Fatalf("load grant: %v", err)
	}
	if saved.UsedDays != 2 || saved.RemainingDays != 3 {
		t.Fatalf("grant after retries = %#v", saved)
	}
	var logs int64
	if err := db.Model(&database.AnnualLeaveConsumeLog{}).Count(&logs).Error; err != nil || logs != 1 {
		t.Fatalf("consume logs=%d err=%v", logs, err)
	}
}
