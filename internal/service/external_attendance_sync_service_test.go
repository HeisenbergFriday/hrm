package service

import (
	"context"
	"database/sql"
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

func TestBuildAttendanceSourceRowKeyStable(t *testing.T) {
	row := repository.ExternalAttendanceRow{
		UserID:     "u1",
		WorkDate:   "2026-07-01",
		CheckType:  "OnDuty",
		SourceType: "USER",
		PlanID:     "p1",
	}
	ts := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	row.UserCheckTime = sql.NullTime{Valid: true, Time: ts}
	k1 := buildAttendanceSourceRowKey("xiaotie", row)
	k2 := buildAttendanceSourceRowKey("xiaotie", row)
	if k1 == "" || k1 != k2 {
		t.Fatalf("unstable: %s %s", k1, k2)
	}
	if buildAttendanceSourceRowKey("muteng", row) == k1 {
		t.Fatal("org must differ")
	}
}

func TestNormalizeExternalCheckType(t *testing.T) {
	if normalizeExternalCheckType("OnDuty") != "上班" {
		t.Fatal("OnDuty")
	}
	if normalizeExternalCheckType("OffDuty") != "下班" {
		t.Fatal("OffDuty")
	}
}

func TestCollectExternalBusinessPunchesUsesTypedAttendanceResults(t *testing.T) {
	row := repository.ExternalAttendanceRow{
		CheckType:            "OnDuty",
		CheckRecordListJSON:  `[{"user_check_time":"2026-07-03 08:55:00"},{"user_check_time":"2026-07-03 18:35:00"}]`,
		AttendanceResultJSON: `[{"check_type":"OnDuty","user_check_time":"2026-07-03 08:55:00"},{"check_type":"OffDuty","user_check_time":"2026-07-03 18:35:00"}]`,
	}
	punches := collectExternalBusinessPunches(row)
	if len(punches) != 2 {
		t.Fatalf("punch count=%d want 2", len(punches))
	}
	if punches[0].checkType != "上班" || punches[1].checkType != "下班" {
		t.Fatalf("unexpected check types: %#v", punches)
	}
}

func TestParseFlexibleTime(t *testing.T) {
	if parseFlexibleTime("") != nil {
		t.Fatal("empty")
	}
	if parseFlexibleTime("2026-07-01 09:00:00") == nil {
		t.Fatal("datetime")
	}
}

func TestSanitizeExternalErrRedactsPassword(t *testing.T) {
	msg := sanitizeExternalErr(errors.New("access denied password=secret"))
	if msg == "access denied password=secret" {
		t.Fatal("password not redacted")
	}
}

func TestCorpMappingNoDefaultFallback(t *testing.T) {
	if database.MapOrgIDByCorpID("nope") != "" {
		t.Fatal("must not fallback")
	}
}

func TestRunDisabledRejects(t *testing.T) {
	svc := NewExternalAttendanceSyncService(nil, nil, "xiaotie", time.Minute, false)
	// local nil causes init error first — construct with dummy local using nil is ok for enabled check first
	svc.local = repository.NewExternalAttendanceLocalRepository(nil, "xiaotie")
	_, err := svc.Run(context.Background(), ExternalSyncRunOptions{Source: "attendance"})
	if !errors.Is(err, ErrExternalSyncDisabled) {
		t.Fatalf("want disabled, got %v", err)
	}
}

func TestDorisAttendanceRecalculationDeduplicatesWrittenPairs(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.User{}, &database.Attendance{}); err != nil {
		t.Fatalf("migrate attendance tables: %v", err)
	}

	const orgID = "xiaotie"
	localUID := database.ScopedExternalID(orgID, "external-user-1")
	svc := NewExternalAttendanceSyncService(
		nil,
		repository.NewExternalAttendanceLocalRepository(db, orgID),
		orgID,
		time.Minute,
		true,
	)
	checkTime := time.Date(2026, 8, 5, 18, 30, 0, 0, dingtalk.ApprovalBusinessLocation())
	row := repository.ExternalAttendanceRow{
		UserID:        "external-user-1",
		WorkDate:      "2026-08-05",
		CheckType:     "OffDuty",
		UserCheckTime: sql.NullTime{Valid: true, Time: checkTime},
		SourceType:    "USER",
	}
	if err := svc.applyBusinessAttendance(localUID, row.UserID, row); err != nil {
		t.Fatalf("first Doris attendance write: %v", err)
	}
	if err := svc.applyBusinessAttendance(localUID, row.UserID, row); err != nil {
		t.Fatalf("duplicate Doris attendance write: %v", err)
	}

	callCount := 0
	svc.SetRetryableOvertimeRecalculator(func(pairs []repository.UserDatePair) (int, error) {
		callCount++
		if len(pairs) != 1 || pairs[0].UserID != localUID || pairs[0].WorkDate != row.WorkDate {
			t.Fatalf("recalculation pairs = %#v", pairs)
		}
		return 1, nil
	})
	if err := svc.recalculateAffectedOvertime(); err != nil {
		t.Fatalf("recalculate Doris attendance: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("recalculator call count = %d, want 1", callCount)
	}

	var attendanceCount int64
	if err := db.Model(&database.Attendance{}).
		Where("org_id = ? AND user_id = ?", orgID, localUID).
		Count(&attendanceCount).Error; err != nil {
		t.Fatalf("count persisted Doris attendance: %v", err)
	}
	if attendanceCount != 1 {
		t.Fatalf("attendance count = %d, want 1", attendanceCount)
	}
}

func TestDorisEarlyMorningAttendanceAffectsPreviousWorkDate(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.User{}, &database.Attendance{}); err != nil {
		t.Fatalf("migrate attendance tables: %v", err)
	}

	const orgID = "xiaotie"
	localUID := database.ScopedExternalID(orgID, "external-night-user")
	svc := NewExternalAttendanceSyncService(
		nil,
		repository.NewExternalAttendanceLocalRepository(db, orgID),
		orgID,
		time.Minute,
		true,
	)
	row := repository.ExternalAttendanceRow{
		UserID:        "external-night-user",
		WorkDate:      "2026-08-06",
		CheckType:     "OffDuty",
		UserCheckTime: sql.NullTime{Valid: true, Time: time.Date(2026, 8, 6, 1, 0, 0, 0, dingtalk.ApprovalBusinessLocation())},
		SourceType:    "USER",
	}
	if err := svc.applyBusinessAttendance(localUID, row.UserID, row); err != nil {
		t.Fatalf("write early-morning Doris attendance: %v", err)
	}
	assertUserDatePairs(t, svc.AffectedAttendanceUserDatePairs(), localUID, "2026-08-05", "2026-08-06")
}

func TestPageTieKeyUsedForCursorNotEmptyRecord(t *testing.T) {
	row := repository.ExternalAttendanceRow{
		UserID:       "u1",
		WorkDate:     "2026-07-01",
		CheckType:    "OnDuty",
		DBUpdateTime: time.Now(),
		RecordID:     "",
	}
	row.PageTieKey = repository.BuildAttendancePageTieKey(row.UserID, row.WorkDate, row.CheckType, row.UserCheckTime, row.SourceType, row.PlanID, row.ProcInstID, row.RecordID)
	if row.PageTieKey == "" || row.PageTieKey == row.RecordID {
		t.Fatal("page tie key must be non-empty composite when record_id empty")
	}
}

func TestBuildDepartmentSourceRowKey(t *testing.T) {
	a := buildDepartmentSourceRowKey("xiaotie", "u1", "d1")
	b := buildDepartmentSourceRowKey("xiaotie", "u1", "d1")
	if a == "" || a != b {
		t.Fatal("unstable")
	}
}

func TestApplyExternalSyncJobStatusSuccess(t *testing.T) {
	job := &database.ExternalSyncJob{Inserted: 3, Updated: 1, Failed: 0}
	applyExternalSyncJobStatus(job, nil)
	if job.Status != "success" {
		t.Fatalf("status=%s", job.Status)
	}
}

func TestApplyExternalSyncJobStatusPartial(t *testing.T) {
	job := &database.ExternalSyncJob{Inserted: 2, Updated: 0, Failed: 1}
	applyExternalSyncJobStatus(job, []string{"approve_list parse failures: 1"})
	if job.Status != "partial" {
		t.Fatalf("status=%s", job.Status)
	}
	if !strings.Contains(job.ErrorSummary, "approve_list parse failures") {
		t.Fatalf("summary=%s", job.ErrorSummary)
	}
}

func TestApplyExternalSyncJobStatusFailedZeroSuccess(t *testing.T) {
	job := &database.ExternalSyncJob{Inserted: 0, Updated: 0, Failed: 5}
	applyExternalSyncJobStatus(job, []string{"attendance:boom"})
	if job.Status != "failed" {
		t.Fatalf("status=%s want failed", job.Status)
	}
	if job.ErrorSummary == "" {
		t.Fatal("error summary required for failed")
	}
}

func TestJoinErrorSummaryTruncates(t *testing.T) {
	long := strings.Repeat("x", 1200)
	msg := joinErrorSummary([]string{long}, "")
	if len(msg) > 1000 {
		t.Fatalf("len=%d", len(msg))
	}
}

func newExternalSyncLifecycleService(t *testing.T) (*ExternalAttendanceSyncService, *gorm.DB) {
	t.Helper()
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.ExternalSyncJob{}, &database.ExternalSyncLock{}); err != nil {
		t.Fatalf("migrate external sync task tables: %v", err)
	}
	svc := NewExternalAttendanceSyncService(
		&repository.ExternalAttendanceSourceRepository{},
		repository.NewExternalAttendanceLocalRepository(db, "xiaotie"),
		"xiaotie",
		time.Minute,
		true,
	)
	return svc, db
}

func TestExternalSyncRunSuccessPersistsTerminalStateAndCounts(t *testing.T) {
	svc, db := newExternalSyncLifecycleService(t)
	svc.syncAttendanceFn = func(_ context.Context, job *database.ExternalSyncJob, _ time.Duration) error {
		job.Inserted, job.Updated, job.Skipped = 2, 3, 4
		return nil
	}

	job, err := svc.Run(context.Background(), ExternalSyncRunOptions{Source: "attendance"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if job.Status != "success" || job.FinishedAt == nil {
		t.Fatalf("job terminal state = %#v", job)
	}
	var persisted database.ExternalSyncJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if persisted.Status != "success" || persisted.Inserted != 2 || persisted.Updated != 3 || persisted.Skipped != 4 || persisted.FinishedAt == nil {
		t.Fatalf("persisted job = %#v", persisted)
	}
}

func TestExternalSyncRunBusinessErrorAndContextCancelFail(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "business error", err: errors.New("source unavailable")},
		{name: "context canceled", err: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, db := newExternalSyncLifecycleService(t)
			svc.syncAttendanceFn = func(_ context.Context, _ *database.ExternalSyncJob, _ time.Duration) error { return tt.err }
			job, runErr := svc.Run(context.Background(), ExternalSyncRunOptions{Source: "attendance"})
			if runErr == nil || job.Status != "failed" || job.FinishedAt == nil {
				t.Fatalf("job=%#v err=%v", job, runErr)
			}
			var persisted database.ExternalSyncJob
			if err := db.First(&persisted, job.ID).Error; err != nil {
				t.Fatalf("load job: %v", err)
			}
			if persisted.Status != "failed" || persisted.FinishedAt == nil || persisted.ErrorSummary == "" {
				t.Fatalf("persisted job=%#v", persisted)
			}
		})
	}
}

func TestExternalSyncRunPanicIsRecovered(t *testing.T) {
	svc, db := newExternalSyncLifecycleService(t)
	svc.syncAttendanceFn = func(context.Context, *database.ExternalSyncJob, time.Duration) error { panic("boom") }

	job, err := svc.Run(context.Background(), ExternalSyncRunOptions{Source: "attendance"})
	if err == nil || job.Status != "failed" || job.FinishedAt == nil || !strings.Contains(job.ErrorSummary, "panic") {
		t.Fatalf("job=%#v err=%v", job, err)
	}
	var persisted database.ExternalSyncJob
	if err := db.First(&persisted, job.ID).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if persisted.Status != "failed" || !strings.Contains(persisted.ErrorSummary, "panic") {
		t.Fatalf("persisted panic job=%#v", persisted)
	}
}

func TestExternalSyncAllContinuesAfterChildFailure(t *testing.T) {
	svc, _ := newExternalSyncLifecycleService(t)
	svc.syncAttendanceFn = func(context.Context, *database.ExternalSyncJob, time.Duration) error {
		return errors.New("attendance failed")
	}
	svc.syncDepartmentsFn = func(_ context.Context, job *database.ExternalSyncJob, _ time.Duration, _ bool) error {
		job.Inserted = 1
		return nil
	}

	job, err := svc.Run(context.Background(), ExternalSyncRunOptions{Source: "all"})
	if err != nil {
		t.Fatalf("partial all should not return fatal error: %v", err)
	}
	if job.Status != "partial" || job.Inserted != 1 || !strings.Contains(job.ErrorSummary, "attendance") {
		t.Fatalf("job=%#v", job)
	}
}

func TestExternalSyncDuplicateSubmissionUsesDatabaseGate(t *testing.T) {
	svc, db := newExternalSyncLifecycleService(t)
	first, conflict, err := svc.PrepareRun(ExternalSyncRunOptions{Source: "attendance"})
	if err != nil || first == nil || conflict != nil {
		t.Fatalf("first prepare: prepared=%#v conflict=%#v err=%v", first, conflict, err)
	}
	second, conflict, err := svc.PrepareRun(ExternalSyncRunOptions{Source: "department"})
	if !errors.Is(err, ErrExternalSyncLocked) || second != nil || conflict == nil || conflict.ID != first.Job.ID {
		t.Fatalf("duplicate prepare: prepared=%#v conflict=%#v err=%v", second, conflict, err)
	}
	var count int64
	if err := db.Model(&database.ExternalSyncJob{}).Where("status = ?", "running").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("running count=%d err=%v", count, err)
	}
}

func TestExternalSyncRecoversStaleRunningJobAsFailed(t *testing.T) {
	svc, db := newExternalSyncLifecycleService(t)
	svc.SetTaskTimeout(time.Minute)
	started := time.Now().Add(-10 * time.Minute)
	job := &database.ExternalSyncJob{OrgID: "xiaotie", Trigger: "manual", Source: "all", Status: "running", StartedAt: started}
	if err := db.Create(job).Error; err != nil {
		t.Fatalf("create stale job: %v", err)
	}
	if _, err := svc.RecoverStaleJobs(context.Background()); err != nil {
		t.Fatalf("recover stale: %v", err)
	}
	var recovered database.ExternalSyncJob
	if err := db.First(&recovered, job.ID).Error; err != nil {
		t.Fatalf("load stale job: %v", err)
	}
	if recovered.Status != "failed" || recovered.FinishedAt == nil || !strings.Contains(recovered.ErrorSummary, "任务执行中断或服务重启") {
		t.Fatalf("recovered job=%#v", recovered)
	}
}

func TestExternalSyncTerminalPersistFailureIsLogged(t *testing.T) {
	svc, db := newExternalSyncLifecycleService(t)
	var logs []string
	svc.logf = func(format string, args ...interface{}) { logs = append(logs, fmt.Sprintf(format, args...)) }
	prepared, _, err := svc.PrepareRun(ExternalSyncRunOptions{Source: "attendance"})
	if err != nil || prepared == nil {
		t.Fatalf("prepare: prepared=%#v err=%v", prepared, err)
	}
	callbackName := "test_external_sync_terminal_failure"
	if err := db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Name == "ExternalSyncJob" {
			_ = tx.AddError(errors.New("terminal write blocked"))
		}
	}); err != nil {
		t.Fatalf("register callback: %v", err)
	}
	t.Cleanup(func() { _ = db.Callback().Update().Remove(callbackName) })
	svc.syncAttendanceFn = func(_ context.Context, job *database.ExternalSyncJob, _ time.Duration) error {
		job.Inserted = 1
		return nil
	}
	job, err := svc.RunPrepared(context.Background(), prepared)
	if err == nil || job == nil || job.Status != "success" {
		t.Fatalf("job=%#v err=%v", job, err)
	}
	matched := false
	for _, line := range logs {
		if strings.Contains(line, "stage=terminal_persist") {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatalf("terminal persist failure was not logged: %v", logs)
	}
}

func TestInitialSyncStartTimeDefaultIsEpoch(t *testing.T) {
	// Ensure first backfill can read data older than 30 days when env unset.
	t.Setenv("EXTERNAL_ATTENDANCE_INITIAL_START_TIME", "")
	start := database.InitialSyncStartTime()
	if !start.Equal(time.Unix(0, 0).UTC()) && start.After(time.Unix(0, 0).UTC().Add(time.Second)) {
		// Allow UTC normalization differences of zero instant
		if start.Unix() != 0 {
			t.Fatalf("want epoch, got %v", start)
		}
	}
	// Explicitly verify older than 30 days window
	thirtyDaysAgo := time.Now().UTC().Add(-30 * 24 * time.Hour)
	if !start.Before(thirtyDaysAgo) {
		t.Fatalf("initial start %v must be before 30 days ago", start)
	}
}

func TestInitialSyncStartTimeConfigured(t *testing.T) {
	t.Setenv("EXTERNAL_ATTENDANCE_INITIAL_START_TIME", "2026-04-01")
	start := database.InitialSyncStartTime()
	if start.Year() != 2026 || start.Month() != time.April || start.Day() != 1 {
		t.Fatalf("got %v", start)
	}
}
