package database

import (
	"context"
	"errors"
	"strings"
	"testing"

	"peopleops/internal/requestmeta"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestInitOperationalDatabaseDoesNotMigrateOrSeed(t *testing.T) {
	originalDB := DB
	var openedDB *gorm.DB
	t.Cleanup(func() {
		if openedDB != nil {
			if sqlDB, err := openedDB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		DB = originalDB
	})

	var capturedConfig *gorm.Config
	err := initOperationalDatabase(func(config *gorm.Config) (*gorm.DB, error) {
		capturedConfig = config
		db, openErr := gorm.Open(sqlite.Open(":memory:"), config)
		openedDB = db
		return db, openErr
	})
	if err != nil {
		t.Fatalf("init operational database: %v", err)
	}
	if capturedConfig == nil || capturedConfig.Logger != logger.Discard {
		t.Fatal("operational database must configure the silent GORM logger before opening")
	}
	if DB == nil {
		t.Fatal("operational database was not installed")
	}
	if DB.Migrator().HasTable(&Organization{}) || DB.Migrator().HasTable(&User{}) {
		t.Fatal("operational database initialization must not migrate or seed tables")
	}

	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: "muteng"})
	var users []User
	statement := DB.WithContext(ctx).Session(&gorm.Session{DryRun: true}).Model(&User{}).Find(&users).Statement
	if !strings.Contains(statement.SQL.String(), "org_id") {
		t.Fatalf("operational database query missing organization callback: %s", statement.SQL.String())
	}
}

func TestInitOperationalDatabaseFailureKeepsExistingHandle(t *testing.T) {
	originalDB := DB
	sentinel := &gorm.DB{}
	DB = sentinel
	t.Cleanup(func() { DB = originalDB })

	err := initOperationalDatabase(func(*gorm.Config) (*gorm.DB, error) {
		return nil, errors.New("connection refused")
	})
	if err == nil {
		t.Fatal("expected connection failure")
	}
	if DB != sentinel {
		t.Fatal("failed operational initialization must not replace the existing database handle")
	}
}

func TestInitOperationalRequiresDatabaseURL(t *testing.T) {
	originalDB := DB
	sentinel := &gorm.DB{}
	DB = sentinel
	t.Cleanup(func() { DB = originalDB })
	t.Setenv("DATABASE_URL", "")

	if err := InitOperational(); err == nil {
		t.Fatal("expected missing DATABASE_URL to fail closed")
	}
	if DB != sentinel {
		t.Fatal("missing DATABASE_URL must not replace the existing database handle")
	}
}
