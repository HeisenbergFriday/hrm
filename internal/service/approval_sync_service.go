package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

const (
	ApprovalSyncStatusSuccess = "success"
	ApprovalSyncStatusPartial = "partial"
	ApprovalSyncStatusFailed  = "failed"

	ApprovalSyncErrorConfigMissing = "APPROVAL_PROCESS_CODES_MISSING"
	ApprovalSyncErrorInvalidDate   = "APPROVAL_SYNC_DATE_INVALID"
	ApprovalSyncErrorTimeout       = "APPROVAL_SYNC_TIMEOUT"
	ApprovalSyncErrorPartialFetch  = "APPROVAL_SYNC_PARTIAL_FETCH"
	ApprovalSyncErrorReconcile     = "APPROVAL_RECONCILE_FAILED"
	ApprovalSyncErrorNotAccessible = "APPROVAL_PROCESS_NOT_ACCESSIBLE"
	ApprovalSyncErrorInternal      = "APPROVAL_SYNC_FAILED"
)

var (
	ErrApprovalProcessCodesMissing  = errors.New("no approval process codes configured")
	ErrApprovalSyncDateInvalid      = errors.New("approval sync date range invalid")
	ErrApprovalProcessNotAccessible = errors.New("approval process is not accessible for organization")
)

func ApprovalSyncLocation() *time.Location {
	return dingtalk.ApprovalBusinessLocation()
}

func ApprovalSyncNow() time.Time {
	return time.Now().In(ApprovalSyncLocation())
}

func ValidateApprovalSyncDates(startDate, endDate string, now time.Time) error {
	return validateApprovalSyncDates(startDate, endDate, now)
}

type ApprovalSyncInput struct {
	ProcessCode string `json:"process_code"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
}

type ApprovalSyncPlan struct {
	ProcessCodes []string `json:"process_codes"`
	StartDate    string   `json:"start_date"`
	EndDate      string   `json:"end_date"`
}

type ApprovalSyncProcessResult struct {
	ProcessCode             string `json:"process_code"`
	Status                  string `json:"status"`
	FetchedCount            int    `json:"fetched_count"`
	FetchFailCount          int    `json:"fetch_fail_count"`
	SuccessCount            int    `json:"success_count"`
	FailCount               int    `json:"fail_count"`
	ReconciledCount         int    `json:"reconciled_count"`
	ReconcileReversedCount  int    `json:"reconcile_reversed_count"`
	ReconcileRetryableCount int    `json:"reconcile_retryable_count"`
	ReconcileSkippedCount   int    `json:"reconcile_skipped_count"`
	ReconcileFailCount      int    `json:"reconcile_fail_count"`
	ErrorCode               string `json:"error_code,omitempty"`
	Error                   string `json:"error,omitempty"`
}

type ApprovalSyncResult struct {
	Status                  string                      `json:"status"`
	Processes               []ApprovalSyncProcessResult `json:"processes"`
	ProcessCount            int                         `json:"process_count"`
	SucceededProcesses      int                         `json:"succeeded_processes"`
	FailedProcesses         int                         `json:"failed_processes"`
	FetchedCount            int                         `json:"fetched_count"`
	FetchFailCount          int                         `json:"fetch_fail_count"`
	SuccessCount            int                         `json:"success_count"`
	FailCount               int                         `json:"fail_count"`
	ReconciledCount         int                         `json:"reconciled_count"`
	ReconcileReversedCount  int                         `json:"reconcile_reversed_count"`
	ReconcileRetryableCount int                         `json:"reconcile_retryable_count"`
	ReconcileSkippedCount   int                         `json:"reconcile_skipped_count"`
	ReconcileFailCount      int                         `json:"reconcile_fail_count"`
	StartDate               string                      `json:"start_date"`
	EndDate                 string                      `json:"end_date"`
	SyncTime                string                      `json:"sync_time"`
	DurationMS              int64                       `json:"duration_ms"`
	RequestID               string                      `json:"request_id,omitempty"`
	DiscoveryErrorCode      string                      `json:"discovery_error_code,omitempty"`
	DiscoveryError          string                      `json:"discovery_error,omitempty"`
}

type approvalSyncStore interface {
	UpsertByOrgProcessID(*database.Approval) error
}

type ApprovalSyncService struct {
	orgID          string
	store          approvalSyncStore
	configForOrg   func(string) (dingtalk.Config, error)
	resolveName    func(string) (string, error)
	fetchApprovals func(context.Context, string, string, string, string) (dingtalk.ApprovalFetchResult, error)
	reconciler     approvalBusinessReconciler
}

func NewApprovalSyncService(db *gorm.DB, orgID string) *ApprovalSyncService {
	return &ApprovalSyncService{
		orgID:        strings.TrimSpace(orgID),
		store:        repository.NewApprovalRepositoryWithOrgID(db, orgID),
		reconciler:   NewApprovalBusinessReconciliationService(db, orgID),
		configForOrg: dingtalk.ConfigForOrgID,
		resolveName: func(originatorUserID string) (string, error) {
			var user database.User
			err := db.Where("org_id = ? AND (user_id = ? OR ding_talk_user_id = ?)", strings.TrimSpace(orgID), originatorUserID, originatorUserID).
				First(&user).Error
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(user.Name), nil
		},
		fetchApprovals: func(ctx context.Context, orgID, processCode, startDate, endDate string) (dingtalk.ApprovalFetchResult, error) {
			return dingtalk.GetApprovalsForOrgContextWithResult(ctx, orgID, processCode, startDate, endDate)
		},
	}
}

func (s *ApprovalSyncService) Prepare(input ApprovalSyncInput, now time.Time) (ApprovalSyncPlan, error) {
	if s == nil || strings.TrimSpace(s.orgID) == "" {
		return ApprovalSyncPlan{}, fmt.Errorf("org_id required for approval sync")
	}
	now = now.In(dingtalk.ApprovalBusinessLocation())
	startDate := strings.TrimSpace(input.StartDate)
	endDate := strings.TrimSpace(input.EndDate)
	if startDate == "" {
		startDate = now.AddDate(0, -1, 0).Format("2006-01-02")
	}
	if endDate == "" {
		endDate = now.Format("2006-01-02")
	}
	if err := validateApprovalSyncDates(startDate, endDate, now); err != nil {
		return ApprovalSyncPlan{}, err
	}

	if s.configForOrg == nil {
		return ApprovalSyncPlan{}, ErrApprovalProcessCodesMissing
	}
	config, err := s.configForOrg(s.orgID)
	if err != nil {
		return ApprovalSyncPlan{}, err
	}
	configuredCodes := StableApprovalProcessCodes(config.ProcessCodes)
	if len(configuredCodes) == 0 {
		return ApprovalSyncPlan{}, ErrApprovalProcessCodesMissing
	}
	processCode := strings.TrimSpace(input.ProcessCode)
	if processCode != "" {
		if containsApprovalProcessCode(configuredCodes, processCode) {
			return ApprovalSyncPlan{ProcessCodes: []string{processCode}, StartDate: startDate, EndDate: endDate}, nil
		}
		return ApprovalSyncPlan{}, ErrApprovalProcessNotAccessible
	}
	return ApprovalSyncPlan{ProcessCodes: configuredCodes, StartDate: startDate, EndDate: endDate}, nil
}

func validateApprovalSyncDates(startDate, endDate string, now time.Time) error {
	location := dingtalk.ApprovalBusinessLocation()
	now = now.In(location)
	start, err := time.ParseInLocation("2006-01-02", startDate, location)
	if err != nil {
		return fmt.Errorf("%w: start_date must use YYYY-MM-DD", ErrApprovalSyncDateInvalid)
	}
	end, err := time.ParseInLocation("2006-01-02", endDate, location)
	if err != nil {
		return fmt.Errorf("%w: end_date must use YYYY-MM-DD", ErrApprovalSyncDateInvalid)
	}
	if end.Before(start) {
		return fmt.Errorf("%w: end_date must not be before start_date", ErrApprovalSyncDateInvalid)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if end.After(today) {
		return fmt.Errorf("%w: end_date must not be in the future", ErrApprovalSyncDateInvalid)
	}
	return nil
}

func containsApprovalProcessCode(codes []string, target string) bool {
	target = strings.TrimSpace(target)
	for _, code := range codes {
		if code == target {
			return true
		}
	}
	return false
}

// StableApprovalProcessCodes removes blank and duplicate values and returns a stable order.
func StableApprovalProcessCodes(configured map[string]string) []string {
	seen := make(map[string]struct{}, len(configured))
	codes := make([]string, 0, len(configured))
	for _, raw := range configured {
		code := strings.TrimSpace(raw)
		if code == "" {
			continue
		}
		if _, exists := seen[code]; exists {
			continue
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func (s *ApprovalSyncService) Run(ctx context.Context, plan ApprovalSyncPlan, requestID string) ApprovalSyncResult {
	startedAt := time.Now()
	result := ApprovalSyncResult{
		Status:       ApprovalSyncStatusSuccess,
		Processes:    make([]ApprovalSyncProcessResult, 0, len(plan.ProcessCodes)),
		ProcessCount: len(plan.ProcessCodes),
		StartDate:    plan.StartDate,
		EndDate:      plan.EndDate,
		RequestID:    requestID,
	}
	for _, processCode := range plan.ProcessCodes {
		processResult := s.syncProcess(ctx, processCode, plan.StartDate, plan.EndDate)
		result.Processes = append(result.Processes, processResult)
		result.FetchedCount += processResult.FetchedCount
		result.FetchFailCount += processResult.FetchFailCount
		result.SuccessCount += processResult.SuccessCount
		result.FailCount += processResult.FailCount
		result.ReconciledCount += processResult.ReconciledCount
		result.ReconcileReversedCount += processResult.ReconcileReversedCount
		result.ReconcileRetryableCount += processResult.ReconcileRetryableCount
		result.ReconcileSkippedCount += processResult.ReconcileSkippedCount
		result.ReconcileFailCount += processResult.ReconcileFailCount
		if processResult.Status == ApprovalSyncStatusSuccess {
			result.SucceededProcesses++
		} else {
			result.FailedProcesses++
		}
	}

	switch {
	case result.FailedProcesses == 0:
		result.Status = ApprovalSyncStatusSuccess
	case result.SucceededProcesses == 0 && result.SuccessCount == 0:
		result.Status = ApprovalSyncStatusFailed
	default:
		result.Status = ApprovalSyncStatusPartial
	}
	result.SyncTime = time.Now().In(dingtalk.ApprovalBusinessLocation()).Format(time.RFC3339)
	result.DurationMS = time.Since(startedAt).Milliseconds()
	return result
}

func (s *ApprovalSyncService) syncProcess(ctx context.Context, processCode, startDate, endDate string) ApprovalSyncProcessResult {
	processResult := ApprovalSyncProcessResult{ProcessCode: processCode, Status: ApprovalSyncStatusSuccess}
	if err := ctx.Err(); err != nil {
		processResult.Status = ApprovalSyncStatusFailed
		processResult.ErrorCode, processResult.Error = approvalSyncSafeError(err)
		return processResult
	}
	fetchResult, err := s.fetchApprovals(ctx, s.orgID, processCode, startDate, endDate)
	if err != nil {
		processResult.Status = ApprovalSyncStatusFailed
		processResult.ErrorCode, processResult.Error = approvalSyncSafeError(err)
		return processResult
	}
	processResult.FetchedCount = len(fetchResult.Instances)
	processResult.FetchFailCount = fetchResult.DetailFailCount
	for _, instance := range fetchResult.Instances {
		if err := ctx.Err(); err != nil {
			processResult.FailCount += len(fetchResult.Instances) - processResult.SuccessCount - processResult.FailCount
			processResult.Status = ApprovalSyncStatusFailed
			processResult.ErrorCode, processResult.Error = approvalSyncSafeError(err)
			break
		}
		applicantName := strings.TrimSpace(instance.OriginatorUserID)
		if s.resolveName != nil {
			if resolved, err := s.resolveName(instance.OriginatorUserID); err == nil && strings.TrimSpace(resolved) != "" {
				applicantName = strings.TrimSpace(resolved)
			}
		}
		approval := approvalFromDingTalk(s.orgID, processCode, applicantName, instance)
		if err := s.store.UpsertByOrgProcessID(approval); err != nil {
			processResult.FailCount++
			continue
		}
		processResult.SuccessCount++
		if s.reconciler == nil {
			continue
		}
		reconcileResult, err := s.reconciler.Reconcile(ctx, approval)
		if err != nil {
			processResult.ReconcileFailCount++
			log.Printf("[ApprovalSync] approval reconciliation failed org=%q process_code=%q process_id=%q error_type=%T", s.orgID, processCode, approval.ProcessID, err)
			continue
		}
		switch reconcileResult.Status {
		case ApprovalReconcileStatusApplied:
			processResult.ReconciledCount++
		case ApprovalReconcileStatusReversed:
			processResult.ReconciledCount++
			processResult.ReconcileReversedCount++
		case ApprovalReconcileStatusRetryable:
			processResult.ReconcileRetryableCount++
		default:
			processResult.ReconcileSkippedCount++
		}
	}
	if processResult.ErrorCode == "" {
		switch {
		case processResult.FetchFailCount == 0 && processResult.FailCount == 0 && processResult.ReconcileFailCount == 0 && processResult.ReconcileRetryableCount == 0:
			processResult.Status = ApprovalSyncStatusSuccess
		case processResult.SuccessCount == 0:
			processResult.Status = ApprovalSyncStatusFailed
			if processResult.ReconcileFailCount > 0 || processResult.ReconcileRetryableCount > 0 {
				processResult.ErrorCode = ApprovalSyncErrorReconcile
				processResult.Error = "审批已同步，但业务对账失败"
			} else if processResult.FetchFailCount > 0 && processResult.FailCount == 0 {
				processResult.ErrorCode = ApprovalSyncErrorPartialFetch
				processResult.Error = "审批详情拉取失败"
			} else {
				processResult.ErrorCode = ApprovalSyncErrorInternal
				processResult.Error = "审批数据写入失败"
			}
		default:
			processResult.Status = ApprovalSyncStatusPartial
			if processResult.ReconcileFailCount > 0 || processResult.ReconcileRetryableCount > 0 {
				processResult.ErrorCode = ApprovalSyncErrorReconcile
				processResult.Error = "部分审批业务对账失败"
			} else if processResult.FetchFailCount > 0 && processResult.FailCount == 0 {
				processResult.ErrorCode = ApprovalSyncErrorPartialFetch
				processResult.Error = "部分审批详情拉取失败"
			} else {
				processResult.ErrorCode = ApprovalSyncErrorInternal
				processResult.Error = "部分审批数据拉取或写入失败"
			}
		}
	}
	return processResult
}

func approvalFromDingTalk(orgID, processCode, applicantName string, instance dingtalk.ApprovalInstance) *database.Approval {
	location := dingtalk.ApprovalBusinessLocation()
	createTime, _ := time.ParseInLocation("2006-01-02 15:04:05", instance.CreateTime, location)
	finishTime, _ := time.ParseInLocation("2006-01-02 15:04:05", instance.FinishTime, location)
	content := make(map[string]interface{})
	for _, formValue := range instance.FormValues {
		name, _ := formValue["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		content[name] = formValue["value"]
	}
	return &database.Approval{
		OrgID:         orgID,
		ProcessID:     strings.TrimSpace(instance.ProcessInstanceID),
		Title:         instance.Title,
		ApplicantID:   instance.OriginatorUserID,
		ApplicantName: applicantName,
		Status:        instance.Status,
		CreateTime:    createTime,
		FinishTime:    finishTime,
		Content:       content,
		Extension: map[string]interface{}{
			"result":       instance.Result,
			"process_code": processCode,
			"source":       "dingtalk_sync",
		},
	}
}

func approvalSyncSafeError(err error) (string, string) {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ApprovalSyncErrorTimeout, "审批同步执行超时，请稍后查询或重试"
	}
	if code := dingtalk.SyncErrorCode(err); code != "" {
		message := dingtalk.SyncErrorSafeMessage(err)
		if message == "" {
			message = "钉钉审批同步失败，请检查应用配置或权限"
		}
		return code, message
	}
	return ApprovalSyncErrorInternal, "审批同步失败，请稍后重试"
}

func ApprovalSyncPreparationError(err error) (string, string) {
	switch {
	case errors.Is(err, ErrApprovalProcessCodesMissing):
		return ApprovalSyncErrorConfigMissing, "当前企业未配置可同步的审批流程代码"
	case errors.Is(err, ErrApprovalSyncDateInvalid):
		return ApprovalSyncErrorInvalidDate, "同步日期范围无效，请检查开始和结束日期"
	case errors.Is(err, ErrApprovalProcessNotAccessible):
		return ApprovalSyncErrorNotAccessible, "当前企业无权访问指定审批流程"
	default:
		return approvalSyncSafeError(err)
	}
}
