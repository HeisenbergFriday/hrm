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
