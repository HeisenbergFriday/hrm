package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"peopleops/internal/database"

	"github.com/gin-gonic/gin"
)

func TestExternalApprovalRouteRegistered(t *testing.T) {
	router := SetupRouter()
	for _, route := range router.Routes() {
		if route.Method == "GET" && route.Path == "/api/v1/approvals/oa-data" {
			return
		}
	}
	t.Fatal("GET /api/v1/approvals/oa-data is not registered")
}

func TestExternalApprovalDetailsRejectsOtherOrganization(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/v1/approvals/oa-data", nil)
	context.Set("orgID", database.OrgIDXiaotie)

	ExternalApprovalDetails(context)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
