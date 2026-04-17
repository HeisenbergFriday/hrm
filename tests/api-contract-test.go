package tests

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"peopleops/internal/api"

	"github.com/stretchr/testify/assert"
)

// API响应结构
type APIResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// 测试API契约
func TestAPIContract(t *testing.T) {
	// 创建gin引擎
	router := api.SetupRouter()

	// 测试用例
	testCases := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		expectedCode   int
		expectedMessage string
	}{
		// 健康检查
		{
			name:           "健康检查",
			method:         "GET",
			path:           "/health",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "ok",
		},
		
		// 认证相关
		{
			name:           "登录成功",
			method:         "POST",
			path:           "/api/v1/auth/login",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "登出",
			method:         "POST",
			path:           "/api/v1/auth/logout",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取当前用户信息",
			method:         "GET",
			path:           "/api/v1/auth/me",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 用户相关
		{
			name:           "获取用户列表",
			method:         "GET",
			path:           "/api/v1/users",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取用户详情",
			method:         "GET",
			path:           "/api/v1/users/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "更新用户信息",
			method:         "PUT",
			path:           "/api/v1/users/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 部门相关
		{
			name:           "获取部门列表",
			method:         "GET",
			path:           "/api/v1/departments",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取部门详情",
			method:         "GET",
			path:           "/api/v1/departments/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 组织与员工模块
		{
			name:           "获取部门树",
			method:         "GET",
			path:           "/api/v1/org/departments/tree",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取员工列表",
			method:         "GET",
			path:           "/api/v1/org/employees",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取员工详情",
			method:         "GET",
			path:           "/api/v1/org/employees/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "同步组织数据",
			method:         "POST",
			path:           "/api/v1/org/sync",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 考勤模块
		{
			name:           "获取考勤记录",
			method:         "GET",
			path:           "/api/v1/attendance/records",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取考勤统计",
			method:         "GET",
			path:           "/api/v1/attendance/stats",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "同步考勤数据",
			method:         "POST",
			path:           "/api/v1/attendance/sync",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "导出考勤数据",
			method:         "POST",
			path:           "/api/v1/attendance/export",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取导出记录",
			method:         "GET",
			path:           "/api/v1/attendance/exports",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取最近同步时间",
			method:         "GET",
			path:           "/api/v1/attendance/last-sync",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 审批模块
		{
			name:           "获取审批模板",
			method:         "GET",
			path:           "/api/v1/approvals/templates",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取审批实例",
			method:         "GET",
			path:           "/api/v1/approvals/instances",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取审批详情",
			method:         "GET",
			path:           "/api/v1/approvals/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "同步审批数据",
			method:         "POST",
			path:           "/api/v1/approvals/sync",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 权限管理模块
		{
			name:           "获取角色列表",
			method:         "GET",
			path:           "/api/v1/permission/roles",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "创建角色",
			method:         "POST",
			path:           "/api/v1/permission/roles",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取权限列表",
			method:         "GET",
			path:           "/api/v1/permission/permissions",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 审计日志模块
		{
			name:           "获取审计日志",
			method:         "GET",
			path:           "/api/v1/audit/logs",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 任务中心模块
		{
			name:           "获取任务列表",
			method:         "GET",
			path:           "/api/v1/jobs",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "运行任务",
			method:         "POST",
			path:           "/api/v1/jobs/1/run",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 员工档案中心模块
		{
			name:           "获取员工档案列表",
			method:         "GET",
			path:           "/api/v1/employee/profiles",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取员工档案详情",
			method:         "GET",
			path:           "/api/v1/employee/profiles/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "创建员工档案",
			method:         "POST",
			path:           "/api/v1/employee/profiles",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "更新员工档案",
			method:         "PUT",
			path:           "/api/v1/employee/profiles/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取转岗列表",
			method:         "GET",
			path:           "/api/v1/employee/transfers",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "创建转岗申请",
			method:         "POST",
			path:           "/api/v1/employee/transfers",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取离职列表",
			method:         "GET",
			path:           "/api/v1/employee/resignations",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "创建离职申请",
			method:         "POST",
			path:           "/api/v1/employee/resignations",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取入职列表",
			method:         "GET",
			path:           "/api/v1/employee/onboardings",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "创建入职申请",
			method:         "POST",
			path:           "/api/v1/employee/onboardings",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 人才分析模块
		{
			name:           "获取人才分析列表",
			method:         "GET",
			path:           "/api/v1/talent/analysis",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取人才分析详情",
			method:         "GET",
			path:           "/api/v1/talent/analysis/1",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "创建人才分析",
			method:         "POST",
			path:           "/api/v1/talent/analysis",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		
		// 同步相关
		{
			name:           "同步部门数据",
			method:         "POST",
			path:           "/api/v1/sync/departments",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "同步用户数据",
			method:         "POST",
			path:           "/api/v1/sync/users",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
		{
			name:           "获取同步状态",
			method:         "GET",
			path:           "/api/v1/sync/status",
			expectedStatus: http.StatusOK,
			expectedCode:   http.StatusOK,
			expectedMessage: "success",
		},
	}

	// 执行测试用例
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 创建测试请求
			req, err := http.NewRequest(tc.method, tc.path, nil)
			assert.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			// 创建响应记录器
			w := httptest.NewRecorder()

			// 执行请求
			router.ServeHTTP(w, req)

			// 检查响应状态码
			assert.Equal(t, tc.expectedStatus, w.Code)

			// 解析响应体
			var response APIResponse
			err = json.Unmarshal(w.Body.Bytes(), &response)
			assert.NoError(t, err)

			// 检查响应结构
			assert.Equal(t, tc.expectedCode, response.Code)
			assert.Equal(t, tc.expectedMessage, response.Message)
			assert.NotNil(t, response.Data)
		})
	}
}
