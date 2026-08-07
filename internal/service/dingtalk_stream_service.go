package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"

	"github.com/open-dingtalk/dingtalk-stream-sdk-go/event"
	"github.com/open-dingtalk/dingtalk-stream-sdk-go/payload"
	"gorm.io/gorm"
)

const (
	dingTalkEventTypeInstanceChange = "bpms_instance_change"
	dingTalkEventTypeTaskChange     = "bpms_task_change"
)

// ApprovalDetailFetcher loads full approval detail from DingTalk.
type ApprovalDetailFetcher func(orgID, processInstanceID string) (*dingtalk.ApprovalInstance, error)

// StreamEventResult is the internal processing outcome used by handlers.
type StreamEventResult string

const (
	StreamEventResultSuccess StreamEventResult = "SUCCESS"
	StreamEventResultLater   StreamEventResult = "LATER"
)

type dingTalkEventStore interface {
	TryBeginProcessing(log *database.DingTalkEventLog) error
	MarkSuccess(orgID, eventID string) error
	MarkSkipped(orgID, eventID, reason string) error
	MarkFailed(orgID, eventID, reason string) error
}

type approvalStore interface {
	UpsertByOrgProcessID(approval *database.Approval) error
}

// DingTalkStreamService processes DingTalk Stream approval events into local tables.
type DingTalkStreamService struct {
	orgID           string
	eventRepo       dingTalkEventStore
	approvalRepo    approvalStore
	fetchDetail     ApprovalDetailFetcher
	logPayload      bool
	resolveUserName func(orgID, userID string) string
}

// NewDingTalkStreamService constructs a stream service bound to one organization.
func NewDingTalkStreamService(db *gorm.DB, orgID string) *DingTalkStreamService {
	orgID = database.NormalizeOrganizationID(orgID)
	svc := &DingTalkStreamService{
		orgID:        orgID,
		eventRepo:    repository.NewDingTalkEventRepositoryWithOrgID(db, orgID),
		approvalRepo: repository.NewApprovalRepositoryWithOrgID(db, orgID),
		fetchDetail:  dingtalk.GetApprovalDetailForOrg,
		resolveUserName: func(orgID, userID string) string {
			userID = strings.TrimSpace(userID)
			if userID == "" || db == nil {
				return userID
			}
			var user database.User
			err := db.Where("org_id = ? AND user_id = ?", orgID, userID).First(&user).Error
			if err == nil && strings.TrimSpace(user.Name) != "" {
				return user.Name
			}
			return userID
		},
	}
	return svc
}

// WithDetailFetcher overrides the DingTalk detail API (tests).
func (s *DingTalkStreamService) WithDetailFetcher(fn ApprovalDetailFetcher) *DingTalkStreamService {
	if fn != nil {
		s.fetchDetail = fn
	}
	return s
}

// WithLogPayload enables optional raw payload logging (may contain sensitive form data).
func (s *DingTalkStreamService) WithLogPayload(enabled bool) *DingTalkStreamService {
	s.logPayload = enabled
	return s
}

// HandleDataFrame is the Stream SDK event router entrypoint.
// 业务结果通过 SUCCESS/LATER 响应体表达；始终返回 nil error，避免 SDK 把 Go error 当成连接层失败。
func (s *DingTalkStreamService) HandleDataFrame(ctx context.Context, dataFrame *payload.DataFrame) (*payload.DataFrameResponse, error) {
	header := event.NewEventHeaderFromDataFrame(dataFrame)
	payloadData := ""
	if dataFrame != nil {
		payloadData = dataFrame.Data
	}
	result, err := s.ProcessEvent(ctx, header, payloadData)
	if err != nil {
		log.Printf(
			"钉钉 Stream 事件处理异常: type=%s event_id=%s err=%v",
			header.EventType,
			header.EventId,
			err,
		)
	}
	switch result {
	case StreamEventResultLater:
		resp, respErr := event.NewLaterResponse()
		return resp, respErr
	default:
		resp, respErr := event.NewSuccessResponse()
		return resp, respErr
	}
}

// ProcessEvent implements the business pipeline and returns SUCCESS/LATER.
func (s *DingTalkStreamService) ProcessEvent(ctx context.Context, header *event.EventHeader, rawData string) (StreamEventResult, error) {
	_ = ctx
	if header == nil {
		return StreamEventResultSuccess, nil
	}

	eventID := strings.TrimSpace(header.EventId)
	eventType := strings.TrimSpace(header.EventType)
	bornAt := "未知"
	if header.EventBornTime > 0 {
		bornAt = time.UnixMilli(header.EventBornTime).Format(time.RFC3339)
	}
	log.Printf("收到钉钉事件: type=%s event_id=%s born_at=%s", eventType, eventID, bornAt)
	if s.logPayload && strings.TrimSpace(rawData) != "" {
		log.Printf("钉钉事件原始数据（可能包含敏感业务信息）: %s", rawData)
	}

	if eventID == "" {
		// Without eventId we cannot guarantee idempotency; ACK to avoid poison loops.
		log.Printf("钉钉事件缺少 eventId，已跳过: type=%s", eventType)
		return StreamEventResultSuccess, nil
	}

	fields := parseDingTalkEventPayload(rawData)
	summary := map[string]interface{}{
		"event_type":          eventType,
		"process_instance_id": fields.ProcessInstanceID,
		"process_code":        fields.ProcessCode,
		"type":                fields.ChangeType,
		"result":              fields.Result,
		"staff_id":            fields.StaffID,
		"task_id":             fields.TaskID,
	}

	beginLog := &database.DingTalkEventLog{
		OrgID:             s.orgID,
		EventID:           eventID,
		EventType:         eventType,
		ProcessInstanceID: fields.ProcessInstanceID,
		ProcessCode:       fields.ProcessCode,
		ChangeType:        fields.ChangeType,
		Result:            fields.Result,
		Status:            repository.DingTalkEventStatusProcessing,
		EventBornTime:     header.EventBornTime,
		PayloadSummary:    summary,
	}
	if err := s.eventRepo.TryBeginProcessing(beginLog); err != nil {
		switch {
		case errors.Is(err, repository.ErrDingTalkEventAlreadyProcessed):
			log.Printf("钉钉事件已处理，幂等跳过: event_id=%s type=%s", eventID, eventType)
			return StreamEventResultSuccess, nil
		case errors.Is(err, repository.ErrDingTalkEventInProgress):
			log.Printf("钉钉事件处理中，请求稍后重试: event_id=%s type=%s", eventID, eventType)
			return StreamEventResultLater, err
		default:
			log.Printf("钉钉事件幂等登记失败: event_id=%s err=%v", eventID, err)
			return StreamEventResultLater, err
		}
	}

	switch eventType {
	case dingTalkEventTypeInstanceChange:
		if strings.TrimSpace(fields.ProcessInstanceID) == "" {
			reason := "missing processInstanceId"
			if err := s.eventRepo.MarkSkipped(s.orgID, eventID, reason); err != nil {
				log.Printf("钉钉畸形实例事件标记跳过失败，返回 LATER: event_id=%s err=%v", eventID, err)
				return StreamEventResultLater, err
			}
			log.Printf("钉钉实例事件缺少 processInstanceId，已跳过: event_id=%s", eventID)
			return StreamEventResultSuccess, nil
		}
		if err := s.handleInstanceChange(fields); err != nil {
			_ = s.eventRepo.MarkFailed(s.orgID, eventID, err.Error())
			log.Printf("钉钉实例事件处理失败，返回 LATER: event_id=%s process_instance_id=%s err=%v", eventID, fields.ProcessInstanceID, err)
			return StreamEventResultLater, err
		}
		if err := s.eventRepo.MarkSuccess(s.orgID, eventID); err != nil {
			log.Printf("钉钉事件标记成功失败，返回 LATER: event_id=%s err=%v", eventID, err)
			return StreamEventResultLater, err
		}
		return StreamEventResultSuccess, nil

	case dingTalkEventTypeTaskChange:
		// v1: only record task events; do not mutate approvals final status.
		if err := s.eventRepo.MarkSuccess(s.orgID, eventID); err != nil {
			log.Printf("钉钉任务事件标记成功失败，返回 LATER: event_id=%s err=%v", eventID, err)
			return StreamEventResultLater, err
		}
		log.Printf("钉钉任务事件已记录: event_id=%s process_instance_id=%s task_id=%s", eventID, fields.ProcessInstanceID, fields.TaskID)
		return StreamEventResultSuccess, nil

	default:
		// 安全诊断日志：记录未知事件的 eventType 和 payload 字段名（不含值），
		// 用于确认钉钉是否下发"机器人进群/退群"等事件。
		// 严禁记录 SessionWebhook、Token、Secret、消息正文等敏感信息。
		fieldNames := extractPayloadFieldNames(rawData)
		log.Printf(
			"钉钉未知事件诊断: org_id=%s event_id=%s type=%s field_names=%v",
			s.orgID, eventID, eventType, fieldNames,
		)

		reason := fmt.Sprintf("unsupported event type: %s", eventType)
		if err := s.eventRepo.MarkSkipped(s.orgID, eventID, reason); err != nil {
			log.Printf("钉钉未知事件标记跳过失败，返回 LATER: event_id=%s err=%v", eventID, err)
			return StreamEventResultLater, err
		}
		log.Printf("钉钉未知事件已安全跳过: event_id=%s type=%s", eventID, eventType)
		return StreamEventResultSuccess, nil
	}
}

func (s *DingTalkStreamService) handleInstanceChange(fields dingTalkEventFields) error {
	processInstanceID := strings.TrimSpace(fields.ProcessInstanceID)
	if processInstanceID == "" {
		// Caller should mark skipped; keep defensive guard.
		return errors.New("missing processInstanceId")
	}
	if s.fetchDetail == nil {
		return errors.New("approval detail fetcher is not configured")
	}

	detail, err := s.fetchDetail(s.orgID, processInstanceID)
	if err != nil {
		return fmt.Errorf("fetch approval detail: %w", err)
	}
	if detail == nil {
		return errors.New("approval detail is empty")
	}

	approval, err := buildApprovalFromDetail(s.orgID, fields.ProcessCode, detail, s.resolveUserName)
	if err != nil {
		return err
	}
	if err := s.approvalRepo.UpsertByOrgProcessID(approval); err != nil {
		return fmt.Errorf("upsert approval: %w", err)
	}
	log.Printf(
		"审批实例已同步: org_id=%s process_id=%s status=%s result=%s",
		s.orgID,
		approval.ProcessID,
		approval.Status,
		detail.Result,
	)
	return nil
}

type dingTalkEventFields struct {
	ProcessInstanceID string
	ProcessCode       string
	ChangeType        string
	Result            string
	StaffID           string
	TaskID            string
}

// extractPayloadFieldNames 解析 JSON payload 并返回顶层字段名列表（不含值）。
// 用于诊断日志，帮助确认钉钉下发的事件类型和字段结构。
// 严禁返回敏感字段的值，只返回字段名。
func extractPayloadFieldNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	// 敏感字段黑名单，这些字段名不出现在日志中
	sensitiveFields := map[string]bool{
		"sessionWebhook": true,
		"sessionwebhook": true,
		"token":          true,
		"secret":         true,
		"appSecret":      true,
		"appsecret":      true,
		"access_token":   true,
		"accessToken":    true,
		"password":       true,
		"authorization":  true,
	}
	names := make([]string, 0, len(payload))
	for key := range payload {
		if sensitiveFields[key] {
			continue
		}
		names = append(names, key)
	}
	return names
}

func parseDingTalkEventPayload(raw string) dingTalkEventFields {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return dingTalkEventFields{}
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return dingTalkEventFields{}
	}
	return dingTalkEventFields{
		ProcessInstanceID: firstString(payload, "processInstanceId", "process_instance_id"),
		ProcessCode:       firstString(payload, "processCode", "process_code"),
		ChangeType:        firstString(payload, "type", "changeType", "change_type"),
		Result:            firstString(payload, "result"),
		StaffID:           firstString(payload, "staffId", "staff_id"),
		TaskID:            firstString(payload, "taskId", "task_id"),
	}
}

func firstString(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			switch t := v.(type) {
			case string:
				if s := strings.TrimSpace(t); s != "" {
					return s
				}
			case fmt.Stringer:
				if s := strings.TrimSpace(t.String()); s != "" {
					return s
				}
			}
		}
	}
	return ""
}

func buildApprovalFromDetail(
	orgID string,
	processCode string,
	detail *dingtalk.ApprovalInstance,
	resolveName func(orgID, userID string) string,
) (*database.Approval, error) {
	if detail == nil {
		return nil, errors.New("nil approval detail")
	}
	processID := strings.TrimSpace(detail.ProcessInstanceID)
	if processID == "" {
		return nil, errors.New("process_instance_id missing in detail")
	}

	content := make(map[string]interface{})
	for _, fv := range detail.FormValues {
		name, _ := fv["name"].(string)
		if name == "" {
			continue
		}
		if value, ok := fv["value"]; ok {
			content[name] = value
		}
	}

	applicantID := strings.TrimSpace(detail.OriginatorUserID)
	applicantName := applicantID
	if resolveName != nil {
		applicantName = resolveName(orgID, applicantID)
	}

	createTime := parseDingTalkTime(detail.CreateTime)
	finishTime := parseDingTalkTime(detail.FinishTime)
	status := mapDingTalkApprovalStatus(detail.Status, detail.Result)

	ext := map[string]interface{}{
		"result":     detail.Result,
		"raw_status": detail.Status,
		"source":     "dingtalk_stream",
	}
	if code := strings.TrimSpace(processCode); code != "" {
		ext["process_code"] = code
	}

	return &database.Approval{
		OrgID:         database.NormalizeOrganizationID(orgID),
		ProcessID:     processID,
		Title:         strings.TrimSpace(detail.Title),
		ApplicantID:   applicantID,
		ApplicantName: applicantName,
		Status:        status,
		CreateTime:    createTime,
		FinishTime:    finishTime,
		Content:       content,
		Extension:     ext,
	}, nil
}

// mapDingTalkApprovalStatus normalizes DingTalk instance status/result into local status values.
// Local statuses stay compatible with existing SyncApproval consumers:
// RUNNING / COMPLETED / TERMINATED / CANCELED, with result stored in Extension.
func mapDingTalkApprovalStatus(status, result string) string {
	status = strings.ToUpper(strings.TrimSpace(status))
	result = strings.ToLower(strings.TrimSpace(result))

	switch status {
	case "NEW", "RUNNING":
		return "RUNNING"
	case "COMPLETED":
		// Keep COMPLETED so existing overtime/leave matchers still work;
		// detailed agree/refuse lives in Extension.result.
		return "COMPLETED"
	case "TERMINATED":
		return "TERMINATED"
	case "CANCELED", "CANCELLED":
		return "CANCELED"
	}

	// Fallback for event-only payloads without full detail status.
	switch result {
	case "agree", "refuse", "redirect":
		return "COMPLETED"
	default:
		if status != "" {
			return status
		}
		return "RUNNING"
	}
}

func parseDingTalkTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05.000Z07:00",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t
		}
		if t, err := time.Parse(layout, value); err == nil {
			return t
		}
	}
	return time.Time{}
}
