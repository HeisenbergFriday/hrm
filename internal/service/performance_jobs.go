package service

import (
	"log"
	"os"
	"strconv"
	"time"

	"gorm.io/gorm"
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

func (s *PerformanceJobScheduler) RunSelfEvalReminderOnce(now time.Time) {
	result, err := NewPerformanceService(s.db).SendDueSelfEvalAutoReminders(now)
	if err != nil {
		log.Printf("[PerformanceJobs] 自评自动提醒失败: %v", err)
		return
	}
	log.Printf(
		"[PerformanceJobs] 自评自动提醒完成: scanned=%d matched=%d candidates=%d sent=%d skipped=%d already_sent=%d failed=%d",
		result.ActivitiesScanned,
		result.ActivitiesMatched,
		result.Candidates,
		result.Sent,
		result.Skipped,
		result.AlreadySent,
		result.Failed,
	)
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
