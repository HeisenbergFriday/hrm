package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestPerformanceHandlersMissingOrgContextSingleResponse 确保缺 org 时只写一次 401 JSON。
func TestPerformanceHandlersMissingOrgContextSingleResponse(t *testing.T) {
	handlers := []struct {
		name    string
		handler func(*gin.Context)
	}{
		{"GetPerformanceScopeOptions", GetPerformanceScopeOptions},
		{"GetPerformanceActivities", GetPerformanceActivities},
	}

	for _, tt := range handlers {
		t.Run(tt.name, func(t *testing.T) {
			// 不设置 orgID，模拟 JWT 缺组织上下文。
			c, recorder := performanceHandlerContextAs(t, "", "user-1")
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/probe", nil)

			tt.handler(c)

			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401; body = %s", recorder.Code, recorder.Body.String())
			}

			body := recorder.Body.Bytes()
			// 必须能被 json.Unmarshal 成单一 Response，不能出现两段 JSON 拼接。
			var resp Response
			if err := json.Unmarshal(body, &resp); err != nil {
				t.Fatalf("body must be a single JSON object: %v; body=%s", err, string(body))
			}
			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("code = %d, want 401", resp.Code)
			}
			if !strings.Contains(resp.Message, "组织") {
				t.Fatalf("message should mention org context, got %q", resp.Message)
			}
			// 额外防御：body 中不应出现第二个 {"code":
			if strings.Count(string(body), `"code"`) != 1 {
				t.Fatalf("expected exactly one Response code field, body=%s", string(body))
			}
		})
	}
}

func TestCurrentOrgIDDoesNotWriteResponse(t *testing.T) {
	c, recorder := performanceHandlerContextAs(t, "", "user-1")
	_, err := currentOrgID(c)
	if err == nil {
		t.Fatalf("expected ErrMissingOrgContext")
	}
	if recorder.Body.Len() != 0 {
		t.Fatalf("currentOrgID must not write HTTP body, got %s", recorder.Body.String())
	}
	if recorder.Code != http.StatusOK && recorder.Code != 0 {
		// httptest.ResponseRecorder 默认 200；关键是 body 为空。
		t.Logf("recorder.Code=%d (ok if body empty)", recorder.Code)
	}
}

func TestRespondScopeErrorMapsMissingOrgTo401Once(t *testing.T) {
	c, recorder := performanceHandlerContextAs(t, "test-org", "user-1")
	respondScopeError(c, ErrMissingOrgContext, "fallback")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	var resp Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v body=%s", err, recorder.Body.String())
	}
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("code = %d, want 401", resp.Code)
	}
}
