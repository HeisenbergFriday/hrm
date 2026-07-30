package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWeekSchedulePushRoutesAreRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := SetupRouter()
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}
	for _, want := range []string{
		"POST /api/v1/week-schedule/push/personal",
		"POST /api/v1/week-schedule/push/group",
		"GET /api/v1/week-schedule/group-targets",
		"DELETE /api/v1/week-schedule/group-targets/:id",
		"GET /api/v1/week-schedule/group-image",
	} {
		if _, ok := routes[want]; !ok {
			t.Fatalf("route %s is not registered", want)
		}
	}
}

func TestParseWeekSchedulePushUserIDsJSONArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("user_ids", `["u1"," u2 ","" ,"u1"]`)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/week-schedule/push/personal", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("parse form: %v", err)
	}

	ids, err := parseWeekSchedulePushUserIDs(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 3 || ids[0] != "u1" || ids[1] != "u2" || ids[2] != "u1" {
		// parser does not de-dupe; service layer does. Accept raw order with empties stripped.
		t.Fatalf("ids=%v", ids)
	}
}

func TestParseWeekSchedulePushUserIDsCommaSeparated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("user_ids", "a, b , ,c")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	ids, err := parseWeekSchedulePushUserIDs(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 3 || ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Fatalf("ids=%v", ids)
	}
}

func TestPushWeekScheduleToGroupRejectsClientCredentialOverrides(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("robotCode", "client-controlled")
	_ = w.WriteField("group_target_id", "1")
	_ = w.WriteField("month", "2026-07")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/week-schedule/push/group", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = req
	c.Set("orgID", "org-a")
	PushWeekScheduleToGroup(c)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if !bytes.Contains(recorder.Body.Bytes(), []byte("不得包含")) {
		t.Fatalf("body=%s", recorder.Body.String())
	}
}

func TestServeWeekScheduleGroupImageRejectsInvalidToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/week-schedule/group-image?token=invalid", nil)
	ServeWeekScheduleGroupImage(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}
