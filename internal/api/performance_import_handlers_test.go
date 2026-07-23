package api

import (
	"bytes"
	"database/sql/driver"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func performanceImportUploadRequest(t *testing.T, fileName string, content []byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/performance/imports/analyze", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestAnalyzePerformanceActivityImportRejectsMissingFile(t *testing.T) {
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/imports/analyze", nil)

	AnalyzePerformanceActivityImport(c)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "请上传绩效 Excel 文件") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAnalyzePerformanceActivityImportRejectsNonXLSX(t *testing.T) {
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = performanceImportUploadRequest(t, "performance.xls", []byte("xls"))

	AnalyzePerformanceActivityImport(c)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "仅支持 .xlsx") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestAnalyzePerformanceActivityImportRejectsOversizedFile(t *testing.T) {
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = performanceImportUploadRequest(t, "performance.xlsx", bytes.Repeat([]byte{'x'}, performanceActivityImportUploadMaxBytes+1))

	AnalyzePerformanceActivityImport(c)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "不能超过 10MB") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestCommitPerformanceActivityImportRejectsInvalidJSON(t *testing.T) {
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodPost, "/api/v1/performance/imports/batch-1/commit", strings.NewReader("{"))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{{Key: "batch_id", Value: "batch-1"}}

	CommitPerformanceActivityImport(c)

	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "提交参数格式错误") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetPerformanceActivityImportBatchReturnsNotFound(t *testing.T) {
	performanceHandlerTestDBWith(t, apiImportTableResponse(
		"performance_import_batches",
		[]string{"id", "org_id", "batch_key"},
		[][]driver.Value{},
	))
	c, recorder := performanceHandlerAdminContext(t)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/performance/imports/missing", nil)
	c.Params = gin.Params{{Key: "batch_id", Value: "missing"}}

	GetPerformanceActivityImportBatch(c)

	if recorder.Code != http.StatusNotFound || !strings.Contains(recorder.Body.String(), "导入批次不存在") {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}
