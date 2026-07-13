package repository

import (
	stdsql "database/sql"
	"errors"
	"strings"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestRequireOrgID(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{"empty rejected", "", "", ErrMissingOrgID},
		{"whitespace rejected", "   ", "", ErrMissingOrgID},
		{"trims valid", "  muteng  ", "muteng", nil},
		{"passes through", "xiaotie", "xiaotie", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RequireOrgID(tc.input)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("got = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestEnsureSameOrg(t *testing.T) {
	t.Run("inherits tenant when entity empty", func(t *testing.T) {
		got, err := EnsureSameOrg("muteng", "")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "muteng" {
			t.Fatalf("got = %q, want muteng", got)
		}
	})
	t.Run("passes when identical", func(t *testing.T) {
		got, err := EnsureSameOrg("default", "default")
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if got != "default" {
			t.Fatalf("got = %q, want default", got)
		}
	})
	t.Run("rejects cross-org write", func(t *testing.T) {
		_, err := EnsureSameOrg("muteng", "xiaotie")
		if !errors.Is(err, ErrOrgMismatch) {
			t.Fatalf("err = %v, want ErrOrgMismatch", err)
		}
	})
	t.Run("rejects missing tenant", func(t *testing.T) {
		_, err := EnsureSameOrg("  ", "xiaotie")
		if !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("err = %v, want ErrMissingOrgID", err)
		}
	})
}

// TestScopeOrgAttachesWhere 验证 ScopeOrg 在生成 SQL 中会附加 org 过滤。
// 使用 GORM DryRun 避免真实执行；无需外部数据库。
func TestScopeOrgAttachesWhere(t *testing.T) {
	db := newDryRunGORM(t)

	// 三个组织都应生成对应的 WHERE 片段。
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		stmt := db.Session(&gorm.Session{DryRun: true}).
			Scopes(ScopeOrg(org, "org_id")).
			Table("users").
			Where("user_id = ?", "alice").
			Find(&struct{}{}).Statement

		sql := stmt.SQL.String()
		if !containsIgnoreCase(sql, "org_id = ?") {
			t.Fatalf("expected org_id filter in SQL, got %s", sql)
		}
		// 断言 org 值出现在参数列表中。
		found := false
		for _, v := range stmt.Vars {
			if s, ok := v.(string); ok && s == org {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected org=%q in stmt.Vars, got %#v", org, stmt.Vars)
		}
	}

	// 空 org 时不附加过滤（供旧构造迁移期使用）。
	stmt := db.Session(&gorm.Session{DryRun: true}).
		Scopes(ScopeOrg("", "org_id")).
		Table("users").
		Where("user_id = ?", "alice").
		Find(&struct{}{}).Statement
	if containsIgnoreCase(stmt.SQL.String(), "org_id = ?") {
		t.Fatalf("empty orgID should not attach org filter, got %s", stmt.SQL.String())
	}
}

func TestScopeOrgQualifiedColumn(t *testing.T) {
	db := newDryRunGORM(t)
	stmt := db.Session(&gorm.Session{DryRun: true}).
		Scopes(ScopeOrg("muteng", "users.org_id")).
		Table("users").
		Find(&struct{}{}).Statement
	if !containsIgnoreCase(stmt.SQL.String(), "users.org_id = ?") {
		t.Fatalf("expected qualified column filter, got %s", stmt.SQL.String())
	}
}

// ===== helpers =====

func newDryRunGORM(t *testing.T) *gorm.DB {
	t.Helper()
	// 使用 stubPerformanceDB 提供的 driver 只是为了让 gorm.Open 成功；DryRun 不会真正执行。
	sqlDB := openStubSQLForTenantTests(t)
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open gorm: %v", err)
	}
	return db
}

// openStubSQLForTenantTests 借用 performance stub driver 打开一个空 *sql.DB。DryRun 不会真正执行 SQL。
func openStubSQLForTenantTests(t *testing.T) *stdsql.DB {
	t.Helper()
	stubPerformanceDriverOnce.Do(func() {
		stdsql.Register(stubPerformanceDriverName, stubPerformanceDriver{})
	})
	dsn := "tenant-scope-" + t.Name()
	stubPerformanceDBs.Store(dsn, &stubPerformanceDB{})
	t.Cleanup(func() { stubPerformanceDBs.Delete(dsn) })
	sqlDB, err := stdsql.Open(stubPerformanceDriverName, dsn)
	if err != nil {
		t.Fatalf("open sql: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return sqlDB
}

func containsIgnoreCase(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}
