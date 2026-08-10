package service

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

const (
	ApprovalReconcileStatusApplied   = "applied"
	ApprovalReconcileStatusSkipped   = "skipped"
	ApprovalReconcileStatusReversed  = "reversed"
	ApprovalReconcileStatusRetryable = "retryable"
	ApprovalReconcileStatusFailed    = "failed"

	ApprovalReconcileBusinessAnnualLeave = "annual_leave"
	ApprovalReconcileBusinessOvertime    = "overtime"
)

type ApprovalBusinessReconcileResult struct {
	Status   string
	Business string
}

type approvalBusinessReconciler interface {
	Reconcile(context.Context, *database.Approval) (ApprovalBusinessReconcileResult, error)
}

type ApprovalBusinessReconciliationService struct {
	orgID              string
	approvalRepo       *repository.ApprovalRepository
	annualLeaveService *AnnualLeaveGrantService
	overtimeService    *OvertimeMatchingService
	annualLeaveKeyword string
}

func NewApprovalBusinessReconciliationService(db *gorm.DB, orgID string) *ApprovalBusinessReconciliationService {
	keyword := strings.TrimSpace(os.Getenv("ANNUAL_LEAVE_APPROVAL_KEYWORD"))
	if keyword == "" {
		keyword = "年假"
	}
	return &ApprovalBusinessReconciliationService{
		orgID:              strings.TrimSpace(orgID),
		approvalRepo:       repository.NewApprovalRepositoryWithOrgID(db, orgID),
		annualLeaveService: NewAnnualLeaveGrantServiceWithOrgID(db, orgID),
		overtimeService:    NewOvertimeMatchingServiceWithOrgID(db, orgID),
		annualLeaveKeyword: keyword,
	}
}

func (s *ApprovalBusinessReconciliationService) Reconcile(ctx context.Context, synced *database.Approval) (ApprovalBusinessReconcileResult, error) {
	if err := ctx.Err(); err != nil {
		return ApprovalBusinessReconcileResult{}, err
	}
	if s == nil || strings.TrimSpace(s.orgID) == "" {
		return ApprovalBusinessReconcileResult{}, fmt.Errorf("org_id required for approval reconciliation")
	}
	if synced == nil {
		return ApprovalBusinessReconcileResult{}, fmt.Errorf("approval required for reconciliation")
	}
	if strings.TrimSpace(synced.OrgID) != s.orgID {
		return ApprovalBusinessReconcileResult{}, fmt.Errorf("approval organization mismatch")
	}
	approval, err := s.approvalRepo.FindByProcessID(synced.ProcessID)
	if err != nil {
		return ApprovalBusinessReconcileResult{}, fmt.Errorf("load synchronized approval: %w", err)
	}
	if s.overtimeService.isOvertimeApproval(approval) {
		if !isApprovalBusinessEffective(approval.Status, approvalResultFromExtension(approval.Extension)) {
			return s.reverseOvertimeApproval(approval)
		}
		before, beforeErr := s.overtimeService.matchRepo.FindByApprovalID(approval.ID)
		if beforeErr != nil && beforeErr != gorm.ErrRecordNotFound {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessOvertime}, beforeErr
		}
		if beforeErr == nil && !isOvertimeRetryableMatchStatus(before.MatchStatus) && before.MatchStatus != "rolled_back" {
			needsSettlement := overtimeMatchNeedsSettlement(before)
			if err := s.overtimeService.ensureExistingMatchSettled(before); err != nil {
				return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessOvertime}, err
			}
			if needsSettlement {
				return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusApplied, Business: ApprovalReconcileBusinessOvertime}, nil
			}
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusSkipped, Business: ApprovalReconcileBusinessOvertime}, nil
		}
		if err := s.overtimeService.MatchApproval(approval.ID); err != nil {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessOvertime}, err
		}
		match, err := s.overtimeService.matchRepo.FindByApprovalID(approval.ID)
		if err != nil {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessOvertime}, err
		}
		if isOvertimeRetryableMatchStatus(match.MatchStatus) {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusRetryable, Business: ApprovalReconcileBusinessOvertime}, nil
		}
		if match.EffectiveOvertimeMinutes <= 0 {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusSkipped, Business: ApprovalReconcileBusinessOvertime}, nil
		}
		return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusApplied, Business: ApprovalReconcileBusinessOvertime}, nil
	}
	if strings.Contains(strings.TrimSpace(approval.Title), s.annualLeaveKeyword) {
		approvalRef := approvalBusinessReference(approval.ProcessID)
		if !isApprovalBusinessEffective(approval.Status, approvalResultFromExtension(approval.Extension)) {
			rollback, err := s.annualLeaveService.RollbackAnnualLeave(approvalRef, approval.Title+"（审批撤销冲正）")
			if err != nil {
				return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessAnnualLeave}, err
			}
			status := ApprovalReconcileStatusSkipped
			if rollback.Changed {
				status = ApprovalReconcileStatusReversed
			}
			return ApprovalBusinessReconcileResult{Status: status, Business: ApprovalReconcileBusinessAnnualLeave}, nil
		}
		days := parseApprovalLeaveDays(approval.Content)
		if days <= 0 {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessAnnualLeave}, fmt.Errorf("annual leave days missing")
		}
		start, end, err := parseApprovalLeaveBusinessPeriod(approval.Content)
		if err != nil {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessAnnualLeave}, err
		}
		mutation, err := s.annualLeaveService.ConsumeAnnualLeaveForPeriod(
			approval.ApplicantID,
			days,
			approvalRef,
			approval.Title+"（审批同步对账）",
			start,
			end,
		)
		if err != nil {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessAnnualLeave}, err
		}
		if !mutation.Changed {
			return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusSkipped, Business: ApprovalReconcileBusinessAnnualLeave}, nil
		}
		return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusApplied, Business: ApprovalReconcileBusinessAnnualLeave}, nil
	}

	return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusSkipped}, nil
}

func overtimeMatchNeedsSettlement(match *database.OvertimeMatchResult) bool {
	if match == nil || match.EffectiveOvertimeMinutes <= 0 {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(match.LocalBalanceStatus), "success") {
		return true
	}
	syncStatus := strings.ToLower(strings.TrimSpace(match.DingtalkSyncStatus))
	return syncStatus != "success" && syncStatus != "skipped"
}

func (s *ApprovalBusinessReconciliationService) reverseOvertimeApproval(approval *database.Approval) (ApprovalBusinessReconcileResult, error) {
	match, err := s.overtimeService.matchRepo.FindByApprovalID(approval.ID)
	if err == gorm.ErrRecordNotFound {
		return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusSkipped, Business: ApprovalReconcileBusinessOvertime}, nil
	}
	if err != nil {
		return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessOvertime}, err
	}
	if match.MatchStatus == "rolled_back" {
		return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusSkipped, Business: ApprovalReconcileBusinessOvertime}, nil
	}
	if err := s.overtimeService.RollbackApprovalMatch(approval.ID); err != nil {
		return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusFailed, Business: ApprovalReconcileBusinessOvertime}, err
	}
	return ApprovalBusinessReconcileResult{Status: ApprovalReconcileStatusReversed, Business: ApprovalReconcileBusinessOvertime}, nil
}

func parseApprovalLeaveBusinessPeriod(content map[string]interface{}) (time.Time, time.Time, error) {
	startRaw := findApprovalContentValue(content, []string{"leave_start_time", "start_time", "请假开始时间", "请假开始日期", "开始时间", "开始日期"})
	endRaw := findApprovalContentValue(content, []string{"leave_end_time", "end_time", "请假结束时间", "请假结束日期", "结束时间", "结束日期"})
	if startRaw == "" || endRaw == "" {
		for _, field := range []string{"请假", "年假"} {
			formJSON, ok := content[field].(string)
			if !ok || strings.TrimSpace(formJSON) == "" {
				continue
			}
			if startRaw == "" {
				startRaw = firstNonEmptyApprovalValue(
					extractDingTalkComponentValue(formJSON, "startTime"),
					extractDingTalkExtValue(formJSON, "_from"),
					extractTimeFromOvertimeData(formJSON, "startTime"),
					extractTimeFromOvertimeData(formJSON, "_from"),
				)
			}
			if endRaw == "" {
				endRaw = firstNonEmptyApprovalValue(
					extractDingTalkComponentValue(formJSON, "finishTime"),
					extractDingTalkExtValue(formJSON, "_to"),
					extractTimeFromOvertimeData(formJSON, "finishTime"),
					extractTimeFromOvertimeData(formJSON, "_to"),
				)
			}
		}
	}
	start := parseAnnualLeaveBusinessTime(startRaw)
	end := parseAnnualLeaveBusinessTime(endRaw)
	if start.IsZero() || end.IsZero() {
		return time.Time{}, time.Time{}, fmt.Errorf("annual leave business date missing or invalid")
	}
	if end.Before(start) {
		return time.Time{}, time.Time{}, fmt.Errorf("annual leave business date range invalid")
	}
	return start, end, nil
}

func firstNonEmptyApprovalValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func parseAnnualLeaveBusinessTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	location := ApprovalSyncLocation()
	for _, layout := range []string{"2006-01-02 15:04:05", "2006-01-02 15:04", "2006-01-02", "2006/01/02 15:04:05", "2006/01/02 15:04", "2006/01/02", time.RFC3339} {
		if parsed, err := time.ParseInLocation(layout, value, location); err == nil {
			return parsed.In(location)
		}
	}
	return time.Time{}
}

func isApprovalBusinessEffective(status, result string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "completed") &&
		strings.EqualFold(strings.TrimSpace(result), "agree")
}

func approvalBusinessReference(processID string) string {
	return "approval:" + strings.TrimSpace(processID)
}
