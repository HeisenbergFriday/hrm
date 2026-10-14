package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peopleops/internal/middleware"
	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type toolboxSyncJobsStub struct {
	start func(context.Context, string, service.DingtalkSyncRequest) (*service.ToolboxDingtalkSyncJob, error)
	run   func(context.Context, string, string, service.DingtalkSyncRequest)
}

func (s *toolboxSyncJobsStub) Start(ctx context.Context, userID string, req service.DingtalkSyncRequest) (*service.ToolboxDingtalkSyncJob, error) {
	return s.start(ctx, userID, req)
}
func (s *toolboxSyncJobsStub) Get(context.Context, string, string) (*service.ToolboxDingtalkSyncJob, error) {
	return nil, gorm.ErrRecordNotFound
}
func (s *toolboxSyncJobsStub) Run(ctx context.Context, userID, requestID string, req service.DingtalkSyncRequest) {
	s.run(ctx, userID, requestID, req)
}

func TestStartToolboxDingtalkSyncFlushes202BeforeExternalWork(t *testing.T) {
	c, recorder := newApprovalSyncHandlerContext(t, "org-a", http.MethodPost, "/attendance/toolbox/dingtalk-sync/start", `{"start_date":"2026-08-01","end_date":"2026-09-30","flow_keys":["leave"]}`)
	parent, cancel := context.WithCancel(c.Request.Context())
	c.Request = c.Request.WithContext(parent)
	defer cancel()
	originalFactory, originalLaunch := newToolboxDingtalkSyncJobs, launchToolboxDingtalkSync
	t.Cleanup(func() { newToolboxDingtalkSyncJobs, launchToolboxDingtalkSync = originalFactory, originalLaunch })
	persisted, ran := false, false
	newToolboxDingtalkSyncJobs = func(_ *gorm.DB, orgID string) toolboxDingtalkSyncJobs {
		if orgID != "org-a" {
			t.Fatal("wrong tenant")
		}
		return &toolboxSyncJobsStub{
			start: func(_ context.Context, userID string, req service.DingtalkSyncRequest) (*service.ToolboxDingtalkSyncJob, error) {
				if userID != "tester" || len(req.FlowKeys) != 1 || req.FlowKeys[0] != "leave" {
					t.Fatal("wrong input")
				}
				persisted = true
				return &service.ToolboxDingtalkSyncJob{RequestID: "job-1", Status: "running"}, nil
			},
			run: func(ctx context.Context, _, requestID string, _ service.DingtalkSyncRequest) {
				if ctx.Err() != nil || requestID != "job-1" {
					t.Fatal("client cancellation leaked")
				}
				ran = true
			},
		}
	}
	launchToolboxDingtalkSync = func(run func()) {
		if !persisted || recorder.Code != http.StatusAccepted || !recorder.Flushed {
			t.Fatal("external work preceded acceptance")
		}
		var response struct {
			Data service.ToolboxDingtalkSyncJob `json:"data"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil || response.Data.RequestID != "job-1" {
			t.Fatalf("acceptance = %s", recorder.Body.String())
		}
		cancel()
		run()
	}
	StartToolboxDingtalkSync(c)
	if !ran {
		t.Fatal("background task not scheduled")
	}
}

func TestStartToolboxDingtalkSyncRejectsMissingTenantAndClientTenant(t *testing.T) {
	for _, test := range []struct {
		org, body string
		status    int
	}{
		{"", `{}`, http.StatusUnauthorized},
		{"org-a", `{"org_id":"org-b"}`, http.StatusForbidden},
	} {
		c, recorder := newApprovalSyncHandlerContext(t, test.org, http.MethodPost, "/attendance/toolbox/dingtalk-sync/start", test.body)
		StartToolboxDingtalkSync(c)
		if recorder.Code != test.status {
			t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
		}
	}
}

func TestToolboxSyncRoutesRequireDingtalkPermission(t *testing.T) {
	originalFactory, originalLaunch := newToolboxDingtalkSyncJobs, launchToolboxDingtalkSync
	t.Cleanup(func() { newToolboxDingtalkSyncJobs, launchToolboxDingtalkSync = originalFactory, originalLaunch })
	newToolboxDingtalkSyncJobs = func(*gorm.DB, string) toolboxDingtalkSyncJobs {
		return &toolboxSyncJobsStub{start: func(context.Context, string, service.DingtalkSyncRequest) (*service.ToolboxDingtalkSyncJob, error) {
			return &service.ToolboxDingtalkSyncJob{RequestID: "job-1", Status: "running"}, nil
		}}
	}
	launchToolboxDingtalkSync = func(func()) {}
	for _, test := range []struct {
		permissions  []string
		method, path string
		status       int
	}{
		{nil, http.MethodPost, "/start", http.StatusForbidden},
		{[]string{"attendance_toolbox_operate"}, http.MethodPost, "/start", http.StatusForbidden},
		{[]string{"attendance_toolbox_dingtalk_sync"}, http.MethodPost, "/start", http.StatusAccepted},
		{[]string{"attendance_manage"}, http.MethodPost, "/start", http.StatusAccepted},
		{nil, http.MethodGet, "/jobs/job-1", http.StatusForbidden},
		{[]string{"attendance_toolbox_dingtalk_sync"}, http.MethodGet, "/jobs/job-1", http.StatusNotFound},
	} {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("orgID", "org-a")
			c.Set("userID", "user-a")
			middleware.SetAuthContextForTest(c, &middleware.AuthContext{
				OrgID: "org-a", UserID: "user-a", RawUserID: "user-a",
				PermissionSet: permSet(test.permissions), MenuKeySet: permSet([]string{"menu:attendance-toolbox"}),
			})
			c.Next()
		})
		registerAttendanceToolboxRoutes(router.Group("/api/v1/attendance"))
		request := httptest.NewRequest(test.method, "/api/v1/attendance/toolbox/dingtalk-sync"+test.path, strings.NewReader(`{"start_date":"2026-08-01","end_date":"2026-09-30"}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		if recorder.Code != test.status {
			t.Fatalf("permissions=%v route=%s: status=%d body=%s", test.permissions, test.path, recorder.Code, recorder.Body.String())
		}
	}
}
