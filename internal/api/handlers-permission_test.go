package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// 测试权限控制
func TestPermissionControl(t *testing.T) {
	// 创建gin引擎
	router := SetupRouter()

	// 测试用例：未授权访问需要认证的接口
	t.Run("未授权访问需要认证的接口", func(t *testing.T) {
		// 创建测试请求
		req, err := http.NewRequest("GET", "/api/v1/users", nil)
		assert.NoError(t, err)

		// 创建响应记录器
		w := httptest.NewRecorder()

		// 执行请求
		router.ServeHTTP(w, req)

		// 检查响应
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	// 测试用例：获取角色列表
	t.Run("获取角色列表", func(t *testing.T) {
		// 创建测试请求
		req, err := http.NewRequest("GET", "/api/v1/permission/roles", nil)
		assert.NoError(t, err)
		req.Header.Set("Authorization", "Bearer mock_token")

		// 创建响应记录器
		w := httptest.NewRecorder()

		// 执行请求
		router.ServeHTTP(w, req)

		// 检查响应
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "items")
		assert.Contains(t, w.Body.String(), "total")
	})

	// 测试用例：创建角色
	t.Run("创建角色", func(t *testing.T) {
		// 创建测试请求
		reqBody := strings.NewReader(`{"name": "测试角色", "description": "测试角色描述"}`)
		req, err := http.NewRequest("POST", "/api/v1/permission/roles", reqBody)
		assert.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer mock_token")

		// 创建响应记录器
		w := httptest.NewRecorder()

		// 执行请求
		router.ServeHTTP(w, req)

		// 检查响应
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "role")
		assert.Contains(t, w.Body.String(), "测试角色")
	})

	// 测试用例：获取权限列表
	t.Run("获取权限列表", func(t *testing.T) {
		// 创建测试请求
		req, err := http.NewRequest("GET", "/api/v1/permission/permissions", nil)
		assert.NoError(t, err)
		req.Header.Set("Authorization", "Bearer mock_token")

		// 创建响应记录器
		w := httptest.NewRecorder()

		// 执行请求
		router.ServeHTTP(w, req)

		// 检查响应
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), "items")
		assert.Contains(t, w.Body.String(), "total")
	})
}
