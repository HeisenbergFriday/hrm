package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/service"

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

func TestWeekScheduleGroupPushPermissionRegression(t *testing.T) {
	tests := []struct {
		name       string
		permission string
		wantStatus int
	}{
		{name: "missing permission", wantStatus: http.StatusForbidden},
		{name: "group push permission", permission: service.WeekScheduleGroupPushPermission, wantStatus: http.StatusOK},
		{name: "attendance manage compatibility", permission: "attendance_manage", wantStatus: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/week-schedule/push/group", nil)
			c.Set("userID", "u-1")
			c.Set("orgID", "org-a")
			permissions := map[string]struct{}{}
			if tc.permission != "" {
				permissions[tc.permission] = struct{}{}
			}
			middleware.SetAuthContextForTest(c, &middleware.AuthContext{
				OrgID: "org-a", UserID: "u-1", PermissionSet: permissions, MenuKeySet: map[string]struct{}{},
			})
			middleware.RequirePermission(service.WeekScheduleGroupPushPermission, "attendance_manage")(c)
			if tc.wantStatus == http.StatusOK {
				if c.IsAborted() {
					t.Fatalf("request unexpectedly aborted: status=%d body=%s", recorder.Code, recorder.Body.String())
				}
				return
			}
			if !c.IsAborted() || recorder.Code != tc.wantStatus {
				t.Fatalf("aborted=%t status=%d body=%s", c.IsAborted(), recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestWeekSchedulePersonalPushRegressionStillUsesPersonalUploadContract(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("user_ids", `["u-1"]`)
	_ = w.Close()
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/week-schedule/push/personal", body)
	c.Request.Header.Set("Content-Type", w.FormDataContentType())
	c.Set("orgID", "org-a")
	PushPersonalWeekSchedule(c)
	if recorder.Code != http.StatusBadRequest || !bytes.Contains(recorder.Body.Bytes(), []byte("请上传作息表图片")) {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
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

func TestRespondWeekScheduleGroupPushErrorUsesConfirmedUnavailableMessage(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	respondWeekScheduleGroupPushError(c, &dingtalk.SyncError{
		Code:        dingtalk.ErrorCodeGroupUnavailable,
		SafeMessage: "old internal summary",
	})
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	want := "机器人已不在该群，请重新添加机器人并在群内 @机器人完成绑定。"
	if !bytes.Contains(recorder.Body.Bytes(), []byte(want)) {
		t.Fatalf("body=%s", recorder.Body.String())
	}
}
