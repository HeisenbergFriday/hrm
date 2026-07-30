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

func TestOrgServiceBaseEmployeeQueryIncludesSecondaryDepartmentMemberships(t *testing.T) {
	db := newDryRunServiceDB(t, "muteng")
	svc := NewOrgService(db)
	var users []database.User
	stmt := svc.baseEmployeeQuery([]string{"dept-secondary"}).Select("users.*").Find(&users).Statement
	sql := strings.ToLower(stmt.SQL.String())
	if !strings.Contains(sql, "user_department_memberships") || !strings.Contains(sql, "exists") {
		t.Fatalf("sql = %s, want secondary department membership EXISTS query", stmt.SQL.String())
	}
}

func TestRollupTreeUniqueCountsDeduplicatesMultiDepartmentEmployee(t *testing.T) {
	root := &OrgDepartmentTreeNode{ID: "root"}
	left := &OrgDepartmentTreeNode{ID: "left"}
	right := &OrgDepartmentTreeNode{ID: "right"}
	root.Children = []*OrgDepartmentTreeNode{left, right}
	directUsers := make(map[string]*departmentUserSets)
	ensureDepartmentUserSets(directUsers, "left").add("employee-1", "active")
	ensureDepartmentUserSets(directUsers, "right").add("employee-1", "active")
	ensureDepartmentUserSets(directUsers, "right").add("employee-2", "inactive")

	rollupTreeUniqueCounts(root, directUsers)

	if left.ActiveCount != 1 || left.Headcount != 1 {
		t.Fatalf("left counts = active:%d total:%d", left.ActiveCount, left.Headcount)
	}
	if right.ActiveCount != 1 || right.InactiveCount != 1 || right.Headcount != 2 {
		t.Fatalf("right counts = active:%d inactive:%d total:%d", right.ActiveCount, right.InactiveCount, right.Headcount)
	}
	if root.ActiveCount != 1 || root.InactiveCount != 1 || root.Headcount != 2 {
		t.Fatalf("root counts = active:%d inactive:%d total:%d, want unique 1/1/2", root.ActiveCount, root.InactiveCount, root.Headcount)
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
