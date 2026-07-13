package repository

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

// TestUserRepository_ScopedQueriesCarryOrgAcrossThreeTenants 验证三个组织下
// 各类只读查询生成的 SQL 都包含 org 过滤，且参数携带正确的 org 值。
func TestUserRepository_ScopedQueriesCarryOrgAcrossThreeTenants(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := newDryRunGORM(t)
			repo := NewUserRepositoryWithOrgID(db, org)

			// FindByUserID
			assertStmtCarriesOrg(t, "FindByUserID", captureFindByUserID(t, db, repo), org, "org_id = ?")
			// FindByEmail
			assertStmtCarriesOrg(t, "FindByEmail", captureFindByEmail(t, db, repo), org, "org_id = ?")
			// FindByMobile
			assertStmtCarriesOrg(t, "FindByMobile", captureFindByMobile(t, db, repo), org, "org_id = ?")
			// FindByID
			assertStmtCarriesOrg(t, "FindByID", captureFindByID(t, db, repo), org, "org_id = ?")
			// Delete
			assertStmtCarriesOrg(t, "Delete", captureDelete(t, db, repo), org, "org_id = ?")
		})
	}
}

// TestUserRepository_CreateEnforcesTenantOrg 验证 Create 会：
// - 拒绝 user.OrgID 与仓储绑定组织不一致
// - 空 user.OrgID 时补齐为仓储组织
func TestUserRepository_CreateEnforcesTenantOrg(t *testing.T) {
	db := newDryRunGORM(t)

	t.Run("rejects cross-org write", func(t *testing.T) {
		repo := NewUserRepositoryWithOrgID(db, "muteng")
		err := repo.Create(&database.User{UserID: "alice", OrgID: "xiaotie"})
		if !errors.Is(err, ErrOrgMismatch) {
			t.Fatalf("err = %v, want ErrOrgMismatch", err)
		}
	})

	t.Run("inherits tenant org when user.OrgID empty", func(t *testing.T) {
		repo := NewUserRepositoryWithOrgID(db, "default")
		user := &database.User{UserID: "alice"}
		// DryRun 下不会真正插入，但 EnsureSameOrg 会先执行并回填 user.OrgID。
		_ = repo.Create(user) // DryRun 返回的 err 可忽略
		if user.OrgID != "default" {
			t.Fatalf("user.OrgID = %q, want default", user.OrgID)
		}
	})
}

// TestUserRepository_UpdateEnforcesTenantOrg 与 Create 类似，验证 Update 的 org 一致性。
func TestUserRepository_UpdateEnforcesTenantOrg(t *testing.T) {
	db := newDryRunGORM(t)

	t.Run("rejects cross-org update", func(t *testing.T) {
		repo := NewUserRepositoryWithOrgID(db, "xiaotie")
		err := repo.Update(&database.User{ID: 1, UserID: "alice", OrgID: "muteng"})
		if !errors.Is(err, ErrOrgMismatch) {
			t.Fatalf("err = %v, want ErrOrgMismatch", err)
		}
	})

	t.Run("inherits tenant org when user.OrgID empty", func(t *testing.T) {
		repo := NewUserRepositoryWithOrgID(db, "muteng")
		user := &database.User{ID: 2, UserID: "bob"}
		_ = repo.Update(user)
		if user.OrgID != "muteng" {
			t.Fatalf("user.OrgID = %q, want muteng", user.OrgID)
		}
	})
}

// TestUserRepository_LegacyEmptyOrgStillPermits 验证保留旧构造（迁移期兼容）：
// orgID 为空时读路径不加过滤，写路径不做校验。用于确认不会误伤后台任务/工具场景。
func TestUserRepository_LegacyEmptyOrgStillPermits(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewUserRepository(db)

	cap := captureFindByUserID(t, db, repo)
	if strings.Contains(strings.ToLower(cap.SQL()), "org_id = ?") {
		t.Fatalf("legacy constructor should not attach org filter, got %s", cap.SQL())
	}

	// 写路径不校验 user.OrgID，直接透传给 GORM。
	err := repo.Create(&database.User{UserID: "alice", OrgID: "any"})
	// DryRun 会导致 db.Create 返回错误（无法执行）；此处只关心不会被 EnsureSameOrg 拦下。
	if errors.Is(err, ErrOrgMismatch) || errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("legacy constructor should not enforce tenant, got %v", err)
	}
}

// ==================== helpers ====================

// captured 记录 GORM callback 里生成的最终 SQL 和参数。DryRun 下 session.Statement
// 会被 Find/First 内部替换，所以在 callback 里主动拷贝。
type captured struct {
	sql  string
	vars []interface{}
}

func (c *captured) SQL() string          { return c.sql }
func (c *captured) Vars() []interface{}  { return c.vars }

var captureCallbackSeq atomic.Int64

func captureQuery(t *testing.T, db *gorm.DB, repo *UserRepository, do func(*UserRepository)) *captured {
	t.Helper()
	cap := &captured{}
	session := db.Session(&gorm.Session{DryRun: true, NewDB: false})
	snapshot := func(tx *gorm.DB) {
		cap.sql = tx.Statement.SQL.String()
		cap.vars = append([]interface{}{}, tx.Statement.Vars...)
	}
	seq := captureCallbackSeq.Add(1)
	_ = session.Callback().Query().After("gorm:query").
		Register(fmt.Sprintf("tenant-test:query:capture:%d", seq), snapshot)
	_ = session.Callback().Row().After("gorm:row").
		Register(fmt.Sprintf("tenant-test:row:capture:%d", seq), snapshot)
	_ = session.Callback().Delete().After("gorm:delete").
		Register(fmt.Sprintf("tenant-test:delete:capture:%d", seq), snapshot)

	old := repo.db
	repo.db = session
	defer func() { repo.db = old }()
	do(repo)
	return cap
}

func captureFindByUserID(t *testing.T, db *gorm.DB, repo *UserRepository) *captured {
	t.Helper()
	return captureQuery(t, db, repo, func(r *UserRepository) { _, _ = r.FindByUserID("alice") })
}

func captureFindByEmail(t *testing.T, db *gorm.DB, repo *UserRepository) *captured {
	t.Helper()
	return captureQuery(t, db, repo, func(r *UserRepository) { _, _ = r.FindByEmail("alice@example.com") })
}

func captureFindByMobile(t *testing.T, db *gorm.DB, repo *UserRepository) *captured {
	t.Helper()
	return captureQuery(t, db, repo, func(r *UserRepository) { _, _ = r.FindByMobile("13800000000") })
}

func captureFindByID(t *testing.T, db *gorm.DB, repo *UserRepository) *captured {
	t.Helper()
	return captureQuery(t, db, repo, func(r *UserRepository) { _, _ = r.FindByID("42") })
}

func captureDelete(t *testing.T, db *gorm.DB, repo *UserRepository) *captured {
	t.Helper()
	return captureQuery(t, db, repo, func(r *UserRepository) { _ = r.Delete("alice") })
}

func assertStmtCarriesOrg(t *testing.T, label string, cap *captured, org, sqlFragment string) {
	t.Helper()
	sql := cap.SQL()
	if !containsIgnoreCase(sql, sqlFragment) {
		t.Fatalf("[%s] expected SQL to contain %q, got %s", label, sqlFragment, sql)
	}
	for _, v := range cap.Vars() {
		if s, ok := v.(string); ok && s == org {
			return
		}
	}
	t.Fatalf("[%s] expected org=%q in Vars %#v", label, org, cap.Vars())
}
