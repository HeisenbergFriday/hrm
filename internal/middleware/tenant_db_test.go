package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"

	stdsql "database/sql"
	"database/sql/driver"
)

func TestCurrentOrgID_FailsClosedWhenMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	// 不设置 orgID
	_, err := CurrentOrgID(c)
	if !errors.Is(err, ErrMissingOrgContext) {
		t.Fatalf("err = %v, want ErrMissingOrgContext", err)
	}
}

func TestCurrentOrgID_ReturnsTrimmedValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("orgID", "  muteng  ")
	got, err := CurrentOrgID(c)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "muteng" {
		t.Fatalf("got = %q, want muteng", got)
	}
}

func TestTenantDB_WritesOrgIntoContextForThreeOrgs(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest("GET", "/whatever", nil)
			c.Set("orgID", org)
			// 手动模拟 RequestMetrics 注入 requestDB。
			db := newTenantDBFixture(t)
			c.Set(requestDBKey, db)

			tenantDB, err := TenantDB(c)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			got, err := requestmeta.TenantID(tenantDB.Statement.Context)
			if err != nil {
				t.Fatalf("tenant id missing: %v", err)
			}
			if got != org {
				t.Fatalf("got = %q, want %q", got, org)
			}
		})
	}
}

func TestTenantDB_FailsClosedWhenOrgMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("GET", "/whatever", nil)
	c.Set(requestDBKey, newTenantDBFixture(t))

	_, err := TenantDB(c)
	if !errors.Is(err, ErrMissingOrgContext) {
		t.Fatalf("err = %v, want ErrMissingOrgContext", err)
	}
}

func TestTenantContext_ReplacesRequestDBWithTenantContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("orgID", "muteng")
		c.Set(requestDBKey, newTenantDBFixture(t))
		c.Next()
	})
	router.Use(TenantContext())
	router.GET("/tenant", func(c *gin.Context) {
		got, err := requestmeta.TenantID(RequestDB(c).Statement.Context)
		if err != nil {
			t.Fatalf("tenant id missing from RequestDB: %v", err)
		}
		if got != "muteng" {
			t.Fatalf("got = %q, want muteng", got)
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest("GET", "/tenant", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body=%s", recorder.Code, recorder.Body.String())
	}
}

// ================= helpers =================

type tenantFixtureDriver struct{}

func (tenantFixtureDriver) Open(string) (driver.Conn, error) { return tenantFixtureConn{}, nil }

type tenantFixtureConn struct{}

func (tenantFixtureConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (tenantFixtureConn) Close() error                        { return nil }
func (tenantFixtureConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }

var tenantFixtureRegistered bool

func newTenantDBFixture(t *testing.T) *gorm.DB {
	t.Helper()
	if !tenantFixtureRegistered {
		stdsql.Register("peopleops_tenant_middleware_fixture", tenantFixtureDriver{})
		tenantFixtureRegistered = true
	}
	sqlDB, err := stdsql.Open("peopleops_tenant_middleware_fixture", "")
	if err != nil {
		t.Fatalf("open fixture sql db: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open fixture gorm: %v", err)
	}
	return db
}
