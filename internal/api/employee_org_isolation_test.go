package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestGetEmployeeProfiles_MissingOrgContextFailClosed 验证普通请求缺组织时不得返回员工数据。
func TestGetEmployeeProfiles_MissingOrgContextFailClosed(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/employee-profiles?department_id=same-dept", "", "")
	GetEmployeeProfiles(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "组织") {
		t.Fatalf("body should mention missing org, got %s", recorder.Body.String())
	}
}

func TestGetEmployeeProfile_MissingOrgContextFailClosed(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/employee-profiles/1", "", "")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	GetEmployeeProfile(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetEmployeeProfiles_RejectsCrossOrgQueryParam(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/employee-profiles?org_id=other-org", "", "org-a")
	GetEmployeeProfiles(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "不允许通过参数切换到其它组织") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestCreateEmployeeProfile_RejectsCrossOrgBody(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/employee-profiles",
		`{"user_id":"u1","employee_id":"e1","org_id":"other-org"}`, "org-a")
	CreateEmployeeProfile(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}
