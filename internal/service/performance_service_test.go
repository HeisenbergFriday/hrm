package service

import (
	"context"
	stdsql "database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"peopleops/internal/database"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestPerformanceLevelByScoreBoundaries(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{-1, "D"},
		{0, "D"},
		{59.99, "D"},
		{60, "C"},
		{79.99, "C"},
		{80, "B"},
		{89.99, "B"},
		{90, "A"},
		{99.99, "A"},
		{100, "S"},
		{120, "S"},
	}

	for _, tt := range tests {
		if got := PerformanceLevelByScore(tt.score); got != tt.want {
			t.Fatalf("PerformanceLevelByScore(%v) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestNormalizePerformanceParticipantStatus(t *testing.T) {
	status, err := normalizePerformanceParticipantStatus(" manager_submitted ")
	if err != nil {
		t.Fatalf("normalizePerformanceParticipantStatus() error = %v", err)
	}
	if status != "manager_submitted" {
		t.Fatalf("status = %q, want manager_submitted", status)
	}

	if _, err := normalizePerformanceParticipantStatus("unknown"); err == nil {
		t.Fatalf("unknown participant status should be rejected")
	}
}

func TestRequirePerformanceReason(t *testing.T) {
	reason, err := requirePerformanceReason("  调整组织口径  ", "原因不能为空")
	if err != nil {
		t.Fatalf("requirePerformanceReason() error = %v", err)
	}
	if reason != "调整组织口径" {
		t.Fatalf("reason = %q, want trimmed reason", reason)
	}
	if _, err := requirePerformanceReason(" ", "原因不能为空"); err == nil {
		t.Fatalf("blank reason should be rejected")
	}
}

func TestResetParticipantProgressArtifactsForStatusClearsDownstreamFields(t *testing.T) {
	now := time.Now()
	departmentScore := 9.1
	participant := &database.PerformanceParticipant{
		Status:                 "result_confirmed",
		TotalManagerScore:      7,
		BonusScore:             1,
		PenaltyScore:           0.5,
		AdjustedScore:          9.1,
		SuggestedLevel:         "B",
		FinalLevel:             "A",
		AdjustReason:           "department adjusted",
		DepartmentAdjusted:     true,
		DepartmentFinalScore:   &departmentScore,
		DepartmentFinalLevel:   "A",
		DepartmentAdjustReason: "department adjusted",
		DepartmentAdjustedAt:   &now,
		DepartmentAdjustedBy:   "u1",
		HRConfirmedAt:          &now,
		HRConfirmedBy:          "hr1",
		ConfirmedAt:            &now,
		ConfirmedBy:            "publisher",
		IsLocked:               true,
		LockedAt:               &now,
		LockedBy:               "locker",
		ForceLocked:            true,
		ForceLockedReason:      "overdue",
	}

	resetFields := resetParticipantProgressArtifactsForStatus(participant, "manager_submitted")
	resetFieldSet := map[string]bool{}
	for _, field := range resetFields {
		resetFieldSet[field] = true
	}

	if !resetFieldSet["department_evaluation"] || !resetFieldSet["hr_review"] || !resetFieldSet["locked"] {
		t.Fatalf("reset fields = %#v, want department/hr/locked reset", resetFields)
	}
	if participant.DepartmentAdjusted || participant.DepartmentFinalScore != nil || participant.DepartmentFinalLevel != "" || participant.DepartmentAdjustedAt != nil || participant.DepartmentAdjustedBy != "" {
		t.Fatalf("department evaluation artifacts should be cleared: %#v", participant)
	}
	if participant.HRConfirmedAt != nil || participant.HRConfirmedBy != "" || participant.ConfirmedAt != nil || participant.ConfirmedBy != "" {
		t.Fatalf("confirmation artifacts should be cleared: %#v", participant)
	}
	if participant.IsLocked || participant.LockedAt != nil || participant.LockedBy != "" || participant.ForceLocked || participant.ForceLockedReason != "" {
		t.Fatalf("lock artifacts should be cleared: %#v", participant)
	}
	if participant.FinalLevel != "B" {
		t.Fatalf("FinalLevel = %q, want suggested level B", participant.FinalLevel)
	}
	if participant.AdjustedScore != 7.5 {
		t.Fatalf("AdjustedScore = %v, want 7.5", participant.AdjustedScore)
	}
}

func TestCanSaveNewFlowPlanAfterTargetSetting(t *testing.T) {
	activity := &database.PerformanceActivity{FlowType: PerformanceFlowNew, Status: "manager_evaluation"}
	participant := &database.PerformanceParticipant{Status: "pending"}
	if !canSaveNewFlowPlanAfterTargetSetting(activity, participant) {
		t.Fatalf("new flow pending participant should be allowed to save plan after target setting")
	}

	activity.Status = "target_approval"
	if !canSaveNewFlowPlanAfterTargetSetting(activity, participant) {
		t.Fatalf("new flow pending participant should be allowed to save plan during target approval")
	}

	participant.Status = "target_set"
	if canSaveNewFlowPlanAfterTargetSetting(activity, participant) {
		t.Fatalf("approved target should not be reopened")
	}

	participant.Status = "pending"
	activity.FlowType = PerformanceFlowOld
	if canSaveNewFlowPlanAfterTargetSetting(activity, participant) {
		t.Fatalf("old flow should not use late plan saving")
	}
}

func TestDefaultPerformanceWorkflowConfigNewFlowMatchesMutengProcess(t *testing.T) {
	config := defaultPerformanceWorkflowConfig(PerformanceFlowNew)
	nodes, ok := config["nodes"].([]string)
	if !ok {
		t.Fatalf("nodes type = %T, want []string", config["nodes"])
	}
	want := []string{
		"target_setting",
		"target_approval",
		"self_evaluation",
		"manager_evaluation",
		"department_evaluation",
		"hr_review",
		"result_publish",
		"archive",
	}
	if !reflect.DeepEqual(nodes, want) {
		t.Fatalf("new flow nodes = %#v, want %#v", nodes, want)
	}
}

func TestNormalizePerformanceActivityKindDefaultsOnlyNewCreation(t *testing.T) {
	kind, err := normalizePerformanceActivityKind(PerformanceFlowNew, "", true)
	if err != nil {
		t.Fatalf("normalizePerformanceActivityKind() error = %v", err)
	}
	if kind != PerformanceActivityKindGoalSetting {
		t.Fatalf("new activity default kind = %q, want %q", kind, PerformanceActivityKindGoalSetting)
	}

	kind, err = normalizePerformanceActivityKind(PerformanceFlowNew, "", false)
	if err != nil {
		t.Fatalf("normalizePerformanceActivityKind(no default) error = %v", err)
	}
	if kind != "" {
		t.Fatalf("existing empty kind = %q, want empty for historical compatibility", kind)
	}

	kind, err = normalizePerformanceActivityKind(PerformanceFlowOld, PerformanceActivityKindReviewScoring, true)
	if err != nil {
		t.Fatalf("old flow kind normalization error = %v", err)
	}
	if kind != "" {
		t.Fatalf("old flow kind = %q, want empty", kind)
	}
}

func TestActivityKindWorkflowAndGoalPhase(t *testing.T) {
	goalActivity := &database.PerformanceActivity{FlowType: PerformanceFlowNew, ActivityKind: PerformanceActivityKindGoalSetting}
	applyActivityKindWorkflowDefaults(goalActivity)
	goalNodes, ok := goalActivity.WorkflowConfig["nodes"].([]string)
	if !ok {
		t.Fatalf("goal activity nodes type = %T", goalActivity.WorkflowConfig["nodes"])
	}
	wantGoalNodes := []string{"target_setting", "target_approval", "archive"}
	if !reflect.DeepEqual(goalNodes, wantGoalNodes) {
		t.Fatalf("goal activity nodes = %#v, want %#v", goalNodes, wantGoalNodes)
	}
	if phase := targetSettingGoalPhaseForActivity(goalActivity); phase != PerformanceGoalPhasePlan {
		t.Fatalf("goal setting phase = %q, want plan", phase)
	}

	reviewActivity := &database.PerformanceActivity{FlowType: PerformanceFlowNew, ActivityKind: PerformanceActivityKindReviewScoring}
	applyActivityKindWorkflowDefaults(reviewActivity)
	reviewNodes, ok := reviewActivity.WorkflowConfig["nodes"].([]string)
	if !ok {
		t.Fatalf("review activity nodes type = %T", reviewActivity.WorkflowConfig["nodes"])
	}
	wantReviewNodes := []string{"target_setting", "self_evaluation", "manager_evaluation", "department_evaluation", "hr_review", "result_publish", "archive"}
	if !reflect.DeepEqual(reviewNodes, wantReviewNodes) {
		t.Fatalf("review activity nodes = %#v, want %#v", reviewNodes, wantReviewNodes)
	}
	if phase := targetSettingGoalPhaseForActivity(reviewActivity); phase != PerformanceGoalPhaseReview {
		t.Fatalf("review scoring phase = %q, want review", phase)
	}
}

func TestPerformanceLevelByRuleConfigNewFlowBoundaries(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{5.99, "D"},
		{6, "C"},
		{7.49, "C"},
		{7.5, "B"},
		{8.99, "B"},
		{9, "A"},
		{10, "A"},
		{10.01, "S"},
	}

	for _, tt := range tests {
		if got := PerformanceLevelByRuleConfig(tt.score, nil, PerformanceFlowNew); got != tt.want {
			t.Fatalf("PerformanceLevelByRuleConfig(%v, new) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestNormalizeNewPerformanceGoalRecordsDoesNotInjectFixedItems(t *testing.T) {
	records := []GoalRecordRequest{
		{
			ID:           12,
			SectionType:  "key_action",
			GoalType:     "fixed",
			FixedKey:     "manager_arrangement",
			ItemName:     "tampered",
			Weight:       0.5,
			ActualResult: "done",
			SelfScore:    8,
			ManagerScore: 9,
			Attachments:  []string{"file-1"},
			SortOrder:    7,
		},
		{
			SectionType: "quantitative",
			GoalPhase:   "plan",
			GoalType:    "kpi",
			ItemName:    "销售额",
			Weight:      0.7,
			TargetValue: "100万",
		},
	}

	got := normalizeNewPerformanceGoalRecords(records, "plan")
	if len(got) != 1 {
		t.Fatalf("normalizeNewPerformanceGoalRecords() length = %d, want 1", len(got))
	}
	if got[0].ItemName != "销售额" || got[0].GoalType != "kpi" || got[0].Weight != 0.7 {
		t.Fatalf("variable item order/content changed: %#v", got[0])
	}
	if got[0].GoalPhase != "plan" || got[0].IsFixed || got[0].FixedKey != "" {
		t.Fatalf("new flow plan item should stay variable plan item: %#v", got[0])
	}
	if got[0].TargetValue != "" {
		t.Fatalf("new flow plan item should clear target value: %#v", got[0])
	}

	reviewGot := normalizeNewPerformanceGoalRecords(records, "review")
	if len(reviewGot) != 1 || reviewGot[0].TargetValue != "100万" || reviewGot[0].GoalPhase != "review" {
		t.Fatalf("new flow review supplement item should keep target value: %#v", reviewGot)
	}
}

func TestPerformanceDistributionPercentagesNewFlowDefault(t *testing.T) {
	activity := &database.PerformanceActivity{FlowType: PerformanceFlowNew}
	got := performanceDistributionPercentages(activity)
	want := map[string]int{"S": 5, "A": 15, "B": 60, "C": 15, "D": 5}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("performanceDistributionPercentages(new) = %#v, want %#v", got, want)
	}
}

func TestGetDistributionCheckUsesNewFlowActivityDefaults(t *testing.T) {
	rows := make([][]driver.Value, 0, 20)
	for i := 1; i <= 20; i++ {
		level := "B"
		if i <= 2 {
			level = "S"
		}
		rows = append(rows, []driver.Value{int64(i), "activity-1", "manager_submitted", level})
	}

	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "flow_type"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "quarterly", "manager_evaluation", PerformanceFlowNew},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "final_level"},
			rows:    rows,
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
	if check.Passed {
		t.Fatalf("new flow S quota should fail at 2/20 with 5%% default: %#v", check)
	}
	if got := check.Distribution["S"]; got.ExpectedPercent != 5 || got.ExpectedCount != 1 || got.ActualCount != 2 {
		t.Fatalf("S distribution = %#v, want 5%% expectedCount=1 actualCount=2", got)
	}
}

func TestGetDistributionCheckPrefersSuggestedLevelDuringManagerEvaluation(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "flow_type"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "quarterly", "manager_evaluation", PerformanceFlowNew},
			},
		},
		stubQueryResponse{
			match: stubTableMatcher("performance_participants"),
			columns: []string{
				"id", "activity_id", "status", "final_level", "suggested_level", "department_adjusted", "department_final_level",
			},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "manager_submitted", "A", "D", false, ""},
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
	if got := check.Distribution["A"].ActualCount; got != 0 {
		t.Fatalf("A actual count = %d, want 0", got)
	}
	if got := check.Distribution["D"].ActualCount; got != 1 {
		t.Fatalf("D actual count = %d, want 1", got)
	}
}

func TestPerformanceSelfEvalURL(t *testing.T) {
	t.Setenv("DINGTALK_APP_HOME_URL", "https://peopleops.example/app")

	if got := PerformanceSelfEvalURL("", 1); got != "" {
		t.Fatalf("empty activity URL = %q, want empty", got)
	}
	if got := PerformanceSelfEvalURL("activity /x", 0); got != "" {
		t.Fatalf("zero participant URL = %q, want empty", got)
	}

	want := "https://peopleops.example/app/performance-self-eval/activity%20%2Fx/7"
	if got := PerformanceSelfEvalURL("activity /x", 7); got != want {
		t.Fatalf("PerformanceSelfEvalURL() = %q, want %q", got, want)
	}
}

func TestDueSelfEvalReminderRound(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, loc)
	activity := &database.PerformanceActivity{SelfEvalEndAt: "2026-06-05"}

	key, days, ok := dueSelfEvalReminderRound(activity, now)
	if !ok || key != "self_eval_due_in_3d" || days != 3 {
		t.Fatalf("round = (%q, %d, %v), want 3-day reminder", key, days, ok)
	}

	activity.ReminderConfig = map[string]interface{}{"self_eval_reminder_days": []interface{}{float64(2), float64(0)}}
	key, days, ok = dueSelfEvalReminderRound(activity, now)
	if ok || days != 3 || key != "" {
		t.Fatalf("custom offsets should skip 3-day reminder, got (%q, %d, %v)", key, days, ok)
	}

	activity.SelfEvalEndAt = "2026-06-02"
	key, days, ok = dueSelfEvalReminderRound(activity, now)
	if !ok || key != "self_eval_due_today" || days != 0 {
		t.Fatalf("round = (%q, %d, %v), want due today", key, days, ok)
	}

	activity.ReminderConfig = map[string]interface{}{"self_eval_auto_reminder_enabled": false}
	if _, _, ok = dueSelfEvalReminderRound(activity, now); ok {
		t.Fatalf("disabled auto reminder should not match")
	}
}

func TestSendDueSelfEvalAutoRemindersSendsPendingOnly(t *testing.T) {
	originalSender := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = originalSender })
	sentTo := make([]string, 0)
	sendPerformanceActionCardToUser = func(userID, title, content, actionTitle, actionURL string) error {
		sentTo = append(sentTo, userID)
		if !strings.Contains(content, "距离截止还有 3 天") {
			t.Fatalf("content = %q, want deadline reminder text", content)
		}
		if !strings.Contains(actionURL, "/performance-self-eval/1/1") {
			t.Fatalf("actionURL = %q, want self eval link", actionURL)
		}
		return nil
	}

	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status", "self_eval_end_at"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "self_evaluation", "2026-06-05"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
				performanceParticipantStubRow(2, "self_submitted", "done", 90, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "performance_reminder_logs") && strings.Contains(lower, "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(0)}},
		},
	)

	result, err := svc.SendDueSelfEvalAutoReminders(time.Date(2026, 6, 2, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("SendDueSelfEvalAutoReminders() error = %v", err)
	}
	if result.ActivitiesScanned != 1 || result.ActivitiesMatched != 1 || result.Candidates != 1 || result.Sent != 1 || result.Failed != 0 {
		t.Fatalf("result = %#v, want one sent pending participant", result)
	}
	if !reflect.DeepEqual(sentTo, []string{"user-1"}) {
		t.Fatalf("sentTo = %#v, want user-1 only", sentTo)
	}
}

func TestSendDueSelfEvalAutoRemindersSkipsAlreadySentRound(t *testing.T) {
	originalSender := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = originalSender })
	sendPerformanceActionCardToUser = func(userID, title, content, actionTitle, actionURL string) error {
		t.Fatalf("sender should not be called for already sent reminder")
		return nil
	}

	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status", "self_eval_end_at"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "self_evaluation", "2026-06-05"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "performance_reminder_logs") && strings.Contains(lower, "count(")
			},
			columns: []string{"count"},
			rows:    [][]driver.Value{{int64(1)}},
		},
	)

	result, err := svc.SendDueSelfEvalAutoReminders(time.Date(2026, 6, 2, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("SendDueSelfEvalAutoReminders() error = %v", err)
	}
	if result.Sent != 0 || result.AlreadySent != 1 {
		t.Fatalf("result = %#v, want already sent skip", result)
	}
}

func TestPerformanceNoticeURLs(t *testing.T) {
	t.Setenv("DINGTALK_APP_HOME_URL", "https://peopleops.example/app")

	tests := []struct {
		name string
		got  string
		want string
	}{
		{
			name: "manager eval",
			got:  PerformanceManagerEvalURL("activity /x", 7),
			want: "https://peopleops.example/app/performance-manager-eval/activity%20%2Fx/7",
		},
		{
			name: "result",
			got:  PerformanceResultURL("activity /x", 7),
			want: "https://peopleops.example/app/performance-result/activity%20%2Fx/7",
		},
		{
			name: "overview with activity",
			got:  PerformanceOverviewURL("activity /x"),
			want: "https://peopleops.example/app/performance-overview?activity_id=activity+%2Fx",
		},
		{
			name: "overview without activity",
			got:  PerformanceOverviewURL(""),
			want: "https://peopleops.example/app/performance-overview",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Fatalf("URL = %q, want %q", tt.got, tt.want)
			}
		})
	}

	if got := PerformanceManagerEvalURL("", 7); got != "" {
		t.Fatalf("empty manager activity URL = %q, want empty", got)
	}
	if got := PerformanceResultURL("activity", 0); got != "" {
		t.Fatalf("zero result participant URL = %q, want empty", got)
	}
}

func TestNormalizeManagerSources(t *testing.T) {
	sourceTests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", ManagerSourceManual, false},
		{"direct_manager", ManagerSourceDirectManager, false},
		{"department_head", ManagerSourceDepartmentHead, false},
		{"center_head", ManagerSourceCenterHead, false},
		{"manual", ManagerSourceManual, false},
		{"import", ManagerSourceImport, false},
		{"empty", ManagerSourceEmpty, false},
		{"system", ManagerSourceSystem, false},
		{"bad-source", "", true},
	}
	for _, tt := range sourceTests {
		got, err := normalizeAssessmentManagerSource(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("normalizeAssessmentManagerSource(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeAssessmentManagerSource(%q) error = %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeAssessmentManagerSource(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	defaultTests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", ManagerSourceDirectManager, false},
		{"department_head", ManagerSourceDepartmentHead, false},
		{"center_head", ManagerSourceCenterHead, false},
		{"empty", ManagerSourceEmpty, false},
		{"manual", "", true},
		{"import", "", true},
		{"system", "", true},
	}
	for _, tt := range defaultTests {
		got, err := normalizeDefaultAssessmentManagerSource(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("normalizeDefaultAssessmentManagerSource(%q) expected error", tt.input)
			}
			continue
		}
		if err != nil {
			t.Fatalf("normalizeDefaultAssessmentManagerSource(%q) error = %v", tt.input, err)
		}
		if got != tt.want {
			t.Fatalf("normalizeDefaultAssessmentManagerSource(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}

	assignmentTests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"", ManagerSourceImport, false},
		{"empty", ManagerSourceImport, false},
		{"system", ManagerSourceImport, false},
		{"manual", ManagerSourceManual, false},
		{"direct_manager", ManagerSourceDirectManager, false},
		{"bad-source", "", true},
	}
	for _, tt := range assignmentTests {
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

func TestResolveManagerInfo(t *testing.T) {
	user := database.User{
		ManagerUserID: " manager-1 ",
		ManagerName:   " Boss ",
		Extension: map[string]interface{}{
			"manager_user_id": "ignored",
			"manager_name":    "Ignored",
		},
	}
	managerID, managerName := resolveManagerInfo(user)
	if managerID != "manager-1" || managerName != "Boss" {
		t.Fatalf("explicit manager = (%q, %q), want (manager-1, Boss)", managerID, managerName)
	}

	user = database.User{
		Extension: map[string]interface{}{
			"leader_user_id":  " leader-1 ",
			"supervisor_name": " Supervisor ",
		},
	}
	managerID, managerName = resolveManagerInfo(user)
	if managerID != "leader-1" || managerName != "Supervisor" {
		t.Fatalf("extension manager = (%q, %q), want (leader-1, Supervisor)", managerID, managerName)
	}
}

func TestCheckTimeWindow(t *testing.T) {
	future := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	past := time.Now().AddDate(0, 0, -1).Format("2006-01-02")

	if err := checkTimeWindow(&database.PerformanceActivity{StrictTimeMode: false, SelfEvalStartAt: future}, "self_evaluation"); err != nil {
		t.Fatalf("non-strict time window error = %v", err)
	}
	if err := checkTimeWindow(&database.PerformanceActivity{StrictTimeMode: true, SelfEvalStartAt: future}, "self_evaluation"); err == nil {
		t.Fatalf("future strict self evaluation start should fail")
	}
	if err := checkTimeWindow(&database.PerformanceActivity{StrictTimeMode: true, SelfEvalEndAt: past}, "self_evaluation"); err == nil {
		t.Fatalf("past strict self evaluation end should fail")
	}
	if err := checkTimeWindow(&database.PerformanceActivity{StrictTimeMode: true, ManagerEvalStartAt: past, ManagerEvalEndAt: future}, "manager_evaluation"); err != nil {
		t.Fatalf("current strict manager evaluation window error = %v", err)
	}
	if err := checkTimeWindow(&database.PerformanceActivity{StrictTimeMode: true}, "unknown"); err != nil {
		t.Fatalf("unknown stage without dates error = %v", err)
	}
}

func TestParticipantStageCompletionAndEvidence(t *testing.T) {
	tests := []struct {
		status string
		stage  string
		want   bool
	}{
		{"target_set", "target_setting", true},
		{"target_pending_approval", "target_setting", false},
		{"self_submitted", "self_evaluation", true},
		{"self_submitted", "manager_evaluation", false},
		{"manager_submitted", "manager_evaluation", true},
		{"employee_confirmed", "employee_confirmation", true},
		{"manager_confirmed", "manager_confirmation", true},
		{"hr_confirmed", "hr_confirmation", true},
		{"locked", "hr_confirmation", true},
		{"inactive", "self_evaluation", false},
		{"manager_submitted", "unknown", false},
	}
	for _, tt := range tests {
		if got := participantCompletedStage(tt.status, tt.stage); got != tt.want {
			t.Fatalf("participantCompletedStage(%q, %q) = %v, want %v", tt.status, tt.stage, got, tt.want)
		}
	}

	now := time.Now()
	evidenceTests := []struct {
		name        string
		participant database.PerformanceParticipant
		stage       string
		want        bool
	}{
		{
			name:        "employee confirmation needs timestamp at exact status",
			participant: database.PerformanceParticipant{Status: "employee_confirmed"},
			stage:       "employee_confirmation",
			want:        false,
		},
		{
			name:        "employee confirmation with timestamp",
			participant: database.PerformanceParticipant{Status: "employee_confirmed", EmployeeConfirmedAt: &now},
			stage:       "employee_confirmation",
			want:        true,
		},
		{
			name:        "later status is enough for employee confirmation evidence",
			participant: database.PerformanceParticipant{Status: "manager_confirmed"},
			stage:       "employee_confirmation",
			want:        true,
		},
		{
			name:        "manager confirmation needs timestamp at exact status",
			participant: database.PerformanceParticipant{Status: "manager_confirmed"},
			stage:       "manager_confirmation",
			want:        false,
		},
		{
			name:        "hr confirmation needs timestamp at exact status",
			participant: database.PerformanceParticipant{Status: "hr_confirmed"},
			stage:       "hr_confirmation",
			want:        false,
		},
		{
			name:        "locked always has evidence",
			participant: database.PerformanceParticipant{Status: "locked"},
			stage:       "hr_confirmation",
			want:        true,
		},
	}
	for _, tt := range evidenceTests {
		t.Run(tt.name, func(t *testing.T) {
			if got := participantHasStageEvidence(tt.participant, tt.stage); got != tt.want {
				t.Fatalf("participantHasStageEvidence() = %v, want %v", got, tt.want)
			}
		})
	}

	if !isIgnoredPerformanceParticipantStatus("inactive") {
		t.Fatalf("inactive should be ignored")
	}
	if !isIgnoredPerformanceParticipantStatus("removed_from_scope") {
		t.Fatalf("removed_from_scope should be ignored")
	}
	if isIgnoredPerformanceParticipantStatus("pending") {
		t.Fatalf("pending should not be ignored")
	}
}

func TestActivityIncludesUser(t *testing.T) {
	user := database.User{UserID: "user-1", DepartmentID: "dept-1"}

	if !activityIncludesUser(&database.PerformanceActivity{}, user) {
		t.Fatalf("empty scope should include user")
	}
	if !activityIncludesUser(&database.PerformanceActivity{TargetDepartmentIDs: []string{" dept-1 "}}, user) {
		t.Fatalf("matching department scope should include user")
	}
	if activityIncludesUser(&database.PerformanceActivity{TargetDepartmentIDs: []string{"dept-2"}}, user) {
		t.Fatalf("different department scope should exclude user")
	}
	if !activityIncludesUser(&database.PerformanceActivity{TargetEmployeeIDs: []string{" user-1 "}, TargetDepartmentIDs: []string{"dept-2"}}, user) {
		t.Fatalf("matching employee scope should include user")
	}
	if activityIncludesUser(&database.PerformanceActivity{TargetEmployeeIDs: []string{"user-2"}, TargetDepartmentIDs: []string{"dept-1"}}, user) {
		t.Fatalf("employee scope should override department scope")
	}
}

func TestActivityIncludesUserInDepartmentUsesSnapshotDepartment(t *testing.T) {
	user := database.User{UserID: "user-1", DepartmentID: "current-dept"}
	activity := &database.PerformanceActivity{TargetDepartmentIDs: []string{"snapshot-dept"}}

	if !activityIncludesUserInDepartment(activity, user, "snapshot-dept") {
		t.Fatalf("snapshot department should include user")
	}
	if activityIncludesUserInDepartment(activity, user, "current-dept") {
		t.Fatalf("current department should not include user when snapshot department differs")
	}
	employeeScoped := &database.PerformanceActivity{
		TargetEmployeeIDs:   []string{"user-1"},
		TargetDepartmentIDs: []string{"other-dept"},
	}
	if !activityIncludesUserInDepartment(employeeScoped, user, "snapshot-dept") {
		t.Fatalf("explicit employee scope should still include user")
	}
}

func TestPerformanceOrgSnapshotForUserUsesTransferHistory(t *testing.T) {
	user := database.User{
		UserID:       "user-1",
		DepartmentID: "dept-c",
		Position:     "Current Position",
	}
	transfers := []database.EmployeeTransfer{
		{
			ID:                1,
			UserID:            "user-1",
			OldDepartmentID:   "dept-a",
			OldDepartmentName: "Dept A",
			OldPosition:       "Role A",
			NewDepartmentID:   "dept-b",
			NewDepartmentName: "Dept B",
			NewPosition:       "Role B",
			TransferDate:      "2026-01-01",
			Status:            "approved",
		},
		{
			ID:                2,
			UserID:            "user-1",
			OldDepartmentID:   "dept-b",
			OldDepartmentName: "Dept B",
			OldPosition:       "Role B",
			NewDepartmentID:   "dept-c",
			NewDepartmentName: "Dept C",
			NewPosition:       "Role C",
			TransferDate:      "2026-06-01",
			Status:            "approved",
		},
		{
			ID:                3,
			UserID:            "user-1",
			OldDepartmentID:   "dept-c",
			OldDepartmentName: "Dept C",
			OldPosition:       "Role C",
			NewDepartmentID:   "dept-d",
			NewDepartmentName: "Dept D",
			NewPosition:       "Role D",
			TransferDate:      "2026-07-01",
			Status:            "pending",
		},
	}

	asOfMarch := mustPerformanceTestDate(t, "2026-03-31")
	snapshot := performanceOrgSnapshotForUser(user, transfers, asOfMarch, true, nil)
	if snapshot.DepartmentID != "dept-b" || snapshot.DepartmentName != "Dept B" || snapshot.Position != "Role B" {
		t.Fatalf("March snapshot = %#v, want dept-b/Dept B/Role B", snapshot)
	}

	beforeTransfers := mustPerformanceTestDate(t, "2025-12-31")
	snapshot = performanceOrgSnapshotForUser(user, transfers, beforeTransfers, true, nil)
	if snapshot.DepartmentID != "dept-a" || snapshot.DepartmentName != "Dept A" || snapshot.Position != "Role A" {
		t.Fatalf("pre-transfer snapshot = %#v, want dept-a/Dept A/Role A", snapshot)
	}

	afterPendingTransfer := mustPerformanceTestDate(t, "2026-08-01")
	snapshot = performanceOrgSnapshotForUser(user, transfers, afterPendingTransfer, true, nil)
	if snapshot.DepartmentID != "dept-c" || snapshot.DepartmentName != "Dept C" || snapshot.Position != "Role C" {
		t.Fatalf("pending transfer should be ignored, snapshot = %#v", snapshot)
	}
}

func TestParsePerformanceSnapshotDateRejectsInvalidDate(t *testing.T) {
	if _, _, err := parsePerformanceSnapshotDate("2026/06/30"); err == nil {
		t.Fatalf("invalid snapshot date should return error")
	}
	if _, ok, err := parsePerformanceSnapshotDate(""); err != nil || ok {
		t.Fatalf("empty snapshot date = ok %v err %v, want ok false nil err", ok, err)
	}
}

func mustPerformanceTestDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.ParseInLocation("2006-01-02", value, time.Local)
	if err != nil {
		t.Fatalf("parse test date %s: %v", value, err)
	}
	return parsed
}

func TestNewPerformanceParticipantFromUser(t *testing.T) {
	user := database.User{
		UserID:        "user-1",
		Name:          "Alice",
		DepartmentID:  "dept-1",
		Position:      "Engineer",
		Status:        "active",
		ManagerUserID: "manager-1",
		ManagerName:   "Boss",
	}
	participant := newPerformanceParticipantFromUser("activity-1", "operator-1", user, "Product")

	if participant.ActivityID != "activity-1" || participant.EmployeeID != "user-1" || participant.EmployeeName != "Alice" {
		t.Fatalf("participant identity not copied correctly: %#v", participant)
	}
	if participant.DepartmentID != "dept-1" || participant.DepartmentName != "Product" || participant.Position != "Engineer" {
		t.Fatalf("participant org fields not copied correctly: %#v", participant)
	}
	if participant.Status != "pending" || participant.EmployeeStatus != "active" {
		t.Fatalf("participant status = (%q, %q), want (pending, active)", participant.Status, participant.EmployeeStatus)
	}
	if ptrStringValue(participant.ManagerID) != "manager-1" || ptrStringValue(participant.ManagerName) != "Boss" {
		t.Fatalf("manager = (%q, %q), want (manager-1, Boss)", ptrStringValue(participant.ManagerID), ptrStringValue(participant.ManagerName))
	}
	if ptrStringValue(participant.DirectManagerIDSnapshot) != "manager-1" || ptrStringValue(participant.DirectManagerNameSnapshot) != "Boss" {
		t.Fatalf("direct manager snapshot not set: %#v", participant)
	}
	if participant.ManagerSource != ManagerSourceDirectManager || participant.ManagerConfigStatus != ManagerConfigConfigured {
		t.Fatalf("manager source/config = (%q, %q)", participant.ManagerSource, participant.ManagerConfigStatus)
	}

	noManager := newPerformanceParticipantFromUser("activity-1", "operator-1", database.User{UserID: "user-2", Status: "active"}, "")
	if noManager.ManagerID != nil || noManager.ManagerName != nil {
		t.Fatalf("unexpected manager on user without manager: %#v", noManager)
	}
	if noManager.ManagerSource != ManagerSourceEmpty || noManager.ManagerConfigStatus != ManagerConfigPending {
		t.Fatalf("empty manager source/config = (%q, %q)", noManager.ManagerSource, noManager.ManagerConfigStatus)
	}
}

func TestNewPerformanceParticipantForActivityDefaultSources(t *testing.T) {
	svc := &PerformanceService{}
	user := database.User{
		UserID:        "user-1",
		Name:          "Alice",
		DepartmentID:  "dept-1",
		Status:        "active",
		ManagerUserID: "manager-1",
		ManagerName:   "Boss",
	}
	activity := &database.PerformanceActivity{ID: 42, DefaultAssessmentManagerSource: ManagerSourceDirectManager}
	participant := svc.newPerformanceParticipantForActivity(activity, "operator-1", user, "Product")
	if participant.ActivityID != "42" {
		t.Fatalf("ActivityID = %q, want 42", participant.ActivityID)
	}
	if ptrStringValue(participant.ManagerID) != "manager-1" || participant.ManagerSource != ManagerSourceDirectManager {
		t.Fatalf("direct manager assignment not applied: %#v", participant)
	}

	selfManaged := user
	selfManaged.ManagerUserID = selfManaged.UserID
	selfManaged.ManagerName = selfManaged.Name
	participant = svc.newPerformanceParticipantForActivity(activity, "operator-1", selfManaged, "Product")
	if participant.ManagerID != nil || participant.ManagerName != nil {
		t.Fatalf("self manager should stay pending instead of assigning self: %#v", participant)
	}
	if participant.ManagerSource != ManagerSourceEmpty || participant.ManagerConfigStatus != ManagerConfigPending {
		t.Fatalf("self manager source/config = (%q, %q)", participant.ManagerSource, participant.ManagerConfigStatus)
	}

	activity.DefaultAssessmentManagerSource = ManagerSourceEmpty
	participant = svc.newPerformanceParticipantForActivity(activity, "operator-1", user, "Product")
	if participant.ManagerID != nil || participant.ManagerName != nil {
		t.Fatalf("empty default source should not assign manager: %#v", participant)
	}
	if participant.ManagerSource != ManagerSourceEmpty || participant.ManagerConfigStatus != ManagerConfigPending {
		t.Fatalf("empty default source config = (%q, %q)", participant.ManagerSource, participant.ManagerConfigStatus)
	}

	participant = svc.newPerformanceParticipantForActivity(nil, "operator-1", user, "Product")
	if participant.ActivityID != "" || ptrStringValue(participant.ManagerID) != "manager-1" {
		t.Fatalf("nil activity participant = %#v", participant)
	}
}

func TestRefreshPerformanceParticipantProfileDoesNotOverwriteAssessmentManager(t *testing.T) {
	oldManagerID := "old-manager"
	oldManagerName := "Old Boss"
	oldDirectManagerID := "old-direct"
	oldDirectManagerName := "Old Direct"
	existing := &database.PerformanceParticipant{
		ID:                        11,
		ActivityID:                "activity-1",
		EmployeeID:                "user-1",
		EmployeeName:              "Old Name",
		DepartmentID:              "old-dept",
		DepartmentName:            "Old Dept",
		Position:                  "Old Position",
		EmployeeStatus:            "inactive",
		Status:                    "inactive",
		ManagerID:                 &oldManagerID,
		ManagerName:               &oldManagerName,
		DirectManagerIDSnapshot:   &oldDirectManagerID,
		DirectManagerNameSnapshot: &oldDirectManagerName,
		ManagerSource:             ManagerSourceManual,
		ManagerOverridden:         true,
	}
	now := time.Now()
	changed, logs := refreshPerformanceParticipantProfile(existing, database.User{
		UserID:        "user-1",
		Name:          "New Name",
		DepartmentID:  "new-dept",
		Position:      "New Position",
		Status:        "active",
		ManagerUserID: "new-manager",
		ManagerName:   "New Boss",
	}, "New Dept", "activity-1", "operator-1", now)

	if !changed {
		t.Fatalf("expected participant profile to change")
	}
	if existing.Status != "pending" || existing.EmployeeStatus != "active" {
		t.Fatalf("status = (%q, %q), want (pending, active)", existing.Status, existing.EmployeeStatus)
	}
	if existing.DepartmentID != "new-dept" || existing.DepartmentName != "New Dept" {
		t.Fatalf("department = (%q, %q), want (new-dept, New Dept)", existing.DepartmentID, existing.DepartmentName)
	}
	if existing.EmployeeName != "New Name" || existing.Position != "New Position" || existing.UpdatedBy != "operator-1" {
		t.Fatalf("profile fields not updated: %#v", existing)
	}
	if ptrStringValue(existing.ManagerID) != "old-manager" || ptrStringValue(existing.ManagerName) != "Old Boss" {
		t.Fatalf("assessment manager was overwritten: (%q, %q)", ptrStringValue(existing.ManagerID), ptrStringValue(existing.ManagerName))
	}
	if ptrStringValue(existing.DirectManagerIDSnapshot) != "old-direct" || ptrStringValue(existing.DirectManagerNameSnapshot) != "Old Direct" {
		t.Fatalf("direct manager snapshot was overwritten: (%q, %q)", ptrStringValue(existing.DirectManagerIDSnapshot), ptrStringValue(existing.DirectManagerNameSnapshot))
	}
	if len(logs) != 2 {
		t.Fatalf("change logs length = %d, want 2: %#v", len(logs), logs)
	}
}

func TestActivityManagerAssignmentHelpers(t *testing.T) {
	left := []database.PerformanceActivityManagerAssignment{
		{UserID: " user-1 ", EmployeeID: "e1", AssessmentManagerUserID: "manager-1", AssessmentManagerSource: " import ", ManagerOverrideReason: " reason "},
		{UserID: "user-2", EmployeeID: "e2", AssessmentManagerUserID: "manager-2", AssessmentManagerSource: "manual"},
	}
	right := []database.PerformanceActivityManagerAssignment{
		{UserID: "user-2", EmployeeID: "e2", AssessmentManagerUserID: "manager-2", AssessmentManagerSource: "MANUAL"},
		{UserID: "user-1", EmployeeID: "e1", AssessmentManagerUserID: "manager-1", AssessmentManagerSource: "IMPORT", ManagerOverrideReason: "reason"},
	}
	if !sameActivityManagerAssignments(left, right) {
		t.Fatalf("assignments with same normalized values should be equal")
	}
	right[0].ManagerOverrideReason = "changed"
	if sameActivityManagerAssignments(left, right) {
		t.Fatalf("assignments with different reason should not be equal")
	}

	byUser := activityManagerAssignmentsByUser([]database.PerformanceActivityManagerAssignment{
		{UserID: "", AssessmentManagerUserID: "manager-0"},
		{UserID: "user-1", AssessmentManagerUserID: ""},
		{UserID: " user-1 ", AssessmentManagerUserID: " manager-1 "},
	})
	if len(byUser) != 1 || byUser["user-1"].AssessmentManagerUserID != "manager-1" {
		t.Fatalf("activityManagerAssignmentsByUser() = %#v", byUser)
	}

	if shouldApplyActivityManagerAssignment(nil) {
		t.Fatalf("nil participant should not receive assignment")
	}
	if !shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{}) {
		t.Fatalf("non-overridden participant should receive assignment")
	}
	if shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{ManagerOverridden: true, ManagerSource: ManagerSourceManual}) {
		t.Fatalf("manual overridden participant should not receive assignment")
	}
	if !shouldApplyActivityManagerAssignment(&database.PerformanceParticipant{ManagerOverridden: true, ManagerSource: ManagerSourceImport}) {
		t.Fatalf("import overridden participant should receive refreshed import assignment")
	}
}

func TestApplyActivityManagerAssignmentUpdatesAndLogs(t *testing.T) {
	oldManagerID := "old-manager"
	oldManagerName := "Old Boss"
	participant := &database.PerformanceParticipant{
		ID:            12,
		ActivityID:    "activity-1",
		EmployeeID:    "user-1",
		ManagerID:     &oldManagerID,
		ManagerName:   &oldManagerName,
		ManagerSource: ManagerSourceDirectManager,
	}
	now := time.Now()
	changed, log := applyActivityManagerAssignment(
		participant,
		database.PerformanceActivityManagerAssignment{
			AssessmentManagerUserID: " manager-1 ",
			AssessmentManagerSource: "",
			ManagerOverrideReason:   " quarterly override ",
		},
		map[string]database.User{
			"manager-1": {UserID: "manager-1", Name: "New Boss", Status: "active"},
		},
		"operator-1",
		now,
	)

	if !changed || log == nil {
		t.Fatalf("expected assignment to change participant and create log")
	}
	if ptrStringValue(participant.ManagerID) != "manager-1" || ptrStringValue(participant.ManagerName) != "New Boss" {
		t.Fatalf("manager = (%q, %q), want (manager-1, New Boss)", ptrStringValue(participant.ManagerID), ptrStringValue(participant.ManagerName))
	}
	if participant.ManagerSource != ManagerSourceImport || !participant.ManagerOverridden || participant.ManagerOverrideReason != "quarterly override" {
		t.Fatalf("manager metadata not normalized: %#v", participant)
	}
	if participant.ManagerConfigStatus != ManagerConfigConfigured || participant.UpdatedBy != "operator-1" {
		t.Fatalf("manager config/update fields = (%q, %q)", participant.ManagerConfigStatus, participant.UpdatedBy)
	}
	if log.OldManagerID != "old-manager" || log.NewManagerID != "manager-1" || log.NewManagerName != "New Boss" {
		t.Fatalf("log manager fields not populated: %#v", log)
	}
	if log.Source != "import" || log.OperatorID != "operator-1" || !log.ChangedAt.Equal(now) {
		t.Fatalf("log metadata not populated: %#v", log)
	}

	changed, log = applyActivityManagerAssignment(
		participant,
		database.PerformanceActivityManagerAssignment{
			AssessmentManagerUserID: "manager-1",
			AssessmentManagerSource: "",
			ManagerOverrideReason:   "quarterly override",
		},
		map[string]database.User{
			"manager-1": {UserID: "manager-1", Name: "New Boss", Status: "active"},
		},
		"operator-1",
		now,
	)
	if changed || log != nil {
		t.Fatalf("same assignment should be idempotent, got changed=%v log=%#v", changed, log)
	}

	self := &database.PerformanceParticipant{EmployeeID: "user-1"}
	changed, log = applyActivityManagerAssignment(self, database.PerformanceActivityManagerAssignment{AssessmentManagerUserID: "user-1"}, nil, "operator-1", now)
	if changed || log != nil {
		t.Fatalf("self manager assignment should be ignored, got changed=%v log=%#v", changed, log)
	}
}

func TestApplyActivityManagerAssignmentMarksMissingManagerInvalid(t *testing.T) {
	participant := &database.PerformanceParticipant{ID: 1, EmployeeID: "user-1", ActivityID: "activity-1"}
	changed, log := applyActivityManagerAssignment(
		participant,
		database.PerformanceActivityManagerAssignment{AssessmentManagerUserID: "missing-manager", AssessmentManagerSource: "manual"},
		nil,
		"operator-1",
		time.Now(),
	)
	if !changed || log == nil {
		t.Fatalf("missing manager should still update participant and log invalid config")
	}
	if participant.ManagerConfigStatus != ManagerConfigInvalid {
		t.Fatalf("ManagerConfigStatus = %q, want %q", participant.ManagerConfigStatus, ManagerConfigInvalid)
	}
	if ptrStringValue(participant.ManagerID) != "missing-manager" {
		t.Fatalf("ManagerID = %q, want missing-manager", ptrStringValue(participant.ManagerID))
	}
}

func TestAssessmentManagerSourceAllowsUserWithoutDatabase(t *testing.T) {
	svc := &PerformanceService{}
	currentManager := "manager-1"
	directManager := "direct-1"
	participant := &database.PerformanceParticipant{
		EmployeeID:                "user-1",
		ManagerID:                 &currentManager,
		ManagerSource:             ManagerSourceManual,
		DirectManagerIDSnapshot:   &directManager,
		ManagerConfigStatus:       ManagerConfigConfigured,
		ManagerOverridden:         true,
		ManagerOverrideReason:     "manual",
		DirectManagerNameSnapshot: stringPtrOrNil("Direct Boss"),
	}

	if svc.assessmentManagerSourceAllowsUser(nil, "manager-1", ManagerSourceManual) {
		t.Fatalf("nil participant should be denied")
	}
	if svc.assessmentManagerSourceAllowsUser(participant, " ", ManagerSourceManual) {
		t.Fatalf("blank manager should be denied")
	}
	if svc.assessmentManagerSourceAllowsUser(participant, "manager-1", "bad-source") {
		t.Fatalf("bad source should be denied")
	}
	if !svc.assessmentManagerSourceAllowsUser(participant, "manager-1", ManagerSourceManual) {
		t.Fatalf("current manager/source should be allowed")
	}
	if !svc.assessmentManagerSourceAllowsUser(participant, "any-user", ManagerSourceImport) {
		t.Fatalf("import source should allow explicit user")
	}
	if !svc.assessmentManagerSourceAllowsUser(participant, "direct-1", ManagerSourceDirectManager) {
		t.Fatalf("direct manager snapshot should be allowed")
	}
}

func TestStringSetAndGeneralNormalizationHelpers(t *testing.T) {
	if !sameStringSet([]string{" b ", "a", "a", ""}, []string{"a", "b"}) {
		t.Fatalf("sameStringSet should ignore order, blanks, duplicates, and outer spaces")
	}
	if sameStringSet([]string{"a", "b"}, []string{"a", "c"}) {
		t.Fatalf("different string sets should not match")
	}

	if got := stringPtrOrNil(" value "); got == nil || *got != "value" {
		t.Fatalf("stringPtrOrNil trim = %#v, want value", got)
	}
	if got := stringPtrOrNil(" "); got != nil {
		t.Fatalf("stringPtrOrNil blank = %#v, want nil", got)
	}
	if got := formatManagerValue(" manager-1 ", " Boss "); got != "manager-1/Boss" {
		t.Fatalf("formatManagerValue = %q, want manager-1/Boss", got)
	}
	if got := formatManagerValue("manager-1", " "); got != "manager-1" {
		t.Fatalf("formatManagerValue without name = %q, want manager-1", got)
	}
	if got := normalizeGoalWeight(75); got != 0.75 {
		t.Fatalf("normalizeGoalWeight(75) = %v, want 0.75", got)
	}
	if got := normalizeGoalWeight(0.4); got != 0.4 {
		t.Fatalf("normalizeGoalWeight(0.4) = %v, want 0.4", got)
	}
	if got := normalizeGoalWeight(-1); got != 0 {
		t.Fatalf("normalizeGoalWeight(-1) = %v, want 0", got)
	}
	if got := quotaMaxCount(11, 15); got != 2 {
		t.Fatalf("quotaMaxCount(11, 15) = %d, want 2", got)
	}
	if got := quotaMaxCount(0, 15); got != 0 {
		t.Fatalf("quotaMaxCount(0, 15) = %d, want 0", got)
	}
	if got := roundScore(1.235); got != 1.24 {
		t.Fatalf("roundScore(1.235) = %v, want 1.24", got)
	}
}

func TestTargetSettingApproved(t *testing.T) {
	if !targetSettingApproved("target_set", nil) {
		t.Fatalf("target_set status should be approved")
	}
	if !targetSettingApproved("manager_submitted", nil) {
		t.Fatalf("later participant status should be approved")
	}
	if !targetSettingApproved("pending", []database.PerformanceGoalRecord{{ApprovalStatus: "approved"}}) {
		t.Fatalf("approved goal record should approve target setting")
	}
	if targetSettingApproved("pending", []database.PerformanceGoalRecord{{ApprovalStatus: "pending"}}) {
		t.Fatalf("pending goal record should not approve target setting")
	}
}

func TestGetResultSummaryCountsStagesAndIgnoresInactive(t *testing.T) {
	now := time.Now()
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			performanceParticipantStubRow(2, "self_submitted", "self done", 88, 0, "", false, nil, nil, nil),
			performanceParticipantStubRow(3, "manager_submitted", "", 0, 92, "A", false, nil, nil, nil),
			performanceParticipantStubRow(4, "employee_confirmed", "", 0, 90, "A", false, now, nil, nil),
			performanceParticipantStubRow(5, "manager_confirmed", "", 0, 70, "C", false, nil, now, nil),
			performanceParticipantStubRow(6, "hr_confirmed", "", 0, 100, "S", false, nil, nil, now),
			performanceParticipantStubRow(7, "locked", "", 0, 50, "D", true, nil, nil, nil),
			performanceParticipantStubRow(8, "inactive", "", 0, 0, "S", false, nil, nil, nil),
		},
	})

	summary, err := svc.GetResultSummary("activity-1")
	if err != nil {
		t.Fatalf("GetResultSummary() error = %v", err)
	}
	assertSummaryInt := func(key string, want int) {
		t.Helper()
		if got := summary[key].(int); got != want {
			t.Fatalf("%s = %d, want %d", key, got, want)
		}
	}
	assertSummaryInt("total_participants", 7)
	assertSummaryInt("target_set_count", 7)
	assertSummaryInt("self_submitted_count", 6)
	assertSummaryInt("manager_submitted_count", 5)
	assertSummaryInt("employee_confirmed_count", 4)
	assertSummaryInt("manager_confirmed_count", 3)
	assertSummaryInt("hr_confirmed_count", 2)
	assertSummaryInt("locked_count", 1)
	assertSummaryInt("result_confirmed_count", 2)

	dist := summary["level_distribution"].(map[string]int)
	if dist["S"] != 1 || dist["A"] != 2 || dist["B"] != 0 || dist["C"] != 1 || dist["D"] != 1 {
		t.Fatalf("level_distribution = %#v", dist)
	}
}

func TestGetDistributionCheckUsesRulesAndIgnoresInactive(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "final_level"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "manager_submitted", "S"},
				{int64(2), "activity-1", "manager_submitted", "S"},
				{int64(3), "activity-1", "manager_submitted", "A"},
				{int64(4), "activity-1", "manager_submitted", "B"},
				{int64(5), "activity-1", "inactive", "S"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_distribution_rules"),
			columns: []string{"id", "activity_id", "level", "distribution_percent", "description"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "S", int64(25), "top"},
				{int64(2), "activity-1", "A", int64(25), "strong"},
				{int64(3), "activity-1", "B", int64(50), "normal"},
				{int64(4), "activity-1", "C", int64(0), "low"},
				{int64(5), "activity-1", "D", int64(0), "bottom"},
			},
		},
	)

	check, err := svc.GetDistributionCheck("activity-1")
	if err != nil {
		t.Fatalf("GetDistributionCheck() error = %v", err)
	}
	if check.Passed {
		t.Fatalf("distribution should fail when S exceeds quota: %#v", check)
	}
	if check.TotalCount != 4 {
		t.Fatalf("TotalCount = %d, want 4", check.TotalCount)
	}
	if len(check.ExceededLevels) != 1 || check.ExceededLevels[0].Level != "S" || check.ExceededLevels[0].Expected != 1 || check.ExceededLevels[0].Actual != 2 {
		t.Fatalf("ExceededLevels = %#v", check.ExceededLevels)
	}
	if got := check.Distribution["S"]; got.Status != "exceeded" || got.ExpectedCount != 1 || got.ActualCount != 2 {
		t.Fatalf("S distribution = %#v", got)
	}
	if got := check.Distribution["A"]; got.Status != "ok" || got.ExpectedCount != 1 || got.ActualCount != 1 {
		t.Fatalf("A distribution = %#v", got)
	}
}

func TestEnsureParticipantStageCompleteWithDatabaseRows(t *testing.T) {
	now := time.Now()
	completeSvc := newStubPerformanceService(t,
		performanceActivityResponse("employee_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "employee_confirmed_at"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "employee_confirmed", now},
				{int64(2), "activity-1", "manager_confirmed", nil},
				{int64(3), "activity-1", "inactive", nil},
			},
		},
	)
	if err := completeSvc.ensureParticipantStageComplete("activity-1", "employee_confirmation"); err != nil {
		t.Fatalf("complete employee confirmation stage error = %v", err)
	}

	incompleteSvc := newStubPerformanceService(t,
		performanceActivityResponse("employee_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status", "employee_confirmed_at"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "employee_confirmed", nil},
			},
		},
	)
	if err := incompleteSvc.ensureParticipantStageComplete("activity-1", "employee_confirmation"); err == nil {
		t.Fatalf("missing confirmation timestamp should block stage completion")
	}

	emptySvc := newStubPerformanceService(t,
		performanceActivityResponse("employee_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id", "activity_id", "status"},
			rows: [][]driver.Value{
				{int64(1), "activity-1", "inactive"},
			},
		},
	)
	if err := emptySvc.ensureParticipantStageComplete("activity-1", "employee_confirmation"); err == nil {
		t.Fatalf("only ignored participants should block stage completion")
	}
}

func TestGetHRConfirmDeadlineStatusWithDatabaseRows(t *testing.T) {
	deadline := time.Now().AddDate(0, 0, -2).Format("2006-01-02")
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status", "hr_confirm_deadline"},
			rows: [][]driver.Value{
				{int64(1), "Q1", "hr_confirmation", deadline},
			},
		},
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
	if status["deadline"] != deadline {
		t.Fatalf("deadline = %v, want %s", status["deadline"], deadline)
	}
	if status["pending_count"] != 1 {
		t.Fatalf("pending_count = %v, want 1", status["pending_count"])
	}
	if status["overdue"] != true || status["can_force_lock"] != true {
		t.Fatalf("deadline status = %#v, want overdue and can_force_lock", status)
	}
}

func TestValidateTemplateSections(t *testing.T) {
	valid := []PerformanceTemplateSectionRequest{
		{
			Name:        " Results ",
			SectionType: "score",
			Weight:      60,
			Items: []PerformanceTemplateItemRequest{
				{Name: "KPI", MaxScore: 100, Weight: 50},
				{Name: "Quality", MaxScore: 100, Weight: 50},
			},
		},
		{
			Name:        "Culture",
			SectionType: "score",
			Weight:      40,
			Items:       []PerformanceTemplateItemRequest{{Name: "Values", MaxScore: 100, Weight: 100}},
		},
	}
	if err := validateTemplateSections(valid); err != nil {
		t.Fatalf("validateTemplateSections(valid) error = %v", err)
	}

	invalidCases := []struct {
		name     string
		sections []PerformanceTemplateSectionRequest
	}{
		{
			name: "blank section name",
			sections: []PerformanceTemplateSectionRequest{{
				Name:   " ",
				Weight: 100,
				Items:  []PerformanceTemplateItemRequest{{Name: "Item", MaxScore: 100, Weight: 100}},
			}},
		},
		{
			name: "empty items",
			sections: []PerformanceTemplateSectionRequest{{
				Name:   "Section",
				Weight: 100,
			}},
		},
		{
			name: "blank item name",
			sections: []PerformanceTemplateSectionRequest{{
				Name:   "Section",
				Weight: 100,
				Items:  []PerformanceTemplateItemRequest{{Name: " ", MaxScore: 100, Weight: 100}},
			}},
		},
		{
			name: "invalid max score",
			sections: []PerformanceTemplateSectionRequest{{
				Name:   "Section",
				Weight: 100,
				Items:  []PerformanceTemplateItemRequest{{Name: "Item", MaxScore: 0, Weight: 100}},
			}},
		},
		{
			name: "invalid item weight",
			sections: []PerformanceTemplateSectionRequest{{
				Name:   "Section",
				Weight: 100,
				Items:  []PerformanceTemplateItemRequest{{Name: "Item", MaxScore: 100, Weight: 101}},
			}},
		},
		{
			name: "item weights do not total 100",
			sections: []PerformanceTemplateSectionRequest{{
				Name:   "Section",
				Weight: 100,
				Items:  []PerformanceTemplateItemRequest{{Name: "Item", MaxScore: 100, Weight: 90}},
			}},
		},
		{
			name: "section weights do not total 100",
			sections: []PerformanceTemplateSectionRequest{{
				Name:   "Section",
				Weight: 90,
				Items:  []PerformanceTemplateItemRequest{{Name: "Item", MaxScore: 100, Weight: 100}},
			}},
		},
	}
	for _, tt := range invalidCases {
		t.Run(tt.name, func(t *testing.T) {
			if err := validateTemplateSections(tt.sections); err == nil {
				t.Fatalf("validateTemplateSections(%s) expected error", tt.name)
			}
		})
	}
}

func TestLoadTemplateForActivityRejectsMismatchedFlowType(t *testing.T) {
	templateID := uint(7)
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_templates"),
			columns: []string{"id", "name", "code", "flow_type", "status"},
			rows: [][]driver.Value{
				{int64(templateID), "Legacy Template", PerformanceTemplateCodeOld, PerformanceFlowOld, "active"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_sections"),
			columns: []string{"id", "template_id", "name"},
			rows:    nil,
		},
	)

	_, _, err := svc.loadTemplateForActivity(&templateID, PerformanceFlowNew)
	if err == nil || !strings.Contains(err.Error(), "流程模板与流程类型不一致") {
		t.Fatalf("loadTemplateForActivity mismatch error = %v", err)
	}
}

func TestValidateActivityIndicatorLibraryRequiresSameTemplate(t *testing.T) {
	libraryID := uint(10)
	activityTemplateID := uint(1)
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: []string{"id", "department_id", "department_name", "template_id", "name", "default_cycle", "status"},
		rows: [][]driver.Value{
			{int64(libraryID), "dept-1", "Product", int64(2), "沐腾指标库", "monthly", "active"},
		},
	})

	err := svc.validateActivityIndicatorLibrary(&libraryID, "monthly", &activityTemplateID)
	if err == nil || !strings.Contains(err.Error(), "指标库所属流程模板与活动流程模板不一致") {
		t.Fatalf("validateActivityIndicatorLibrary() error = %v, want template mismatch", err)
	}
}

func TestBuildTemplateParts(t *testing.T) {
	sections, items, counts := buildTemplateParts([]PerformanceTemplateSectionRequest{
		{
			Name:              " Results ",
			SectionType:       " score ",
			Weight:            70,
			SortOrder:         2,
			IsScoreRequired:   true,
			IsCommentRequired: true,
			Items: []PerformanceTemplateItemRequest{
				{Name: " KPI ", Description: "d1", MaxScore: 100, Weight: 60, SortOrder: 1},
				{Name: " Quality ", Description: "d2", MaxScore: 50, Weight: 40, SortOrder: 2},
			},
		},
		{
			Name:        "Culture",
			SectionType: "text",
			Weight:      30,
			Items:       []PerformanceTemplateItemRequest{{Name: "Values", MaxScore: 100, Weight: 100}},
		},
	})

	if len(sections) != 2 || len(items) != 3 || !reflect.DeepEqual(counts, []int{2, 1}) {
		t.Fatalf("buildTemplateParts sizes = sections:%d items:%d counts:%v", len(sections), len(items), counts)
	}
	if sections[0].Name != "Results" || sections[0].SectionType != "score" || !sections[0].IsScoreRequired || !sections[0].IsCommentRequired {
		t.Fatalf("section not normalized: %#v", sections[0])
	}
	if items[0].Name != "KPI" || items[1].Name != "Quality" {
		t.Fatalf("items not normalized: %#v", items[:2])
	}
}

func TestIgnoredParticipantStatusList(t *testing.T) {
	statuses := ignoredParticipantStatusList()
	statusSet := make(map[string]struct{}, len(statuses))
	for _, status := range statuses {
		statusSet[status] = struct{}{}
	}
	for _, want := range []string{"inactive", "removed_from_scope"} {
		if _, ok := statusSet[want]; !ok {
			t.Fatalf("ignoredParticipantStatusList() missing %q from %v", want, statuses)
		}
	}
	if len(statusSet) != len(statuses) {
		t.Fatalf("ignoredParticipantStatusList() contains duplicates: %v", statuses)
	}
}

func TestNormalizeTimeOrEmpty(t *testing.T) {
	if got := normalizeTimeOrEmpty(" "); got != "" {
		t.Fatalf("normalizeTimeOrEmpty blank = %q, want empty", got)
	}
	input := "2026-06-05T10:20:30Z"
	if got := normalizeTimeOrEmpty(input); got != input {
		t.Fatalf("normalizeTimeOrEmpty RFC3339 = %q, want %q", got, input)
	}
	if got := normalizeTimeOrEmpty("2026-06-05"); got != "2026-06-05" {
		t.Fatalf("normalizeTimeOrEmpty date = %q, want 2026-06-05", got)
	}
}

func TestSortRulesByLevel(t *testing.T) {
	rules := []database.PerformanceDistributionRule{
		{Level: "D"},
		{Level: "A"},
		{Level: "S"},
	}
	sortRulesByLevel(rules)
	got := []string{rules[0].Level, rules[1].Level, rules[2].Level}
	if strings.Join(got, ",") != "A,D,S" {
		t.Fatalf("sorted levels = %v, want [A D S]", got)
	}
}

const stubPerformanceDriverName = "peopleops_stub_mysql"

var (
	stubPerformanceDriverOnce sync.Once
	stubPerformanceDBs        sync.Map
)

type stubQueryResponse struct {
	match   func(query string, args []driver.NamedValue) bool
	columns []string
	rows    [][]driver.Value
}

type stubPerformanceDB struct {
	queries []stubQueryResponse
}

type stubPerformanceDriver struct{}

type stubPerformanceConn struct {
	db *stubPerformanceDB
}

type stubPerformanceStmt struct {
	conn  *stubPerformanceConn
	query string
}

type stubPerformanceRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type stubPerformanceTx struct{}

type stubPerformanceResult struct{}

func newStubPerformanceService(t *testing.T, queries ...stubQueryResponse) *PerformanceService {
	t.Helper()
	stubPerformanceDriverOnce.Do(func() {
		stdsql.Register(stubPerformanceDriverName, stubPerformanceDriver{})
	})

	dsn := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	stubPerformanceDBs.Store(dsn, &stubPerformanceDB{queries: queries})
	t.Cleanup(func() {
		stubPerformanceDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(stubPerformanceDriverName, dsn)
	if err != nil {
		t.Fatalf("open stub sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open stub gorm db: %v", err)
	}
	return NewPerformanceService(db)
}

func stubTableMatcher(table string) func(string, []driver.NamedValue) bool {
	table = strings.ToLower(table)
	return func(query string, _ []driver.NamedValue) bool {
		return strings.Contains(strings.ToLower(query), table)
	}
}

func hrConfirmReminderRecipientsResponse(userIDs ...string) stubQueryResponse {
	rows := make([][]driver.Value, 0, len(userIDs))
	for _, userID := range userIDs {
		rows = append(rows, []driver.Value{userID, userID + " name"})
	}
	return stubQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "users") &&
				strings.Contains(lower, "user_roles") &&
				strings.Contains(lower, "role_permissions") &&
				strings.Contains(lower, "permissions")
		},
		columns: []string{"user_id", "name"},
		rows:    rows,
	}
}

func performanceParticipantStubColumns() []string {
	return []string{
		"id",
		"activity_id",
		"employee_id",
		"employee_name",
		"department_id",
		"department_name",
		"status",
		"self_score",
		"self_summary",
		"manager_score",
		"final_level",
		"is_locked",
		"employee_confirmed_at",
		"manager_confirmed_at",
		"hr_confirmed_at",
	}
}

func performanceParticipantStubRow(id int64, status string, selfSummary string, selfScore float64, managerScore float64, finalLevel string, locked bool, employeeConfirmedAt interface{}, managerConfirmedAt interface{}, hrConfirmedAt interface{}) []driver.Value {
	return []driver.Value{
		id,
		"activity-1",
		fmt.Sprintf("user-%d", id),
		fmt.Sprintf("User %d", id),
		"dept-1",
		"Dept",
		status,
		selfScore,
		selfSummary,
		managerScore,
		finalLevel,
		locked,
		employeeConfirmedAt,
		managerConfirmedAt,
		hrConfirmedAt,
	}
}

func (d stubPerformanceDriver) Open(name string) (driver.Conn, error) {
	value, ok := stubPerformanceDBs.Load(name)
	if !ok {
		return nil, fmt.Errorf("stub db %s not registered", name)
	}
	return &stubPerformanceConn{db: value.(*stubPerformanceDB)}, nil
}

func (c *stubPerformanceConn) Prepare(query string) (driver.Stmt, error) {
	return &stubPerformanceStmt{conn: c, query: query}, nil
}

func (c *stubPerformanceConn) Close() error {
	return nil
}

func (c *stubPerformanceConn) Begin() (driver.Tx, error) {
	return stubPerformanceTx{}, nil
}

func (c *stubPerformanceConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return stubPerformanceTx{}, nil
}

func (c *stubPerformanceConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(query, args)
}

func (c *stubPerformanceConn) ExecContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return stubPerformanceResult{}, nil
}

func (c *stubPerformanceConn) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, response := range c.db.queries {
		if response.match != nil && response.match(query, args) {
			rows := make([][]driver.Value, len(response.rows))
			for i := range response.rows {
				rows[i] = append([]driver.Value(nil), response.rows[i]...)
			}
			return &stubPerformanceRows{
				columns: append([]string(nil), response.columns...),
				rows:    rows,
			}, nil
		}
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (s *stubPerformanceStmt) Close() error {
	return nil
}

func (s *stubPerformanceStmt) NumInput() int {
	return -1
}

func (s *stubPerformanceStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return stubPerformanceResult{}, nil
}

func (s *stubPerformanceStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return s.conn.query(s.query, named)
}

func (r *stubPerformanceRows) Columns() []string {
	return r.columns
}

func (r *stubPerformanceRows) Close() error {
	return nil
}

func (r *stubPerformanceRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.index]
	r.index++
	for i := range dest {
		dest[i] = nil
		if i < len(row) {
			dest[i] = row[i]
		}
	}
	return nil
}

func (stubPerformanceTx) Commit() error {
	return nil
}

func (stubPerformanceTx) Rollback() error {
	return nil
}

func (stubPerformanceResult) LastInsertId() (int64, error) {
	return 1, nil
}

func (stubPerformanceResult) RowsAffected() (int64, error) {
	return 1, nil
}
