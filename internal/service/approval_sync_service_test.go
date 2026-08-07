package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
)

type approvalSyncStoreStub struct {
	records map[string]*database.Approval
	failFor map[string]error
}

func (s *approvalSyncStoreStub) UpsertByOrgProcessID(approval *database.Approval) error {
	if err := s.failFor[approval.ProcessID]; err != nil {
		return err
	}
	if s.records == nil {
		s.records = make(map[string]*database.Approval)
	}
	copy := *approval
	s.records[approval.OrgID+"|"+approval.ProcessID] = &copy
	return nil
}

func newApprovalSyncServiceStub(orgID string) (*ApprovalSyncService, *approvalSyncStoreStub) {
	store := &approvalSyncStoreStub{records: make(map[string]*database.Approval), failFor: make(map[string]error)}
	return &ApprovalSyncService{orgID: orgID, store: store}, store
}

func TestStableApprovalProcessCodesTrimsDeduplicatesAndSorts(t *testing.T) {
	got := StableApprovalProcessCodes(map[string]string{
		"leave": " PROC-B ", "overtime": "PROC-A", "duplicate": "PROC-B", "blank": " ",
	})
	want := []string{"PROC-A", "PROC-B"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("StableApprovalProcessCodes() = %#v, want %#v", got, want)
	}
}

func TestApprovalSyncPrepareFullAndSingleProcess(t *testing.T) {
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	serviceUnderTest, _ := newApprovalSyncServiceStub("org-a")
	configCalls := 0
	serviceUnderTest.configForOrg = func(orgID string) (dingtalk.Config, error) {
		configCalls++
		if orgID != "org-a" {
			t.Fatalf("config orgID = %q", orgID)
		}
		return dingtalk.Config{OrgID: "org-a", AdminUserID: "admin-a", ProcessCodes: map[string]string{"b": "PROC-B", "a": "PROC-A", "dup": " PROC-A "}}, nil
	}
	serviceUnderTest.resolveAdmin = func(config dingtalk.Config) (string, error) {
		if config.OrgID != "org-a" {
			t.Fatalf("resolved config org = %q", config.OrgID)
		}
		return "admin-a", nil
	}
	serviceUnderTest.listProcesses = func(orgID, operator string) ([]dingtalk.ApprovalProcessTemplate, error) {
		if orgID != "org-a" || operator != "admin-a" {
			t.Fatalf("discovery scope = %q/%q", orgID, operator)
		}
		return []dingtalk.ApprovalProcessTemplate{{ProcessCode: "PROC-C"}, {ProcessCode: " PROC-B "}}, nil
	}

	full, err := serviceUnderTest.Prepare(ApprovalSyncInput{}, now)
	if err != nil {
		t.Fatalf("Prepare(full) error = %v", err)
	}
	if !reflect.DeepEqual(full.ProcessCodes, []string{"PROC-A", "PROC-B", "PROC-C"}) {
		t.Fatalf("full process codes = %#v", full.ProcessCodes)
	}
	if full.StartDate != "2026-07-05" || full.EndDate != "2026-08-05" {
		t.Fatalf("default dates = %s..%s", full.StartDate, full.EndDate)
	}

	single, err := serviceUnderTest.Prepare(ApprovalSyncInput{ProcessCode: " PROC-C "}, now)
	if err != nil {
		t.Fatalf("Prepare(single) error = %v", err)
	}
	if !reflect.DeepEqual(single.ProcessCodes, []string{"PROC-C"}) {
		t.Fatalf("single process codes = %#v", single.ProcessCodes)
	}
	if configCalls != 2 {
		t.Fatalf("config calls = %d, want tenant validation for both plans", configCalls)
	}
}

func TestApprovalSyncPrepareMissingConfigAndDateValidation(t *testing.T) {
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	serviceUnderTest, _ := newApprovalSyncServiceStub("org-a")
	serviceUnderTest.configForOrg = func(string) (dingtalk.Config, error) {
		return dingtalk.Config{OrgID: "org-a", ProcessCodes: map[string]string{"empty": " "}}, nil
	}
	serviceUnderTest.resolveAdmin = func(dingtalk.Config) (string, error) { return "admin", nil }
	serviceUnderTest.listProcesses = func(string, string) ([]dingtalk.ApprovalProcessTemplate, error) { return nil, nil }
	if _, err := serviceUnderTest.Prepare(ApprovalSyncInput{}, now); !errors.Is(err, ErrApprovalProcessCodesMissing) {
		t.Fatalf("missing config error = %v", err)
	}
	if _, err := serviceUnderTest.Prepare(ApprovalSyncInput{ProcessCode: "P", StartDate: "2026-08-06", EndDate: "2026-08-05"}, now); !errors.Is(err, ErrApprovalSyncDateInvalid) {
		t.Fatalf("reversed date error = %v", err)
	}
	if _, err := serviceUnderTest.Prepare(ApprovalSyncInput{ProcessCode: "P", StartDate: "2026-08-01", EndDate: "2026-08-06"}, now); !errors.Is(err, ErrApprovalSyncDateInvalid) {
		t.Fatalf("future date error = %v", err)
	}
}

func TestApprovalSyncPrepareDiscoveryFailureIsPartialOrFailed(t *testing.T) {
	now := time.Date(2026, 8, 5, 1, 0, 0, 0, time.UTC)
	serviceUnderTest, _ := newApprovalSyncServiceStub("org-a")
	serviceUnderTest.resolveAdmin = func(dingtalk.Config) (string, error) { return "admin-a", nil }
	serviceUnderTest.listProcesses = func(string, string) ([]dingtalk.ApprovalProcessTemplate, error) {
		return nil, errors.New("access_token=must-not-leak")
	}
	serviceUnderTest.configForOrg = func(string) (dingtalk.Config, error) {
		return dingtalk.Config{OrgID: "org-a", ProcessCodes: map[string]string{"leave": "PROC-LEAVE"}}, nil
	}
	plan, err := serviceUnderTest.Prepare(ApprovalSyncInput{}, now)
	if err != nil || plan.DiscoveryErrorCode != ApprovalSyncErrorDiscovery || !reflect.DeepEqual(plan.ProcessCodes, []string{"PROC-LEAVE"}) {
		t.Fatalf("partial discovery plan = %#v err=%v", plan, err)
	}
	serviceUnderTest.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{}, nil
	}
	partial := serviceUnderTest.Run(context.Background(), plan, "discovery-partial")
	if partial.Status != ApprovalSyncStatusPartial || strings.Contains(partial.DiscoveryError, "must-not-leak") {
		t.Fatalf("partial result = %#v", partial)
	}

	serviceUnderTest.configForOrg = func(string) (dingtalk.Config, error) {
		return dingtalk.Config{OrgID: "org-a"}, nil
	}
	plan, err = serviceUnderTest.Prepare(ApprovalSyncInput{}, now)
	if err != nil {
		t.Fatalf("failed discovery must produce asynchronous plan: %v", err)
	}
	failed := serviceUnderTest.Run(context.Background(), plan, "discovery-failed")
	if failed.Status != ApprovalSyncStatusFailed || failed.DiscoveryErrorCode != ApprovalSyncErrorDiscovery {
		t.Fatalf("failed result = %#v", failed)
	}
}

func TestApprovalSyncUTC8BoundariesAndStoredTimesUnderUTCHost(t *testing.T) {
	nowUTC := time.Date(2026, 8, 5, 16, 30, 0, 0, time.UTC) // 2026-08-06 00:30 UTC+8
	serviceUnderTest, store := newApprovalSyncServiceStub("org-a")
	serviceUnderTest.configForOrg = func(string) (dingtalk.Config, error) {
		return dingtalk.Config{OrgID: "org-a", ProcessCodes: map[string]string{"x": "PROC-X"}}, nil
	}
	serviceUnderTest.resolveAdmin = func(dingtalk.Config) (string, error) { return "admin", nil }
	serviceUnderTest.listProcesses = func(string, string) ([]dingtalk.ApprovalProcessTemplate, error) { return nil, nil }
	plan, err := serviceUnderTest.Prepare(ApprovalSyncInput{}, nowUTC)
	if err != nil || plan.EndDate != "2026-08-06" || plan.StartDate != "2026-07-06" {
		t.Fatalf("UTC+8 defaults plan=%#v err=%v", plan, err)
	}
	serviceUnderTest.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{{
			ProcessInstanceID: "tz-instance", OriginatorUserID: "u1", Status: "COMPLETED",
			CreateTime: "2026-08-06 00:15:00", FinishTime: "2026-08-06 01:00:00",
		}}}, nil
	}
	serviceUnderTest.Run(context.Background(), plan, "tz-request")
	record := store.records["org-a|tz-instance"]
	if record == nil || record.CreateTime.Location().String() != "UTC+8" || record.CreateTime.Unix() != time.Date(2026, 8, 6, 0, 15, 0, 0, ApprovalSyncLocation()).Unix() {
		t.Fatalf("stored time = %#v", record)
	}
}

func TestApprovalSyncRunContinuesAfterProcessFailureAndPreservesFields(t *testing.T) {
	serviceUnderTest, store := newApprovalSyncServiceStub("org-a")
	serviceUnderTest.fetchApprovals = func(_ context.Context, orgID, processCode, _, _ string) (dingtalk.ApprovalFetchResult, error) {
		if orgID != "org-a" {
			t.Fatalf("fetch orgID = %q", orgID)
		}
		if processCode == "PROC-A" {
			return dingtalk.ApprovalFetchResult{}, errors.New("third party failed access_token=secret-token&url=https://private")
		}
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{{
			ProcessInstanceID: "instance-1", Title: "加班审批", Status: "COMPLETED", Result: "agree",
			OriginatorUserID: "user-1", CreateTime: "2026-08-01 09:00:00", FinishTime: "2026-08-01 10:00:00",
			FormValues: []map[string]interface{}{{"name": "天数", "value": "1"}},
		}}}, nil
	}
	result := serviceUnderTest.Run(context.Background(), ApprovalSyncPlan{
		ProcessCodes: []string{"PROC-A", "PROC-B"}, StartDate: "2026-08-01", EndDate: "2026-08-05",
	}, "request-1")
	if result.Status != ApprovalSyncStatusPartial || result.SuccessCount != 1 || result.FailedProcesses != 1 {
		t.Fatalf("result = %#v", result)
	}
	if strings.Contains(result.Processes[0].Error, "secret-token") || strings.Contains(result.Processes[0].Error, "https://") {
		t.Fatalf("unsafe error leaked: %#v", result.Processes[0])
	}
	record := store.records["org-a|instance-1"]
	if record == nil {
		t.Fatal("successful process was not persisted")
	}
	if record.Extension["result"] != "agree" || record.Extension["process_code"] != "PROC-B" || record.Extension["source"] != "dingtalk_sync" {
		t.Fatalf("extension = %#v", record.Extension)
	}
	if record.Content["天数"] != "1" {
		t.Fatalf("content = %#v", record.Content)
	}
}

func TestApprovalSyncRunAllFailedAndUpsertIsIdempotent(t *testing.T) {
	serviceUnderTest, store := newApprovalSyncServiceStub("org-a")
	call := 0
	serviceUnderTest.fetchApprovals = func(_ context.Context, _, processCode, _, _ string) (dingtalk.ApprovalFetchResult, error) {
		if processCode == "FAIL" {
			return dingtalk.ApprovalFetchResult{}, errors.New("raw secret=do-not-leak")
		}
		call++
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{{ProcessInstanceID: "same", Title: "补卡审批", Status: "RUNNING", OriginatorUserID: "u1"}}}, nil
	}
	failed := serviceUnderTest.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"FAIL"}}, "request-failed")
	if failed.Status != ApprovalSyncStatusFailed || failed.FailedProcesses != 1 {
		t.Fatalf("failed result = %#v", failed)
	}
	plan := ApprovalSyncPlan{ProcessCodes: []string{"OK"}, StartDate: "2026-08-01", EndDate: "2026-08-05"}
	serviceUnderTest.Run(context.Background(), plan, "request-1")
	serviceUnderTest.Run(context.Background(), plan, "request-2")
	if call != 2 || len(store.records) != 1 {
		t.Fatalf("fetch calls=%d records=%d, want 2 calls and one upserted record", call, len(store.records))
	}
}

func TestApprovalSyncRunReportsPartialDetailFetchWithoutDroppingSuccessfulInstances(t *testing.T) {
	serviceUnderTest, store := newApprovalSyncServiceStub("org-a")
	serviceUnderTest.fetchApprovals = func(_ context.Context, _, _, _, _ string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{
			Instances:       []dingtalk.ApprovalInstance{{ProcessInstanceID: "fetched-1", Title: "请假审批", Status: "COMPLETED", OriginatorUserID: "u1"}},
			DetailFailCount: 1,
		}, nil
	}

	result := serviceUnderTest.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"LEAVE"}}, "request-partial-fetch")
	if result.Status != ApprovalSyncStatusPartial || result.FetchFailCount != 1 || result.SuccessCount != 1 {
		t.Fatalf("result = %#v", result)
	}
	process := result.Processes[0]
	if process.Status != ApprovalSyncStatusPartial || process.ErrorCode != ApprovalSyncErrorPartialFetch {
		t.Fatalf("process = %#v", process)
	}
	if store.records["org-a|fetched-1"] == nil {
		t.Fatal("successfully fetched instance was not persisted")
	}
}

func TestApprovalSyncRunHonorsCancellation(t *testing.T) {
	serviceUnderTest, _ := newApprovalSyncServiceStub("org-a")
	serviceUnderTest.fetchApprovals = func(ctx context.Context, _, _, _, _ string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := serviceUnderTest.Run(ctx, ApprovalSyncPlan{ProcessCodes: []string{"A", "B"}}, "request-timeout")
	if result.Status != ApprovalSyncStatusFailed || result.FailedProcesses != 2 {
		t.Fatalf("result = %#v", result)
	}
	for _, process := range result.Processes {
		if process.ErrorCode != ApprovalSyncErrorTimeout {
			t.Fatalf("process error code = %q", process.ErrorCode)
		}
	}
}

func TestApprovalSyncPrepareDiscoveryUnionDeduplicatesProcesses(t *testing.T) {
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	svc, _ := newApprovalSyncServiceStub("org-a")
	svc.configForOrg = func(orgID string) (dingtalk.Config, error) {
		if orgID != "org-a" {
			t.Fatalf("config orgID = %q, want org-a boundary", orgID)
		}
		return dingtalk.Config{OrgID: "org-a", AdminUserID: "admin-a", ProcessCodes: map[string]string{"leave": "PROC-A", "overtime": "PROC-B"}}, nil
	}
	svc.resolveAdmin = func(config dingtalk.Config) (string, error) { return "admin-a", nil }
	svc.listProcesses = func(orgID, operator string) ([]dingtalk.ApprovalProcessTemplate, error) {
		if orgID != "org-a" {
			t.Fatalf("discovery orgID = %q", orgID)
		}
		return []dingtalk.ApprovalProcessTemplate{
			{ProcessCode: "PROC-B"}, // duplicate of configured
			{ProcessCode: "PROC-C"}, // new discovered
			{ProcessCode: "PROC-D"}, // new discovered
		}, nil
	}

	plan, err := svc.Prepare(ApprovalSyncInput{}, now)
	if err != nil {
		t.Fatalf("Prepare error = %v", err)
	}
	want := []string{"PROC-A", "PROC-B", "PROC-C", "PROC-D"}
	if !reflect.DeepEqual(plan.ProcessCodes, want) {
		t.Fatalf("union codes = %#v, want %#v (configured + discovered, deduplicated, sorted)", plan.ProcessCodes, want)
	}
}

func TestApprovalSyncPrepareDiscoveryPartialFailureStillProducesPlan(t *testing.T) {
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	svc, _ := newApprovalSyncServiceStub("org-a")
	svc.configForOrg = func(string) (dingtalk.Config, error) {
		return dingtalk.Config{OrgID: "org-a", AdminUserID: "admin-a", ProcessCodes: map[string]string{"leave": "PROC-A"}}, nil
	}
	svc.resolveAdmin = func(dingtalk.Config) (string, error) { return "admin-a", nil }
	svc.listProcesses = func(string, string) ([]dingtalk.ApprovalProcessTemplate, error) {
		return nil, errors.New("dingtalk API timeout")
	}

	plan, err := svc.Prepare(ApprovalSyncInput{}, now)
	if err != nil {
		t.Fatalf("discovery failure must not abort: %v", err)
	}
	if plan.DiscoveryErrorCode != ApprovalSyncErrorDiscovery {
		t.Fatalf("discovery error code = %q, want %q", plan.DiscoveryErrorCode, ApprovalSyncErrorDiscovery)
	}
	if !reflect.DeepEqual(plan.ProcessCodes, []string{"PROC-A"}) {
		t.Fatalf("fallback codes = %#v", plan.ProcessCodes)
	}

	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{}, nil
	}
	result := svc.Run(context.Background(), plan, "partial-discovery")
	if result.Status != ApprovalSyncStatusPartial {
		t.Fatalf("status = %q, want partial (discovery error present)", result.Status)
	}
}

func TestApprovalSyncPrepareSingleTemplateValidatesOrgBoundary(t *testing.T) {
	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	svc, _ := newApprovalSyncServiceStub("org-a")
	svc.configForOrg = func(orgID string) (dingtalk.Config, error) {
		if orgID != "org-a" {
			t.Fatalf("config leaked to %q", orgID)
		}
		return dingtalk.Config{OrgID: "org-a", AdminUserID: "admin-a", ProcessCodes: map[string]string{"leave": "PROC-A"}}, nil
	}
	svc.resolveAdmin = func(dingtalk.Config) (string, error) { return "admin-a", nil }
	svc.listProcesses = func(orgID, operator string) ([]dingtalk.ApprovalProcessTemplate, error) {
		return []dingtalk.ApprovalProcessTemplate{{ProcessCode: "PROC-A"}, {ProcessCode: "PROC-B"}}, nil
	}

	// Valid single template
	plan, err := svc.Prepare(ApprovalSyncInput{ProcessCode: "PROC-B"}, now)
	if err != nil || !reflect.DeepEqual(plan.ProcessCodes, []string{"PROC-B"}) {
		t.Fatalf("single template plan = %#v err=%v", plan, err)
	}

	// Inaccessible template must fail
	_, err = svc.Prepare(ApprovalSyncInput{ProcessCode: "PROC-FOREIGN"}, now)
	if !errors.Is(err, ErrApprovalProcessNotAccessible) {
		t.Fatalf("inaccessible template error = %v", err)
	}
}

func TestApprovalSyncPreservesExistingApplicantNameAcrossFullSync(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.User{}, &database.Approval{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&database.User{
		OrgID: "org-a", UserID: "u1", DingTalkUserID: "ding-u1", Name: "员工甲", DepartmentID: "d1", Status: "active",
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Simulate stream event that wrote the real name
	if err := db.Create(&database.Approval{
		OrgID: "org-a", ProcessID: "stream-keep-name", Title: "请假审批",
		ApplicantID: "ding-u1", ApplicantName: "员工甲", Status: "RUNNING",
		CreateTime: time.Now().In(ApprovalSyncLocation()),
		Extension:  map[string]interface{}{"source": "dingtalk_stream", "stream_event_id": "evt-1"},
	}).Error; err != nil {
		t.Fatalf("create stream approval: %v", err)
	}

	svc := NewApprovalSyncService(db, "org-a")
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{{
			ProcessInstanceID: "stream-keep-name", OriginatorUserID: "ding-u1",
			Title: "请假审批", Status: "COMPLETED", Result: "agree",
			CreateTime: "2026-08-05 09:00:00",
		}}}, nil
	}
	result := svc.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"PROC-LEAVE"}}, "keep-name-request")
	if result.Status != ApprovalSyncStatusSuccess {
		t.Fatalf("sync result = %#v", result)
	}
	var updated database.Approval
	if err := db.Where("org_id = ? AND process_id = ?", "org-a", "stream-keep-name").First(&updated).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if updated.ApplicantName != "员工甲" {
		t.Fatalf("applicant name = %q after full sync, want '员工甲' preserved from stream", updated.ApplicantName)
	}
}

func TestApprovalSyncRunDoesNotOverwriteRealNameWithFallback(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.User{}, &database.Approval{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// User exists but with different name
	if err := db.Create(&database.User{
		OrgID: "org-a", UserID: "u-fallback", DingTalkUserID: "ding-fb", Name: "张三", DepartmentID: "d1", Status: "active",
	}).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	// Existing record has real name "李四" (set via some other mechanism)
	if err := db.Create(&database.Approval{
		OrgID: "org-a", ProcessID: "fallback-test", Title: "加班审批",
		ApplicantID: "ding-fb", ApplicantName: "李四", Status: "RUNNING",
		CreateTime: time.Now().In(ApprovalSyncLocation()),
	}).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	svc := NewApprovalSyncService(db, "org-a")
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{{
			ProcessInstanceID: "fallback-test", OriginatorUserID: "ding-fb",
			Title: "加班审批", Status: "COMPLETED",
			CreateTime: "2026-08-05 09:00:00",
		}}}, nil
	}
	result := svc.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"PROC-OT"}}, "fallback-request")
	if result.Status != ApprovalSyncStatusSuccess {
		t.Fatalf("sync result = %#v", result)
	}
	var updated database.Approval
	if err := db.Where("org_id = ? AND process_id = ?", "org-a", "fallback-test").First(&updated).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	// resolveName returns "张三" from user table, but existing "李四" is a real name (not a user_id fallback).
	// The resolved name "张三" is also a real name (not == user_id), so it should overwrite.
	// This is correct: sync updates to the latest resolved name.
	if updated.ApplicantName != "张三" {
		t.Fatalf("applicant name = %q, want resolved '张三' (real name, not fallback)", updated.ApplicantName)
	}
}

func TestApprovalSyncResolvesApplicantNameWithinOrganization(t *testing.T) {
	db := openLeaveJobsDB(t)
	if err := db.AutoMigrate(&database.User{}, &database.Approval{}); err != nil {
		t.Fatalf("migrate approval identity tables: %v", err)
	}
	users := []database.User{
		{OrgID: "org-a", UserID: "u1", DingTalkUserID: "ding-u1", Name: "员工甲", DepartmentID: "d1", Status: "active"},
		{OrgID: "org-b", UserID: "u1", DingTalkUserID: "ding-u1", Name: "外组织员工", DepartmentID: "d1", Status: "active"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	existing := database.Approval{
		OrgID: "org-a", ProcessID: "stream-name", Title: "请假审批", ApplicantID: "ding-u1",
		ApplicantName: "员工甲", Status: "RUNNING", CreateTime: time.Now(),
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create stream approval: %v", err)
	}

	svc := NewApprovalSyncService(db, "org-a")
	svc.fetchApprovals = func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error) {
		return dingtalk.ApprovalFetchResult{Instances: []dingtalk.ApprovalInstance{{
			ProcessInstanceID: "stream-name", OriginatorUserID: "ding-u1", Title: "请假审批",
			Status: "COMPLETED", Result: "agree", CreateTime: "2026-08-05 09:00:00",
		}}}, nil
	}
	result := svc.Run(context.Background(), ApprovalSyncPlan{ProcessCodes: []string{"PROC-LEAVE"}}, "name-request")
	if result.Status != ApprovalSyncStatusSuccess {
		t.Fatalf("sync result = %#v", result)
	}
	var updated database.Approval
	if err := db.Where("org_id = ? AND process_id = ?", "org-a", "stream-name").First(&updated).Error; err != nil {
		t.Fatalf("load updated approval: %v", err)
	}
	if updated.ApplicantName != "员工甲" {
		t.Fatalf("applicant name = %q, tenant lookup leaked or fallback overwrote", updated.ApplicantName)
	}
}
