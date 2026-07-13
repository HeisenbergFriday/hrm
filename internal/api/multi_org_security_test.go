package api

import (
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"peopleops/internal/database"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// newSecurityCtx 构造已经过 JWT/AuthContext 中间件的请求上下文：写入 orgID / userID，
// 免除后续 handler 内的 currentOrgIDOrAbort 依赖真实 JWT 链路。
func newSecurityCtx(t *testing.T, method, target, body, orgID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	c.Set("orgID", orgID)
	c.Set("userID", "tester")
	return c, recorder
}

func decodeSecurityResponse(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, string(body))
	}
	return resp
}

func TestLogin_RequiresOrgID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	// 请求缺少 org_id：应该被 binding:required 拦下。
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"alice","password":"x"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	Login(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "组织") {
		t.Fatalf("body should mention org requirement, got %s", recorder.Body.String())
	}
}

func TestLogin_UnknownOrgRejected(t *testing.T) {
	originalDB := database.DB
	// 组织表查询返回空结果 → GetOrganizationByOrgID 报 not found → 400。
	database.DB = newAPIPerformanceImportStubDB(t,
		apiImportTableResponse("organizations", []string{"id", "org_id", "status"}, nil),
	)
	t.Cleanup(func() {
		database.DB = originalDB
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"alice","password":"x","org_id":"ghost"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	Login(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "组织不存在或未激活") {
		t.Fatalf("body should reject unknown org, got %s", recorder.Body.String())
	}
}

func TestLogin_WrongPasswordDoesNotFallbackAcrossOrgs(t *testing.T) {
	originalDB := database.DB
	// 组织存在，但用户名查询按 (org_id,user_id) 精准过滤时返回空 → 401。
	// 关键在于：即使别的组织存在同名用户，也不会被回退查询到。
	database.DB = newAPIPerformanceImportStubDB(t,
		apiImportTableResponse("organizations", []string{"id", "org_id", "status"}, [][]driver.Value{
			{int64(1), "muteng", "active"},
		}),
		apiImportTableResponse("users", []string{"id", "user_id", "name", "org_id", "status"}, nil),
	)
	t.Cleanup(func() {
		database.DB = originalDB
	})

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		bytes.NewReader([]byte(`{"username":"alice","password":"x","org_id":"muteng"}`)))
	c.Request.Header.Set("Content-Type", "application/json")

	Login(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestAssignUserRole_RejectsCrossOrgBody(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/permission/users/roles/assign",
		`{"user_id":"bob","role_id":42,"org_id":"other-org"}`, "muteng")

	AssignUserRole(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	resp := decodeSecurityResponse(t, recorder.Body.Bytes())
	if msg, _ := resp["message"].(string); !strings.Contains(msg, "不允许通过参数切换到其它组织") {
		t.Fatalf("unexpected message: %v", resp["message"])
	}
}

func TestRemoveUserRole_RejectsCrossOrgBody(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/permission/users/roles/remove",
		`{"user_id":"bob","role_id":42,"org_id":"other-org"}`, "muteng")

	RemoveUserRole(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetUserRoles_RejectsCrossOrgQuery(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/permission/users/bob/roles?org_id=other-org", "", "muteng")
	c.Params = gin.Params{{Key: "user_id", Value: "bob"}}

	GetUserRoles(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetUserPermissions_RejectsCrossOrgQuery(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/permission/users/bob/permissions?org_id=other-org", "", "muteng")
	c.Params = gin.Params{{Key: "user_id", Value: "bob"}}

	GetUserPermissions(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetRoleUsers_RejectsCrossOrgQuery(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/permission/roles/1/users?org_id=other-org", "", "muteng")
	c.Params = gin.Params{{Key: "role_id", Value: "1"}}

	GetRoleUsers(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestPermissionHandler_RejectsMissingOrgContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/permission/users/roles/assign",
		bytes.NewReader([]byte(`{"user_id":"bob","role_id":42}`)))
	c.Request.Header.Set("Content-Type", "application/json")
	// 故意不设置 orgID。
	c.Set("userID", "tester")

	AssignUserRole(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSyncOrgData_RejectsTargetOrgIDQuery(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/org/sync?target_org_id=other-org", "", "muteng")

	SyncOrgData(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSyncOrgData_RejectsOrgIDQuery(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/org/sync?org_id=other-org", "", "muteng")

	SyncOrgData(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}
