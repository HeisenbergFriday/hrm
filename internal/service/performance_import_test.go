package service

import (
	"archive/zip"
	"bytes"
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

func TestParsePerformanceParticipantImportXLSXTemplate(t *testing.T) {
	data := buildTestParticipantImportXLSX(t)

	result, err := ParsePerformanceParticipantImportXLSX(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ParsePerformanceParticipantImportXLSX() error = %v", err)
	}

	if result.ActivityName != "2026年Q2绩效考核" {
		t.Fatalf("ActivityName = %q", result.ActivityName)
	}
	if result.ParsedCount != 2 {
		t.Fatalf("ParsedCount = %d", result.ParsedCount)
	}
	if result.DuplicateCount != 1 {
		t.Fatalf("DuplicateCount = %d", result.DuplicateCount)
	}
	want := []string{"00123", "456"}
	if len(result.EmployeeIDs) != len(want) {
		t.Fatalf("EmployeeIDs length = %d, want %d: %#v", len(result.EmployeeIDs), len(want), result.EmployeeIDs)
	}
	for i := range want {
		if result.EmployeeIDs[i] != want[i] {
			t.Fatalf("EmployeeIDs[%d] = %q, want %q", i, result.EmployeeIDs[i], want[i])
		}
	}
}

func buildTestParticipantImportXLSX(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Sheet1" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		"xl/sharedStrings.xml": `<?xml version="1.0" encoding="UTF-8"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="12" uniqueCount="12">
  <si><t>绩效活动</t></si>
  <si><t>工号</t></si>
  <si><t>姓名</t></si>
  <si><t>一级部门</t></si>
  <si><t>二级部门</t></si>
  <si><t>三级部门</t></si>
  <si><t>2026年Q2绩效考核</t></si>
  <si><t>00123</t></si>
  <si><t>张三</t></si>
  <si><t>总部</t></si>
  <si><t>产品部</t></si>
  <si><t>平台组</t></si>
</sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="2">
      <c r="A2" t="s"><v>0</v></c>
      <c r="B2" t="s"><v>1</v></c>
      <c r="C2" t="s"><v>2</v></c>
      <c r="D2" t="s"><v>3</v></c>
      <c r="E2" t="s"><v>4</v></c>
      <c r="F2" t="s"><v>5</v></c>
    </row>
    <row r="3">
      <c r="A3" t="s"><v>6</v></c>
      <c r="B3" t="s"><v>7</v></c>
      <c r="C3" t="s"><v>8</v></c>
      <c r="D3" t="s"><v>9</v></c>
      <c r="E3" t="s"><v>10</v></c>
      <c r="F3" t="s"><v>11</v></c>
    </row>
    <row r="4">
      <c r="A4" t="s"><v>6</v></c>
      <c r="B4" t="s"><v>7</v></c>
    </row>
    <row r="5">
      <c r="A5" t="s"><v>6</v></c>
      <c r="B5"><v>456.0</v></c>
    </row>
  </sheetData>
</worksheet>`,
	}
	for name, content := range files {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

func TestParsePerformanceParticipantRowsManagerColumnsAndWarnings(t *testing.T) {
	rows := []xlsxImportRow{
		{
			Number: 2,
			Values: []string{
				"绩效活动",
				"employeeid",
				"name",
				"manageruserid",
				"managername",
				"managersource",
				"reason",
			},
		},
		{Number: 3, Values: []string{"Q1", "00123", "Alice", "789.0", "Boss", "manual", "temporary override"}},
		{Number: 4, Values: []string{"Q1", "00123", "Duplicate"}},
		{Number: 5, Values: []string{"Q1", " "}},
		{Number: 6, Values: []string{"Q2", "4.56E2", "Bob"}},
	}

	result, err := parsePerformanceParticipantRows(rows)
	if err != nil {
		t.Fatalf("parsePerformanceParticipantRows() error = %v", err)
	}

	if result.ActivityName != "Q1" {
		t.Fatalf("ActivityName = %q, want Q1", result.ActivityName)
	}
	if result.ParsedCount != 2 {
		t.Fatalf("ParsedCount = %d, want 2", result.ParsedCount)
	}
	if result.ImportedCount != 2 {
		t.Fatalf("ImportedCount = %d, want 2", result.ImportedCount)
	}
	if result.DuplicateCount != 1 {
		t.Fatalf("DuplicateCount = %d, want 1", result.DuplicateCount)
	}
	wantIDs := []string{"00123", "456"}
	for i := range wantIDs {
		if result.EmployeeIDs[i] != wantIDs[i] {
			t.Fatalf("EmployeeIDs[%d] = %q, want %q", i, result.EmployeeIDs[i], wantIDs[i])
		}
	}
	if len(result.SkippedRows) != 1 || result.SkippedRows[0].Row != 5 {
		t.Fatalf("SkippedRows = %#v, want row 5", result.SkippedRows)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("Warnings = %#v, want one warning for multiple activity names", result.Warnings)
	}
	if len(result.rawRows) != 2 {
		t.Fatalf("rawRows length = %d, want 2", len(result.rawRows))
	}
	first := result.rawRows[0]
	if first.AssessmentManagerEmployeeID != "789" ||
		first.AssessmentManagerName != "Boss" ||
		first.AssessmentManagerSource != "manual" ||
		first.ManagerOverrideReason != "temporary override" {
		t.Fatalf("raw manager columns not parsed: %#v", first)
	}
}

func TestParsePerformanceParticipantRowsRejectsMissingHeader(t *testing.T) {
	_, err := parsePerformanceParticipantRows([]xlsxImportRow{
		{Number: 1, Values: []string{"employeeid"}},
		{Number: 2, Values: []string{"00123"}},
	})
	if err == nil {
		t.Fatalf("parsePerformanceParticipantRows() expected missing header error")
	}
}

func TestNormalizeImportedEmployeeID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", ""},
		{" '00123 ", "00123"},
		{"123.0", "123"},
		{"123.000", "123"},
		{"4.56E2", "456"},
		{"1.25E2", "125"},
		{"abc.0", "abc.0"},
	}

	for _, tt := range tests {
		if got := normalizeImportedEmployeeID(tt.input); got != tt.want {
			t.Fatalf("normalizeImportedEmployeeID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestReadLimitedRejectsOversize(t *testing.T) {
	_, err := readLimited(strings.NewReader("abcd"), 3)
	if err == nil {
		t.Fatalf("readLimited() expected oversize error")
	}

	data, err := readLimited(strings.NewReader("abc"), 3)
	if err != nil {
		t.Fatalf("readLimited() error = %v", err)
	}
	if string(data) != "abc" {
		t.Fatalf("readLimited() = %q, want abc", string(data))
	}
}

func TestResolveXLSXRelationshipTarget(t *testing.T) {
	tests := []struct {
		source string
		target string
		want   string
	}{
		{"xl/workbook.xml", "worksheets/sheet1.xml", "xl/worksheets/sheet1.xml"},
		{"xl/workbook.xml", "/xl/worksheets/sheet1.xml", "xl/worksheets/sheet1.xml"},
		{"xl/_rels/workbook.xml.rels", "../worksheets/sheet1.xml", "xl/worksheets/sheet1.xml"},
		{"xl/workbook.xml", `worksheets\sheet2.xml`, "xl/worksheets/sheet2.xml"},
	}

	for _, tt := range tests {
		if got := resolveXLSXRelationshipTarget(tt.source, tt.target); got != tt.want {
			t.Fatalf("resolveXLSXRelationshipTarget(%q, %q) = %q, want %q", tt.source, tt.target, got, tt.want)
		}
	}
}

func TestResolveImportedPerformanceEmployeesMatchesProfilesUsersDepartmentsAndManagers(t *testing.T) {
	svc := newImportStubPerformanceService(t,
		importStubTableResponse("employee_profiles", []string{"id", "user_id", "employee_id"}, [][]driver.Value{
			{int64(1), "user-1", "E001"},
			{int64(2), "manager-1", "M001"},
			{int64(3), "user-2", "E002"},
			{int64(4), "manager-2", "M002"},
		}),
		importStubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				query = strings.ToLower(query)
				return strings.Contains(query, "users") && strings.Contains(query, "name =")
			},
			columns: []string{"id", "user_id", "name", "department_id", "status"},
			rows: [][]driver.Value{
				{int64(4), "manager-2", "Manager Two", "dept-2", "active"},
			},
		},
		importStubTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
			{int64(1), "user-1", "Alice", "dept-1", "active"},
			{int64(2), "manager-1", "Manager One", "dept-1", "active"},
			{int64(3), "user-2", "Bob", "dept-2", "active"},
			{int64(4), "manager-2", "Manager Two", "dept-2", "active"},
		}),
		importStubTableResponse("departments", []string{"id", "department_id", "name"}, [][]driver.Value{
			{int64(1), "dept-1", "Product"},
			{int64(2), "dept-2", "Sales"},
		}),
	)
	result := &PerformanceParticipantImportResult{
		rawRows: []performanceParticipantImportRawRow{
			{
				Row:                         3,
				EmployeeID:                  "E001",
				AssessmentManagerEmployeeID: "M001",
				AssessmentManagerSource:     "manual",
				ManagerOverrideReason:       "quarterly override",
			},
			{
				Row:                   4,
				EmployeeID:            "user-2",
				AssessmentManagerName: "Manager Two",
			},
		},
	}

	if err := svc.ResolveImportedPerformanceEmployees(result); err != nil {
		t.Fatalf("ResolveImportedPerformanceEmployees() error = %v", err)
	}

	if result.ParsedCount != 2 || result.ImportedCount != 2 {
		t.Fatalf("counts = parsed:%d imported:%d, want 2/2", result.ParsedCount, result.ImportedCount)
	}
	wantUserIDs := []string{"user-1", "user-2"}
	for i, want := range wantUserIDs {
		if result.EmployeeIDs[i] != want {
			t.Fatalf("EmployeeIDs[%d] = %q, want %q", i, result.EmployeeIDs[i], want)
		}
	}
	if len(result.Employees) != 2 {
		t.Fatalf("Employees length = %d, want 2", len(result.Employees))
	}
	first := result.Employees[0]
	if first.UserID != "user-1" ||
		first.EmployeeID != "E001" ||
		first.DepartmentName != "Product" ||
		first.AssessmentManagerUserID != "manager-1" ||
		first.AssessmentManagerEmployeeID != "M001" ||
		first.AssessmentManagerName != "Manager One" ||
		first.AssessmentManagerSource != ManagerSourceManual ||
		first.ManagerOverrideReason != "quarterly override" {
		t.Fatalf("first resolved employee = %#v", first)
	}
	second := result.Employees[1]
	if second.UserID != "user-2" ||
		second.EmployeeID != "E002" ||
		second.DepartmentName != "Sales" ||
		second.AssessmentManagerUserID != "manager-2" ||
		second.AssessmentManagerName != "Manager Two" ||
		second.AssessmentManagerSource != ManagerSourceImport {
		t.Fatalf("second resolved employee = %#v", second)
	}
	if len(result.ManagerAssignments) != 2 {
		t.Fatalf("ManagerAssignments length = %d, want 2: %#v", len(result.ManagerAssignments), result.ManagerAssignments)
	}
	if len(result.MissingEmployeeIDs) != 0 || len(result.InactiveEmployeeIDs) != 0 || len(result.ManagerAssignmentSkippedRows) != 0 {
		t.Fatalf("unexpected misses/inactive/skips: missing=%v inactive=%v managerSkips=%v", result.MissingEmployeeIDs, result.InactiveEmployeeIDs, result.ManagerAssignmentSkippedRows)
	}
}

func TestResolveImportedPerformanceEmployeesReportsMissingInactiveAndInvalidManagers(t *testing.T) {
	svc := newImportStubPerformanceService(t,
		importStubTableResponse("employee_profiles", []string{"id", "user_id", "employee_id"}, [][]driver.Value{
			{int64(1), "user-1", "E001"},
			{int64(2), "user-3", "E003"},
		}),
		importStubTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
			{int64(1), "user-1", "Alice", "dept-1", "active"},
			{int64(3), "user-3", "Inactive User", "dept-1", "inactive"},
		}),
		importStubTableResponse("departments", []string{"id", "department_id", "name"}, [][]driver.Value{
			{int64(1), "dept-1", "Product"},
		}),
	)
	result := &PerformanceParticipantImportResult{
		rawRows: []performanceParticipantImportRawRow{
			{Row: 3, EmployeeID: "E001", AssessmentManagerEmployeeID: "M404"},
			{Row: 4, EmployeeID: "E404"},
			{Row: 5, EmployeeID: "E003"},
		},
	}

	if err := svc.ResolveImportedPerformanceEmployees(result); err != nil {
		t.Fatalf("ResolveImportedPerformanceEmployees() error = %v", err)
	}

	if result.ImportedCount != 1 || len(result.EmployeeIDs) != 1 || result.EmployeeIDs[0] != "user-1" {
		t.Fatalf("resolved employees = count:%d ids:%v, want user-1 only", result.ImportedCount, result.EmployeeIDs)
	}
	if len(result.MissingEmployeeIDs) != 1 || result.MissingEmployeeIDs[0] != "E404" {
		t.Fatalf("MissingEmployeeIDs = %#v, want [E404]", result.MissingEmployeeIDs)
	}
	if len(result.InactiveEmployeeIDs) != 1 || result.InactiveEmployeeIDs[0] != "E003" {
		t.Fatalf("InactiveEmployeeIDs = %#v, want [E003]", result.InactiveEmployeeIDs)
	}
	if len(result.ManagerAssignments) != 0 {
		t.Fatalf("ManagerAssignments = %#v, want empty because manager was invalid", result.ManagerAssignments)
	}
	if len(result.ManagerAssignmentSkippedRows) != 1 || result.ManagerAssignmentSkippedRows[0].Row != 3 {
		t.Fatalf("ManagerAssignmentSkippedRows = %#v, want row 3", result.ManagerAssignmentSkippedRows)
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("Warnings = %#v, want missing/inactive/manager skipped warnings", result.Warnings)
	}
}

func TestResolveImportedPerformanceEmployeesSkipsSelfManagerAssignment(t *testing.T) {
	svc := newImportStubPerformanceService(t,
		importStubTableResponse("employee_profiles", []string{"id", "user_id", "employee_id"}, [][]driver.Value{
			{int64(1), "user-1", "E001"},
		}),
		importStubTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
			{int64(1), "user-1", "Alice", "dept-1", "active"},
		}),
		importStubTableResponse("departments", []string{"id", "department_id", "name"}, [][]driver.Value{
			{int64(1), "dept-1", "Product"},
		}),
	)
	result := &PerformanceParticipantImportResult{
		rawRows: []performanceParticipantImportRawRow{
			{Row: 3, EmployeeID: "E001", AssessmentManagerEmployeeID: "E001"},
		},
	}

	if err := svc.ResolveImportedPerformanceEmployees(result); err != nil {
		t.Fatalf("ResolveImportedPerformanceEmployees() error = %v", err)
	}

	if result.ImportedCount != 1 || len(result.Employees) != 1 {
		t.Fatalf("ImportedCount/Employees = %d/%d, want 1/1", result.ImportedCount, len(result.Employees))
	}
	if result.Employees[0].AssessmentManagerUserID != "" || len(result.ManagerAssignments) != 0 {
		t.Fatalf("self manager should be skipped, employee=%#v assignments=%#v", result.Employees[0], result.ManagerAssignments)
	}
	if len(result.ManagerAssignmentSkippedRows) != 1 || result.ManagerAssignmentSkippedRows[0].Row != 3 {
		t.Fatalf("ManagerAssignmentSkippedRows = %#v, want row 3", result.ManagerAssignmentSkippedRows)
	}
	if !strings.Contains(result.ManagerAssignmentSkippedRows[0].Reason, "本人") {
		t.Fatalf("self manager skip reason = %q, want mention 本人", result.ManagerAssignmentSkippedRows[0].Reason)
	}
}

const importStubDriverName = "peopleops_import_stub_mysql"

var (
	importStubDriverOnce sync.Once
	importStubDBs        sync.Map
)

type importStubQueryResponse struct {
	match   func(query string, args []driver.NamedValue) bool
	columns []string
	rows    [][]driver.Value
}

type importStubDB struct {
	queries []importStubQueryResponse
}

type importStubDriver struct{}

type importStubConn struct {
	db *importStubDB
}

type importStubStmt struct {
	conn  *importStubConn
	query string
}

type importStubRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type importStubTx struct{}

type importStubResult struct{}

func newImportStubPerformanceService(t *testing.T, queries ...importStubQueryResponse) *PerformanceService {
	t.Helper()
	importStubDriverOnce.Do(func() {
		stdsql.Register(importStubDriverName, importStubDriver{})
	})

	dsn := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	importStubDBs.Store(dsn, &importStubDB{queries: queries})
	t.Cleanup(func() {
		importStubDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(importStubDriverName, dsn)
	if err != nil {
		t.Fatalf("open import stub sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open import stub gorm db: %v", err)
	}
	return NewPerformanceServiceWithOrgID(db, "test-org")
}

func importStubTableResponse(table string, columns []string, rows [][]driver.Value) importStubQueryResponse {
	table = strings.ToLower(table)
	return importStubQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			return strings.Contains(strings.ToLower(query), table)
		},
		columns: columns,
		rows:    rows,
	}
}

func (d importStubDriver) Open(name string) (driver.Conn, error) {
	value, ok := importStubDBs.Load(name)
	if !ok {
		return nil, fmt.Errorf("import stub db %s not registered", name)
	}
	return &importStubConn{db: value.(*importStubDB)}, nil
}

func (c *importStubConn) Prepare(query string) (driver.Stmt, error) {
	return &importStubStmt{conn: c, query: query}, nil
}

func (c *importStubConn) Close() error {
	return nil
}

func (c *importStubConn) Begin() (driver.Tx, error) {
	return importStubTx{}, nil
}

func (c *importStubConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return importStubTx{}, nil
}

func (c *importStubConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(query, args)
}

func (c *importStubConn) ExecContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return importStubResult{}, nil
}

func (c *importStubConn) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, response := range c.db.queries {
		if response.match != nil && response.match(query, args) {
			rows := make([][]driver.Value, len(response.rows))
			for i := range response.rows {
				rows[i] = append([]driver.Value(nil), response.rows[i]...)
			}
			return &importStubRows{
				columns: append([]string(nil), response.columns...),
				rows:    rows,
			}, nil
		}
	}
	return nil, fmt.Errorf("unexpected import query: %s", query)
}

func (s *importStubStmt) Close() error {
	return nil
}

func (s *importStubStmt) NumInput() int {
	return -1
}

func (s *importStubStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return importStubResult{}, nil
}

func (s *importStubStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return s.conn.query(s.query, named)
}

func (r *importStubRows) Columns() []string {
	return r.columns
}

func (r *importStubRows) Close() error {
	return nil
}

func (r *importStubRows) Next(dest []driver.Value) error {
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

func (importStubTx) Commit() error {
	return nil
}

func (importStubTx) Rollback() error {
	return nil
}

func (importStubResult) LastInsertId() (int64, error) {
	return 1, nil
}

func (importStubResult) RowsAffected() (int64, error) {
	return 1, nil
}
