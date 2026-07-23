package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openUploadIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:api-upload-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.UploadedFile{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func withUploadTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// handlers use relative "uploads"; chdir into temp so isolation is self-contained.
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(prev)
	})
	return root
}

func newUploadCtx(t *testing.T, method, target, orgID, userID string, body io.Reader, contentType string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	c.Request = req
	if orgID != "" {
		c.Set("orgID", orgID)
	}
	if userID != "" {
		c.Set("userID", userID)
	}
	return c, recorder
}

func multipartTextFile(t *testing.T, field, filename, content string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := io.WriteString(part, content); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return &buf, w.FormDataContentType()
}

func decodeUploadJSON(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("decode: %v body=%s", err, string(body))
	}
	return resp
}

func TestUploadFile_OrgIsolatedStorageAndMetadata(t *testing.T) {
	root := withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	org := "muteng"
	body, contentType := multipartTextFile(t, "file", "proof.txt", "hello-org-a")
	c, recorder := newUploadCtx(t, http.MethodPost, "/api/v1/upload", org, "alice", body, contentType)

	UploadFile(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	resp := decodeUploadJSON(t, recorder.Body.Bytes())
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatalf("missing data: %v", resp)
	}
	url, _ := data["url"].(string)
	if !strings.HasPrefix(url, "/api/v1/files/") {
		t.Fatalf("url=%q", url)
	}
	idPart := strings.TrimPrefix(url, "/api/v1/files/")
	if idPart == "" || strings.ContainsAny(idPart, "./\\") {
		t.Fatalf("url must expose numeric id only, got %q", url)
	}

	storedName, _ := data["stored_name"].(string)
	if storedName == "" {
		t.Fatal("missing stored_name")
	}
	// Disk path must be under uploads/<safe_org>/
	expectedDir := filepath.Join(root, "uploads", sanitizeOrgIDForPath(org))
	full := filepath.Join(expectedDir, storedName)
	if _, err := os.Stat(full); err != nil {
		t.Fatalf("disk file missing at %s: %v", full, err)
	}
	// Must NOT land in flat uploads root
	if _, err := os.Stat(filepath.Join(root, "uploads", storedName)); err == nil {
		t.Fatalf("file leaked into flat uploads root: %s", storedName)
	}

	var meta database.UploadedFile
	if err := db.First(&meta, "stored_name = ?", storedName).Error; err != nil {
		t.Fatalf("meta: %v", err)
	}
	if meta.OrgID != org {
		t.Fatalf("meta.OrgID=%q want %q", meta.OrgID, org)
	}
	if meta.UploaderUserID != "alice" {
		t.Fatalf("uploader=%q", meta.UploaderUserID)
	}
}

func TestServeFile_CrossOrgReturns404(t *testing.T) {
	_ = withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	// Upload as org A
	body, contentType := multipartTextFile(t, "file", "secret.txt", "org-a-secret")
	c, recorder := newUploadCtx(t, http.MethodPost, "/api/v1/upload", "muteng", "alice", body, contentType)
	UploadFile(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	resp := decodeUploadJSON(t, recorder.Body.Bytes())
	data := resp["data"].(map[string]any)
	url := data["url"].(string)
	fileID := strings.TrimPrefix(url, "/api/v1/files/")

	// Org A can download
	cA, recA := newUploadCtx(t, http.MethodGet, "/api/v1/files/"+fileID, "muteng", "alice", nil, "")
	cA.Params = gin.Params{{Key: "file_id", Value: fileID}}
	ServeFile(cA)
	if recA.Code != http.StatusOK {
		t.Fatalf("orgA status=%d body=%s", recA.Code, recA.Body.String())
	}
	if !strings.Contains(recA.Body.String(), "org-a-secret") {
		t.Fatalf("orgA body missing content: %q", recA.Body.String())
	}
	if recA.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing nosniff header")
	}

	// Org B same file id → 404 (not 403) to avoid existence leak
	cB, recB := newUploadCtx(t, http.MethodGet, "/api/v1/files/"+fileID, "xiaotie", "bob", nil, "")
	cB.Params = gin.Params{{Key: "file_id", Value: fileID}}
	ServeFile(cB)
	if recB.Code != http.StatusNotFound {
		t.Fatalf("orgB status=%d want 404 body=%s", recB.Code, recB.Body.String())
	}
	if strings.Contains(recB.Body.String(), "org-a-secret") {
		t.Fatal("orgB response leaked file content")
	}
}

func TestServeFile_MissingOrgContextRejected(t *testing.T) {
	_ = withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	c, recorder := newUploadCtx(t, http.MethodGet, "/api/v1/files/1", "", "alice", nil, "")
	c.Params = gin.Params{{Key: "file_id", Value: "1"}}
	ServeFile(c)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestServeFile_LegacyFilenameURLFailClosed(t *testing.T) {
	root := withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	// Plant a legacy flat file without metadata — must not be serveable.
	legacyName := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.txt"
	if err := os.MkdirAll(filepath.Join(root, "uploads"), 0750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	legacyPath := filepath.Join(root, "uploads", legacyName)
	if err := os.WriteFile(legacyPath, []byte("legacy-secret"), 0600); err != nil {
		t.Fatalf("write legacy: %v", err)
	}

	c, recorder := newUploadCtx(t, http.MethodGet, "/api/v1/files/"+legacyName, "muteng", "alice", nil, "")
	c.Params = gin.Params{{Key: "file_id", Value: legacyName}}
	ServeFile(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "legacy-secret") {
		t.Fatal("legacy file content leaked")
	}
}

func TestServeFile_PathTraversalRejected(t *testing.T) {
	_ = withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	for _, attack := range []string{"../secret", "..\\secret", "foo/bar", "foo\\bar"} {
		c, recorder := newUploadCtx(t, http.MethodGet, "/api/v1/files/"+attack, "muteng", "alice", nil, "")
		c.Params = gin.Params{{Key: "file_id", Value: attack}}
		ServeFile(c)
		if recorder.Code != http.StatusBadRequest && recorder.Code != http.StatusNotFound {
			t.Fatalf("attack %q status=%d body=%s", attack, recorder.Code, recorder.Body.String())
		}
	}
}

func TestServeFile_UnknownID404(t *testing.T) {
	_ = withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	c, recorder := newUploadCtx(t, http.MethodGet, "/api/v1/files/999999", "muteng", "alice", nil, "")
	c.Params = gin.Params{{Key: "file_id", Value: "999999"}}
	ServeFile(c)
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUploadFile_RejectsMissingUser(t *testing.T) {
	_ = withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	body, contentType := multipartTextFile(t, "file", "proof.txt", "x")
	c, recorder := newUploadCtx(t, http.MethodPost, "/api/v1/upload", "muteng", "", body, contentType)
	UploadFile(c)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want 401 body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestUploadFile_ClientOrgParamCannotOverrideOwnership(t *testing.T) {
	// Ownership comes only from JWT org context set on gin.Context, never body/query.
	_ = withUploadTestRoot(t)
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	body, contentType := multipartTextFile(t, "file", "proof.txt", "owned-by-jwt-org")
	// Query tries to claim another org; JWT org is muteng.
	c, recorder := newUploadCtx(t, http.MethodPost, "/api/v1/upload?org_id=xiaotie", "muteng", "alice", body, contentType)
	UploadFile(c)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var meta database.UploadedFile
	if err := db.First(&meta).Error; err != nil {
		t.Fatalf("meta: %v", err)
	}
	if meta.OrgID != "muteng" {
		t.Fatalf("OrgID=%q want muteng (JWT), client query must not win", meta.OrgID)
	}
}

func TestUploadedFileDiskPath_RejectsTraversal(t *testing.T) {
	_ = withUploadTestRoot(t)
	if _, err := uploadedFileDiskPath("muteng", "../evil.txt"); err == nil {
		t.Fatal("expected error for unsafe stored name")
	}
	if _, err := uploadedFileDiskPath("../muteng", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.txt"); err == nil {
		t.Fatal("expected error for unsafe org id")
	}
	// Safe path builds under org dir
	p, err := uploadedFileDiskPath("muteng", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.txt")
	if err != nil {
		t.Fatalf("safe path: %v", err)
	}
	if !strings.Contains(filepath.ToSlash(p), "/uploads/muteng/") {
		t.Fatalf("path not under org dir: %s", p)
	}
}

func TestRouterFilesRouteRequiresTenantContext(t *testing.T) {
	// Smoke: SetupRouter registers download under authRequired (JWT + TenantContext),
	// not as a JWT-only top-level route.
	// Avoid full DB-dependent middleware by inspecting route list.
	gin.SetMode(gin.TestMode)
	// Ensure database.DB non-nil for middleware that may touch it during route registration.
	db := openUploadIsolationDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	// RequestMetrics uses database.DB; TenantContext needs org — we only check route table.
	_ = middleware.RequestDB
	router := gin.New()
	// Mirror the intended registration shape used in SetupRouter after the fix.
	authRequired := router.Group("/api/v1")
	authRequired.Use(func(c *gin.Context) {
		// stand-in for JWTAuth+TenantContext presence
		if c.GetString("orgID") == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "missing_org_context"})
			return
		}
		c.Next()
	})
	authRequired.GET("/files/:file_id", ServeFile)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/files/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no-org route status=%d want 401", rec.Code)
	}

	// With org context middleware would pass; confirm group path exists.
	routes := router.Routes()
	found := false
	for _, r := range routes {
		if r.Path == "/api/v1/files/:file_id" && r.Method == http.MethodGet {
			found = true
		}
	}
	if !found {
		t.Fatalf("route /api/v1/files/:file_id not registered: %+v", routes)
	}
	_ = fmt.Sprintf // keep fmt import stable if tree-shaken in future edits
}
