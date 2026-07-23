//go:build integration

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/middleware"
	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Integration repro for the PerformanceOverview boot requests.
//
// This test NEVER runs under plain `go test ./...`:
//   - it is guarded by the `integration` build tag, and
//   - it requires DATABASE_URL to be provided explicitly (no default DSN,
//     no embedded credentials). Missing DATABASE_URL skips the test.
//
// Run:
//
//	DATABASE_URL='user:pass@tcp(host:3306)/dbname?charset=utf8mb4&parseTime=True&loc=Local' \
//	  go test -tags integration ./internal/api -run TestIntegrationPerformanceOverviewBootRequests -v -count=1
//
// Optional: PERF_REPRO_ORG_ID / PERF_REPRO_USER_ID select the login identity
// (defaults: org "default", user "admin").
func TestIntegrationPerformanceOverviewBootRequests(t *testing.T) {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set; skipping integration repro test")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	// Match production Init(): global org-scope callbacks rewrite queries with org_id.
	database.RegisterOrganizationCallbacksForTest(db)
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })

	orgID := os.Getenv("PERF_REPRO_ORG_ID")
	if orgID == "" {
		orgID = "default"
	}
	userID := os.Getenv("PERF_REPRO_USER_ID")
	if userID == "" {
		userID = "admin"
	}

	call := func(name, method, path string, handler gin.HandlerFunc) {
		t.Helper()
		gin.SetMode(gin.TestMode)
		rec := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(rec)
		req := httptest.NewRequest(method, path, nil)
		info := &requestmeta.RequestInfo{OrgID: orgID, Route: path}
		ctx := requestmeta.WithRequestInfo(req.Context(), info)
		ctx = requestmeta.WithTenant(ctx, orgID)
		req = req.WithContext(ctx)
		c.Request = req
		c.Set("orgID", orgID)
		c.Set("userID", userID)
		c.Set("requestDB", db.WithContext(ctx))

		handler(c)

		t.Logf("%s status=%d", name, rec.Code)
		if rec.Code != http.StatusOK {
			t.Errorf("%s expected 200, got %d; body = %s", name, rec.Code, rec.Body.String())
			return
		}
		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Errorf("%s invalid json: %v", name, err)
			return
		}
		if code, _ := resp["code"].(float64); code != 200 {
			t.Errorf("%s business code=%v message=%v", name, resp["code"], resp["message"])
		}
	}

	// The three boot requests PerformanceOverview actually issues now:
	// activities + scope-options + templates. The legacy /users and
	// /departments calls were replaced by /performance/scope-options.
	call("activities", http.MethodGet, "/api/v1/performance/activities?page=1&page_size=100", GetPerformanceActivities)
	call("scope-options", http.MethodGet, "/api/v1/performance/scope-options?page=1&page_size=2000", GetPerformanceScopeOptions)
	call("templates", http.MethodGet, "/api/v1/performance/templates?page=1&page_size=1000&status=active", GetPerformanceTemplates)

	// Data-scope resolution used by router middleware.
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest(http.MethodGet, "/probe", nil)
	info := &requestmeta.RequestInfo{OrgID: orgID}
	ctx := requestmeta.WithRequestInfo(req.Context(), info)
	ctx = requestmeta.WithTenant(ctx, orgID)
	req = req.WithContext(ctx)
	c.Request = req
	c.Set("orgID", orgID)
	c.Set("userID", userID)
	c.Set("requestDB", db.WithContext(ctx))
	if _, err := middleware.UserDataScope(c); err != nil {
		t.Errorf("UserDataScope failed: %v", err)
	}
}
