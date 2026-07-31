package service

import (
	"testing"
	"time"

	"peopleops/internal/database"
)

func TestLifecyclePredicatesDatePriorityAndBoundaries(t *testing.T) {
	now := time.Date(2026, 7, 31, 0, 0, 0, 0, time.Local)
	tests := []struct {
		name          string
		snapshot      orgEmployeeSnapshot
		wantProbation bool
		wantWarning   bool
	}{
		{name: "invalid date", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "not-a-date"}},
		{name: "past date remains probation", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "2026-07-30"}, wantProbation: true},
		{name: "today", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "2026-07-31"}, wantProbation: true, wantWarning: true},
		{name: "future day 30", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "2026-08-30"}, wantProbation: true, wantWarning: true},
		{name: "future day 31", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "2026-08-31"}, wantProbation: true},
		{name: "probation end fallback", snapshot: orgEmployeeSnapshot{Status: "active", ProbationEndDate: "2026-08-01"}, wantProbation: true, wantWarning: true},
		{name: "planned date wins over probation end", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "2026-08-31", ProbationEndDate: "2026-08-01"}, wantProbation: true},
		{name: "invalid planned date does not fall back", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "invalid", ProbationEndDate: "2026-08-01"}},
		{name: "actual regular date excludes", snapshot: orgEmployeeSnapshot{Status: "active", PlannedRegularDate: "2026-08-01", ActualRegularDate: "2026-07-31"}},
		{name: "inactive excludes", snapshot: orgEmployeeSnapshot{Status: "inactive", PlannedRegularDate: "2026-08-01"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isProbationEmployee(tt.snapshot); got != tt.wantProbation {
				t.Fatalf("probation = %v, want %v", got, tt.wantProbation)
			}
			if got := isRegularizationWarning(tt.snapshot, now); got != tt.wantWarning {
				t.Fatalf("warning = %v, want %v", got, tt.wantWarning)
			}
		})
	}
}

func TestOverviewAndEmployeeLifecycleFiltersUseSamePredicates(t *testing.T) {
	db := openOrgServiceMembershipDB(t)
	if err := db.Create(&database.Department{OrgID: "org-r", DepartmentID: "root", DingTalkDepartmentID: "1", Name: "总部"}).Error; err != nil {
		t.Fatalf("seed department: %v", err)
	}
	type profileDates struct {
		planned   string
		probation string
		actual    string
		status    string
	}
	fixtures := map[string]profileDates{
		"past":             {planned: "2026-07-30", status: "active"},
		"today":            {planned: "2026-07-31", status: "active"},
		"day-30":           {planned: "2026-08-30", status: "active"},
		"day-31":           {planned: "2026-08-31", status: "active"},
		"both":             {planned: "2026-08-31", probation: "2026-08-01", status: "active"},
		"invalid-priority": {planned: "invalid", probation: "2026-08-01", status: "active"},
		"actual":           {planned: "2026-08-01", actual: "2026-07-31", status: "active"},
		"inactive":         {planned: "2026-08-01", status: "inactive"},
	}
	for userID, dates := range fixtures {
		seedOrgServiceMembershipEmployeeWithStatus(t, db, "org-r", userID, "root", dates.status)
		if err := db.Model(&database.EmployeeProfile{}).
			Where("org_id = ? AND user_id = ?", "org-r", userID).
			Updates(map[string]interface{}{
				"planned_regular_date": dates.planned,
				"probation_end_date":   dates.probation,
				"actual_regular_date":  dates.actual,
			}).Error; err != nil {
			t.Fatalf("update profile %s: %v", userID, err)
		}
	}

	svc := NewOrgServiceWithOrgID(db, "org-r")
	svc.nowFn = func() time.Time { return time.Date(2026, 7, 31, 12, 0, 0, 0, time.Local) }
	snapshots, err := svc.listEmployeeSnapshots(nil)
	if err != nil {
		t.Fatalf("list snapshots: %v", err)
	}
	summary, _ := svc.buildOverviewSummary(snapshots, nil)
	if summary.ProbationEmployeeCount != 5 || summary.PlannedRegularizationCount != 2 {
		t.Fatalf("overview probation=%d warning=%d", summary.ProbationEmployeeCount, summary.PlannedRegularizationCount)
	}

	_, probationTotal, err := svc.ListEmployees(nil, 1, 20, OrgEmployeeFilters{FilterType: "probation"})
	if err != nil {
		t.Fatalf("list probation: %v", err)
	}
	_, warningTotal, err := svc.ListEmployees(nil, 1, 20, OrgEmployeeFilters{FilterType: "regularization_warning"})
	if err != nil {
		t.Fatalf("list warning: %v", err)
	}
	if probationTotal != int64(summary.ProbationEmployeeCount) || warningTotal != int64(summary.PlannedRegularizationCount) {
		t.Fatalf("list totals probation=%d warning=%d do not match overview", probationTotal, warningTotal)
	}
}
