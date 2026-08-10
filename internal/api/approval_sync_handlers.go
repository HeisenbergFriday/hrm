package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/middleware"
	"peopleops/internal/repository"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	approvalSyncType                = "approvals"
	approvalSyncMaxExecutionTimeout = 15 * time.Minute
	approvalSyncStatusTimeout       = 5 * time.Second
	approvalSyncStaleAfter          = 16 * time.Minute
	approvalSyncPanicCode           = "APPROVAL_SYNC_PANIC"
	approvalSyncStaleCode           = "APPROVAL_SYNC_STALE"
)

var (
	approvalSyncExecutionTimeout     = approvalSyncMaxExecutionTimeout
	launchApprovalSyncBackground     = func(task func()) { go task() }
	newApprovalSyncServiceForRequest = func(db *gorm.DB, orgID string) approvalSyncRunner {
		return service.NewApprovalSyncService(db, orgID)
	}
	newApprovalSyncTaskRepositoryForRequest = func(db *gorm.DB, orgID string) approvalSyncTaskStore {
		return repository.NewApprovalSyncTaskRepositoryWithOrgID(db, orgID)
	}
)

type approvalSyncRunner interface {
	Prepare(service.ApprovalSyncInput, time.Time) (service.ApprovalSyncPlan, error)
	Run(context.Context, service.ApprovalSyncPlan, string) service.ApprovalSyncResult
}

type approvalSyncTaskStore interface {
	Acquire(*database.ApprovalSyncTask, time.Time, time.Time) (*database.ApprovalSyncTask, error)
	Find(string, string) (*database.ApprovalSyncTask, error)
	FindActive(string) (*database.ApprovalSyncTask, error)
	Complete(*database.ApprovalSyncTask) error
	FailIfStale(*database.ApprovalSyncTask, time.Time, time.Time, map[string]interface{}) (bool, error)
}

type approvalSyncRequest struct {
	service.ApprovalSyncInput
	OrgID       *string `json:"org_id"`
	TargetOrgID *string `json:"target_org_id"`
}

func bindApprovalSyncRequest(c *gin.Context, orgID string) (service.ApprovalSyncInput, bool) {
	if !rejectCrossOrgParam(c, orgID,
		c.Query("org_id"), c.Query("target_org_id"),
		c.GetHeader("X-Org-ID"), c.GetHeader("X-Organization-ID"),
	) {
		return service.ApprovalSyncInput{}, false
	}
	var request approvalSyncRequest
	if err := c.ShouldBindJSON(&request); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请求格式错误"})
		return service.ApprovalSyncInput{}, false
	}
	if !rejectClientOrganizationID(c, request.OrgID) || !rejectClientOrganizationID(c, request.TargetOrgID) {
		return service.ApprovalSyncInput{}, false
	}
	return request.ApprovalSyncInput, true
}

func prepareApprovalSync(c *gin.Context) (string, approvalSyncRunner, service.ApprovalSyncPlan, bool) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return "", nil, service.ApprovalSyncPlan{}, false
	}
	input, ok := bindApprovalSyncRequest(c, orgID)
	if !ok {
		return "", nil, service.ApprovalSyncPlan{}, false
	}
	syncService := newApprovalSyncServiceForRequest(middleware.RequestDB(c), orgID)
	plan, err := syncService.Prepare(input, time.Now())
	if err != nil {
		errorCode, message := service.ApprovalSyncPreparationError(err)
		status := http.StatusInternalServerError
		resultStatus := "failed"
		switch errorCode {
		case service.ApprovalSyncErrorConfigMissing:
			status = http.StatusBadRequest
			resultStatus = "config_missing"
		case service.ApprovalSyncErrorInvalidDate:
			status = http.StatusBadRequest
		case service.ApprovalSyncErrorNotAccessible:
			status = http.StatusForbidden
			resultStatus = "forbidden"
		}
		log.Printf("[approval-sync] org=%s stage=prepare result=failed error_code=%s", redactOrgIDForSyncLog(orgID), sanitizeSyncLogText(errorCode))
		c.JSON(status, Response{Code: status, Message: message, Data: gin.H{"status": resultStatus, "error_code": errorCode}})
		return "", nil, service.ApprovalSyncPlan{}, false
	}
	return orgID, syncService, plan, true
}

func approvalSyncRequestID(c *gin.Context, now time.Time) string {
	requestID := orgSyncRequestID(c, now)
	if strings.HasPrefix(requestID, "org-sync-") {
		return "approval-sync-" + strings.TrimPrefix(requestID, "org-sync-")
	}
	return requestID
}

func newApprovalSyncExecutionContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := approvalSyncExecutionTimeout
	if timeout <= 0 || timeout > approvalSyncMaxExecutionTimeout {
		timeout = approvalSyncMaxExecutionTimeout
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

func newApprovalSyncStatusContext(source *gin.Context) (*gin.Context, context.CancelFunc) {
	statusContext := source.Copy()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(source.Request.Context()), approvalSyncStatusTimeout)
	middleware.RebindRequestContext(statusContext, ctx)
	return statusContext, cancel
}

func approvalSyncStatusUpdate(result service.ApprovalSyncResult) orgSyncStatusUpdate {
	result = safeApprovalSyncPersistedResult(result)
	// fail_count has one unit: approval instance failures (fetch detail + write + reconciliation).
	failCount := result.FailCount + result.FetchFailCount + result.ReconcileFailCount
	return orgSyncStatusUpdate{
		SyncType:     approvalSyncType,
		Status:       result.Status,
		Message:      approvalSyncResultMessage(result),
		RequestID:    result.RequestID,
		DurationMS:   result.DurationMS,
		ErrorCode:    approvalSyncResultErrorCode(result),
		SuccessCount: result.SuccessCount,
		FailCount:    failCount,
		Details: map[string]interface{}{
			"result": result, "failed_processes": result.FailedProcesses,
		},
	}
}

func safeApprovalSyncPersistedResult(result service.ApprovalSyncResult) service.ApprovalSyncResult {
	switch result.Status {
	case service.ApprovalSyncStatusSuccess, service.ApprovalSyncStatusPartial, service.ApprovalSyncStatusFailed:
	default:
		result.Status = service.ApprovalSyncStatusFailed
	}
	result.RequestID = sanitizeSyncLogText(result.RequestID)
	if result.DiscoveryErrorCode != "" {
		result.DiscoveryErrorCode = sanitizeSyncLogText(result.DiscoveryErrorCode)
		result.DiscoveryError = "审批流程准备失败，请检查企业流程配置或应用权限"
	}
	for index := range result.Processes {
		process := &result.Processes[index]
		process.ProcessCode = sanitizeSyncLogText(process.ProcessCode)
		process.ErrorCode = sanitizeSyncLogText(process.ErrorCode)
		if process.Error == "" {
			continue
		}
		switch process.ErrorCode {
		case service.ApprovalSyncErrorTimeout:
			process.Error = "审批同步执行超时，请稍后查询或重试"
		case service.ApprovalSyncErrorPartialFetch:
			if process.Status == service.ApprovalSyncStatusPartial {
				process.Error = "部分审批详情拉取失败"
			} else {
				process.Error = "审批详情拉取失败"
			}
		case service.ApprovalSyncErrorReconcile:
			process.Error = "审批已同步，但业务对账失败，请稍后重试"
		default:
			process.Error = "审批同步失败，请检查企业流程配置、应用权限或稍后重试"
		}
	}
	return result
}

func approvalSyncResultMessage(result service.ApprovalSyncResult) string {
	switch result.Status {
	case service.ApprovalSyncStatusSuccess:
		return fmt.Sprintf("审批同步成功：%d 个流程，写入 %d 条", result.ProcessCount, result.SuccessCount)
	case service.ApprovalSyncStatusPartial:
		return fmt.Sprintf("审批同步部分成功：成功写入 %d 条，%d 个流程未完全成功", result.SuccessCount, result.FailedProcesses)
	default:
		return "审批同步失败，请检查企业流程配置、应用权限或稍后重试"
	}
}

func approvalSyncResultErrorCode(result service.ApprovalSyncResult) string {
	for _, process := range result.Processes {
		if process.ErrorCode != "" {
			return process.ErrorCode
		}
	}
	if result.Status == service.ApprovalSyncStatusFailed {
		return service.ApprovalSyncErrorInternal
	}
	return ""
}

func approvalSyncPanicResult(plan service.ApprovalSyncPlan, requestID string, startedAt time.Time) service.ApprovalSyncResult {
	processes := make([]service.ApprovalSyncProcessResult, 0, len(plan.ProcessCodes))
	for _, processCode := range plan.ProcessCodes {
		processes = append(processes, service.ApprovalSyncProcessResult{
			ProcessCode: processCode,
			Status:      service.ApprovalSyncStatusFailed,
			ErrorCode:   approvalSyncPanicCode,
			Error:       "审批同步异常终止，请稍后重试",
		})
	}
	return service.ApprovalSyncResult{
		Status:          service.ApprovalSyncStatusFailed,
		Processes:       processes,
		ProcessCount:    len(plan.ProcessCodes),
		FailedProcesses: len(plan.ProcessCodes),
		StartDate:       plan.StartDate,
		EndDate:         plan.EndDate,
		SyncTime:        time.Now().In(service.ApprovalSyncLocation()).Format(time.RFC3339),
		DurationMS:      time.Since(startedAt).Milliseconds(),
		RequestID:       requestID,
	}
}

func respondApprovalSyncConflict(c *gin.Context, requestID string) {
	c.JSON(http.StatusConflict, Response{
		Code:    http.StatusConflict,
		Message: "当前企业的审批同步正在执行，请勿重复提交",
		Data:    gin.H{"status": "running", "request_id": requestID},
	})
}

func newRunningApprovalSyncTask(orgID, requestID string, plan service.ApprovalSyncPlan, now time.Time) *database.ApprovalSyncTask {
	return &database.ApprovalSyncTask{
		OrgID: orgID, Type: approvalSyncType, RequestID: requestID,
		Status: "running", Message: "审批数据正在后台同步", StartedAt: now, HeartbeatAt: now,
		Details: map[string]interface{}{
			"start_date": plan.StartDate, "end_date": plan.EndDate,
			"process_count": len(plan.ProcessCodes),
		},
	}
}

func acquireApprovalSyncTask(store approvalSyncTaskStore, task *database.ApprovalSyncTask, now time.Time) (*database.ApprovalSyncTask, error) {
	return store.Acquire(task, now.Add(-approvalSyncStaleAfter), now)
}

func completeApprovalSyncTask(store approvalSyncTaskStore, result service.ApprovalSyncResult, message string) error {
	result = safeApprovalSyncPersistedResult(result)
	finishedAt := time.Now().In(service.ApprovalSyncLocation())
	return store.Complete(&database.ApprovalSyncTask{
		Type: approvalSyncType, RequestID: result.RequestID,
		Status: result.Status, Message: message, ErrorCode: approvalSyncResultErrorCode(result),
		SuccessCount: result.SuccessCount, FailCount: result.FailCount + result.FetchFailCount + result.ReconcileFailCount,
		FailedProcesses: result.FailedProcesses, DurationMS: result.DurationMS,
		HeartbeatAt: finishedAt, FinishedAt: &finishedAt,
		Details: map[string]interface{}{"result": result, "failed_processes": result.FailedProcesses},
	})
}

func respondApprovalSyncResult(c *gin.Context, result service.ApprovalSyncResult) {
	status := http.StatusOK
	switch result.Status {
	case service.ApprovalSyncStatusPartial:
		status = http.StatusMultiStatus
	case service.ApprovalSyncStatusFailed:
		status = http.StatusInternalServerError
	}
	c.JSON(status, Response{Code: status, Message: result.Status, Data: result})
}

// syncApprovalCompat keeps the original endpoint while delegating all work to ApprovalSyncService.
func syncApprovalCompat(c *gin.Context) {
	orgID, _, plan, ok := prepareApprovalSync(c)
	if !ok {
		return
	}
	requestID := approvalSyncRequestID(c, time.Now())
	taskStore := newApprovalSyncTaskRepositoryForRequest(middleware.RequestDB(c), orgID)
	conflict, err := acquireApprovalSyncTask(taskStore, newRunningApprovalSyncTask(orgID, requestID, plan, time.Now()), time.Now())
	if errors.Is(err, repository.ErrApprovalSyncTaskRunning) && conflict != nil {
		respondApprovalSyncConflict(c, conflict.RequestID)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "创建审批同步任务失败，请稍后重试"})
		return
	}

	executionContext, cancel := newApprovalSyncExecutionContext(c.Request.Context())
	defer cancel()
	middleware.RebindRequestContext(c, executionContext)
	syncService := newApprovalSyncServiceForRequest(middleware.RequestDB(c), orgID)
	result := syncService.Run(executionContext, plan, requestID)
	result.RequestID = requestID
	result = safeApprovalSyncPersistedResult(result)
	if err := completeApprovalSyncTask(taskStore, result, approvalSyncResultMessage(result)); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "审批同步已执行，但任务结果保存失败", Data: gin.H{"request_id": requestID}})
		return
	}
	statusContext, cancelStatus := newApprovalSyncStatusContext(c)
	defer cancelStatus()
	if err := writeOrgSyncStatusForRequest(statusContext, orgID, approvalSyncStatusUpdate(result)); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "审批同步已执行，但结果状态保存失败", Data: gin.H{"request_id": requestID}})
		return
	}
	respondApprovalSyncResult(c, result)
}

// StartApprovalSync starts a persisted background task and returns immediately.
func StartApprovalSync(c *gin.Context) {
	orgID, _, plan, ok := prepareApprovalSync(c)
	if !ok {
		return
	}
	startedAt := time.Now()
	requestID := approvalSyncRequestID(c, startedAt)
	taskStore := newApprovalSyncTaskRepositoryForRequest(middleware.RequestDB(c), orgID)
	conflict, err := acquireApprovalSyncTask(taskStore, newRunningApprovalSyncTask(orgID, requestID, plan, startedAt), startedAt)
	if errors.Is(err, repository.ErrApprovalSyncTaskRunning) && conflict != nil {
		respondApprovalSyncConflict(c, conflict.RequestID)
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "创建审批同步任务失败，请稍后重试"})
		return
	}

	executionContext, cancelExecution := newApprovalSyncExecutionContext(c.Request.Context())
	backgroundContext := c.Copy()
	middleware.RebindRequestContext(backgroundContext, executionContext)
	if err := writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{
		SyncType:  approvalSyncType,
		Status:    "running",
		Message:   "审批数据正在后台同步",
		RequestID: requestID,
		Details: map[string]interface{}{
			"start_date":    plan.StartDate,
			"end_date":      plan.EndDate,
			"process_count": len(plan.ProcessCodes),
		},
	}); err != nil {
		cancelExecution()
		failed := service.ApprovalSyncResult{Status: service.ApprovalSyncStatusFailed, RequestID: requestID, StartDate: plan.StartDate, EndDate: plan.EndDate}
		_ = completeApprovalSyncTask(taskStore, failed, "审批同步状态初始化失败")
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "创建审批同步任务失败，请稍后重试"})
		return
	}

	c.JSON(http.StatusAccepted, Response{Code: http.StatusAccepted, Message: "running", Data: gin.H{"status": "running", "request_id": requestID}})

	launchApprovalSyncBackground(func() {
		defer cancelExecution()
		defer func() {
			if recovered := recover(); recovered != nil {
				result := approvalSyncPanicResult(plan, requestID, startedAt)
				statusContext, cancelStatus := newApprovalSyncStatusContext(backgroundContext)
				defer cancelStatus()
				finalTaskStore := newApprovalSyncTaskRepositoryForRequest(middleware.RequestDB(statusContext), orgID)
				if err := completeApprovalSyncTask(finalTaskStore, result, approvalSyncResultMessage(result)); err != nil {
					log.Printf("[approval-sync] request_id=%s org=%s result=panic_task_persist_failed error_type=%T", sanitizeSyncLogText(requestID), redactOrgIDForSyncLog(orgID), err)
				}
				if err := writeOrgSyncStatusForRequest(statusContext, orgID, approvalSyncStatusUpdate(result)); err != nil {
					log.Printf("[approval-sync] request_id=%s org=%s result=panic_status_persist_failed error_type=%T", sanitizeSyncLogText(requestID), redactOrgIDForSyncLog(orgID), err)
				}
				log.Printf("[approval-sync] request_id=%s org=%s result=panic panic_type=%T", sanitizeSyncLogText(requestID), redactOrgIDForSyncLog(orgID), recovered)
			}
		}()

		syncService := newApprovalSyncServiceForRequest(middleware.RequestDB(backgroundContext), orgID)
		result := syncService.Run(executionContext, plan, requestID)
		result.RequestID = requestID
		result = safeApprovalSyncPersistedResult(result)
		statusContext, cancelStatus := newApprovalSyncStatusContext(backgroundContext)
		defer cancelStatus()
		finalTaskStore := newApprovalSyncTaskRepositoryForRequest(middleware.RequestDB(statusContext), orgID)
		if err := completeApprovalSyncTask(finalTaskStore, result, approvalSyncResultMessage(result)); err != nil {
			log.Printf("[approval-sync] request_id=%s org=%s result=task_persist_failed error_type=%T", sanitizeSyncLogText(requestID), redactOrgIDForSyncLog(orgID), err)
		}
		if err := writeOrgSyncStatusForRequest(statusContext, orgID, approvalSyncStatusUpdate(result)); err != nil {
			log.Printf("[approval-sync] request_id=%s org=%s result=final_status_persist_failed error_type=%T", sanitizeSyncLogText(requestID), redactOrgIDForSyncLog(orgID), err)
		}
	})
}

// GetApprovalSyncResult validates org, task type, and request_id before returning state.
func GetApprovalSyncResult(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	rawRequestID := strings.TrimSpace(c.Param("request_id"))
	requestID := sanitizeSyncLogText(rawRequestID)
	if requestID == "" || len(rawRequestID) > 128 || len(requestID) > 128 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "同步请求编号无效"})
		return
	}
	taskStore := newApprovalSyncTaskRepositoryForRequest(middleware.RequestDB(c), orgID)
	task, err := taskStore.Find(approvalSyncType, requestID)
	if errors.Is(err, gorm.ErrRecordNotFound) || task == nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "未找到该审批同步任务"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "查询审批同步任务失败，请稍后重试"})
		return
	}
	if task.Status == "running" {
		now := time.Now()
		staleResult := approvalSyncStaleResult(task, now)
		staleDetails := map[string]interface{}{"result": staleResult, "failed_processes": staleResult.FailedProcesses}
		if stale, staleErr := taskStore.FailIfStale(task, now.Add(-approvalSyncStaleAfter), now, staleDetails); staleErr != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "查询审批同步任务失败，请稍后重试"})
			return
		} else if stale {
			c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: service.ApprovalSyncStatusFailed, Data: staleResult})
			return
		}
		c.JSON(http.StatusAccepted, Response{Code: http.StatusAccepted, Message: "running", Data: gin.H{
			"status": "running", "request_id": requestID, "duration_ms": time.Since(task.StartedAt).Milliseconds(),
		}})
		return
	}
	result, exists := task.Details["result"]
	if !exists {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "同步已结束，但结果详情不可用"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: task.Status, Data: result})
}

func approvalSyncStaleResult(task *database.ApprovalSyncTask, now time.Time) service.ApprovalSyncResult {
	startDate, _ := task.Details["start_date"].(string)
	endDate, _ := task.Details["end_date"].(string)
	return service.ApprovalSyncResult{
		Status: service.ApprovalSyncStatusFailed, Processes: []service.ApprovalSyncProcessResult{},
		StartDate: startDate, EndDate: endDate, RequestID: task.RequestID,
		SyncTime:           now.In(service.ApprovalSyncLocation()).Format(time.RFC3339),
		DurationMS:         now.Sub(task.StartedAt).Milliseconds(),
		DiscoveryErrorCode: approvalSyncStaleCode,
		DiscoveryError:     "审批同步任务已失效，请重新发起",
	}
}
