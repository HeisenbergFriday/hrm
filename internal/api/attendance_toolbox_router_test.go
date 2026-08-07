package api

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestAttendanceToolboxRouterRegistersStructuredEndpoints guards against the
// recurrence of a deployed backend that returns 404 for the structured toolbox
// workflows (the endpoint existed in handlers but was never mounted).
func TestAttendanceToolboxRouterRegistersStructuredEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := SetupRouter()
	routes := make(map[string]struct{})
	for _, route := range router.Routes() {
		routes[route.Method+" "+route.Path] = struct{}{}
	}

	for _, route := range []string{
		"POST /api/v1/attendance/toolbox/workflows/dingtalk_sync",
		"POST /api/v1/attendance/toolbox/workflows/quick",
		"POST /api/v1/attendance/toolbox/workflows/:module",
		"GET /api/v1/attendance/toolbox/runs/:run_id",
		"GET /api/v1/attendance/toolbox/runs/:run_id/files/:file_key",
		"GET /api/v1/attendance/toolbox/runs/:run_id/zip",
		"GET /api/v1/attendance/toolbox/runs/:run_id/preview",
		"POST /api/v1/attendance/toolbox/dingtalk-sync",
		"POST /api/v1/attendance/toolbox/parttime-monthly-punch",
	} {
		if _, ok := routes[route]; !ok {
			t.Fatalf("missing backend route: %s", route)
		}
	}
}

// TestAttendanceToolboxRouterCoversFrontendAPIPaths verifies every
// /attendance/toolbox/* call made by the frontend api.ts has a registered
// backend route, so a newly deployed backend never 404s the page.
func TestAttendanceToolboxRouterCoversFrontendAPIPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := SetupRouter()
	backend := make(map[string]struct{})
	for _, route := range router.Routes() {
		path := strings.TrimPrefix(route.Path, "/api/v1")
		backend[route.Method+" "+normalizeToolboxRoutePattern(path)] = struct{}{}
	}

	frontendCalls := frontendToolboxAPICalls(t)
	var missing []string
	for _, call := range frontendCalls {
		if _, ok := backend[call]; !ok {
			missing = append(missing, call)
		}
	}

	if len(missing) > 0 {
		t.Fatalf("frontend attendance/toolbox API calls missing backend routes:\n%s", strings.Join(missing, "\n"))
	}
}

func frontendToolboxAPICalls(t *testing.T) []string {
	t.Helper()

	apiFile := filepath.Join("..", "..", "frontend", "src", "services", "api.ts")
	content, err := os.ReadFile(apiFile)
	if err != nil {
		t.Fatalf("read frontend api file: %v", err)
	}

	callRE := regexp.MustCompile("api\\.(get|post|put|delete)\\(\\s*[`'](/attendance/toolbox[^`']+)[`']")
	matches := callRE.FindAllStringSubmatch(string(content), -1)
	if len(matches) == 0 {
		t.Fatalf("no frontend attendance/toolbox API calls found in %s", apiFile)
	}

	seen := make(map[string]struct{})
	for _, match := range matches {
		method := strings.ToUpper(match[1])
		path := normalizeToolboxRoutePattern(match[2])
		seen[method+" "+path] = struct{}{}
	}

	calls := make([]string, 0, len(seen))
	for call := range seen {
		calls = append(calls, call)
	}
	sort.Strings(calls)
	return calls
}

func normalizeToolboxRoutePattern(path string) string {
	path = strings.TrimSpace(path)
	path = regexp.MustCompile(`\$\{[^}]+\}`).ReplaceAllString(path, ":param")
	path = regexp.MustCompile(`:[^/]+`).ReplaceAllString(path, ":param")
	return path
}
