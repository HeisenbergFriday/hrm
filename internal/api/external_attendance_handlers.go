package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/middleware"
	"peopleops/internal/repository"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

var (
	launchExternalAttendanceSyncBackground = func(task func()) { go task() }
	externalAttendanceExecutionTimeout     = 10 * time.Minute
)

// ExternalAttendanceSyncStatus GET /api/v1/attendance/external-sync/status
func ExternalAttendanceSyncStatus(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	svc := newExternalSyncService(c, orgID)
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	view, err := svc.GetStatus(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: 200, Message: "success", Data: view})
}

// ExternalAttendanceSyncRun POST /api/v1/attendance/external-sync/run
func ExternalAttendanceSyncRun(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var req struct {
		Source                 string `json:"source"`
		LookbackMinutes        int    `json:"lookback_minutes"`
		FullDepartmentSnapshot bool   `json:"full_department_snapshot"`
	}
	// Empty body is allowed; malformed JSON must be rejected with 400.
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "invalid JSON body"})
		return
	}

	src := strings.TrimSpace(req.Source)
	if src == "" {
		src = "all"
	}
	switch src {
	case "all", "attendance", "department":
	default:
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "invalid source; use all|attendance|department"})
		return
	}
	if req.LookbackMinutes < 0 || req.LookbackMinutes > 60*24*30 {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "lookback_minutes out of range (0..43200)"})
		return
	}

	cfg := database.LoadExternalAttendanceConfig()
	if !cfg.Enabled {
		c.JSON(http.StatusServiceUnavailable, Response{Code: 503, Message: "external attendance sync is disabled"})
		return
	}
	if strings.TrimSpace(cfg.DSN) == "" {
		c.JSON(http.StatusServiceUnavailable, Response{Code: 503, Message: "external attendance source is not configured"})
		return
	}

	executionCtx, cancelExecution := context.WithTimeout(context.WithoutCancel(c.Request.Context()), externalAttendanceExecutionTimeout)
	backgroundContext := c.Copy()
	middleware.RebindRequestContext(backgroundContext, executionCtx)
	svc := newExternalSyncControlService(backgroundContext, orgID)
	operator := ""
	if authCtx, err := middleware.GetAuthContext(c); err == nil && authCtx != nil {
		operator = authCtx.UserID
	}
	opt := service.ExternalSyncRunOptions{
		Source:                 src,
		Trigger:                "manual",
		OperatorUserID:         operator,
		FullDepartmentSnapshot: req.FullDepartmentSnapshot,
	}
	if req.LookbackMinutes > 0 {
		opt.Lookback = time.Duration(req.LookbackMinutes) * time.Minute
	}

	prepared, conflict, err := svc.PrepareRun(opt)
	if err != nil {
		cancelExecution()
		switch {
		case errors.Is(err, service.ErrExternalSyncDisabled):
			c.JSON(http.StatusServiceUnavailable, Response{Code: 503, Message: err.Error()})
			return
		case errors.Is(err, service.ErrExternalSyncNotConfig):
			c.JSON(http.StatusServiceUnavailable, Response{Code: 503, Message: err.Error()})
			return
		case errors.Is(err, service.ErrExternalSyncLocked):
			c.JSON(http.StatusConflict, Response{
				Code:    http.StatusConflict,
				Message: "当前组织已有外部同步任务运行中，请等待任务完成",
				Data:    conflict,
			})
			return
		default:
			log.Printf("[ExternalAttendanceSync] org=%s stage=prepare result=failed error_type=%T", orgID, err)
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "创建同步任务失败，请稍后重试"})
			return
		}
	}

	c.JSON(http.StatusAccepted, Response{Code: http.StatusAccepted, Message: "同步任务已启动", Data: prepared.Job})
	launchExternalAttendanceSyncBackground(func() {
		defer cancelExecution()
		defer func() {
			if recovered := recover(); recovered != nil {
				if recoverErr := svc.FailPreparedRun(executionCtx, prepared, "任务执行异常终止（panic）"); recoverErr != nil {
					log.Printf("[ExternalAttendanceSync] org=%s job=%d stage=async_recovery result=persist_failed error_type=%T", orgID, prepared.Job.ID, recoverErr)
				}
				log.Printf("[ExternalAttendanceSync] org=%s job=%d stage=async_entry result=panic panic_type=%T", orgID, prepared.Job.ID, recovered)
			}
		}()
		runner := newExternalSyncService(backgroundContext, orgID)
		job, runErr := runner.RunPrepared(executionCtx, prepared)
		if runErr != nil {
			status := prepared.Job.Status
			if job != nil {
				status = job.Status
			}
			log.Printf("[ExternalAttendanceSync] org=%s job=%d stage=background result=%s error_type=%T", orgID, prepared.Job.ID, status, runErr)
		}
	})
}

// ExternalAttendanceSyncJobs GET /api/v1/attendance/external-sync/jobs
func ExternalAttendanceSyncJobs(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	svc := newExternalSyncControlService(c, orgID)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	jobs, total, err := svc.ListJobs(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: 200, Message: "success", Data: gin.H{
		"list":      jobs,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}})
}

// ExternalAttendanceSyncJobDetail GET /api/v1/attendance/external-sync/jobs/:id
func ExternalAttendanceSyncJobDetail(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	id64, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id64 == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "invalid job id"})
		return
	}
	svc := newExternalSyncControlService(c, orgID)
	job, err := svc.GetJob(uint(id64))
	if err != nil {
		c.JSON(http.StatusNotFound, Response{Code: 404, Message: "job not found"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: 200, Message: "success", Data: job})
}

// ExternalAttendanceDailyResults GET /api/v1/attendance/external-sync/daily-results
func ExternalAttendanceDailyResults(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}

	now := time.Now()
	defaultEnd := now.Format("2006-01-02")
	defaultStart := now.AddDate(0, 0, -6).Format("2006-01-02")
	startDate := strings.TrimSpace(c.DefaultQuery("start_date", defaultStart))
	endDate := strings.TrimSpace(c.DefaultQuery("end_date", defaultEnd))
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "invalid start_date; use YYYY-MM-DD"})
		return
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "invalid end_date; use YYYY-MM-DD"})
		return
	}
	if end.Before(start) {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "end_date must not be before start_date"})
		return
	}
	if end.Sub(start) > 90*24*time.Hour {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "date range must not exceed 90 days"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	svc := newExternalSyncControlService(c, orgID)
	status := strings.ToLower(strings.TrimSpace(c.DefaultQuery("status", "all")))
	allowedStatuses := map[string]bool{
		"all": true, "normal": true, "exception": true, "approval": true,
		"late": true, "serious_late": true, "early": true,
		"not_signed": true, "absenteeism": true, "leave": true,
		"business_trip": true, "outing": true, "card_correction": true, "overtime": true,
	}
	if !allowedStatuses[status] {
		c.JSON(http.StatusBadRequest, Response{Code: 400, Message: "invalid status filter"})
		return
	}

	items, total, summary, err := svc.ListDailyResults(service.ExternalAttendanceDailyQuery{
		StartDate:    startDate,
		EndDate:      endDate,
		UserID:       strings.TrimSpace(c.Query("user_id")),
		DepartmentID: strings.TrimSpace(c.Query("department_id")),
		Status:       status,
		Page:         page,
		PageSize:     pageSize,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: 500, Message: err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: 200, Message: "success", Data: gin.H{
		"items":      items,
		"total":      total,
		"page":       page,
		"page_size":  pageSize,
		"start_date": startDate,
		"end_date":   endDate,
		"summary":    summary,
	}})
}

func newExternalSyncControlService(c *gin.Context, orgID string) *service.ExternalAttendanceSyncService {
	cfg := database.LoadExternalAttendanceConfig()
	requestDB := middleware.RequestDB(c)
	local := repository.NewExternalAttendanceLocalRepository(requestDB, orgID)
	lookback := time.Duration(cfg.LookbackMinutes) * time.Minute
	svc := service.NewExternalAttendanceSyncService(nil, local, orgID, lookback, cfg.Enabled)
	attendanceSvc := service.NewAttendanceServiceWithOrgID(requestDB, orgID)
	svc.SetRetryableOvertimeRecalculator(attendanceSvc.RecalculateRetryableOvertime)
	return svc
}

func newExternalSyncService(c *gin.Context, orgID string) *service.ExternalAttendanceSyncService {
	cfg := database.LoadExternalAttendanceConfig()
	var sourceRepo *repository.ExternalAttendanceSourceRepository
	if strings.TrimSpace(cfg.DSN) != "" {
		if err := database.InitExternalAttendanceDB(); err == nil {
			if db := database.GetExternalAttendanceDB(); db != nil {
				sourceRepo = repository.NewExternalAttendanceSourceRepository(db, cfg.QueryTimeout)
			}
		}
	}
	requestDB := middleware.RequestDB(c)
	local := repository.NewExternalAttendanceLocalRepository(requestDB, orgID)
	lookback := time.Duration(cfg.LookbackMinutes) * time.Minute
	svc := service.NewExternalAttendanceSyncService(sourceRepo, local, orgID, lookback, cfg.Enabled)
	attendanceSvc := service.NewAttendanceServiceWithOrgID(requestDB, orgID)
	svc.SetRetryableOvertimeRecalculator(attendanceSvc.RecalculateRetryableOvertime)
	return svc
}
