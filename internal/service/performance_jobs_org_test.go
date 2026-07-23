package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/requestmeta"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	stdsql "database/sql"
	"database/sql/driver"
	"fmt"
	"sync"
	"time"
)

func TestNewPerformanceService_ResolvesTenantAndRequestInfoOrg(t *testing.T) {
	db := newDryRunPerformanceDB(t)

	tenantDB := db.WithContext(requestmeta.WithTenant(context.Background(), "muteng"))
	tenantService := NewPerformanceService(tenantDB)
	if tenantService.orgErr != nil {
		t.Fatalf("TenantID-only service error = %v", tenantService.orgErr)
	}
	if tenantService.tenantOrgID() != "muteng" {
		t.Fatalf("tenantOrgID = %q, want muteng", tenantService.tenantOrgID())
	}

	info := &requestmeta.RequestInfo{OrgID: "xiaotie"}
	infoDB := db.WithContext(requestmeta.WithRequestInfo(context.Background(), info))
	requestInfoService := NewPerformanceService(infoDB)
	if requestInfoService.orgErr != nil {
		t.Fatalf("RequestInfo-only service error = %v", requestInfoService.orgErr)
	}
	if requestInfoService.tenantOrgID() != "xiaotie" {
		t.Fatalf("tenantOrgID from RequestInfo = %q, want xiaotie", requestInfoService.tenantOrgID())
	}
}

func TestNewPerformanceServiceWithOrgID_EmptyFailsClosed(t *testing.T) {
	stub := &stubPerformanceDB{}
	db := newStubPerformanceServiceWithDB(t, stub).db
	svc := NewPerformanceServiceWithOrgID(db, "  ")

	_, _, err := svc.ListActivities(1, 10, "", "", "", "", nil)
	if !errors.Is(err, ErrMissingOrgContext) {
		t.Fatalf("ListActivities error = %v, want ErrMissingOrgContext", err)
	}
	assertNoPerformanceSQL(t, stub)
}

func TestNewPerformanceService_FailsClosedWithoutOrgContext(t *testing.T) {
	stub := &stubPerformanceDB{}
	db := newStubPerformanceServiceWithDB(t, stub).db
	svc := NewPerformanceService(db)

	_, _, err := svc.ListActivities(1, 10, "", "", "", "", nil)
	if !errors.Is(err, ErrMissingOrgContext) {
		t.Fatalf("ListActivities error = %v, want ErrMissingOrgContext", err)
	}
	assertNoPerformanceSQL(t, stub)
}

func TestNewPerformanceService_FailsClosedOnConflictingOrgContexts(t *testing.T) {
	stub := &stubPerformanceDB{}
	baseDB := newStubPerformanceServiceWithDB(t, stub).db
	ctx := requestmeta.WithRequestInfo(context.Background(), &requestmeta.RequestInfo{OrgID: "org-b"})
	ctx = requestmeta.WithTenant(ctx, "org-a")
	svc := NewPerformanceService(baseDB.WithContext(ctx))

	_, _, err := svc.ListActivities(1, 10, "", "", "", "", nil)
	if !errors.Is(err, ErrOrgContextMismatch) {
		t.Fatalf("ListActivities error = %v, want ErrOrgContextMismatch", err)
	}
	assertNoPerformanceSQL(t, stub)
}

func TestNewPerformanceServiceWithOrgID_ScopesQueriesByOrg(t *testing.T) {
	db := newDryRunPerformanceDB(t)
	for _, orgID := range []string{"org-a", "org-b"} {
		t.Run(orgID, func(t *testing.T) {
			svc := NewPerformanceServiceWithOrgID(db, orgID)
			var activities []database.PerformanceActivity
			tx := svc.scopedDB().Session(&gorm.Session{DryRun: true}).Find(&activities)
			if tx.Error != nil {
				t.Fatalf("build scoped query: %v", tx.Error)
			}
			sqlText := strings.ToLower(tx.Statement.SQL.String())
			if !strings.Contains(sqlText, "org_id") {
				t.Fatalf("query missing org_id predicate: %s", sqlText)
			}
			if got := fmt.Sprint(tx.Statement.Vars...); !strings.Contains(got, orgID) {
				t.Fatalf("query vars = %q, want %q", got, orgID)
			}
		})
	}
}

// TestSendDueSelfEvalAutoReminders_ScopesByOrg 同一 activity 业务键跨 org 时，
// org-a service 查询必须带 org_id=org-a，不得扫到 org-b。
func TestSendDueSelfEvalAutoReminders_ScopesByOrg(t *testing.T) {
	var captured []string
	var mu sync.Mutex

	stub := &stubPerformanceDB{
		queries: []stubQueryResponse{
			{
				match: func(query string, args []driver.NamedValue) bool {
					mu.Lock()
					captured = append(captured, strings.ToLower(query)+"|"+fmt.Sprint(argsValues(args)...))
					mu.Unlock()
					return strings.Contains(strings.ToLower(query), "performance_activities")
				},
				columns: []string{"id", "name", "status", "self_eval_end_at", "org_id"},
				rows:    nil, // 本测试只断言 SQL 作用域
			},
		},
	}
	svc := newStubPerformanceServiceWithDB(t, stub)
	svc.orgID = "org-a"

	_, err := svc.SendDueSelfEvalAutoReminders(time.Date(2026, 6, 2, 9, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("SendDueSelfEvalAutoReminders: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(captured) == 0 {
		t.Fatalf("expected activity scan SQL")
	}
	found := false
	for _, q := range captured {
		if strings.Contains(q, "org_id") && strings.Contains(q, "org-a") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("activity scan must include org_id=org-a; got %v", captured)
	}
	for _, q := range captured {
		if strings.Contains(q, "org-b") {
			t.Fatalf("org-a service must not bind org-b: %s", q)
		}
	}
}

// TestNewPerformanceServiceWithOrgID_DoesNotLeakAcrossOrgs 构造器绑定的 org 互不串扰。
func TestNewPerformanceServiceWithOrgID_DoesNotLeakAcrossOrgs(t *testing.T) {
	db := newDryRunPerformanceDB(t)
	a := NewPerformanceServiceWithOrgID(db, "org-a")
	b := NewPerformanceServiceWithOrgID(db, "org-b")
	if a.tenantOrgID() == b.tenantOrgID() {
		t.Fatalf("org-a and org-b services must differ")
	}
	if a.tenantOrgID() != "org-a" || b.tenantOrgID() != "org-b" {
		t.Fatalf("got a=%q b=%q", a.tenantOrgID(), b.tenantOrgID())
	}
}

func TestPerformanceJob_StopsWhenOrganizationEnumerationFails(t *testing.T) {
	stub := &stubPerformanceDB{}
	db := newStubPerformanceServiceWithDB(t, stub).db
	wantErr := errors.New("organization query failed")
	original := listActivePerformanceOrganizations
	listActivePerformanceOrganizations = func() ([]database.Organization, error) {
		return nil, wantErr
	}
	t.Cleanup(func() { listActivePerformanceOrganizations = original })

	scheduler := NewPerformanceJobScheduler(db)
	ids, err := scheduler.listActiveOrgIDs()
	if !errors.Is(err, wantErr) {
		t.Fatalf("listActiveOrgIDs error = %v, want %v", err, wantErr)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want empty", ids)
	}
	scheduler.RunSelfEvalReminderOnce(time.Now())
	assertNoPerformanceSQL(t, stub)
}

func TestPerformanceJob_StopsWhenNoActiveOrganizations(t *testing.T) {
	original := listActivePerformanceOrganizations
	listActivePerformanceOrganizations = func() ([]database.Organization, error) {
		return []database.Organization{{OrgID: "  "}}, nil
	}
	t.Cleanup(func() { listActivePerformanceOrganizations = original })

	ids, err := NewPerformanceJobScheduler(newDryRunPerformanceDB(t)).listActiveOrgIDs()
	if !errors.Is(err, ErrNoActivePerformanceOrganizations) {
		t.Fatalf("listActiveOrgIDs error = %v, want ErrNoActivePerformanceOrganizations", err)
	}
	if len(ids) != 0 {
		t.Fatalf("ids = %v, want empty", ids)
	}
}

func TestPerformanceJob_RunSelfEvalReminderOnceScopesEachOrganization(t *testing.T) {
	stub := &stubPerformanceDB{queries: []stubQueryResponse{{
		match:   stubTableMatcher("performance_activities"),
		columns: []string{"id", "name", "status", "self_eval_end_at", "org_id"},
	}}}
	db := newStubPerformanceServiceWithDB(t, stub).db
	original := listActivePerformanceOrganizations
	listActivePerformanceOrganizations = func() ([]database.Organization, error) {
		return []database.Organization{{OrgID: "org-a"}, {OrgID: "org-b"}}, nil
	}
	t.Cleanup(func() { listActivePerformanceOrganizations = original })

	NewPerformanceJobScheduler(db).RunSelfEvalReminderOnce(time.Date(2026, 6, 2, 9, 0, 0, 0, time.Local))

	stub.mu.Lock()
	calls := append([]stubPerformanceCall(nil), stub.queryCalls...)
	stub.mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("activity scan calls = %d, want 2; calls=%v", len(calls), calls)
	}
	for _, orgID := range []string{"org-a", "org-b"} {
		found := false
		for _, call := range calls {
			queryAndArgs := strings.ToLower(call.query) + "|" + fmt.Sprint(argsValues(call.args)...)
			if strings.Contains(queryAndArgs, "org_id") && strings.Contains(queryAndArgs, orgID) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing scoped activity scan for %s; calls=%v", orgID, calls)
		}
	}
}

func assertNoPerformanceSQL(t *testing.T, stub *stubPerformanceDB) {
	t.Helper()
	stub.mu.Lock()
	defer stub.mu.Unlock()
	if len(stub.queryCalls) != 0 || len(stub.execCalls) != 0 || stub.beginCalls != 0 {
		t.Fatalf("fail-closed service executed SQL: queries=%v execs=%v begins=%d", stub.queryCalls, stub.execCalls, stub.beginCalls)
	}
}

func argsValues(args []driver.NamedValue) []any {
	out := make([]any, 0, len(args))
	for _, a := range args {
		out = append(out, a.Value)
	}
	return out
}

func newDryRunPerformanceDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Reuse the stub driver with empty responses for DryRun-like opens.
	stubPerformanceDriverOnce.Do(func() {
		stdsql.Register(stubPerformanceDriverName, stubPerformanceDriver{})
	})
	dsn := fmt.Sprintf("dryrun-%s-%d", t.Name(), time.Now().UnixNano())
	stubPerformanceDBs.Store(dsn, &stubPerformanceDB{})
	t.Cleanup(func() { stubPerformanceDBs.Delete(dsn) })
	sqlDB, err := stdsql.Open(stubPerformanceDriverName, dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("gorm: %v", err)
	}
	return db
}
