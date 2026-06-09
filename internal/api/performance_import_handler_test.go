package api

import (
	"archive/zip"
	"bytes"
	"context"
	stdsql "database/sql"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"peopleops/internal/database"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestImportPerformanceActivityParticipantsHandlerParsesAndResolvesUpload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		apiImportTableResponse("employee_profiles", []string{"id", "user_id", "employee_id"}, [][]driver.Value{
			{int64(1), "user-1", "E001"},
			{int64(2), "manager-1", "M001"},
		}),
		apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
			{int64(1), "user-1", "Alice", "dept-1", "active"},
			{int64(2), "manager-1", "Boss", "dept-1", "active"},
		}),
		apiImportTableResponse("departments", []string{"id", "department_id", "name"}, [][]driver.Value{
			{int64(1), "dept-1", "Product"},
		}),
	)
	t.Cleanup(func() {
		database.DB = originalDB
	})

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartPerformanceImportRequest(t, "participants.xlsx", buildAPIPerformanceImportXLSX(t))

	ImportPerformanceActivityParticipants(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var response map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data := response["data"].(map[string]interface{})
	result := data["result"].(map[string]interface{})
	if result["imported_count"].(float64) != 1 {
		t.Fatalf("imported_count = %#v, want 1", result["imported_count"])
	}
	employees := result["employees"].([]interface{})
	employee := employees[0].(map[string]interface{})
	if employee["user_id"] != "user-1" ||
		employee["employee_id"] != "E001" ||
		employee["department_name"] != "Product" ||
		employee["assessment_manager_user_id"] != "manager-1" ||
		employee["assessment_manager_employee_id"] != "M001" {
		t.Fatalf("resolved employee = %#v", employee)
	}
	assignments := result["manager_assignments"].([]interface{})
	if len(assignments) != 1 {
		t.Fatalf("manager_assignments length = %d, want 1", len(assignments))
	}
}

func TestImportPerformanceActivityParticipantsHandlerRejectsNonXLSX(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = newMultipartPerformanceImportRequest(t, "participants.txt", []byte("not xlsx"))

	ImportPerformanceActivityParticipants(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func newMultipartPerformanceImportRequest(t *testing.T, filename string, content []byte) *http.Request {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create multipart file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write multipart file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/import", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

func buildAPIPerformanceImportXLSX(t *testing.T) []byte {
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
		"xl/worksheets/sheet1.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="1">
      <c r="A1" t="inlineStr"><is><t>绩效活动</t></is></c>
      <c r="B1" t="inlineStr"><is><t>工号</t></is></c>
      <c r="C1" t="inlineStr"><is><t>姓名</t></is></c>
      <c r="D1" t="inlineStr"><is><t>一级部门</t></is></c>
      <c r="E1" t="inlineStr"><is><t>二级部门</t></is></c>
      <c r="F1" t="inlineStr"><is><t>三级部门</t></is></c>
      <c r="G1" t="inlineStr"><is><t>考核上级工号</t></is></c>
    </row>
    <row r="2">
      <c r="A2" t="inlineStr"><is><t>Q1</t></is></c>
      <c r="B2" t="inlineStr"><is><t>E001</t></is></c>
      <c r="C2" t="inlineStr"><is><t>Alice</t></is></c>
      <c r="D2" t="inlineStr"><is><t>Product</t></is></c>
      <c r="G2" t="inlineStr"><is><t>M001</t></is></c>
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

const apiImportStubDriverName = "peopleops_api_import_stub_mysql"

var (
	apiImportStubDriverOnce sync.Once
	apiImportStubDBs        sync.Map
)

type apiImportQueryResponse struct {
	match   func(query string, args []driver.NamedValue) bool
	columns []string
	rows    [][]driver.Value
}

type apiImportStubDB struct {
	queries []apiImportQueryResponse
}

type apiImportDriver struct{}

type apiImportConn struct {
	db *apiImportStubDB
}

type apiImportStmt struct {
	conn  *apiImportConn
	query string
}

type apiImportRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type apiImportTx struct{}

type apiImportResult struct{}

func newAPIPerformanceImportStubDB(t *testing.T, queries ...apiImportQueryResponse) *gorm.DB {
	t.Helper()
	apiImportStubDriverOnce.Do(func() {
		stdsql.Register(apiImportStubDriverName, apiImportDriver{})
	})

	dsn := fmt.Sprintf("%s-%d", t.Name(), time.Now().UnixNano())
	apiImportStubDBs.Store(dsn, &apiImportStubDB{queries: queries})
	t.Cleanup(func() {
		apiImportStubDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(apiImportStubDriverName, dsn)
	if err != nil {
		t.Fatalf("open api import stub sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open api import stub gorm db: %v", err)
	}
	return db
}

func apiImportTableResponse(table string, columns []string, rows [][]driver.Value) apiImportQueryResponse {
	table = strings.ToLower(table)
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			return strings.Contains(strings.ToLower(query), table)
		},
		columns: columns,
		rows:    rows,
	}
}

func (d apiImportDriver) Open(name string) (driver.Conn, error) {
	value, ok := apiImportStubDBs.Load(name)
	if !ok {
		return nil, fmt.Errorf("api import stub db %s not registered", name)
	}
	return &apiImportConn{db: value.(*apiImportStubDB)}, nil
}

func (c *apiImportConn) Prepare(query string) (driver.Stmt, error) {
	return &apiImportStmt{conn: c, query: query}, nil
}

func (c *apiImportConn) Close() error {
	return nil
}

func (c *apiImportConn) Begin() (driver.Tx, error) {
	return apiImportTx{}, nil
}

func (c *apiImportConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return apiImportTx{}, nil
}

func (c *apiImportConn) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.query(query, args)
}

func (c *apiImportConn) ExecContext(_ context.Context, _ string, _ []driver.NamedValue) (driver.Result, error) {
	return apiImportResult{}, nil
}

func (c *apiImportConn) query(query string, args []driver.NamedValue) (driver.Rows, error) {
	for _, response := range c.db.queries {
		if response.match != nil && response.match(query, args) {
			rows := make([][]driver.Value, len(response.rows))
			for i := range response.rows {
				rows[i] = append([]driver.Value(nil), response.rows[i]...)
			}
			return &apiImportRows{
				columns: append([]string(nil), response.columns...),
				rows:    rows,
			}, nil
		}
	}
	return nil, fmt.Errorf("unexpected api import query: %s", query)
}

func (s *apiImportStmt) Close() error {
	return nil
}

func (s *apiImportStmt) NumInput() int {
	return -1
}

func (s *apiImportStmt) Exec(_ []driver.Value) (driver.Result, error) {
	return apiImportResult{}, nil
}

func (s *apiImportStmt) Query(args []driver.Value) (driver.Rows, error) {
	named := make([]driver.NamedValue, len(args))
	for i, arg := range args {
		named[i] = driver.NamedValue{Ordinal: i + 1, Value: arg}
	}
	return s.conn.query(s.query, named)
}

func (r *apiImportRows) Columns() []string {
	return r.columns
}

func (r *apiImportRows) Close() error {
	return nil
}

func (r *apiImportRows) Next(dest []driver.Value) error {
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

func (apiImportTx) Commit() error {
	return nil
}

func (apiImportTx) Rollback() error {
	return nil
}

func (apiImportResult) LastInsertId() (int64, error) {
	return 1, nil
}

func (apiImportResult) RowsAffected() (int64, error) {
	return 1, nil
}
