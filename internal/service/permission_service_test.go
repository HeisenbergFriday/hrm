package service

import (
	stdsql "database/sql"
	"database/sql/driver"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func newStubPermissionService(t *testing.T, queries ...stubQueryResponse) *PermissionService {
	t.Helper()
	stubPerformanceDriverOnce.Do(func() {
		stdsql.Register(stubPerformanceDriverName, stubPerformanceDriver{})
	})

	dsn := fmt.Sprintf("permission-%s-%d", t.Name(), time.Now().UnixNano())
	stubPerformanceDBs.Store(dsn, &stubPerformanceDB{queries: queries})
	t.Cleanup(func() {
		stubPerformanceDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(stubPerformanceDriverName, dsn)
	if err != nil {
		t.Fatalf("open stub sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open stub gorm db: %v", err)
	}
	return NewPermissionService(db)
}

func TestResolveUserScopeExpandsRoleAndManagedDepartments(t *testing.T) {
	svc := newStubPermissionService(t,
		stubQueryResponse{
			match:   stubTableMatcher("users"),
			columns: []string{"id", "user_id", "name", "department_id", "status"},
			rows:    [][]driver.Value{{int64(1), "manager-1", "Manager", "dept-a", "active"}},
		},
		stubQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				return strings.Contains(lower, "roles") && strings.Contains(lower, "user_roles")
			},
			columns: []string{"id", "name", "description"},
			rows:    [][]driver.Value{{int64(1), "department-role", "department data scope"}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("data_permissions"),
			columns: []string{"id", "role_id", "scope", "department_keys"},
			rows:    [][]driver.Value{{int64(1), int64(1), "department", `["dept-a"]`}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("departments"),
			columns: []string{"id", "department_id", "name", "parent_id", "extension"},
			rows: [][]driver.Value{
				{int64(1), "dept-a", "Dept A", "", []byte(`{}`)},
				{int64(2), "dept-a-child", "Dept A Child", "dept-a", []byte(`{}`)},
				{int64(3), "dept-b", "Dept B", "", []byte(`{"department_head_user_id":"manager-1"}`)},
				{int64(4), "dept-b-child", "Dept B Child", "dept-b", []byte(`{}`)},
				{int64(5), "dept-c", "Dept C", "", []byte(`{"department_head_user_id":"other"}`)},
			},
		},
	)

	scope, err := svc.ResolveUserScope("manager-1")
	if err != nil {
		t.Fatalf("ResolveUserScope() error = %v", err)
	}
	if scope == nil || scope.Mode != "department" {
		t.Fatalf("ResolveUserScope() scope = %#v, want department scope", scope)
	}
	wantRoots := []string{"dept-a", "dept-b"}
	if !reflect.DeepEqual(scope.RootDepartmentIDs, wantRoots) {
		t.Fatalf("RootDepartmentIDs = %#v, want %#v", scope.RootDepartmentIDs, wantRoots)
	}
	wantDepartments := []string{"dept-a", "dept-a-child", "dept-b", "dept-b-child"}
	if !reflect.DeepEqual(scope.DepartmentIDs, wantDepartments) {
		t.Fatalf("DepartmentIDs = %#v, want %#v", scope.DepartmentIDs, wantDepartments)
	}
}
