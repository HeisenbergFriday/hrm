package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

const (
	approvalAutomaticSyncType             = "approvals"
	approvalAutomaticSyncDefaultInterval  = 15 * time.Minute
	approvalAutomaticSyncDefaultLookback  = 48 * time.Hour
	approvalAutomaticSyncDefaultDelay     = 30 * time.Second
	approvalAutomaticSyncExecutionTimeout = 10 * time.Minute
	approvalAutomaticSyncStaleAfter       = 16 * time.Minute
)

type approvalAutomaticSyncRunner interface {
	Prepare(ApprovalSyncInput, time.Time) (ApprovalSyncPlan, error)
	Run(context.Context, ApprovalSyncPlan, string) ApprovalSyncResult
}

type approvalAutomaticSyncTaskStore interface {
	Acquire(*database.ApprovalSyncTask, time.Time, time.Time) (*database.ApprovalSyncTask, error)
	Complete(*database.ApprovalSyncTask) error
}

// ApprovalSyncJobScheduler periodically reconciles a short recent window.
// DingTalk Stream remains the real-time path; this job only repairs missed or
// delayed notifications and shares the manual-sync database lock.
type ApprovalSyncJobScheduler struct {
	db               *gorm.DB
	enabled          bool
	interval         time.Duration
	lookback         time.Duration
	initialDelay     time.Duration
	executionTimeout time.Duration
	staleAfter       time.Duration
	now              func() time.Time
	listOrgIDs       func() ([]string, error)
	newRunner        func(string) approvalAutomaticSyncRunner
	newTaskStore     func(string) approvalAutomaticSyncTaskStore
}

func NewApprovalSyncJobScheduler(db *gorm.DB) *ApprovalSyncJobScheduler {
	s := &ApprovalSyncJobScheduler{
		db:               db,
		enabled:          approvalAutomaticSyncEnabled(),
		interval:         approvalAutomaticSyncInterval(),
		lookback:         approvalAutomaticSyncLookback(),
		initialDelay:     approvalAutomaticSyncDefaultDelay,
		executionTimeout: approvalAutomaticSyncExecutionTimeout,
		staleAfter:       approvalAutomaticSyncStaleAfter,
		now:              ApprovalSyncNow,
	}
	s.listOrgIDs = s.activeOrganizationIDs
	s.newRunner = func(orgID string) approvalAutomaticSyncRunner {
		return NewApprovalSyncService(db, orgID)
	}
	s.newTaskStore = func(orgID string) approvalAutomaticSyncTaskStore {
		return repository.NewApprovalSyncTaskRepositoryWithOrgID(db, orgID)
	}
	return s
}

func (s *ApprovalSyncJobScheduler) Start() {
	if s == nil || s.db == nil {
		return
	}
	if !s.enabled {
		log.Println("[ApprovalSyncJobs] disabled (APPROVAL_AUTO_SYNC_ENABLED=false)")
		return
	}
	go s.loop()
	log.Printf("[ApprovalSyncJobs] started interval=%s lookback=%s", s.interval, s.lookback)
}

func (s *ApprovalSyncJobScheduler) loop() {
	timer := time.NewTimer(s.initialDelay)
	defer timer.Stop()
	<-timer.C

	for {
		s.RunOnce()
		time.Sleep(s.interval)
	}
}

// RunOnce executes one bounded compensation pass. It is exported for focused
// tests and operational verification; normal callers should use Start.
func (s *ApprovalSyncJobScheduler) RunOnce() {
	if s == nil || !s.enabled || s.listOrgIDs == nil {
		return
	}
	orgIDs, err := s.listOrgIDs()
	if err != nil {
		log.Printf("[ApprovalSyncJobs] stage=list_orgs result=failed error_type=%T", err)
		return
	}
	for _, orgID := range orgIDs {
		orgID = strings.TrimSpace(orgID)
		if orgID == "" {
			continue
		}
		s.runOrganization(orgID)
	}
}

func (s *ApprovalSyncJobScheduler) runOrganization(orgID string) {
	now := s.now().In(ApprovalSyncLocation())
	runner := s.newRunner(orgID)
	plan, err := runner.Prepare(ApprovalSyncInput{
		StartDate: now.Add(-s.lookback).Format("2006-01-02"),
		EndDate:   now.Format("2006-01-02"),
	}, now)
	if err != nil {
		if errors.Is(err, ErrApprovalProcessCodesMissing) {
			log.Printf("[ApprovalSyncJobs] org=%s result=skipped reason=process_codes_missing", orgID)
			return
		}
		log.Printf("[ApprovalSyncJobs] org=%s stage=prepare result=failed error_type=%T", orgID, err)
		return
	}

	requestID := fmt.Sprintf("approval-auto-%d", now.UnixNano())
	store := s.newTaskStore(orgID)
	running := NewRunningApprovalSyncTask(orgID, approvalAutomaticSyncType, requestID, "审批数据正在自动补偿同步", plan, now)
	conflict, err := store.Acquire(running, now.Add(-s.staleAfter), now)
	if errors.Is(err, repository.ErrApprovalSyncTaskRunning) {
		conflictID := ""
		if conflict != nil {
			conflictID = conflict.RequestID
		}
		log.Printf("[ApprovalSyncJobs] org=%s result=skipped reason=sync_running request_id=%s", orgID, conflictID)
		return
	}
	if err != nil {
		log.Printf("[ApprovalSyncJobs] org=%s stage=acquire result=failed error_type=%T", orgID, err)
		return
	}

	startedAt := time.Now()
	completed := false
	defer func() {
		if recovered := recover(); recovered != nil {
			result := ApprovalSyncResult{
				Status:             ApprovalSyncStatusFailed,
				ProcessCount:       len(plan.ProcessCodes),
				FailedProcesses:    len(plan.ProcessCodes),
				StartDate:          plan.StartDate,
				EndDate:            plan.EndDate,
				RequestID:          requestID,
				SyncTime:           s.now().In(ApprovalSyncLocation()).Format(time.RFC3339),
				DurationMS:         time.Since(startedAt).Milliseconds(),
				DiscoveryErrorCode: ApprovalSyncErrorInternal,
				DiscoveryError:     "审批自动同步异常终止，请等待下一轮重试",
			}
			finishedAt := s.now().In(ApprovalSyncLocation())
			if completeErr := store.Complete(CompletedApprovalSyncTask(
				approvalAutomaticSyncType, result, ApprovalSyncResultMessage(result), ApprovalSyncErrorInternal, finishedAt,
			)); completeErr != nil {
				log.Printf("[ApprovalSyncJobs] org=%s stage=panic_finalize result=failed error_type=%T", orgID, completeErr)
			}
			log.Printf("[ApprovalSyncJobs] org=%s result=panic panic_type=%T", orgID, recovered)
			return
		}
		if !completed {
			log.Printf("[ApprovalSyncJobs] org=%s result=unfinished", orgID)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), s.executionTimeout)
	defer cancel()
	result := runner.Run(ctx, plan, requestID)
	cancel()
	result.RequestID = requestID
	finishedAt := s.now().In(ApprovalSyncLocation())
	if err := store.Complete(CompletedApprovalSyncTask(
		approvalAutomaticSyncType,
		result,
		ApprovalSyncResultMessage(result),
		ApprovalSyncResultErrorCode(result),
		finishedAt,
	)); err != nil {
		log.Printf("[ApprovalSyncJobs] org=%s stage=finalize result=failed error_type=%T", orgID, err)
		return
	}
	completed = true
	log.Printf(
		"[ApprovalSyncJobs] org=%s result=%s fetched=%d written=%d failed=%d reconciled=%d",
		orgID,
		result.Status,
		result.FetchedCount,
		result.SuccessCount,
		result.FailCount+result.FetchFailCount+result.ReconcileFailCount,
		result.ReconciledCount,
	)
}

func (s *ApprovalSyncJobScheduler) activeOrganizationIDs() ([]string, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("approval sync jobs: database is required")
	}
	var orgs []database.Organization
	if err := s.db.Select("org_id").Where("status = ?", "active").Order("id ASC").Find(&orgs).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(orgs))
	for _, org := range orgs {
		if id := strings.TrimSpace(org.OrgID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func approvalAutomaticSyncEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("APPROVAL_AUTO_SYNC_ENABLED"))
	if raw == "" {
		return true
	}
	enabled, err := strconv.ParseBool(raw)
	if err != nil {
		return true
	}
	return enabled
}

func approvalAutomaticSyncInterval() time.Duration {
	return approvalAutomaticSyncDuration("APPROVAL_AUTO_SYNC_INTERVAL", approvalAutomaticSyncDefaultInterval, time.Minute, 24*time.Hour)
}

func approvalAutomaticSyncLookback() time.Duration {
	return approvalAutomaticSyncDuration("APPROVAL_AUTO_SYNC_LOOKBACK", approvalAutomaticSyncDefaultLookback, 24*time.Hour, 30*24*time.Hour)
}

func approvalAutomaticSyncDuration(name string, fallback, min, max time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value < min || value > max {
		return fallback
	}
	return value
}
