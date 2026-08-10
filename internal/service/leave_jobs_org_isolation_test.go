package service

import (
	"fmt"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openLeaveJobsDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:leave-jobs-%s-%d?mode=memory&cache=shared", t.Name(), time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return db
}

// TestLeaveJobScheduler_BusinessListEmptyWhenDBUninitializedAndSeedFallsBackDefault
// documents the split between ordinary jobs and cold-start seed:
// - listActiveOrgIDs never invents default when org catalog / global DB is unavailable
// - listActiveOrgIDsForSeed may return [default] for SeedDefaultRules only
func TestLeaveJobScheduler_BusinessListEmptyWhenDBUninitializedAndSeedFallsBackDefault(t *testing.T) {
	// Ensure global database.DB is nil so we hit the uninitialized path.
	// Do not restore if it was already non-nil in this process; unit env typically has nil.
	prev := database.DB
	database.DB = nil
	t.Cleanup(func() { database.DB = prev })

	s := &LeaveJobScheduler{db: openLeaveJobsDB(t)}

	business, err := s.listActiveOrgIDs()
	if err != nil {
		t.Fatalf("listActiveOrgIDs err = %v, want nil", err)
	}
	if len(business) != 0 {
		t.Fatalf("business list = %v, want empty when global DB is nil (no default invent)", business)
	}

	seed, err := s.listActiveOrgIDsForSeed()
	if err != nil {
		t.Fatalf("listActiveOrgIDsForSeed: %v", err)
	}
	if len(seed) != 1 || seed[0] != database.DefaultOrganizationID {
		t.Fatalf("seed list = %v, want [%s]", seed, database.DefaultOrganizationID)
	}
}
