package api

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/middleware"

	"github.com/gin-gonic/gin"
)

// scope-options 必须经过真实 middleware（JWT + performanceRead），不直接调 handler。

func TestPerformanceScopeOptionsRouterPermissions(t *testing.T) {
	t.Run("missing token is unauthorized", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		installPerformanceRouterTestDB(t, nil)
		router := SetupRouter()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/performance/scope-options", nil)
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("performance read permission can access without user_manage", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		installPerformanceScopeOptionsTestDB(t, []string{"performance:result:view"}, nil, "all", nil)
		router := SetupRouter()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/performance/scope-options?page=1&page_size=100", nil)
		req.Header.Set("Authorization", "Bearer "+performanceRouterTestToken(t))
		router.ServeHTTP(rec, req)

		assertScopeOptionsHTTP200(t, rec)
		data := decodeScopeOptionsData(t, rec)
		employees := scopeEmployees(t, data)
		if !scopeEmployeeIDs(employees).has("u-1") {
			t.Fatalf("expected employee u-1 in response, got %#v", employees)
		}
		if strings.Contains(rec.Body.String(), "user_manage") {
			t.Fatalf("must not require user_manage: %s", rec.Body.String())
		}
	})

	t.Run("menu only permission can access", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		installPerformanceScopeOptionsMenuOnlyDB(t)
		router := SetupRouter()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/performance/scope-options?page=1&page_size=100", nil)
		req.Header.Set("Authorization", "Bearer "+performanceRouterTestToken(t))
		router.ServeHTTP(rec, req)

		assertScopeOptionsHTTP200(t, rec)
		data := decodeScopeOptionsData(t, rec)
		employees := scopeEmployees(t, data)
		if !scopeEmployeeIDs(employees).has("u-1") {
			t.Fatalf("menu-only user should still receive org employees, got %#v", employees)
		}
	})

	t.Run("no performance permission is forbidden", func(t *testing.T) {
		gin.SetMode(gin.TestMode)
		installPerformanceRouterTestDB(t, []string{"attendance_manage"})
		router := SetupRouter()
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/performance/scope-options", nil)
		req.Header.Set("Authorization", "Bearer "+performanceRouterTestToken(t))
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status=%d want 403 body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestGetPerformanceScopeOptionsFieldShapeAndSkip(t *testing.T) {
	// handler 级：强制返回非空员工集，验证字段精简、跳过无 user_id、不返回敏感字段。
	// ListEmployees 依赖 users JOIN employee_profiles；count(*) 必须单独匹配返回单列。
	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		scopeOptionsCountResponse(2),
		// JOIN 列表：含空 user_id，handler 层必须跳过并 warning
		scopeOptionsListEmployeesResponse([][]driver.Value{
			{int64(1), "u-1", "Alice", "dept-1", "active", "test-org"},
			{int64(2), "", "NoUserID", "dept-1", "active", "test-org"},
		}),
		scopeOptionsEmployeeProfilesResponse([][]driver.Value{
			{int64(1), "u-1", "E001", "test-org"},
		}),
		apiImportTableResponse("departments", []string{"id", "department_id", "name", "org_id", "parent_id"}, [][]driver.Value{
			{int64(1), "dept-1", "Product", "test-org", ""},
		}),
		apiImportTableResponse("data_permissions", []string{"id", "role_id", "scope", "department_keys"}, nil),
		apiImportTableResponse("roles", []string{"id", "name"}, nil),
		apiImportTableResponse("user_roles", []string{"id", "user_id", "role_id"}, nil),
		apiImportTableResponse("permissions", []string{"id", "code"}, nil),
		apiImportTableResponse("role_permissions", []string{"id", "role_id", "permission_id"}, nil),
		apiImportTableResponse("menu_permissions", []string{"id", "role_id", "menu_keys"}, nil),
	)
	t.Cleanup(func() { database.DB = originalDB })

	c, rec := performanceHandlerContextAs(t, "test-org", "admin")
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/scope-options?page=1&page_size=100", nil)
	GetPerformanceScopeOptions(c)

	assertScopeOptionsHTTP200(t, rec)
	data := decodeScopeOptionsData(t, rec)
	employees := scopeEmployees(t, data)
	if len(employees) == 0 {
		t.Fatalf("expected at least one employee, body=%s", rec.Body.String())
	}

	ids := scopeEmployeeIDs(employees)
	if !ids.has("u-1") {
		t.Fatalf("expected u-1 present, employees=%#v", employees)
	}
	if ids.has("") {
		t.Fatalf("empty user_id must be skipped, employees=%#v", employees)
	}
	for _, emp := range employees {
		if strAny(emp["name"]) == "NoUserID" {
			t.Fatalf("employee without user_id must be skipped: %#v", emp)
		}
	}

	for _, emp := range employees {
		for k := range emp {
			switch k {
			case "user_id", "name", "department_id", "department_name", "status":
			default:
				t.Fatalf("unexpected employee field %q in %#v", k, emp)
			}
		}
		if emp["mobile"] != nil || emp["email"] != nil || emp["id_card"] != nil {
			t.Fatalf("must not return sensitive fields: %#v", emp)
		}
		if strings.TrimSpace(strAny(emp["user_id"])) == "" {
			t.Fatalf("empty user_id should have been skipped: %#v", emp)
		}
	}

	warnings, _ := data["warnings"].([]any)
	if len(warnings) == 0 {
		t.Fatalf("expected missing user_id warning, data=%#v body=%s", data, rec.Body.String())
	}
	if !strings.Contains(strAny(warnings[0]), "user_id") {
		t.Fatalf("warning should mention missing user_id: %#v", warnings)
	}
}

func TestGetPerformanceScopeOptionsDataScopeModes(t *testing.T) {
	// department / self 通过权限 scope 限制可见员工；other-org 永远不可见。
	cases := []struct {
		name            string
		scope           string
		departmentKeys  []string
		keyword         string
		wantUserIDs     []string
		forbiddenUserID string
	}{
		{
			name:            "all returns current org employees",
			scope:           "all",
			wantUserIDs:     []string{"u-1", "u-2"},
			forbiddenUserID: "u-other",
		},
		{
			name:            "department returns only allowed department",
			scope:           "department",
			departmentKeys:  []string{"dept-1"},
			wantUserIDs:     []string{"u-1"},
			forbiddenUserID: "u-2",
		},
		{
			name:            "self returns only current user",
			scope:           "self",
			keyword:         "Tester",
			wantUserIDs:     []string{"tester"},
			forbiddenUserID: "u-1",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			observation := installPerformanceScopeOptionsDataScopeDB(t, tt.scope, tt.departmentKeys)
			// 用 handler + 已设 org 的上下文，直接验证 scope 过滤结果。
			// 普通用户（非 admin）才会进入 ResolveUserScope 的 data_permissions 路径。
			c, rec := performanceHandlerContextAs(t, "default", "tester")
			// 注入与 data_permissions 一致的角色权限，使 resolvePerformanceScope 走真实 DB。
			// 此处 stub 已含 roles/user_roles/data_permissions。
			requestURL := "/api/v1/performance/scope-options?page=1&page_size=100"
			if tt.keyword != "" {
				requestURL += "&keyword=" + tt.keyword
			}
			c.Request = httptest.NewRequest(http.MethodGet, requestURL, nil)
			GetPerformanceScopeOptions(c)

			assertScopeOptionsHTTP200(t, rec)
			data := decodeScopeOptionsData(t, rec)
			employees := scopeEmployees(t, data)
			ids := scopeEmployeeIDs(employees)
			for _, want := range tt.wantUserIDs {
				if !ids.has(want) {
					t.Fatalf("scope=%s missing %s; employees=%#v", tt.scope, want, employees)
				}
			}
			if tt.forbiddenUserID != "" && ids.has(tt.forbiddenUserID) {
				t.Fatalf("scope=%s must not include %s; employees=%#v", tt.scope, tt.forbiddenUserID, employees)
			}
			if ids.has("u-other") {
				t.Fatalf("other-org employee must never appear; employees=%#v", employees)
			}
			if !observation.employeeListSeen {
				t.Fatalf("scope=%s employee list query was not observed", tt.scope)
			}
			if !observation.orgScoped {
				t.Fatalf("scope=%s employee list query must include current org_id; sql=%s args=%#v", tt.scope, observation.sql, observation.args)
			}
			if !observation.statusScoped {
				t.Fatalf("scope=%s employee list query must include active status; sql=%s args=%#v", tt.scope, observation.sql, observation.args)
			}
			if tt.keyword != "" && !observation.searchScoped {
				t.Fatalf("scope=%s employee list query must include keyword filter; sql=%s args=%#v", tt.scope, observation.sql, observation.args)
			}
		})
	}
}

func TestGetPerformanceScopeOptionsMissingOrgSingleJSON(t *testing.T) {
	c, rec := performanceHandlerContextAs(t, "", "user-1")
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/scope-options", nil)
	GetPerformanceScopeOptions(c)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("body must be single JSON: %v body=%s", err, rec.Body.String())
	}
	if strings.Count(rec.Body.String(), `"code"`) != 1 {
		t.Fatalf("duplicate JSON responses: %s", rec.Body.String())
	}
}

func TestRequirePermissionRequiresOrgBeforeAdminBypass(t *testing.T) {
	t.Run("admin missing org returns 401 single json", func(t *testing.T) {
		c, rec := performanceHandlerContextAs(t, "", "admin")
		if requirePermission(c, "performance:activity:manage") {
			t.Fatalf("admin without org must be rejected")
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
		}
		var resp Response
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("body must be single JSON: %v body=%s", err, rec.Body.String())
		}
		if strings.Count(rec.Body.String(), `"code"`) != 1 {
			t.Fatalf("duplicate JSON: %s", rec.Body.String())
		}
	})

	t.Run("system missing org returns 401", func(t *testing.T) {
		c, rec := performanceHandlerContextAs(t, "", "system")
		if requirePermission(c, "performance:activity:manage") {
			t.Fatalf("system without org must be rejected")
		}
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status=%d want 401 body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("admin with org bypasses feature permission", func(t *testing.T) {
		c, rec := performanceHandlerContextAs(t, performanceHandlerTestOrgID, "admin")
		if !requirePermission(c, "performance:activity:manage") {
			t.Fatalf("admin with org should bypass permission codes; body=%s", rec.Body.String())
		}
		if rec.Body.Len() != 0 {
			t.Fatalf("success path must not write body, got %s", rec.Body.String())
		}
	})

	t.Run("hasPerformancePermission admin missing org errors", func(t *testing.T) {
		c, _ := performanceHandlerContextAs(t, "", "admin")
		ok, err := hasPerformancePermission(c, "performance:result:view")
		if err == nil || ok {
			t.Fatalf("admin without org must error, ok=%v err=%v", ok, err)
		}
	})
}

// ---- helpers ----

func assertScopeOptionsHTTP200(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 body=%s", rec.Code, rec.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v body=%s", err, rec.Body.String())
	}
	if resp.Code != http.StatusOK {
		t.Fatalf("business code=%d want 200 message=%s body=%s", resp.Code, resp.Message, rec.Body.String())
	}
}

func decodeScopeOptionsData(t *testing.T, rec *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	raw := map[string]any{}
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, rec.Body.String())
	}
	data, ok := raw["data"].(map[string]any)
	if !ok || data == nil {
		t.Fatalf("missing data: %s", rec.Body.String())
	}
	return data
}

func scopeEmployees(t *testing.T, data map[string]any) []map[string]any {
	t.Helper()
	raw, ok := data["employees"].([]any)
	if !ok {
		t.Fatalf("employees key missing or wrong type: %#v", data)
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		emp, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("employee item not object: %#v", item)
		}
		out = append(out, emp)
	}
	return out
}

type scopeIDSet map[string]struct{}

func scopeEmployeeIDs(employees []map[string]any) scopeIDSet {
	s := make(scopeIDSet)
	for _, emp := range employees {
		s[strings.TrimSpace(strAny(emp["user_id"]))] = struct{}{}
	}
	return s
}

func (s scopeIDSet) has(id string) bool {
	_, ok := s[id]
	return ok
}

func strAny(v any) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

// scopeOptionsCountResponse 必须优先匹配 count(*)，返回单列 count，避免 Scan 列数错误。
func scopeOptionsCountResponse(n int64) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			return strings.Contains(lower, "count(*)") && strings.Contains(lower, "users")
		},
		columns: []string{"count"},
		rows:    [][]driver.Value{{n}},
	}
}

// scopeOptionsListEmployeesResponse 匹配 ListEmployees 的 JOIN 查询（users + employee_profiles）。
// 必须排在纯 employee_profiles 匹配之前，否则 JOIN 会被 profile 列定义污染。
func scopeOptionsListEmployeesResponse(rows [][]driver.Value, observers ...*scopeOptionsQueryObservation) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			if strings.Contains(lower, "count(*)") {
				return false
			}
			matched := strings.Contains(lower, "users") && strings.Contains(lower, "employee_profiles")
			if matched && len(observers) > 0 {
				observeScopeOptionsEmployeeListQuery(observers[0], query, args)
			}
			return matched
		},
		columns: []string{"id", "user_id", "name", "department_id", "status", "org_id"},
		rows:    rows,
	}
}

func scopeOptionsEmployeeProfilesResponse(rows [][]driver.Value) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, _ []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			// 仅纯 employee_profiles 表查询；排除 users JOIN
			if strings.Contains(lower, "users") {
				return false
			}
			return strings.Contains(lower, "employee_profiles")
		},
		columns: []string{"id", "user_id", "employee_id", "org_id"},
		rows:    rows,
	}
}

func scopeOptionsUsersSelectResponse(rows [][]driver.Value, observers ...*scopeOptionsQueryObservation) apiImportQueryResponse {
	return apiImportQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			lower := strings.ToLower(query)
			if strings.Contains(lower, "count(*)") {
				return false
			}
			if strings.Contains(lower, "employee_profiles") {
				return false
			}
			// JWT / self 模式单表 users 查询
			matched := strings.Contains(lower, "from `users`") || strings.Contains(lower, "from users")
			if matched && strings.Contains(lower, "order by") && len(observers) > 0 {
				observeScopeOptionsEmployeeListQuery(observers[0], query, args)
			}
			return matched
		},
		columns: []string{"id", "user_id", "name", "department_id", "status", "org_id"},
		rows:    rows,
	}
}

type scopeOptionsQueryObservation struct {
	employeeListSeen bool
	orgScoped        bool
	statusScoped     bool
	searchScoped     bool
	sql              string
	args             []any
}

func observeScopeOptionsEmployeeListQuery(observation *scopeOptionsQueryObservation, query string, args []driver.NamedValue) {
	if observation == nil {
		return
	}
	observation.employeeListSeen = true
	observation.sql = query
	observation.args = make([]any, 0, len(args))
	for _, arg := range args {
		observation.args = append(observation.args, arg.Value)
	}
	normalizedSQL := strings.ReplaceAll(strings.ToLower(query), "`", "")
	observation.orgScoped = strings.Contains(normalizedSQL, "users.org_id") && namedArgsContain(args, "default")
	observation.statusScoped = strings.Contains(normalizedSQL, "users.status") && namedArgsContain(args, "active")
	observation.searchScoped = strings.Contains(normalizedSQL, " like ") && namedArgsContain(args, "%Tester%")
}

func namedArgsContain(args []driver.NamedValue, want string) bool {
	for _, arg := range args {
		if strings.TrimSpace(fmt.Sprint(arg.Value)) == want {
			return true
		}
	}
	return false
}

func installPerformanceScopeOptionsMenuOnlyDB(t *testing.T) {
	t.Helper()
	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		apiPerformanceRouterSessionResponse(),
		// JWT 用户加载：精确匹配 user_id=tester
		apiImportQueryResponse{
			match: func(query string, args []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				if !strings.Contains(lower, "from `users`") && !strings.Contains(lower, "from users") {
					return false
				}
				if strings.Contains(lower, "count(*)") || strings.Contains(lower, "employee_profiles") {
					return false
				}
				for _, a := range args {
					if a.Value == "tester" {
						return true
					}
				}
				return strings.Contains(lower, "user_id")
			},
			columns: []string{"id", "user_id", "name", "department_id", "status", "org_id"},
			rows:    [][]driver.Value{{int64(1), "tester", "Tester", "dept-1", "active", "default"}},
		},
		apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				q := strings.ToLower(query)
				return (strings.Contains(q, "from `roles`") || strings.Contains(q, "from roles")) &&
					strings.Contains(q, "user_roles")
			},
			columns: []string{"id", "name", "description"},
			rows:    [][]driver.Value{{int64(1), "menu-role", "menu only"}},
		},
		apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "menu_permissions")
			},
			columns: []string{"id", "role_id", "menu_keys"},
			rows:    [][]driver.Value{{int64(1), int64(1), `["menu:performance-overview"]`}},
		},
		apiPerformanceRouterPermissionsResponse(nil),
		// 必须在空 tableResponse 之前：无配置会落 self，导致只返回 tester。
		apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "data_permissions")
			},
			columns: []string{"id", "role_id", "scope", "department_keys"},
			rows:    [][]driver.Value{{int64(1), int64(1), "all", "[]"}},
		},
		apiImportTableResponse("departments", []string{"id", "department_id", "name", "org_id", "parent_id"}, [][]driver.Value{
			{int64(1), "dept-1", "Product", "default", ""},
		}),
		scopeOptionsCountResponse(1),
		scopeOptionsListEmployeesResponse([][]driver.Value{
			{int64(2), "u-1", "Alice", "dept-1", "active", "default"},
		}),
		scopeOptionsEmployeeProfilesResponse([][]driver.Value{
			{int64(1), "u-1", "E001", "default"},
		}),
	)
	t.Cleanup(func() { database.DB = originalDB })
}

func installPerformanceScopeOptionsTestDB(t *testing.T, permissionCodes, menuKeys []string, scope string, departmentKeys []string) {
	t.Helper()
	deptJSON := "[]"
	if len(departmentKeys) > 0 {
		b, _ := json.Marshal(departmentKeys)
		deptJSON = string(b)
	}
	menuJSON := "[]"
	if len(menuKeys) > 0 {
		b, _ := json.Marshal(menuKeys)
		menuJSON = string(b)
	}
	if scope == "" {
		scope = "all"
	}

	originalDB := database.DB
	queries := []apiImportQueryResponse{
		apiPerformanceRouterSessionResponse(),
		// JWT 用户：tester
		apiImportQueryResponse{
			match: func(query string, args []driver.NamedValue) bool {
				lower := strings.ToLower(query)
				if strings.Contains(lower, "count(*)") || strings.Contains(lower, "employee_profiles") {
					return false
				}
				if !strings.Contains(lower, "from `users`") && !strings.Contains(lower, "from users") {
					return false
				}
				for _, a := range args {
					if a.Value == "tester" {
						return true
					}
				}
				return false
			},
			columns: []string{"id", "user_id", "name", "department_id", "status", "org_id"},
			rows:    [][]driver.Value{{int64(1), "tester", "Tester", "dept-1", "active", "default"}},
		},
		apiPerformanceRouterRolesResponse(permissionCodes),
		apiPerformanceRouterPermissionsResponse(permissionCodes),
		apiPerformanceRouterTableResponse("menu_permissions", []string{"id", "role_id", "menu_keys"}, nil),
		// 必须显式 data_permissions：无配置时 ResolveUserScope 会落 self，导致只返回 tester。
		apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "data_permissions")
			},
			columns: []string{"id", "role_id", "scope", "department_keys"},
			rows:    [][]driver.Value{{int64(1), int64(1), scope, deptJSON}},
		},
		apiImportTableResponse("departments", []string{"id", "department_id", "name", "org_id", "parent_id"}, [][]driver.Value{
			{int64(1), "dept-1", "Product", "default", ""},
			{int64(2), "dept-2", "Sales", "default", ""},
		}),
		scopeOptionsCountResponse(2),
		scopeOptionsListEmployeesResponse([][]driver.Value{
			{int64(2), "u-1", "Alice", "dept-1", "active", "default"},
			{int64(3), "u-2", "Bob", "dept-2", "active", "default"},
		}),
		scopeOptionsEmployeeProfilesResponse([][]driver.Value{
			{int64(1), "tester", "E-tester", "default"},
			{int64(2), "u-1", "E001", "default"},
			{int64(3), "u-2", "E002", "default"},
		}),
	}
	if len(menuKeys) > 0 {
		queries = append(queries, apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "menu_permissions")
			},
			columns: []string{"id", "role_id", "menu_keys"},
			rows:    [][]driver.Value{{int64(1), int64(1), menuJSON}},
		})
	}
	if len(menuKeys) > 0 && len(permissionCodes) == 0 {
		queries = append(queries, apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				q := strings.ToLower(query)
				return (strings.Contains(q, "from `roles`") || strings.Contains(q, "from roles")) &&
					strings.Contains(q, "user_roles")
			},
			columns: []string{"id", "name", "description"},
			rows:    [][]driver.Value{{int64(1), "menu-role", "menu only"}},
		})
	}

	database.DB = newAPIPerformanceImportStubDB(t, queries...)
	t.Cleanup(func() { database.DB = originalDB })
	_ = time.Now
	_ = middleware.RequestDB
}

func installPerformanceScopeOptionsDataScopeDB(t *testing.T, scope string, departmentKeys []string) *scopeOptionsQueryObservation {
	t.Helper()
	observation := &scopeOptionsQueryObservation{}
	deptJSON := "[]"
	if len(departmentKeys) > 0 {
		b, _ := json.Marshal(departmentKeys)
		deptJSON = string(b)
	}
	if scope == "" {
		scope = "all"
	}

	listRows := [][]driver.Value{
		{int64(1), "tester", "Tester", "dept-1", "active", "default"},
		{int64(2), "u-1", "Alice", "dept-1", "active", "default"},
		{int64(3), "u-2", "Bob", "dept-2", "active", "default"},
	}
	countN := int64(3)
	switch scope {
	case "department":
		listRows = [][]driver.Value{
			{int64(2), "u-1", "Alice", "dept-1", "active", "default"},
		}
		countN = 1
	case "self":
		listRows = [][]driver.Value{
			{int64(1), "tester", "Tester", "dept-1", "active", "default"},
		}
		countN = 1
	}

	originalDB := database.DB
	database.DB = newAPIPerformanceImportStubDB(t,
		apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				q := strings.ToLower(query)
				return (strings.Contains(q, "from `roles`") || strings.Contains(q, "from roles")) &&
					strings.Contains(q, "user_roles")
			},
			columns: []string{"id", "name", "description"},
			rows:    [][]driver.Value{{int64(1), "scoped-role", "scoped"}},
		},
		apiImportQueryResponse{
			match: func(query string, _ []driver.NamedValue) bool {
				return strings.Contains(strings.ToLower(query), "data_permissions")
			},
			columns: []string{"id", "role_id", "scope", "department_keys"},
			rows:    [][]driver.Value{{int64(1), int64(1), scope, deptJSON}},
		},
		apiImportTableResponse("departments", []string{"id", "department_id", "name", "org_id", "parent_id", "extension"}, [][]driver.Value{
			{int64(1), "dept-1", "Product", "default", "", nil},
			{int64(2), "dept-2", "Sales", "default", "", nil},
		}),
		// self 模式 / JWT：users 单表
		scopeOptionsUsersSelectResponse([][]driver.Value{
			{int64(1), "tester", "Tester", "dept-1", "active", "default"},
		}, observation),
		scopeOptionsCountResponse(countN),
		scopeOptionsListEmployeesResponse(listRows, observation),
		scopeOptionsEmployeeProfilesResponse([][]driver.Value{
			{int64(1), "tester", "E-tester", "default"},
			{int64(2), "u-1", "E001", "default"},
			{int64(3), "u-2", "E002", "default"},
		}),
		apiImportTableResponse("menu_permissions", []string{"id", "role_id", "menu_keys"}, nil),
		apiImportTableResponse("permissions", []string{"id", "code"}, nil),
		apiImportTableResponse("role_permissions", []string{"id", "role_id", "permission_id"}, nil),
	)
	t.Cleanup(func() { database.DB = originalDB })
	return observation
}
