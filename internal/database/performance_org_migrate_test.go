package database

import (
	"context"
	stdsql "database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 迁移函数的 stub 驱动：验证软删除参与人也会修复、关联表同步、失败返回 error。

const migrateStubDriverName = "peopleops_migrate_stub_mysql"

var (
	migrateStubDriverOnce sync.Once
	migrateStubDBs        sync.Map
)

type migrateStubQuery struct {
	match   func(query string) bool
	columns []string
	rows    [][]driver.Value
	err     error
}

type migrateStubExec struct {
	match        func(query string) bool
	rowsAffected int64
	err          error
}

type migrateStubDB struct {
	mu         sync.Mutex
	queries    []migrateStubQuery
	execs      []migrateStubExec
	execCalls  []string
	beginCalls int
	commits    int
	rollbacks  int
}

type migrateStubDriver struct{}
type migrateStubConn struct{ db *migrateStubDB }
type migrateStubStmt struct {
	conn  *migrateStubConn
	query string
}
type migrateStubRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}
type migrateStubTx struct{ db *migrateStubDB }
type migrateStubResult struct{ n int64 }

func openMigrateStubDB(t *testing.T, stub *migrateStubDB) *gorm.DB {
	t.Helper()
	migrateStubDriverOnce.Do(func() {
		stdsql.Register(migrateStubDriverName, migrateStubDriver{})
	})
	dsn := fmt.Sprintf("migrate-%s-%d", t.Name(), time.Now().UnixNano())
	migrateStubDBs.Store(dsn, stub)
	t.Cleanup(func() { migrateStubDBs.Delete(dsn) })

	sqlDB, err := stdsql.Open(migrateStubDriverName, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}
	return db
}

func (migrateStubDriver) Open(name string) (driver.Conn, error) {
	v, ok := migrateStubDBs.Load(name)
	if !ok {
		return nil, fmt.Errorf("stub missing")
	}
	return &migrateStubConn{db: v.(*migrateStubDB)}, nil
}
func (c *migrateStubConn) Prepare(query string) (driver.Stmt, error) {
	return &migrateStubStmt{conn: c, query: query}, nil
}
func (c *migrateStubConn) Close() error { return nil }
func (c *migrateStubConn) Begin() (driver.Tx, error) {
	c.db.mu.Lock()
	c.db.beginCalls++
	c.db.mu.Unlock()
	return &migrateStubTx{db: c.db}, nil
}
func (c *migrateStubConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	return c.query(query)
}
func (c *migrateStubConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	return c.exec(query)
}
func (c *migrateStubConn) query(query string) (driver.Rows, error) {
	// information_schema / HasTable 探测
	lower := strings.ToLower(query)
	if strings.Contains(lower, "information_schema") || strings.Contains(lower, "show tables") || strings.Contains(lower, "sqlite_master") {
		return &migrateStubRows{columns: []string{"count"}, rows: [][]driver.Value{{int64(1)}}}, nil
	}
	for _, q := range c.db.queries {
		if q.match != nil && q.match(query) {
			if q.err != nil {
				return nil, q.err
			}
			rows := make([][]driver.Value, len(q.rows))
			for i := range q.rows {
				rows[i] = append([]driver.Value(nil), q.rows[i]...)
			}
			return &migrateStubRows{columns: append([]string(nil), q.columns...), rows: rows}, nil
		}
	}
	// default count 0
	if strings.Contains(lower, "count(*)") {
		return &migrateStubRows{columns: []string{"cnt"}, rows: [][]driver.Value{{int64(0)}}}, nil
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}
func (c *migrateStubConn) exec(query string) (driver.Result, error) {
	c.db.mu.Lock()
	c.db.execCalls = append(c.db.execCalls, query)
	c.db.mu.Unlock()
	for _, e := range c.db.execs {
		if e.match != nil && e.match(query) {
			if e.err != nil {
				return nil, e.err
			}
			return migrateStubResult{n: e.rowsAffected}, nil
		}
	}
	return migrateStubResult{n: 1}, nil
}
func (s *migrateStubStmt) Close() error  { return nil }
func (s *migrateStubStmt) NumInput() int { return -1 }
func (s *migrateStubStmt) Exec(args []driver.Value) (driver.Result, error) {
	return s.conn.exec(s.query)
}
func (s *migrateStubStmt) Query(args []driver.Value) (driver.Rows, error) {
	return s.conn.query(s.query)
}
func (r *migrateStubRows) Columns() []string { return r.columns }
func (r *migrateStubRows) Close() error      { return nil }
func (r *migrateStubRows) Next(dest []driver.Value) error {
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
func (tx *migrateStubTx) Commit() error {
	tx.db.mu.Lock()
	tx.db.commits++
	tx.db.mu.Unlock()
	return nil
}
func (tx *migrateStubTx) Rollback() error {
	tx.db.mu.Lock()
	tx.db.rollbacks++
	tx.db.mu.Unlock()
	return nil
}
func (r migrateStubResult) LastInsertId() (int64, error) { return 1, nil }
func (r migrateStubResult) RowsAffected() (int64, error) { return r.n, nil }

func TestMigratePerformanceParticipantOrgIDsIncludesSoftDeleted(t *testing.T) {
	participantUpdateSeen := false
	logUpdateSeen := false
	versionUpdateSeen := false
	participantSQLHasNoDeletedFilter := false
	participantCountUsesBinaryActivityIDComparison := false
	participantUpdateUsesBinaryActivityIDComparison := false

	stub := &migrateStubDB{
		queries: []migrateStubQuery{
			{
				match: func(q string) bool {
					lower := strings.ToLower(q)
					if strings.Contains(lower, "count(*)") && strings.Contains(lower, "from performance_participants p") {
						participantCountUsesBinaryActivityIDComparison =
							strings.Contains(lower, "cast(a.id as binary) = cast(p.activity_id as binary)") &&
								!strings.Contains(lower, "cast(a.id as char)")
						return true
					}
					return false
				},
				columns: []string{"cnt"},
				rows:    [][]driver.Value{{int64(2)}},
			},
		},
		execs: []migrateStubExec{
			{
				match: func(q string) bool {
					lower := strings.ToLower(q)
					return strings.Contains(lower, "update performance_participants") ||
						(strings.Contains(lower, "update") && strings.Contains(lower, "performance_participants") && strings.Contains(lower, "set"))
				},
				rowsAffected: 2,
			},
			{
				match: func(q string) bool {
					return strings.Contains(strings.ToLower(q), "performance_relationship_change_logs")
				},
				rowsAffected: 1,
			},
			{
				match: func(q string) bool {
					return strings.Contains(strings.ToLower(q), "performance_review_versions")
				},
				rowsAffected: 1,
			},
		},
	}
	// wrap execs to record soft-delete filter absence
	origExecs := stub.execs
	stub.execs = []migrateStubExec{
		{
			match: func(q string) bool {
				lower := strings.ToLower(q)
				if strings.Contains(lower, "update performance_participants") {
					participantUpdateSeen = true
					participantUpdateUsesBinaryActivityIDComparison =
						strings.Contains(lower, "cast(a.id as binary) = cast(p.activity_id as binary)") &&
							!strings.Contains(lower, "cast(a.id as char)")
					// 关键：不得再过滤 p.deleted_at IS NULL
					if !strings.Contains(lower, "p.deleted_at is null") {
						participantSQLHasNoDeletedFilter = true
					}
					return true
				}
				return false
			},
			rowsAffected: 2,
		},
		{
			match: func(q string) bool {
				if strings.Contains(strings.ToLower(q), "relationship_change_logs") {
					logUpdateSeen = true
					return true
				}
				return false
			},
			rowsAffected: 1,
		},
		{
			match: func(q string) bool {
				if strings.Contains(strings.ToLower(q), "review_versions") {
					versionUpdateSeen = true
					return true
				}
				return false
			},
			rowsAffected: 1,
		},
	}
	_ = origExecs

	db := openMigrateStubDB(t, stub)
	if err := MigratePerformanceParticipantOrgIDsFromActivity(db); err != nil {
		t.Fatalf("migrate error: %v", err)
	}
	if !participantUpdateSeen {
		t.Fatalf("expected performance_participants UPDATE; execCalls=%v", stub.execCalls)
	}
	if !participantSQLHasNoDeletedFilter {
		t.Fatalf("participant repair SQL must NOT contain deleted_at IS NULL; execCalls=%v", stub.execCalls)
	}
	if !participantCountUsesBinaryActivityIDComparison {
		t.Fatalf("participant count SQL must compare activity IDs without string collation")
	}
	if !participantUpdateUsesBinaryActivityIDComparison {
		t.Fatalf("participant update SQL must compare activity IDs without string collation; execCalls=%v", stub.execCalls)
	}
	if !logUpdateSeen {
		t.Fatalf("expected performance_relationship_change_logs UPDATE; execCalls=%v", stub.execCalls)
	}
	if !versionUpdateSeen {
		t.Fatalf("expected performance_review_versions UPDATE; execCalls=%v", stub.execCalls)
	}
	if stub.beginCalls == 0 {
		t.Fatalf("expected transaction begin")
	}
	if stub.commits == 0 {
		t.Fatalf("expected commit after successful migration; rollbacks=%d", stub.rollbacks)
	}
}

func TestMigratePerformanceParticipantOrgIDsFailsAndRollsBackOnRelatedError(t *testing.T) {
	stub := &migrateStubDB{
		queries: []migrateStubQuery{
			{
				match: func(q string) bool {
					return strings.Contains(strings.ToLower(q), "count(*)")
				},
				columns: []string{"cnt"},
				rows:    [][]driver.Value{{int64(1)}},
			},
		},
		execs: []migrateStubExec{
			// 仅匹配“修参与人本身”的 UPDATE；关联表 SQL 也会 JOIN participants，不能误伤。
			{
				match: func(q string) bool {
					lower := strings.ToLower(q)
					return strings.Contains(lower, "update performance_participants") ||
						(strings.Contains(lower, "update") &&
							strings.Contains(lower, "performance_participants p") &&
							strings.Contains(lower, "set p.org_id") &&
							!strings.Contains(lower, "relationship_change_logs") &&
							!strings.Contains(lower, "review_versions"))
				},
				rowsAffected: 1,
			},
			// 关联表更新失败必须返回 error 并回滚（不吞错）。
			{
				match: func(q string) bool {
					lower := strings.ToLower(q)
					return strings.Contains(lower, "relationship_change_logs") ||
						strings.Contains(lower, "review_versions")
				},
				err: fmt.Errorf("simulated related org_id update failure"),
			},
		},
	}
	db := openMigrateStubDB(t, stub)
	err := MigratePerformanceParticipantOrgIDsFromActivity(db)
	if err == nil {
		t.Fatalf("related table error must surface; begin=%d commit=%d rollback=%d execCalls=%v",
			stub.beginCalls, stub.commits, stub.rollbacks, stub.execCalls)
	}
	if stub.beginCalls > 0 && stub.rollbacks == 0 {
		t.Fatalf("expected rollback on related update failure; commits=%d", stub.commits)
	}
	if stub.commits != 0 {
		t.Fatalf("must not commit when related update fails")
	}
}

func TestMigratePerformanceParticipantOrgIDsIdempotentWhenAligned(t *testing.T) {
	stub := &migrateStubDB{
		queries: []migrateStubQuery{
			{
				match: func(q string) bool {
					return strings.Contains(strings.ToLower(q), "count(*)")
				},
				columns: []string{"cnt"},
				rows:    [][]driver.Value{{int64(0)}},
			},
		},
	}
	db := openMigrateStubDB(t, stub)
	if err := MigratePerformanceParticipantOrgIDsFromActivity(db); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// 再次执行仍成功（幂等）
	if err := MigratePerformanceParticipantOrgIDsFromActivity(db); err != nil {
		t.Fatalf("second run: %v", err)
	}
}
