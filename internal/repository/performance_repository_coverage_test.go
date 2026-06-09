package repository

import (
	"database/sql/driver"
	"errors"
	"strings"
	"testing"

	"peopleops/internal/database"
)

func findPerformanceQueryCall(t *testing.T, calls []stubPerformanceCall, table string, count bool) stubPerformanceCall {
	t.Helper()
	table = strings.ToLower(table)
	for _, call := range calls {
		lower := strings.ToLower(call.query)
		if strings.Contains(lower, table) && (strings.Contains(lower, "count(*)") == count) {
			return call
		}
	}
	t.Fatalf("query log missing table=%s count=%v: %#v", table, count, calls)
	return stubPerformanceCall{}
}

func assertPerformanceQueryHasFragments(t *testing.T, query string, fragments ...string) {
	t.Helper()
	lower := strings.ToLower(query)
	for _, fragment := range fragments {
		if !strings.Contains(lower, strings.ToLower(fragment)) {
			t.Fatalf("query missing %q: %s", fragment, query)
		}
	}
}

func assertPerformanceQueryMissingFragments(t *testing.T, query string, fragments ...string) {
	t.Helper()
	lower := strings.ToLower(query)
	for _, fragment := range fragments {
		if strings.Contains(lower, strings.ToLower(fragment)) {
			t.Fatalf("query should not contain %q: %s", fragment, query)
		}
	}
}

func assertPerformanceArgsContainAll(t *testing.T, args []driver.NamedValue, values ...string) {
	t.Helper()
	for _, value := range values {
		if !stubPerformanceArgsContain(args, value) {
			t.Fatalf("query args missing %q: %#v", value, args)
		}
	}
}

func assertPerformanceExecHappened(t *testing.T, calls []stubPerformanceCall, fragments ...string) {
	t.Helper()
	matcher := stubPerformanceSQLMatcher(fragments...)
	for _, call := range calls {
		if matcher(call.query, call.args) {
			return
		}
	}
	t.Fatalf("exec log missing fragments %v: %#v", fragments, calls)
}

func assertPerformanceNoExec(t *testing.T, calls []stubPerformanceCall, fragments ...string) {
	t.Helper()
	matcher := stubPerformanceSQLMatcher(fragments...)
	for _, call := range calls {
		if matcher(call.query, call.args) {
			t.Fatalf("exec log should not contain fragments %v: %#v", fragments, calls)
		}
	}
}

func assertPerformanceNoQuery(t *testing.T, calls []stubPerformanceCall, fragments ...string) {
	t.Helper()
	matcher := stubPerformanceSQLMatcher(fragments...)
	for _, call := range calls {
		if matcher(call.query, call.args) {
			t.Fatalf("query log should not contain fragments %v: %#v", fragments, calls)
		}
	}
}

func assertPerformanceTransactionCounts(t *testing.T, stub *stubPerformanceDB, wantBegin, wantCommit, wantRollback int) {
	t.Helper()
	begin, commit, rollback := stub.transactionCounts()
	if begin != wantBegin || commit != wantCommit || rollback != wantRollback {
		t.Fatalf("transaction counts begin=%d commit=%d rollback=%d, want %d/%d/%d", begin, commit, rollback, wantBegin, wantCommit, wantRollback)
	}
}

func participantIDMatcher(participantID uint) func(string, []driver.NamedValue) bool {
	return func(query string, args []driver.NamedValue) bool {
		if !stubPerformanceTableMatcher("performance_participants")(query, args) || strings.Contains(strings.ToLower(query), "count(*)") {
			return false
		}
		return performanceArgsContainUint(args, participantID)
	}
}

func participantActivityMatcher(participantID uint, activityID string) func(string, []driver.NamedValue) bool {
	return func(query string, args []driver.NamedValue) bool {
		return participantIDMatcher(participantID)(query, args) && stubPerformanceArgsContain(args, activityID)
	}
}

func performanceArgsContainUint(args []driver.NamedValue, want uint) bool {
	for _, arg := range args {
		switch value := arg.Value.(type) {
		case int64:
			if value == int64(want) {
				return true
			}
		case int:
			if value == int(want) {
				return true
			}
		case uint:
			if value == want {
				return true
			}
		case uint64:
			if value == uint64(want) {
				return true
			}
		}
	}
	return false
}

func performanceArgsContainFloat(args []driver.NamedValue, want float64) bool {
	for _, arg := range args {
		value, ok := arg.Value.(float64)
		if ok && value == want {
			return true
		}
	}
	return false
}

func managerEvaluationItemsForCoverage() []struct {
	ItemKey   string
	ItemScore float64
	ItemValue string
} {
	return []struct {
		ItemKey   string
		ItemScore float64
		ItemValue string
	}{
		{ItemKey: "goal-1", ItemScore: 88, ItemValue: "达成"},
	}
}

func managerEvaluationsForCoverage(participantID uint, score float64, level string, items []struct {
	ItemKey   string
	ItemScore float64
	ItemValue string
}) []struct {
	ParticipantID   uint
	ManagerScore    float64
	SuggestedLevel  string
	ManagerComment  string
	EvaluationItems []struct {
		ItemKey   string
		ItemScore float64
		ItemValue string
	}
} {
	return []struct {
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
		{ParticipantID: participantID, ManagerScore: score, SuggestedLevel: level, ManagerComment: "覆盖率补充", EvaluationItems: items},
	}
}

func TestActivityRepo_FindAll_DateFiltersBuildExpectedQuery(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceCountMatcher("performance_activities"),
				columns: []string{"count(*)"},
				rows:    [][]driver.Value{{int64(1)}},
			},
			{
				match:   stubPerformanceSelectMatcher("performance_activities"),
				columns: activityColumns(),
				rows:    [][]driver.Value{activityRow(1, "Q1", "quarterly", "active")},
			},
		},
	})
	repo := NewPerformanceActivityRepository(db)

	items, total, err := repo.FindAll(1, 10, "", "", "2026-01-01", "2026-03-31", nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("FindAll() total=%d len=%d, want 1/1", total, len(items))
	}

	countCall := findPerformanceQueryCall(t, stub.queryLog(), "performance_activities", true)
	assertPerformanceQueryHasFragments(t, countCall.query, "deleted_at is null", "start_date >=", "end_date <=")
	assertPerformanceArgsContainAll(t, countCall.args, "2026-01-01", "2026-03-31")
}

func TestActivityRepo_FindAll_CountError(t *testing.T) {
	errCount := errors.New("activity count failed")
	db := newPerformanceTestDB(t, stubPerformanceQueryResponse{
		match: stubPerformanceCountMatcher("performance_activities"),
		err:   errCount,
	})
	repo := NewPerformanceActivityRepository(db)

	items, total, err := repo.FindAll(1, 10, "", "", "", "", nil)
	if !errors.Is(err, errCount) {
		t.Fatalf("FindAll() error = %v, want %v", err, errCount)
	}
	if items != nil || total != 0 {
		t.Fatalf("FindAll() items=%#v total=%d, want nil/0", items, total)
	}
}

func TestActivityRepo_FindAll_FindError(t *testing.T) {
	errFind := errors.New("activity find failed")
	db := newPerformanceTestDB(t,
		stubPerformanceQueryResponse{
			match:   stubPerformanceCountMatcher("performance_activities"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubPerformanceQueryResponse{
			match: stubPerformanceSelectMatcher("performance_activities"),
			err:   errFind,
		},
	)
	repo := NewPerformanceActivityRepository(db)

	items, total, err := repo.FindAll(1, 10, "", "", "", "", nil)
	if !errors.Is(err, errFind) {
		t.Fatalf("FindAll() error = %v, want %v", err, errFind)
	}
	if items != nil || total != 0 {
		t.Fatalf("FindAll() items=%#v total=%d, want nil/0 on find error", items, total)
	}
}

func TestActivityRepo_FindAllByUserID_AllFiltersBuildExpectedQuery(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceCountMatcher("performance_activities"),
				columns: []string{"count(*)"},
				rows:    [][]driver.Value{{int64(1)}},
			},
			{
				match:   stubPerformanceSelectMatcher("performance_activities"),
				columns: activityColumns(),
				rows:    [][]driver.Value{activityRow(1, "Q1", "quarterly", "active")},
			},
		},
	})
	repo := NewPerformanceActivityRepository(db)

	items, total, err := repo.FindAllByUserID(1, 10, "active", " Q1 ", "2026-01-01", "2026-03-31", []string{"emp-1", "emp-2"})
	if err != nil {
		t.Fatalf("FindAllByUserID() error = %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("FindAllByUserID() total=%d len=%d, want 1/1", total, len(items))
	}

	countCall := findPerformanceQueryCall(t, stub.queryLog(), "performance_activities", true)
	assertPerformanceQueryHasFragments(t, countCall.query,
		"status =",
		"name like",
		"description like",
		"start_date >=",
		"end_date <=",
		"performance_participants",
		"employee_id in",
	)
	assertPerformanceArgsContainAll(t, countCall.args, "active", "%Q1%", "2026-01-01", "2026-03-31", "emp-1", "emp-2")
}

func TestActivityRepo_FindAllByUserID_CountError(t *testing.T) {
	errCount := errors.New("user activity count failed")
	db := newPerformanceTestDB(t, stubPerformanceQueryResponse{
		match: stubPerformanceCountMatcher("performance_activities"),
		err:   errCount,
	})
	repo := NewPerformanceActivityRepository(db)

	items, total, err := repo.FindAllByUserID(1, 10, "", "", "", "", []string{"emp-1"})
	if !errors.Is(err, errCount) {
		t.Fatalf("FindAllByUserID() error = %v, want %v", err, errCount)
	}
	if items != nil || total != 0 {
		t.Fatalf("FindAllByUserID() items=%#v total=%d, want nil/0", items, total)
	}
}

func TestActivityRepo_FindAllByUserID_FindError(t *testing.T) {
	errFind := errors.New("user activity find failed")
	db := newPerformanceTestDB(t,
		stubPerformanceQueryResponse{
			match:   stubPerformanceCountMatcher("performance_activities"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubPerformanceQueryResponse{
			match: stubPerformanceSelectMatcher("performance_activities"),
			err:   errFind,
		},
	)
	repo := NewPerformanceActivityRepository(db)

	items, total, err := repo.FindAllByUserID(1, 10, "", "", "", "", []string{"emp-1"})
	if !errors.Is(err, errFind) {
		t.Fatalf("FindAllByUserID() error = %v, want %v", err, errFind)
	}
	if items != nil || total != 0 {
		t.Fatalf("FindAllByUserID() items=%#v total=%d, want nil/0 on find error", items, total)
	}
}

func TestParticipantRepo_FindAll_DefaultStatusExcludesInactiveAndRemoved(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceCountMatcher("performance_participants"),
				columns: []string{"count(*)"},
				rows:    [][]driver.Value{{int64(1)}},
			},
			{
				match:   stubPerformanceSelectMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(1, "act-1", "emp-1", "Alice", "dept-1", "pending")},
			},
		},
	})
	repo := NewPerformanceParticipantRepository(db)

	participants, total, err := repo.FindAll("act-1", 1, 10, "", "", "", "", nil, nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 || len(participants) != 1 {
		t.Fatalf("FindAll() total=%d len=%d, want 1/1", total, len(participants))
	}

	countCall := findPerformanceQueryCall(t, stub.queryLog(), "performance_participants", true)
	assertPerformanceQueryHasFragments(t, countCall.query, "activity_id =", "deleted_at is null", "status not in")
	assertPerformanceArgsContainAll(t, countCall.args, "act-1", "inactive", "removed_from_scope")
}

func TestParticipantRepo_FindAll_ExplicitStatusDoesNotAddDefaultExclusion(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceCountMatcher("performance_participants"),
				columns: []string{"count(*)"},
				rows:    [][]driver.Value{{int64(1)}},
			},
			{
				match:   stubPerformanceSelectMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(1, "act-1", "emp-1", "Alice", "dept-1", "inactive")},
			},
		},
	})
	repo := NewPerformanceParticipantRepository(db)

	participants, total, err := repo.FindAll("act-1", 1, 10, "", "", "inactive", "", nil, nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 || len(participants) != 1 {
		t.Fatalf("FindAll() total=%d len=%d, want 1/1", total, len(participants))
	}

	countCall := findPerformanceQueryCall(t, stub.queryLog(), "performance_participants", true)
	assertPerformanceQueryHasFragments(t, countCall.query, "activity_id =", "status =")
	assertPerformanceQueryMissingFragments(t, countCall.query, "status not in")
	assertPerformanceArgsContainAll(t, countCall.args, "act-1", "inactive")
}

func TestParticipantRepo_FindAll_CountError(t *testing.T) {
	errCount := errors.New("participant count failed")
	db := newPerformanceTestDB(t, stubPerformanceQueryResponse{
		match: stubPerformanceCountMatcher("performance_participants"),
		err:   errCount,
	})
	repo := NewPerformanceParticipantRepository(db)

	participants, total, err := repo.FindAll("act-1", 1, 10, "", "", "", "", nil, nil)
	if !errors.Is(err, errCount) {
		t.Fatalf("FindAll() error = %v, want %v", err, errCount)
	}
	if participants != nil || total != 0 {
		t.Fatalf("FindAll() participants=%#v total=%d, want nil/0", participants, total)
	}
}

func TestParticipantRepo_FindAll_FindError(t *testing.T) {
	errFind := errors.New("participant find failed")
	db := newPerformanceTestDB(t,
		stubPerformanceQueryResponse{
			match:   stubPerformanceCountMatcher("performance_participants"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubPerformanceQueryResponse{
			match: stubPerformanceSelectMatcher("performance_participants"),
			err:   errFind,
		},
	)
	repo := NewPerformanceParticipantRepository(db)

	participants, total, err := repo.FindAll("act-1", 1, 10, "", "", "", "", nil, nil)
	if !errors.Is(err, errFind) {
		t.Fatalf("FindAll() error = %v, want %v", err, errFind)
	}
	if participants != nil || total != 0 {
		t.Fatalf("FindAll() participants=%#v total=%d, want nil/0 on find error", participants, total)
	}
}

func TestParticipantRepo_CountByActivityAndStatus_Error(t *testing.T) {
	errCount := errors.New("participant status count failed")
	db := newPerformanceTestDB(t, stubPerformanceQueryResponse{
		match: stubPerformanceCountMatcher("performance_participants"),
		err:   errCount,
	})
	repo := NewPerformanceParticipantRepository(db)

	count, err := repo.CountByActivityAndStatus("act-1", "pending")
	if !errors.Is(err, errCount) {
		t.Fatalf("CountByActivityAndStatus() error = %v, want %v", err, errCount)
	}
	if count != 0 {
		t.Fatalf("CountByActivityAndStatus() count=%d, want 0", count)
	}
}

func TestTemplateRepo_Create_TemplateCreateError(t *testing.T) {
	errCreate := errors.New("template create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_templates", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceTemplateRepository(db)

	err := repo.Create(&database.PerformanceTemplate{Name: "模板"}, nil, nil, nil)
	if !errors.Is(err, errCreate) {
		t.Fatalf("Create() error = %v, want %v", err, errCreate)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestTemplateRepo_Create_SectionCreateError(t *testing.T) {
	errCreate := errors.New("section create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_template_sections", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceTemplateRepository(db)

	err := repo.Create(&database.PerformanceTemplate{Name: "模板"}, []database.PerformanceTemplateSection{{Name: "维度"}}, nil, []int{0})
	if !errors.Is(err, errCreate) {
		t.Fatalf("Create() error = %v, want %v", err, errCreate)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestTemplateRepo_Create_ItemCreateError(t *testing.T) {
	errCreate := errors.New("item create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_template_items", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceTemplateRepository(db)

	err := repo.Create(
		&database.PerformanceTemplate{Name: "模板"},
		[]database.PerformanceTemplateSection{{Name: "维度"}},
		[]database.PerformanceTemplateItem{{Name: "指标"}},
		[]int{1},
	)
	if !errors.Is(err, errCreate) {
		t.Fatalf("Create() error = %v, want %v", err, errCreate)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestTemplateRepo_GetByID_SectionsError(t *testing.T) {
	errQuery := errors.New("sections query failed")
	db := newPerformanceTestDB(t,
		stubPerformanceQueryResponse{
			match:   stubPerformanceSQLMatcher("from", "performance_templates"),
			columns: templateColumns(),
			rows:    [][]driver.Value{templateRow(1, "模板", "active")},
		},
		stubPerformanceQueryResponse{
			match: stubPerformanceSQLMatcher("from", "performance_template_sections"),
			err:   errQuery,
		},
	)
	repo := NewPerformanceTemplateRepository(db)

	template, sections, items, err := repo.GetByID(1)
	if !errors.Is(err, errQuery) {
		t.Fatalf("GetByID() error = %v, want %v", err, errQuery)
	}
	if template != nil || sections != nil || items != nil {
		t.Fatalf("GetByID() template=%#v sections=%#v items=%#v, want nils", template, sections, items)
	}
}

func TestTemplateRepo_GetByID_ItemsError(t *testing.T) {
	errQuery := errors.New("items query failed")
	db := newPerformanceTestDB(t,
		stubPerformanceQueryResponse{
			match:   stubPerformanceSQLMatcher("from", "performance_templates"),
			columns: templateColumns(),
			rows:    [][]driver.Value{templateRow(1, "模板", "active")},
		},
		stubPerformanceQueryResponse{
			match:   stubPerformanceSQLMatcher("from", "performance_template_sections"),
			columns: []string{"id", "template_id", "name", "section_type", "weight", "sort_order", "is_score_required", "is_comment_required", "created_at", "updated_at", "deleted_at"},
			rows: [][]driver.Value{{
				uint(10), uint(1), "维度", "score", 100.0, 1, true, false, nil, nil, nil,
			}},
		},
		stubPerformanceQueryResponse{
			match: stubPerformanceSQLMatcher("from", "performance_template_items"),
			err:   errQuery,
		},
	)
	repo := NewPerformanceTemplateRepository(db)

	template, sections, items, err := repo.GetByID(1)
	if !errors.Is(err, errQuery) {
		t.Fatalf("GetByID() error = %v, want %v", err, errQuery)
	}
	if template != nil || sections != nil || items != nil {
		t.Fatalf("GetByID() template=%#v sections=%#v items=%#v, want nils", template, sections, items)
	}
}

func TestTemplateRepo_GetByID_NoSectionsSkipsItemsQuery(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceSQLMatcher("from", "performance_templates"),
				columns: templateColumns(),
				rows:    [][]driver.Value{templateRow(1, "模板", "active")},
			},
			{
				match:   stubPerformanceSQLMatcher("from", "performance_template_sections"),
				columns: []string{"id", "template_id", "name", "section_type", "weight", "sort_order", "is_score_required", "is_comment_required", "created_at", "updated_at", "deleted_at"},
				rows:    [][]driver.Value{},
			},
		},
	})
	repo := NewPerformanceTemplateRepository(db)

	template, sections, items, err := repo.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if template == nil || template.ID != 1 {
		t.Fatalf("GetByID() template=%#v, want id=1", template)
	}
	if len(sections) != 0 || len(items) != 0 {
		t.Fatalf("GetByID() sections=%d items=%d, want 0/0", len(sections), len(items))
	}
	assertPerformanceNoQuery(t, stub.queryLog(), "performance_template_items")
}

func TestTemplateRepo_FindAll_CountError(t *testing.T) {
	errCount := errors.New("template count failed")
	db := newPerformanceTestDB(t, stubPerformanceQueryResponse{
		match: stubPerformanceCountMatcher("performance_templates"),
		err:   errCount,
	})
	repo := NewPerformanceTemplateRepository(db)

	items, total, err := repo.FindAll(1, 10, "")
	if !errors.Is(err, errCount) {
		t.Fatalf("FindAll() error = %v, want %v", err, errCount)
	}
	if items != nil || total != 0 {
		t.Fatalf("FindAll() items=%#v total=%d, want nil/0", items, total)
	}
}

func TestTemplateRepo_FindAll_FindError(t *testing.T) {
	errFind := errors.New("template find failed")
	db := newPerformanceTestDB(t,
		stubPerformanceQueryResponse{
			match:   stubPerformanceCountMatcher("performance_templates"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubPerformanceQueryResponse{
			match: stubPerformanceSelectMatcher("performance_templates"),
			err:   errFind,
		},
	)
	repo := NewPerformanceTemplateRepository(db)

	items, total, err := repo.FindAll(1, 10, "")
	if !errors.Is(err, errFind) {
		t.Fatalf("FindAll() error = %v, want %v", err, errFind)
	}
	if items != nil || total != 0 {
		t.Fatalf("FindAll() items=%#v total=%d, want nil/0 on find error", items, total)
	}
}

func TestTemplateRepo_IsReferencedByActivity_FalseWhenColumnMissing(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: stubPerformanceHasColumnResponses("performance_activities", "template_id", 0),
	})
	repo := NewPerformanceTemplateRepository(db)

	referenced, err := repo.IsReferencedByActivity(1)
	if err != nil {
		t.Fatalf("IsReferencedByActivity() error = %v", err)
	}
	if referenced {
		t.Fatal("IsReferencedByActivity() = true, want false when column is missing")
	}
	for _, call := range stub.queryLog() {
		lower := strings.ToLower(call.query)
		if strings.Contains(lower, "performance_activities") && strings.Contains(lower, "template_id =") {
			t.Fatalf("IsReferencedByActivity() should skip activity count when column is missing: %s", call.query)
		}
	}
}

func TestReviewVersionRepo_CreateSelfEvaluationVersion_CreateVersionError(t *testing.T) {
	errCreate := errors.New("self version create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "pending")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_review_versions", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.CreateSelfEvaluationVersion("10", 85, "B", "总结", nil, "emp-1")
	if !errors.Is(err, errCreate) {
		t.Fatalf("CreateSelfEvaluationVersion() error = %v, want %v", err, errCreate)
	}
	if version != nil {
		t.Fatalf("CreateSelfEvaluationVersion() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestReviewVersionRepo_CreateSelfEvaluationVersion_UpdateParticipantError(t *testing.T) {
	errUpdate := errors.New("self participant update failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "pending")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_participants", "update"), err: errUpdate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.CreateSelfEvaluationVersion("10", 85, "B", "总结", nil, "emp-1")
	if !errors.Is(err, errUpdate) {
		t.Fatalf("CreateSelfEvaluationVersion() error = %v, want %v", err, errUpdate)
	}
	if version != nil {
		t.Fatalf("CreateSelfEvaluationVersion() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
	assertPerformanceExecHappened(t, stub.execLog(), "performance_review_versions", "insert")
	assertPerformanceExecHappened(t, stub.execLog(), "performance_participants", "update")
}

func TestReviewVersionRepo_CreateSelfEvaluationVersion_PreservesManagerSubmittedStatus(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "manager_submitted")},
			},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.CreateSelfEvaluationVersion("10", 85, "B", "总结", nil, "emp-1")
	if err != nil {
		t.Fatalf("CreateSelfEvaluationVersion() error = %v", err)
	}
	if version == nil || version.ReviewType != "self" {
		t.Fatalf("CreateSelfEvaluationVersion() version=%#v, want self version", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 1, 0)
	var participantUpdate *stubPerformanceCall
	for _, call := range stub.execLog() {
		if stubPerformanceSQLMatcher("performance_participants", "update")(call.query, call.args) {
			callCopy := call
			participantUpdate = &callCopy
			break
		}
	}
	if participantUpdate == nil {
		t.Fatal("CreateSelfEvaluationVersion() did not update participant")
	}
	assertPerformanceArgsContainAll(t, participantUpdate.args, "manager_submitted", "emp-1")
}

func TestReviewVersionRepo_CreateManagerEvaluationVersion_CreateVersionError(t *testing.T) {
	errCreate := errors.New("manager version create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_review_versions", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.CreateManagerEvaluationVersion("10", 88, "A", "评价", nil, "mgr-1")
	if !errors.Is(err, errCreate) {
		t.Fatalf("CreateManagerEvaluationVersion() error = %v, want %v", err, errCreate)
	}
	if version != nil {
		t.Fatalf("CreateManagerEvaluationVersion() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestReviewVersionRepo_CreateManagerEvaluationVersion_UpdateParticipantError(t *testing.T) {
	errUpdate := errors.New("manager participant update failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_participants", "update"), err: errUpdate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.CreateManagerEvaluationVersion("10", 88, "A", "评价", nil, "mgr-1")
	if !errors.Is(err, errUpdate) {
		t.Fatalf("CreateManagerEvaluationVersion() error = %v, want %v", err, errUpdate)
	}
	if version != nil {
		t.Fatalf("CreateManagerEvaluationVersion() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
	assertPerformanceExecHappened(t, stub.execLog(), "performance_review_versions", "insert")
	assertPerformanceExecHappened(t, stub.execLog(), "performance_participants", "update")
}

func TestReviewVersionRepo_CreateManagerEvaluationVersion_PreservesResultConfirmedStatusAndInfersFinalLevel(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "result_confirmed")},
			},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.CreateManagerEvaluationVersion("10", 88, " ", "评价", nil, "mgr-1")
	if err != nil {
		t.Fatalf("CreateManagerEvaluationVersion() error = %v", err)
	}
	if version == nil || version.FinalLevel != "B" {
		t.Fatalf("CreateManagerEvaluationVersion() final_level=%#v, want B", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 1, 0)
	var participantUpdate *stubPerformanceCall
	for _, call := range stub.execLog() {
		if stubPerformanceSQLMatcher("performance_participants", "update")(call.query, call.args) {
			callCopy := call
			participantUpdate = &callCopy
			break
		}
	}
	if participantUpdate == nil {
		t.Fatal("CreateManagerEvaluationVersion() did not update participant")
	}
	assertPerformanceArgsContainAll(t, participantUpdate.args, "result_confirmed", "B", "mgr-1")
}

func TestReviewVersionRepo_AdjustFinalLevel_CreateVersionError(t *testing.T) {
	errCreate := errors.New("adjust version create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "manager_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_review_versions", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.AdjustFinalLevel("10", "S", "调级原因", "hr-1")
	if !errors.Is(err, errCreate) {
		t.Fatalf("AdjustFinalLevel() error = %v, want %v", err, errCreate)
	}
	if version != nil {
		t.Fatalf("AdjustFinalLevel() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestReviewVersionRepo_AdjustFinalLevel_UpdateParticipantError(t *testing.T) {
	errUpdate := errors.New("adjust participant update failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "manager_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_participants", "update"), err: errUpdate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.AdjustFinalLevel("10", "S", "调级原因", "hr-1")
	if !errors.Is(err, errUpdate) {
		t.Fatalf("AdjustFinalLevel() error = %v, want %v", err, errUpdate)
	}
	if version != nil {
		t.Fatalf("AdjustFinalLevel() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
	assertPerformanceExecHappened(t, stub.execLog(), "performance_review_versions", "insert")
	assertPerformanceExecHappened(t, stub.execLog(), "performance_participants", "update")
}

func TestReviewVersionRepo_ConfirmResult_CreateVersionError(t *testing.T) {
	errCreate := errors.New("confirm version create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "manager_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_review_versions", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.ConfirmResult("10", "确认意见", "emp-1")
	if !errors.Is(err, errCreate) {
		t.Fatalf("ConfirmResult() error = %v, want %v", err, errCreate)
	}
	if version != nil {
		t.Fatalf("ConfirmResult() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestReviewVersionRepo_ConfirmResult_UpdateParticipantError(t *testing.T) {
	errUpdate := errors.New("confirm participant update failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "manager_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_participants", "update"), err: errUpdate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	version, err := repo.ConfirmResult("10", "确认意见", "emp-1")
	if !errors.Is(err, errUpdate) {
		t.Fatalf("ConfirmResult() error = %v, want %v", err, errUpdate)
	}
	if version != nil {
		t.Fatalf("ConfirmResult() version=%#v, want nil", version)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
	assertPerformanceExecHappened(t, stub.execLog(), "performance_review_versions", "insert")
	assertPerformanceExecHappened(t, stub.execLog(), "performance_participants", "update")
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_EmptyEvaluations(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{})
	repo := NewPerformanceReviewVersionRepository(db)

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", nil, "mgr-1")
	if err != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() error = %v", err)
	}
	if versions == nil || len(versions) != 0 {
		t.Fatalf("BatchCreateManagerEvaluationVersions() versions=%#v, want empty non-nil slice", versions)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 1, 0)
	if len(stub.queryLog()) != 0 || len(stub.execLog()) != 0 {
		t.Fatalf("BatchCreateManagerEvaluationVersions() queries=%#v execs=%#v, want none", stub.queryLog(), stub.execLog())
	}
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_CreateVersionError(t *testing.T) {
	errCreate := errors.New("batch version create failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_review_versions", "insert"), err: errCreate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", managerEvaluationsForCoverage(10, 88, "A", managerEvaluationItemsForCoverage()), "mgr-1")
	if !errors.Is(err, errCreate) {
		t.Fatalf("BatchCreateManagerEvaluationVersions() error = %v, want %v", err, errCreate)
	}
	if versions != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() versions=%#v, want nil", versions)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_WithItemsSkipsGoalDistribution(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_participants", "update"), result: stubPerformanceRowsAffected(1)},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", managerEvaluationsForCoverage(10, 88, "A", managerEvaluationItemsForCoverage()), "mgr-1")
	if err != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() error = %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("BatchCreateManagerEvaluationVersions() len=%d, want 1", len(versions))
	}
	assertPerformanceTransactionCounts(t, stub, 1, 1, 0)
	assertPerformanceNoQuery(t, stub.queryLog(), "performance_goal_records")
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_GoalRecordQueryErrorIgnored(t *testing.T) {
	errQuery := errors.New("goal records query failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
			{
				match: goalRecordMatcher("select"),
				err:   errQuery,
			},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", managerEvaluationsForCoverage(10, 88, "A", nil), "mgr-1")
	if err != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() error = %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("BatchCreateManagerEvaluationVersions() len=%d, want 1", len(versions))
	}
	assertPerformanceTransactionCounts(t, stub, 1, 1, 0)
	assertPerformanceNoExec(t, stub.execLog(), "performance_goal_records", "update")
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_ZeroTotalWeightSkipsGoalUpdates(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
			{
				match:   stubPerformanceSQLMatcher("performance_goal_records", "participant_id", "section_type !="),
				columns: performanceGoalRecordColumns(),
				rows: [][]driver.Value{
					performanceGoalRecordRow(1, 10, 0),
					performanceGoalRecordRow(2, 10, 0),
				},
			},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", managerEvaluationsForCoverage(10, 88, "A", nil), "mgr-1")
	if err != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() error = %v", err)
	}
	if len(versions) != 1 {
		t.Fatalf("BatchCreateManagerEvaluationVersions() len=%d, want 1", len(versions))
	}
	assertPerformanceTransactionCounts(t, stub, 1, 1, 0)
	assertPerformanceNoExec(t, stub.execLog(), "performance_goal_records", "update")
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_GoalRecordUpdateError(t *testing.T) {
	errUpdate := errors.New("goal record update failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
			{
				match:   stubPerformanceSQLMatcher("performance_goal_records", "participant_id", "section_type !="),
				columns: performanceGoalRecordColumns(),
				rows:    [][]driver.Value{performanceGoalRecordRow(1, 10, 100)},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_goal_records", "update", "manager_score"), err: errUpdate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", managerEvaluationsForCoverage(10, 88, "A", nil), "mgr-1")
	if !errors.Is(err, errUpdate) {
		t.Fatalf("BatchCreateManagerEvaluationVersions() error = %v, want %v", err, errUpdate)
	}
	if versions != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() versions=%#v, want nil", versions)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_UpdateParticipantError(t *testing.T) {
	errUpdate := errors.New("batch participant update failed")
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
		},
		execs: []stubPerformanceExecResponse{
			{match: stubPerformanceExecMatcher("performance_participants", "update"), err: errUpdate},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", managerEvaluationsForCoverage(10, 88, "A", managerEvaluationItemsForCoverage()), "mgr-1")
	if !errors.Is(err, errUpdate) {
		t.Fatalf("BatchCreateManagerEvaluationVersions() error = %v, want %v", err, errUpdate)
	}
	if versions != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() versions=%#v, want nil", versions)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
	assertPerformanceExecHappened(t, stub.execLog(), "performance_review_versions", "insert")
	assertPerformanceExecHappened(t, stub.execLog(), "performance_participants", "update")
}

func TestReviewVersionRepo_BatchCreateManagerEvaluationVersions_SecondParticipantNotFoundRollsBack(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   participantIDMatcher(10),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "self_submitted")},
			},
			{
				match:   participantIDMatcher(20),
				columns: participantColumns(),
				rows:    [][]driver.Value{},
			},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	evaluations := []struct {
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
		{ParticipantID: 10, ManagerScore: 88, SuggestedLevel: "A", ManagerComment: "first", EvaluationItems: managerEvaluationItemsForCoverage()},
		{ParticipantID: 20, ManagerScore: 82, SuggestedLevel: "B", ManagerComment: "second", EvaluationItems: managerEvaluationItemsForCoverage()},
	}

	versions, err := repo.BatchCreateManagerEvaluationVersions("act-1", evaluations, "mgr-1")
	if err == nil {
		t.Fatal("BatchCreateManagerEvaluationVersions() should return error for second missing participant")
	}
	if versions != nil {
		t.Fatalf("BatchCreateManagerEvaluationVersions() versions=%#v, want nil", versions)
	}
	assertPerformanceTransactionCounts(t, stub, 1, 0, 1)
}

func TestReviewVersionRepo_GetParticipantLocked_UsesForUpdate(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_participants"),
				columns: participantColumns(),
				rows:    [][]driver.Value{participantRow(10, "act-1", "emp-1", "Alice", "dept-1", "manager_submitted")},
			},
		},
	})
	repo := NewPerformanceReviewVersionRepository(db)

	participant, err := repo.getParticipantLocked("10")
	if err != nil {
		t.Fatalf("getParticipantLocked() error = %v", err)
	}
	if participant == nil || participant.ID != 10 {
		t.Fatalf("getParticipantLocked() participant=%#v, want id=10", participant)
	}
	selectCall := findPerformanceQueryCall(t, stub.queryLog(), "performance_participants", false)
	assertPerformanceQueryHasFragments(t, selectCall.query, "for update")
}

func TestRelationshipChangeLogRepo_ListByParticipant_BuildsDeletedAtAndOrder(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_relationship_change_logs"),
				columns: relationshipChangeLogColumns(),
				rows:    [][]driver.Value{relationshipChangeLogRow(1, 10, "act-1", "manager_change")},
			},
		},
	})
	repo := NewPerformanceRelationshipChangeLogRepository(db)

	logs, err := repo.ListByParticipant("10")
	if err != nil {
		t.Fatalf("ListByParticipant() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("ListByParticipant() len=%d, want 1", len(logs))
	}
	selectCall := findPerformanceQueryCall(t, stub.queryLog(), "performance_relationship_change_logs", false)
	assertPerformanceQueryHasFragments(t, selectCall.query, "participant_id", "deleted_at is null", "order by", "changed_at")
}

func TestRelationshipChangeLogRepo_ListByActivity_BuildsDeletedAtAndOrder(t *testing.T) {
	db, stub := newPerformanceTestDBWithStub(t, &stubPerformanceDB{
		queries: []stubPerformanceQueryResponse{
			{
				match:   stubPerformanceTableMatcher("performance_relationship_change_logs"),
				columns: relationshipChangeLogColumns(),
				rows:    [][]driver.Value{relationshipChangeLogRow(1, 10, "act-1", "manager_change")},
			},
		},
	})
	repo := NewPerformanceRelationshipChangeLogRepository(db)

	logs, err := repo.ListByActivity("act-1")
	if err != nil {
		t.Fatalf("ListByActivity() error = %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("ListByActivity() len=%d, want 1", len(logs))
	}
	selectCall := findPerformanceQueryCall(t, stub.queryLog(), "performance_relationship_change_logs", false)
	assertPerformanceQueryHasFragments(t, selectCall.query, "activity_id", "deleted_at is null", "order by", "changed_at")
}
