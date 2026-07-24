package service

import (
	"context"
	"errors"
	"testing"

	"peopleops/internal/database"
)

type approvalSourceStub struct {
	rows     []map[string]interface{}
	total    int64
	corpName string
	keyword  string
}

func (s *approvalSourceStub) ListApprovalDetails(_ context.Context, corpName, keyword string, _, _ int) ([]map[string]interface{}, int64, error) {
	s.corpName = corpName
	s.keyword = keyword
	return s.rows, s.total, nil
}

func TestExternalApprovalServiceMutengScopeAndKey(t *testing.T) {
	source := &approvalSourceStub{
		rows:  []map[string]interface{}{{"process_instance_id": "proc-1", "approval_title": "请假"}},
		total: 1,
	}
	svc := NewExternalApprovalService(source, database.OrgIDMuteng)
	items, total, err := svc.List(context.Background(), ExternalApprovalQuery{Keyword: "请假", Page: 1, PageSize: 20})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].Key != "proc-1" {
		t.Fatalf("unexpected result: total=%d items=%#v", total, items)
	}
	if source.corpName != database.CorpNameForOrg(database.OrgIDMuteng) || source.keyword != "请假" {
		t.Fatalf("source args = corp=%q keyword=%q", source.corpName, source.keyword)
	}
}

func TestExternalApprovalServiceRejectsOtherOrg(t *testing.T) {
	source := &approvalSourceStub{}
	svc := NewExternalApprovalService(source, database.OrgIDXiaotie)
	_, _, err := svc.List(context.Background(), ExternalApprovalQuery{})
	if !errors.Is(err, ErrExternalApprovalForbidden) {
		t.Fatalf("error = %v, want ErrExternalApprovalForbidden", err)
	}
	if source.corpName != "" {
		t.Fatalf("source must not be called for forbidden org")
	}
}
