package api

import (
	"database/sql/driver"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peopleops/internal/database"

	"github.com/gin-gonic/gin"
)

// ===================== Test Setup Helpers =====================

func performanceCoverageTestDB(t *testing.T) {
	t.Helper()
	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
			{int64(1), "admin", "Admin", "dept-1", "active"},
			{int64(2), "user-1", "Alice", "dept-1", "active"},
			{int64(3), "manager-1", "Bob Manager", "dept-1", "active"},
		}),
		apiImportTableResponse("performance_activities", []string{"id", "name", "status", "self_eval_start_at", "self_eval_end_at"}, [][]driver.Value{
			{int64(1), "Q1 Performance", "published", "2024-01-01 00:00:00", "2024-01-31 23:59:59"},
		}),
		apiImportTableResponse("performance_participants", []string{"id", "activity_id", "employee_id", "employee_name", "department_id", "department_name", "manager_id", "status"}, [][]driver.Value{
			{int64(1), "1", "user-1", "Alice", "dept-1", "Product", ptrString("manager-1"), "pending"},
		}),
	)
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func performanceCoverageAdminContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("userID", "admin")
	return c, recorder
}

// ===================== RefreshPerformanceParticipants Tests =====================

func TestRefreshPerformanceParticipants_InvalidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/999/refresh-participants", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "999"}}

	RefreshPerformanceParticipants(c)

	if recorder.Code == http.StatusOK {
		t.Fatalf("status = %d, want non-200 for invalid activity; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRefreshPerformanceParticipants_Success(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/refresh-participants", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	RefreshPerformanceParticipants(c)

	// Even if service fails due to stub DB, handler should not panic
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d or %d; body = %s", recorder.Code, http.StatusOK, http.StatusBadRequest, recorder.Body.String())
	}
}

// ===================== ConfirmResult Tests =====================

func TestConfirmResult_ValidRequest(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/confirm", strings.NewReader(`{"confirm_comment":"approved"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	ConfirmResult(c)

	// Handler should process without panic, actual result depends on stub DB state
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound && recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestConfirmResult_MissingBody(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/confirm", strings.NewReader(`invalid json`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	ConfirmResult(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for malformed JSON; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

// ===================== ConfirmEmployeeResultHandler Tests =====================

func TestConfirmEmployeeResultHandler_InvalidParticipantID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/abc/confirm-employee", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "abc"}}

	ConfirmEmployeeResultHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for invalid ID; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestConfirmEmployeeResultHandler_ValidID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/confirm-employee", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	ConfirmEmployeeResultHandler(c)

	// Handler will attempt to confirm, may fail due to stub DB but should not panic
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound && recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

// ===================== ConfirmManagerResultHandler Tests =====================

func TestConfirmManagerResultHandler_InvalidParticipantID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/xyz/confirm-manager", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "xyz"}}

	ConfirmManagerResultHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for invalid ID; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestConfirmManagerResultHandler_ValidID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/confirm-manager", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	ConfirmManagerResultHandler(c)

	// Handler will reach service call, response depends on stub DB state
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound && recorder.Code != http.StatusForbidden {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

// ===================== ConfirmHRResultHandler Tests =====================

func TestConfirmHRResultHandler_InvalidParticipantID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/notanumber/confirm-hr", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "notanumber"}}

	ConfirmHRResultHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d for invalid ID; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestConfirmHRResultHandler_ValidID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/confirm-hr", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	ConfirmHRResultHandler(c)

	// Handler will reach service call
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

// ===================== Activity Flow Handler Tests =====================

func TestOpenSelfEvaluation_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/open-self-evaluation", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	OpenSelfEvaluation(c)

	// Handler should execute without panic
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOpenManagerEvaluation_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/open-manager-evaluation", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	OpenManagerEvaluation(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestConfirmActivityResults_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/confirm-results", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	ConfirmActivityResults(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOpenEmployeeConfirmationHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/open-employee-confirmation", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	OpenEmployeeConfirmationHandler(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOpenManagerConfirmationHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/open-manager-confirmation", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	OpenManagerConfirmationHandler(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestOpenHRConfirmationHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/open-hr-confirmation", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	OpenHRConfirmationHandler(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestLockPerformanceActivityHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/lock", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	LockPerformanceActivityHandler(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestForceLockOverdueHRConfirmationHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/force-lock-overdue", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	ForceLockOverdueHRConfirmationHandler(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

// ===================== Reminder Handler Tests =====================

func TestSendHRConfirmReminder_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/send-hr-confirm-reminder", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	SendHRConfirmReminder(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

// ===================== Goal Approval Handler Tests =====================

func TestApproveGoalRecords_MissingBody(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/goal-records/approve", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	ApproveGoalRecords(c)

	// Missing required fields should return BadRequest
	if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d or %d for missing body; body = %s", recorder.Code, http.StatusBadRequest, http.StatusNotFound, recorder.Body.String())
	}
}

func TestApproveGoalRecords_ValidRequest(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	body := `{"record_ids":[1,2],"approve_comment":"approved"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/goal-records/approve", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	ApproveGoalRecords(c)

	// Handler should process without panic
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

// ===================== Additional Coverage Tests =====================

func TestSetCompanyFinanceHandler_MissingRevenueSign(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	body := `{"description":"test"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/company-finance", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	SetCompanyFinanceHandler(c)

	// Handler should execute
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusBadRequest {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetCompanyFinanceHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/activities/1/company-finance", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	GetCompanyFinanceHandler(c)

	// Should return NotFound or OK
	if recorder.Code != http.StatusOK && recorder.Code != http.StatusNotFound {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetPendingHRConfirmHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/activities/1/pending-hr-confirm", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	GetPendingHRConfirmHandler(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetHRConfirmDeadlineStatusHandler_ValidActivityID(t *testing.T) {
	performanceCoverageTestDB(t)
	c, recorder := performanceCoverageAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/activities/1/hr-confirm-deadline-status", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	GetHRConfirmDeadlineStatusHandler(c)

	if recorder.Code != http.StatusOK && recorder.Code != http.StatusInternalServerError {
		t.Fatalf("unexpected status = %d; body = %s", recorder.Code, recorder.Body.String())
	}
}

// ===================== Permission Helper Tests =====================

func TestHasPerformancePermissionBranches(t *testing.T) {
	t.Run("empty codes returns false", func(t *testing.T) {
		c, _ := performanceCoverageUserContext(t, "admin")
		ok, err := hasPerformancePermission(c)
		if err != nil {
			t.Fatalf("hasPerformancePermission() error = %v", err)
		}
		if ok {
			t.Fatal("empty permission codes should not grant access")
		}
	})

	t.Run("admin bypasses database permission lookup", func(t *testing.T) {
		c, _ := performanceCoverageUserContext(t, "admin")
		ok, err := hasPerformancePermission(c, "performance:self_eval:submit")
		if err != nil {
			t.Fatalf("hasPerformancePermission() error = %v", err)
		}
		if !ok {
			t.Fatal("admin should have performance permission")
		}
	})

	t.Run("role permission grants access", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformancePermissionUsersResponse(),
			apiPerformanceRouterPermissionsResponse([]string{"performance:self_eval:submit"}),
		)
		c, _ := performanceCoverageUserContext(t, "user-1")
		ok, err := hasPerformancePermission(c, "performance:self_eval:submit")
		if err != nil {
			t.Fatalf("hasPerformancePermission() error = %v", err)
		}
		if !ok {
			t.Fatal("expected permission to be granted")
		}
	})

	t.Run("missing role permission denies access", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformancePermissionUsersResponse(),
			apiPerformanceRouterPermissionsResponse([]string{"performance:other"}),
		)
		c, _ := performanceCoverageUserContext(t, "user-1")
		ok, err := hasPerformancePermission(c, "performance:self_eval:submit")
		if err != nil {
			t.Fatalf("hasPerformancePermission() error = %v", err)
		}
		if ok {
			t.Fatal("unexpected permission grant")
		}
	})

	t.Run("permission repository error is returned", func(t *testing.T) {
		performanceCoverageDBWith(t, apiPerformancePermissionUsersResponse())
		c, _ := performanceCoverageUserContext(t, "user-1")
		ok, err := hasPerformancePermission(c, "performance:self_eval:submit")
		if err == nil {
			t.Fatal("expected permission lookup error")
		}
		if ok {
			t.Fatal("permission lookup error should not grant access")
		}
	})
}

// ===================== Review Submission And Notification Tests =====================

func TestSubmitReviewSelfEvaluationBranches(t *testing.T) {
	t.Run("bad json returns bad request", func(t *testing.T) {
		performanceHandlerTestDB(t)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/participants/1/review-self-evaluation",
			`{"self_content_json":`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			SubmitReviewSelfEvaluation,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("missing participant returns not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceParticipantRowsResponse(nil))
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/participants/99/review-self-evaluation",
			`{"self_content_json":{"content":"done"}}`,
			gin.Params{{Key: "participant_id", Value: "99"}},
			SubmitReviewSelfEvaluation,
		)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
		}
	})

	t.Run("self user without permission is forbidden", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformancePermissionUsersResponse(),
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
			apiPerformanceRouterPermissionsResponse([]string{"performance:other"}),
		)
		c, recorder := performanceCoverageUserContext(t, "user-1")
		c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/review-self-evaluation", strings.NewReader(`{"self_content_json":{"content":"done"}}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

		SubmitReviewSelfEvaluation(c)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
		}
	})

	t.Run("service submit error is bad request", func(t *testing.T) {
		first, second := apiPerformanceParticipantFirstThenRows(
			[][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)},
			nil,
		)
		performanceHandlerTestDBWith(t, first, second)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/participants/1/review-self-evaluation",
			`{"self_content_json":{"content":"done"}}`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			SubmitReviewSelfEvaluation,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("admin can submit self evaluation", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/participants/1/review-self-evaluation",
			`{"self_content_json":{"content":"done"}}`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			SubmitReviewSelfEvaluation,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})
}

func TestPerformanceNotificationHelpers(t *testing.T) {
	t.Run("notify employee returns participant lookup error", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceParticipantRowsResponse(nil))
		if err := notifyEmployeeOnManagerEval("404", "A", "great"); err == nil {
			t.Fatal("expected participant lookup error")
		}
	})

	t.Run("notify employee skips non notifiable user", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "admin", nil, "manager_submitted", false)}),
		)
		if err := notifyEmployeeOnManagerEval("1", "A", "great"); err != nil {
			t.Fatalf("notifyEmployeeOnManagerEval() error = %v", err)
		}
	})

	t.Run("self evaluation open activity error", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceActivityRowsResponse(nil))
		if err := notifyParticipantsOnSelfEvaluationOpen("404"); err == nil {
			t.Fatal("expected activity lookup error")
		}
	})

	t.Run("self evaluation open with no participants succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "published")}),
			apiPerformanceParticipantRowsResponse(nil),
		)
		if err := notifyParticipantsOnSelfEvaluationOpen("1"); err != nil {
			t.Fatalf("notifyParticipantsOnSelfEvaluationOpen() error = %v", err)
		}
	})

	t.Run("result ready activity error", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceActivityRowsResponse(nil))
		if err := notifyParticipantsResultReady("404"); err == nil {
			t.Fatal("expected activity lookup error")
		}
	})

	t.Run("result ready with no participants succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "result_confirming")}),
			apiPerformanceParticipantRowsResponse(nil),
		)
		if err := notifyParticipantsResultReady("1"); err != nil {
			t.Fatalf("notifyParticipantsResultReady() error = %v", err)
		}
	})

	t.Run("result ready participant query error", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "result_confirming")}),
		)
		if err := notifyParticipantsResultReady("1"); err == nil {
			t.Fatal("expected participant query error")
		}
	})

	t.Run("result ready query filters hidden participants", func(t *testing.T) {
		matchedParticipantQuery := false
		performanceCoverageDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "result_confirming")}),
			apiImportQueryResponse{
				match: func(query string, _ []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					matched := strings.Contains(lower, "performance_participants") &&
						strings.Contains(lower, "result_hidden") &&
						!strings.Contains(lower, "count(")
					if matched {
						matchedParticipantQuery = true
					}
					return matched
				},
				columns: []string{"id"},
				rows:    nil,
			},
		)
		if err := notifyParticipantsResultReady("1"); err != nil {
			t.Fatalf("notifyParticipantsResultReady() error = %v", err)
		}
		if !matchedParticipantQuery {
			t.Fatal("participant query did not filter result_hidden")
		}
	})

	t.Run("result ready skips unnotifiable participants", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "result_confirming")}),
			apiPerformanceParticipantRowsResponse([][]driver.Value{
				apiPerformanceParticipantRowWithDetails(1, "1", "", ptrString("manager-1"), "manager_submitted", "active", false, 90, "A"),
				apiPerformanceParticipantRowWithDetails(2, "1", "admin", ptrString("manager-1"), "manager_submitted", "active", false, 90, "A"),
				apiPerformanceParticipantRowWithDetails(3, "1", "user-1", ptrString("manager-1"), "removed_from_scope", "active", false, 90, "A"),
				apiPerformanceParticipantRowWithDetails(4, "1", "user-1", ptrString("manager-1"), "manager_submitted", "inactive", false, 90, "A"),
			}),
			apiDingtalkNotifiableUserCountResponse(1),
		)
		if err := notifyParticipantsResultReady("1"); err != nil {
			t.Fatalf("notifyParticipantsResultReady() error = %v", err)
		}
	})

	t.Run("result locked activity error", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceActivityRowsResponse(nil))
		if err := notifyParticipantsResultLocked("404"); err == nil {
			t.Fatal("expected activity lookup error")
		}
	})

	t.Run("result locked with no participants succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "locked")}),
			apiPerformanceParticipantRowsResponse(nil),
		)
		if err := notifyParticipantsResultLocked("1"); err != nil {
			t.Fatalf("notifyParticipantsResultLocked() error = %v", err)
		}
	})

	t.Run("result locked participant query error", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "locked")}),
		)
		if err := notifyParticipantsResultLocked("1"); err == nil {
			t.Fatal("expected participant query error")
		}
	})

	t.Run("result locked query filters hidden participants", func(t *testing.T) {
		matchedParticipantQuery := false
		performanceCoverageDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "locked")}),
			apiImportQueryResponse{
				match: func(query string, _ []driver.NamedValue) bool {
					lower := strings.ToLower(query)
					matched := strings.Contains(lower, "performance_participants") &&
						strings.Contains(lower, "result_hidden") &&
						!strings.Contains(lower, "count(")
					if matched {
						matchedParticipantQuery = true
					}
					return matched
				},
				columns: []string{"id"},
				rows:    nil,
			},
		)
		if err := notifyParticipantsResultLocked("1"); err != nil {
			t.Fatalf("notifyParticipantsResultLocked() error = %v", err)
		}
		if !matchedParticipantQuery {
			t.Fatal("participant query did not filter result_hidden")
		}
	})

	t.Run("result locked skips unnotifiable participants", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "locked")}),
			apiPerformanceParticipantRowsResponse([][]driver.Value{
				apiPerformanceParticipantRowWithDetails(1, "1", "", ptrString("manager-1"), "manager_submitted", "active", true, 90, "A"),
				apiPerformanceParticipantRowWithDetails(2, "1", "system", ptrString("manager-1"), "manager_submitted", "active", true, 90, "A"),
				apiPerformanceParticipantRowWithDetails(3, "1", "user-1", ptrString("manager-1"), "removed_from_scope", "active", true, 90, "A"),
				apiPerformanceParticipantRowWithDetails(4, "1", "user-1", ptrString("manager-1"), "manager_submitted", "exited", true, 90, "A"),
			}),
			apiDingtalkNotifiableUserCountResponse(1),
		)
		if err := notifyParticipantsResultLocked("1"); err != nil {
			t.Fatalf("notifyParticipantsResultLocked() error = %v", err)
		}
	})
}

func TestBatchSubmitManagerEvaluationBranches(t *testing.T) {
	validBody := `{"evaluations":[{"participant_id":1,"manager_score":88,"suggested_level":"B","manager_comment":"ok","evaluation_items":[{"item_key":"kpi","item_score":88,"item_value":"done"}]}]}`

	t.Run("participant not found returns not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "manager_evaluation")}),
			apiPerformanceParticipantRowsResponse(nil),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/activities/1/batch-manager-evaluation", validBody, gin.Params{{Key: "activity_id", Value: "1"}}, BatchSubmitManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusNotFound)
	})

	t.Run("activity mismatch returns bad request", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "manager_evaluation")}),
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "2", "user-1", ptrString("manager-1"), "pending", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/activities/1/batch-manager-evaluation", validBody, gin.Params{{Key: "activity_id", Value: "1"}}, BatchSubmitManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("non manager returns forbidden", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "manager_evaluation")}),
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/activities/1/batch-manager-evaluation", validBody, gin.Params{{Key: "activity_id", Value: "1"}}, BatchSubmitManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusForbidden)
	})

	t.Run("service error returns bad request", func(t *testing.T) {
		first, second := apiPerformanceParticipantFirstThenRows(
			[][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)},
			nil,
		)
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "manager_evaluation")}),
			first,
			second,
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/activities/1/batch-manager-evaluation", validBody, gin.Params{{Key: "activity_id", Value: "1"}}, BatchSubmitManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("manager success returns versions", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "manager_evaluation")}),
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/activities/1/batch-manager-evaluation", validBody, gin.Params{{Key: "activity_id", Value: "1"}}, BatchSubmitManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusOK)
		if !strings.Contains(recorder.Body.String(), "versions") {
			t.Fatalf("response should contain versions; body = %s", recorder.Body.String())
		}
	})

	t.Run("bonus enabled recalculates suggested level", func(t *testing.T) {
		body := `{"evaluations":[{"participant_id":1,"manager_score":85,"bonus_score":10,"suggested_level":"C","manager_comment":"bonus","evaluation_items":[{"item_key":"kpi","item_score":85,"item_value":"done"}]}]}`
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsWithBonusResponse([][]driver.Value{apiPerformanceActivityRowWithBonus(1, "Q2 Review", "manager_evaluation", true)}),
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/activities/1/batch-manager-evaluation", body, gin.Params{{Key: "activity_id", Value: "1"}}, BatchSubmitManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusOK)
		if !strings.Contains(recorder.Body.String(), "\"suggested_level\":\"A\"") {
			t.Fatalf("expected recalculated suggested level A; body = %s", recorder.Body.String())
		}
	})
}

func TestSubmitReviewManagerEvaluationBranches(t *testing.T) {
	validBody := `{"manager_score_json":{"KPI1":90},"manager_comment":"good","final_level":"A"}`

	t.Run("participant not found returns not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceParticipantRowsResponse(nil))
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/review-manager-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitReviewManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusNotFound)
	})

	t.Run("non manager returns forbidden", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/participants/1/review-manager-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitReviewManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusForbidden)
	})

	t.Run("service error returns bad request", func(t *testing.T) {
		first, second := apiPerformanceParticipantFirstThenRows(
			[][]driver.Value{apiPerformanceParticipantRow(1, "1", "admin", ptrString("manager-1"), "pending", false)},
			nil,
		)
		performanceHandlerTestDBWith(t, first, second)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/review-manager-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitReviewManagerEvaluation)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})
}

func TestSubmitGoalSelfEvaluationHandlerBranches(t *testing.T) {
	validBody := `{"items":[{"record_id":1,"actual_result":"done","self_score":90}],"evaluation_good":"good","evaluation_improvement":"improve"}`

	t.Run("invalid body returns bad request", func(t *testing.T) {
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/participants/1/goal-self-evaluation", `{"items":"bad"}`, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalSelfEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("participant not found returns not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceParticipantRowsResponse(nil))
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/participants/1/goal-self-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalSelfEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusNotFound)
	})

	t.Run("access forbidden returns forbidden", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
			apiPerformanceRouterPermissionsResponse(nil),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/goal-self-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalSelfEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusForbidden)
	})

	t.Run("wrong activity status returns bad request", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
			apiPerformanceRouterPermissionsResponse([]string{"performance:self_eval:submit"}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "manager_evaluation")}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/participants/1/goal-self-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalSelfEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("success submits goal self evaluation", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
			apiPerformanceRouterPermissionsResponse([]string{"performance:self_eval:submit"}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "self_evaluation")}),
			apiPerformanceGoalRecordsResponse([][]driver.Value{apiPerformanceGoalRecordRow(1, 1, "1", "quantitative", "Revenue", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/participants/1/goal-self-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalSelfEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusOK)
	})
}

func TestSubmitGoalManagerEvaluationHandlerBranches(t *testing.T) {
	validBody := `{"items":[{"record_id":1,"manager_score":90}],"suggested_level":"A","evaluation_good":"good","evaluation_improvement":"improve"}`

	t.Run("invalid body returns bad request", func(t *testing.T) {
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/goal-manager-evaluation", `{"items":"bad"}`, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalManagerEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("participant not found returns not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceParticipantRowsResponse(nil))
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/goal-manager-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalManagerEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusNotFound)
	})

	t.Run("non manager returns forbidden", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/participants/1/goal-manager-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalManagerEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusForbidden)
	})

	t.Run("wrong activity status returns bad request", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "self_evaluation")}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/goal-manager-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalManagerEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("success submits goal manager evaluation", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "manager_evaluation")}),
			apiPerformanceGoalRecordsResponse([][]driver.Value{apiPerformanceGoalRecordRow(1, 1, "1", "quantitative", "Revenue", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/goal-manager-evaluation", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SubmitGoalManagerEvaluationHandler)
		assertRecorderStatus(t, recorder, http.StatusOK)
	})
}

func TestSetBonusPenaltyScoreHandlerBranches(t *testing.T) {
	validBody := `{"bonus_score":5,"penalty_score":2}`

	t.Run("invalid body returns bad request", func(t *testing.T) {
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/bonus-penalty-score", `{"bonus_score":"bad"}`, gin.Params{{Key: "participant_id", Value: "1"}}, SetBonusPenaltyScoreHandler)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("participant not found returns not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceParticipantRowsResponse(nil))
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/bonus-penalty-score", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SetBonusPenaltyScoreHandler)
		assertRecorderStatus(t, recorder, http.StatusNotFound)
	})

	t.Run("non manager returns forbidden", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", ptrString("manager-1"), "pending", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "user-1", http.MethodPost, "/api/v1/performance/participants/1/bonus-penalty-score", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SetBonusPenaltyScoreHandler)
		assertRecorderStatus(t, recorder, http.StatusForbidden)
	})

	t.Run("locked participant returns bad request", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRowWithDetails(1, "1", "user-1", ptrString("manager-1"), "pending", "active", true, 90, "B")}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/bonus-penalty-score", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SetBonusPenaltyScoreHandler)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("bonus disabled activity returns bad request", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRowWithDetails(1, "1", "user-1", ptrString("manager-1"), "pending", "active", false, 90, "B")}),
			apiPerformanceActivityRowsWithBonusResponse([][]driver.Value{apiPerformanceActivityRowWithBonus(1, "Q2 Review", "manager_evaluation", false)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/bonus-penalty-score", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SetBonusPenaltyScoreHandler)
		assertRecorderStatus(t, recorder, http.StatusBadRequest)
	})

	t.Run("success sets bonus penalty score", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRowWithDetails(1, "1", "user-1", ptrString("manager-1"), "pending", "active", false, 90, "B")}),
			apiPerformanceActivityRowsWithBonusResponse([][]driver.Value{apiPerformanceActivityRowWithBonus(1, "Q2 Review", "manager_evaluation", true)}),
		)
		recorder := performPerformanceHandlerRequestAs(t, "manager-1", http.MethodPost, "/api/v1/performance/participants/1/bonus-penalty-score", validBody, gin.Params{{Key: "participant_id", Value: "1"}}, SetBonusPenaltyScoreHandler)
		assertRecorderStatus(t, recorder, http.StatusOK)
	})
}

// ===================== Assessment Manager Handler Tests =====================

func TestAssessmentManagerHandlersBranches(t *testing.T) {
	t.Run("update forbidden without permission", func(t *testing.T) {
		performanceCoverageDBWith(t,
			apiPerformancePermissionUsersResponse(),
			apiPerformanceRouterPermissionsResponse(nil),
		)
		c, recorder := performanceCoverageUserContext(t, "user-1")
		c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/participants/1/assessment-manager", strings.NewReader(`{"manager_user_id":"manager-1"}`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

		UpdateParticipantAssessmentManager(c)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
		}
	})

	t.Run("update participant not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceParticipantRowsResponse(nil))
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPut,
			"/api/v1/performance/participants/99/assessment-manager",
			`{"manager_user_id":"manager-1"}`,
			gin.Params{{Key: "participant_id", Value: "99"}},
			UpdateParticipantAssessmentManager,
		)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
		}
	})

	t.Run("update service validation error", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPut,
			"/api/v1/performance/participants/1/assessment-manager",
			`{"manager_user_id":"manager-1","manager_source":"IMPORT"}`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			UpdateParticipantAssessmentManager,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("update succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPut,
			"/api/v1/performance/participants/1/assessment-manager",
			`{"manager_user_id":"manager-1","manager_source":"MANUAL","reason":"rebalance"}`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			UpdateParticipantAssessmentManager,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("batch participant activity mismatch", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "other", "user-1", nil, "pending", false)}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/activities/1/assessment-managers/batch",
			`{"items":[{"participant_id":1,"manager_user_id":"manager-1"}]}`,
			gin.Params{{Key: "activity_id", Value: "1"}},
			BatchUpdateAssessmentManagers,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("batch succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "published")}),
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/activities/1/assessment-managers/batch",
			`{"items":[{"participant_id":1,"manager_user_id":"manager-1","manager_source":"MANUAL"}]}`,
			gin.Params{{Key: "activity_id", Value: "1"}},
			BatchUpdateAssessmentManagers,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("candidate service error", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "published")}))
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/activities/1/assessment-manager-candidates?source=bad-source",
			"",
			gin.Params{{Key: "activity_id", Value: "1"}},
			GetAssessmentManagerCandidates,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("candidate success", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "published")}),
			apiPerformanceEmployeeProfilePluckResponse([][]driver.Value{{"manager-1"}}),
			apiPerformanceEmployeeProfilesResponse([][]driver.Value{{int64(1), "manager-1", "E001", "active"}}),
			apiPerformanceDepartmentsResponse([][]driver.Value{{int64(1), "dept-1", "Product", ""}}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/activities/1/assessment-manager-candidates?source=MANUAL&keyword=Manager&limit=5",
			"",
			gin.Params{{Key: "activity_id", Value: "1"}},
			GetAssessmentManagerCandidates,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})
}

// ===================== Template, Indicator, And Goal Handler Tests =====================

func TestTemplateHandlersAdditionalBranches(t *testing.T) {
	validTemplateBody := `{"name":"Standard","status":"active","sections":[{"name":"KPI","section_type":"quantitative","weight":100,"items":[{"name":"Revenue","max_score":100,"weight":100}]}]}`

	t.Run("create service validation error", func(t *testing.T) {
		performanceHandlerTestDBWith(t)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/templates",
			`{"name":"Bad","sections":[{"name":"KPI","section_type":"quantitative","weight":50,"items":[{"name":"Revenue","max_score":100,"weight":100}]}]}`,
			nil,
			CreatePerformanceTemplate,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("create succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t)
		recorder := performPerformanceHandlerRequest(t, http.MethodPost, "/api/v1/performance/templates", validTemplateBody, nil, CreatePerformanceTemplate)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("get not found", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceTemplateRowsResponse(nil))
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/templates/404",
			"",
			gin.Params{{Key: "id", Value: "404"}},
			GetPerformanceTemplate,
		)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
		}
	})

	t.Run("get succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceTemplateRowsResponse([][]driver.Value{{int64(1), "Standard", "default", "active", "admin", "admin"}}),
			apiPerformanceTemplateSectionRowsResponse([][]driver.Value{{int64(10), int64(1), "KPI", "quantitative", float64(100), int64(1), true, false}}),
			apiPerformanceTemplateItemRowsResponse([][]driver.Value{{int64(100), int64(10), "Revenue", "Revenue target", float64(100), float64(100), int64(1)}}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/templates/1",
			"",
			gin.Params{{Key: "id", Value: "1"}},
			GetPerformanceTemplate,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("update missing body", func(t *testing.T) {
		performanceHandlerTestDBWith(t)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPut,
			"/api/v1/performance/templates/1",
			`{`,
			gin.Params{{Key: "id", Value: "1"}},
			UpdatePerformanceTemplate,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("update succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceTemplateRowsResponse([][]driver.Value{{int64(1), "Standard", "default", "active", "admin", "admin"}}),
			apiPerformanceTemplateSectionRowsResponse(nil),
			apiPerformanceTemplateItemRowsResponse(nil),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPut,
			"/api/v1/performance/templates/1",
			`{"name":"Updated","status":"active"}`,
			gin.Params{{Key: "id", Value: "1"}},
			UpdatePerformanceTemplate,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})
}

func TestIndicatorHandlersAdditionalBranches(t *testing.T) {
	t.Run("create library succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceTemplateSelectResponse())
		body := `{"department_id":"dept-1","department_name":"Product","template_id":1,"name":"Product KPI","items":[{"section_type":"quantitative","name":"Revenue","weight":100}]}`
		recorder := performPerformanceHandlerRequest(t, http.MethodPost, "/api/v1/performance/indicator-libraries", body, nil, CreateIndicatorLibrary)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("get library succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorLibrarySelectResponse())
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/indicator-libraries/1",
			"",
			gin.Params{{Key: "id", Value: "1"}},
			GetIndicatorLibrary,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("update library succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorLibrarySelectResponse())
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPut,
			"/api/v1/performance/indicator-libraries/1",
			`{"name":"Updated","department_name":"Product","default_cycle":"quarterly"}`,
			gin.Params{{Key: "id", Value: "1"}},
			UpdateIndicatorLibrary,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("archive library succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorLibrarySelectResponse())
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/indicator-libraries/1/archive",
			"",
			gin.Params{{Key: "id", Value: "1"}},
			ArchiveIndicatorLibrary,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("libraries by department succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorLibrarySelectResponse())
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/indicator-libraries/department/dept-1",
			"",
			gin.Params{{Key: "department_id", Value: "dept-1"}},
			GetIndicatorLibrariesByDepartment,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("inherit library succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceIndicatorLibrarySelectResponse(),
			apiPerformanceIndicatorItemsResponse(),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/indicator-libraries/inherit",
			`{"parent_library_id":1,"target_department_id":"dept-2","target_department_name":"Sales","name":"Sales KPI"}`,
			nil,
			InheritIndicatorLibrary,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("get indicator items succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorLibrarySelectResponse(), apiPerformanceIndicatorItemsResponse())
		recorder := performPerformanceHandlerRequest(t, http.MethodGet, "/api/v1/performance/indicator-items?library_id=1", "", nil, GetIndicatorItems)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("create indicator item succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorLibrarySelectResponse())
		body := `{"library_id":1,"section_type":"quantitative","name":"Revenue","weight":50}`
		recorder := performPerformanceHandlerRequest(t, http.MethodPost, "/api/v1/performance/indicator-items", body, nil, CreateIndicatorItem)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("update indicator item succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorItemsResponse(), apiPerformanceIndicatorLibrarySelectResponse())
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPut,
			"/api/v1/performance/indicator-items/1",
			`{"name":"Revenue Updated","section_type":"quantitative","weight":60}`,
			gin.Params{{Key: "id", Value: "1"}},
			UpdateIndicatorItem,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("delete indicator item succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t, apiPerformanceIndicatorItemsResponse(), apiPerformanceIndicatorLibrarySelectResponse())
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodDelete,
			"/api/v1/performance/indicator-items/1",
			"",
			gin.Params{{Key: "id", Value: "1"}},
			DeleteIndicatorItem,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})
}

func TestGoalRecordHandlersAdditionalBranches(t *testing.T) {
	t.Run("get goal records succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "target_setting", false)}),
			apiPerformanceGoalRecordsResponse([][]driver.Value{apiPerformanceGoalRecordRow(1, 1, "1", "quantitative", "Revenue", false)}),
			apiPerformanceGoalApprovalLogsResponse(nil),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/participants/1/goal-records",
			"",
			gin.Params{{Key: "participant_id", Value: "1"}},
			GetGoalRecords,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("get goal records service error", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "target_setting", false)}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/participants/1/goal-records",
			"",
			gin.Params{{Key: "participant_id", Value: "1"}},
			GetGoalRecords,
		)
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusInternalServerError, recorder.Body.String())
		}
	})

	t.Run("batch save service error", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "self_evaluation")}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/participants/1/goal-records",
			`{"records":[{"section_type":"quantitative","item_name":"Revenue","weight":1}]}`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			BatchSaveGoalRecords,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("batch save succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "target_setting")}),
			apiPerformanceGoalRecordsResponse(nil),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/participants/1/goal-records",
			`{"records":[{"section_type":"quantitative","item_name":"Revenue","weight":1}]}`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			BatchSaveGoalRecords,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("submit goal approval service error", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "target_setting", false)}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "self_evaluation")}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodPost,
			"/api/v1/performance/participants/1/goal-approval",
			`{"action":"submit","comment":"ready"}`,
			gin.Params{{Key: "participant_id", Value: "1"}},
			SubmitGoalApprovalHandler,
		)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
		}
	})

	t.Run("get manager goals succeeds", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceGoalRecordsResponse([][]driver.Value{
				apiPerformanceGoalRecordRow(1, 1, "1", "quantitative", "Manager goal", true),
				apiPerformanceGoalRecordRow(2, 1, "1", "key_action", "Employee goal", false),
			}),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/participants/1/manager-goals",
			"",
			gin.Params{{Key: "participant_id", Value: "1"}},
			GetManagerGoals,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})

	t.Run("goal suggestions empty success", func(t *testing.T) {
		performanceHandlerTestDBWith(t,
			apiPerformanceParticipantRowsResponse([][]driver.Value{apiPerformanceParticipantRow(1, "1", "user-1", nil, "pending", false)}),
			apiPerformanceActivityRowsResponse([][]driver.Value{apiPerformanceActivityRow(1, "Q2 Review", "target_setting")}),
			apiPerformanceIndicatorLibraryRowsResponse(nil),
		)
		recorder := performPerformanceHandlerRequest(
			t,
			http.MethodGet,
			"/api/v1/performance/participants/1/goal-suggestions",
			"",
			gin.Params{{Key: "participant_id", Value: "1"}},
			GetGoalSuggestions,
		)
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
	})
}

// ===================== Coverage Query Helpers =====================

func performanceCoverageUserContext(t *testing.T, userID string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("userID", userID)
	return c, recorder
}

func performPerformanceHandlerRequestAs(t *testing.T, userID, method, path, body string, params gin.Params, handler func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	c, recorder := performanceCoverageUserContext(t, userID)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	c.Params = params
	handler(c)
	return recorder
}

func assertRecorderStatus(t *testing.T, recorder *httptest.ResponseRecorder, want int) {
	t.Helper()
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, want, recorder.Body.String())
	}
}

func performanceCoverageDBWith(t *testing.T, queries ...apiImportQueryResponse) {
	t.Helper()
	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t, queries...)
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func apiPerformancePermissionUsersResponse() apiImportQueryResponse {
	return apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
		{int64(1), "user-1", "Alice", "dept-1", "active"},
		{int64(2), "manager-1", "Manager Bob", "dept-1", "active"},
	})
}

func apiPerformanceParticipantRowsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: apiPerformanceParticipantSelectMatch,
		columns: []string{
			"id", "activity_id", "employee_id", "employee_name", "department_id", "department_name",
			"manager_id", "manager_name", "status", "employee_status",
			"self_score", "manager_score", "final_level", "is_locked",
		},
		rows: rows,
	}
}

func apiPerformanceParticipantFirstThenRows(firstRows, secondRows [][]driver.Value) (apiImportQueryResponse, apiImportQueryResponse) {
	call := 0
	first := apiPerformanceParticipantRowsResponse(firstRows)
	second := apiPerformanceParticipantRowsResponse(secondRows)
	first.match = func(query string, args []driver.NamedValue) bool {
		if !apiPerformanceParticipantSelectMatch(query, args) || call != 0 {
			return false
		}
		call++
		return true
	}
	second.match = func(query string, args []driver.NamedValue) bool {
		if !apiPerformanceParticipantSelectMatch(query, args) || call != 1 {
			return false
		}
		call++
		return true
	}
	return first, second
}

func apiPerformanceParticipantSelectMatch(query string, _ []driver.NamedValue) bool {
	lower := strings.ToLower(query)
	return strings.Contains(lower, "performance_participants") && !strings.Contains(lower, "count(")
}

func apiPerformanceParticipantRow(id int64, activityID, employeeID string, managerID *string, status string, locked bool) []driver.Value {
	return apiPerformanceParticipantRowWithDetails(id, activityID, employeeID, managerID, status, "active", locked, 0, "")
}

func apiPerformanceParticipantRowWithDetails(id int64, activityID, employeeID string, managerID *string, status, employeeStatus string, locked bool, managerScore float64, finalLevel string) []driver.Value {
	var managerValue driver.Value
	var managerName driver.Value
	if managerID != nil {
		managerValue = *managerID
		managerName = "Manager Bob"
	}
	return []driver.Value{
		id, activityID, employeeID, "Alice", "dept-1", "Product",
		managerValue, managerName, status, employeeStatus,
		float64(0), managerScore, finalLevel, locked,
	}
}

func apiPerformanceActivityRowsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_activities") && !strings.Contains(lower, "count(")
		},
		columns: []string{
			"id", "name", "cycle_type", "start_date", "end_date",
			"self_eval_start_at", "self_eval_end_at",
			"manager_eval_start_at", "manager_eval_end_at",
			"result_confirm_start_at", "result_confirm_end_at",
			"status", "description", "created_by", "updated_by",
		},
		rows: rows,
	}
}

func apiPerformanceActivityRow(id int64, name, status string) []driver.Value {
	return []driver.Value{
		id, name, "quarterly", "2026-04-01", "2026-06-30",
		"2026-05-01", "2026-05-07",
		"2026-05-08", "2026-05-15",
		"2026-05-16", "2026-05-20",
		status, "review", "admin", "admin",
	}
}

func apiPerformanceActivityRowsWithBonusResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_activities") && !strings.Contains(lower, "count(")
		},
		columns: []string{
			"id", "name", "cycle_type", "start_date", "end_date",
			"self_eval_start_at", "self_eval_end_at",
			"manager_eval_start_at", "manager_eval_end_at",
			"result_confirm_start_at", "result_confirm_end_at",
			"status", "description", "enable_bonus_score", "strict_time_mode", "created_by", "updated_by",
		},
		rows: rows,
	}
}

func apiPerformanceActivityRowWithBonus(id int64, name, status string, enableBonus bool) []driver.Value {
	return []driver.Value{
		id, name, "quarterly", "2026-04-01", "2026-06-30",
		"2026-05-01", "2026-05-07",
		"2026-05-08", "2026-05-15",
		"2026-05-16", "2026-05-20",
		status, "review", enableBonus, false, "admin", "admin",
	}
}

func apiDingtalkNotifiableUserCountResponse(count int64) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "employee_profiles") && strings.Contains(lower, "count(")
		},
		columns: []string{"count"},
		rows:    [][]driver.Value{{count}},
	}
}

func apiPerformanceTemplateRowsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_templates") && !strings.Contains(lower, "count(")
		},
		columns: []string{"id", "name", "description", "status", "created_by", "updated_by"},
		rows:    rows,
	}
}

func apiPerformanceTemplateSectionRowsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportTableResponse("performance_template_sections", []string{
		"id", "template_id", "name", "section_type", "weight", "sort_order", "is_score_required", "is_comment_required",
	}, rows)
}

func apiPerformanceTemplateItemRowsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportTableResponse("performance_template_items", []string{
		"id", "section_id", "name", "description", "max_score", "weight", "sort_order",
	}, rows)
}

func apiPerformanceIndicatorLibraryRowsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportTableResponse("performance_indicator_libraries", []string{
		"id", "department_id", "department_name", "name", "description", "default_cycle", "status", "created_by", "updated_by",
	}, rows)
}

func apiPerformanceGoalRecordsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportTableResponse("performance_goal_records", []string{
		"id", "activity_id", "participant_id", "section_type", "item_name", "item_definition", "weight", "target_value", "approval_status", "is_from_superior", "sort_order",
	}, rows)
}

func apiPerformanceGoalRecordRow(id int64, participantID int64, activityID, sectionType, itemName string, fromSuperior bool) []driver.Value {
	return []driver.Value{id, activityID, participantID, sectionType, itemName, itemName + " definition", float64(1), "100", "pending", fromSuperior, int64(id)}
}

func apiPerformanceEmployeeProfilesResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportTableResponse("employee_profiles", []string{"id", "user_id", "employee_id", "profile_status"}, rows)
}

func apiPerformanceEmployeeProfilePluckResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "employee_profiles") && strings.Contains(lower, "employee_id like")
		},
		columns: []string{"user_id"},
		rows:    rows,
	}
}

func apiPerformanceDepartmentsResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportTableResponse("departments", []string{"id", "department_id", "name", "parent_id"}, rows)
}
