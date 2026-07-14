package api

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRequestBaseURLUsesForwardedHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	req := httptest.NewRequest("GET", "http://internal.local/login", nil)
	req.Host = "internal.local:8080"
	req.Header.Set("X-Forwarded-Proto", "https, http")
	req.Header.Set("X-Forwarded-Host", "peopleops.example.com, internal.local:8080")
	c.Request = req

	got := requestBaseURL(c)
	want := "https://peopleops.example.com"
	if got != want {
		t.Fatalf("requestBaseURL() = %q, want %q", got, want)
	}
}

func TestNormalizeForwardedHostStripsSchemeAndSlash(t *testing.T) {
	t.Parallel()

	got := normalizeForwardedHost("https://peopleops.example.com/")
	want := "peopleops.example.com"
	if got != want {
		t.Fatalf("normalizeForwardedHost() = %q, want %q", got, want)
	}
}
