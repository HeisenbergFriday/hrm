package api

import (
	"database/sql/driver"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
)

// ===================== Test Helpers =====================

func performanceHandlerTestDB(t *testing.T) {
	t.Helper()
	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
			{int64(1), "admin", "Admin", "dept-1", "active"},
			{int64(2), "user-1", "Alice", "dept-1", "active"},
		}),
		apiPerformanceActivitySelectResponse("self_evaluation"),
		// 绩效参与人表 - 用于 participant lookup
		apiImportTableResponse("performance_participants", []string{"id", "activity_id", "employee_id", "employee_name", "department_id", "department_name", "manager_id", "manager_name", "status", "self_score", "manager_score", "final_level"}, [][]driver.Value{
			{int64(1), "1", "user-1", "Alice", "dept-1", "Product", ptrString("manager-1"), ptrString("Manager Bob"), "pending", float64(0), float64(0), ""},
		}),
	)
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func ptrString(s string) *string {
	return &s
}

func performanceHandlerAdminContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("userID", "admin")
	c.Set("orgID", database.DefaultOrganizationID)
	c.Request = performanceTestRequest(http.MethodGet, "/", nil)
	return c, recorder
}

func performanceTestRequest(method, target string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, target, body)
	info := &requestmeta.RequestInfo{OrgID: database.DefaultOrganizationID}
	ctx := requestmeta.WithRequestInfo(req.Context(), info)
	ctx = requestmeta.WithTenant(ctx, database.DefaultOrganizationID)
	return req.WithContext(ctx)
}

// ===================== Input Validation Tests (no permission check needed for admin) =====================

func TestCreatePerformanceActivityHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"name": "test"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreatePerformanceActivity(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestUpdatePerformanceActivityHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"name": "test"}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/activities/1", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	UpdatePerformanceActivity(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestPutDistributionRulesHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/activities/1/distribution-rules", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	PutDistributionRules(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestPutDistributionRulesHandlerInvalidBody(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"rules": "not an array"}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/activities/1/distribution-rules", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	PutDistributionRules(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestUpdateParticipantAssessmentManagerHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"manager_user_id": "manager-1"}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/participants/invalid/assessment-manager", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	UpdateParticipantAssessmentManager(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestUpdateParticipantAssessmentManagerHandlerMissingBody(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/participants/1/assessment-manager", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	UpdateParticipantAssessmentManager(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetAssessmentManagerCandidatesHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/activities/1/assessment-manager-candidates?participant_id=invalid", nil)
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	GetAssessmentManagerCandidates(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSubmitSelfEvaluationHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"self_score": 80}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/self-evaluation", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	SubmitSelfEvaluation(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSubmitManagerEvaluationHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"manager_score": 85}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/manager-evaluation", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	SubmitManagerEvaluation(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestAdjustFinalLevelHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"final_level": "A"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/adjust-level", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	AdjustFinalLevel(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetGoalRecordsHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/participants/invalid/goal-records", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	GetGoalRecords(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestBatchSaveGoalRecordsHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"records": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/invalid/goal-records", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	BatchSaveGoalRecords(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestBatchSaveGoalRecordsHandlerEmptyRecords(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"records": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/goal-records", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	BatchSaveGoalRecords(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSubmitGoalApprovalHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"action": "submit"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/invalid/goal-approval", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	SubmitGoalApprovalHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestRejectGoalRecordsHandlerMissingComment(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/reject-goals", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	RejectGoalRecords(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetManagerGoalsHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/participants/invalid/manager-goals", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	GetManagerGoals(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetGoalSuggestionsHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/participants/invalid/goal-suggestions", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	GetGoalSuggestions(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestBatchAssignGoalsHandlerMissingBody(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"participant_ids": [1], "targets": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/batch-assign-goals", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	BatchAssignGoals(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestBatchAssignGoalsHandlerInvalidBody(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"participant_ids": "not an array", "targets": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/batch-assign-goals", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	BatchAssignGoals(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSubmitGoalSelfEvaluationHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"items": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/invalid/goal-self-evaluation", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	SubmitGoalSelfEvaluationHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSubmitGoalManagerEvaluationHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"items": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/invalid/goal-manager-evaluation", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	SubmitGoalManagerEvaluationHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSetBonusPenaltyScoreHandlerInvalidParticipantID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"bonus_score": 10}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/invalid/bonus-penalty-score", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "invalid"}}

	SetBonusPenaltyScoreHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestTriggerPerformanceInterviewHandlerMissingType(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/trigger-interview", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	TriggerPerformanceInterview(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSetHRConfirmDeadlineHandlerMissingDeadline(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/hr-confirm-deadline", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	SetHRConfirmDeadlineHandler(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestCreatePerformanceTemplateHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"description": "test"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/templates", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreatePerformanceTemplate(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetPerformanceTemplateHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/templates/invalid", nil)
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	GetPerformanceTemplate(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestUpdatePerformanceTemplateHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"name": "updated"}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/templates/invalid", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	UpdatePerformanceTemplate(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestCreateIndicatorLibraryHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"name": "test"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/indicator-libraries", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateIndicatorLibrary(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestCreateIndicatorLibraryHandlerEmptyItems(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"department_id": "dept-1", "department_name": "Product", "name": "test", "items": []}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/indicator-libraries", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateIndicatorLibrary(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetIndicatorLibraryHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/indicator-libraries/invalid", nil)
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	GetIndicatorLibrary(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestUpdateIndicatorLibraryHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"name": "updated"}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/indicator-libraries/invalid", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	UpdateIndicatorLibrary(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestArchiveIndicatorLibraryHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/indicator-libraries/invalid/archive", nil)
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	ArchiveIndicatorLibrary(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetIndicatorLibrariesByDepartmentHandlerEmptyDepartmentID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/indicator-libraries/department/", nil)
	c.Params = gin.Params{{Key: "department_id", Value: ""}}

	GetIndicatorLibrariesByDepartment(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestInheritIndicatorLibraryHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"parent_library_id": 1}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/indicator-libraries/inherit", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	InheritIndicatorLibrary(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestGetIndicatorItemsHandlerInvalidLibraryID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/indicator-items?library_id=invalid", nil)

	GetIndicatorItems(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestCreateIndicatorItemHandlerMissingRequired(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"library_id": 1}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/indicator-items", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	CreateIndicatorItem(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestUpdateIndicatorItemHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"name": "updated"}`
	c.Request = httptest.NewRequest(http.MethodPut, "/api/v1/performance/indicator-items/invalid", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	UpdateIndicatorItem(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestDeleteIndicatorItemHandlerInvalidID(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodDelete, "/api/v1/performance/indicator-items/invalid", nil)
	c.Params = gin.Params{{Key: "id", Value: "invalid"}}

	DeleteIndicatorItem(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestBatchConfirmResultsHandlerInvalidBody(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"participant_ids": "not an array"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/batch-confirm-results", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	BatchConfirmResults(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestBatchSubmitManagerEvaluationHandlerInvalidBody(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"evaluations": "not an array"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/activities/1/batch-manager-evaluation", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "activity_id", Value: "1"}}

	BatchSubmitManagerEvaluation(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

func TestSubmitReviewManagerEvaluationHandlerMissingFinalLevel(t *testing.T) {
	performanceHandlerTestDB(t)
	c, recorder := performanceHandlerAdminContext(t)
	body := `{"manager_comment": "表现不错"}`
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/participants/1/review-manager-evaluation", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "participant_id", Value: "1"}}

	SubmitReviewManagerEvaluation(c)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
}

// ===================== Happy Path Handler Tests =====================

func TestPerformanceActivityQueryHandlersHappyPath(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		params  gin.Params
		handler func(*gin.Context)
		queries []apiImportQueryResponse
	}{
		{
			name:    "list activities",
			method:  http.MethodGet,
			path:    "/api/v1/performance/activities?page=1&page_size=10",
			handler: GetPerformanceActivities,
			queries: []apiImportQueryResponse{
				apiPerformanceActivityCountResponse(1),
				apiPerformanceActivitySelectResponse("self_evaluation"),
			},
		},
		{
			name:    "get activity",
			method:  http.MethodGet,
			path:    "/api/v1/performance/activities/1",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: GetPerformanceActivity,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("self_evaluation"),
			},
		},
		{
			name:    "publish activity idempotently",
			method:  http.MethodPost,
			path:    "/api/v1/performance/activities/1/publish",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: PublishPerformanceActivity,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("self_evaluation"),
			},
		},
		{
			name:    "close archived activity idempotently",
			method:  http.MethodPost,
			path:    "/api/v1/performance/activities/1/close",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: ClosePerformanceActivity,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("archived"),
			},
		},
		{
			name:    "start activity idempotently",
			method:  http.MethodPost,
			path:    "/api/v1/performance/activities/1/start",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: StartPerformanceActivity,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("target_setting"),
			},
		},
		{
			name:    "open target setting idempotently",
			method:  http.MethodPost,
			path:    "/api/v1/performance/activities/1/open-target-setting",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: OpenTargetSettingHandler,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("target_setting"),
			},
		},
		{
			name:    "archive activity idempotently",
			method:  http.MethodPost,
			path:    "/api/v1/performance/activities/1/archive",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: ArchivePerformanceActivity,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("archived"),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			performanceHandlerTestDBWith(t, tt.queries...)
			recorder := performPerformanceHandlerRequest(t, tt.method, tt.path, "", tt.params, tt.handler)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
		})
	}
}

func TestPerformanceParticipantAndDistributionHandlersHappyPath(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		params  gin.Params
		handler func(*gin.Context)
		queries []apiImportQueryResponse
	}{
		{
			name:    "list participants",
			path:    "/api/v1/performance/activities/1/participants",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: GetPerformanceParticipants,
			queries: []apiImportQueryResponse{
				apiPerformanceParticipantCountResponse(1),
				apiPerformanceParticipantSelectResponse("self_submitted"),
			},
		},
		{
			name:    "get participant",
			path:    "/api/v1/performance/participants/1",
			params:  gin.Params{{Key: "participant_id", Value: "1"}},
			handler: GetParticipant,
			queries: []apiImportQueryResponse{
				apiPerformanceParticipantSelectResponse("self_submitted"),
				apiPerformanceGoalApprovalLogsResponse(nil),
				apiPerformanceActivitySelectResponse("self_evaluation"),
			},
		},
		{
			name:    "get distribution rules",
			path:    "/api/v1/performance/activities/1/distribution-rules",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: GetDistributionRules,
			queries: []apiImportQueryResponse{
				apiPerformanceDistributionRulesResponse(),
			},
		},
		{
			name:    "get realtime distribution check",
			path:    "/api/v1/performance/activities/1/realtime-distribution-check",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: GetRealtimeDistributionCheck,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("manager_evaluation"),
				apiPerformanceParticipantCountResponse(1),
				apiPerformanceParticipantSelectResponse("manager_submitted"),
				apiPerformanceDistributionRulesResponse(),
			},
		},
		{
			name:    "get result summary",
			path:    "/api/v1/performance/activities/1/result-summary",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: GetPerformanceResultSummary,
			queries: []apiImportQueryResponse{
				apiPerformanceParticipantSelectResponse("manager_submitted"),
			},
		},
		{
			name:    "get distribution check",
			path:    "/api/v1/performance/activities/1/distribution-check",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: GetPerformanceDistributionCheck,
			queries: []apiImportQueryResponse{
				apiPerformanceActivitySelectResponse("manager_evaluation"),
				apiPerformanceParticipantSelectResponse("manager_submitted"),
				apiPerformanceDistributionRulesResponse(),
			},
		},
		{
			name:    "get participant versions",
			path:    "/api/v1/performance/participants/1/versions",
			params:  gin.Params{{Key: "participant_id", Value: "1"}},
			handler: GetParticipantVersions,
			queries: []apiImportQueryResponse{
				apiPerformanceReviewVersionsResponse(),
			},
		},
		{
			name:    "get participant relationship logs",
			path:    "/api/v1/performance/participants/1/relationship-change-logs",
			params:  gin.Params{{Key: "participant_id", Value: "1"}},
			handler: GetParticipantRelationshipChangeLogs,
			queries: []apiImportQueryResponse{
				apiPerformanceRelationshipLogsResponse(),
			},
		},
		{
			name:    "get activity relationship logs",
			path:    "/api/v1/performance/activities/1/relationship-change-logs",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: GetActivityRelationshipChangeLogs,
			queries: []apiImportQueryResponse{
				apiPerformanceRelationshipLogsResponse(),
			},
		},
		{
			name:    "send manager reminders with no recipients",
			path:    "/api/v1/performance/activities/1/send-manager-eval-reminder",
			params:  gin.Params{{Key: "activity_id", Value: "1"}},
			handler: SendManagerEvalReminder,
			queries: []apiImportQueryResponse{
				apiPerformanceParticipantEmptyResponse(),
			},
		},
		{
			name:    "get indicator libraries",
			path:    "/api/v1/performance/indicator-libraries",
			handler: GetIndicatorLibraries,
			queries: []apiImportQueryResponse{
				apiPerformanceIndicatorLibraryCountResponse(1),
				apiPerformanceIndicatorLibrarySelectResponse(),
			},
		},
		{
			name:    "search indicator items",
			path:    "/api/v1/performance/indicator-items/search?keyword=Revenue&library_ids=1&section_type=quantitative",
			handler: SearchIndicatorItems,
			queries: []apiImportQueryResponse{
				apiPerformanceIndicatorItemsResponse(),
			},
		},
		{
			name:    "get templates",
			path:    "/api/v1/performance/templates",
			handler: GetPerformanceTemplates,
			queries: []apiImportQueryResponse{
				apiPerformanceTemplateCountResponse(1),
				apiPerformanceTemplateSelectResponse(),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			performanceHandlerTestDBWith(t, tt.queries...)
			recorder := performPerformanceHandlerRequest(t, http.MethodGet, tt.path, "", tt.params, tt.handler)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
		})
	}
}

func TestSendSelfEvalReminderHandlerNoRecipients(t *testing.T) {
	performanceHandlerTestDBWith(t,
		apiPerformanceActivitySelectResponse("self_evaluation"),
		apiPerformanceParticipantEmptyResponse(),
	)
	recorder := performPerformanceHandlerRequest(
		t,
		http.MethodPost,
		"/api/v1/performance/activities/1/send-self-eval-reminder",
		"",
		gin.Params{{Key: "activity_id", Value: "1"}},
		SendSelfEvalReminder,
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"pending":0`) || !strings.Contains(recorder.Body.String(), `"candidates":0`) {
		t.Fatalf("body does not include reminder result counts: %s", recorder.Body.String())
	}
}

func TestGetMyPerformanceParticipantsBatch(t *testing.T) {
	performanceHandlerUserTestDBWith(t,
		apiPerformanceEmployeeProfilesResponse([][]driver.Value{
			{int64(10), "user-1", "E001", "active"},
		}),
		apiPerformanceParticipantRowsResponse([][]driver.Value{
			apiPerformanceParticipantRowWithDetails(1, "1", "user-1", ptrString("manager-1"), "target_set", "active", false, 0, ""),
		}),
	)
	c, recorder := performanceHandlerAdminContext(t)
	c.Set("userID", "user-1")
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/participants/my?activity_ids=1,2", nil)

	GetMyPerformanceParticipants(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"items_by_activity"`) || !strings.Contains(recorder.Body.String(), `"1"`) {
		t.Fatalf("body = %s, want items_by_activity with activity 1", recorder.Body.String())
	}
}

func TestAttachMyParticipantToActivities(t *testing.T) {
	performanceHandlerUserTestDBWith(t,
		apiPerformanceEmployeeProfilesResponse([][]driver.Value{
			{int64(10), "user-1", "E001", "active"},
		}),
		apiPerformanceParticipantRowsResponse([][]driver.Value{
			apiPerformanceParticipantRowWithDetails(1, "1", "E001", ptrString("manager-1"), "target_set", "active", false, 0, ""),
		}),
	)
	c, _ := performanceHandlerAdminContext(t)
	c.Set("userID", "user-1")

	items, err := attachMyParticipantToActivities(c, []database.PerformanceActivity{
		{ID: 1, Name: "Q2 绩效"},
		{ID: 2, Name: "Q3 绩效"},
	})
	if err != nil {
		t.Fatalf("attachMyParticipantToActivities() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if items[0].MyParticipant == nil || items[0].MyParticipant.EmployeeID != "E001" {
		t.Fatalf("items[0].MyParticipant = %#v, want employee E001", items[0].MyParticipant)
	}
	if items[1].MyParticipant != nil {
		t.Fatalf("items[1].MyParticipant = %#v, want nil", items[1].MyParticipant)
	}
}

func performanceHandlerUserTestDBWith(t *testing.T, queries ...apiImportQueryResponse) {
	t.Helper()
	originalDB := database.DB
	baseQueries := []apiImportQueryResponse{apiPerformanceUsersWithUserFirstResponse()}
	baseQueries = append(baseQueries, queries...)
	database.DB = newAPIPerformanceImportStubDB(t, baseQueries...)
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func performanceHandlerTestDBWith(t *testing.T, queries ...apiImportQueryResponse) {
	t.Helper()
	originalDB := database.DB
	baseQueries := []apiImportQueryResponse{apiPerformanceUsersResponse()}
	baseQueries = append(baseQueries, queries...)
	baseQueries = append(baseQueries,
		apiPerformanceActivitySelectResponse("self_evaluation"),
		apiPerformanceParticipantSelectResponse("manager_submitted"),
	)
	database.DB = newAPIPerformanceImportStubDB(t, baseQueries...)
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func performPerformanceHandlerRequest(t *testing.T, method, path, body string, params gin.Params, handler func(*gin.Context)) *httptest.ResponseRecorder {
	t.Helper()
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = performanceTestRequest(method, path, strings.NewReader(body))
	if body != "" {
		c.Request.Header.Set("Content-Type", "application/json")
	}
	c.Params = params
	handler(c)
	return recorder
}

func apiPerformanceUsersResponse() apiImportQueryResponse {
	return apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
		{int64(1), "admin", "Admin", "dept-1", "active"},
		{int64(2), "user-1", "Alice", "dept-1", "active"},
		{int64(3), "manager-1", "Manager Bob", "dept-1", "active"},
	})
}

func apiPerformanceUsersWithUserFirstResponse() apiImportQueryResponse {
	return apiImportTableResponse("users", []string{"id", "user_id", "name", "department_id", "status"}, [][]driver.Value{
		{int64(2), "user-1", "Alice", "dept-1", "active"},
		{int64(1), "admin", "Admin", "dept-1", "active"},
		{int64(3), "manager-1", "Manager Bob", "dept-1", "active"},
	})
}

func apiPerformanceActivityCountResponse(total int64) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_activities") && strings.Contains(lower, "count(")
		},
		columns: []string{"count"},
		rows:    [][]driver.Value{{total}},
	}
}

func apiPerformanceActivitySelectResponse(status string) apiImportQueryResponse {
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
		rows: [][]driver.Value{{
			int64(1), "Q2 Review", "quarterly", "2026-04-01", "2026-06-30",
			"2026-05-01", "2026-05-07",
			"2026-05-08", "2026-05-15",
			"2026-05-16", "2026-05-20",
			status, "review", "admin", "admin",
		}},
	}
}

func apiPerformanceParticipantCountResponse(total int64) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_participants") && strings.Contains(lower, "count(")
		},
		columns: []string{"count"},
		rows:    [][]driver.Value{{total}},
	}
}

func apiPerformanceParticipantSelectResponse(status string) apiImportQueryResponse {
	managerID := "manager-1"
	managerName := "Manager Bob"
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_participants") && !strings.Contains(lower, "count(")
		},
		columns: []string{
			"id", "activity_id", "employee_id", "employee_name", "department_id", "department_name",
			"manager_id", "manager_name", "status", "employee_status",
			"self_score", "manager_score", "final_level", "is_locked",
		},
		rows: [][]driver.Value{{
			int64(1), "1", "user-1", "Alice", "dept-1", "Product",
			managerID, managerName, status, "active",
			float64(80), float64(90), "A", false,
		}},
	}
}

func apiPerformanceParticipantEmptyResponse() apiImportQueryResponse {
	return apiImportQueryResponse{
		match:   apiPerformanceParticipantSelectResponse("").match,
		columns: []string{"id", "activity_id", "employee_id", "employee_name", "status"},
		rows:    nil,
	}
}

func apiPerformanceDistributionRulesResponse() apiImportQueryResponse {
	return apiImportTableResponse("performance_distribution_rules", []string{"id", "activity_id", "level", "distribution_percent", "description"}, [][]driver.Value{
		{int64(1), "1", "A", float64(30), "top"},
		{int64(2), "1", "B", float64(70), "normal"},
	})
}

func apiPerformanceGoalApprovalLogsResponse(rows [][]driver.Value) apiImportQueryResponse {
	if rows == nil {
		rows = [][]driver.Value{}
	}
	return apiImportTableResponse("performance_goal_approval_logs", []string{"id", "participant_id", "activity_id", "action", "approver_id", "approver_name", "created_by"}, rows)
}

func apiPerformanceReviewVersionsResponse() apiImportQueryResponse {
	return apiImportTableResponse("performance_review_versions", []string{"id", "participant_id", "activity_id", "review_type", "created_by"}, [][]driver.Value{
		{int64(1), int64(1), "1", "self", "user-1"},
	})
}

func apiPerformanceRelationshipLogsResponse() apiImportQueryResponse {
	return apiImportTableResponse("performance_relationship_change_logs", []string{"id", "activity_id", "participant_id", "user_id", "change_type", "changed_at"}, [][]driver.Value{
		{int64(1), "1", int64(1), "user-1", "assessment_manager_changed", time.Now()},
	})
}

func apiPerformanceIndicatorLibraryCountResponse(total int64) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_indicator_libraries") && strings.Contains(lower, "count(")
		},
		columns: []string{"count"},
		rows:    [][]driver.Value{{total}},
	}
}

func apiPerformanceIndicatorLibrarySelectResponse() apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_indicator_libraries") && !strings.Contains(lower, "count(")
		},
		columns: []string{"id", "department_id", "department_name", "name", "description", "default_cycle", "status", "created_by", "updated_by"},
		rows: [][]driver.Value{
			{int64(1), "dept-1", "Product", "Product KPI", "Quarterly KPIs", "quarterly", "active", "admin", "admin"},
		},
	}
}

func apiPerformanceIndicatorItemsResponse() apiImportQueryResponse {
	return apiImportTableResponse("performance_indicator_items", []string{
		"id", "library_id", "section_type", "name", "description", "indicator_type", "default_weight", "weight", "is_default", "sort_order",
	}, [][]driver.Value{
		{int64(1), int64(1), "quantitative", "Revenue", "Revenue target", "quantitative", float64(50), float64(50), true, int64(1)},
	})
}

func apiPerformanceTemplateCountResponse(total int64) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_templates") && strings.Contains(lower, "count(")
		},
		columns: []string{"count"},
		rows:    [][]driver.Value{{total}},
	}
}

func apiPerformanceTemplateSelectResponse() apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_templates") && !strings.Contains(lower, "count(")
		},
		columns: []string{"id", "name", "description", "status", "created_by", "updated_by"},
		rows: [][]driver.Value{
			{int64(1), "Standard", "default", "active", "admin", "admin"},
		},
	}
}

// ===================== Pure Helper Function Tests =====================

func TestFlexibleJSONString(t *testing.T) {
	tests := []struct {
		name     string
		input    interface{}
		expected string
	}{
		{"nil", nil, ""},
		{"string", "hello", "hello"},
		{"float64", float64(3.14), "3.14"},
		{"float32", float32(2.5), "2.5"},
		{"int", 42, "42"},
		{"int64", int64(100), "100"},
		{"uint", uint(50), "50"},
		{"uint64", uint64(200), "200"},
		{"default", struct{}{}, "{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := flexibleJSONString(tt.input)
			if result != tt.expected {
				t.Fatalf("flexibleJSONString(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestShouldNotifyParticipant(t *testing.T) {
	tests := []struct {
		name        string
		participant database.PerformanceParticipant
		expected    bool
	}{
		{
			name:        "empty employee id",
			participant: database.PerformanceParticipant{EmployeeID: ""},
			expected:    false,
		},
		{
			name:        "removed from scope",
			participant: database.PerformanceParticipant{EmployeeID: "user-1", Status: "removed_from_scope"},
			expected:    false,
		},
		{
			name:        "result hidden",
			participant: database.PerformanceParticipant{EmployeeID: "user-1", ResultHidden: true},
			expected:    false,
		},
		{
			name:        "inactive employee",
			participant: database.PerformanceParticipant{EmployeeID: "user-1", EmployeeStatus: "inactive"},
			expected:    false,
		},
		{
			name:        "exited employee",
			participant: database.PerformanceParticipant{EmployeeID: "user-1", EmployeeStatus: "exited"},
			expected:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := shouldNotifyParticipant(tt.participant)
			if result != tt.expected {
				t.Fatalf("shouldNotifyParticipant() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestFormatSelfEvaluationWindow(t *testing.T) {
	tests := []struct {
		name     string
		startAt  string
		endAt    string
		expected string
	}{
		{"both", "2026-04-01", "2026-04-07", "2026-04-01 - 2026-04-07"},
		{"start only", "2026-04-01", "", "2026-04-01"},
		{"end only", "", "2026-04-07", "2026-04-07"},
		{"neither", "", "", "请查看绩效活动配置"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSelfEvaluationWindow(tt.startAt, tt.endAt)
			if result != tt.expected {
				t.Fatalf("formatSelfEvaluationWindow(%q, %q) = %q, want %q", tt.startAt, tt.endAt, result, tt.expected)
			}
		})
	}
}

func TestCurrentOperatorID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test empty userID
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("userID", "")
	result := currentOperatorID(c)
	if result != "system" {
		t.Fatalf("currentOperatorID() = %q, want system", result)
	}

	// Test with userID (no DB lookup needed)
	recorder2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(recorder2)
	c2.Set("userID", "user-1")
	result2 := currentOperatorID(c2)
	if result2 != "user-1" {
		t.Fatalf("currentOperatorID() = %q, want user-1", result2)
	}
}

func TestCurrentOperatorName(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Test empty userID and userName
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set("userID", "")
	c.Set("userName", "")
	result := currentOperatorName(c)
	if result != "system" {
		t.Fatalf("currentOperatorName() = %q, want system", result)
	}

	// Test with userName
	recorder2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(recorder2)
	c2.Set("userID", "")
	c2.Set("userName", "Alice")
	result2 := currentOperatorName(c2)
	if result2 != "Alice" {
		t.Fatalf("currentOperatorName() = %q, want Alice", result2)
	}
}

func TestDisplayUserName(t *testing.T) {
	// Test empty string
	result := displayUserName("")
	if result != "" {
		t.Fatalf("displayUserName(\"\") = %q, want empty", result)
	}
}

func TestLogPerformanceNotifyError(t *testing.T) {
	// Test nil error (should not panic)
	logPerformanceNotifyError("test action", "user-1", nil)

	// Test non-nil error (should not panic)
	logPerformanceNotifyError("test action", "user-1", fmt.Errorf("test error"))
}

func TestToPerformanceTemplateRequest(t *testing.T) {
	req := performanceTemplatePayload{
		Name:        "test",
		Description: "desc",
		Status:      "active",
		Sections: []struct {
			Name              string  `json:"name" binding:"required"`
			SectionType       string  `json:"section_type" binding:"required"`
			Weight            float64 `json:"weight" binding:"required"`
			SortOrder         int     `json:"sort_order"`
			IsScoreRequired   bool    `json:"is_score_required"`
			IsCommentRequired bool    `json:"is_comment_required"`
			Items             []struct {
				Name        string  `json:"name" binding:"required"`
				Description string  `json:"description"`
				MaxScore    float64 `json:"max_score" binding:"required"`
				Weight      float64 `json:"weight" binding:"required"`
				SortOrder   int     `json:"sort_order"`
			} `json:"items" binding:"required"`
		}{
			{
				Name:        "section1",
				SectionType: "quantitative",
				Weight:      0.7,
				SortOrder:   1,
				Items: []struct {
					Name        string  `json:"name" binding:"required"`
					Description string  `json:"description"`
					MaxScore    float64 `json:"max_score" binding:"required"`
					Weight      float64 `json:"weight" binding:"required"`
					SortOrder   int     `json:"sort_order"`
				}{
					{Name: "item1", MaxScore: 100, Weight: 1, SortOrder: 1},
				},
			},
		},
	}

	result := toPerformanceTemplateRequest(req)

	if result.Name != "test" {
		t.Fatalf("Name = %q, want test", result.Name)
	}
	if result.Description != "desc" {
		t.Fatalf("Description = %q, want desc", result.Description)
	}
	if len(result.Sections) != 1 {
		t.Fatalf("Sections length = %d, want 1", len(result.Sections))
	}
	if result.Sections[0].Name != "section1" {
		t.Fatalf("Section Name = %q, want section1", result.Sections[0].Name)
	}
	if len(result.Sections[0].Items) != 1 {
		t.Fatalf("Items length = %d, want 1", len(result.Sections[0].Items))
	}
}

func TestNormalizeParticipantConfirmers(t *testing.T) {
	// Test nil participant
	normalizeParticipantConfirmers(nil)

	// Test with empty confirmers (no DB lookup needed)
	participant := &database.PerformanceParticipant{
		EmployeeConfirmedBy: "",
		ManagerConfirmedBy:  "",
		HRConfirmedBy:       "",
	}
	normalizeParticipantConfirmers(participant)

	// Should not panic
	if participant == nil {
		t.Fatal("participant should not be nil")
	}
}

func TestVerifySelfParticipant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	participant := &database.PerformanceParticipant{EmployeeID: "user-1"}

	// Test admin bypass
	adminRecorder := httptest.NewRecorder()
	adminCtx, _ := gin.CreateTestContext(adminRecorder)
	adminCtx.Set("userID", "admin")
	if !verifySelfParticipant(adminCtx, participant) {
		t.Fatal("admin should bypass self participant check")
	}

	// Test matching user
	matchRecorder := httptest.NewRecorder()
	matchCtx, _ := gin.CreateTestContext(matchRecorder)
	matchCtx.Set("userID", "user-1")
	if !verifySelfParticipant(matchCtx, participant) {
		t.Fatal("matching user should pass self participant check")
	}

	// Test non-matching user
	denyRecorder := httptest.NewRecorder()
	denyCtx, _ := gin.CreateTestContext(denyRecorder)
	denyCtx.Set("userID", "user-2")
	if verifySelfParticipant(denyCtx, participant) {
		t.Fatal("non-matching user should fail self participant check")
	}
	if denyRecorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", denyRecorder.Code, http.StatusForbidden)
	}
}

func TestVerifyManagerOfParticipant(t *testing.T) {
	gin.SetMode(gin.TestMode)

	managerID := "manager-1"
	participant := &database.PerformanceParticipant{EmployeeID: "user-1", ManagerID: &managerID}

	// Test admin bypass
	adminRecorder := httptest.NewRecorder()
	adminCtx, _ := gin.CreateTestContext(adminRecorder)
	adminCtx.Set("userID", "admin")
	if !verifyManagerOfParticipant(adminCtx, participant) {
		t.Fatal("admin should bypass manager check")
	}

	// Test matching manager
	matchRecorder := httptest.NewRecorder()
	matchCtx, _ := gin.CreateTestContext(matchRecorder)
	matchCtx.Set("userID", "manager-1")
	if !verifyManagerOfParticipant(matchCtx, participant) {
		t.Fatal("matching manager should pass manager check")
	}

	// Test non-matching user
	denyRecorder := httptest.NewRecorder()
	denyCtx, _ := gin.CreateTestContext(denyRecorder)
	denyCtx.Set("userID", "user-2")
	if verifyManagerOfParticipant(denyCtx, participant) {
		t.Fatal("non-matching user should fail manager check")
	}
	if denyRecorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", denyRecorder.Code, http.StatusForbidden)
	}

	// Test participant without manager
	noManagerRecorder := httptest.NewRecorder()
	noManagerCtx, _ := gin.CreateTestContext(noManagerRecorder)
	noManagerCtx.Set("userID", "manager-1")
	noManagerParticipant := &database.PerformanceParticipant{EmployeeID: "user-1"}
	if verifyManagerOfParticipant(noManagerCtx, noManagerParticipant) {
		t.Fatal("participant without manager should fail manager check")
	}
	if noManagerRecorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", noManagerRecorder.Code, http.StatusForbidden)
	}
}

func TestVerifyPerformanceParticipantAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	participant := &database.PerformanceParticipant{
		EmployeeID:   "user-1",
		DepartmentID: "dept-1",
	}

	// Test nil participant
	nilRecorder := httptest.NewRecorder()
	nilCtx, _ := gin.CreateTestContext(nilRecorder)
	nilCtx.Set("userID", "admin")
	if verifyPerformanceParticipantAccess(nilCtx, nil, nil, nil, nil) {
		t.Fatal("nil participant should fail")
	}
	if nilRecorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", nilRecorder.Code, http.StatusNotFound)
	}

	// Test admin bypass
	adminRecorder := httptest.NewRecorder()
	adminCtx, _ := gin.CreateTestContext(adminRecorder)
	adminCtx.Set("userID", "admin")
	if !verifyPerformanceParticipantAccess(adminCtx, participant, nil, nil, nil) {
		t.Fatal("admin should bypass participant access check")
	}
}

func TestRedactHiddenPerformanceResultFields(t *testing.T) {
	score := 9.0
	now := time.Now()
	participant := &database.PerformanceParticipant{
		SelfScore:                    8,
		SelfLevel:                    "A",
		SelfSummary:                  "self",
		ManagerScore:                 9,
		ManagerComment:               "manager",
		SuggestedLevel:               "A",
		FinalLevel:                   "S",
		AdjustReason:                 "adjust",
		TotalSelfScore:               8,
		TotalManagerScore:            9,
		BonusScore:                   1,
		PenaltyScore:                 0.5,
		AdjustedScore:                9.5,
		DepartmentAdjusted:           true,
		DepartmentFinalScore:         &score,
		DepartmentFinalLevel:         "S",
		DepartmentAdjustReason:       "department",
		DepartmentAdjustedAt:         &now,
		DepartmentAdjustedBy:         "hr",
		ResultHidden:                 true,
		ResultHiddenReason:           "hidden",
		ResultHiddenAt:               &now,
		ResultHiddenBy:               "hr",
		EmployeeConfirmedAt:          &now,
		EmployeeConfirmedBy:          "employee",
		ManagerConfirmedAt:           &now,
		ManagerConfirmedBy:           "manager",
		HRConfirmedAt:                &now,
		HRConfirmedBy:                "hr",
		SelfEvaluationGood:           "good",
		SelfEvaluationImprovement:    "improve",
		ManagerEvaluationGood:        "mgood",
		ManagerEvaluationImprovement: "mimprove",
	}

	redactHiddenPerformanceResultFields(participant)

	if !participant.ResultHidden {
		t.Fatalf("result hidden flag should be preserved")
	}
	if participant.FinalLevel != "" || participant.ManagerScore != 0 || participant.AdjustedScore != 0 {
		t.Fatalf("result fields were not redacted: level=%q manager=%v adjusted=%v", participant.FinalLevel, participant.ManagerScore, participant.AdjustedScore)
	}
	if participant.DepartmentFinalScore != nil || participant.ResultHiddenReason != "" || participant.ManagerConfirmedAt != nil {
		t.Fatalf("hidden metadata/confirmation fields were not redacted")
	}
}

func TestDenyHiddenPerformanceResultForEmployeeAllowsHiddenResultViewPermission(t *testing.T) {
	performanceHandlerUserTestDBWith(t,
		apiPerformancePermissionCodesResponse("performance:hidden_result:view"),
		apiPerformanceRolesResponse(),
	)
	c, recorder := performanceHandlerAdminContext(t)
	c.Set("userID", "user-1")

	blocked := denyHiddenPerformanceResultForEmployee(c, &database.PerformanceParticipant{
		EmployeeID:   "user-1",
		ResultHidden: true,
	})

	if blocked {
		t.Fatalf("hidden result view permission should be allowed to view hidden result; body = %s", recorder.Body.String())
	}
}

func TestDenyHiddenPerformanceResultForEmployeeBlocksPrivilegedNonHiddenViewOperator(t *testing.T) {
	performanceHandlerUserTestDBWith(t,
		apiPerformancePermissionCodesResponse("performance:activity:manage", "performance:result_publish:manage", "performance:result_visibility:manage"),
		apiPerformanceRolesResponse(),
	)
	c, recorder := performanceHandlerAdminContext(t)
	c.Set("userID", "user-1")

	blocked := denyHiddenPerformanceResultForEmployee(c, &database.PerformanceParticipant{
		EmployeeID:   "user-1",
		ResultHidden: true,
	})

	if !blocked {
		t.Fatalf("hidden result should be blocked without hidden result view permission even with performance permissions")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}

func TestDenyHiddenPerformanceResultForEmployeeBlocksSelfWithoutPrivilege(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = originalDB
	})
	c, recorder := performanceHandlerAdminContext(t)
	c.Set("userID", "user-1")

	blocked := denyHiddenPerformanceResultForEmployee(c, &database.PerformanceParticipant{
		EmployeeID:   "user-1",
		ResultHidden: true,
	})

	if !blocked {
		t.Fatalf("hidden self result should be blocked for non-privileged employee")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"result_hidden":true`) {
		t.Fatalf("body = %s, want result_hidden marker", recorder.Body.String())
	}
}

func TestDenyHiddenPerformanceResultForEmployeeBlocksNonPrivilegedOperator(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = originalDB
	})
	c, recorder := performanceHandlerAdminContext(t)
	c.Set("userID", "manager-1")

	blocked := denyHiddenPerformanceResultForEmployee(c, &database.PerformanceParticipant{
		EmployeeID:   "user-1",
		ResultHidden: true,
	})

	if !blocked {
		t.Fatalf("hidden result should be blocked for non-privileged operator")
	}
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}

func TestGetPreviousParticipantResultRejectsHiddenPreviousResultForEmployee(t *testing.T) {
	previousActivityID := uint(1)
	performanceHandlerUserTestDBWith(t,
		apiPerformancePermissionCodesResponse("performance:result:view"),
		apiPerformanceRolesResponse(),
		apiPerformanceDataPermissionAllResponse(),
		apiPerformanceParticipantByIDResponse(2, "2", "user-1", false),
		apiPerformanceActivityByIDResponse(2, "Q2 Review", "self_evaluation", "new", &previousActivityID),
		apiPerformanceActivityByIDResponse(1, "Q1 Review", "locked", "new", nil),
		apiPerformanceParticipantByActivityAndEmployeeResponse(1, "1", "user-1", true),
		apiPerformanceGoalRecordsByParticipantResponse(1),
		apiPerformanceReviewVersionsByParticipantResponse(1),
		apiPerformanceGoalApprovalLogsResponse(nil),
	)

	c, recorder := performanceHandlerAdminContext(t)
	c.Set("userID", "user-1")
	c.Request = performanceTestRequest(http.MethodGet, "/api/v1/performance/participants/2/previous-result", nil)
	c.Params = gin.Params{{Key: "participant_id", Value: "2"}}

	GetPreviousParticipantResult(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
	body := recorder.Body.String()
	if !strings.Contains(body, `"result_hidden":true`) {
		t.Fatalf("body = %s, want result_hidden marker", body)
	}
	if strings.Contains(body, `"final_level":"S"`) || strings.Contains(body, `"manager_score":9.5`) || strings.Contains(body, "hidden-achievement") {
		t.Fatalf("hidden previous result leaked in body: %s", body)
	}
}

func apiPerformancePermissionCodesResponse(codes ...string) apiImportQueryResponse {
	rows := make([][]driver.Value, 0, len(codes))
	for i, code := range codes {
		rows = append(rows, []driver.Value{int64(i + 1), code, code})
	}
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "permissions") && strings.Contains(lower, "role_permissions")
		},
		columns: []string{"id", "name", "code"},
		rows:    rows,
	}
}

func apiPerformanceRolesResponse() apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "roles") && strings.Contains(lower, "user_roles")
		},
		columns: []string{"id", "name", "description"},
		rows:    [][]driver.Value{{int64(1), "employee", "employee"}},
	}
}

func apiPerformanceDataPermissionAllResponse() apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			return strings.Contains(strings.ToLower(query), "data_permissions")
		},
		columns: []string{"id", "role_id", "scope", "department_keys"},
		rows:    [][]driver.Value{{int64(1), int64(1), "all", "[]"}},
	}
}

func apiPerformanceActivityByIDResponse(id uint, name, status, flowType string, previousReviewActivityID *uint) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_activities") &&
				!strings.Contains(lower, "count(") &&
				apiImportArgsContain(args, id)
		},
		columns: []string{
			"id", "name", "cycle_type", "start_date", "end_date", "status", "flow_type", "previous_review_activity_id",
		},
		rows: [][]driver.Value{{
			int64(id), name, "quarterly", "2026-04-01", "2026-06-30", status, flowType, optionalUintDriverValue(previousReviewActivityID),
		}},
	}
}

func apiPerformanceParticipantByIDResponse(id uint, activityID, employeeID string, resultHidden bool) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_participants") &&
				!strings.Contains(lower, "count(") &&
				!strings.Contains(lower, "activity_id =") &&
				apiImportArgsContain(args, id)
		},
		columns: apiPerformanceHiddenParticipantColumns(),
		rows: [][]driver.Value{{
			int64(id), activityID, employeeID, "Alice", "dept-1", "Product", ptrString("manager-1"), ptrString("Manager Bob"),
			"manager_submitted", "active", float64(8.5), float64(9.5), "S", float64(9.5), resultHidden, false,
		}},
	}
}

func apiPerformanceParticipantByActivityAndEmployeeResponse(id uint, activityID, employeeID string, resultHidden bool) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_participants") &&
				strings.Contains(lower, "activity_id") &&
				strings.Contains(lower, "employee_id") &&
				apiImportArgsContain(args, activityID) &&
				apiImportArgsContain(args, employeeID)
		},
		columns: apiPerformanceHiddenParticipantColumns(),
		rows: [][]driver.Value{{
			int64(id), activityID, employeeID, "Alice", "dept-1", "Product", ptrString("manager-1"), ptrString("Manager Bob"),
			"locked", "active", float64(8.5), float64(9.5), "S", float64(9.5), resultHidden, true,
		}},
	}
}

func apiPerformanceHiddenParticipantColumns() []string {
	return []string{
		"id", "activity_id", "employee_id", "employee_name", "department_id", "department_name",
		"manager_id", "manager_name", "status", "employee_status",
		"self_score", "manager_score", "final_level", "adjusted_score", "result_hidden", "is_locked",
	}
}

func apiPerformanceGoalRecordsByParticipantResponse(participantID uint) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_goal_records") && apiImportArgsContain(args, participantID)
		},
		columns: []string{"id", "activity_id", "participant_id", "section_type", "item_name", "actual_result", "manager_score"},
		rows: [][]driver.Value{{
			int64(1), "1", int64(participantID), "quantitative", "hidden-metric", "hidden-achievement", float64(9.5),
		}},
	}
}

func apiPerformanceReviewVersionsByParticipantResponse(participantID uint) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "performance_review_versions") && apiImportArgsContain(args, participantID)
		},
		columns: []string{"id", "participant_id", "activity_id", "review_type", "created_by", "manager_score", "final_level"},
		rows: [][]driver.Value{{
			int64(1), int64(participantID), "1", "manager", "manager-1", float64(9.5), "S",
		}},
	}
}

func apiImportArgsContain(args []driver.NamedValue, expected interface{}) bool {
	expectedText := fmt.Sprint(expected)
	for _, arg := range args {
		if fmt.Sprint(arg.Value) == expectedText {
			return true
		}
	}
	return false
}

func optionalUintDriverValue(value *uint) driver.Value {
	if value == nil {
		return nil
	}
	return int64(*value)
}
