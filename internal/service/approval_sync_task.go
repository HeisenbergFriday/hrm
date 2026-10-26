package service

import (
	"strings"
	"time"

	"peopleops/internal/database"
)

// NewRunningApprovalSyncTask creates the shared persisted lock/task shape used
// by manual and automatic approval synchronization.
func NewRunningApprovalSyncTask(orgID, syncType, requestID, message string, plan ApprovalSyncPlan, now time.Time) *database.ApprovalSyncTask {
	return &database.ApprovalSyncTask{
		OrgID:       strings.TrimSpace(orgID),
		Type:        strings.TrimSpace(syncType),
		RequestID:   strings.TrimSpace(requestID),
		Status:      "running",
		Message:     strings.TrimSpace(message),
		StartedAt:   now,
		HeartbeatAt: now,
		Details: map[string]interface{}{
			"start_date":    plan.StartDate,
			"end_date":      plan.EndDate,
			"process_count": len(plan.ProcessCodes),
		},
	}
}

// CompletedApprovalSyncTask converts a service result into the shared
// persisted terminal task shape. Callers remain responsible for sanitizing any
// user-visible message before passing it here.
func CompletedApprovalSyncTask(syncType string, result ApprovalSyncResult, message, errorCode string, finishedAt time.Time) *database.ApprovalSyncTask {
	return &database.ApprovalSyncTask{
		Type:            strings.TrimSpace(syncType),
		RequestID:       strings.TrimSpace(result.RequestID),
		Status:          result.Status,
		Message:         strings.TrimSpace(message),
		ErrorCode:       strings.TrimSpace(errorCode),
		SuccessCount:    result.SuccessCount,
		FailCount:       result.FailCount + result.FetchFailCount + result.ReconcileFailCount,
		FailedProcesses: result.FailedProcesses,
		DurationMS:      result.DurationMS,
		HeartbeatAt:     finishedAt,
		FinishedAt:      &finishedAt,
		Details: map[string]interface{}{
			"result":           result,
			"failed_processes": result.FailedProcesses,
		},
	}
}

func ApprovalSyncResultMessage(result ApprovalSyncResult) string {
	switch result.Status {
	case ApprovalSyncStatusSuccess:
		return "审批自动同步成功"
	case ApprovalSyncStatusPartial:
		return "审批自动同步部分成功"
	default:
		return "审批自动同步失败"
	}
}

func ApprovalSyncResultErrorCode(result ApprovalSyncResult) string {
	if code := strings.TrimSpace(result.DiscoveryErrorCode); code != "" {
		return code
	}
	for _, process := range result.Processes {
		if code := strings.TrimSpace(process.ErrorCode); code != "" {
			return code
		}
	}
	if result.Status == ApprovalSyncStatusFailed {
		return ApprovalSyncErrorInternal
	}
	return ""
}
