package database

import (
	"context"
	"strings"
	"testing"

	"peopleops/internal/requestmeta"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newDryRunOrgScopeDB(t *testing.T) *gorm.DB {
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
	registerOrganizationCallbacks(db)
	return db
}

func TestOrganizationScopeAppliesToModelQueries(t *testing.T) {
	db := newDryRunOrgScopeDB(t)
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: "muteng"})

	var users []User
	stmt := db.WithContext(ctx).Model(&User{}).Find(&users).Statement
	sql := stmt.SQL.String()

	if !strings.Contains(sql, "`users`.`org_id` = ?") {
		t.Fatalf("sql = %s, want users org scope", sql)
	}
	if len(stmt.Vars) == 0 || stmt.Vars[len(stmt.Vars)-1] != "muteng" {
		t.Fatalf("vars = %#v, want muteng org scope var", stmt.Vars)
	}
}

func TestOrganizationScopeAppliesToKnownTableQueries(t *testing.T) {
	db := newDryRunOrgScopeDB(t)
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: "muteng"})

	var rows []map[string]interface{}
	stmt := db.WithContext(ctx).Table("users").Select("user_id").Find(&rows).Statement
	sql := stmt.SQL.String()

	if !strings.Contains(sql, "`users`.`org_id` = ?") {
		t.Fatalf("sql = %s, want users org scope for Table query", sql)
	}
	if len(stmt.Vars) == 0 || stmt.Vars[len(stmt.Vars)-1] != "muteng" {
		t.Fatalf("vars = %#v, want muteng org scope var", stmt.Vars)
	}
}

func TestOrganizationScopeFillsOrgIDOnCreate(t *testing.T) {
	db := newDryRunOrgScopeDB(t)
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: "muteng"})

	user := User{
		UserID:       "muteng:u1",
		Name:         "Test User",
		DepartmentID: "muteng:1",
		Status:       "active",
	}
	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		t.Fatalf("create dry-run user: %v", err)
	}
	if user.OrgID != "muteng" {
		t.Fatalf("user.OrgID = %q, want muteng", user.OrgID)
	}
}
