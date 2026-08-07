package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
	"peopleops/internal/requestmeta"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type approvalSyncRunnerStub struct {
	prepare func(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error)
	run     func(context.Context, service.ApprovalSyncPlan, string) service.ApprovalSyncResult
}

func (s *approvalSyncRunnerStub) Prepare(input service.ApprovalSyncInput, now time.Time) (service.ApprovalSyncPlan, error) {
	return s.prepare(input, now)
}

func (s *approvalSyncRunnerStub) Run(ctx context.Context, plan service.ApprovalSyncPlan, requestID string) service.ApprovalSyncResult {
	return s.run(ctx, plan, requestID)
}

func newApprovalSyncHandlerContext(t *testing.T, orgID, method, target, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(method, target, bytes.NewBufferString(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("orgID", orgID)
	c.Set("userID", "tester")
	c.Set("requestID", "approval-test-request")
	c.Request = c.Request.WithContext(requestmeta.WithTenant(c.Request.Context(), orgID))
	if method == http.MethodGet {
		c.Params = gin.Params{{Key: "request_id", Value: strings.TrimPrefix(target, "/api/v1/approvals/sync/")}}
	}
	return c, recorder
}

func approvalSyncTerminalResult(status string, requestID string) service.ApprovalSyncResult {
	return service.ApprovalSyncResult{
		Status: status, ProcessCount: 1, SucceededProcesses: 1, SuccessCount: 2,
		StartDate: "2026-08-01", EndDate: "2026-08-05", SyncTime: time.Now().Format(time.RFC3339),
		DurationMS: 10, RequestID: requestID,
		Processes: []service.ApprovalSyncProcessResult{{ProcessCode: "PROC-1", Status: status, FetchedCount: 2, SuccessCount: 2}},
	}
}

func stubApprovalSyncFactory(t *testing.T, runner approvalSyncRunner) {
	t.Helper()
	original := newApprovalSyncServiceForRequest
	newApprovalSyncServiceForRequest = func(*gorm.DB, string) approvalSyncRunner { return runner }
	t.Cleanup(func() { newApprovalSyncServiceForRequest = original })
}

func TestStartApprovalSyncUsesBlankProcessCodeForFullPlanAndPersistsTerminalState(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	var captured service.ApprovalSyncInput
	runner := &approvalSyncRunnerStub{
		prepare: func(input service.ApprovalSyncInput, _ time.Time) (service.ApprovalSyncPlan, error) {
			captured = input
			return service.ApprovalSyncPlan{ProcessCodes: []string{"PROC-A", "PROC-B"}, StartDate: "2026-07-05", EndDate: "2026-08-05"}, nil
		},
		run: func(_ context.Context, plan service.ApprovalSyncPlan, requestID string) service.ApprovalSyncResult {
			result := approvalSyncTerminalResult(service.ApprovalSyncStatusSuccess, requestID)
			result.Processes = make([]service.ApprovalSyncProcessResult, 0, len(plan.ProcessCodes))
			result.ProcessCount = len(plan.ProcessCodes)
			for _, code := range plan.ProcessCodes {
				result.Processes = append(result.Processes, service.ApprovalSyncProcessResult{ProcessCode: code, Status: service.ApprovalSyncStatusSuccess, SuccessCount: 1})
			}
			return result
		},
	}
	stubApprovalSyncFactory(t, runner)
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)

	StartApprovalSync(c)

	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if captured.ProcessCode != "" {
		t.Fatalf("process code = %q, want blank full-sync selector", captured.ProcessCode)
	}
	requestID := "approval-test-request"
	status := waitForApprovalSyncTask(t, db, "org-a", requestID, service.ApprovalSyncStatusSuccess)
	if status.Type != approvalSyncType || status.OrgID != "org-a" || status.Details["result"] == nil {
		t.Fatalf("persisted status = %#v", status)
	}
}

func TestApprovalSyncRejectsClientOrganizationAndMissingConfiguration(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	runner := &approvalSyncRunnerStub{
		prepare: func(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error) {
			return service.ApprovalSyncPlan{}, service.ErrApprovalProcessCodesMissing
		},
		run: func(context.Context, service.ApprovalSyncPlan, string) service.ApprovalSyncResult {
			t.Fatal("Run must not be called")
			return service.ApprovalSyncResult{}
		},
	}
	stubApprovalSyncFactory(t, runner)
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{"org_id":"org-b"}`)
	StartApprovalSync(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("cross-org status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	c, recorder = newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)
	StartApprovalSync(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("missing config status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), service.ApprovalSyncErrorConfigMissing) {
		t.Fatalf("missing config error code not returned: %s", recorder.Body.String())
	}
}

func TestApprovalSyncConflictIsPerOrganization(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	now := time.Now()
	repoA := repository.NewApprovalSyncTaskRepositoryWithOrgID(db, "org-a")
	if conflict, err := repoA.Acquire(&database.ApprovalSyncTask{Type: approvalSyncType, RequestID: "real-running-request"}, now.Add(-approvalSyncStaleAfter), now); err != nil || conflict != nil {
		t.Fatalf("acquire org-a: conflict=%#v err=%v", conflict, err)
	}
	runner := &approvalSyncRunnerStub{
		prepare: func(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error) {
			return service.ApprovalSyncPlan{ProcessCodes: []string{"PROC-A"}}, nil
		},
		run: func(context.Context, service.ApprovalSyncPlan, string) service.ApprovalSyncResult {
			t.Fatal("conflicting task must not run")
			return service.ApprovalSyncResult{}
		},
	}
	stubApprovalSyncFactory(t, runner)
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)
	StartApprovalSync(c)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "real-running-request") {
		t.Fatalf("conflict response = %d %s", recorder.Code, recorder.Body.String())
	}
	repoB := repository.NewApprovalSyncTaskRepositoryWithOrgID(db, "org-b")
	if conflict, err := repoB.Acquire(&database.ApprovalSyncTask{Type: approvalSyncType, RequestID: "org-b-request"}, now.Add(-approvalSyncStaleAfter), now); err != nil || conflict != nil {
		t.Fatalf("different organization must acquire: conflict=%#v err=%v", conflict, err)
	}
}

func TestGetApprovalSyncResultChecksOrgAndTaskType(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	status := database.ApprovalSyncTask{
		OrgID: "org-a", Type: approvalSyncType, RequestID: "request-1", Status: service.ApprovalSyncStatusSuccess,
		StartedAt: time.Now(), HeartbeatAt: time.Now(), Details: map[string]interface{}{"result": map[string]interface{}{"status": "success"}},
	}
	if err := db.Create(&status).Error; err != nil {
		t.Fatalf("create status: %v", err)
	}
	c, recorder := newApprovalSyncHandlerContext(t, "org-b", http.MethodGet, "/api/v1/approvals/sync/request-1", "")
	GetApprovalSyncResult(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("cross-org status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	c, recorder = newApprovalSyncHandlerContext(t, "org-a", http.MethodGet, "/api/v1/approvals/sync/request-1", "")
	GetApprovalSyncResult(c)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"success"`) {
		t.Fatalf("same-org status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	wrongType := database.ApprovalSyncTask{
		OrgID: "org-a", Type: "organization", RequestID: "request-wrong-type", Status: service.ApprovalSyncStatusSuccess,
		StartedAt: time.Now(), HeartbeatAt: time.Now(), Details: map[string]interface{}{"result": map[string]interface{}{"status": "success"}},
	}
	if err := db.Create(&wrongType).Error; err != nil {
		t.Fatalf("create wrong-type status: %v", err)
	}
	c, recorder = newApprovalSyncHandlerContext(t, "org-a", http.MethodGet, "/api/v1/approvals/sync/request-wrong-type", "")
	GetApprovalSyncResult(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("wrong-type status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestApprovalSyncHistoricalRequestRemainsQueryableAndStaleRunningFails(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	repo := repository.NewApprovalSyncTaskRepositoryWithOrgID(db, "org-a")
	now := time.Now()
	for _, requestID := range []string{"old-request", "new-request"} {
		if conflict, err := repo.Acquire(&database.ApprovalSyncTask{Type: approvalSyncType, RequestID: requestID}, now.Add(-approvalSyncStaleAfter), now); err != nil || conflict != nil {
			t.Fatalf("acquire %s: conflict=%#v err=%v", requestID, conflict, err)
		}
		finished := now.Add(time.Second)
		if err := repo.Complete(&database.ApprovalSyncTask{
			Type: approvalSyncType, RequestID: requestID, Status: service.ApprovalSyncStatusSuccess,
			HeartbeatAt: finished, FinishedAt: &finished,
			Details: map[string]interface{}{"result": map[string]interface{}{"status": "success", "request_id": requestID}},
		}); err != nil {
			t.Fatalf("complete %s: %v", requestID, err)
		}
	}
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodGet, "/api/v1/approvals/sync/old-request", "")
	GetApprovalSyncResult(c)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "old-request") {
		t.Fatalf("historical response = %d %s", recorder.Code, recorder.Body.String())
	}

	staleAt := now.Add(-approvalSyncStaleAfter - time.Minute)
	if conflict, err := repo.Acquire(&database.ApprovalSyncTask{Type: approvalSyncType, RequestID: "stale-request", StartedAt: staleAt}, staleAt.Add(-time.Minute), staleAt); err != nil || conflict != nil {
		t.Fatalf("acquire stale task: conflict=%#v err=%v", conflict, err)
	}
	c, recorder = newApprovalSyncHandlerContext(t, "org-a", http.MethodGet, "/api/v1/approvals/sync/stale-request", "")
	GetApprovalSyncResult(c)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), approvalSyncStaleCode) {
		t.Fatalf("stale response = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestApprovalSyncConsecutiveTasksOldRequestIDRemainsQueryable(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewApprovalSyncTaskRepositoryWithOrgID(db, "org-a")
	now := time.Now()

	// First task completes
	if conflict, err := repo.Acquire(&database.ApprovalSyncTask{Type: approvalSyncType, RequestID: "first-request"}, now.Add(-approvalSyncStaleAfter), now); err != nil || conflict != nil {
		t.Fatalf("acquire first: conflict=%#v err=%v", conflict, err)
	}
	finished1 := now.Add(time.Second)
	if err := repo.Complete(&database.ApprovalSyncTask{
		Type: approvalSyncType, RequestID: "first-request", Status: service.ApprovalSyncStatusSuccess,
		HeartbeatAt: finished1, FinishedAt: &finished1,
		Details: map[string]interface{}{"result": map[string]interface{}{"status": "success", "request_id": "first-request", "success_count": 5}},
	}); err != nil {
		t.Fatalf("complete first: %v", err)
	}

	// Second task completes
	if conflict, err := repo.Acquire(&database.ApprovalSyncTask{Type: approvalSyncType, RequestID: "second-request"}, now.Add(-approvalSyncStaleAfter), finished1.Add(time.Second)); err != nil || conflict != nil {
		t.Fatalf("acquire second: conflict=%#v err=%v", conflict, err)
	}
	finished2 := finished1.Add(2 * time.Second)
	if err := repo.Complete(&database.ApprovalSyncTask{
		Type: approvalSyncType, RequestID: "second-request", Status: service.ApprovalSyncStatusPartial,
		HeartbeatAt: finished2, FinishedAt: &finished2,
		Details: map[string]interface{}{"result": map[string]interface{}{"status": "partial", "request_id": "second-request", "success_count": 3}},
	}); err != nil {
		t.Fatalf("complete second: %v", err)
	}

	// Old request_id should still be queryable
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodGet, "/api/v1/approvals/sync/first-request", "")
	GetApprovalSyncResult(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("old request status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "first-request") {
		t.Fatalf("old request body missing request_id: %s", recorder.Body.String())
	}

	// New request_id should also be queryable
	c, recorder = newApprovalSyncHandlerContext(t, "org-a", http.MethodGet, "/api/v1/approvals/sync/second-request", "")
	GetApprovalSyncResult(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("new request status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "second-request") {
		t.Fatalf("new request body missing request_id: %s", recorder.Body.String())
	}
}

func TestApprovalSyncConflictReturnsRealRequestID(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	now := time.Now()
	repo := repository.NewApprovalSyncTaskRepositoryWithOrgID(db, "org-a")
	// Pre-acquire a running task with a known request_id
	if conflict, err := repo.Acquire(&database.ApprovalSyncTask{Type: approvalSyncType, RequestID: "real-running-123"}, now.Add(-approvalSyncStaleAfter), now); err != nil || conflict != nil {
		t.Fatalf("acquire: conflict=%#v err=%v", conflict, err)
	}

	runner := &approvalSyncRunnerStub{
		prepare: func(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error) {
			return service.ApprovalSyncPlan{ProcessCodes: []string{"PROC-A"}}, nil
		},
		run: func(context.Context, service.ApprovalSyncPlan, string) service.ApprovalSyncResult {
			t.Fatal("conflict must not run")
			return service.ApprovalSyncResult{}
		},
	}
	stubApprovalSyncFactory(t, runner)
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)
	StartApprovalSync(c)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "real-running-123") {
		t.Fatalf("409 response must contain real request_id 'real-running-123', got: %s", body)
	}
}

func TestApprovalSyncStatusFailCountUsesApprovalInstanceUnit(t *testing.T) {
	update := approvalSyncStatusUpdate(service.ApprovalSyncResult{FailCount: 2, FetchFailCount: 3, FailedProcesses: 4})
	if update.FailCount != 5 {
		t.Fatalf("fail_count = %d, want 5 failed approval instances", update.FailCount)
	}
	if update.Details["failed_processes"] != 4 {
		t.Fatalf("failed_processes details = %#v", update.Details)
	}
}

func TestApprovalSyncPreparationErrorDoesNotExposeThirdPartyDetails(t *testing.T) {
	code, message := service.ApprovalSyncPreparationError(errors.New("access_token=secret url=https://private"))
	if code != service.ApprovalSyncErrorInternal || strings.Contains(message, "secret") || strings.Contains(message, "https://") {
		t.Fatalf("unsafe preparation message code=%s message=%q", code, message)
	}
}

func waitForApprovalSyncTask(t *testing.T, db *gorm.DB, orgID, requestID, wanted string) database.ApprovalSyncTask {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		var status database.ApprovalSyncTask
		err := db.Where("org_id = ? AND type = ? AND request_id = ?", orgID, approvalSyncType, requestID).First(&status).Error
		if err == nil && status.Status == wanted {
			return status
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("approval sync status did not become %q", wanted)
	return database.ApprovalSyncTask{}
}
