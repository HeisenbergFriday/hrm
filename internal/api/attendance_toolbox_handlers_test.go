package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peopleops/internal/middleware"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

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
