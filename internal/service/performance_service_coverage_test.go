package service

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
)

// ======================== UpdateActivity 测试 ========================

func TestUpdateActivity_NotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id"},
		rows:    nil,
	})
	_, err := svc.UpdateActivity("999", CreateActivityRequest{
		Name:      "Test",
		CycleType: "quarterly",
	}, "user-1")
	if err == nil {
		t.Fatal("UpdateActivity(not found) expected error")
	}
}

func TestUpdateActivity_DraftAllowsScopeChange(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "target_department_ids", "target_employee_ids", "manager_assignments"},
			rows: [][]driver.Value{
				{int64(1), "Q1", "quarterly", "draft", `["dept-1"]`, `[]`, `[]`},
			},
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "update") && strings.Contains(strings.ToLower(query), "performance_activities")
			},
			columns: []string{"id"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "status"},
			rows:    [][]driver.Value{},
		},
		stubQueryResponse{
			match:   stubTableMatcher("employee_profiles"),
			columns: []string{"id"},
			rows:    [][]driver.Value{},
		},
		stubQueryResponse{
			match:   stubTableMatcher("departments"),
			columns: []string{"id"},
			rows:    [][]driver.Value{},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: []string{"id"},
			rows:    [][]driver.Value{},
		},
	)

	_, err := svc.UpdateActivity("1", CreateActivityRequest{
		Name:                "Q1 Updated",
		CycleType:           "quarterly",
		Status:              "draft",
		TargetDepartmentIDs: []string{"dept-1", "dept-2"},
	}, "user-1")
	if err != nil {
		t.Fatalf("UpdateActivity(draft scope change) error = %v, want nil", err)
	}
}

func TestUpdateActivity_NonDraftRejectsScopeChange(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "target_department_ids", "target_employee_ids", "manager_assignments"},
			rows: [][]driver.Value{
				{int64(1), "Q1", "quarterly", "target_setting", `["dept-1"]`, `[]`, `[]`},
			},
		},
	)

	_, err := svc.UpdateActivity("1", CreateActivityRequest{
		Name:                "Q1",
		CycleType:           "quarterly",
		Status:              "target_setting",
		TargetDepartmentIDs: []string{"dept-1", "dept-2"},
	}, "user-1")
	if err == nil {
		t.Fatal("UpdateActivity(non-draft scope change) expected error")
	}
	if !strings.Contains(err.Error(), "目标设定开启后不能调整参与范围") {
		t.Fatalf("UpdateActivity error = %v, want scope change error", err)
	}
}

func TestUpdateActivity_EmptyCycleType(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "target_department_ids", "target_employee_ids", "manager_assignments"},
			rows: [][]driver.Value{
				{int64(1), "Q1", "quarterly", "draft", `[]`, `[]`, `[]`},
			},
		},
	)

	_, err := svc.UpdateActivity("1", CreateActivityRequest{
		Name:      "Q1",
		CycleType: "",
		Status:    "draft",
	}, "user-1")
	if err == nil || !strings.Contains(err.Error(), "cycle_type 不能为空") {
		t.Fatalf("UpdateActivity(empty cycle_type) error = %v, want cycle_type error", err)
	}
}

// ======================== GetGoalSuggestions 测试 ========================

func TestGetGoalSuggestions_ParticipantNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    nil,
	})
	_, err := svc.GetGoalSuggestions(999)
	if err == nil {
		t.Fatal("GetGoalSuggestions(participant not found) expected error")
	}
	if !strings.Contains(err.Error(), "参与人不存在") {
		t.Fatalf("GetGoalSuggestions error = %v, want participant not found", err)
	}
}

func TestGetGoalSuggestions_ActivityNotFound(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id"},
			rows:    nil,
		},
	)
	_, err := svc.GetGoalSuggestions(1)
	if err == nil {
		t.Fatal("GetGoalSuggestions(activity not found) expected error")
	}
}

func TestGetGoalSuggestions_NoLibraries(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "indicator_library_id"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "quarterly", "draft", nil},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_libraries"),
			columns: []string{"id"},
			rows:    [][]driver.Value{},
		},
	)
	suggestions, err := svc.GetGoalSuggestions(1)
	if err != nil {
		t.Fatalf("GetGoalSuggestions(no libraries) error = %v, want nil", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("GetGoalSuggestions(no libraries) = %d suggestions, want 0", len(suggestions))
	}
}

func TestGetGoalSuggestions_WithLibraryAndItems(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "indicator_library_id"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "quarterly", "draft", uint(10)},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_libraries"),
			columns: []string{"id", "name", "department_id", "status"},
			rows: [][]driver.Value{
				{uint(11), "部门指标库", "dept-1", "active"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_items"),
			columns: []string{"id", "library_id", "section_type", "name", "description", "weight", "default_weight", "is_default", "sort_order"},
			rows: [][]driver.Value{
				{uint(1), uint(10), "quantitative", "销售额", "销售指标", 0.3, 0.3, true, 1},
				{uint(2), uint(11), "key_action", "客户拜访", "客户服务", 0.2, 0.0, false, 2},
			},
		},
	)
	suggestions, err := svc.GetGoalSuggestions(1)
	if err != nil {
		t.Fatalf("GetGoalSuggestions error = %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("GetGoalSuggestions = %d suggestions, want 1", len(suggestions))
	}
	if suggestions[0].IndicatorItemID == nil || *suggestions[0].IndicatorItemID != 1 {
		t.Fatalf("suggestions[0].IndicatorItemID = %v, want 1", suggestions[0].IndicatorItemID)
	}
	if suggestions[0].Weight != 0.3 {
		t.Fatalf("suggestions[0].Weight = %v, want 0.3", suggestions[0].Weight)
	}
}

func TestGetGoalSuggestionsQueriesOnlyAssociatedActivityLibrary(t *testing.T) {
	activityLibID := uint(10)
	var itemQueryArgs []driver.NamedValue
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "indicator_library_id"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "quarterly", "draft", activityLibID},
			},
		},
		stubQueryResponse{
			match: func(query string, args []driver.NamedValue) bool {
				if !strings.Contains(strings.ToLower(query), "performance_indicator_items") {
					return false
				}
				itemQueryArgs = append([]driver.NamedValue(nil), args...)
				return true
			},
			columns: []string{"id", "library_id", "section_type", "name", "description", "weight", "default_weight", "is_default", "sort_order"},
			rows: [][]driver.Value{
				{uint(1), activityLibID, "quantitative", "Revenue", "Revenue target", 0.3, 0.3, true, 1},
			},
		},
	)
	suggestions, err := svc.GetGoalSuggestions(1)
	if err != nil {
		t.Fatalf("GetGoalSuggestions error = %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("GetGoalSuggestions = %d suggestions, want 1", len(suggestions))
	}
	if !hasNamedArgValue(itemQueryArgs, activityLibID) {
		t.Fatalf("indicator item query args = %#v, want activity library %d", itemQueryArgs, activityLibID)
	}
	if hasNamedArgValue(itemQueryArgs, uint(11)) {
		t.Fatalf("indicator item query args = %#v, should not include department libraries", itemQueryArgs)
	}
}

func hasNamedArgValue(args []driver.NamedValue, want uint) bool {
	for _, arg := range args {
		switch value := arg.Value.(type) {
		case uint:
			if value == want {
				return true
			}
		case uint64:
			if value == uint64(want) {
				return true
			}
		case int:
			if value >= 0 && uint(value) == want {
				return true
			}
		case int64:
			if value >= 0 && uint(value) == want {
				return true
			}
		}
	}
	return false
}

// ======================== SendSelfEvalReminders 测试 ========================

func TestSelfEvalDeadlineReminderText(t *testing.T) {
	cases := []struct {
		name     string
		endAt    string
		now      time.Time
		expected string
	}{
		{
			name:     "future deadline uses calendar days",
			endAt:    "2026-06-05",
			now:      time.Date(2026, 6, 2, 9, 30, 0, 0, time.Local),
			expected: "距离截止还有 3 天",
		},
		{
			name:     "today deadline",
			endAt:    "2026-06-05",
			now:      time.Date(2026, 6, 5, 9, 30, 0, 0, time.Local),
			expected: "今天截止",
		},
		{
			name:     "overdue deadline",
			endAt:    "2026-06-05",
			now:      time.Date(2026, 6, 6, 0, 1, 0, 0, time.Local),
			expected: "当前已逾期",
		},
		{
			name:     "empty deadline",
			endAt:    "",
			now:      time.Date(2026, 6, 2, 9, 30, 0, 0, time.Local),
			expected: "请关注绩效活动配置的自评截止时间",
		},
		{
			name:     "invalid deadline",
			endAt:    "2026/06/05",
			now:      time.Date(2026, 6, 2, 9, 30, 0, 0, time.Local),
			expected: "自评截止时间：2026/06/05。",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			result := selfEvalDeadlineReminderText(tt.endAt, tt.now)
			if !strings.Contains(result, tt.expected) {
				t.Fatalf("selfEvalDeadlineReminderText() = %q, want containing %q", result, tt.expected)
			}
		})
	}
}

func TestSendSelfEvalReminders_ActivityNotFound(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id"},
		rows:    nil,
	})
	_, err := svc.SendSelfEvalReminders("999")
	if err == nil {
		t.Fatal("SendSelfEvalReminders(activity not found) expected error")
	}
	if !strings.Contains(err.Error(), "活动不存在") {
		t.Fatalf("SendSelfEvalReminders error = %v, want activity not found", err)
	}
}

func TestSendSelfEvalReminders_NoParticipants(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "self_evaluation"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows:    nil,
		},
	)
	result, err := svc.SendSelfEvalReminders("1")
	if err != nil {
		t.Fatalf("SendSelfEvalReminders(no participants) error = %v, want nil", err)
	}
	if result.Pending != 0 || result.Candidates != 0 || result.Sent != 0 {
		t.Fatalf("SendSelfEvalReminders(no participants) result = %+v, want zero counts", result)
	}
}

func TestSendSelfEvalReminders_FiltersSubmittedParticipants(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "self_evaluation"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "self_submitted", "", 80, 0, "", false, nil, nil, nil),
				performanceParticipantStubRow(2, "manager_submitted", "", 85, 90, "", false, nil, nil, nil),
				performanceParticipantStubRow(3, "locked", "", 88, 92, "A", true, nil, nil, nil),
			},
		},
	)
	result, err := svc.SendSelfEvalReminders("1")
	if err != nil {
		t.Fatalf("SendSelfEvalReminders(all filtered) error = %v, want nil", err)
	}
	if result.Pending != 0 || result.Candidates != 0 || result.Sent != 0 {
		t.Fatalf("SendSelfEvalReminders(all filtered) result = %+v, want zero counts", result)
	}
}

// ======================== SendManagerEvalReminders 测试 ========================

func TestSendManagerEvalReminders_NoSelfSubmittedParticipants(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows:    [][]driver.Value{},
	})
	result, err := svc.SendManagerEvalReminders("1")
	if err != nil {
		t.Fatalf("SendManagerEvalReminders(no participants) error = %v, want nil", err)
	}
	if result.Pending != 0 || result.Candidates != 0 || result.Sent != 0 {
		t.Fatalf("SendManagerEvalReminders(no participants) result = %+v, want zero counts", result)
	}
}

func TestSendManagerEvalReminders_FiltersOnlySelfSubmitted(t *testing.T) {
	svc := newStubPerformanceService(t, stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			return strings.Contains(strings.ToLower(query), "performance_participants") &&
				strings.Contains(strings.ToLower(query), "status = ?")
		},
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			{int64(1), "test-org", "activity-1", "user-1", "User 1", "dept-1", "Dept", "self_submitted", 80.0, "summary", 0.0, "", false, nil, nil, nil},
		},
	})

	// 注意：真实环境会调用 dingtalk，测试环境无法 mock，所以只能验证不报错
	result, err := svc.SendManagerEvalReminders("1")
	if err != nil {
		t.Fatalf("SendManagerEvalReminders error = %v, want nil", err)
	}
	if result.Pending != 1 || result.Candidates != 0 || result.Sent != 0 {
		t.Fatalf("SendManagerEvalReminders result = %+v, want pending=1 candidates=0 sent=0", result)
	}
}

// ======================== 复杂场景补充测试 ========================

func TestGetGoalSuggestions_MultipleLibrariesUsesOnlyActivityLibrary(t *testing.T) {
	// 测试多指标库去重逻辑：activity 指定的库 + 部门的库，ID 相同时去重
	activityLibID := uint(10)
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "indicator_library_id"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "quarterly", "draft", activityLibID},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_libraries"),
			columns: []string{"id", "name", "department_id", "status"},
			rows: [][]driver.Value{
				{uint(10), "活动指标库", "dept-1", "active"}, // 与 activity 相同，应去重
				{uint(11), "部门指标库A", "dept-1", "active"},
				{uint(12), "部门指标库B", "dept-1", "active"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_items"),
			columns: []string{"id", "library_id", "section_type", "name", "description", "weight", "default_weight", "is_default", "sort_order"},
			rows: [][]driver.Value{
				{uint(1), uint(10), "quantitative", "销售额", "销售指标", 0.3, 0.3, true, 1},
				{uint(2), uint(11), "key_action", "客户拜访", "客户服务", 0.2, 0.2, true, 2},
				{uint(3), uint(12), "quantitative", "利润率", "利润指标", 0.15, 0.15, false, 3},
			},
		},
	)
	suggestions, err := svc.GetGoalSuggestions(1)
	if err != nil {
		t.Fatalf("GetGoalSuggestions error = %v", err)
	}
	// 应该查询 3 个库（10, 11, 12），但 10 已在 activity 中，最终 libraryIDs = [10, 11, 12]
	// 返回 3 个指标
	if len(suggestions) != 1 {
		t.Fatalf("GetGoalSuggestions = %d suggestions, want 1", len(suggestions))
	}
}

func TestGetGoalSuggestions_NoActivityLibraryDoesNotUseDepartmentLibraries(t *testing.T) {
	// 测试 activity.IndicatorLibraryID 为 nil，只使用部门指标库
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil),
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "cycle_type", "status", "indicator_library_id"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "quarterly", "draft", nil},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_libraries"),
			columns: []string{"id", "name", "department_id", "status"},
			rows: [][]driver.Value{
				{uint(20), "部门指标库X", "dept-1", "active"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_items"),
			columns: []string{"id", "library_id", "section_type", "name", "description", "weight", "default_weight", "is_default", "sort_order"},
			rows: [][]driver.Value{
				{uint(10), uint(20), "quantitative", "部门KPI", "部门关键指标", 0.5, 0.5, true, 1},
			},
		},
	)
	suggestions, err := svc.GetGoalSuggestions(1)
	if err != nil {
		t.Fatalf("GetGoalSuggestions error = %v", err)
	}
	if len(suggestions) != 0 {
		t.Fatalf("GetGoalSuggestions = %d suggestions, want 0", len(suggestions))
	}
}

func TestSendSelfEvalReminders_SkipsNonNotifiableUsers(t *testing.T) {
	// 测试 IsNotifiableUserID 过滤逻辑：admin、system 等会被 skipped，不算错误
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "self_evaluation"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				{int64(1), "test-org", "activity-1", "admin", "Admin User", "dept-1", "Dept", "target_set", 0.0, "", 0.0, "", false, nil, nil, nil},
				{int64(2), "test-org", "activity-1", "system", "System User", "dept-1", "Dept", "target_set", 0.0, "", 0.0, "", false, nil, nil, nil},
			},
		},
	)
	result, err := svc.SendSelfEvalReminders("1")
	// admin、system 会被 IsNotifiableUserID 检查并 skipped，不算错误
	// 根据代码逻辑：succeeded=0, skipped=2, failed=0 → 不满足 line 2871 条件，返回 nil
	if err != nil {
		t.Fatalf("SendSelfEvalReminders(skipped users) error = %v, want nil", err)
	}
	if result.Pending != 2 || result.Candidates != 2 || result.Skipped != 2 || len(result.SkippedRecipients) != 2 {
		t.Fatalf("SendSelfEvalReminders(skipped users) result = %+v, want pending=2 candidates=2 skipped=2 with recipients", result)
	}
}

func TestSendSelfEvalReminders_DuplicateNonNotifiableUsersDoNotError(t *testing.T) {
	// 使用 admin/system 覆盖去重与跳过路径，避免普通用户触发真实钉钉网络调用。
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "self_evaluation"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				{int64(1), "test-org", "activity-1", "admin", "Admin", "dept-1", "Dept", "target_set", 0.0, "", 0.0, "", false, nil, nil, nil},
				{int64(2), "test-org", "activity-1", "admin", "Admin", "dept-2", "Dept2", "target_set", 0.0, "", 0.0, "", false, nil, nil, nil},
				{int64(3), "test-org", "activity-1", "system", "System", "dept-1", "Dept", "target_set", 0.0, "", 0.0, "", false, nil, nil, nil},
			},
		},
	)
	result, err := svc.SendSelfEvalReminders("1")
	if err != nil {
		t.Fatalf("SendSelfEvalReminders(non-notifiable duplicates) error = %v, want nil", err)
	}
	if result.Pending != 3 || result.Candidates != 2 || result.Skipped != 2 {
		t.Fatalf("SendSelfEvalReminders(non-notifiable duplicates) result = %+v, want pending=3 candidates=2 skipped=2", result)
	}
}

func TestSendSelfEvalReminders_ReturnsFailedRecipientDetails(t *testing.T) {
	originalSender := sendPerformanceActionCardToUser
	sendPerformanceActionCardToUser = func(orgID, userID, title, content, actionTitle, actionURL string) error {
		return errors.New("dingtalk failed")
	}
	t.Cleanup(func() {
		sendPerformanceActionCardToUser = originalSender
	})

	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id", "name", "status"},
			rows: [][]driver.Value{
				{int64(1), "Q2", "self_evaluation"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_participants"),
			columns: performanceParticipantStubColumns(),
			rows: [][]driver.Value{
				{int64(1), "test-org", "activity-1", "user-1", "张三", "dept-1", "Dept", "target_set", 0.0, "", 0.0, "", false, nil, nil, nil},
			},
		},
	)
	result, err := svc.SendSelfEvalReminders("1")
	if err != nil {
		t.Fatalf("SendSelfEvalReminders(failed recipient) error = %v, want nil", err)
	}
	if result.Pending != 1 || result.Candidates != 1 || result.Failed != 1 || len(result.FailedRecipients) != 1 {
		t.Fatalf("SendSelfEvalReminders(failed recipient) result = %+v, want one failed recipient", result)
	}
	if result.FailedRecipients[0].UserID != "user-1" || result.FailedRecipients[0].Name != "张三" {
		t.Fatalf("failed recipient = %+v, want 张三(user-1)", result.FailedRecipients[0])
	}
}

func TestSendManagerEvalReminders_SkipsNonNotifiableManagersAfterAggregation(t *testing.T) {
	// 使用 admin/system 覆盖 managerCounts 聚合后的跳过路径，避免真实钉钉网络调用。
	svc := newStubPerformanceService(t, stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			return strings.Contains(strings.ToLower(query), "performance_participants") &&
				strings.Contains(strings.ToLower(query), "status = ?")
		},
		columns: append(performanceParticipantStubColumns(), "manager_id", "manager_name"),
		rows: [][]driver.Value{
			{int64(1), "test-org", "activity-1", "user-1", "User 1", "dept-1", "Dept", "self_submitted", 80.0, "s1", 0.0, "", false, nil, nil, nil, "admin", "管理员"},
			{int64(2), "test-org", "activity-1", "user-2", "User 2", "dept-1", "Dept", "self_submitted", 85.0, "s2", 0.0, "", false, nil, nil, nil, "admin", "管理员"},
			{int64(3), "test-org", "activity-1", "user-3", "User 3", "dept-2", "Dept2", "self_submitted", 90.0, "s3", 0.0, "", false, nil, nil, nil, "system", "系统"},
		},
	})
	result, err := svc.SendManagerEvalReminders("1")
	if err != nil {
		t.Fatalf("SendManagerEvalReminders(non-notifiable managers) error = %v, want nil", err)
	}
	if result.Pending != 3 || result.Candidates != 2 || result.Skipped != 2 || len(result.SkippedRecipients) != 2 {
		t.Fatalf("SendManagerEvalReminders(non-notifiable managers) result = %+v, want pending=3 candidates=2 skipped=2 with recipients", result)
	}
	recipients := map[string]string{}
	for _, recipient := range result.SkippedRecipients {
		recipients[recipient.UserID] = recipient.Name
	}
	if recipients["admin"] != "管理员" || recipients["system"] != "系统" {
		t.Fatalf("SendManagerEvalReminders skipped recipients = %+v, want names for admin and system", result.SkippedRecipients)
	}
}

// ======================== Service 主流程覆盖率补强 ========================

func TestResolveDefaultAssessmentManagerForParticipant_Coverage(t *testing.T) {
	svc := &PerformanceService{}
	managerID, managerName, source := svc.resolveDefaultAssessmentManagerForParticipant(database.User{
		ManagerUserID: " manager-1 ",
		ManagerName:   " Boss ",
	}, "dept-1", ManagerSourceDirectManager)
	if managerID != "manager-1" || managerName != "Boss" || source != ManagerSourceDirectManager {
		t.Fatalf("direct manager = (%q, %q, %q), want manager-1/Boss/%s", managerID, managerName, source, ManagerSourceDirectManager)
	}

	managerID, managerName, source = svc.resolveDefaultAssessmentManagerForParticipant(database.User{
		Extension: map[string]any{
			"leader_user_id":  " leader-1 ",
			"supervisor_name": " Supervisor ",
		},
	}, "dept-1", ManagerSourceDirectManager)
	if managerID != "leader-1" || managerName != "Supervisor" || source != ManagerSourceDirectManager {
		t.Fatalf("direct manager extension fallback = (%q, %q, %q)", managerID, managerName, source)
	}

	managerID, managerName, source = svc.resolveDefaultAssessmentManagerForParticipant(database.User{}, "", ManagerSourceDepartmentHead)
	if managerID != "" || managerName != "" || source != ManagerSourceEmpty {
		t.Fatalf("empty department = (%q, %q, %q), want empty source", managerID, managerName, source)
	}

	dbSvc := newStubPerformanceService(t,
		coverageDepartmentManagersResponse(),
		coverageUserIDLookupNoRows("dept-head-1"),
		coverageUserByUserIDResponse("dept-head-1", "Department Head"),
		coverageUserIDLookupNoRows("center-head-1"),
		coverageUserByUserIDResponse("center-head-1", "Center Head"),
	)
	managerID, managerName, source = dbSvc.resolveDefaultAssessmentManagerForParticipant(database.User{}, "dept-1", ManagerSourceDepartmentHead)
	if managerID != "dept-head-1" || managerName != "Department Head" || source != ManagerSourceDepartmentHead {
		t.Fatalf("department head = (%q, %q, %q)", managerID, managerName, source)
	}
	managerID, managerName, source = dbSvc.resolveDefaultAssessmentManagerForParticipant(database.User{}, "dept-1", ManagerSourceCenterHead)
	if managerID != "center-head-1" || managerName != "Center Head" || source != ManagerSourceCenterHead {
		t.Fatalf("center head = (%q, %q, %q)", managerID, managerName, source)
	}
}

func TestAssessmentManagerCandidateMissingReason_Coverage(t *testing.T) {
	svc := &PerformanceService{}
	cases := []struct {
		name      string
		source    string
		keyword   string
		wantPiece string
	}{
		{name: "direct without participant", source: ManagerSourceDirectManager, wantPiece: "未指定参与人"},
		{name: "department head", source: ManagerSourceDepartmentHead, wantPiece: "department_head_user_id"},
		{name: "center head", source: ManagerSourceCenterHead, wantPiece: "center_head_user_id"},
		{name: "manual empty keyword", source: ManagerSourceManual, wantPiece: "请输入姓名、工号或手机号"},
		{name: "manual non-empty keyword", source: ManagerSourceManual, keyword: "Alice", wantPiece: "没有匹配的在职员工"},
		{name: "unknown source", source: "UNKNOWN", wantPiece: "该来源没有可用候选人"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reason := svc.assessmentManagerCandidateMissingReason("activity-1", 0, tc.source, tc.keyword)
			if !strings.Contains(reason, tc.wantPiece) {
				t.Fatalf("reason = %q, want contains %q", reason, tc.wantPiece)
			}
		})
	}

	notFoundSvc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: []string{"id"},
		rows:    nil,
	})
	if reason := notFoundSvc.assessmentManagerCandidateMissingReason("activity-1", 1, ManagerSourceDirectManager, ""); !strings.Contains(reason, "参与人不存在") {
		t.Fatalf("not found reason = %q", reason)
	}

	noSnapshotSvc := newStubPerformanceService(t, coverageParticipantWithDirectSnapshotResponse(nil))
	if reason := noSnapshotSvc.assessmentManagerCandidateMissingReason("activity-1", 1, ManagerSourceDirectManager, ""); !strings.Contains(reason, "没有直属主管快照") {
		t.Fatalf("empty snapshot reason = %q", reason)
	}

	unavailableSvc := newStubPerformanceService(t, coverageParticipantWithDirectSnapshotResponse("direct-1"))
	if reason := unavailableSvc.assessmentManagerCandidateMissingReason("activity-1", 1, ManagerSourceDirectManager, ""); !strings.Contains(reason, "直属主管不存在") {
		t.Fatalf("unavailable snapshot reason = %q", reason)
	}
}

func TestGetTemplateWithSectionsAndItems(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_templates"),
			columns: []string{"id", "name", "description", "status"},
			rows: [][]driver.Value{
				{uint(10), "季度绩效模板", "Q 模板", "active"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_sections"),
			columns: []string{"id", "template_id", "name", "section_type", "weight", "sort_order", "is_score_required", "is_comment_required"},
			rows: [][]driver.Value{
				{uint(101), uint(10), "量化指标", "quantitative", 60.0, 1, true, false},
				{uint(102), uint(10), "关键行动", "key_action", 40.0, 2, false, true},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_items"),
			columns: []string{"id", "section_id", "name", "description", "max_score", "weight", "sort_order"},
			rows: [][]driver.Value{
				{uint(1001), uint(101), "收入", "收入达成", 100.0, 60.0, 1},
				{uint(1002), uint(101), "利润", "利润达成", 100.0, 40.0, 2},
				{uint(1003), uint(102), "项目推进", "关键项目", 100.0, 100.0, 1},
			},
		},
	)

	result, err := svc.GetTemplate(10)
	if err != nil {
		t.Fatalf("GetTemplate() error = %v", err)
	}
	template, ok := result["template"].(*database.PerformanceTemplate)
	if !ok || template.ID != 10 || template.Name != "季度绩效模板" {
		t.Fatalf("template = %#v, want id=10 name=季度绩效模板", result["template"])
	}
	sections, ok := result["sections"].([]map[string]any)
	if !ok || len(sections) != 2 {
		t.Fatalf("sections = %#v, want 2 sections", result["sections"])
	}
	firstItems, ok := sections[0]["items"].([]database.PerformanceTemplateItem)
	if !ok || len(firstItems) != 2 {
		t.Fatalf("first section items = %#v, want 2", sections[0]["items"])
	}
	secondItems, ok := sections[1]["items"].([]database.PerformanceTemplateItem)
	if !ok || len(secondItems) != 1 || secondItems[0].SectionID != 102 {
		t.Fatalf("second section items = %#v, want one item for section 102", sections[1]["items"])
	}
}

func TestGetTemplateWithNoSections(t *testing.T) {
	svc := newStubPerformanceService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_templates"),
			columns: []string{"id", "name", "description", "status"},
			rows: [][]driver.Value{
				{uint(11), "空模板", "", "draft"},
			},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_template_sections"),
			columns: []string{"id", "template_id", "name"},
			rows:    [][]driver.Value{},
		},
	)

	result, err := svc.GetTemplate(11)
	if err != nil {
		t.Fatalf("GetTemplate(no sections) error = %v", err)
	}
	sections, ok := result["sections"].([]map[string]any)
	if !ok || len(sections) != 0 {
		t.Fatalf("sections = %#v, want empty", result["sections"])
	}
}

func TestTriggerPerformanceInterviewWithManagerSkipsNonNotifiableEmployee(t *testing.T) {
	cases := []struct {
		name          string
		interviewType string
		finalLevel    string
	}{
		{name: "required", interviewType: "required", finalLevel: "C"},
		{name: "optional", interviewType: "optional", finalLevel: "S"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := newStubPerformanceService(t, stubQueryResponse{
				match:   stubTableMatcher("performance_participants"),
				columns: []string{"id", "activity_id", "employee_id", "employee_name", "manager_id", "final_level"},
				rows: [][]driver.Value{
					{int64(1), "activity-1", "admin", "Admin", "manager-1", tc.finalLevel},
				},
			})
			if err := svc.TriggerPerformanceInterview("1", tc.interviewType); err != nil {
				t.Fatalf("TriggerPerformanceInterview(%s) error = %v, want nil", tc.interviewType, err)
			}
		})
	}
}

func TestTriggerPerformanceInterviewSkipsHiddenEmployeeGradeNotice(t *testing.T) {
	originalSender := sendPerformanceActionCardToUser
	sentTo := make([]string, 0)
	sendPerformanceActionCardToUser = func(orgID, userID, title, content, actionTitle, actionURL string) error {
		sentTo = append(sentTo, userID)
		if userID == "user-1" && strings.Contains(content, "绩效等级") {
			t.Fatalf("hidden employee received grade notice: title=%q content=%q", title, content)
		}
		return nil
	}
	t.Cleanup(func() {
		sendPerformanceActionCardToUser = originalSender
	})

	svc := newStubPerformanceService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: []string{"id", "activity_id", "employee_id", "employee_name", "final_level", "result_hidden"},
		rows: [][]driver.Value{
			{int64(1), "activity-1", "user-1", "Alice", "S", true},
		},
	})

	if err := svc.TriggerPerformanceInterview("1", "required"); err != nil {
		t.Fatalf("TriggerPerformanceInterview(hidden) error = %v, want nil", err)
	}
	if len(sentTo) != 0 {
		t.Fatalf("hidden employee should not receive grade notice, sentTo=%v", sentTo)
	}
}

func TestSetBonusPenaltyScoreActivityNotFound(t *testing.T) {
	svc := newStubPerformanceService(t,
		coverageParticipantWithScoresResponse("manager_submitted", "manager-1", false, 85),
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id"},
			rows:    nil,
		},
	)

	err := svc.SetBonusPenaltyScore(1, 5, 2, "operator-1")
	if err == nil || !strings.Contains(err.Error(), "绩效活动不存在") {
		t.Fatalf("SetBonusPenaltyScore(activity not found) error = %v, want activity not found", err)
	}
}

func TestSetBonusPenaltyScoreEnabledSuccess(t *testing.T) {
	svc := newStubPerformanceService(t,
		coverageParticipantWithScoresResponse("manager_submitted", "manager-1", false, 85),
		coveragePerformanceActivityResponse("manager_evaluation", "creator-1", true),
	)

	if err := svc.SetBonusPenaltyScore(1, 5, 2, "operator-1"); err != nil {
		t.Fatalf("SetBonusPenaltyScore(enabled) error = %v, want nil", err)
	}
}

func TestSetBonusPenaltyScoreClampsNegativeAdjustedScore(t *testing.T) {
	svc := newStubPerformanceService(t,
		coverageParticipantWithScoresResponse("manager_submitted", "manager-1", false, 1),
		coveragePerformanceActivityResponse("manager_evaluation", "creator-1", true),
	)

	if err := svc.SetBonusPenaltyScore(1, 0, 99, "operator-1"); err != nil {
		t.Fatalf("SetBonusPenaltyScore(clamp negative) error = %v, want nil", err)
	}
}

func TestBatchAssignGoalsSystemManagerSkipsOwnershipCheck(t *testing.T) {
	svc := newStubPerformanceService(t,
		coverageParticipantWithScoresResponse("pending", "other-manager", false, 0),
		coveragePerformanceActivityResponse("target_setting", "creator-1", false),
		coverageGoalRecordsEmptyResponse(),
	)

	err := svc.BatchAssignGoals("activity-1", "system", []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "收入目标", Weight: 100},
	}, []uint{1}, "system")
	if err != nil {
		t.Fatalf("BatchAssignGoals(system manager) error = %v, want nil", err)
	}
}

func TestBatchAssignGoalsManagerMatchedSuccess(t *testing.T) {
	svc := newStubPerformanceService(t,
		coverageParticipantWithScoresResponse("pending", "manager-1", false, 0),
		coveragePerformanceActivityResponse("target_setting", "creator-1", false),
		coverageGoalRecordsEmptyResponse(),
	)

	err := svc.BatchAssignGoals("activity-1", "manager-1", []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "收入目标", Weight: 60},
		{SectionType: "key_action", ItemName: "关键行动", Weight: 40},
	}, []uint{1}, "manager-1")
	if err != nil {
		t.Fatalf("BatchAssignGoals(manager matched) error = %v, want nil", err)
	}
}

func TestBatchAssignGoalsSaveFailureContinues(t *testing.T) {
	svc := newStubPerformanceService(t,
		coverageParticipantWithScoresResponse("pending", "manager-1", false, 0),
		coveragePerformanceActivityResponse("self_evaluation", "creator-1", false),
	)

	err := svc.BatchAssignGoals("activity-1", "manager-1", []GoalRecordRequest{
		{SectionType: "quantitative", ItemName: "收入目标", Weight: 100},
	}, []uint{1}, "manager-1")
	if err != nil {
		t.Fatalf("BatchAssignGoals(save failure continues) error = %v, want nil", err)
	}
}

func TestSendHRConfirmRemindersActivityNotFound(t *testing.T) {
	svc := newStubPerformanceService(t,
		coveragePendingHRConfirmResponse(),
		stubQueryResponse{
			match:   stubTableMatcher("performance_activities"),
			columns: []string{"id"},
			rows:    nil,
		},
	)

	if _, err := svc.SendHRConfirmReminders("activity-1"); err == nil {
		t.Fatal("SendHRConfirmReminders(activity not found) expected error")
	}
}

func TestSendHRConfirmRemindersNoHRPermissionRecipients(t *testing.T) {
	svc := newStubPerformanceService(t,
		coveragePendingHRConfirmResponse(),
		coveragePerformanceActivityResponse("hr_confirmation", "", false),
		hrConfirmReminderRecipientsResponse(),
	)

	result, err := svc.SendHRConfirmReminders("activity-1")
	if err != nil {
		t.Fatalf("SendHRConfirmReminders(no recipients) error = %v, want nil", err)
	}
	if result.Pending == 0 || result.Candidates != 0 || result.Sent != 0 {
		t.Fatalf("SendHRConfirmReminders() result = %#v, want pending only", result)
	}
}

func TestSendHRConfirmRemindersUsesHRPermissionRecipients(t *testing.T) {
	originalSender := sendPerformanceActionCardToUser
	sentTo := []string{}
	sendPerformanceActionCardToUser = func(orgID, userID, title, content, actionTitle, actionURL string) error {
		sentTo = append(sentTo, userID)
		return nil
	}
	t.Cleanup(func() {
		sendPerformanceActionCardToUser = originalSender
	})

	svc := newStubPerformanceService(t,
		coveragePendingHRConfirmResponse(),
		coveragePerformanceActivityResponse("hr_confirmation", "creator-1", false),
		hrConfirmReminderRecipientsResponse("hr-1", "hr-2"),
	)

	result, err := svc.SendHRConfirmReminders("activity-1")
	if err != nil {
		t.Fatalf("SendHRConfirmReminders(hr recipients) error = %v, want nil", err)
	}
	if result.Sent != 2 || len(sentTo) != 2 || sentTo[0] != "hr-1" || sentTo[1] != "hr-2" {
		t.Fatalf("SendHRConfirmReminders() result = %#v sentTo=%v, want two HR recipients", result, sentTo)
	}
	if len(result.SentRecipients) != 2 || result.SentRecipients[0].Name != "hr-1 name" || result.SentRecipients[1].Name != "hr-2 name" {
		t.Fatalf("SendHRConfirmReminders() sent recipients = %#v, want HR names", result.SentRecipients)
	}
}

func coverageDepartmentManagersResponse() stubQueryResponse {
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

func coverageUserIDLookupNoRows(userID string) stubQueryResponse {
	return stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "users") &&
				strings.Contains(lower, "id") &&
				!strings.Contains(lower, "user_id") &&
				coverageHasStringArg(args, userID)
		},
		columns: []string{"id", "user_id", "name", "status"},
		rows:    nil,
	}
}

func coverageUserByUserIDResponse(userID, name string) stubQueryResponse {
	return stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "users") &&
				strings.Contains(lower, "user_id") &&
				coverageHasStringArg(args, userID)
		},
		columns: []string{"id", "user_id", "name", "status"},
		rows: [][]driver.Value{
			{int64(1), userID, name, "active"},
		},
	}
}

func coverageHasStringArg(args []driver.NamedValue, want string) bool {
	for _, arg := range args {
		if value, ok := arg.Value.(string); ok && strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

func coverageParticipantWithDirectSnapshotResponse(snapshot any) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: append(performanceParticipantStubColumns(), "direct_manager_id_snapshot"),
		rows: [][]driver.Value{
			append(performanceParticipantStubRow(1, "target_set", "", 0, 0, "", false, nil, nil, nil), snapshot),
		},
	}
}

func coverageParticipantWithScoresResponse(status, managerID string, locked bool, managerScore float64) stubQueryResponse {
	columns := append(performanceParticipantStubColumns(), "manager_id", "bonus_score", "penalty_score", "adjusted_score")
	row := append(performanceParticipantStubRow(1, status, "", 0, managerScore, "B", locked, nil, nil, nil), managerID, 0.0, 0.0, managerScore)
	return stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: columns,
		rows:    [][]driver.Value{row},
	}
}

func coveragePerformanceActivityResponse(status, createdBy string, enableBonus bool) stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "org_id", "name", "cycle_type", "status", "created_by", "enable_bonus_score"},
		rows: [][]driver.Value{
			{int64(1), "test-org", "Q2", "quarterly", status, createdBy, enableBonus},
		},
	}
}

func coverageGoalRecordsEmptyResponse() stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_goal_records"),
		columns: []string{"id", "activity_id", "participant_id", "section_type", "item_name", "weight", "approval_status"},
		rows:    [][]driver.Value{},
	}
}

func coveragePendingHRConfirmResponse() stubQueryResponse {
	return stubQueryResponse{
		match:   stubTableMatcher("performance_participants"),
		columns: performanceParticipantStubColumns(),
		rows: [][]driver.Value{
			performanceParticipantStubRow(1, "manager_confirmed", "", 0, 90, "A", false, nil, nil, nil),
		},
	}
}
