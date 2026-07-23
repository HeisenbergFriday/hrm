package service

import (
	"errors"
	"fmt"
	"log"
	"os"
	"peopleops/internal/database"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

var (
	ErrNoActivePerformanceOrganizations = errors.New("performance jobs: no active organizations")
	listActivePerformanceOrganizations  = database.ListActiveOrganizations
)

type PerformanceJobScheduler struct {
	db *gorm.DB
}

func NewPerformanceJobScheduler(db *gorm.DB) *PerformanceJobScheduler {
	return &PerformanceJobScheduler{db: db}
}

// Start 启动绩效模块后台任务。
func (s *PerformanceJobScheduler) Start() {
	if s == nil || s.db == nil {
		return
	}
	go s.runSelfEvalReminderJob()
	log.Println("[PerformanceJobs] 定时任务已启动")
}

func (s *PerformanceJobScheduler) runSelfEvalReminderJob() {
	for {
		next := nextDailyAt(performanceSelfEvalReminderHour(), 0)
		log.Printf("[PerformanceJobs] 自评自动提醒将在 %s 执行", next.Format("2006-01-02 15:04:05"))
		time.Sleep(time.Until(next))
		s.RunSelfEvalReminderOnce(time.Now())
	}
}

// RunSelfEvalReminderOnce 逐活跃组织执行自评自动提醒，禁止无 org 的全库扫描。
func (s *PerformanceJobScheduler) RunSelfEvalReminderOnce(now time.Time) {
	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		log.Printf("[PerformanceJobs] 读取组织失败: %v", err)
		return
	}
	total := &AutoSelfEvalReminderResult{}
	for _, orgID := range orgs {
		result, err := NewPerformanceServiceWithOrgID(s.db, orgID).SendDueSelfEvalAutoReminders(now)
		if err != nil {
			log.Printf("[PerformanceJobs] org=%s 自评自动提醒失败: %v", orgID, err)
			continue
		}
		if result == nil {
			continue
		}
		log.Printf(
			"[PerformanceJobs] org=%s 自评自动提醒完成: scanned=%d matched=%d candidates=%d sent=%d skipped=%d already_sent=%d failed=%d",
			orgID,
			result.ActivitiesScanned,
			result.ActivitiesMatched,
			result.Candidates,
			result.Sent,
			result.Skipped,
			result.AlreadySent,
			result.Failed,
		)
		total.ActivitiesScanned += result.ActivitiesScanned
		total.ActivitiesMatched += result.ActivitiesMatched
		total.Candidates += result.Candidates
		total.Sent += result.Sent
		total.Skipped += result.Skipped
		total.AlreadySent += result.AlreadySent
		total.Failed += result.Failed
	}
	log.Printf(
		"[PerformanceJobs] 自评自动提醒汇总 org_count=%d scanned=%d matched=%d candidates=%d sent=%d skipped=%d already_sent=%d failed=%d",
		len(orgs),
		total.ActivitiesScanned,
		total.ActivitiesMatched,
		total.Candidates,
		total.Sent,
		total.Skipped,
		total.AlreadySent,
		total.Failed,
	)
}

// listActiveOrgIDs returns active organizations or an error. Enumeration failures and
// empty results stop the current run; the scheduler must never guess a default tenant.
func (s *PerformanceJobScheduler) listActiveOrgIDs() ([]string, error) {
	orgs, err := listActivePerformanceOrganizations()
	if err != nil {
		return nil, fmt.Errorf("performance jobs: list active organizations: %w", err)
	}
	ids := make([]string, 0, len(orgs))
	for _, org := range orgs {
		id := strings.TrimSpace(org.OrgID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, ErrNoActivePerformanceOrganizations
	}
	return ids, nil
}

func performanceSelfEvalReminderHour() int {
	raw := os.Getenv("PERFORMANCE_SELF_EVAL_REMINDER_HOUR")
	if raw == "" {
		return 9
	}
	hour, err := strconv.Atoi(raw)
	if err != nil || hour < 0 || hour > 23 {
		return 9
	}
	return hour
}

func nextDailyAt(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
