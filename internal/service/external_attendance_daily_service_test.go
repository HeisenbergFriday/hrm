package service

import (
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
)

func TestBuildExternalAttendanceDailyResultsKeepsLateAndLeave(t *testing.T) {
	onDuty := time.Date(2026, 7, 16, 9, 12, 0, 0, time.Local)
	offDuty := time.Date(2026, 7, 16, 18, 5, 0, 0, time.Local)
	begin := time.Date(2026, 7, 16, 14, 0, 0, 0, time.Local)
	end := time.Date(2026, 7, 16, 16, 0, 0, 0, time.Local)

	rows := []database.ExternalAttendanceRaw{
		{
			LocalUserID: "xiaotie:u1", ExternalUserID: "u1", WorkDate: "2026-07-16",
			SourceRowKey: "row-on", CheckType: "OnDuty", UserCheckTime: &onDuty,
			TimeResult: "Late", LocationResult: "Normal", SourceType: "USER",
			SourceUpdatedAt: onDuty,
		},
		{
			LocalUserID: "xiaotie:u1", ExternalUserID: "u1", WorkDate: "2026-07-16",
			SourceRowKey: "row-off", CheckType: "OffDuty", UserCheckTime: &offDuty,
			TimeResult: "Normal", LocationResult: "Normal", SourceType: "USER",
			SourceUpdatedAt: offDuty,
		},
	}
	links := []database.ExternalAttendanceApproveLink{
		{
			SourceRowKey: "row-on", ItemKey: "leave-1", ProcInstID: "proc-1",
			TagName: "请假", SubType: "年假", BeginTime: &begin, EndTime: &end,
			Duration: "2.0", DurationUnit: "HOUR",
		},
	}
	profiles := map[string]repository.ExternalAttendanceUserProfile{
		"xiaotie:u1": {
			LocalUserID: "xiaotie:u1", UserName: "张三",
			DepartmentID: "d1", DepartmentName: "研发部",
		},
	}

	items := BuildExternalAttendanceDailyResults(rows, links, profiles)
	if len(items) != 1 {
		t.Fatalf("items=%d want 1", len(items))
	}
	item := items[0]
	if item.UserName != "张三" || item.DepartmentName != "研发部" {
		t.Fatalf("profile not applied: %#v", item)
	}
	if item.OnDutyTime == nil || !item.OnDutyTime.Equal(onDuty) {
		t.Fatalf("on duty=%v", item.OnDutyTime)
	}
	if item.OffDutyTime == nil || !item.OffDutyTime.Equal(offDuty) {
		t.Fatalf("off duty=%v", item.OffDutyTime)
	}
	assertDailyStatusLabel(t, item.Statuses, "迟到")
	assertDailyStatusLabel(t, item.Statuses, "年假 2小时")
	if !item.HasException {
		t.Fatal("late day must be exceptional")
	}
	if len(item.Approvals) != 1 || item.Approvals[0].Label != "年假 2小时" {
		t.Fatalf("approvals=%#v", item.Approvals)
	}
}

func TestDailyTimeResultStatusMappings(t *testing.T) {
	cases := map[string]string{
		"Late":        "迟到",
		"SeriousLate": "严重迟到",
		"Early":       "早退",
		"NotSigned":   "缺卡",
		"Absenteeism": "旷工",
	}
	for input, label := range cases {
		status, ok := dailyTimeResultStatus(input)
		if !ok || status.Label != label {
			t.Fatalf("input=%s status=%#v ok=%v", input, status, ok)
		}
	}
	if _, ok := dailyTimeResultStatus("Normal"); ok {
		t.Fatal("normal must not produce an exception status")
	}
}

func TestBuildExternalAttendanceDailyResultsSortsAndDeduplicatesPunches(t *testing.T) {
	t1 := time.Date(2026, 7, 16, 18, 2, 0, 0, time.Local)
	t2 := time.Date(2026, 7, 16, 8, 58, 0, 0, time.Local)
	rows := []database.ExternalAttendanceRaw{
		{LocalUserID: "u1", WorkDate: "2026-07-16", SourceRowKey: "a", CheckType: "OffDuty", UserCheckTime: &t1, SourceUpdatedAt: t1},
		{LocalUserID: "u1", WorkDate: "2026-07-16", SourceRowKey: "b", CheckType: "OnDuty", UserCheckTime: &t2, SourceUpdatedAt: t2},
		{LocalUserID: "u1", WorkDate: "2026-07-16", SourceRowKey: "c", CheckType: "OnDuty", UserCheckTime: &t2, SourceUpdatedAt: t2},
	}
	items := BuildExternalAttendanceDailyResults(rows, nil, nil)
	if len(items) != 1 || len(items[0].Punches) != 2 {
		t.Fatalf("items=%#v", items)
	}
	if !items[0].Punches[0].CheckTime.Equal(t2) || !items[0].Punches[1].CheckTime.Equal(t1) {
		t.Fatalf("punches not sorted: %#v", items[0].Punches)
	}
	assertDailyStatusLabel(t, items[0].Statuses, "正常")
}

func TestDailySummaryAndStatusFilter(t *testing.T) {
	items := []ExternalAttendanceDailyResult{
		{Key: "normal", HasException: false},
		{Key: "late", HasException: true, Statuses: []ExternalAttendanceDailyStatus{{Code: "late", Label: "迟到"}}},
		{Key: "leave", HasException: false, Approvals: []ExternalAttendanceDailyApproval{{Label: "年假 2小时"}}, Statuses: []ExternalAttendanceDailyStatus{{Code: "leave:年假", Label: "年假 2小时"}}},
	}
	summary := summarizeExternalAttendanceDailyResults(items)
	if summary.Total != 3 || summary.Normal != 1 || summary.Exception != 1 || summary.WithApproval != 1 {
		t.Fatalf("summary=%#v", summary)
	}
	if got := filterExternalAttendanceDailyResults(items, "exception"); len(got) != 1 || got[0].Key != "late" {
		t.Fatalf("exception filter=%#v", got)
	}
	if got := filterExternalAttendanceDailyResults(items, "leave"); len(got) != 1 || got[0].Key != "leave" {
		t.Fatalf("leave filter=%#v", got)
	}
	if got := filterExternalAttendanceDailyResults(items, "normal"); len(got) != 1 || got[0].Key != "normal" {
		t.Fatalf("normal filter=%#v", got)
	}
}

func assertDailyStatusLabel(t *testing.T, statuses []ExternalAttendanceDailyStatus, label string) {
	t.Helper()
	for _, status := range statuses {
		if status.Label == label {
			return
		}
	}
	t.Fatalf("missing status %q in %#v", label, statuses)
}
