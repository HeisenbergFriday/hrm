package repository

import (
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupWeekScheduleRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	if err := db.AutoMigrate(&database.WeekScheduleRule{}); err != nil {
		t.Fatalf("migrate week schedule rule table: %v", err)
	}

	return db
}

func TestDeleteRuleAllowsRecreateSameScope(t *testing.T) {
	db := setupWeekScheduleRepositoryTestDB(t)
	repo := NewWeekScheduleRepository(db)

	rule := &database.WeekScheduleRule{
		ScopeType: "company",
		ScopeID:   "",
		ScopeName: "全公司",
		BaseDate:  "2026-04-20",
		Pattern:   "big_first",
		Status:    "active",
	}

	if err := repo.CreateRule(rule); err != nil {
		t.Fatalf("create initial rule: %v", err)
	}

	if err := repo.DeleteRule(rule.ID); err != nil {
		t.Fatalf("delete initial rule: %v", err)
	}

	recreated := &database.WeekScheduleRule{
		ScopeType: "company",
		ScopeID:   "",
		ScopeName: "全公司",
		BaseDate:  "2026-04-27",
		Pattern:   "small_first",
		Status:    "active",
	}

	if err := repo.CreateRule(recreated); err != nil {
		t.Fatalf("recreate same scope after delete should succeed: %v", err)
	}
}
