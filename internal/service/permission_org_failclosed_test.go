package service

import (
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPermissionService_EmptyOrgDoesNotFallbackDefault(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:perm-fail-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&database.Role{}, &database.User{}, &database.UserRole{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := db.Create(&database.Role{OrgID: "default", Name: "admin"}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	svc := NewPermissionService(db) // no tenant context
	if got := svc.effectiveOrgID(""); got == database.DefaultOrganizationID || got == "default" {
		t.Fatalf("effectiveOrgID empty context = %q, must not invent default", got)
	}

	svc2 := NewPermissionServiceWithOrgID(db, "")
	if got := svc2.effectiveOrgID(""); got == "default" {
		t.Fatalf("WithOrgID empty must not fallback default, got %q", got)
	}
}
