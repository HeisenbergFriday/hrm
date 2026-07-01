package api

import (
	"database/sql/driver"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/gin-gonic/gin"
)

func TestPerformanceRouterCoversFrontendAPIPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := SetupRouter()
	backend := make(map[string]struct{})
	for _, route := range router.Routes() {
		if !strings.HasPrefix(route.Path, "/api/v1/performance") {
			continue
		}
		path := strings.TrimPrefix(route.Path, "/api/v1")
		backend[route.Method+" "+normalizePerformanceRoutePattern(path)] = struct{}{}
	}

	frontendCalls := frontendPerformanceAPICalls(t)
	var missing []string
	for _, call := range frontendCalls {
		if _, ok := backend[call]; !ok {
			missing = append(missing, call)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("frontend performance API calls missing backend routes:\n%s", strings.Join(missing, "\n"))
	}
}

func TestPerformanceRouterRegistersCriticalRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := SetupRouter()
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, route := range []string{
		"GET /api/v1/performance/activities",
		"POST /api/v1/performance/activities",
		"POST /api/v1/performance/participants/import",
		"POST /api/v1/performance/auto-score",
		"GET /api/v1/performance/activities/:activity_id/assessment-manager-candidates",
		"PUT /api/v1/performance/participants/:participant_id/assessment-manager",
		"POST /api/v1/performance/activities/:activity_id/assessment-managers/batch",
		"POST /api/v1/performance/reviews/:participant_id/self-evaluation",
		"POST /api/v1/performance/participants/:participant_id/confirm-employee",
		"POST /api/v1/performance/participants/:participant_id/confirm-manager",
		"POST /api/v1/performance/participants/:participant_id/confirm-hr",
		"POST /api/v1/performance/activities/:activity_id/send-hr-confirm-reminder",
		"POST /api/v1/performance/indicator-libraries",
		"PUT /api/v1/performance/indicator-items/:id",
		"GET /api/v1/performance/templates/:id",
		"PUT /api/v1/performance/templates/:id",
		"GET /api/v1/performance/goal-records/:participant_id",
		"POST /api/v1/performance/goal-records/:participant_id/review-supplement",
		"POST /api/v1/performance/activities/:activity_id/batch-assign-goals",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("route %s is not registered", route)
		}
	}
}

func TestPerformanceRouterEnforcesJWTAndPermissions(t *testing.T) {
	t.Run("missing token is unauthorized before handler", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		installPerformanceRouterTestDB(t, nil)

		router := SetupRouter()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/performance/auto-score", strings.NewReader(`{"items":[]}`))
		request.Header.Set("Content-Type", "application/json")

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
		}
	})

	t.Run("valid token without permission is forbidden before handler", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		installPerformanceRouterTestDB(t, nil)

		router := SetupRouter()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/performance/auto-score", strings.NewReader(`{"items":[]}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+performanceRouterTestToken(t))

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
		}
	})

	t.Run("required permission reaches handler", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		installPerformanceRouterTestDB(t, []string{"performance:activity:manage"})

		router := SetupRouter()
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/api/v1/performance/auto-score", strings.NewReader(`{
			"items": [{
				"record_id": 1,
				"section_type": "quantitative",
				"weight": 1,
				"target_value": "100",
				"actual_result": "100"
			}]
		}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", "Bearer "+performanceRouterTestToken(t))

		router.ServeHTTP(recorder, request)

		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
		}
		var response Response
		if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if response.Message != "success" {
			t.Fatalf("message = %q, want success", response.Message)
		}
	})
}

func TestPerformanceAppealUpdateRouteAllowsActivityManagers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	installPerformanceRouterTestDB(t, []string{"performance:activity:manage"})

	router := SetupRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/performance/appeals/1", strings.NewReader(`{"status":"processing"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+performanceRouterTestToken(t))

	router.ServeHTTP(recorder, request)

	if recorder.Code == http.StatusForbidden {
		t.Fatalf("activity manager was blocked by appeal route permission; body = %s", recorder.Body.String())
	}
}

func TestPerformanceAppealUpdateRouteRejectsResultViewOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	installPerformanceRouterTestDB(t, []string{"performance:result:view"})

	router := SetupRouter()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut, "/api/v1/performance/appeals/1", strings.NewReader(`{"status":"processing"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+performanceRouterTestToken(t))

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusForbidden, recorder.Body.String())
	}
}

func frontendPerformanceAPICalls(t *testing.T) []string {
	t.Helper()

	apiFile := filepath.Join("..", "..", "frontend", "src", "services", "api.ts")
	content, err := os.ReadFile(apiFile)
	if err != nil {
		t.Fatalf("read frontend api file: %v", err)
	}

	callRE := regexp.MustCompile("api\\.(get|post|put|delete)\\(\\s*[`'](/performance[^`']+)[`']")
	matches := callRE.FindAllStringSubmatch(string(content), -1)
	if len(matches) == 0 {
		t.Fatalf("no frontend performance API calls found in %s", apiFile)
	}

	seen := make(map[string]struct{})
	for _, match := range matches {
		method := strings.ToUpper(match[1])
		path := normalizePerformanceRoutePattern(match[2])
		seen[method+" "+path] = struct{}{}
	}

	calls := make([]string, 0, len(seen))
	for call := range seen {
		calls = append(calls, call)
	}
	sort.Strings(calls)
	return calls
}

func normalizePerformanceRoutePattern(path string) string {
	path = strings.TrimSpace(path)
	path = regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(path, ":param")
	path = regexp.MustCompile(`:[^/]+`).ReplaceAllString(path, ":param")
	return path
}

func performanceRouterTestToken(t *testing.T) string {
	t.Helper()

	t.Setenv("JWT_SECRET", "performance-router-test-secret-32chars")
	token, _, err := signAuthToken(&database.User{ID: 1, UserID: "tester", Name: "Tester", OrgID: "default"}, "test-session")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return token
}

func installPerformanceRouterTestDB(t *testing.T, permissionCodes []string) {
	t.Helper()

	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		apiPerformanceRouterUserResponse(),
		apiPerformanceRouterSessionResponse(),
		apiPerformanceRouterRolesResponse(permissionCodes),
		apiPerformanceRouterPermissionsResponse(permissionCodes),
		apiPerformanceRouterTableResponse("menu_permissions", []string{"id", "role_id", "menu_keys"}, nil),
		apiPerformanceRouterTableResponse("data_permissions", []string{"id", "role_id", "scope", "department_keys"}, nil),
	)
	t.Cleanup(func() {
		database.DB = originalDB
	})
}

func apiPerformanceRouterSessionResponse() apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			query = strings.ToLower(query)
			return strings.Contains(query, "from `user_sessions`") || strings.Contains(query, "from user_sessions")
		},
		columns: []string{"id", "user_id", "session_id", "token", "expires_at"},
		rows: [][]driver.Value{
			{int64(1), "tester", "test-session", "test-token-hash", time.Now().Add(time.Hour)},
		},
	}
}

func apiPerformanceRouterUserResponse() apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			query = strings.ToLower(query)
			return strings.Contains(query, "from `users`") || strings.Contains(query, "from users")
		},
		columns: []string{"id", "user_id", "name", "department_id", "status"},
		rows: [][]driver.Value{
			{int64(1), "tester", "Tester", "dept-1", "active"},
		},
	}
}

func apiPerformanceRouterRolesResponse(permissionCodes []string) apiImportQueryResponse {
	rows := [][]driver.Value(nil)
	if len(permissionCodes) > 0 {
		rows = [][]driver.Value{{int64(1), "performance-role", "test role"}}
	}
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			query = strings.ToLower(query)
			return (strings.Contains(query, "from `roles`") || strings.Contains(query, "from roles")) &&
				strings.Contains(query, "user_roles")
		},
		columns: []string{"id", "name", "description"},
		rows:    rows,
	}
}

func apiPerformanceRouterPermissionsResponse(permissionCodes []string) apiImportQueryResponse {
	rows := make([][]driver.Value, 0, len(permissionCodes))
	for index, code := range permissionCodes {
		rows = append(rows, []driver.Value{int64(index + 1), code, code, ""})
	}
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			query = strings.ToLower(query)
			return (strings.Contains(query, "from `permissions`") || strings.Contains(query, "from permissions")) &&
				strings.Contains(query, "role_permissions")
		},
		columns: []string{"id", "name", "code", "description"},
		rows:    rows,
	}
}

func apiPerformanceRouterTableResponse(table string, columns []string, rows [][]driver.Value) apiImportQueryResponse {
	table = strings.ToLower(table)
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			query = strings.ToLower(query)
			return strings.Contains(query, "from `"+table+"`") || strings.Contains(query, "from "+table)
		},
		columns: columns,
		rows:    rows,
	}
}
