package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"

	"gorm.io/gorm"
)

func migrateApprovalReconciliationStateTests(t *testing.T) *gorm.DB {
	t.Helper()
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(
		&database.Approval{},
		&database.Attendance{},
		&database.AnnualLeaveGrant{},
		&database.AnnualLeaveConsumeLog{},
		&database.AnnualLeaveConsumeRequest{},
		&database.OvertimeRuleConfig{},
		&database.OvertimeMatchResult{},
		&database.OvertimeSyncHistory{},
		&database.OvertimeSupplementaryRequest{},
		&database.CompensatoryLeaveLedger{},
	); err != nil {
		t.Fatalf("migrate reconciliation state tables: %v", err)
	}
	return db
}

func TestAnnualLeaveApprovalReversalAndReactivationAreAuditableAndIdempotent(t *testing.T) {
	db := migrateApprovalReconciliationStateTests(t)
	grant := database.AnnualLeaveGrant{OrgID: "org-a", UserID: "user-1", Year: 2025, Quarter: 1, GrantedDays: 3, RemainingDays: 3, GrantType: "normal"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	approval := database.Approval{
		OrgID: "org-a", ProcessID: "leave-transition", ApplicantID: "user-1", Title: "年假审批", Status: "COMPLETED",
		Content:   map[string]interface{}{"天数": "1", "开始日期": "2025-02-10", "结束日期": "2025-02-10"},
		Extension: map[string]interface{}{"result": "agree"},
	}
	if err := db.Create(&approval).Error; err != nil {
		t.Fatal(err)
	}
	reconciler := NewApprovalBusinessReconciliationService(db, "org-a")
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusApplied)

	approval.Status = "CANCELED"
	approval.Extension = map[string]interface{}{"result": "agree"}
	if err := db.Save(&approval).Error; err != nil {
		t.Fatal(err)
	}
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusReversed)
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusSkipped)
	assertGrantBalance(t, db, grant.ID, 0, 3)

	approval.Status = "COMPLETED"
	approval.Extension = map[string]interface{}{"result": "agree"}
	if err := db.Save(&approval).Error; err != nil {
		t.Fatal(err)
	}
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusApplied)
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusSkipped)
	assertGrantBalance(t, db, grant.ID, 1, 2)
	approval.Status = "TERMINATED"
	approval.Extension = map[string]interface{}{"result": "refuse"}
	if err := db.Save(&approval).Error; err != nil {
		t.Fatal(err)
	}
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusReversed)
	assertGrantBalance(t, db, grant.ID, 0, 3)

	var logs []database.AnnualLeaveConsumeLog
	if err := db.Where("org_id = ? AND approval_ref = ?", "org-a", approvalBusinessReference(approval.ProcessID)).Order("id").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 4 || logs[0].EntryType != "consume" || logs[1].EntryType != "reversal" || logs[2].EntryType != "consume" || logs[2].OperationNo != 2 || logs[3].EntryType != "reversal" || logs[3].OperationNo != 2 {
		t.Fatalf("unexpected annual leave audit logs: %#v", logs)
	}
}

func TestAnnualLeaveBusinessDateYearQuarterAndCrossYearSplit(t *testing.T) {
	db := migrateApprovalReconciliationStateTests(t)
	grants := []database.AnnualLeaveGrant{
		{OrgID: "org-a", UserID: "history-user", Year: 2025, Quarter: 1, GrantedDays: 2, RemainingDays: 2, GrantType: "normal"},
		{OrgID: "org-a", UserID: "history-user", Year: 2026, Quarter: 1, GrantedDays: 5, RemainingDays: 5, GrantType: "normal"},
		{OrgID: "org-a", UserID: "cross-user", Year: 2025, Quarter: 4, GrantedDays: 2, RemainingDays: 2, GrantType: "normal"},
		{OrgID: "org-a", UserID: "cross-user", Year: 2026, Quarter: 1, GrantedDays: 2, RemainingDays: 2, GrantType: "normal"},
		{OrgID: "org-a", UserID: "history-user", Year: 2025, Quarter: 2, GrantedDays: 4, RemainingDays: 4, GrantType: "normal"},
	}
	if err := db.Create(&grants).Error; err != nil {
		t.Fatal(err)
	}
	reconciler := NewApprovalBusinessReconciliationService(db, "org-a")
	history := createAnnualLeaveApproval(t, db, "org-a", "history-2025", "history-user", 1, "2025-03-10", "2025-03-10")
	assertReconcileStatus(t, reconciler, history, ApprovalReconcileStatusApplied)
	assertGrantBalance(t, db, grants[0].ID, 1, 1)
	assertGrantBalance(t, db, grants[1].ID, 0, 5)
	assertGrantBalance(t, db, grants[4].ID, 0, 4)

	cross := createAnnualLeaveApproval(t, db, "org-a", "cross-year", "cross-user", 2, "2025-12-31", "2026-01-01")
	assertReconcileStatus(t, reconciler, cross, ApprovalReconcileStatusApplied)
	assertGrantBalance(t, db, grants[2].ID, 1, 1)
	assertGrantBalance(t, db, grants[3].ID, 1, 1)
	var logs []database.AnnualLeaveConsumeLog
	if err := db.Where("org_id = ? AND approval_ref = ? AND entry_type = ?", "org-a", approvalBusinessReference(cross.ProcessID), "consume").Order("business_start_date").Find(&logs).Error; err != nil {
		t.Fatal(err)
	}
	if len(logs) != 2 || logs[0].BusinessStartDate != "2025-12-31" || logs[0].Days != 1 || logs[1].BusinessStartDate != "2026-01-01" || logs[1].Days != 1 {
		t.Fatalf("cross-year logs = %#v", logs)
	}
}

func TestAnnualLeaveBusinessPeriodParsesDingTalkComponentJSON(t *testing.T) {
	content := map[string]interface{}{
		"请假": `[{"props":{"bizAlias":"startTime"},"value":"2025-12-31 09:00"},{"props":{"bizAlias":"finishTime"},"value":"2026-01-01 18:00"}]`,
	}
	start, end, err := parseApprovalLeaveBusinessPeriod(content)
	if err != nil {
		t.Fatal(err)
	}
	if start.In(ApprovalSyncLocation()).Format("2006-01-02") != "2025-12-31" || end.In(ApprovalSyncLocation()).Format("2006-01-02") != "2026-01-01" {
		t.Fatalf("start=%v end=%v", start, end)
	}
}

func TestAnnualLeaveMissingBusinessDateFailsClosedAndKeepsApproval(t *testing.T) {
	db := migrateApprovalReconciliationStateTests(t)
	grant := database.AnnualLeaveGrant{OrgID: "org-a", UserID: "user-1", Year: 2025, Quarter: 1, GrantedDays: 2, RemainingDays: 2, GrantType: "normal"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	approval := database.Approval{OrgID: "org-a", ProcessID: "missing-date", ApplicantID: "user-1", Title: "年假审批", Status: "COMPLETED", Content: map[string]interface{}{"天数": "1", "开始日期": "bad-date"}, Extension: map[string]interface{}{"result": "agree"}}
	if err := db.Create(&approval).Error; err != nil {
		t.Fatal(err)
	}
	result, err := NewApprovalBusinessReconciliationService(db, "org-a").Reconcile(context.Background(), &approval)
	if err == nil || result.Status != ApprovalReconcileStatusFailed {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	assertGrantBalance(t, db, grant.ID, 0, 2)
	var approvalCount, requestCount int64
	db.Model(&database.Approval{}).Where("org_id = ? AND process_id = ?", "org-a", approval.ProcessID).Count(&approvalCount)
	db.Model(&database.AnnualLeaveConsumeRequest{}).Where("org_id = ?", "org-a").Count(&requestCount)
	if approvalCount != 1 || requestCount != 0 {
		t.Fatalf("approvalCount=%d requestCount=%d", approvalCount, requestCount)
	}
}

func TestApprovalSyncMarksPartialWhenAnnualLeaveRollbackFailsWithoutRevertingApproval(t *testing.T) {
	db := migrateApprovalReconciliationStateTests(t)
	grant := database.AnnualLeaveGrant{OrgID: "org-a", UserID: "user-1", Year: 2025, Quarter: 1, GrantedDays: 2, RemainingDays: 2, GrantType: "normal"}
	if err := db.Create(&grant).Error; err != nil {
		t.Fatal(err)
	}
	instance := dingtalk.ApprovalInstance{
		ProcessInstanceID: "rollback-failure", OriginatorUserID: "user-1", Title: "年假审批", Status: "COMPLETED", Result: "agree",
		FormValues: []map[string]interface{}{{"name": "天数", "value": "1"}, {"name": "开始日期", "value": "2025-02-10"}, {"name": "结束日期", "value": "2025-02-10"}},
	}
	svc := NewApprovalSyncService(db, "org-a")
	svc.resolveName = nil
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{instance}}, nil
	}
	plan := ApprovalSyncPlan{ProcessCodes: []string{"PROC-LEAVE"}}
	if result := svc.Run(context.Background(), plan, "apply"); result.Status != ApprovalSyncStatusSuccess {
		t.Fatalf("apply result=%#v", result)
	}
	if err := db.Delete(&database.AnnualLeaveGrant{}, grant.ID).Error; err != nil {
		t.Fatal(err)
	}
	instance.Status = "CANCELED"
	result := svc.Run(context.Background(), plan, "rollback")
	if result.Status != ApprovalSyncStatusPartial || result.ReconcileFailCount != 1 || result.Processes[0].ErrorCode != ApprovalSyncErrorReconcile {
		t.Fatalf("rollback result=%#v", result)
	}
	var saved database.Approval
	if err := db.Where("org_id = ? AND process_id = ?", "org-a", instance.ProcessInstanceID).First(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Status != "CANCELED" {
		t.Fatalf("approval status=%s, want CANCELED", saved.Status)
	}
	var request database.AnnualLeaveConsumeRequest
	if err := db.Where("org_id = ? AND request_ref = ?", "org-a", approvalBusinessReference(instance.ProcessInstanceID)).First(&request).Error; err != nil {
		t.Fatal(err)
	}
	if request.Status != "applied" {
		t.Fatalf("request status=%s, want applied for retry", request.Status)
	}
}

func TestOvertimeTerminationRollbackAndLateAttendanceRetry(t *testing.T) {
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "false")
	db := migrateApprovalReconciliationStateTests(t)
	loc := ApprovalSyncLocation()
	approval := database.Approval{
		OrgID: "org-a", ProcessID: "late-overtime", ApplicantID: "overtime-user", Title: "加班审批", Status: "COMPLETED",
		Content:   map[string]interface{}{"加班开始时间": "2025-05-10 18:00:00", "加班结束时间": "2025-05-10 21:00:00"},
		Extension: map[string]interface{}{"result": "agree"},
	}
	if err := db.Create(&approval).Error; err != nil {
		t.Fatal(err)
	}
	reconciler := NewApprovalBusinessReconciliationService(db, "org-a")
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusRetryable)
	var first database.OvertimeMatchResult
	if err := db.Where("org_id = ? AND approval_id = ?", "org-a", approval.ID).First(&first).Error; err != nil {
		t.Fatal(err)
	}
	if first.MatchStatus != "no_clock_record" {
		t.Fatalf("first status=%s", first.MatchStatus)
	}
	attendances := []database.Attendance{
		{OrgID: "org-a", UserID: approval.ApplicantID, CheckTime: time.Date(2025, 5, 10, 18, 0, 0, 0, loc), CheckType: "OnDuty"},
		{OrgID: "org-a", UserID: approval.ApplicantID, CheckTime: time.Date(2025, 5, 10, 21, 0, 0, 0, loc), CheckType: "OffDuty"},
	}
	if err := db.Create(&attendances).Error; err != nil {
		t.Fatal(err)
	}
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusApplied)
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusSkipped)
	var rematched database.OvertimeMatchResult
	if err := db.First(&rematched, first.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rematched.ID != first.ID || rematched.EffectiveOvertimeMinutes != 180 || rematched.LocalBalanceStatus != "success" {
		t.Fatalf("rematched=%#v", rematched)
	}
	var creditCount int64
	db.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ? AND source_match_id = ? AND ledger_type = ?", "org-a", first.ID, "credit").Count(&creditCount)
	if creditCount != 1 {
		t.Fatalf("creditCount=%d", creditCount)
	}
	var supplementary database.OvertimeSupplementaryRequest
	if err := db.Where("org_id = ? AND match_result_id = ?", "org-a", first.ID).First(&supplementary).Error; err != nil || supplementary.Status != "resolved" {
		t.Fatalf("supplementary=%#v err=%v", supplementary, err)
	}

	approval.Status = "TERMINATED"
	approval.Extension = map[string]interface{}{"result": "refuse"}
	if err := db.Save(&approval).Error; err != nil {
		t.Fatal(err)
	}
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusReversed)
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusSkipped)
	var net int
	db.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ? AND source_match_id = ?", "org-a", first.ID).
		Select("COALESCE(SUM(credit_minutes)-SUM(debit_minutes),0)").Scan(&net)
	if net != 0 {
		t.Fatalf("overtime ledger net=%d", net)
	}
	t.Setenv("DINGTALK_COMP_TIME_SYNC_ENABLED", "true")
	absoluteSyncCalls := 0
	reconciler.overtimeService.setAbsoluteCompTimeQuotaFunc = func(orgID, userID string, year, totalMinutes int, _ string) error {
		absoluteSyncCalls++
		if orgID != "org-a" || userID != approval.ApplicantID || year != 2025 || totalMinutes != 180 {
			t.Fatalf("absolute sync args = org:%s user:%s year:%d minutes:%d", orgID, userID, year, totalMinutes)
		}
		return nil
	}
	approval.Status = "COMPLETED"
	approval.Extension = map[string]interface{}{"result": "agree"}
	if err := db.Save(&approval).Error; err != nil {
		t.Fatal(err)
	}
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusApplied)
	assertReconcileStatus(t, reconciler, &approval, ApprovalReconcileStatusSkipped)
	db.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ? AND source_match_id = ?", "org-a", first.ID).
		Select("COALESCE(SUM(credit_minutes)-SUM(debit_minutes),0)").Scan(&net)
	if net != 180 {
		t.Fatalf("reactivated overtime ledger net=%d", net)
	}
	db.Model(&database.CompensatoryLeaveLedger{}).Where("org_id = ? AND source_match_id = ? AND ledger_type = ?", "org-a", first.ID, "credit").Count(&creditCount)
	if creditCount != 2 {
		t.Fatalf("reactivated creditCount=%d, want 2 audit credits across two operations", creditCount)
	}
	if absoluteSyncCalls != 1 {
		t.Fatalf("reactivation absolute sync calls=%d, want 1", absoluteSyncCalls)
	}
}

func TestAnnualLeaveConcurrentRequestGateAndTenantIsolation(t *testing.T) {
	db := migrateApprovalReconciliationStateTests(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(8)
	grants := []database.AnnualLeaveGrant{
		{OrgID: "org-a", UserID: "same-user", Year: 2025, Quarter: 1, GrantedDays: 1, RemainingDays: 1, GrantType: "normal"},
		{OrgID: "org-a", UserID: "same-user", Year: 2025, Quarter: 2, GrantedDays: 2, RemainingDays: 2, GrantType: "normal"},
		{OrgID: "org-b", UserID: "same-user", Year: 2025, Quarter: 1, GrantedDays: 3, RemainingDays: 3, GrantType: "normal"},
	}
	if err := db.Create(&grants).Error; err != nil {
		t.Fatal(err)
	}
	loc := ApprovalSyncLocation()
	start := time.Date(2025, 4, 10, 0, 0, 0, 0, loc)
	const workers = 8
	startGate := make(chan struct{})
	errs := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startGate
			_, callErr := NewAnnualLeaveGrantServiceWithOrgID(db, "org-a").ConsumeAnnualLeaveForPeriod("same-user", 2, "approval:same-process", "并发审批", start, start)
			errs <- callErr
		}()
	}
	close(startGate)
	wg.Wait()
	close(errs)
	for callErr := range errs {
		if callErr != nil {
			t.Fatalf("concurrent consume: %v", callErr)
		}
	}
	assertGrantBalance(t, db, grants[0].ID, 1, 0)
	assertGrantBalance(t, db, grants[1].ID, 1, 1)
	if _, err := NewAnnualLeaveGrantServiceWithOrgID(db, "org-b").ConsumeAnnualLeaveForPeriod("same-user", 2, "approval:same-process", "另一企业同键", start, start); err != nil {
		t.Fatal(err)
	}
	assertGrantBalance(t, db, grants[2].ID, 2, 1)
	var gates int64
	db.Model(&database.AnnualLeaveConsumeRequest{}).Where("request_ref = ?", "approval:same-process").Count(&gates)
	if gates != 2 {
		t.Fatalf("request gates=%d, want one per org", gates)
	}
}

func assertReconcileStatus(t *testing.T, reconciler *ApprovalBusinessReconciliationService, approval *database.Approval, want string) {
	t.Helper()
	result, err := reconciler.Reconcile(context.Background(), approval)
	if err != nil || result.Status != want {
		t.Fatalf("reconcile %s: result=%#v err=%v want=%s", approval.ProcessID, result, err, want)
	}
}

func createAnnualLeaveApproval(t *testing.T, db *gorm.DB, orgID, processID, userID string, days float64, start, end string) *database.Approval {
	t.Helper()
	approval := &database.Approval{
		OrgID: orgID, ProcessID: processID, ApplicantID: userID, Title: "年假审批", Status: "COMPLETED",
		Content:   map[string]interface{}{"天数": fmt.Sprintf("%.2f", days), "开始日期": start, "结束日期": end},
		Extension: map[string]interface{}{"result": "agree"},
	}
	if err := db.Create(approval).Error; err != nil {
		t.Fatal(err)
	}
	return approval
}

func assertGrantBalance(t *testing.T, db *gorm.DB, grantID uint, used, remaining float64) {
	t.Helper()
	var grant database.AnnualLeaveGrant
	if err := db.First(&grant, grantID).Error; err != nil {
		t.Fatal(err)
	}
	if grant.UsedDays != used || grant.RemainingDays != remaining {
		t.Fatalf("grant %d used=%v remaining=%v want=%v/%v", grantID, grant.UsedDays, grant.RemainingDays, used, remaining)
	}
}
