package api

import (
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
)

func TestPerformanceSelfParticipantVerification(t *testing.T) {
	participant := &database.PerformanceParticipant{EmployeeID: "employee-1"}

	allowedContext, allowedRecorder := newPerformanceAccessContext("employee-1")
	if !verifySelfParticipant(allowedContext, participant) {
		t.Fatalf("employee should be allowed to access own participant")
	}
	if allowedRecorder.Code != http.StatusOK {
		t.Fatalf("allowed recorder status = %d, want default 200", allowedRecorder.Code)
	}

	deniedContext, deniedRecorder := newPerformanceAccessContext("employee-2")
	if verifySelfParticipant(deniedContext, participant) {
		t.Fatalf("different employee should not be allowed to access participant")
	}
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d, want %d", deniedRecorder.Code, http.StatusForbidden)
	}

	adminContext, _ := newPerformanceAccessContext("admin")
	if !verifySelfParticipant(adminContext, participant) {
		t.Fatalf("admin should bypass self participant check")
	}
}

func TestPerformanceManagerParticipantVerification(t *testing.T) {
	managerID := "manager-1"
	participant := &database.PerformanceParticipant{EmployeeID: "employee-1", ManagerID: &managerID}

	allowedContext, _ := newPerformanceAccessContext("manager-1")
	if !verifyManagerOfParticipant(allowedContext, participant) {
		t.Fatalf("assigned manager should be allowed")
	}

	deniedContext, deniedRecorder := newPerformanceAccessContext("manager-2")
	if verifyManagerOfParticipant(deniedContext, participant) {
		t.Fatalf("different manager should not be allowed")
	}
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d, want %d", deniedRecorder.Code, http.StatusForbidden)
	}

	noManagerContext, noManagerRecorder := newPerformanceAccessContext("manager-1")
	if verifyManagerOfParticipant(noManagerContext, &database.PerformanceParticipant{EmployeeID: "employee-1"}) {
		t.Fatalf("participant without manager should not pass manager verification")
	}
	if noManagerRecorder.Code != http.StatusForbidden {
		t.Fatalf("no manager status = %d, want %d", noManagerRecorder.Code, http.StatusForbidden)
	}
}

func TestPerformanceParticipantVerificationUsesIdentityAliases(t *testing.T) {
	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
			{int64(1), "user-1", "Alice", "dept-1", "active"},
			{int64(2), "manager-1", "Manager", "dept-1", "active"},
		}),
		apiImportTableResponse("employee_profiles", []string{"id", "user_id", "employee_id"}, [][]driver.Value{
			{int64(10), "user-1", "E001"},
			{int64(11), "manager-1", "M001"},
		}),
	)
	t.Cleanup(func() {
		database.DB = originalDB
	})

	selfContext, _ := newPerformanceAccessContext("user-1")
	if !verifySelfParticipant(selfContext, &database.PerformanceParticipant{EmployeeID: "E001"}) {
		t.Fatalf("employee profile number should match current user")
	}

	managerID := "manager-1"
	managerContext, _ := newPerformanceAccessContext("2")
	if !verifyManagerOfParticipant(managerContext, &database.PerformanceParticipant{EmployeeID: "user-1", ManagerID: &managerID}) {
		t.Fatalf("numeric auth id should match assigned manager user id")
	}
}

func TestIndicatorLibraryAccessRespectsDepartmentScope(t *testing.T) {
	tests := []struct {
		name           string
		libraryDeptID  string
		wantStatusCode int
	}{
		{name: "allowed department", libraryDeptID: "dept-1", wantStatusCode: http.StatusOK},
		{name: "outside department", libraryDeptID: "dept-2", wantStatusCode: http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalDB := database.DB
			database.DB = newAPIPerformanceImportStubDB(t,
				apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
					{int64(1), "manager-1", "Manager", "dept-1", "active"},
				}),
				apiPerformanceIndicatorLibraryRowsResponse([][]driver.Value{
					{int64(1), tt.libraryDeptID, "Scoped Dept", "Scoped KPI", "", "quarterly", "active", "admin", "admin"},
				}),
				apiPerformanceRouterRolesResponse([]string{"performance:indicator:manage"}),
				apiPerformanceRouterTableResponse("data_permissions", []string{"id", "role_id", "scope", "department_keys"}, [][]driver.Value{
					{int64(1), int64(1), "department", `["dept-1"]`},
				}),
				apiImportTableResponse("departments", []string{"id", "department_id", "name", "parent_id", "extension"}, [][]driver.Value{
					{int64(1), "dept-1", "Product", "", nil},
				}),
			)
			t.Cleanup(func() {
				database.DB = originalDB
			})

			c, recorder := newPerformanceAccessContext("manager-1")
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/indicator-libraries/1", nil)
			c.Params = gin.Params{{Key: "id", Value: "1"}}

			GetIndicatorLibrary(c)

			if recorder.Code != tt.wantStatusCode {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, tt.wantStatusCode, recorder.Body.String())
			}
		})
	}
}

func TestPerformanceHandlersRejectBadInputBeforeDatabaseWork(t *testing.T) {
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		method  string
		target  string
		body    string
		params  gin.Params
	}{
		{
			name:    "create activity missing required body",
			handler: CreatePerformanceActivity,
			method:  http.MethodPost,
			target:  "/api/v1/performance/activities",
			body:    `{}`,
		},
		{
			name:    "confirm employee invalid participant id",
			handler: ConfirmEmployeeResultHandler,
			method:  http.MethodPost,
			target:  "/api/v1/performance/participants/not-a-number/confirm-employee",
			body:    `{}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "confirm manager invalid participant id",
			handler: ConfirmManagerResultHandler,
			method:  http.MethodPost,
			target:  "/api/v1/performance/participants/not-a-number/confirm-manager",
			body:    `{}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "confirm hr invalid participant id",
			handler: ConfirmHRResultHandler,
			method:  http.MethodPost,
			target:  "/api/v1/performance/participants/not-a-number/confirm-hr",
			body:    `{}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "update assessment manager invalid participant id",
			handler: UpdateParticipantAssessmentManager,
			method:  http.MethodPut,
			target:  "/api/v1/performance/participants/not-a-number/assessment-manager",
			body:    `{"manager_user_id":"manager-1"}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "batch assessment manager missing items",
			handler: BatchUpdateAssessmentManagers,
			method:  http.MethodPost,
			target:  "/api/v1/performance/activities/1/assessment-managers/batch",
			body:    `{}`,
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 先绑定 org 再校验入参：缺 org 现在会在 requirePermission 层直接 401。
			c, recorder := performanceHandlerContextAs(t, performanceHandlerTestOrgID, "admin")
			c.Params = tt.params
			c.Request = httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")
			if database.DB != nil {
				c.Set("requestDB", database.DB.WithContext(c.Request.Context()))
			}

			tt.handler(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func newPerformanceAccessContext(userID string) (*gin.Context, *httptest.ResponseRecorder) {
	// 统一写入 orgID + userID + requestmeta + requestDB，与 JWT/Tenant 中间件一致。
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	info := &requestmeta.RequestInfo{OrgID: performanceHandlerTestOrgID}
	ctx := requestmeta.WithRequestInfo(req.Context(), info)
	ctx = requestmeta.WithTenant(ctx, performanceHandlerTestOrgID)
	c.Request = req.WithContext(ctx)
	c.Set("orgID", performanceHandlerTestOrgID)
	c.Set("userID", userID)
	if database.DB != nil {
		c.Set("requestDB", database.DB.WithContext(ctx))
	}
	return c, recorder
}
