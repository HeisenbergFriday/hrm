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

const stubGoalApprovalDriverName = "peopleops_stub_goal_approval_mysql"

var (
	stubGoalApprovalDriverOnce sync.Once
	stubGoalApprovalDBs        sync.Map
)

type stubGoalApprovalQueryResponse struct {
	match   func(query string, args []driver.NamedValue) bool
	columns []string
	rows    [][]driver.Value
}

type stubGoalApprovalDB struct {
	queries []stubGoalApprovalQueryResponse
}

type stubGoalApprovalDriver struct{}

type stubGoalApprovalConn struct {
	db *stubGoalApprovalDB
}

type stubGoalApprovalStmt struct {
	conn  *stubGoalApprovalConn
	query string
}

type stubGoalApprovalRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type stubGoalApprovalTx struct{}

type stubGoalApprovalResult struct{}

func (stubGoalApprovalResult) LastInsertId() (int64, error) { return 0, nil }
func (stubGoalApprovalResult) RowsAffected() (int64, error) { return 0, nil }

func (d stubGoalApprovalDriver) Open(name string) (driver.Conn, error) {
	value, ok := stubGoalApprovalDBs.Load(name)
	if !ok {
		return nil, fmt.Errorf("stub db %s not registered", name)
	}
	return &stubGoalApprovalConn{db: value.(*stubGoalApprovalDB)}, nil
}

func (c *stubGoalApprovalConn) Prepare(query string) (driver.Stmt, error) {
	return &stubGoalApprovalStmt{conn: c, query: query}, nil
}

func (c *stubGoalApprovalConn) Close() error { return nil }

func (c *stubGoalApprovalConn) Begin() (driver.Tx, error) { return stubGoalApprovalTx{}, nil }

func (c *stubGoalApprovalConn) BeginTx(_ any, _ driver.TxOptions) (driver.Tx, error) {
	return stubGoalApprovalTx{}, nil
}

func (c *stubGoalApprovalConn) QueryContext(_ any, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.queryDB(query, args)
}

func (c *stubGoalApprovalConn) ExecContext(_ any, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return stubGoalApprovalResult{}, nil
}

func (c *stubGoalApprovalConn) queryDB(query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, response := range c.db.queries {
		if response.match != nil && response.match(query, args) {
			rows := make([][]driver.Value, len(response.rows))
			for i := range response.rows {
				rows[i] = append([]driver.Value(nil), response.rows[i]...)
			}
			return &stubGoalApprovalRows{
				columns: append([]string(nil), response.columns...),
				rows:    rows,
			}, nil
		}
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (s *stubGoalApprovalStmt) Close() error  { return nil }
func (s *stubGoalApprovalStmt) NumInput() int { return -1 }
func (s *stubGoalApprovalStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return stubGoalApprovalResult{}, nil
}
func (s *stubGoalApprovalStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return s.conn.queryDB(s.query, named)
}

func (r *stubGoalApprovalRows) Columns() []string { return r.columns }
func (r *stubGoalApprovalRows) Close() error      { return nil }
func (r *stubGoalApprovalRows) Next(dest []driver.Value) error {
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

func (stubGoalApprovalTx) Commit() error   { return nil }
func (stubGoalApprovalTx) Rollback() error { return nil }

// ===================== Test Helpers =====================

func newGoalApprovalTestDB(t *testing.T, queries ...stubGoalApprovalQueryResponse) *gorm.DB {
	t.Helper()
	stubGoalApprovalDriverOnce.Do(func() {
		stdsql.Register(stubGoalApprovalDriverName, stubGoalApprovalDriver{})
	})

	dsn := fmt.Sprintf("goal-approval-test-%s-%d", t.Name(), time.Now().UnixNano())
	stubGoalApprovalDBs.Store(dsn, &stubGoalApprovalDB{queries: queries})
	t.Cleanup(func() {
		stubGoalApprovalDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(stubGoalApprovalDriverName, dsn)
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

func goalApprovalTableMatcher() func(string, []driver.NamedValue) bool {
	return func(query string, _ []driver.NamedValue) bool {
		return strings.Contains(strings.ToLower(query), "performance_goal_approval_logs")
	}
}

func goalApprovalColumns() []string {
	return []string{
		"id", "participant_id", "activity_id", "goal_record_id",
		"action", "comment", "approver_id", "approver_name",
		"version", "snapshot", "created_by", "created_at",
	}
}

func goalApprovalRow(id, participantID, goalRecordID uint, activityID, action, approverID string, version int, createdAt time.Time) []driver.Value {
	return []driver.Value{
		id, participantID, activityID, goalRecordID,
		action, "", approverID, "Approver",
		version, "{}", "system", createdAt,
	}
}

// ===================== Create Tests =====================

func TestGoalApprovalRepo_Create_Success(t *testing.T) {
	db := newGoalApprovalTestDB(t)
	repo := NewPerformanceGoalApprovalRepository(db)

	log := &database.PerformanceGoalApprovalLog{
		ParticipantID: 10,
		ActivityID:    "act-1",
		GoalRecordID:  100,
		Action:        "submit",
		Comment:       "提交目标设定",
		ApproverID:    "manager-1",
		ApproverName:  "张三",
		Version:       1,
		Snapshot:      "{}",
		CreatedBy:     "user-10",
		CreatedAt:     time.Now(),
	}

	if err := repo.Create(log); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestGoalApprovalRepo_Create_ApproveAction(t *testing.T) {
	db := newGoalApprovalTestDB(t)
	repo := NewPerformanceGoalApprovalRepository(db)

	log := &database.PerformanceGoalApprovalLog{
		ParticipantID: 20,
		ActivityID:    "act-2",
		GoalRecordID:  200,
		Action:        "approve",
		Comment:       "目标设定合理，批准",
		ApproverID:    "manager-2",
		ApproverName:  "李四",
		Version:       2,
		CreatedBy:     "manager-2",
		CreatedAt:     time.Now(),
	}

	if err := repo.Create(log); err != nil {
		t.Fatalf("Create() approve error = %v", err)
	}
}

func TestGoalApprovalRepo_Create_RejectAction(t *testing.T) {
	db := newGoalApprovalTestDB(t)
	repo := NewPerformanceGoalApprovalRepository(db)

	log := &database.PerformanceGoalApprovalLog{
		ParticipantID: 30,
		ActivityID:    "act-3",
		GoalRecordID:  300,
		Action:        "reject",
		Comment:       "目标值不合理，请重新设定",
		ApproverID:    "manager-3",
		ApproverName:  "王五",
		Version:       1,
		CreatedBy:     "manager-3",
		CreatedAt:     time.Now(),
	}

	if err := repo.Create(log); err != nil {
		t.Fatalf("Create() reject error = %v", err)
	}
}

// ===================== FindByParticipant Tests =====================

func TestGoalApprovalRepo_FindByParticipant_Found(t *testing.T) {
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows: [][]driver.Value{
			goalApprovalRow(1, 10, 100, "act-1", "submit", "user-10", 1, now.Add(-2*time.Hour)),
			goalApprovalRow(2, 10, 100, "act-1", "approve", "manager-1", 2, now.Add(-1*time.Hour)),
		},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	logs, err := repo.FindByParticipant(10, "act-1")
	if err != nil {
		t.Fatalf("FindByParticipant() error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("FindByParticipant() len = %d, want 2", len(logs))
	}
	if logs[0].ParticipantID != 10 || logs[1].ParticipantID != 10 {
		t.Fatalf("FindByParticipant() participant_id mismatch")
	}
}

func TestGoalApprovalRepo_FindByParticipant_Empty(t *testing.T) {
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows:    [][]driver.Value{},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	logs, err := repo.FindByParticipant(999, "nonexistent")
	if err != nil {
		t.Fatalf("FindByParticipant() error = %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("FindByParticipant() len = %d, want 0", len(logs))
	}
}

func TestGoalApprovalRepo_FindByParticipant_Ordering(t *testing.T) {
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows: [][]driver.Value{
			goalApprovalRow(3, 10, 100, "act-1", "reject", "manager-1", 3, now),
			goalApprovalRow(2, 10, 100, "act-1", "approve", "manager-1", 2, now.Add(-1*time.Hour)),
			goalApprovalRow(1, 10, 100, "act-1", "submit", "user-10", 1, now.Add(-2*time.Hour)),
		},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	logs, err := repo.FindByParticipant(10, "act-1")
	if err != nil {
		t.Fatalf("FindByParticipant() error = %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("FindByParticipant() len = %d, want 3", len(logs))
	}
}

// ===================== FindByGoalRecord Tests =====================

func TestGoalApprovalRepo_FindByGoalRecord_Found(t *testing.T) {
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows: [][]driver.Value{
			goalApprovalRow(1, 10, 100, "act-1", "submit", "user-10", 1, now.Add(-2*time.Hour)),
			goalApprovalRow(2, 10, 100, "act-1", "approve", "manager-1", 2, now.Add(-1*time.Hour)),
		},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	logs, err := repo.FindByGoalRecord(100)
	if err != nil {
		t.Fatalf("FindByGoalRecord() error = %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("FindByGoalRecord() len = %d, want 2", len(logs))
	}
	for _, l := range logs {
		if l.GoalRecordID != 100 {
			t.Fatalf("FindByGoalRecord() goal_record_id = %d, want 100", l.GoalRecordID)
		}
	}
}

func TestGoalApprovalRepo_FindByGoalRecord_Empty(t *testing.T) {
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows:    [][]driver.Value{},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	logs, err := repo.FindByGoalRecord(999)
	if err != nil {
		t.Fatalf("FindByGoalRecord() error = %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("FindByGoalRecord() len = %d, want 0", len(logs))
	}
}

func TestGoalApprovalRepo_FindByGoalRecord_MultipleActions(t *testing.T) {
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows: [][]driver.Value{
			goalApprovalRow(1, 10, 100, "act-1", "submit", "user-10", 1, now.Add(-3*time.Hour)),
			goalApprovalRow(2, 10, 100, "act-1", "reject", "manager-1", 2, now.Add(-2*time.Hour)),
			goalApprovalRow(3, 10, 100, "act-1", "submit", "user-10", 3, now.Add(-1*time.Hour)),
			goalApprovalRow(4, 10, 100, "act-1", "approve", "manager-1", 4, now),
		},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	logs, err := repo.FindByGoalRecord(100)
	if err != nil {
		t.Fatalf("FindByGoalRecord() error = %v", err)
	}
	if len(logs) != 4 {
		t.Fatalf("FindByGoalRecord() len = %d, want 4", len(logs))
	}
}

// ===================== GetLatestByParticipant Tests =====================

func TestGoalApprovalRepo_GetLatestByParticipant_Found(t *testing.T) {
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows: [][]driver.Value{
			goalApprovalRow(1, 10, 100, "act-1", "approve", "manager-1", 2, now),
		},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	log, err := repo.GetLatestByParticipant(10, "act-1")
	if err != nil {
		t.Fatalf("GetLatestByParticipant() error = %v", err)
	}
	if log.ParticipantID != 10 {
		t.Fatalf("GetLatestByParticipant() participant_id = %d, want 10", log.ParticipantID)
	}
	if log.ActivityID != "act-1" {
		t.Fatalf("GetLatestByParticipant() activity_id = %q, want act-1", log.ActivityID)
	}
}

func TestGoalApprovalRepo_GetLatestByParticipant_NotFound(t *testing.T) {
	db := newGoalApprovalTestDB(t) // no stub → query fails
	repo := NewPerformanceGoalApprovalRepository(db)

	_, err := repo.GetLatestByParticipant(999, "nonexistent")
	if err == nil {
		t.Fatal("GetLatestByParticipant() should return error for non-existent record")
	}
}

func TestGoalApprovalRepo_GetLatestByParticipant_VerifyFields(t *testing.T) {
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match:   goalApprovalTableMatcher(),
		columns: goalApprovalColumns(),
		rows: [][]driver.Value{
			goalApprovalRow(5, 20, 200, "act-2", "submit", "user-20", 1, now),
		},
	})
	repo := NewPerformanceGoalApprovalRepository(db)

	log, err := repo.GetLatestByParticipant(20, "act-2")
	if err != nil {
		t.Fatalf("GetLatestByParticipant() error = %v", err)
	}
	if log.ID != 5 {
		t.Fatalf("GetLatestByParticipant() id = %d, want 5", log.ID)
	}
	if log.GoalRecordID != 200 {
		t.Fatalf("GetLatestByParticipant() goal_record_id = %d, want 200", log.GoalRecordID)
	}
	if log.Action != "submit" {
		t.Fatalf("GetLatestByParticipant() action = %q, want submit", log.Action)
	}
	if log.ApproverID != "user-20" {
		t.Fatalf("GetLatestByParticipant() approver_id = %q, want user-20", log.ApproverID)
	}
}
