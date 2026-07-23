package service

import (
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
	"peopleops/internal/database"
)

func TestCreateActivityTrimsDefaultsAndValidatesInput(t *testing.T) {
	svc := newStubPerformanceService(t,
		activeUserResponse("manager-1", "Boss"),
		performanceActivityResponse("draft", ""),
		performanceDepartmentsResponse(),
		performanceParticipantsResponse(nil),
	)

	activity, err := svc.CreateActivity(CreateActivityRequest{
		Name:        " Q2 Review ",
		CycleType:   " quarterly ",
		Status:      " draft ",
		Description: "desc",
		ManagerAssignments: []database.PerformanceActivityManagerAssignment{
			{
				UserID:                  " employee-1 ",
				AssessmentManagerUserID: " manager-1 ",
				AssessmentManagerSource: " manual ",
				ManagerOverrideReason:   " temporary ",
			},
		},
	}, "creator-1")
	if err != nil {
		t.Fatalf("CreateActivity() error = %v", err)
	}
	if activity.Name != "Q2 Review" || activity.CycleType != "quarterly" || activity.Status != "draft" {
		t.Fatalf("activity fields not normalized: %#v", activity)
	}
	if activity.DefaultAssessmentManagerSource != ManagerSourceDirectManager {
		t.Fatalf("DefaultAssessmentManagerSource = %q, want %q", activity.DefaultAssessmentManagerSource, ManagerSourceDirectManager)
	}
	if len(activity.ManagerAssignments) != 1 ||
		activity.ManagerAssignments[0].UserID != "employee-1" ||
		activity.ManagerAssignments[0].AssessmentManagerUserID != "manager-1" ||
		activity.ManagerAssignments[0].AssessmentManagerName != "Boss" ||
		activity.ManagerAssignments[0].AssessmentManagerSource != ManagerSourceManual ||
		activity.ManagerAssignments[0].ManagerOverrideReason != "temporary" {
		t.Fatalf("manager assignment not normalized: %#v", activity.ManagerAssignments)
	}

	if _, err := svc.CreateActivity(CreateActivityRequest{CycleType: "quarterly"}, "creator-1"); err == nil {
		t.Fatalf("CreateActivity() expected blank name error")
	}
	if _, err := svc.CreateActivity(CreateActivityRequest{Name: "Q2", CycleType: "quarterly", DefaultAssessmentManagerSource: "manual"}, "creator-1"); err == nil {
		t.Fatalf("CreateActivity() expected invalid default manager source error")
	}
}

func TestCreateActivitySyncsDraftParticipants(t *testing.T) {
	activeUsersQueried := false
	participantsQueried := false
	svc := newStubPerformanceService(t,
		performanceActivityResponse("draft", ""),
		stubQueryResponse{
			match: func(query string, args []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				if !strings.Contains(lower, "users") || strings.Contains(lower, "user_roles") {
					return false
				}
				hasActive := false
				for _, arg := range args {
					if arg.Value == "active" {
						hasActive = true
						break
					}
				}
				if hasActive {
					activeUsersQueried = true
					return true
				}
				return false
			},
			columns: []string{"id", "user_id", "name", "department_id", "status"},
			rows: [][]driver.Value{
				{int64(1), "employee-1", "Alice", "dept-1", "active"},
			},
		},
		performanceDepartmentsResponse([]driver.Value{int64(1), "dept-1", "Engineering"}),
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				if strings.Contains(strings.ToLower(query), "performance_participants") {
					participantsQueried = true
					return true
				}
				return false
			},
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)

	if _, err := svc.CreateActivity(CreateActivityRequest{
		Name:              "Q2",
		CycleType:         "quarterly",
		Status:            "draft",
		TargetEmployeeIDs: []string{"employee-1"},
	}, "creator-1"); err != nil {
		t.Fatalf("CreateActivity() error = %v", err)
	}
	if !activeUsersQueried || !participantsQueried {
		t.Fatalf("CreateActivity() did not sync draft participants: activeUsersQueried=%v participantsQueried=%v", activeUsersQueried, participantsQueried)
	}
}

func TestUpdateActivityRejectsScopeChangeAfterDraft(t *testing.T) {
	svc := newStubPerformanceService(t, performanceActivityResponse("target_setting", ""))

	_, err := svc.UpdateActivity("1", CreateActivityRequest{
		Name:                "Q2",
		CycleType:           "quarterly",
		Status:              "target_setting",
		TargetEmployeeIDs:   []string{"employee-1"},
		TargetDepartmentIDs: []string{"dept-1"},
	}, "operator-1")
	if err == nil {
		t.Fatalf("UpdateActivity() expected scope change error after draft")
	}
}

func TestSetDistributionRulesValidatesAndPersistsNormalizedRules(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_distribution_rules"),
		columns: []string{"id", "activity_id", "level", "distribution_percent", "description", "created_by", "updated_by"},
		rows: [][]driver.Value{
			{int64(1), "activity-1", "S", int64(15), "top", "operator-1", "operator-1"},
			{int64(2), "activity-1", "A", int64(20), "strong", "operator-1", "operator-1"},
			{int64(3), "activity-1", "B", int64(40), "solid", "operator-1", "operator-1"},
			{int64(4), "activity-1", "C", int64(10), "watch", "operator-1", "operator-1"},
			{int64(5), "activity-1", "D", int64(15), "risk", "operator-1", "operator-1"},
		},
	})

	if _, err := svc.SetDistributionRules("activity-1", nil, "operator-1"); err == nil {
		t.Fatalf("SetDistributionRules() expected empty rules error")
	}
	if _, err := svc.SetDistributionRules("activity-1", []struct {
		Level               string
		DistributionPercent float64
		Description         string
	}{
		{Level: "S", DistributionPercent: 50},
		{Level: "S", DistributionPercent: 50},
	}, "operator-1"); err == nil {
		t.Fatalf("SetDistributionRules() expected duplicate level error")
	}
	if _, err := svc.SetDistributionRules("activity-1", []struct {
		Level               string
		DistributionPercent float64
		Description         string
	}{
		{Level: "S", DistributionPercent: 10},
		{Level: "A", DistributionPercent: 10},
	}, "operator-1"); err == nil {
		t.Fatalf("SetDistributionRules() expected invalid total error")
	}

	rules, err := svc.SetDistributionRules("activity-1", []struct {
		Level               string
		DistributionPercent float64
		Description         string
	}{
		{Level: " S ", DistributionPercent: 15, Description: "top"},
		{Level: "A", DistributionPercent: 20, Description: "strong"},
		{Level: "B", DistributionPercent: 40, Description: "solid"},
		{Level: "C", DistributionPercent: 10, Description: "watch"},
		{Level: "D", DistributionPercent: 15, Description: "risk"},
	}, "operator-1")
	if err != nil {
		t.Fatalf("SetDistributionRules() error = %v", err)
	}
	if len(rules) != 5 || rules[0].Level != "S" || rules[0].DistributionPercent != 15 {
		t.Fatalf("rules = %#v, want normalized persisted rules", rules)
	}
}

func TestOpenEmployeeConfirmationBlocksExceededDistribution(t *testing.T) {
	svc := newStubPerformanceService(t,
		performanceActivityResponse("manager_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 95, "S", false, nil, nil, nil),
				performanceParticipantStubRow(2, "manager_submitted", "", 0, 94, "S", false, nil, nil, nil),
				performanceParticipantStubRow(3, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
				performanceParticipantStubRow(4, "manager_submitted", "", 0, 85, "B", false, nil, nil, nil),
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

	if err := svc.OpenEmployeeConfirmation("activity-1", "operator-1"); err == nil {
		t.Fatalf("OpenEmployeeConfirmation() expected distribution exceeded error")
	}
}

func TestOpenConfirmationStagesRequireEvidence(t *testing.T) {
	now := time.Now()

	managerConfirmationSvc := newStubPerformanceService(t,
		performanceActivityResponse("employee_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, nil, nil, nil),
			},
		},
	)
	if err := managerConfirmationSvc.OpenManagerConfirmation("activity-1", "operator-1"); err == nil {
		t.Fatalf("OpenManagerConfirmation() expected missing employee confirmation timestamp error")
	}

	hrConfirmationSvc := newStubPerformanceService(t,
		performanceActivityResponse("manager_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, now, now, nil),
			},
		},
	)
	if err := hrConfirmationSvc.OpenHRConfirmation("activity-1", "operator-1"); err != nil {
		t.Fatalf("OpenHRConfirmation() error = %v", err)
	}
}

func TestMutengReviewScoringUsesHRReviewInsteadOfLegacyConfirmation(t *testing.T) {
	now := time.Now()
	hrReviewSvc := newStubPerformanceService(t,
		mutengReviewScoringActivityResponse("department_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: append(performanceParticipantStubColumns(), "department_adjusted_at"),
			rows: [][]driver.Value{
				append(performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, nil, nil), now),
			},
		},
	)
	if err := hrReviewSvc.OpenHRReview("activity-1", "operator-1"); err != nil {
		t.Fatalf("OpenHRReview(muteng review scoring) error = %v", err)
	}

	hrConfirmationSvc := newStubPerformanceService(t,
		mutengReviewScoringActivityResponse("department_evaluation", ""),
	)
	if err := hrConfirmationSvc.OpenHRConfirmation("activity-1", "operator-1"); err == nil || !strings.Contains(err.Error(), "不包含HR确认节点") {
		t.Fatalf("OpenHRConfirmation(muteng review scoring) error = %v, want HR confirmation blocked", err)
	}

	employeeConfirmationSvc := newStubPerformanceService(t,
		mutengReviewScoringActivityResponse("hr_review", ""),
	)
	if err := employeeConfirmationSvc.OpenEmployeeConfirmation("activity-1", "operator-1"); err == nil || !strings.Contains(err.Error(), "不包含员工确认节点") {
		t.Fatalf("OpenEmployeeConfirmation(muteng review scoring) error = %v, want employee confirmation blocked", err)
	}
}

func TestPublishCloseArchiveAndLockActivityFlows(t *testing.T) {
	publishSvc := newStubPerformanceService(t,
		performanceActivityResponse("target_setting", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
	)
	if err := publishSvc.PublishActivity("activity-1", "operator-1"); err != nil {
		t.Fatalf("PublishActivity() error = %v", err)
	}

	closeSvc := newStubPerformanceService(t, performanceActivityResponse("locked", ""))
	if err := closeSvc.CloseActivity("activity-1", "operator-1"); err != nil {
		t.Fatalf("CloseActivity() error = %v", err)
	}

	archiveSvc := newStubPerformanceService(t, performanceActivityResponse("result_confirmed", ""))
	if err := archiveSvc.ArchiveActivity("activity-1", "operator-1"); err != nil {
		t.Fatalf("ArchiveActivity() error = %v", err)
	}

	now := time.Now()
	lockSvc := newStubPerformanceService(t,
		performanceActivityResponse("hr_confirmation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "hr_confirmed", "", 0, 90, "A", false, nil, now, now),
			},
		},
	)
	if err := lockSvc.LockActivity("activity-1", "operator-1"); err != nil {
		t.Fatalf("LockActivity() error = %v", err)
	}
}

func TestOpenSelfEvaluationAllowsParticipantLevelReadiness(t *testing.T) {
	svc := newStubPerformanceService(t,
		newFlowPerformanceActivityResponse("target_approval", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceGoalRecordResponse(),
	)

	if err := svc.OpenSelfEvaluation("activity-1", "operator-1"); err != nil {
		t.Fatalf("OpenSelfEvaluation(new flow with participant-level readiness) error = %v", err)
	}
}

func TestConfirmHRResultAdvancesMutengParticipantWithoutWaitingForPeers(t *testing.T) {
	tests := []struct {
		name                string
		incompletePeerCount int64
		wantAggregateStatus string
	}{
		{
			name:                "another participant is still scoring",
			incompletePeerCount: 1,
		},
		{
			name:                "last participant completes HR review",
			incompletePeerCount: 0,
			wantAggregateStatus: "result_publish",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Now()
			participantColumns := append(performanceParticipantStubColumns(), "department_adjusted_at", "result_hidden", "result_hidden_reason")
			participantRow := append(
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, nil, nil),
				now,
				true,
				"system:unpublished",
			)
			svc := newStubPerformanceService(t,
				stubQueryResponse{
					match: func(query string, _ []driver.NamedValue) bool {
						lower := strings.ToLower(query)
						return strings.Contains(lower, "performance_participants") &&
							strings.Contains(lower, "count(*)") && strings.Count(lower, "not in") >= 2
					},
					columns: []string{"count(*)"},
					rows:    [][]driver.Value{{tt.incompletePeerCount}},
				},
				stubQueryResponse{
					match: func(query string, _ []driver.NamedValue) bool {
						lower := strings.ToLower(query)
						return strings.Contains(lower, "performance_participants") &&
							strings.Contains(lower, "count(*)") && strings.Count(lower, "not in") == 1
					},
					columns: []string{"count(*)"},
					rows:    [][]driver.Value{{int64(2)}},
				},
				stubQueryResponse{
					match: func(query string, _ []driver.NamedValue) bool {
						lower := strings.ToLower(query)
						return strings.Contains(lower, "performance_participants") && !strings.Contains(lower, "count(*)")
					},
					columns: participantColumns,
					rows:    [][]driver.Value{participantRow},
				},
				mutengReviewScoringActivityResponse("self_evaluation", ""),
			)

			aggregateStatus := ""
			callbackName := "test:muteng-aggregate:" + strings.ReplaceAll(tt.name, " ", "-")
			if err := svc.db.Callback().Update().Before("gorm:update").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table != "performance_activities" {
					return
				}
				if values, ok := tx.Statement.Dest.(map[string]interface{}); ok {
					aggregateStatus, _ = values["status"].(string)
				}
			}); err != nil {
				t.Fatalf("register update callback: %v", err)
			}

			originalSender := sendPerformanceActionCardToUser
			notified := make(chan struct{}, 1)
			sendPerformanceActionCardToUser = func(_, _, _, _, _, _ string) error {
				notified <- struct{}{}
				return nil
			}
			defer func() { sendPerformanceActionCardToUser = originalSender }()

			if err := svc.ConfirmHRResult(1, "hr-1"); err != nil {
				t.Fatalf("ConfirmHRResult() error = %v", err)
			}
			select {
			case <-notified:
			case <-time.After(time.Second):
				t.Fatal("result publication notification was not sent")
			}
			if aggregateStatus != tt.wantAggregateStatus {
				t.Fatalf("aggregate activity status update = %q, want %q", aggregateStatus, tt.wantAggregateStatus)
			}
		})
	}
}

func TestMutengParticipantIndependentGateHelpers(t *testing.T) {
	oldFlow := &database.PerformanceActivity{FlowType: PerformanceFlowOld, Status: "self_evaluation"}
	if mutengTargetWorkflowOpen(oldFlow) || mutengReviewPipelineOpen(oldFlow) {
		t.Fatal("old flow must not use Muteng independent gates")
	}

	goalSetting := &database.PerformanceActivity{
		FlowType:     PerformanceFlowNew,
		ActivityKind: PerformanceActivityKindGoalSetting,
		Status:       "self_evaluation",
	}
	if !mutengTargetWorkflowOpen(goalSetting) {
		t.Fatal("goal_setting should allow target workflow after target phase is open")
	}
	if mutengReviewPipelineOpen(goalSetting) {
		t.Fatal("goal_setting must never open review/HR pipeline")
	}

	reviewDraft := &database.PerformanceActivity{
		FlowType:     PerformanceFlowNew,
		ActivityKind: PerformanceActivityKindReviewScoring,
		Status:       "draft",
	}
	if mutengTargetWorkflowOpen(reviewDraft) || mutengReviewPipelineOpen(reviewDraft) {
		t.Fatal("draft review activity must stay closed until admin opens gates")
	}

	reviewTargetSetting := &database.PerformanceActivity{
		FlowType:     PerformanceFlowNew,
		ActivityKind: PerformanceActivityKindReviewScoring,
		Status:       "target_setting",
	}
	if !mutengTargetWorkflowOpen(reviewTargetSetting) {
		t.Fatal("target_setting should open target workflow")
	}
	if mutengReviewPipelineOpen(reviewTargetSetting) {
		t.Fatal("target_setting must not open review pipeline")
	}

	// After self-evaluation is opened, every later historical aggregate status
	// still allows individual target/review progression without waiting peers.
	for _, status := range []string{
		"self_evaluation", "manager_evaluation", "department_evaluation", "hr_review",
		"result_publish", "employee_confirmation", "manager_confirmation", "hr_confirmation",
	} {
		activity := &database.PerformanceActivity{
			FlowType:     PerformanceFlowNew,
			ActivityKind: PerformanceActivityKindReviewScoring,
			Status:       status,
		}
		if !mutengTargetWorkflowOpen(activity) {
			t.Fatalf("mutengTargetWorkflowOpen(%q) = false, want true", status)
		}
		if !mutengReviewPipelineOpen(activity) {
			t.Fatalf("mutengReviewPipelineOpen(%q) = false, want true", status)
		}
	}

	for _, status := range []string{"hr_confirmed", "locked", "result_confirmed"} {
		if !mutengPublishedParticipantStatus(status) {
			t.Fatalf("mutengPublishedParticipantStatus(%q) = false, want true", status)
		}
	}
	if mutengPublishedParticipantStatus("manager_confirmed") {
		t.Fatal("manager_confirmed is not published yet")
	}
}

func TestMutengParticipantStageGatesDoNotRequireAggregateStage(t *testing.T) {
	// Target approve uses participant readiness + target gate, not activity=target_approval.
	// Use empty activity_kind (historical hybrid / goal plan path) so scoring activities
	// that intentionally skip goal approval are not mixed into this gate check.
	approveSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "performance_participants") &&
					strings.Contains(lower, "count(*)")
			},
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_pending_approval", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		newFlowPerformanceActivityResponse("self_evaluation", ""),
		stubQueryResponse{
			match:   stubTableMatcher("performance_goal_approval_logs"),
			columns: []string{"id", "participant_id", "activity_id", "action", "version"},
			rows: [][]driver.Value{
				{int64(9), int64(1), "activity-1", "submit", int64(1)},
			},
		},
		activeUserResponse("manager-1", "Boss"),
	)
	if err := approveSvc.SubmitGoalApproval(1, "approve", "ok", "manager-1"); err != nil {
		t.Fatalf("SubmitGoalApproval(approve while peers may lag) error = %v", err)
	}

	// Department evaluation accepts a ready participant while activity remains on self_evaluation.
	level := "A"
	score := 90.0
	deptSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
			},
		},
		mutengReviewScoringActivityResponse("self_evaluation", ""),
	)
	if _, _, err := deptSvc.DepartmentAdjustParticipantResult(1, level, &score, "确认不调整", "dept-1"); err != nil {
		t.Fatalf("DepartmentAdjustParticipantResult(independent) error = %v", err)
	}

	// Self-eval and manager-eval gates reject closed Muteng activity, accept open pipeline.
	closedSelfSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		mutengReviewScoringActivityResponse("target_setting", ""),
	)
	if err := closedSelfSvc.SubmitGoalSelfEvaluation(1, nil, nil, "good", "improve", "user-1"); err == nil || !strings.Contains(err.Error(), "尚未开启员工自评") {
		t.Fatalf("self eval before open error = %v, want self-evaluation gate rejection", err)
	}

	// Old flow still requires aggregate manager_evaluation for manager scoring.
	oldManagerSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "self_submitted", "", 80, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("self_evaluation", ""),
		performanceGoalRecordResponse(
			performanceGoalRecordRow(10, "quantitative", "Revenue", 1),
		),
	)
	if err := oldManagerSvc.SubmitGoalManagerEvaluation(1, []GoalManagerEvaluationItem{
		{RecordID: 10, ManagerScore: 90},
	}, nil, "A", "good", "improve", "manager-1"); err == nil || !strings.Contains(err.Error(), "不允许提交主管评分") {
		t.Fatalf("old flow manager eval error = %v, want aggregate stage rejection", err)
	}
}

func TestConfirmHRResultPreservesManualHideAndIsIdempotentForMuteng(t *testing.T) {
	now := time.Now()
	participantColumns := append(performanceParticipantStubColumns(), "department_adjusted_at", "result_hidden", "result_hidden_reason")
	participantRow := append(
		performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, nil, nil),
		now,
		true,
		"manual:privacy",
	)

	var hiddenAfterSave *bool
	var hiddenReasonAfterSave string
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "performance_participants") &&
					strings.Contains(lower, "count(*)") && strings.Count(lower, "not in") >= 2
			},
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "performance_participants") &&
					strings.Contains(lower, "count(*)") && strings.Count(lower, "not in") == 1
			},
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "performance_participants") && !strings.Contains(lower, "count(*)")
			},
			columns: participantColumns,
			rows:    [][]driver.Value{participantRow},
		},
		mutengReviewScoringActivityResponse("self_evaluation", ""),
	)
	if err := svc.db.Callback().Update().Before("gorm:update").Register("test:muteng-manual-hide", func(tx *gorm.DB) {
		if tx.Statement.Table != "performance_participants" {
			return
		}
		if participant, ok := tx.Statement.Dest.(*database.PerformanceParticipant); ok {
			hidden := participant.ResultHidden
			hiddenAfterSave = &hidden
			hiddenReasonAfterSave = participant.ResultHiddenReason
		}
	}); err != nil {
		t.Fatalf("register update callback: %v", err)
	}

	originalSender := sendPerformanceActionCardToUser
	sendPerformanceActionCardToUser = func(_, _, _, _, _, _ string) error { return nil }
	defer func() { sendPerformanceActionCardToUser = originalSender }()

	if err := svc.ConfirmHRResult(1, "hr-1"); err != nil {
		t.Fatalf("ConfirmHRResult(manual hide) error = %v", err)
	}
	if hiddenAfterSave == nil || !*hiddenAfterSave || hiddenReasonAfterSave != "manual:privacy" {
		t.Fatalf("manual hide was altered: hidden=%v reason=%q", hiddenAfterSave, hiddenReasonAfterSave)
	}

	// Idempotent path: already hr_confirmed returns nil without requiring activity aggregate stage.
	idempotentSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "hr_confirmed", "", 0, 90, "A", false, nil, now, now),
			},
		},
		mutengReviewScoringActivityResponse("self_evaluation", ""),
	)
	if err := idempotentSvc.ConfirmHRResult(1, "hr-1"); err != nil {
		t.Fatalf("ConfirmHRResult(muteng idempotent) error = %v", err)
	}
}

func TestConfirmHRResultRejectsMutengGoalSettingActivity(t *testing.T) {
	now := time.Now()
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
			columns: []string{"id", "name", "cycle_type", "status", "flow_type", "activity_kind", "hr_confirm_deadline", "created_by"},
			rows: [][]driver.Value{
				{int64(1), "Q2 Plan", "quarterly", "target_setting", PerformanceFlowNew, PerformanceActivityKindGoalSetting, "", "creator-1"},
			},
		},
	)
	if err := svc.ConfirmHRResult(1, "hr-1"); err == nil || !strings.Contains(err.Error(), "目标设定活动不包含HR审核节点") {
		t.Fatalf("ConfirmHRResult(goal_setting) error = %v, want goal-setting rejection", err)
	}
}

func TestNormalizePreviousReviewActivityIDRequiresCompletedMutengActivity(t *testing.T) {
	previousID := uint(2)
	tests := []struct {
		name      string
		response  stubQueryResponse
		wantError string
	}{
		{
			name:      "rejects old flow previous activity",
			response:  performanceActivityCandidateResponse(2, "locked", PerformanceFlowOld, "quarterly"),
			wantError: "沐腾科技流程模版",
		},
		{
			name:      "rejects unfinished muteng previous activity",
			response:  performanceActivityCandidateResponse(2, "target_setting", PerformanceFlowNew, "quarterly"),
			wantError: "必须已完成",
		},
		{
			name:      "rejects cycle mismatch",
			response:  performanceActivityCandidateResponse(2, "archived", PerformanceFlowNew, "monthly"),
			wantError: "周期类型",
		},
		{
			name:      "accepts completed muteng previous activity",
			response:  performanceActivityCandidateResponse(2, "archived", PerformanceFlowNew, "quarterly"),
			wantError: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc := newStubPerformanceService(t, tt.response)
			got, err := svc.normalizePreviousReviewActivityID(1, PerformanceFlowNew, PerformanceActivityKindReviewScoring, "quarterly", &previousID)
			if tt.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantError) {
					t.Fatalf("normalizePreviousReviewActivityID() error = %v, want contains %q", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizePreviousReviewActivityID() error = %v", err)
			}
			if got == nil || *got != previousID {
				t.Fatalf("normalizePreviousReviewActivityID() = %v, want %d", got, previousID)
			}
		})
	}
}

func TestFindPreviousPlanActivityFiltersCompletedMutengActivities(t *testing.T) {
	queriedCompletedStatus := false
	svc := newStubPerformanceService(t, stubQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			if strings.Contains(lower, "performance_activities") && strings.Contains(lower, "status in") {
				queriedCompletedStatus = true
				return true
			}
			return false
		},
		columns: []string{"id", "name", "cycle_type", "status", "flow_type", "created_by"},
		rows:    nil,
	})

	previous, err := svc.findPreviousPlanActivity(&database.PerformanceActivity{
		ID:        3,
		FlowType:  PerformanceFlowNew,
		CycleType: "quarterly",
		StartDate: "2026-04-01",
	})
	if err != nil {
		t.Fatalf("findPreviousPlanActivity() error = %v", err)
	}
	if previous != nil {
		t.Fatalf("findPreviousPlanActivity() previous = %#v, want nil", previous)
	}
	if !queriedCompletedStatus {
		t.Fatalf("findPreviousPlanActivity() did not filter completed activity statuses")
	}
}

func TestBatchSaveReviewGoalRecordsAllowsNewFlowTargetSetParticipant(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		newFlowPerformanceActivityResponse("target_setting", ""),
		performanceGoalRecordResponse(),
	)

	if _, err := svc.BatchSaveReviewGoalRecords(1, []GoalRecordRequest{
		{SectionType: "quantitative", GoalPhase: PerformanceGoalPhaseReview, GoalType: "kpi", ItemName: "Revenue", ItemDefinition: "Revenue target", Weight: 0.5, TargetValue: "100"},
		{SectionType: "key_action", GoalPhase: PerformanceGoalPhaseReview, GoalType: "okr", ItemName: "Launch", ItemDefinition: "Launch plan", Weight: 0.5, TargetValue: "Delivered"},
	}, "operator-1"); err != nil {
		t.Fatalf("BatchSaveReviewGoalRecords(new flow target_set) error = %v", err)
	}
}

func TestConfirmEmployeeAndHRResultGates(t *testing.T) {
	employeeSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_submitted", "", 0, 90, "A", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("employee_confirmation", ""),
	)
	if err := employeeSvc.ConfirmEmployeeResult(1, "employee-1"); err != nil {
		t.Fatalf("ConfirmEmployeeResult() error = %v", err)
	}

	hrMissingLevelSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "", false, nil, time.Now(), nil),
			},
		},
		performanceActivityResponse("hr_confirmation", ""),
	)
	if err := hrMissingLevelSvc.ConfirmHRResult(1, "hr-1"); err == nil {
		t.Fatalf("ConfirmHRResult() expected missing final level error")
	}

	hrSuccessSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, time.Now(), nil),
			},
		},
		performanceActivityResponse("hr_confirmation", ""),
	)
	if err := hrSuccessSvc.ConfirmHRResult(1, "hr-1"); err != nil {
		t.Fatalf("ConfirmHRResult() error = %v", err)
	}
}

func TestConfirmManagerAndGoalEvaluationFlows(t *testing.T) {
	confirmManagerSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "employee_confirmed", "", 0, 90, "A", false, time.Now(), nil, nil),
			},
		},
		performanceActivityResponse("manager_confirmation", ""),
	)
	if err := confirmManagerSvc.ConfirmManagerResult(1, "manager-1"); err != nil {
		t.Fatalf("ConfirmManagerResult() error = %v", err)
	}

	selfEvalSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("self_evaluation", ""),
		performanceGoalRecordResponse(
			performanceGoalRecordRow(10, "quantitative", "Revenue", 0.5),
			performanceGoalRecordRow(11, "key_action", "Launch", 0.5),
		),
	)
	if err := selfEvalSvc.SubmitGoalSelfEvaluation(1, []GoalSelfEvaluationItem{
		{RecordID: 10, ActualResult: "done", SelfScore: 80},
		{RecordID: 11, ActualResult: "done", SelfScore: 90},
	}, nil, "good", "improve", "employee-1"); err != nil {
		t.Fatalf("SubmitGoalSelfEvaluation() error = %v", err)
	}

	reopenedSelfEvalSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		newFlowPerformanceActivityResponse("result_publish", ""),
		performanceGoalRecordResponse(
			performanceGoalRecordRow(10, "quantitative", "Revenue", 0.5),
			performanceGoalRecordRow(11, "key_action", "Launch", 0.5),
		),
	)
	if err := reopenedSelfEvalSvc.SubmitGoalSelfEvaluation(1, []GoalSelfEvaluationItem{
		{RecordID: 10, ActualResult: "reopened done", SelfScore: 8},
		{RecordID: 11, ActualResult: "reopened done", SelfScore: 9},
	}, nil, "reopened good", "reopened improve", "employee-1"); err != nil {
		t.Fatalf("SubmitGoalSelfEvaluation() reopened in result_publish error = %v", err)
	}

	managerEvalSvc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "self_submitted", "done", 85, 0, "", false, nil, nil, nil),
			},
		},
		performanceActivityResponse("manager_evaluation", ""),
		performanceGoalRecordResponse(
			performanceGoalRecordRow(10, "quantitative", "Revenue", 0.5),
			performanceGoalRecordRow(11, "key_action", "Launch", 0.5),
		),
	)
	if err := managerEvalSvc.SubmitGoalManagerEvaluation(1, []GoalManagerEvaluationItem{
		{RecordID: 10, ManagerScore: 88},
		{RecordID: 11, ManagerScore: 92},
	}, nil, "", "good", "improve", "manager-1"); err != nil {
		t.Fatalf("SubmitGoalManagerEvaluation() error = %v", err)
	}
}

func TestForceLockOverdueHRConfirmationCountsParticipants(t *testing.T) {
	deadline := time.Now().AddDate(0, 0, -3).Format("2006-01-02")
	now := time.Now()
	svc := newStubPerformanceService(t,
		performanceActivityResponse("hr_confirmation", deadline),
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, now, nil),
				performanceParticipantStubRow(2, "hr_confirmed", "", 0, 88, "B", false, nil, now, now),
				performanceParticipantStubRow(3, "locked", "", 0, 80, "B", true, nil, now, now),
			},
		},
	)

	result, err := svc.ForceLockOverdueHRConfirmation("activity-1", "hr-1")
	if err != nil {
		t.Fatalf("ForceLockOverdueHRConfirmation() error = %v", err)
	}
	if result["force_locked_count"] != 1 || result["locked_count"] != 2 || result["already_locked_count"] != 1 || result["total_count"] != 3 {
		t.Fatalf("force lock result = %#v", result)
	}
}

func TestSetCompanyFinanceCreatesDefaultEqual(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_company_finances"),
		columns: []string{"id", "activity_id", "revenue_sign", "description", "remark", "set_by"},
		rows:    nil,
	})

	finance, err := svc.SetCompanyFinance("activity-1", " ", "balanced", "remark", "hr-1")
	if err != nil {
		t.Fatalf("SetCompanyFinance() error = %v", err)
	}
	if finance.RevenueSign != "equal" || finance.Description != "balanced" || finance.Remark != "remark" || finance.SetBy != "hr-1" || finance.CreatedBy != "hr-1" {
		t.Fatalf("finance not initialized with defaults: %#v", finance)
	}
}

func TestBatchSaveGoalRecordsValidatesTargetSettingWeights(t *testing.T) {
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

	_, err := svc.BatchSaveGoalRecords(1, []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "Revenue", Weight: 60},
		{SectionType: "key_action", ItemName: "Launch", Weight: 30},
	}, "employee-1")
	if err == nil {
		t.Fatalf("BatchSaveGoalRecords() expected total weight validation error")
	}

	successSvc := newStubPerformanceService(t,
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
	if _, err := successSvc.BatchSaveGoalRecords(1, []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "Revenue", Weight: 60},
		{SectionType: "key_action", ItemName: "Launch", Weight: 40},
	}, "employee-1"); err != nil {
		t.Fatalf("BatchSaveGoalRecords() error = %v", err)
	}
}

func TestSubmitGoalApprovalSubmitFlow(t *testing.T) {
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
			rows:    nil,
		},
		activeUserResponse("employee-1", "Alice"),
	)

	if err := svc.SubmitGoalApproval(1, "submit", "ready", "employee-1"); err != nil {
		t.Fatalf("SubmitGoalApproval(submit) error = %v", err)
	}
}

func performanceActivityResponse(status, deadline string) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "cycle_type", "status", "hr_confirm_deadline", "created_by", "org_id"},
		rows: [][]driver.Value{
			{int64(1), "Q2", "quarterly", status, deadline, "creator-1", "test-org"},
		},
	}
}

func newFlowPerformanceActivityResponse(status, deadline string) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "cycle_type", "status", "flow_type", "hr_confirm_deadline", "created_by"},
		rows: [][]driver.Value{
			{int64(1), "Q2", "quarterly", status, PerformanceFlowNew, deadline, "creator-1"},
		},
	}
}

func mutengReviewScoringActivityResponse(status, deadline string) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "org_id", "name", "cycle_type", "status", "flow_type", "activity_kind", "hr_confirm_deadline", "created_by"},
		rows: [][]driver.Value{
			{int64(1), "test-org", "Q2", "quarterly", status, PerformanceFlowNew, PerformanceActivityKindReviewScoring, deadline, "creator-1"},
		},
	}
}

func performanceActivityCandidateResponse(id int64, status, flowType, cycleType string) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "cycle_type", "status", "flow_type", "created_by"},
		rows: [][]driver.Value{
			{id, "Previous", cycleType, status, flowType, "creator-1"},
		},
	}
}

func performanceGoalRecordResponse(rows ...[]driver.Value) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_goal_records"),
		columns: performanceGoalRecordColumns(),
		rows:    rows,
	}
}

func performanceGoalRecordColumns() []string {
	return []string{
		"id",
		"activity_id",
		"participant_id",
		"section_type",
		"item_name",
		"weight",
		"approval_status",
	}
}

func performanceGoalRecordRow(id int64, sectionType, itemName string, weight float64) []driver.Value {
	return []driver.Value{id, "activity-1", int64(1), sectionType, itemName, weight, "pending"}
}

func activeUserResponse(userID, name string) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("users"),
		columns: []string{"id", "user_id", "name", "department_id", "status"},
		rows: [][]driver.Value{
			{int64(1), userID, name, "dept-1", "active"},
		},
	}
}

func performanceDepartmentsResponse(rows ...[]driver.Value) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("departments"),
		columns: []string{"id", "department_id", "name"},
		rows:    rows,
	}
}

func performanceParticipantsResponse(rows [][]driver.Value) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    rows,
	}
}
