package service

import (
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// openTargetDraftActivity 返回 draft 活动 stub，可附带目标范围与 org_id。
// target_* 字段以 JSON 字节返回，匹配 gorm serializer:json。
func openTargetDraftActivity(targetEmployees, targetDepartments []string) stubQueryResponse {
	return stubQueryResponse{
		match: stubTableMatcher("performance_activities"),
		columns: []string{
			"id", "name", "cycle_type", "status", "flow_type", "org_id",
			"target_employee_ids", "target_department_ids", "created_by",
		},
		rows: [][]driver.Value{
			{
				int64(1), "Q2", "quarterly", "draft", PerformanceFlowOld, "test-org",
				jsonArrayBytes(targetEmployees), jsonArrayBytes(targetDepartments), "operator-1",
			},
		},
	}
}

func openTargetNewFlowDraftWithPrevious(previousID uint) stubQueryResponse {
	return stubQueryResponse{
		match: stubTableMatcher("performance_activities"),
		columns: []string{
			"id", "name", "cycle_type", "status", "flow_type", "activity_kind", "org_id",
			"target_employee_ids", "target_department_ids", "previous_review_activity_id", "created_by",
		},
		rows: [][]driver.Value{
			{
				int64(1), "Q2 Review", "quarterly", "draft", PerformanceFlowNew, PerformanceActivityKindReviewScoring, "test-org",
				jsonArrayBytes([]string{"user-1"}), jsonArrayBytes(nil), int64(previousID), "operator-1",
			},
		},
	}
}

func jsonArrayBytes(values []string) []byte {
	if len(values) == 0 {
		return []byte("[]")
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		parts = append(parts, fmt.Sprintf("%q", v))
	}
	return []byte("[" + strings.Join(parts, ",") + "]")
}

func openTargetActiveUsers(rows [][]driver.Value) stubQueryResponse {
	return stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			if !strings.Contains(lower, "users") || strings.Contains(lower, "user_roles") {
				return false
			}
			for _, arg := range args {
				if arg.Value == "active" {
					return true
				}
			}
			return strings.Contains(lower, "status")
		},
		columns: []string{"id", "user_id", "name", "department_id", "status", "org_id"},
		rows:    rows,
	}
}

func openTargetCountResponse(count int64) stubQueryResponse {
	return stubQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_participants") && strings.Contains(lower, "count(")
		},
		columns: []string{"count"},
		rows:    [][]driver.Value{{count}},
	}
}

func openTargetParticipantsSelect(rows [][]driver.Value) stubQueryResponse {
	return stubQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_participants") && !strings.Contains(lower, "count(")
		},
		columns: []string{
			"id", "org_id", "activity_id", "employee_id", "employee_name",
			"department_id", "department_name", "status", "employee_status",
		},
		rows: rows,
	}
}

func collectParticipantInserts(stub *stubPerformanceDB) []stubPerformanceCall {
	out := make([]stubPerformanceCall, 0)
	for _, call := range stub.execCalls {
		lower := strings.ToLower(call.query)
		if strings.Contains(lower, "insert") && strings.Contains(lower, "performance_participants") {
			out = append(out, call)
		}
	}
	return out
}

func argsContain(args []driver.NamedValue, want any) bool {
	for _, arg := range args {
		if arg.Value == want {
			return true
		}
		// 字符串比较兜底（driver 可能包装类型）
		if fmt.Sprint(arg.Value) == fmt.Sprint(want) {
			return true
		}
	}
	return false
}

func TestOpenTargetSettingSetsParticipantOrgIDFromActivity(t *testing.T) {
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			openTargetDraftActivity([]string{"user-1"}, nil),
			openTargetActiveUsers([][]driver.Value{
				{int64(1), "user-1", "Alice", "dept-1", "active", "test-org"},
			}),
			performanceDepartmentsResponse([]driver.Value{int64(1), "dept-1", "Product"}),
			openTargetParticipantsSelect(nil),
			openTargetCountResponse(1),
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)

	warnings, err := svc.OpenTargetSetting("1", "operator-1")
	if err != nil {
		t.Fatalf("OpenTargetSetting() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want empty", warnings)
	}
	if stub.beginCalls == 0 {
		t.Fatalf("expected transaction begin")
	}
	if stub.commitCalls == 0 {
		t.Fatalf("expected transaction commit")
	}

	inserts := collectParticipantInserts(stub)
	if len(inserts) == 0 {
		t.Fatalf("expected INSERT into performance_participants; execCalls=%d", len(stub.execCalls))
	}
	// 精确断言：至少一条 INSERT 同时携带 org_id=test-org 与 employee_id=user-1
	matched := false
	for _, call := range inserts {
		if argsContain(call.args, "test-org") && argsContain(call.args, "user-1") {
			matched = true
			break
		}
	}
	if !matched {
		// 打印参数便于排查 GORM 绑定顺序
		for i, call := range inserts {
			vals := make([]any, 0, len(call.args))
			for _, a := range call.args {
				vals = append(vals, a.Value)
			}
			t.Logf("insert[%d] args=%#v sql=%s", i, vals, call.query)
		}
		t.Fatalf("participant INSERT must include org_id=test-org and employee_id=user-1")
	}
}

func TestOpenTargetSettingUnionDepartmentAndEmployeesDedup(t *testing.T) {
	// user-1 既在部门又在指定员工 → 只纳入一次；user-2 仅指定员工；user-3 仅部门；user-4 排除。
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			openTargetDraftActivity([]string{"user-1", "user-2"}, []string{"dept-1"}),
			openTargetActiveUsers([][]driver.Value{
				{int64(1), "user-1", "Alice", "dept-1", "active", "test-org"},
				{int64(2), "user-2", "Bob", "dept-2", "active", "test-org"},
				{int64(3), "user-3", "Carol", "dept-1", "active", "test-org"},
				{int64(4), "user-4", "Dave", "dept-9", "active", "test-org"},
			}),
			performanceDepartmentsResponse(
				[]driver.Value{int64(1), "dept-1", "Product"},
				[]driver.Value{int64(2), "dept-2", "Sales"},
			),
			openTargetParticipantsSelect(nil),
			openTargetCountResponse(3),
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)
	if _, err := svc.OpenTargetSetting("1", "operator-1"); err != nil {
		t.Fatalf("OpenTargetSetting() error = %v", err)
	}

	inserts := collectParticipantInserts(stub)
	// 统计每个 employee_id 的 INSERT 次数
	counts := map[string]int{}
	for _, call := range inserts {
		for _, id := range []string{"user-1", "user-2", "user-3", "user-4"} {
			if argsContain(call.args, id) {
				counts[id]++
			}
		}
	}
	if counts["user-1"] != 1 {
		t.Fatalf("user-1 insert count=%d want 1 (union dedup); inserts=%d counts=%#v", counts["user-1"], len(inserts), counts)
	}
	if counts["user-2"] != 1 {
		t.Fatalf("user-2 insert count=%d want 1; counts=%#v", counts["user-2"], counts)
	}
	if counts["user-3"] != 1 {
		t.Fatalf("user-3 insert count=%d want 1; counts=%#v", counts["user-3"], counts)
	}
	if counts["user-4"] != 0 {
		t.Fatalf("user-4 must not be inserted; counts=%#v", counts)
	}
	if len(inserts) != 3 {
		t.Fatalf("total participant inserts=%d want 3; counts=%#v", len(inserts), counts)
	}
}

func TestOpenTargetSettingPartialInvalidEmployeesReturnsWarning(t *testing.T) {
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			openTargetDraftActivity([]string{"user-1", "ghost-1"}, []string{"dept-1"}),
			openTargetActiveUsers([][]driver.Value{
				{int64(1), "user-1", "Alice", "dept-1", "active", "test-org"},
			}),
			performanceDepartmentsResponse([]driver.Value{int64(1), "dept-1", "Product"}),
			openTargetParticipantsSelect(nil),
			openTargetCountResponse(1),
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)
	warnings, err := svc.OpenTargetSetting("1", "operator-1")
	if err != nil {
		t.Fatalf("OpenTargetSetting() error = %v", err)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected sanitized unavailable-employee warning")
	}
	if !strings.Contains(warnings[0], "不可用、已离职或不属于当前企业") || !strings.Contains(warnings[0], "ghost-1") {
		t.Fatalf("unexpected warning: %v", warnings)
	}
	if stub.rollbackCalls != 0 {
		t.Fatalf("partial invalid should commit, not rollback")
	}
	if stub.commitCalls == 0 {
		t.Fatalf("expected commit")
	}
}

func TestOpenTargetSettingAllInvalidEmployeesHardFailsAndRollsBack(t *testing.T) {
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			openTargetDraftActivity([]string{"ghost-1", "ghost-2"}, nil),
			openTargetActiveUsers(nil),
			performanceDepartmentsResponse(),
			openTargetParticipantsSelect(nil),
			openTargetCountResponse(0),
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)
	_, err := svc.OpenTargetSetting("1", "operator-1")
	if err == nil {
		t.Fatalf("expected hard fail when no participants")
	}
	if !strings.Contains(err.Error(), "不可用、已离职或不属于当前企业") {
		t.Fatalf("error should include unavailable-employee text, got %v", err)
	}
	if stub.beginCalls == 0 {
		t.Fatalf("expected transaction")
	}
	if stub.rollbackCalls == 0 {
		t.Fatalf("expected rollback when total==0")
	}
	if stub.commitCalls != 0 {
		t.Fatalf("must not commit on hard fail")
	}
}

func TestOpenTargetSettingPreviousPlanSyncFailureRollsBack(t *testing.T) {
	// 评分活动强制绑定 previous_review_activity_id；GetByID(previous) 返回错误 → 事务回滚。
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			// 第一次锁活动 / GetByID 当前活动
			openTargetNewFlowDraftWithPrevious(99),
			openTargetActiveUsers([][]driver.Value{
				{int64(1), "user-1", "Alice", "dept-1", "active", "test-org"},
			}),
			performanceDepartmentsResponse([]driver.Value{int64(1), "dept-1", "Product"}),
			openTargetParticipantsSelect(nil),
			// findPreviousPlanActivity → actRepo.GetByID("99") 再查 performance_activities
			{
				match: func(query string, args []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					if !strings.Contains(lower, "performance_activities") {
						return false
					}
					// 匹配按 id 取 previous 的查询
					for _, arg := range args {
						if arg.Value == int64(99) || arg.Value == "99" || fmt.Sprint(arg.Value) == "99" {
							return true
						}
					}
					// 第二次 activities 查询（非首次 draft 行）
					return strings.Contains(lower, "id") && strings.Contains(lower, "limit")
				},
				err: errors.New("previous plan lookup failed"),
			},
		},
	}
	// 用动态计数：第一次 activities 成功返回 draft，后续 activities 强制失败
	activityHits := 0
	orig := stub.queries[0]
	stub.queries[0] = stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			if !stubTableMatcher("performance_activities")(query, args) {
				return false
			}
			activityHits++
			return true
		},
		columns: orig.columns,
		rows:    orig.rows,
		err:     nil,
	}
	// 覆盖后续 activities 查询失败
	stub.queries = append(stub.queries, stubQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			if !stubTableMatcher("performance_activities")(query, args) {
				return false
			}
			// 第 2 次及以后的 activities 查询失败（previous lookup）
			return activityHits >= 2
		},
		err: errors.New("previous plan sync failed"),
	})

	svc := newStubPerformanceServiceWithDB(t, stub)
	_, err := svc.OpenTargetSetting("1", "operator-1")
	if err == nil {
		t.Fatalf("expected previous-plan sync failure; begin=%d commit=%d rollback=%d activityHits=%d",
			stub.beginCalls, stub.commitCalls, stub.rollbackCalls, activityHits)
	}
	if stub.beginCalls == 0 {
		t.Fatalf("expected transaction begin")
	}
	if stub.rollbackCalls == 0 {
		t.Fatalf("expected rollback after previous-plan failure; commit=%d err=%v", stub.commitCalls, err)
	}
	if stub.commitCalls != 0 {
		t.Fatalf("must not commit after previous-plan failure")
	}
}

func TestOpenTargetSettingStatusUpdateFailureRollsBack(t *testing.T) {
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			openTargetDraftActivity([]string{"user-1"}, nil),
			openTargetActiveUsers([][]driver.Value{
				{int64(1), "user-1", "Alice", "dept-1", "active", "test-org"},
			}),
			performanceDepartmentsResponse([]driver.Value{int64(1), "dept-1", "Product"}),
			openTargetParticipantsSelect(nil),
			openTargetCountResponse(1),
		},
		execs: []stubExecResponse{
			{
				match: func(query string, args []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					if !strings.Contains(lower, "performance_activities") {
						return false
					}
					if !strings.Contains(lower, "update") {
						return false
					}
					for _, arg := range args {
						if arg.Value == "target_setting" {
							return true
						}
					}
					return strings.Contains(lower, "target_setting")
				},
				err: errors.New("status update failed"),
			},
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)
	_, err := svc.OpenTargetSetting("1", "operator-1")
	if err == nil {
		t.Fatalf("expected status update failure")
	}
	if stub.rollbackCalls == 0 {
		t.Fatalf("expected rollback when status update fails; begin=%d commit=%d", stub.beginCalls, stub.commitCalls)
	}
	if stub.commitCalls != 0 {
		t.Fatalf("must not commit after status update failure")
	}
}

func TestOpenTargetSettingFixesExistingWrongOrgID(t *testing.T) {
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			openTargetDraftActivity([]string{"user-1"}, nil),
			openTargetActiveUsers([][]driver.Value{
				{int64(1), "user-1", "Alice", "dept-1", "active", "test-org"},
			}),
			performanceDepartmentsResponse([]driver.Value{int64(1), "dept-1", "Product"}),
			openTargetParticipantsSelect([][]driver.Value{
				{int64(10), "default", "1", "user-1", "Alice", "dept-1", "Product", "pending", "active"},
			}),
			openTargetCountResponse(1),
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)
	if _, err := svc.OpenTargetSetting("1", "operator-1"); err != nil {
		t.Fatalf("OpenTargetSetting() error = %v", err)
	}

	// 必须出现更新参与人 id=10 且 org_id=test-org 的 SAVE/UPDATE
	foundFix := false
	for _, call := range stub.execCalls {
		lower := strings.ToLower(call.query)
		if !strings.Contains(lower, "performance_participants") {
			continue
		}
		if !strings.Contains(lower, "update") && !strings.Contains(lower, "set") {
			continue
		}
		// 参数中同时出现主键 10 与修复后的 org
		if (argsContain(call.args, int64(10)) || argsContain(call.args, 10) || argsContain(call.args, "10")) &&
			argsContain(call.args, "test-org") {
			foundFix = true
			break
		}
	}
	if !foundFix {
		for i, call := range stub.execCalls {
			vals := make([]any, 0, len(call.args))
			for _, a := range call.args {
				vals = append(vals, a.Value)
			}
			t.Logf("exec[%d] sql=%s args=%#v", i, call.query, vals)
		}
		t.Fatalf("expected UPDATE of participant id=10 with org_id=test-org")
	}
}

func TestOpenTargetSettingNeverIncludesOtherOrgUsers(t *testing.T) {
	activity := openTargetDraftActivity([]string{"other-org-user"}, nil)
	_ = activity
	// 指定跨企业员工：active 用户列表不含该 ID → 视为不可用
	active := map[string]struct{}{}
	// 通过 collectUnavailableTargetEmployeeIDs 语义验证
	svcStub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			openTargetDraftActivity([]string{"other-org-user"}, nil),
			openTargetActiveUsers(nil), // 当前企业无此人
			performanceDepartmentsResponse(),
			openTargetParticipantsSelect(nil),
			openTargetCountResponse(0),
		},
	}
	svc := newStubPerformanceServiceWithDB(t, svcStub)
	_, err := svc.OpenTargetSetting("1", "operator-1")
	if err == nil {
		t.Fatalf("cross-org only target must hard-fail")
	}
	if !strings.Contains(err.Error(), "不可用、已离职或不属于当前企业") {
		t.Fatalf("error = %v", err)
	}
	if len(collectParticipantInserts(svcStub)) != 0 {
		t.Fatalf("must not insert cross-org participants")
	}
	_ = active
}

func TestLoadPerformanceTransferHistoryByUserIsolatesOrgs(t *testing.T) {
	seenOrgFilter := false
	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			{
				match: func(query string, args []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					if !strings.Contains(lower, "employee_transfers") {
						return false
					}
					for _, arg := range args {
						if arg.Value == "org-a" {
							seenOrgFilter = true
						}
					}
					return true
				},
				columns: []string{
					"id", "org_id", "transfer_id", "user_id", "user_name",
					"old_department_id", "old_department_name", "old_position",
					"new_department_id", "new_department_name", "new_position",
					"transfer_date", "status",
				},
				rows: [][]driver.Value{
					{
						int64(1), "org-a", "t-a", "shared-user", "Shared",
						"dept-a1", "A1", "P1", "dept-a2", "A2", "P2",
						"2026-01-01", "approved",
					},
					{
						int64(2), "org-b", "t-b", "shared-user", "Shared",
						"dept-b1", "B1", "Q1", "dept-b2", "B2", "Q2",
						"2026-02-01", "approved",
					},
				},
			},
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)
	svc.orgID = "org-a"

	result, err := svc.loadPerformanceTransferHistoryByUser(svc.db, "org-a", []string{"shared-user"})
	if err != nil {
		t.Fatalf("loadPerformanceTransferHistoryByUser() error = %v", err)
	}
	transfers := result["shared-user"]
	if len(transfers) != 1 {
		t.Fatalf("transfers = %#v, want only org-a record", transfers)
	}
	if transfers[0].OrgID != "org-a" || transfers[0].NewDepartmentID != "dept-a2" {
		t.Fatalf("unexpected transfer: %#v", transfers[0])
	}
	if !seenOrgFilter {
		t.Fatalf("expected explicit org_id filter in query args")
	}
}
