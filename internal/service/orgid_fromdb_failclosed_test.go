package service

import (
	"context"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/requestmeta"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func openBareDryRunDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.New(mysql.Config{
		DSN:                       "gorm:gorm@tcp(127.0.0.1:9910)/gorm?charset=utf8mb4&parseTime=True&loc=Local",
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		DryRun:                 true,
		DisableAutomaticPing:   true,
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	return db
}

func TestRequireOrgIDFromDB_NoFallbackDefault(t *testing.T) {
	db := openBareDryRunDB(t)
	// No requestmeta / tenant on context.
	if _, err := requireOrgIDFromDB(db); err == nil || !strings.Contains(err.Error(), "missing organization") {
		t.Fatalf("requireOrgIDFromDB empty context err=%v, want missing organization", err)
	}
	// Explicit empty org is still missing.
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: ""})
	db = db.WithContext(ctx)
	if _, err := requireOrgIDFromDB(db); err == nil {
		t.Fatalf("empty RequestInfo.OrgID must fail")
	}
	// Bound org succeeds and is normalized.
	ctx = requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: " muteng "})
	db = db.WithContext(ctx)
	got, err := requireOrgIDFromDB(db)
	if err != nil || got != "muteng" {
		t.Fatalf("got=%q err=%v", got, err)
	}
}

func TestResolveServiceOrgID_PrefersBoundThenDB(t *testing.T) {
	db := openBareDryRunDB(t)
	got, err := resolveServiceOrgID("xiaotie", db)
	if err != nil || got != "xiaotie" {
		t.Fatalf("bound wins: got=%q err=%v", got, err)
	}
	if _, err := resolveServiceOrgID("", db); err == nil {
		t.Fatalf("empty bound + empty db must fail")
	}
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: "muteng"})
	db = db.WithContext(ctx)
	got, err = resolveServiceOrgID("", db)
	if err != nil || got != "muteng" {
		t.Fatalf("db context: got=%q err=%v", got, err)
	}
}

func TestOrgServiceBaseEmployeeQuery_EmptyOrgFailClosed(t *testing.T) {
	db := openBareDryRunDB(t)
	svc := NewOrgService(db) // no org binding, no requestmeta
	var users []database.User
	stmt := svc.baseEmployeeQuery(nil).Select("users.*").Find(&users).Statement
	sql := strings.ToLower(stmt.SQL.String())
	if !strings.Contains(sql, "1 = 0") && !strings.Contains(sql, "1=0") {
		t.Fatalf("empty org baseEmployeeQuery must fail closed, sql=%s", stmt.SQL.String())
	}
}

func TestOrgIDFromDB_IsDeprecatedFallbackOnly(t *testing.T) {
	// Document residual behavior: orgIDFromDB still falls back to default when
	// unbound. Business paths must not call it; this asserts the residual
	// contract so regressions that reintroduce business usage are obvious.
	db := openBareDryRunDB(t)
	if got := orgIDFromDB(db); got != database.DefaultOrganizationID {
		t.Fatalf("deprecated orgIDFromDB unbound = %q, residual contract is default", got)
	}
}
