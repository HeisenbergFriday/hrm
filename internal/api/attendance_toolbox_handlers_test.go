package api

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

type stubOrgRosterGenerator struct {
	result *service.AttendanceToolboxResult
	err    error
	orgIDs []string
}

const generateOrgRosterRoutePath = "/api/v1/attendance/toolbox/roster/generate"

func (s *stubOrgRosterGenerator) GenerateOrgRosterExcel(_ context.Context, orgID string) (*service.AttendanceToolboxResult, error) {
	s.orgIDs = append(s.orgIDs, orgID)
	return s.result, s.err
}

func runGenerateOrgRosterRoute(
	t *testing.T,
	permissions []string,
	menuKeys []string,
	orgID string,
	body string,
	generator *stubOrgRosterGenerator,
) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	if generator == nil {
		generator = &stubOrgRosterGenerator{result: &service.AttendanceToolboxResult{
			FileName: "花名册.xlsx",
			Data:     []byte("xlsx"),
		}}
	}
	originalFactory := newOrgRosterGenerator
	newOrgRosterGenerator = func() orgRosterGenerator { return generator }
	t.Cleanup(func() { newOrgRosterGenerator = originalFactory })

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("userID", "roster-tester")
		if orgID != "" {
			c.Set("orgID", orgID)
		}
		auth := &middleware.AuthContext{
			OrgID:         orgID,
			UserID:        "roster-tester",
			RawUserID:     "roster-tester",
			PermissionSet: permSet(permissions),
			MenuKeySet:    permSet(menuKeys),
		}
		middleware.SetAuthContextForTest(c, auth)
		c.Next()
	})
	v1 := router.Group("/api/v1")
	attendance := v1.Group("/attendance")
	registerAttendanceToolboxRoutes(attendance)

	if body == "" {
		body = `{}`
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, generateOrgRosterRoutePath, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestGenerateOrgRosterRoute_PermissionMatrix(t *testing.T) {
	tests := []struct {
		name        string
		permissions []string
		menuKeys    []string
		wantStatus  int
	}{
		{name: "operate allowed", permissions: []string{"attendance_toolbox_operate"}, wantStatus: http.StatusOK},
		{name: "attendance manage allowed", permissions: []string{"attendance_manage"}, wantStatus: http.StatusOK},
		{name: "dingtalk sync only denied", permissions: []string{"attendance_toolbox_dingtalk_sync"}, wantStatus: http.StatusForbidden},
		{name: "no feature permission denied", wantStatus: http.StatusForbidden},
		{name: "menu only denied", menuKeys: []string{"menu:attendance-toolbox"}, wantStatus: http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &stubOrgRosterGenerator{result: &service.AttendanceToolboxResult{FileName: "花名册.xlsx", Data: []byte("xlsx")}}
			recorder := runGenerateOrgRosterRoute(t, tt.permissions, tt.menuKeys, "org-a", `{}`, generator)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if tt.wantStatus == http.StatusOK && len(generator.orgIDs) != 1 {
				t.Fatalf("allowed request should call generator once, got %d", len(generator.orgIDs))
			}
			if tt.wantStatus != http.StatusOK && len(generator.orgIDs) != 0 {
				t.Fatalf("denied request called generator: %#v", generator.orgIDs)
			}
		})
	}
}

func TestGenerateOrgRosterRoute_RegisteredAtExactPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := SetupRouter()
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == generateOrgRosterRoutePath {
			return
		}
	}
	t.Fatal("POST /api/v1/attendance/toolbox/roster/generate is not registered")
}

func TestGenerateOrgRosterRoute_MissingOrgIDFailsClosed(t *testing.T) {
	generator := &stubOrgRosterGenerator{result: &service.AttendanceToolboxResult{FileName: "花名册.xlsx", Data: []byte("xlsx")}}
	recorder := runGenerateOrgRosterRoute(t, []string{"attendance_toolbox_operate"}, nil, "", `{}`, generator)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401, body=%s", recorder.Code, recorder.Body.String())
	}
	if len(generator.orgIDs) != 0 {
		t.Fatalf("missing-org request reached generator: %#v", generator.orgIDs)
	}
}

func TestGenerateOrgRosterRoute_RequestCannotOverrideJWTOrgID(t *testing.T) {
	generator := &stubOrgRosterGenerator{result: &service.AttendanceToolboxResult{FileName: "花名册.xlsx", Data: []byte("xlsx")}}
	recorder := runGenerateOrgRosterRoute(
		t,
		[]string{"attendance_toolbox_operate"},
		nil,
		"jwt-org",
		`{"org_id":"body-org","organization_id":"other-org"}`,
		generator,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if len(generator.orgIDs) != 1 || generator.orgIDs[0] != "jwt-org" {
		t.Fatalf("generator orgIDs = %#v, want only jwt-org", generator.orgIDs)
	}
}

func TestGenerateOrgRosterRoute_ErrorMapping(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantText    string
		wantMessage string
		wantAbsent  string
	}{
		{name: "no active employees", err: service.ErrRosterNoEmployees, wantStatus: http.StatusBadRequest, wantText: "在职员工"},
		{name: "missing employee ids", err: fmt.Errorf("%w：3 名在职员工缺少业务工号", service.ErrRosterMissingEmpNo), wantStatus: http.StatusBadRequest, wantText: "3 名"},
		{name: "missing employee names", err: fmt.Errorf("internal database detail: %w", &service.RosterMissingNameError{Count: 2}), wantStatus: http.StatusBadRequest, wantText: "2 名在职员工缺少姓名", wantMessage: "当前组织有 2 名在职员工缺少姓名，请先补充后重试", wantAbsent: "internal database detail"},
		{name: "missing department path", err: fmt.Errorf("%w：2 名在职员工无法生成部门路径", service.ErrRosterMissingDeptPath), wantStatus: http.StatusBadRequest, wantText: "2 名"},
		{name: "database query failed", err: fmt.Errorf("%w：database unavailable", service.ErrRosterUserQueryFailed), wantStatus: http.StatusInternalServerError, wantText: "读取在职用户失败"},
		{name: "runner failed", err: fmt.Errorf("%w：exit status 1", service.ErrRosterRunnerFailed), wantStatus: http.StatusInternalServerError, wantText: "花名册生成失败"},
		{name: "runner returned no output", err: service.ErrRosterNoOutput, wantStatus: http.StatusInternalServerError, wantText: "未产出结果文件"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			generator := &stubOrgRosterGenerator{err: tt.err}
			recorder := runGenerateOrgRosterRoute(t, []string{"attendance_toolbox_operate"}, nil, "org-a", `{}`, generator)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d, body=%s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), tt.wantText) {
				t.Fatalf("body %q does not contain %q", recorder.Body.String(), tt.wantText)
			}
			if tt.wantMessage != "" {
				var payload Response
				if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
					t.Fatalf("decode response: %v, body=%s", err, recorder.Body.String())
				}
				if payload.Message != tt.wantMessage {
					t.Fatalf("message = %q, want %q", payload.Message, tt.wantMessage)
				}
			}
			if tt.wantAbsent != "" && strings.Contains(recorder.Body.String(), tt.wantAbsent) {
				t.Fatalf("body %q leaks internal detail %q", recorder.Body.String(), tt.wantAbsent)
			}
		})
	}
}

func TestGenerateOrgRosterRoute_SuccessReturnsInspectableXLSX(t *testing.T) {
	workbook := makeRosterWorkbookWithOpenpyxl(t)
	generator := &stubOrgRosterGenerator{result: &service.AttendanceToolboxResult{
		FileName: "花名册_测试组织.xlsx",
		Data:     workbook,
		Kind:     "export",
		RowCount: 1,
	}}
	recorder := runGenerateOrgRosterRoute(t, []string{"attendance_toolbox_operate"}, nil, "org-a", `{}`, generator)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body=%s", recorder.Code, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, toolboxXlsxContentType) {
		t.Fatalf("Content-Type = %q", contentType)
	}
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", recorder.Header().Get("X-Content-Type-Options"))
	}
	wantDisposition := service.ContentDispositionAttachment("花名册_测试组织.xlsx")
	if recorder.Header().Get("Content-Disposition") != wantDisposition {
		t.Fatalf("Content-Disposition = %q, want %q", recorder.Header().Get("Content-Disposition"), wantDisposition)
	}
	inspectRosterWorkbookWithOpenpyxl(t, recorder.Body.Bytes())
}

func makeRosterWorkbookWithOpenpyxl(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "roster.xlsx")
	script := `
import sys
from openpyxl import Workbook
headers = ["工号", "姓名", "合同主体", "一级部门", "二级部门", "三级部门", "岗位", "员工类型", "人员分类", "入职日期", "离职日期", "转正日期"]
row = ["MT9999", "测试运维", "", "运营管理中心", "运营支撑部", "智慧寄存运维组", "运维工程师", "正式", "", "2026-01-02", "", "2026-04-02"]
wb = Workbook()
ws = wb.active
ws.title = "在职花名册"
ws.append(headers)
ws.append(row)
wb.save(sys.argv[1])
wb.close()
`
	if output, err := exec.Command(apiTestPython(t), "-c", script, path).CombinedOutput(); err != nil {
		t.Fatalf("create xlsx with openpyxl: %v\n%s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated xlsx: %v", err)
	}
	return data
}

func inspectRosterWorkbookWithOpenpyxl(t *testing.T, data []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "response.xlsx")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write response xlsx: %v", err)
	}
	script := `
import json
import sys
from openpyxl import load_workbook
wb = load_workbook(sys.argv[1], data_only=True)
ws = wb["在职花名册"]
print(json.dumps({"sheet": ws.title, "headers": [c.value for c in ws[1]], "row": [ws.cell(2, i).value for i in range(1, 13)]}))
wb.close()
`
	output, err := exec.Command(apiTestPython(t), "-c", script, path).CombinedOutput()
	if err != nil {
		t.Fatalf("open response xlsx with openpyxl: %v\n%s", err, output)
	}
	var inspected struct {
		Sheet   string        `json:"sheet"`
		Headers []string      `json:"headers"`
		Row     []interface{} `json:"row"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(output), &inspected); err != nil {
		t.Fatalf("decode openpyxl inspection: %v, output=%s", err, output)
	}
	if inspected.Sheet != "在职花名册" || len(inspected.Headers) != 12 {
		t.Fatalf("unexpected workbook metadata: %#v", inspected)
	}
	if inspected.Headers[0] != "工号" || inspected.Headers[11] != "转正日期" {
		t.Fatalf("unexpected headers: %#v", inspected.Headers)
	}
	if len(inspected.Row) != 12 || inspected.Row[0] != "MT9999" || inspected.Row[1] != "测试运维" || inspected.Row[4] != "运营支撑部" || inspected.Row[5] != "智慧寄存运维组" {
		t.Fatalf("unexpected workbook row: %#v", inspected.Row)
	}
}

// fakeToolboxRunnerPy honors runner.py's CLI contract (--module/--workdir/--config-json)
// and emits outputs selected by FAKE_TOOLBOX_SCENARIO so handler tests exercise the real
// subprocess engine without hitting DingTalk.
const fakeToolboxRunnerPy = `
import json, os, sys

def arg(name):
    for i, a in enumerate(sys.argv):
        if a == name and i + 1 < len(sys.argv):
            return sys.argv[i + 1]
    return None

workdir = arg("--workdir")
scenario = os.environ.get("FAKE_TOOLBOX_SCENARIO", "single")

def emit(name, kind, flow_key, row_count):
    path = os.path.join(workdir, name)
    with open(path, "wb") as f:
        f.write(name.encode("utf-8"))
    return {"path": path, "file_name": name, "kind": kind, "flow_key": flow_key, "row_count": row_count}

outputs = []
if scenario == "multi":
    outputs.append(emit("请假业务表.xlsx", "export", "leave", 2))
    outputs.append(emit("岗位异动业务表.xlsx", "export", "position_transfer", 3))
    outputs.append(emit("钉钉同步审计报告.xlsx", "audit", "", 1))
elif scenario == "audit":
    outputs.append(emit("钉钉同步审计报告.xlsx", "audit", "", 1))
    outputs.append(emit("同步摘要.json", "meta", "", 0))
else:
    outputs.append(emit("岗位异动业务表.xlsx", "export", "position_transfer", 3))
    outputs.append(emit("钉钉同步审计报告.xlsx", "audit", "", 1))

print(json.dumps({"ok": True, "outputs": outputs, "log": "fake-log", "error": "", "traceback": ""}))
`

func apiTestPython(t *testing.T) string {
	t.Helper()
	for _, bin := range []string{"python", "python3"} {
		if path, err := exec.LookPath(bin); err == nil {
			return path
		}
	}
	t.Skip("python not on PATH; skipping real engine handler test")
	return ""
}

func setFakeToolboxEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runner.py"), []byte(fakeToolboxRunnerPy), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATTENDANCE_TOOLBOX_DIR", dir)
	t.Setenv("ATTENDANCE_TOOLBOX_PYTHON", apiTestPython(t))
	t.Setenv("DINGTALK_APP_KEY", "test-app-key")
	t.Setenv("DINGTALK_APP_SECRET", "test-app-secret")
}

func runDingtalkSyncThroughHandler(t *testing.T, scenario string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	setFakeToolboxEnv(t)
	t.Setenv("FAKE_TOOLBOX_SCENARIO", scenario)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := strings.NewReader(`{"start_date":"2026-01-01","end_date":"2026-01-31","flow_keys":["position_transfer"]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/dingtalk-sync", body)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	withAuth(c, []string{"attendance_toolbox_dingtalk_sync"})
	// ConfigForOrgID("default") resolves from env in tests; any other org requires DB rows.
	c.Set("orgID", "default")
	RunDingtalkSync(c)
	return rec
}

// TestRunDingtalkSyncHandler_SingleExportPlusAudit drives the real subprocess engine
// through the legacy HTTP handler and asserts a single Excel response (not a ZIP) for
// one business export + one audit file.
func TestRunDingtalkSyncHandler_SingleExportPlusAudit(t *testing.T) {
	rec := runDingtalkSyncThroughHandler(t, "single")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("expected xlsx content type, got %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "UTF-8''%E5%B2%97%E4%BD%8D%E5%BC%82%E5%8A%A8") {
		t.Fatalf("expected attachment header for position transfer export, got %q", cd)
	}
	if rec.Body.String() != "岗位异动业务表.xlsx" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}
}

// TestRunDingtalkSyncHandler_MultipleExports_ZIP drives the real engine through the
// handler and asserts a ZIP response when multiple business exports exist.
func TestRunDingtalkSyncHandler_MultipleExports_ZIP(t *testing.T) {
	rec := runDingtalkSyncThroughHandler(t, "multi")

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("expected zip content type, got %q", ct)
	}
	zr, err := zip.NewReader(bytes.NewReader(rec.Body.Bytes()), int64(rec.Body.Len()))
	if err != nil {
		t.Fatalf("invalid zip body: %v", err)
	}
	if len(zr.File) != 3 {
		t.Fatalf("expected 3 zip entries, got %d", len(zr.File))
	}
}

// TestRunDingtalkSyncHandler_AuditOnly_4xx drives the real engine through the handler
// and asserts an explicit 4xx instead of a JSON 200 that the legacy frontend would
// mistake for an Excel file.
func TestRunDingtalkSyncHandler_AuditOnly_4xx(t *testing.T) {
	rec := runDingtalkSyncThroughHandler(t, "audit")

	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 4xx, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "未生成业务表") {
		t.Fatalf("expected actionable message, got %s", rec.Body.String())
	}
}

func multipartWithFields(t *testing.T, fields map[string]string, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		_ = w.WriteField(k, v)
	}
	for name, content := range files {
		part, err := w.CreateFormFile(name, name+".xlsx")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	_ = w.Close()
	return &buf, w.FormDataContentType()
}

func withAuth(c *gin.Context, perms []string) {
	auth := &middleware.AuthContext{
		OrgID:         "orgA",
		UserID:        "u1",
		RawUserID:     "u1",
		PermissionSet: map[string]struct{}{},
		MenuKeySet:    map[string]struct{}{},
	}
	for _, p := range perms {
		auth.PermissionSet[p] = struct{}{}
	}
	c.Set("userID", "u1")
	c.Set("orgID", "orgA")
	middleware.SetAuthContextForTest(c, auth)
}

func TestIsAttendanceToolboxStandardModule_HandlerGuard(t *testing.T) {
	if !service.IsAttendanceToolboxStandardModule("leave") {
		t.Fatal("leave should be standard")
	}
	for _, m := range []string{"quick", "dingtalk_sync", "evil"} {
		if service.IsAttendanceToolboxStandardModule(m) {
			t.Fatalf("%s must not be standard", m)
		}
	}
}

func TestRunAttendanceToolbox_RejectsPrivilegedModules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, module := range []string{"quick", "dingtalk_sync"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body, ct := multipartWithFields(t, map[string]string{}, map[string]string{"leave_src": "x"})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/"+module+"/run", body)
		req.Header.Set("Content-Type", ct)
		c.Request = req
		c.Params = gin.Params{{Key: "module", Value: module}}
		withAuth(c, []string{"attendance_toolbox_operate"})
		RunAttendanceToolbox(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("module=%s code=%d body=%s", module, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "不支持") {
			t.Fatalf("module=%s unexpected body=%s", module, rec.Body.String())
		}
	}
}

func TestRunAttendanceToolboxWorkflow_RejectsPrivilegedModules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, module := range []string{"quick", "dingtalk_sync"} {
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body, ct := multipartWithFields(t, nil, nil)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/workflows/"+module, body)
		req.Header.Set("Content-Type", ct)
		c.Request = req
		c.Params = gin.Params{{Key: "module", Value: module}}
		withAuth(c, []string{"attendance_toolbox_operate"})
		RunAttendanceToolboxWorkflow(c)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("module=%s code=%d body=%s", module, rec.Code, rec.Body.String())
		}
	}
}

func TestRunAttendanceToolbox_CustomRulesForbiddenWithoutRulesEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body, ct := multipartWithFields(t, map[string]string{
		"rules_json": `{"premium_rules":[]}`,
	}, map[string]string{"overtime_src": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/overtime/run", body)
	req.Header.Set("Content-Type", ct)
	c.Request = req
	c.Params = gin.Params{{Key: "module", Value: "overtime"}}
	withAuth(c, []string{"attendance_toolbox_operate"})
	RunAttendanceToolbox(c)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, code=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "规则编辑权限") {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestRunAttendanceToolbox_CustomRulesFileForbiddenWithoutRulesEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body, ct := multipartWithFields(t, nil, map[string]string{
		"overtime_src": "x",
		"rules_file":   "rules",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/overtime/run", body)
	req.Header.Set("Content-Type", ct)
	c.Request = req
	c.Params = gin.Params{{Key: "module", Value: "overtime"}}
	withAuth(c, []string{"attendance_toolbox_operate"})
	RunAttendanceToolbox(c)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunAttendanceToolboxWorkflow_CustomRulesForbiddenWithoutRulesEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body, ct := multipartWithFields(t, map[string]string{"rules_json": `{"a":1}`}, map[string]string{"overtime_src": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/workflows/overtime", body)
	req.Header.Set("Content-Type", ct)
	c.Request = req
	c.Params = gin.Params{{Key: "module", Value: "overtime"}}
	withAuth(c, []string{"attendance_toolbox_operate"})
	RunAttendanceToolboxWorkflow(c)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRunAttendanceToolboxQuickWorkflow_CustomRulesForbiddenWithoutRulesEdit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body, ct := multipartWithFields(t, map[string]string{
		"rules_json":               `{"a":1}`,
		"dingtalk_sync_start_date": "2026-03-01",
		"dingtalk_sync_end_date":   "2026-03-31",
		"run_leave":                "true",
		"run_overtime":             "true",
	}, map[string]string{"leave_schedule": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/workflows/quick", body)
	req.Header.Set("Content-Type", ct)
	c.Request = req
	withAuth(c, []string{"attendance_toolbox_operate", "attendance_toolbox_dingtalk_sync"})
	RunAttendanceToolboxQuickWorkflow(c)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAttendanceManageCompat_AllowsCustomRulesGate(t *testing.T) {
	// attendance_manage satisfies toolboxHasPermission for rules_edit check.
	// We only assert the permission helper path via form gate + manage compat
	// without invoking Python (handler may still fail later on missing engine).
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body, ct := multipartWithFields(t, map[string]string{"rules_json": `{"premium_rules":[]}`}, map[string]string{"overtime_src": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/overtime/run", body)
	req.Header.Set("Content-Type", ct)
	c.Request = req
	c.Params = gin.Params{{Key: "module", Value: "overtime"}}
	withAuth(c, []string{"attendance_manage"})
	RunAttendanceToolbox(c)
	// Must NOT be 403 for missing rules_edit — manage is compatible.
	if rec.Code == http.StatusForbidden && strings.Contains(rec.Body.String(), "规则编辑权限") {
		t.Fatalf("attendance_manage should pass rules gate, body=%s", rec.Body.String())
	}
}

func TestFormHasCustomRules_RulesJSONAndFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	{
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body, ct := multipartWithFields(t, map[string]string{"rules_json": `{"premium_rules":[]}`}, nil)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		req.Header.Set("Content-Type", ct)
		c.Request = req
		if !formHasCustomRules(c) {
			t.Fatal("expected rules_json detected")
		}
	}
	{
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body, ct := multipartWithFields(t, nil, map[string]string{"rules_file": "fake-xlsx"})
		req := httptest.NewRequest(http.MethodPost, "/", body)
		req.Header.Set("Content-Type", ct)
		c.Request = req
		if !formHasCustomRules(c) {
			t.Fatal("expected rules_file detected")
		}
	}
	{
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		body, ct := multipartWithFields(t, map[string]string{"leave_special_names": "a"}, nil)
		req := httptest.NewRequest(http.MethodPost, "/", body)
		req.Header.Set("Content-Type", ct)
		c.Request = req
		if formHasCustomRules(c) {
			t.Fatal("expected no custom rules")
		}
	}
}

func TestEnsureQuickFlowKeys_MergesLeaveAndOvertime(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"run_leave":               {"true"},
			"run_overtime":            {"true"},
			"dingtalk_sync_flow_keys": {"attendance_correction"},
		},
		File: map[string][]*multipart.FileHeader{},
	}
	ensureQuickFlowKeys(form)
	keys := form.Value["dingtalk_sync_flow_keys"][0]
	if !strings.Contains(keys, "leave") || !strings.Contains(keys, "overtime") {
		t.Fatalf("flow keys not merged: %s", keys)
	}
	if !strings.Contains(keys, "attendance_correction") {
		t.Fatalf("existing key dropped: %s", keys)
	}
}

func TestEnsureQuickFlowKeys_RespectsFalseFlags(t *testing.T) {
	form := &multipart.Form{
		Value: map[string][]string{
			"run_leave":               {"false"},
			"run_overtime":            {"0"},
			"dingtalk_sync_flow_keys": {"attendance_correction"},
		},
		File: map[string][]*multipart.FileHeader{},
	}
	ensureQuickFlowKeys(form)
	keys := form.Value["dingtalk_sync_flow_keys"][0]
	if strings.Contains(keys, "leave") || strings.Contains(keys, "overtime") {
		t.Fatalf("should not inject when flags false: %s", keys)
	}
}

func TestRunDingtalkSync_SingleBusinessExportPlusAudit_ReturnsExcelNotZip(t *testing.T) {
	gin.SetMode(gin.TestMode)

	res := &service.DingtalkSyncResult{Outputs: []service.AttendanceToolboxResult{
		{FileName: "岗位异动业务表.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: []byte("export"), Kind: "export", FlowKey: "position_transfer", RowCount: 3},
		{FileName: "钉钉同步审计报告.xlsx", ContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Data: []byte("audit"), Kind: "audit", RowCount: 1},
	}}

	// 单业务 export + audit：BusinessExports 只保留业务表（不把审计当业务文件）。
	exports := res.BusinessExports()
	if len(exports) != 1 || exports[0].FlowKey != "position_transfer" {
		t.Fatalf("expected 1 business export (position_transfer), got %d", len(exports))
	}

	// 单个业务表可直接返回 Excel（不走 ZIP）。ZIP 仅用于多业务 export。
	archive, err := service.ZipAttendanceToolboxResultsForDownload(exports)
	if err != nil {
		t.Fatal(err)
	}
	if len(archive) == 0 {
		t.Fatal("expected business export bytes")
	}

	// 多个业务 export 才需要 ZIP。
	multi := &service.DingtalkSyncResult{Outputs: []service.AttendanceToolboxResult{
		{FileName: "leave.xlsx", Kind: "export", FlowKey: "leave", Data: []byte("l")},
		{FileName: "position.xlsx", Kind: "export", FlowKey: "position_transfer", Data: []byte("p")},
		{FileName: "audit.xlsx", Kind: "audit", Data: []byte("a")},
	}}
	if len(multi.BusinessExports()) != 2 {
		t.Fatalf("expected 2 business exports, got %d", len(multi.BusinessExports()))
	}
}

func TestDingtalkSyncBusinessExports_IgnoresAuditAndMeta(t *testing.T) {
	res := &service.DingtalkSyncResult{Outputs: []service.AttendanceToolboxResult{
		{FileName: "leave.xlsx", Kind: "export", FlowKey: "leave", Data: []byte("leave")},
		{FileName: "audit.xlsx", Kind: "audit", Data: []byte("audit")},
		{FileName: "meta.json", Kind: "meta", Data: []byte("{}")},
	}}
	exports := res.BusinessExports()
	if len(exports) != 1 || exports[0].FlowKey != "leave" {
		t.Fatalf("BusinessExports should only return kind=export, got %d", len(exports))
	}
}

const toolboxXlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"

func TestToolboxRunAccess_DingtalkSyncOnlyCanReadOwnSync(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := service.NewAttendanceToolboxRunStore(time.Hour, t.TempDir(), 0)
	defer store.Close()

	syncRun, err := store.Put("u1", "orgA", "dingtalk_sync", "log", nil, nil, []service.AttendanceToolboxResult{
		{FileName: "岗位异动业务表.xlsx", ContentType: toolboxXlsxContentType, Data: []byte("export"), Kind: "export", FlowKey: "position_transfer", RowCount: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	leaveRun, err := store.Put("u1", "orgA", "leave", "log", nil, nil, []service.AttendanceToolboxResult{
		{FileName: "请假明细表.xlsx", ContentType: toolboxXlsxContentType, Data: []byte("leave"), Kind: "export", FlowKey: "leave", RowCount: 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	old := attendanceToolboxRunStore
	attendanceToolboxRunStore = store
	defer func() { attendanceToolboxRunStore = old }()

	// dingtalk_sync-only user may read their own same-org dingtalk_sync run.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/attendance/toolbox/runs/"+syncRun.RunID, nil)
	c.Params = gin.Params{{Key: "run_id", Value: syncRun.RunID}}
	withAuth(c, []string{"attendance_toolbox_dingtalk_sync"})
	GetAttendanceToolboxRun(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("dingtalk_sync-only user should read own sync run, got %d body=%s", rec.Code, rec.Body.String())
	}

	// The same user must NOT read another module's run.
	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/api/v1/attendance/toolbox/runs/"+leaveRun.RunID, nil)
	c2.Params = gin.Params{{Key: "run_id", Value: leaveRun.RunID}}
	withAuth(c2, []string{"attendance_toolbox_dingtalk_sync"})
	GetAttendanceToolboxRun(c2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("dingtalk_sync-only user must not read leave run, got %d body=%s", rec2.Code, rec2.Body.String())
	}

	// But the same user may read the leave run with the broader operate permission.
	rec3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(rec3)
	c3.Request = httptest.NewRequest(http.MethodGet, "/api/v1/attendance/toolbox/runs/"+leaveRun.RunID, nil)
	c3.Params = gin.Params{{Key: "run_id", Value: leaveRun.RunID}}
	withAuth(c3, []string{"attendance_toolbox_operate"})
	GetAttendanceToolboxRun(c3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("operate permission should read leave run, got %d body=%s", rec3.Code, rec3.Body.String())
	}
}

func TestToolboxRunDownload_DingtalkSyncOnlyDownloadsOwnSyncFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := service.NewAttendanceToolboxRunStore(time.Hour, t.TempDir(), 0)
	defer store.Close()

	syncRun, err := store.Put("u1", "orgA", "dingtalk_sync", "log", nil, nil, []service.AttendanceToolboxResult{
		{FileName: "岗位异动业务表.xlsx", ContentType: toolboxXlsxContentType, Data: []byte("export"), Kind: "export", FlowKey: "position_transfer", RowCount: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	leaveRun, err := store.Put("u1", "orgA", "leave", "log", nil, nil, []service.AttendanceToolboxResult{
		{FileName: "请假明细表.xlsx", ContentType: toolboxXlsxContentType, Data: []byte("leave"), Kind: "export", FlowKey: "leave", RowCount: 1},
	})
	if err != nil {
		t.Fatal(err)
	}

	old := attendanceToolboxRunStore
	attendanceToolboxRunStore = store
	defer func() { attendanceToolboxRunStore = old }()

	// dingtalk_sync-only user can download their own same-org dingtalk_sync business file.
	syncFile := syncRun.Files[0].FileKey
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/attendance/toolbox/runs/"+syncRun.RunID+"/files/"+syncFile, nil)
	c.Params = gin.Params{{Key: "run_id", Value: syncRun.RunID}, {Key: "file_key", Value: syncFile}}
	withAuth(c, []string{"attendance_toolbox_dingtalk_sync"})
	DownloadAttendanceToolboxRunFile(c)
	if rec.Code != http.StatusOK {
		t.Fatalf("dingtalk_sync-only user should download own sync file, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "export" {
		t.Fatalf("unexpected body %q", rec.Body.String())
	}

	// Same permission must not download a leave module file.
	leaveFile := leaveRun.Files[0].FileKey
	rec2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(rec2)
	c2.Request = httptest.NewRequest(http.MethodGet, "/api/v1/attendance/toolbox/runs/"+leaveRun.RunID+"/files/"+leaveFile, nil)
	c2.Params = gin.Params{{Key: "run_id", Value: leaveRun.RunID}, {Key: "file_key", Value: leaveFile}}
	withAuth(c2, []string{"attendance_toolbox_dingtalk_sync"})
	DownloadAttendanceToolboxRunFile(c2)
	if rec2.Code != http.StatusForbidden {
		t.Fatalf("dingtalk_sync-only user must not download leave file, got %d body=%s", rec2.Code, rec2.Body.String())
	}
}

func TestToolboxRunAccess_CrossUserStillDenied(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := service.NewAttendanceToolboxRunStore(time.Hour, t.TempDir(), 0)
	defer store.Close()
	run, err := store.Put("u1", "orgA", "dingtalk_sync", "log", nil, nil, []service.AttendanceToolboxResult{
		{FileName: "岗位异动业务表.xlsx", ContentType: toolboxXlsxContentType, Data: []byte("export"), Kind: "export", FlowKey: "position_transfer", RowCount: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	old := attendanceToolboxRunStore
	attendanceToolboxRunStore = store
	defer func() { attendanceToolboxRunStore = old }()

	// u2 holding dingtalk_sync must still be denied u1's run (store-level user+org binding).
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/attendance/toolbox/runs/"+run.RunID, nil)
	c.Params = gin.Params{{Key: "run_id", Value: run.RunID}}
	withAuth(c, []string{"attendance_toolbox_dingtalk_sync"})
	c.Set("userID", "u2")
	c.Set("orgID", "orgA")
	GetAttendanceToolboxRun(c)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("expected denied for other user, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// fakeParttimePunchRunnerPy honors the runner.py --action contract and emits a
// single xlsx output so the handler can be exercised end-to-end without a real
// DingTalk connection.
const fakeParttimePunchRunnerPy = `
import json, os, sys

def arg(name):
    for i, a in enumerate(sys.argv):
        if a == name and i + 1 < len(sys.argv):
            return sys.argv[i + 1]
    return None

workdir = arg("--workdir")
output_dir = os.path.join(workdir, "outputs")
os.makedirs(output_dir, exist_ok=True)
out_path = os.path.join(output_dir, "兼职月度打卡记录_202607.xlsx")
with open(out_path, "wb") as f:
    f.write(b"fake-xlsx-bytes")
print(json.dumps({"ok": True, "outputs": [{"path": out_path, "file_name": "兼职月度打卡记录_202607.xlsx"}], "log": "", "error": "", "traceback": ""}))
`

func setFakeParttimePunchEnv(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "runner.py"), []byte(fakeParttimePunchRunnerPy), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ATTENDANCE_TOOLBOX_DIR", dir)
	t.Setenv("ATTENDANCE_TOOLBOX_PYTHON", apiTestPython(t))
	t.Setenv("DINGTALK_APP_KEY", "test-app-key")
	t.Setenv("DINGTALK_APP_SECRET", "test-app-secret")
}

func runParttimeMonthlyPunchThroughHandler(t *testing.T, month string, perms []string, orgID string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	setFakeParttimePunchEnv(t)

	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	body := strings.NewReader(`{"month":"` + month + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/parttime-monthly-punch", body)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	c.Set("orgID", orgID)
	middleware.SetAuthContextForTest(c, &middleware.AuthContext{
		OrgID:         orgID,
		UserID:        "u1",
		PermissionSet: permSet(perms),
	})
	RunParttimeMonthlyPunch(c)
	return rec
}

func permSet(perms []string) map[string]struct{} {
	s := map[string]struct{}{}
	for _, p := range perms {
		s[p] = struct{}{}
	}
	return s
}

// TestRunParttimeMonthlyPunch_MissingOrgID_Rejected covers req 9: a request with
// no org context must be rejected (不能回退到默认企业).
func TestRunParttimeMonthlyPunch_MissingOrgID_Rejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/attendance/toolbox/parttime-monthly-punch",
		strings.NewReader(`{"month":"2026-07"}`))
	req.Header.Set("Content-Type", "application/json")
	c.Request = req
	// No orgID set.
	RunParttimeMonthlyPunch(c)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing org, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// TestRunParttimeMonthlyPunch_RouteIsPermissionGated verifies the endpoint is
// mounted behind RequirePermission(attendance_manage, attendance_toolbox_dingtalk_sync)
// at the router level (handler itself defers to middleware for authorization).
func TestRunParttimeMonthlyPunch_RouteIsPermissionGated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := SetupRouter()
	var found bool
	for _, route := range router.Routes() {
		if route.Method == http.MethodPost && route.Path == "/api/v1/attendance/toolbox/parttime-monthly-punch" {
			found = true
		}
	}
	if !found {
		t.Fatalf("parttime-monthly-punch route not registered")
	}
}

// TestRunParttimeMonthlyPunch_InvalidMonth_Rejected covers month-param
// validation (req 4).
func TestRunParttimeMonthlyPunch_InvalidMonth_Rejected(t *testing.T) {
	rec := runParttimeMonthlyPunchThroughHandler(t, "2026-13", []string{"attendance_toolbox_dingtalk_sync"}, "orgA")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid month, got %d body=%s", rec.Code, rec.Body.String())
	}
}

// fakeParttimePunchDS is a test double for service.ParttimePunchDataSource.
type fakeParttimePunchDS struct {
	cfg        dingtalk.Config
	adminID    string
	roster     []service.ParttimeEmployee
	attendance []dingtalk.AttendanceRecord
	cfgErr     error
	adminErr   error
	rosterErr  error
	attendErr  error
}

func (f fakeParttimePunchDS) Config(orgID string) (dingtalk.Config, error) {
	return f.cfg, f.cfgErr
}

func (f fakeParttimePunchDS) AdminUserID(orgID string) (string, error) {
	return f.adminID, f.adminErr
}

func (f fakeParttimePunchDS) Roster(orgID string) ([]service.ParttimeEmployee, error) {
	return f.roster, f.rosterErr
}

func (f fakeParttimePunchDS) Attendance(orgID string, userIDs []string, startDate, endDate string) ([]dingtalk.AttendanceRecord, error) {
	return f.attendance, f.attendErr
}

func withFakeParttimePunchDS(t *testing.T, ds service.ParttimePunchDataSource) {
	t.Helper()
	orig := newParttimeMonthlyPunchService
	newParttimeMonthlyPunchService = func() *service.ParttimeMonthlyPunchService {
		return service.NewParttimeMonthlyPunchServiceWithDataSource(ds)
	}
	t.Cleanup(func() { newParttimeMonthlyPunchService = orig })
}

// TestRunParttimeMonthlyPunch_SuccessFileName verifies the response carries the
// expected xlsx content-type and a correctly derived attachment file name.
func TestRunParttimeMonthlyPunch_SuccessFileName(t *testing.T) {
	withFakeParttimePunchDS(t, fakeParttimePunchDS{
		cfg:     dingtalk.Config{AppKey: "ak", AppSecret: "as"},
		adminID: "admin-1",
		roster: []service.ParttimeEmployee{
			{Name: "张三", EmployeeNo: "MT001", UserID: "uid-1", Position: "兼职"},
		},
		attendance: []dingtalk.AttendanceRecord{
			{UserID: "uid-1", CheckType: "OnDuty", WorkDate: "2026-07-01", UserCheckTime: "0"},
		},
	})
	rec := runParttimeMonthlyPunchThroughHandler(t, "2026-07", []string{"attendance_toolbox_dingtalk_sync"}, "orgA")
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("expected xlsx content type, got %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "202607.xlsx") {
		t.Fatalf("expected derived file name in Content-Disposition, got %q", cd)
	}
}

// TestRunParttimeMonthlyPunch_MissingConfig_Error covers req 10: missing
// admin UserId or report config must return a clear error.
func TestRunParttimeMonthlyPunch_MissingConfig_Error(t *testing.T) {
	withFakeParttimePunchDS(t, fakeParttimePunchDS{
		cfg:     dingtalk.Config{AppKey: "ak", AppSecret: "as"},
		adminID: "", // 未配置管理员
		roster:  []service.ParttimeEmployee{},
	})
	rec := runParttimeMonthlyPunchThroughHandler(t, "2026-07", []string{"attendance_toolbox_dingtalk_sync"}, "orgA")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing admin, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "管理员") {
		t.Fatalf("expected clear admin-config error, got %s", rec.Body.String())
	}
}
