package api

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"log"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/repository"
	"peopleops/internal/requestmeta"
	"peopleops/internal/service"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// 钉钉登录 state 存储（防 CSRF）
var (
	dingtalkStates   = make(map[string]loginState)
	dingtalkStatesMu sync.Mutex
	dingtalkStateTTL = 5 * time.Minute
)

// 组织同步进程内并发锁：同一组织的用户、部门和全量同步共享同一门闩。
// 该锁只适用于单实例部署；多实例部署需要后续引入分布式锁或任务队列。
var orgSyncLocks sync.Map // orgID -> *sync.Mutex

type orgSyncStatusUpdate struct {
	SyncType     string
	Status       string
	Message      string
	RequestID    string
	DurationMS   int64
	ErrorCode    string
	SuccessCount int
	FailCount    int
	Details      map[string]interface{}
}

// 测试缝隙：可替换的钉钉同步函数，避免测试时真实调用钉钉 API
var (
	syncDepartmentsForOrg     = dingtalk.SyncDepartmentsForOrg
	syncUsersForOrg           = dingtalk.SyncUsersForOrg
	syncUsersWithDeptsForOrg  = dingtalk.SyncUsersWithDeptsForOrg
	persistOrgSyncDepartments = func(c *gin.Context, orgID string, depts []dingtalk.DeptInfo) (service.OrgDepartmentSyncResult, error) {
		orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
		return orgService.SyncDepartmentsWithChangeLog(orgID, dingtalkDepartmentsToOrgSyncItems(orgID, depts), "dingtalk_sync")
	}
	writeOrgSyncStatusForRequest = func(c *gin.Context, orgID string, update orgSyncStatusUpdate) error {
		update.Message = sanitizeSyncLogText(update.Message)
		update.RequestID = sanitizeSyncLogText(update.RequestID)
		update.ErrorCode = sanitizeSyncLogText(update.ErrorCode)
		err := service.NewSyncService(middleware.RequestDB(c)).UpdateSyncStatusDetails(&database.SyncStatus{
			OrgID:        orgID,
			Type:         update.SyncType,
			LastSyncTime: time.Now(),
			Status:       update.Status,
			Message:      update.Message,
			RequestID:    update.RequestID,
			DurationMS:   update.DurationMS,
			ErrorCode:    update.ErrorCode,
			SuccessCount: update.SuccessCount,
			FailCount:    update.FailCount,
			Details:      update.Details,
		})
		if err != nil {
			log.Printf("[sync-status] type=%s status=%s result=failed error_type=%T", update.SyncType, update.Status, err)
		}
		return err
	}
)

var (
	errDingTalkOrgSelectionRequired       = errors.New("dingtalk org_id is required when multiple organizations are configured")
	errDingTalkLoginOrgResolutionFailed   = errors.New("unable to resolve local organization from dingtalk login identity")
	errDingTalkLoginOrgResolutionConflict = errors.New("dingtalk login identity matches multiple local organizations")
	errDingTalkSelectedOrgMismatch        = errors.New("selected dingtalk organization does not match callback organization")
	errDingTalkSelectedOrgUnverified      = errors.New("unable to verify selected dingtalk organization from callback identity")
)

type loginState struct {
	CreatedAt  time.Time
	OrgID      string
	OAuthOrgID string
}

func updateSyncStatus(syncService *service.SyncService, orgID, syncType, status, message string) {
	if syncService == nil {
		return
	}
	if err := syncService.UpdateSyncStatus(orgID, syncType, status, message); err != nil {
		log.Printf("[sync-status] type=%s status=%s result=failed error_type=%T", syncType, status, err)
	}
}

func generateLoginState(orgID string) string {
	return generateLoginStateWithOAuthOrgID(orgID, "")
}

func generateLoginStateWithOAuthOrgID(orgID, oauthOrgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID != "" {
		orgID = database.NormalizeOrganizationID(orgID)
	}
	oauthOrgID = strings.TrimSpace(oauthOrgID)
	if oauthOrgID != "" {
		oauthOrgID = database.NormalizeOrganizationID(oauthOrgID)
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto random unavailable: %v", err))
	}
	state := hex.EncodeToString(b)
	dingtalkStatesMu.Lock()
	dingtalkStates[state] = loginState{
		CreatedAt:  time.Now(),
		OrgID:      orgID,
		OAuthOrgID: oauthOrgID,
	}
	dingtalkStatesMu.Unlock()
	return state
}

func validateLoginState(state string) (string, bool) {
	entry, ok := validateLoginStateEntry(state)
	if !ok {
		return "", false
	}
	return strings.TrimSpace(entry.OrgID), true
}

func validateLoginStateEntry(state string) (loginState, bool) {
	if state == "" {
		return loginState{}, false
	}
	dingtalkStatesMu.Lock()
	defer dingtalkStatesMu.Unlock()
	entry, ok := dingtalkStates[state]
	if !ok {
		return loginState{}, false
	}
	delete(dingtalkStates, state)
	entry.OrgID = strings.TrimSpace(entry.OrgID)
	entry.OAuthOrgID = strings.TrimSpace(entry.OAuthOrgID)
	return entry, time.Since(entry.CreatedAt) < dingtalkStateTTL
}

func resolveRequestedDingTalkCallbackOrgID(stateOrgID, requestOrgID string) (string, error) {
	stateOrgID = strings.TrimSpace(stateOrgID)
	if stateOrgID != "" {
		stateOrgID = database.NormalizeOrganizationID(stateOrgID)
	}

	requestOrgID = strings.TrimSpace(requestOrgID)
	if requestOrgID != "" {
		requestOrgID = database.NormalizeOrganizationID(requestOrgID)
	}

	if stateOrgID != "" && requestOrgID != "" && stateOrgID != requestOrgID {
		return "", fmt.Errorf("dingtalk org mismatch: state=%s request=%s", stateOrgID, requestOrgID)
	}
	if stateOrgID != "" {
		return stateOrgID, nil
	}
	return requestOrgID, nil
}

func validateDingTalkSelectedOrgIdentity(selectedOrgID string, userInfo map[string]interface{}) error {
	selectedOrgID = strings.TrimSpace(selectedOrgID)
	if selectedOrgID == "" {
		return nil
	}
	selectedOrgID = database.NormalizeOrganizationID(selectedOrgID)

	callbackCorpID := getStringByKeys(userInfo, "corpId", "corpID", "corp_id", "corpid")
	if callbackCorpID != "" {
		cfg, err := dingtalk.ConfigForOrgID(selectedOrgID)
		if err != nil {
			return err
		}
		expectedCorpID := strings.TrimSpace(cfg.NormalizedForAPI().CorpID)
		if expectedCorpID != "" && callbackCorpID != expectedCorpID {
			return fmt.Errorf("%w: selected_org_id=%s expected_corp_id=%s callback_corp_id=%s", errDingTalkSelectedOrgMismatch, selectedOrgID, expectedCorpID, callbackCorpID)
		}
		if expectedCorpID != "" {
			return nil
		}
	}

	associatedUserID := getStringByKeys(userInfo, "associated_user_id", "associatedUserId", "userid", "userId")
	if associatedUserID != "" {
		return nil
	}

	return fmt.Errorf("%w: selected_org_id=%s", errDingTalkSelectedOrgUnverified, selectedOrgID)
}

func resolveDingTalkQRStateOrgID(requestedOrgID string, hasMultipleOrganizations bool) string {
	_ = hasMultipleOrganizations
	requestedOrgID = strings.TrimSpace(requestedOrgID)
	if requestedOrgID == "" {
		requestedOrgID = strings.TrimSpace(os.Getenv("DINGTALK_QR_DEFAULT_ORG_ID"))
	}
	if requestedOrgID == "" {
		return ""
	}
	return database.NormalizeOrganizationID(requestedOrgID)
}

func resolveDingTalkQROAuthOrgID(requestedOrgID string, hasMultipleOrganizations bool) string {
	_ = hasMultipleOrganizations
	requestedOrgID = strings.TrimSpace(requestedOrgID)
	if requestedOrgID == "" {
		requestedOrgID = strings.TrimSpace(os.Getenv("DINGTALK_QR_DEFAULT_ORG_ID"))
	}
	if requestedOrgID == "" {
		return ""
	}
	return database.NormalizeOrganizationID(requestedOrgID)
}

func cleanupOldStates() {
	dingtalkStatesMu.Lock()
	defer dingtalkStatesMu.Unlock()
	for state, entry := range dingtalkStates {
		if time.Since(entry.CreatedAt) > dingtalkStateTTL {
			delete(dingtalkStates, state)
		}
	}
}

// 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

var allowedUploadExtensions = []string{
	".jpg", ".jpeg", ".png", ".gif", ".webp",
	".pdf",
	".docx", ".xlsx", ".pptx",
	".txt", ".csv", ".md",
}

var allowedUploadExtensionSet = func() map[string]struct{} {
	values := make(map[string]struct{}, len(allowedUploadExtensions))
	for _, ext := range allowedUploadExtensions {
		values[ext] = struct{}{}
	}
	return values
}()

const (
	maxUploadImagePixels       = 40_000_000
	maxUploadImageDimension    = 10_000
	maxUploadArchiveFiles      = 1000
	maxUploadArchiveTotalBytes = 100 * 1024 * 1024
	maxUploadArchiveEntryBytes = 50 * 1024 * 1024
	maxUploadArchiveRatio      = 100
)

func allowedUploadExtensionText() string {
	labels := make([]string, 0, len(allowedUploadExtensions))
	for _, ext := range allowedUploadExtensions {
		labels = append(labels, strings.TrimPrefix(ext, "."))
	}
	return strings.Join(labels, "/")
}

func isAllowedUploadExtension(ext string) bool {
	_, ok := allowedUploadExtensionSet[strings.ToLower(ext)]
	return ok
}

func isSafeUploadFilename(filename string) bool {
	filename = strings.TrimSpace(filename)
	if filename == "" || filename != filepath.Base(filename) {
		return false
	}
	if strings.Contains(filename, "..") || strings.ContainsAny(filename, `/\`) {
		return false
	}
	for _, ch := range filename {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return false
	}
	return isAllowedUploadExtension(filepath.Ext(filename))
}

func sanitizeOrgIDForPath(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return ""
	}
	// Fail closed on path-like org identifiers before any mapping.
	if strings.Contains(orgID, "..") || strings.ContainsAny(orgID, `/\`) {
		return ""
	}
	var b strings.Builder
	b.Grow(len(orgID))
	for _, ch := range orgID {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == '-' {
			b.WriteRune(ch)
			continue
		}
		// Map other runes to underscore so org dirs stay path-safe without leaking raw values into traversals.
		b.WriteByte('_')
	}
	out := b.String()
	out = strings.Trim(out, ".")
	if out == "" || out == "." || out == ".." {
		return ""
	}
	return out
}

func uploadRootDir() string {
	return "uploads"
}

func uploadedFileDiskPath(orgID, storedName string) (string, error) {
	safeOrg := sanitizeOrgIDForPath(orgID)
	if safeOrg == "" {
		return "", errors.New("invalid organization id for storage path")
	}
	if !isSafeUploadFilename(storedName) {
		return "", errors.New("invalid stored file name")
	}
	root, err := filepath.Abs(uploadRootDir())
	if err != nil {
		return "", err
	}
	full := filepath.Join(root, safeOrg, storedName)
	// Ensure resolved path stays under uploads/<org>/ (path traversal defense).
	rel, err := filepath.Rel(root, full)
	if err != nil {
		return "", err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path escapes upload root")
	}
	if !strings.HasPrefix(rel, safeOrg+string(filepath.Separator)) && rel != safeOrg {
		return "", errors.New("path escapes organization directory")
	}
	return full, nil
}

func detectUploadContentType(file *multipart.FileHeader, ext string) string {
	src, err := file.Open()
	if err != nil {
		return "application/octet-stream"
	}
	defer func() { _ = src.Close() }()
	header := make([]byte, 512)
	n, err := src.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "application/octet-stream"
	}
	if n == 0 {
		return "application/octet-stream"
	}
	detected := http.DetectContentType(header[:n])
	if detected != "" && detected != "application/octet-stream" {
		return detected
	}
	switch strings.ToLower(ext) {
	case ".pdf":
		return "application/pdf"
	case ".txt":
		return "text/plain; charset=utf-8"
	case ".csv":
		return "text/csv; charset=utf-8"
	case ".md":
		return "text/markdown; charset=utf-8"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return "application/octet-stream"
	}
}

func contentDispositionFilename(originalName, fallback string) string {
	name := strings.TrimSpace(originalName)
	if name == "" {
		name = fallback
	}
	name = filepath.Base(name)
	name = strings.ReplaceAll(name, "\"", "")
	name = strings.ReplaceAll(name, "\r", "")
	name = strings.ReplaceAll(name, "\n", "")
	if name == "" || name == "." || name == ".." {
		name = fallback
	}
	// RFC 5987 filename* for non-ASCII; also provide ASCII fallback.
	ascii := make([]rune, 0, len(name))
	for _, ch := range name {
		if ch < 0x20 || ch > 0x7e || ch == '"' || ch == '\\' {
			ascii = append(ascii, '_')
			continue
		}
		ascii = append(ascii, ch)
	}
	asciiName := string(ascii)
	if asciiName == "" {
		asciiName = fallback
	}
	return fmt.Sprintf("filename=\"%s\"; filename*=UTF-8''%s", asciiName, url.PathEscape(name))
}

func respondUploadedFileNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, Response{
		Code:    http.StatusNotFound,
		Message: "文件不存在",
	})
}

func validateUploadContent(file *multipart.FileHeader, ext string) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	header := make([]byte, 512)
	n, err := src.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	header = header[:n]
	if len(header) == 0 {
		return errors.New("empty file")
	}

	contentType := http.DetectContentType(header)
	ext = strings.ToLower(ext)
	switch ext {
	case ".jpg", ".jpeg":
		if contentType == "image/jpeg" && bytes.HasPrefix(header, []byte{0xff, 0xd8, 0xff}) {
			return validateImageUpload(file)
		}
	case ".png":
		if contentType == "image/png" && bytes.HasPrefix(header, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
			return validateImageUpload(file)
		}
	case ".gif":
		if contentType == "image/gif" && (bytes.HasPrefix(header, []byte("GIF87a")) || bytes.HasPrefix(header, []byte("GIF89a"))) {
			return validateImageUpload(file)
		}
	case ".webp":
		if len(header) >= 12 && bytes.HasPrefix(header, []byte("RIFF")) && string(header[8:12]) == "WEBP" {
			return nil
		}
	case ".pdf":
		if contentType == "application/pdf" && bytes.HasPrefix(header, []byte("%PDF-")) {
			return nil
		}
	case ".txt", ".csv", ".md":
		if isTextUploadContent(header) {
			return nil
		}
	case ".docx", ".xlsx", ".pptx":
		if isZipUploadContent(header) {
			return validateZipUpload(file)
		}
	}

	return fmt.Errorf("file content does not match extension %s", ext)
}

func validateImageUpload(file *multipart.FileHeader) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	config, _, err := image.DecodeConfig(src)
	if err != nil {
		return fmt.Errorf("invalid image: %w", err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return errors.New("invalid image dimensions")
	}
	if config.Width > maxUploadImageDimension || config.Height > maxUploadImageDimension {
		return fmt.Errorf("image dimension exceeds %d", maxUploadImageDimension)
	}
	if config.Width*config.Height > maxUploadImagePixels {
		return fmt.Errorf("image pixels exceeds %d", maxUploadImagePixels)
	}
	return nil
}

func validateZipUpload(file *multipart.FileHeader) error {
	src, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	reader, err := zip.NewReader(src, file.Size)
	if err != nil {
		return fmt.Errorf("invalid zip archive: %w", err)
	}
	if len(reader.File) > maxUploadArchiveFiles {
		return fmt.Errorf("archive contains too many files: %d", len(reader.File))
	}

	var totalUncompressed uint64
	for _, entry := range reader.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if isMacroUploadEntry(entry.Name) {
			return fmt.Errorf("archive contains macro or active content: %s", entry.Name)
		}
		if entry.UncompressedSize64 > maxUploadArchiveEntryBytes {
			return fmt.Errorf("archive entry too large: %s", entry.Name)
		}
		totalUncompressed += entry.UncompressedSize64
		if totalUncompressed > maxUploadArchiveTotalBytes {
			return errors.New("archive uncompressed size exceeds limit")
		}
		if entry.CompressedSize64 > 0 && entry.UncompressedSize64/entry.CompressedSize64 > maxUploadArchiveRatio {
			return fmt.Errorf("archive compression ratio too high: %s", entry.Name)
		}
		if strings.Contains(entry.Name, "..") || strings.HasPrefix(entry.Name, "/") || strings.HasPrefix(entry.Name, "\\") {
			return fmt.Errorf("archive entry has unsafe path: %s", entry.Name)
		}
	}
	return nil
}

func isMacroUploadEntry(name string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(name), "\\", "/"))
	base := pathBase(normalized)
	if base == "vbaproject.bin" || base == "vbadata.xml" {
		return true
	}
	return strings.Contains(normalized, "/activex/") ||
		strings.Contains(normalized, "/macrosheets/") ||
		strings.Contains(normalized, "/vba/")
}

func pathBase(value string) string {
	if value == "" {
		return ""
	}
	index := strings.LastIndex(value, "/")
	if index < 0 {
		return value
	}
	return value[index+1:]
}

func scanUploadForThreats(file *multipart.FileHeader) error {
	addr := strings.TrimSpace(os.Getenv("CLAMAV_ADDR"))
	if addr == "" {
		if envIsTrue("UPLOAD_REQUIRE_ANTIVIRUS") {
			return errors.New("antivirus scanner is required but CLAMAV_ADDR is empty")
		}
		return nil
	}
	return scanUploadWithClamAV(file, addr, uploadAntivirusTimeout())
}

func scanUploadWithClamAV(file *multipart.FileHeader, addr string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("connect antivirus scanner: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return fmt.Errorf("set antivirus scanner deadline: %w", err)
	}
	if _, err := conn.Write([]byte("zINSTREAM\x00")); err != nil {
		return fmt.Errorf("start antivirus scan: %w", err)
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	buffer := make([]byte, 32*1024)
	for {
		n, readErr := src.Read(buffer)
		if n > 0 {
			var size [4]byte
			binary.BigEndian.PutUint32(size[:], uint32(n))
			if _, err := conn.Write(size[:]); err != nil {
				return fmt.Errorf("send antivirus scan chunk size: %w", err)
			}
			if _, err := conn.Write(buffer[:n]); err != nil {
				return fmt.Errorf("send antivirus scan chunk: %w", err)
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return fmt.Errorf("read upload for antivirus scan: %w", readErr)
		}
	}

	var zero [4]byte
	if _, err := conn.Write(zero[:]); err != nil {
		return fmt.Errorf("finish antivirus scan: %w", err)
	}

	responseBytes, err := readClamAVResponse(conn)
	if err != nil {
		return fmt.Errorf("read antivirus scan response: %w", err)
	}
	response := strings.TrimSpace(strings.TrimRight(string(responseBytes), "\x00"))
	if strings.Contains(response, "FOUND") {
		return fmt.Errorf("malware detected: %s", response)
	}
	if !strings.Contains(response, "OK") {
		return fmt.Errorf("unexpected antivirus scan response: %s", response)
	}
	return nil
}

func readClamAVResponse(conn net.Conn) ([]byte, error) {
	response := make([]byte, 0, 128)
	buffer := make([]byte, 1)
	for len(response) < 4096 {
		n, err := conn.Read(buffer)
		if n > 0 {
			if buffer[0] == 0 || buffer[0] == '\n' {
				break
			}
			response = append(response, buffer[0])
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
	}
	return response, nil
}

func uploadAntivirusTimeout() time.Duration {
	seconds := 10
	if raw := strings.TrimSpace(os.Getenv("CLAMAV_TIMEOUT_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 120 {
			seconds = parsed
		}
	}
	return time.Duration(seconds) * time.Second
}

func envIsTrue(name string) bool {
	raw := strings.TrimSpace(os.Getenv(name))
	return raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes")
}

func isZipUploadContent(header []byte) bool {
	return bytes.HasPrefix(header, []byte("PK\x03\x04")) ||
		bytes.HasPrefix(header, []byte("PK\x05\x06")) ||
		bytes.HasPrefix(header, []byte("PK\x07\x08"))
}

func isTextUploadContent(header []byte) bool {
	if bytes.Contains(header, []byte{0}) {
		return false
	}
	trimmed := bytes.TrimPrefix(header, []byte{0xef, 0xbb, 0xbf})
	return utf8.Valid(trimmed)
}

// 分页响应结构
type PagedResponse struct {
	Items interface{} `json:"items"`
	Total int64       `json:"total"`
}

func applyDingTalkProfileFields(profile *database.EmployeeProfile, user dingtalk.UserInfo, status string) {
	profile.WorkEmail = user.Email
	profile.ProfileStatus = status
	if user.HiredDate != "" {
		profile.EntryDate = user.HiredDate
	}
	if user.PlannedRegularDate != "" {
		profile.PlannedRegularDate = user.PlannedRegularDate
	}
	if user.ProbationEndDate != "" {
		profile.ProbationEndDate = user.ProbationEndDate
	}
	if user.ActualRegularDate != "" {
		profile.ActualRegularDate = user.ActualRegularDate
	}
	if user.EmploymentType != "" {
		profile.EmploymentType = user.EmploymentType
		profile.EmploymentTypeCode = user.EmploymentTypeCode
	}
	if user.JobLevel != "" {
		profile.JobLevel = user.JobLevel
	}
	if user.JobFamily != "" {
		profile.JobFamily = user.JobFamily
	}
}

// HealthCheck 健康检查

func resolveOrgScope(c *gin.Context) (*service.OrgDataScope, error) {
	return middleware.UserDataScope(c)
}

func respondOrgAccessDenied(c *gin.Context) {
	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: "当前账号无权访问该组织数据",
	})
}

// employeeServiceForRequest 绑定 JWT org 的员工服务；缺 org 时 abort 并返回 nil。
// 禁止客户端传 org_id；所有员工档案/入转调离查询必须经此入口。
func employeeServiceForRequest(c *gin.Context) (*service.EmployeeService, string, bool) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return nil, "", false
	}
	if !rejectCrossOrgParam(c, orgID, c.Query("org_id"), c.Query("target_org_id")) {
		return nil, "", false
	}
	return service.NewEmployeeServiceWithOrgID(middleware.RequestDB(c), orgID), orgID, true
}

// employeeServiceForOrg 在已知 org（同步/登录建档）场景下构造组织绑定服务。
func employeeServiceForOrg(c *gin.Context, orgID string) *service.EmployeeService {
	return service.NewEmployeeServiceWithOrgID(middleware.RequestDB(c), database.NormalizeOrganizationID(orgID))
}

func currentUserHasAnyPermission(c *gin.Context, permissionCodes ...string) bool {
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID == "" || len(permissionCodes) == 0 {
		return false
	}
	ok, err := middleware.HasAnyPermission(c, permissionCodes...)
	return err == nil && ok
}

func resolveScopeAndApplyFilters(c *gin.Context, filters map[string]string) (*service.OrgDataScope, bool) {
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "鑾峰彇缁勭粐鑼冨洿澶辫触",
			Data:    gin.H{"error": err.Error()},
		})
		return nil, false
	}
	applyOrgScopeToFilters(scope, filters)
	return scope, true
}

func applyOrgScopeToFilters(scope *service.OrgDataScope, filters map[string]string) {
	if scope == nil || scope.IsAll() {
		return
	}
	if scope.IsSelf() {
		if len(scope.UserIDs) > 0 {
			filters["user_id"] = scope.UserIDs[0]
		} else {
			filters["user_id"] = "__scope_no_user__"
		}
		delete(filters, "department_id")
		delete(filters, "department_ids")
		return
	}

	if len(scope.DepartmentIDs) == 0 {
		filters["department_id"] = "__scope_no_department__"
		return
	}
	if requestedDepartmentID := strings.TrimSpace(filters["department_id"]); requestedDepartmentID != "" {
		if !scope.AllowsDepartment(requestedDepartmentID) {
			filters["department_id"] = "__scope_no_department__"
		}
		return
	}
	filters["department_ids"] = strings.Join(scope.DepartmentIDs, ",")
}

func csvFilterValues(value string) []string {
	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func canAccessUserByScope(scope *service.OrgDataScope, user *database.User) bool {
	if scope == nil || scope.IsAll() {
		return true
	}
	if scope.IsSelf() {
		return scope.AllowsUser(user)
	}
	return scope.AllowsDepartment(user.DepartmentID)
}

// loadUserByUserID loads a user by business user_id within the given organization.
// orgID is required (fail-closed); empty org never falls back to a global or default query.
func loadUserByUserID(orgID, userID string) (*database.User, error) {
	return loadUserByAuthIDInOrg(orgID, userID)
}

// loadUserByAuthID is intentionally unavailable without an organization context.
// Callers must use loadUserByAuthIDInOrg with the JWT-bound org_id.
func loadUserByAuthID(authUserID string) (*database.User, error) {
	_ = authUserID
	return nil, ErrMissingOrgContext
}

// loadUserByAuthIDInOrg loads a user by user_id (or numeric primary key) scoped to orgID.
// Missing orgID is fail-closed: no global lookup and no default-organization fallback.
func loadUserByAuthIDInOrg(orgID, authUserID string) (*database.User, error) {
	authUserID = strings.TrimSpace(authUserID)
	if authUserID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgContext
	}
	// Do not use NormalizeOrganizationID here: empty must fail, not become "default".

	var user database.User
	query := database.DB.Where("org_id = ? AND user_id = ? AND deleted_at IS NULL", orgID, authUserID)
	tx := query.Limit(1).Find(&user)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected > 0 {
		return &user, nil
	}

	if !isNumericString(authUserID) {
		return nil, gorm.ErrRecordNotFound
	}
	user = database.User{}
	query = database.DB.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, authUserID)
	tx = query.Limit(1).Find(&user)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

func isNumericString(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func ensureCanAccessAttendanceUser(c *gin.Context, userID string) (*database.User, bool) {
	// Attendance data access must resolve the target employee inside the JWT organization only.
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return nil, false
	}

	user, err := loadUserByUserID(orgID, userID)
	if err != nil {
		if errors.Is(err, ErrMissingOrgContext) {
			respondMissingOrgContext(c)
			return nil, false
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// Not found in current org is treated as access denied to avoid cross-org probing.
			respondOrgAccessDenied(c)
			return nil, false
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取员工信息失败",
			Data:    gin.H{"error": err.Error()},
		})
		return nil, false
	}

	if currentUserHasAnyPermission(c, "attendance_manage") {
		return user, true
	}

	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return nil, false
	}
	if !canAccessUserByScope(scope, user) {
		respondOrgAccessDenied(c)
		return nil, false
	}
	return user, true
}

func dingtalkDepartmentsToOrgSyncItems(orgID string, depts []dingtalk.DeptInfo) []service.OrgDepartmentSyncItem {
	orgID = database.NormalizeOrganizationID(orgID)
	items := make([]service.OrgDepartmentSyncItem, 0, len(depts))
	for _, d := range depts {
		rawDepartmentID := fmt.Sprintf("%d", d.DeptID)
		rawParentID := fmt.Sprintf("%d", d.ParentID)
		headUserIDs := make([]string, 0, len(d.DeptManagerUserIDs))
		for _, headUserID := range d.DeptManagerUserIDs {
			if scoped := scopedDingTalkID(orgID, headUserID); scoped != "" {
				headUserIDs = append(headUserIDs, scoped)
			}
		}
		items = append(items, service.OrgDepartmentSyncItem{
			OrgID:                orgID,
			DepartmentID:         scopedDingTalkID(orgID, rawDepartmentID),
			DingTalkDepartmentID: rawDepartmentID,
			Name:                 d.Name,
			ParentID:             scopedDingTalkID(orgID, rawParentID),
			DingTalkParentID:     rawParentID,
			HeadUserIDs:          headUserIDs,
			Extension:            d.Extension,
		})
	}
	return items
}

func shouldOverwriteEmptyDingTalkOrgFields(c *gin.Context) bool {
	raw := strings.ToLower(strings.TrimSpace(firstNonEmptyQuery(c, "overwrite_empty", "full_overwrite", "overwrite_empty_manager")))
	return raw == "1" || raw == "true" || raw == "yes"
}

func firstNonEmptyQuery(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(c.Query(key)); value != "" {
			return value
		}
	}
	return ""
}

func explicitOrgIDFromRequest(c *gin.Context) string {
	if c == nil {
		return ""
	}
	return firstNonEmptyQuery(c, "org_id", "org")
}

func dingTalkOrgIDFromRequest(c *gin.Context, fallback string) (string, error) {
	if value := explicitOrgIDFromRequest(c); value != "" {
		return database.NormalizeOrganizationID(value), nil
	}
	return defaultDingTalkOrgID(fallback)
}

func dingTalkOrgIDFromOptionalValue(value string) (string, error) {
	if strings.TrimSpace(value) != "" {
		return database.NormalizeOrganizationID(value), nil
	}
	return defaultDingTalkOrgID(database.DefaultOrganizationID)
}

func defaultDingTalkOrgID(fallback string) (string, error) {
	multiple, err := hasMultipleActiveOrganizations()
	if err != nil {
		return "", err
	}
	if multiple {
		return "", errDingTalkOrgSelectionRequired
	}
	return database.NormalizeOrganizationID(fallback), nil
}

func hasMultipleActiveOrganizations() (bool, error) {
	if database.DB == nil {
		return false, nil
	}
	var count int64
	err := database.DB.Model(&database.Organization{}).
		Where("status = ? AND deleted_at IS NULL", "active").
		Count(&count).Error
	return count > 1, err
}

func respondDingTalkOrgSelectionError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "resolve dingtalk organization failed"
	if errors.Is(err, errDingTalkOrgSelectionRequired) {
		status = http.StatusBadRequest
		message = "missing dingtalk org_id"
	}
	c.JSON(status, Response{
		Code:    status,
		Message: message,
		Data:    gin.H{"error": err.Error()},
	})
}

func uniqueNormalizedOrgIDs(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized := database.NormalizeOrganizationID(value)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func findSingleActiveOrganizationID(orgIDs []string) (string, error) {
	orgIDs = uniqueNormalizedOrgIDs(orgIDs...)
	if len(orgIDs) == 0 {
		return "", gorm.ErrRecordNotFound
	}
	if database.DB == nil {
		if len(orgIDs) == 1 {
			return orgIDs[0], nil
		}
		return "", errDingTalkLoginOrgResolutionConflict
	}

	var rows []struct {
		OrgID string
	}
	if err := database.DB.Model(&database.Organization{}).
		Select("org_id").
		Where("org_id IN ? AND status = ? AND deleted_at IS NULL", orgIDs, "active").
		Scan(&rows).Error; err != nil {
		return "", err
	}

	activeOrgIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		activeOrgIDs = append(activeOrgIDs, row.OrgID)
	}
	activeOrgIDs = uniqueNormalizedOrgIDs(activeOrgIDs...)
	switch len(activeOrgIDs) {
	case 0:
		return "", gorm.ErrRecordNotFound
	case 1:
		return activeOrgIDs[0], nil
	default:
		return "", errDingTalkLoginOrgResolutionConflict
	}
}

func findActiveOrganizationIDByCorpID(corpID string) (string, error) {
	corpID = strings.TrimSpace(corpID)
	if corpID == "" {
		return "", gorm.ErrRecordNotFound
	}
	if database.DB == nil {
		return "", gorm.ErrRecordNotFound
	}

	var rows []struct {
		OrgID string
	}
	if err := database.DB.Model(&database.Organization{}).
		Select("org_id").
		Where("corp_id = ? AND status = ? AND deleted_at IS NULL", corpID, "active").
		Scan(&rows).Error; err != nil {
		return "", err
	}

	orgIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		orgIDs = append(orgIDs, row.OrgID)
	}
	return findSingleActiveOrganizationID(orgIDs)
}

func findActiveOrganizationIDByBinding(unionID, openID string) (string, error) {
	unionID = strings.TrimSpace(unionID)
	openID = strings.TrimSpace(openID)
	if unionID == "" && openID == "" {
		return "", gorm.ErrRecordNotFound
	}
	if database.DB == nil {
		return "", gorm.ErrRecordNotFound
	}

	query := database.DB.Model(&database.DingTalkBinding{}).Select("org_id")
	switch {
	case unionID != "" && openID != "":
		query = query.Where("(union_id = ? OR open_id = ?)", unionID, openID)
	case unionID != "":
		query = query.Where("union_id = ?", unionID)
	default:
		query = query.Where("open_id = ?", openID)
	}

	var rows []struct {
		OrgID string
	}
	if err := query.Scan(&rows).Error; err != nil {
		return "", err
	}

	orgIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		orgIDs = append(orgIDs, row.OrgID)
	}
	return findSingleActiveOrganizationID(orgIDs)
}

func allowDingTalkUnionFallbackForUnscopedLogin() (bool, error) {
	multiple, err := hasMultipleActiveOrganizations()
	if err != nil {
		return false, err
	}
	return !multiple, nil
}

func findLocalUserByDingTalkBindingInOrg(orgID, unionID, openID string) (*database.User, error) {
	orgID = database.NormalizeOrganizationID(orgID)
	unionID = strings.TrimSpace(unionID)
	openID = strings.TrimSpace(openID)
	if unionID == "" && openID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	if database.DB == nil {
		return nil, gorm.ErrRecordNotFound
	}

	query := database.DB.Model(&database.DingTalkBinding{}).Where("org_id = ?", orgID)
	switch {
	case unionID != "" && openID != "":
		query = query.Where("(union_id = ? OR open_id = ?)", unionID, openID)
	case unionID != "":
		query = query.Where("union_id = ?", unionID)
	default:
		query = query.Where("open_id = ?", openID)
	}

	var binding database.DingTalkBinding
	if err := query.First(&binding).Error; err != nil {
		return nil, err
	}

	userIDs := uniqueNonEmptyDingTalkLoginStrings(binding.UserID, scopedDingTalkID(orgID, binding.DingTalkUserID))
	dingTalkUserIDs := uniqueNonEmptyDingTalkLoginStrings(binding.DingTalkUserID)
	var user database.User
	err := database.DB.
		Where("org_id = ? AND (user_id IN ? OR ding_talk_user_id IN ?) AND deleted_at IS NULL", orgID, userIDs, dingTalkUserIDs).
		First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func rememberDingTalkBindingForLogin(orgID string, user *database.User, dingTalkUserID, unionID, openID string) error {
	if user == nil {
		return nil
	}
	orgID = database.NormalizeOrganizationID(orgID)
	localUserID := strings.TrimSpace(user.UserID)
	dingTalkUserID = firstNonEmptyString(dingTalkUserID, user.DingTalkUserID)
	unionID = strings.TrimSpace(unionID)
	openID = strings.TrimSpace(openID)
	if database.DB == nil || localUserID == "" || dingTalkUserID == "" || (unionID == "" && openID == "") {
		return nil
	}

	query := database.DB.Where("org_id = ? AND (user_id = ? OR ding_talk_user_id = ?)", orgID, localUserID, dingTalkUserID)
	if unionID != "" {
		query = query.Or("org_id = ? AND union_id = ?", orgID, unionID)
	}
	if openID != "" {
		query = query.Or("org_id = ? AND open_id = ?", orgID, openID)
	}

	var binding database.DingTalkBinding
	err := query.First(&binding).Error
	fields := map[string]interface{}{
		"org_id":            orgID,
		"user_id":           localUserID,
		"ding_talk_user_id": dingTalkUserID,
	}
	if unionID != "" {
		fields["union_id"] = unionID
	}
	if openID != "" {
		fields["open_id"] = openID
	}

	if err == nil {
		return database.DB.Model(&binding).Updates(fields).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return database.DB.Model(&database.DingTalkBinding{}).Create(fields).Error
}

func uniqueNonEmptyDingTalkLoginStrings(values ...string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return []string{"__empty__"}
	}
	return result
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func findActiveOrganizationIDByDingTalkUserID(candidates ...string) (string, error) {
	cleaned := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			cleaned = append(cleaned, candidate)
		}
	}
	if len(cleaned) == 0 {
		return "", gorm.ErrRecordNotFound
	}
	if database.DB == nil {
		return "", gorm.ErrRecordNotFound
	}

	var rows []struct {
		OrgID string
	}
	if err := database.DB.Model(&database.User{}).
		Select("DISTINCT org_id").
		Where("ding_talk_user_id IN ? AND deleted_at IS NULL", cleaned).
		Scan(&rows).Error; err != nil {
		return "", err
	}

	orgIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		orgIDs = append(orgIDs, row.OrgID)
	}
	return findSingleActiveOrganizationID(orgIDs)
}

func resolveActiveOrganizationIDByUnionID(unionID string) (string, error) {
	unionID = strings.TrimSpace(unionID)
	if unionID == "" {
		return "", gorm.ErrRecordNotFound
	}

	configs, err := dingtalk.ListActiveOrganizationConfigs()
	if err != nil {
		return "", err
	}

	matchedOrgIDs := make([]string, 0, len(configs))
	for _, cfg := range configs {
		if _, resolveErr := dingtalk.GetUserIDByUnionIDForOrg(cfg.OrgID, unionID); resolveErr == nil {
			matchedOrgIDs = append(matchedOrgIDs, cfg.OrgID)
		}
	}

	return findSingleActiveOrganizationID(matchedOrgIDs)
}

func resolveDingTalkCallbackOrgID(userInfo map[string]interface{}) (string, error) {
	associatedUserID := getStringByKeys(userInfo, "associated_user_id", "associatedUserId", "userid", "userId")
	if orgID, err := findActiveOrganizationIDByDingTalkUserID(associatedUserID); err == nil {
		return orgID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	if corpID := getStringByKeys(userInfo, "corpId", "corpID", "corp_id", "corpid"); corpID != "" {
		if orgID, err := findActiveOrganizationIDByCorpID(corpID); err == nil {
			return orgID, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}

	unionID := getStringByKeys(userInfo, "unionId", "unionid", "union_id")
	openID := getStringByKeys(userInfo, "openId", "openid", "open_id")

	allowStoredBindingFallback, err := allowDingTalkUnionFallbackForUnscopedLogin()
	if err != nil {
		return "", err
	}
	if allowStoredBindingFallback {
		if unionID != "" {
			if orgID, err := resolveActiveOrganizationIDByUnionID(unionID); err == nil {
				return orgID, nil
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return "", err
			}
		}

		if orgID, err := findActiveOrganizationIDByBinding(unionID, openID); err == nil {
			return orgID, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}

	return "", errDingTalkLoginOrgResolutionFailed
}

func dingTalkUserInfoKeys(data map[string]interface{}) []string {
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func scopedDingTalkID(orgID, id string) string {
	return database.ScopedExternalID(orgID, id)
}

func newLocalUserFromDingTalk(orgID string, u dingtalk.UserInfo, deptID, status string) *database.User {
	orgID = database.NormalizeOrganizationID(orgID)
	user := &database.User{
		OrgID:          orgID,
		UserID:         scopedDingTalkID(orgID, u.UserID),
		DingTalkUserID: strings.TrimSpace(u.UserID),
		Name:           u.Name,
		Email:          u.Email,
		Mobile:         normalizeDingTalkMobile(u.Mobile),
		DepartmentID:   scopedDingTalkID(orgID, deptID),
		Position:       u.Position,
		Avatar:         u.Avatar,
		Status:         status,
		ManagerUserID:  scopedDingTalkID(orgID, u.ManagerUserID),
		ManagerName:    strings.TrimSpace(u.ManagerName),
	}
	applyDingTalkOrgDiagnostics(user, u)
	return user
}

func ensureLocalUserForDingTalkLogin(c *gin.Context, orgID string, u dingtalk.UserInfo, deptID, source string) (*database.User, error) {
	// Fail-closed: never NormalizeOrganizationID("") → "default" for first-login provision.
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	orgID = database.NormalizeOrganizationID(orgID)
	userService := service.NewUserServiceWithOrgID(middleware.RequestDB(c), orgID)
	user, err := findLocalUserByDingTalkIdentityInOrg(orgID, userService, u.UserID)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	status := "active"
	if !u.Active {
		status = "inactive"
	}
	newUser := newLocalUserFromDingTalk(orgID, u, deptID, status)
	if err := userService.CreateUser(newUser); err != nil {
		return nil, err
	}

	permissionService := service.NewPermissionServiceWithOrgID(middleware.RequestDB(c), orgID)
	if _, err := assignDefaultEmployeeRoleForSyncedUser(permissionService, newUser.UserID, source); err != nil {
		return nil, err
	}

	employeeService := employeeServiceForOrg(c, orgID)
	profile := &database.EmployeeProfile{
		OrgID:      orgID,
		UserID:     newUser.UserID,
		EmployeeID: newUser.UserID,
	}
	applyDingTalkProfileFields(profile, u, status)
	if err := employeeService.CreateProfile(profile); err != nil {
		return nil, err
	}

	log.Printf("[%s] auto-provisioned local user for org_id=%s dingtalk_user_id=%s", source, orgID, strings.TrimSpace(u.UserID))
	return newUser, nil
}

func applyDingTalkOrgUser(existing *database.User, orgID string, u dingtalk.UserInfo, deptID, status string, overwriteEmpty bool) {
	orgID = database.NormalizeOrganizationID(orgID)
	existing.OrgID = orgID
	existing.DingTalkUserID = strings.TrimSpace(u.UserID)
	existing.Name = u.Name
	existing.Email = u.Email
	if mobile := normalizeDingTalkMobile(u.Mobile); mobile != "" {
		existing.Mobile = mobile
	}
	existing.DepartmentID = scopedDingTalkID(orgID, deptID)
	if strings.TrimSpace(u.Position) != "" || overwriteEmpty {
		existing.Position = strings.TrimSpace(u.Position)
	}
	existing.Avatar = u.Avatar
	existing.Status = status
	if strings.TrimSpace(u.ManagerUserID) != "" || overwriteEmpty {
		existing.ManagerUserID = scopedDingTalkID(orgID, u.ManagerUserID)
		existing.ManagerName = strings.TrimSpace(u.ManagerName)
	}
	applyDingTalkOrgDiagnostics(existing, u)
}

func normalizeDingTalkMobile(value string) string {
	value = strings.TrimSpace(value)
	if value == "10000000000" {
		return ""
	}
	return value
}

func applyDingTalkOrgDiagnostics(user *database.User, u dingtalk.UserInfo) {
	if user.Extension == nil {
		user.Extension = map[string]interface{}{}
	}
	if u.PositionSyncDiagnostic != nil {
		user.Extension["dingtalk_position_sync"] = u.PositionSyncDiagnostic
	}
	user.Extension["dingtalk_org_user_sync"] = map[string]interface{}{
		"user_api":                    "topapi/v2/user/list",
		"hrm_api":                     "topapi/smartwork/hrm/employee/v2/list",
		"position":                    strings.TrimSpace(u.Position),
		"position_source":             strings.TrimSpace(u.PositionSource),
		"direct_manager_user_id":      strings.TrimSpace(u.ManagerUserID),
		"direct_manager_name":         strings.TrimSpace(u.ManagerName),
		"direct_manager_source":       strings.TrimSpace(u.ManagerSource),
		"direct_manager_missing_note": "When DingTalk has no direct manager value, local users.manager_user_id/users.manager_name are preserved unless full overwrite is requested.",
		"synced_at":                   time.Now().Format(time.RFC3339),
	}
}

func assignDefaultEmployeeRoleForSyncedUser(permissionService *service.PermissionService, userID, source string) (bool, error) {
	assigned, err := permissionService.AssignDefaultEmployeeRoleIfUnassigned(userID)
	if err != nil {
		log.Printf("[%s] 为新增用户 %s 分配普通员工角色失败: %v", source, userID, err)
		return false, err
	}
	if assigned {
		log.Printf("[%s] 已为新增用户 %s 分配普通员工角色", source, userID)
	}
	return assigned, nil
}

func createOperationAuditLog(c *gin.Context, operation, resource string, details map[string]interface{}) {
	userID := fmt.Sprint(c.GetString("userID"))
	if userID == "" {
		if value, ok := c.Get("userID"); ok {
			userID = fmt.Sprint(value)
		}
	}

	userName := strings.TrimSpace(c.GetString("userName"))
	if userName == "" {
		if value, ok := c.Get("userName"); ok {
			userName = fmt.Sprint(value)
		}
	}
	if user, err := loadUserByAuthIDInOrg(c.GetString("orgID"), userID); err == nil {
		userID = user.UserID
		if strings.TrimSpace(userName) == "" {
			userName = user.Name
		}
	}
	if userID == "" {
		userID = "system"
	}
	if userName == "" {
		userName = "system"
	}

	orgID := strings.TrimSpace(c.GetString("orgID"))
	auditService := service.NewAuditService(middleware.RequestDB(c))
	_ = auditService.CreateLog(&database.OperationLog{
		OrgID:     orgID,
		UserID:    userID,
		UserName:  userName,
		Operation: operation,
		Resource:  resource,
		IP:        c.ClientIP(),
		Details:   details,
	})
}

func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"status": "ok"},
	})
}

const (
	defaultAuthTokenTTLMinutes = 8 * 60
	minAuthTokenTTLMinutes     = 5
	maxAuthTokenTTLMinutes     = 24 * 60
	authCookiePath             = "/"
)

func generateSessionToken(c *gin.Context, user *database.User) (string, time.Time, error) {
	sessionID, err := generateRandomHex(16)
	if err != nil {
		return "", time.Time{}, err
	}
	tokenString, expiresAt, err := signAuthToken(user, sessionID)
	if err != nil {
		return "", time.Time{}, err
	}
	session := database.UserSession{
		OrgID:     database.NormalizeOrganizationID(user.OrgID),
		UserID:    user.UserID,
		SessionID: sessionID,
		Token:     hashToken(tokenString),
		ExpiresAt: expiresAt,
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
	}
	if err := database.DB.Create(&session).Error; err != nil {
		return "", time.Time{}, err
	}
	return tokenString, expiresAt, nil
}

func signAuthToken(user *database.User, sessionID string) (string, time.Time, error) {
	if user == nil {
		return "", time.Time{}, errors.New("missing user")
	}
	userID := strings.TrimSpace(user.UserID)
	if userID == "" {
		userID = strconv.FormatUint(uint64(user.ID), 10)
	}
	now := time.Now()
	expiresAt := now.Add(authTokenTTL())
	claims := &middleware.Claims{
		OrgID:          database.NormalizeOrganizationID(user.OrgID),
		UserID:         userID,
		UserDBID:       strconv.FormatUint(uint64(user.ID), 10),
		UserName:       user.Name,
		SessionID:      sessionID,
		SessionVersion: middleware.SessionVersion(),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	secret, err := middleware.JWTSecret()
	if err != nil {
		return "", time.Time{}, err
	}
	tokenString, err := token.SignedString(secret)
	return tokenString, expiresAt, err
}

func authTokenTTL() time.Duration {
	minutes := defaultAuthTokenTTLMinutes
	if raw := strings.TrimSpace(os.Getenv("JWT_TTL_MINUTES")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			minutes = parsed
		}
	}
	if minutes < minAuthTokenTTLMinutes {
		minutes = minAuthTokenTTLMinutes
	}
	if minutes > maxAuthTokenTTLMinutes {
		minutes = maxAuthTokenTTLMinutes
	}
	return time.Duration(minutes) * time.Minute
}

func generateRandomHex(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("crypto random unavailable: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func respondAuthSuccess(c *gin.Context, user *database.User, token string, expiresAt time.Time) {
	if _, err := setAuthCookies(c, token, expiresAt); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "设置登录状态失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"user":       buildAuthUserPayload(user),
			"expires_at": expiresAt,
			"auth_mode":  "cookie",
		},
	})
}

func setAuthCookies(c *gin.Context, token string, expiresAt time.Time) (string, error) {
	csrfToken, err := generateRandomHex(16)
	if err != nil {
		return "", err
	}

	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 0 {
		maxAge = 0
	}
	secure := authCookieSecure(c)
	sameSite := authCookieSameSite()

	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.AuthCookieName,
		Value:    token,
		Path:     authCookiePath,
		MaxAge:   maxAge,
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   secure,
		SameSite: sameSite,
	})
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     middleware.CSRFCookieName,
		Value:    csrfToken,
		Path:     authCookiePath,
		MaxAge:   maxAge,
		Expires:  expiresAt,
		HttpOnly: false,
		Secure:   secure,
		SameSite: sameSite,
	})

	return csrfToken, nil
}

func clearAuthCookies(c *gin.Context) {
	expiredAt := time.Unix(0, 0)
	secure := authCookieSecure(c)
	sameSite := authCookieSameSite()
	for _, name := range []string{middleware.AuthCookieName, middleware.CSRFCookieName} {
		http.SetCookie(c.Writer, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     authCookiePath,
			MaxAge:   -1,
			Expires:  expiredAt,
			HttpOnly: name == middleware.AuthCookieName,
			Secure:   secure,
			SameSite: sameSite,
		})
	}
}

func authCookieSecure(c *gin.Context) bool {
	if raw := strings.TrimSpace(os.Getenv("AUTH_COOKIE_SECURE")); raw != "" {
		return raw == "1" || strings.EqualFold(raw, "true") || strings.EqualFold(raw, "yes")
	}
	if c != nil && c.Request != nil {
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			return true
		}
	}
	return false
}

func authCookieSameSite() http.SameSite {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_COOKIE_SAMESITE"))) {
	case "strict":
		return http.SameSiteStrictMode
	case "none":
		return http.SameSiteNoneMode
	default:
		return http.SameSiteLaxMode
	}
}

func isActiveUser(user *database.User) bool {
	return user != nil && strings.EqualFold(strings.TrimSpace(user.Status), "active")
}

func rejectInactiveLogin(c *gin.Context, user *database.User, loginType string) {
	userID, userName := "", ""
	if user != nil {
		userID = user.UserID
		userName = user.Name
	}
	database.DB.Create(&database.LoginLog{
		OrgID: database.NormalizeOrganizationID(func() string {
			if user != nil {
				return user.OrgID
			}
			return c.GetString("orgID")
		}()),
		UserID:      userID,
		UserName:    userName,
		LoginType:   loginType,
		LoginStatus: "failed",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		ErrorMsg:    "inactive user",
	})
	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: "账号已停用，请联系管理员",
	})
}

// revokeActiveSessionsForUser revokes active sessions for one user inside one organization.
// Missing orgID is fail-closed: never revoke sessions across organizations by user_id alone.
func revokeActiveSessionsForUser(orgID, userID, reason string) {
	orgID = strings.TrimSpace(orgID)
	userID = strings.TrimSpace(userID)
	if orgID == "" || userID == "" {
		log.Printf("[security] skip revoke sessions: missing org_id or user_id org_id=%q user_id=%q reason=%s", orgID, userID, reason)
		return
	}
	now := time.Now()
	tx := database.DB.Model(&database.UserSession{}).
		Where("org_id = ? AND user_id = ? AND revoked_at IS NULL", orgID, userID).
		Update("revoked_at", &now)
	if tx.Error != nil {
		log.Printf("[security] revoke sessions for org_id=%s user_id=%s reason=%s failed: %v", orgID, userID, reason, tx.Error)
	}
}

// buildUserMenuKeys 聚合用户的菜单权限 key 列表
func permissionServiceForOrg(orgID string) *service.PermissionService {
	if database.DB == nil {
		return service.NewPermissionService(nil)
	}
	info := &requestmeta.RequestInfo{OrgID: database.NormalizeOrganizationID(orgID)}
	ctx := requestmeta.WithRequestInfo(context.Background(), info)
	return service.NewPermissionService(database.DB.WithContext(ctx))
}

func buildUserMenuKeys(user *database.User) []string {
	permService := permissionServiceForOrg(user.OrgID)
	keys, err := permService.GetUserMenuKeys(user.UserID)
	if err != nil {
		return []string{}
	}
	return keys
}

func buildUserPermissions(user *database.User) []string {
	permService := permissionServiceForOrg(user.OrgID)
	permissions, err := permService.GetUserPermissions(user.UserID)
	if err != nil {
		return []string{}
	}
	return permissions
}

func buildAuthUserPayload(user *database.User) gin.H {
	return gin.H{
		"id":               user.ID,
		"org_id":           database.NormalizeOrganizationID(user.OrgID),
		"user_id":          user.UserID,
		"dingtalk_user_id": user.DingTalkUserID,
		"name":             user.Name,
		"email":            user.Email,
		"mobile":           user.Mobile,
		"department_id":    user.DepartmentID,
		"position":         user.Position,
		"avatar":           user.Avatar,
		"status":           user.Status,
		"menu_keys":        buildUserMenuKeys(user),
		"permissions":      buildUserPermissions(user),
	}
}

// Login 登录
func Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		OrgID    string `json:"org_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "用户名、密码和组织都不能为空",
		})
		return
	}

	orgID := strings.TrimSpace(req.OrgID)
	if orgID == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请选择要登录的组织",
		})
		return
	}
	if _, err := database.GetOrganizationByOrgID(orgID); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "组织不存在或未激活",
		})
		return
	}

	userService := service.NewUserServiceWithOrgID(database.DB, orgID)
	user, err := userService.GetUserByOrgAndUserID(orgID, strings.TrimSpace(req.Username))
	if err != nil {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "用户名或密码错误",
		})
		return
	}

	// 校验密码
	if !database.CheckPassword(req.Password, user.Password) {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "用户名或密码错误",
		})
		return
	}

	// 生成 JWT token
	if !isActiveUser(user) {
		rejectInactiveLogin(c, user, "local")
		return
	}

	tokenString, expiresAt, err := generateSessionToken(c, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "生成令牌失败",
		})
		return
	}

	// 写入 LoginLog
	database.DB.Create(&database.LoginLog{
		OrgID:       database.NormalizeOrganizationID(user.OrgID),
		UserID:      user.UserID,
		UserName:    user.Name,
		LoginType:   "local",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	respondAuthSuccess(c, user, tokenString, expiresAt)
}

// GetUsers 获取用户列表
func GetUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	// 选人组件可能请求较大分页；与仓储上限对齐，防止误用超大值。
	if pageSize > 1000 {
		pageSize = 1000
	}
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}

	if !currentUserHasAnyPermission(c, "user_manage", "permission_manage") {
		scope, err := resolveOrgScope(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "鑾峰彇缁勭粐鑼冨洿澶辫触",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
		users, total, err := orgService.ListEmployees(scope, page, pageSize, service.OrgEmployeeFilters{
			DepartmentID: c.Query("department_id"),
			Search:       c.Query("search"),
			Status:       c.Query("status"),
		})
		if err != nil {
			if errors.Is(err, service.ErrOrgAccessDenied) {
				respondOrgAccessDenied(c)
				return
			}
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "鑾峰彇鐢ㄦ埛鍒楄〃澶辫触",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data:    PagedResponse{Items: users, Total: total},
		})
		return
	}

	userService := service.NewUserServiceWithOrgID(middleware.RequestDB(c), orgID)
	users, total, err := userService.SearchUsers(page, pageSize, c.Query("search"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取用户列表失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    PagedResponse{Items: users, Total: total},
	})
}

// GetUser 获取用户详情
func GetUser(c *gin.Context) {
	id := c.Param("id")

	userService := service.NewUserService(middleware.RequestDB(c))
	user, err := userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "用户不存在",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}
	if !currentUserHasAnyPermission(c, "user_manage", "permission_manage") {
		scope, err := resolveOrgScope(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "鑾峰彇缁勭粐鑼冨洿澶辫触",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		if !canAccessUserByScope(scope, user) {
			respondOrgAccessDenied(c)
			return
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"user": user},
	})
}

// UpdateUser 更新用户信息
func UpdateUser(c *gin.Context) {
	id := c.Param("id")

	var updateData struct {
		Extension     *map[string]interface{} `json:"extension"`
		ManagerUserID *string                 `json:"manager_user_id"`
		ManagerName   *string                 `json:"manager_name"`
	}

	if err := c.ShouldBindJSON(&updateData); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	userService := service.NewUserService(middleware.RequestDB(c))
	user, err := userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "用户不存在",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	if updateData.Extension != nil {
		user.Extension = *updateData.Extension
	}
	if updateData.ManagerUserID != nil {
		managerUserID := strings.TrimSpace(*updateData.ManagerUserID)
		if managerUserID == "" {
			user.ManagerUserID = ""
			user.ManagerName = ""
		} else {
			if managerUserID == user.UserID {
				c.JSON(http.StatusBadRequest, Response{
					Code:    http.StatusBadRequest,
					Message: "直属主管不能设置为员工本人",
				})
				return
			}
			manager, managerErr := userService.GetUserByUserID(managerUserID)
			if managerErr != nil || strings.TrimSpace(manager.Status) != "active" {
				c.JSON(http.StatusBadRequest, Response{
					Code:    http.StatusBadRequest,
					Message: "直属主管不存在或不是在职状态",
				})
				return
			}
			user.ManagerUserID = manager.UserID
			if updateData.ManagerName != nil && strings.TrimSpace(*updateData.ManagerName) != "" {
				user.ManagerName = strings.TrimSpace(*updateData.ManagerName)
			} else {
				user.ManagerName = manager.Name
			}
		}
	}
	if err := userService.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新用户失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"user": user},
	})
}

// GetDepartments 获取部门列表
func GetDepartments(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	departmentService := service.NewDepartmentServiceWithOrgID(middleware.RequestDB(c), orgID)
	departments, err := departmentService.GetAllDepartments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门列表失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"departments": departments},
	})
}

// GetDepartment 获取部门详情
func GetDepartment(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")

	departmentService := service.NewDepartmentServiceWithOrgID(middleware.RequestDB(c), orgID)
	department, err := departmentService.GetDepartmentByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "部门不存在",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"department": department},
	})
}

type orgSyncUserDependencies struct {
	FindUser                     func(string) (*database.User, error)
	FindUserByDingTalkID         func(string) (*database.User, error)
	FindDepartmentByDingTalkID   func(string) (*database.Department, error)
	CreateUser                   func(*database.User) error
	UpdateUser                   func(*database.User) error
	AssignDefaultRole            func(string) (bool, error)
	FindProfile                  func(string) (*database.EmployeeProfile, error)
	CreateProfile                func(*database.EmployeeProfile) error
	UpdateProfile                func(*database.EmployeeProfile) error
	ReplaceDepartmentMemberships func(string, []string) error
	DeactivateMissingUsers       func([]string) ([]string, error)
	RevokeSessions               func(string)
}

func orgSyncUserDependenciesForRequest(c *gin.Context, orgID string) orgSyncUserDependencies {
	userService := service.NewUserServiceWithOrgID(middleware.RequestDB(c), orgID)
	employeeService := employeeServiceForOrg(c, orgID)
	permissionService := service.NewPermissionServiceWithOrgID(middleware.RequestDB(c), orgID)
	departmentService := service.NewDepartmentServiceWithOrgID(middleware.RequestDB(c), orgID)
	return orgSyncUserDependencies{
		FindUser:                     userService.GetUserByUserID,
		FindUserByDingTalkID:         userService.GetUserByDingTalkUserID,
		FindDepartmentByDingTalkID:   departmentService.GetDepartmentByDingTalkDepartmentID,
		CreateUser:                   userService.CreateUser,
		UpdateUser:                   userService.UpdateUser,
		AssignDefaultRole:            permissionService.AssignDefaultEmployeeRoleIfUnassigned,
		FindProfile:                  employeeService.GetProfileByUserID,
		CreateProfile:                employeeService.CreateProfile,
		UpdateProfile:                employeeService.UpdateProfile,
		ReplaceDepartmentMemberships: userService.ReplaceDepartmentMemberships,
		DeactivateMissingUsers:       userService.DeactivateUsersMissingFromDingTalk,
		RevokeSessions: func(userID string) {
			revokeActiveSessionsForUser(orgID, userID, "sync_org_inactive")
		},
	}
}

func dedupeOrgSyncUsers(users []dingtalk.UserInfo) []dingtalk.UserInfo {
	result := make([]dingtalk.UserInfo, 0, len(users))
	indexes := make(map[string]int, len(users))
	for _, user := range users {
		key := strings.TrimSpace(user.UserID)
		if key == "" {
			result = append(result, user)
			continue
		}
		if index, ok := indexes[key]; ok {
			result[index] = user
			continue
		}
		indexes[key] = len(result)
		result = append(result, user)
	}
	return result
}

func syncDingTalkUsers(ctx context.Context, orgID string, users []dingtalk.UserInfo, overwriteEmpty bool, deps orgSyncUserDependencies, requestID, maskedOrgID string, startTime time.Time) orgSyncEmployeeStageResult {
	result := orgSyncEmployeeStageResult{Status: "success", OverwriteEmpty: overwriteEmpty, HRMFieldStatus: "success"}
	dedupedUsers := dedupeOrgSyncUsers(users)
	for itemIndex, user := range dedupedUsers {
		if err := ctx.Err(); err != nil {
			result.Status = "failed"
			if result.SuccessCount > 0 {
				result.Status = "partial_failed"
			}
			result.Error = employeeSyncCanceledMessage
			result.ErrorCode = employeeSyncCanceledErrorCode
			logOrgSyncError(requestID, maskedOrgID, "employees", "request_canceled", err, startTime, result.SuccessCount, result.FailCount)
			break
		}
		itemFailed := false
		userID := strings.TrimSpace(user.UserID)
		if strings.TrimSpace(user.Position) == "" {
			result.PositionMissingCount++
		}
		if user.Active {
			if strings.TrimSpace(user.EmploymentType) == "" {
				result.EmploymentTypeMissingCount++
			}
			if strings.TrimSpace(user.JobLevel) == "" {
				result.JobLevelMissingCount++
			}
			if strings.TrimSpace(user.JobFamily) == "" {
				result.JobFamilyMissingCount++
			}
			if strings.TrimSpace(user.ActualRegularDate) == "" && strings.TrimSpace(user.PlannedRegularDate) == "" && strings.TrimSpace(user.ProbationEndDate) == "" {
				result.RegularizationDateMissingCount++
			}
		}
		if strings.EqualFold(strings.TrimSpace(user.HRMFieldSyncStatus), dingtalk.HRMFieldSyncStatusFailed) {
			result.HRMFieldStatus = "failed"
		}
		if strings.EqualFold(strings.TrimSpace(user.HRMFieldSyncStatus), dingtalk.HRMFieldSyncStatusNoFields) && result.HRMFieldStatus != "failed" {
			result.HRMFieldStatus = "success_no_fields"
		}
		if userID == "" {
			itemFailed = true
			logOrgSyncError(requestID, maskedOrgID, "employees", "validation", errors.New("missing dingtalk user id"), startTime, result.SuccessCount, result.FailCount+1)
			result.FailCount++
			continue
		}

		deptID := ""
		localDepartmentIDs := make([]string, 0, len(user.DeptIDList))
		seenDepartmentIDs := make(map[string]struct{}, len(user.DeptIDList))
		for _, rawDepartmentID := range user.DeptIDList {
			externalDepartmentID := fmt.Sprintf("%d", rawDepartmentID)
			if _, exists := seenDepartmentIDs[externalDepartmentID]; exists {
				continue
			}
			seenDepartmentIDs[externalDepartmentID] = struct{}{}
			localDepartmentID := ""
			if deps.FindDepartmentByDingTalkID != nil {
				department, err := deps.FindDepartmentByDingTalkID(externalDepartmentID)
				if err != nil {
					if errors.Is(err, gorm.ErrRecordNotFound) {
						continue
					}
					itemFailed = true
					logOrgSyncError(requestID, maskedOrgID, "employees", "department_lookup", err, startTime, result.SuccessCount, result.FailCount+1)
					continue
				}
				if department == nil || strings.TrimSpace(department.DepartmentID) == "" {
					continue
				}
				if departmentOrgID := strings.TrimSpace(department.OrgID); departmentOrgID != "" && departmentOrgID != orgID {
					itemFailed = true
					logOrgSyncError(requestID, maskedOrgID, "employees", "department_org_mismatch", errors.New("department belongs to another organization"), startTime, result.SuccessCount, result.FailCount+1)
					continue
				}
				localDepartmentID = strings.TrimSpace(department.DepartmentID)
			} else {
				// 测试缝隙或显式无仓储调用方仍保持 scoped ID；生产依赖始终提供租户部门查询。
				localDepartmentID = scopedDingTalkID(orgID, externalDepartmentID)
			}
			if deptID == "" {
				deptID = externalDepartmentID
			}
			localDepartmentIDs = append(localDepartmentIDs, localDepartmentID)
		}
		if len(user.DeptIDList) > 0 && len(localDepartmentIDs) == 0 {
			logOrgSyncError(requestID, maskedOrgID, "employees", "department_validation", errors.New("no valid departments"), startTime, result.SuccessCount, result.FailCount+1)
			result.FailCount++
			continue
		}
		status := "active"
		if !user.Active {
			status = "inactive"
		}
		localDepartmentID := scopedDingTalkID(orgID, deptID)
		if len(localDepartmentIDs) > 0 {
			localDepartmentID = localDepartmentIDs[0]
		}
		localManagerUserID := scopedDingTalkID(orgID, user.ManagerUserID)
		if deps.FindUserByDingTalkID != nil && strings.TrimSpace(user.ManagerUserID) != "" {
			if manager, err := deps.FindUserByDingTalkID(user.ManagerUserID); err == nil && manager != nil && strings.TrimSpace(manager.UserID) != "" {
				localManagerUserID = manager.UserID
			} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				itemFailed = true
				logOrgSyncError(requestID, maskedOrgID, "employees", "manager_lookup", err, startTime, result.SuccessCount, result.FailCount+1)
			}
		}

		localUserID := scopedDingTalkID(orgID, userID)
		existing, findErr := deps.FindUser(localUserID)
		if errors.Is(findErr, gorm.ErrRecordNotFound) && deps.FindUserByDingTalkID != nil {
			if stableUser, stableErr := deps.FindUserByDingTalkID(userID); stableErr == nil && stableUser != nil {
				existing = stableUser
				localUserID = stableUser.UserID
				findErr = nil
			} else if stableErr != nil && !errors.Is(stableErr, gorm.ErrRecordNotFound) {
				findErr = stableErr
			}
		}
		created := false
		userPersisted := false
		if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
			itemFailed = true
			logOrgSyncError(requestID, maskedOrgID, "employees", "user_lookup", findErr, startTime, result.SuccessCount, result.FailCount+1)
		} else if errors.Is(findErr, gorm.ErrRecordNotFound) {
			newUser := newLocalUserFromDingTalk(orgID, user, deptID, status)
			newUser.DepartmentID = localDepartmentID
			newUser.ManagerUserID = localManagerUserID
			if err := deps.CreateUser(newUser); err != nil {
				itemFailed = true
				logOrgSyncError(requestID, maskedOrgID, "employees", "user_create", err, startTime, result.SuccessCount, result.FailCount+1)
			} else {
				created = true
				userPersisted = true
				assigned, err := deps.AssignDefaultRole(localUserID)
				if err != nil {
					itemFailed = true
					logOrgSyncError(requestID, maskedOrgID, "employees", "role_assign", err, startTime, result.SuccessCount, result.FailCount+1)
				} else if assigned {
					result.DefaultRoleAssignedCount++
				}
			}
		} else {
			applyDingTalkOrgUser(existing, orgID, user, deptID, status, overwriteEmpty)
			existing.DepartmentID = localDepartmentID
			existing.ManagerUserID = localManagerUserID
			if err := deps.UpdateUser(existing); err != nil {
				itemFailed = true
				logOrgSyncError(requestID, maskedOrgID, "employees", "user_update", err, startTime, result.SuccessCount, result.FailCount+1)
			} else if !isActiveUser(existing) && deps.RevokeSessions != nil {
				userPersisted = true
				deps.RevokeSessions(existing.UserID)
			} else {
				userPersisted = true
			}
		}
		if userPersisted && deps.ReplaceDepartmentMemberships != nil {
			if err := deps.ReplaceDepartmentMemberships(localUserID, localDepartmentIDs); err != nil {
				itemFailed = true
				logOrgSyncError(requestID, maskedOrgID, "employees", "department_memberships", err, startTime, result.SuccessCount, result.FailCount+1)
			}
		}

		// 用户主数据写入失败时无法安全写档案；角色失败后仍继续档案写入用于数据修复。
		if !itemFailed || created {
			profile, profileErr := deps.FindProfile(localUserID)
			if created || errors.Is(profileErr, gorm.ErrRecordNotFound) {
				profile = &database.EmployeeProfile{OrgID: orgID, UserID: localUserID, EmployeeID: localUserID}
				applyDingTalkProfileFields(profile, user, status)
				if err := deps.CreateProfile(profile); err != nil {
					itemFailed = true
					logOrgSyncError(requestID, maskedOrgID, "employees", "profile_create", err, startTime, result.SuccessCount, result.FailCount+1)
				}
			} else if profileErr != nil {
				itemFailed = true
				logOrgSyncError(requestID, maskedOrgID, "employees", "profile_lookup", profileErr, startTime, result.SuccessCount, result.FailCount+1)
			} else {
				profile.OrgID = orgID
				applyDingTalkProfileFields(profile, user, status)
				if err := deps.UpdateProfile(profile); err != nil {
					itemFailed = true
					logOrgSyncError(requestID, maskedOrgID, "employees", "profile_update", err, startTime, result.SuccessCount, result.FailCount+1)
				}
			}
		}

		if itemFailed {
			result.FailCount++
			log.Printf("[org-sync] request_id=%s org=%s stage=employees result=item_failed item_index=%d success_count=%d fail_count=%d",
				sanitizeSyncLogText(requestID), maskedOrgID, itemIndex, result.SuccessCount, result.FailCount)
		} else {
			result.SuccessCount++
		}
	}
	deactivationFailed := false
	if ctx.Err() == nil && deps.DeactivateMissingUsers != nil && len(dedupedUsers) > 0 {
		sourceUserIDs := make([]string, 0, len(dedupedUsers))
		for _, user := range dedupedUsers {
			if userID := strings.TrimSpace(user.UserID); userID != "" {
				sourceUserIDs = append(sourceUserIDs, userID)
			}
		}
		deactivatedUserIDs, err := deps.DeactivateMissingUsers(sourceUserIDs)
		if err != nil {
			deactivationFailed = true
			result.DeactivationStatus = "failed"
			result.DeactivationError = employeeDeactivationFailedMessage
			logOrgSyncError(requestID, maskedOrgID, "employees", "deactivate_missing_users", err, startTime, result.SuccessCount, result.FailCount)
		} else {
			result.DeactivationStatus = "success"
			result.DeactivatedMissingCount = len(deactivatedUserIDs)
			if deps.RevokeSessions != nil {
				for _, userID := range deactivatedUserIDs {
					deps.RevokeSessions(userID)
				}
			}
		}
	}
	if result.FailCount > 0 {
		result.Status = "failed"
		if result.SuccessCount > 0 {
			result.Status = "partial_failed"
		}
		result.Error = employeeSyncFailedMessage
		result.ErrorCode = employeeSyncFailedErrorCode
	}
	if deactivationFailed {
		result.Status = "partial_failed"
		result.Error = employeeDeactivationFailedMessage
		result.ErrorCode = employeeDeactivationFailedErrorCode
	}
	if result.HRMFieldStatus == "failed" {
		result.HRMFieldError = employeeHRMSyncFailedMessage
		if result.Status == "success" {
			result.Status = "partial_failed"
			result.Error = employeeHRMSyncFailedMessage
			result.ErrorCode = dingtalk.ErrorCodePermissionDenied
		}
	}
	if result.HRMFieldStatus == "success_no_fields" {
		result.HRMFieldError = employeeHRMNoFieldsMessage
	}
	return result
}

func validateOrgSyncRequestSelectors(c *gin.Context, orgID string) bool {
	if !rejectCrossOrgParam(c, orgID,
		c.Query("org_id"), c.Query("target_org_id"),
		c.GetHeader("X-Org-ID"), c.GetHeader("X-Organization-ID"),
	) {
		return false
	}
	var input struct {
		OrgID       *string `json:"org_id"`
		TargetOrgID *string `json:"target_org_id"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "请求格式错误"})
			return false
		}
		if !rejectClientOrganizationID(c, input.OrgID) || !rejectClientOrganizationID(c, input.TargetOrgID) {
			return false
		}
	}
	return true
}

func orgSyncStageHTTPStatus(status string) int {
	switch status {
	case "success":
		return http.StatusOK
	case "partial_failed":
		return http.StatusMultiStatus
	default:
		return http.StatusInternalServerError
	}
}

func respondOrgSyncStage(c *gin.Context, syncType string, result orgSyncEmployeeStageResult, requestID string, duration time.Duration, extra gin.H) {
	data := gin.H{
		"status": result.Status, "success_count": result.SuccessCount, "fail_count": result.FailCount,
		"error": result.Error, "error_code": result.ErrorCode, "request_id": requestID, "duration_ms": duration.Milliseconds(),
	}
	for key, value := range extra {
		data[key] = value
	}
	messageText := "success"
	switch result.Status {
	case "partial_failed":
		messageText = "同步部分完成，请前往同步日志查看失败项"
	case "failed":
		messageText = employeeSyncFailedMessage
		if syncType == "departments" {
			messageText = departmentSyncFailedMessage
		}
	}
	statusCode := orgSyncStageHTTPStatus(result.Status)
	c.JSON(statusCode, Response{Code: statusCode, Message: messageText, Data: data})
}

// SyncUsers 同步用户
func SyncUsers(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok || !validateOrgSyncRequestSelectors(c, orgID) {
		return
	}
	startTime := time.Now()
	requestID := orgSyncRequestID(c, startTime)
	maskedOrgID := redactOrgIDForSyncLog(orgID)
	unlock, locked := tryAcquireOrgSync(orgID)
	if !locked {
		respondOrgSyncConflict(c, requestID, maskedOrgID, startTime)
		return
	}
	defer unlock()

	users, err := syncUsersForOrg(orgID)
	if err != nil {
		logOrgSyncError(requestID, maskedOrgID, "employees", "source_fetch", err, startTime, 0, 1)
		errorCode, safeMessage := orgSyncErrorDetails(err, employeeSyncFailedErrorCode, employeeSyncFailedMessage)
		result := orgSyncEmployeeStageResult{Status: "failed", FailCount: 1, Error: safeMessage, ErrorCode: errorCode}
		_ = writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{SyncType: "users", Status: "failed", Message: safeMessage, RequestID: requestID, DurationMS: time.Since(startTime).Milliseconds(), ErrorCode: errorCode, FailCount: 1})
		respondOrgSyncStage(c, "users", result, requestID, time.Since(startTime), nil)
		return
	}

	result := syncDingTalkUsers(c.Request.Context(), orgID, users, shouldOverwriteEmptyDingTalkOrgFields(c), orgSyncUserDependenciesForRequest(c, orgID), requestID, maskedOrgID, startTime)
	statusMessage := fmt.Sprintf("同步 %d 个用户", result.SuccessCount)
	switch result.Status {
	case "partial_failed":
		if result.FailCount == 0 && result.HRMFieldStatus == "failed" {
			statusMessage = employeeHRMSyncFailedMessage
		} else {
			statusMessage = fmt.Sprintf("同步部分完成：成功 %d，失败 %d", result.SuccessCount, result.FailCount)
		}
	case "failed":
		statusMessage = employeeSyncFailedMessage
	}
	_ = writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{SyncType: "users", Status: result.Status, Message: statusMessage, RequestID: requestID, DurationMS: time.Since(startTime).Milliseconds(), ErrorCode: result.ErrorCode, SuccessCount: result.SuccessCount, FailCount: result.FailCount})
	respondOrgSyncStage(c, "users", result, requestID, time.Since(startTime), gin.H{
		"position_missing_count": result.PositionMissingCount, "overwrite_empty": result.OverwriteEmpty,
		"default_role_assigned_count":       result.DefaultRoleAssignedCount,
		"employment_type_missing_count":     result.EmploymentTypeMissingCount,
		"job_level_missing_count":           result.JobLevelMissingCount,
		"job_family_missing_count":          result.JobFamilyMissingCount,
		"regularization_date_missing_count": result.RegularizationDateMissingCount,
		"hrm_field_status":                  result.HRMFieldStatus,
		"hrm_field_error":                   result.HRMFieldError,
		"deactivated_missing_count":         result.DeactivatedMissingCount,
		"deactivation_status":               result.DeactivationStatus,
		"deactivation_error":                result.DeactivationError,
	})
}

// SyncDepartments 同步部门
func SyncDepartments(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok || !validateOrgSyncRequestSelectors(c, orgID) {
		return
	}
	startTime := time.Now()
	requestID := orgSyncRequestID(c, startTime)
	maskedOrgID := redactOrgIDForSyncLog(orgID)
	unlock, locked := tryAcquireOrgSync(orgID)
	if !locked {
		respondOrgSyncConflict(c, requestID, maskedOrgID, startTime)
		return
	}
	defer unlock()

	result := orgSyncEmployeeStageResult{Status: "success"}
	changeLogCount := 0
	depts, err := syncDepartmentsForOrg(orgID)
	if err == nil {
		err = validateOrgSyncDepartments(depts)
	}
	if err == nil {
		persisted, persistErr := persistOrgSyncDepartments(c, orgID, depts)
		if persistErr == nil {
			result.SuccessCount = persisted.Count
			changeLogCount = persisted.ChangeLogCount
		} else {
			err = &orgSyncCodedError{
				Code:        departmentPersistErrorCode,
				SafeMessage: departmentPersistFailedMessage,
				Cause:       persistErr,
			}
		}
	}
	if err != nil {
		result.Status = "failed"
		result.FailCount = max(len(depts), 1)
		result.ErrorCode, result.Error = orgSyncErrorDetails(err, dingtalk.ErrorCodeResponseInvalid, departmentSyncFailedMessage)
		logOrgSyncError(requestID, maskedOrgID, "departments", "sync", err, startTime, 0, result.FailCount)
		_ = writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{SyncType: "departments", Status: "failed", Message: result.Error, RequestID: requestID, DurationMS: time.Since(startTime).Milliseconds(), ErrorCode: result.ErrorCode, FailCount: result.FailCount})
	} else {
		_ = writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{SyncType: "departments", Status: "success", Message: fmt.Sprintf("同步 %d 个部门", result.SuccessCount), RequestID: requestID, DurationMS: time.Since(startTime).Milliseconds(), SuccessCount: result.SuccessCount})
	}
	respondOrgSyncStage(c, "departments", result, requestID, time.Since(startTime), gin.H{"change_log_count": changeLogCount})
}

// GetDingTalkConfig 返回钉钉前端配置（corpId 等），供 JS-SDK 初始化
func GetDingTalkConfig(c *gin.Context) {
	orgID, err := dingTalkOrgIDFromRequest(c, database.DefaultOrganizationID)
	if err != nil {
		respondDingTalkOrgSelectionError(c, err)
		return
	}
	cfg, _ := dingtalk.ConfigForOrgID(orgID)
	cfg = cfg.NormalizedForAPI()
	corpID := cfg.CorpID
	missingConfig := []string{}
	if corpID == "" {
		missingConfig = append(missingConfig, "DINGTALK_CORP_ID")
	}
	log.Printf("[dingtalk/config] org_id=%s host=%s missing=%v", orgID, c.Request.Host, missingConfig)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"corp_id": corpID,
			"org_id":  orgID,
			"missing": missingConfig,
		},
	})
}

// ListActiveOrganizations 返回所有活跃企业列表（供登录页选择企业，无需登录）
func ListActiveOrganizations(c *gin.Context) {
	if database.DB == nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "database not initialized",
		})
		return
	}

	var orgs []database.Organization
	if err := database.DB.
		Select("org_id, name, corp_id").
		Where("status = ? AND deleted_at IS NULL", "active").
		Order("name ASC").
		Find(&orgs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "query organizations failed: " + err.Error(),
		})
		return
	}

	// 脱敏：不返回 corp_id 等敏感字段，仅返回前端展示需要的
	type orgItem struct {
		OrgID string `json:"org_id"`
		Name  string `json:"name"`
	}
	items := make([]orgItem, 0, len(orgs))
	for _, org := range orgs {
		items = append(items, orgItem{OrgID: org.OrgID, Name: org.Name})
	}
	defaultQROrgID := strings.TrimSpace(os.Getenv("DINGTALK_QR_DEFAULT_ORG_ID"))
	if defaultQROrgID != "" {
		defaultQROrgID = database.NormalizeOrganizationID(defaultQROrgID)
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"organizations":     items,
			"default_qr_org_id": defaultQROrgID,
		},
	})
}

// DingTalkQRLoginStart 閽夐拤鎵爜鐧诲綍寮€濮?
func DingTalkQRLoginStart(c *gin.Context) {
	requestedOrgID := strings.TrimSpace(explicitOrgIDFromRequest(c))
	hasMultipleOrganizations, err := hasMultipleActiveOrganizations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "resolve dingtalk qr login organization scope failed",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	oauthOrgID := resolveDingTalkQROAuthOrgID(requestedOrgID, hasMultipleOrganizations)
	oauthConfig, err := dingtalk.ResolveOAuthLoginConfig(oauthOrgID)
	if err != nil {
		status := http.StatusInternalServerError
		message := "resolve dingtalk qr login config failed"
		if strings.Contains(err.Error(), "shared dingtalk oauth login config is required") ||
			strings.Contains(err.Error(), "shared dingtalk oauth org") {
			status = http.StatusBadRequest
			message = "shared dingtalk oauth login config is required for direct qr login"
		}
		c.JSON(status, Response{
			Code:    status,
			Message: message,
			Data:    gin.H{"error": err.Error()},
		})
		return
	}
	stateOrgID := resolveDingTalkQRStateOrgID(requestedOrgID, hasMultipleOrganizations)
	state := generateLoginStateWithOAuthOrgID(stateOrgID, oauthConfig.OrgID)
	redirectURI := resolveDingTalkRedirectURI(c)
	log.Printf("[dingtalk/qr/start] requested_org_id=%s state_org_id=%s oauth_org_id=%s multiple_orgs=%t redirect_uri=%s host=%s forwarded_host=%s ua=%s", requestedOrgID, stateOrgID, oauthConfig.OrgID, hasMultipleOrganizations, redirectURI, c.Request.Host, c.GetHeader("X-Forwarded-Host"), c.GetHeader("User-Agent"))

	qrCodeURL, err := dingtalk.GetQRCodeWithRedirectForConfig(oauthConfig, state, redirectURI)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get qrcode failed",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"qr_code_url": qrCodeURL,
		},
	})
}

// DingTalkInAppLogin 閽夐拤鍐呭厤鐧?
func DingTalkInAppLogin(c *gin.Context) {
	var req struct {
		Code  string `json:"code" binding:"required"`
		OrgID string `json:"org_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "invalid request",
		})
		return
	}
	orgID, err := dingTalkOrgIDFromOptionalValue(req.OrgID)
	if err != nil {
		respondDingTalkOrgSelectionError(c, err)
		return
	}
	log.Printf("[dingtalk/in-app] org_id=%s host=%s has_code=%t ua=%s", orgID, c.Request.Host, strings.TrimSpace(req.Code) != "", c.GetHeader("User-Agent"))

	userid, err := dingtalk.GetUserIDByInAppCodeForOrg(orgID, req.Code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "dingtalk in-app login failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/in-app] resolved_userid=%s", userid)

	userDetail, err := dingtalk.GetUserDetailByUserIDForOrg(orgID, userid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get dingtalk user detail failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/in-app] user_detail=%v", userDetail)

	name, _ := userDetail["name"].(string)
	email, _ := userDetail["email"].(string)
	mobile, _ := userDetail["mobile"].(string)
	avatar, _ := userDetail["avatar"].(string)
	position, _ := userDetail["title"].(string)
	deptID := "1"
	if deptList, ok := userDetail["dept_id_list"].([]interface{}); ok && len(deptList) > 0 {
		if id, ok := deptList[0].(float64); ok {
			deptID = fmt.Sprintf("%d", int64(id))
		}
	}

	userService := service.NewUserServiceWithOrgID(middleware.RequestDB(c), orgID)
	user, err := findLocalUserByDingTalkIdentityInOrg(orgID, userService, userid)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user, err = ensureLocalUserForDingTalkLogin(c, orgID, dingtalk.UserInfo{
				UserID:   userid,
				Name:     name,
				Email:    email,
				Mobile:   mobile,
				Avatar:   avatar,
				Position: position,
				Active:   true,
			}, deptID, "DingTalkInAppLogin")
			if err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Code:    http.StatusInternalServerError,
					Message: "auto provision local user failed: " + err.Error(),
				})
				return
			}
		} else {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "query local user failed: " + err.Error(),
			})
			return
		}
	}

	if !isActiveUser(user) {
		rejectInactiveLogin(c, user, "dingtalk_in_app")
		return
	}

	user.Name = name
	user.Avatar = avatar
	user.Position = position
	user.OrgID = orgID
	user.DingTalkUserID = strings.TrimSpace(userid)
	user.DepartmentID = scopedDingTalkID(orgID, deptID)
	if err := assignUserEmailSafelyInOrg(orgID, userService, user, email); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user email failed: " + err.Error(),
		})
		return
	}
	if err := assignUserMobileSafelyInOrg(orgID, userService, user, mobile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user mobile failed: " + err.Error(),
		})
		return
	}
	if err := userService.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user failed: " + err.Error(),
		})
		return
	}

	tokenString, expiresAt, err := generateSessionToken(c, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "generate token failed",
		})
		return
	}

	database.DB.Create(&database.LoginLog{
		OrgID:       database.NormalizeOrganizationID(user.OrgID),
		UserID:      user.UserID,
		UserName:    user.Name,
		LoginType:   "dingtalk_in_app",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	respondAuthSuccess(c, user, tokenString, expiresAt)
}

// DingTalkCallback 閽夐拤鍥炶皟
func DingTalkCallback(c *gin.Context) {
	code := c.Query("authCode")
	if code == "" {
		code = c.Query("code")
	}
	state := c.Query("state")
	log.Printf("[dingtalk/callback] host=%s has_code=%t has_state=%t ua=%s", c.Request.Host, code != "", state != "", c.GetHeader("User-Agent"))

	// 校验 state（防 CSRF）
	// State is validated by the legacy line above; keep this block focused on the result.
	stateEntry, validState := validateLoginStateEntry(state)
	if !validState {
		log.Printf("[dingtalk/callback] invalid or expired state")
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效或过期的登录状态，请重新扫码",
		})
		return
	}

	// 清理过期 state
	orgID := strings.TrimSpace(stateEntry.OrgID)
	oauthOrgID := strings.TrimSpace(stateEntry.OAuthOrgID)
	requestedOrgID := explicitOrgIDFromRequest(c)
	resolvedRequestedOrgID, err := resolveRequestedDingTalkCallbackOrgID(orgID, requestedOrgID)
	if err != nil {
		log.Printf("[dingtalk/callback] org mismatch state_org_id=%s request_org_id=%s", orgID, requestedOrgID)
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "dingtalk org mismatch, please restart login",
		})
		return
	}
	orgID = resolvedRequestedOrgID

	cleanupOldStates()

	if code == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "missing auth code",
		})
		return
	}

	oauthConfigOrgID := oauthOrgID
	if oauthConfigOrgID == "" {
		oauthConfigOrgID = orgID
	}
	oauthConfig, err := dingtalk.ResolveOAuthLoginConfig(oauthConfigOrgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "resolve dingtalk login config failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/callback] state_org_id=%s request_org_id=%s oauth_org_id=%s oauth_config_org_id=%s", orgID, requestedOrgID, oauthOrgID, oauthConfig.OrgID)

	userInfo, err := dingtalk.GetUserInfoByCodeWithConfig(oauthConfig, code)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get dingtalk user info failed: " + err.Error(),
		})
		return
	}
	associatedUserID := getStringByKeys(userInfo, "associated_user_id", "associatedUserId", "userid", "userId")
	unionID := getStringByKeys(userInfo, "unionId", "unionid", "union_id")
	openID := getStringByKeys(userInfo, "openId", "openid", "open_id")
	corpID := getStringByKeys(userInfo, "corpId", "corpID", "corp_id", "corpid")
	log.Printf("[dingtalk/callback] user_info_received associated_user_id=%t unionid=%t corp_id=%t keys=%v", associatedUserID != "", unionID != "", corpID != "", dingTalkUserInfoKeys(userInfo))
	if err := validateDingTalkSelectedOrgIdentity(orgID, userInfo); err != nil {
		log.Printf("[dingtalk/callback] selected org verification failed state_org_id=%s oauth_org_id=%s corp_id=%s associated_user_id=%t unionid=%t err=%v", orgID, oauthConfig.OrgID, corpID, associatedUserID != "", unionID != "", err)
		message := "登录选择的企业与钉钉组织账号不一致，请返回登录页重新选择一致的企业"
		if errors.Is(err, errDingTalkSelectedOrgUnverified) {
			message = "钉钉未返回可验证的组织身份，无法确认与所选企业一致，请返回登录页重新扫码"
		} else if !errors.Is(err, errDingTalkSelectedOrgMismatch) {
			message = "验证钉钉登录企业失败: " + err.Error()
		}
		c.JSON(http.StatusForbidden, Response{
			Code:    http.StatusForbidden,
			Message: message,
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	resolvedOrgID := orgID
	if resolvedOrgID == "" {
		resolvedOrgID, err = resolveDingTalkCallbackOrgID(userInfo)
		if err != nil {
			status := http.StatusForbidden
			message := "无法确认本次钉钉选择的企业，请从目标企业入口重新发起登录"
			if errors.Is(err, errDingTalkLoginOrgResolutionConflict) {
				status = http.StatusConflict
				message = "dingtalk login identity matches multiple local organizations"
			} else if errors.Is(err, errDingTalkLoginOrgResolutionFailed) && unionID != "" {
				message = "钉钉未返回本次选择的企业信息，无法确认登录企业，请联系管理员调整钉钉授权方案"
			} else if !errors.Is(err, errDingTalkLoginOrgResolutionFailed) {
				status = http.StatusInternalServerError
				message = "resolve dingtalk login organization failed: " + err.Error()
			}
			c.JSON(status, Response{
				Code:    status,
				Message: message,
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		log.Printf("[dingtalk/callback] resolved_org_id=%s from identity", resolvedOrgID)
	}

	dtUserID := associatedUserID
	if dtUserID == "" && unionID != "" {
		resolvedUserID, resolveErr := dingtalk.GetUserIDByUnionIDForOrg(resolvedOrgID, unionID)
		if resolveErr == nil {
			dtUserID = resolvedUserID
		} else {
			log.Printf("[dingtalk/callback] resolve unionid failed: org_id=%s union_id=%s err=%v", resolvedOrgID, unionID, resolveErr)
		}
	}

	if dtUserID == "" && openID == "" {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "missing dingtalk user identity",
		})
		return
	}

	var name, email, mobile, avatar, position string
	deptID := "1"
	if dtUserID != "" {
		userDetail, detailErr := dingtalk.GetUserDetailByUserIDForOrg(resolvedOrgID, dtUserID)
		if detailErr == nil {
			name, _ = userDetail["name"].(string)
			email, _ = userDetail["email"].(string)
			mobile, _ = userDetail["mobile"].(string)
			avatar, _ = userDetail["avatar"].(string)
			position, _ = userDetail["title"].(string)
			if deptList, ok := userDetail["dept_id_list"].([]interface{}); ok && len(deptList) > 0 {
				if id, ok := deptList[0].(float64); ok {
					deptID = fmt.Sprintf("%d", int64(id))
				}
			}
		}
	}
	if name == "" {
		name, _ = userInfo["nick"].(string)
	}
	if email == "" {
		email, _ = userInfo["email"].(string)
	}
	if mobile == "" {
		mobile, _ = userInfo["mobile"].(string)
	}
	if avatar == "" {
		avatar, _ = userInfo["avatarUrl"].(string)
	}

	userService := service.NewUserServiceWithOrgID(middleware.RequestDB(c), resolvedOrgID)
	user, err := findLocalUserByDingTalkIdentityInOrg(resolvedOrgID, userService, dtUserID, associatedUserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if boundUser, bindErr := findLocalUserByDingTalkBindingInOrg(resolvedOrgID, unionID, openID); bindErr == nil {
				user = boundUser
				if strings.TrimSpace(dtUserID) == "" {
					dtUserID = strings.TrimSpace(boundUser.DingTalkUserID)
				}
				goto localUserReady
			} else if !errors.Is(bindErr, gorm.ErrRecordNotFound) {
				c.JSON(http.StatusInternalServerError, Response{
					Code:    http.StatusInternalServerError,
					Message: "query dingtalk binding failed: " + bindErr.Error(),
				})
				return
			}
			if strings.TrimSpace(dtUserID) != "" {
				user, err = ensureLocalUserForDingTalkLogin(c, resolvedOrgID, dingtalk.UserInfo{
					UserID:   dtUserID,
					Name:     name,
					Email:    email,
					Mobile:   mobile,
					Avatar:   avatar,
					Position: position,
					Active:   true,
				}, deptID, "DingTalkCallback")
				if err == nil {
					goto localUserReady
				}
				log.Printf("[dingtalk/callback] auto provision failed org_id=%s dingtalk_user_id=%s err=%v", resolvedOrgID, dtUserID, err)
				c.JSON(http.StatusInternalServerError, Response{
					Code:    http.StatusInternalServerError,
					Message: "auto provision local user failed: " + err.Error(),
				})
				return
			}
			if strings.TrimSpace(unionID) != "" {
				c.JSON(http.StatusForbidden, Response{
					Code:    http.StatusForbidden,
					Message: "unable to resolve dingtalk userid from unionId, please grant DingTalk address book permission and retry",
				})
				return
			}
			identityForLog := dtUserID
			if identityForLog == "" {
				identityForLog = openID
			}
			if identityForLog == "" {
				identityForLog = unionID
			}
			respondDingTalkUserNotSynced(c, resolvedOrgID, "dingtalk_qr", identityForLog, name)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "query local user failed: " + err.Error(),
		})
		return
	}

localUserReady:
	if !isActiveUser(user) {
		rejectInactiveLogin(c, user, "dingtalk_qr")
		return
	}

	user.Name = name
	user.Avatar = avatar
	user.Position = position
	user.OrgID = resolvedOrgID
	if strings.TrimSpace(dtUserID) != "" {
		user.DingTalkUserID = strings.TrimSpace(dtUserID)
	}
	user.DepartmentID = scopedDingTalkID(resolvedOrgID, deptID)
	if err := assignUserEmailSafelyInOrg(resolvedOrgID, userService, user, email); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user email failed: " + err.Error(),
		})
		return
	}
	if err := assignUserMobileSafelyInOrg(resolvedOrgID, userService, user, mobile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user mobile failed: " + err.Error(),
		})
		return
	}
	if err := userService.UpdateUser(user); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user failed: " + err.Error(),
		})
		return
	}
	if err := rememberDingTalkBindingForLogin(resolvedOrgID, user, dtUserID, unionID, openID); err != nil {
		log.Printf("[dingtalk/callback] remember binding failed org_id=%s user_id=%s dingtalk_user_id=%s err=%v", resolvedOrgID, user.UserID, dtUserID, err)
	}

	tokenString, expiresAt, err := generateSessionToken(c, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "generate token failed",
		})
		return
	}

	database.DB.Create(&database.LoginLog{
		OrgID:       database.NormalizeOrganizationID(user.OrgID),
		UserID:      user.UserID,
		UserName:    user.Name,
		LoginType:   "dingtalk_qr",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	respondAuthSuccess(c, user, tokenString, expiresAt)
}

func respondDingTalkUserNotSynced(c *gin.Context, orgID, loginType, userID, userName string) {
	database.DB.Create(&database.LoginLog{
		OrgID:       database.NormalizeOrganizationID(orgID),
		UserID:      userID,
		UserName:    userName,
		LoginType:   loginType,
		LoginStatus: "failed",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		ErrorMsg:    "user not synced",
	})

	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Message: "dingtalk user not synced, please sync org data first",
	})
}

func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := firstForwardedHeaderValue(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = forwardedProto
	}

	host := normalizeForwardedHost(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return dingtalk.GetAppHomeURL()
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func firstForwardedHeaderValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return strings.TrimSpace(strings.Split(value, ",")[0])
}

func normalizeForwardedHost(value string) string {
	host := firstForwardedHeaderValue(value)
	if host == "" {
		return ""
	}

	if strings.Contains(host, "://") {
		if parsed, err := url.Parse(host); err == nil && parsed.Host != "" {
			host = parsed.Host
		}
	}

	return strings.TrimSuffix(host, "/")
}

func getStringByKeys(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func findLocalUserByDingTalkIdentity(userService *service.UserService, candidates ...string) (*database.User, error) {
	return findLocalUserByDingTalkIdentityInOrg(database.DefaultOrganizationID, userService, candidates...)
}

func findLocalUserByDingTalkIdentityInOrg(orgID string, userService *service.UserService, candidates ...string) (*database.User, error) {
	orgID = database.NormalizeOrganizationID(orgID)
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		localUserID := scopedDingTalkID(orgID, candidate)
		var user database.User
		err := database.DB.
			Where("org_id = ? AND (user_id = ? OR ding_talk_user_id = ?) AND deleted_at IS NULL", orgID, localUserID, candidate).
			First(&user).Error
		if err == nil {
			return &user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func assignUserEmailSafelyInOrg(orgID string, userService *service.UserService, user *database.User, email string) error {
	orgID = database.NormalizeOrganizationID(orgID)
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}

	var existing database.User
	err := database.DB.Where("org_id = ? AND email = ? AND deleted_at IS NULL", orgID, email).First(&existing).Error
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip email update for user_id=%s because email=%s already belongs to user_id=%s", user.UserID, email, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	user.Email = email
	return nil
}

func assignUserMobileSafelyInOrg(orgID string, userService *service.UserService, user *database.User, mobile string) error {
	orgID = database.NormalizeOrganizationID(orgID)
	mobile = strings.TrimSpace(mobile)
	if mobile == "" {
		return nil
	}

	var existing database.User
	err := database.DB.Where("org_id = ? AND mobile = ? AND deleted_at IS NULL", orgID, mobile).First(&existing).Error
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip mobile update for user_id=%s because mobile=%s already belongs to user_id=%s", user.UserID, mobile, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	user.Mobile = mobile
	return nil
}

func resolveDingTalkAppHomeURL(c *gin.Context) string {
	return resolveDingTalkAppHomeURLForOrg(c, database.DefaultOrganizationID)
}

func resolveDingTalkAppHomeURLForOrg(c *gin.Context, orgID string) string {
	if configured := dingtalk.GetConfiguredAppHomeURLForOrg(orgID); configured != "" {
		return configured
	}
	if configured := dingtalk.GetConfiguredAppHomeURL(); configured != "" {
		return configured
	}
	return requestBaseURL(c)
}

func resolveDingTalkRedirectURI(c *gin.Context) string {
	if configured := dingtalk.GetConfiguredRedirectURI(); configured != "" {
		return configured
	}
	return strings.TrimRight(resolveDingTalkAppHomeURL(c), "/") + "/callback"
}

// Logout 登出
func Logout(c *gin.Context) {
	clearAuthCookies(c)

	if sessionID := strings.TrimSpace(c.GetString("sessionID")); sessionID != "" {
		now := time.Now()
		userID := strings.TrimSpace(c.GetString("userID"))
		orgID := strings.TrimSpace(c.GetString("orgID"))
		// Session revoke must stay inside the authenticated organization boundary.
		if orgID != "" && userID != "" {
			database.DB.Model(&database.UserSession{}).
				Where("org_id = ? AND session_id = ? AND user_id = ? AND revoked_at IS NULL", orgID, sessionID, userID).
				Update("revoked_at", &now)
		} else {
			log.Printf("[security] logout skip session revoke: missing org_id or user_id org_id=%q user_id=%q session_id=%q", orgID, userID, sessionID)
		}
	}

	// 记录登出日志（必须绑定 JWT org_id，禁止无 org 审计）
	userID, _ := c.Get("userID")
	userName, _ := c.Get("userName")
	if uid, ok := userID.(string); ok {
		uname, _ := userName.(string)
		orgID := strings.TrimSpace(c.GetString("orgID"))
		if user, err := loadUserByAuthIDInOrg(orgID, uid); err == nil {
			uid = user.UserID
			if strings.TrimSpace(uname) == "" {
				uname = user.Name
			}
		}
		if orgID != "" {
			middleware.RequestDB(c).Create(&database.OperationLog{
				OrgID:     orgID,
				UserID:    uid,
				UserName:  uname,
				Operation: "登出",
				Resource:  "系统",
				IP:        c.ClientIP(),
			})
		} else {
			log.Printf("[security] logout skip operation log: missing org_id user_id=%q", uid)
		}
	}

	c.JSON(200, Response{
		Code:    200,
		Message: "success",
	})
}

// GetCurrentUser 获取当前用户信息
func GetCurrentUser(c *gin.Context) {
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "未登录",
		})
		return
	}

	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}

	user, err := loadUserByAuthIDInOrg(orgID, userID)
	if err != nil {
		if errors.Is(err, ErrMissingOrgContext) {
			respondMissingOrgContext(c)
			return
		}
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "用户不存在",
		})
		return
	}

	c.JSON(200, Response{
		Code:    200,
		Message: "success",
		Data: gin.H{
			"user": buildAuthUserPayload(user),
		},
	})
}

// GetSyncStatus 获取同步状态
func GetSyncStatus(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	syncService := service.NewSyncService(middleware.RequestDB(c))
	statuses, err := syncService.GetAllSyncStatus(orgID)
	if err != nil {
		// 没有同步记录时返回空状态
		c.JSON(200, Response{
			Code:    200,
			Message: "success",
			Data: gin.H{
				"status": gin.H{
					"departments": gin.H{"last_sync_time": nil, "status": "never"},
					"users":       gin.H{"last_sync_time": nil, "status": "never"},
				},
			},
		})
		return
	}

	result := gin.H{}
	for _, s := range statuses {
		result[s.Type] = gin.H{
			"last_sync_time": s.LastSyncTime,
			"status":         s.Status,
			"message":        s.Message,
			"request_id":     s.RequestID,
			"duration_ms":    s.DurationMS,
			"error_code":     s.ErrorCode,
			"success_count":  s.SuccessCount,
			"fail_count":     s.FailCount,
			"details":        s.Details,
		}
	}
	// 确保 departments 和 users 总存在
	if _, ok := result["departments"]; !ok {
		result["departments"] = gin.H{"last_sync_time": nil, "status": "never"}
	}
	if _, ok := result["users"]; !ok {
		result["users"] = gin.H{"last_sync_time": nil, "status": "never"}
	}

	c.JSON(200, Response{
		Code:    200,
		Message: "success",
		Data:    gin.H{"status": result},
	})
}

func GetOrgOverview(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
	overview, err := orgService.GetOverview(scope, c.Query("department_id"))
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织概览失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"overview": overview},
	})
}

func GetScopedDepartments(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	// Bind org explicitly so department queries never fall back across tenants.
	orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
	departments, err := orgService.GetVisibleDepartments(scope)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门列表失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"departments": departments,
			"scope":       scope,
		},
	})
}

func GetOrgDepartmentTree(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	// ?all=true 跳过 scope 过滤，用于配置数据权限时展示全部部门
	if c.Query("all") == "true" {
		if !currentUserHasAnyPermission(c, "permission_manage", "user_manage") {
			respondOrgAccessDenied(c)
			return
		}
		orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
		tree, err := orgService.GetDepartmentTree(nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "获取部门树失败",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data:    gin.H{"tree": tree, "scope": nil},
		})
		return
	}

	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
	tree, err := orgService.GetDepartmentTree(scope)
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门树失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"tree":  tree,
			"scope": scope,
		},
	})
}

func GetOrgDepartmentHistory(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
	logs, err := orgService.GetDepartmentHistory(scope, c.Param("id"), limit)
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门变更历史失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": logs,
			"total": len(logs),
		},
	})
}

func GetOrgEmployees(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
	users, total, err := orgService.ListEmployees(scope, page, pageSize, service.OrgEmployeeFilters{
		DepartmentID: c.Query("department_id"),
		Search:       c.Query("search"),
		Status:       c.Query("status"),
		FilterType:   c.Query("filter_type"),
	})
	if err != nil {
		if errors.Is(err, service.ErrOrgAccessDenied) {
			respondOrgAccessDenied(c)
			return
		}
		if errors.Is(err, service.ErrInvalidOrgEmployeeFilter) {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "员工筛选类型无效"})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取员工列表失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": users,
			"total": total,
			"scope": scope,
		},
	})
}

func GetOrgEmployeeDetail(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(middleware.RequestDB(c), orgID)
	detail, err := orgService.GetEmployeeAggregate(scope, c.Param("id"))
	if err != nil {
		switch {
		case errors.Is(err, service.ErrOrgAccessDenied):
			respondOrgAccessDenied(c)
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, Response{
				Code:    http.StatusNotFound,
				Message: "员工不存在",
			})
		default:
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "获取员工详情失败",
				Data:    gin.H{"error": err.Error()},
			})
		}
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"detail": detail},
	})
}

func GetOrgEmployeePositionSyncDiagnostic(c *gin.Context) {
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}
	var user database.User
	if err := middleware.RequestDB(c).Where("id = ? AND deleted_at IS NULL", c.Param("id")).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "员工不存在",
		})
		return
	}
	if !canAccessUserByScope(scope, &user) {
		respondOrgAccessDenied(c)
		return
	}
	var diagnostic interface{} = nil
	if user.Extension != nil {
		diagnostic = user.Extension["dingtalk_position_sync"]
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"user_id":    user.UserID,
			"name":       user.Name,
			"position":   user.Position,
			"diagnostic": diagnostic,
		},
	})
}

// GetDepartmentTree 获取部门树
func GetDepartmentTree(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	departmentService := service.NewDepartmentServiceWithOrgID(middleware.RequestDB(c), orgID)
	departments, err := departmentService.GetAllDepartments()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取部门列表失败",
		})
		return
	}

	// 构建树形结构
	type TreeNode struct {
		ID       string      `json:"id"`
		Name     string      `json:"name"`
		ParentID string      `json:"parent_id"`
		Children []*TreeNode `json:"children"`
	}

	nodeMap := make(map[string]*TreeNode)
	var roots []*TreeNode

	for _, dept := range departments {
		node := &TreeNode{
			ID:       dept.DepartmentID,
			Name:     dept.Name,
			ParentID: dept.ParentID,
			Children: []*TreeNode{},
		}
		nodeMap[dept.DepartmentID] = node
	}

	for _, node := range nodeMap {
		if parent, ok := nodeMap[node.ParentID]; ok {
			parent.Children = append(parent.Children, node)
		} else {
			roots = append(roots, node)
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"tree": roots},
	})
}

// GetEmployees 获取员工列表
func GetEmployees(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")

	userService := service.NewUserService(middleware.RequestDB(c))

	var users []database.User
	var total int64
	var err error

	if departmentID != "" {
		users, total, err = userService.GetSyncedEmployeesByDepartment(departmentID, page, pageSize)
	} else {
		users, total, err = userService.GetSyncedEmployees(page, pageSize)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取员工列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": users,
			"total": total,
		},
	})
}

// GetEmployee 获取员工详情
func GetEmployee(c *gin.Context) {
	id := c.Param("id")
	employeeService, _, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	userService := service.NewUserService(middleware.RequestDB(c))
	user, err := userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "员工不存在",
		})
		return
	}

	// 一并返回员工档案（按 user_id 查），避免前端再发请求
	profile, _ := employeeService.GetProfileByUserID(user.UserID)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"employee": user, "profile": profile},
	})
}

// syncOverallResult 汇总阶段执行结果，返回整体状态与对应 HTTP 状态码。
type syncOverallResult struct {
	OverallStatus string // "success" | "partial_failed" | "failed"
	HTTPStatus    int
}

func computeSyncOverallResult(deptStatus, userStatus string) syncOverallResult {
	if deptStatus == "success" && userStatus == "success" {
		return syncOverallResult{OverallStatus: "success", HTTPStatus: http.StatusOK}
	}
	if deptStatus != "partial_failed" && userStatus != "partial_failed" && deptStatus != "success" && userStatus != "success" {
		return syncOverallResult{OverallStatus: "failed", HTTPStatus: http.StatusInternalServerError}
	}
	return syncOverallResult{OverallStatus: "partial_failed", HTTPStatus: http.StatusMultiStatus}
}

type orgSyncStageResult struct {
	Status       string `json:"status"`
	SuccessCount int    `json:"success_count"`
	FailCount    int    `json:"fail_count"`
	Error        string `json:"error"`
	ErrorCode    string `json:"error_code"`
}

type orgSyncEmployeeStageResult struct {
	Status                         string `json:"status"`
	SuccessCount                   int    `json:"success_count"`
	FailCount                      int    `json:"fail_count"`
	Error                          string `json:"error"`
	ErrorCode                      string `json:"error_code"`
	PositionMissingCount           int    `json:"position_missing_count"`
	EmploymentTypeMissingCount     int    `json:"employment_type_missing_count"`
	JobLevelMissingCount           int    `json:"job_level_missing_count"`
	JobFamilyMissingCount          int    `json:"job_family_missing_count"`
	RegularizationDateMissingCount int    `json:"regularization_date_missing_count"`
	HRMFieldStatus                 string `json:"hrm_field_status"`
	HRMFieldError                  string `json:"hrm_field_error"`
	OverwriteEmpty                 bool   `json:"overwrite_empty"`
	DefaultRoleAssignedCount       int    `json:"default_role_assigned_count"`
	DeactivatedMissingCount        int    `json:"deactivated_missing_count"`
	DeactivationStatus             string `json:"deactivation_status"`
	DeactivationError              string `json:"deactivation_error"`
}

type orgSyncResponseData struct {
	OverallStatus string                     `json:"overall_status"`
	Departments   orgSyncStageResult         `json:"departments"`
	Employees     orgSyncEmployeeStageResult `json:"employees"`
	SyncTime      string                     `json:"sync_time"`
	DurationMS    int64                      `json:"duration_ms"`
	RequestID     string                     `json:"request_id"`
}

var runOrgSyncForRequest = performOrgSync

var (
	syncURLTokenPattern = regexp.MustCompile(`(?i)([?&](?:access_token|token|secret|password|app_?key|app_?secret|authorization|cookie)=)[^&\s]+`)
	syncKeyValuePattern = regexp.MustCompile(`(?i)["']?\b(access[_-]?token|token|secret|password|authorization|cookie|app[_-]?key|app[_-]?secret|dsn|database[_-]?url|db[_-]?password)\b["']?(?:\s*[:=]\s*|\s+)("[^"]*"|'[^']*'|[^\s,;]+)`)
	syncBearerPattern   = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;]+`)
	syncDSNPattern      = regexp.MustCompile(`(?i)[^\s:]+:[^@\s]+@(?:tcp\([^)]*\)|[^/\s]+)/[^\s]+`)
	syncSQLPattern      = regexp.MustCompile(`(?is)\b(select|insert|update|delete|replace|alter|drop|create|truncate)\b\s+.*`)
)

func sanitizeSyncLogText(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = syncURLTokenPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = syncKeyValuePattern.ReplaceAllString(value, `${1}=[REDACTED]`)
	value = syncBearerPattern.ReplaceAllString(value, `Bearer [REDACTED]`)
	value = syncDSNPattern.ReplaceAllString(value, `[DSN_REDACTED]`)
	value = syncSQLPattern.ReplaceAllString(value, `[SQL_REDACTED]`)
	if len(value) > 512 {
		value = value[:512] + "..."
	}
	return value
}

func safeSyncErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	summary := sanitizeSyncLogText(err.Error())
	if summary == "" {
		return "error summary unavailable"
	}
	return summary
}

func syncErrorCategory(err error) string {
	if code := dingtalk.SyncErrorCode(err); code != "" {
		switch code {
		case dingtalk.ErrorCodeNetworkFailed:
			return "network"
		case dingtalk.ErrorCodeConfigMissing:
			return "config"
		case dingtalk.ErrorCodePermissionDenied:
			return "permission"
		case dingtalk.ErrorCodeTokenFailed:
			return "token"
		default:
			return "external_response"
		}
	}
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return "network"
	}
	messageText := strings.ToLower(err.Error())
	if strings.Contains(messageText, "sql") || strings.Contains(messageText, "database") || strings.Contains(messageText, "gorm") || strings.Contains(messageText, "dsn") {
		return "database"
	}
	return "internal"
}

func logOrgSyncError(requestID, maskedOrgID, stage, operation string, err error, startTime time.Time, successCount, failCount int) {
	log.Printf("[org-sync] request_id=%s org=%s stage=%s operation=%s result=failed error_category=%s error_summary=%q duration_ms=%d success_count=%d fail_count=%d",
		sanitizeSyncLogText(requestID), sanitizeSyncLogText(maskedOrgID), sanitizeSyncLogText(stage), sanitizeSyncLogText(operation),
		syncErrorCategory(err), safeSyncErrorSummary(err), time.Since(startTime).Milliseconds(), successCount, failCount)
}

func tryAcquireOrgSync(orgID string) (func(), bool) {
	orgMuIface, _ := orgSyncLocks.LoadOrStore(orgID, &sync.Mutex{})
	orgMu := orgMuIface.(*sync.Mutex)
	if !orgMu.TryLock() {
		return nil, false
	}
	return orgMu.Unlock, true
}

func respondOrgSyncConflict(c *gin.Context, requestID, maskedOrgID string, startTime time.Time) {
	log.Printf("[org-sync] request_id=%s org=%s stage=lock result=conflict duration_ms=%d success_count=0 fail_count=1",
		sanitizeSyncLogText(requestID), sanitizeSyncLogText(maskedOrgID), time.Since(startTime).Milliseconds())
	c.JSON(http.StatusConflict, Response{
		Code:    http.StatusConflict,
		Message: "该组织正在同步中，请勿重复提交",
		Data:    gin.H{"request_id": requestID},
	})
}

func validateOrgSyncDepartments(depts []dingtalk.DeptInfo) error {
	if len(depts) == 0 {
		return &orgSyncCodedError{Code: dingtalk.ErrorCodeDepartmentEmpty, SafeMessage: "钉钉返回的部门数据为空", Cause: errors.New("department payload is empty")}
	}
	seen := make(map[int64]struct{}, len(depts))
	rootFound := false
	for _, dept := range depts {
		if dept.DeptID <= 0 || strings.TrimSpace(dept.Name) == "" {
			return &orgSyncCodedError{Code: dingtalk.ErrorCodeResponseInvalid, SafeMessage: "钉钉返回的部门数据格式异常", Cause: errors.New("department payload validation failed")}
		}
		if _, ok := seen[dept.DeptID]; ok {
			return &orgSyncCodedError{Code: dingtalk.ErrorCodeResponseInvalid, SafeMessage: "钉钉返回的部门数据格式异常", Cause: errors.New("duplicate department id")}
		}
		seen[dept.DeptID] = struct{}{}
		if dept.DeptID == 1 {
			rootFound = true
		}
	}
	if !rootFound {
		return &orgSyncCodedError{Code: dingtalk.ErrorCodeResponseInvalid, SafeMessage: "钉钉返回的根部门数据异常", Cause: errors.New("root department is missing")}
	}
	return nil
}

type orgSyncCodedError struct {
	Code        string
	SafeMessage string
	Cause       error
}

func (e *orgSyncCodedError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause == nil {
		return e.Code
	}
	return e.Code + ": " + safeSyncErrorSummary(e.Cause)
}

func orgSyncErrorDetails(err error, fallbackCode, fallbackMessage string) (string, string) {
	if err == nil {
		return "", ""
	}
	var coded *orgSyncCodedError
	if errors.As(err, &coded) {
		return coded.Code, coded.SafeMessage
	}
	if code := dingtalk.SyncErrorCode(err); code != "" {
		messageText := dingtalk.SyncErrorSafeMessage(err)
		if messageText == "" {
			messageText = fallbackMessage
		}
		return code, messageText
	}
	return fallbackCode, fallbackMessage
}

func orgSyncRequestID(c *gin.Context, now time.Time) string {
	if value, ok := c.Get("requestID"); ok {
		if requestID, ok := value.(string); ok && strings.TrimSpace(requestID) != "" {
			if requestID = sanitizeSyncLogText(strings.TrimSpace(requestID)); len(requestID) <= 128 {
				return requestID
			}
		}
	}
	if requestID := strings.TrimSpace(c.GetHeader("X-Request-ID")); requestID != "" && len(requestID) <= 128 {
		return sanitizeSyncLogText(requestID)
	}
	return fmt.Sprintf("org-sync-%d", now.UnixMilli())
}

func redactOrgIDForSyncLog(orgID string) string {
	runes := []rune(sanitizeSyncLogText(strings.TrimSpace(orgID)))
	if len(runes) <= 4 {
		return "***"
	}
	return string(runes[:2]) + "***" + string(runes[len(runes)-2:])
}

// SyncOrgData 同步组织数据
func SyncOrgData(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok || !validateOrgSyncRequestSelectors(c, orgID) {
		return
	}

	startTime := time.Now()
	requestID := orgSyncRequestID(c, startTime)
	maskedOrgID := redactOrgIDForSyncLog(orgID)

	unlock, locked := tryAcquireOrgSync(orgID)
	if !locked {
		respondOrgSyncConflict(c, requestID, maskedOrgID, startTime)
		return
	}
	defer unlock()

	// 组织同步是服务器长任务。外层代理或浏览器断开连接后继续使用带租户值的
	// 独立执行上下文，避免请求取消把尚未处理的全部员工误记为失败。
	syncContext, cancelSync := newOrgSyncExecutionContext(c.Request.Context())
	defer cancelSync()
	middleware.RebindRequestContext(c, syncContext)

	result, overall := executeOrgSync(c, orgID, requestID, maskedOrgID, startTime)

	c.JSON(overall.HTTPStatus, Response{
		Code:    overall.HTTPStatus,
		Message: result.OverallStatus,
		Data:    result,
	})
}

// StartOrgSyncData 启动后台组织同步。启动与查询均为短请求，避免外层代理在长任务完成前返回 504。
func StartOrgSyncData(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok || !validateOrgSyncRequestSelectors(c, orgID) {
		return
	}
	startTime := time.Now()
	requestID := orgSyncRequestID(c, startTime)
	maskedOrgID := redactOrgIDForSyncLog(orgID)
	unlock, locked := tryAcquireOrgSync(orgID)
	if !locked {
		respondOrgSyncConflict(c, requestID, maskedOrgID, startTime)
		return
	}

	syncContext, cancelSync := newOrgSyncExecutionContext(c.Request.Context())
	backgroundContext := c.Copy()
	middleware.RebindRequestContext(backgroundContext, syncContext)
	if err := writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{
		SyncType:  "organization",
		Status:    "running",
		Message:   "组织数据正在后台同步",
		RequestID: requestID,
	}); err != nil {
		cancelSync()
		unlock()
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建组织同步任务失败，请稍后重试",
			Data:    gin.H{"request_id": requestID},
		})
		return
	}

	go func() {
		defer unlock()
		defer cancelSync()
		defer func() {
			if recovered := recover(); recovered != nil {
				statusContext, cancelStatus := newOrgSyncStatusContext(backgroundContext)
				defer cancelStatus()
				failedResult := orgSyncResponseData{
					OverallStatus: "failed",
					Departments:   orgSyncStageResult{Status: "failed", FailCount: 1, Error: orgSyncPanicMessage, ErrorCode: orgSyncPanicErrorCode},
					Employees:     orgSyncEmployeeStageResult{Status: "skipped", Error: employeeSyncSkippedMessage, ErrorCode: employeeSyncSkippedErrorCode},
					SyncTime:      time.Now().Format(time.RFC3339),
					DurationMS:    time.Since(startTime).Milliseconds(),
					RequestID:     requestID,
				}
				if err := writeOrgSyncStatusForRequest(statusContext, orgID, orgSyncStatusUpdate{
					SyncType:   "organization",
					Status:     "failed",
					Message:    orgSyncPanicMessage,
					RequestID:  requestID,
					DurationMS: failedResult.DurationMS,
					ErrorCode:  orgSyncPanicErrorCode,
					FailCount:  1,
					Details:    map[string]interface{}{"result": failedResult},
				}); err != nil {
					log.Printf("[SyncOrgData] request_id=%s org=%s stage=background result=panic_status_persist_failed error_type=%T", sanitizeSyncLogText(requestID), maskedOrgID, err)
				}
				log.Printf("[SyncOrgData] request_id=%s org=%s stage=background result=panic panic_type=%T", sanitizeSyncLogText(requestID), maskedOrgID, recovered)
			}
		}()

		result, _ := executeOrgSync(backgroundContext, orgID, requestID, maskedOrgID, startTime)
		statusContext, cancelStatus := newOrgSyncStatusContext(backgroundContext)
		defer cancelStatus()
		if err := writeOrgSyncStatusForRequest(statusContext, orgID, orgSyncStatusUpdate{
			SyncType:     "organization",
			Status:       result.OverallStatus,
			Message:      result.OverallStatus,
			RequestID:    requestID,
			DurationMS:   result.DurationMS,
			ErrorCode:    orgSyncResultErrorCode(result),
			SuccessCount: result.Departments.SuccessCount + result.Employees.SuccessCount,
			FailCount:    result.Departments.FailCount + result.Employees.FailCount,
			Details:      map[string]interface{}{"result": result},
		}); err != nil {
			log.Printf("[SyncOrgData] request_id=%s org=%s stage=background result=final_status_persist_failed error_type=%T", sanitizeSyncLogText(requestID), maskedOrgID, err)
		}
	}()

	c.JSON(http.StatusAccepted, Response{
		Code:    http.StatusAccepted,
		Message: "running",
		Data: gin.H{
			"status":     "running",
			"request_id": requestID,
		},
	})
}

// GetOrgSyncResult 查询当前组织指定后台同步请求的运行状态或最终完整结果。
func GetOrgSyncResult(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	rawRequestID := strings.TrimSpace(c.Param("request_id"))
	requestID := sanitizeSyncLogText(rawRequestID)
	if requestID == "" || len(rawRequestID) > 128 || len(requestID) > 128 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "同步请求编号无效"})
		return
	}
	status, err := service.NewSyncService(middleware.RequestDB(c)).GetSyncStatusByRequestID(orgID, "organization", requestID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "未找到该同步任务"})
		return
	}
	if err != nil {
		log.Printf("[SyncOrgData] request_id=%s org=%s stage=query result=failed error_type=%T", requestID, redactOrgIDForSyncLog(orgID), err)
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "查询组织同步任务失败，请稍后重试"})
		return
	}
	if status == nil {
		c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "未找到该同步任务"})
		return
	}
	if status.Status == "running" {
		c.JSON(http.StatusAccepted, Response{
			Code:    http.StatusAccepted,
			Message: "running",
			Data: gin.H{
				"status":      "running",
				"request_id":  requestID,
				"duration_ms": time.Since(status.LastSyncTime).Milliseconds(),
			},
		})
		return
	}
	result, exists := status.Details["result"]
	if !exists {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "同步已结束，但结果详情不可用"})
		return
	}
	httpStatus := http.StatusInternalServerError
	switch status.Status {
	case "success":
		httpStatus = http.StatusOK
	case "partial_failed":
		httpStatus = http.StatusMultiStatus
	}
	c.JSON(httpStatus, Response{Code: httpStatus, Message: status.Status, Data: result})
}

func executeOrgSync(c *gin.Context, orgID, requestID, maskedOrgID string, startTime time.Time) (orgSyncResponseData, syncOverallResult) {
	result := runOrgSyncForRequest(c, orgID, requestID, startTime)
	if errors.Is(c.Request.Context().Err(), context.DeadlineExceeded) {
		result = markOrgSyncTimedOut(result)
	}
	overall := computeSyncOverallResult(result.Departments.Status, result.Employees.Status)
	result.OverallStatus = overall.OverallStatus
	result.SyncTime = time.Now().Format(time.RFC3339)
	result.DurationMS = time.Since(startTime).Milliseconds()
	result.RequestID = requestID
	log.Printf("[SyncOrgData] request_id=%s org=%s 阶段=overall 结果=%s 耗时=%dms 成功数=%d 失败数=%d dept_ok=%d dept_fail=%d user_ok=%d user_fail=%d",
		requestID, maskedOrgID, result.OverallStatus, result.DurationMS,
		result.Departments.SuccessCount+result.Employees.SuccessCount,
		result.Departments.FailCount+result.Employees.FailCount,
		result.Departments.SuccessCount, result.Departments.FailCount,
		result.Employees.SuccessCount, result.Employees.FailCount)
	return result, overall
}

const (
	orgSyncMaxExecutionTimeout          = 15 * time.Minute
	orgSyncStatusPersistTimeout         = 5 * time.Second
	departmentSyncFailedMessage         = "部门同步失败，请前往同步日志查看或联系管理员"
	departmentPersistFailedMessage      = "部门数据写入失败，原有组织数据未被覆盖"
	employeeSyncFailedMessage           = "员工同步失败，请前往同步日志查看或联系管理员"
	employeeSyncCanceledMessage         = "同步连接已中断，员工同步未完成，请前往同步日志确认"
	employeeSyncSkippedMessage          = "部门同步未完成，已跳过员工同步"
	employeeHRMSyncFailedMessage        = "员工基础资料已同步，但智能人事字段同步失败，请检查钉钉应用的智能人事花名册权限"
	employeeHRMNoFieldsMessage          = "智能人事接口调用成功，但未获取到员工类型、职级、岗位序列字段，请检查钉钉应用的花名册字段权限或字段代码配置"
	departmentPersistErrorCode          = "DEPARTMENT_PERSIST_FAILED"
	employeeSyncSkippedErrorCode        = "EMPLOYEE_SYNC_SKIPPED"
	employeeSyncFailedErrorCode         = "EMPLOYEE_SYNC_FAILED"
	employeeSyncCanceledErrorCode       = "EMPLOYEE_SYNC_CANCELED"
	employeeDeactivationFailedErrorCode = "EMPLOYEE_DEACTIVATION_FAILED"
	employeeDeactivationFailedMessage   = "员工基础资料已同步，但历史员工安全停用未完成，请前往同步日志查看"
	orgSyncTimeoutErrorCode             = "ORG_SYNC_TIMEOUT"
	orgSyncTimeoutMessage               = "组织同步执行超时，请前往同步日志查看或稍后重试"
	orgSyncPanicErrorCode               = "ORG_SYNC_PANIC"
	orgSyncPanicMessage                 = "组织同步异常终止，请前往同步日志查看或联系管理员"
)

var orgSyncExecutionTimeout = orgSyncMaxExecutionTimeout

func newOrgSyncExecutionContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := orgSyncExecutionTimeout
	if timeout <= 0 || timeout > orgSyncMaxExecutionTimeout {
		timeout = orgSyncMaxExecutionTimeout
	}
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

func newOrgSyncStatusContext(source *gin.Context) (*gin.Context, context.CancelFunc) {
	statusContext := source.Copy()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(source.Request.Context()), orgSyncStatusPersistTimeout)
	middleware.RebindRequestContext(statusContext, ctx)
	return statusContext, cancel
}

func markOrgSyncTimedOut(result orgSyncResponseData) orgSyncResponseData {
	if result.Departments.Status == "" {
		result.Departments.Status = "failed"
		result.Departments.FailCount = max(result.Departments.FailCount, 1)
		result.Departments.Error = orgSyncTimeoutMessage
		result.Departments.ErrorCode = orgSyncTimeoutErrorCode
	}
	if result.Employees.Status == "" || result.Employees.Status == "success" {
		result.Employees.Status = "failed"
		result.Employees.FailCount = max(result.Employees.FailCount, 1)
		result.Employees.Error = orgSyncTimeoutMessage
		result.Employees.ErrorCode = orgSyncTimeoutErrorCode
	} else if result.Employees.ErrorCode == "" {
		result.Employees.Error = orgSyncTimeoutMessage
		result.Employees.ErrorCode = orgSyncTimeoutErrorCode
	}
	return result
}

func orgSyncResultErrorCode(result orgSyncResponseData) string {
	if result.Departments.ErrorCode != "" {
		return result.Departments.ErrorCode
	}
	return result.Employees.ErrorCode
}

func fetchOrgSyncUsers(orgID string, depts []dingtalk.DeptInfo, departmentStageErr error) ([]dingtalk.UserInfo, error) {
	if departmentStageErr != nil {
		return nil, errors.New(employeeSyncSkippedMessage)
	}
	users, userErr := syncUsersWithDeptsForOrg(orgID, depts)
	return users, userErr
}

func performOrgSync(c *gin.Context, orgID, requestID string, startTime time.Time) orgSyncResponseData {
	maskedOrgID := redactOrgIDForSyncLog(orgID)

	// --- 阶段 1：同步部门 ---
	depts, departmentStageErr := syncDepartmentsForOrg(orgID)
	deptSuccessCount := 0
	deptFailCount := 0
	deptStatus := "success"
	deptErrMsg := ""
	deptErrorCode := ""
	if departmentStageErr == nil {
		departmentStageErr = validateOrgSyncDepartments(depts)
	}
	if departmentStageErr != nil {
		deptStatus = "failed"
		deptFailCount = max(len(depts), 1)
		deptErrorCode, deptErrMsg = orgSyncErrorDetails(departmentStageErr, dingtalk.ErrorCodeResponseInvalid, departmentSyncFailedMessage)
		logOrgSyncError(requestID, maskedOrgID, "departments", "source_or_validation", departmentStageErr, startTime, 0, deptFailCount)
	} else {
		deptResult, err := persistOrgSyncDepartments(c, orgID, depts)
		if err != nil {
			departmentStageErr = &orgSyncCodedError{Code: departmentPersistErrorCode, SafeMessage: departmentPersistFailedMessage, Cause: err}
			deptStatus = "failed"
			deptFailCount = max(len(depts), 1)
			deptErrorCode = departmentPersistErrorCode
			deptErrMsg = departmentPersistFailedMessage
			logOrgSyncError(requestID, maskedOrgID, "departments", "persist", departmentStageErr, startTime, 0, deptFailCount)
		} else {
			deptSuccessCount = deptResult.Count
			log.Printf("[org-sync] request_id=%s org=%s stage=departments result=success duration_ms=%d success_count=%d fail_count=0",
				sanitizeSyncLogText(requestID), maskedOrgID, time.Since(startTime).Milliseconds(), deptSuccessCount)
		}
	}
	deptSyncMessage := fmt.Sprintf("同步 %d 个部门", deptSuccessCount)
	if deptStatus != "success" {
		deptSyncMessage = deptErrMsg
	}
	_ = writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{SyncType: "departments", Status: deptStatus, Message: deptSyncMessage, RequestID: requestID, DurationMS: time.Since(startTime).Milliseconds(), ErrorCode: deptErrorCode, SuccessCount: deptSuccessCount, FailCount: deptFailCount})

	// --- 阶段 2：同步用户（复用已有部门列表，避免重复调用 SyncDepartments） ---
	users, userErr := fetchOrgSyncUsers(orgID, depts, departmentStageErr)
	overwriteEmpty := shouldOverwriteEmptyDingTalkOrgFields(c)
	userResult := orgSyncEmployeeStageResult{Status: "success", OverwriteEmpty: overwriteEmpty}
	if userErr != nil {
		if departmentStageErr != nil {
			userResult.Status = "skipped"
			userResult.FailCount = 0
			userResult.Error = employeeSyncSkippedMessage
			userResult.ErrorCode = employeeSyncSkippedErrorCode
		} else {
			userResult.Status = "failed"
			userResult.FailCount = 1
			userResult.ErrorCode, userResult.Error = orgSyncErrorDetails(userErr, employeeSyncFailedErrorCode, employeeSyncFailedMessage)
		}
		logOrgSyncError(requestID, maskedOrgID, "employees", "source_or_skipped", userErr, startTime, 0, userResult.FailCount)
	} else {
		userResult = syncDingTalkUsers(c.Request.Context(), orgID, users, overwriteEmpty, orgSyncUserDependenciesForRequest(c, orgID), requestID, maskedOrgID, startTime)
	}

	userSyncMessage := fmt.Sprintf("同步 %d 个用户", userResult.SuccessCount)
	switch userResult.Status {
	case "partial_failed":
		if userResult.FailCount == 0 && userResult.HRMFieldStatus == "failed" {
			userSyncMessage = employeeHRMSyncFailedMessage
		} else {
			userSyncMessage = fmt.Sprintf("同步部分完成：成功 %d，失败 %d", userResult.SuccessCount, userResult.FailCount)
		}
	case "failed":
		userSyncMessage = userResult.Error
	case "skipped":
		userSyncMessage = employeeSyncSkippedMessage
	}
	_ = writeOrgSyncStatusForRequest(c, orgID, orgSyncStatusUpdate{SyncType: "users", Status: userResult.Status, Message: userSyncMessage, RequestID: requestID, DurationMS: time.Since(startTime).Milliseconds(), ErrorCode: userResult.ErrorCode, SuccessCount: userResult.SuccessCount, FailCount: userResult.FailCount})
	log.Printf("[org-sync] request_id=%s org=%s stage=employees result=%s duration_ms=%d success_count=%d fail_count=%d",
		sanitizeSyncLogText(requestID), maskedOrgID, userResult.Status, time.Since(startTime).Milliseconds(), userResult.SuccessCount, userResult.FailCount)

	return orgSyncResponseData{
		Departments: orgSyncStageResult{
			Status:       deptStatus,
			SuccessCount: deptSuccessCount,
			FailCount:    deptFailCount,
			Error:        deptErrMsg,
			ErrorCode:    deptErrorCode,
		},
		Employees: userResult,
	}
}

// GetAttendanceRecords 获取考勤记录列表
func GetAttendanceRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"user_id":       c.Query("user_id"),
		"department_id": c.Query("department_id"),
		"start_date":    c.Query("start_date"),
		"end_date":      c.Query("end_date"),
	}
	if !currentUserHasAnyPermission(c, "attendance_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	attendanceService := service.NewAttendanceService(middleware.RequestDB(c))
	records, total, err := attendanceService.GetRecords(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取考勤记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": records,
			"total": total,
		},
	})
}

// SyncAttendance 同步考勤数据
func SyncAttendance(c *gin.Context) {
	var req struct {
		StartDate string `json:"start_date"`
		EndDate   string `json:"end_date"`
		Force     bool   `json:"force"` // true 时先删除该范围内旧记录再重新拉取
	}
	c.ShouldBindJSON(&req)

	if req.StartDate == "" {
		req.StartDate = time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	}
	if req.EndDate == "" {
		req.EndDate = time.Now().Format("2006-01-02")
	}

	if req.Force {
		cst := time.FixedZone("CST", 8*3600)
		start, err1 := time.ParseInLocation("2006-01-02", req.StartDate, cst)
		end, err2 := time.ParseInLocation("2006-01-02", req.EndDate, cst)
		if err1 != nil || err2 != nil {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "日期格式错误"})
			return
		}
		end = end.AddDate(0, 0, 1) // 包含 end 当天
		if err := middleware.RequestDB(c).Where("check_time >= ? AND check_time < ?", start, end).Delete(&database.Attendance{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "清理旧记录失败: " + err.Error()})
			return
		}
	}

	syncService := service.NewSyncService(middleware.RequestDB(c))
	orgID := database.NormalizeOrganizationID(c.GetString("orgID"))

	// 获取所有用户的钉钉 UserID
	var users []database.User
	middleware.RequestDB(c).Select("user_id, name").Find(&users)

	var userIDs []string
	userNameMap := make(map[string]string)
	for _, u := range users {
		if u.UserID != "" && u.UserID != "admin" {
			userIDs = append(userIDs, u.UserID)
			userNameMap[u.UserID] = u.Name
		}
	}

	if len(userIDs) == 0 {
		updateSyncStatus(syncService, orgID, "attendance", "success", "没有需要同步的用户")
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data: gin.H{
				"sync_status": gin.H{"count": 0, "status": "success", "sync_time": time.Now()},
			},
		})
		return
	}

	records, err := dingtalk.GetAttendanceForOrg(orgID, userIDs, req.StartDate, req.EndDate)
	if err != nil {
		updateSyncStatus(syncService, orgID, "attendance", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步考勤失败: " + err.Error(),
		})
		return
	}

	// 写入数据库
	count := 0
	for _, r := range records {
		checkType := "上班"
		if r.CheckType == "OffDuty" {
			checkType = "下班"
		}
		checkTime, _ := time.ParseInLocation("2006-01-02 15:04:05", r.UserCheckTime, time.FixedZone("CST", 8*3600))

		record := &database.Attendance{
			UserID:    r.UserID,
			UserName:  userNameMap[r.UserID],
			CheckTime: checkTime,
			CheckType: checkType,
			Location:  r.LocationResult,
			Extension: map[string]interface{}{
				"time_result":     r.TimeResult,
				"location_result": r.LocationResult,
			},
		}
		if r.TimeResult == "Late" || r.TimeResult == "Early" || r.TimeResult == "NotSigned" {
			abnormalType := "迟到"
			if r.TimeResult == "Early" {
				abnormalType = "早退"
			} else if r.TimeResult == "NotSigned" {
				abnormalType = "缺勤"
			}
			record.Extension["abnormal_type"] = abnormalType
		}

		if err := service.NewAttendanceService(middleware.RequestDB(c)).SaveRecord(record); err != nil {
			updateSyncStatus(syncService, orgID, "attendance", "failed", err.Error())
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "鍚屾鑰冨嫟澶辫触: " + err.Error(),
			})
			return
		}
		count++
	}

	updateSyncStatus(syncService, orgID, "attendance", "success", fmt.Sprintf("同步 %d 条考勤记录", count))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"sync_status": gin.H{
				"count":      count,
				"status":     "success",
				"sync_time":  time.Now(),
				"start_date": req.StartDate,
				"end_date":   req.EndDate,
			},
		},
	})
}

// ExportAttendance 导出考勤数据
func ExportAttendance(c *gin.Context) {
	var req struct {
		StartDate    string `json:"start_date" binding:"required"`
		EndDate      string `json:"end_date" binding:"required"`
		UserID       string `json:"user_id"`
		DepartmentID string `json:"department_id"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误：开始日期和结束日期不能为空",
		})
		return
	}

	// 获取当前用户信息
	userID, _ := c.Get("userID")
	userName, _ := c.Get("userName")
	uid, _ := userID.(string)
	uname, _ := userName.(string)
	if user, err := loadUserByAuthIDInOrg(c.GetString("orgID"), uid); err == nil {
		uid = user.UserID
		if strings.TrimSpace(uname) == "" {
			uname = user.Name
		}
	}

	fileName := fmt.Sprintf("attendance_%s_%s.xlsx", req.StartDate, req.EndDate)
	export := &database.AttendanceExport{
		UserID:    uid,
		UserName:  uname,
		FileName:  fileName,
		Status:    "pending",
		StartDate: req.StartDate,
		EndDate:   req.EndDate,
	}

	attendanceService := service.NewAttendanceService(middleware.RequestDB(c))
	if err := attendanceService.CreateExport(export); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建导出任务失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"export_id":    export.ID,
			"file_name":    export.FileName,
			"record_count": 0,
			"status":       export.Status,
			"created_at":   export.CreatedAt,
		},
	})
}

// GetAttendanceExports 获取导出记录列表
func GetAttendanceExports(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	attendanceService := service.NewAttendanceService(middleware.RequestDB(c))
	exports, total, err := attendanceService.GetExports(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取导出记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": exports,
			"total": total,
		},
	})
}

// GetLastSyncTime 获取最近同步时间
func GetLastSyncTime(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	attendanceService := service.NewAttendanceService(middleware.RequestDB(c))
	status, err := attendanceService.GetLastSyncTime(orgID)
	if err != nil {
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data: gin.H{
				"attendance": gin.H{
					"last_sync_time": nil,
					"status":         "never",
					"record_count":   0,
				},
			},
		})
		return
	}

	var count int64
	requestDB := middleware.RequestDB(c)
	// Attendance counts are tenant-scoped: never aggregate across organizations.
	countQuery := requestDB.Model(&database.Attendance{}).Where("org_id = ?", orgID)
	if !currentUserHasAnyPermission(c, "attendance_manage") {
		filters := map[string]string{}
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
		if userID := strings.TrimSpace(filters["user_id"]); userID != "" {
			countQuery = countQuery.Where("user_id = ?", userID)
		} else if departmentID := strings.TrimSpace(filters["department_id"]); departmentID != "" {
			countQuery = countQuery.Where("user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id = ? AND deleted_at IS NULL)", orgID, departmentID)
		} else if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
			countQuery = countQuery.Where("user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id IN ? AND deleted_at IS NULL)", orgID, departmentIDs)
		}
	}
	countQuery.Count(&count)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"attendance": gin.H{
				"last_sync_time": status.LastSyncTime,
				"status":         status.Status,
				"record_count":   count,
			},
		},
	})
}

// GetApprovalTemplates 获取审批模板列表
func GetApprovalTemplates(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	approvalService := service.NewApprovalServiceWithOrgID(middleware.RequestDB(c), orgID)
	templates, total, err := approvalService.GetTemplates()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取审批模板失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": templates,
			"total": total,
		},
	})
}

// GetApprovalInstances 获取审批实例列表
func GetApprovalInstances(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	templateID := c.Query("template_id")
	filters := map[string]string{
		"status":       c.Query("status"),
		"template_id":  templateID,
		"applicant_id": c.Query("applicant_id"),
		"title":        strings.TrimSpace(c.Query("title")),
		"start_date":   c.Query("start_date"),
		"end_date":     c.Query("end_date"),
	}

	db := middleware.RequestDB(c)
	approvalService := service.NewApprovalServiceWithOrgID(db, orgID)

	// category 参数：仅当未显式指定 template_id 时才生效（template_id 优先），
	// 并强制走白名单避免 SQL 注入或未知取值退化为无条件查询。
	// 匹配策略走审批标题关键字，因为 extension->>'$.template_id' 库里大量为空。
	if templateID == "" {
		if categoryKey, ok := service.ParseApprovalCategory(c.Query("category")); ok {
			var keywords []string
			include := true
			if categoryKey == service.ApprovalCategoryOther {
				keywords = service.AllApprovalCategoryTitleKeywords()
				include = false
			} else {
				keywords = service.ApprovalCategoryTitleKeywords(categoryKey)
			}
			delete(filters, "template_id")
			instances, total, err := approvalService.GetInstancesFilteredByTitleKeywords(page, pageSize, keywords, include, filters)
			if err != nil {
				c.JSON(http.StatusInternalServerError, Response{
					Code:    http.StatusInternalServerError,
					Message: "获取审批实例失败",
				})
				return
			}
			c.JSON(http.StatusOK, Response{
				Code:    http.StatusOK,
				Message: "success",
				Data: gin.H{
					"items": instances,
					"total": total,
				},
			})
			return
		}
	}

	instances, total, err := approvalService.GetInstances(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取审批实例失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": instances,
			"total": total,
		},
	})
}

// GetApproval 获取审批详情
func GetApproval(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")

	approvalService := service.NewApprovalServiceWithOrgID(middleware.RequestDB(c), orgID)
	approval, err := approvalService.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "审批不存在",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"approval": approval},
	})
}

// SyncApproval 同步审批数据
func SyncApproval(c *gin.Context) {
	syncApprovalCompat(c)
}

// GetRoles 获取角色列表
func GetRoles(c *gin.Context) {
	permService := service.NewPermissionService(middleware.RequestDB(c))
	roles, total, err := permService.GetRoles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取角色列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": roles,
			"total": total,
		},
	})
}

// CreateRole 创建角色
func CreateRole(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	role := &database.Role{
		Name:        req.Name,
		Description: req.Description,
	}

	permService := service.NewPermissionService(middleware.RequestDB(c))
	if err := permService.CreateRole(role); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建角色失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"role": role},
	})
}

// UpdateRole 更新角色
func UpdateRole(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("role_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "无效的角色ID"})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	role := &database.Role{
		Name:        req.Name,
		Description: req.Description,
	}
	role.ID = uint(id)

	permService := service.NewPermissionService(middleware.RequestDB(c))
	if err := permService.UpdateRole(role); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, Response{Code: http.StatusNotFound, Message: "当前企业下未找到该角色"})
			return
		}
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新角色失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"role": role},
	})
}

// GetPermissions 获取权限列表
func GetPermissions(c *gin.Context) {
	permService := service.NewPermissionService(middleware.RequestDB(c))
	permissions, total, err := permService.GetPermissions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取权限列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": permissions,
			"total": total,
		},
	})
}

// GetUserRoles 获取指定用户的角色列表
func GetUserRoles(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "user_id 不能为空"})
		return
	}
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !rejectCrossOrgParam(c, orgID, c.Query("org_id")) {
		return
	}
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	roles, err := permService.GetUserRolesInOrg(orgID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取用户角色失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"roles": roles}})
}

// AssignUserRole 给用户分配角色
func AssignUserRole(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		OrgID  string `json:"org_id"`
		RoleID uint   `json:"role_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	if req.UserID == "" || req.RoleID == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "user_id 和 role_id 不能为空"})
		return
	}
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !rejectCrossOrgParam(c, orgID, req.OrgID, c.Query("org_id")) {
		return
	}
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	if err := permService.AssignUserRoleInOrg(orgID, req.UserID, req.RoleID); err != nil {
		if respondPermissionOrgNotFound(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "分配角色失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "角色设置成功"})
}

// RemoveUserRole 移除用户角色
func RemoveUserRole(c *gin.Context) {
	var req struct {
		UserID string `json:"user_id" binding:"required"`
		OrgID  string `json:"org_id"`
		RoleID uint   `json:"role_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	if req.UserID == "" || req.RoleID == 0 {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "user_id 和 role_id 不能为空"})
		return
	}
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !rejectCrossOrgParam(c, orgID, req.OrgID, c.Query("org_id")) {
		return
	}
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	if err := permService.RemoveUserRoleInOrg(orgID, req.UserID, req.RoleID); err != nil {
		if respondPermissionOrgNotFound(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "移除角色失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "角色移除成功"})
}

// respondPermissionOrgNotFound maps cross-org / missing user-or-role errors to 404.
// Intentionally does not distinguish missing vs foreign to avoid org membership probing.
func respondPermissionOrgNotFound(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, service.ErrUserNotInOrg) ||
		errors.Is(err, service.ErrRoleNotInOrg) ||
		errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "用户或角色不存在",
		})
		return true
	}
	return false
}

// GetRoleUsers 获取指定角色下的用户列表
func GetRoleUsers(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	if roleIDStr == "" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 不能为空"})
		return
	}
	var roleID uint
	if _, err := fmt.Sscanf(roleIDStr, "%d", &roleID); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 格式错误"})
		return
	}
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !rejectCrossOrgParam(c, orgID, c.Query("org_id")) {
		return
	}
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	users, err := permService.GetRoleUsersInOrg(orgID, roleID)
	if err != nil {
		if respondPermissionOrgNotFound(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取角色用户失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"users": users}})
}

// GetUserPermissions 获取指定用户的权限码列表
func GetUserPermissions(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "user_id 不能为空"})
		return
	}
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	if !rejectCrossOrgParam(c, orgID, c.Query("org_id")) {
		return
	}
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	permissions, err := permService.GetUserPermissionsInOrg(orgID, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取用户权限失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"permissions": permissions}})
}

// GetMenuPermission 获取角色的菜单权限
func GetMenuPermission(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 格式错误"})
		return
	}
	permService := service.NewPermissionService(middleware.RequestDB(c))
	// 从功能权限码派生菜单 keys（不再读 menu_permissions 旧表）
	menuKeys, err := permService.GetRoleMenuKeys(uint(roleID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取菜单权限失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"menu_keys": menuKeys}})
}

func GetRolePermissions(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 格式错误"})
		return
	}
	permService := service.NewPermissionService(middleware.RequestDB(c))
	permissions, err := permService.GetRolePermissions(uint(roleID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取角色功能权限失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"permissions": permissions}})
}

func SaveRolePermissions(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 格式错误"})
		return
	}
	var req struct {
		PermissionIDs []uint `json:"permission_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	permService := service.NewPermissionService(middleware.RequestDB(c))
	if err := permService.SaveRolePermissions(uint(roleID), req.PermissionIDs); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "保存功能权限失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "功能权限保存成功"})
}

func parseMenuKeysPayload(payload json.RawMessage) ([]string, error) {
	var keys []string
	if err := json.Unmarshal(payload, &keys); err == nil {
		return keys, nil
	}

	var encoded string
	if err := json.Unmarshal(payload, &encoded); err != nil {
		return nil, err
	}
	return service.ParseMenuKeys(encoded)
}

// SaveMenuPermission 保存角色的菜单权限
func SaveMenuPermission(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 格式错误"})
		return
	}
	var req struct {
		MenuKeys json.RawMessage `json:"menu_keys" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	menuKeys, err := parseMenuKeysPayload(req.MenuKeys)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "menu_keys 必须是 JSON 数组"})
		return
	}
	permService := service.NewPermissionService(middleware.RequestDB(c))
	if err := permService.SaveMenuPermissionKeys(uint(roleID), menuKeys); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "保存菜单权限失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "菜单权限保存成功"})
}

// GetDataPermission 获取角色的数据权限
func GetDataPermission(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 格式错误"})
		return
	}
	permService := service.NewPermissionService(middleware.RequestDB(c))
	scope, departmentKeys, err := permService.GetDataPermission(uint(roleID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "获取数据权限失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{
		"scope":           scope,
		"department_keys": departmentKeys,
	}})
}

// SaveDataPermission 保存角色的数据权限
func SaveDataPermission(c *gin.Context) {
	roleIDStr := c.Param("role_id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		log.Printf("[SaveDataPermission] role_id 格式错误: %s", roleIDStr)
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "role_id 格式错误"})
		return
	}
	var req struct {
		Scope          string `json:"scope" binding:"required"`
		DepartmentKeys string `json:"department_keys"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[SaveDataPermission] 参数绑定错误: %v, roleID: %d", err, roleID)
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	log.Printf("[SaveDataPermission] roleID: %d, scope: %s, department_keys: %s", roleID, req.Scope, req.DepartmentKeys)
	if req.Scope != "all" && req.Scope != "department" && req.Scope != "self" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "scope 值无效，仅支持 all、department 或 self"})
		return
	}
	permService := service.NewPermissionService(middleware.RequestDB(c))
	if err := permService.SaveDataPermission(uint(roleID), req.Scope, req.DepartmentKeys); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "保存数据权限失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "数据权限保存成功"})
}

// GetAuditLogs 获取审计日志
func GetAuditLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"user_id":    c.Query("user_id"),
		"start_date": c.Query("start_date"),
		"end_date":   c.Query("end_date"),
	}

	auditService := service.NewAuditService(middleware.RequestDB(c))
	logs, total, err := auditService.GetLogs(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取审计日志失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": logs,
			"total": total,
		},
	})
}

// GetJobs 获取任务列表
func GetJobs(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	// 任务列表基于同步状态表动态生成
	syncService := service.NewSyncService(middleware.RequestDB(c))

	jobs := []gin.H{
		{"id": "1", "name": "同步用户数据", "description": "从钉钉同步用户数据", "type": "sync_users", "status": "idle"},
		{"id": "2", "name": "同步部门数据", "description": "从钉钉同步部门数据", "type": "sync_departments", "status": "idle"},
		{"id": "3", "name": "同步考勤数据", "description": "从钉钉同步考勤数据", "type": "sync_attendance", "status": "idle"},
	}

	typeMap := map[string]string{"1": "users", "2": "departments", "3": "attendance"}
	for i, job := range jobs {
		syncType := typeMap[job["id"].(string)]
		if status, err := syncService.GetSyncStatus(orgID, syncType); err == nil {
			jobs[i]["last_run_time"] = status.LastSyncTime
			jobs[i]["status"] = status.Status
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": jobs,
			"total": len(jobs),
		},
	})
}

// RunJob 运行任务
func RunJob(c *gin.Context) {
	id := c.Param("id")
	orgID, orgOK := currentOrgIDOrAbort(c)
	if !orgOK {
		return
	}

	typeMap := map[string]string{"1": "users", "2": "departments", "3": "attendance"}
	syncType, ok := typeMap[id]
	if !ok {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "任务不存在",
		})
		return
	}

	syncService := service.NewSyncService(middleware.RequestDB(c))
	updateSyncStatus(syncService, orgID, syncType, "success", "手动执行任务")

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"job": gin.H{
				"id":         id,
				"status":     "completed",
				"start_time": time.Now(),
			},
		},
	})
}

// 员工档案中心接口

// GetEmployeeProfiles 获取员工档案列表
func GetEmployeeProfiles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	employeeService, _, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	filters := map[string]string{
		"department_id": c.Query("department_id"),
		"status":        c.Query("status"),
		"keyword":       strings.TrimSpace(c.Query("keyword")),
	}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	profiles, total, err := employeeService.GetProfiles(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取员工档案失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": profiles,
			"total": total,
		},
	})
}

// GetEmployeeProfile 获取员工档案详情
func GetEmployeeProfile(c *gin.Context) {
	id := c.Param("id")
	employeeService, orgID, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	profile, err := employeeService.GetProfileByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "档案不存在",
		})
		return
	}
	// Profile rows from other organizations must never be readable via primary key alone.
	if strings.TrimSpace(profile.OrgID) != "" && strings.TrimSpace(profile.OrgID) != orgID {
		respondOrgAccessDenied(c)
		return
	}
	if !currentUserHasAnyPermission(c, "user_manage") {
		scope, err := resolveOrgScope(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "获取组织范围失败",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		user, err := loadUserByUserID(orgID, profile.UserID)
		if err != nil || !canAccessUserByScope(scope, user) {
			respondOrgAccessDenied(c)
			return
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"profile": profile},
	})
}

// CreateEmployeeProfile 创建员工档案
func CreateEmployeeProfile(c *gin.Context) {
	employeeService, orgID, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	var profile database.EmployeeProfile
	if err := c.ShouldBindJSON(&profile); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}
	if !rejectCrossOrgParam(c, orgID, profile.OrgID) {
		return
	}
	// 强制绑定 JWT 组织，禁止客户端指定 org_id。
	profile.OrgID = orgID

	if profile.ProfileStatus == "" {
		profile.ProfileStatus = "active"
	}

	if err := employeeService.CreateProfile(&profile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建档案失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"profile": profile},
	})
}

// UpdateEmployeeProfile 更新员工档案
func UpdateEmployeeProfile(c *gin.Context) {
	id := c.Param("id")
	employeeService, orgID, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	profile, err := employeeService.GetProfileByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "档案不存在",
		})
		return
	}

	if err := c.ShouldBindJSON(profile); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}
	if !rejectCrossOrgParam(c, orgID, profile.OrgID) {
		return
	}
	// 强制保留 JWT 组织，禁止客户端改写 org_id。
	profile.OrgID = orgID

	if err := employeeService.UpdateProfile(profile); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新档案失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"profile": profile},
	})
}

// GetEmployeeLifecycleLedger 获取入转调离台账
func GetEmployeeLifecycleLedger(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	employeeService, _, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	filters := map[string]string{
		"department_id": c.Query("department_id"),
		"status":        c.Query("status"),
		"keyword":       strings.TrimSpace(c.Query("keyword")),
	}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	items, total, err := employeeService.GetLifecycleLedger(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取入转调离台账失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items, "total": total},
	})
}

func GetTransfers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	employeeService, _, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}
	filters := map[string]string{"status": c.Query("status")}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	transfers, total, err := employeeService.GetTransfers(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取调动记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": transfers, "total": total},
	})
}

// CreateTransfer 创建调动记录
func CreateTransfer(c *gin.Context) {
	employeeService, orgID, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	var transfer database.EmployeeTransfer
	if err := c.ShouldBindJSON(&transfer); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}
	if !rejectCrossOrgParam(c, orgID, transfer.OrgID) {
		return
	}
	transfer.OrgID = orgID

	if transfer.Status == "" {
		transfer.Status = "pending"
	}

	if err := employeeService.CreateTransfer(&transfer); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建调动记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"transfer": transfer},
	})
}

// GetResignations 获取离职记录列表
func GetResignations(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	employeeService, _, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}
	filters := map[string]string{"status": c.Query("status")}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	resignations, total, err := employeeService.GetResignations(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取离职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": resignations, "total": total},
	})
}

// CreateResignation 创建离职记录
func CreateResignation(c *gin.Context) {
	employeeService, orgID, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	var resignation database.EmployeeResignation
	if err := c.ShouldBindJSON(&resignation); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}
	if !rejectCrossOrgParam(c, orgID, resignation.OrgID) {
		return
	}
	resignation.OrgID = orgID

	if resignation.Status == "" {
		resignation.Status = "pending"
	}

	if err := employeeService.CreateResignation(&resignation); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建离职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"resignation": resignation},
	})
}

// GetOnboardings 获取入职记录列表
func GetOnboardings(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	employeeService, _, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}
	filters := map[string]string{"status": c.Query("status")}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	onboardings, total, err := employeeService.GetOnboardings(page, pageSize, filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取入职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": onboardings, "total": total},
	})
}

// CreateOnboarding 创建入职记录
func CreateOnboarding(c *gin.Context) {
	employeeService, orgID, ok := employeeServiceForRequest(c)
	if !ok {
		return
	}

	var onboarding database.EmployeeOnboarding
	if err := c.ShouldBindJSON(&onboarding); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}
	if !rejectCrossOrgParam(c, orgID, onboarding.OrgID) {
		return
	}
	onboarding.OrgID = orgID

	if onboarding.Status == "" {
		onboarding.Status = "pending"
	}

	if err := employeeService.CreateOnboarding(&onboarding); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建入职记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"onboarding": onboarding},
	})
}

type createTalentAnalysisRequest struct {
	OrgID             *string                `json:"org_id"`
	UserID            string                 `json:"user_id" binding:"required"`
	UserName          string                 `json:"user_name" binding:"required"`
	DepartmentID      string                 `json:"department_id" binding:"required"`
	DepartmentName    string                 `json:"department_name" binding:"required"`
	Position          string                 `json:"position" binding:"required"`
	PerformanceScore  float64                `json:"performance_score"`
	PerformanceLevel  string                 `json:"performance_level"`
	PerformanceReview string                 `json:"performance_review"`
	SkillsAssessment  map[string]interface{} `json:"skills_assessment"`
	PotentialScore    float64                `json:"potential_score"`
	PotentialLevel    string                 `json:"potential_level"`
	TrainingRecords   map[string]interface{} `json:"training_records"`
	PromotionRecords  map[string]interface{} `json:"promotion_records"`
	TurnoverRiskScore float64                `json:"turnover_risk_score"`
	TurnoverRiskLevel string                 `json:"turnover_risk_level"`
	AnalysisDate      string                 `json:"analysis_date" binding:"required"`
	Extension         map[string]interface{} `json:"extension"`
}

type weekScheduleRuleRequest struct {
	OrgID     *string `json:"org_id"`
	ScopeType string  `json:"scope_type"`
	ScopeID   string  `json:"scope_id"`
	ScopeName string  `json:"scope_name"`
	BaseDate  string  `json:"base_date"`
	Pattern   string  `json:"pattern"`
	ShiftID   int64   `json:"shift_id"`
	Status    string  `json:"status"`
}

type weekScheduleOverrideRequest struct {
	OrgID         *string `json:"org_id"`
	ScopeType     string  `json:"scope_type" binding:"required"`
	ScopeID       string  `json:"scope_id"`
	WeekStartDate string  `json:"week_start_date" binding:"required"`
	WeekType      string  `json:"week_type" binding:"required"`
	Reason        string  `json:"reason"`
}

type statutoryHolidayRequest struct {
	OrgID *string `json:"org_id"`
	Date  string  `json:"date" binding:"required"`
	Name  string  `json:"name" binding:"required"`
	Type  string  `json:"type" binding:"required"`
	Year  int     `json:"year"`
}

// GetTalentAnalysisList 获取人才分析列表
func GetTalentAnalysisList(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")

	talentService := service.NewTalentServiceWithOrgID(middleware.RequestDB(c), orgID)
	analyses, total, err := talentService.GetList(page, pageSize, departmentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取人才分析失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"items": analyses,
			"total": total,
		},
	})
}

// GetTalentAnalysisDetail 获取人才分析详情
func GetTalentAnalysisDetail(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")

	talentService := service.NewTalentServiceWithOrgID(middleware.RequestDB(c), orgID)
	analysis, err := talentService.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "分析记录不存在",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"analysis": analysis},
	})
}

// CreateTalentAnalysis 创建人才分析
func CreateTalentAnalysis(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var req createTalentAnalysisRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	if !rejectClientOrganizationID(c, req.OrgID) {
		return
	}
	analysis := database.TalentAnalysis{
		OrgID: orgID, UserID: req.UserID, UserName: req.UserName,
		DepartmentID: req.DepartmentID, DepartmentName: req.DepartmentName, Position: req.Position,
		PerformanceScore: req.PerformanceScore, PerformanceLevel: req.PerformanceLevel,
		PerformanceReview: req.PerformanceReview, SkillsAssessment: req.SkillsAssessment,
		PotentialScore: req.PotentialScore, PotentialLevel: req.PotentialLevel,
		TrainingRecords: req.TrainingRecords, PromotionRecords: req.PromotionRecords,
		TurnoverRiskScore: req.TurnoverRiskScore, TurnoverRiskLevel: req.TurnoverRiskLevel,
		AnalysisDate: req.AnalysisDate, Extension: req.Extension,
	}
	talentService := service.NewTalentServiceWithOrgID(middleware.RequestDB(c), orgID)
	if err := talentService.Create(&analysis); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "创建人才分析失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"analysis": analysis}})
}

// ===================== 大小周管理 =====================

// GetWeekScheduleRules 获取所有大小周规则
func GetWeekScheduleRules(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	rules, err := svc.GetAllRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取规则列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": rules},
	})
}

// CreateWeekScheduleRule 创建大小周规则
func CreateWeekScheduleRule(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var req weekScheduleRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	if !rejectClientOrganizationID(c, req.OrgID) {
		return
	}
	rule := database.WeekScheduleRule{
		OrgID: orgID, ScopeType: req.ScopeType, ScopeID: req.ScopeID, ScopeName: req.ScopeName,
		BaseDate: req.BaseDate, Pattern: req.Pattern, ShiftID: req.ShiftID, Status: req.Status,
	}
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	if err := svc.CreateRule(&rule); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "创建规则失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"rule": rule}})
}

// UpdateWeekScheduleRule 更新大小周规则
func UpdateWeekScheduleRule(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)

	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	existing, err := svc.GetRuleByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "规则不存在",
		})
		return
	}

	var input weekScheduleRuleRequest
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}
	if !rejectClientOrganizationID(c, input.OrgID) {
		return
	}

	if input.ScopeType != "" {
		existing.ScopeType = input.ScopeType
	}
	if input.ScopeID != "" || input.ScopeType == "company" {
		existing.ScopeID = input.ScopeID
	}
	if input.ScopeName != "" {
		existing.ScopeName = input.ScopeName
	}
	if input.BaseDate != "" {
		existing.BaseDate = input.BaseDate
	}
	if input.Pattern != "" {
		existing.Pattern = input.Pattern
	}
	if input.Status != "" {
		existing.Status = input.Status
	}

	if err := svc.UpdateRule(existing); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "更新规则失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"rule": existing},
	})
}

// DeleteWeekScheduleRule 删除大小周规则
func DeleteWeekScheduleRule(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	if err := svc.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除规则失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// BatchSetWeekScheduleRules 批量为员工设置大小周规则
func BatchSetWeekScheduleRules(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var input service.BatchSetUserRulesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	if len(input.UserIDs) == 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请选择至少一个员工",
		})
		return
	}

	if input.BaseDate == "" || input.Pattern == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "base_date 和 pattern 不能为空",
		})
		return
	}

	if input.ConflictMode == "" {
		input.ConflictMode = "skip"
	}

	var users []database.User
	if err := middleware.RequestDB(c).Where("org_id = ? AND user_id IN ? AND deleted_at IS NULL", orgID, input.UserIDs).Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "查询用户信息失败",
		})
		return
	}

	userMap := make(map[string]database.User, len(users))
	for _, u := range users {
		userMap[u.UserID] = u
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	result, err := svc.BatchSetUserRules(&input, userMap)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "批量设置失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

// GetDingTalkShifts 获取钉钉班次列表
func GetDingTalkShifts(c *gin.Context) {
	type ShiftItem struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
	}

	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}

	catalogs, catalogErr := service.NewShiftConfigService(middleware.RequestDB(c)).ListShiftCatalogs()
	if catalogErr == nil && len(catalogs) > 0 {
		items := make([]ShiftItem, 0, len(catalogs))
		for _, catalog := range catalogs {
			if catalog.ShiftID <= 0 {
				continue
			}
			items = append(items, ShiftItem{ID: catalog.ShiftID, Name: catalog.Name})
		}
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data:    gin.H{"items": items},
		})
		return
	}

	shifts, err := dingtalk.GetShiftListForOrg(orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取班次列表失败: " + err.Error(),
		})
		return
	}

	var items []ShiftItem
	for _, shift := range shifts {
		if idVal, ok := shift["id"].(float64); ok && int64(idVal) > 0 {
			name, _ := shift["name"].(string)
			items = append(items, ShiftItem{
				ID:   int64(idVal),
				Name: name,
			})
		}
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items},
	})
}

// DebugAttendanceGroups 返回所有考勤组及其班次详情，用于诊断休息班次 ID
func DebugAttendanceGroups(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	opUserID, err := dingtalk.ResolveAdminUserID(orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}

	groups, err := dingtalk.GetAttendanceGroupsForOrg(orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取考勤组失败: " + err.Error(),
		})
		return
	}

	shifts, _ := dingtalk.GetShiftListForOrg(orgID)
	shiftNameMap := make(map[int64]string, len(shifts))
	for _, s := range shifts {
		if id, ok := s["id"].(float64); ok && id > 0 {
			name, _ := s["name"].(string)
			shiftNameMap[int64(id)] = name
		}
	}

	type GroupInfo struct {
		GroupID   interface{} `json:"group_id"`
		GroupName interface{} `json:"group_name"`
		GroupType interface{} `json:"group_type"`
		ShiftIDs  []int64     `json:"shift_ids"`
		Shifts    []gin.H     `json:"shifts"`
		RawKeys   []string    `json:"raw_keys"`
	}

	result := make([]GroupInfo, 0, len(groups))
	for _, g := range groups {
		gid, _ := g["group_id"].(float64)
		info := GroupInfo{
			GroupID:   g["group_id"],
			GroupName: g["group_name"],
			GroupType: g["group_type"],
			RawKeys:   make([]string, 0, len(g)),
		}
		for k := range g {
			info.RawKeys = append(info.RawKeys, k)
		}

		detail, detailErr := dingtalk.GetAttendanceGroupForOrg(orgID, opUserID, int64(gid))
		if detailErr == nil {
			shiftIDs := dingtalk.CollectAttendanceGroupShiftIDs(detail)
			info.ShiftIDs = make([]int64, 0, len(shiftIDs))
			info.Shifts = make([]gin.H, 0, len(shiftIDs))
			for sid := range shiftIDs {
				info.ShiftIDs = append(info.ShiftIDs, sid)
				info.Shifts = append(info.Shifts, gin.H{
					"shift_id":   sid,
					"shift_name": shiftNameMap[sid],
				})
			}
			restID := dingtalk.GetAttendanceGroupRestClassIDForOrg(orgID, detail)
			info.RawKeys = append(info.RawKeys, fmt.Sprintf("detected_rest_shift_id=%d", restID))
		}
		result = append(result, info)
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"groups": result, "all_shifts": shifts},
	})
}

// CreateDingTalkShift 在钉钉创建新班次
func CreateDingTalkShift(c *gin.Context) {
	var input struct {
		Name         string `json:"name" binding:"required"`
		CheckInTime  string `json:"check_in_time" binding:"required"`
		CheckOutTime string `json:"check_out_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	opUserID, err := dingtalk.ResolveAdminUserID(orgID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}

	shiftID, err := dingtalk.CreateShiftForOrg(orgID, opUserID, input.Name, input.CheckInTime, input.CheckOutTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"id": shiftID, "name": input.Name},
	})
}
func GetWeekCalendar(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.Query("user_id"))
	departmentID := strings.TrimSpace(c.Query("department_id"))
	weeksStr := c.DefaultQuery("weeks", "8")
	startDate := c.Query("start_date")

	var weeks int
	fmt.Sscanf(weeksStr, "%d", &weeks)
	if weeks <= 0 {
		weeks = 8
	}

	if !currentUserHasAnyPermission(c, "attendance_manage") {
		scope, err := resolveOrgScope(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "获取组织范围失败",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}

		if userID != "" {
			user, ok := ensureCanAccessAttendanceUser(c, userID)
			if !ok {
				return
			}
			departmentID = user.DepartmentID
		} else if departmentID != "" {
			if !scope.IsAll() && !scope.AllowsDepartment(departmentID) {
				respondOrgAccessDenied(c)
				return
			}
		} else if scope.IsSelf() && len(scope.UserIDs) > 0 {
			userID = scope.UserIDs[0]
			if user, err := loadUserByUserID(orgID, userID); err == nil {
				departmentID = user.DepartmentID
			}
		} else if !scope.IsAll() {
			if len(scope.DepartmentIDs) != 1 {
				respondOrgAccessDenied(c)
				return
			}
			departmentID = scope.DepartmentIDs[0]
		}
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	calendar, err := svc.GetWeekCalendar(userID, departmentID, weeks, startDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取日历失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": calendar},
	})
}

// SetWeekOverride 手动设置某周为大周/小周
func SetWeekOverride(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var req weekScheduleOverrideRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	if !rejectClientOrganizationID(c, req.OrgID) {
		return
	}
	override := database.WeekScheduleOverride{
		OrgID: orgID, ScopeType: req.ScopeType, ScopeID: req.ScopeID,
		WeekStartDate: req.WeekStartDate, WeekType: req.WeekType, Reason: req.Reason,
	}
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	if err := svc.SetOverride(&override); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "设置覆盖失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"override": override}})
}

// DeleteWeekOverride 取消手动覆盖
func DeleteWeekOverride(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	if err := svc.DeleteOverride(id); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除覆盖失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// SyncWeekToDingTalk 将大小周配置推送到钉钉（旧接口：写入考勤排班，页面按钮已不再调用）
func SyncWeekToDingTalk(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var input struct {
		Weeks int `json:"weeks"`
	}
	c.ShouldBindJSON(&input)
	if input.Weeks <= 0 {
		input.Weeks = 4
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	result, err := svc.SyncToDingTalk(input.Weeks)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步到钉钉失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

const (
	weekSchedulePersonalPushMaxBytes = 8 << 20 // 8MB
	weekSchedulePersonalPushMaxUsers = 100
)

// PushPersonalWeekSchedule 推送月作息表图片+文字到指定个人（不写考勤排班）
// multipart fields: image (PNG), user_ids (JSON array or repeated), title, content
func PushPersonalWeekSchedule(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}

	file, err := c.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请上传作息表图片（image）",
		})
		return
	}
	if file.Size <= 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "图片文件不能为空",
		})
		return
	}
	if file.Size > weekSchedulePersonalPushMaxBytes {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "图片不能超过 8MB",
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		// Also accept missing extension when content is PNG (frontend canvas often uses .png).
		if ext != "" {
			c.JSON(http.StatusBadRequest, Response{
				Code:    http.StatusBadRequest,
				Message: "仅支持 PNG/JPEG 图片",
			})
			return
		}
		ext = ".png"
	}
	if err := validateUploadContent(file, ext); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "图片校验失败: " + err.Error(),
		})
		return
	}
	if err := validateImageUpload(file); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "图片尺寸无效: " + err.Error(),
		})
		return
	}

	userIDs, err := parseWeekSchedulePushUserIDs(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}
	if len(userIDs) == 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "收件人 user_ids 不能为空",
		})
		return
	}
	if len(userIDs) > weekSchedulePersonalPushMaxUsers {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("收件人不能超过 %d 人", weekSchedulePersonalPushMaxUsers),
		})
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	content := strings.TrimSpace(c.PostForm("content"))

	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "读取图片失败",
		})
		return
	}
	defer func() { _ = src.Close() }()
	imageBytes, err := io.ReadAll(io.LimitReader(src, weekSchedulePersonalPushMaxBytes+1))
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "读取图片失败: " + err.Error(),
		})
		return
	}
	if len(imageBytes) == 0 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "图片文件不能为空",
		})
		return
	}
	if len(imageBytes) > weekSchedulePersonalPushMaxBytes {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "图片不能超过 8MB",
		})
		return
	}

	filename := file.Filename
	if strings.TrimSpace(filename) == "" {
		filename = "week-schedule.png"
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	result, err := svc.PushPersonalScheduleImage(userIDs, title, content, imageBytes, filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "作息表推送失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: result.Message,
		Data:    result,
	})
}

func parseWeekSchedulePushUserIDs(c *gin.Context) ([]string, error) {
	// Prefer JSON array in a single field: user_ids=["a","b"]
	raw := strings.TrimSpace(c.PostForm("user_ids"))
	if raw == "" {
		// Also accept repeated form fields user_ids=a&user_ids=b
		if values := c.PostFormArray("user_ids"); len(values) > 0 {
			out := make([]string, 0, len(values))
			for _, v := range values {
				if id := strings.TrimSpace(v); id != "" {
					out = append(out, id)
				}
			}
			return out, nil
		}
		// Fallback: user_ids[] style
		if values := c.PostFormArray("user_ids[]"); len(values) > 0 {
			out := make([]string, 0, len(values))
			for _, v := range values {
				if id := strings.TrimSpace(v); id != "" {
					out = append(out, id)
				}
			}
			return out, nil
		}
		return nil, nil
	}

	if strings.HasPrefix(raw, "[") {
		var ids []string
		if err := json.Unmarshal([]byte(raw), &ids); err != nil {
			return nil, fmt.Errorf("user_ids 必须是 JSON 字符串数组")
		}
		out := make([]string, 0, len(ids))
		for _, id := range ids {
			if trimmed := strings.TrimSpace(id); trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out, nil
	}

	// Comma-separated fallback
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if id := strings.TrimSpace(p); id != "" {
			out = append(out, id)
		}
	}
	return out, nil
}

// SyncWeekFromDingTalk 从钉钉拉取大小周配置
func SyncWeekFromDingTalk(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	result, err := svc.SyncFromDingTalkConservative()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "从钉钉同步失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

// GetWeekSyncLogs 获取大小周同步日志
func GetWeekSyncLogs(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	pageStr := c.DefaultQuery("page", "1")
	pageSizeStr := c.DefaultQuery("page_size", "20")

	var page, pageSize int
	fmt.Sscanf(pageStr, "%d", &page)
	fmt.Sscanf(pageSizeStr, "%d", &pageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	logs, total, err := svc.GetSyncLogs(page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取同步日志失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: PagedResponse{
			Items: logs,
			Total: total,
		},
	})
}

// ===================== 法定节假日管理 =====================

// GetHolidays 获取节假日列表（按年）
func GetHolidays(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	yearStr := c.DefaultQuery("year", fmt.Sprintf("%d", time.Now().Year()))
	var year int
	fmt.Sscanf(yearStr, "%d", &year)
	if year <= 0 {
		year = time.Now().Year()
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	holidays, err := svc.GetHolidaysByYear(year)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取节假日列表失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": holidays, "year": year},
	})
}

// CreateHoliday 创建单个节假日
func CreateHoliday(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var req statutoryHolidayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	if !rejectClientOrganizationID(c, req.OrgID) {
		return
	}
	holiday := database.StatutoryHoliday{OrgID: orgID, Date: req.Date, Name: req.Name, Type: req.Type, Year: req.Year}
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	if err := svc.CreateHoliday(&holiday); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "创建节假日失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"holiday": holiday}})
}

// BatchCreateHolidays 批量创建节假日
func BatchCreateHolidays(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	var input struct {
		Holidays []statutoryHolidayRequest `json:"holidays" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "参数错误"})
		return
	}
	holidays := make([]database.StatutoryHoliday, 0, len(input.Holidays))
	for _, req := range input.Holidays {
		if !rejectClientOrganizationID(c, req.OrgID) {
			return
		}
		holidays = append(holidays, database.StatutoryHoliday{
			OrgID: orgID, Date: req.Date, Name: req.Name, Type: req.Type, Year: req.Year,
		})
	}
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	created, err := svc.BatchCreateHolidays(holidays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "批量创建节假日失败: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "success", Data: gin.H{"created": created, "total": len(holidays)}})
}

// SyncHolidaysFromJuhe 从聚合数据API同步节假日
func SyncHolidaysFromJuhe(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	created, err := svc.SyncHolidaysFromJuhe()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "从聚合数据同步节假日失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"created": created},
	})
}

// DeleteHoliday 删除节假日
func DeleteHoliday(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleServiceWithOrgID(middleware.RequestDB(c), orgID)
	if err := svc.DeleteHoliday(id); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除节假日失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// ===================== 员工下班时间配置 =====================

// GetShiftConfigs 获取所有员工的下班时间配置（含默认 18:30 的员工）
func GetShiftConfigs(c *gin.Context) {
	svc := service.NewShiftConfigService(middleware.RequestDB(c))
	items, err := svc.GetAllWithUsers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取配置失败: " + err.Error(),
		})
		return
	}
	if !currentUserHasAnyPermission(c, "attendance_manage") {
		scope, err := resolveOrgScope(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "获取组织范围失败",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		filtered := make([]service.EmployeeShiftItem, 0, len(items))
		allowedUsers := make(map[string]struct{}, len(scope.UserIDs))
		if scope.IsSelf() {
			for _, userID := range scope.UserIDs {
				userID = strings.TrimSpace(userID)
				if userID != "" {
					allowedUsers[userID] = struct{}{}
				}
			}
		}
		for _, item := range items {
			if scope.IsSelf() {
				if _, ok := allowedUsers[item.UserID]; ok {
					filtered = append(filtered, item)
				}
				continue
			}
			if scope.IsAll() || scope.AllowsDepartment(item.DepartmentID) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items},
	})
}

// SetShiftConfigs 批量/单个设置员工下班时间（仅写本地 DB，不调用钉钉 API）
func GetShiftCatalogs(c *gin.Context) {
	svc := service.NewShiftConfigService(middleware.RequestDB(c))
	items, err := svc.ListShiftCatalogs()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "????????????: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"items": items},
	})
}

func PreviewShiftConfigs(c *gin.Context) {
	var input service.PreviewShiftConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(middleware.RequestDB(c))
	result, err := svc.Preview(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "预览失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    result,
	})
}

func SetShiftConfigs(c *gin.Context) {
	var input service.SetShiftConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(middleware.RequestDB(c))
	count, err := svc.SetConfigs(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "设置失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"updated": count},
	})
}

// DeleteShiftConfig 删除员工自定义下班时间（恢复默认 18:30）
func ApplyShiftConfigs(c *gin.Context) {
	var input service.ApplyShiftConfigInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "??????: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(middleware.RequestDB(c))
	result, err := svc.ApplyAndSync(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "???????????: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: result.Message,
		Data:    result,
	})
}

func DeleteShiftConfig(c *gin.Context) {
	userID := c.Param("user_id")
	svc := service.NewShiftConfigService(middleware.RequestDB(c))
	if err := svc.DeleteConfig(userID); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "删除失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
	})
}

// GetOrCreateCustomShift 查找或创建钉钉班次，返回班次 ID
func GetOrCreateCustomShift(c *gin.Context) {
	var input struct {
		Name     string `json:"name" binding:"required"`
		CheckIn  string `json:"check_in" binding:"required"`
		CheckOut string `json:"check_out" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误: " + err.Error(),
		})
		return
	}

	svc := service.NewShiftConfigService(middleware.RequestDB(c))
	shiftID, err := svc.GetOrCreateShift(input.Name, input.CheckIn, input.CheckOut)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取/创建班次失败: " + err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"shift_id": shiftID},
	})
}

// UploadFile 文件上传：按 JWT org_id 建立所有权，磁盘写入 uploads/<safe_org_id>/<stored_name>，
// 对外返回 /api/v1/files/<id>。禁止客户端传 org_id 决定归属。
func UploadFile(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}
	userID := strings.TrimSpace(c.GetString("userID"))
	if userID == "" {
		c.JSON(http.StatusUnauthorized, Response{
			Code:    http.StatusUnauthorized,
			Message: "未登录或会话已失效",
		})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请选择要上传的文件",
		})
		return
	}

	// 限制文件大小 (10MB)
	if file.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "文件大小不能超过10MB",
		})
		return
	}

	// 文件类型白名单
	ext := strings.ToLower(filepath.Ext(file.Filename))
	if !isAllowedUploadExtension(ext) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: fmt.Sprintf("不支持的文件类型，允许: %s", allowedUploadExtensionText()),
		})
		return
	}

	if err := validateUploadContent(file, ext); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "文件内容与扩展名不匹配或不受支持",
		})
		return
	}
	if err := scanUploadForThreats(file); err != nil {
		log.Printf("[upload-security] scan failed filename=%s size=%d: %v", file.Filename, file.Size, err)
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "文件安全扫描未通过",
		})
		return
	}

	// 生成随机唯一文件名（避免时间戳可预测）
	randBytes := make([]byte, 16)
	if _, err := rand.Read(randBytes); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "生成文件名失败",
		})
		return
	}
	storedName := fmt.Sprintf("%s%s", hex.EncodeToString(randBytes), ext)
	filePath, err := uploadedFileDiskPath(orgID, storedName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建上传路径失败",
		})
		return
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0750); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建上传目录失败",
		})
		return
	}

	// 保存文件
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "保存文件失败",
		})
		return
	}

	contentType := detectUploadContentType(file, ext)
	meta := &database.UploadedFile{
		OrgID:          orgID,
		UploaderUserID: userID,
		StoredName:     storedName,
		OriginalName:   filepath.Base(file.Filename),
		ContentType:    contentType,
		Size:           file.Size,
	}

	db := middleware.RequestDB(c)
	repo, err := repository.NewUploadedFileRepositoryWithOrgID(db, orgID)
	if err != nil {
		_ = os.Remove(filePath)
		respondMissingOrgContext(c)
		return
	}
	if err := repo.Create(meta); err != nil {
		_ = os.Remove(filePath)
		log.Printf("[upload] persist metadata failed org=%s user=%s: %v", orgID, userID, err)
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "保存文件元数据失败",
		})
		return
	}

	fileURL := fmt.Sprintf("/api/v1/files/%d", meta.ID)
	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "上传成功",
		Data: gin.H{
			"id":           meta.ID,
			"url":          fileURL,
			"name":         meta.OriginalName,
			"size":         meta.Size,
			"content_type": meta.ContentType,
			"stored_name":  meta.StoredName,
		},
	})
}

// ServeFile 提供文件访问：必须经 JWTAuth + TenantContext。
// 使用当前 JWT org_id 查询元数据；当前组织没有该文件时统一 404，不暴露是否属于其他企业。
// 旧版 /api/v1/files/<random_filename> 无元数据时 fail-closed。
func ServeFile(c *gin.Context) {
	orgID, ok := currentOrgIDOrAbort(c)
	if !ok {
		return
	}

	rawID := strings.TrimSpace(c.Param("file_id"))
	if rawID == "" {
		// Backward-compatible param name from pre-isolation route.
		rawID = strings.TrimSpace(c.Param("filename"))
	}
	if rawID == "" {
		respondUploadedFileNotFound(c)
		return
	}

	// Path traversal / injection via param: reject anything that is not a positive integer id.
	// Legacy filename URLs are intentionally rejected (fail-closed without ownership metadata).
	fileID, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || fileID == 0 {
		// Distinguish clearly unsafe path segments (../ etc.) still as not-found / bad-request without leaking.
		if strings.Contains(rawID, "..") || strings.ContainsAny(rawID, `/\`) {
			c.JSON(http.StatusBadRequest, Response{
				Code:    http.StatusBadRequest,
				Message: "无效的文件标识",
			})
			return
		}
		// Old random-filename style IDs: refuse without org ownership proof.
		respondUploadedFileNotFound(c)
		return
	}

	db := middleware.RequestDB(c)
	repo, err := repository.NewUploadedFileRepositoryWithOrgID(db, orgID)
	if err != nil {
		respondMissingOrgContext(c)
		return
	}
	meta, err := repo.FindByID(uint(fileID))
	if err != nil {
		// Record missing OR belongs to another org (scoped query) → same 404.
		respondUploadedFileNotFound(c)
		return
	}

	filePath, err := uploadedFileDiskPath(meta.OrgID, meta.StoredName)
	if err != nil {
		respondUploadedFileNotFound(c)
		return
	}
	if _, err := os.Stat(filePath); err != nil {
		respondUploadedFileNotFound(c)
		return
	}

	ext := strings.ToLower(filepath.Ext(meta.StoredName))
	disposition := "attachment"
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" ||
		ext == ".pdf" || ext == ".txt" || ext == ".csv" || ext == ".md" {
		disposition = "inline"
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Content-Disposition", fmt.Sprintf("%s; %s", disposition, contentDispositionFilename(meta.OriginalName, meta.StoredName)))
	if strings.TrimSpace(meta.ContentType) != "" {
		c.Header("Content-Type", meta.ContentType)
	}
	c.File(filePath)
}
