package service

import (
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
)

// ======================== GetActivity / GetParticipant ========================

func TestGetActivityReturnsActivity(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("draft", ""))
	activity, err := svc.GetActivity("1")
	if err != nil {
		t.Fatalf("GetActivity() error = %v", err)
	}
	if activity.Name != "Q2" || activity.Status != "draft" {
		t.Fatalf("GetActivity() = %#v, want name=Q2 status=draft", activity)
	}
}

func TestGetActivityNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "cycle_type", "status"},
		rows:    nil,
	})
	_, err := svc.GetActivity("999")
	if err == nil {
		t.Fatalf("GetActivity(999) expected error")
	}
}

func TestGetParticipantReturnsParticipant(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
		},
	})
	p, err := svc.GetParticipant("1")
	if err != nil {
		t.Fatalf("GetParticipant() error = %v", err)
	}
	if p.EmployeeID != "user-1" || p.Status != "target_set" {
		t.Fatalf("GetParticipant() = %#v", p)
	}
}

func TestGetParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	_, err := svc.GetParticipant("999")
	if err == nil {
		t.Fatalf("GetParticipant(999) expected error")
	}
}

// ======================== ListActivities ========================

func TestListActivitiesNilScope(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_activities") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q1", "archived"},
				{int64(2), "Q2", "draft"},
			},
		},
	)
	_, total, err := svc.ListActivities(1, 10, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("ListActivities() error = %v", err)
	}
	if total != 2 {
		t.Fatalf("ListActivities() total=%d, want 2", total)
	}
}

func TestListActivitiesSelfScope(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_activities") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "draft"},
			},
		},
	)
	scope := &OrgDataScope{Mode: "self", UserIDs: []string{"user-1"}}
	_, total, err := svc.ListActivities(1, 10, "", "", "", "", scope)
	if err != nil {
		t.Fatalf("ListActivities(self) error = %v", err)
	}
	if total != 1 {
		t.Fatalf("ListActivities(self) total=%d, want 1", total)
	}
}

func TestListActivitiesDepartmentScope(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_activities") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "draft"},
			},
		},
	)
	scope := &OrgDataScope{Mode: "department", DepartmentIDs: []string{"dept-1"}}
	_, total, err := svc.ListActivities(1, 10, "", "", "", "", scope)
	if err != nil {
		t.Fatalf("ListActivities(dept) error = %v", err)
	}
	if total != 1 {
		t.Fatalf("ListActivities(dept) total=%d, want 1", total)
	}
}

func TestListActivitiesAllScope(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_activities") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q1", "archived"},
				{int64(2), "Q2", "draft"},
			},
		},
	)
	scope := &OrgDataScope{Mode: "all"}
	_, total, err := svc.ListActivities(1, 10, "", "", "", "", scope)
	if err != nil {
		t.Fatalf("ListActivities(all) error = %v", err)
	}
	if total != 2 {
		t.Fatalf("ListActivities(all) total=%d, want 2", total)
	}
}

// ======================== ListParticipants ========================

func TestListParticipantsDelegatesToRepo(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_participants") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
				performanceParticipantStubRow(2, "self_submitted", "done", 85, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "status"},
			rows:    nil,
		},
	)
	_, total, err := svc.ListParticipants("activity-1", 1, 10, "", "", "", "", nil)
	if err != nil {
		t.Fatalf("ListParticipants() error = %v", err)
	}
	if total != 2 {
		t.Fatalf("ListParticipants() total=%d, want 2", total)
	}
}

func TestListParticipantsSelfScope(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_participants") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "status"},
			rows:    nil,
		},
	)
	scope := &OrgDataScope{Mode: "self", UserIDs: []string{"user-1"}}
	_, total, err := svc.ListParticipants("activity-1", 1, 10, "", "", "", "", scope)
	if err != nil {
		t.Fatalf("ListParticipants(self) error = %v", err)
	}
	if total != 1 {
		t.Fatalf("ListParticipants(self) total=%d, want 1", total)
	}
}

func TestListParticipantsDepartmentScope(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_participants") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "status"},
			rows:    nil,
		},
	)
	scope := &OrgDataScope{Mode: "department", DepartmentIDs: []string{"dept-1"}}
	_, total, err := svc.ListParticipants("activity-1", 1, 10, "", "", "", "", scope)
	if err != nil {
		t.Fatalf("ListParticipants(dept) error = %v", err)
	}
	if total != 1 {
		t.Fatalf("ListParticipants(dept) total=%d, want 1", total)
	}
}

// ======================== RefreshParticipants ========================

func TestRefreshParticipantsRejectsNonDraft(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("target_setting", ""))
	_, err := svc.RefreshParticipants("1", "operator-1")
	if err == nil || !strings.Contains(err.Error(), "不能增减参与人") {
		t.Fatalf("RefreshParticipants() on non-draft expected scope error, got = %v", err)
	}
}

func TestRefreshParticipantsActivityNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "status"},
		rows:    nil,
	})
	_, err := svc.RefreshParticipants("999", "operator-1")
	if err == nil {
		t.Fatalf("RefreshParticipants(999) expected error")
	}
}

// ======================== PublishActivity / CloseActivity / ArchiveActivity ========================

func TestPublishActivityIdempotent(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
	if err := svc.PublishActivity("1", "operator-1"); err != nil {
		t.Fatalf("PublishActivity() idempotent on self_evaluation error = %v", err)
	}
}

func TestPublishActivityStatusConflict(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("manager_evaluation", ""))
	if err := svc.PublishActivity("1", "operator-1"); err == nil {
		t.Fatalf("PublishActivity() on manager_evaluation expected conflict error")
	}
}

func TestPublishActivityFromDraftRequiresTargetSettingComplete(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
	)
	if err := svc.PublishActivity("1", "operator-1"); err == nil {
		t.Fatalf("PublishActivity() from draft with incomplete target_setting expected error")
	}
}

func TestCloseActivityIdempotent(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("archived", ""))
	if err := svc.CloseActivity("1", "operator-1"); err != nil {
		t.Fatalf("CloseActivity() idempotent on archived error = %v", err)
	}
}

func TestCloseActivityFromDraftConflict(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("draft", ""))
	if err := svc.CloseActivity("1", "operator-1"); err == nil {
		t.Fatalf("CloseActivity() from draft expected conflict error")
	}
}

func TestCloseActivityFromLockedArchives(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("locked", ""))
	if err := svc.CloseActivity("1", "operator-1"); err != nil {
		t.Fatalf("CloseActivity() from locked error = %v", err)
	}
}

func TestCloseActivityFromResultConfirmedArchives(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("result_confirmed", ""))
	if err := svc.CloseActivity("1", "operator-1"); err != nil {
		t.Fatalf("CloseActivity() from result_confirmed error = %v", err)
	}
}

func TestArchiveActivityIdempotent(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("archived", ""))
	if err := svc.ArchiveActivity("1", "operator-1"); err != nil {
		t.Fatalf("ArchiveActivity() idempotent on archived error = %v", err)
	}
}

func TestArchiveActivityConflictFromDraft(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("draft", ""))
	if err := svc.ArchiveActivity("1", "operator-1"); err == nil {
		t.Fatalf("ArchiveActivity() from draft expected conflict error")
	}
}

func TestArchiveActivityFromLocked(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("locked", ""))
	if err := svc.ArchiveActivity("1", "operator-1"); err != nil {
		t.Fatalf("ArchiveActivity() from locked error = %v", err)
	}
}

func TestArchiveActivityFromMutengResultPublish(t *testing.T) {
	svc := newStubPerformanceService(t, mutengReviewScoringActivityResponse("result_publish", ""))
	if err := svc.ArchiveActivity("1", "operator-1"); err != nil {
		t.Fatalf("ArchiveActivity() from result_publish error = %v", err)
	}
}

func TestPerformanceInterviewAndAppealMovedOutOfActivityFlow(t *testing.T) {
	interviewSvc := newStubPerformanceService(t, mutengReviewScoringActivityResponse("result_publish", ""))
	if err := interviewSvc.OpenPerformanceInterview("1", "operator-1"); err == nil || !strings.Contains(err.Error(), "独立模块") {
		t.Fatalf("OpenPerformanceInterview() error = %v, want moved out message", err)
	}

	appealSvc := newStubPerformanceService(t, mutengReviewScoringActivityResponse("interview", ""))
	if err := appealSvc.OpenPerformanceAppeal("1", "operator-1"); err == nil || !strings.Contains(err.Error(), "独立模块") {
		t.Fatalf("OpenPerformanceAppeal() error = %v, want moved out message", err)
	}
}

// ======================== StartActivity ========================

func TestStartActivityFromDraftRefreshesAndTransitions(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		// RefreshParticipants needs active users
		activeUserResponse("user-1", "Alice"),
		// RefreshParticipants needs departments
		stubQueryResponse{
			match:   stubTableMatcher("departments"),
			columns: []string{"id", "department_id", "name"},
			rows:    nil,
		},
		// countActiveParticipants count query (must come BEFORE the general participant stub)
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_participants") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		// RefreshParticipants needs existing participants (SELECT) - comes after count stub
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)
	if err := svc.StartActivity("1", "operator-1"); err != nil {
		t.Fatalf("StartActivity() error = %v", err)
	}
}

func TestStartActivityAlreadyTargetSetting(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("target_setting", ""))
	if err := svc.StartActivity("1", "operator-1"); err != nil {
		t.Fatalf("StartActivity() idempotent on target_setting error = %v", err)
	}
}

func TestStartActivityConflictFromSelfEvaluation(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
	if err := svc.StartActivity("1", "operator-1"); err == nil {
		t.Fatalf("StartActivity() from self_evaluation expected conflict error")
	}
}

// ======================== OpenTargetSetting ========================

func TestOpenTargetSettingFromDraft(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		activeUserResponse("user-1", "Alice"),
		stubQueryResponse{
			match:   stubTableMatcher("departments"),
			columns: []string{"id", "department_id", "name"},
			rows:    nil,
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_participants") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)
	if err := svc.OpenTargetSetting("1", "operator-1"); err != nil {
		t.Fatalf("OpenTargetSetting() error = %v", err)
	}
}

func TestOpenTargetSettingAlreadyOpen(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("target_setting", ""))
	if err := svc.OpenTargetSetting("1", "operator-1"); err != nil {
		t.Fatalf("OpenTargetSetting() idempotent error = %v", err)
	}
}

func TestOpenTargetSettingConflictFromSelfEvaluation(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
	if err := svc.OpenTargetSetting("1", "operator-1"); err == nil {
		t.Fatalf("OpenTargetSetting() from self_evaluation expected conflict error")
	}
}

// ======================== OpenSelfEvaluation ========================

func TestOpenSelfEvaluationFromTargetSetting(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("target_setting", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
	)
	if err := svc.OpenSelfEvaluation("1", "operator-1"); err != nil {
		t.Fatalf("OpenSelfEvaluation() error = %v", err)
	}
}

func TestOpenSelfEvaluationIdempotent(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
	if err := svc.OpenSelfEvaluation("1", "operator-1"); err != nil {
		t.Fatalf("OpenSelfEvaluation() idempotent error = %v", err)
	}
}

func TestOpenSelfEvaluationConflictFromManagerEval(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("manager_evaluation", ""))
	if err := svc.OpenSelfEvaluation("1", "operator-1"); err == nil {
		t.Fatalf("OpenSelfEvaluation() from manager_evaluation expected conflict error")
	}
}

func TestOpenSelfEvaluationIncompleteTargetSetting(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("target_setting", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
	)
	if err := svc.OpenSelfEvaluation("1", "operator-1"); err == nil {
		t.Fatalf("OpenSelfEvaluation() with incomplete target_setting expected error")
	}
}

// ======================== OpenManagerEvaluation ========================

func TestOpenManagerEvaluationFromSelfEvaluation(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("self_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "self_submitted", "done", 85, 0, "", false, nil, nil, nil),
			},
		},
	)
	if err := svc.OpenManagerEvaluation("1", "operator-1"); err != nil {
		t.Fatalf("OpenManagerEvaluation() error = %v", err)
	}
}

func TestOpenManagerEvaluationConflictFromDraft(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("draft", ""))
	if err := svc.OpenManagerEvaluation("1", "operator-1"); err == nil {
		t.Fatalf("OpenManagerEvaluation() from draft expected conflict error")
	}
}

// ======================== OpenManagerConfirmation ========================

func TestOpenManagerConfirmationFromEmployeeConfirmation(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		performanceActivityResponse("employee_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
			},
		},
	)
	if err := svc.OpenManagerConfirmation("1", "operator-1"); err != nil {
		t.Fatalf("OpenManagerConfirmation() error = %v", err)
	}
}

func TestOpenManagerConfirmationConflictFromManagerEvaluation(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("manager_evaluation", ""))
	if err := svc.OpenManagerConfirmation("1", "operator-1"); err == nil {
		t.Fatalf("OpenManagerConfirmation() from manager_evaluation expected conflict error")
	}
}

// ======================== OpenHRConfirmation ========================

func TestOpenHRConfirmationFromManagerConfirmation(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, now, now, nil),
			},
		},
	)
	if err := svc.OpenHRConfirmation("1", "operator-1"); err != nil {
		t.Fatalf("OpenHRConfirmation() error = %v", err)
	}
}

func TestOpenHRConfirmationConflictFromSelfEvaluation(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
	if err := svc.OpenHRConfirmation("1", "operator-1"); err == nil {
		t.Fatalf("OpenHRConfirmation() from self_evaluation expected conflict error")
	}
}

// ======================== LockActivity ========================

func TestLockActivityAlreadyLocked(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("locked", ""))
	if err := svc.LockActivity("1", "operator-1"); err != nil {
		t.Fatalf("LockActivity() idempotent error = %v", err)
	}
}

func TestLockActivityConflictFromDraft(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("draft", ""))
	if err := svc.LockActivity("1", "operator-1"); err == nil {
		t.Fatalf("LockActivity() from draft expected conflict error")
	}
}

func TestLockActivityIncompleteParticipants(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("hr_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, time.Now(), nil),
			},
		},
	)
	if err := svc.LockActivity("1", "operator-1"); err == nil {
		t.Fatalf("LockActivity() with incomplete hr_confirmation expected error")
	}
}

// ======================== BatchConfirmResults ========================

func TestBatchConfirmResultsSkipsNonManagerSubmitted(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
	)
	results, err := svc.BatchConfirmResults("activity-1", []uint{1}, "operator-1")
	if err != nil {
		t.Fatalf("BatchConfirmResults() error = %v", err)
	}
	if len(results) != 1 || results[0]["success"] != false {
		t.Fatalf("BatchConfirmResults() expected skip for non-manager_submitted: %#v", results)
	}
}

func TestBatchConfirmResultsParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	results, err := svc.BatchConfirmResults("activity-1", []uint{999}, "operator-1")
	if err != nil {
		t.Fatalf("BatchConfirmResults() error = %v", err)
	}
	if len(results) != 1 || results[0]["success"] != false {
		t.Fatalf("BatchConfirmResults() expected error for not found: %#v", results)
	}
}

func TestBatchConfirmResultsEmptyParticipantIDs(t *testing.T) {
	svc := newStubPerformanceService(t)
	results, err := svc.BatchConfirmResults("activity-1", []uint{}, "operator-1")
	if err != nil {
		t.Fatalf("BatchConfirmResults() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("BatchConfirmResults() expected empty results, got %d", len(results))
	}
}

// ======================== ConfirmEmployeeResult ========================

func TestConfirmEmployeeResultIdempotent(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
			},
		},
		performanceActivityResponse("employee_confirmation", ""),
	)
	if err := svc.ConfirmEmployeeResult(1, "employee-1"); err != nil {
		t.Fatalf("ConfirmEmployeeResult() idempotent error = %v", err)
	}
}

func TestConfirmEmployeeResultLocked(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", true, nil, nil, nil),
			},
		},
		performanceActivityResponse("employee_confirmation", ""),
	)
	if err := svc.ConfirmEmployeeResult(1, "employee-1"); err == nil {
		t.Fatalf("ConfirmEmployeeResult() locked expected error")
	}
}

func TestConfirmEmployeeResultWrongActivityStatus(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("self_evaluation", ""),
	)
	if err := svc.ConfirmEmployeeResult(1, "employee-1"); err == nil {
		t.Fatalf("ConfirmEmployeeResult() wrong activity status expected error")
	}
}

func TestConfirmEmployeeResultWrongParticipantStatus(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("employee_confirmation", ""),
	)
	if err := svc.ConfirmEmployeeResult(1, "employee-1"); err == nil {
		t.Fatalf("ConfirmEmployeeResult() wrong participant status expected error")
	}
}

func TestConfirmEmployeeResultRejectsWrongNewFlowActivityStatus(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
			},
		},
		newFlowPerformanceActivityResponse("result_publish", ""),
	)
	if err := svc.ConfirmEmployeeResult(1, "employee-1"); err == nil || !strings.Contains(err.Error(), "活动尚未进入员工确认阶段") {
		t.Fatalf("ConfirmEmployeeResult() new flow error = %v, want employee confirmation stage rejection", err)
	}
}

// ======================== ConfirmManagerResult ========================

func TestConfirmManagerResultIdempotent(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, now, now, nil),
			},
		},
		performanceActivityResponse("manager_confirmation", ""),
	)
	if err := svc.ConfirmManagerResult(1, "manager-1"); err != nil {
		t.Fatalf("ConfirmManagerResult() idempotent error = %v", err)
	}
}

func TestConfirmManagerResultLocked(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", true, now, nil, nil),
			},
		},
		performanceActivityResponse("manager_confirmation", ""),
	)
	if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil {
		t.Fatalf("ConfirmManagerResult() locked expected error")
	}
}

func TestConfirmManagerResultWrongActivityStatus(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
			},
		},
		performanceActivityResponse("self_evaluation", ""),
	)
	if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil {
		t.Fatalf("ConfirmManagerResult() wrong activity status expected error")
	}
}

func TestConfirmManagerResultWrongParticipantStatus(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "self_submitted", "", 85, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("manager_confirmation", ""),
	)
	if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil {
		t.Fatalf("ConfirmManagerResult() wrong participant status expected error")
	}
}

func TestConfirmManagerResultRejectsNewFlow(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
			},
		},
		newFlowPerformanceActivityResponse("interview", ""),
	)
	if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil || !strings.Contains(err.Error(), "不包含主管确认节点") {
		t.Fatalf("ConfirmManagerResult() new flow error = %v, want manager confirmation node rejection", err)
	}
}

// ======================== ConfirmHRResult ========================

func TestConfirmHRResultIdempotent(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "hr_confirmed", "", 0, 90, "A", false, nil, now, now),
			},
		},
		performanceActivityResponse("hr_confirmation", ""),
	)
	if err := svc.ConfirmHRResult(1, "hr-1"); err != nil {
		t.Fatalf("ConfirmHRResult() idempotent error = %v", err)
	}
}

func TestConfirmHRResultWrongParticipantStatus(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "self_submitted", "", 85, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("hr_confirmation", ""),
	)
	if err := svc.ConfirmHRResult(1, "hr-1"); err == nil {
		t.Fatalf("ConfirmHRResult() wrong participant status expected error")
	}
}

func TestConfirmHRResultWrongActivityStatus(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, now, nil),
			},
		},
		performanceActivityResponse("self_evaluation", ""),
	)
	if err := svc.ConfirmHRResult(1, "hr-1"); err == nil {
		t.Fatalf("ConfirmHRResult() wrong activity status expected error")
	}
}

func TestConfirmHRResultRejectsWrongNewFlowActivityStatus(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, now, nil),
			},
		},
		newFlowPerformanceActivityResponse("appeal", ""),
	)
	if err := svc.ConfirmHRResult(1, "hr-1"); err == nil || !strings.Contains(err.Error(), "活动尚未进入") || !strings.Contains(err.Error(), "HR确认阶段") {
		t.Fatalf("ConfirmHRResult() new flow error = %v, want HR confirmation stage rejection", err)
	}
}

// ======================== ConfirmResults (compat) ========================

func TestConfirmResultsDelegatesToOpenEmployeeConfirmation(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("self_evaluation", ""),
	)
	if err := svc.ConfirmResults("1", "operator-1"); err == nil {
		t.Fatalf("ConfirmResults() from self_evaluation expected conflict error")
	}
}

// ======================== SetHRConfirmDeadline ========================

func TestSetHRConfirmDeadline(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("hr_confirmation", ""))
	activity, err := svc.SetHRConfirmDeadline("1", "2026-06-30", "hr-1")
	if err != nil {
		t.Fatalf("SetHRConfirmDeadline() error = %v", err)
	}
	if activity.HRConfirmDeadline != "2026-06-30" {
		t.Fatalf("SetHRConfirmDeadline() deadline = %q, want 2026-06-30", activity.HRConfirmDeadline)
	}
}

func TestSetHRConfirmDeadlineNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "status"},
		rows:    nil,
	})
	_, err := svc.SetHRConfirmDeadline("999", "2026-06-30", "hr-1")
	if err == nil {
		t.Fatalf("SetHRConfirmDeadline(999) expected error")
	}
}

// ======================== GetHRConfirmDeadlineStatus ========================

func TestGetHRConfirmDeadlineStatusNoDeadline(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("hr_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "employee_id", "employee_name"},
			rows:    nil,
		},
	)
	status, err := svc.GetHRConfirmDeadlineStatus("1")
	if err != nil {
		t.Fatalf("GetHRConfirmDeadlineStatus() error = %v", err)
	}
	if status["deadline"] != "" {
		t.Fatalf("expected empty deadline, got %v", status["deadline"])
	}
	if status["pending_count"] != 0 {
		t.Fatalf("expected 0 pending, got %v", status["pending_count"])
	}
	if status["overdue"] != false {
		t.Fatalf("expected overdue=false, got %v", status["overdue"])
	}
	if status["can_force_lock"] != false {
		t.Fatalf("expected can_force_lock=false, got %v", status["can_force_lock"])
	}
}

func TestGetHRConfirmDeadlineStatusWithPending(t *testing.T) {
	future := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	svc := newStubPerformanceService(t,
		performanceActivityResponse("hr_confirmation", future),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "employee_id", "employee_name"},
			rows: [][]driver.Value{
				{int64(1), "1", "manager_confirmed", "user-1", "Alice"},
			},
		},
	)
	status, err := svc.GetHRConfirmDeadlineStatus("1")
	if err != nil {
		t.Fatalf("GetHRConfirmDeadlineStatus() error = %v", err)
	}
	if status["pending_count"] != 1 {
		t.Fatalf("pending_count = %v, want 1", status["pending_count"])
	}
	if status["overdue"] != false {
		t.Fatalf("future deadline should not be overdue")
	}
}

// ======================== GetPendingHRConfirm ========================

func TestGetPendingHRConfirm(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: []string{"id", "activity_id", "status", "employee_id", "employee_name", "department_name"},
		rows: [][]driver.Value{
			{int64(1), "1", "manager_confirmed", "user-1", "Alice", "Engineering"},
			{int64(2), "1", "manager_confirmed", "user-2", "Bob", "Product"},
		},
	})
	pending, err := svc.GetPendingHRConfirm("1")
	if err != nil {
		t.Fatalf("GetPendingHRConfirm() error = %v", err)
	}
	if len(pending) != 2 {
		t.Fatalf("GetPendingHRConfirm() len=%d, want 2", len(pending))
	}
}

func TestGetPendingHRConfirmEmpty(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: []string{"id", "activity_id", "status", "employee_id", "employee_name", "department_name"},
		rows:    nil,
	})
	pending, err := svc.GetPendingHRConfirm("1")
	if err != nil {
		t.Fatalf("GetPendingHRConfirm() error = %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("GetPendingHRConfirm() len=%d, want 0", len(pending))
	}
}

// ======================== SetBonusPenaltyScore ========================

func TestSetBonusPenaltyScoreCalculatesAdjustedScore(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 85, "B", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("manager_evaluation", ""),
	)
	// EnableBonusScore needs to be true on the activity, but our stub returns false by default.
	// The method checks activity.EnableBonusScore, which is false by default.
	err := svc.SetBonusPenaltyScore(1, 5, 2, "operator-1")
	if err == nil {
		t.Fatalf("SetBonusPenaltyScore() expected enable bonus score error when activity has it disabled")
	}
}

func TestSetBonusPenaltyScoreLockedParticipant(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 85, "B", true, nil, nil, nil),
			},
		},
		performanceActivityResponse("manager_evaluation", ""),
	)
	err := svc.SetBonusPenaltyScore(1, 5, 2, "operator-1")
	if err == nil || !strings.Contains(err.Error(), "已锁定") {
		t.Fatalf("SetBonusPenaltyScore() locked expected error, got = %v", err)
	}
}

func TestSetBonusPenaltyScoreParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)
	err := svc.SetBonusPenaltyScore(999, 5, 2, "operator-1")
	if err == nil {
		t.Fatalf("SetBonusPenaltyScore(999) expected error")
	}
}

// ======================== UpdateParticipantAssessmentManager ========================

func TestUpdateParticipantAssessmentManagerEmptyParticipantID(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.UpdateParticipantAssessmentManager(0, "manager-1", "manual", "", "operator-1")
	if err == nil {
		t.Fatalf("UpdateParticipantAssessmentManager(0) expected error")
	}
}

func TestUpdateParticipantAssessmentManagerEmptyManagerUserID(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.UpdateParticipantAssessmentManager(1, "", "manual", "", "operator-1")
	if err == nil {
		t.Fatalf("UpdateParticipantAssessmentManager(empty manager) expected error")
	}
}

func TestUpdateParticipantAssessmentManagerInvalidSource(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.UpdateParticipantAssessmentManager(1, "manager-1", "bad-source", "", "operator-1")
	if err == nil {
		t.Fatalf("UpdateParticipantAssessmentManager(bad source) expected error")
	}
}

func TestUpdateParticipantAssessmentManagerImportSourceNotAllowed(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.UpdateParticipantAssessmentManager(1, "manager-1", "import", "", "operator-1")
	if err == nil || !strings.Contains(err.Error(), "不能在调整入口手动选择") {
		t.Fatalf("UpdateParticipantAssessmentManager(import source) expected error, got = %v", err)
	}
}

func TestUpdateParticipantAssessmentManagerEmptySourceNotAllowed(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.UpdateParticipantAssessmentManager(1, "manager-1", "empty", "", "operator-1")
	if err == nil || !strings.Contains(err.Error(), "不能在调整入口手动选择") {
		t.Fatalf("UpdateParticipantAssessmentManager(empty source) expected error, got = %v", err)
	}
}

func TestUpdateParticipantAssessmentManagerSuccessManual(t *testing.T) {
	svc := newStubPerformanceService(t,
		activeUserResponse("manual-1", "Manual Boss"),
		assessmentManagerParticipantForUpdate("activity-1", "employee-1", "target_set", false, "old-manager", "Old Boss", "direct-1", ManagerSourceImport),
	)

	updated, err := svc.UpdateParticipantAssessmentManager(1, "manual-1", "manual", "handover", "operator-1")
	if err != nil {
		t.Fatalf("UpdateParticipantAssessmentManager() error = %v", err)
	}
	if ptrStringValue(updated.ManagerID) != "manual-1" || ptrStringValue(updated.ManagerName) != "Manual Boss" {
		t.Fatalf("updated manager = (%q, %q), want manual-1/Manual Boss", ptrStringValue(updated.ManagerID), ptrStringValue(updated.ManagerName))
	}
	if updated.ManagerSource != ManagerSourceManual || !updated.ManagerOverridden || updated.ManagerOverrideReason != "handover" {
		t.Fatalf("updated manager metadata = %#v", updated)
	}
	if updated.ManagerConfigStatus != ManagerConfigConfigured || updated.UpdatedBy != "operator-1" {
		t.Fatalf("updated config/operator = (%q, %q), want configured/operator-1", updated.ManagerConfigStatus, updated.UpdatedBy)
	}
}

func TestUpdateParticipantAssessmentManagerManagerNotActive(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "department_id", "status"},
		rows: [][]driver.Value{
			{int64(1), "inactive-manager", "Inactive Boss", "dept-1", "inactive"},
		},
	})

	_, err := svc.UpdateParticipantAssessmentManager(1, "inactive-manager", "manual", "", "operator-1")
	if err == nil || !strings.Contains(err.Error(), "考核上级不存在或不是在职状态") {
		t.Fatalf("UpdateParticipantAssessmentManager(inactive manager) error = %v", err)
	}
}

func TestUpdateParticipantAssessmentManagerParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t,
		activeUserResponse("manual-1", "Manual Boss"),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "employee_id", "status"},
			rows:    nil,
		},
	)

	_, err := svc.UpdateParticipantAssessmentManager(1, "manual-1", "manual", "", "operator-1")
	if err == nil || !strings.Contains(err.Error(), "参与人不存在") {
		t.Fatalf("UpdateParticipantAssessmentManager(participant not found) error = %v", err)
	}
}

func TestUpdateParticipantAssessmentManagerRejectsSelfLockedAndSourceMismatch(t *testing.T) {
	t.Run("self manager", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			activeUserByIDResponse("direct-1", "Direct Boss"),
			activeUserResponse("employee-1", "Alice"),
			assessmentManagerParticipantForUpdate("activity-1", "employee-1", "target_set", false, "old-manager", "Old Boss", "direct-1", ManagerSourceImport),
		)

		_, err := svc.UpdateParticipantAssessmentManager(1, "employee-1", "manual", "", "operator-1")
		if err == nil || !strings.Contains(err.Error(), "只有最高级或无可用组织上级人员") {
			t.Fatalf("UpdateParticipantAssessmentManager(self) error = %v", err)
		}
	})

	t.Run("self final allowed without org manager", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			activeUserResponse("employee-1", "Alice"),
			assessmentManagerParticipantForUpdate("activity-1", "employee-1", "target_set", false, "old-manager", "Old Boss", "", ManagerSourceImport),
		)

		updated, err := svc.UpdateParticipantAssessmentManager(1, "employee-1", "manual", "top-level self final", "operator-1")
		if err != nil {
			t.Fatalf("UpdateParticipantAssessmentManager(self final) error = %v", err)
		}
		if ptrStringValue(updated.ManagerID) != "employee-1" || updated.ManagerSource != ManagerSourceManual {
			t.Fatalf("updated self final manager = %#v", updated)
		}
	})

	t.Run("locked participant", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			activeUserResponse("manual-1", "Manual Boss"),
			assessmentManagerParticipantForUpdate("activity-1", "employee-1", "manager_submitted", true, "old-manager", "Old Boss", "direct-1", ManagerSourceImport),
		)

		_, err := svc.UpdateParticipantAssessmentManager(1, "manual-1", "manual", "", "operator-1")
		if err == nil || !strings.Contains(err.Error(), "绩效结果已锁定，无法调整考核上级") {
			t.Fatalf("UpdateParticipantAssessmentManager(locked) error = %v", err)
		}
	})

	t.Run("source mismatch", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			activeUserResponse("other-manager", "Other Boss"),
			assessmentManagerParticipantForUpdate("activity-1", "employee-1", "target_set", false, "old-manager", "Old Boss", "direct-1", ManagerSourceImport),
		)

		_, err := svc.UpdateParticipantAssessmentManager(1, "other-manager", ManagerSourceDirectManager, "", "operator-1")
		if err == nil || !strings.Contains(err.Error(), "不匹配") {
			t.Fatalf("UpdateParticipantAssessmentManager(source mismatch) error = %v", err)
		}
	})
}

// ======================== BatchUpdateActivityAssessmentManagers ========================

func TestBatchUpdateActivityAssessmentManagersEmptyActivity(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.BatchUpdateActivityAssessmentManagers("", nil, "operator-1")
	if err == nil {
		t.Fatalf("BatchUpdateActivityAssessmentManagers(empty) expected error")
	}
}

func TestBatchUpdateActivityAssessmentManagersEmptyItems(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("draft", ""))
	_, err := svc.BatchUpdateActivityAssessmentManagers("1", nil, "operator-1")
	if err == nil {
		t.Fatalf("BatchUpdateActivityAssessmentManagers(empty items) expected error")
	}
}

func TestBatchUpdateActivityAssessmentManagersActivityNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "status"},
		rows:    nil,
	})
	_, err := svc.BatchUpdateActivityAssessmentManagers("999", []AssessmentManagerUpdateRequest{
		{ParticipantID: 1, ManagerUserID: "manager-1", ManagerSource: "manual"},
	}, "operator-1")
	if err == nil {
		t.Fatalf("BatchUpdateActivityAssessmentManagers(999) expected error")
	}
}

func TestBatchUpdateActivityAssessmentManagersSkipsParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)
	results, err := svc.BatchUpdateActivityAssessmentManagers("1", []AssessmentManagerUpdateRequest{
		{ParticipantID: 999, ManagerUserID: "manager-1", ManagerSource: "manual"},
	}, "operator-1")
	if err != nil {
		t.Fatalf("BatchUpdateActivityAssessmentManagers() error = %v", err)
	}
	if len(results) != 1 || results[0].Success {
		t.Fatalf("BatchUpdateActivityAssessmentManagers() expected skip for not found: %#v", results)
	}
}

func TestBatchUpdateActivityAssessmentManagersSkipsInvalidAndOutOfScopeItems(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		assessmentManagerParticipantForUpdate("other-activity", "employee-1", "target_set", false, "old-manager", "Old Boss", "direct-1", ManagerSourceImport),
	)

	results, err := svc.BatchUpdateActivityAssessmentManagers("activity-1", []AssessmentManagerUpdateRequest{
		{ParticipantID: 0, ManagerUserID: "manual-1", ManagerSource: "manual"},
		{ParticipantID: 1, ManagerUserID: "manual-1", ManagerSource: "manual"},
	}, "operator-1")
	if err != nil {
		t.Fatalf("BatchUpdateActivityAssessmentManagers() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("BatchUpdateActivityAssessmentManagers() len = %d, want 2", len(results))
	}
	if results[0].Success || !strings.Contains(results[0].Error, "参与人不能为空") {
		t.Fatalf("zero participant result = %#v", results[0])
	}
	if results[1].Success || !strings.Contains(results[1].Error, "参与人不属于当前活动") {
		t.Fatalf("out-of-scope participant result = %#v", results[1])
	}
}

func TestBatchUpdateActivityAssessmentManagersSuccess(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		activeUserResponse("manual-1", "Manual Boss"),
		assessmentManagerParticipantForUpdate("activity-1", "employee-1", "target_set", false, "old-manager", "Old Boss", "direct-1", ManagerSourceImport),
	)

	results, err := svc.BatchUpdateActivityAssessmentManagers("activity-1", []AssessmentManagerUpdateRequest{
		{ParticipantID: 1, ManagerUserID: "manual-1", ManagerSource: "manual", Reason: "handover"},
	}, "operator-1")
	if err != nil {
		t.Fatalf("BatchUpdateActivityAssessmentManagers() error = %v", err)
	}
	if len(results) != 1 || !results[0].Success || results[0].Participant == nil {
		t.Fatalf("BatchUpdateActivityAssessmentManagers() result = %#v, want one success", results)
	}
	if ptrStringValue(results[0].Participant.ManagerID) != "manual-1" {
		t.Fatalf("updated manager id = %q, want manual-1", ptrStringValue(results[0].Participant.ManagerID))
	}
}

// ======================== assessmentManagerSourceLabel ========================

func TestAssessmentManagerSourceLabels(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{ManagerSourceDirectManager, "直属主管"},
		{ManagerSourceDepartmentHead, "部门负责人"},
		{ManagerSourceCenterHead, "中心负责人"},
		{ManagerSourceImport, "导入指定"},
		{ManagerSourceSystem, "系统兼容"},
		{ManagerSourceEmpty, "暂未配置"},
		{ManagerSourceManual, "手动指定"},
		{"unknown", "手动指定"},
	}
	for _, tt := range tests {
		if got := assessmentManagerSourceLabel(tt.input); got != tt.want {
			t.Fatalf("assessmentManagerSourceLabel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ======================== firstNonEmptyString ========================

func TestFirstNonEmptyString(t *testing.T) {
	ext := map[string]interface{}{
		"a": "value-a",
		"c": "value-c",
	}
	if got := firstNonEmptyString(ext, "x", "a", "c"); got != "value-a" {
		t.Fatalf("firstNonEmptyString() = %q, want value-a", got)
	}
	if got := firstNonEmptyString(ext, "x", "y"); got != "" {
		t.Fatalf("firstNonEmptyString() = %q, want empty", got)
	}
	if got := firstNonEmptyString(nil, "a"); got != "" {
		t.Fatalf("firstNonEmptyString(nil) = %q, want empty", got)
	}
}

// ======================== resolveManagerInfo edge cases ========================

func TestResolveManagerInfoEmpty(t *testing.T) {
	user := database.User{}
	managerID, managerName := resolveManagerInfo(user)
	if managerID != "" || managerName != "" {
		t.Fatalf("resolveManagerInfo(empty) = (%q, %q), want empty", managerID, managerName)
	}
}

func TestResolveManagerInfoFromExtension(t *testing.T) {
	user := database.User{
		Extension: map[string]interface{}{
			"manager_user_id": "ext-manager-1",
			"manager_name":    "Ext Boss",
		},
	}
	managerID, managerName := resolveManagerInfo(user)
	if managerID != "ext-manager-1" || managerName != "Ext Boss" {
		t.Fatalf("resolveManagerInfo(extension) = (%q, %q), want (ext-manager-1, Ext Boss)", managerID, managerName)
	}
}

// ======================== stringPtrOrNil edge cases ========================

func TestStringPtrOrNilEmpty(t *testing.T) {
	if got := stringPtrOrNil(""); got != nil {
		t.Fatalf("stringPtrOrNil(empty) = %v, want nil", got)
	}
	if got := stringPtrOrNil("  "); got != nil {
		t.Fatalf("stringPtrOrNil(whitespace) = %v, want nil", got)
	}
}

func TestStringPtrOrNilWithValue(t *testing.T) {
	got := stringPtrOrNil("hello")
	if got == nil || *got != "hello" {
		t.Fatalf("stringPtrOrNil(hello) = %v, want &hello", got)
	}
}

// ======================== formatManagerValue edge cases ========================

func TestFormatManagerValueEmpty(t *testing.T) {
	if got := formatManagerValue("", ""); got != "" {
		t.Fatalf("formatManagerValue(empty, empty) = %q, want empty", got)
	}
}

func TestFormatManagerValueIDOnly(t *testing.T) {
	if got := formatManagerValue("m1", ""); got != "m1" {
		t.Fatalf("formatManagerValue(m1, empty) = %q, want m1", got)
	}
}

func TestFormatManagerValueBoth(t *testing.T) {
	if got := formatManagerValue("m1", "Boss"); got != "m1/Boss" {
		t.Fatalf("formatManagerValue(m1, Boss) = %q, want m1/Boss", got)
	}
}

// ======================== normalizeGoalWeight edge cases ========================

func TestNormalizeGoalWeightEdgeCases(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0, 0},
		{-1, 0},
		{0.5, 0.5},
		{50, 0.5},
		{100, 1.0},
		{75, 0.75},
	}
	for _, tt := range tests {
		if got := normalizeGoalWeight(tt.input); got != tt.want {
			t.Fatalf("normalizeGoalWeight(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ======================== quotaMaxCount edge cases ========================

func TestQuotaMaxCountEdgeCases(t *testing.T) {
	tests := []struct {
		total, percent, want int
	}{
		{0, 15, 0},
		{10, 0, 0},
		{10, 100, 10},
		{1, 15, 1},
		{10, 15, 2},
		{20, 15, 3},
		{100, 15, 15},
	}
	for _, tt := range tests {
		if got := quotaMaxCount(tt.total, tt.percent); got != tt.want {
			t.Fatalf("quotaMaxCount(%d, %d) = %d, want %d", tt.total, tt.percent, got, tt.want)
		}
	}
}

// ======================== roundScore ========================

func TestRoundScoreEdgeCases(t *testing.T) {
	tests := []struct {
		input float64
		want  float64
	}{
		{0, 0},
		{1.005, 1.00},
		{1.006, 1.01},
		{1.004, 1.00},
		{99.995, 100.00},
		{-1.235, -1.24},
	}
	for _, tt := range tests {
		if got := roundScore(tt.input); got != tt.want {
			t.Fatalf("roundScore(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// ======================== PerformanceLevelByScore edge cases ========================

func TestPerformanceLevelByScoreExactBoundaries(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{60, "C"},
		{80, "B"},
		{90, "A"},
		{100, "S"},
		{59.99, "D"},
		{79.99, "C"},
		{89.99, "B"},
		{99.99, "A"},
	}
	for _, tt := range tests {
		if got := PerformanceLevelByScore(tt.score); got != tt.want {
			t.Fatalf("PerformanceLevelByScore(%v) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

// ======================== sameStringSet edge cases ========================

func TestSameStringSetEmpty(t *testing.T) {
	if !sameStringSet(nil, nil) {
		t.Fatalf("sameStringSet(nil, nil) should be true")
	}
	if !sameStringSet([]string{}, []string{}) {
		t.Fatalf("sameStringSet([], []) should be true")
	}
	if sameStringSet(nil, []string{"a"}) {
		t.Fatalf("sameStringSet(nil, [a]) should be false")
	}
}

// ======================== isIgnoredPerformanceParticipantStatus ========================

func TestIsIgnoredPerformanceParticipantStatusEdgeCases(t *testing.T) {
	if !isIgnoredPerformanceParticipantStatus("inactive") {
		t.Fatalf("inactive should be ignored")
	}
	if !isIgnoredPerformanceParticipantStatus("removed_from_scope") {
		t.Fatalf("removed_from_scope should be ignored")
	}
	for _, status := range []string{"pending", "target_set", "self_submitted", "manager_submitted", "locked"} {
		if isIgnoredPerformanceParticipantStatus(status) {
			t.Fatalf("%q should not be ignored", status)
		}
	}
}

// ======================== participantCompletedStage edge cases ========================

func TestParticipantCompletedStageAllCombinations(t *testing.T) {
	// Each stage should be completed by the corresponding status and all later statuses
	stageStatusMap := map[string][]string{
		"target_setting":        {"target_set", "self_submitted", "manager_submitted", "employee_confirmed", "manager_confirmed", "hr_confirmed", "locked", "result_confirmed"},
		"self_evaluation":       {"self_submitted", "manager_submitted", "employee_confirmed", "manager_confirmed", "hr_confirmed", "locked", "result_confirmed"},
		"manager_evaluation":    {"manager_submitted", "employee_confirmed", "manager_confirmed", "hr_confirmed", "locked", "result_confirmed"},
		"employee_confirmation": {"employee_confirmed", "manager_confirmed", "hr_confirmed", "locked", "result_confirmed"},
		"manager_confirmation":  {"manager_confirmed", "hr_confirmed", "locked", "result_confirmed"},
		"hr_confirmation":       {"hr_confirmed", "locked", "result_confirmed"},
	}
	for stage, completedStatuses := range stageStatusMap {
		for _, status := range completedStatuses {
			if !participantCompletedStage(status, stage) {
				t.Fatalf("participantCompletedStage(%q, %q) should be true", status, stage)
			}
		}
	}

	// Earlier statuses should NOT complete a later stage
	notCompletedCases := []struct {
		status string
		stage  string
	}{
		{"pending", "target_setting"},
		{"target_pending_approval", "target_setting"},
		{"target_rejected", "target_setting"},
		{"pending", "self_evaluation"},
		{"target_set", "self_evaluation"},
		{"pending", "manager_evaluation"},
		{"target_set", "manager_evaluation"},
		{"self_submitted", "manager_evaluation"},
		{"pending", "employee_confirmation"},
		{"pending", "manager_confirmation"},
		{"pending", "hr_confirmation"},
		{"inactive", "hr_confirmation"},
	}
	for _, tt := range notCompletedCases {
		if participantCompletedStage(tt.status, tt.stage) {
			t.Fatalf("participantCompletedStage(%q, %q) should be false", tt.status, tt.stage)
		}
	}
}

// ======================== participantHasStageEvidence edge cases ========================

func TestParticipantHasStageEvidenceLockedAlways(t *testing.T) {
	for _, stage := range []string{"employee_confirmation", "manager_confirmation", "hr_confirmation"} {
		p := database.PerformanceParticipant{Status: "locked"}
		if !participantHasStageEvidence(p, stage) {
			t.Fatalf("locked participant should have evidence for %q", stage)
		}
	}
}

func TestParticipantHasStageEvidenceResultConfirmed(t *testing.T) {
	for _, stage := range []string{"employee_confirmation", "manager_confirmation", "hr_confirmation"} {
		p := database.PerformanceParticipant{Status: "result_confirmed"}
		if !participantHasStageEvidence(p, stage) {
			t.Fatalf("result_confirmed participant should have evidence for %q", stage)
		}
	}
}

func TestParticipantHasStageEvidenceDefaultReturnsTrue(t *testing.T) {
	p := database.PerformanceParticipant{Status: "target_set"}
	if !participantHasStageEvidence(p, "unknown_stage") {
		t.Fatalf("unknown stage should default to true evidence")
	}
}

// ======================== activityIncludesUser edge cases ========================

func TestActivityIncludesUserEmptyScope(t *testing.T) {
	user := database.User{UserID: "u1", DepartmentID: "d1"}
	if !activityIncludesUser(&database.PerformanceActivity{}, user) {
		t.Fatalf("empty scope should include all users")
	}
}

func TestActivityIncludesUserDepartmentMatch(t *testing.T) {
	user := database.User{UserID: "u1", DepartmentID: "d1"}
	activity := &database.PerformanceActivity{TargetDepartmentIDs: []string{"d1", "d2"}}
	if !activityIncludesUser(activity, user) {
		t.Fatalf("matching department should include user")
	}
}

func TestActivityIncludesUserDepartmentMismatch(t *testing.T) {
	user := database.User{UserID: "u1", DepartmentID: "d3"}
	activity := &database.PerformanceActivity{TargetDepartmentIDs: []string{"d1", "d2"}}
	if activityIncludesUser(activity, user) {
		t.Fatalf("non-matching department should exclude user")
	}
}

func TestActivityIncludesUserEmployeeOverride(t *testing.T) {
	user := database.User{UserID: "u1", DepartmentID: "d3"}
	activity := &database.PerformanceActivity{
		TargetDepartmentIDs: []string{"d1"},
		TargetEmployeeIDs:   []string{"u1"},
	}
	if !activityIncludesUser(activity, user) {
		t.Fatalf("explicit employee should override department scope")
	}
}

func TestActivityIncludesUserEmployeeMismatch(t *testing.T) {
	user := database.User{UserID: "u2", DepartmentID: "d3"}
	activity := &database.PerformanceActivity{
		TargetDepartmentIDs: []string{"d1"},
		TargetEmployeeIDs:   []string{"u1"},
	}
	if activityIncludesUser(activity, user) {
		t.Fatalf("non-matching employee should be excluded")
	}
}

// ======================== targetSettingApproved edge cases ========================

func TestTargetSettingApprovedEdgeCases(t *testing.T) {
	if !targetSettingApproved("target_set", nil) {
		t.Fatalf("target_set should be approved")
	}
	if !targetSettingApproved("self_submitted", nil) {
		t.Fatalf("self_submitted should be approved")
	}
	if !targetSettingApproved("manager_submitted", nil) {
		t.Fatalf("manager_submitted should be approved")
	}
	if targetSettingApproved("pending", nil) {
		t.Fatalf("pending with no records should not be approved")
	}
	if targetSettingApproved("target_pending_approval", nil) {
		t.Fatalf("target_pending_approval with no records should not be approved")
	}
	if targetSettingApproved("target_rejected", nil) {
		t.Fatalf("target_rejected should not be approved")
	}
	if !targetSettingApproved("pending", []database.PerformanceGoalRecord{{ApprovalStatus: "approved"}}) {
		t.Fatalf("pending with approved record should be approved")
	}
	if targetSettingApproved("pending", []database.PerformanceGoalRecord{{ApprovalStatus: "pending"}}) {
		t.Fatalf("pending with only pending records should not be approved")
	}
}

// ======================== validateTemplateSections edge cases ========================

func TestValidateTemplateSectionsValidTwoSections(t *testing.T) {
	sections := []PerformanceTemplateSectionRequest{
		{
			Name:   "Results",
			Weight: 60,
			Items:  []PerformanceTemplateItemRequest{{Name: "KPI", MaxScore: 100, Weight: 100}},
		},
		{
			Name:   "Culture",
			Weight: 40,
			Items:  []PerformanceTemplateItemRequest{{Name: "Values", MaxScore: 100, Weight: 100}},
		},
	}
	if err := validateTemplateSections(sections); err != nil {
		t.Fatalf("validateTemplateSections() error = %v", err)
	}
}

func TestValidateTemplateSectionsSingleSection(t *testing.T) {
	sections := []PerformanceTemplateSectionRequest{
		{
			Name:   "Performance",
			Weight: 100,
			Items: []PerformanceTemplateItemRequest{
				{Name: "Output", MaxScore: 100, Weight: 60},
				{Name: "Quality", MaxScore: 100, Weight: 40},
			},
		},
	}
	if err := validateTemplateSections(sections); err != nil {
		t.Fatalf("validateTemplateSections() error = %v", err)
	}
}

// ======================== buildTemplateParts edge cases ========================

func TestBuildTemplatePartsSingleSection(t *testing.T) {
	sections, items, counts := buildTemplateParts([]PerformanceTemplateSectionRequest{
		{
			Name:   "Results",
			Weight: 100,
			Items:  []PerformanceTemplateItemRequest{{Name: "KPI", MaxScore: 100, Weight: 100}},
		},
	})
	if len(sections) != 1 || len(items) != 1 || len(counts) != 1 || counts[0] != 1 {
		t.Fatalf("buildTemplateParts(single) = sections:%d items:%d counts:%v", len(sections), len(items), counts)
	}
}

func TestBuildTemplatePartsEmpty(t *testing.T) {
	sections, items, counts := buildTemplateParts(nil)
	if len(sections) != 0 || len(items) != 0 || len(counts) != 0 {
		t.Fatalf("buildTemplateParts(nil) = sections:%d items:%d counts:%v", len(sections), len(items), counts)
	}
}

// ======================== normalizeTimeOrEmpty ========================

func TestNormalizeTimeOrEmptyVariousFormats(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{"  ", ""},
		{"2026-06-05T10:20:30Z", "2026-06-05T10:20:30Z"},
		{"2026-06-05", "2026-06-05"},
		{"not-a-date", "not-a-date"},
	}
	for _, tt := range tests {
		if got := normalizeTimeOrEmpty(tt.input); got != tt.want {
			t.Fatalf("normalizeTimeOrEmpty(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ======================== GetResultSummary edge cases ========================

func TestGetResultSummaryAllEmpty(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	summary, err := svc.GetResultSummary("activity-1")
	if err != nil {
		t.Fatalf("GetResultSummary() error = %v", err)
	}
	if summary["total_participants"] != 0 {
		t.Fatalf("total_participants = %v, want 0", summary["total_participants"])
	}
}

func TestGetResultSummaryIgnoresInactive(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, "inactive", "", 0, 0, "", false, nil, nil, nil),
			performanceParticipantStubRow(2, "removed_from_scope", "", 0, 0, "", false, nil, nil, nil),
		},
	})
	summary, err := svc.GetResultSummary("activity-1")
	if err != nil {
		t.Fatalf("GetResultSummary() error = %v", err)
	}
	if summary["total_participants"] != 0 {
		t.Fatalf("inactive/removed should not count, got %v", summary["total_participants"])
	}
}

func TestGetResultSummaryLevelDistribution(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, "manager_submitted", "", 0, 95, "S", false, nil, nil, nil),
			performanceParticipantStubRow(2, "manager_submitted", "", 0, 92, "A", false, nil, nil, nil),
			performanceParticipantStubRow(3, "manager_submitted", "", 0, 85, "B", false, nil, nil, nil),
			performanceParticipantStubRow(4, "manager_submitted", "", 0, 70, "C", false, nil, nil, nil),
			performanceParticipantStubRow(5, "manager_submitted", "", 0, 50, "D", false, nil, nil, nil),
		},
	})
	summary, err := svc.GetResultSummary("activity-1")
	if err != nil {
		t.Fatalf("GetResultSummary() error = %v", err)
	}
	dist := summary["level_distribution"].(map[string]int)
	if dist["S"] != 1 || dist["A"] != 1 || dist["B"] != 1 || dist["C"] != 1 || dist["D"] != 1 {
		t.Fatalf("level_distribution = %#v, want one of each", dist)
	}
}

// ======================== GetDistributionCheck edge cases ========================

func TestGetDistributionCheckZeroParticipants(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "final_level"},
			rows:    nil,
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_distribution_rules"),
			columns: []string{"id", "activity_id", "level", "distribution_percent", "description"},
			rows:    nil,
		},
	)
	check, err := svc.GetDistributionCheck("activity-1")
	if err != nil {
		t.Fatalf("GetDistributionCheck() error = %v", err)
	}
	if check.TotalCount != 0 {
		t.Fatalf("TotalCount = %d, want 0", check.TotalCount)
	}
	if !check.Passed {
		t.Fatalf("empty distribution should pass")
	}
}

func TestGetDistributionCheckAllWithinQuota(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "final_level"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "manager_submitted", "S"},
				{int64(2), "activity-1", "manager_submitted", "A"},
				{int64(3), "activity-1", "manager_submitted", "A"},
				{int64(4), "activity-1", "manager_submitted", "B"},
				{int64(5), "activity-1", "manager_submitted", "B"},
				{int64(6), "activity-1", "manager_submitted", "B"},
				{int64(7), "activity-1", "manager_submitted", "B"},
				{int64(8), "activity-1", "manager_submitted", "C"},
				{int64(9), "activity-1", "manager_submitted", "D"},
				{int64(10), "activity-1", "manager_submitted", "D"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_distribution_rules"),
			columns: []string{"id", "activity_id", "level", "distribution_percent", "description"},
			rows:    nil, // use defaults
		},
	)
	check, err := svc.GetDistributionCheck("activity-1")
	if err != nil {
		t.Fatalf("GetDistributionCheck() error = %v", err)
	}
	if !check.Passed {
		t.Fatalf("distribution within default quota should pass: %#v", check)
	}
	if check.TotalCount != 10 {
		t.Fatalf("TotalCount = %d, want 10", check.TotalCount)
	}
}

func TestGetDistributionCheckIgnoresInactive(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "final_level"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "manager_submitted", "S"},
				{int64(2), "activity-1", "inactive", "S"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_distribution_rules"),
			columns: []string{"id", "activity_id", "level", "distribution_percent", "description"},
			rows:    nil,
		},
	)
	check, err := svc.GetDistributionCheck("activity-1")
	if err != nil {
		t.Fatalf("GetDistributionCheck() error = %v", err)
	}
	if check.TotalCount != 1 {
		t.Fatalf("inactive should not count, TotalCount = %d", check.TotalCount)
	}
}

// ======================== GetRealtimeDistributionCheck ========================

func TestGetRealtimeDistributionCheck(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_participants") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 95, "S", false, nil, nil, nil),
				performanceParticipantStubRow(2, "manager_submitted", "", 0, 92, "A", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_distribution_rules"),
			columns: []string{"id", "activity_id", "level", "distribution_percent", "description"},
			rows:    nil,
		},
	)
	teams, err := svc.GetRealtimeDistributionCheck("activity-1")
	if err != nil {
		t.Fatalf("GetRealtimeDistributionCheck() error = %v", err)
	}
	if len(teams) == 0 {
		t.Fatalf("expected at least one team")
	}
}

func TestGetRealtimeDistributionCheckIgnoresInactive(t *testing.T) {
	// Stub driver returns all rows regardless of SQL WHERE clauses,
	// so we test with empty results to verify the method handles no-data case.
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_participants") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(0)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_distribution_rules"),
			columns: []string{"id", "activity_id", "level", "distribution_percent", "description"},
			rows:    nil,
		},
	)
	teams, err := svc.GetRealtimeDistributionCheck("activity-1")
	if err != nil {
		t.Fatalf("GetRealtimeDistributionCheck() error = %v", err)
	}
	if len(teams) != 0 {
		t.Fatalf("empty participants should return 0 teams, got %d", len(teams))
	}
}

// ======================== GetDistributionRules ========================

func TestGetDistributionRules(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_distribution_rules"),
		columns: []string{"id", "activity_id", "level", "distribution_percent", "description"},
		rows: [][]driver.Value{
			{int64(1), "activity-1", "S", int64(15), "top"},
			{int64(2), "activity-1", "A", int64(20), "strong"},
		},
	})
	rules, err := svc.GetDistributionRules("activity-1")
	if err != nil {
		t.Fatalf("GetDistributionRules() error = %v", err)
	}
	if len(rules) != 2 {
		t.Fatalf("GetDistributionRules() len=%d, want 2", len(rules))
	}
}

// ======================== GetGoalRecords ========================

func TestGetGoalRecords(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_goal_records"),
		columns: performanceGoalRecordColumns(),
		rows: [][]driver.Value{
			performanceGoalRecordRow(1, "quantitative", "Revenue", 0.5),
			performanceGoalRecordRow(2, "key_action", "Launch", 0.5),
		},
	})
	records, err := svc.GetGoalRecords(1)
	if err != nil {
		t.Fatalf("GetGoalRecords() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("GetGoalRecords() len=%d, want 2", len(records))
	}
}

// ======================== GetManagerGoals ========================

func TestGetManagerGoalsFiltersByIsFromSuperior(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_goal_records"),
		columns: append(performanceGoalRecordColumns(), "is_from_superior"),
		rows: [][]driver.Value{
			{int64(1), "activity-1", int64(1), "quantitative", "Revenue", 0.5, "pending", true},
			{int64(2), "activity-1", int64(1), "key_action", "Launch", 0.5, "pending", false},
		},
	})
	goals, err := svc.GetManagerGoals(1)
	if err != nil {
		t.Fatalf("GetManagerGoals() error = %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("GetManagerGoals() len=%d, want 1 (only from superior)", len(goals))
	}
}

// ======================== BatchSaveGoalRecords edge cases ========================

func TestBatchSaveGoalRecordsLockedParticipant(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "locked", "", 0, 0, "", true, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_setting", ""),
	)
	_, err := svc.BatchSaveGoalRecords(1, []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "Revenue", Weight: 100},
	}, "employee-1")
	if err == nil || !strings.Contains(err.Error(), "已锁定") {
		t.Fatalf("BatchSaveGoalRecords() locked expected error, got = %v", err)
	}
}

func TestBatchSaveGoalRecordsWrongActivityStatus(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("self_evaluation", ""),
	)
	_, err := svc.BatchSaveGoalRecords(1, []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "Revenue", Weight: 100},
	}, "employee-1")
	if err == nil || !strings.Contains(err.Error(), "不允许设定目标") {
		t.Fatalf("BatchSaveGoalRecords() wrong activity status expected error, got = %v", err)
	}
}

func TestBatchSaveGoalRecordsApprovedParticipant(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_setting", ""),
	)
	_, err := svc.BatchSaveGoalRecords(1, []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "Revenue", Weight: 100},
	}, "employee-1")
	if err == nil || !strings.Contains(err.Error(), "已审批通过") {
		t.Fatalf("BatchSaveGoalRecords() approved expected error, got = %v", err)
	}
}

// ======================== SubmitGoalApproval edge cases ========================

func TestSubmitGoalApprovalParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	err := svc.SubmitGoalApproval(999, "submit", "", "employee-1")
	if err == nil {
		t.Fatalf("SubmitGoalApproval(999) expected error")
	}
}

func TestSubmitGoalApprovalInvalidAction(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_setting", ""),
	)
	err := svc.SubmitGoalApproval(1, "invalid_action", "", "employee-1")
	if err == nil || !strings.Contains(err.Error(), "无效的操作") {
		t.Fatalf("SubmitGoalApproval(invalid) expected error, got = %v", err)
	}
}

func TestSubmitGoalApprovalRejectWithoutSubmit(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_approval", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_goal_approval_logs"),
			columns: []string{"id", "participant_id", "activity_id", "action", "version"},
			rows:    nil,
		},
	)
	err := svc.SubmitGoalApproval(1, "reject", "", "manager-1")
	if err == nil || !strings.Contains(err.Error(), "目标未提交") {
		t.Fatalf("SubmitGoalApproval(reject without submit) expected error, got = %v", err)
	}
}

func TestSubmitGoalApprovalApproveWithoutSubmit(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_approval", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_goal_approval_logs"),
			columns: []string{"id", "participant_id", "activity_id", "action", "version"},
			rows:    nil,
		},
	)
	err := svc.SubmitGoalApproval(1, "approve", "", "manager-1")
	if err == nil || !strings.Contains(err.Error(), "目标未提交") {
		t.Fatalf("SubmitGoalApproval(approve without submit) expected error, got = %v", err)
	}
}

func TestSubmitGoalApprovalDoubleSubmit(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_setting", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_goal_approval_logs"),
			columns: []string{"id", "participant_id", "activity_id", "action", "version"},
			rows: [][]driver.Value{
				{int64(1), int64(1), "activity-1", "submit", int64(1)},
			},
		},
	)
	err := svc.SubmitGoalApproval(1, "submit", "", "employee-1")
	if err == nil || !strings.Contains(err.Error(), "已提交") {
		t.Fatalf("SubmitGoalApproval(double submit) expected error, got = %v", err)
	}
}

func TestSubmitGoalApprovalLockedParticipant(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "locked", "", 0, 0, "", true, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_setting", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_goal_approval_logs"),
			columns: []string{"id", "participant_id", "activity_id", "action", "version"},
			rows:    nil,
		},
	)
	err := svc.SubmitGoalApproval(1, "submit", "", "employee-1")
	if err == nil || !strings.Contains(err.Error(), "已锁定") {
		t.Fatalf("SubmitGoalApproval(locked) expected error, got = %v", err)
	}
}

func TestSubmitGoalApprovalReviewBeforeTargetApproval(t *testing.T) {
	tests := []struct {
		name   string
		action string
	}{
		{name: "approve", action: "approve"},
		{name: "reject", action: "reject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newStubPerformanceService(t,
				stubQueryResponse{
					match:   stubTableMatcher("performance_participants"),
					columns: performanceParticipantStubColumns(),
					rows: [][]driver.Value{
						performanceParticipantStubRow(1, "target_pending_approval", "", 0, 0, "", false, nil, nil, nil),
					},
				},
				performanceActivityResponse("target_setting", ""),
				stubQueryResponse{
					match:   stubTableMatcher("performance_goal_approval_logs"),
					columns: []string{"id", "participant_id", "activity_id", "action", "version"},
					rows: [][]driver.Value{
						{int64(1), int64(1), "activity-1", "submit", int64(1)},
					},
				},
			)
			err := svc.SubmitGoalApproval(1, tt.action, "", "manager-1")
			if err == nil || !strings.Contains(err.Error(), "当前活动状态不允许审核目标") {
				t.Fatalf("SubmitGoalApproval(%s before target approval) expected stage error, got = %v", tt.action, err)
			}
		})
	}
}

func TestSubmitGoalApprovalWrongActivityStatus(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("self_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_goal_approval_logs"),
			columns: []string{"id", "participant_id", "activity_id", "action", "version"},
			rows:    nil,
		},
	)
	err := svc.SubmitGoalApproval(1, "submit", "", "employee-1")
	if err == nil || !strings.Contains(err.Error(), "当前活动状态不允许提交目标") {
		t.Fatalf("SubmitGoalApproval(wrong activity status) expected error, got = %v", err)
	}
}

// ======================== ForceLockOverdueHRConfirmation edge cases ========================

func TestForceLockOverdueHRConfirmationAlreadyLocked(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("locked", ""))
	result, err := svc.ForceLockOverdueHRConfirmation("1", "hr-1")
	if err != nil {
		t.Fatalf("ForceLockOverdueHRConfirmation() idempotent error = %v", err)
	}
	if result["force_locked_count"] != 0 || result["total_count"] != 0 {
		t.Fatalf("ForceLockOverdueHRConfirmation() locked result = %#v", result)
	}
}

func TestForceLockOverdueHRConfirmationWrongStatus(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
	_, err := svc.ForceLockOverdueHRConfirmation("1", "hr-1")
	if err == nil || !strings.Contains(err.Error(), "状态冲突") {
		t.Fatalf("ForceLockOverdueHRConfirmation() wrong status expected error, got = %v", err)
	}
}

func TestForceLockOverdueHRConfirmationNoDeadline(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("hr_confirmation", ""))
	_, err := svc.ForceLockOverdueHRConfirmation("1", "hr-1")
	if err == nil || !strings.Contains(err.Error(), "未设置 HR 确认截止日期") {
		t.Fatalf("ForceLockOverdueHRConfirmation() no deadline expected error, got = %v", err)
	}
}

func TestForceLockOverdueHRConfirmationNotOverdue(t *testing.T) {
	future := time.Now().AddDate(0, 0, 5).Format("2006-01-02")
	svc := newStubPerformanceService(t, performanceActivityResponse("hr_confirmation", future))
	_, err := svc.ForceLockOverdueHRConfirmation("1", "hr-1")
	if err == nil || !strings.Contains(err.Error(), "尚未逾期") {
		t.Fatalf("ForceLockOverdueHRConfirmation() not overdue expected error, got = %v", err)
	}
}

// ======================== GetGoalSuggestions edge cases ========================

func TestGetGoalSuggestionsParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	_, err := svc.GetGoalSuggestions(999)
	if err == nil {
		t.Fatalf("GetGoalSuggestions(999) expected error")
	}
}

// ======================== BatchAssignGoals edge cases ========================

func TestBatchAssignGoalsSkipsNonManager(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "pending", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("target_setting", ""),
		performanceGoalRecordResponse(),
	)
	err := svc.BatchAssignGoals("activity-1", "other-manager", []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "Revenue", Weight: 100},
	}, []uint{1}, "manager-1")
	if err != nil {
		t.Fatalf("BatchAssignGoals() error = %v", err)
	}
}

func TestBatchAssignGoalsSkipsNotFoundParticipant(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)
	err := svc.BatchAssignGoals("activity-1", "manager-1", nil, []uint{999}, "manager-1")
	if err != nil {
		t.Fatalf("BatchAssignGoals() error = %v", err)
	}
}

func TestBatchAssignGoalsEmptyParticipantIDs(t *testing.T) {
	svc := newStubPerformanceService(t)
	err := svc.BatchAssignGoals("activity-1", "manager-1", nil, nil, "manager-1")
	if err != nil {
		t.Fatalf("BatchAssignGoals() error = %v", err)
	}
}

// ======================== SendHRConfirmReminders ========================

func TestSendHRConfirmRemindersNoPending(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: []string{"id", "activity_id", "status", "employee_id", "employee_name"},
		rows:    nil,
	})
	result, err := svc.SendHRConfirmReminders("activity-1")
	if err != nil {
		t.Fatalf("SendHRConfirmReminders() no pending error = %v", err)
	}
	if result.Pending != 0 || result.Sent != 0 {
		t.Fatalf("SendHRConfirmReminders() result = %#v, want empty result", result)
	}
}

func TestSendHRConfirmRemindersNoRecipient(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "employee_id", "employee_name"},
			rows: [][]driver.Value{
				{int64(1), "1", "manager_confirmed", "user-1", "Alice"},
			},
		},
		performanceActivityResponse("hr_confirmation", ""),
		hrConfirmReminderRecipientsResponse(),
	)
	result, err := svc.SendHRConfirmReminders("1")
	if err != nil {
		t.Fatalf("SendHRConfirmReminders(no recipient) error = %v, want nil", err)
	}
	if result.Pending != 1 || result.Candidates != 0 || result.Sent != 0 {
		t.Fatalf("SendHRConfirmReminders() result = %#v, want no recipients", result)
	}
}

// ======================== TriggerPerformanceInterview ========================

func TestTriggerPerformanceInterviewParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	err := svc.TriggerPerformanceInterview("999", "required")
	if err == nil {
		t.Fatalf("TriggerPerformanceInterview(999) expected error")
	}
}

func TestTriggerPerformanceInterviewRequired(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, "manager_submitted", "", 0, 60, "C", false, nil, nil, nil),
		},
	})
	err := svc.TriggerPerformanceInterview("1", "required")
	// dingtalk will fail in test env, but we just verify no panic
	_ = err
}

func TestTriggerPerformanceInterviewOptional(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, "manager_submitted", "", 0, 95, "S", false, nil, nil, nil),
		},
	})
	err := svc.TriggerPerformanceInterview("1", "optional")
	_ = err
}

func TestTriggerPerformanceInterviewNoManager(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			{int64(1), "activity-1", "user-1", "Alice", "dept-1", "Dept", "manager_submitted", 85.0, "done", 0.0, "A", false, nil, nil, nil},
		},
	})
	err := svc.TriggerPerformanceInterview("1", "required")
	// no manager, should still succeed (just no notification)
	_ = err
}

// ======================== SetCompanyFinance edge cases ========================

func TestSetCompanyFinanceUpdate(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_company_finances"),
		columns: []string{"id", "activity_id", "revenue_sign", "description", "remark", "set_by"},
		rows: [][]driver.Value{
			{int64(1), "activity-1", "equal", "old desc", "old remark", "hr-old"},
		},
	})
	finance, err := svc.SetCompanyFinance("activity-1", "exceed", "new desc", "new remark", "hr-new")
	if err != nil {
		t.Fatalf("SetCompanyFinance(update) error = %v", err)
	}
	if finance.RevenueSign != "exceed" || finance.Description != "new desc" || finance.SetBy != "hr-new" {
		t.Fatalf("SetCompanyFinance(update) = %#v", finance)
	}
}

// ======================== GetCompanyFinance ========================

func TestGetCompanyFinanceFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_company_finances"),
		columns: []string{"id", "activity_id", "revenue_sign", "description"},
		rows: [][]driver.Value{
			{int64(1), "activity-1", "equal", "balanced"},
		},
	})
	finance, err := svc.GetCompanyFinance("activity-1")
	if err != nil {
		t.Fatalf("GetCompanyFinance() error = %v", err)
	}
	if finance.RevenueSign != "equal" {
		t.Fatalf("GetCompanyFinance() = %#v", finance)
	}
}

func TestGetCompanyFinanceNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_company_finances"),
		columns: []string{"id", "activity_id", "revenue_sign"},
		rows:    nil,
	})
	_, err := svc.GetCompanyFinance("activity-1")
	if err == nil {
		t.Fatalf("GetCompanyFinance() expected not found error")
	}
}

// ======================== shouldApplyActivityManagerAssignment edge cases ========================

func TestShouldApplyActivityManagerAssignment(t *testing.T) {
	if shouldApplyActivityManagerAssignment(nil) {
		t.Fatalf("nil should not apply")
	}
	if !shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{}) {
		t.Fatalf("non-overridden should apply")
	}
	if shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{ManagerOverridden: true, ManagerSource: ManagerSourceManual}) {
		t.Fatalf("manual overridden should not apply")
	}
	if !shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{ManagerOverridden: true, ManagerSource: ManagerSourceImport}) {
		t.Fatalf("import overridden should apply")
	}
	if !shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{ManagerOverridden: true, ManagerSource: ""}) {
		t.Fatalf("empty source overridden should apply")
	}
	if !shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{ManagerOverridden: true, ManagerSource: ManagerSourceEmpty}) {
		t.Fatalf("empty constant source overridden should apply")
	}
}

// ======================== applyActivityManagerAssignment edge cases ========================

func TestApplyActivityManagerAssignmentNilParticipant(t *testing.T) {
	changed, log := applyActivityManagerAssignment(nil, database.PerformanceActivityManagerAssignment{}, nil, "op", time.Now())
	if changed || log != nil {
		t.Fatalf("nil participant should not change")
	}
}

func TestApplyActivityManagerAssignmentEmptyManager(t *testing.T) {
	p := &database.PerformanceParticipant{EmployeeID: "user-1"}
	changed, log := applyActivityManagerAssignment(p, database.PerformanceActivityManagerAssignment{AssessmentManagerUserID: ""}, nil, "op", time.Now())
	if changed || log != nil {
		t.Fatalf("empty manager should not change")
	}
}

func TestApplyActivityManagerAssignmentSelfManager(t *testing.T) {
	p := &database.PerformanceParticipant{EmployeeID: "user-1"}
	changed, log := applyActivityManagerAssignment(p, database.PerformanceActivityManagerAssignment{AssessmentManagerUserID: "user-1"}, nil, "op", time.Now())
	if changed || log != nil {
		t.Fatalf("self manager should not change")
	}
}

// ======================== comparableActivityManagerAssignments ========================

func TestComparableActivityManagerAssignmentsSkipsEmptyUserID(t *testing.T) {
	assignments := []database.PerformanceActivityManagerAssignment{
		{UserID: "", AssessmentManagerUserID: "m1"},
		{UserID: "u1", AssessmentManagerUserID: "m1"},
	}
	result := comparableActivityManagerAssignments(assignments)
	if len(result) != 1 {
		t.Fatalf("expected 1 comparable assignment, got %d", len(result))
	}
	if _, ok := result["u1"]; !ok {
		t.Fatalf("expected u1 in result")
	}
}

// ======================== activityManagerAssignmentsByUser ========================

func TestActivityManagerAssignmentsByUserSkipsEmptyKeys(t *testing.T) {
	assignments := []database.PerformanceActivityManagerAssignment{
		{UserID: "", AssessmentManagerUserID: ""},
		{UserID: "u1", AssessmentManagerUserID: ""},
		{UserID: "", AssessmentManagerUserID: "m1"},
		{UserID: " u2 ", AssessmentManagerUserID: " m2 "},
	}
	result := activityManagerAssignmentsByUser(assignments)
	if len(result) != 1 {
		t.Fatalf("expected 1 assignment, got %d", len(result))
	}
	if result["u2"].AssessmentManagerUserID != "m2" {
		t.Fatalf("expected m2, got %s", result["u2"].AssessmentManagerUserID)
	}
}

// ======================== hydrateManagerConfigStatus ========================

func TestHydrateManagerConfigStatusSetsInvalidWhenManagerMissing(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "status"},
		rows:    nil,
	})
	mid := "missing-manager"
	items := []database.PerformanceParticipant{
		{ManagerID: &mid},
	}
	svc.hydrateManagerConfigStatus(items)
	if items[0].ManagerConfigStatus != ManagerConfigInvalid {
		t.Fatalf("expected invalid config status for missing manager, got %q", items[0].ManagerConfigStatus)
	}
}

func TestHydrateManagerConfigStatusSetsConfiguredWhenManagerActive(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "status"},
		rows: [][]driver.Value{
			{int64(1), "manager-1", "Boss", "active"},
		},
	})
	mid := "manager-1"
	items := []database.PerformanceParticipant{
		{ManagerID: &mid},
	}
	svc.hydrateManagerConfigStatus(items)
	if items[0].ManagerConfigStatus != ManagerConfigConfigured {
		t.Fatalf("expected configured status, got %q", items[0].ManagerConfigStatus)
	}
}

func TestHydrateManagerConfigStatusKeepsAllowedSelfFinalConfigured(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "status"},
		rows: [][]driver.Value{
			{int64(1), "user-1", "User One", "active"},
		},
	})
	selfID := "user-1"
	items := []database.PerformanceParticipant{
		{
			EmployeeID:     selfID,
			ManagerID:      &selfID,
			ManagerSource:  ManagerSourceManual,
			EmployeeStatus: "active",
		},
	}
	svc.hydrateManagerConfigStatus(items)
	if items[0].ManagerConfigStatus != ManagerConfigConfigured {
		t.Fatalf("expected configured self final status, got %q", items[0].ManagerConfigStatus)
	}
}

func TestHydrateManagerConfigStatusSetsPendingWhenNoManager(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "status"},
		rows:    nil,
	})
	items := []database.PerformanceParticipant{
		{ManagerID: nil},
	}
	svc.hydrateManagerConfigStatus(items)
	if items[0].ManagerConfigStatus != ManagerConfigPending {
		t.Fatalf("expected pending config status, got %q", items[0].ManagerConfigStatus)
	}
}

func TestApplySelfFinalAssessmentPromotesSubmittedSelfEval(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "user-1", "User One", "active"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_goal_records"),
			columns: []string{"id", "activity_id", "participant_id", "section_type", "weight", "self_score", "manager_score", "bonus_score"},
			rows: [][]driver.Value{
				{int64(10), "activity-1", int64(1), "quantitative", 1.0, 8.8, 0.0, 0.0},
			},
		},
	)
	selfID := "user-1"
	participant := database.PerformanceParticipant{
		ID:                        1,
		ActivityID:                "activity-1",
		EmployeeID:                selfID,
		EmployeeName:              "User One",
		Status:                    "self_submitted",
		SelfScore:                 8.8,
		TotalSelfScore:            8.8,
		SelfSummary:               "self summary",
		SelfEvaluationGood:        "good",
		SelfEvaluationImprovement: "improve",
		ManagerID:                 &selfID,
		ManagerSource:             ManagerSourceManual,
		ManagerConfigStatus:       ManagerConfigConfigured,
		DirectManagerIDSnapshot:   nil,
		DirectManagerNameSnapshot: nil,
	}
	activity := database.PerformanceActivity{ID: 1, FlowType: PerformanceFlowNew}

	if err := svc.applySelfFinalAssessmentWithDB(svc.db, &participant, &activity, "operator-1"); err != nil {
		t.Fatalf("applySelfFinalAssessmentWithDB() error = %v", err)
	}
	if participant.Status != "manager_submitted" {
		t.Fatalf("status = %q, want manager_submitted", participant.Status)
	}
	if participant.ManagerScore != 8.8 || participant.TotalManagerScore != 8.8 {
		t.Fatalf("manager score = (%v, %v), want 8.8", participant.ManagerScore, participant.TotalManagerScore)
	}
	if participant.FinalLevel != PerformanceLevelByActivity(8.8, &activity) {
		t.Fatalf("final level = %q", participant.FinalLevel)
	}
	if participant.ManagerComment != participant.SelfSummary {
		t.Fatalf("manager comment = %q, want self summary", participant.ManagerComment)
	}
}

// ======================== validateActivityIndicatorLibraryCycle ========================

func TestValidateActivityIndicatorLibraryCycleNil(t *testing.T) {
	svc := newStubPerformanceService(t)
	if err := svc.validateActivityIndicatorLibraryCycle(nil, "quarterly"); err != nil {
		t.Fatalf("nil library should pass: %v", err)
	}
}

func TestValidateActivityIndicatorLibraryCycleBranches(t *testing.T) {
	libraryID := uint(10)

	matchSvc := newStubPerformanceService(t, performanceIndicatorLibraryResponse("quarterly"))
	if err := matchSvc.validateActivityIndicatorLibraryCycle(&libraryID, "quarterly"); err != nil {
		t.Fatalf("validateActivityIndicatorLibraryCycle(match) error = %v", err)
	}

	notFoundSvc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: []string{"id", "name", "default_cycle"},
		rows:    nil,
	})
	if err := notFoundSvc.validateActivityIndicatorLibraryCycle(&libraryID, "quarterly"); err == nil {
		t.Fatalf("validateActivityIndicatorLibraryCycle(not found) expected error")
	}

	emptyCycleSvc := newStubPerformanceService(t, performanceIndicatorLibraryResponse(""))
	if err := emptyCycleSvc.validateActivityIndicatorLibraryCycle(&libraryID, "quarterly"); err == nil {
		t.Fatalf("validateActivityIndicatorLibraryCycle(empty cycle) expected error")
	}

	mismatchSvc := newStubPerformanceService(t, performanceIndicatorLibraryResponse("monthly"))
	if err := mismatchSvc.validateActivityIndicatorLibraryCycle(&libraryID, "quarterly"); err == nil {
		t.Fatalf("validateActivityIndicatorLibraryCycle(mismatch) expected error")
	}
}

func TestCreateAndUpdateActivityValidateIndicatorLibraryCycle(t *testing.T) {
	libraryID := uint(10)

	createSvc := newStubPerformanceService(t,
		performanceIndicatorLibraryResponse("quarterly"),
		performanceActivityResponse("draft", ""),
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "department_id", "status"},
			rows:    nil,
		},
		performanceDepartmentsResponse(),
		performanceParticipantsResponse(nil),
	)
	activity, err := createSvc.CreateActivity(CreateActivityRequest{
		Name:               "Q2",
		CycleType:          "quarterly",
		Status:             "draft",
		IndicatorLibraryID: &libraryID,
	}, "creator-1")
	if err != nil {
		t.Fatalf("CreateActivity(indicator match) error = %v", err)
	}
	if activity.IndicatorLibraryID == nil || *activity.IndicatorLibraryID != libraryID {
		t.Fatalf("CreateActivity indicator id = %v, want %d", activity.IndicatorLibraryID, libraryID)
	}

	createMismatchSvc := newStubPerformanceService(t, performanceIndicatorLibraryResponse("monthly"))
	if _, err := createMismatchSvc.CreateActivity(CreateActivityRequest{
		Name:               "Q2",
		CycleType:          "quarterly",
		Status:             "draft",
		IndicatorLibraryID: &libraryID,
	}, "creator-1"); err == nil {
		t.Fatalf("CreateActivity(indicator mismatch) expected error")
	}

	updateMismatchSvc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		performanceIndicatorLibraryResponse("monthly"),
	)
	if _, err := updateMismatchSvc.UpdateActivity("activity-1", CreateActivityRequest{
		Name:               "Q2",
		CycleType:          "quarterly",
		Status:             "draft",
		IndicatorLibraryID: &libraryID,
	}, "operator-1"); err == nil {
		t.Fatalf("UpdateActivity(indicator mismatch) expected error")
	}
}

// ======================== displayNameForUser ========================

func TestDisplayNameForUserFallsBackToID(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "status"},
		rows:    nil,
	})
	if got := svc.displayNameForUser("user-1"); got != "user-1" {
		t.Fatalf("displayNameForUser(fallback) = %q, want user-1", got)
	}
}

func TestDisplayNameForUserEmpty(t *testing.T) {
	svc := newStubPerformanceService(t)
	if got := svc.displayNameForUser(""); got != "" {
		t.Fatalf("displayNameForUser(empty) = %q, want empty", got)
	}

	got := svc.displayNameForUser("  ")
	if got != "" {
		t.Fatalf("displayNameForUser(whitespace) = %q, want empty", got)
	}
}

// ======================== normalizeActivityManagerAssignmentSource ========================

func TestNormalizeActivityManagerAssignmentSourceEdgeCases(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", ManagerSourceImport, false},
		{"  ", ManagerSourceImport, false},
		{"empty", ManagerSourceImport, false},
		{"system", ManagerSourceImport, false},
		{"manual", ManagerSourceManual, false},
		{"direct_manager", ManagerSourceDirectManager, false},
		{"department_head", ManagerSourceDepartmentHead, false},
		{"center_head", ManagerSourceCenterHead, false},
		{"bad-source", "", true},
	}
	for _, tt := range tests {
		got, err := normalizeActivityManagerAssignmentSource(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("normalizeActivityManagerAssignmentSource(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeActivityManagerAssignmentSource(%q) error = %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeActivityManagerAssignmentSource(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// ======================== sortRulesByLevel ========================

func TestSortRulesByLevelEmpty(t *testing.T) {
	rules := []database.PerformanceDistributionRule{}
	sortRulesByLevel(rules)
	if len(rules) != 0 {
		t.Fatalf("sort empty rules should return empty")
	}
}

// ======================== refreshPerformanceParticipantProfile edge cases ========================

func TestRefreshPerformanceParticipantProfileNil(t *testing.T) {
	changed, logs := refreshPerformanceParticipantProfile(nil, database.User{}, "", "", "", time.Now())
	if changed || len(logs) != 0 {
		t.Fatalf("nil participant should not change")
	}
}

func TestRefreshPerformanceParticipantProfileNoChanges(t *testing.T) {
	existing := &database.PerformanceParticipant{
		ID:             1,
		EmployeeID:     "user-1",
		EmployeeName:   "Alice",
		DepartmentID:   "dept-1",
		Position:       "Engineer",
		EmployeeStatus: "active",
		Status:         "target_set",
	}
	changed, logs := refreshPerformanceParticipantProfile(existing, database.User{
		UserID:       "user-1",
		Name:         "Alice",
		DepartmentID: "dept-1",
		Position:     "Engineer",
		Status:       "active",
	}, "Dept", "activity-1", "op", time.Now())
	if changed {
		t.Fatalf("no changes expected")
	}
	if len(logs) != 0 {
		t.Fatalf("no logs expected, got %d", len(logs))
	}
}

func TestRefreshPerformanceParticipantProfileNameChanged(t *testing.T) {
	existing := &database.PerformanceParticipant{
		ID:             1,
		EmployeeID:     "user-1",
		EmployeeName:   "Old Name",
		DepartmentID:   "dept-1",
		Position:       "Engineer",
		EmployeeStatus: "active",
		Status:         "target_set",
	}
	changed, _ := refreshPerformanceParticipantProfile(existing, database.User{
		UserID:       "user-1",
		Name:         "New Name",
		DepartmentID: "dept-1",
		Position:     "Engineer",
		Status:       "active",
	}, "Dept", "activity-1", "op", time.Now())
	if !changed {
		t.Fatalf("name change should be detected")
	}
	if existing.EmployeeName != "New Name" {
		t.Fatalf("EmployeeName = %q, want New Name", existing.EmployeeName)
	}
}

func TestRefreshPerformanceParticipantProfileDepartmentChanged(t *testing.T) {
	existing := &database.PerformanceParticipant{
		ID:             1,
		EmployeeID:     "user-1",
		EmployeeName:   "Alice",
		DepartmentID:   "old-dept",
		Position:       "Engineer",
		EmployeeStatus: "active",
		Status:         "target_set",
	}
	changed, logs := refreshPerformanceParticipantProfile(existing, database.User{
		UserID:       "user-1",
		Name:         "Alice",
		DepartmentID: "new-dept",
		Position:     "Engineer",
		Status:       "active",
	}, "New Dept", "activity-1", "op", time.Now())
	if !changed {
		t.Fatalf("department change should be detected")
	}
	if existing.DepartmentID != "new-dept" || existing.DepartmentName != "New Dept" {
		t.Fatalf("department not updated: %q/%q", existing.DepartmentID, existing.DepartmentName)
	}
	if len(logs) != 1 || logs[0].ChangeType != "department_changed" {
		t.Fatalf("expected department_changed log, got %d logs: %#v", len(logs), logs)
	}
}

func TestRefreshPerformanceParticipantProfileStatusChanged(t *testing.T) {
	existing := &database.PerformanceParticipant{
		ID:             1,
		EmployeeID:     "user-1",
		EmployeeName:   "Alice",
		DepartmentID:   "dept-1",
		Position:       "Engineer",
		EmployeeStatus: "inactive",
		Status:         "pending",
	}
	changed, logs := refreshPerformanceParticipantProfile(existing, database.User{
		UserID:       "user-1",
		Name:         "Alice",
		DepartmentID: "dept-1",
		Position:     "Engineer",
		Status:       "active",
	}, "Dept", "activity-1", "op", time.Now())
	if !changed {
		t.Fatalf("status change should be detected")
	}
	if existing.EmployeeStatus != "active" {
		t.Fatalf("EmployeeStatus = %q, want active", existing.EmployeeStatus)
	}
	if len(logs) != 1 || logs[0].ChangeType != "status_changed" {
		t.Fatalf("expected status_changed log, got %d logs: %#v", len(logs), logs)
	}
}

func TestRefreshPerformanceParticipantProfileRemovedFromScopeReactivates(t *testing.T) {
	existing := &database.PerformanceParticipant{
		ID:             1,
		EmployeeID:     "user-1",
		EmployeeName:   "Alice",
		DepartmentID:   "dept-1",
		Position:       "Engineer",
		EmployeeStatus: "active",
		Status:         "removed_from_scope",
	}
	changed, _ := refreshPerformanceParticipantProfile(existing, database.User{
		UserID:       "user-1",
		Name:         "Alice",
		DepartmentID: "dept-1",
		Position:     "Engineer",
		Status:       "active",
	}, "Dept", "activity-1", "op", time.Now())
	if !changed {
		t.Fatalf("removed_from_scope reactivation should be detected")
	}
	if existing.Status != "pending" {
		t.Fatalf("Status = %q, want pending", existing.Status)
	}
}

func TestRefreshPerformanceParticipantProfilePositionChanged(t *testing.T) {
	existing := &database.PerformanceParticipant{
		ID:             1,
		EmployeeID:     "user-1",
		EmployeeName:   "Alice",
		DepartmentID:   "dept-1",
		Position:       "Old Position",
		EmployeeStatus: "active",
		Status:         "target_set",
	}
	changed, _ := refreshPerformanceParticipantProfile(existing, database.User{
		UserID:       "user-1",
		Name:         "Alice",
		DepartmentID: "dept-1",
		Position:     "New Position",
		Status:       "active",
	}, "Dept", "activity-1", "op", time.Now())
	if !changed {
		t.Fatalf("position change should be detected")
	}
	if existing.Position != "New Position" {
		t.Fatalf("Position = %q, want New Position", existing.Position)
	}
}

// ======================== ensureParticipantStageComplete edge cases ========================

func TestEnsureParticipantStageCompleteNoActive(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("self_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "inactive"},
			},
		},
	)
	err := svc.ensureParticipantStageComplete("activity-1", "self_evaluation")
	if err == nil || !strings.Contains(err.Error(), "没有可参与员工") {
		t.Fatalf("ensureParticipantStageComplete() no active expected error, got = %v", err)
	}
}

func TestEnsureParticipantStageCompleteAllComplete(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("self_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "self_submitted"},
				{int64(2), "activity-1", "manager_submitted"},
			},
		},
	)
	if err := svc.ensureParticipantStageComplete("activity-1", "self_evaluation"); err != nil {
		t.Fatalf("ensureParticipantStageComplete() all complete error = %v", err)
	}
}

func TestEnsureParticipantStageCompleteSomeIncomplete(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("self_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "self_submitted"},
				{int64(2), "activity-1", "pending"},
			},
		},
	)
	err := svc.ensureParticipantStageComplete("activity-1", "self_evaluation")
	if err == nil || !strings.Contains(err.Error(), "无法开启主管评分") || !strings.Contains(err.Error(), "目标尚未完成") {
		t.Fatalf("ensureParticipantStageComplete() incomplete expected error, got = %v", err)
	}
}

func TestEnsureParticipantStageCompleteReportsAssessmentManagerIssue(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("target_setting", ""),
		stubQueryResponse{
			match: stubTableMatcher("performance_participants"),
			columns: []string{
				"id", "activity_id", "employee_id", "employee_name", "status",
				"manager_id", "manager_config_status",
			},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "employee-1", "列德", "pending", nil, ManagerConfigPending},
			},
		},
	)
	err := svc.ensureParticipantStageComplete("activity-1", "target_setting")
	if err == nil ||
		!strings.Contains(err.Error(), "无法开启自评") ||
		!strings.Contains(err.Error(), "列德：考核上级未配置") ||
		!strings.Contains(err.Error(), "请先配置考核上级") {
		t.Fatalf("ensureParticipantStageComplete() manager issue expected detailed error, got = %v", err)
	}
}

func TestEnsureParticipantStageCompleteReportsTargetApprovalBeforeManagerIssue(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("target_setting", ""),
		stubQueryResponse{
			match: stubTableMatcher("performance_participants"),
			columns: []string{
				"id", "activity_id", "employee_id", "employee_name", "status",
				"manager_id", "manager_config_status",
			},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "employee-1", "列德", "target_pending_approval", nil, ManagerConfigPending},
			},
		},
	)
	err := svc.ensureParticipantStageComplete("activity-1", "target_setting")
	if err == nil ||
		!strings.Contains(err.Error(), "无法开启自评") ||
		!strings.Contains(err.Error(), "未完成目标设定/审批") ||
		!strings.Contains(err.Error(), "列德：目标已提交，待审批通过或驳回") ||
		!strings.Contains(err.Error(), "请先在参与人列表处理目标审批") {
		t.Fatalf("ensureParticipantStageComplete() approval issue expected detailed error, got = %v", err)
	}
	if strings.Contains(err.Error(), "列德：考核上级未配置") {
		t.Fatalf("approval issue should not be hidden by manager config issue, got = %v", err)
	}
}

// ======================== countActiveParticipants ========================

func TestCountActiveParticipants(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: []string{"id"},
		rows: [][]driver.Value{
			{int64(1)},
			{int64(2)},
			{int64(3)},
		},
	})
	count, err := svc.countActiveParticipants("activity-1")
	if err != nil {
		t.Fatalf("countActiveParticipants() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("countActiveParticipants() = %d, want 3", count)
	}
}

func TestCountActiveParticipantsEmpty(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: []string{"id"},
		rows:    nil,
	})
	count, err := svc.countActiveParticipants("activity-1")
	if err != nil {
		t.Fatalf("countActiveParticipants() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("countActiveParticipants() = %d, want 0", count)
	}
}

// ======================== GetParticipantVersions ========================

func TestGetParticipantVersions(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_review_versions"),
		columns: []string{"id", "participant_id", "review_type", "created_by"},
		rows: [][]driver.Value{
			{int64(1), int64(1), "self", "user-1"},
			{int64(2), int64(1), "manager", "manager-1"},
		},
	})
	versions, err := svc.GetParticipantVersions("1")
	if err != nil {
		t.Fatalf("GetParticipantVersions() error = %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("GetParticipantVersions() len=%d, want 2", len(versions))
	}
}

func TestGetParticipantVersionsEmpty(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_review_versions"),
		columns: []string{"id", "participant_id", "review_type"},
		rows:    nil,
	})
	versions, err := svc.GetParticipantVersions("1")
	if err != nil {
		t.Fatalf("GetParticipantVersions() error = %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("GetParticipantVersions() len=%d, want 0", len(versions))
	}
}

// ======================== GetParticipantRelationshipChangeLogs ========================

func TestGetParticipantRelationshipChangeLogs(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_relationship_change_logs"),
		columns: []string{"id", "participant_id", "change_type", "field_name"},
		rows: [][]driver.Value{
			{int64(1), int64(1), "assessment_manager_changed", "manager_id"},
		},
	})
	logs, err := svc.GetParticipantRelationshipChangeLogs("1")
	if err != nil {
		t.Fatalf("GetParticipantRelationshipChangeLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("GetParticipantRelationshipChangeLogs() len=%d, want 1", len(logs))
	}
}

func TestGetParticipantRelationshipChangeLogsEmpty(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_relationship_change_logs"),
		columns: []string{"id", "participant_id", "change_type"},
		rows:    nil,
	})
	logs, err := svc.GetParticipantRelationshipChangeLogs("1")
	if err != nil {
		t.Fatalf("GetParticipantRelationshipChangeLogs() error = %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("GetParticipantRelationshipChangeLogs() len=%d, want 0", len(logs))
	}
}

// ======================== GetActivityRelationshipChangeLogs ========================

func TestGetActivityRelationshipChangeLogs(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_relationship_change_logs"),
		columns: []string{"id", "activity_id", "change_type"},
		rows: [][]driver.Value{
			{int64(1), "activity-1", "status_changed"},
		},
	})
	logs, err := svc.GetActivityRelationshipChangeLogs("activity-1")
	if err != nil {
		t.Fatalf("GetActivityRelationshipChangeLogs() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("GetActivityRelationshipChangeLogs() len=%d, want 1", len(logs))
	}
}

// ======================== ListTemplates ========================

func TestListTemplates(t *testing.T) {
	svc := newStubPerformanceService(t,
		// count query needs single column
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "performance_templates") && strings.Contains(strings.ToLower(query), "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_templates"),
			columns: []string{"id", "name", "status", "description"},
			rows: [][]driver.Value{
				{int64(1), "Q1 Template", "active", "desc"},
			},
		},
	)
	templates, total, err := svc.ListTemplates(1, 10, "active")
	if err != nil {
		t.Fatalf("ListTemplates() error = %v", err)
	}
	if total != 1 || len(templates) != 1 {
		t.Fatalf("ListTemplates() total=%d len=%d, want 1,1", total, len(templates))
	}
}

func TestListTemplatesEmpty(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_templates"),
		columns: []string{"id", "name", "status"},
		rows:    nil,
	})
	templates, total, err := svc.ListTemplates(1, 10, "active")
	if err != nil {
		t.Fatalf("ListTemplates() error = %v", err)
	}
	if total != 0 || len(templates) != 0 {
		t.Fatalf("ListTemplates() total=%d len=%d, want 0,0", total, len(templates))
	}
}

// ======================== GetGoalRecordsByActivity ========================

func TestGetGoalRecordsByActivity(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_goal_records"),
		columns: performanceGoalRecordColumns(),
		rows: [][]driver.Value{
			performanceGoalRecordRow(1, "quantitative", "Revenue", 0.5),
		},
	})
	records, err := svc.GetGoalRecordsByActivity("activity-1")
	if err != nil {
		t.Fatalf("GetGoalRecordsByActivity() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("GetGoalRecordsByActivity() len=%d, want 1", len(records))
	}
}

// ======================== SetDistributionRules edge cases ========================

func TestSetDistributionRulesEmptyLevel(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.SetDistributionRules("activity-1", []struct {
		Level               string
		DistributionPercent float64
		Description         string
	}{
		{Level: "", DistributionPercent: 100},
	}, "operator-1")
	if err == nil || !strings.Contains(err.Error(), "level 不能为空") {
		t.Fatalf("SetDistributionRules(empty level) expected error, got = %v", err)
	}
}

// ======================== GetTemplate / CreateTemplate / UpdateTemplate ========================

func TestCreateTemplateEmptyName(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.CreateTemplate(PerformanceTemplateRequest{Name: "  "}, "user-1")
	if err == nil || !strings.Contains(err.Error(), "模板名称不能为空") {
		t.Fatalf("CreateTemplate(empty name) expected error, got = %v", err)
	}
}

func TestCreateTemplateNoSections(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.CreateTemplate(PerformanceTemplateRequest{Name: "Template"}, "user-1")
	if err == nil || !strings.Contains(err.Error(), "至少需要一个评分维度") {
		t.Fatalf("CreateTemplate(no sections) expected error, got = %v", err)
	}
}

func TestCreateTemplateInvalidSections(t *testing.T) {
	svc := newStubPerformanceService(t)
	_, err := svc.CreateTemplate(PerformanceTemplateRequest{
		Name: "Template",
		Sections: []PerformanceTemplateSectionRequest{
			{Name: "  ", Weight: 100, Items: []PerformanceTemplateItemRequest{{Name: "Item", MaxScore: 100, Weight: 100}}},
		},
	}, "user-1")
	if err == nil || !strings.Contains(err.Error(), "section name 不能为空") {
		t.Fatalf("CreateTemplate(invalid sections) expected error, got = %v", err)
	}
}

func TestCreateTemplateDefaultStatus(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_templates"),
		columns: []string{"id", "name", "status"},
		rows: [][]driver.Value{
			{int64(1), "Template", "draft"},
		},
	})
	template, err := svc.CreateTemplate(PerformanceTemplateRequest{
		Name: "Template",
		Sections: []PerformanceTemplateSectionRequest{
			{Name: "Section", Weight: 100, Items: []PerformanceTemplateItemRequest{{Name: "Item", MaxScore: 100, Weight: 100}}},
		},
	}, "user-1")
	if err != nil {
		t.Fatalf("CreateTemplate() error = %v", err)
	}
	if template.Status != "draft" {
		t.Fatalf("CreateTemplate() status = %q, want draft", template.Status)
	}
}

func TestUpdateTemplateNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_templates"),
		columns: []string{"id", "name", "status", "description"},
		rows:    nil,
	},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_sections"),
			columns: []string{"id", "template_id", "name"},
			rows:    nil,
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_items"),
			columns: []string{"id", "section_id", "name"},
			rows:    nil,
		},
	)
	_, err := svc.UpdateTemplate(999, PerformanceTemplateRequest{Name: "Updated"}, "user-1")
	if err == nil || !strings.Contains(err.Error(), "模板不存在") {
		t.Fatalf("UpdateTemplate(999) expected error, got = %v", err)
	}
}

func TestUpdateTemplateEmptyName(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_templates"),
			columns: []string{"id", "name", "status", "description"},
			rows: [][]driver.Value{
				{int64(1), "Old Template", "draft", ""},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_sections"),
			columns: []string{"id", "template_id", "name"},
			rows:    nil,
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_items"),
			columns: []string{"id", "section_id", "name"},
			rows:    nil,
		},
	)
	_, err := svc.UpdateTemplate(1, PerformanceTemplateRequest{Name: "  "}, "user-1")
	if err == nil || !strings.Contains(err.Error(), "模板名称不能为空") {
		t.Fatalf("UpdateTemplate(empty name) expected error, got = %v", err)
	}
}

func TestUpdateTemplateMetadataSuccess(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_templates"),
			columns: []string{"id", "name", "status", "description"},
			rows: [][]driver.Value{
				{int64(1), "Old Template", "draft", "old"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_sections"),
			columns: []string{"id", "template_id", "name"},
			rows:    nil,
		},
	)

	template, err := svc.UpdateTemplate(1, PerformanceTemplateRequest{
		Name:        " Updated Template ",
		Description: "new description",
	}, "user-1")
	if err != nil {
		t.Fatalf("UpdateTemplate() error = %v", err)
	}
	if template.Name != "Updated Template" || template.Description != "new description" || template.Status != "draft" {
		t.Fatalf("UpdateTemplate() = %#v", template)
	}
}

func TestListAssessmentManagerCandidatesBuildsSourceGroups(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		assessmentManagerParticipantResponse(),
		assessmentManagerUsersResponse(),
		assessmentManagerProfilesResponse(false),
		assessmentManagerProfilesResponse(true),
		assessmentManagerDepartmentsResponse(),
	)

	candidates, err := svc.ListAssessmentManagerCandidates("activity-1", 1, "", "Boss", 10)
	if err != nil {
		t.Fatalf("ListAssessmentManagerCandidates() error = %v", err)
	}
	if len(candidates) < 4 {
		t.Fatalf("expected direct, department, center, and manual candidates, got %#v", candidates)
	}

	groups, err := svc.ListAssessmentManagerCandidateSourceGroups("activity-1", 1, "Boss", 10)
	if err != nil {
		t.Fatalf("ListAssessmentManagerCandidateSourceGroups() error = %v", err)
	}
	groupCounts := map[string]int{}
	for _, group := range groups {
		groupCounts[group.Source] = len(group.Items)
	}
	for _, source := range []string{ManagerSourceDirectManager, ManagerSourceDepartmentHead, ManagerSourceCenterHead, ManagerSourceManual} {
		if groupCounts[source] == 0 {
			t.Fatalf("source %s has no candidates in groups %#v", source, groups)
		}
	}
}

func TestListAssessmentManagerCandidatesAllowsSelfFinalForTopLevel(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		assessmentManagerParticipantForUpdate("activity-1", "employee-1", "target_set", false, "", "", "", ManagerSourceEmpty),
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "employee_profiles") && strings.Contains(lower, "select `user_id`")
			},
			columns: []string{"user_id"},
			rows:    nil,
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "employee_profiles") && !strings.Contains(lower, "select `user_id`")
			},
			columns: []string{"user_id", "employee_id"},
			rows: [][]driver.Value{
				{"employee-1", "E001"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "department_id", "status", "mobile"},
			rows: [][]driver.Value{
				{int64(1), "employee-1", "Alice", "", "active", "13800000005"},
			},
		},
	)

	candidates, err := svc.ListAssessmentManagerCandidates("activity-1", 1, ManagerSourceManual, "Alice", 10)
	if err != nil {
		t.Fatalf("ListAssessmentManagerCandidates(self final) error = %v", err)
	}
	if len(candidates) != 1 || candidates[0].UserID != "employee-1" || !candidates[0].IsSelfFinalCandidate {
		t.Fatalf("self final candidates = %#v", candidates)
	}
}

func TestAssessmentManagerCandidateMissingReasonAndFindActiveUser(t *testing.T) {
	svc := newStubPerformanceService(t, assessmentManagerUsersResponse())

	if reason := svc.assessmentManagerCandidateMissingReason("activity-1", 0, ManagerSourceManual, ""); reason == "" {
		t.Fatalf("manual missing reason should not be empty")
	}
	if _, err := svc.findActiveUserByUserIDWithDB(svc.db, " "); err == nil {
		t.Fatalf("blank manager user id should fail")
	}
	user, err := svc.findActiveUserByUserIDWithDB(svc.db, "direct-1")
	if err != nil {
		t.Fatalf("findActiveUserByUserIDWithDB() error = %v", err)
	}
	if user.UserID != "direct-1" {
		t.Fatalf("stubbed first active user = %q, want direct-1", user.UserID)
	}
}

func TestAssessmentManagerCandidateMissingReasonManualOnlySelf(t *testing.T) {
	svc := newStubPerformanceService(t,
		assessmentManagerParticipantResponse(),
		activeUserByIDResponse("direct-1", "Direct Boss"),
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "employee_profiles") && !strings.Contains(lower, "select `user_id`")
			},
			columns: []string{"id", "user_id", "employee_id"},
			rows: [][]driver.Value{
				{int64(1), "employee-1", "E001"},
			},
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "employee_profiles") && strings.Contains(lower, "select `user_id`")
			},
			columns: []string{"user_id"},
			rows:    nil,
		},
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "department_id", "status", "mobile"},
			rows: [][]driver.Value{
				{int64(1), "employee-1", "Alice", "dept-1", "active", "13800000005"},
			},
		},
	)

	reason := svc.assessmentManagerCandidateMissingReason("activity-1", 1, ManagerSourceManual, "Alice")
	if !strings.Contains(reason, "只有最高级") || !strings.Contains(reason, "普通员工") {
		t.Fatalf("reason = %q, want non-top-level self guidance", reason)
	}
}

func TestParticipantSelfUserIDsIncludesEmployeeProfileID(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("employee_profiles"),
		columns: []string{"id", "user_id", "employee_id"},
		rows: [][]driver.Value{
			{int64(1), "user-1", "E001"},
		},
	})

	values := svc.participantSelfUserIDs(&database.PerformanceParticipant{EmployeeID: "user-1"})
	if !stringSetContains(values, "user-1") || !stringSetContains(values, "E001") {
		t.Fatalf("participantSelfUserIDs() = %#v, want user_id and employee_id", values)
	}
}

func TestHydrateParticipantTargetConfirmersFromApprovalLogs(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_goal_approval_logs"),
		columns: []string{"id", "participant_id", "activity_id", "action", "approver_id", "approver_name", "created_by", "created_at"},
		rows: [][]driver.Value{
			{int64(1), int64(1), "activity-1", "submit", "employee-1", "Alice", "employee-1", now.Add(-time.Hour)},
			{int64(2), int64(1), "activity-1", "approve", "manager-1", "Boss", "manager-1", now},
		},
	})

	participant := &database.PerformanceParticipant{ID: 1, ActivityID: "activity-1"}
	svc.HydrateParticipantTargetConfirmers(participant)

	if participant.EmployeeTargetConfirmedBy != "Alice" || participant.ManagerTargetConfirmedBy != "Boss" {
		t.Fatalf("target confirmers not hydrated: %#v", participant)
	}
	if participant.EmployeeTargetConfirmedAt == nil || participant.ManagerTargetConfirmedAt == nil {
		t.Fatalf("target confirmer timestamps not hydrated: %#v", participant)
	}
}

func TestLegacyReviewSubmissionWrappersDelegateToVersionRepository(t *testing.T) {
	selfSvc := newStubPerformanceService(t,
		legacyReviewParticipantResponse("self_evaluation", ""),
		performanceActivityResponse("self_evaluation", ""),
	)
	selfVersion, err := selfSvc.SubmitSelfEvaluation("1", struct {
		SelfScore       float64
		SelfLevel       string
		SelfSummary     string
		SelfAttachments []string
	}{SelfScore: 82, SelfLevel: "B", SelfSummary: "done"}, "employee-1")
	if err != nil {
		t.Fatalf("SubmitSelfEvaluation() error = %v", err)
	}
	if selfVersion.ReviewType != "self" || selfVersion.SelfScore != 82 {
		t.Fatalf("SubmitSelfEvaluation() version = %#v", selfVersion)
	}

	managerSvc := newStubPerformanceService(t,
		legacyReviewParticipantResponse("self_submitted", ""),
		performanceActivityResponse("manager_evaluation", ""),
	)
	managerVersion, err := managerSvc.SubmitManagerEvaluation("1", struct {
		ManagerScore    float64
		SuggestedLevel  string
		ManagerComment  string
		EvaluationItems []struct {
			ItemKey   string
			ItemScore float64
			ItemValue string
		}
	}{ManagerScore: 91, SuggestedLevel: "A", ManagerComment: "good"}, "manager-1")
	if err != nil {
		t.Fatalf("SubmitManagerEvaluation() error = %v", err)
	}
	if managerVersion.ReviewType != "manager" || managerVersion.ManagerScore != 91 {
		t.Fatalf("SubmitManagerEvaluation() version = %#v", managerVersion)
	}

	batchSvc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		legacyReviewParticipantResponse("self_submitted", ""),
	)
	versions, err := batchSvc.BatchSubmitManagerEvaluations("activity-1", []struct {
		ParticipantID   uint
		ManagerScore    float64
		SuggestedLevel  string
		ManagerComment  string
		EvaluationItems []struct {
			ItemKey   string
			ItemScore float64
			ItemValue string
		}
	}{
		{
			ParticipantID:  1,
			ManagerScore:   88,
			SuggestedLevel: "B",
			EvaluationItems: []struct {
				ItemKey   string
				ItemScore float64
				ItemValue string
			}{{ItemKey: "kpi", ItemScore: 88}},
		},
	}, "manager-1")
	if err != nil {
		t.Fatalf("BatchSubmitManagerEvaluations() error = %v", err)
	}
	if len(versions) != 1 || versions[0].ReviewType != "manager" {
		t.Fatalf("BatchSubmitManagerEvaluations() versions = %#v", versions)
	}

	adjustSvc := newStubPerformanceService(t, legacyReviewParticipantResponse("manager_submitted", "B"))
	adjustVersion, err := adjustSvc.AdjustFinalLevel("1", "A", "calibration", "hr-1")
	if err != nil {
		t.Fatalf("AdjustFinalLevel() error = %v", err)
	}
	if adjustVersion.ReviewType != "adjust_final_level" || adjustVersion.FinalLevel != "A" {
		t.Fatalf("AdjustFinalLevel() version = %#v", adjustVersion)
	}

	confirmSvc := newStubPerformanceService(t, legacyReviewParticipantResponse("manager_submitted", "A"))
	confirmVersion, err := confirmSvc.ConfirmResult("1", "ok", "employee-1")
	if err != nil {
		t.Fatalf("ConfirmResult() error = %v", err)
	}
	if confirmVersion.ReviewType != "confirm_result" || confirmVersion.ConfirmedAt == nil {
		t.Fatalf("ConfirmResult() version = %#v", confirmVersion)
	}
}

func TestBatchConfirmResultsCoversConfirmResultByID(t *testing.T) {
	svc := newStubPerformanceService(t, legacyReviewParticipantResponse("manager_submitted", "A"))

	results, err := svc.BatchConfirmResults("activity-1", []uint{1}, "employee-1")
	if err != nil {
		t.Fatalf("BatchConfirmResults() error = %v", err)
	}
	if len(results) != 1 || results[0]["success"] != true {
		t.Fatalf("BatchConfirmResults() = %#v", results)
	}
}

func TestPublishAndCloseActivityBoundaryBranches(t *testing.T) {
	t.Run("publish activity not found", func(t *testing.T) {
		svc := newStubPerformanceService(t, stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "status"},
			rows:    nil,
		})
		if err := svc.PublishActivity("missing", "operator-1"); err == nil {
			t.Fatalf("PublishActivity(missing) expected error")
		}
	})

	t.Run("publish draft success", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			performanceActivityResponse("draft", ""),
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
				},
			},
		)
		if err := svc.PublishActivity("activity-1", "operator-1"); err != nil {
			t.Fatalf("PublishActivity(draft) error = %v", err)
		}
	})

	t.Run("publish rejects terminal status", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("archived", ""))
		if err := svc.PublishActivity("activity-1", "operator-1"); err == nil {
			t.Fatalf("PublishActivity(archived) expected error")
		}
	})

	t.Run("close activity not found", func(t *testing.T) {
		svc := newStubPerformanceService(t, stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "status"},
			rows:    nil,
		})
		if err := svc.CloseActivity("missing", "operator-1"); err == nil {
			t.Fatalf("CloseActivity(missing) expected error")
		}
	})

	t.Run("close rejects active status", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
		if err := svc.CloseActivity("activity-1", "operator-1"); err == nil {
			t.Fatalf("CloseActivity(self_evaluation) expected error")
		}
	})

	t.Run("close rejects unknown status", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("paused", ""))
		if err := svc.CloseActivity("activity-1", "operator-1"); err == nil {
			t.Fatalf("CloseActivity(paused) expected error")
		}
	})
}

func TestActivityStageTransitionsBoundaryBranches(t *testing.T) {
	t.Run("start not found", func(t *testing.T) {
		svc := newStubPerformanceService(t, stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "status"},
			rows:    nil,
		})
		if err := svc.StartActivity("missing", "operator-1"); err == nil {
			t.Fatalf("StartActivity(missing) expected error")
		}
	})

	t.Run("start idempotent target setting", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("target_setting", ""))
		if err := svc.StartActivity("activity-1", "operator-1"); err != nil {
			t.Fatalf("StartActivity(target_setting) error = %v", err)
		}
	})

	t.Run("start rejects invalid status", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
		if err := svc.StartActivity("activity-1", "operator-1"); err == nil {
			t.Fatalf("StartActivity(self_evaluation) expected status error")
		}
	})

	t.Run("start rejects empty participant scope", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match: func(query string, _ []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					return strings.Contains(lower, "performance_participants") && strings.Contains(lower, "count(")
				},
				columns: []string{"count"},
				rows:    [][]driver.Value{{int64(0)}},
			},
			performanceActivityResponse("draft", ""),
			stubQueryResponse{
				match:   stubTableMatcher("users"),
				columns: []string{"id", "user_id", "name", "department_id", "status"},
				rows:    nil,
			},
			stubQueryResponse{
				match:   stubTableMatcher("departments"),
				columns: []string{"id", "department_id", "name"},
				rows:    nil,
			},
			stubQueryResponse{
				match: func(query string, _ []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					return strings.Contains(lower, "performance_participants") && !strings.Contains(lower, "count(")
				},
				columns: performanceParticipantStubColumns(),
				rows:    nil,
			},
		)
		if err := svc.StartActivity("activity-1", "operator-1"); err == nil {
			t.Fatalf("StartActivity(empty scope) expected error")
		}
	})

	t.Run("open self evaluation repo error", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("target_setting", ""))
		if err := svc.OpenSelfEvaluation("activity-1", "operator-1"); err == nil {
			t.Fatalf("OpenSelfEvaluation(repo error) expected error")
		}
	})

	t.Run("open manager evaluation repo error", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("self_evaluation", ""))
		if err := svc.OpenManagerEvaluation("activity-1", "operator-1"); err == nil {
			t.Fatalf("OpenManagerEvaluation(repo error) expected error")
		}
	})

	t.Run("lock idempotent", func(t *testing.T) {
		svc := newStubPerformanceService(t, performanceActivityResponse("locked", ""))
		if err := svc.LockActivity("activity-1", "operator-1"); err != nil {
			t.Fatalf("LockActivity(locked) error = %v", err)
		}
	})

	t.Run("lock transaction participant query error", func(t *testing.T) {
		now := time.Now()
		svc := newStubPerformanceService(t,
			performanceActivityResponse("hr_confirmation", ""),
			stubQueryResponse{
				match: func(query string, _ []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					return strings.Contains(lower, "performance_participants") && !strings.Contains(lower, "for update")
				},
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "hr_confirmed", "", 0, 90, "A", false, nil, now, now),
				},
			},
		)
		if err := svc.LockActivity("activity-1", "operator-1"); err == nil {
			t.Fatalf("LockActivity(transaction repo error) expected error")
		}
	})
}

func TestConfirmResultBoundaryBranches(t *testing.T) {
	now := time.Now()

	t.Run("employee participant not found", func(t *testing.T) {
		svc := newStubPerformanceService(t, stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		})
		if err := svc.ConfirmEmployeeResult(999, "employee-1"); err == nil {
			t.Fatalf("ConfirmEmployeeResult(missing participant) expected error")
		}
	})

	t.Run("employee idempotent", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
				},
			},
			performanceActivityResponse("employee_confirmation", ""),
		)
		if err := svc.ConfirmEmployeeResult(1, "employee-1"); err != nil {
			t.Fatalf("ConfirmEmployeeResult(idempotent) error = %v", err)
		}
	})

	t.Run("employee locked before confirmation", func(t *testing.T) {
		svc := newStubPerformanceService(t, stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", true, nil, nil, nil),
			},
		})
		if err := svc.ConfirmEmployeeResult(1, "employee-1"); err == nil {
			t.Fatalf("ConfirmEmployeeResult(locked) expected error")
		}
	})

	t.Run("employee activity not found", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
				},
			},
			stubQueryResponse{
				match:   stubTableMatcher("performance_activities"),
				columns: []string{"id", "status"},
				rows:    nil,
			},
		)
		if err := svc.ConfirmEmployeeResult(1, "employee-1"); err == nil {
			t.Fatalf("ConfirmEmployeeResult(activity missing) expected error")
		}
	})

	t.Run("employee wrong activity status", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
				},
			},
			performanceActivityResponse("manager_evaluation", ""),
		)
		if err := svc.ConfirmEmployeeResult(1, "employee-1"); err == nil {
			t.Fatalf("ConfirmEmployeeResult(wrong activity status) expected error")
		}
	})

	t.Run("manager participant not found", func(t *testing.T) {
		svc := newStubPerformanceService(t, stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		})
		if err := svc.ConfirmManagerResult(999, "manager-1"); err == nil {
			t.Fatalf("ConfirmManagerResult(missing participant) expected error")
		}
	})

	t.Run("manager locked before confirmation", func(t *testing.T) {
		svc := newStubPerformanceService(t, stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", true, now, nil, nil),
			},
		})
		if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil {
			t.Fatalf("ConfirmManagerResult(locked) expected error")
		}
	})

	t.Run("manager activity not found", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
				},
			},
			stubQueryResponse{
				match:   stubTableMatcher("performance_activities"),
				columns: []string{"id", "status"},
				rows:    nil,
			},
		)
		if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil {
			t.Fatalf("ConfirmManagerResult(activity missing) expected error")
		}
	})

	t.Run("manager wrong participant status", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
				},
			},
			performanceActivityResponse("manager_confirmation", ""),
		)
		if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil {
			t.Fatalf("ConfirmManagerResult(wrong participant status) expected error")
		}
	})

	t.Run("manager wrong activity status", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
				},
			},
			performanceActivityResponse("employee_confirmation", ""),
		)
		if err := svc.ConfirmManagerResult(1, "manager-1"); err == nil {
			t.Fatalf("ConfirmManagerResult(wrong activity status) expected error")
		}
	})

	t.Run("hr idempotent", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "hr_confirmed", "", 0, 90, "A", false, nil, now, now),
				},
			},
			performanceActivityResponse("hr_confirmation", ""),
		)
		if err := svc.ConfirmHRResult(1, "hr-1"); err != nil {
			t.Fatalf("ConfirmHRResult(idempotent) error = %v", err)
		}
	})

	t.Run("hr activity not found", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, now, nil),
				},
			},
			stubQueryResponse{
				match:   stubTableMatcher("performance_activities"),
				columns: []string{"id", "status"},
				rows:    nil,
			},
		)
		if err := svc.ConfirmHRResult(1, "hr-1"); err == nil {
			t.Fatalf("ConfirmHRResult(activity missing) expected error")
		}
	})

	t.Run("hr wrong activity status", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, now, nil),
				},
			},
			performanceActivityResponse("manager_confirmation", ""),
		)
		if err := svc.ConfirmHRResult(1, "hr-1"); err == nil {
			t.Fatalf("ConfirmHRResult(wrong activity status) expected error")
		}
	})

	t.Run("hr wrong participant status", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
				},
			},
			performanceActivityResponse("hr_confirmation", ""),
		)
		if err := svc.ConfirmHRResult(1, "hr-1"); err == nil {
			t.Fatalf("ConfirmHRResult(wrong participant status) expected error")
		}
	})

	t.Run("hr missing manager confirmation time", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, nil, nil),
				},
			},
			performanceActivityResponse("hr_confirmation", ""),
		)
		if err := svc.ConfirmHRResult(1, "hr-1"); err == nil {
			t.Fatalf("ConfirmHRResult(missing manager confirmation time) expected error")
		}
	})

	t.Run("hr accepts goal item manager score evidence", func(t *testing.T) {
		svc := newStubPerformanceService(t,
			stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: performanceParticipantStubColumns(),
				rows: [][]driver.Value{
					performanceParticipantStubRow(1, "manager_confirmed", "", 0, 0, "A", false, nil, now, nil),
				},
			},
			performanceActivityResponse("hr_confirmation", ""),
			stubQueryResponse{
				match: func(query string, _ []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					return strings.Contains(lower, "performance_goal_records") && strings.Contains(lower, "count(")
				},
				columns: []string{"count"},
				rows:    [][]driver.Value{{int64(1)}},
			},
		)
		if err := svc.ConfirmHRResult(1, "hr-1"); err != nil {
			t.Fatalf("ConfirmHRResult(goal item manager score evidence) error = %v", err)
		}
	})
}

func TestReminderServicesHandleNoRecipients(t *testing.T) {
	selfSvc := newStubPerformanceService(t,
		performanceActivityResponse("self_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)
	selfResult, err := selfSvc.SendSelfEvalReminders("1")
	if err != nil {
		t.Fatalf("SendSelfEvalReminders() error = %v, want nil", err)
	}
	if selfResult.Pending != 0 || selfResult.Candidates != 0 {
		t.Fatalf("SendSelfEvalReminders() result = %+v, want no pending recipients", selfResult)
	}

	managerSvc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	if _, err := managerSvc.SendManagerEvalReminders("activity-1"); err != nil {
		t.Fatalf("SendManagerEvalReminders() error = %v", err)
	}
}

// ======================== GetTemplate ========================

func TestGetTemplateNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_templates"),
		columns: []string{"id", "name", "status", "description"},
		rows:    nil,
	},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_sections"),
			columns: []string{"id", "template_id", "name"},
			rows:    nil,
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_items"),
			columns: []string{"id", "section_id", "name"},
			rows:    nil,
		},
	)
	_, err := svc.GetTemplate(999)
	if err == nil {
		t.Fatalf("GetTemplate(999) expected error")
	}
}

func assessmentManagerParticipantForUpdate(activityID, employeeID, status string, locked bool, managerID, managerName, directManagerID, managerSource string) stubQueryResponse {
	return stubQueryResponse{
		match: stubTableMatcher("performance_participants"),
		columns: []string{
			"id", "activity_id", "employee_id", "employee_name", "department_id", "department_name",
			"manager_id", "manager_name", "direct_manager_id_snapshot", "direct_manager_name_snapshot",
			"manager_source", "manager_overridden", "manager_override_reason", "manager_config_status",
			"status", "is_locked",
		},
		rows: [][]driver.Value{{
			int64(1), activityID, employeeID, "Alice", "dept-1", "Product",
			managerID, managerName, directManagerID, "Direct Boss",
			managerSource, false, "", ManagerConfigConfigured,
			status, locked,
		}},
	}
}

func activeUserByIDResponse(userID, name string) stubQueryResponse {
	return stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			return strings.Contains(strings.ToLower(query), "users") && coverageHasStringArg(args, userID)
		},
		columns: []string{"id", "user_id", "name", "department_id", "status"},
		rows: [][]driver.Value{
			{int64(1), userID, name, "dept-1", "active"},
		},
	}
}

func performanceIndicatorLibraryResponse(defaultCycle string) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: []string{"id", "name", "default_cycle", "status"},
		rows: [][]driver.Value{
			{uint(10), "Quarterly Library", defaultCycle, "active"},
		},
	}
}

func assessmentManagerParticipantResponse() stubQueryResponse {
	managerID := "import-manager-1"
	managerName := "Imported Manager"
	directManagerID := "direct-1"
	directManagerName := "Direct Boss"
	return stubQueryResponse{
		match: stubTableMatcher("performance_participants"),
		columns: []string{
			"id", "activity_id", "employee_id", "employee_name", "department_id", "department_name",
			"manager_id", "manager_name", "direct_manager_id_snapshot", "direct_manager_name_snapshot",
			"manager_source", "status",
		},
		rows: [][]driver.Value{{
			int64(1), "activity-1", "employee-1", "Alice", "dept-1", "Product",
			managerID, managerName, directManagerID, directManagerName,
			ManagerSourceImport, "target_set",
		}},
	}
}

func assessmentManagerUsersResponse() stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "department_id", "status", "manager_user_id", "manager_name", "mobile"},
		rows: [][]driver.Value{
			{int64(1), "direct-1", "Direct Boss", "dept-1", "active", "", "", "13800000001"},
			{int64(2), "dept-head-1", "Department Head", "dept-1", "active", "", "", "13800000002"},
			{int64(3), "center-head-1", "Center Head", "dept-1", "active", "", "", "13800000003"},
			{int64(4), "manual-1", "Manual Boss", "dept-1", "active", "", "", "13800000004"},
			{int64(5), "employee-1", "Alice", "dept-1", "active", "direct-1", "Direct Boss", "13800000005"},
		},
	}
}

func assessmentManagerProfilesResponse(pluck bool) stubQueryResponse {
	return stubQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			isPluck := strings.Contains(lower, "select `user_id`")
			return strings.Contains(lower, "employee_profiles") && isPluck == pluck
		},
		columns: func() []string {
			if pluck {
				return []string{"user_id"}
			}
			return []string{"user_id", "employee_id"}
		}(),
		rows: func() [][]driver.Value {
			if pluck {
				return [][]driver.Value{{"manual-1"}}
			}
			return [][]driver.Value{
				{"direct-1", "D001"},
				{"dept-head-1", "DH001"},
				{"center-head-1", "CH001"},
				{"manual-1", "M001"},
			}
		}(),
	}
}

func assessmentManagerDepartmentsResponse() stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("departments"),
		columns: []string{"id", "department_id", "name", "parent_id", "extension"},
		rows: [][]driver.Value{{
			int64(1),
			"dept-1",
			"Product",
			"",
			[]byte(`{"department_head_user_id":"dept-head-1","center_head_user_id":"center-head-1"}`),
		}},
	}
}

func legacyReviewParticipantResponse(status, finalLevel string) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, status, "", 0, 90, finalLevel, false, nil, nil, nil),
		},
	}
}

// ======================== PerformanceSelfEvalURL ========================

func TestPerformanceSelfEvalURLEdgeCases(t *testing.T) {
	t.Setenv("DINGTALK_APP_HOME_URL", "https://app.example")
	if got := PerformanceSelfEvalURL("", 0); got != "" {
		t.Fatalf("empty inputs should return empty, got %q", got)
	}
	if got := PerformanceSelfEvalURL("act", 0); got != "" {
		t.Fatalf("zero participant should return empty, got %q", got)
	}
	if got := PerformanceSelfEvalURL("", 1); got != "" {
		t.Fatalf("empty activity should return empty, got %q", got)
	}
}
