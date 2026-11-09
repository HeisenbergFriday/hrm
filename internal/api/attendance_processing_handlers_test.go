package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestLegacyAttendanceProcessingMissingMonthReturnsError(t *testing.T) {
	for _, module := range []string{"leave", "overtime", "subsidy", "final", "parttime"} {
		t.Run(module, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			if err := writer.WriteField("unrelated", "value"); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/attendance/processing/"+module, &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = req
			processAttendanceViaToolbox(c, module)
			if w.Code != http.StatusBadRequest || w.Header().Get("Content-Disposition") != "" {
				t.Fatalf("expected HTTP 400 without attachment, got %d: %s", w.Code, w.Body.String())
			}
			var response Response
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Message, "请先选择处理月份") {
				t.Fatalf("expected actionable missing-month error, got %+v", response)
			}
		})
	}
}
