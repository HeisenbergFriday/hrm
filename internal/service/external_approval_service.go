package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"peopleops/internal/database"
)

var ErrExternalApprovalForbidden = errors.New("external approval data is only available to the muteng organization")

type ExternalApprovalSource interface {
	ListApprovalDetails(context.Context, string, string, int, int) ([]map[string]interface{}, int64, error)
}

type ExternalApprovalQuery struct {
	Keyword  string
	Page     int
	PageSize int
}

type ExternalApprovalRecord struct {
	Key    string                 `json:"key"`
	Fields map[string]interface{} `json:"fields"`
}

type ExternalApprovalService struct {
	source   ExternalApprovalSource
	orgID    string
	corpName string
}

func NewExternalApprovalService(source ExternalApprovalSource, orgID string) *ExternalApprovalService {
	return &ExternalApprovalService{source: source, orgID: strings.TrimSpace(orgID), corpName: database.CorpNameForOrg(database.OrgIDMuteng)}
}

func (s *ExternalApprovalService) List(ctx context.Context, query ExternalApprovalQuery) ([]ExternalApprovalRecord, int64, error) {
	if s == nil || s.source == nil {
		return nil, 0, errors.New("external approval source unavailable")
	}
	if strings.ToLower(strings.TrimSpace(s.orgID)) != database.OrgIDMuteng {
		return nil, 0, ErrExternalApprovalForbidden
	}
	rows, total, err := s.source.ListApprovalDetails(ctx, s.corpName, strings.TrimSpace(query.Keyword), query.Page, query.PageSize)
	if err != nil {
		return nil, 0, err
	}
	items := make([]ExternalApprovalRecord, 0, len(rows))
	for index, fields := range rows {
		key := approvalRecordKey(fields, index)
		items = append(items, ExternalApprovalRecord{Key: key, Fields: fields})
	}
	return items, total, nil
}

func approvalRecordKey(fields map[string]interface{}, index int) string {
	for _, name := range []string{"process_instance_id", "business_id", "id"} {
		if value, ok := fields[name]; ok && strings.TrimSpace(toString(value)) != "" {
			return toString(value)
		}
	}
	return "row-" + toString(index)
}

func toString(value interface{}) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
