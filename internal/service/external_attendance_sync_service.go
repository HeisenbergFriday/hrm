package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"
)

const (
	externalSyncSourceAttendance = "attendance"
	externalSyncSourceDepartment = "department"
	externalSyncSourceAll        = "all"
	externalSyncDefaultTimeout   = 10 * time.Minute
	externalSyncTerminalTimeout  = 5 * time.Second
	externalSyncRecoveryGrace    = time.Minute
	externalSyncStaleSummary     = "任务执行中断或服务重启，已由系统安全标记为失败"
)

// Sentinel errors for HTTP mapping.
var (
	ErrExternalSyncDisabled   = errors.New("external attendance sync is disabled")
	ErrExternalSyncNotConfig  = errors.New("external attendance source is not configured")
	ErrExternalSyncLocked     = repository.ErrExternalSyncLocked
	ErrExternalSyncNoProgress = errors.New("external attendance sync cursor did not advance")
)

// ExternalAttendanceSyncService imports Doris → local staging → business tables.
type ExternalAttendanceSyncService struct {
	source     *repository.ExternalAttendanceSourceRepository
	local      *repository.ExternalAttendanceLocalRepository
	orgID      string
	lookback   time.Duration
	pageSize   int
	cfgEnabled bool
	// run-scoped counter: approve_list JSON parse failures (reported in ErrorSummary)
	approveParseFailures int
	// affectedAttendancePairs collects (user_id, work_date) pairs from attendance
	// rows that were successfully written during the current run.
	affectedAttendancePairs       []attendanceUserDatePair
	attendanceBusinessWriteFailed bool
	retryableOvertimeRecalculator func([]repository.UserDatePair) (int, error)
	// taskTimeout is the maximum execution time for a sync task
	taskTimeout       time.Duration
	logf              func(string, ...interface{})
	syncAttendanceFn  func(context.Context, *database.ExternalSyncJob, time.Duration) error
	syncDepartmentsFn func(context.Context, *database.ExternalSyncJob, time.Duration, bool) error
}

type attendanceUserDatePair struct {
	UserID   string
	WorkDate string
}

type ExternalSyncRunOptions struct {
	Source                 string // attendance|department|all
	Trigger                string // manual|cron
	OperatorUserID         string
	Lookback               time.Duration // 0 => default
	FullDepartmentSnapshot bool
}

type ExternalSyncStatusView struct {
	OrgID                        string                        `json:"org_id"`
	OrgName                      string                        `json:"org_name"`
	Enabled                      bool                          `json:"enabled"`
	SourceHealthy                bool                          `json:"source_healthy"`
	SourceLatencyMS              int64                         `json:"source_latency_ms"`
	SourceError                  string                        `json:"source_error,omitempty"`
	ExternalLastAttendanceUpdate *time.Time                    `json:"external_last_attendance_update,omitempty"`
	ExternalLastDepartmentUpdate *time.Time                    `json:"external_last_department_update,omitempty"`
	Cursors                      []database.ExternalSyncCursor `json:"cursors"`
	LastJob                      *database.ExternalSyncJob     `json:"last_job,omitempty"`
	ActiveJob                    *database.ExternalSyncJob     `json:"active_job,omitempty"`
}

// ExternalSyncPreparedRun is persisted before asynchronous work starts.
// Lock ownership stays private so clients cannot release another worker's lock.
type ExternalSyncPreparedRun struct {
	Job     *database.ExternalSyncJob
	owner   string
	options ExternalSyncRunOptions
}

func NewExternalAttendanceSyncService(
	source *repository.ExternalAttendanceSourceRepository,
	local *repository.ExternalAttendanceLocalRepository,
	orgID string,
	lookback time.Duration,
	enabled bool,
) *ExternalAttendanceSyncService {
	if lookback <= 0 {
		lookback = 30 * time.Minute
	}
	return &ExternalAttendanceSyncService{
		source:      source,
		local:       local,
		orgID:       database.NormalizeOrganizationID(orgID),
		lookback:    lookback,
		pageSize:    200,
		cfgEnabled:  enabled,
		taskTimeout: externalSyncDefaultTimeout,
		logf:        log.Printf,
	}
}

// SetRetryableOvertimeRecalculator binds the post-write overtime reconciliation
// used by both HTTP and cron entry points. Tests inject a deterministic function.
func (s *ExternalAttendanceSyncService) SetRetryableOvertimeRecalculator(fn func([]repository.UserDatePair) (int, error)) {
	if s != nil {
		s.retryableOvertimeRecalculator = fn
	}
}

// SetTaskTimeout sets the maximum execution time for a sync task
func (s *ExternalAttendanceSyncService) SetTaskTimeout(timeout time.Duration) {
	if timeout > 0 {
		s.taskTimeout = timeout
	}
}

func (s *ExternalAttendanceSyncService) GetStatus(ctx context.Context) (*ExternalSyncStatusView, error) {
	if _, err := s.RecoverStaleJobs(ctx); err != nil {
		return nil, err
	}
	view := &ExternalSyncStatusView{
		OrgID:   s.orgID,
		OrgName: database.CorpNameForOrg(s.orgID),
		Enabled: s.cfgEnabled && database.IsKnownExternalOrg(s.orgID),
	}
	if !database.IsKnownExternalOrg(s.orgID) {
		view.SourceError = "current org is not mapped for external attendance"
		return view, nil
	}
	if !s.cfgEnabled {
		view.SourceError = "external attendance sync is disabled"
	}

	start := time.Now()
	if _, err := database.ExternalAttendanceHealth(ctx); err != nil {
		view.SourceHealthy = false
		if view.SourceError == "" {
			view.SourceError = sanitizeExternalErr(err)
		}
	} else {
		view.SourceHealthy = true
		view.SourceLatencyMS = time.Since(start).Milliseconds()
	}

	if s.source != nil && view.SourceHealthy {
		if t, err := s.source.MaxAttendanceUpdateTime(ctx, database.CorpIDForOrg(s.orgID)); err == nil {
			view.ExternalLastAttendanceUpdate = t
		}
		if t, err := s.source.MaxDepartmentUpdateTime(ctx, database.CorpNameForOrg(s.orgID)); err == nil {
			view.ExternalLastDepartmentUpdate = t
		}
	}

	if s.local != nil {
		if cursors, err := s.local.ListCursors(); err == nil {
			view.Cursors = cursors
		}
		if job, err := s.local.LatestJob(); err == nil {
			view.LastJob = job
		}
		active, err := s.local.ActiveJob()
		if err != nil {
			return nil, err
		}
		view.ActiveJob = active
	}
	return view, nil
}

func (s *ExternalAttendanceSyncService) ListJobs(page, pageSize int) ([]database.ExternalSyncJob, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), externalSyncTerminalTimeout)
	defer cancel()
	if _, err := s.RecoverStaleJobs(ctx); err != nil {
		return nil, 0, err
	}
	return s.local.ListJobs(page, pageSize)
}

func (s *ExternalAttendanceSyncService) GetJob(id uint) (*database.ExternalSyncJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), externalSyncTerminalTimeout)
	defer cancel()
	if _, err := s.RecoverStaleJobs(ctx); err != nil {
		return nil, err
	}
	return s.local.GetJob(id)
}

func (s *ExternalAttendanceSyncService) effectiveTaskTimeout() time.Duration {
	if s == nil || s.taskTimeout <= 0 {
		return externalSyncDefaultTimeout
	}
	return s.taskTimeout
}

// RecoverStaleJobs marks only jobs older than the execution limit plus a grace
// period as failed. It is safe to call repeatedly from startup and read APIs.
func (s *ExternalAttendanceSyncService) RecoverStaleJobs(ctx context.Context) (int64, error) {
	if s == nil || s.local == nil {
		return 0, fmt.Errorf("external sync service not initialized")
	}
	now := time.Now()
	staleBefore := now.Add(-(s.effectiveTaskTimeout() + externalSyncRecoveryGrace))
	return s.local.WithContext(ctx).RecoverStaleRunningJobs(staleBefore, now, externalSyncStaleSummary)
}

// PrepareRun atomically acquires the database gate and persists the running
// task before the handler returns 202.
func (s *ExternalAttendanceSyncService) PrepareRun(opt ExternalSyncRunOptions) (*ExternalSyncPreparedRun, *database.ExternalSyncJob, error) {
	if s == nil || s.local == nil {
		return nil, nil, fmt.Errorf("external sync service not initialized")
	}
	if !s.cfgEnabled {
		return nil, nil, ErrExternalSyncDisabled
	}
	if !database.IsKnownExternalOrg(s.orgID) {
		return nil, nil, fmt.Errorf("org %s is not configured for external attendance sync", s.orgID)
	}
	source := strings.TrimSpace(opt.Source)
	if source == "" {
		source = externalSyncSourceAll
	}
	switch source {
	case externalSyncSourceAttendance, externalSyncSourceDepartment, externalSyncSourceAll:
	default:
		return nil, nil, fmt.Errorf("invalid source %q", source)
	}
	opt.Source = source

	now := time.Now()
	job := &database.ExternalSyncJob{
		OrgID:          s.orgID,
		Trigger:        defaultStr(opt.Trigger, "manual"),
		Source:         source,
		Status:         "running",
		StartedAt:      now,
		OperatorUserID: opt.OperatorUserID,
	}
	owner := fmt.Sprintf("%s-%d-%s", s.orgID, now.UnixNano(), defaultStr(opt.OperatorUserID, "system"))
	timeout := s.effectiveTaskTimeout()
	conflict, err := s.local.AcquireJob(
		job,
		owner,
		timeout+externalSyncRecoveryGrace,
		now.Add(-(timeout + externalSyncRecoveryGrace)),
		externalSyncStaleSummary,
	)
	if err != nil {
		if errors.Is(err, repository.ErrExternalSyncLocked) {
			if conflict == nil {
				conflict, _ = s.local.ActiveJob()
			}
			return nil, conflict, ErrExternalSyncLocked
		}
		return nil, nil, err
	}
	return &ExternalSyncPreparedRun{Job: job, owner: owner, options: opt}, nil, nil
}

// Run executes an incremental sync synchronously. HTTP callers use PrepareRun
// plus RunPrepared so the accepted response is not tied to task completion.
func (s *ExternalAttendanceSyncService) Run(ctx context.Context, opt ExternalSyncRunOptions) (*database.ExternalSyncJob, error) {
	prepared, conflict, err := s.PrepareRun(opt)
	if err != nil {
		return conflict, err
	}
	return s.RunPrepared(ctx, prepared)
}

// RunPrepared executes a persisted task under a hard timeout. The cloned
// repository is bound to the execution context; terminal writes use a separate
// context in executePrepared.
func (s *ExternalAttendanceSyncService) RunPrepared(ctx context.Context, prepared *ExternalSyncPreparedRun) (*database.ExternalSyncJob, error) {
	if s == nil || s.local == nil || prepared == nil || prepared.Job == nil || prepared.owner == "" {
		return nil, fmt.Errorf("external sync prepared run is invalid")
	}
	if s.source == nil {
		err := s.FailPreparedRun(ctx, prepared, "外部考勤数据源初始化失败")
		return prepared.Job, errors.Join(ErrExternalSyncNotConfig, err)
	}
	runCtx, cancel := context.WithTimeout(ctx, s.effectiveTaskTimeout())
	defer cancel()
	runner := *s
	runner.local = s.local.WithContext(runCtx)
	return runner.executePrepared(runCtx, prepared)
}

// FailPreparedRun is the asynchronous entry point's last-resort recovery when
// an unexpected panic escapes RunPrepared itself.
func (s *ExternalAttendanceSyncService) FailPreparedRun(ctx context.Context, prepared *ExternalSyncPreparedRun, summary string) error {
	if s == nil || s.local == nil || prepared == nil || prepared.Job == nil {
		return fmt.Errorf("external sync prepared run is invalid")
	}
	job := prepared.Job
	finished := time.Now()
	job.Status = "failed"
	job.FinishedAt = &finished
	job.ErrorSummary = joinErrorSummary([]string{summary}, job.ErrorSummary)
	terminalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), externalSyncTerminalTimeout)
	defer cancel()
	terminalLocal := s.local.WithContext(terminalCtx)
	var result error
	if err := terminalLocal.UpdateJob(job); err != nil {
		s.log("org=%s job=%d stage=panic_terminal_persist result=failed error_type=%T", s.orgID, job.ID, err)
		result = errors.Join(result, err)
	}
	if err := terminalLocal.ReleaseSyncLock(job.Source, prepared.owner); err != nil {
		s.log("org=%s job=%d stage=panic_lock_release result=failed error_type=%T", s.orgID, job.ID, err)
		result = errors.Join(result, err)
	}
	return result
}

func (s *ExternalAttendanceSyncService) executePrepared(ctx context.Context, prepared *ExternalSyncPreparedRun) (job *database.ExternalSyncJob, returnErr error) {
	job = prepared.Job
	opt := prepared.options
	lookback := s.lookback
	if opt.Lookback > 0 {
		lookback = opt.Lookback
	}

	s.approveParseFailures = 0
	s.affectedAttendancePairs = nil
	s.attendanceBusinessWriteFailed = false
	var runErrs []string
	defer func() {
		if recovered := recover(); recovered != nil {
			runErrs = append(runErrs, "internal:任务执行异常终止")
			returnErr = fmt.Errorf("external sync panic: %T", recovered)
			s.log("org=%s job=%d stage=execute result=panic panic_type=%T", s.orgID, job.ID, recovered)
		}
		if s.approveParseFailures > 0 {
			runErrs = append(runErrs, fmt.Sprintf("approve_list parse failures: %d", s.approveParseFailures))
		}
		finished := time.Now()
		job.FinishedAt = &finished
		applyExternalSyncJobStatus(job, runErrs)
		if job.Status == "failed" && returnErr == nil {
			returnErr = errors.New(job.ErrorSummary)
		}

		terminalCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), externalSyncTerminalTimeout)
		defer cancel()
		terminalLocal := s.local.WithContext(terminalCtx)
		if err := terminalLocal.UpdateJob(job); err != nil {
			s.log("org=%s job=%d stage=terminal_persist result=failed error_type=%T", s.orgID, job.ID, err)
			returnErr = errors.Join(returnErr, fmt.Errorf("persist external sync terminal state: %w", err))
		}
		if err := terminalLocal.ReleaseSyncLock(job.Source, prepared.owner); err != nil {
			s.log("org=%s job=%d stage=lock_release result=failed error_type=%T", s.orgID, job.ID, err)
			returnErr = errors.Join(returnErr, fmt.Errorf("release external sync lock: %w", err))
		}
	}()

	if job.Source == externalSyncSourceAttendance || job.Source == externalSyncSourceAll {
		if err := s.runStage(job.ID, "attendance", func() error {
			if s.syncAttendanceFn != nil {
				return s.syncAttendanceFn(ctx, job, lookback)
			}
			return s.syncAttendance(ctx, job, lookback)
		}); err != nil {
			runErrs = append(runErrs, "attendance:"+sanitizeExternalErr(err))
		} else if !s.attendanceBusinessWriteFailed {
			if err := s.runStage(job.ID, "overtime_recalc", s.recalculateAffectedOvertime); err != nil {
				runErrs = append(runErrs, "overtime_recalc:"+sanitizeSyncError(err))
			}
		}
	}
	if job.Source == externalSyncSourceDepartment || job.Source == externalSyncSourceAll {
		if err := s.runStage(job.ID, "department", func() error {
			if s.syncDepartmentsFn != nil {
				return s.syncDepartmentsFn(ctx, job, lookback, opt.FullDepartmentSnapshot)
			}
			return s.syncDepartments(ctx, job, lookback, opt.FullDepartmentSnapshot)
		}); err != nil {
			runErrs = append(runErrs, "department:"+sanitizeExternalErr(err))
		}
	}
	return job, nil
}

func (s *ExternalAttendanceSyncService) runStage(jobID uint, stage string, fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.log("org=%s job=%d stage=%s result=panic panic_type=%T", s.orgID, jobID, stage, recovered)
			err = fmt.Errorf("任务执行异常终止（%s panic）", stage)
		}
	}()
	return fn()
}

func (s *ExternalAttendanceSyncService) log(format string, args ...interface{}) {
	if s != nil && s.logf != nil {
		s.logf("[ExternalAttendanceSync] "+format, args...)
		return
	}
	log.Printf("[ExternalAttendanceSync] "+format, args...)
}

// applyExternalSyncJobStatus classifies a finished job:
//   - success: no stage errors and Failed==0
//   - partial: at least one successful write AND (Failed>0 or stage errors)
//   - failed: zero successful writes AND (Failed>0 or stage errors)
func applyExternalSyncJobStatus(job *database.ExternalSyncJob, runErrs []string) {
	if job == nil {
		return
	}
	successCount := job.Inserted + job.Updated
	hasStageErr := len(runErrs) > 0
	switch {
	case !hasStageErr && job.Failed == 0:
		job.Status = "success"
	case successCount > 0 && (job.Failed > 0 || hasStageErr):
		job.Status = "partial"
		job.ErrorSummary = joinErrorSummary(runErrs, job.ErrorSummary)
	case successCount == 0 && (job.Failed > 0 || hasStageErr):
		job.Status = "failed"
		job.ErrorSummary = joinErrorSummary(runErrs, job.ErrorSummary)
	default:
		job.Status = "success"
	}
}

func joinErrorSummary(parts []string, existing string) string {
	all := make([]string, 0, len(parts)+1)
	if strings.TrimSpace(existing) != "" {
		all = append(all, strings.TrimSpace(existing))
	}
	all = append(all, parts...)
	msg := strings.Join(all, "; ")
	if len(msg) > 1000 {
		return msg[:1000]
	}
	return msg
}

func (s *ExternalAttendanceSyncService) syncAttendance(ctx context.Context, job *database.ExternalSyncJob, lookback time.Duration) error {
	corpID := database.CorpIDForOrg(s.orgID)
	if corpID == "" {
		return fmt.Errorf("missing corp_id mapping for org %s", s.orgID)
	}

	cur, err := s.local.GetCursor(repository.ExternalSourceAttendanceTable)
	if err != nil {
		return err
	}
	var cursorTime time.Time
	afterKey := ""
	if cur != nil && cur.CursorTime != nil {
		// Incremental: lookback window for late arrivals; do not reuse old tie key after rewind.
		cursorTime = cur.CursorTime.Add(-lookback)
		afterKey = ""
	} else {
		// First-time historical backfill (full history or EXTERNAL_ATTENDANCE_INITIAL_START_TIME).
		cursorTime = database.InitialSyncStartTime()
		if !strings.HasSuffix(job.Trigger, "-backfill") {
			job.Trigger += "-backfill"
		}
	}
	job.CursorFrom = &cursorTime

	var highWater *time.Time
	highKey := ""

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := s.source.ListAttendanceSince(ctx, corpID, cursorTime, afterKey, s.pageSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}

		pageLastTime := rows[len(rows)-1].DBUpdateTime
		pageLastKey := rows[len(rows)-1].PageTieKey
		if pageLastKey == "" {
			return fmt.Errorf("%w: empty page_tie_key", ErrExternalSyncNoProgress)
		}
		// No-progress protection: last tuple must strictly exceed the cursor.
		if pageLastTime.Before(cursorTime) || (pageLastTime.Equal(cursorTime) && pageLastKey <= afterKey) {
			return fmt.Errorf("%w: time=%s key=%s", ErrExternalSyncNoProgress, pageLastTime.Format(time.RFC3339Nano), pageLastKey)
		}

		for _, row := range rows {
			mappedOrg := database.MapOrgIDByCorpID(row.CorpID)
			if mappedOrg == "" {
				job.Failed++
				log.Printf("[ExternalAttendanceSync] unknown corp_id rejected org=%s corp_id=%s", s.orgID, row.CorpID)
				continue
			}
			if mappedOrg != s.orgID {
				job.Skipped++
				continue
			}
			if err := s.applyAttendanceRow(row, job); err != nil {
				job.Failed++
				log.Printf("[ExternalAttendanceSync] apply attendance failed org=%s: %v", s.orgID, sanitizeExternalErr(err))
				continue
			}
			t := row.DBUpdateTime
			if highWater == nil || t.After(*highWater) || (t.Equal(*highWater) && row.PageTieKey > highKey) {
				highWater = &t
				highKey = row.PageTieKey
			}
		}

		// Advance using PageTieKey (never empty record_id alone).
		cursorTime = pageLastTime
		afterKey = pageLastKey
		if len(rows) < s.pageSize {
			break
		}
	}

	now := time.Now()
	newCur := &database.ExternalSyncCursor{
		OrgID:         s.orgID,
		SourceTable:   repository.ExternalSourceAttendanceTable,
		LastSuccessAt: &now,
	}
	if highWater != nil {
		newCur.CursorTime = highWater
		newCur.CursorTieKey = highKey
		job.CursorTo = highWater
	} else if cur != nil {
		newCur.CursorTime = cur.CursorTime
		newCur.CursorTieKey = cur.CursorTieKey
	}
	return s.local.SaveCursor(newCur)
}

func (s *ExternalAttendanceSyncService) applyAttendanceRow(row repository.ExternalAttendanceRow, job *database.ExternalSyncJob) error {
	externalUID := strings.TrimSpace(row.UserID)
	if externalUID == "" {
		job.Skipped++
		return nil
	}
	localUID := database.ScopedExternalID(s.orgID, externalUID)
	sourceKey := buildAttendanceSourceRowKey(s.orgID, row)

	var userCheck *time.Time
	if row.UserCheckTime.Valid {
		t := row.UserCheckTime.Time
		userCheck = &t
	}
	var planCheck *time.Time
	if row.PlanCheckTime.Valid {
		t := row.PlanCheckTime.Time
		planCheck = &t
	}

	// Raw JSON without PII-heavy free text when possible; keep structural fields only.
	rawPayload, _ := json.Marshal(map[string]interface{}{
		"user_id":        externalUID,
		"corp_id":        row.CorpID,
		"corp_name":      row.CorpName,
		"work_date":      row.WorkDate,
		"record_id":      row.RecordID,
		"check_type":     row.CheckType,
		"db_update_time": row.DBUpdateTime,
	})

	raw := &database.ExternalAttendanceRaw{
		OrgID:                    s.orgID,
		SourceTable:              repository.ExternalSourceAttendanceTable,
		SourceRowKey:             sourceKey,
		ExternalUserID:           externalUID,
		LocalUserID:              localUID,
		CorpID:                   row.CorpID,
		CorpName:                 row.CorpName,
		WorkDate:                 row.WorkDate,
		RecordID:                 row.RecordID,
		CheckType:                row.CheckType,
		UserCheckTime:            userCheck,
		PlanCheckTime:            planCheck,
		PlanID:                   row.PlanID,
		ProcInstID:               row.ProcInstID,
		SourceType:               row.SourceType,
		TimeResult:               row.TimeResult,
		LocationResult:           row.LocationResult,
		GroupID:                  row.GroupID,
		UserAddress:              row.UserAddress,
		RawJSON:                  string(rawPayload),
		ApproveListJSON:          row.ApproveListJSON,
		CheckRecordListJSON:      row.CheckRecordListJSON,
		AttendanceResultListJSON: row.AttendanceResultJSON,
		ClassSettingInfoJSON:     row.ClassSettingInfoJSON,
		UserAttendanceInfoJSON:   row.UserAttendanceJSON,
		SourceUpdatedAt:          row.DBUpdateTime,
		SyncStatus:               "pending",
	}

	ins, upd, skip, err := s.local.UpsertAttendanceRaw(raw)
	if err != nil {
		return err
	}
	if skip {
		job.Skipped++
		return nil
	}
	if ins {
		job.Inserted++
	}
	if upd {
		job.Updated++
	}

	approveFailed := false
	if err := s.applyApproveLinks(sourceKey, row.ApproveListJSON); err != nil {
		approveFailed = true
		job.Failed++
		s.approveParseFailures++
		raw.SyncStatus = "partial"
		raw.ApplyError = "approve_list parse failed"
		log.Printf("[ExternalAttendanceSync] approve_list parse failed org=%s", s.orgID)
		// Keep raw approve JSON for offline diagnosis; do not log it.
		_, _, _, _ = s.local.UpsertAttendanceRaw(raw)
	}

	if err := s.applyBusinessAttendance(localUID, externalUID, row); err != nil {
		raw.SyncStatus = "failed"
		raw.ApplyError = truncateErr(err)
		_, _, _, _ = s.local.UpsertAttendanceRaw(raw)
		return err
	}
	if !approveFailed {
		raw.SyncStatus = "applied"
		raw.ApplyError = ""
		_, _, _, _ = s.local.UpsertAttendanceRaw(raw)
	}
	return nil
}

func (s *ExternalAttendanceSyncService) applyApproveLinks(sourceRowKey, approveListJSON string) error {
	approveListJSON = strings.TrimSpace(approveListJSON)
	if approveListJSON == "" || approveListJSON == "null" || approveListJSON == "[]" {
		return nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(approveListJSON), &items); err != nil {
		var sJSON string
		if err2 := json.Unmarshal([]byte(approveListJSON), &sJSON); err2 == nil {
			if err3 := json.Unmarshal([]byte(sJSON), &items); err3 != nil {
				return err
			}
		} else {
			return err
		}
	}
	for _, item := range items {
		procID := stringField(item, "procInst_id", "proc_inst_id", "process_id")
		if procID == "" {
			continue
		}
		rawItem, _ := json.Marshal(item)
		link := &database.ExternalAttendanceApproveLink{
			OrgID:        s.orgID,
			SourceRowKey: sourceRowKey,
			ProcInstID:   procID,
			TagName:      stringField(item, "tag_name", "tagName"),
			BizType:      stringField(item, "biz_type", "bizType"),
			SubType:      stringField(item, "sub_type", "subType"),
			Duration:     stringField(item, "duration"),
			DurationUnit: stringField(item, "duration_unit", "durationUnit"),
			RawItemJSON:  string(rawItem),
		}
		if t := parseFlexibleTime(stringField(item, "begin_time", "beginTime")); t != nil {
			link.BeginTime = t
		}
		if t := parseFlexibleTime(stringField(item, "end_time", "endTime")); t != nil {
			link.EndTime = t
		}
		if t := parseFlexibleTime(stringField(item, "gmt_finished", "gmtFinished")); t != nil {
			link.GmtFinished = t
		}
		begin := ""
		end := ""
		if link.BeginTime != nil {
			begin = link.BeginTime.UTC().Format(time.RFC3339Nano)
		}
		if link.EndTime != nil {
			end = link.EndTime.UTC().Format(time.RFC3339Nano)
		}
		link.ItemKey = repository.BuildApproveItemKey(
			link.ProcInstID, link.TagName, link.BizType, link.SubType,
			begin, end, link.Duration, link.DurationUnit,
		)
		if err := s.local.UpsertApproveLink(link); err != nil {
			return err
		}
	}
	return nil
}

type externalBusinessPunch struct {
	checkType string
	checkTime time.Time
	source    string
	timeRes   string
	locRes    string
}

func collectExternalBusinessPunches(row repository.ExternalAttendanceRow) []externalBusinessPunch {
	punches := make([]externalBusinessPunch, 0, 4)

	if row.UserCheckTime.Valid {
		punches = append(punches, externalBusinessPunch{
			checkType: normalizeExternalCheckType(row.CheckType),
			checkTime: row.UserCheckTime.Time,
			source:    row.SourceType,
			timeRes:   row.TimeResult,
			locRes:    row.LocationResult,
		})
	}

	resultPunchCount := 0
	if strings.TrimSpace(row.AttendanceResultJSON) != "" && row.AttendanceResultJSON != "null" {
		var list []map[string]interface{}
		if err := json.Unmarshal([]byte(row.AttendanceResultJSON), &list); err == nil {
			for _, it := range list {
				ts := stringField(it, "userCheckTime", "user_check_time", "checkTime", "check_time")
				t := parseFlexibleTime(ts)
				if t == nil {
					continue
				}
				checkType := stringField(it, "checkType", "check_type")
				if checkType == "" {
					continue
				}
				punches = append(punches, externalBusinessPunch{
					checkType: normalizeExternalCheckType(stringField(it, "checkType", "check_type")),
					checkTime: *t,
					source:    defaultStr(stringField(it, "sourceType", "source_type"), row.SourceType),
					timeRes:   defaultStr(stringField(it, "timeResult", "time_result"), row.TimeResult),
					locRes:    defaultStr(stringField(it, "locationResult", "location_result"), row.LocationResult),
				})
				resultPunchCount++
			}
		}
	}

	// check_record_list usually has punch timestamps but no OnDuty/OffDuty type.
	// Use it only when the typed attendance_result_list is unavailable.
	if resultPunchCount == 0 && strings.TrimSpace(row.CheckRecordListJSON) != "" && row.CheckRecordListJSON != "null" {
		var list []map[string]interface{}
		if err := json.Unmarshal([]byte(row.CheckRecordListJSON), &list); err == nil {
			for _, it := range list {
				ts := stringField(it, "userCheckTime", "user_check_time", "checkTime", "check_time")
				t := parseFlexibleTime(ts)
				checkType := stringField(it, "checkType", "check_type")
				if t == nil || checkType == "" {
					continue
				}
				punches = append(punches, externalBusinessPunch{
					checkType: normalizeExternalCheckType(checkType),
					checkTime: *t,
					source:    defaultStr(stringField(it, "sourceType", "source_type"), row.SourceType),
					timeRes:   defaultStr(stringField(it, "timeResult", "time_result"), row.TimeResult),
					locRes:    defaultStr(stringField(it, "locationResult", "location_result"), row.LocationResult),
				})
			}
		}
	}
	return punches
}

func (s *ExternalAttendanceSyncService) applyBusinessAttendance(localUID, externalUID string, row repository.ExternalAttendanceRow) error {
	punches := collectExternalBusinessPunches(row)

	seen := map[string]struct{}{}
	affectedPairs := make([]repository.UserDatePair, 0, len(punches))
	for _, p := range punches {
		if p.checkTime.IsZero() {
			continue
		}
		key := p.checkType + "|" + p.checkTime.Format(time.RFC3339Nano)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		att := &database.Attendance{
			OrgID:     s.orgID,
			UserID:    localUID,
			UserName:  "", // filled by repo LookupUserName for inserts; never overwrites non-empty
			CheckType: p.checkType,
			CheckTime: p.checkTime,
			Location:  p.locRes,
			Extension: map[string]interface{}{
				"time_result":      p.timeRes,
				"location_result":  p.locRes,
				"sourceType":       p.source,
				"external_user_id": externalUID,
				"work_date":        row.WorkDate,
				"record_id":        row.RecordID,
				"import_source":    "external_doris",
			},
		}
		if err := s.local.UpsertBusinessAttendance(att); err != nil {
			s.attendanceBusinessWriteFailed = true
			return err
		}
		affectedPairs = append(affectedPairs, attendanceAffectedUserDatePairs(localUID, p.checkTime)...)
	}
	// Track the punch date and, for early-morning punches, the previous overtime
	// work date whose matching window extends through 06:00.
	for _, pair := range affectedPairs {
		s.affectedAttendancePairs = append(s.affectedAttendancePairs, attendanceUserDatePair(pair))
	}
	return nil
}

// AffectedAttendanceUserDatePairs returns the deduplicated (user_id, work_date)
// pairs that were successfully written during the last Run(). Callers use this
// to trigger retryable overtime recalculation.
func (s *ExternalAttendanceSyncService) AffectedAttendanceUserDatePairs() []repository.UserDatePair {
	if len(s.affectedAttendancePairs) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var pairs []repository.UserDatePair
	for _, p := range s.affectedAttendancePairs {
		key := p.UserID + ":" + p.WorkDate
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		pairs = append(pairs, repository.UserDatePair{UserID: p.UserID, WorkDate: p.WorkDate})
	}
	return deduplicateUserDatePairs(pairs)
}

func (s *ExternalAttendanceSyncService) recalculateAffectedOvertime() error {
	pairs := s.AffectedAttendanceUserDatePairs()
	if len(pairs) == 0 || s.retryableOvertimeRecalculator == nil {
		return nil
	}
	_, err := s.retryableOvertimeRecalculator(pairs)
	if err != nil {
		log.Printf("[ExternalAttendanceSync] org=%s affected_pairs=%d 加班重算失败，等待定时补偿: %s",
			s.orgID, len(pairs), sanitizeSyncError(err))
	}
	return err
}

func (s *ExternalAttendanceSyncService) syncDepartments(ctx context.Context, job *database.ExternalSyncJob, lookback time.Duration, fullSnapshot bool) error {
	corpName := database.CorpNameForOrg(s.orgID)
	if corpName == "" {
		return fmt.Errorf("missing corp_name mapping for org %s", s.orgID)
	}

	cur, err := s.local.GetCursor(repository.ExternalSourceDepartmentTable)
	if err != nil {
		return err
	}
	var cursorTime time.Time
	afterKey := ""
	if fullSnapshot {
		cursorTime = time.Unix(0, 0)
	} else if cur != nil && cur.CursorTime != nil {
		cursorTime = cur.CursorTime.Add(-lookback)
	} else {
		// First-time backfill: full history (or configured initial start).
		cursorTime = database.InitialSyncStartTime()
	}
	if job.CursorFrom == nil {
		job.CursorFrom = &cursorTime
	}

	snapshotID := ""
	if fullSnapshot {
		snapshotID = fmt.Sprintf("dept-%s-%d", s.orgID, time.Now().Unix())
	}

	var highWater *time.Time
	highKey := ""
	deptStageFailed := 0

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := s.source.ListDepartmentsSince(ctx, corpName, cursorTime, afterKey, s.pageSize)
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		pageLastTime := rows[len(rows)-1].DBUpdateTime
		pageLastKey := rows[len(rows)-1].PageTieKey
		if pageLastKey == "" || pageLastTime.Before(cursorTime) || (pageLastTime.Equal(cursorTime) && pageLastKey <= afterKey) {
			return fmt.Errorf("%w: department cursor", ErrExternalSyncNoProgress)
		}

		for _, row := range rows {
			mapped := database.MapOrgIDByCorpName(row.CorpName)
			if mapped == "" {
				job.Failed++
				deptStageFailed++
				log.Printf("[ExternalAttendanceSync] unknown corp_name rejected org=%s", s.orgID)
				continue
			}
			if mapped != s.orgID {
				job.Skipped++
				continue
			}
			if err := s.applyDepartmentRow(row, job, snapshotID); err != nil {
				job.Failed++
				deptStageFailed++
				log.Printf("[ExternalAttendanceSync] apply department failed org=%s: %v", s.orgID, sanitizeExternalErr(err))
				continue
			}
			t := row.DBUpdateTime
			if highWater == nil || t.After(*highWater) || (t.Equal(*highWater) && row.PageTieKey > highKey) {
				highWater = &t
				highKey = row.PageTieKey
			}
		}
		cursorTime = pageLastTime
		afterKey = pageLastKey
		if len(rows) < s.pageSize {
			break
		}
	}

	// Deactivate only after a clean full snapshot with zero stage failures.
	if fullSnapshot && snapshotID != "" && deptStageFailed == 0 {
		if _, err := s.local.DeactivateMissingDepartmentRelations(snapshotID, time.Now()); err != nil {
			return fmt.Errorf("deactivate missing department relations: %w", err)
		}
	} else if fullSnapshot && deptStageFailed > 0 {
		log.Printf("[ExternalAttendanceSync] skip deactivate: dept stage failures=%d org=%s", deptStageFailed, s.orgID)
	}

	now := time.Now()
	newCur := &database.ExternalSyncCursor{
		OrgID:         s.orgID,
		SourceTable:   repository.ExternalSourceDepartmentTable,
		LastSuccessAt: &now,
	}
	if highWater != nil {
		newCur.CursorTime = highWater
		newCur.CursorTieKey = highKey
		job.CursorTo = highWater
	} else if cur != nil {
		newCur.CursorTime = cur.CursorTime
		newCur.CursorTieKey = cur.CursorTieKey
	}
	if deptStageFailed > 0 {
		return fmt.Errorf("department sync partial failures: %d", deptStageFailed)
	}
	return s.local.SaveCursor(newCur)
}

func (s *ExternalAttendanceSyncService) applyDepartmentRow(row repository.ExternalDepartmentRow, job *database.ExternalSyncJob, snapshotID string) error {
	externalUID := strings.TrimSpace(row.UserID)
	deptID := strings.TrimSpace(row.DepartmentID)
	if externalUID == "" || deptID == "" {
		job.Skipped++
		return nil
	}
	localUID := database.ScopedExternalID(s.orgID, externalUID)
	sourceKey := buildDepartmentSourceRowKey(s.orgID, externalUID, deptID)

	raw := &database.ExternalUserDepartmentRaw{
		OrgID:                    s.orgID,
		SourceRowKey:             sourceKey,
		ExternalUserID:           externalUID,
		LocalUserID:              localUID,
		CorpName:                 row.CorpName,
		UserName:                 row.UserName,
		Title:                    row.Title,
		WorkPlace:                row.WorkPlace,
		DepartmentID:             deptID,
		DepartmentName:           row.DepartmentName,
		DepartmentLevel:          row.DepartmentLevel,
		FirstLevelDepartmentID:   row.FirstLevelDepartmentID,
		FirstLevelDepartmentName: row.FirstLevelDepartmentName,
		SourceUpdatedAt:          row.DBUpdateTime,
		SnapshotID:               snapshotID,
		SyncStatus:               "pending",
	}
	ins, upd, skip, err := s.local.UpsertDepartmentRaw(raw)
	if err != nil {
		return err
	}
	if skip {
		job.Skipped++
		return nil
	}
	if ins {
		job.Inserted++
	}
	if upd {
		job.Updated++
	}

	now := time.Now()
	rel := &database.UserDepartmentRelation{
		OrgID:                    s.orgID,
		UserID:                   localUID,
		ExternalUserID:           externalUID,
		DepartmentID:             deptID,
		DepartmentName:           row.DepartmentName,
		DepartmentLevel:          row.DepartmentLevel,
		FirstLevelDepartmentID:   row.FirstLevelDepartmentID,
		FirstLevelDepartmentName: row.FirstLevelDepartmentName,
		Title:                    row.Title,
		WorkPlace:                row.WorkPlace,
		IsActive:                 true,
		SourceUpdatedAt:          row.DBUpdateTime,
		LastSeenSnapshotID:       snapshotID,
		LastSeenAt:               &now,
	}
	if err := s.local.UpsertDepartmentRelation(rel); err != nil {
		raw.SyncStatus = "failed"
		raw.ApplyError = truncateErr(err)
		_, _, _, _ = s.local.UpsertDepartmentRaw(raw)
		return err
	}
	raw.SyncStatus = "applied"
	raw.ApplyError = ""
	_, _, _, _ = s.local.UpsertDepartmentRaw(raw)
	return nil
}

func buildAttendanceSourceRowKey(orgID string, row repository.ExternalAttendanceRow) string {
	if strings.TrimSpace(row.RecordID) != "" {
		return hashKey(orgID, "record", row.RecordID)
	}
	uct := ""
	if row.UserCheckTime.Valid {
		uct = row.UserCheckTime.Time.UTC().Format(time.RFC3339Nano)
	}
	return hashKey(orgID, "composite",
		row.UserID, row.WorkDate, row.CheckType, uct, row.SourceType, row.PlanID, row.ProcInstID)
}

func buildDepartmentSourceRowKey(orgID, externalUserID, departmentID string) string {
	return hashKey(orgID, "dept", externalUserID, departmentID)
}

func hashKey(parts ...string) string {
	h := sha256.New()
	for i, p := range parts {
		if i > 0 {
			h.Write([]byte{0})
		}
		h.Write([]byte(p))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func normalizeExternalCheckType(v string) string {
	switch strings.TrimSpace(v) {
	case "", "OnDuty", "上班":
		return "上班"
	case "OffDuty", "下班":
		return "下班"
	default:
		return v
	}
}

func stringField(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil {
			switch t := v.(type) {
			case string:
				return t
			case float64:
				return fmt.Sprintf("%.0f", t)
			case json.Number:
				return t.String()
			default:
				b, _ := json.Marshal(t)
				return strings.Trim(string(b), `"`)
			}
		}
	}
	return ""
}

func parseFlexibleTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"2006-01-02",
		"2006/01/02 15:04:05",
	}
	cst := time.FixedZone("CST", 8*3600)
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, s, cst); err == nil {
			return &t
		}
	}
	return nil
}

func sanitizeExternalErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "password") {
		return "external source error (details redacted)"
	}
	if strings.Contains(msg, "@tcp(") {
		return "external source connection error"
	}
	return truncateErr(err)
}

func truncateErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 400 {
		return msg[:400]
	}
	return msg
}

func defaultStr(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}
