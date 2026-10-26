package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
)

type fakeAutomaticApprovalRunner struct {
	plan         ApprovalSyncPlan
	prepareErr   error
	result       ApprovalSyncResult
	prepareCalls int
	runCalls     int
	panicOnRun   bool
}

func (f *fakeAutomaticApprovalRunner) Prepare(ApprovalSyncInput, time.Time) (ApprovalSyncPlan, error) {
	f.prepareCalls++
	return f.plan, f.prepareErr
}

func (f *fakeAutomaticApprovalRunner) Run(context.Context, ApprovalSyncPlan, string) ApprovalSyncResult {
	f.runCalls++
	if f.panicOnRun {
		panic("automatic approval sync test panic")
	}
	return f.result
}

type fakeAutomaticApprovalTaskStore struct {
	acquired    []*database.ApprovalSyncTask
	completed   []*database.ApprovalSyncTask
	conflict    *database.ApprovalSyncTask
	acquireErr  error
	completeErr error
}

func (f *fakeAutomaticApprovalTaskStore) Acquire(task *database.ApprovalSyncTask, _, _ time.Time) (*database.ApprovalSyncTask, error) {
	f.acquired = append(f.acquired, task)
	return f.conflict, f.acquireErr
}

func (f *fakeAutomaticApprovalTaskStore) Complete(task *database.ApprovalSyncTask) error {
	f.completed = append(f.completed, task)
	return f.completeErr
}

func newAutomaticApprovalSchedulerForTest(
	orgIDs []string,
	runners map[string]*fakeAutomaticApprovalRunner,
	stores map[string]*fakeAutomaticApprovalTaskStore,
) *ApprovalSyncJobScheduler {
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, ApprovalSyncLocation())
	return &ApprovalSyncJobScheduler{
		enabled:          true,
		lookback:         48 * time.Hour,
		executionTimeout: time.Minute,
		staleAfter:       approvalAutomaticSyncStaleAfter,
		now:              func() time.Time { return now },
		listOrgIDs:       func() ([]string, error) { return orgIDs, nil },
		newRunner: func(orgID string) approvalAutomaticSyncRunner {
			return runners[orgID]
		},
		newTaskStore: func(orgID string) approvalAutomaticSyncTaskStore {
			return stores[orgID]
		},
	}
}

func TestApprovalSyncJobSchedulerRunOnceUsesSharedSyncAndPersistsResult(t *testing.T) {
	runner := &fakeAutomaticApprovalRunner{
		plan: ApprovalSyncPlan{ProcessCodes: []string{"PROC-LEAVE"}, StartDate: "2026-10-24", EndDate: "2026-10-26"},
		result: ApprovalSyncResult{
			Status: ApprovalSyncStatusSuccess, ProcessCount: 1, SucceededProcesses: 1,
			FetchedCount: 2, SuccessCount: 2, ReconciledCount: 1,
		},
	}
	store := &fakeAutomaticApprovalTaskStore{}
	scheduler := newAutomaticApprovalSchedulerForTest(
		[]string{"org-a"},
		map[string]*fakeAutomaticApprovalRunner{"org-a": runner},
		map[string]*fakeAutomaticApprovalTaskStore{"org-a": store},
	)

	scheduler.RunOnce()

	if runner.prepareCalls != 1 || runner.runCalls != 1 {
		t.Fatalf("prepare=%d run=%d want 1/1", runner.prepareCalls, runner.runCalls)
	}
	if len(store.acquired) != 1 || store.acquired[0].Type != "approvals" || store.acquired[0].Status != "running" {
		t.Fatalf("unexpected acquired task: %#v", store.acquired)
	}
	if len(store.completed) != 1 || store.completed[0].Status != ApprovalSyncStatusSuccess || store.completed[0].SuccessCount != 2 {
		t.Fatalf("unexpected completed task: %#v", store.completed)
	}
}

func TestApprovalSyncJobSchedulerSkipsWhenManualSyncIsRunning(t *testing.T) {
	runner := &fakeAutomaticApprovalRunner{
		plan: ApprovalSyncPlan{ProcessCodes: []string{"PROC"}, StartDate: "2026-10-24", EndDate: "2026-10-26"},
	}
	store := &fakeAutomaticApprovalTaskStore{
		conflict:   &database.ApprovalSyncTask{RequestID: "manual-running"},
		acquireErr: repository.ErrApprovalSyncTaskRunning,
	}
	scheduler := newAutomaticApprovalSchedulerForTest(
		[]string{"org-a"},
		map[string]*fakeAutomaticApprovalRunner{"org-a": runner},
		map[string]*fakeAutomaticApprovalTaskStore{"org-a": store},
	)

	scheduler.RunOnce()

	if runner.runCalls != 0 {
		t.Fatalf("automatic run should skip while manual task is active, got %d", runner.runCalls)
	}
	if len(store.completed) != 0 {
		t.Fatalf("conflicting task must not be completed by automatic job")
	}
}

func TestApprovalSyncJobSchedulerSkipsOrganizationWithoutProcessCodes(t *testing.T) {
	runner := &fakeAutomaticApprovalRunner{prepareErr: ErrApprovalProcessCodesMissing}
	store := &fakeAutomaticApprovalTaskStore{}
	scheduler := newAutomaticApprovalSchedulerForTest(
		[]string{"org-a"},
		map[string]*fakeAutomaticApprovalRunner{"org-a": runner},
		map[string]*fakeAutomaticApprovalTaskStore{"org-a": store},
	)

	scheduler.RunOnce()

	if runner.runCalls != 0 || len(store.acquired) != 0 {
		t.Fatalf("missing process configuration must not create a task")
	}
}

func TestApprovalSyncJobSchedulerPanicReleasesPersistentLock(t *testing.T) {
	runner := &fakeAutomaticApprovalRunner{
		plan:       ApprovalSyncPlan{ProcessCodes: []string{"PROC"}, StartDate: "2026-10-24", EndDate: "2026-10-26"},
		panicOnRun: true,
	}
	store := &fakeAutomaticApprovalTaskStore{}
	scheduler := newAutomaticApprovalSchedulerForTest(
		[]string{"org-a"},
		map[string]*fakeAutomaticApprovalRunner{"org-a": runner},
		map[string]*fakeAutomaticApprovalTaskStore{"org-a": store},
	)

	scheduler.RunOnce()

	if len(store.completed) != 1 || store.completed[0].Status != ApprovalSyncStatusFailed {
		t.Fatalf("panic must persist failed terminal task: %#v", store.completed)
	}
	if store.completed[0].ErrorCode != ApprovalSyncErrorInternal {
		t.Fatalf("error code=%q", store.completed[0].ErrorCode)
	}
}

func TestApprovalAutomaticSyncConfiguration(t *testing.T) {
	t.Setenv("APPROVAL_AUTO_SYNC_INTERVAL", "")
	if got := approvalAutomaticSyncInterval(); got != 15*time.Minute {
		t.Fatalf("default interval=%s want=15m", got)
	}
	t.Setenv("APPROVAL_AUTO_SYNC_ENABLED", "false")
	if approvalAutomaticSyncEnabled() {
		t.Fatal("expected automatic sync disabled")
	}
	t.Setenv("APPROVAL_AUTO_SYNC_ENABLED", "true")
	if !approvalAutomaticSyncEnabled() {
		t.Fatal("expected automatic sync enabled")
	}
	t.Setenv("APPROVAL_AUTO_SYNC_ENABLED", "invalid")
	if !approvalAutomaticSyncEnabled() {
		t.Fatal("invalid value must fall back to enabled")
	}
	t.Setenv("APPROVAL_AUTO_SYNC_INTERVAL", "30s")
	if got := approvalAutomaticSyncInterval(); got != approvalAutomaticSyncDefaultInterval {
		t.Fatalf("too-small interval=%s want fallback=%s", got, approvalAutomaticSyncDefaultInterval)
	}
	t.Setenv("APPROVAL_AUTO_SYNC_INTERVAL", "10m")
	if got := approvalAutomaticSyncInterval(); got != 10*time.Minute {
		t.Fatalf("interval=%s", got)
	}
}

func TestApprovalSyncJobSchedulerStopsOnOrganizationListingError(t *testing.T) {
	scheduler := newAutomaticApprovalSchedulerForTest(nil, nil, nil)
	scheduler.listOrgIDs = func() ([]string, error) { return nil, errors.New("database unavailable") }
	scheduler.RunOnce()
}
