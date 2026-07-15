package database

import (
	"testing"
	"time"
)

func TestBuildMutengParticipantPipelineUpdatesEmployeeConfirmed(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	employeeConfirmedAt := time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC)

	result := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
		Status:              "employee_confirmed",
		EmployeeConfirmedAt: &employeeConfirmedAt,
		EmployeeConfirmedBy: "employee-1",
		ResultHidden:        true,
		ResultHiddenReason:  "system:unpublished",
	}, now)

	if !result.CountedActive || !result.CountedCompleted {
		t.Fatalf("counted active/completed = %v/%v, want true/true", result.CountedActive, result.CountedCompleted)
	}
	if result.Status != "hr_confirmed" {
		t.Fatalf("status = %q, want hr_confirmed", result.Status)
	}
	if result.Updates["status"] != "hr_confirmed" {
		t.Fatalf("updates.status = %#v, want hr_confirmed", result.Updates["status"])
	}
	if got, ok := result.Updates["hr_confirmed_at"].(*time.Time); !ok || got == nil || !got.Equal(employeeConfirmedAt) {
		t.Fatalf("hr_confirmed_at = %#v, want historical employee confirmation time", result.Updates["hr_confirmed_at"])
	}
	if result.Updates["hr_confirmed_by"] != "employee-1" {
		t.Fatalf("hr_confirmed_by = %#v, want employee-1", result.Updates["hr_confirmed_by"])
	}
	if result.Updates["result_hidden"] != false {
		t.Fatalf("result_hidden = %#v, want false for published participant", result.Updates["result_hidden"])
	}
	if result.Updates["result_hidden_reason"] != "" {
		t.Fatalf("result_hidden_reason = %#v, want empty", result.Updates["result_hidden_reason"])
	}
	if got, ok := result.Updates["confirmed_at"].(*time.Time); !ok || got == nil || !got.Equal(employeeConfirmedAt) {
		t.Fatalf("confirmed_at = %#v, want published timestamp", result.Updates["confirmed_at"])
	}
	if result.Updates["updated_by"] != "system:muteng-participant-pipeline" {
		t.Fatalf("updated_by = %#v", result.Updates["updated_by"])
	}
}

func TestBuildMutengParticipantPipelineUpdatesManagerRecheck(t *testing.T) {
	now := time.Now()
	result := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
		Status:       "manager_recheck",
		ResultHidden: false,
	}, now)

	if result.Status != "self_submitted" {
		t.Fatalf("status = %q, want self_submitted", result.Status)
	}
	if result.Updates["status"] != "self_submitted" {
		t.Fatalf("updates.status = %#v", result.Updates["status"])
	}
	if result.Updates["is_locked"] != false {
		t.Fatalf("is_locked = %#v, want false", result.Updates["is_locked"])
	}
	if _, ok := result.Updates["locked_at"]; !ok || result.Updates["locked_at"] != nil {
		t.Fatalf("locked_at = %#v, want nil", result.Updates["locked_at"])
	}
	if result.Updates["locked_by"] != "" {
		t.Fatalf("locked_by = %#v, want empty", result.Updates["locked_by"])
	}
	if result.Updates["result_hidden"] != true {
		t.Fatalf("result_hidden = %#v, want true for unfinished participant", result.Updates["result_hidden"])
	}
	if result.Updates["result_hidden_reason"] != "system:unpublished" {
		t.Fatalf("result_hidden_reason = %#v, want system:unpublished", result.Updates["result_hidden_reason"])
	}
	if result.CountedCompleted {
		t.Fatal("manager_recheck should not count as completed")
	}
}

func TestBuildMutengParticipantPipelineUpdatesUnpublishedHide(t *testing.T) {
	now := time.Now()
	result := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
		Status:       "self_submitted",
		ResultHidden: false,
	}, now)

	if result.Updates["result_hidden"] != true {
		t.Fatalf("result_hidden = %#v, want true", result.Updates["result_hidden"])
	}
	if result.Updates["result_hidden_reason"] != "system:unpublished" {
		t.Fatalf("result_hidden_reason = %#v, want system:unpublished", result.Updates["result_hidden_reason"])
	}
	if result.CountedCompleted {
		t.Fatal("unfinished participant should not count as completed")
	}
}

func TestBuildMutengParticipantPipelineUpdatesPublishedClearsSystemUnpublished(t *testing.T) {
	now := time.Now()
	hrAt := time.Date(2026, 5, 20, 8, 0, 0, 0, time.UTC)
	result := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
		Status:             "hr_confirmed",
		HRConfirmedAt:      &hrAt,
		HRConfirmedBy:      "hr-1",
		ResultHidden:       true,
		ResultHiddenReason: "system:unpublished",
	}, now)

	if !result.CountedCompleted {
		t.Fatal("hr_confirmed should count as completed")
	}
	if result.Updates["result_hidden"] != false {
		t.Fatalf("result_hidden = %#v, want false", result.Updates["result_hidden"])
	}
	if result.Updates["result_hidden_reason"] != "" {
		t.Fatalf("result_hidden_reason = %#v, want empty", result.Updates["result_hidden_reason"])
	}
	if got, ok := result.Updates["confirmed_at"].(*time.Time); !ok || got == nil || !got.Equal(hrAt) {
		t.Fatalf("confirmed_at = %#v, want hr_confirmed_at", result.Updates["confirmed_at"])
	}
	if result.Updates["confirmed_by"] != "hr-1" {
		t.Fatalf("confirmed_by = %#v, want hr-1", result.Updates["confirmed_by"])
	}
}

func TestBuildMutengParticipantPipelineUpdatesKeepsManualHide(t *testing.T) {
	now := time.Now()

	// Published participant with a manual hide reason must keep it.
	published := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
		Status:             "hr_confirmed",
		HRConfirmedAt:      &now,
		HRConfirmedBy:      "hr-1",
		ConfirmedAt:        &now,
		ConfirmedBy:        "hr-1",
		ResultHidden:       true,
		ResultHiddenReason: "manual:privacy",
	}, now)
	if _, ok := published.Updates["result_hidden"]; ok {
		t.Fatalf("published manual hide must not be cleared, updates=%#v", published.Updates)
	}
	if published.CountedCompleted != true {
		t.Fatal("published participant should still count as completed")
	}

	// Unfinished participant that is already manually hidden should not be rewritten.
	unfinished := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
		Status:             "manager_submitted",
		ResultHidden:       true,
		ResultHiddenReason: "manual:privacy",
	}, now)
	if len(unfinished.Updates) != 0 {
		t.Fatalf("manual hide on unfinished participant should be left alone, updates=%#v", unfinished.Updates)
	}
	if unfinished.CountedActive != true || unfinished.CountedCompleted {
		t.Fatalf("counted = %v/%v, want true/false", unfinished.CountedActive, unfinished.CountedCompleted)
	}
}

func TestBuildMutengParticipantPipelineUpdatesIgnoresInactive(t *testing.T) {
	now := time.Now()
	for _, status := range []string{"inactive", "removed_from_scope"} {
		result := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{Status: status}, now)
		if result.CountedActive || result.CountedCompleted || len(result.Updates) != 0 {
			t.Fatalf("status %s should be ignored, got %#v", status, result)
		}
	}
}

func TestBuildMutengParticipantPipelineUpdatesIdempotentForAlreadyMigrated(t *testing.T) {
	now := time.Now()
	hrAt := now
	result := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
		Status:             "hr_confirmed",
		HRConfirmedAt:      &hrAt,
		HRConfirmedBy:      "hr-1",
		ConfirmedAt:        &hrAt,
		ConfirmedBy:        "hr-1",
		ResultHidden:       false,
		ResultHiddenReason: "",
	}, now)
	if len(result.Updates) != 0 {
		t.Fatalf("already migrated published participant should be no-op, updates=%#v", result.Updates)
	}
	if !result.CountedCompleted {
		t.Fatal("already migrated published participant should still count as completed")
	}
}

func TestMigrateMutengActivityAggregateStatus(t *testing.T) {
	tests := []struct {
		name           string
		activityKind   string
		currentStatus  string
		activeCount    int
		completedCount int
		want           string
	}{
		{
			name:          "goal_setting target_approval becomes target_setting",
			activityKind:  "goal_setting",
			currentStatus: "target_approval",
			want:          "target_setting",
		},
		{
			name:          "goal_setting other status stays",
			activityKind:  "goal_setting",
			currentStatus: "target_setting",
			want:          "target_setting",
		},
		{
			name:           "review all complete becomes result_publish",
			activityKind:   "review_scoring",
			currentStatus:  "self_evaluation",
			activeCount:    2,
			completedCount: 2,
			want:           "result_publish",
		},
		{
			name:           "review partial complete becomes self_evaluation",
			activityKind:   "review_scoring",
			currentStatus:  "hr_review",
			activeCount:    2,
			completedCount: 1,
			want:           "self_evaluation",
		},
		{
			name:           "review still in target_setting stays",
			activityKind:   "review_scoring",
			currentStatus:  "target_setting",
			activeCount:    2,
			completedCount: 0,
			want:           "target_setting",
		},
		{
			name:           "inactive-only activity does not force result_publish",
			activityKind:   "review_scoring",
			currentStatus:  "department_evaluation",
			activeCount:    0,
			completedCount: 0,
			want:           "self_evaluation",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := migrateMutengActivityAggregateStatus(tt.activityKind, tt.currentStatus, tt.activeCount, tt.completedCount)
			if got != tt.want {
				t.Fatalf("migrateMutengActivityAggregateStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMutengParticipantPipelineActiveActivityStatusesExcludesTerminal(t *testing.T) {
	statuses := mutengParticipantPipelineActiveActivityStatuses()
	for _, blocked := range []string{"locked", "archived", "draft"} {
		for _, status := range statuses {
			if status == blocked {
				t.Fatalf("active statuses unexpectedly include %q", blocked)
			}
		}
	}
}
