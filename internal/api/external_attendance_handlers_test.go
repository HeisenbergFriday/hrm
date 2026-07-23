package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
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
