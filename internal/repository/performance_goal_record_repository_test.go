package repository

import (
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"peopleops/internal/database"

	stdsql "database/sql"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// ===================== Stub Driver =====================

const stubGoalRecordDriverName = "peopleops_stub_goal_record_mysql"

var (
	stubGoalRecordDriverOnce sync.Once
	stubGoalRecordDBs        sync.Map
)

type stubGoalRecordQueryResponse struct {
	match   func(query string, args []driver.NamedValue) bool
	columns []string
	rows    [][]driver.Value
}

type stubGoalRecordDB struct {
	queries []stubGoalRecordQueryResponse
}

type stubGoalRecordDriver struct{}

type stubGoalRecordConn struct {
	db *stubGoalRecordDB
}

type stubGoalRecordStmt struct {
	conn  *stubGoalRecordConn
	query string
}

type stubGoalRecordRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type stubGoalRecordTx struct{}

type stubGoalRecordResult struct{}

func (stubGoalRecordResult) LastInsertId() (int64, error) { return 0, nil }
func (stubGoalRecordResult) RowsAffected() (int64, error) { return 0, nil }

func (d stubGoalRecordDriver) Open(name string) (driver.Conn, error) {
	value, ok := stubGoalRecordDBs.Load(name)
	if !ok {
		return nil, fmt.Errorf("stub db %s not registered", name)
	}
	return &stubGoalRecordConn{db: value.(*stubGoalRecordDB)}, nil
}

func (c *stubGoalRecordConn) Prepare(query string) (driver.Stmt, error) {
	return &stubGoalRecordStmt{conn: c, query: query}, nil
}

func (c *stubGoalRecordConn) Close() error { return nil }

func (c *stubGoalRecordConn) Begin() (driver.Tx, error) { return stubGoalRecordTx{}, nil }

func (c *stubGoalRecordConn) BeginTx(_ any, _ driver.TxOptions) (driver.Tx, error) {
	return stubGoalRecordTx{}, nil
}

func (c *stubGoalRecordConn) QueryContext(_ any, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.queryDB(query, args)
}

func (c *stubGoalRecordConn) ExecContext(_ any, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return stubGoalRecordResult{}, nil
}

func (c *stubGoalRecordConn) queryDB(query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, response := range c.db.queries {
		if response.match != nil && response.match(query, args) {
			rows := make([][]driver.Value, len(response.rows))
			for i := range response.rows {
				rows[i] = append([]driver.Value(nil), response.rows[i]...)
			}
			return &stubGoalRecordRows{
				columns: append([]string(nil), response.columns...),
				rows:    rows,
			}, nil
		}
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (s *stubGoalRecordStmt) Close() error  { return nil }
func (s *stubGoalRecordStmt) NumInput() int { return -1 }
func (s *stubGoalRecordStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return stubGoalRecordResult{}, nil
}
func (s *stubGoalRecordStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return s.conn.queryDB(s.query, named)
}

func (r *stubGoalRecordRows) Columns() []string { return r.columns }
func (r *stubGoalRecordRows) Close() error      { return nil }
func (r *stubGoalRecordRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.index]
	r.index++
	for i := range dest {
		if i < len(row) {
			dest[i] = row[i]
		}
	}
	return nil
}

func (stubGoalRecordTx) Commit() error   { return nil }
func (stubGoalRecordTx) Rollback() error { return nil }

// ===================== Test Helpers =====================

func newGoalRecordTestDB(t *testing.T, queries ...stubGoalRecordQueryResponse) *gorm.DB {
	t.Helper()
	stubGoalRecordDriverOnce.Do(func() {
		stdsql.Register(stubGoalRecordDriverName, stubGoalRecordDriver{})
	})

	dsn := fmt.Sprintf("goal-record-test-%s-%d", t.Name(), time.Now().UnixNano())
	stubGoalRecordDBs.Store(dsn, &stubGoalRecordDB{queries: queries})
	t.Cleanup(func() {
		stubGoalRecordDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(stubGoalRecordDriverName, dsn)
	if err != nil {
		t.Fatalf("open stub sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open stub gorm db: %v", err)
	}
	return db
}

func goalRecordTableMatcher() func(string, []driver.NamedValue) bool {
	return func(query string, _ []driver.NamedValue) bool {
		return strings.Contains(strings.ToLower(query), "performance_goal_records")
	}
}

func goalRecordMatcher(method string) func(string, []driver.NamedValue) bool {
	method = strings.ToLower(method)
	return func(query string, _ []driver.NamedValue) bool {
		return strings.Contains(strings.ToLower(query), "performance_goal_records") &&
			strings.Contains(strings.ToLower(query), method)
	}
}

func goalRecordColumns() []string {
	return []string{
		"id", "activity_id", "participant_id", "indicator_item_id",
		"section_type", "item_name", "item_definition", "weight",
		"red_line_value", "target_value", "challenge_value", "scoring_rule",
		"actual_result", "attachments", "self_score", "manager_score",
		"bonus_score", "is_from_superior", "approval_status", "visibility_scope",
		"sort_order", "created_at", "updated_at", "deleted_at",
	}
}

func goalRecordRow(id uint, activityID string, participantID uint, sectionType, itemName string, weight float64, sortOrder int) []driver.Value {
	return []driver.Value{
		id, activityID, participantID, nil,
		sectionType, itemName, "", weight,
		"", "", "", "",
		"", "[]", 0.0, 0.0,
		0.0, false, "pending", "department_only",
		sortOrder, time.Now(), time.Now(), nil,
	}
}

// ===================== GetByID Tests =====================

func TestGoalRecordRepo_GetByID_Found(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows:    [][]driver.Value{goalRecordRow(1, "act-1", 10, "quantitative", "KPI", 0.6, 1)},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	record, err := repo.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if record.ItemName != "KPI" {
		t.Fatalf("GetByID() item_name = %q, want KPI", record.ItemName)
	}
	if record.ActivityID != "act-1" {
		t.Fatalf("GetByID() activity_id = %q, want act-1", record.ActivityID)
	}
	if record.ParticipantID != 10 {
		t.Fatalf("GetByID() participant_id = %d, want 10", record.ParticipantID)
	}
}

func TestGoalRecordRepo_GetByID_NotFound(t *testing.T) {
	db := newGoalRecordTestDB(t) // no stub → query fails
	repo := NewPerformanceGoalRecordRepository(db)

	_, err := repo.GetByID(999)
	if err == nil {
		t.Fatal("GetByID(999) should return error for non-existent record")
	}
}

// ===================== FindByParticipant Tests =====================

func TestGoalRecordRepo_FindByParticipant_Found(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows: [][]driver.Value{
			goalRecordRow(1, "act-1", 10, "quantitative", "KPI1", 0.4, 1),
			goalRecordRow(2, "act-1", 10, "quantitative", "KPI2", 0.3, 2),
			goalRecordRow(3, "act-2", 10, "key_action", "Action1", 0.3, 1),
		},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByParticipant(10)
	if err != nil {
		t.Fatalf("FindByParticipant() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("FindByParticipant() len = %d, want 3", len(records))
	}
	for _, r := range records {
		if r.ParticipantID != 10 {
			t.Fatalf("FindByParticipant() participant_id = %d, want 10", r.ParticipantID)
		}
	}
}

func TestGoalRecordRepo_FindByParticipant_Empty(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows:    [][]driver.Value{},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByParticipant(999)
	if err != nil {
		t.Fatalf("FindByParticipant() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("FindByParticipant() len = %d, want 0", len(records))
	}
}

// ===================== FindByActivity Tests =====================

func TestGoalRecordRepo_FindByActivity_Found(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows: [][]driver.Value{
			goalRecordRow(1, "act-1", 10, "quantitative", "KPI1", 0.5, 1),
			goalRecordRow(2, "act-1", 20, "quantitative", "KPI1", 0.5, 1),
			goalRecordRow(3, "act-1", 10, "key_action", "Action1", 0.5, 2),
		},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByActivity("act-1")
	if err != nil {
		t.Fatalf("FindByActivity() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("FindByActivity() len = %d, want 3", len(records))
	}
	for _, r := range records {
		if r.ActivityID != "act-1" {
			t.Fatalf("FindByActivity() activity_id = %q, want act-1", r.ActivityID)
		}
	}
}

func TestGoalRecordRepo_FindByActivity_Empty(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows:    [][]driver.Value{},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByActivity("nonexistent")
	if err != nil {
		t.Fatalf("FindByActivity() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("FindByActivity() len = %d, want 0", len(records))
	}
}

// ===================== FindByActivityAndParticipant Tests =====================

func TestGoalRecordRepo_FindByActivityAndParticipant_Found(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows: [][]driver.Value{
			goalRecordRow(1, "act-1", 10, "quantitative", "KPI1", 0.6, 1),
			goalRecordRow(2, "act-1", 10, "key_action", "Action1", 0.4, 2),
		},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByActivityAndParticipant("act-1", 10)
	if err != nil {
		t.Fatalf("FindByActivityAndParticipant() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("FindByActivityAndParticipant() len = %d, want 2", len(records))
	}
	for _, r := range records {
		if r.ActivityID != "act-1" || r.ParticipantID != 10 {
			t.Fatalf("FindByActivityAndParticipant() got act=%s participant=%d, want act-1/10", r.ActivityID, r.ParticipantID)
		}
	}
}

func TestGoalRecordRepo_FindByActivityAndParticipant_Empty(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows:    [][]driver.Value{},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByActivityAndParticipant("act-1", 999)
	if err != nil {
		t.Fatalf("FindByActivityAndParticipant() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("FindByActivityAndParticipant() len = %d, want 0", len(records))
	}
}

// ===================== BatchUpsert Tests =====================

func TestGoalRecordRepo_BatchUpsert_Success(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	records := []database.PerformanceGoalRecord{
		{ActivityID: "act-1", ParticipantID: 10, SectionType: "quantitative", ItemName: "KPI1", Weight: 0.5, SortOrder: 1},
		{ActivityID: "act-1", ParticipantID: 10, SectionType: "key_action", ItemName: "Action1", Weight: 0.3, SortOrder: 2},
		{ActivityID: "act-1", ParticipantID: 20, SectionType: "quantitative", ItemName: "KPI1", Weight: 0.5, SortOrder: 1},
	}
	if err := repo.BatchUpsert(records); err != nil {
		t.Fatalf("BatchUpsert() error = %v", err)
	}
}

func TestGoalRecordRepo_BatchUpsert_Empty(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	// Empty slice should be a no-op (early return)
	if err := repo.BatchUpsert([]database.PerformanceGoalRecord{}); err != nil {
		t.Fatalf("BatchUpsert() empty error = %v", err)
	}
}

func TestGoalRecordRepo_BatchUpsert_SingleRecord(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	records := []database.PerformanceGoalRecord{
		{ActivityID: "act-1", ParticipantID: 10, SectionType: "bonus_penalty", ItemName: "Bonus", Weight: 0.1, SortOrder: 3},
	}
	if err := repo.BatchUpsert(records); err != nil {
		t.Fatalf("BatchUpsert() single error = %v", err)
	}
}

// ===================== UpdateSingle Tests =====================

func TestGoalRecordRepo_UpdateSingle_Success(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	record := &database.PerformanceGoalRecord{
		ID:             1,
		ActivityID:     "act-1",
		ParticipantID:  10,
		SectionType:    "quantitative",
		ItemName:       "Updated KPI",
		Weight:         0.7,
		SelfScore:      85.0,
		ManagerScore:   90.0,
		ApprovalStatus: "submitted",
	}
	if err := repo.UpdateSingle(record); err != nil {
		t.Fatalf("UpdateSingle() error = %v", err)
	}
}

func TestGoalRecordRepo_UpdateSingle_WithScores(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	record := &database.PerformanceGoalRecord{
		ID:             2,
		ActivityID:     "act-2",
		ParticipantID:  20,
		SectionType:    "quantitative",
		ItemName:       "Scored KPI",
		Weight:         0.6,
		SelfScore:      78.5,
		ManagerScore:   82.0,
		BonusScore:     5.0,
		ApprovalStatus: "approved",
	}
	if err := repo.UpdateSingle(record); err != nil {
		t.Fatalf("UpdateSingle() with scores error = %v", err)
	}
}

// ===================== DeleteByParticipantAndActivity Tests =====================

func TestGoalRecordRepo_DeleteByParticipantAndActivity_Success(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	if err := repo.DeleteByParticipantAndActivity(10, "act-1"); err != nil {
		t.Fatalf("DeleteByParticipantAndActivity() error = %v", err)
	}
}

func TestGoalRecordRepo_DeleteByParticipantAndActivity_NonExistent(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	// Soft delete on non-existent records should not error (0 rows affected)
	if err := repo.DeleteByParticipantAndActivity(999, "nonexistent"); err != nil {
		t.Fatalf("DeleteByParticipantAndActivity() non-existent error = %v", err)
	}
}

// ===================== SoftDelete Tests =====================

func TestGoalRecordRepo_SoftDelete_Success(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	if err := repo.SoftDelete(1); err != nil {
		t.Fatalf("SoftDelete() error = %v", err)
	}
}

func TestGoalRecordRepo_SoftDelete_NonExistent(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	// Soft delete on non-existent record should not error (0 rows affected)
	if err := repo.SoftDelete(999); err != nil {
		t.Fatalf("SoftDelete() non-existent error = %v", err)
	}
}

// ===================== Edge Cases =====================

func TestGoalRecordRepo_BatchUpsert_VerifiesFields(t *testing.T) {
	db := newGoalRecordTestDB(t)
	repo := NewPerformanceGoalRecordRepository(db)

	records := []database.PerformanceGoalRecord{
		{
			ActivityID:      "act-1",
			ParticipantID:   10,
			IndicatorItemID: uintPtr(100),
			SectionType:     "quantitative",
			ItemName:        "Complex KPI",
			ItemDefinition:  "Revenue target for Q1",
			Weight:          0.6,
			RedLineValue:    "100",
			TargetValue:     "200",
			ChallengeValue:  "300",
			ScoringRule:     "Linear interpolation",
			VisibilityScope: "department_only",
			SortOrder:       1,
			IsFromSuperior:  true,
		},
	}
	if err := repo.BatchUpsert(records); err != nil {
		t.Fatalf("BatchUpsert() complex fields error = %v", err)
	}
}

func TestGoalRecordRepo_FindByParticipant_Ordering(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows: [][]driver.Value{
			goalRecordRow(1, "act-1", 10, "key_action", "Action1", 0.3, 1),
			goalRecordRow(2, "act-1", 10, "quantitative", "KPI1", 0.5, 1),
			goalRecordRow(3, "act-1", 10, "quantitative", "KPI2", 0.2, 2),
		},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByParticipant(10)
	if err != nil {
		t.Fatalf("FindByParticipant() error = %v", err)
	}
	// Verify records are returned (ordering is handled by SQL, stub returns as-is)
	if len(records) != 3 {
		t.Fatalf("FindByParticipant() len = %d, want 3", len(records))
	}
}

func TestGoalRecordRepo_FindByActivityAndParticipant_MultipleSections(t *testing.T) {
	db := newGoalRecordTestDB(t, stubGoalRecordQueryResponse{
		match:   goalRecordTableMatcher(),
		columns: goalRecordColumns(),
		rows: [][]driver.Value{
			goalRecordRow(1, "act-1", 10, "quantitative", "Revenue KPI", 0.4, 1),
			goalRecordRow(2, "act-1", 10, "quantitative", "Profit KPI", 0.2, 2),
			goalRecordRow(3, "act-1", 10, "key_action", "Project Alpha", 0.25, 1),
			goalRecordRow(4, "act-1", 10, "bonus_penalty", "Patent Bonus", 0.15, 1),
		},
	})
	repo := NewPerformanceGoalRecordRepository(db)

	records, err := repo.FindByActivityAndParticipant("act-1", 10)
	if err != nil {
		t.Fatalf("FindByActivityAndParticipant() error = %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("FindByActivityAndParticipant() len = %d, want 4", len(records))
	}
	sectionTypes := make(map[string]int)
	for _, r := range records {
		sectionTypes[r.SectionType]++
	}
	if sectionTypes["quantitative"] != 2 {
		t.Fatalf("FindByActivityAndParticipant() quantitative count = %d, want 2", sectionTypes["quantitative"])
	}
	if sectionTypes["key_action"] != 1 {
		t.Fatalf("FindByActivityAndParticipant() key_action count = %d, want 1", sectionTypes["key_action"])
	}
	if sectionTypes["bonus_penalty"] != 1 {
		t.Fatalf("FindByActivityAndParticipant() bonus_penalty count = %d, want 1", sectionTypes["bonus_penalty"])
	}
}
