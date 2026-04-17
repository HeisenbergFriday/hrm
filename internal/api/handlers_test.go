package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 测试健康检查接口
func TestHealthCheck(t *testing.T) {
	// 创建gin引擎
	router := gin.Default()
	router.GET("/health", HealthCheck)

	// 创建测试请求
	req, err := http.NewRequest("GET", "/health", nil)
	assert.NoError(t, err)

	// 创建响应记录器
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "ok")
}

// 测试登录接口
func TestLogin(t *testing.T) {
	// 创建gin引擎
	router := gin.Default()
	router.POST("/api/v1/auth/login", Login)

	// 创建测试请求
	reqBody := strings.NewReader(`{"username": "admin", "password": "123456"}`)
	req, err := http.NewRequest("POST", "/api/v1/auth/login", reqBody)
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	// 创建响应记录器
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "token")
}

// 测试获取用户列表接口
func TestGetUsers(t *testing.T) {
	// 创建gin引擎
	router := gin.Default()
	router.GET("/api/v1/users", GetUsers)

	// 创建测试请求
	req, err := http.NewRequest("GET", "/api/v1/users", nil)
	assert.NoError(t, err)

	// 创建响应记录器
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "users")
	assert.Contains(t, w.Body.String(), "total")
}

// 测试获取部门列表接口
func TestGetDepartments(t *testing.T) {
	// 创建gin引擎
	router := gin.Default()
	router.GET("/api/v1/departments", GetDepartments)

	// 创建测试请求
	req, err := http.NewRequest("GET", "/api/v1/departments", nil)
	assert.NoError(t, err)

	// 创建响应记录器
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "departments")
}

// 测试获取考勤记录接口
func TestGetAttendanceRecords(t *testing.T) {
	// 创建gin引擎
	router := gin.Default()
	router.GET("/api/v1/attendance/records", GetAttendanceRecords)

	// 创建测试请求
	req, err := http.NewRequest("GET", "/api/v1/attendance/records", nil)
	assert.NoError(t, err)

	// 创建响应记录器
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "items")
	assert.Contains(t, w.Body.String(), "total")
}

// 测试同步部门数据接口
func TestSyncDepartments(t *testing.T) {
	// 创建gin引擎
	router := gin.Default()
	router.POST("/api/v1/sync/departments", SyncDepartments)

	// 创建测试请求
	req, err := http.NewRequest("POST", "/api/v1/sync/departments", nil)
	assert.NoError(t, err)

	// 创建响应记录器
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "count")
}

// 测试同步用户数据接口
func TestSyncUsers(t *testing.T) {
	// 创建gin引擎
	router := gin.Default()
	router.POST("/api/v1/sync/users", SyncUsers)

	// 创建测试请求
	req, err := http.NewRequest("POST", "/api/v1/sync/users", nil)
	assert.NoError(t, err)

	// 创建响应记录器
	w := httptest.NewRecorder()

	// 执行请求
	router.ServeHTTP(w, req)

	// 检查响应
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "count")
}
