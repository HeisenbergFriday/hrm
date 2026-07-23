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

func newDryRunServiceDB(t *testing.T, orgID string) *gorm.DB {
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
		t.Fatalf("open dry-run db: %v", err)
	}
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: orgID})
	return db.WithContext(ctx)
}

func TestOrgServiceBaseEmployeeQueryScopesJoinedProfilesByOrg(t *testing.T) {
	db := newDryRunServiceDB(t, "muteng")
	svc := NewOrgService(db)

	var users []database.User
	stmt := svc.baseEmployeeQuery(nil).Select("users.*").Find(&users).Statement
	sql := stmt.SQL.String()

	if !strings.Contains(sql, "employee_profiles.org_id = ?") {
		t.Fatalf("sql = %s, want employee_profiles org scope in join", sql)
	}
	if len(stmt.Vars) == 0 || stmt.Vars[0] != "muteng" {
		t.Fatalf("vars = %#v, want muteng as join org var", stmt.Vars)
	}
}

func TestOrgServiceBuildEmployeeListQuerySelfScopesOrgStatusAndSearch(t *testing.T) {
	db := newDryRunServiceDB(t, "org-a")
	svc := NewOrgServiceWithOrgID(db, "org-a")
	scope := &OrgDataScope{Mode: "self", UserIDs: []string{" shared-user "}}

	query, err := svc.buildEmployeeListQuery(scope, OrgEmployeeFilters{
		Status: " active ",
		Search: " Alice ",
	})
	if err != nil {
		t.Fatalf("buildEmployeeListQuery() error = %v", err)
	}

	var users []database.User
	stmt := query.Select("users.*").Find(&users).Statement
	sql := strings.ToLower(stmt.SQL.String())
	for _, fragment := range []string{"org_id", "user_id", "status", " like "} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("sql = %s, want fragment %q", stmt.SQL.String(), fragment)
		}
	}

	wants := []any{"org-a", "shared-user", "active", "%Alice%"}
	for _, want := range wants {
		found := false
		for _, got := range stmt.Vars {
			if got == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("vars = %#v, want %q", stmt.Vars, want)
		}
	}
}
