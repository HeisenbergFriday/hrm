package api

import (
	"bytes"
	"context"
	"encoding/json"
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

func captureApprovalSyncBackground(t *testing.T) *func() {
	t.Helper()
	original := launchApprovalSyncBackground
	var captured func()
	launchApprovalSyncBackground = func(task func()) { captured = task }
	t.Cleanup(func() { launchApprovalSyncBackground = original })
	return &captured
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

func TestStartApprovalSyncPersistsAcceptedStateBeforeLaunchingExternalWork(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	runStarted := make(chan struct{})
	releaseRun := make(chan struct{})
	runner := &approvalSyncRunnerStub{
		prepare: func(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error) {
			return service.ApprovalSyncPlan{ProcessCodes: []string{"PROC-A"}, StartDate: "2026-08-01", EndDate: "2026-08-05"}, nil
		},
		run: func(context.Context, service.ApprovalSyncPlan, string) service.ApprovalSyncResult {
			close(runStarted)
			<-releaseRun
			return approvalSyncTerminalResult(service.ApprovalSyncStatusSuccess, "approval-test-request")
		},
	}
	stubApprovalSyncFactory(t, runner)

	var scheduled func()
	var taskAtLaunch database.ApprovalSyncTask
	var statusAtLaunch database.SyncStatus
	var taskErr, statusErr error
	var responseCodeAtLaunch int
	var responseBodyAtLaunch string
	originalLauncher := launchApprovalSyncBackground
	t.Cleanup(func() { launchApprovalSyncBackground = originalLauncher })
	var recorder *httptest.ResponseRecorder
	launchApprovalSyncBackground = func(task func()) {
		responseCodeAtLaunch = recorder.Code
		responseBodyAtLaunch = recorder.Body.String()
		taskErr = db.Where("org_id = ? AND type = ? AND request_id = ?", "org-a", approvalSyncType, "approval-test-request").First(&taskAtLaunch).Error
		statusErr = db.Where("org_id = ? AND type = ?", "org-a", approvalSyncType).First(&statusAtLaunch).Error
		scheduled = task
	}
	c, responseRecorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)
	recorder = responseRecorder

	StartApprovalSync(c)

	if responseCodeAtLaunch != http.StatusAccepted || !strings.Contains(responseBodyAtLaunch, `"request_id":"approval-test-request"`) {
		t.Fatalf("response at background launch = %d %s", responseCodeAtLaunch, responseBodyAtLaunch)
	}
	if taskErr != nil || taskAtLaunch.Status != "running" || taskAtLaunch.RequestID != "approval-test-request" {
		t.Fatalf("task at background launch = %#v err=%v", taskAtLaunch, taskErr)
	}
	if statusErr != nil || statusAtLaunch.Status != "running" || statusAtLaunch.RequestID != "approval-test-request" {
		t.Fatalf("sync status at background launch = %#v err=%v", statusAtLaunch, statusErr)
	}
	if scheduled == nil {
		t.Fatal("background task was not scheduled")
	}
	select {
	case <-runStarted:
		t.Fatal("external work ran before the accepted response and running state were persisted")
	default:
	}

	go scheduled()
	select {
	case <-runStarted:
	case <-time.After(time.Second):
		t.Fatal("scheduled external work did not start")
	}
	close(releaseRun)
	waitForApprovalSyncTask(t, db, "org-a", "approval-test-request", service.ApprovalSyncStatusSuccess)
}

func TestApprovalSyncBackgroundIgnoresClientCancellation(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	observedContext := make(chan error, 1)
	runner := &approvalSyncRunnerStub{
		prepare: func(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error) {
			return service.ApprovalSyncPlan{ProcessCodes: []string{"PROC-A"}}, nil
		},
		run: func(ctx context.Context, _ service.ApprovalSyncPlan, requestID string) service.ApprovalSyncResult {
			observedContext <- ctx.Err()
			return approvalSyncTerminalResult(service.ApprovalSyncStatusSuccess, requestID)
		},
	}
	stubApprovalSyncFactory(t, runner)
	scheduled := captureApprovalSyncBackground(t)
	clientContext, cancelClient := context.WithCancel(context.Background())
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)
	c.Request = c.Request.WithContext(requestmeta.WithTenant(clientContext, "org-a"))

	StartApprovalSync(c)
	cancelClient()
	if recorder.Code != http.StatusAccepted || *scheduled == nil {
		t.Fatalf("start response = %d body=%s scheduled=%v", recorder.Code, recorder.Body.String(), *scheduled != nil)
	}
	(*scheduled)()
	if err := <-observedContext; err != nil {
		t.Fatalf("background context inherited client cancellation: %v", err)
	}
	waitForApprovalSyncTask(t, db, "org-a", "approval-test-request", service.ApprovalSyncStatusSuccess)
}

func TestApprovalSyncBackgroundFailurePersistsSafeTerminalState(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	runner := &approvalSyncRunnerStub{
		prepare: func(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error) {
			return service.ApprovalSyncPlan{ProcessCodes: []string{"PROC-A"}}, nil
		},
		run: func(_ context.Context, _ service.ApprovalSyncPlan, _ string) service.ApprovalSyncResult {
			return service.ApprovalSyncResult{
				Status:       service.ApprovalSyncStatusFailed,
				ProcessCount: 1, FailedProcesses: 1,
				Processes: []service.ApprovalSyncProcessResult{{
					ProcessCode: "PROC-A", Status: service.ApprovalSyncStatusFailed,
					ErrorCode: service.ApprovalSyncErrorInternal,
					Error:     "dingtalk raw error access_token=secret-token&url=https://private.example/path?secret=hidden",
				}},
			}
		},
	}
	stubApprovalSyncFactory(t, runner)
	scheduled := captureApprovalSyncBackground(t)
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)

	StartApprovalSync(c)
	if recorder.Code != http.StatusAccepted || *scheduled == nil {
		t.Fatalf("start response = %d body=%s", recorder.Code, recorder.Body.String())
	}
	(*scheduled)()
	task := waitForApprovalSyncTask(t, db, "org-a", "approval-test-request", service.ApprovalSyncStatusFailed)
	var syncStatus database.SyncStatus
	if err := db.Where("org_id = ? AND type = ?", "org-a", approvalSyncType).First(&syncStatus).Error; err != nil {
		t.Fatalf("query terminal sync status: %v", err)
	}
	if syncStatus.Status != service.ApprovalSyncStatusFailed {
		t.Fatalf("sync status = %#v", syncStatus)
	}
	persisted, err := json.Marshal(map[string]interface{}{"task": task.Details, "status": syncStatus.Details})
	if err != nil {
		t.Fatalf("marshal persisted details: %v", err)
	}
	for _, sensitive := range []string{"secret-token", "private.example", "secret=hidden", "dingtalk raw error"} {
		if bytes.Contains(persisted, []byte(sensitive)) {
			t.Fatalf("persisted terminal state contains %q: %s", sensitive, persisted)
		}
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
	var taskCount int64
	if err := db.Model(&database.ApprovalSyncTask{}).Count(&taskCount).Error; err != nil {
		t.Fatalf("count approval sync tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("missing configuration created %d approval sync tasks", taskCount)
	}
}

func TestApprovalSyncRejectsUnconfiguredExplicitProcessBeforeCreatingTask(t *testing.T) {
	db := useOrgSyncTestDB(t)
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatalf("migrate approval sync tasks: %v", err)
	}
	runner := &approvalSyncRunnerStub{
		prepare: func(input service.ApprovalSyncInput, _ time.Time) (service.ApprovalSyncPlan, error) {
			if input.ProcessCode != "PROC-NOT-CONFIGURED" {
				t.Fatalf("process code = %q", input.ProcessCode)
			}
			return service.ApprovalSyncPlan{}, service.ErrApprovalProcessNotAccessible
		},
		run: func(context.Context, service.ApprovalSyncPlan, string) service.ApprovalSyncResult {
			t.Fatal("Run must not be called")
			return service.ApprovalSyncResult{}
		},
	}
	stubApprovalSyncFactory(t, runner)
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{"process_code":"PROC-NOT-CONFIGURED"}`)

	StartApprovalSync(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), service.ApprovalSyncErrorNotAccessible) {
		t.Fatalf("not accessible error code not returned: %s", recorder.Body.String())
	}
	var taskCount int64
	if err := db.Model(&database.ApprovalSyncTask{}).Count(&taskCount).Error; err != nil {
		t.Fatalf("count approval sync tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("unconfigured explicit process created %d approval sync tasks", taskCount)
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
		run: func(_ context.Context, _ service.ApprovalSyncPlan, requestID string) service.ApprovalSyncResult {
			return approvalSyncTerminalResult(service.ApprovalSyncStatusSuccess, requestID)
		},
	}
	stubApprovalSyncFactory(t, runner)
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)
	StartApprovalSync(c)
	if recorder.Code != http.StatusConflict || !strings.Contains(recorder.Body.String(), "real-running-request") {
		t.Fatalf("conflict response = %d %s", recorder.Code, recorder.Body.String())
	}
	scheduled := captureApprovalSyncBackground(t)
	c, recorder = newApprovalSyncHandlerContext(t, "org-b", http.MethodPost, "/api/v1/approvals/sync/start", `{}`)
	StartApprovalSync(c)
	if recorder.Code != http.StatusAccepted || *scheduled == nil {
		t.Fatalf("different organization start = %d body=%s", recorder.Code, recorder.Body.String())
	}
	var orgBTask database.ApprovalSyncTask
	if err := db.Where("org_id = ? AND type = ? AND request_id = ?", "org-b", approvalSyncType, "approval-test-request").First(&orgBTask).Error; err != nil || orgBTask.Status != "running" {
		t.Fatalf("different organization task = %#v err=%v", orgBTask, err)
	}
	(*scheduled)()
	waitForApprovalSyncTask(t, db, "org-b", "approval-test-request", service.ApprovalSyncStatusSuccess)
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
	update := approvalSyncStatusUpdate(service.ApprovalSyncResult{FailCount: 2, FetchFailCount: 3, ReconcileFailCount: 4, FailedProcesses: 5})
	if update.FailCount != 9 {
		t.Fatalf("fail_count = %d, want 9 failed approval instance stages", update.FailCount)
	}
	if update.Details["failed_processes"] != 5 {
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
