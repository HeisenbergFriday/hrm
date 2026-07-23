package service

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
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
