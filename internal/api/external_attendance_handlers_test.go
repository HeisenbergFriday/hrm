package api

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func newExternalSyncCtx(t *testing.T, method, target, body, orgID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	var reader *bytes.Reader
	if body == "" {
		reader = bytes.NewReader(nil)
	} else {
		reader = bytes.NewReader([]byte(body))
	}
	req := httptest.NewRequest(method, target, reader)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	c.Request = req
	c.Set("orgID", orgID)
	c.Set("userID", "tester")
	return c, rec
}

func TestExternalAttendanceSyncRun_MalformedJSON(t *testing.T) {
	c, rec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", "{not-json", "xiaotie")
	ExternalAttendanceSyncRun(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceSyncRun_InvalidSource(t *testing.T) {
	// Handler order: bind -> source -> lookback -> enabled -> dsn
	c, rec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", `{"source":"nope"}`, "xiaotie")
	ExternalAttendanceSyncRun(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceSyncRun_LookbackOutOfRange(t *testing.T) {
	c, rec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", `{"source":"attendance","lookback_minutes":999999}`, "xiaotie")
	ExternalAttendanceSyncRun(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceSyncRun_Disabled(t *testing.T) {
	t.Setenv("EXTERNAL_ATTENDANCE_SYNC_ENABLED", "false")
	t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "user:pass@tcp(127.0.0.1:9030)/dwd")
	c, rec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", `{"source":"attendance"}`, "xiaotie")
	ExternalAttendanceSyncRun(c)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceSyncRun_NotConfigured(t *testing.T) {
	t.Setenv("EXTERNAL_ATTENDANCE_SYNC_ENABLED", "true")
	// Clear URL and host so DSN empty
	t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "")
	t.Setenv("EXTERNAL_ATTENDANCE_DB_HOST", "")
	c, rec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", `{"source":"attendance"}`, "xiaotie")
	ExternalAttendanceSyncRun(c)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceSyncRun_EmptyBodyAllowedWhenDisabled(t *testing.T) {
	// Empty body is allowed by binder (EOF); then disabled returns 503 not 400.
	t.Setenv("EXTERNAL_ATTENDANCE_SYNC_ENABLED", "false")
	t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "user:pass@tcp(127.0.0.1:9030)/dwd")
	c, rec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", "", "xiaotie")
	ExternalAttendanceSyncRun(c)
	if rec.Code == http.StatusBadRequest {
		t.Fatalf("empty body must not be 400: %s", rec.Body.String())
	}
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceSyncRun_AcceptsOnceAndRejectsDuplicate(t *testing.T) {
	dsn := fmt.Sprintf("file:external-sync-handler-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, sqlErr := db.DB(); sqlErr == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if err := db.AutoMigrate(&database.ExternalSyncJob{}, &database.ExternalSyncLock{}); err != nil {
		t.Fatalf("migrate task tables: %v", err)
	}
	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })

	previousLauncher := launchExternalAttendanceSyncBackground
	previousTimeout := externalAttendanceExecutionTimeout
	launched := 0
	launchExternalAttendanceSyncBackground = func(func()) { launched++ }
	externalAttendanceExecutionTimeout = time.Second
	t.Cleanup(func() {
		launchExternalAttendanceSyncBackground = previousLauncher
		externalAttendanceExecutionTimeout = previousTimeout
	})
	t.Setenv("EXTERNAL_ATTENDANCE_SYNC_ENABLED", "true")
	t.Setenv("EXTERNAL_ATTENDANCE_DATABASE_URL", "configured-for-control-plane-test")

	firstCtx, firstRec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", `{"source":"all"}`, "xiaotie")
	ExternalAttendanceSyncRun(firstCtx)
	if firstRec.Code != http.StatusAccepted || !strings.Contains(firstRec.Body.String(), `"status":"running"`) {
		t.Fatalf("first response code=%d body=%s", firstRec.Code, firstRec.Body.String())
	}
	if launched != 1 {
		t.Fatalf("background launch count=%d want 1", launched)
	}

	secondCtx, secondRec := newExternalSyncCtx(t, http.MethodPost, "/api/v1/attendance/external-sync/run", `{"source":"attendance"}`, "xiaotie")
	ExternalAttendanceSyncRun(secondCtx)
	if secondRec.Code != http.StatusConflict || !strings.Contains(secondRec.Body.String(), "已有外部同步任务运行中") {
		t.Fatalf("second response code=%d body=%s", secondRec.Code, secondRec.Body.String())
	}
	var count int64
	if err := db.Model(&database.ExternalSyncJob{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("job count=%d err=%v", count, err)
	}
}

func TestExternalAttendanceSyncJobDetail_InvalidID(t *testing.T) {
	c, rec := newExternalSyncCtx(t, http.MethodGet, "/api/v1/attendance/external-sync/jobs/abc", "", "xiaotie")
	c.Params = gin.Params{{Key: "id", Value: "abc"}}
	ExternalAttendanceSyncJobDetail(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceSyncJobDetail_MissingOrg(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/attendance/external-sync/jobs/1", nil)
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	// no orgID set
	ExternalAttendanceSyncJobDetail(c)
	// currentOrgIDOrAbort should reject
	if rec.Code == http.StatusOK {
		t.Fatalf("must not leak job without org: %s", rec.Body.String())
	}
}

func TestExternalAttendanceDailyResults_InvalidDateRange(t *testing.T) {
	c, rec := newExternalSyncCtx(t, http.MethodGet, "/api/v1/attendance/external-sync/daily-results?start_date=2026-07-20&end_date=2026-07-01", "", "xiaotie")
	ExternalAttendanceDailyResults(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceDailyResults_RangeTooLarge(t *testing.T) {
	c, rec := newExternalSyncCtx(t, http.MethodGet, "/api/v1/attendance/external-sync/daily-results?start_date=2026-01-01&end_date=2026-07-01", "", "xiaotie")
	ExternalAttendanceDailyResults(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceDailyResults_InvalidStatus(t *testing.T) {
	c, rec := newExternalSyncCtx(t, http.MethodGet, "/api/v1/attendance/external-sync/daily-results?status=nope", "", "xiaotie")
	ExternalAttendanceDailyResults(c)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalAttendanceRoutesRegistered(t *testing.T) {
	router := SetupRouter()
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, route := range []string{
		"GET /api/v1/attendance/external-sync/status",
		"GET /api/v1/attendance/external-sync/daily-results",
		"POST /api/v1/attendance/external-sync/run",
		"GET /api/v1/attendance/external-sync/jobs",
		"GET /api/v1/attendance/external-sync/jobs/:id",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("route %s is not registered", route)
		}
	}
}
