package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDingTalkShiftHandlersRejectMissingOrganizationContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler gin.HandlerFunc
	}{
		{
			name:    "get shifts",
			method:  http.MethodGet,
			path:    "/api/v1/shift-config/dingtalk-shifts",
			handler: GetDingTalkShifts,
		},
		{
			name:    "debug attendance groups",
			method:  http.MethodGet,
			path:    "/api/v1/shift-config/debug-attendance-groups",
			handler: DebugAttendanceGroups,
		},
		{
			name:    "create shift",
			method:  http.MethodPost,
			path:    "/api/v1/shift-config/dingtalk-shifts",
			body:    `{"name":"白班","check_in_time":"09:00","check_out_time":"18:00"}`,
			handler: CreateDingTalkShift,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(test.method, test.path, strings.NewReader(test.body))
			if test.body != "" {
				context.Request.Header.Set("Content-Type", "application/json")
			}

			test.handler(context)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
			}
			if !strings.Contains(recorder.Body.String(), "缺少组织上下文") {
				t.Fatalf("body = %s, want missing organization message", recorder.Body.String())
			}
		})
	}
}
