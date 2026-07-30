package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/requestmeta"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type orgSyncTestEnvelope struct {
	Code    int                 `json:"code"`
	Message string              `json:"message"`
	Data    orgSyncResponseData `json:"data"`
}

func successfulOrgSyncResult() orgSyncResponseData {
	return orgSyncResponseData{
		Departments: orgSyncStageResult{Status: "success", SuccessCount: 19},
		Employees:   orgSyncEmployeeStageResult{Status: "success", SuccessCount: 525},
	}
}

func stubOrgSyncRunner(t *testing.T, runner func(*gin.Context, string, string, time.Time) orgSyncResponseData) {
	t.Helper()
	original := runOrgSyncForRequest
	runOrgSyncForRequest = runner
	t.Cleanup(func() { runOrgSyncForRequest = original })
}

func newOrgSyncContext(t *testing.T, orgID, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, target, bytes.NewBufferString(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	if orgID != "" {
		c.Set("orgID", orgID)
		c.Request = c.Request.WithContext(requestmeta.WithTenant(c.Request.Context(), orgID))
	}
	c.Set("userID", "tester")
	c.Set("requestID", "org-sync-test-request")
	return c, recorder
}

func decodeOrgSyncEnvelope(t *testing.T, recorder *httptest.ResponseRecorder) orgSyncTestEnvelope {
	t.Helper()
	var response orgSyncTestEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return response
}

func stubOrgSyncSources(t *testing.T, departments func(string) ([]dingtalk.DeptInfo, error), users func(string) ([]dingtalk.UserInfo, error), usersWithDepartments func(string, []dingtalk.DeptInfo) ([]dingtalk.UserInfo, error), persist func(*gin.Context, string, []dingtalk.DeptInfo) (service.OrgDepartmentSyncResult, error)) {
	t.Helper()
	originalDepartments := syncDepartmentsForOrg
	originalUsers := syncUsersForOrg
	originalUsersWithDepartments := syncUsersWithDeptsForOrg
	originalPersist := persistOrgSyncDepartments
	if departments != nil {
		syncDepartmentsForOrg = departments
	}
	if users != nil {
		syncUsersForOrg = users
	}
	if usersWithDepartments != nil {
		syncUsersWithDeptsForOrg = usersWithDepartments
	}
	if persist != nil {
		persistOrgSyncDepartments = persist
	}
	t.Cleanup(func() {
		syncDepartmentsForOrg = originalDepartments
		syncUsersForOrg = originalUsers
		syncUsersWithDeptsForOrg = originalUsersWithDepartments
		persistOrgSyncDepartments = originalPersist
	})
}

func stubOrgSyncStatusWriter(t *testing.T, writer func(*gin.Context, string, orgSyncStatusUpdate)) {
	t.Helper()
	original := writeOrgSyncStatusForRequest
	writeOrgSyncStatusForRequest = func(c *gin.Context, orgID string, update orgSyncStatusUpdate) error {
		writer(c, orgID, update)
		return nil
	}
	t.Cleanup(func() { writeOrgSyncStatusForRequest = original })
}

func useOrgSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	if err := db.AutoMigrate(&database.SyncStatus{}); err != nil {
		t.Fatalf("migrate sync status: %v", err)
	}
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })
	return db
}

func newOrgSyncQueryContext(t *testing.T, orgID, requestID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	c, recorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync/"+requestID, "")
	c.Request.Method = http.MethodGet
	c.Params = gin.Params{{Key: "request_id", Value: requestID}}
	return c, recorder
}

func waitForOrgSyncStatus(t *testing.T, db *gorm.DB, orgID, requestID, wanted string) database.SyncStatus {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var status database.SyncStatus
		err := db.Where("org_id = ? AND type = ? AND request_id = ?", orgID, "organization", requestID).First(&status).Error
		if err == nil && status.Status == wanted {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("sync status did not become %q for org=%q request=%q", wanted, orgID, requestID)
	return database.SyncStatus{}
}

func TestComputeSyncOverallResult(t *testing.T) {
	tests := []struct {
		name        string
		departments string
		employees   string
		status      string
		httpStatus  int
	}{
		{name: "all success", departments: "success", employees: "success", status: "success", httpStatus: http.StatusOK},
		{name: "department failed and employees skipped", departments: "failed", employees: "skipped", status: "failed", httpStatus: http.StatusInternalServerError},
		{name: "employee partial", departments: "success", employees: "partial_failed", status: "partial_failed", httpStatus: http.StatusMultiStatus},
		{name: "all failed", departments: "failed", employees: "failed", status: "failed", httpStatus: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := computeSyncOverallResult(test.departments, test.employees)
			if result.OverallStatus != test.status || result.HTTPStatus != test.httpStatus {
				t.Fatalf("result = %#v, want status=%s http=%d", result, test.status, test.httpStatus)
			}
		})
	}
}

func TestSyncOrgData_ResponseContract(t *testing.T) {
	tests := []struct {
		name       string
		result     orgSyncResponseData
		httpStatus int
		overall    string
	}{
		{name: "all success", result: successfulOrgSyncResult(), httpStatus: http.StatusOK, overall: "success"},
		{
			name: "department failed and employees skipped",
			result: orgSyncResponseData{
				Departments: orgSyncStageResult{Status: "failed", FailCount: 1, Error: departmentSyncFailedMessage, ErrorCode: dingtalk.ErrorCodeResponseInvalid},
				Employees:   orgSyncEmployeeStageResult{Status: "skipped", Error: employeeSyncSkippedMessage, ErrorCode: employeeSyncSkippedErrorCode},
			},
			httpStatus: http.StatusInternalServerError,
			overall:    "failed",
		},
		{
			name: "employee partial failed",
			result: orgSyncResponseData{
				Departments: orgSyncStageResult{Status: "success", SuccessCount: 19},
				Employees:   orgSyncEmployeeStageResult{Status: "partial_failed", SuccessCount: 500, FailCount: 25, Error: employeeSyncFailedMessage},
			},
			httpStatus: http.StatusMultiStatus,
			overall:    "partial_failed",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stubOrgSyncRunner(t, func(*gin.Context, string, string, time.Time) orgSyncResponseData {
				return test.result
			})
			c, recorder := newOrgSyncContext(t, "contract-org-"+test.name, "/api/v1/org/sync", `{}`)
			SyncOrgData(c)

			if recorder.Code != test.httpStatus {
				t.Fatalf("status = %d, want %d; body=%s", recorder.Code, test.httpStatus, recorder.Body.String())
			}
			response := decodeOrgSyncEnvelope(t, recorder)
			if response.Message != test.overall || response.Data.OverallStatus != test.overall {
				t.Fatalf("message/data status = %q/%q, want %q", response.Message, response.Data.OverallStatus, test.overall)
			}
			if response.Data.RequestID == "" || response.Data.DurationMS < 0 || response.Data.SyncTime == "" {
				t.Fatalf("missing diagnostics: %#v", response.Data)
			}
		})
	}
}

func TestSyncOrgDataDetachesClientCancellation(t *testing.T) {
	stubOrgSyncRunner(t, func(c *gin.Context, _ string, _ string, _ time.Time) orgSyncResponseData {
		if err := c.Request.Context().Err(); err != nil {
			t.Fatalf("sync context remained canceled: %v", err)
		}
		if _, ok := c.Request.Context().Deadline(); !ok {
			t.Fatal("sync context has no execution deadline")
		}
		return successfulOrgSyncResult()
	})

	c, recorder := newOrgSyncContext(t, "detached-context-org", "/api/v1/org/sync", `{}`)
	canceledContext, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(canceledContext)
	cancel()

	SyncOrgData(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
}

func TestFetchOrgSyncUsers_DepartmentFailureSkipsEmployees(t *testing.T) {
	originalUsers := syncUsersWithDeptsForOrg
	userCalls := 0
	syncUsersWithDeptsForOrg = func(string, []dingtalk.DeptInfo) ([]dingtalk.UserInfo, error) {
		userCalls++
		return nil, nil
	}
	t.Cleanup(func() {
		syncUsersWithDeptsForOrg = originalUsers
	})

	deptErr := errors.New("department source unavailable")
	users, userErr := fetchOrgSyncUsers("source-failure-org", nil, deptErr)
	if userErr == nil {
		t.Fatal("expected employee stage to fail after department source failure")
	}
	if userCalls != 0 || users != nil {
		t.Fatalf("employee source called after department failure: calls=%d users=%v", userCalls, users)
	}
}

func TestSyncOrgData_ConcurrentSameOrgRejectedAndLockReleased(t *testing.T) {
	orgID := "same-org-lock-test"
	orgSyncLocks.Delete(orgID)
	started := make(chan struct{})
	release := make(chan struct{})
	stubOrgSyncRunner(t, func(_ *gin.Context, gotOrgID, _ string, _ time.Time) orgSyncResponseData {
		if gotOrgID == orgID {
			select {
			case <-started:
			default:
				close(started)
				<-release
			}
		}
		return successfulOrgSyncResult()
	})

	firstCtx, firstRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
	firstDone := make(chan struct{})
	go func() {
		SyncOrgData(firstCtx)
		close(firstDone)
	}()
	<-started

	secondCtx, secondRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
	SyncOrgData(secondCtx)
	if secondRecorder.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want 409; body=%s", secondRecorder.Code, secondRecorder.Body.String())
	}

	close(release)
	<-firstDone
	if firstRecorder.Code != http.StatusOK {
		t.Fatalf("first status = %d, want 200", firstRecorder.Code)
	}

	thirdCtx, thirdRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
	SyncOrgData(thirdCtx)
	if thirdRecorder.Code != http.StatusOK {
		t.Fatalf("lock was not released, status=%d body=%s", thirdRecorder.Code, thirdRecorder.Body.String())
	}
}

func TestSyncOrgData_DifferentOrganizationsDoNotBlock(t *testing.T) {
	orgA := "parallel-org-a"
	orgB := "parallel-org-b"
	orgSyncLocks.Delete(orgA)
	orgSyncLocks.Delete(orgB)
	started := make(chan struct{})
	release := make(chan struct{})
	stubOrgSyncRunner(t, func(_ *gin.Context, orgID, _ string, _ time.Time) orgSyncResponseData {
		if orgID == orgA {
			close(started)
			<-release
		}
		return successfulOrgSyncResult()
	})

	ctxA, recorderA := newOrgSyncContext(t, orgA, "/api/v1/org/sync", `{}`)
	doneA := make(chan struct{})
	go func() {
		SyncOrgData(ctxA)
		close(doneA)
	}()
	<-started

	ctxB, recorderB := newOrgSyncContext(t, orgB, "/api/v1/org/sync", `{}`)
	SyncOrgData(ctxB)
	if recorderB.Code != http.StatusOK {
		t.Fatalf("org B status = %d, want 200; body=%s", recorderB.Code, recorderB.Body.String())
	}
	close(release)
	<-doneA
	if recorderA.Code != http.StatusOK {
		t.Fatalf("org A status = %d, want 200", recorderA.Code)
	}
}

func TestSyncOrgData_RejectsClientOrganizationSelectors(t *testing.T) {
	stubOrgSyncRunner(t, func(*gin.Context, string, string, time.Time) orgSyncResponseData {
		t.Fatal("sync runner must not execute for cross-organization input")
		return orgSyncResponseData{}
	})
	tests := []struct {
		name   string
		target string
		body   string
		header string
	}{
		{name: "query org_id", target: "/api/v1/org/sync?org_id=other-org", body: `{}`},
		{name: "query target_org_id", target: "/api/v1/org/sync?target_org_id=other-org", body: `{}`},
		{name: "body org_id", target: "/api/v1/org/sync", body: `{"org_id":"other-org"}`},
		{name: "header org_id", target: "/api/v1/org/sync", body: `{}`, header: "other-org"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c, recorder := newOrgSyncContext(t, "current-org", test.target, test.body)
			if test.header != "" {
				c.Request.Header.Set("X-Org-ID", test.header)
			}
			SyncOrgData(c)
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSyncOrgData_MissingOrgContext(t *testing.T) {
	c, recorder := newOrgSyncContext(t, "", "/api/v1/org/sync", `{}`)
	SyncOrgData(c)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestSafeSyncErrorSummary_RedactsSecretsAndControlCharacters(t *testing.T) {
	err := errors.New("request failed\nhttps://oapi.example/path?access_token=token-value&x=1 password=hunter2 Authorization Bearer-secret Cookie session-cookie \"AppKey\":\"app-key\" AppSecret app-secret root:db-pass@tcp(db:3306)/peopleops SQL SELECT * FROM users")
	summary := safeSyncErrorSummary(err)
	for _, secret := range []string{"token-value", "hunter2", "Bearer-secret", "session-cookie", "app-key", "app-secret", "db-pass", "SELECT * FROM users", "\n"} {
		if strings.Contains(summary, secret) {
			t.Fatalf("summary leaked %q: %s", secret, summary)
		}
	}
	if !strings.Contains(summary, "request failed") || !strings.Contains(summary, "[REDACTED]") {
		t.Fatalf("summary lost safe diagnostic context: %s", summary)
	}
}

func TestValidateOrgSyncDepartmentsRejectsEmptyAndMissingRoot(t *testing.T) {
	tests := []struct {
		name     string
		depts    []dingtalk.DeptInfo
		wantCode string
	}{
		{name: "empty payload", wantCode: dingtalk.ErrorCodeDepartmentEmpty},
		{
			name:     "missing root",
			depts:    []dingtalk.DeptInfo{{DeptID: 2, Name: "研发部", ParentID: 1}},
			wantCode: dingtalk.ErrorCodeResponseInvalid,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOrgSyncDepartments(test.depts)
			if err == nil {
				t.Fatal("validation unexpectedly succeeded")
			}
			code, _ := orgSyncErrorDetails(err, "fallback", "fallback")
			if code != test.wantCode {
				t.Fatalf("error code = %q, want %q; err=%v", code, test.wantCode, err)
			}
		})
	}
}

func TestPerformOrgSync_DepartmentFinalFailureSkipsEmployeesAndStoresSafeMessages(t *testing.T) {
	userCalls := 0
	type statusWrite struct {
		OrgID, Type, Status, Message string
	}
	var statuses []statusWrite
	stubOrgSyncStatusWriter(t, func(_ *gin.Context, orgID string, update orgSyncStatusUpdate) {
		statuses = append(statuses, statusWrite{orgID, update.SyncType, update.Status, update.Message})
	})
	secretError := errors.New("persist failed access_token=token-value password=hunter2 SQL UPDATE departments SET name='x'")
	stubOrgSyncSources(t,
		func(string) ([]dingtalk.DeptInfo, error) {
			return []dingtalk.DeptInfo{
				{DeptID: 1, Name: "根部门", ParentID: 0},
				{DeptID: 2, Name: "研发部", ParentID: 1},
			}, nil
		},
		nil,
		func(string, []dingtalk.DeptInfo) ([]dingtalk.UserInfo, error) {
			userCalls++
			return nil, nil
		},
		func(*gin.Context, string, []dingtalk.DeptInfo) (service.OrgDepartmentSyncResult, error) {
			return service.OrgDepartmentSyncResult{}, secretError
		},
	)

	var logs bytes.Buffer
	originalWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(originalWriter) })

	c, _ := newOrgSyncContext(t, "persist-failure-org", "/api/v1/org/sync", `{}`)
	result := performOrgSync(c, "persist-failure-org", "persist-failure-request", time.Now())

	if result.Departments.Status != "failed" || result.Departments.ErrorCode != departmentPersistErrorCode ||
		result.Employees.Status != "skipped" || result.Employees.ErrorCode != employeeSyncSkippedErrorCode {
		t.Fatalf("unexpected stage result: %#v", result)
	}
	if result.Employees.Error != employeeSyncSkippedMessage || userCalls != 0 {
		t.Fatalf("employee stage was not skipped safely: calls=%d result=%#v", userCalls, result.Employees)
	}

	encoded, _ := json.Marshal(struct {
		Result   orgSyncResponseData
		Statuses []statusWrite
		Logs     string
	}{result, statuses, logs.String()})
	visible := string(encoded)
	for _, secret := range []string{"token-value", "hunter2", "UPDATE departments"} {
		if strings.Contains(visible, secret) {
			t.Fatalf("response/status/log leaked %q: %s", secret, visible)
		}
	}
	if !strings.Contains(logs.String(), "persist-failure-request") || !strings.Contains(logs.String(), "error_summary") {
		t.Fatalf("log missing request diagnostics: %s", logs.String())
	}
}

func TestPerformOrgSync_DepartmentValidationFailureSkipsEmployees(t *testing.T) {
	userCalls := 0
	persistCalls := 0
	stubOrgSyncStatusWriter(t, func(*gin.Context, string, orgSyncStatusUpdate) {})
	stubOrgSyncSources(t,
		func(string) ([]dingtalk.DeptInfo, error) {
			return []dingtalk.DeptInfo{{DeptID: 0, Name: ""}}, nil
		},
		nil,
		func(string, []dingtalk.DeptInfo) ([]dingtalk.UserInfo, error) {
			userCalls++
			return nil, nil
		},
		func(*gin.Context, string, []dingtalk.DeptInfo) (service.OrgDepartmentSyncResult, error) {
			persistCalls++
			return service.OrgDepartmentSyncResult{}, nil
		},
	)
	c, _ := newOrgSyncContext(t, "validation-failure-org", "/api/v1/org/sync", `{}`)
	result := performOrgSync(c, "validation-failure-org", "validation-request", time.Now())
	if result.Departments.Status != "failed" || result.Employees.Error != employeeSyncSkippedMessage {
		t.Fatalf("unexpected result: %#v", result)
	}
	if persistCalls != 0 || userCalls != 0 {
		t.Fatalf("invalid departments reached downstream stages: persist=%d users=%d", persistCalls, userCalls)
	}
}

func TestSyncDingTalkUsers_CountsEachEmployeeOnce(t *testing.T) {
	tests := []struct {
		name             string
		existing         bool
		createUserErr    error
		updateUserErr    error
		roleErr          error
		createProfileErr error
		users            []dingtalk.UserInfo
		wantSuccess      int
		wantFail         int
	}{
		{name: "user create failure", createUserErr: errors.New("create failed"), wantFail: 1},
		{name: "default role failure", roleErr: errors.New("role failed"), wantFail: 1},
		{name: "profile create failure", createProfileErr: errors.New("profile failed"), wantFail: 1},
		{name: "role and profile failure", roleErr: errors.New("role failed"), createProfileErr: errors.New("profile failed"), wantFail: 1},
		{name: "existing user update failure", existing: true, updateUserErr: errors.New("update failed"), wantFail: 1},
		{
			name: "deduplicated success",
			users: []dingtalk.UserInfo{
				{UserID: "employee-1", Name: "第一版", Active: true},
				{UserID: "employee-1", Name: "第二版", Active: true},
			},
			wantSuccess: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			users := test.users
			if len(users) == 0 {
				users = []dingtalk.UserInfo{{UserID: "employee-1", Name: "员工", Active: true}}
			}
			deps := orgSyncUserDependencies{
				FindUser: func(userID string) (*database.User, error) {
					if test.existing {
						return &database.User{OrgID: "count-org", UserID: userID, Status: "active"}, nil
					}
					return nil, gorm.ErrRecordNotFound
				},
				CreateUser: func(*database.User) error { return test.createUserErr },
				UpdateUser: func(*database.User) error { return test.updateUserErr },
				AssignDefaultRole: func(string) (bool, error) {
					return test.roleErr == nil, test.roleErr
				},
				FindProfile: func(userID string) (*database.EmployeeProfile, error) {
					if test.existing {
						return &database.EmployeeProfile{OrgID: "count-org", UserID: userID}, nil
					}
					return nil, gorm.ErrRecordNotFound
				},
				CreateProfile: func(*database.EmployeeProfile) error { return test.createProfileErr },
				UpdateProfile: func(*database.EmployeeProfile) error { return nil },
			}
			result := syncDingTalkUsers(context.Background(), "count-org", users, false, deps, "count-request", "co***rg", time.Now())
			if result.SuccessCount != test.wantSuccess || result.FailCount != test.wantFail {
				t.Fatalf("result=%#v, want success=%d fail=%d", result, test.wantSuccess, test.wantFail)
			}
			uniqueCount := len(dedupeOrgSyncUsers(users))
			if result.SuccessCount+result.FailCount > uniqueCount {
				t.Fatalf("overlapping counts: result=%#v unique=%d", result, uniqueCount)
			}
		})
	}
}

func TestSyncDingTalkUsers_PartialFailureHasNonOverlappingCounts(t *testing.T) {
	users := []dingtalk.UserInfo{
		{UserID: "employee-ok", Name: "成功员工", Active: true},
		{UserID: "employee-fail", Name: "失败员工", Active: true},
	}
	deps := orgSyncUserDependencies{
		FindUser: func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		CreateUser: func(user *database.User) error {
			if strings.Contains(user.UserID, "employee-fail") {
				return errors.New("create failed")
			}
			return nil
		},
		UpdateUser:        func(*database.User) error { return nil },
		AssignDefaultRole: func(string) (bool, error) { return true, nil },
		FindProfile:       func(string) (*database.EmployeeProfile, error) { return nil, gorm.ErrRecordNotFound },
		CreateProfile:     func(*database.EmployeeProfile) error { return nil },
		UpdateProfile:     func(*database.EmployeeProfile) error { return nil },
	}
	result := syncDingTalkUsers(context.Background(), "partial-org", users, false, deps, "partial-request", "pa***rg", time.Now())
	if result.Status != "partial_failed" || result.SuccessCount != 1 || result.FailCount != 1 {
		t.Fatalf("unexpected partial result: %#v", result)
	}
}

func TestSyncDingTalkUsersStopsWhenRequestCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	dependencyCalls := 0
	deps := orgSyncUserDependencies{
		FindUser: func(string) (*database.User, error) {
			dependencyCalls++
			return nil, gorm.ErrRecordNotFound
		},
	}
	result := syncDingTalkUsers(ctx, "cancel-org", []dingtalk.UserInfo{{
		UserID: "employee-1",
		Name:   "测试员工",
		Active: true,
	}}, false, deps, "cancel-request", "ca***rg", time.Now())

	if dependencyCalls != 0 {
		t.Fatalf("dependencies called after cancellation: %d", dependencyCalls)
	}
	if result.Status != "failed" || result.ErrorCode != employeeSyncCanceledErrorCode || result.SuccessCount != 0 || result.FailCount != 0 {
		t.Fatalf("unexpected canceled result: %#v", result)
	}
}

func TestDingTalkOrgUserIgnoresMissingAndPlaceholderMobile(t *testing.T) {
	created := newLocalUserFromDingTalk("mobile-org", dingtalk.UserInfo{
		UserID: "employee-1",
		Name:   "测试员工",
		Mobile: "10000000000",
	}, "1", "active")
	if created.Mobile != "" {
		t.Fatalf("placeholder mobile persisted for new user: %q", created.Mobile)
	}

	existing := &database.User{OrgID: "mobile-org", UserID: "employee-1", Mobile: "13800000000"}
	applyDingTalkOrgUser(existing, "mobile-org", dingtalk.UserInfo{UserID: "employee-1"}, "1", "active", true)
	if existing.Mobile != "13800000000" {
		t.Fatalf("empty source mobile overwrote local mobile: %q", existing.Mobile)
	}
	applyDingTalkOrgUser(existing, "mobile-org", dingtalk.UserInfo{UserID: "employee-1", Mobile: "10000000000"}, "1", "active", false)
	if existing.Mobile != "13800000000" {
		t.Fatalf("placeholder source mobile overwrote local mobile: %q", existing.Mobile)
	}
	applyDingTalkOrgUser(existing, "mobile-org", dingtalk.UserInfo{UserID: "employee-1", Mobile: "13900000000"}, "1", "active", false)
	if existing.Mobile != "13900000000" {
		t.Fatalf("real source mobile was not applied: %q", existing.Mobile)
	}
}

func TestSyncDingTalkUsersPreservesHistoricalLocalIdentity(t *testing.T) {
	existing := &database.User{
		OrgID:          "history-org",
		UserID:         "legacy-user-id",
		DingTalkUserID: "dt-user-1",
		Name:           "旧姓名",
		DepartmentID:   "legacy-department-id",
		Status:         "active",
	}
	profile := &database.EmployeeProfile{
		OrgID:      "history-org",
		UserID:     "legacy-user-id",
		EmployeeID: "legacy-user-id",
	}
	createCalls := 0
	var updated *database.User
	deps := orgSyncUserDependencies{
		FindUser: func(userID string) (*database.User, error) {
			if userID != scopedDingTalkID("history-org", "dt-user-1") {
				t.Fatalf("unexpected scoped lookup: %q", userID)
			}
			return nil, gorm.ErrRecordNotFound
		},
		FindUserByDingTalkID: func(userID string) (*database.User, error) {
			switch userID {
			case "dt-user-1":
				return existing, nil
			case "dt-manager-1":
				return &database.User{OrgID: "history-org", UserID: "legacy-manager-id", DingTalkUserID: userID}, nil
			default:
				return nil, gorm.ErrRecordNotFound
			}
		},
		FindDepartmentByDingTalkID: func(departmentID string) (*database.Department, error) {
			if departmentID != "2" {
				t.Fatalf("unexpected department lookup: %q", departmentID)
			}
			return &database.Department{OrgID: "history-org", DepartmentID: "legacy-department-id", DingTalkDepartmentID: "2"}, nil
		},
		CreateUser: func(*database.User) error {
			createCalls++
			return nil
		},
		UpdateUser: func(user *database.User) error {
			copy := *user
			updated = &copy
			return nil
		},
		AssignDefaultRole: func(string) (bool, error) { return false, nil },
		FindProfile: func(userID string) (*database.EmployeeProfile, error) {
			if userID != "legacy-user-id" {
				t.Fatalf("profile lookup used new scoped id: %q", userID)
			}
			return profile, nil
		},
		CreateProfile: func(*database.EmployeeProfile) error { return nil },
		UpdateProfile: func(*database.EmployeeProfile) error { return nil },
	}

	result := syncDingTalkUsers(context.Background(), "history-org", []dingtalk.UserInfo{{
		UserID:        "dt-user-1",
		Name:          "新姓名",
		DeptIDList:    []int64{2},
		ManagerUserID: "dt-manager-1",
		Active:        true,
	}}, false, deps, "history-request", "hi***rg", time.Now())
	if result.Status != "success" || result.SuccessCount != 1 || result.FailCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if createCalls != 0 || updated == nil {
		t.Fatalf("historical user was not updated in place: creates=%d updated=%#v", createCalls, updated)
	}
	if updated.UserID != "legacy-user-id" || updated.DingTalkUserID != "dt-user-1" ||
		updated.DepartmentID != "legacy-department-id" || updated.ManagerUserID != "legacy-manager-id" {
		t.Fatalf("historical identity or references changed unexpectedly: %#v", updated)
	}
}

func TestSyncDingTalkUsersPersistsAllDepartmentMembershipsAndDeactivatesMissingUsers(t *testing.T) {
	var createdUser *database.User
	var membershipUserID string
	var membershipDepartmentIDs []string
	var sourceUserIDs []string
	var revokedUserIDs []string
	deps := orgSyncUserDependencies{
		FindUser:             func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		FindUserByDingTalkID: func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		FindDepartmentByDingTalkID: func(departmentID string) (*database.Department, error) {
			if departmentID == "9" {
				return nil, gorm.ErrRecordNotFound
			}
			return &database.Department{DepartmentID: "local-dept-" + departmentID}, nil
		},
		CreateUser: func(user *database.User) error {
			copy := *user
			createdUser = &copy
			return nil
		},
		UpdateUser:        func(*database.User) error { return nil },
		AssignDefaultRole: func(string) (bool, error) { return false, nil },
		FindProfile:       func(string) (*database.EmployeeProfile, error) { return nil, gorm.ErrRecordNotFound },
		CreateProfile:     func(*database.EmployeeProfile) error { return nil },
		UpdateProfile:     func(*database.EmployeeProfile) error { return nil },
		ReplaceDepartmentMemberships: func(userID string, departmentIDs []string) error {
			membershipUserID = userID
			membershipDepartmentIDs = append([]string(nil), departmentIDs...)
			return nil
		},
		DeactivateMissingUsers: func(userIDs []string) ([]string, error) {
			sourceUserIDs = append([]string(nil), userIDs...)
			return []string{"stale-local-user"}, nil
		},
		RevokeSessions: func(userID string) { revokedUserIDs = append(revokedUserIDs, userID) },
	}

	result := syncDingTalkUsers(context.Background(), "multi-dept-org", []dingtalk.UserInfo{{
		UserID:     "dt-user-1",
		Name:       "多部门员工",
		DeptIDList: []int64{9, 2, 3, 2},
		Active:     true,
	}}, false, deps, "multi-dept-request", "mu***rg", time.Now())

	if result.Status != "success" || result.SuccessCount != 1 || result.FailCount != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.DeactivatedMissingCount != 1 {
		t.Fatalf("deactivated count = %d, want 1", result.DeactivatedMissingCount)
	}
	if createdUser == nil || createdUser.DepartmentID != "local-dept-2" {
		t.Fatalf("created user = %#v, want primary department local-dept-2", createdUser)
	}
	if membershipUserID != scopedDingTalkID("multi-dept-org", "dt-user-1") {
		t.Fatalf("membership user id = %q", membershipUserID)
	}
	wantDepartments := []string{"local-dept-2", "local-dept-3"}
	if len(membershipDepartmentIDs) != len(wantDepartments) {
		t.Fatalf("memberships = %#v, want %#v", membershipDepartmentIDs, wantDepartments)
	}
	for index := range wantDepartments {
		if membershipDepartmentIDs[index] != wantDepartments[index] {
			t.Fatalf("memberships = %#v, want %#v", membershipDepartmentIDs, wantDepartments)
		}
	}
	if len(sourceUserIDs) != 1 || sourceUserIDs[0] != "dt-user-1" {
		t.Fatalf("source user ids = %#v", sourceUserIDs)
	}
	if len(revokedUserIDs) != 1 || revokedUserIDs[0] != "stale-local-user" {
		t.Fatalf("revoked user ids = %#v", revokedUserIDs)
	}
}

func TestSyncDingTalkUsersDeactivationFailureDoesNotInflateEmployeeCounts(t *testing.T) {
	deps := orgSyncUserDependencies{
		FindUser:          func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		CreateUser:        func(*database.User) error { return nil },
		AssignDefaultRole: func(string) (bool, error) { return false, nil },
		FindProfile:       func(string) (*database.EmployeeProfile, error) { return nil, gorm.ErrRecordNotFound },
		CreateProfile:     func(*database.EmployeeProfile) error { return nil },
		DeactivateMissingUsers: func([]string) ([]string, error) {
			return nil, errors.New("deactivation database failure")
		},
	}

	result := syncDingTalkUsers(context.Background(), "deactivation-org", []dingtalk.UserInfo{{
		UserID: "employee-1",
		Name:   "员工",
		Active: true,
	}}, false, deps, "deactivation-request", "de***rg", time.Now())
	if result.Status != "partial_failed" || result.ErrorCode != employeeDeactivationFailedErrorCode {
		t.Fatalf("unexpected deactivation failure result: %#v", result)
	}
	if result.SuccessCount != 1 || result.FailCount != 0 || result.SuccessCount+result.FailCount > 1 {
		t.Fatalf("deactivation failure inflated employee counts: %#v", result)
	}
	if result.DeactivationStatus != "failed" || result.DeactivationError != employeeDeactivationFailedMessage {
		t.Fatalf("missing safe deactivation diagnostics: %#v", result)
	}
}

func TestSyncDingTalkUsersEmptySourceDoesNotDeactivate(t *testing.T) {
	deactivateCalled := false
	revokeCalled := false
	deps := orgSyncUserDependencies{
		FindUser:               func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		CreateUser:             func(*database.User) error { return nil },
		UpdateUser:             func(*database.User) error { return nil },
		AssignDefaultRole:      func(string) (bool, error) { return false, nil },
		FindProfile:            func(string) (*database.EmployeeProfile, error) { return nil, gorm.ErrRecordNotFound },
		CreateProfile:          func(*database.EmployeeProfile) error { return nil },
		UpdateProfile:          func(*database.EmployeeProfile) error { return nil },
		DeactivateMissingUsers: func([]string) ([]string, error) { deactivateCalled = true; return nil, nil },
		RevokeSessions:         func(string) { revokeCalled = true },
	}
	result := syncDingTalkUsers(context.Background(), "empty-source-org", nil, false, deps, "empty-source-request", "em***rg", time.Now())
	if deactivateCalled {
		t.Fatalf("deactivate must not run for empty source list")
	}
	if revokeCalled {
		t.Fatalf("sessions must not be revoked for empty source list")
	}
	if result.DeactivatedMissingCount != 0 {
		t.Fatalf("deactivated count = %d, want 0", result.DeactivatedMissingCount)
	}
}

func TestSyncDingTalkUsersCanceledContextDoesNotDeactivate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	deactivateCalled := false
	deps := orgSyncUserDependencies{
		FindUser:               func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		DeactivateMissingUsers: func([]string) ([]string, error) { deactivateCalled = true; return nil, nil },
	}
	result := syncDingTalkUsers(ctx, "cancel-deactivate-org", []dingtalk.UserInfo{{
		UserID: "employee-1",
		Name:   "测试员工",
		Active: true,
	}}, false, deps, "cancel-deactivate-request", "ca***rg", time.Now())
	if deactivateCalled {
		t.Fatalf("deactivate must not run after request cancellation")
	}
	if result.Status != "failed" || result.ErrorCode != employeeSyncCanceledErrorCode {
		t.Fatalf("unexpected canceled result: %#v", result)
	}
	if result.DeactivatedMissingCount != 0 {
		t.Fatalf("deactivated count = %d, want 0", result.DeactivatedMissingCount)
	}
}

func TestSyncDingTalkUsersDeactivateFailureKeepsEmployeeCountsAndRevokesOnlyForSuccess(t *testing.T) {
	var revokedUserIDs []string
	deps := orgSyncUserDependencies{
		FindUser:               func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		CreateUser:             func(*database.User) error { return nil },
		AssignDefaultRole:      func(string) (bool, error) { return false, nil },
		FindProfile:            func(string) (*database.EmployeeProfile, error) { return nil, gorm.ErrRecordNotFound },
		CreateProfile:          func(*database.EmployeeProfile) error { return nil },
		DeactivateMissingUsers: func([]string) ([]string, error) { return nil, errors.New("deactivate failed") },
		RevokeSessions:         func(userID string) { revokedUserIDs = append(revokedUserIDs, userID) },
	}
	result := syncDingTalkUsers(context.Background(), "deactivate-fail-org", []dingtalk.UserInfo{{
		UserID: "employee-1",
		Name:   "测试员工",
		Active: true,
	}}, false, deps, "deactivate-fail-request", "de***rg", time.Now())
	if result.SuccessCount != 1 || result.FailCount != 0 {
		t.Fatalf("employee counts = success %d fail %d, want 1/0", result.SuccessCount, result.FailCount)
	}
	if result.Status != "partial_failed" || result.ErrorCode != employeeDeactivationFailedErrorCode {
		t.Fatalf("deactivation failure was not reported as partial: %#v", result)
	}
	if result.DeactivatedMissingCount != 0 {
		t.Fatalf("deactivated count = %d, want 0 on failure", result.DeactivatedMissingCount)
	}
	if len(revokedUserIDs) != 0 {
		t.Fatalf("sessions revoked on deactivate failure: %#v", revokedUserIDs)
	}
}

func TestSyncDingTalkUsersRejectsCrossTenantDepartment(t *testing.T) {
	createCalls := 0
	membershipCalls := 0
	deps := orgSyncUserDependencies{
		FindUser:             func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		FindUserByDingTalkID: func(string) (*database.User, error) { return nil, gorm.ErrRecordNotFound },
		FindDepartmentByDingTalkID: func(string) (*database.Department, error) {
			return &database.Department{OrgID: "org-b", DepartmentID: "shared-department"}, nil
		},
		CreateUser: func(*database.User) error {
			createCalls++
			return nil
		},
		ReplaceDepartmentMemberships: func(string, []string) error {
			membershipCalls++
			return nil
		},
	}

	result := syncDingTalkUsers(context.Background(), "org-a", []dingtalk.UserInfo{{
		UserID:     "shared-user",
		Name:       "跨租户部门员工",
		DeptIDList: []int64{2},
		Active:     true,
	}}, false, deps, "cross-org-request", "or***-a", time.Now())

	if result.Status != "failed" || result.SuccessCount != 0 || result.FailCount != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if createCalls != 0 || membershipCalls != 0 {
		t.Fatalf("cross-tenant department was persisted: create=%d memberships=%d", createCalls, membershipCalls)
	}
}

func TestStartOrgSyncDataReturnsAcceptedAndCompletesInBackground(t *testing.T) {
	orgID := "async-org"
	orgSyncLocks.Delete(orgID)
	updates := make(chan orgSyncStatusUpdate, 4)
	stubOrgSyncStatusWriter(t, func(_ *gin.Context, gotOrgID string, update orgSyncStatusUpdate) {
		if gotOrgID != orgID {
			t.Errorf("org id = %q, want %q", gotOrgID, orgID)
		}
		updates <- update
	})
	stubOrgSyncRunner(t, func(*gin.Context, string, string, time.Time) orgSyncResponseData {
		return successfulOrgSyncResult()
	})

	c, recorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync/start", `{}`)
	StartOrgSyncData(c)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", recorder.Code, recorder.Body.String())
	}

	deadline := time.After(2 * time.Second)
	seenRunning := false
	for {
		select {
		case update := <-updates:
			if update.SyncType != "organization" {
				continue
			}
			if update.Status == "running" {
				seenRunning = true
				continue
			}
			if update.Status != "success" || update.Details["result"] == nil {
				t.Fatalf("final update = %#v", update)
			}
			if !seenRunning {
				t.Fatal("final status arrived before running status")
			}
			return
		case <-deadline:
			t.Fatal("background organization sync did not complete")
		}
	}
}

func TestOrgSyncBackgroundPersistsAndQueriesRunningThenComplete(t *testing.T) {
	db := useOrgSyncTestDB(t)
	orgID := "async-persist-org"
	requestID := "org-sync-test-request"
	orgSyncLocks.Delete(orgID)
	release := make(chan struct{})
	started := make(chan struct{})
	stubOrgSyncRunner(t, func(c *gin.Context, gotOrgID, gotRequestID string, _ time.Time) orgSyncResponseData {
		if gotOrgID != orgID || gotRequestID != requestID {
			t.Errorf("runner scope = %q/%q, want %q/%q", gotOrgID, gotRequestID, orgID, requestID)
		}
		if deadline, ok := c.Request.Context().Deadline(); !ok || time.Until(deadline) > orgSyncMaxExecutionTimeout {
			t.Errorf("background context deadline = %v, want at most %s", deadline, orgSyncMaxExecutionTimeout)
		}
		close(started)
		<-release
		return successfulOrgSyncResult()
	})

	startContext, startRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync/start", `{}`)
	StartOrgSyncData(startContext)
	if startRecorder.Code != http.StatusAccepted || !strings.Contains(startRecorder.Body.String(), `"status":"running"`) || !strings.Contains(startRecorder.Body.String(), requestID) {
		t.Fatalf("unexpected start response: status=%d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	<-started

	runningContext, runningRecorder := newOrgSyncQueryContext(t, orgID, requestID)
	GetOrgSyncResult(runningContext)
	if runningRecorder.Code != http.StatusAccepted || !strings.Contains(runningRecorder.Body.String(), `"status":"running"`) {
		t.Fatalf("unexpected running response: status=%d body=%s", runningRecorder.Code, runningRecorder.Body.String())
	}

	close(release)
	status := waitForOrgSyncStatus(t, db, orgID, requestID, "success")
	if status.Details == nil || status.Details["result"] == nil {
		t.Fatalf("final result was not persisted: %#v", status)
	}
	completedContext, completedRecorder := newOrgSyncQueryContext(t, orgID, requestID)
	GetOrgSyncResult(completedContext)
	if completedRecorder.Code != http.StatusOK || !strings.Contains(completedRecorder.Body.String(), `"overall_status":"success"`) || !strings.Contains(completedRecorder.Body.String(), `"success_count":525`) {
		t.Fatalf("unexpected completed response: status=%d body=%s", completedRecorder.Code, completedRecorder.Body.String())
	}
}

func TestGetOrgSyncResultStatusSemanticsAndTenantIsolation(t *testing.T) {
	db := useOrgSyncTestDB(t)
	tests := []struct {
		name       string
		orgID      string
		requestID  string
		status     string
		httpStatus int
		result     orgSyncResponseData
	}{
		{name: "success", orgID: "query-success", requestID: "req-success", status: "success", httpStatus: http.StatusOK, result: orgSyncResponseData{OverallStatus: "success", RequestID: "req-success", Departments: orgSyncStageResult{Status: "success"}, Employees: orgSyncEmployeeStageResult{Status: "success"}}},
		{name: "partial", orgID: "query-partial", requestID: "req-partial", status: "partial_failed", httpStatus: http.StatusMultiStatus, result: orgSyncResponseData{OverallStatus: "partial_failed", RequestID: "req-partial", Departments: orgSyncStageResult{Status: "success"}, Employees: orgSyncEmployeeStageResult{Status: "partial_failed", Error: employeeSyncFailedMessage}}},
		{name: "failed", orgID: "query-failed", requestID: "req-failed", status: "failed", httpStatus: http.StatusInternalServerError, result: orgSyncResponseData{OverallStatus: "failed", RequestID: "req-failed", Departments: orgSyncStageResult{Status: "failed", Error: departmentSyncFailedMessage}, Employees: orgSyncEmployeeStageResult{Status: "skipped", Error: employeeSyncSkippedMessage}}},
	}
	for _, test := range tests {
		if err := db.Create(&database.SyncStatus{
			OrgID: test.orgID, Type: "organization", LastSyncTime: time.Now(), Status: test.status,
			RequestID: test.requestID, Details: map[string]interface{}{"result": test.result},
		}).Error; err != nil {
			t.Fatalf("seed %s status: %v", test.name, err)
		}
		c, recorder := newOrgSyncQueryContext(t, test.orgID, test.requestID)
		GetOrgSyncResult(c)
		if recorder.Code != test.httpStatus || !strings.Contains(recorder.Body.String(), `"request_id":"`+test.requestID+`"`) {
			t.Fatalf("%s query: status=%d body=%s", test.name, recorder.Code, recorder.Body.String())
		}
	}

	mismatchContext, mismatchRecorder := newOrgSyncQueryContext(t, "query-success", "another-request")
	GetOrgSyncResult(mismatchContext)
	if mismatchRecorder.Code != http.StatusNotFound {
		t.Fatalf("request mismatch status=%d body=%s", mismatchRecorder.Code, mismatchRecorder.Body.String())
	}
	crossOrgContext, crossOrgRecorder := newOrgSyncQueryContext(t, "another-org", "req-success")
	GetOrgSyncResult(crossOrgContext)
	if crossOrgRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-org query status=%d body=%s", crossOrgRecorder.Code, crossOrgRecorder.Body.String())
	}
}

func TestGetOrgSyncResultDatabaseFailureReturnsSafeServerError(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = originalDB })
	c, recorder := newOrgSyncQueryContext(t, "query-database-error", "safe-request")
	GetOrgSyncResult(c)
	if recorder.Code != http.StatusInternalServerError || strings.Contains(recorder.Body.String(), "missing organization context") {
		t.Fatalf("unsafe database failure response: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestStartOrgSyncDataRejectsSameOrgAndAllowsDifferentOrg(t *testing.T) {
	db := useOrgSyncTestDB(t)
	orgA, orgB := "async-lock-a", "async-lock-b"
	orgSyncLocks.Delete(orgA)
	orgSyncLocks.Delete(orgB)
	release := make(chan struct{})
	started := make(chan string, 2)
	stubOrgSyncRunner(t, func(_ *gin.Context, orgID, _ string, _ time.Time) orgSyncResponseData {
		started <- orgID
		<-release
		return successfulOrgSyncResult()
	})

	firstContext, firstRecorder := newOrgSyncContext(t, orgA, "/api/v1/org/sync/start", `{}`)
	StartOrgSyncData(firstContext)
	if firstRecorder.Code != http.StatusAccepted {
		t.Fatalf("first start status=%d body=%s", firstRecorder.Code, firstRecorder.Body.String())
	}
	<-started
	duplicateContext, duplicateRecorder := newOrgSyncContext(t, orgA, "/api/v1/org/sync/start", `{}`)
	StartOrgSyncData(duplicateContext)
	if duplicateRecorder.Code != http.StatusConflict {
		t.Fatalf("duplicate start status=%d body=%s", duplicateRecorder.Code, duplicateRecorder.Body.String())
	}
	differentContext, differentRecorder := newOrgSyncContext(t, orgB, "/api/v1/org/sync/start", `{}`)
	StartOrgSyncData(differentContext)
	if differentRecorder.Code != http.StatusAccepted {
		t.Fatalf("different-org start status=%d body=%s", differentRecorder.Code, differentRecorder.Body.String())
	}
	<-started
	close(release)
	waitForOrgSyncStatus(t, db, orgA, "org-sync-test-request", "success")
	waitForOrgSyncStatus(t, db, orgB, "org-sync-test-request", "success")
}

func TestStartOrgSyncDataRecoversPanicWritesSafeFailureAndReleasesLock(t *testing.T) {
	db := useOrgSyncTestDB(t)
	orgID := "async-panic-org"
	requestID := "org-sync-test-request"
	orgSyncLocks.Delete(orgID)
	var logs bytes.Buffer
	originalLogWriter := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() { log.SetOutput(originalLogWriter) })
	stubOrgSyncRunner(t, func(*gin.Context, string, string, time.Time) orgSyncResponseData {
		panic("Token=must-not-leak SQL SELECT * FROM secrets")
	})

	startContext, startRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync/start", `{}`)
	StartOrgSyncData(startContext)
	if startRecorder.Code != http.StatusAccepted {
		t.Fatalf("panic task start status=%d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	status := waitForOrgSyncStatus(t, db, orgID, requestID, "failed")
	visible := status.Message + status.ErrorCode + logs.String()
	if status.ErrorCode != orgSyncPanicErrorCode || status.Details["result"] == nil {
		t.Fatalf("unsafe or incomplete panic status: %#v", status)
	}
	for _, secret := range []string{"must-not-leak", "SELECT * FROM secrets"} {
		if strings.Contains(visible, secret) {
			t.Fatalf("panic path leaked %q: %s", secret, visible)
		}
	}
	deadline := time.Now().Add(time.Second)
	for {
		if unlock, ok := tryAcquireOrgSync(orgID); ok {
			unlock()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("organization lock was not released after panic")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestStartOrgSyncDataTimeoutIsDetachedAndPersisted(t *testing.T) {
	db := useOrgSyncTestDB(t)
	orgID := "async-timeout-org"
	requestID := "org-sync-test-request"
	orgSyncLocks.Delete(orgID)
	originalTimeout := orgSyncExecutionTimeout
	orgSyncExecutionTimeout = 30 * time.Millisecond
	t.Cleanup(func() { orgSyncExecutionTimeout = originalTimeout })
	observedContext := make(chan error, 1)
	stubOrgSyncRunner(t, func(c *gin.Context, _ string, _ string, _ time.Time) orgSyncResponseData {
		<-c.Request.Context().Done()
		observedContext <- c.Request.Context().Err()
		return orgSyncResponseData{}
	})

	clientContext, cancelClient := context.WithCancel(context.Background())
	startContext, startRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync/start", `{}`)
	startContext.Request = startContext.Request.WithContext(requestmeta.WithTenant(clientContext, orgID))
	StartOrgSyncData(startContext)
	cancelClient()
	if startRecorder.Code != http.StatusAccepted {
		t.Fatalf("timeout task start status=%d body=%s", startRecorder.Code, startRecorder.Body.String())
	}
	if err := <-observedContext; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("background context ended with %v, want deadline exceeded", err)
	}
	status := waitForOrgSyncStatus(t, db, orgID, requestID, "failed")
	if status.ErrorCode != orgSyncTimeoutErrorCode || strings.Contains(status.Message, "context deadline exceeded") {
		t.Fatalf("timeout status is not safe: %#v", status)
	}
	queryContext, queryRecorder := newOrgSyncQueryContext(t, orgID, requestID)
	GetOrgSyncResult(queryContext)
	if queryRecorder.Code != http.StatusInternalServerError || !strings.Contains(queryRecorder.Body.String(), orgSyncTimeoutErrorCode) {
		t.Fatalf("timeout query status=%d body=%s", queryRecorder.Code, queryRecorder.Body.String())
	}
}

func TestStartOrgSyncDataDoesNotLaunchWhenRunningStatusCannotPersist(t *testing.T) {
	orgID := "async-persist-failure-org"
	orgSyncLocks.Delete(orgID)
	originalWriter := writeOrgSyncStatusForRequest
	originalRunner := runOrgSyncForRequest
	runnerCalled := false
	writeOrgSyncStatusForRequest = func(*gin.Context, string, orgSyncStatusUpdate) error {
		return errors.New("dsn=must-not-leak")
	}
	runOrgSyncForRequest = func(*gin.Context, string, string, time.Time) orgSyncResponseData {
		runnerCalled = true
		return successfulOrgSyncResult()
	}
	t.Cleanup(func() {
		writeOrgSyncStatusForRequest = originalWriter
		runOrgSyncForRequest = originalRunner
	})

	c, recorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync/start", `{}`)
	StartOrgSyncData(c)
	if recorder.Code != http.StatusInternalServerError || runnerCalled || strings.Contains(recorder.Body.String(), "must-not-leak") {
		t.Fatalf("unexpected persistence failure response: status=%d called=%v body=%s", recorder.Code, runnerCalled, recorder.Body.String())
	}
	if unlock, ok := tryAcquireOrgSync(orgID); !ok {
		t.Fatal("organization lock remained held after running status persistence failed")
	} else {
		unlock()
	}
}

func TestLegacyOrgSyncEndpoints_DoNotExposeRawErrors(t *testing.T) {
	tests := []struct {
		name       string
		handler    gin.HandlerFunc
		department func(string) ([]dingtalk.DeptInfo, error)
		users      func(string) ([]dingtalk.UserInfo, error)
	}{
		{
			name:    "users",
			handler: SyncUsers,
			users: func(string) ([]dingtalk.UserInfo, error) {
				return nil, errors.New("https://example.test?access_token=token-value password=hunter2 SQL SELECT * FROM users")
			},
		},
		{
			name:    "departments",
			handler: SyncDepartments,
			department: func(string) ([]dingtalk.DeptInfo, error) {
				return nil, errors.New("https://example.test?access_token=token-value password=hunter2 SQL SELECT * FROM departments")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var statusMessages []string
			stubOrgSyncStatusWriter(t, func(_ *gin.Context, _ string, update orgSyncStatusUpdate) {
				statusMessages = append(statusMessages, update.Message)
			})
			stubOrgSyncSources(t, test.department, test.users, nil, nil)
			var logs bytes.Buffer
			originalWriter := log.Writer()
			log.SetOutput(&logs)
			t.Cleanup(func() { log.SetOutput(originalWriter) })

			c, recorder := newOrgSyncContext(t, "safe-error-org", "/api/v1/sync/"+test.name, `{}`)
			test.handler(c)
			visible := recorder.Body.String() + strings.Join(statusMessages, " ") + logs.String()
			for _, secret := range []string{"token-value", "hunter2", "SELECT * FROM"} {
				if strings.Contains(visible, secret) {
					t.Fatalf("legacy endpoint leaked %q: %s", secret, visible)
				}
			}
			if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "request_id") {
				t.Fatalf("unexpected safe response: status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestSyncDepartmentsReturnsSafePersistFailureCode(t *testing.T) {
	stubOrgSyncStatusWriter(t, func(*gin.Context, string, orgSyncStatusUpdate) {})
	stubOrgSyncSources(t,
		func(string) ([]dingtalk.DeptInfo, error) {
			return []dingtalk.DeptInfo{{DeptID: 1, Name: "根部门", ParentID: 0}}, nil
		},
		nil,
		nil,
		func(*gin.Context, string, []dingtalk.DeptInfo) (service.OrgDepartmentSyncResult, error) {
			return service.OrgDepartmentSyncResult{}, errors.New("duplicate access_token=must-not-leak")
		},
	)
	c, recorder := newOrgSyncContext(t, "persist-code-org", "/api/v1/sync/departments", `{}`)
	SyncDepartments(c)
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), departmentPersistErrorCode) ||
		!strings.Contains(recorder.Body.String(), departmentPersistFailedMessage) {
		t.Fatalf("missing safe persistence diagnostics: %s", recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "must-not-leak") {
		t.Fatalf("response leaked persistence detail: %s", recorder.Body.String())
	}
}

func TestLegacyOrgSyncEndpoints_MissingAndCrossOrgContexts(t *testing.T) {
	handlers := []struct {
		name    string
		handler gin.HandlerFunc
	}{
		{name: "users", handler: SyncUsers},
		{name: "departments", handler: SyncDepartments},
	}
	for _, endpoint := range handlers {
		t.Run(endpoint.name+" missing org", func(t *testing.T) {
			c, recorder := newOrgSyncContext(t, "", "/api/v1/sync/"+endpoint.name, `{}`)
			endpoint.handler(c)
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d want=401 body=%s", recorder.Code, recorder.Body.String())
			}
		})
		for _, selector := range []struct {
			name   string
			target string
			body   string
			header string
		}{
			{name: "query", target: "/api/v1/sync/" + endpoint.name + "?org_id=other-org", body: `{}`},
			{name: "body", target: "/api/v1/sync/" + endpoint.name, body: `{"org_id":"other-org"}`},
			{name: "header", target: "/api/v1/sync/" + endpoint.name, body: `{}`, header: "other-org"},
		} {
			t.Run(endpoint.name+" rejects "+selector.name, func(t *testing.T) {
				c, recorder := newOrgSyncContext(t, "current-org", selector.target, selector.body)
				if selector.header != "" {
					c.Request.Header.Set("X-Org-ID", selector.header)
				}
				endpoint.handler(c)
				if recorder.Code != http.StatusForbidden {
					t.Fatalf("status=%d want=403 body=%s", recorder.Code, recorder.Body.String())
				}
			})
		}
	}
}

func TestOrganizationSyncEntrypointsShareSameOrgLock(t *testing.T) {
	stubOrgSyncStatusWriter(t, func(*gin.Context, string, orgSyncStatusUpdate) {})

	t.Run("full sync blocks user sync", func(t *testing.T) {
		orgID := "full-blocks-user"
		orgSyncLocks.Delete(orgID)
		started := make(chan struct{})
		release := make(chan struct{})
		stubOrgSyncRunner(t, func(*gin.Context, string, string, time.Time) orgSyncResponseData {
			close(started)
			<-release
			return successfulOrgSyncResult()
		})
		fullCtx, _ := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
		done := make(chan struct{})
		go func() { SyncOrgData(fullCtx); close(done) }()
		<-started
		userCtx, userRecorder := newOrgSyncContext(t, orgID, "/api/v1/sync/users", `{}`)
		SyncUsers(userCtx)
		if userRecorder.Code != http.StatusConflict {
			t.Fatalf("user status=%d want=409 body=%s", userRecorder.Code, userRecorder.Body.String())
		}
		close(release)
		<-done
	})

	t.Run("department sync blocks full sync", func(t *testing.T) {
		orgID := "department-blocks-full"
		orgSyncLocks.Delete(orgID)
		started := make(chan struct{})
		release := make(chan struct{})
		stubOrgSyncSources(t,
			func(string) ([]dingtalk.DeptInfo, error) {
				close(started)
				<-release
				return nil, nil
			}, nil, nil,
			func(*gin.Context, string, []dingtalk.DeptInfo) (service.OrgDepartmentSyncResult, error) {
				return service.OrgDepartmentSyncResult{}, nil
			},
		)
		deptCtx, _ := newOrgSyncContext(t, orgID, "/api/v1/sync/departments", `{}`)
		done := make(chan struct{})
		go func() { SyncDepartments(deptCtx); close(done) }()
		<-started
		fullCtx, fullRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
		SyncOrgData(fullCtx)
		if fullRecorder.Code != http.StatusConflict {
			t.Fatalf("full status=%d want=409 body=%s", fullRecorder.Code, fullRecorder.Body.String())
		}
		close(release)
		<-done
	})

	t.Run("user sync blocks department sync", func(t *testing.T) {
		orgID := "user-blocks-department"
		orgSyncLocks.Delete(orgID)
		started := make(chan struct{})
		release := make(chan struct{})
		stubOrgSyncSources(t, nil,
			func(string) ([]dingtalk.UserInfo, error) {
				close(started)
				<-release
				return nil, nil
			}, nil, nil,
		)
		userCtx, _ := newOrgSyncContext(t, orgID, "/api/v1/sync/users", `{}`)
		done := make(chan struct{})
		go func() { SyncUsers(userCtx); close(done) }()
		<-started
		deptCtx, deptRecorder := newOrgSyncContext(t, orgID, "/api/v1/sync/departments", `{}`)
		SyncDepartments(deptCtx)
		if deptRecorder.Code != http.StatusConflict {
			t.Fatalf("department status=%d want=409 body=%s", deptRecorder.Code, deptRecorder.Body.String())
		}
		close(release)
		<-done
	})
}

func TestOrganizationSyncEntrypointsDifferentOrganizationsDoNotBlock(t *testing.T) {
	stubOrgSyncStatusWriter(t, func(*gin.Context, string, orgSyncStatusUpdate) {})
	stubOrgSyncSources(t, nil, func(string) ([]dingtalk.UserInfo, error) { return nil, nil }, nil, nil)
	orgA := "cross-entry-org-a"
	orgB := "cross-entry-org-b"
	orgSyncLocks.Delete(orgA)
	orgSyncLocks.Delete(orgB)
	started := make(chan struct{})
	release := make(chan struct{})
	stubOrgSyncRunner(t, func(_ *gin.Context, orgID, _ string, _ time.Time) orgSyncResponseData {
		if orgID == orgA {
			close(started)
			<-release
		}
		return successfulOrgSyncResult()
	})

	fullCtx, _ := newOrgSyncContext(t, orgA, "/api/v1/org/sync", `{}`)
	done := make(chan struct{})
	go func() { SyncOrgData(fullCtx); close(done) }()
	<-started

	userCtx, userRecorder := newOrgSyncContext(t, orgB, "/api/v1/sync/users", `{}`)
	SyncUsers(userCtx)
	if userRecorder.Code != http.StatusOK {
		t.Fatalf("different organization was blocked: status=%d body=%s", userRecorder.Code, userRecorder.Body.String())
	}
	close(release)
	<-done
}

func TestOrganizationSyncLockReleasedAfterFailedResult(t *testing.T) {
	orgID := "failed-result-release-org"
	orgSyncLocks.Delete(orgID)
	stubOrgSyncRunner(t, func(*gin.Context, string, string, time.Time) orgSyncResponseData {
		return orgSyncResponseData{
			Departments: orgSyncStageResult{Status: "failed", FailCount: 1, Error: departmentSyncFailedMessage},
			Employees:   orgSyncEmployeeStageResult{Status: "failed", FailCount: 1, Error: employeeSyncSkippedMessage},
		}
	})
	firstCtx, firstRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
	SyncOrgData(firstCtx)
	if firstRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("first status=%d want=500", firstRecorder.Code)
	}
	secondCtx, secondRecorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
	SyncOrgData(secondCtx)
	if secondRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("lock remained held after failed result: status=%d body=%s", secondRecorder.Code, secondRecorder.Body.String())
	}
}

func TestOrganizationSyncLockReleasedAfterPanic(t *testing.T) {
	orgID := "panic-release-org"
	orgSyncLocks.Delete(orgID)
	originalRunner := runOrgSyncForRequest
	runOrgSyncForRequest = func(*gin.Context, string, string, time.Time) orgSyncResponseData {
		panic("test panic")
	}
	func() {
		defer func() { _ = recover() }()
		c, _ := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
		SyncOrgData(c)
	}()
	runOrgSyncForRequest = func(*gin.Context, string, string, time.Time) orgSyncResponseData {
		return successfulOrgSyncResult()
	}
	t.Cleanup(func() { runOrgSyncForRequest = originalRunner })
	c, recorder := newOrgSyncContext(t, orgID, "/api/v1/org/sync", `{}`)
	SyncOrgData(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("lock not released after panic: status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
