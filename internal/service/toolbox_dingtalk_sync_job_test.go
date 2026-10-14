package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func toolboxSyncJobTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "tasks.db")), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&database.ApprovalSyncTask{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func toolboxSyncTestRequest() DingtalkSyncRequest {
	padding := 31
	return DingtalkSyncRequest{StartDate: "2026-08-01", EndDate: "2026-09-30", FlowKeys: []string{"leave"}, PaddingDays: &padding}
}

func TestToolboxSyncJobPersistenceIsolationAndClientCancellation(t *testing.T) {
	db := toolboxSyncJobTestDB(t)
	s := NewToolboxDingtalkSyncJobService(db, "org-a")
	executions := 0
	s.execute = func(ctx context.Context, userID, orgID, module string, config map[string]interface{}) (*AttendanceToolboxRun, error) {
		executions++
		if ctx.Err() != nil || userID != "user-a" || orgID != "org-a" || module != "dingtalk_sync" || config["dingtalk_sync_padding_days"] != 31 {
			t.Fatal("wrong execution context")
		}
		return &AttendanceToolboxRun{RunID: "stored-run"}, nil
	}
	parent, cancel := context.WithCancel(context.Background())
	job, err := s.Start(parent, "user-a", toolboxSyncTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	if executions != 0 || job.Status != "running" {
		t.Fatal("start executed external work")
	}
	if _, err := s.Start(parent, "user-b", toolboxSyncTestRequest()); !errors.Is(err, repository.ErrApprovalSyncTaskRunning) {
		t.Fatalf("duplicate = %v", err)
	}
	transfer := toolboxSyncTestRequest()
	transfer.FlowKeys = []string{"position_transfer"}
	if _, err := s.Start(parent, "user-a", transfer); err != nil {
		t.Fatalf("unrelated auto pull blocked: %v", err)
	}
	other := NewToolboxDingtalkSyncJobService(db, "org-b")
	if _, err := other.Start(parent, "user-a", toolboxSyncTestRequest()); err != nil {
		t.Fatal(err)
	}
	if _, err := other.Get(parent, "user-a", job.RequestID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-org = %v", err)
	}
	if _, err := s.Get(parent, "user-b", job.RequestID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user = %v", err)
	}
	cancel()
	s.Run(parent, "user-a", job.RequestID, toolboxSyncTestRequest())
	// A fresh service must read persisted state, not a process-local job map.
	reloaded, err := NewToolboxDingtalkSyncJobService(db, "org-a").Get(context.Background(), "user-a", job.RequestID)
	if err != nil || reloaded.Status != "success" || reloaded.RunID != "stored-run" || executions != 1 {
		t.Fatalf("terminal = %#v, %v", reloaded, err)
	}
	if _, err := s.Start(context.Background(), "user-a", toolboxSyncTestRequest()); err != nil {
		t.Fatalf("lock not released: %v", err)
	}
}

func TestToolboxSyncJobFailureTimeoutAndPanicPersistSafeTerminal(t *testing.T) {
	for _, mode := range []string{"failure", "timeout", "panic"} {
		t.Run(mode, func(t *testing.T) {
			db := toolboxSyncJobTestDB(t)
			s := NewToolboxDingtalkSyncJobService(db, "org-a")
			s.toolbox.timeout = 10 * time.Millisecond
			s.execute = func(ctx context.Context, _, _, _ string, _ map[string]interface{}) (*AttendanceToolboxRun, error) {
				if mode == "panic" {
					panic("sensitive upstream detail")
				}
				if mode == "timeout" {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				return nil, errors.New("sensitive upstream detail")
			}
			job, err := s.Start(context.Background(), "user-a", toolboxSyncTestRequest())
			if err != nil {
				t.Fatal(err)
			}
			s.Run(context.Background(), "user-a", job.RequestID, toolboxSyncTestRequest())
			result, err := s.Get(context.Background(), "user-a", job.RequestID)
			if err != nil || result.Status != "failed" || result.RunID != "" || strings.Contains(result.Message, "sensitive") {
				t.Fatalf("terminal = %#v, %v", result, err)
			}
			var task database.ApprovalSyncTask
			if err := db.Where("request_id = ?", job.RequestID).First(&task).Error; err != nil {
				t.Fatal(err)
			}
			if task.ActiveKey != nil || task.FinishedAt == nil {
				t.Fatal("terminal state did not release lock")
			}
			if mode == "timeout" && task.ErrorCode != "TOOLBOX_SYNC_TIMEOUT" {
				t.Fatalf("timeout code = %s", task.ErrorCode)
			}
		})
	}
}

func TestToolboxSyncJobStaleRecoveryRejectsLateCompletion(t *testing.T) {
	db := toolboxSyncJobTestDB(t)
	s := NewToolboxDingtalkSyncJobService(db, "org-a")
	job, err := s.Start(context.Background(), "user-a", toolboxSyncTestRequest())
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&database.ApprovalSyncTask{}).Where("request_id = ?", job.RequestID).Update("heartbeat_at", time.Now().Add(-time.Hour)).Error; err != nil {
		t.Fatal(err)
	}
	result, err := s.Get(context.Background(), "user-a", job.RequestID)
	if err != nil || result.Status != "failed" {
		t.Fatalf("stale = %#v, %v", result, err)
	}
	s.execute = func(context.Context, string, string, string, map[string]interface{}) (*AttendanceToolboxRun, error) {
		return &AttendanceToolboxRun{RunID: "late-run"}, nil
	}
	s.Run(context.Background(), "user-a", job.RequestID, toolboxSyncTestRequest())
	result, err = s.Get(context.Background(), "user-a", job.RequestID)
	if err != nil || result.Status != "failed" || result.RunID != "" {
		t.Fatalf("resurrected stale task = %#v, %v", result, err)
	}
	if _, err := s.Start(context.Background(), "user-a", toolboxSyncTestRequest()); err != nil {
		t.Fatal(err)
	}
}

func TestToolboxSyncJobInvalidInputsNeverCreateTask(t *testing.T) {
	db := toolboxSyncJobTestDB(t)
	s := NewToolboxDingtalkSyncJobService(db, "org-a")
	for _, req := range []DingtalkSyncRequest{
		{StartDate: "bad", EndDate: "2026-09-30"},
		{StartDate: "2026-10-01", EndDate: "2026-09-30"},
		{StartDate: "2026-08-01", EndDate: "2026-09-30", FlowKeys: []string{"unknown"}},
	} {
		if _, err := s.Start(context.Background(), "user-a", req); !errors.Is(err, ErrToolboxSyncInvalidRequest) {
			t.Fatalf("invalid = %v", err)
		}
	}
	var count int64
	if err := db.Model(&database.ApprovalSyncTask{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("task count = %d, %v", count, err)
	}
}
