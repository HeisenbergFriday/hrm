package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
)

type fakeEventStore struct {
	mu              sync.Mutex
	records         map[string]*database.DingTalkEventLog // key: org|event
	beginFn         func(log *database.DingTalkEventLog) error
	failMarkSuccess bool
	failMark        bool
}

func newFakeEventStore() *fakeEventStore {
	return &fakeEventStore{records: make(map[string]*database.DingTalkEventLog)}
}

func (f *fakeEventStore) key(orgID, eventID string) string {
	return orgID + "|" + eventID
}

func (f *fakeEventStore) TryBeginProcessing(log *database.DingTalkEventLog) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.beginFn != nil {
		return f.beginFn(log)
	}
	k := f.key(log.OrgID, log.EventID)
	if existing, ok := f.records[k]; ok {
		switch existing.Status {
		case repository.DingTalkEventStatusSuccess, repository.DingTalkEventStatusSkipped:
			return repository.ErrDingTalkEventAlreadyProcessed
		case repository.DingTalkEventStatusProcessing:
			return repository.ErrDingTalkEventInProgress
		}
	}
	cp := *log
	cp.Status = repository.DingTalkEventStatusProcessing
	f.records[k] = &cp
	return nil
}

func (f *fakeEventStore) MarkSuccess(orgID, eventID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failMarkSuccess {
		return errors.New("mark success failed")
	}
	k := f.key(orgID, eventID)
	if rec, ok := f.records[k]; ok {
		rec.Status = repository.DingTalkEventStatusSuccess
		rec.ErrorMessage = ""
	}
	return nil
}

func (f *fakeEventStore) MarkSkipped(orgID, eventID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failMark {
		return errors.New("mark skipped failed")
	}
	k := f.key(orgID, eventID)
	if rec, ok := f.records[k]; ok {
		rec.Status = repository.DingTalkEventStatusSkipped
		rec.ErrorMessage = reason
	}
	return nil
}

func (f *fakeEventStore) MarkFailed(orgID, eventID, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failMark {
		return errors.New("mark failed failed")
	}
	k := f.key(orgID, eventID)
	if rec, ok := f.records[k]; ok {
		rec.Status = repository.DingTalkEventStatusFailed
		rec.ErrorMessage = reason
	}
	return nil
}

type fakeApprovalStore struct {
	mu        sync.Mutex
	byKey     map[string]*database.Approval // org|process
	upsertErr error
	upsertN   int
}

func newFakeApprovalStore() *fakeApprovalStore {
	return &fakeApprovalStore{byKey: make(map[string]*database.Approval)}
}

func (f *fakeApprovalStore) UpsertByOrgProcessID(approval *database.Approval) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.upsertN++
	if f.upsertErr != nil {
		return f.upsertErr
	}
	k := approval.OrgID + "|" + approval.ProcessID
	cp := *approval
	if cp.Content != nil {
		contentCopy := make(map[string]interface{}, len(cp.Content))
		for kk, vv := range cp.Content {
			contentCopy[kk] = vv
		}
		cp.Content = contentCopy
	}
	if cp.Extension != nil {
		extCopy := make(map[string]interface{}, len(cp.Extension))
		for kk, vv := range cp.Extension {
			extCopy[kk] = vv
		}
		cp.Extension = extCopy
	}
	f.byKey[k] = &cp
	return nil
}

func newTestStreamService(orgID string, events *fakeEventStore, approvals *fakeApprovalStore, fetch ApprovalDetailFetcher) *DingTalkStreamService {
	svc := &DingTalkStreamService{
		orgID:        database.NormalizeOrganizationID(orgID),
		eventRepo:    events,
		approvalRepo: approvals,
		fetchDetail:  fetch,
		resolveUserName: func(_, userID string) string {
			if userID == "u1" {
				return "张三"
			}
			return userID
		},
	}
	return svc
}

func TestProcessEvent_InstanceStartSuccess(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	svc := newTestStreamService("org-a", events, approvals, func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
		if orgID != "org-a" || processInstanceID != "pi-1" {
			t.Fatalf("unexpected fetch args org=%s pi=%s", orgID, processInstanceID)
		}
		return &dingtalk.ApprovalInstance{
			ProcessInstanceID: "pi-1",
			Title:             "加班申请",
			Status:            "RUNNING",
			Result:            "",
			CreateTime:        "2026-07-14 10:00:00",
			OriginatorUserID:  "u1",
			FormValues: []map[string]interface{}{
				{"name": "时长", "value": "2"},
			},
		}, nil
	})

	result, err := svc.ProcessEvent(context.Background(), &event.EventHeader{
		EventId:       "evt-start-1",
		EventType:     dingTalkEventTypeInstanceChange,
		EventBornTime: time.Now().UnixMilli(),
	}, `{"processInstanceId":"pi-1","processCode":"PROC-OT","type":"start"}`)
	if err != nil {
		t.Fatalf("ProcessEvent error: %v", err)
	}
	if result != StreamEventResultSuccess {
		t.Fatalf("result=%s want SUCCESS", result)
	}
	got, ok := approvals.byKey["org-a|pi-1"]
	if !ok {
		t.Fatal("approval not upserted")
	}
	if got.Status != "RUNNING" {
		t.Fatalf("status=%s want RUNNING", got.Status)
	}
	if got.ApplicantName != "张三" {
		t.Fatalf("applicant name=%s", got.ApplicantName)
	}
	if got.Extension["process_code"] != "PROC-OT" {
		t.Fatalf("process_code=%v", got.Extension["process_code"])
	}
	if events.records["org-a|evt-start-1"].Status != repository.DingTalkEventStatusSuccess {
		t.Fatalf("event status=%s", events.records["org-a|evt-start-1"].Status)
	}
}

func TestProcessEvent_CompletedAndRefuseMapping(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		result     string
		wantStatus string
	}{
		{name: "agree", status: "COMPLETED", result: "agree", wantStatus: "COMPLETED"},
		{name: "refuse", status: "COMPLETED", result: "refuse", wantStatus: "COMPLETED"},
		{name: "terminated", status: "TERMINATED", result: "", wantStatus: "TERMINATED"},
		{name: "canceled", status: "CANCELED", result: "", wantStatus: "CANCELED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := newFakeEventStore()
			approvals := newFakeApprovalStore()
			svc := newTestStreamService("org-a", events, approvals, func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
				return &dingtalk.ApprovalInstance{
					ProcessInstanceID: processInstanceID,
					Title:             "审批",
					Status:            tt.status,
					Result:            tt.result,
					CreateTime:        "2026-07-14 10:00:00",
					FinishTime:        "2026-07-14 11:00:00",
					OriginatorUserID:  "u1",
				}, nil
			})
			result, err := svc.ProcessEvent(context.Background(), &event.EventHeader{
				EventId:   "evt-" + tt.name,
				EventType: dingTalkEventTypeInstanceChange,
			}, `{"processInstanceId":"pi-map","type":"finish","result":"`+tt.result+`"}`)
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			if result != StreamEventResultSuccess {
				t.Fatalf("result=%s", result)
			}
			got := approvals.byKey["org-a|pi-map"]
			if got.Status != tt.wantStatus {
				t.Fatalf("status=%s want %s", got.Status, tt.wantStatus)
			}
			if got.Extension["result"] != tt.result {
				t.Fatalf("extension.result=%v", got.Extension["result"])
			}
		})
	}
}

func TestProcessEvent_DuplicateEventIDIdempotent(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	fetchN := 0
	svc := newTestStreamService("org-a", events, approvals, func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
		fetchN++
		return &dingtalk.ApprovalInstance{
			ProcessInstanceID: processInstanceID,
			Title:             "审批",
			Status:            "RUNNING",
			CreateTime:        "2026-07-14 10:00:00",
			OriginatorUserID:  "u1",
		}, nil
	})

	header := &event.EventHeader{EventId: "evt-dup", EventType: dingTalkEventTypeInstanceChange}
	payload := `{"processInstanceId":"pi-dup","type":"start"}`
	if _, err := svc.ProcessEvent(context.Background(), header, payload); err != nil {
		t.Fatalf("first process error: %v", err)
	}
	result, err := svc.ProcessEvent(context.Background(), header, payload)
	if err != nil {
		t.Fatalf("second process error: %v", err)
	}
	if result != StreamEventResultSuccess {
		t.Fatalf("result=%s", result)
	}
	if fetchN != 1 {
		t.Fatalf("fetchN=%d want 1", fetchN)
	}
	if approvals.upsertN != 1 {
		t.Fatalf("upsertN=%d want 1", approvals.upsertN)
	}
}

func TestProcessEvent_MissingProcessInstanceID(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	fetchN := 0
	svc := newTestStreamService("org-a", events, approvals, func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
		fetchN++
		return nil, errors.New("should not be called")
	})
	result, err := svc.ProcessEvent(context.Background(), &event.EventHeader{
		EventId:   "evt-missing-pi",
		EventType: dingTalkEventTypeInstanceChange,
	}, `{"type":"start"}`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != StreamEventResultSuccess {
		t.Fatalf("result=%s want SUCCESS", result)
	}
	if fetchN != 0 {
		t.Fatalf("fetch should not be called")
	}
	if approvals.upsertN != 0 {
		t.Fatalf("upsert should not be called")
	}
	rec := events.records["org-a|evt-missing-pi"]
	if rec == nil {
		t.Fatal("event log missing")
	}
	if rec.Status != repository.DingTalkEventStatusSkipped {
		t.Fatalf("status=%s want skipped", rec.Status)
	}
	if rec.ErrorMessage == "" {
		t.Fatal("expected skip reason in error_message")
	}
}

func TestProcessEvent_DetailFetchFailureReturnsLater(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	svc := newTestStreamService("org-a", events, approvals, func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
		return nil, errors.New("dingtalk api unavailable")
	})
	result, err := svc.ProcessEvent(context.Background(), &event.EventHeader{
		EventId:   "evt-api-fail",
		EventType: dingTalkEventTypeInstanceChange,
	}, `{"processInstanceId":"pi-api","type":"start"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if result != StreamEventResultLater {
		t.Fatalf("result=%s want LATER", result)
	}
	if events.records["org-a|evt-api-fail"].Status != repository.DingTalkEventStatusFailed {
		t.Fatalf("event status=%s", events.records["org-a|evt-api-fail"].Status)
	}
}

func TestProcessEvent_DBWriteFailureReturnsLater(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	approvals.upsertErr = errors.New("db write failed")
	svc := newTestStreamService("org-a", events, approvals, func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
		return &dingtalk.ApprovalInstance{
			ProcessInstanceID: processInstanceID,
			Title:             "审批",
			Status:            "RUNNING",
			CreateTime:        "2026-07-14 10:00:00",
			OriginatorUserID:  "u1",
		}, nil
	})
	result, err := svc.ProcessEvent(context.Background(), &event.EventHeader{
		EventId:   "evt-db-fail",
		EventType: dingTalkEventTypeInstanceChange,
	}, `{"processInstanceId":"pi-db","type":"start"}`)
	if err == nil {
		t.Fatal("expected error")
	}
	if result != StreamEventResultLater {
		t.Fatalf("result=%s want LATER", result)
	}
}

func TestProcessEvent_SameProcessIDDifferentOrgIsolated(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	fetch := func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
		return &dingtalk.ApprovalInstance{
			ProcessInstanceID: processInstanceID,
			Title:             "审批-" + orgID,
			Status:            "COMPLETED",
			Result:            "agree",
			CreateTime:        "2026-07-14 10:00:00",
			FinishTime:        "2026-07-14 12:00:00",
			OriginatorUserID:  "u-" + orgID,
		}, nil
	}
	svcA := newTestStreamService("org-a", events, approvals, fetch)
	svcB := newTestStreamService("org-b", events, approvals, fetch)

	if _, err := svcA.ProcessEvent(context.Background(), &event.EventHeader{
		EventId: "evt-a", EventType: dingTalkEventTypeInstanceChange,
	}, `{"processInstanceId":"same-pi","type":"finish","result":"agree"}`); err != nil {
		t.Fatalf("org-a error: %v", err)
	}
	if _, err := svcB.ProcessEvent(context.Background(), &event.EventHeader{
		EventId: "evt-b", EventType: dingTalkEventTypeInstanceChange,
	}, `{"processInstanceId":"same-pi","type":"finish","result":"agree"}`); err != nil {
		t.Fatalf("org-b error: %v", err)
	}

	a := approvals.byKey["org-a|same-pi"]
	b := approvals.byKey["org-b|same-pi"]
	if a == nil || b == nil {
		t.Fatalf("missing approvals a=%v b=%v", a, b)
	}
	if a.Title != "审批-org-a" || b.Title != "审批-org-b" {
		t.Fatalf("titles mixed: a=%s b=%s", a.Title, b.Title)
	}
	if a.ApplicantID == b.ApplicantID {
		t.Fatalf("applicant ids should differ across orgs")
	}
}

func TestProcessEvent_TaskChangeSafeReceive(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	fetchN := 0
	svc := newTestStreamService("org-a", events, approvals, func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error) {
		fetchN++
		return nil, errors.New("should not fetch for task change")
	})
	result, err := svc.ProcessEvent(context.Background(), &event.EventHeader{
		EventId:   "evt-task-1",
		EventType: dingTalkEventTypeTaskChange,
	}, `{"processInstanceId":"pi-task","taskId":"t-1","type":"finish","result":"agree"}`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != StreamEventResultSuccess {
		t.Fatalf("result=%s", result)
	}
	if fetchN != 0 {
		t.Fatalf("detail fetch should be skipped for task change")
	}
	if approvals.upsertN != 0 {
		t.Fatalf("approval must not change on task event")
	}
	if events.records["org-a|evt-task-1"].Status != repository.DingTalkEventStatusSuccess {
		t.Fatalf("task event not marked success")
	}
}

func TestProcessEvent_UnknownEventSkipped(t *testing.T) {
	events := newFakeEventStore()
	approvals := newFakeApprovalStore()
	svc := newTestStreamService("org-a", events, approvals, nil)
	result, err := svc.ProcessEvent(context.Background(), &event.EventHeader{
		EventId:   "evt-unknown",
		EventType: "chat_update",
	}, `{"foo":"bar"}`)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if result != StreamEventResultSuccess {
		t.Fatalf("result=%s", result)
	}
	if events.records["org-a|evt-unknown"].Status != repository.DingTalkEventStatusSkipped {
		t.Fatalf("status=%s", events.records["org-a|evt-unknown"].Status)
	}
	if approvals.upsertN != 0 {
		t.Fatalf("unknown event should not upsert approvals")
	}
}

func TestMapDingTalkApprovalStatus(t *testing.T) {
	if got := mapDingTalkApprovalStatus("running", ""); got != "RUNNING" {
		t.Fatalf("got %s", got)
	}
	if got := mapDingTalkApprovalStatus("COMPLETED", "refuse"); got != "COMPLETED" {
		t.Fatalf("got %s", got)
	}
	if got := mapDingTalkApprovalStatus("", "agree"); got != "COMPLETED" {
		t.Fatalf("got %s", got)
	}
}

func TestParseDingTalkEventPayload(t *testing.T) {
	fields := parseDingTalkEventPayload(`{"processInstanceId":"pi","processCode":"PC","type":"start","result":"agree","taskId":"tk"}`)
	if fields.ProcessInstanceID != "pi" || fields.ProcessCode != "PC" || fields.ChangeType != "start" || fields.TaskID != "tk" {
		t.Fatalf("fields=%+v", fields)
	}
}
