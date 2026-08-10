package service

import (
	"context"
	"log"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

// ExternalAttendanceJobScheduler runs incremental external sync for all mapped orgs.
// Controlled by EXTERNAL_ATTENDANCE_SYNC_ENABLED and EXTERNAL_ATTENDANCE_SYNC_INTERVAL.
type ExternalAttendanceJobScheduler struct {
	db *gorm.DB
}

func NewExternalAttendanceJobScheduler(db *gorm.DB) *ExternalAttendanceJobScheduler {
	return &ExternalAttendanceJobScheduler{db: db}
}

// Start launches the background loop. No-op when disabled or DSN missing.
func (s *ExternalAttendanceJobScheduler) Start() {
	cfg := database.LoadExternalAttendanceConfig()
	if !cfg.Enabled {
		log.Println("[ExternalAttendanceJobs] disabled (EXTERNAL_ATTENDANCE_SYNC_ENABLED=false)")
		return
	}
	if cfg.DSN == "" {
		log.Println("[ExternalAttendanceJobs] skipped: external DSN not configured")
		return
	}
	interval := cfg.SyncInterval
	if interval < time.Minute {
		interval = time.Minute
	}
	go s.loop(interval, cfg)
	log.Printf("[ExternalAttendanceJobs] started interval=%s", interval)
}

func (s *ExternalAttendanceJobScheduler) loop(interval time.Duration, cfg database.ExternalAttendanceConfig) {
	// Initial delay to let service stabilize.
	time.Sleep(30 * time.Second)
	for {
		s.runOnce(cfg)
		time.Sleep(interval)
	}
}

func (s *ExternalAttendanceJobScheduler) runOnce(cfg database.ExternalAttendanceConfig) {
	if err := database.InitExternalAttendanceDB(); err != nil {
		log.Printf("[ExternalAttendanceJobs] init source failed: %v", sanitizeExternalErr(err))
		return
	}
	srcDB := database.GetExternalAttendanceDB()
	if srcDB == nil {
		return
	}
	source := repository.NewExternalAttendanceSourceRepository(srcDB, cfg.QueryTimeout)
	lookback := time.Duration(cfg.LookbackMinutes) * time.Minute

	for _, m := range database.ExternalCorpMappings {
		local := repository.NewExternalAttendanceLocalRepository(s.db, m.OrgID)
		svc := NewExternalAttendanceSyncService(source, local, m.OrgID, lookback, cfg.Enabled)
		attendanceSvc := NewAttendanceServiceWithOrgID(s.db, m.OrgID)
		svc.SetRetryableOvertimeRecalculator(attendanceSvc.RecalculateRetryableOvertime)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		job, err := svc.Run(ctx, ExternalSyncRunOptions{
			Source:         externalSyncSourceAll,
			Trigger:        "cron",
			OperatorUserID: "system:external-attendance-cron",
		})
		cancel()
		if err != nil {
			log.Printf("[ExternalAttendanceJobs] org=%s err=%v", m.OrgID, sanitizeExternalErr(err))
			continue
		}
		if job != nil {
			log.Printf("[ExternalAttendanceJobs] org=%s status=%s ins=%d upd=%d skip=%d fail=%d",
				m.OrgID, job.Status, job.Inserted, job.Updated, job.Skipped, job.Failed)
		}
	}
}
