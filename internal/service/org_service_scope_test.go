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
