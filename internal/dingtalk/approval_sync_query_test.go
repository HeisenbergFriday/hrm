package dingtalk

import (
	"reflect"
	"testing"
	"time"
)

func TestBuildApprovalQueryWindowsSplitsAt120DaysWithoutGaps(t *testing.T) {
	now := time.Date(2026, 8, 5, 4, 0, 0, 0, time.UTC) // 12:00 UTC+8
	windows, err := buildApprovalQueryWindows("2025-12-01", "2026-08-05", now)
	if err != nil {
		t.Fatalf("buildApprovalQueryWindows() error = %v", err)
	}
	if len(windows) != 3 {
		t.Fatalf("window count = %d, want 3", len(windows))
	}
	for index, window := range windows {
		if window.End.Sub(window.Start) > approvalQueryMaxWindow {
			t.Fatalf("window %d exceeds 120 days: %s", index, window.End.Sub(window.Start))
		}
		if index > 0 && !windows[index-1].End.Equal(window.Start) {
			t.Fatalf("window %d is not continuous: previous=%s current=%s", index, windows[index-1].End, window.Start)
		}
	}
	wantEnd := now.In(ApprovalBusinessLocation()).Add(-approvalQueryClockSkew)
	if !windows[len(windows)-1].End.Equal(wantEnd) {
		t.Fatalf("last end = %s, want safe current time %s", windows[len(windows)-1].End, wantEnd)
	}
	_, offset := windows[0].Start.Zone()
	if offset != 8*60*60 {
		t.Fatalf("window timezone offset = %d, want UTC+8", offset)
	}
}

func TestAppendUniqueApprovalInstanceIDsDeduplicatesAcrossWindows(t *testing.T) {
	seen := make(map[string]struct{})
	ids := appendUniqueApprovalInstanceIDs(nil, seen, []interface{}{"instance-1", " instance-2 ", "", 3})
	ids = appendUniqueApprovalInstanceIDs(ids, seen, []interface{}{"instance-2", "instance-3", "instance-1"})
	want := []string{"instance-1", "instance-2", "instance-3"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("ids = %#v, want %#v", ids, want)
	}
}

func TestApprovalFetchResultTracksDetailFailuresSeparately(t *testing.T) {
	result := ApprovalFetchResult{
		Instances:       []ApprovalInstance{{ProcessInstanceID: "instance-1"}},
		DetailFailCount: 2,
	}
	if len(result.Instances) != 1 || result.DetailFailCount != 2 {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildApprovalQueryWindowsRejectsInvalidAndFutureDates(t *testing.T) {
	now := time.Date(2026, 8, 5, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		startDate string
		endDate   string
	}{
		{name: "invalid start", startDate: "2026/08/01", endDate: "2026-08-05"},
		{name: "invalid end", startDate: "2026-08-01", endDate: "05-08-2026"},
		{name: "reversed", startDate: "2026-08-05", endDate: "2026-08-04"},
		{name: "future", startDate: "2026-08-01", endDate: "2026-08-06"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := buildApprovalQueryWindows(test.startDate, test.endDate, now); err == nil {
				t.Fatal("buildApprovalQueryWindows() error = nil, want validation error")
			}
		})
	}
}
