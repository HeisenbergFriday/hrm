package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/gin-gonic/gin"
)

func TestPerformanceSelfParticipantVerification(t *testing.T) {
	participant := &database.PerformanceParticipant{EmployeeID: "employee-1"}

	allowedContext, allowedRecorder := newPerformanceAccessContext("employee-1")
	if !verifySelfParticipant(allowedContext, participant) {
		t.Fatalf("employee should be allowed to access own participant")
	}
	if allowedRecorder.Code != http.StatusOK {
		t.Fatalf("allowed recorder status = %d, want default 200", allowedRecorder.Code)
	}

	deniedContext, deniedRecorder := newPerformanceAccessContext("employee-2")
	if verifySelfParticipant(deniedContext, participant) {
		t.Fatalf("different employee should not be allowed to access participant")
	}
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d, want %d", deniedRecorder.Code, http.StatusForbidden)
	}

	adminContext, _ := newPerformanceAccessContext("admin")
	if !verifySelfParticipant(adminContext, participant) {
		t.Fatalf("admin should bypass self participant check")
	}
}

func TestPerformanceManagerParticipantVerification(t *testing.T) {
	managerID := "manager-1"
	participant := &database.PerformanceParticipant{EmployeeID: "employee-1", ManagerID: &managerID}

	allowedContext, _ := newPerformanceAccessContext("manager-1")
	if !verifyManagerOfParticipant(allowedContext, participant) {
		t.Fatalf("assigned manager should be allowed")
	}

	deniedContext, deniedRecorder := newPerformanceAccessContext("manager-2")
	if verifyManagerOfParticipant(deniedContext, participant) {
		t.Fatalf("different manager should not be allowed")
	}
	if deniedRecorder.Code != http.StatusForbidden {
		t.Fatalf("denied status = %d, want %d", deniedRecorder.Code, http.StatusForbidden)
	}

	noManagerContext, noManagerRecorder := newPerformanceAccessContext("manager-1")
	if verifyManagerOfParticipant(noManagerContext, &database.PerformanceParticipant{EmployeeID: "employee-1"}) {
		t.Fatalf("participant without manager should not pass manager verification")
	}
	if noManagerRecorder.Code != http.StatusForbidden {
		t.Fatalf("no manager status = %d, want %d", noManagerRecorder.Code, http.StatusForbidden)
	}
}

func TestPerformanceHandlersRejectBadInputBeforeDatabaseWork(t *testing.T) {
	tests := []struct {
		name    string
		handler gin.HandlerFunc
		method  string
		target  string
		body    string
		params  gin.Params
	}{
		{
			name:    "create activity missing required body",
			handler: CreatePerformanceActivity,
			method:  http.MethodPost,
			target:  "/api/v1/performance/activities",
			body:    `{}`,
		},
		{
			name:    "confirm employee invalid participant id",
			handler: ConfirmEmployeeResultHandler,
			method:  http.MethodPost,
			target:  "/api/v1/performance/participants/not-a-number/confirm-employee",
			body:    `{}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "confirm manager invalid participant id",
			handler: ConfirmManagerResultHandler,
			method:  http.MethodPost,
			target:  "/api/v1/performance/participants/not-a-number/confirm-manager",
			body:    `{}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "confirm hr invalid participant id",
			handler: ConfirmHRResultHandler,
			method:  http.MethodPost,
			target:  "/api/v1/performance/participants/not-a-number/confirm-hr",
			body:    `{}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "update assessment manager invalid participant id",
			handler: UpdateParticipantAssessmentManager,
			method:  http.MethodPut,
			target:  "/api/v1/performance/participants/not-a-number/assessment-manager",
			body:    `{"manager_user_id":"manager-1"}`,
			params:  gin.Params{{Key: "participant_id", Value: "not-a-number"}},
		},
		{
			name:    "batch assessment manager missing items",
			handler: BatchUpdateAssessmentManagers,
			method:  http.MethodPost,
			target:  "/api/v1/performance/activities/1/assessment-managers/batch",
			body:    `{}`,
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			c.Set("userID", "admin")
			c.Params = tt.params
			c.Request = httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))
			c.Request.Header.Set("Content-Type", "application/json")

			tt.handler(c)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func newPerformanceAccessContext(userID string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("userID", userID)
	return c, recorder
}
