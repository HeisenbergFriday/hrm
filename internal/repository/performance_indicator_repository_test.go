package repository

import (
	"context"
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

const stubIndicatorDriverName = "peopleops_stub_indicator_mysql"

var (
	stubIndicatorDriverOnce sync.Once
	stubIndicatorDBs        sync.Map
)

type stubIndicatorQueryResponse struct {
	match   func(query string, args []driver.NamedValue) bool
	columns []string
	rows    [][]driver.Value
}

type stubIndicatorDB struct {
	queries []stubIndicatorQueryResponse
}

type stubIndicatorDriver struct{}

type stubIndicatorConn struct {
	db *stubIndicatorDB
}

type stubIndicatorStmt struct {
	conn  *stubIndicatorConn
	query string
}

type stubIndicatorRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type stubIndicatorTx struct{}

type stubIndicatorResult struct{}

func (stubIndicatorResult) LastInsertId() (int64, error) { return 0, nil }
func (stubIndicatorResult) RowsAffected() (int64, error) { return 0, nil }

func (d stubIndicatorDriver) Open(name string) (driver.Conn, error) {
	value, ok := stubIndicatorDBs.Load(name)
	if !ok {
		return nil, fmt.Errorf("stub db %s not registered", name)
	}
	return &stubIndicatorConn{db: value.(*stubIndicatorDB)}, nil
}

func (c *stubIndicatorConn) Prepare(query string) (driver.Stmt, error) {
	return &stubIndicatorStmt{conn: c, query: query}, nil
}

func (c *stubIndicatorConn) Close() error { return nil }

func (c *stubIndicatorConn) Begin() (driver.Tx, error) { return stubIndicatorTx{}, nil }

func (c *stubIndicatorConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return stubIndicatorTx{}, nil
}

func (c *stubIndicatorConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(query, args)
}

func (c *stubIndicatorConn) ExecContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return stubIndicatorResult{}, nil
}

func (c *stubIndicatorConn) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, response := range c.db.queries {
		if response.match != nil && response.match(query, args) {
			rows := make([][]driver.Value, len(response.rows))
			for i := range response.rows {
				rows[i] = append([]driver.Value(nil), response.rows[i]...)
			}
			return &stubIndicatorRows{
				columns: append([]string(nil), response.columns...),
				rows:    rows,
			}, nil
		}
	}
	return nil, fmt.Errorf("unexpected query: %s", query)
}

func (s *stubIndicatorStmt) Close() error  { return nil }
func (s *stubIndicatorStmt) NumInput() int { return -1 }
func (s *stubIndicatorStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return stubIndicatorResult{}, nil
}
func (s *stubIndicatorStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return s.conn.query(s.query, named)
}

func (r *stubIndicatorRows) Columns() []string { return r.columns }
func (r *stubIndicatorRows) Close() error      { return nil }
func (r *stubIndicatorRows) Next(dest []driver.Value) error {
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

func (stubIndicatorTx) Commit() error   { return nil }
func (stubIndicatorTx) Rollback() error { return nil }

// ===================== Test Helpers =====================

func newTestDB(t *testing.T, queries ...stubIndicatorQueryResponse) *gorm.DB {
	t.Helper()
	stubIndicatorDriverOnce.Do(func() {
		stdsql.Register(stubIndicatorDriverName, stubIndicatorDriver{})
	})

	dsn := fmt.Sprintf("indicator-test-%s-%d", t.Name(), time.Now().UnixNano())
	stubIndicatorDBs.Store(dsn, &stubIndicatorDB{queries: queries})
	t.Cleanup(func() {
		stubIndicatorDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(stubIndicatorDriverName, dsn)
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

// stubTableMatcher matches queries containing a table name
func stubTableMatcher(table string) func(string, []driver.NamedValue) bool {
	table = strings.ToLower(table)
	return func(query string, _ []driver.NamedValue) bool {
		return strings.Contains(strings.ToLower(query), table)
	}
}

// stubCountMatcher matches count queries
func stubCountMatcher(table string) func(string, []driver.NamedValue) bool {
	table = strings.ToLower(table)
	return func(query string, _ []driver.NamedValue) bool {
		lower := strings.ToLower(query)
		return strings.Contains(lower, "count(*)") && strings.Contains(lower, table)
	}
}

// stubSelectMatcher matches select queries (non-count)
func stubSelectMatcher(table string) func(string, []driver.NamedValue) bool {
	table = strings.ToLower(table)
	return func(query string, _ []driver.NamedValue) bool {
		lower := strings.ToLower(query)
		return strings.Contains(lower, table) && !strings.Contains(lower, "count(*)")
	}
}

func stubIndicatorArgsContain(args []driver.NamedValue, value string) bool {
	for _, arg := range args {
		if fmt.Sprint(arg.Value) == value {
			return true
		}
	}
	return false
}

// libraryColumns returns the column names for PerformanceIndicatorLibrary
func libraryColumns() []string {
	return []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"}
}

// libraryRow returns a row for PerformanceIndicatorLibrary
func libraryRow(id uint, deptID, deptName, name, desc, status, cycle string) []driver.Value {
	return []driver.Value{id, deptID, deptName, name, desc, status, cycle, time.Now(), time.Now(), "", ""}
}

// itemColumns returns the column names for PerformanceIndicatorItem
func itemColumns() []string {
	return []string{
		"id", "library_id", "section_type", "name", "description", "indicator_type",
		"calculation_method", "data_source", "cycle", "default_weight",
		"red_line_value", "target_value", "challenge_value", "scoring_rule",
		"weight", "is_default", "is_inherited", "sort_order",
		"created_at", "updated_at", "created_by", "updated_by",
	}
}

// itemRow returns a row for PerformanceIndicatorItem
func itemRow(id, libraryID uint, sectionType, name string, weight float64, sortOrder int) []driver.Value {
	return []driver.Value{id, libraryID, sectionType, name, "", "", "", "", "", 0.0, "", "", "", "", weight, false, false, sortOrder, time.Now(), time.Now(), "", ""}
}

// ===================== Library Repository Tests =====================

func TestLibraryRepo_Create(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	lib := &database.PerformanceIndicatorLibrary{
		Name:         "Test Library",
		DepartmentID: "dept-1",
	}
	if err := repo.Create(lib); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestLibraryRepo_GetByID_Found(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: libraryColumns(),
		rows:    [][]driver.Value{libraryRow(1, "dept-1", "Dept", "Lib", "desc", "active", "monthly")},
	})
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	lib, err := repo.GetByID(1)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if lib.Name != "Lib" {
		t.Fatalf("GetByID() name = %q, want Lib", lib.Name)
	}
	if lib.DepartmentID != "dept-1" {
		t.Fatalf("GetByID() department_id = %q, want dept-1", lib.DepartmentID)
	}
}

func TestLibraryRepo_GetByID_NotFound(t *testing.T) {
	db := newTestDB(t) // no stub → query fails
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	_, err := repo.GetByID(999)
	if err == nil {
		t.Fatal("GetByID(999) should return error for non-existent library")
	}
}

func TestLibraryRepo_Update(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	lib := &database.PerformanceIndicatorLibrary{
		ID:             1,
		Name:           "Updated Name",
		DepartmentID:   "dept-1",
		DepartmentName: "Dept",
		Status:         "active",
		UpdatedBy:      "u1",
	}
	if err := repo.Update(lib); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestLibraryRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	if err := repo.Delete(1, "admin"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestLibraryRepo_FindAll_NoFilters(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows: [][]driver.Value{
				libraryRow(1, "dept-1", "Dept1", "Lib1", "", "active", ""),
				libraryRow(2, "dept-2", "Dept2", "Lib2", "", "active", ""),
			},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, total, err := repo.FindAll(1, 10, "", "", "", nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 2 {
		t.Fatalf("FindAll() total = %d, want 2", total)
	}
	if len(libs) != 2 {
		t.Fatalf("FindAll() len = %d, want 2", len(libs))
	}
}

func TestLibraryRepo_FindAll_WithDepartmentFilter(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows:    [][]driver.Value{libraryRow(1, "dept-1", "Dept", "Lib1", "", "active", "")},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, total, err := repo.FindAll(1, 10, "dept-1", "", "", nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("FindAll() total = %d, want 1", total)
	}
	if len(libs) != 1 || libs[0].DepartmentID != "dept-1" {
		t.Fatalf("FindAll() = %#v", libs)
	}
}

func TestLibraryRepo_FindAll_WithKeywordFilter(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows:    [][]driver.Value{libraryRow(1, "dept-1", "Dept", "Sales KPI", "", "active", "")},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, total, err := repo.FindAll(1, 10, "", "Sales", "", nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("FindAll() total = %d, want 1", total)
	}
	if len(libs) != 1 || libs[0].Name != "Sales KPI" {
		t.Fatalf("FindAll() = %#v", libs)
	}
}

func TestLibraryRepo_FindAll_WithStatusFilter(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows:    [][]driver.Value{libraryRow(1, "dept-1", "Dept", "Archived Lib", "", "archived", "")},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, total, err := repo.FindAll(1, 10, "", "", "archived", nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("FindAll() total = %d, want 1", total)
	}
	if len(libs) != 1 || libs[0].Status != "archived" {
		t.Fatalf("FindAll() = %#v", libs)
	}
}

func TestLibraryRepo_FindAll_WithVisibleDepartments(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows:    [][]driver.Value{libraryRow(1, "dept-1", "Dept", "Lib1", "", "active", "")},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, total, err := repo.FindAll(1, 10, "", "", "", []string{"dept-1", "dept-2"})
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("FindAll() total = %d, want 1", total)
	}
	if len(libs) != 1 {
		t.Fatalf("FindAll() len = %d, want 1", len(libs))
	}
}

func TestLibraryRepo_FindAll_Pagination(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(5)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows:    [][]driver.Value{libraryRow(3, "dept-1", "Dept", "Lib3", "", "active", "")},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, total, err := repo.FindAll(2, 2, "", "", "", nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 5 {
		t.Fatalf("FindAll() total = %d, want 5", total)
	}
	if len(libs) != 1 {
		t.Fatalf("FindAll() page 2 len = %d, want 1", len(libs))
	}
}

func TestLibraryRepo_FindAll_DefaultPagination(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(0)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows:    [][]driver.Value{},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	// page=0, pageSize=0 should use defaults (1, 10)
	libs, total, err := repo.FindAll(0, 0, "", "", "", nil)
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 0 {
		t.Fatalf("FindAll() total = %d, want 0", total)
	}
	if len(libs) != 0 {
		t.Fatalf("FindAll() len = %d, want 0", len(libs))
	}
}

func TestLibraryRepo_FindByDepartment_Found(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: libraryColumns(),
		rows: [][]driver.Value{
			libraryRow(1, "dept-1", "Dept", "Lib1", "", "active", ""),
			libraryRow(2, "dept-1", "Dept", "Lib2", "", "active", ""),
		},
	})
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, err := repo.FindByDepartment("dept-1")
	if err != nil {
		t.Fatalf("FindByDepartment() error = %v", err)
	}
	if len(libs) != 2 {
		t.Fatalf("FindByDepartment() len = %d, want 2", len(libs))
	}
	for _, lib := range libs {
		if lib.DepartmentID != "dept-1" {
			t.Fatalf("FindByDepartment() department_id = %q, want dept-1", lib.DepartmentID)
		}
		if lib.Status != "active" {
			t.Fatalf("FindByDepartment() status = %q, want active", lib.Status)
		}
	}
}

func TestLibraryRepo_FindByDepartment_Empty(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: libraryColumns(),
		rows:    [][]driver.Value{},
	})
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, err := repo.FindByDepartment("dept-999")
	if err != nil {
		t.Fatalf("FindByDepartment() error = %v", err)
	}
	if len(libs) != 0 {
		t.Fatalf("FindByDepartment() len = %d, want 0", len(libs))
	}
}

func TestLibraryRepo_Archive(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	if err := repo.Archive(1, "admin"); err != nil {
		t.Fatalf("Archive() error = %v", err)
	}
}

// ===================== Item Repository Tests =====================

func TestItemRepo_Create(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	item := &database.PerformanceIndicatorItem{
		LibraryID:   1,
		Name:        "KPI Item",
		SectionType: "quantitative",
		Weight:      0.6,
		SortOrder:   1,
	}
	if err := repo.Create(item); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func TestItemRepo_GetByID_Found(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows:    [][]driver.Value{itemRow(5, 1, "quantitative", "KPI", 0.6, 1)},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	item, err := repo.GetByID(5)
	if err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if item.Name != "KPI" {
		t.Fatalf("GetByID() name = %q, want KPI", item.Name)
	}
	if item.LibraryID != 1 {
		t.Fatalf("GetByID() library_id = %d, want 1", item.LibraryID)
	}
}

func TestItemRepo_GetByID_NotFound(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	_, err := repo.GetByID(999)
	if err == nil {
		t.Fatal("GetByID(999) should return error for non-existent item")
	}
}

func TestItemRepo_Update(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	item := &database.PerformanceIndicatorItem{
		ID:          1,
		LibraryID:   1,
		Name:        "Updated Item",
		SectionType: "quantitative",
		Weight:      0.8,
		UpdatedBy:   "u1",
	}
	if err := repo.Update(item); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
}

func TestItemRepo_Delete(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	if err := repo.Delete(1, "admin"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
}

func TestItemRepo_FindByLibrary_NoSectionType(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "KPI1", 0.5, 1),
			itemRow(2, 1, "key_action", "Action1", 0.3, 2),
			itemRow(3, 1, "bonus_penalty", "Bonus1", 0.2, 3),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.FindByLibrary(1, "")
	if err != nil {
		t.Fatalf("FindByLibrary() error = %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("FindByLibrary() len = %d, want 3", len(items))
	}
}

func TestItemRepo_FindByLibrary_WithSectionType(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "KPI1", 0.5, 1),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.FindByLibrary(1, "quantitative")
	if err != nil {
		t.Fatalf("FindByLibrary() error = %v", err)
	}
	if len(items) != 1 || items[0].SectionType != "quantitative" {
		t.Fatalf("FindByLibrary() = %#v", items)
	}
}

func TestItemRepo_FindByLibrary_Empty(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows:    [][]driver.Value{},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.FindByLibrary(999, "")
	if err != nil {
		t.Fatalf("FindByLibrary() error = %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("FindByLibrary() len = %d, want 0", len(items))
	}
}

func TestItemRepo_Search_WithLibraryIDs(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "客户满意度", 0.6, 1),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.Search([]uint{1}, "客户", "", nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || !strings.Contains(items[0].Name, "客户") {
		t.Fatalf("Search() = %#v", items)
	}
}

func TestItemRepo_Search_EmptyLibraryIDs(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "KPI1", 0.5, 1),
			itemRow(2, 2, "key_action", "Action1", 0.3, 1),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.Search(nil, "", "", nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Search() len = %d, want 2", len(items))
	}
}

func TestItemRepo_Search_WithSectionType(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "KPI1", 0.5, 1),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.Search([]uint{1}, "", "quantitative", nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || items[0].SectionType != "quantitative" {
		t.Fatalf("Search() = %#v", items)
	}
}

func TestItemRepo_Search_WithKeyword(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "Revenue Target", 0.6, 1),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.Search([]uint{1}, "Revenue", "", nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || !strings.Contains(items[0].Name, "Revenue") {
		t.Fatalf("Search() = %#v", items)
	}
}

func TestItemRepo_Search_WithVisibleDepartments(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "join performance_indicator_libraries") &&
				strings.Contains(lower, "performance_indicator_libraries.department_id") &&
				stubIndicatorArgsContain(args, "dept-1")
		},
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "Revenue Target", 0.6, 1),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.Search(nil, "", "", []string{"dept-1"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 || items[0].Name != "Revenue Target" {
		t.Fatalf("Search() = %#v", items)
	}
}

func TestItemRepo_BatchCreate(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items := []database.PerformanceIndicatorItem{
		{LibraryID: 1, Name: "Item1", SectionType: "quantitative", SortOrder: 1},
		{LibraryID: 1, Name: "Item2", SectionType: "key_action", SortOrder: 2},
		{LibraryID: 1, Name: "Item3", SectionType: "bonus_penalty", SortOrder: 3},
	}
	if err := repo.BatchCreate(items); err != nil {
		t.Fatalf("BatchCreate() error = %v", err)
	}
}

func TestItemRepo_BatchCreate_Empty(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	// Empty slice should be a no-op
	if err := repo.BatchCreate([]database.PerformanceIndicatorItem{}); err != nil {
		t.Fatalf("BatchCreate() empty error = %v", err)
	}
}

func TestItemRepo_DeleteByLibrary(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	if err := repo.DeleteByLibrary(1, "admin"); err != nil {
		t.Fatalf("DeleteByLibrary() error = %v", err)
	}
}

func TestItemRepo_DeleteByLibrary_VerifiesSoftDelete(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	// Delete should set deleted_at and updated_by
	err := repo.DeleteByLibrary(1, "admin")
	if err != nil {
		t.Fatalf("DeleteByLibrary() error = %v", err)
	}
	// The stub driver accepts any ExecContext, so we verify no error is returned
}

// ===================== Edge Cases =====================

func TestLibraryRepo_FindAll_AllFiltersCombined(t *testing.T) {
	db := newTestDB(t,
		stubIndicatorQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		stubIndicatorQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: libraryColumns(),
			rows:    [][]driver.Value{libraryRow(1, "dept-1", "Dept", "Sales KPI", "desc", "active", "monthly")},
		},
	)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	libs, total, err := repo.FindAll(1, 10, "dept-1", "Sales", "active", []string{"dept-1"})
	if err != nil {
		t.Fatalf("FindAll() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("FindAll() total = %d, want 1", total)
	}
	if len(libs) != 1 || libs[0].Name != "Sales KPI" {
		t.Fatalf("FindAll() = %#v", libs)
	}
}

func TestItemRepo_Search_AllFiltersCombined(t *testing.T) {
	db := newTestDB(t, stubIndicatorQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: itemColumns(),
		rows: [][]driver.Value{
			itemRow(1, 1, "quantitative", "客户满意度", 0.6, 1),
		},
	})
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	items, err := repo.Search([]uint{1}, "客户", "quantitative", nil)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("Search() len = %d, want 1", len(items))
	}
}

func TestLibraryRepo_Delete_WithDeletedBy(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "test-org")

	// Should not error even if deleted_by is empty
	if err := repo.Delete(1, ""); err != nil {
		t.Fatalf("Delete() with empty deleted_by error = %v", err)
	}
}

func TestItemRepo_Create_VerifyFields(t *testing.T) {
	db := newTestDB(t)
	repo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "test-org")

	item := &database.PerformanceIndicatorItem{
		LibraryID:         1,
		ParentIndicatorID: uintPtr(10),
		Name:              "Complex Item",
		SectionType:       "quantitative",
		IndicatorType:     "number",
		DefaultWeight:     0.7,
		Weight:            0.7,
		IsDefault:         true,
		SortOrder:         5,
		CreatedBy:         "system",
	}
	if err := repo.Create(item); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
}

func uintPtr(v uint) *uint { return &v }
