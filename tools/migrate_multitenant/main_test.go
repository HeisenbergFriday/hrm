package main

import (
	"bytes"
	stdsql "database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"peopleops/internal/tenant/registry"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRunUnknownSubcommand(t *testing.T) {
	err := runReport([]string{"--dsn", ""}, io.Discard)
	if err == nil {
		t.Fatal("expected error for empty DSN")
	}
}

func TestDiscover_ReportsSchemaSnapshotForThreeOrgLikeTables(t *testing.T) {
	db := newMigrateStubDB(t, func(q string, args []driver.Value) (columns []string, rows [][]driver.Value, ok bool) {
		lq := strings.ToLower(q)
		switch {
		case strings.Contains(lq, "select database()"):
			return []string{"DATABASE()"}, [][]driver.Value{{"peopleops_test"}}, true
		case strings.Contains(lq, "information_schema.tables") && strings.Contains(lq, "table_name = ?"):
			table := args[0].(string)
			// 只让 users/organizations/organization_users/approvals/performance_activities/roles 六张表"存在"，
			// 其它都返回 0 —— 用来模拟数据库还没有 schema expand 时的状态。
			existing := map[string]bool{
				"users":                  true,
				"departments":            true,
				"organizations":          true,
				"organization_users":     true,
				"approvals":              true,
				"performance_activities": true,
				"roles":                  true,
			}
			cnt := int64(0)
			if existing[table] {
				cnt = 1
			}
			return []string{"COUNT(*)"}, [][]driver.Value{{cnt}}, true
		case strings.Contains(lq, "information_schema.columns") && strings.Contains(lq, "column_name = 'org_id'"):
			table := args[0].(string)
			// users/departments/organization_users/approvals 假设已经有 org_id 列，其中 approvals 是 nullable。
			// performance_activities 尚未加列，返回空结果。
			switch table {
			case "users", "departments", "organization_users":
				return []string{"IS_NULLABLE"}, [][]driver.Value{{"NO"}}, true
			case "approvals":
				return []string{"IS_NULLABLE"}, [][]driver.Value{{"YES"}}, true
			}
			return []string{"IS_NULLABLE"}, [][]driver.Value{}, true
		case strings.Contains(lq, "information_schema.statistics"):
			table := args[0].(string)
			if table == "users" {
				return []string{"INDEX_NAME"}, [][]driver.Value{{"idx_org_user_id"}}, true
			}
			return []string{"INDEX_NAME"}, [][]driver.Value{}, true
		case strings.Contains(lq, "count(*) from `approvals` where org_id is null"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(42)}}, true
		case strings.Contains(lq, "count(*) from `approvals`"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(100)}}, true
		case strings.Contains(lq, "count(*) from `users` where org_id is null"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
		case strings.Contains(lq, "count(*) from `users`"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(3)}}, true
		case strings.Contains(lq, "count(*) from `departments`") && strings.Contains(lq, "where"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
		case strings.Contains(lq, "count(*) from `departments`"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(30)}}, true
		case strings.Contains(lq, "count(*) from `organization_users`") && strings.Contains(lq, "where"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
		case strings.Contains(lq, "count(*) from `organization_users`"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(5)}}, true
		case strings.Contains(lq, "count(*) from `performance_activities`"):
			// perf activities 存在但尚无 org_id 列；只需要返回总行数，null_org 不查。
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(7)}}, true
		case strings.Contains(lq, "count(*) from `roles`"):
			// roles 是平台表；Discover 不查它的 COUNT，此分支只是防御。
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(4)}}, true
		}
		return nil, nil, false
	})

	report, err := Discover(db)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if report.Database != "peopleops_test" {
		t.Fatalf("Database = %q, want peopleops_test", report.Database)
	}
	byName := map[string]TableFinding{}
	for _, f := range report.Tables {
		byName[f.Name] = f
	}

	users := byName["users"]
	if !users.Exists || !users.HasOrgIDCol || users.OrgIDNullable {
		t.Fatalf("users finding unexpected: %#v", users)
	}
	if users.RowCount != 3 {
		t.Fatalf("users row count = %d, want 3", users.RowCount)
	}

	approvals := byName["approvals"]
	if !approvals.Exists || !approvals.HasOrgIDCol || !approvals.OrgIDNullable {
		t.Fatalf("approvals finding unexpected: %#v", approvals)
	}
	if approvals.NullOrgRows != 42 {
		t.Fatalf("approvals null org rows = %d, want 42", approvals.NullOrgRows)
	}

	if got := byName["performance_activities"]; !got.Exists || got.HasOrgIDCol {
		t.Fatalf("performance_activities: exists=%v hasOrg=%v (want exists=true hasOrg=false)", got.Exists, got.HasOrgIDCol)
	}

	if got := byName["performance_reviews"]; got.Exists {
		t.Fatalf("performance_reviews should be absent, got %#v", got)
	}

	if got := byName["organizations"]; got.HasOrgIDCol {
		t.Fatalf("organizations is platform-global and must NOT report org_id column")
	}

	// Summary 交叉校验：注册表中所有 tenant 表都被计数了。
	totalTenants := len(registry.TenantTables())
	if report.Summary.TotalTenantTables != totalTenants {
		t.Fatalf("summary.TotalTenantTables = %d, want %d", report.Summary.TotalTenantTables, totalTenants)
	}
	if report.Summary.TotalNullOrgRows != 42 {
		t.Fatalf("summary.TotalNullOrgRows = %d, want 42", report.Summary.TotalNullOrgRows)
	}
}

func TestReport_WritesJSONToWriter(t *testing.T) {
	db := newMigrateStubDB(t, func(q string, _ []driver.Value) (columns []string, rows [][]driver.Value, ok bool) {
		lq := strings.ToLower(q)
		switch {
		case strings.Contains(lq, "select database()"):
			return []string{"DATABASE()"}, [][]driver.Value{{"peopleops_test"}}, true
		case strings.Contains(lq, "information_schema.tables"):
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
		}
		return nil, nil, false
	})

	report, err := Discover(db)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	var buf bytes.Buffer
	if err := writeJSON(&buf, report, true); err != nil {
		t.Fatalf("writeJSON: %v", err)
	}
	var decoded DiscoverReport
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("unmarshal report: %v; body=%s", err, buf.String())
	}
	if decoded.Database != "peopleops_test" {
		t.Fatalf("decoded db = %q", decoded.Database)
	}
	// 所有表在模拟数据库中都不存在，Missing 计数应等于 tenant 表总数。
	totalTenants := len(registry.TenantTables())
	if decoded.Summary.MissingTablesInDatabase != totalTenants {
		t.Fatalf("MissingTablesInDatabase = %d, want %d", decoded.Summary.MissingTablesInDatabase, totalTenants)
	}
}

func TestInfer_BuildsReviewedManifestWithoutDefaultFallback(t *testing.T) {
	db := newMigrateStubDB(t, inferFixtureHandler)

	manifest, err := Infer(db, 100)
	if err != nil {
		t.Fatalf("Infer: %v", err)
	}

	var ready, ambiguous BackfillEntry
	for _, entry := range manifest.Entries {
		if entry.Table == "approvals" && entry.PKValue == "10" {
			ready = entry
		}
		if entry.Table == "approvals" && entry.PKValue == "11" {
			ambiguous = entry
		}
	}
	if ready.Status != entryReady || ready.OrgID != "muteng" {
		t.Fatalf("ready entry unexpected: %#v", ready)
	}
	if ambiguous.Status != entryUnresolved || len(ambiguous.Candidates) != 2 {
		t.Fatalf("ambiguous entry unexpected: %#v", ambiguous)
	}
	if manifest.Summary.ReadyRows != 1 || manifest.Summary.UnresolvedRows != 1 {
		t.Fatalf("summary unexpected: %#v", manifest.Summary)
	}
}

func TestApplyManifest_OnlyMatchesReviewedReadyEntries(t *testing.T) {
	db := newMigrateStubDB(t, inferFixtureHandler)
	manifest := InferenceManifest{Entries: []BackfillEntry{
		{Table: "approvals", PKColumn: "id", PKValue: "10", OrgID: "muteng", Status: entryReady},
		{Table: "approvals", PKColumn: "id", PKValue: "11", Status: entryUnresolved},
	}}

	report, err := ApplyManifest(db, manifest, true)
	if err != nil {
		t.Fatalf("ApplyManifest: %v", err)
	}
	if report.Matched != 1 || report.Skipped != 1 || report.Updated != 0 || !report.DryRun {
		t.Fatalf("apply report unexpected: %#v", report)
	}
}

func TestVerify_FailsOnEmptyOrgRows(t *testing.T) {
	db := newMigrateStubDB(t, inferFixtureHandler)

	report, err := Verify(db)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if report.Passed {
		t.Fatalf("verify should fail when approvals has empty org rows: %#v", report)
	}
	if !hasIssue(report.Issues, "approvals", "empty_org_id") {
		t.Fatalf("expected approvals empty_org_id issue, got %#v", report.Issues)
	}
}

func TestContractStatements_PlansNotNullAndIndexes(t *testing.T) {
	db := newMigrateStubDB(t, contractFixtureHandler)

	statements, err := contractStatements(db)
	if err != nil {
		t.Fatalf("contractStatements: %v", err)
	}
	joined := strings.Join(statements, "\n")
	if !strings.Contains(joined, "ALTER TABLE `users` MODIFY COLUMN `org_id` VARCHAR(64) NOT NULL") {
		t.Fatalf("expected users NOT NULL statement, got:\n%s", joined)
	}
	if !strings.Contains(joined, "CREATE UNIQUE INDEX `idx_org_user_id` ON `users` (`org_id`, `user_id`)") {
		t.Fatalf("expected users unique index statement, got:\n%s", joined)
	}
}

func hasIssue(issues []VerificationIssue, table, code string) bool {
	for _, issue := range issues {
		if issue.Table == table && issue.Code == code {
			return true
		}
	}
	return false
}

func inferFixtureHandler(q string, args []driver.Value) (columns []string, rows [][]driver.Value, ok bool) {
	lq := strings.ToLower(q)
	switch {
	case strings.Contains(lq, "select database()"):
		return []string{"DATABASE()"}, [][]driver.Value{{"peopleops_test"}}, true
	case strings.Contains(lq, "information_schema.tables") && strings.Contains(lq, "table_name = ?"):
		table := args[0].(string)
		existing := map[string]bool{"users": true, "departments": true, "approvals": true}
		if existing[table] {
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(1)}}, true
		}
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
	case strings.Contains(lq, "information_schema.columns") && strings.Contains(lq, "column_name = 'org_id'"):
		return []string{"IS_NULLABLE"}, [][]driver.Value{{"YES"}}, true
	case strings.Contains(lq, "information_schema.columns"):
		table := args[0].(string)
		switch table {
		case "users":
			return []string{"COLUMN_NAME"}, [][]driver.Value{{"id"}, {"org_id"}, {"user_id"}}, true
		case "departments":
			return []string{"COLUMN_NAME"}, [][]driver.Value{{"id"}, {"org_id"}, {"department_id"}}, true
		case "approvals":
			return []string{"COLUMN_NAME"}, [][]driver.Value{{"id"}, {"org_id"}, {"applicant_id"}}, true
		default:
			return []string{"COLUMN_NAME"}, [][]driver.Value{}, true
		}
	case strings.Contains(lq, "information_schema.statistics") && strings.Contains(lq, "column_name = 'org_id'"):
		return []string{"INDEX_NAME"}, [][]driver.Value{{"idx_org_id"}}, true
	case strings.Contains(lq, "count(*) from `approvals` where org_id is null"):
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(2)}}, true
	case strings.Contains(lq, "count(*) from `approvals`"):
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(2)}}, true
	case strings.Contains(lq, "count(*) from `users` where org_id is null"):
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
	case strings.Contains(lq, "count(*) from `users`"):
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(3)}}, true
	case strings.Contains(lq, "count(*) from `departments` where org_id is null"):
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
	case strings.Contains(lq, "count(*) from `departments`"):
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(1)}}, true
	case strings.Contains(lq, "select `id`, `applicant_id` from `approvals`"):
		return []string{"id", "applicant_id"}, [][]driver.Value{{int64(10), "alice"}, {int64(11), "bob"}}, true
	case strings.Contains(lq, "select `id`, `user_id` from `users`"):
		return []string{"id", "user_id"}, [][]driver.Value{}, true
	case strings.Contains(lq, "select `department_id`, `id` from `departments`") || strings.Contains(lq, "select `id`, `department_id` from `departments`"):
		return []string{"id", "department_id"}, [][]driver.Value{}, true
	case strings.Contains(lq, "select distinct org_id from `users` where `user_id` = ?"):
		userID := args[0].(string)
		if userID == "alice" {
			return []string{"org_id"}, [][]driver.Value{{"muteng"}}, true
		}
		if userID == "bob" {
			return []string{"org_id"}, [][]driver.Value{{"muteng"}, {"xiaotie"}}, true
		}
		return []string{"org_id"}, [][]driver.Value{}, true
	case strings.Contains(lq, "select distinct org_id from `organization_users`"):
		return []string{"org_id"}, [][]driver.Value{}, true
	}
	if strings.Contains(lq, " join ") && strings.Contains(lq, "count(*)") {
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
	}
	return nil, nil, false
}

func contractFixtureHandler(q string, args []driver.Value) (columns []string, rows [][]driver.Value, ok bool) {
	lq := strings.ToLower(q)
	switch {
	case strings.Contains(lq, "information_schema.tables") && strings.Contains(lq, "table_name = ?"):
		table := args[0].(string)
		if table == "users" {
			return []string{"COUNT(*)"}, [][]driver.Value{{int64(1)}}, true
		}
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
	case strings.Contains(lq, "information_schema.columns") && strings.Contains(lq, "column_name = 'org_id'"):
		return []string{"IS_NULLABLE"}, [][]driver.Value{{"YES"}}, true
	case strings.Contains(lq, "information_schema.columns"):
		return []string{"COLUMN_NAME"}, [][]driver.Value{{"id"}, {"org_id"}, {"user_id"}}, true
	case strings.Contains(lq, "information_schema.statistics") && strings.Contains(lq, "index_name = ?"):
		return []string{"COUNT(*)"}, [][]driver.Value{{int64(0)}}, true
	case strings.Contains(lq, "information_schema.statistics") && strings.Contains(lq, "column_name = 'org_id'"):
		return []string{"INDEX_NAME"}, [][]driver.Value{}, true
	}
	return nil, nil, false
}

// ============================== stub driver ==============================

var migrateStubOnce sync.Once
var migrateStubs sync.Map

const migrateStubDriverName = "peopleops_migrate_stub"

type migrateStubHandler func(query string, args []driver.Value) (columns []string, rows [][]driver.Value, ok bool)

type migrateStubDB struct {
	handler migrateStubHandler
}

func newMigrateStubDB(t *testing.T, handler migrateStubHandler) *gorm.DB {
	t.Helper()
	migrateStubOnce.Do(func() {
		stdsql.Register(migrateStubDriverName, migrateStubDriver{})
	})
	dsn := "migrate-stub-" + t.Name()
	migrateStubs.Store(dsn, &migrateStubDB{handler: handler})
	t.Cleanup(func() { migrateStubs.Delete(dsn) })

	sqlDB, err := stdsql.Open(migrateStubDriverName, dsn)
	if err != nil {
		t.Fatalf("open stub db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true, Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db
}

type migrateStubDriver struct{}

func (migrateStubDriver) Open(name string) (driver.Conn, error) {
	v, ok := migrateStubs.Load(name)
	if !ok {
		return nil, errors.New("stub db not found: " + name)
	}
	return &migrateStubConn{db: v.(*migrateStubDB)}, nil
}

type migrateStubConn struct{ db *migrateStubDB }
type migrateStubStmt struct {
	conn  *migrateStubConn
	query string
}
type migrateStubTx struct{}

func (c *migrateStubConn) Prepare(query string) (driver.Stmt, error) {
	return &migrateStubStmt{conn: c, query: query}, nil
}
func (c *migrateStubConn) Close() error              { return nil }
func (c *migrateStubConn) Begin() (driver.Tx, error) { return migrateStubTx{}, nil }

func (s *migrateStubStmt) Close() error  { return nil }
func (s *migrateStubStmt) NumInput() int { return -1 }
func (s *migrateStubStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return migrateStubResult{}, nil
}
func (s *migrateStubStmt) Query(args []driver.Value) (driver.Rows, error) {
	columns, rows, ok := s.conn.db.handler(s.query, args)
	if !ok {
		return nil, errors.New("stub: unmapped query: " + s.query)
	}
	return &migrateStubRows{cols: columns, rows: rows}, nil
}

type migrateStubRows struct {
	cols []string
	rows [][]driver.Value
	idx  int
}

func (r *migrateStubRows) Columns() []string { return r.cols }
func (r *migrateStubRows) Close() error      { return nil }
func (r *migrateStubRows) Next(dest []driver.Value) error {
	if r.idx >= len(r.rows) {
		return io.EOF
	}
	row := r.rows[r.idx]
	r.idx++
	for i := range dest {
		dest[i] = nil
		if i < len(row) {
			dest[i] = row[i]
		}
	}
	return nil
}

type migrateStubResult struct{}

func (migrateStubResult) LastInsertId() (int64, error) { return 0, nil }
func (migrateStubResult) RowsAffected() (int64, error) { return 0, nil }

func (migrateStubTx) Commit() error   { return nil }
func (migrateStubTx) Rollback() error { return nil }
