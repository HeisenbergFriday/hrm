package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestReadAndRestoreRequestBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/example", strings.NewReader(`{"name":"demo"}`))

	body, err := readAndRestoreRequestBody(req, 1024)
	if err != nil {
		t.Fatalf("readAndRestoreRequestBody() error = %v", err)
	}
	if string(body) != `{"name":"demo"}` {
		t.Fatalf("body = %q", string(body))
	}

	restored, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("read restored body error = %v", err)
	}
	if string(restored) != string(body) {
		t.Fatalf("restored body = %q, want %q", string(restored), string(body))
	}
}

func TestReadAndRestoreRequestBodyTooLarge(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/example", strings.NewReader("abcdef"))

	if _, err := readAndRestoreRequestBody(req, 3); err != errIdempotencyBodyTooLarge {
		t.Fatalf("error = %v, want errIdempotencyBodyTooLarge", err)
	}
}

func TestIdempotencyResponseWriterLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	writer := &idempotencyResponseWriter{
		ResponseWriter: context.Writer,
		maxBytes:       4,
	}

	if _, err := writer.Write([]byte("abcd")); err != nil {
		t.Fatalf("first write error = %v", err)
	}
	if writer.overflow {
		t.Fatalf("overflow after first write = true, want false")
	}
	if writer.body.String() != "abcd" {
		t.Fatalf("captured body = %q", writer.body.String())
	}

	if _, err := writer.Write([]byte("e")); err != nil {
		t.Fatalf("second write error = %v", err)
	}
	if !writer.overflow {
		t.Fatalf("overflow after second write = false, want true")
	}
	if writer.body.Len() != 0 {
		t.Fatalf("captured body length = %d, want 0 after overflow", writer.body.Len())
	}
}

func TestIsIdempotencyMethod(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		if !isIdempotencyMethod(method) {
			t.Fatalf("isIdempotencyMethod(%s) = false, want true", method)
		}
	}
	if isIdempotencyMethod(http.MethodGet) {
		t.Fatalf("isIdempotencyMethod(GET) = true, want false")
	}
}

func TestHashRequestIncludesBody(t *testing.T) {
	left := hashRequest(http.MethodPost, "/api/v1/example", "", []byte(`{"name":"left"}`))
	right := hashRequest(http.MethodPost, "/api/v1/example", "", []byte(`{"name":"right"}`))
	if left == right {
		t.Fatalf("hashRequest should differ when body differs")
	}
}

func TestHashDigestIncludesOrgID(t *testing.T) {
	// 同一 user + key 在不同 org 下 digest 必须不同，允许跨组织复用 Idempotency-Key。
	left := hashDigest("muteng", "user-1", http.MethodPost, "/api/v1/x", "key-1")
	right := hashDigest("xiaotie", "user-1", http.MethodPost, "/api/v1/x", "key-1")
	if left == right {
		t.Fatalf("digest must differ across orgs for same user/key")
	}
	// 同一组织相同输入必须稳定。
	again := hashDigest("muteng", "user-1", http.MethodPost, "/api/v1/x", "key-1")
	if left != again {
		t.Fatalf("digest unstable for same inputs")
	}
}

func TestIdempotencyOrgIDFromContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)

	// Anonymous/unauthenticated: use non-tenant sentinel, never "default".
	if got := idempotencyOrgID(c); got != unauthenticatedIdempotencyOrg {
		t.Fatalf("empty context org = %q, want %q", got, unauthenticatedIdempotencyOrg)
	}

	c.Set("orgID", "muteng")
	if got := idempotencyOrgID(c); got != "muteng" {
		t.Fatalf("org = %q, want muteng", got)
	}
}

func TestResolveIdempotencyOrgID_AuthenticatedMissingOrgFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/x", nil)
	c.Set("userID", "alice")

	orgID, err := resolveIdempotencyOrgID(c)
	if err == nil {
		t.Fatalf("expected missing-org error, got orgID=%q", orgID)
	}
	if orgID != "" {
		t.Fatalf("orgID = %q, want empty on error", orgID)
	}
	// Must not silently land on default tenant.
	if got := idempotencyOrgID(c); got != "" {
		t.Fatalf("idempotencyOrgID on auth-missing-org = %q, want empty", got)
	}
}

func TestHashDigestCrossOrgIsolationAndUnauthenticatedNamespace(t *testing.T) {
	user, method, route, key := "user-1", http.MethodPost, "/api/v1/x", "key-1"
	a := hashDigest("muteng", user, method, route, key)
	b := hashDigest("xiaotie", user, method, route, key)
	anon := hashDigest(unauthenticatedIdempotencyOrg, user, method, route, key)
	def := hashDigest("default", user, method, route, key)
	if a == b {
		t.Fatal("A/B org digests must not collide")
	}
	if anon == def {
		t.Fatal("anonymous namespace must not collide with default tenant")
	}
	if anon == a || anon == b {
		t.Fatal("anonymous namespace must not collide with real orgs")
	}
}
