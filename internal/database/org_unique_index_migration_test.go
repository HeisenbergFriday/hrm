package database

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

const orgUniqueTestDriverName = "peopleops_org_unique_test_mysql"

var (
	orgUniqueTestDriverOnce sync.Once
	orgUniqueTestStates     sync.Map
)

type orgUniqueTestState struct {
	mu     sync.Mutex
	tables map[string]map[string]bool
	// nullableCols: table -> column -> true if nullable. Missing defaults to true.
	nullableCols map[string]map[string]bool
	// columnTypes: table -> column -> MySQL COLUMN_TYPE (default varchar(64)).
	columnTypes       map[string]map[string]string
	indexes           map[string]map[string]uniqueIndexDefinition
	conflicts         map[string][][]driver.Value
	conflictsAfterOrg map[string][][]driver.Value
	orgBackfilled     map[string]bool
	emptyNormalized   map[string]map[string]bool // table -> column normalized ''->NULL
	execs             []string
	queries           []string
	failExecContains  string
}

type orgUniqueTestDriver struct{}
type orgUniqueTestConn struct{ state *orgUniqueTestState }
type orgUniqueTestRows struct {
	columns []string
	rows    [][]driver.Value
	pos     int
}
type orgUniqueTestResult int64

func openOrgUniqueTestDB(t *testing.T, state *orgUniqueTestState) *gorm.DB {
	t.Helper()
	orgUniqueTestDriverOnce.Do(func() { sql.Register(orgUniqueTestDriverName, orgUniqueTestDriver{}) })
	if state.tables == nil {
		state.tables = make(map[string]map[string]bool)
	}
	if state.indexes == nil {
		state.indexes = make(map[string]map[string]uniqueIndexDefinition)
	}
	if state.conflicts == nil {
		state.conflicts = make(map[string][][]driver.Value)
	}
	if state.conflictsAfterOrg == nil {
		state.conflictsAfterOrg = make(map[string][][]driver.Value)
	}
	if state.orgBackfilled == nil {
		state.orgBackfilled = make(map[string]bool)
	}
	if state.nullableCols == nil {
		state.nullableCols = make(map[string]map[string]bool)
	}
	if state.columnTypes == nil {
		state.columnTypes = make(map[string]map[string]string)
	}
	if state.emptyNormalized == nil {
		state.emptyNormalized = make(map[string]map[string]bool)
	}
	dsn := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	orgUniqueTestStates.Store(dsn, state)
	t.Cleanup(func() { orgUniqueTestStates.Delete(dsn) })
	sqlDB, err := sql.Open(orgUniqueTestDriverName, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{Conn: sqlDB, SkipInitializeWithVersion: true}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func (orgUniqueTestDriver) Open(name string) (driver.Conn, error) {
	state, ok := orgUniqueTestStates.Load(name)
	if !ok {
		return nil, fmt.Errorf("missing org unique test state")
	}
	return &orgUniqueTestConn{state: state.(*orgUniqueTestState)}, nil
}

func (c *orgUniqueTestConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *orgUniqueTestConn) Close() error                        { return nil }
func (c *orgUniqueTestConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

func (c *orgUniqueTestConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.mu.Lock()
	c.state.queries = append(c.state.queries, query)
	c.state.mu.Unlock()
	lower := strings.ToLower(query)
	if strings.Contains(lower, "information_schema.tables") {
		table := namedString(args, 0)
		_, ok := c.state.tables[table]
		return testRows([]string{"count"}, [][]driver.Value{{boolCount(ok)}}), nil
	}
	if strings.Contains(lower, "information_schema.columns") && strings.Contains(lower, "count(*)") {
		table, column := namedString(args, 0), namedString(args, 1)
		return testRows([]string{"count"}, [][]driver.Value{{boolCount(c.state.tables[table][column])}}), nil
	}
	if strings.Contains(lower, "information_schema.columns") && strings.Contains(lower, "is_nullable") {
		table, column := namedString(args, 0), namedString(args, 1)
		nullable := "YES"
		if c.state.nullableCols[table] != nil {
			if v, ok := c.state.nullableCols[table][column]; ok && !v {
				nullable = "NO"
			}
		}
		colType := "varchar(64)"
		if c.state.columnTypes[table] != nil {
			if v := c.state.columnTypes[table][column]; v != "" {
				colType = v
			}
		}
		return testRows([]string{"is_nullable", "column_type"}, [][]driver.Value{{nullable, colType}}), nil
	}
	if strings.Contains(lower, "information_schema.statistics") {
		table := namedString(args, 0)
		defs := c.indexDefinitions(table)
		rows := make([][]driver.Value, 0)
		for _, def := range defs {
			for i, column := range def.Columns {
				nonUnique := int64(1)
				if def.Unique {
					nonUnique = 0
				}
				rows = append(rows, []driver.Value{def.Name, column, nonUnique, int64(i + 1)})
			}
		}
		return testRows([]string{"index_name", "column_name", "non_unique", "seq"}, rows), nil
	}
	if strings.Contains(lower, "having count(*) > 1") {
		table := tableAfterFrom(query)
		rows := c.state.conflicts[table]
		if c.state.orgBackfilled[table] && c.state.conflictsAfterOrg[table] != nil {
			rows = c.state.conflictsAfterOrg[table]
		}
		columns := selectedColumnNames(query)
		if len(rows) > 0 {
			columns = make([]string, len(rows[0]))
			for i := range columns {
				columns[i] = fmt.Sprintf("c%d", i)
			}
		}
		return testRows(columns, rows), nil
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (c *orgUniqueTestConn) ExecContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Result, error) {
	c.state.mu.Lock()
	defer c.state.mu.Unlock()
	c.state.execs = append(c.state.execs, query)
	lower := strings.ToLower(query)
	if c.state.failExecContains != "" && strings.Contains(lower, strings.ToLower(c.state.failExecContains)) {
		return nil, fmt.Errorf("forced exec failure for %s", c.state.failExecContains)
	}
	if strings.HasPrefix(strings.TrimSpace(lower), "update ") && strings.Contains(lower, "set `org_id` = 'default'") {
		table := firstBacktickValue(query)
		c.state.orgBackfilled[table] = true
		return orgUniqueTestResult(1), nil
	}
	if strings.HasPrefix(strings.TrimSpace(lower), "update ") && strings.Contains(lower, " = null") && strings.Contains(lower, "trim(") {
		table := firstBacktickValue(query)
		colMatch := regexp.MustCompile(`(?i)SET\s+` + "`" + `([^` + "`" + `]+)` + "`" + `\s*=\s*NULL`).FindStringSubmatch(query)
		if len(colMatch) == 2 {
			if c.state.emptyNormalized[table] == nil {
				c.state.emptyNormalized[table] = make(map[string]bool)
			}
			c.state.emptyNormalized[table][colMatch[1]] = true
		}
		return orgUniqueTestResult(1), nil
	}
	if strings.HasPrefix(strings.TrimSpace(lower), "alter table") {
		table := firstBacktickValue(query)
		addColumnRE := regexp.MustCompile(`(?i)ADD COLUMN\s+` + "`" + `([^` + "`" + `]+)` + "`")
		if match := addColumnRE.FindStringSubmatch(query); len(match) == 2 {
			if c.state.tables[table] == nil {
				c.state.tables[table] = make(map[string]bool)
			}
			c.state.tables[table][match[1]] = true
			return orgUniqueTestResult(1), nil
		}
		modifyRE := regexp.MustCompile(`(?i)MODIFY COLUMN\s+` + "`" + `([^` + "`" + `]+)` + "`")
		if match := modifyRE.FindStringSubmatch(query); len(match) == 2 {
			if c.state.nullableCols[table] == nil {
				c.state.nullableCols[table] = make(map[string]bool)
			}
			c.state.nullableCols[table][match[1]] = true
			return orgUniqueTestResult(1), nil
		}
		if c.state.indexes[table] == nil {
			c.state.indexes[table] = make(map[string]uniqueIndexDefinition)
		}
		for _, name := range regexp.MustCompile(`(?i)DROP INDEX\s+`+"`"+`([^`+"`"+`]+)`+"`").FindAllStringSubmatch(query, -1) {
			delete(c.state.indexes[table], name[1])
		}
		addRE := regexp.MustCompile(`(?i)ADD\s+(UNIQUE\s+)?INDEX\s+` + "`" + `([^` + "`" + `]+)` + "`" + `\s*\(([^)]*)\)`)
		for _, match := range addRE.FindAllStringSubmatch(query, -1) {
			cols := regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(match[3], -1)
			columns := make([]string, 0, len(cols))
			for _, col := range cols {
				columns = append(columns, col[1])
			}
			c.state.indexes[table][match[2]] = uniqueIndexDefinition{Name: match[2], Columns: columns, Unique: strings.TrimSpace(match[1]) != ""}
		}
		return orgUniqueTestResult(1), nil
	}
	return orgUniqueTestResult(1), nil
}

func (c *orgUniqueTestConn) indexDefinitions(table string) []uniqueIndexDefinition {
	defs := make([]uniqueIndexDefinition, 0, len(c.state.indexes[table]))
	for _, def := range c.state.indexes[table] {
		defs = append(defs, def)
	}
	sort.Slice(defs, func(i, j int) bool { return defs[i].Name < defs[j].Name })
	return defs
}

func testRows(columns []string, rows [][]driver.Value) driver.Rows {
	return &orgUniqueTestRows{columns: columns, rows: rows}
}
func (r *orgUniqueTestRows) Columns() []string { return r.columns }
func (r *orgUniqueTestRows) Close() error      { return nil }
func (r *orgUniqueTestRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}
func (r orgUniqueTestResult) LastInsertId() (int64, error) { return 0, nil }
func (r orgUniqueTestResult) RowsAffected() (int64, error) { return int64(r), nil }

func namedString(args []driver.NamedValue, index int) string {
	if index >= len(args) {
		return ""
	}
	return fmt.Sprint(args[index].Value)
}
func boolCount(value bool) int64 {
	if value {
		return 1
	}
	return 0
}
func firstBacktickValue(query string) string {
	match := regexp.MustCompile("`([^`]+)`").FindStringSubmatch(query)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
func tableAfterFrom(query string) string {
	match := regexp.MustCompile(`(?i)FROM\s+` + "`" + `([^` + "`" + `]+)` + "`").FindStringSubmatch(query)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
func selectedColumnNames(query string) []string {
	upper := strings.ToUpper(query)
	start, end := strings.Index(upper, "SELECT "), strings.Index(upper, " FROM ")
	if start < 0 || end < 0 {
		return nil
	}
	parts := strings.Split(query[start+7:end], ",")
	columns := make([]string, len(parts))
	for i := range parts {
		columns[i] = fmt.Sprintf("c%d", i)
	}
	return columns
}

func userOrgUniqueSpec() OrgCompositeUniqueSpec {
	return OrgCompositeUniqueSpec{
		Table: "users", NewIndex: "idx_org_user_id", Columns: []string{"org_id", "user_id"},
		OldIndexes: []string{"uni_users_user_id"}, OldSingleCols: []string{"user_id"},
		AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
	}
}

func stateWithUsers() *orgUniqueTestState {
	return &orgUniqueTestState{
		tables: map[string]map[string]bool{"users": {
			"id": true, "org_id": true, "user_id": true, "ding_talk_user_id": true,
		}},
		indexes:           map[string]map[string]uniqueIndexDefinition{"users": {}},
		conflicts:         make(map[string][][]driver.Value),
		conflictsAfterOrg: make(map[string][][]driver.Value),
		orgBackfilled:     make(map[string]bool),
	}
}

func TestOrgUniqueMigrationEmptyDatabase(t *testing.T) {
	state := &orgUniqueTestState{}
	if err := MigrateOrgCompositeUniqueIndexes(openOrgUniqueTestDB(t, state)); err != nil {
		t.Fatalf("empty database migration: %v", err)
	}
	if len(state.execs) != 0 {
		t.Fatalf("empty database must not execute DDL/DML: %v", state.execs)
	}
}

func TestOrgUniqueMigrationReplacesLegacyIndexAtomically(t *testing.T) {
	state := stateWithUsers()
	state.indexes["users"]["uni_users_user_id"] = uniqueIndexDefinition{Name: "uni_users_user_id", Columns: []string{"user_id"}, Unique: true}
	if err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec()); err != nil {
		t.Fatal(err)
	}
	var alter string
	for _, exec := range state.execs {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(exec)), "ALTER TABLE") {
			alter = exec
		}
	}
	if !strings.Contains(alter, "DROP INDEX `uni_users_user_id`") || !strings.Contains(alter, "ADD UNIQUE INDEX `idx_org_user_id`") {
		t.Fatalf("expected one atomic DROP+ADD ALTER, got %q", alter)
	}
}

func TestOrgUniqueMigrationExistingCorrectIndexStillAudits(t *testing.T) {
	state := stateWithUsers()
	state.indexes["users"]["idx_org_user_id"] = uniqueIndexDefinition{Name: "idx_org_user_id", Columns: []string{"org_id", "user_id"}, Unique: true}
	if err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec()); err != nil {
		t.Fatal(err)
	}
	if !state.orgBackfilled["users"] {
		t.Fatal("existing correct index must not skip org_id backfill/audit")
	}
	for _, exec := range state.execs {
		if strings.Contains(strings.ToUpper(exec), "ADD UNIQUE INDEX") {
			t.Fatalf("correct index must be skipped: %s", exec)
		}
	}
}

func TestOrgUniqueMigrationAllowsSameKeyAcrossOrganizations(t *testing.T) {
	state := stateWithUsers() // conflict query returns no same-org groups
	if err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec()); err != nil {
		t.Fatalf("same business key in different orgs is legal: %v", err)
	}
}

func TestOrgUniqueMigrationStopsOnSameOrganizationDuplicate(t *testing.T) {
	state := stateWithUsers()
	state.conflicts["users"] = [][]driver.Value{{"org-a", "u-1", int64(2), "10,11"}}
	err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec())
	if err == nil {
		t.Fatal("expected conflict failure")
	}
	message := err.Error()
	for _, want := range []string{"table=users", "org_id=org-a", "business_key={user_id=u-1}", "duplicate_count=2", "sample_ids=10,11"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error missing %q: %s", want, message)
		}
	}
	for _, exec := range state.execs {
		if strings.Contains(strings.ToUpper(exec), "ADD UNIQUE INDEX") {
			t.Fatalf("conflict must stop before index DDL: %s", exec)
		}
	}
}

func TestOrgUniqueMigrationAuditsAfterEmptyOrgBackfill(t *testing.T) {
	state := stateWithUsers()
	state.conflictsAfterOrg["users"] = [][]driver.Value{{"default", "u-1", int64(2), "1,2"}}
	err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec())
	if err == nil || !strings.Contains(err.Error(), "org_id=default") {
		t.Fatalf("expected post-backfill default-org conflict, got %v", err)
	}
	if !state.orgBackfilled["users"] {
		t.Fatal("expected empty org_id backfill before conflict query")
	}
}

func TestOrgUniqueMigrationIsRepeatable(t *testing.T) {
	state := stateWithUsers()
	db := openOrgUniqueTestDB(t, state)
	if err := migrateOneOrgCompositeUnique(db, userOrgUniqueSpec()); err != nil {
		t.Fatal(err)
	}
	firstAlterCount := 0
	for _, exec := range state.execs {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(exec)), "ALTER TABLE") {
			firstAlterCount++
		}
	}
	if err := migrateOneOrgCompositeUnique(db, userOrgUniqueSpec()); err != nil {
		t.Fatal(err)
	}
	secondAlterCount := 0
	for _, exec := range state.execs {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(exec)), "ALTER TABLE") {
			secondAlterCount++
		}
	}
	if secondAlterCount != firstAlterCount {
		t.Fatalf("second run created extra DDL: first=%d second=%d execs=%v", firstAlterCount, secondAlterCount, state.execs)
	}
}

func TestOrgUniqueMigrationReplacesWrongTargetColumnOrder(t *testing.T) {
	state := stateWithUsers()
	state.indexes["users"]["idx_org_user_id"] = uniqueIndexDefinition{
		Name: "idx_org_user_id", Columns: []string{"user_id", "org_id"}, Unique: true,
	}
	if err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec()); err != nil {
		t.Fatal(err)
	}
	var alter string
	for _, exec := range state.execs {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(exec)), "ALTER TABLE") {
			alter = exec
		}
	}
	if !strings.Contains(alter, "DROP INDEX `idx_org_user_id`") ||
		!strings.Contains(alter, "ADD UNIQUE INDEX `idx_org_user_id` (`org_id`, `user_id`)") {
		t.Fatalf("wrong-order target must be atomically replaced: %q", alter)
	}
}

func TestOrgUniquePrepareAddsMissingOrgColumnBeforeAudit(t *testing.T) {
	state := stateWithUsers()
	delete(state.tables["users"], "org_id")
	err := migrateOneOrgCompositeUniqueWithOptions(openOrgUniqueTestDB(t, state), userOrgUniqueSpec(), orgUniqueMigrationOptions{
		addMissingOrgColumn: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !state.tables["users"]["org_id"] || !state.orgBackfilled["users"] {
		t.Fatalf("org_id must be added then backfilled before audit: tables=%v execs=%v", state.tables, state.execs)
	}
}

func TestOrgUniqueMigrationDDLErrorIncludesRollbackSQL(t *testing.T) {
	state := stateWithUsers()
	state.indexes["users"]["uni_users_user_id"] = uniqueIndexDefinition{
		Name: "uni_users_user_id", Columns: []string{"user_id"}, Unique: true,
	}
	state.failExecContains = "alter table"
	err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec())
	if err == nil {
		t.Fatal("expected forced DDL failure")
	}
	for _, want := range []string{
		"rollback SQL",
		"DROP INDEX `idx_org_user_id`",
		"ADD UNIQUE INDEX `uni_users_user_id` (`user_id`)",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("DDL error missing %q: %v", want, err)
		}
	}
}

func TestOrgUniqueRollbackSQLRestoresIndexDefinitions(t *testing.T) {
	sqlText := buildOrgUniqueRollbackSQL("users", "idx_org_user_id", []uniqueIndexDefinition{
		{Name: "uni_users_user_id", Columns: []string{"user_id"}, Unique: true},
		{Name: "idx_users_lookup", Columns: []string{"user_id", "email"}, Unique: false},
	}, true)
	for _, want := range []string{
		"DROP INDEX `idx_org_user_id`",
		"ADD UNIQUE INDEX `uni_users_user_id` (`user_id`)",
		"ADD INDEX `idx_users_lookup` (`user_id`, `email`)",
	} {
		if !strings.Contains(sqlText, want) {
			t.Fatalf("rollback SQL missing %q: %s", want, sqlText)
		}
	}
}

func TestOrgUniqueConflictAuditIncludesSoftDeletedRows(t *testing.T) {
	state := stateWithUsers()
	if err := AuditOrgCompositeUniqueConflicts(openOrgUniqueTestDB(t, state), userOrgUniqueSpec()); err != nil {
		t.Fatal(err)
	}
	for _, query := range state.queries {
		if !strings.Contains(strings.ToUpper(query), "HAVING COUNT(*) > 1") {
			continue
		}
		if strings.Contains(strings.ToLower(query), "deleted_at") {
			t.Fatalf("unique index audit must include soft-deleted rows: %s", query)
		}
		return
	}
	t.Fatal("conflict audit query not captured")
}

func TestAnnualLeaveConsumeSchemaExpandBackfillIsRepeatableAndDoesNotDropIndexes(t *testing.T) {
	state := &orgUniqueTestState{
		tables: map[string]map[string]bool{
			"annual_leave_consume_logs": {"id": true, "org_id": true, "approval_ref": true, "grant_id": true},
		},
		indexes: map[string]map[string]uniqueIndexDefinition{
			"annual_leave_consume_logs": {
				"approval_ref": {Name: "approval_ref", Columns: []string{"approval_ref"}, Unique: true},
			},
		},
	}
	db := openOrgUniqueTestDB(t, state)
	if err := MigrateAnnualLeaveConsumeLogSchema(db); err != nil {
		t.Fatal(err)
	}
	if !state.tables["annual_leave_consume_logs"]["request_ref"] {
		t.Fatal("request_ref column was not added")
	}
	firstAlterCount := countExecPrefix(state.execs, "ALTER TABLE")
	if firstAlterCount != 1 {
		t.Fatalf("expected one schema-expand ALTER, got %v", state.execs)
	}
	if err := MigrateAnnualLeaveConsumeLogSchema(db); err != nil {
		t.Fatal(err)
	}
	if countExecPrefix(state.execs, "ALTER TABLE") != firstAlterCount {
		t.Fatalf("second schema-expand run must not add the column again: %v", state.execs)
	}
	foundBackfill := false
	for _, exec := range state.execs {
		upper := strings.ToUpper(exec)
		if strings.Contains(upper, "DROP INDEX") {
			t.Fatalf("schema expand must not drop the legacy index: %s", exec)
		}
		if strings.HasPrefix(strings.TrimSpace(upper), "UPDATE ANNUAL_LEAVE_CONSUME_LOGS") &&
			strings.Contains(exec, "CONCAT('approval:', approval_ref)") {
			foundBackfill = true
		}
	}
	if !foundBackfill {
		t.Fatalf("request_ref backfill not executed: %v", state.execs)
	}
}

func TestShiftCatalogSchemaExpandBackfillIsInjectableAndRepeatable(t *testing.T) {
	state := &orgUniqueTestState{
		tables: map[string]map[string]bool{
			"dingtalk_shift_catalogs": {
				"id": true, "org_id": true, "name": true, "check_in": true, "check_out": true,
			},
		},
		indexes: map[string]map[string]uniqueIndexDefinition{
			"dingtalk_shift_catalogs": {
				"idx_dingtalk_shift_catalogs_name": {
					Name: "idx_dingtalk_shift_catalogs_name", Columns: []string{"name"}, Unique: false,
				},
			},
		},
	}
	db := openOrgUniqueTestDB(t, state)
	if err := MigrateShiftCatalogSchema(db); err != nil {
		t.Fatal(err)
	}
	if !state.tables["dingtalk_shift_catalogs"]["shift_key"] {
		t.Fatal("shift_key column was not added")
	}
	firstAlterCount := countExecPrefix(state.execs, "ALTER TABLE")
	if firstAlterCount != 1 {
		t.Fatalf("expected one shift_key schema-expand ALTER, got %v", state.execs)
	}
	if err := MigrateShiftCatalogSchema(db); err != nil {
		t.Fatal(err)
	}
	if countExecPrefix(state.execs, "ALTER TABLE") != firstAlterCount {
		t.Fatalf("second shift schema run must not add/rebuild indexes: %v", state.execs)
	}
	for _, exec := range state.execs {
		upper := strings.ToUpper(exec)
		if strings.HasPrefix(strings.TrimSpace(upper), "SELECT") || strings.Contains(upper, "ADD UNIQUE INDEX") {
			t.Fatalf("shift schema migration must use set-based DML and leave unique DDL to phase 4: %s", exec)
		}
		if strings.Contains(strings.ToLower(exec), "`shift_key` <>") {
			t.Fatalf("shift schema migration must not overwrite an existing non-empty shift_key: %s", exec)
		}
	}
}

func countExecPrefix(execs []string, prefix string) int {
	count := 0
	for _, exec := range execs {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(exec)), strings.ToUpper(prefix)) {
			count++
		}
	}
	return count
}

func TestReadonlyOrgUniqueAuditContainsOnlySelectsAndNormalizesOrg(t *testing.T) {
	sqlText := ReadonlyOrgUniqueConflictAuditSQL(userOrgUniqueSpec())
	if !strings.Contains(sqlText, "COALESCE(NULLIF(TRIM(`org_id`), ''), 'default')") {
		t.Fatalf("audit SQL must group by post-backfill org_id: %s", sqlText)
	}
	forbidden := regexp.MustCompile(`(?im)^\s*(UPDATE|DELETE|INSERT|ALTER|DROP|CREATE|REPLACE|TRUNCATE)\b`)
	if forbidden.MatchString(sqlText) {
		t.Fatalf("read-only audit contains write DDL/DML: %s", sqlText)
	}
}

func TestPerformanceReminderUniqueIndexIncludesOrgFirst(t *testing.T) {
	typ := reflect.TypeOf(PerformanceReminderLog{})
	fields := []string{"OrgID", "ActivityID", "ParticipantID", "Stage", "ReminderKey", "ReminderDate"}
	for i, name := range fields {
		field, ok := typ.FieldByName(name)
		if !ok {
			t.Fatalf("missing field %s", name)
		}
		tag := field.Tag.Get("gorm")
		want := fmt.Sprintf("uniqueIndex:idx_perf_reminder_org_round,priority:%d", i+1)
		if !strings.Contains(tag, want) {
			t.Fatalf("%s tag missing %s: %s", name, want, tag)
		}
	}
}

func TestOrgUniqueSpecsMatchGormModelIndexes(t *testing.T) {
	models := map[string]interface{}{
		"organization_users":                &OrganizationUser{},
		"users":                             &User{},
		"user_department_memberships":       &UserDepartmentMembership{},
		"departments":                       &Department{},
		"employee_profiles":                 &EmployeeProfile{},
		"attendances":                       &Attendance{},
		"sync_statuses":                     &SyncStatus{},
		"approvals":                         &Approval{},
		"approval_templates":                &ApprovalTemplate{},
		"dingtalk_event_logs":               &DingTalkEventLog{},
		"roles":                             &Role{},
		"user_roles":                        &UserRole{},
		"menu_permissions":                  &MenuPermission{},
		"data_permissions":                  &DataPermission{},
		"employee_shift_configs":            &EmployeeShiftConfig{},
		"dingtalk_shift_catalogs":           &DingTalkShiftCatalog{},
		"week_schedule_rules":               &WeekScheduleRule{},
		"week_schedule_overrides":           &WeekScheduleOverride{},
		"week_schedule_group_targets":       &WeekScheduleGroupTarget{},
		"statutory_holidays":                &StatutoryHoliday{},
		"annual_leave_eligibilities":        &AnnualLeaveEligibility{},
		"annual_leave_grants":               &AnnualLeaveGrant{},
		"overtime_rule_configs":             &OvertimeRuleConfig{},
		"overtime_match_results":            &OvertimeMatchResult{},
		"overtime_sync_histories":           &OvertimeSyncHistory{},
		"annual_leave_consume_logs":         &AnnualLeaveConsumeLog{},
		"employee_transfers":                &EmployeeTransfer{},
		"employee_resignations":             &EmployeeResignation{},
		"employee_onboardings":              &EmployeeOnboarding{},
		"talent_analyses":                   &TalentAnalysis{},
		"ding_talk_bindings":                &DingTalkBinding{},
		"idempotency_records":               &IdempotencyRecord{},
		"performance_reminder_logs":         &PerformanceReminderLog{},
		"external_attendance_raw":           &ExternalAttendanceRaw{},
		"external_attendance_approve_links": &ExternalAttendanceApproveLink{},
		"external_user_department_raw":      &ExternalUserDepartmentRaw{},
		"user_department_relations":         &UserDepartmentRelation{},
		"external_sync_cursors":             &ExternalSyncCursor{},
		"external_sync_locks":               &ExternalSyncLock{},
		"performance_import_batches":        &PerformanceImportBatch{},
	}
	parsed := make(map[string]*schema.Schema)
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		model, ok := models[spec.Table]
		if !ok {
			t.Fatalf("migration spec has no model inventory entry: table=%s index=%s", spec.Table, spec.NewIndex)
		}
		modelSchema := parsed[spec.Table]
		if modelSchema == nil {
			var err error
			modelSchema, err = schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
			if err != nil {
				t.Fatalf("parse model for table %s: %v", spec.Table, err)
			}
			parsed[spec.Table] = modelSchema
		}
		var got []string
		for _, index := range modelSchema.ParseIndexes() {
			if index.Name != spec.NewIndex {
				continue
			}
			if index.Class != "UNIQUE" {
				t.Fatalf("model index is not UNIQUE: table=%s index=%s class=%s", spec.Table, spec.NewIndex, index.Class)
			}
			for _, field := range index.Fields {
				got = append(got, field.DBName)
			}
			break
		}
		if !equalColumns(got, spec.Columns) {
			t.Fatalf("AutoMigrate index conflicts with phase-4 matrix: table=%s index=%s model=%v migration=%v", spec.Table, spec.NewIndex, got, spec.Columns)
		}
	}
}

func TestAnnualLeaveConsumeLegacyApprovalUniqueIsInAtomicMigrationMatrix(t *testing.T) {
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		if spec.Table != "annual_leave_consume_logs" {
			continue
		}
		if !stringSliceContains(spec.OldSingleCols, "approval_ref") {
			t.Fatalf("approval_ref single-column UNIQUE must be replaced by phase-4 atomic migration: %+v", spec)
		}
		return
	}
	t.Fatal("annual_leave_consume_logs phase-4 spec not found")
}

func TestUserDepartmentMembershipUniqueContractStartsWithOrganization(t *testing.T) {
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		if spec.Table != "user_department_memberships" {
			continue
		}
		want := []string{"org_id", "user_id", "department_id"}
		if spec.NewIndex != "idx_user_department_membership" || !equalColumns(spec.Columns, want) {
			t.Fatalf("membership unique contract = %#v, want index with columns %v", spec, want)
		}
		return
	}
	t.Fatal("user_department_memberships unique contract missing from migration matrix")
}

func TestUserDingTalkBackfillStopsBeforeUpdateOnSameOrgConflict(t *testing.T) {
	state := stateWithUsers()
	state.conflicts["users"] = [][]driver.Value{{"org-a", "ding-1", int64(2), "11,12"}}
	err := backfillUserDingTalkIDDB(openOrgUniqueTestDB(t, state), "user_id")
	if err == nil {
		t.Fatal("expected DingTalk backfill conflict")
	}
	for _, want := range []string{"table=users", "org_id=org-a", "ding_talk_user_id=ding-1", "duplicate_count=2", "sample_ids=11,12"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error missing %q: %v", want, err)
		}
	}
	for _, exec := range state.execs {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(exec)), "UPDATE") {
			t.Fatalf("conflicting backfill must not update rows: %s", exec)
		}
	}
}

func TestUserDingTalkBackfillDoesNotSelectOneSurvivor(t *testing.T) {
	state := stateWithUsers()
	if err := backfillUserDingTalkIDDB(openOrgUniqueTestDB(t, state), "user_id"); err != nil {
		t.Fatal(err)
	}
	if len(state.execs) != 1 {
		t.Fatalf("expected one direct backfill UPDATE, got %v", state.execs)
	}
	upper := strings.ToUpper(state.execs[0])
	if strings.Contains(upper, "MIN(") || strings.Contains(upper, "MAX(") || strings.Contains(upper, "LIMIT 1") {
		t.Fatalf("backfill must not choose a survivor: %s", state.execs[0])
	}
	if !strings.Contains(state.execs[0], "SET `ding_talk_user_id` = `user_id`") {
		t.Fatalf("expected set-based backfill of every eligible row: %s", state.execs[0])
	}
}

func TestOrgUniqueMigrationNormalizesEmptyNullableBusinessKeys(t *testing.T) {
	state := stateWithUsers()
	state.tables["users"]["mobile"] = true
	state.nullableCols = map[string]map[string]bool{"users": {"mobile": false}}
	state.columnTypes = map[string]map[string]string{"users": {"mobile": "varchar(32)"}}
	spec := OrgCompositeUniqueSpec{
		Table: "users", NewIndex: "idx_users_org_mobile", Columns: []string{"org_id", "mobile"},
		EmptyNullableCols: []string{"mobile"}, AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
	}
	if err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), spec); err != nil {
		t.Fatal(err)
	}
	if !state.nullableCols["users"]["mobile"] {
		t.Fatal("mobile must become nullable before empty-string normalization")
	}
	if !state.emptyNormalized["users"]["mobile"] {
		t.Fatal("empty mobile strings must be normalized to NULL")
	}
}

func TestOrgUniqueMigrationDoesNotRewriteNonEmptyBusinessKeys(t *testing.T) {
	state := stateWithUsers()
	if err := migrateOneOrgCompositeUnique(openOrgUniqueTestDB(t, state), userOrgUniqueSpec()); err != nil {
		t.Fatal(err)
	}
	for _, exec := range state.execs {
		upper := strings.ToUpper(exec)
		if strings.HasPrefix(strings.TrimSpace(upper), "UPDATE") && strings.Contains(upper, "USER_ID") {
			t.Fatalf("non-empty business key user_id must not be rewritten: %s", exec)
		}
	}
}

func TestOrgUniqueMatrixIncludesPerformanceImportBatch(t *testing.T) {
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		if spec.Table == "performance_import_batches" && spec.NewIndex == "uk_performance_import_batch_org_key" {
			if !equalColumns(spec.Columns, []string{"org_id", "batch_key"}) {
				t.Fatalf("unexpected columns: %v", spec.Columns)
			}
			return
		}
	}
	t.Fatal("performance_import_batches unique contract missing from phase-4 matrix")
}

func TestOrgUniqueMigrationMatrixStartsWithOrgID(t *testing.T) {
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		if err := validateOrgCompositeUniqueSpec(spec); err != nil {
			t.Fatal(err)
		}
	}
}
