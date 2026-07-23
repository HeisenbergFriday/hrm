package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
)

// TestPerformanceBackgroundDB_InjectsTenantAndRequestInfo 后台 goroutine DB 必须同时
// 携带 Tenant 与 RequestInfo，避免 NewPerformanceService 读不到 org。
func TestPerformanceBackgroundDB_InjectsTenantAndRequestInfo(t *testing.T) {
	// 需要 database.DB 非空；用 nil 路径验证 fail-closed 语义。
	// 当 DB 未初始化时返回 nil；当有 org 时结构正确。此处仅验证 helper 在缺 org 时 fail-closed。
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/open-self", nil)
	// 故意不设 orgID
	if db := performanceBackgroundDB(c); db != nil {
		t.Fatalf("missing org must return nil background DB, got %#v", db)
	}

	c.Set("orgID", "org-a")
	// database.DB may be nil in unit tests; if so, still expect nil and skip tenant assertions.
	db := performanceBackgroundDB(c)
	if db == nil {
		t.Skip("database.DB not initialized; skip live context assertion")
	}
	tenant, err := requestmeta.TenantID(db.Statement.Context)
	if err != nil || tenant != "org-a" {
		t.Fatalf("TenantID = %q err=%v, want org-a", tenant, err)
	}
	info := requestmeta.FromContext(db.Statement.Context)
	if info == nil || info.OrgID != "org-a" {
		t.Fatalf("RequestInfo.OrgID missing, info=%+v", info)
	}
}
