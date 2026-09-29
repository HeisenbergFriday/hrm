package repository

import (
	"fmt"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSupplementaryRequestCreateKeepsPendingClockTimesNull(t *testing.T) {
	dsn := fmt.Sprintf("file:supplementary-null-clock-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := db.AutoMigrate(&database.OvertimeSupplementaryRequest{}); err != nil {
		t.Fatalf("migrate supplementary requests: %v", err)
	}

	req := &database.OvertimeSupplementaryRequest{
		OrgID: "org-a", MatchResultID: 1, UserID: "u1", WorkDate: "2026-09-28", Status: "pending",
	}
	if err := NewSupplementaryRequestRepositoryWithOrgID(db, "org-a").Create(req); err != nil {
		t.Fatalf("create pending supplementary request: %v", err)
	}

	var nullClockCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM overtime_supplementary_requests
		WHERE id = ? AND supplementary_clock_in IS NULL AND supplementary_clock_out IS NULL`, req.ID).
		Scan(&nullClockCount).Error; err != nil {
		t.Fatalf("check supplementary clock times: %v", err)
	}
	if nullClockCount != 1 {
		t.Fatalf("NULL clock time count = %d, want 1", nullClockCount)
	}
}
