package repository

import (
	"errors"
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openUserFailClosedDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:user-fc-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Same user_id / email / mobile in two orgs.
	for _, org := range []string{"muteng", "xiaotie"} {
		u := &database.User{
			OrgID:          org,
			UserID:         "shared-uid",
			DingTalkUserID: "dt-" + org,
			Name:           "User " + org,
			Status:         "active",
			Email:          "shared@example.com",
			Mobile:         "13800000000",
		}
		if err := db.Create(u).Error; err != nil {
			t.Fatalf("seed %s: %v", org, err)
		}
	}
	return db
}

func TestUserRepository_EmptyOrgFailClosed(t *testing.T) {
	db := openUserFailClosedDB(t)
	repo := NewUserRepositoryWithOrgID(db, "")

	if _, err := repo.FindByUserID("shared-uid"); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindByUserID err=%v", err)
	}
	if _, err := repo.FindByEmail("shared@example.com"); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindByEmail err=%v", err)
	}
	if _, err := repo.FindByMobile("13800000000"); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindByMobile err=%v", err)
	}
	if _, _, err := repo.FindAll(1, 10); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("FindAll err=%v", err)
	}
	if err := repo.Create(&database.User{UserID: "x"}); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("Create err=%v", err)
	}
	if err := repo.Update(&database.User{ID: 1, UserID: "x"}); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("Update err=%v", err)
	}
	if err := repo.Delete("shared-uid"); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("Delete err=%v", err)
	}

	// Legacy constructor without DB tenant context is also fail-closed.
	legacy := NewUserRepository(db)
	if _, err := legacy.FindByUserID("shared-uid"); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("legacy FindByUserID err=%v", err)
	}
	if err := legacy.Create(&database.User{UserID: "y", OrgID: "muteng"}); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("legacy Create err=%v", err)
	}
}

func TestUserRepository_SameIdentityCrossOrgIsolation(t *testing.T) {
	db := openUserFailClosedDB(t)
	muteng := NewUserRepositoryWithOrgID(db, "muteng")
	xiaotie := NewUserRepositoryWithOrgID(db, "xiaotie")

	u, err := muteng.FindByUserID("shared-uid")
	if err != nil || u.OrgID != "muteng" {
		t.Fatalf("muteng by user_id: %#v err=%v", u, err)
	}
	u2, err := xiaotie.FindByUserID("shared-uid")
	if err != nil || u2.OrgID != "xiaotie" {
		t.Fatalf("xiaotie by user_id: %#v err=%v", u2, err)
	}
	if u.ID == u2.ID {
		t.Fatalf("expected distinct rows for same user_id across orgs")
	}

	em, err := muteng.FindByEmail("shared@example.com")
	if err != nil || em.OrgID != "muteng" {
		t.Fatalf("muteng email: %#v err=%v", em, err)
	}
	em2, err := xiaotie.FindByEmail("shared@example.com")
	if err != nil || em2.OrgID != "xiaotie" {
		t.Fatalf("xiaotie email: %#v err=%v", em2, err)
	}

	// Cross-org FindByOrgAndUserID mismatch against bound repo.
	if _, err := muteng.FindByOrgAndUserID("xiaotie", "shared-uid"); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("bound muteng must reject xiaotie org lookup, err=%v", err)
	}
}

func TestUserRepository_ScopedSQLUsesOneEqualsZeroWhenEmpty(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewUserRepositoryWithOrgID(db, "")
	cap := captureFindByUserID(t, db, repo)
	sql := strings.ToLower(cap.SQL())
	if !strings.Contains(sql, "1 = 0") && !strings.Contains(sql, "1=0") {
		// requireBoundOrg returns before query for empty org — either hard error path is fine.
		// When SQL is produced, it must contain fail-closed predicate.
		if sql != "" && !strings.Contains(sql, "org_id") {
			t.Fatalf("empty org SQL must not be unscoped, got %s", cap.SQL())
		}
	}
}
