package dingtalk

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"peopleops/internal/database"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

var (
	appKey             string
	appSecret          string
	corpID             string
	tokenMu            sync.Mutex
	tokenByOrg         = make(map[string]accessTokenCacheEntry)
	dingTalkHTTPClient = &http.Client{Timeout: 30 * time.Second}
)

const (
	ErrorCodeConfigMissing        = "DINGTALK_CONFIG_MISSING"
	ErrorCodeTokenFailed          = "DINGTALK_TOKEN_FAILED"
	ErrorCodePermissionDenied     = "DINGTALK_PERMISSION_DENIED"
	ErrorCodeNetworkFailed        = "DINGTALK_NETWORK_FAILED"
	ErrorCodeResponseInvalid      = "DINGTALK_RESPONSE_INVALID"
	ErrorCodeDepartmentEmpty      = "DINGTALK_DEPARTMENT_EMPTY"
	ErrorCodeUserSourceIncomplete = "DINGTALK_USER_SOURCE_INCOMPLETE"
)

type SyncError struct {
	Code        string
	SafeMessage string
	detail      string
}

func (e *SyncError) Error() string {
	if e == nil {
		return ""
	}
	if e.detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.detail
}

func SyncErrorCode(err error) string {
	var syncErr *SyncError
	if errors.As(err, &syncErr) {
		return syncErr.Code
	}
	return ""
}

func SyncErrorSafeMessage(err error) string {
	var syncErr *SyncError
	if errors.As(err, &syncErr) {
		return syncErr.SafeMessage
	}
	return ""
}

var (
	dingTalkURLSecretPattern = regexp.MustCompile(`(?i)([?&](?:access_token|token|secret|password|app_?key|app_?secret|authorization|cookie|session_?webhook|webhook)=)[^&\s]+`)
	dingTalkKeyValuePattern  = regexp.MustCompile(`(?i)["']?\b(access[_-]?token|token|secret|password|authorization|cookie|app[_-]?key|app[_-]?secret|session[_-]?webhook|webhook|robot[_-]?code|dsn|database[_-]?url|db[_-]?password)\b["']?(?:\s*[:=]\s*|\s+)("[^"]*"|'[^']*'|[^\s,;]+)`)
	dingTalkBearerPattern    = regexp.MustCompile(`(?i)\bbearer\s+[^\s,;]+`)
	dingTalkDSNPattern       = regexp.MustCompile(`(?i)[^\s:]+:[^@\s]+@(?:tcp\([^)]*\)|[^/\s]+)/[^\s]+`)
)

func sanitizeDingTalkDiagnostic(value string) string {
	value = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, value)
	value = strings.Join(strings.Fields(value), " ")
	value = dingTalkURLSecretPattern.ReplaceAllString(value, `${1}[REDACTED]`)
	value = dingTalkKeyValuePattern.ReplaceAllString(value, `${1}=[REDACTED]`)
	value = dingTalkBearerPattern.ReplaceAllString(value, `Bearer [REDACTED]`)
	value = dingTalkDSNPattern.ReplaceAllString(value, `[DSN_REDACTED]`)
	if len(value) > 512 {
		value = value[:512] + "..."
	}
	return value
}

func newSyncError(code, safeMessage string, cause error) error {
	detail := ""
	if cause != nil {
		detail = sanitizeDingTalkDiagnostic(cause.Error())
	}
	return &SyncError{Code: code, SafeMessage: safeMessage, detail: detail}
}

func safeDingTalkErrorForLog(err error) string {
	if err == nil {
		return ""
	}
	return sanitizeDingTalkDiagnostic(err.Error())
}

type AppConfig struct {
	OrgID           string                       `json:"org_id"`
	Name            string                       `json:"name"`
	CorpID          string                       `json:"corp_id"`
	AppKey          string                       `json:"app_key"`
	AppSecret       string                       `json:"app_secret"`
	AgentID         string                       `json:"agent_id"`
	AdminUserID     string                       `json:"-"`
	RobotCode       string                       `json:"-"`
	AppHomeURL      string                       `json:"app_home_url"`
	RedirectURI     string                       `json:"redirect_uri"`
	Status          string                       `json:"status"`
	HRMFieldCodes   map[string][]string          `json:"-"`
	HRMFieldNames   map[string][]string          `json:"-"`
	HRMFieldOptions map[string]map[string]string `json:"-"`
}

var ErrUserNotNotifiable = errors.New("dingtalk user is not active/notifiable")

type Config struct {
	OrgID           string
	AppKey          string
	AppSecret       string
	CorpID          string
	AgentID         string
	AdminUserID     string
	RobotCode       string
	AppHomeURL      string
	RedirectURI     string
	ProcessCodes    map[string]string
	HRMFieldCodes   map[string][]string
	HRMFieldNames   map[string][]string
	HRMFieldOptions map[string]map[string]string
}

type accessTokenCacheEntry struct {
	token  string
	expiry time.Time
}

type userAccessTokenInfo struct {
	AccessToken string
	Raw         map[string]interface{}
}

func IsUserNotNotifiableError(err error) bool {
	return errors.Is(err, ErrUserNotNotifiable)
}

// 考勤组缓存（5分钟 TTL）
type attendanceGroupCache struct {
	data   []map[string]interface{}
	expiry time.Time
}

type attendanceGroupDetailCache struct {
	data   map[string]interface{}
	expiry time.Time
}

type shiftListCache struct {
	key    string
	data   []map[string]interface{}
	expiry time.Time
}

var (
	attGroupsCache    attendanceGroupCache
	attGroupsCacheMu  sync.Mutex
	attGroupDetailMap sync.Map // key: groupID(int64) → attendanceGroupDetailCache
	shiftCache        shiftListCache
	shiftCacheMu      sync.Mutex
)

func Init() error {
	appKey = os.Getenv("DINGTALK_APP_KEY")
	appSecret = os.Getenv("DINGTALK_APP_SECRET")
	corpID = os.Getenv("DINGTALK_CORP_ID")

	if appKey == "" || appSecret == "" {
		return fmt.Errorf("缂哄皯 DINGTALK_APP_KEY 鎴?DINGTALK_APP_SECRET")
	}

	logrus.Info("閽夐拤瀹㈡埛绔垵濮嬪寲瀹屾垚")
	return nil
}

// GetCorpID 杩斿洖浼佷笟 CorpId锛屼緵鍓嶇 JS-SDK 浣跨敤
func DefaultConfig() Config {
	return Config{
		OrgID:         database.DefaultOrganizationID,
		AppKey:        firstNonEmpty(appKey, os.Getenv("DINGTALK_APP_KEY")),
		AppSecret:     firstNonEmpty(appSecret, os.Getenv("DINGTALK_APP_SECRET")),
		CorpID:        firstNonEmpty(corpID, os.Getenv("DINGTALK_CORP_ID")),
		AgentID:       os.Getenv("DINGTALK_AGENT_ID"),
		AdminUserID:   os.Getenv("DINGTALK_ADMIN_USER_ID"),
		RobotCode:     firstNonEmpty(os.Getenv("DINGTALK_ROBOT_CODE"), os.Getenv("DINGTALK_APP_KEY")),
		AppHomeURL:    firstNonEmpty(os.Getenv("DINGTALK_APP_HOME_URL"), os.Getenv("APP_BASE_URL"), os.Getenv("FRONTEND_BASE_URL")),
		RedirectURI:   os.Getenv("DINGTALK_REDIRECT_URI"),
		ProcessCodes:  processCodesFromEnv(),
		HRMFieldCodes: hrmFieldCodesFromEnv(),
		HRMFieldNames: hrmFieldNamesFromEnv(),
	}
}

func ConfigForOrgID(orgID string) (Config, error) {
	// Fail closed: empty org must not silently become "default" and reuse env credentials.
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return Config{}, newSyncError(ErrorCodeConfigMissing, "钉钉组织配置缺失", errors.New("orgID is empty"))
	}
	orgID = database.NormalizeOrganizationID(orgID)
	if database.DB != nil {
		var org database.Organization
		err := database.DB.Where("org_id = ? AND status = ? AND deleted_at IS NULL", orgID, "active").First(&org).Error
		if err == nil {
			return ConfigFromOrganization(org), nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return Config{}, err
		}
	}
	if orgID == database.DefaultOrganizationID {
		return DefaultConfig(), nil
	}
	return Config{}, newSyncError(ErrorCodeConfigMissing, "钉钉组织配置缺失", fmt.Errorf("dingtalk organization %s not configured", orgID))
}

func ConfigFromOrganization(org database.Organization) Config {
	cfg := Config{
		OrgID:           database.NormalizeOrganizationID(org.OrgID),
		AppKey:          strings.TrimSpace(org.DingTalkAppKey),
		AppSecret:       strings.TrimSpace(org.DingTalkSecret),
		CorpID:          strings.TrimSpace(org.CorpID),
		AgentID:         strings.TrimSpace(org.DingTalkAgentID),
		AdminUserID:     strings.TrimSpace(org.DingTalkAdminUserID),
		RobotCode:       firstNonEmpty(organizationExtensionString(org.Extension, "dingtalk_robot_code"), org.DingTalkAppKey),
		AppHomeURL:      strings.TrimRight(strings.TrimSpace(org.AppHomeURL), "/"),
		RedirectURI:     strings.TrimSpace(org.RedirectURI),
		ProcessCodes:    processCodesFromOrganizationExtension(org.Extension),
		HRMFieldCodes:   hrmFieldMapFromOrganizationExtension(org.Extension, "dingtalk_hrm_field_codes"),
		HRMFieldNames:   hrmFieldMapFromOrganizationExtension(org.Extension, "dingtalk_hrm_field_names"),
		HRMFieldOptions: hrmFieldOptionsFromOrganizationExtension(org.Extension),
	}
	// default org: env fallback for admin is encapsulated here.
	if cfg.OrgID == database.DefaultOrganizationID && cfg.AdminUserID == "" {
		cfg.AdminUserID = strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	}
	if cfg.OrgID == database.DefaultOrganizationID {
		cfg.HRMFieldCodes = mergeHRMFieldMaps(cfg.HRMFieldCodes, hrmFieldCodesFromEnv())
		cfg.HRMFieldNames = mergeHRMFieldMaps(cfg.HRMFieldNames, hrmFieldNamesFromEnv())
	}
	return cfg
}

func (cfg Config) normalized() Config {
	cfg.OrgID = database.NormalizeOrganizationID(cfg.OrgID)
	cfg.AppKey = strings.TrimSpace(cfg.AppKey)
	cfg.AppSecret = strings.TrimSpace(cfg.AppSecret)
	cfg.CorpID = strings.TrimSpace(cfg.CorpID)
	cfg.AgentID = strings.TrimSpace(cfg.AgentID)
	cfg.AdminUserID = strings.TrimSpace(cfg.AdminUserID)
	cfg.RobotCode = strings.TrimSpace(cfg.RobotCode)
	cfg.AppHomeURL = strings.TrimRight(strings.TrimSpace(cfg.AppHomeURL), "/")
	cfg.RedirectURI = strings.TrimSpace(cfg.RedirectURI)
	cfg.ProcessCodes = normalizeProcessCodes(cfg.ProcessCodes)
	cfg.HRMFieldCodes = normalizeHRMFieldMap(cfg.HRMFieldCodes)
	cfg.HRMFieldNames = normalizeHRMFieldMap(cfg.HRMFieldNames)
	cfg.HRMFieldOptions = normalizeHRMFieldOptions(cfg.HRMFieldOptions)
	return cfg
}

func (cfg Config) NormalizedForAPI() Config {
	return cfg.normalized()
}

func (cfg Config) cacheKey() string {
	cfg = cfg.normalized()
	return cfg.OrgID + "|" + cfg.AppKey
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func GetCorpID() string {
	return corpID
}

func InitWithConfig(cfg AppConfig) error {
	appKey = strings.TrimSpace(cfg.AppKey)
	appSecret = strings.TrimSpace(cfg.AppSecret)
	corpID = strings.TrimSpace(cfg.CorpID)

	if appKey == "" || appSecret == "" {
		return fmt.Errorf("missing DingTalk app config for org_id=%s", strings.TrimSpace(cfg.OrgID))
	}

	logrus.Infof("initialized DingTalk app config for org_id=%s", strings.TrimSpace(cfg.OrgID))
	return nil
}

func configFromAppConfig(cfg AppConfig) Config {
	result := Config{
		OrgID:           cfg.OrgID,
		AppKey:          cfg.AppKey,
		AppSecret:       cfg.AppSecret,
		CorpID:          cfg.CorpID,
		AgentID:         cfg.AgentID,
		AdminUserID:     cfg.AdminUserID,
		RobotCode:       cfg.RobotCode,
		AppHomeURL:      cfg.AppHomeURL,
		RedirectURI:     cfg.RedirectURI,
		HRMFieldCodes:   cfg.HRMFieldCodes,
		HRMFieldNames:   cfg.HRMFieldNames,
		HRMFieldOptions: cfg.HRMFieldOptions,
	}.normalized()
	if result.OrgID == database.DefaultOrganizationID {
		result.HRMFieldCodes = mergeHRMFieldMaps(result.HRMFieldCodes, hrmFieldCodesFromEnv())
		result.HRMFieldNames = mergeHRMFieldMaps(result.HRMFieldNames, hrmFieldNamesFromEnv())
	}
	return result.normalized()
}

func appConfigFromConfig(cfg Config) AppConfig {
	cfg = cfg.normalized()
	return AppConfig{
		OrgID:           cfg.OrgID,
		CorpID:          cfg.CorpID,
		AppKey:          cfg.AppKey,
		AppSecret:       cfg.AppSecret,
		AgentID:         cfg.AgentID,
		AdminUserID:     cfg.AdminUserID,
		RobotCode:       cfg.RobotCode,
		AppHomeURL:      cfg.AppHomeURL,
		RedirectURI:     cfg.RedirectURI,
		Status:          "active",
		HRMFieldCodes:   cfg.HRMFieldCodes,
		HRMFieldNames:   cfg.HRMFieldNames,
		HRMFieldOptions: cfg.HRMFieldOptions,
	}
}

func DefaultAppConfig() AppConfig {
	cfg := appConfigFromConfig(DefaultConfig())
	cfg.Name = strings.TrimSpace(os.Getenv("DINGTALK_ORG_NAME"))
	return cfg
}

func ActiveAppConfigs() []AppConfig {
	configs, err := ListActiveOrganizationConfigs()
	if err != nil {
		logrus.Warnf("list active DingTalk organization configs failed: %s", safeDingTalkErrorForLog(err))
		return []AppConfig{DefaultAppConfig()}
	}
	result := make([]AppConfig, 0, len(configs))
	for _, cfg := range configs {
		result = append(result, appConfigFromConfig(cfg))
	}
	return result
}

func GetAppConfigForOrg(orgID string) (AppConfig, bool) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return AppConfig{}, false
	}
	return appConfigFromConfig(cfg), true
}

func GetCorpIDForOrg(orgID string) string {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return ""
	}
	return cfg.normalized().CorpID
}

func GetConfiguredAppHomeURL() string {
	if homeURL := strings.TrimSpace(os.Getenv("DINGTALK_APP_HOME_URL")); homeURL != "" {
		return strings.TrimRight(homeURL, "/")
	}
	if baseURL := strings.TrimSpace(os.Getenv("APP_BASE_URL")); baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}
	if baseURL := strings.TrimSpace(os.Getenv("FRONTEND_BASE_URL")); baseURL != "" {
		return strings.TrimRight(baseURL, "/")
	}

	return ""
}

func GetConfiguredAppHomeURLForOrg(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return ""
	}
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return ""
	}
	cfg = cfg.normalized()
	if homeURL := cfg.AppHomeURL; homeURL != "" {
		return homeURL
	}
	// Only default org may fall back to process-wide env home.
	if cfg.OrgID == database.DefaultOrganizationID {
		return GetConfiguredAppHomeURL()
	}
	return ""
}

func GetAppHomeURL() string {
	if configured := GetConfiguredAppHomeURL(); configured != "" {
		return configured
	}

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8080"
	}

	return fmt.Sprintf("http://localhost:%s", port)
}

func BuildAppURL(appPath string) string {
	baseURL := strings.TrimRight(GetAppHomeURL(), "/")
	appPath = strings.TrimSpace(appPath)
	if appPath == "" {
		return baseURL
	}
	if strings.HasPrefix(appPath, "http://") || strings.HasPrefix(appPath, "https://") {
		return appPath
	}
	return baseURL + "/" + strings.TrimLeft(appPath, "/")
}

// BuildAppURLForOrg builds an absolute app URL for a specific organization home.
// Empty orgID fails closed. Non-default orgs without configured home return "".
func BuildAppURLForOrg(orgID, appPath string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return ""
	}
	appPath = strings.TrimSpace(appPath)
	if strings.HasPrefix(appPath, "http://") || strings.HasPrefix(appPath, "https://") {
		return appPath
	}
	baseURL := strings.TrimRight(GetConfiguredAppHomeURLForOrg(orgID), "/")
	if baseURL == "" {
		if orgID == database.DefaultOrganizationID {
			return BuildAppURL(appPath)
		}
		return ""
	}
	if appPath == "" {
		return baseURL
	}
	return baseURL + "/" + strings.TrimLeft(appPath, "/")
}

// ResolveAdminUserID returns the enterprise op_user_id for org-scoped DingTalk writes.
// Non-default orgs never fall back to DINGTALK_ADMIN_USER_ID.
func ResolveAdminUserID(orgID string) (string, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return "", err
	}
	return ResolveAdminUserIDFromConfig(cfg)
}

// ResolveAdminUserIDFromConfig returns op_user_id from a resolved Config or a clear error.
func ResolveAdminUserIDFromConfig(cfg Config) (string, error) {
	if strings.TrimSpace(cfg.OrgID) == "" {
		return "", fmt.Errorf("organization is required for dingtalk admin resolution")
	}
	cfg = cfg.normalized()
	if cfg.AdminUserID != "" {
		return cfg.AdminUserID, nil
	}
	if cfg.OrgID == database.DefaultOrganizationID {
		if envID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID")); envID != "" {
			return envID, nil
		}
	}
	return "", fmt.Errorf("dingtalk admin user id not configured for org %s", cfg.OrgID)
}

func processCodesFromEnv() map[string]string {
	return normalizeProcessCodes(map[string]string{
		"leave":                 os.Getenv("DINGTALK_PROCESS_LEAVE"),
		"overtime":              os.Getenv("DINGTALK_PROCESS_OVERTIME"),
		"attendance_correction": os.Getenv("DINGTALK_PROCESS_ATTENDANCE_CORRECTION"),
		"position_transfer":     os.Getenv("DINGTALK_PROCESS_POSITION_TRANSFER"),
	})
}

func processCodesFromOrganizationExtension(extension map[string]interface{}) map[string]string {
	if extension == nil {
		return nil
	}
	raw, ok := extension["dingtalk_process_codes"]
	if !ok || raw == nil {
		return nil
	}
	codes := map[string]string{}
	switch values := raw.(type) {
	case map[string]interface{}:
		for key, value := range values {
			codes[key] = fmt.Sprint(value)
		}
	case map[string]string:
		for key, value := range values {
			codes[key] = value
		}
	}
	return normalizeProcessCodes(codes)
}

func normalizeProcessCodes(codes map[string]string) map[string]string {
	if len(codes) == 0 {
		return nil
	}
	normalized := make(map[string]string, len(codes))
	for key, value := range codes {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			normalized[key] = value
		}
	}
	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

const (
	hrmFieldPosition         = "position"
	hrmFieldEmploymentType   = "employment_type"
	hrmFieldJobLevel         = "job_level"
	hrmFieldJobFamily        = "job_family"
	hrmFieldProbationEndDate = "probation_end_date"
)

// HRMFieldSyncStatus values written to UserInfo.HRMFieldSyncStatus.
const (
	HRMFieldSyncStatusSuccess  = "success"
	HRMFieldSyncStatusFailed   = "failed"
	HRMFieldSyncStatusNoFields = "success_no_fields"
)

var supportedHRMFieldKeys = []string{
	hrmFieldPosition,
	hrmFieldEmploymentType,
	hrmFieldJobLevel,
	hrmFieldJobFamily,
	hrmFieldProbationEndDate,
}

func hrmFieldCodesFromEnv() map[string][]string {
	return normalizeHRMFieldMap(map[string][]string{
		hrmFieldPosition:         splitConfiguredList(os.Getenv("DINGTALK_HRM_POSITION_FIELD_CODES")),
		hrmFieldEmploymentType:   splitConfiguredList(os.Getenv("DINGTALK_HRM_EMPLOYMENT_TYPE_FIELD_CODES")),
		hrmFieldJobLevel:         splitConfiguredList(os.Getenv("DINGTALK_HRM_JOB_LEVEL_FIELD_CODES")),
		hrmFieldJobFamily:        splitConfiguredList(os.Getenv("DINGTALK_HRM_JOB_FAMILY_FIELD_CODES")),
		hrmFieldProbationEndDate: splitConfiguredList(os.Getenv("DINGTALK_HRM_PROBATION_END_DATE_FIELD_CODES")),
	})
}

func hrmFieldNamesFromEnv() map[string][]string {
	return normalizeHRMFieldMap(map[string][]string{
		hrmFieldPosition:         splitConfiguredList(os.Getenv("DINGTALK_HRM_POSITION_FIELD_NAMES")),
		hrmFieldEmploymentType:   splitConfiguredList(os.Getenv("DINGTALK_HRM_EMPLOYMENT_TYPE_FIELD_NAMES")),
		hrmFieldJobLevel:         splitConfiguredList(os.Getenv("DINGTALK_HRM_JOB_LEVEL_FIELD_NAMES")),
		hrmFieldJobFamily:        splitConfiguredList(os.Getenv("DINGTALK_HRM_JOB_FAMILY_FIELD_NAMES")),
		hrmFieldProbationEndDate: splitConfiguredList(os.Getenv("DINGTALK_HRM_PROBATION_END_DATE_FIELD_NAMES")),
	})
}

func organizationExtensionString(extension map[string]interface{}, key string) string {
	if extension == nil {
		return ""
	}
	value, ok := extension[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	default:
		return ""
	}
}

func hrmFieldMapFromOrganizationExtension(extension map[string]interface{}, extensionKey string) map[string][]string {
	if extension == nil {
		return nil
	}
	raw, ok := extension[extensionKey]
	if !ok || raw == nil {
		return nil
	}
	result := map[string][]string{}
	switch values := raw.(type) {
	case map[string]interface{}:
		for key, value := range values {
			result[key] = stringListFromConfigValue(value)
		}
	case map[string][]string:
		for key, value := range values {
			result[key] = value
		}
	case map[string]string:
		for key, value := range values {
			result[key] = splitConfiguredList(value)
		}
	}
	return normalizeHRMFieldMap(result)
}

func hrmFieldOptionsFromOrganizationExtension(extension map[string]interface{}) map[string]map[string]string {
	if extension == nil {
		return nil
	}
	raw, ok := extension["dingtalk_hrm_field_options"]
	if !ok || raw == nil {
		return nil
	}
	result := map[string]map[string]string{}
	fields, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	for fieldKey, rawOptions := range fields {
		options, ok := rawOptions.(map[string]interface{})
		if !ok {
			continue
		}
		for code, rawLabel := range options {
			label, ok := rawLabel.(string)
			if !ok {
				continue
			}
			if result[fieldKey] == nil {
				result[fieldKey] = map[string]string{}
			}
			result[fieldKey][code] = label
		}
	}
	return normalizeHRMFieldOptions(result)
}

func normalizeHRMFieldOptions(values map[string]map[string]string) map[string]map[string]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string]map[string]string)
	for _, fieldKey := range supportedHRMFieldKeys {
		for code, label := range values[fieldKey] {
			code = strings.TrimSpace(code)
			label = strings.TrimSpace(label)
			if code == "" || label == "" {
				continue
			}
			if result[fieldKey] == nil {
				result[fieldKey] = make(map[string]string)
			}
			result[fieldKey][code] = label
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func stringListFromConfigValue(value interface{}) []string {
	switch typed := value.(type) {
	case string:
		return splitConfiguredList(typed)
	case []string:
		return uniqueNonEmptyStrings(typed)
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				result = append(result, text)
			}
		}
		return uniqueNonEmptyStrings(result)
	default:
		return nil
	}
}

func normalizeHRMFieldMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	result := make(map[string][]string, len(values))
	for _, key := range supportedHRMFieldKeys {
		items := uniqueNonEmptyStrings(values[key])
		if len(items) > 0 {
			result[key] = items
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func mergeHRMFieldMaps(primary, fallback map[string][]string) map[string][]string {
	result := make(map[string][]string, len(primary)+len(fallback))
	for _, key := range supportedHRMFieldKeys {
		result[key] = uniqueNonEmptyStrings(append(append([]string{}, primary[key]...), fallback[key]...))
	}
	return normalizeHRMFieldMap(result)
}

func buildCallbackURL(appHomeURL string) string {
	appHomeURL = strings.TrimRight(strings.TrimSpace(appHomeURL), "/")
	if appHomeURL == "" {
		return ""
	}
	return appHomeURL + "/callback"
}

func normalizeConfiguredRedirectURI(redirectURI string) string {
	redirectURI = strings.TrimSpace(redirectURI)
	if redirectURI == "" {
		return ""
	}

	parsed, err := url.Parse(redirectURI)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return redirectURI
	}

	if parsed.Path == "" || parsed.Path == "/" {
		parsed.Path = "/callback"
		parsed.RawPath = ""
		return parsed.String()
	}

	return redirectURI
}

func GetConfiguredRedirectURI() string {
	if redirectURI := strings.TrimSpace(os.Getenv("DINGTALK_REDIRECT_URI")); redirectURI != "" {
		return normalizeConfiguredRedirectURI(redirectURI)
	}

	return ""
}

func GetConfiguredRedirectURIForOrg(orgID string) string {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return ""
	}
	if redirectURI := cfg.normalized().RedirectURI; redirectURI != "" {
		return normalizeConfiguredRedirectURI(redirectURI)
	}
	return GetConfiguredRedirectURI()
}

func GetRedirectURI() string {
	if configured := GetConfiguredRedirectURI(); configured != "" {
		return configured
	}

	return buildCallbackURL(GetAppHomeURL())
}

func ListActiveOrganizationConfigs() ([]Config, error) {
	if database.DB == nil {
		return []Config{DefaultConfig().normalized()}, nil
	}

	var orgs []database.Organization
	if err := database.DB.
		Where("status = ? AND deleted_at IS NULL", "active").
		Order("org_id ASC").
		Find(&orgs).Error; err != nil {
		return nil, err
	}

	if len(orgs) == 0 {
		return []Config{DefaultConfig().normalized()}, nil
	}

	configs := make([]Config, 0, len(orgs))
	for _, org := range orgs {
		configs = append(configs, ConfigFromOrganization(org).normalized())
	}

	return configs, nil
}

func sharedOAuthOrgID() string {
	return strings.TrimSpace(os.Getenv("DINGTALK_SHARED_OAUTH_ORG_ID"))
}

func defaultOAuthLoginConfig() (Config, bool) {
	cfg := DefaultConfig().normalized()
	if cfg.AppKey == "" || cfg.AppSecret == "" {
		return Config{}, false
	}
	return cfg, true
}

func sharedOAuthLoginConfigFromConfigs(configs []Config) (Config, error) {
	if sharedOrgID := sharedOAuthOrgID(); sharedOrgID != "" && sharedOrgID != database.DefaultOrganizationID {
		for _, cfg := range configs {
			cfg = cfg.normalized()
			if cfg.OrgID == sharedOrgID {
				if cfg.AppKey == "" || cfg.AppSecret == "" {
					return Config{}, fmt.Errorf("shared dingtalk oauth org %s is missing app credentials", sharedOrgID)
				}
				return cfg, nil
			}
		}
		return Config{}, fmt.Errorf("shared dingtalk oauth org %s not found in active organizations", sharedOrgID)
	}

	if cfg, ok := defaultOAuthLoginConfig(); ok {
		return cfg, nil
	}

	if len(configs) == 1 {
		cfg := configs[0].normalized()
		if cfg.AppKey == "" || cfg.AppSecret == "" {
			return Config{}, fmt.Errorf("active organization %s is missing dingtalk oauth app credentials", cfg.OrgID)
		}
		return cfg, nil
	}

	if len(configs) > 1 {
		shared := configs[0].normalized()
		if shared.AppKey != "" && shared.AppSecret != "" {
			allShared := true
			for _, cfg := range configs[1:] {
				if !oauthConfigsShareCredentials(shared, cfg) {
					allShared = false
					break
				}
			}
			if allShared {
				return shared, nil
			}
		}
	}

	return Config{}, fmt.Errorf("shared dingtalk oauth login config is required; set DINGTALK_APP_KEY/DINGTALK_APP_SECRET or DINGTALK_SHARED_OAUTH_ORG_ID")
}

func ResolveOAuthLoginConfig(orgID string) (Config, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID != "" {
		return ConfigForOrgID(orgID)
	}

	configs, err := ListActiveOrganizationConfigs()
	if err != nil {
		return Config{}, err
	}
	if len(configs) == 0 {
		if cfg, ok := defaultOAuthLoginConfig(); ok {
			return cfg, nil
		}
		return DefaultConfig().normalized(), nil
	}

	return sharedOAuthLoginConfigFromConfigs(configs)
}

func oauthConfigsShareCredentials(left, right Config) bool {
	left = left.normalized()
	right = right.normalized()
	return left.AppKey != "" &&
		left.AppSecret != "" &&
		left.AppKey == right.AppKey &&
		left.AppSecret == right.AppSecret
}

// ===================== Access Token =====================

// GetAccessToken 鑾峰彇浼佷笟鍐呴儴搴旂敤鐨?access_token锛堝甫缂撳瓨锛?
func GetAccessToken() (string, error) {
	return getAccessTokenWithConfig(DefaultConfig())
}

func GetAccessTokenForOrg(orgID string) (string, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return "", err
	}
	return getAccessTokenWithConfig(cfg)
}

func getAccessTokenWithConfig(cfg Config) (string, error) {
	cfg = cfg.normalized()
	if cfg.AppKey == "" || cfg.AppSecret == "" {
		return "", newSyncError(ErrorCodeConfigMissing, "钉钉组织配置缺失", fmt.Errorf("missing dingtalk app credentials for org %s", cfg.OrgID))
	}
	tokenMu.Lock()
	defer tokenMu.Unlock()

	// 缂撳瓨鏈夋晥
	cacheKey := cfg.cacheKey()
	if cached, ok := tokenByOrg[cacheKey]; ok && cached.token != "" && time.Now().Before(cached.expiry) {
		return cached.token, nil
	}

	body := map[string]string{
		"appKey":    cfg.AppKey,
		"appSecret": cfg.AppSecret,
	}
	resp, err := postJSON("https://api.dingtalk.com/v1.0/oauth2/accessToken", body, nil)
	if err != nil {
		if SyncErrorCode(err) == ErrorCodeNetworkFailed {
			return "", err
		}
		return "", newSyncError(ErrorCodeTokenFailed, "获取钉钉访问凭证失败", err)
	}

	accessToken, ok := resp["accessToken"].(string)
	if !ok || strings.TrimSpace(accessToken) == "" {
		return "", newSyncError(ErrorCodeTokenFailed, "获取钉钉访问凭证失败", errors.New("access token response missing accessToken"))
	}

	expireIn := 7200.0
	if v, ok := resp["expireIn"].(float64); ok {
		expireIn = v
	}

	tokenByOrg[cacheKey] = accessTokenCacheEntry{
		token:  accessToken,
		expiry: time.Now().Add(time.Duration(expireIn-60) * time.Second),
	}
	logrus.Infof("dingtalk access_token fetched org_id=%s", cfg.OrgID)
	return accessToken, nil
}

func GetAccessTokenForConfig(cfg AppConfig) (string, error) {
	return getAccessTokenWithConfig(configFromAppConfig(cfg))
}

// ===================== OAuth 鐧诲綍 =====================

// GetQRLoginURL 鑾峰彇閽夐拤鎵爜鐧诲綍 URL
func GetQRCode(state string) (string, error) {
	return GetQRCodeWithRedirect(state, GetRedirectURI())
}

func GetQRCodeWithRedirect(state, redirectURI string) (string, error) {
	return GetQRCodeWithRedirectForOrg(database.DefaultOrganizationID, state, redirectURI)
}

func GetQRCodeWithRedirectForConfig(cfg Config, state, redirectURI string) (string, error) {
	cfg = cfg.normalized()
	if cfg.AppKey == "" {
		return "", fmt.Errorf("missing dingtalk app key for org %s", cfg.OrgID)
	}
	loginURL := fmt.Sprintf(
		"https://login.dingtalk.com/oauth2/auth?redirect_uri=%s&response_type=code&client_id=%s&scope=openid%%20corpid&state=%s&prompt=consent",
		url.QueryEscape(redirectURI),
		cfg.AppKey,
		state,
	)
	return loginURL, nil
}

func GetQRCodeWithConfig(state, redirectURI string, cfg AppConfig) (string, error) {
	return GetQRCodeWithRedirectForConfig(configFromAppConfig(cfg), state, redirectURI)
}

func GetQRCodeWithRedirectForOrg(orgID, state, redirectURI string) (string, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return "", err
	}
	return GetQRCodeWithRedirectForConfig(cfg, state, redirectURI)
}

// GetUserAccessToken 鐢ㄦ巿鏉冪爜鎹㈠彇鐢ㄦ埛 token
func GetUserAccessToken(code string) (string, error) {
	return GetUserAccessTokenForOrg(database.DefaultOrganizationID, code)
}

func getUserAccessTokenInfoWithConfig(cfg Config, code string) (userAccessTokenInfo, error) {
	cfg = cfg.normalized()
	if cfg.AppKey == "" || cfg.AppSecret == "" {
		return userAccessTokenInfo{}, fmt.Errorf("missing dingtalk app credentials for org %s", cfg.OrgID)
	}
	body := map[string]string{
		"clientId":     cfg.AppKey,
		"clientSecret": cfg.AppSecret,
		"code":         code,
		"grantType":    "authorization_code",
	}
	resp, err := postJSON("https://api.dingtalk.com/v1.0/oauth2/userAccessToken", body, nil)
	if err != nil {
		return userAccessTokenInfo{}, fmt.Errorf("鑾峰彇鐢ㄦ埛 access_token 澶辫触: %w", err)
	}

	accessToken, ok := resp["accessToken"].(string)
	if !ok {
		return userAccessTokenInfo{}, fmt.Errorf("鐢ㄦ埛 access_token 鍝嶅簲寮傚父: %v", resp)
	}
	return userAccessTokenInfo{AccessToken: accessToken, Raw: resp}, nil
}

func getUserAccessTokenWithConfig(cfg Config, code string) (string, error) {
	info, err := getUserAccessTokenInfoWithConfig(cfg, code)
	if err != nil {
		return "", err
	}
	return info.AccessToken, nil
}

func GetUserAccessTokenForOrg(orgID, code string) (string, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return "", err
	}
	return getUserAccessTokenWithConfig(cfg, code)
}

func GetUserAccessTokenForConfig(code string, cfg AppConfig) (string, error) {
	return getUserAccessTokenWithConfig(configFromAppConfig(cfg), code)
}

// GetUserInfoByCode 閫氳繃鎺堟潈鐮佽幏鍙栫敤鎴蜂俊鎭紙鏂扮増 OAuth2锛岀敤浜庢壂鐮佺櫥褰曪級
func GetUserInfoByCode(code string) (map[string]interface{}, error) {
	return GetUserInfoByCodeForOrg(database.DefaultOrganizationID, code)
}

func getContactUserInfoWithUserToken(userToken string) (map[string]interface{}, error) {
	headers := map[string]string{
		"x-acs-dingtalk-access-token": userToken,
	}
	return getJSON("https://api.dingtalk.com/v1.0/contact/users/me", headers)
}

func GetUserInfoByCodeWithConfig(cfg Config, code string) (map[string]interface{}, error) {
	// 1. 鍏堢敤 code 鎹㈠彇鐢ㄦ埛 access_token
	tokenInfo, err := getUserAccessTokenInfoWithConfig(cfg, code)
	if err != nil {
		return nil, err
	}

	// 2. 获取 OAuth 个人信息。当前钉钉扫码 OAuth 在部分企业内部应用场景只返回 unionId/openId。
	resp, err := getContactUserInfoWithUserToken(tokenInfo.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇鐢ㄦ埛淇℃伅澶辫触: %w", err)
	}

	return mergeDingTalkOAuthIdentityFields(resp, tokenInfo.Raw), nil
}

func mergeDingTalkOAuthIdentityFields(target, source map[string]interface{}) map[string]interface{} {
	if target == nil {
		target = map[string]interface{}{}
	}
	if len(source) == 0 {
		return target
	}

	for _, key := range []string{
		"corpId", "corpID", "corp_id", "corpid",
		"associated_user_id", "associatedUserId", "userid", "userId",
		"unionId", "unionid", "union_id",
		"openId", "openid", "open_id",
	} {
		if existing, ok := target[key].(string); ok && strings.TrimSpace(existing) != "" {
			continue
		}
		value, ok := source[key].(string)
		if !ok || strings.TrimSpace(value) == "" {
			continue
		}
		target[key] = strings.TrimSpace(value)
	}

	return target
}

func GetUserInfoByCodeForOrg(orgID, code string) (map[string]interface{}, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return nil, err
	}
	return GetUserInfoByCodeWithConfig(cfg, code)
}

func GetUserInfoByCodeForConfig(code string, cfg AppConfig) (map[string]interface{}, error) {
	return GetUserInfoByCodeWithConfig(configFromAppConfig(cfg), code)
}

// GetUserIDByInAppCode 浼佷笟鍐呴儴搴旂敤鍏嶇櫥锛氶€氳繃鍏嶇櫥鐮佽幏鍙栦紒涓氬唴 userid
func GetUserIDByInAppCode(code string) (string, error) {
	return GetUserIDByInAppCodeForOrg(database.DefaultOrganizationID, code)
}

func GetUserIDByInAppCodeForOrg(orgID, code string) (string, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return "", err
	}
	return getUserIDByInAppCodeWithAccessToken(accessToken, code)
}

func GetUserIDByInAppCodeForConfig(code string, cfg AppConfig) (string, error) {
	accessToken, err := GetAccessTokenForConfig(cfg)
	if err != nil {
		return "", err
	}
	return getUserIDByInAppCodeWithAccessToken(accessToken, code)
}

func getUserIDByInAppCodeWithAccessToken(accessToken, code string) (string, error) {
	body := map[string]interface{}{
		"code": code,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/user/getuserinfo?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return "", fmt.Errorf("鍏嶇櫥鑾峰彇鐢ㄦ埛韬唤澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return "", fmt.Errorf("鍏嶇櫥鑾峰彇鐢ㄦ埛韬唤澶辫触: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("鍏嶇櫥鍝嶅簲鏍煎紡寮傚父: %v", resp)
	}

	userid := getString(result, "userid")
	if userid == "" {
		return "", fmt.Errorf("鍏嶇櫥鏈繑鍥?userid")
	}

	return userid, nil
}

// GetUserDetailByUserID 閫氳繃 userid 鑾峰彇鐢ㄦ埛璇︾粏淇℃伅锛圕ontact.User.Read锛?
func GetUserDetailByUserID(userid string) (map[string]interface{}, error) {
	return GetUserDetailByUserIDForOrg(database.DefaultOrganizationID, userid)
}

func GetUserDetailByUserIDForOrg(orgID, userid string) (map[string]interface{}, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}
	return getUserDetailByUserIDWithAccessToken(accessToken, userid)
}

func GetUserDetailByUserIDForConfig(userid string, cfg AppConfig) (map[string]interface{}, error) {
	accessToken, err := GetAccessTokenForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return getUserDetailByUserIDWithAccessToken(accessToken, userid)
}

func getUserDetailByUserIDWithAccessToken(accessToken, userid string) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"userid": userid,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/user/get?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇鐢ㄦ埛璇︽儏澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, fmt.Errorf("鑾峰彇鐢ㄦ埛璇︽儏澶辫触: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("鐢ㄦ埛璇︽儏鏍煎紡寮傚父: %v", resp)
	}

	return result, nil
}

func GetUserIDByUnionID(unionID string) (string, error) {
	return GetUserIDByUnionIDForOrg(database.DefaultOrganizationID, unionID)
}

func GetUserIDByUnionIDForOrg(orgID, unionID string) (string, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return "", err
	}
	return getUserIDByUnionIDWithAccessToken(accessToken, unionID)
}

func GetUserIDByUnionIDForConfig(unionID string, cfg AppConfig) (string, error) {
	accessToken, err := GetAccessTokenForConfig(cfg)
	if err != nil {
		return "", err
	}
	return getUserIDByUnionIDWithAccessToken(accessToken, unionID)
}

func getUserIDByUnionIDWithAccessToken(accessToken, unionID string) (string, error) {
	body := map[string]interface{}{
		"unionid": unionID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/user/getbyunionid?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return "", fmt.Errorf("get userid by unionid failed: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return "", fmt.Errorf("get userid by unionid failed: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("unexpected getbyunionid result: %v", resp)
	}

	userid := getString(result, "userid")
	if userid == "" {
		return "", fmt.Errorf("missing userid in getbyunionid response")
	}

	return userid, nil
}

// ===================== 缁勭粐鏋舵瀯鍚屾 =====================

// DeptInfo 閮ㄩ棬淇℃伅
type DeptInfo struct {
	DeptID             int64                  `json:"dept_id"`
	Name               string                 `json:"name"`
	ParentID           int64                  `json:"parent_id"`
	DeptManagerUserIDs []string               `json:"dept_manager_userids"`
	Extension          map[string]interface{} `json:"extension"`
}

// UserInfo 鐢ㄦ埛淇℃伅
type UserInfo struct {
	UserID                 string                 `json:"userid"`
	Name                   string                 `json:"name"`
	Email                  string                 `json:"email"`
	Mobile                 string                 `json:"mobile"`
	DeptIDList             []int64                `json:"dept_id_list"`
	Position               string                 `json:"title"`
	PositionSource         string                 `json:"position_source"`
	PositionSyncDiagnostic map[string]interface{} `json:"position_sync_diagnostic"`
	ManagerUserID          string                 `json:"manager_user_id"`
	ManagerName            string                 `json:"manager_name"`
	ManagerSource          string                 `json:"manager_source"`
	Avatar                 string                 `json:"avatar"`
	Active                 bool                   `json:"active"`
	HiredDate              string                 `json:"hired_date"` // 入职日期，格式 YYYY-MM-DD
	PlannedRegularDate     string                 `json:"planned_regular_date"`
	ActualRegularDate      string                 `json:"actual_regular_date"`
	ProbationEndDate       string                 `json:"probation_end_date"`
	EmploymentType         string                 `json:"employment_type"`
	EmploymentTypeCode     string                 `json:"employment_type_code"`
	JobLevel               string                 `json:"job_level"`
	JobFamily              string                 `json:"job_family"`
	HRMFieldSyncStatus     string                 `json:"hrm_field_sync_status"`
}

// SyncDepartments 鍚屾鎵€鏈夐儴闂?
func SyncDepartments() ([]DeptInfo, error) {
	return SyncDepartmentsForOrg(database.DefaultOrganizationID)
}

func SyncDepartmentsForOrg(orgID string) ([]DeptInfo, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}

	rootDept, err := fetchDeptDetail(accessToken, 1)
	if err != nil {
		return nil, err
	}
	if rootDept == nil || rootDept.DeptID != 1 || strings.TrimSpace(rootDept.Name) == "" {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的根部门数据异常", errors.New("root department is missing or invalid"))
	}

	allDepts := []DeptInfo{*rootDept}

	// Recursively fetch all departments starting from root department 1.
	if err := fetchDeptTree(accessToken, 1, &allDepts); err != nil {
		return nil, err
	}

	if len(allDepts) == 0 {
		return nil, newSyncError(ErrorCodeDepartmentEmpty, "钉钉返回的部门数据为空", errors.New("department list is empty"))
	}

	logrus.Infof("dingtalk sync departments complete: %d", len(allDepts))
	return allDepts, nil
}

// SyncDepartmentsForConfig 使用指定组织的钉钉应用凭证同步部门（多租户）
func SyncDepartmentsForConfig(cfg AppConfig) ([]DeptInfo, error) {
	accessToken, err := GetAccessTokenForConfig(cfg)
	if err != nil {
		return nil, err
	}

	rootDept, err := fetchDeptDetail(accessToken, 1)
	if err != nil {
		return nil, err
	}
	if rootDept == nil || rootDept.DeptID != 1 || strings.TrimSpace(rootDept.Name) == "" {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的根部门数据异常", errors.New("root department is missing or invalid"))
	}

	allDepts := []DeptInfo{*rootDept}

	// Recursively fetch all departments starting from root department 1.
	if err := fetchDeptTree(accessToken, 1, &allDepts); err != nil {
		return nil, err
	}

	if len(allDepts) == 0 {
		return nil, newSyncError(ErrorCodeDepartmentEmpty, "钉钉返回的部门数据为空", errors.New("department list is empty"))
	}

	logrus.Infof("dingtalk sync departments complete for org=%s: %d", cfg.OrgID, len(allDepts))
	return allDepts, nil
}

func fetchDeptTree(accessToken string, parentID int64, result *[]DeptInfo) error {
	body := map[string]interface{}{
		"dept_id": parentID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/listsub?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return dingTalkDepartmentAPIError("list departments", errcode, errmsg)
	}

	rawResult, exists := resp["result"]
	if !exists {
		return newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department list response missing result"))
	}
	resultList, ok := rawResult.([]interface{})
	if !ok {
		return newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department list result has invalid type"))
	}

	for _, item := range resultList {
		m, ok := item.(map[string]interface{})
		if !ok {
			return newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department list item has invalid type"))
		}
		dept := parseDeptInfo(m)
		if dept.DeptID <= 0 {
			return newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department list item missing dept_id"))
		}
		detail, err := fetchDeptDetail(accessToken, dept.DeptID)
		if err != nil {
			return err
		}
		if detail == nil {
			return newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department detail is nil"))
		}
		if detail.Name != "" {
			dept.Name = detail.Name
		}
		if detail.ParentID != 0 {
			dept.ParentID = detail.ParentID
		}
		dept.DeptManagerUserIDs = detail.DeptManagerUserIDs
		dept.Extension = detail.Extension
		*result = append(*result, dept)

		// Recursively fetch child departments.
		if err := fetchDeptTree(accessToken, dept.DeptID, result); err != nil {
			return err
		}
	}

	return nil
}

func fetchDeptDetail(accessToken string, deptID int64) (*DeptInfo, error) {
	body := map[string]interface{}{
		"dept_id": deptID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/department/get?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, dingTalkDepartmentAPIError("get department detail", errcode, errmsg)
	}

	rawResult, exists := resp["result"]
	if !exists {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department detail response missing result"))
	}
	result, ok := rawResult.(map[string]interface{})
	if !ok {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department detail result has invalid type"))
	}

	dept := parseDeptInfo(result)
	if dept.DeptID <= 0 || strings.TrimSpace(dept.Name) == "" {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的部门数据格式异常", errors.New("department detail missing required fields"))
	}
	return &dept, nil
}

func dingTalkDepartmentAPIError(operation string, errcode float64, errmsg string) error {
	detail := fmt.Errorf("%s failed: errcode=%.0f errmsg=%s", operation, errcode, sanitizeDingTalkDiagnostic(errmsg))
	normalized := strings.ToLower(strings.TrimSpace(errmsg))
	if strings.Contains(normalized, "权限") || strings.Contains(normalized, "permission") || strings.Contains(normalized, "forbidden") || strings.Contains(normalized, "not authorized") || strings.Contains(normalized, "access denied") {
		return newSyncError(ErrorCodePermissionDenied, "钉钉通讯录权限不足", detail)
	}
	return newSyncError(ErrorCodeResponseInvalid, "钉钉返回部门数据失败", detail)
}

func parseDeptInfo(result map[string]interface{}) DeptInfo {
	dept := DeptInfo{
		DeptID:             int64(getFloat(result, "dept_id")),
		Name:               getString(result, "name"),
		ParentID:           int64(getFloat(result, "parent_id")),
		DeptManagerUserIDs: stringSliceFromMappedPaths(result, configuredDingTalkFieldPaths("DINGTALK_DEPARTMENT_HEAD_FIELD_KEYS", defaultDepartmentHeadFieldPaths())),
	}
	dept.Extension = map[string]interface{}{
		"dingtalk_department_api": "topapi/v2/department/get",
		"dingtalk_raw_field_keys": sortedMapKeys(result),
	}
	if len(dept.DeptManagerUserIDs) > 0 {
		dept.Extension["department_head_user_ids"] = dept.DeptManagerUserIDs
		dept.Extension["department_head_user_id"] = dept.DeptManagerUserIDs[0]
		dept.Extension["dingtalk_department_head_source"] = "DINGTALK_DEPARTMENT_HEAD_FIELD_KEYS"
	}
	return dept
}

// SyncUsers 鍚屾鎸囧畾閮ㄩ棬鐨勬墍鏈夌敤鎴?
func SyncUsers() ([]UserInfo, error) {
	return SyncUsersForOrg(database.DefaultOrganizationID)
}

func SyncUsersForOrg(orgID string) ([]UserInfo, error) {
	depts, err := SyncDepartmentsForOrg(orgID)
	if err != nil {
		return nil, fmt.Errorf("鍚屾鐢ㄦ埛鍓嶈幏鍙栭儴闂ㄥけ璐? %w", err)
	}
	return SyncUsersWithDeptsForOrg(orgID, depts)
}

// SyncUsersWithDepts 浣跨敤宸叉湁閮ㄩ棬鍒楄〃鍚屾鎵€鏈夌敤鎴凤紝閬垮厤閲嶅璋冪敤 SyncDepartments
func SyncUsersWithDepts(depts []DeptInfo) ([]UserInfo, error) {
	return SyncUsersWithDeptsForOrg(database.DefaultOrganizationID, depts)
}

func SyncUsersWithDeptsForOrg(orgID string, depts []DeptInfo) ([]UserInfo, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}

	userMap := make(map[string]UserInfo) // 鍘婚噸

	for _, dept := range depts {
		users, err := fetchDeptUsers(accessToken, dept.DeptID)
		if err != nil {
			logrus.Warnf("dingtalk department users fetch failed: dept_id=%d err=%s", dept.DeptID, safeDingTalkErrorForLog(err))
			return nil, newSyncError(ErrorCodeUserSourceIncomplete, "钉钉员工源数据不完整，已停止同步以避免误停用历史员工", err)
		}
		for _, u := range users {
			if existing, ok := userMap[u.UserID]; ok {
				u.DeptIDList = mergeDingTalkDepartmentIDs(existing.DeptIDList, u.DeptIDList)
			}
			userMap[u.UserID] = u
		}
	}
	if len(userMap) == 0 {
		return nil, newSyncError(ErrorCodeUserSourceIncomplete, "钉钉员工源数据为空，已停止同步以避免误停用历史员工", errors.New("all department user lists are empty"))
	}

	if err := enrichUsersWithUserDetails(accessToken, userMap); err != nil {
		logrus.Warnf("dingtalk user detail sync skipped partially: %s", safeDingTalkErrorForLog(err))
	}
	resolveManagerNames(userMap)
	cfg, cfgErr := ConfigForOrgID(orgID)
	if cfgErr != nil {
		return nil, cfgErr
	}
	if err := enrichUsersWithHRMFieldsForOrg(accessToken, userMap, cfg); err != nil {
		logrus.Warnf("dingtalk hrm field sync skipped: %s", safeDingTalkErrorForLog(err))
		markUsersHRMFieldSyncStatus(userMap, HRMFieldSyncStatusFailed)
	} else if hasAnyHRMTargetField(userMap) {
		markUsersHRMFieldSyncStatus(userMap, HRMFieldSyncStatusSuccess)
	} else {
		markUsersHRMFieldSyncStatus(userMap, HRMFieldSyncStatusNoFields)
	}
	resolveManagerNames(userMap)

	var allUsers []UserInfo
	for _, u := range userMap {
		allUsers = append(allUsers, u)
	}

	logrus.Infof("dingtalk sync users complete: %d", len(allUsers))
	return allUsers, nil
}

// SyncUsersWithDeptsForConfig 使用指定组织的钉钉应用凭证同步用户（多租户）
func SyncUsersWithDeptsForConfig(cfg AppConfig, depts []DeptInfo) ([]UserInfo, error) {
	accessToken, err := GetAccessTokenForConfig(cfg)
	if err != nil {
		return nil, err
	}

	userMap := make(map[string]UserInfo) // 去重

	for _, dept := range depts {
		users, err := fetchDeptUsers(accessToken, dept.DeptID)
		if err != nil {
			logrus.Warnf("dingtalk department users fetch failed: dept_id=%d err=%s", dept.DeptID, safeDingTalkErrorForLog(err))
			return nil, newSyncError(ErrorCodeUserSourceIncomplete, "钉钉员工源数据不完整，已停止同步以避免误停用历史员工", err)
		}
		for _, u := range users {
			if existing, ok := userMap[u.UserID]; ok {
				u.DeptIDList = mergeDingTalkDepartmentIDs(existing.DeptIDList, u.DeptIDList)
			}
			userMap[u.UserID] = u
		}
	}
	if len(userMap) == 0 {
		return nil, newSyncError(ErrorCodeUserSourceIncomplete, "钉钉员工源数据为空，已停止同步以避免误停用历史员工", errors.New("all department user lists are empty"))
	}

	if err := enrichUsersWithUserDetails(accessToken, userMap); err != nil {
		logrus.Warnf("dingtalk user detail sync skipped partially: %v", err)
	}
	resolveManagerNames(userMap)
	if err := enrichUsersWithHRMFieldsForOrg(accessToken, userMap, configFromAppConfig(cfg)); err != nil {
		logrus.Warnf("dingtalk hrm field sync skipped: %v", err)
		markUsersHRMFieldSyncStatus(userMap, HRMFieldSyncStatusFailed)
	} else if hasAnyHRMTargetField(userMap) {
		markUsersHRMFieldSyncStatus(userMap, HRMFieldSyncStatusSuccess)
	} else {
		markUsersHRMFieldSyncStatus(userMap, HRMFieldSyncStatusNoFields)
	}
	resolveManagerNames(userMap)

	var allUsers []UserInfo
	for _, u := range userMap {
		allUsers = append(allUsers, u)
	}

	logrus.Infof("dingtalk sync users complete for org=%s: %d", cfg.OrgID, len(allUsers))
	return allUsers, nil
}

func markUsersHRMFieldSyncStatus(users map[string]UserInfo, status string) {
	status = strings.TrimSpace(status)
	for userID, user := range users {
		user.HRMFieldSyncStatus = status
		users[userID] = user
	}
}

// hasAnyHRMTargetField reports whether at least one user has a non-empty
// employment type, job level, or job family value populated from the HRM API.
// It is used to distinguish "API succeeded but returned no target fields"
// from "API succeeded and returned target fields".
func hasAnyHRMTargetField(users map[string]UserInfo) bool {
	for _, user := range users {
		if strings.TrimSpace(user.EmploymentType) != "" ||
			strings.TrimSpace(user.JobLevel) != "" ||
			strings.TrimSpace(user.JobFamily) != "" {
			return true
		}
	}
	return false
}

func enrichUsersWithUserDetails(accessToken string, users map[string]UserInfo) error {
	if len(users) == 0 {
		return nil
	}

	userIDs := make([]string, 0, len(users))
	for userID, user := range users {
		if strings.TrimSpace(userID) == "" {
			continue
		}
		if strings.TrimSpace(user.ManagerUserID) != "" {
			continue
		}
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)

	var firstErr error
	failed := 0
	for _, userID := range userIDs {
		detail, err := fetchDingTalkUserDetail(accessToken, userID)
		if err != nil {
			failed++
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		users[userID] = mergeDingTalkUserDetail(users[userID], detail)
	}
	if failed > 0 {
		return fmt.Errorf("fetch dingtalk user details failed for %d/%d users: %w", failed, len(userIDs), firstErr)
	}
	return nil
}

func fetchDingTalkUserDetail(accessToken, userID string) (map[string]interface{}, error) {
	body := map[string]interface{}{
		"userid":   userID,
		"language": "zh_CN",
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/user/get?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		return nil, fmt.Errorf("fetch dingtalk user detail failed: %s", dingTalkErrorMessage(resp, errcode))
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected dingtalk user detail response: %v", resp)
	}
	return result, nil
}

func mergeDingTalkUserDetail(user UserInfo, detail map[string]interface{}) UserInfo {
	if strings.TrimSpace(user.Position) == "" {
		if position, source := resolveDingTalkPosition(detail); position != "" {
			user.Position = position
			user.PositionSource = prefixedDingTalkSource("topapi/v2/user/get", source)
			user.PositionSyncDiagnostic = buildPositionSyncDiagnostic("topapi/v2/user/get", detail, user.PositionSource, user.Position, "")
		}
	}
	if managerUserID, source := resolveDingTalkDirectManagerID(detail); managerUserID != "" {
		user.ManagerUserID = managerUserID
		user.ManagerSource = prefixedDingTalkSource("topapi/v2/user/get", source)
	}
	if managerName, source := resolveDingTalkDirectManagerName(detail); managerName != "" {
		user.ManagerName = managerName
		if strings.TrimSpace(user.ManagerSource) == "" {
			user.ManagerSource = prefixedDingTalkSource("topapi/v2/user/get", source)
		}
	}
	return user
}

func prefixedDingTalkSource(apiName, source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return ""
	}
	return apiName + "." + source
}

type hrmRegularDates struct {
	Planned            string
	Actual             string
	ProbationEndDate   string
	EmploymentType     string
	EmploymentTypeCode string
	JobLevel           string
	JobFamily          string
	Position           string
	PositionSource     string
}

func enrichUsersWithHRMFields(accessToken string, users map[string]UserInfo) error {
	return enrichUsersWithHRMFieldsForOrg(accessToken, users, DefaultConfig())
}

func enrichUsersWithHRMFieldsForOrg(accessToken string, users map[string]UserInfo, cfg Config) error {
	if len(users) == 0 {
		return nil
	}

	userIDs := make([]string, 0, len(users))
	for userID := range users {
		if userID != "" {
			userIDs = append(userIDs, userID)
		}
	}

	for start := 0; start < len(userIDs); start += 50 {
		end := start + 50
		if end > len(userIDs) {
			end = len(userIDs)
		}
		dates, err := fetchHRMRegularDatesForOrg(accessToken, userIDs[start:end], cfg)
		if err != nil {
			return err
		}
		for userID, regularDates := range dates {
			user, ok := users[userID]
			if !ok {
				continue
			}
			user.PlannedRegularDate = regularDates.Planned
			user.ActualRegularDate = regularDates.Actual
			user.ProbationEndDate = regularDates.ProbationEndDate
			user.EmploymentType = regularDates.EmploymentType
			user.EmploymentTypeCode = regularDates.EmploymentTypeCode
			user.JobLevel = regularDates.JobLevel
			user.JobFamily = regularDates.JobFamily
			if strings.TrimSpace(user.Position) == "" && strings.TrimSpace(regularDates.Position) != "" {
				user.Position = strings.TrimSpace(regularDates.Position)
				user.PositionSource = regularDates.PositionSource
				user.PositionSyncDiagnostic = buildPositionSyncDiagnostic(
					"topapi/smartwork/hrm/employee/v2/list",
					nil,
					user.PositionSource,
					user.Position,
					"",
				)
			}
			users[userID] = user
		}
	}

	return nil
}

func fetchHRMRegularDates(accessToken string, userIDs []string) (map[string]hrmRegularDates, error) {
	return fetchHRMRegularDatesForOrg(accessToken, userIDs, DefaultConfig())
}

func fetchHRMRegularDatesForOrg(accessToken string, userIDs []string, cfg Config) (map[string]hrmRegularDates, error) {
	result := make(map[string]hrmRegularDates)
	if len(userIDs) == 0 {
		return result, nil
	}
	agentID, err := requireDingTalkAgentID(cfg)
	if err != nil {
		return result, err
	}

	body := map[string]interface{}{
		"agentid":           agentID,
		"userid_list":       strings.Join(userIDs, ","),
		"field_filter_list": strings.Join(configuredHRMFieldCodes(cfg), ","),
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/smartwork/hrm/employee/v2/list?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return result, err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		if errmsg == "" {
			errmsg = fmt.Sprintf("unknown errcode %.0f", errcode)
		}
		return result, dingTalkDepartmentAPIError("fetch hrm employee fields", errcode, errmsg)
	}

	rawItems, exists := resp["result"]
	if !exists {
		return result, newSyncError(ErrorCodeResponseInvalid, "钉钉智能人事字段返回格式异常", errors.New("hrm employee response missing result"))
	}
	items, ok := rawItems.([]interface{})
	if !ok {
		return result, newSyncError(ErrorCodeResponseInvalid, "钉钉智能人事字段返回格式异常", errors.New("hrm employee result has invalid type"))
	}
	for _, item := range items {
		record, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		userID := getString(record, "userid")
		if userID == "" {
			continue
		}

		fields, ok := record["field_data_list"].([]interface{})
		if !ok {
			continue
		}
		regularDates := parseHRMEmployeeFields(fields, cfg)
		result[userID] = regularDates
	}

	return result, nil
}

func parseHRMEmployeeFields(fields []interface{}, cfg Config) hrmRegularDates {
	var result hrmRegularDates
	for _, field := range fields {
		fieldMap, ok := field.(map[string]interface{})
		if !ok {
			continue
		}
		value := extractHRMFieldValue(fieldMap)
		switch getString(fieldMap, "field_code") {
		case "sys01-planRegularTime":
			result.Planned = value
		case "sys01-regularTime":
			result.Actual = value
		}
		if result.ProbationEndDate == "" && matchesConfiguredHRMField(fieldMap, cfg, hrmFieldProbationEndDate) {
			result.ProbationEndDate = value
		}
		if result.EmploymentType == "" && matchesConfiguredHRMField(fieldMap, cfg, hrmFieldEmploymentType) {
			result.EmploymentTypeCode, result.EmploymentType = resolveHRMEmploymentType(fieldMap, cfg)
		}
		if result.JobLevel == "" && matchesConfiguredHRMField(fieldMap, cfg, hrmFieldJobLevel) {
			result.JobLevel = extractHRMTextFieldValue(fieldMap)
		}
		if result.JobFamily == "" && matchesConfiguredHRMField(fieldMap, cfg, hrmFieldJobFamily) {
			result.JobFamily = extractHRMTextFieldValue(fieldMap)
		}
		if result.Position == "" && matchesConfiguredHRMField(fieldMap, cfg, hrmFieldPosition) {
			result.Position = extractHRMTextFieldValue(fieldMap)
			result.PositionSource = firstNonEmptyStringValue(getString(fieldMap, "field_code"), getString(fieldMap, "field_name"), getString(fieldMap, "name"))
		}
	}
	return result
}

func getDingTalkAgentID() int64 {
	return dingTalkAgentIDFromConfig(DefaultConfig())
}

func dingTalkAgentIDFromConfig(cfg Config) int64 {
	id, _ := requireDingTalkAgentID(cfg)
	return id
}

func requireDingTalkAgentID(cfg Config) (int64, error) {
	cfg = cfg.normalized()
	raw := strings.TrimSpace(cfg.AgentID)
	if raw == "" && cfg.OrgID == database.DefaultOrganizationID {
		raw = strings.TrimSpace(os.Getenv("DINGTALK_AGENT_ID"))
	}
	if raw == "" {
		return 0, newSyncError(ErrorCodeConfigMissing, "钉钉组织配置缺少 AgentID", fmt.Errorf("missing dingtalk agent id for org %s", cfg.OrgID))
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, newSyncError(ErrorCodeConfigMissing, "钉钉组织配置中的 AgentID 格式错误", fmt.Errorf("invalid dingtalk agent id for org %s", cfg.OrgID))
	}
	return id, nil
}

func extractHRMFieldValue(field map[string]interface{}) string {
	values, ok := field["field_value_list"].([]interface{})
	if !ok {
		return ""
	}
	for _, item := range values {
		valueMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		value := normalizeDingTalkDate(stringValue(valueMap["value"]))
		if value != "" {
			return value
		}
		value = normalizeDingTalkDate(stringValue(valueMap["label"]))
		if value != "" {
			return value
		}
	}
	return ""
}

func stringValue(v interface{}) string {
	switch value := v.(type) {
	case string:
		return value
	case float64:
		if value == 0 {
			return ""
		}
		return strconv.FormatInt(int64(value), 10)
	case int64:
		if value == 0 {
			return ""
		}
		return strconv.FormatInt(value, 10)
	case int:
		if value == 0 {
			return ""
		}
		return strconv.Itoa(value)
	default:
		return ""
	}
}

func normalizeDingTalkDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) >= len("2006-01-02") && value[4] == '-' && value[7] == '-' {
		return value[:10]
	}
	if ts, err := strconv.ParseInt(value, 10, 64); err == nil && ts > 0 {
		if ts > 1_000_000_000_000 {
			return time.UnixMilli(ts).Format("2006-01-02")
		}
		if ts > 1_000_000_000 {
			return time.Unix(ts, 0).Format("2006-01-02")
		}
	}
	return value
}

type VacationType struct {
	LeaveCode     string  `json:"leave_code"`
	LeaveName     string  `json:"leave_name"`
	LeaveViewUnit string  `json:"leave_view_unit"`
	HoursInPerDay float64 `json:"hours_in_per_day"`
	LeaveType     int     `json:"leave_type"` // 0=调休(TURN), 1=普通假期
	GrantType     int     `json:"grant_type"` // 0=自动, 1=手动
	Source        string  `json:"source"`     // "inner"=后台手动创建, 其他=API创建
	BizType       string  `json:"biz_type"`   // "general_leave"=普通假期, "lieu_leave"=调休
}

func ListVacationTypes(opUserID string) ([]VacationType, error) {
	return ListVacationTypesForOrg(database.DefaultOrganizationID, opUserID)
}

func ListVacationTypesForOrg(orgID, opUserID string) ([]VacationType, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return nil, err
	}
	return listVacationTypesWithConfig(cfg, opUserID)
}

func ListVacationTypesForConfig(opUserID string, cfg AppConfig) ([]VacationType, error) {
	return listVacationTypesWithConfig(configFromAppConfig(cfg), opUserID)
}

func listVacationTypesWithConfig(cfg Config, opUserID string) ([]VacationType, error) {
	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return nil, err
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/type/list?access_token=%s", accessToken),
		map[string]interface{}{
			"vacation_source": "all",
			"op_userid":       opUserID,
		},
	)
	if err != nil {
		return nil, err
	}
	if errcode, _ := resp["errcode"].(float64); errcode != 0 {
		return nil, fmt.Errorf("list vacation types failed: %s", dingTalkErrorMessage(resp, errcode))
	}
	items, ok := resp["result"].([]interface{})
	if !ok {
		return nil, nil
	}
	result := make([]VacationType, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		vt := VacationType{
			LeaveCode:     getString(m, "leave_code"),
			LeaveName:     getString(m, "leave_name"),
			LeaveViewUnit: getString(m, "leave_view_unit"),
			HoursInPerDay: getFloat(m, "hours_in_per_day"),
			LeaveType:     int(getFloat(m, "leave_type")),
			GrantType:     int(getFloat(m, "grant_type")),
			Source:        getString(m, "source"),
			BizType:       getString(m, "biz_type"),
		}
		logrus.Debugf("[vacation-type] leaveCode=%s name=%s leaveType=%d grantType=%d unit=%s source=%s bizType=%s",
			vt.LeaveCode, vt.LeaveName, vt.LeaveType, vt.GrantType, vt.LeaveViewUnit, vt.Source, vt.BizType)
		result = append(result, vt)
	}
	return result, nil
}

func UpdateAnnualLeaveQuota(userID string, year int, days float64, reason string) error {
	return UpdateAnnualLeaveQuotaForOrg(database.DefaultOrganizationID, userID, year, days, reason)
}

func UpdateAnnualLeaveQuotaForOrg(orgID, userID string, year int, days float64, reason string) error {
	if days <= 0 {
		return nil
	}
	opUserID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	if opUserID == "" {
		return fmt.Errorf("missing DINGTALK_ADMIN_USER_ID")
	}

	leaveCode, hoursPerDay, err := resolveAnnualLeaveTypeForOrg(orgID, opUserID)
	if err != nil {
		return err
	}
	if hoursPerDay <= 0 {
		hoursPerDay = getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8)
	}

	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return err
	}

	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(year, 12, 31, 23, 59, 59, 0, time.Local)
	// 钉钉单位：1/100 天（100 = 1天，250 = 2.5天）
	quotaPerDay := int64(math.Round(days * 100))
	quotaPerHour := int64(math.Round(days * hoursPerDay * 100))

	logrus.Infof("[leave-sync] UpdateAnnualLeaveQuota userID=%s year=%d days=%.2f leaveCode=%s quotaPerDay=%d",
		userID, year, days, leaveCode, quotaPerDay)

	updateBody := map[string]interface{}{
		"op_userid": opUserID,
		"leave_quotas": []map[string]interface{}{
			{
				"userid":             userID,
				"leave_code":         leaveCode,
				"quota_num_per_day":  quotaPerDay,
				"quota_num_per_hour": quotaPerHour,
				"quota_cycle":        strconv.Itoa(year),
				"start_time":         start.UnixMilli(),
				"end_time":           end.UnixMilli(),
				"reason":             reason,
			},
		},
	}

	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/quota/update?access_token=%s", accessToken),
		updateBody,
	)
	if err != nil {
		return err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode == 0 {
		return nil
	}

	logrus.Warnf("[leave-sync] quota/update errcode=%.0f errmsg=%s", errcode, dingTalkErrorMessage(resp, errcode))

	// errcode 880015：用户在钉钉中尚无此假期配额记录。
	// 新版 API 两步走：先 GET 初始化记录，再 POST 写入实际配额。
	if errcode == 880015 {
		newAPIHeaders := map[string]string{"x-acs-dingtalk-access-token": accessToken}

		// 步骤 1：初始化配额记录（若记录已存在则幂等，不会重置）
		initURL := fmt.Sprintf(
			"https://api.dingtalk.com/v1.0/attendance/leaves/initializations/balances?opUserId=%s&userId=%s&leaveCode=%s",
			url.QueryEscape(opUserID), url.QueryEscape(userID), url.QueryEscape(leaveCode),
		)
		if _, initErr := getJSON(initURL, newAPIHeaders); initErr != nil {
			return fmt.Errorf("initialize annual leave quota record failed: %w", initErr)
		}

		// 步骤 2：写入实际配额（JSON 数组 body，opUserId 放 query）
		setURL := fmt.Sprintf(
			"https://api.dingtalk.com/v1.0/attendance/leaves/quota?opUserId=%s",
			url.QueryEscape(opUserID),
		)
		setBody := []map[string]interface{}{{
			"userId":          userID,
			"leaveCode":       leaveCode,
			"quotaNumPerDay":  quotaPerDay,
			"quotaNumPerHour": quotaPerHour,
			"startTime":       start.UnixMilli(),
			"endTime":         end.UnixMilli(),
			"quotaCycle":      strconv.Itoa(year),
			"reason":          reason,
		}}
		if _, setErr := postJSON(setURL, setBody, newAPIHeaders); setErr != nil {
			return fmt.Errorf("set annual leave quota after init failed: %w", setErr)
		}

		logrus.Infof("[leave-sync] new API init+set success userID=%s year=%d days=%.2f", userID, year, days)
		return nil
	}

	return fmt.Errorf("update annual leave quota failed: errcode=%.0f %s", errcode, dingTalkErrorMessage(resp, errcode))
}

func UpdateCompensatoryLeaveQuota(userID string, minutes int, workDate string, reason string) error {
	return UpdateCompensatoryLeaveQuotaForOrg(database.DefaultOrganizationID, userID, minutes, workDate, reason)
}

func UpdateCompensatoryLeaveQuotaForOrg(orgID, userID string, minutes int, workDate string, reason string) error {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return err
	}
	return updateCompensatoryLeaveQuotaWithConfig(cfg, userID, minutes, workDate, reason)
}

func UpdateCompensatoryLeaveQuotaForConfig(cfg AppConfig, userID string, minutes int, workDate string, reason string) error {
	return updateCompensatoryLeaveQuotaWithConfig(configFromAppConfig(cfg), userID, minutes, workDate, reason)
}

func updateCompensatoryLeaveQuotaWithConfig(cfg Config, userID string, minutes int, workDate string, reason string) error {
	if minutes <= 0 {
		return nil
	}
	opUserID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	if opUserID == "" {
		return fmt.Errorf("missing DINGTALK_ADMIN_USER_ID")
	}

	leaveCode, hoursPerDay, err := resolveCompensatoryLeaveTypeWithConfig(cfg, opUserID)
	if err != nil {
		return err
	}
	if hoursPerDay <= 0 {
		hoursPerDay = getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8)
	}

	// 验证leaveCode和userID
	if leaveCode == "" {
		return fmt.Errorf("leaveCode is empty")
	}
	if userID == "" {
		return fmt.Errorf("userID is empty")
	}

	logrus.Infof("[comp-leave-sync] UpdateCompensatoryLeaveQuota userID=%s minutes=%d leaveCode=%s workDate=%s",
		userID, minutes, leaveCode, workDate)

	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return err
	}

	year := time.Now().Year()
	if parsedWorkDate, err := time.ParseInLocation("2006-01-02", workDate, time.Local); err == nil {
		year = parsedWorkDate.Year()
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(year, 12, 31, 23, 59, 59, 0, time.Local)
	hours := float64(minutes) / 60.0
	addQuotaPerHour := int64(math.Round(hours * 100))

	initErr := ensureVacationBalanceInitialized(accessToken, opUserID, userID, leaveCode, year, start, end)
	if initErr != nil {
		logrus.Warnf("[comp-leave-sync] initialize balance failed, trying batch update anyway: %v", initErr)
	}

	_, currentQuotaPerHour, err := getVacationQuotaByYear(accessToken, opUserID, userID, leaveCode, year)
	if err != nil {
		return fmt.Errorf("get current compensatory leave quota failed: %w", err)
	}
	targetQuotaPerHour := currentQuotaPerHour + addQuotaPerHour
	targetQuotaPerDay := quotaHourToDay(targetQuotaPerHour, hoursPerDay)

	logrus.Infof("[comp-leave-sync] current=%d add=%d target=%d (hour x100)", currentQuotaPerHour, addQuotaPerHour, targetQuotaPerHour)
	requestID, updateErr := updateVacationQuotaOAPI(accessToken, opUserID, userID, leaveCode, year, targetQuotaPerDay, targetQuotaPerHour, start, end, reason)
	if updateErr != nil {
		return fmt.Errorf("batch update compensatory leave quota failed: %w", updateErr)
	}

	logrus.Infof("[comp-leave-sync] batch update success userID=%s workDate=%s minutes=%d requestID=%s",
		userID, workDate, minutes, requestID)
	return nil
}

func SetCompensatoryLeaveQuota(userID string, year int, totalMinutes int, reason string) error {
	return SetCompensatoryLeaveQuotaForOrg(database.DefaultOrganizationID, userID, year, totalMinutes, reason)
}

func SetCompensatoryLeaveQuotaForOrg(orgID, userID string, year int, totalMinutes int, reason string) error {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return err
	}
	return setCompensatoryLeaveQuotaWithConfig(cfg, userID, year, totalMinutes, reason)
}

func SetCompensatoryLeaveQuotaForConfig(cfg AppConfig, userID string, year int, totalMinutes int, reason string) error {
	return setCompensatoryLeaveQuotaWithConfig(configFromAppConfig(cfg), userID, year, totalMinutes, reason)
}

func setCompensatoryLeaveQuotaWithConfig(cfg Config, userID string, year int, totalMinutes int, reason string) error {
	if totalMinutes < 0 {
		return fmt.Errorf("totalMinutes cannot be negative")
	}
	opUserID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	if opUserID == "" {
		return fmt.Errorf("missing DINGTALK_ADMIN_USER_ID")
	}
	leaveCode, hoursPerDay, err := resolveCompensatoryLeaveTypeWithConfig(cfg, opUserID)
	if err != nil {
		return err
	}
	if hoursPerDay <= 0 {
		hoursPerDay = getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8)
	}
	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return err
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(year, 12, 31, 23, 59, 59, 0, time.Local)
	if err := ensureVacationBalanceInitialized(accessToken, opUserID, userID, leaveCode, year, start, end); err != nil {
		logrus.Warnf("[comp-leave-sync] initialize balance before absolute set failed: %v", err)
	}
	quotaPerHour := int64(math.Round(float64(totalMinutes) / 60.0 * 100))
	quotaPerDay := quotaHourToDay(quotaPerHour, hoursPerDay)
	_, err = updateVacationQuotaOAPI(accessToken, opUserID, userID, leaveCode, year, quotaPerDay, quotaPerHour, start, end, reason)
	return err
}

func quotaHourToDay(quotaPerHour int64, hoursPerDay float64) int64 {
	if hoursPerDay <= 0 {
		hoursPerDay = 8
	}
	return int64(math.Round(float64(quotaPerHour) / hoursPerDay))
}

func ensureVacationBalanceInitialized(accessToken, opUserID, userID, leaveCode string, year int, start, end time.Time) error {
	exists, _, err := getVacationQuotaByYear(accessToken, opUserID, userID, leaveCode, year)
	if err == nil && exists {
		return nil
	}
	if err != nil {
		logrus.Warnf("[comp-leave-sync] query existing leave quota failed, initializing anyway: %v", err)
	}

	if err := initializeVacationBalance(accessToken, opUserID, userID, leaveCode); err == nil {
		return nil
	} else {
		logrus.Warnf("[comp-leave-sync] v1 initialize balance failed, trying oapi init: %v", err)
	}

	return initVacationQuotaOAPI(accessToken, opUserID, userID, leaveCode, year, 0, 0, start, end, "init compensatory leave balance")
}

func initializeVacationBalance(accessToken, opUserID, userID, leaveCode string) error {
	headers := map[string]string{"x-acs-dingtalk-access-token": accessToken}
	initURL := fmt.Sprintf(
		"https://api.dingtalk.com/v1.0/attendance/leaves/initializations/balances?opUserId=%s&userId=%s&leaveCode=%s",
		url.QueryEscape(opUserID), url.QueryEscape(userID), url.QueryEscape(leaveCode),
	)
	resp, err := getJSON(initURL, headers)
	if err != nil {
		return err
	}
	return checkDingTalkV1Response(resp, "initialize leave balance")
}

func updateVacationQuotaOAPI(accessToken, opUserID, userID, leaveCode string, year int, quotaPerDay, quotaPerHour int64, start, end time.Time, reason string) (string, error) {
	body := map[string]interface{}{
		"op_userid": opUserID,
		"leave_quotas": []map[string]interface{}{
			{
				"userid":             userID,
				"leave_code":         leaveCode,
				"quota_num_per_day":  quotaPerDay,
				"quota_num_per_hour": quotaPerHour,
				"quota_cycle":        strconv.Itoa(year),
				"start_time":         start.UnixMilli(),
				"end_time":           end.UnixMilli(),
				"reason":             reason,
			},
		},
	}
	if bodyBytes, mErr := json.Marshal(body); mErr == nil {
		logrus.Infof("[comp-leave-sync] POST /topapi/attendance/vacation/quota/update body=%s", string(bodyBytes))
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/quota/update?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return "", err
	}
	if err := checkVacationQuotaOAPIResponse(resp, "update leave quota"); err != nil {
		return "", err
	}
	return getString(resp, "request_id"), nil
}

func checkDingTalkV1Response(resp map[string]interface{}, action string) error {
	if success, ok := resp["success"].(bool); ok && !success {
		return fmt.Errorf("%s failed: %v", action, resp)
	}
	if code := strings.TrimSpace(getString(resp, "code")); code != "" && code != "0" {
		if message := strings.TrimSpace(getString(resp, "message")); message != "" {
			return fmt.Errorf("%s failed: code=%s message=%s", action, code, message)
		}
		return fmt.Errorf("%s failed: code=%s", action, code)
	}
	return nil
}

func checkVacationQuotaOAPIResponse(resp map[string]interface{}, action string) error {
	if errcode, _ := resp["errcode"].(float64); errcode != 0 {
		return fmt.Errorf("%s failed: %s", action, dingTalkErrorMessage(resp, errcode))
	}
	if success, ok := resp["success"].(bool); ok && !success {
		return fmt.Errorf("%s failed: success=false", action)
	}
	items, ok := resp["result"].([]interface{})
	if !ok {
		return nil
	}
	for _, item := range items {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if reason := strings.TrimSpace(getString(itemMap, "reason")); reason != "" {
			return fmt.Errorf("%s failed: %s", action, reason)
		}
	}
	return nil
}

func getVacationQuotaByYear(accessToken, opUserID, userID, leaveCode string, year int) (bool, int64, error) {
	body := map[string]interface{}{
		"op_userid":  opUserID,
		"userids":    userID,
		"leave_code": leaveCode,
		"offset":     0,
		"size":       10,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/quota/list?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return false, 0, err
	}
	if errcode, _ := resp["errcode"].(float64); errcode != 0 {
		return false, 0, fmt.Errorf("list vacation quota failed: %s", dingTalkErrorMessage(resp, errcode))
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return false, 0, nil
	}
	quotas, ok := result["leave_quotas"].([]interface{})
	if !ok {
		return false, 0, nil
	}
	for _, item := range quotas {
		quota, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if getString(quota, "quota_cycle") != strconv.Itoa(year) {
			continue
		}
		return true, int64(getFloat(quota, "quota_num_per_hour")), nil
	}
	return false, 0, nil
}

// InitVacationQuota 通过 quota/init 接口初始化（重置）指定员工的假期配额。
// quotaPerDay / quotaPerHour 单位均为 1/100（例如 0 = 0天，100 = 1天，800 = 1天×8小时）。
// year 决定生效的配额周期（start_time / end_time 自动设为该年 1-1 到 12-31）。
func InitVacationQuota(userID, leaveCode string, year int, quotaPerDay, quotaPerHour int64, reason string) error {
	return InitVacationQuotaForOrg(database.DefaultOrganizationID, userID, leaveCode, year, quotaPerDay, quotaPerHour, reason)
}

func InitVacationQuotaForOrg(orgID, userID, leaveCode string, year int, quotaPerDay, quotaPerHour int64, reason string) error {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return err
	}
	return initVacationQuotaWithConfig(cfg, userID, leaveCode, year, quotaPerDay, quotaPerHour, reason)
}

func InitVacationQuotaForConfig(cfg AppConfig, userID, leaveCode string, year int, quotaPerDay, quotaPerHour int64, reason string) error {
	return initVacationQuotaWithConfig(configFromAppConfig(cfg), userID, leaveCode, year, quotaPerDay, quotaPerHour, reason)
}

func initVacationQuotaWithConfig(cfg Config, userID, leaveCode string, year int, quotaPerDay, quotaPerHour int64, reason string) error {
	opUserID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID"))
	if opUserID == "" {
		return fmt.Errorf("missing DINGTALK_ADMIN_USER_ID")
	}
	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return err
	}
	start := time.Date(year, 1, 1, 0, 0, 0, 0, time.Local)
	end := time.Date(year, 12, 31, 23, 59, 59, 0, time.Local)
	return initVacationQuotaOAPI(accessToken, opUserID, userID, leaveCode, year, quotaPerDay, quotaPerHour, start, end, reason)
}

func initVacationQuotaOAPI(accessToken, opUserID, userID, leaveCode string, year int, quotaPerDay, quotaPerHour int64, start, end time.Time, reason string) error {
	body := map[string]interface{}{
		"op_userid": opUserID,
		"leave_quotas": map[string]interface{}{
			"userid":             userID,
			"leave_code":         leaveCode,
			"quota_num_per_day":  quotaPerDay,
			"quota_num_per_hour": quotaPerHour,
			"quota_cycle":        strconv.Itoa(year),
			"start_time":         start.UnixMilli(),
			"end_time":           end.UnixMilli(),
			"reason":             reason,
		},
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/quota/init?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return err
	}
	return checkVacationQuotaOAPIResponse(resp, "init leave quota")
}

type cachedLeaveType struct {
	leaveCode   string
	hoursPerDay float64
	expiry      time.Time
}

var (
	annualLeaveTypeCache         = make(map[string]cachedLeaveType)
	annualLeaveTypeCacheMu       sync.Mutex
	compensatoryLeaveTypeCache   = make(map[string]cachedLeaveType)
	compensatoryLeaveTypeCacheMu sync.Mutex
)

const defaultCompensatoryLeaveCode = "fd5600a2-d0df-4d9f-8022-7e5f0833130c"

func resolveAnnualLeaveType(opUserID string) (string, float64, error) {
	return resolveAnnualLeaveTypeForOrg(database.DefaultOrganizationID, opUserID)
}

func resolveAnnualLeaveTypeForOrg(orgID, opUserID string) (string, float64, error) {
	orgID = database.NormalizeOrganizationID(orgID)
	if code := strings.TrimSpace(os.Getenv("DINGTALK_ANNUAL_LEAVE_CODE")); code != "" {
		return code, getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8), nil
	}

	annualLeaveTypeCacheMu.Lock()
	defer annualLeaveTypeCacheMu.Unlock()

	if cached := annualLeaveTypeCache[orgID]; cached.leaveCode != "" && time.Now().Before(cached.expiry) {
		return cached.leaveCode, cached.hoursPerDay, nil
	}

	leaveName := strings.TrimSpace(os.Getenv("DINGTALK_ANNUAL_LEAVE_NAME"))
	if leaveName == "" {
		leaveName = "年假"
	}
	types, err := ListVacationTypesForOrg(orgID, opUserID)
	if err != nil {
		return "", 0, err
	}
	for _, item := range types {
		if item.LeaveName == leaveName {
			annualLeaveTypeCache[orgID] = cachedLeaveType{
				leaveCode:   item.LeaveCode,
				hoursPerDay: item.HoursInPerDay,
				expiry:      time.Now().Add(time.Hour),
			}
			return item.LeaveCode, item.HoursInPerDay, nil
		}
	}
	return "", 0, fmt.Errorf("annual leave type %q not found in DingTalk; set DINGTALK_ANNUAL_LEAVE_CODE", leaveName)
}

func createCompensatoryLeaveType(opUserID, leaveName string) (string, error) {
	return createCompensatoryLeaveTypeForOrg(database.DefaultOrganizationID, opUserID, leaveName)
}

func createCompensatoryLeaveTypeForOrg(orgID, opUserID, leaveName string) (string, error) {
	return CreateCustomLeaveTypeForOrg(orgID, opUserID, leaveName, false)
}

func createCompensatoryLeaveTypeForConfig(opUserID, leaveName string, cfg AppConfig) (string, error) {
	return CreateCustomLeaveTypeForConfig(opUserID, leaveName, false, cfg)
}

func CreateCustomLeaveType(opUserID, leaveName string, freedomLeave bool) (string, error) {
	return CreateCustomLeaveTypeForOrg(database.DefaultOrganizationID, opUserID, leaveName, freedomLeave)
}

func CreateCustomLeaveTypeForOrg(orgID, opUserID, leaveName string, freedomLeave bool) (string, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return "", err
	}
	return createCustomLeaveTypeWithConfig(cfg, opUserID, leaveName, freedomLeave)
}

func CreateCustomLeaveTypeForConfig(opUserID, leaveName string, freedomLeave bool, cfg AppConfig) (string, error) {
	return createCustomLeaveTypeWithConfig(configFromAppConfig(cfg), opUserID, leaveName, freedomLeave)
}

func createCustomLeaveTypeWithConfig(cfg Config, opUserID, leaveName string, freedomLeave bool) (string, error) {
	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return "", err
	}
	hoursPerDay := getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8)
	body := map[string]interface{}{
		"op_userid":         opUserID,
		"leave_name":        leaveName,
		"leave_view_unit":   "hour",
		"biz_type":          "general_leave",
		"natural_day_leave": false,
		"hours_in_per_day":  int64(math.Round(hoursPerDay * 100)),
		"paid_leave":        false,
		"freedom_leave":     freedomLeave,
		"extras":            `{"validity_type":"absolute_time","validity_value":"12-31"}`,
		"submit_time_rule": map[string]interface{}{
			"enable_time_limit": false,
		},
		"leave_certificate": map[string]interface{}{
			"enable": false,
		},
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/vacation/type/create?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return "", fmt.Errorf("create leave type failed: %w", err)
	}
	if errcode, _ := resp["errcode"].(float64); errcode != 0 {
		return "", fmt.Errorf("create leave type failed: %s", dingTalkErrorMessage(resp, errcode))
	}
	leaveCode := extractCreatedLeaveCode(resp)
	if leaveCode == "" {
		return "", fmt.Errorf("create leave type: missing leaveCode in response: %v", resp)
	}
	logrus.Infof("[leave-sync] created custom leave type: name=%s leaveCode=%s freedomLeave=%t", leaveName, leaveCode, freedomLeave)
	return leaveCode, nil
}

func extractCreatedLeaveCode(resp map[string]interface{}) string {
	if code := getString(resp, "leaveCode"); code != "" {
		return code
	}
	if code := getString(resp, "leave_code"); code != "" {
		return code
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return ""
	}
	if code := getString(result, "leaveCode"); code != "" {
		return code
	}
	return getString(result, "leave_code")
}

func resolveCompensatoryLeaveType(opUserID string) (string, float64, error) {
	return resolveCompensatoryLeaveTypeForOrg(database.DefaultOrganizationID, opUserID)
}

func resolveCompensatoryLeaveTypeForOrg(orgID, opUserID string) (string, float64, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return "", 0, err
	}
	return resolveCompensatoryLeaveTypeWithConfig(cfg, opUserID)
}

func resolveCompensatoryLeaveTypeForConfig(opUserID string, cfg AppConfig) (string, float64, error) {
	return resolveCompensatoryLeaveTypeWithConfig(configFromAppConfig(cfg), opUserID)
}

func resolveCompensatoryLeaveTypeWithConfig(cfg Config, opUserID string) (string, float64, error) {
	cfg = cfg.normalized()
	orgID := cfg.OrgID
	if code := strings.TrimSpace(os.Getenv("DINGTALK_LIEU_LEAVE_CODE")); code != "" {
		return code, getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8), nil
	}
	if code := strings.TrimSpace(os.Getenv("DINGTALK_COMPENSATORY_LEAVE_CODE")); code != "" {
		return code, getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8), nil
	}
	if defaultCompensatoryLeaveCode != "" {
		return defaultCompensatoryLeaveCode, getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8), nil
	}

	compensatoryLeaveTypeCacheMu.Lock()
	defer compensatoryLeaveTypeCacheMu.Unlock()

	if cached := compensatoryLeaveTypeCache[orgID]; cached.leaveCode != "" && time.Now().Before(cached.expiry) {
		return cached.leaveCode, cached.hoursPerDay, nil
	}

	leaveName := strings.TrimSpace(os.Getenv("DINGTALK_LIEU_LEAVE_NAME"))
	if leaveName == "" {
		leaveName = strings.TrimSpace(os.Getenv("DINGTALK_COMPENSATORY_LEAVE_NAME"))
	}
	if leaveName == "" {
		leaveName = "手动发放"
	}

	types, err := listVacationTypesWithConfig(cfg, opUserID)
	if err != nil {
		return "", 0, err
	}

	// 优先查找所有类型的假期，不限制source和bizType
	hoursPerDay := getEnvFloat("DINGTALK_LEAVE_HOURS_PER_DAY", 8)
	for _, item := range types {
		if item.LeaveName == leaveName {
			if item.HoursInPerDay > 0 {
				hoursPerDay = item.HoursInPerDay
			}
			compensatoryLeaveTypeCache[orgID] = cachedLeaveType{
				leaveCode:   item.LeaveCode,
				hoursPerDay: hoursPerDay,
				expiry:      time.Now().Add(time.Hour),
			}
			logrus.Infof("[leave-sync] found existing compensatory leave type: name=%s leaveCode=%s source=%s bizType=%s", item.LeaveName, item.LeaveCode, item.Source, item.BizType)
			return item.LeaveCode, hoursPerDay, nil
		}
	}

	// 没有找到符合条件的假期类型，自动通过 API 创建一个
	logrus.Infof("[leave-sync] no compensatory leave type found, creating one: name=%s", leaveName)
	leaveCode, err := createCustomLeaveTypeWithConfig(cfg, opUserID, leaveName, false)
	if err != nil {
		// 如果创建失败，可能是因为已存在相同名称的假期类型
		// 重新获取假期类型列表并查找已存在的假期类型
		logrus.Warnf("[leave-sync] create leave type failed, trying to find existing one: %v", err)
		types, err := listVacationTypesWithConfig(cfg, opUserID)
		if err != nil {
			return "", 0, fmt.Errorf("auto-create compensatory leave type %q failed and cannot find existing one: %w", leaveName, err)
		}
		// 再次查找所有类型的假期
		for _, item := range types {
			if item.LeaveName == leaveName {
				if item.HoursInPerDay > 0 {
					hoursPerDay = item.HoursInPerDay
				}
				compensatoryLeaveTypeCache[orgID] = cachedLeaveType{
					leaveCode:   item.LeaveCode,
					hoursPerDay: hoursPerDay,
					expiry:      time.Now().Add(time.Hour),
				}
				logrus.Infof("[leave-sync] found existing compensatory leave type after creation attempt: name=%s leaveCode=%s source=%s bizType=%s", item.LeaveName, item.LeaveCode, item.Source, item.BizType)
				return item.LeaveCode, hoursPerDay, nil
			}
		}
		return "", 0, fmt.Errorf("auto-create compensatory leave type %q failed: %w", leaveName, err)
	}
	compensatoryLeaveTypeCache[orgID] = cachedLeaveType{
		leaveCode:   leaveCode,
		hoursPerDay: hoursPerDay,
		expiry:      time.Now().Add(time.Hour),
	}
	logrus.Infof("[leave-sync] created new compensatory leave type: name=%s leaveCode=%s", leaveName, leaveCode)
	return leaveCode, hoursPerDay, nil
}

func getEnvFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func dingTalkErrorMessage(resp map[string]interface{}, errcode float64) string {
	parts := make([]string, 0, 3)
	if errmsg := strings.TrimSpace(getString(resp, "errmsg")); errmsg != "" {
		parts = append(parts, errmsg)
	}
	if subMsg := strings.TrimSpace(getString(resp, "sub_msg")); subMsg != "" {
		parts = append(parts, subMsg)
	}
	if len(parts) == 0 {
		return fmt.Sprintf("unknown errcode %.0f", errcode)
	}
	return strings.Join(parts, "; ")
}

func resolveDingTalkPosition(raw map[string]interface{}) (string, string) {
	return stringFromMappedPaths(raw, configuredDingTalkFieldPaths("DINGTALK_POSITION_FIELD_KEYS", defaultPositionFieldPaths()))
}

func resolveDingTalkDirectManagerID(raw map[string]interface{}) (string, string) {
	return stringFromMappedPaths(raw, configuredDingTalkFieldPaths("DINGTALK_DIRECT_MANAGER_FIELD_KEYS", defaultDirectManagerFieldPaths()))
}

func resolveDingTalkDirectManagerName(raw map[string]interface{}) (string, string) {
	return stringFromMappedPaths(raw, configuredDingTalkFieldPaths("DINGTALK_DIRECT_MANAGER_NAME_FIELD_KEYS", defaultDirectManagerNameFieldPaths()))
}

func defaultPositionFieldPaths() []string {
	return []string{
		"title",
		"position",
		"jobTitle",
		"job_title",
		"extension.title",
		"extension.position",
		"extension.jobTitle",
		"extension.job_title",
		"extension.岗位",
		"extension.职位",
		"extAttrs.title",
		"extAttrs.position",
		"extAttrs.jobTitle",
		"extAttrs.job_title",
		"extAttrs.岗位",
		"extAttrs.职位",
		"ext_attrs.title",
		"ext_attrs.position",
		"ext_attrs.岗位",
		"ext_attrs.职位",
	}
}

func defaultDirectManagerFieldPaths() []string {
	return []string{
		"manager_userid",
		"manager_user_id",
		"managerUserid",
		"managerUserId",
		"leader_userid",
		"leader_user_id",
		"supervisor_userid",
		"supervisor_user_id",
		"extension.manager_userid",
		"extension.manager_user_id",
		"extension.leader_userid",
		"extension.leader_user_id",
		"extension.supervisor_userid",
		"extension.supervisor_user_id",
		"extAttrs.manager_user_id",
		"extAttrs.leader_user_id",
		"extAttrs.supervisor_user_id",
		"ext_attrs.manager_user_id",
		"ext_attrs.leader_user_id",
		"ext_attrs.supervisor_user_id",
	}
}

func defaultDirectManagerNameFieldPaths() []string {
	return []string{
		"manager_name",
		"managerName",
		"leader_name",
		"leaderName",
		"supervisor_name",
		"supervisorName",
		"extension.manager_name",
		"extension.managerName",
		"extension.leader_name",
		"extension.leaderName",
		"extension.supervisor_name",
		"extension.supervisorName",
		"extAttrs.manager_name",
		"extAttrs.leader_name",
		"extAttrs.supervisor_name",
		"ext_attrs.manager_name",
		"ext_attrs.leader_name",
		"ext_attrs.supervisor_name",
	}
}

func defaultDepartmentHeadFieldPaths() []string {
	return []string{
		"dept_manager_userid_list",
		"dept_manager_userids",
		"deptManagerUseridList",
		"manager_userid_list",
		"manager_user_id_list",
		"leader_userid_list",
		"head_userid_list",
		"owner_userid",
		"extension.dept_manager_userid_list",
		"extension.department_head_user_ids",
		"extension.department_head_user_id",
	}
}

func configuredDingTalkFieldPaths(envKey string, defaults []string) []string {
	configured := splitConfiguredList(os.Getenv(envKey))
	if len(configured) == 0 {
		return uniqueNonEmptyStrings(defaults)
	}
	return uniqueNonEmptyStrings(append(configured, defaults...))
}

func splitConfiguredList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\r' || r == '\t'
	})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}
	return values
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := normalizeFieldName(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func stringFromMappedPaths(raw map[string]interface{}, paths []string) (string, string) {
	for _, path := range paths {
		value := stringFromDynamicPath(raw, strings.Split(path, "."))
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), path
		}
	}
	return "", ""
}

func stringSliceFromMappedPaths(raw map[string]interface{}, paths []string) []string {
	for _, path := range paths {
		values := stringSliceFromDynamicPath(raw, strings.Split(path, "."))
		if len(values) > 0 {
			return uniqueNonEmptyStrings(values)
		}
	}
	return nil
}

func stringFromDynamicPath(value interface{}, parts []string) string {
	if len(parts) == 0 {
		return stringFromDynamicValue(value)
	}
	switch current := value.(type) {
	case map[string]interface{}:
		if child, ok := mapValueByFieldName(current, parts[0]); ok {
			if value := stringFromDynamicPath(child, parts[1:]); strings.TrimSpace(value) != "" {
				return value
			}
		}
		if len(parts) == 1 {
			return stringFromDingTalkAttribute(current, parts[0])
		}
	case []interface{}:
		if attrValue := stringFromDingTalkAttributeList(current, parts[0]); strings.TrimSpace(attrValue) != "" {
			return attrValue
		}
		for _, item := range current {
			if value := stringFromDynamicPath(item, parts); strings.TrimSpace(value) != "" {
				return value
			}
		}
	}
	return ""
}

func stringSliceFromDynamicPath(value interface{}, parts []string) []string {
	if len(parts) == 0 {
		return stringSliceFromDynamicValue(value)
	}
	switch current := value.(type) {
	case map[string]interface{}:
		if child, ok := mapValueByFieldName(current, parts[0]); ok {
			if values := stringSliceFromDynamicPath(child, parts[1:]); len(values) > 0 {
				return values
			}
		}
	case []interface{}:
		for _, item := range current {
			if values := stringSliceFromDynamicPath(item, parts); len(values) > 0 {
				return values
			}
		}
	}
	return nil
}

func mapValueByFieldName(raw map[string]interface{}, key string) (interface{}, bool) {
	if value, ok := raw[key]; ok {
		return value, true
	}
	target := normalizeFieldName(key)
	for rawKey, value := range raw {
		if normalizeFieldName(rawKey) == target {
			return value, true
		}
	}
	return nil, false
}

func normalizeFieldName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, " ", "")
	return value
}

func stringFromDingTalkAttribute(raw map[string]interface{}, wantedKey string) string {
	label := firstNonEmptyStringValue(
		raw["field_code"],
		raw["fieldCode"],
		raw["field_name"],
		raw["fieldName"],
		raw["name"],
		raw["label"],
		raw["key"],
		raw["code"],
	)
	if label == "" || normalizeFieldName(label) != normalizeFieldName(wantedKey) {
		return ""
	}
	return firstNonEmptyStringValue(
		raw["value"],
		raw["field_value"],
		raw["fieldValue"],
		raw["text"],
		raw["label_value"],
		raw["labelValue"],
		raw["field_value_list"],
		raw["fieldValueList"],
	)
}

func stringFromDingTalkAttributeList(list []interface{}, wantedKey string) string {
	for _, item := range list {
		raw, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if value := stringFromDingTalkAttribute(raw, wantedKey); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyStringValue(values ...interface{}) string {
	for _, value := range values {
		if text := stringFromDynamicValue(value); strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func stringFromDynamicValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == 0 {
			return ""
		}
		return strconv.FormatInt(int64(v), 10)
	case int64:
		if v == 0 {
			return ""
		}
		return strconv.FormatInt(v, 10)
	case int:
		if v == 0 {
			return ""
		}
		return strconv.Itoa(v)
	case bool:
		return strconv.FormatBool(v)
	case []interface{}:
		for _, item := range v {
			if text := stringFromDynamicValue(item); strings.TrimSpace(text) != "" {
				return text
			}
		}
	case map[string]interface{}:
		return firstNonEmptyStringValue(v["value"], v["label"], v["name"], v["text"])
	}
	return ""
}

func stringSliceFromDynamicValue(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		parts := strings.FieldsFunc(v, func(r rune) bool {
			return r == ',' || r == ';' || r == '，' || r == '；' || r == '\n' || r == '\r'
		})
		values := make([]string, 0, len(parts))
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				values = append(values, trimmed)
			}
		}
		if len(values) == 0 && strings.TrimSpace(v) != "" {
			values = append(values, strings.TrimSpace(v))
		}
		return values
	case []interface{}:
		values := make([]string, 0, len(v))
		for _, item := range v {
			values = append(values, stringSliceFromDynamicValue(item)...)
		}
		return values
	default:
		if text := stringFromDynamicValue(value); text != "" {
			return []string{text}
		}
	}
	return nil
}

func buildPositionSyncDiagnostic(apiName string, raw map[string]interface{}, source, position, failureReason string) map[string]interface{} {
	status := "mapped"
	if strings.TrimSpace(position) == "" {
		status = "missing"
		if failureReason == "" {
			failureReason = "DingTalk response did not include a value matching the configured position field mapping; verify contact/HRM permissions and DINGTALK_POSITION_FIELD_KEYS or DINGTALK_HRM_POSITION_FIELD_CODES."
		}
	}
	diag := map[string]interface{}{
		"status":                   status,
		"api":                      apiName,
		"position":                 position,
		"position_mapping_field":   source,
		"position_mapping_fields":  configuredDingTalkFieldPaths("DINGTALK_POSITION_FIELD_KEYS", defaultPositionFieldPaths()),
		"hrm_position_field_codes": splitConfiguredList(os.Getenv("DINGTALK_HRM_POSITION_FIELD_CODES")),
		"app_permission_note":      "DingTalk app permissions are not introspectable from this response; verify contact user read permissions and HRM roster field permissions when HRM mapping is used.",
		"failure_reason":           failureReason,
	}
	if raw != nil {
		diag["raw_field_keys"] = sortedMapKeys(raw)
		if truthyEnv("DINGTALK_SYNC_STORE_RAW_USER") {
			diag["raw_response"] = raw
		}
	}
	return diag
}

func logDingTalkUserFieldDiagnostic(userID string, raw map[string]interface{}, position, positionSource, managerUserID, managerSource string) {
	if truthyEnv("DINGTALK_SYNC_LOG_RAW_USER") {
		logrus.Infof("[dingtalk/user/list] raw_user_response user_id=%s payload=%s", userID, compactJSON(raw))
	}
	if strings.TrimSpace(position) == "" {
		logrus.Warnf("[dingtalk/user/list] position missing user_id=%s fields=%v mapping_fields=%v hrm_field_codes=%v reason=%s",
			userID,
			sortedMapKeys(raw),
			configuredDingTalkFieldPaths("DINGTALK_POSITION_FIELD_KEYS", defaultPositionFieldPaths()),
			splitConfiguredList(os.Getenv("DINGTALK_HRM_POSITION_FIELD_CODES")),
			"DingTalk response did not include a matched position field or the app lacks the required field permission",
		)
	} else {
		logrus.Infof("[dingtalk/user/list] position mapped user_id=%s source=%s", userID, positionSource)
	}
	if strings.TrimSpace(managerUserID) == "" {
		logrus.Infof("[dingtalk/user/list] direct manager missing user_id=%s fields=%v mapping_fields=%v; local manager will be preserved unless overwrite is explicitly requested",
			userID,
			sortedMapKeys(raw),
			configuredDingTalkFieldPaths("DINGTALK_DIRECT_MANAGER_FIELD_KEYS", defaultDirectManagerFieldPaths()),
		)
	} else {
		logrus.Infof("[dingtalk/user/list] direct manager mapped user_id=%s manager_user_id=%s source=%s", userID, managerUserID, managerSource)
	}
}

func sortedMapKeys(raw map[string]interface{}) []string {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func compactJSON(value interface{}) string {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(data)
}

func truthyEnv(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

// defaultHRMFieldCodes returns the standard DingTalk HRM system field codes that
// should always be requested in field_filter_list so the API returns them.
// Reference: https://open.dingtalk.com/document/isvapp/get-roster-field-group-details
func defaultHRMFieldCodes(fieldKey string) []string {
	switch fieldKey {
	case hrmFieldEmploymentType:
		return []string{"sys01-employeeType"}
	case hrmFieldJobLevel:
		return []string{"sys01-positionLevel"}
	case hrmFieldPosition:
		return []string{"sys00-position"}
	default:
		return nil
	}
}

func configuredHRMFieldCodes(cfg Config) []string {
	cfg = cfg.normalized()
	codes := []string{"sys01-planRegularTime", "sys01-regularTime"}
	for _, key := range supportedHRMFieldKeys {
		codes = append(codes, defaultHRMFieldCodes(key)...)
		codes = append(codes, cfg.HRMFieldCodes[key]...)
	}
	return uniqueNonEmptyStrings(codes)
}

func matchesConfiguredHRMField(field map[string]interface{}, cfg Config, fieldKey string) bool {
	cfg = cfg.normalized()
	identifiers := []string{
		getString(field, "field_code"),
		getString(field, "fieldCode"),
		getString(field, "field_name"),
		getString(field, "fieldName"),
		getString(field, "name"),
		getString(field, "label"),
	}
	candidates := append([]string{}, defaultHRMFieldCodes(fieldKey)...)
	candidates = append(candidates, cfg.HRMFieldCodes[fieldKey]...)
	candidates = append(candidates, cfg.HRMFieldNames[fieldKey]...)
	candidates = append(candidates, defaultHRMFieldNames(fieldKey)...)
	for _, identifier := range identifiers {
		if strings.TrimSpace(identifier) == "" {
			continue
		}
		for _, candidate := range candidates {
			if normalizeFieldName(identifier) == normalizeFieldName(candidate) {
				return true
			}
		}
	}
	return false
}

func defaultHRMFieldNames(fieldKey string) []string {
	switch fieldKey {
	case hrmFieldPosition:
		return []string{"岗位", "职位", "position", "jobTitle", "job_title", "title"}
	case hrmFieldEmploymentType:
		return []string{"员工类型", "雇佣类型", "用工类型", "人员类型", "employmentType", "employment_type"}
	case hrmFieldJobLevel:
		return []string{"职级", "职等", "岗位等级", "jobLevel", "job_level"}
	case hrmFieldJobFamily:
		return []string{"岗位序列", "职位序列", "职族", "职类", "人员类别", "jobFamily", "job_family"}
	case hrmFieldProbationEndDate:
		return []string{"试用期结束日期", "试用期到期日", "试用结束日期", "probationEndDate", "probation_end_date"}
	default:
		return nil
	}
}

func extractHRMTextFieldValue(field map[string]interface{}) string {
	values, ok := field["field_value_list"].([]interface{})
	if !ok {
		values, ok = field["fieldValueList"].([]interface{})
	}
	if ok {
		for _, item := range values {
			valueMap, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if value := firstNonEmptyStringValue(valueMap["value"], valueMap["label"], valueMap["text"], valueMap["name"]); value != "" {
				return value
			}
		}
	}
	return firstNonEmptyStringValue(field["value"], field["label"], field["text"], field["name"])
}

func resolveHRMEmploymentType(field map[string]interface{}, cfg Config) (string, string) {
	rawValue, responseLabel := extractHRMOptionValue(field)
	if rawValue == "" && responseLabel == "" {
		return "", ""
	}
	if responseLabel != "" && responseLabel != rawValue {
		return rawValue, responseLabel
	}
	if rawValue != "" {
		if label := strings.TrimSpace(cfg.normalized().HRMFieldOptions[hrmFieldEmploymentType][rawValue]); label != "" {
			return rawValue, label
		}
		if containsHanText(rawValue) {
			return "", rawValue
		}
		return rawValue, fmt.Sprintf("未知类型（代码：%s）", rawValue)
	}
	return "", responseLabel
}

func extractHRMOptionValue(field map[string]interface{}) (string, string) {
	values, ok := field["field_value_list"].([]interface{})
	if !ok {
		values, _ = field["fieldValueList"].([]interface{})
	}
	for _, item := range values {
		valueMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		rawValue := scalarStringPreserveZero(valueMap["value"])
		if rawValue == "" {
			rawValue = scalarStringPreserveZero(valueMap["code"])
		}
		label := firstNonEmptyStringValue(
			valueMap["label"], valueMap["text"], valueMap["name"],
			valueMap["field_name"], valueMap["fieldName"],
			valueMap["option_name"], valueMap["optionName"],
		)
		if label == "" && rawValue != "" {
			label = findHRMOptionLabel(field, rawValue)
		}
		if rawValue != "" || label != "" {
			return rawValue, label
		}
	}
	rawValue := scalarStringPreserveZero(field["value"])
	label := firstNonEmptyStringValue(field["label"], field["text"], field["option_name"], field["optionName"])
	if label == "" && rawValue != "" {
		label = findHRMOptionLabel(field, rawValue)
	}
	return rawValue, label
}

func findHRMOptionLabel(field map[string]interface{}, code string) string {
	for _, key := range []string{
		"options", "option_list", "optionList", "field_options", "fieldOptions",
		"option_data_list", "optionDataList", "field_value_options", "fieldValueOptions",
	} {
		switch options := field[key].(type) {
		case map[string]interface{}:
			switch option := options[code].(type) {
			case map[string]interface{}:
				if label := firstNonEmptyStringValue(option["label"], option["text"], option["name"], option["field_name"], option["fieldName"]); label != "" {
					return label
				}
			default:
				if label := firstNonEmptyStringValue(option); label != "" {
					return label
				}
			}
		case []interface{}:
			for _, option := range options {
				optionMap, ok := option.(map[string]interface{})
				if !ok {
					continue
				}
				optionCode := firstNonEmptyPreserveZero(optionMap["value"], optionMap["code"], optionMap["id"], optionMap["key"])
				if optionCode != code {
					continue
				}
				if label := firstNonEmptyStringValue(optionMap["label"], optionMap["text"], optionMap["name"], optionMap["field_name"], optionMap["fieldName"]); label != "" {
					return label
				}
			}
		}
	}
	return ""
}

func firstNonEmptyPreserveZero(values ...interface{}) string {
	for _, value := range values {
		if text := scalarStringPreserveZero(value); text != "" {
			return text
		}
	}
	return ""
}

func scalarStringPreserveZero(value interface{}) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(typed), 'f', -1, 32)
	case int:
		return strconv.Itoa(typed)
	case int8:
		return strconv.FormatInt(int64(typed), 10)
	case int16:
		return strconv.FormatInt(int64(typed), 10)
	case int32:
		return strconv.FormatInt(int64(typed), 10)
	case int64:
		return strconv.FormatInt(typed, 10)
	case uint:
		return strconv.FormatUint(uint64(typed), 10)
	case uint8:
		return strconv.FormatUint(uint64(typed), 10)
	case uint16:
		return strconv.FormatUint(uint64(typed), 10)
	case uint32:
		return strconv.FormatUint(uint64(typed), 10)
	case uint64:
		return strconv.FormatUint(typed, 10)
	default:
		return ""
	}
}

func containsHanText(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func resolveManagerNames(users map[string]UserInfo) {
	for userID, user := range users {
		if strings.TrimSpace(user.ManagerUserID) == "" || strings.TrimSpace(user.ManagerName) != "" {
			continue
		}
		manager, ok := users[user.ManagerUserID]
		if !ok {
			continue
		}
		user.ManagerName = manager.Name
		users[userID] = user
	}
}

func fetchDeptUsers(accessToken string, deptID int64) ([]UserInfo, error) {
	var allUsers []UserInfo
	cursor := 0

	for {
		body := map[string]interface{}{
			"dept_id": deptID,
			"cursor":  cursor,
			"size":    100,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/v2/user/list?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, err
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, dingTalkDepartmentAPIError("fetch department users", errcode, errmsg)
		}

		rawResult, exists := resp["result"]
		if !exists {
			return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工数据格式异常", errors.New("department users response missing result"))
		}
		result, ok := rawResult.(map[string]interface{})
		if !ok {
			return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工数据格式异常", errors.New("department users result has invalid type"))
		}

		rawList, exists := result["list"]
		if !exists {
			return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工数据格式异常", errors.New("department users result missing list"))
		}
		list, ok := rawList.([]interface{})
		if !ok {
			return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工数据格式异常", errors.New("department users list has invalid type"))
		}

		for _, item := range list {
			m, ok := item.(map[string]interface{})
			if !ok {
				return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工数据格式异常", errors.New("department users item has invalid type"))
			}
			position, positionSource := resolveDingTalkPosition(m)
			managerUserID, managerSource := resolveDingTalkDirectManagerID(m)
			managerName, _ := resolveDingTalkDirectManagerName(m)
			user := UserInfo{
				UserID:                 getString(m, "userid"),
				Name:                   getString(m, "name"),
				Email:                  getString(m, "email"),
				Mobile:                 getString(m, "mobile"),
				Position:               position,
				PositionSource:         positionSource,
				PositionSyncDiagnostic: buildPositionSyncDiagnostic("topapi/v2/user/list", m, positionSource, position, ""),
				ManagerUserID:          managerUserID,
				ManagerName:            managerName,
				ManagerSource:          managerSource,
				Avatar:                 getString(m, "avatar"),
				Active:                 getBool(m, "active"),
			}
			if strings.TrimSpace(user.UserID) == "" {
				return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工数据格式异常", errors.New("department users item missing userid"))
			}
			logDingTalkUserFieldDiagnostic(user.UserID, m, user.Position, user.PositionSource, user.ManagerUserID, user.ManagerSource)

			// hired_date 是毫秒时间戳，转成 YYYY-MM-DD
			if ts, ok := m["hired_date"].(float64); ok && ts > 0 {
				user.HiredDate = time.UnixMilli(int64(ts)).Format("2006-01-02")
			}

			// 澶勭悊绌?email 鐨勬儏鍐碉紝鐢熸垚鍞竴 email
			if user.Email == "" {
				user.Email = user.UserID + "@dingtalk.com"
			}

			// 手机号缺失时保持为空，由持久化层写入 NULL；禁止使用共享占位手机号，
			// 否则会触发租户内手机号唯一索引冲突。
			user.Mobile = strings.TrimSpace(user.Mobile)
			if deptList, ok := m["dept_id_list"].([]interface{}); ok && len(deptList) > 0 {
				for _, d := range deptList {
					if id, ok := d.(float64); ok {
						user.DeptIDList = append(user.DeptIDList, int64(id))
					}
				}
			}
			user.DeptIDList = mergeDingTalkDepartmentIDs(user.DeptIDList, []int64{deptID})
			allUsers = append(allUsers, user)
		}

		hasMore, ok := result["has_more"].(bool)
		if !ok {
			return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工数据格式异常", errors.New("department users result missing has_more"))
		}
		if !hasMore {
			break
		}
		nextCursor, ok := result["next_cursor"].(float64)
		if !ok || int(nextCursor) == cursor {
			return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回的员工分页游标异常", errors.New("department users next_cursor missing or unchanged"))
		}
		cursor = int(nextCursor)
	}

	return allUsers, nil
}

func mergeDingTalkDepartmentIDs(groups ...[]int64) []int64 {
	result := make([]int64, 0)
	seen := make(map[int64]struct{})
	for _, group := range groups {
		for _, departmentID := range group {
			if departmentID <= 0 {
				continue
			}
			if _, exists := seen[departmentID]; exists {
				continue
			}
			seen[departmentID] = struct{}{}
			result = append(result, departmentID)
		}
	}
	return result
}

// ===================== 鑰冨嫟鍚屾 =====================

// AttendanceRecord 鑰冨嫟璁板綍
type AttendanceRecord struct {
	UserID            string `json:"userId"`
	CheckType         string `json:"checkType"` // OnDuty / OffDuty
	WorkDate          string `json:"workDate"`
	UserCheckTime     string `json:"userCheckTime"`
	BaseCheckTime     string `json:"baseCheckTime"`
	PlanCheckTime     string `json:"planCheckTime"`
	GroupID           int64  `json:"groupId"`
	PlanID            int64  `json:"planId"`
	LocationResult    string `json:"locationResult"` // Normal / Outside
	TimeResult        string `json:"timeResult"`     // Normal / Late / Early
	SourceType        string `json:"sourceType"`
	IsLegal           string `json:"isLegal"`
	InvalidRecordType string `json:"invalidRecordType"`
	InvalidRecordMsg  string `json:"invalidRecordMsg"`
}

// GetAttendance 鑾峰彇鑰冨嫟鏁版嵁
func GetAttendance(userIDs []string, startDate, endDate string) ([]AttendanceRecord, error) {
	return GetAttendanceForOrg(database.DefaultOrganizationID, userIDs, startDate, endDate)
}

func GetAttendanceForOrg(orgID string, userIDs []string, startDate, endDate string) ([]AttendanceRecord, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}

	var allRecords []AttendanceRecord
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid attendance start date %q: %w", startDate, err)
	}
	endDateTime, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid attendance end date %q: %w", endDate, err)
	}
	if endDateTime.Before(start) {
		return nil, fmt.Errorf("attendance end date %s is before start date %s", endDate, startDate)
	}

	for windowStart := start; !windowStart.After(endDateTime); windowStart = windowStart.AddDate(0, 0, 7) {
		windowEnd := windowStart.AddDate(0, 0, 6)
		if windowEnd.After(endDateTime) {
			windowEnd = endDateTime
		}

		// DingTalk listRecord allows at most 50 users per request and a 7-day date range.
		for i := 0; i < len(userIDs); i += 50 {
			batchEnd := i + 50
			if batchEnd > len(userIDs) {
				batchEnd = len(userIDs)
			}
			batch := userIDs[i:batchEnd]

			body := map[string]interface{}{
				"checkDateFrom": windowStart.Format("2006-01-02") + " 00:00:00",
				"checkDateTo":   windowEnd.Format("2006-01-02") + " 23:59:59",
				"userIds":       batch,
				"isI18n":        false,
			}
			resp, err := postJSONOAPI(
				fmt.Sprintf("https://oapi.dingtalk.com/attendance/listRecord?access_token=%s", accessToken),
				body,
			)
			if err != nil {
				return nil, err
			}

			errcode, _ := resp["errcode"].(float64)
			if errcode != 0 {
				return nil, fmt.Errorf("list attendance records failed: %s", dingTalkErrorMessage(resp, errcode))
			}

			recordList, ok := resp["recordresult"].([]interface{})
			if !ok || len(recordList) == 0 {
				continue
			}

			for _, item := range recordList {
				m, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				userID := getString(m, "userId")
				if userID == "" {
					userID = getString(m, "userid")
				}
				record := AttendanceRecord{
					UserID:            userID,
					CheckType:         getString(m, "checkType"),
					WorkDate:          formatDingTalkDateTime(m["workDate"]),
					UserCheckTime:     formatDingTalkDateTime(m["userCheckTime"]),
					BaseCheckTime:     formatDingTalkDateTime(m["baseCheckTime"]),
					PlanCheckTime:     formatDingTalkDateTime(m["planCheckTime"]),
					GroupID:           int64(getFloat(m, "groupId")),
					PlanID:            int64(getFloat(m, "planId")),
					LocationResult:    getString(m, "locationResult"),
					TimeResult:        getString(m, "timeResult"),
					SourceType:        getString(m, "sourceType"),
					IsLegal:           getString(m, "isLegal"),
					InvalidRecordType: getString(m, "invalidRecordType"),
					InvalidRecordMsg:  getString(m, "invalidRecordMsg"),
				}
				allRecords = append(allRecords, record)
			}
		}
	}

	logrus.Infof("dingtalk sync attendance complete: %d", len(allRecords))
	return allRecords, nil
}

func formatDingTalkDateTime(value interface{}) string {
	switch v := value.(type) {
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil && ts > 0 {
			return formatUnixTime(ts)
		}
		return v
	case float64:
		if v <= 0 {
			return ""
		}
		return formatUnixTime(int64(v))
	case int64:
		if v <= 0 {
			return ""
		}
		return formatUnixTime(v)
	case int:
		if v <= 0 {
			return ""
		}
		return formatUnixTime(int64(v))
	default:
		return ""
	}
}

func formatUnixTime(ts int64) string {
	if ts > 1_000_000_000_000 {
		return time.UnixMilli(ts).In(time.Local).Format("2006-01-02 15:04:05")
	}
	return time.Unix(ts, 0).In(time.Local).Format("2006-01-02 15:04:05")
}

// ===================== 瀹℃壒鍚屾 =====================

// ApprovalInstance 瀹℃壒瀹炰緥
type ApprovalInstance struct {
	ProcessInstanceID string                   `json:"process_instance_id"`
	Title             string                   `json:"title"`
	Status            string                   `json:"status"`
	Result            string                   `json:"result"`
	CreateTime        string                   `json:"create_time"`
	FinishTime        string                   `json:"finish_time"`
	OriginatorUserID  string                   `json:"originator_userid"`
	FormValues        []map[string]interface{} `json:"form_component_values"`
}

// GetApprovals 鑾峰彇瀹℃壒瀹炰緥鍒楄〃
func GetApprovals(processCode, startDate, endDate string) ([]ApprovalInstance, error) {
	return GetApprovalsForOrg(database.DefaultOrganizationID, processCode, startDate, endDate)
}

func GetApprovalsForOrg(orgID, processCode, startDate, endDate string) ([]ApprovalInstance, error) {
	return GetApprovalsForOrgContext(context.Background(), orgID, processCode, startDate, endDate)
}

const (
	approvalQueryMaxWindow = 120 * 24 * time.Hour
	approvalQueryClockSkew = time.Minute
)

type approvalQueryWindow struct {
	Start time.Time
	End   time.Time
}

// ApprovalFetchResult keeps successfully fetched details while reporting
// instance-detail failures separately so callers can persist partial results.
type ApprovalFetchResult struct {
	Instances       []ApprovalInstance
	DetailFailCount int
}

func buildApprovalQueryWindows(startDate, endDate string, now time.Time) ([]approvalQueryWindow, error) {
	location := ApprovalBusinessLocation()
	now = now.In(location)
	start, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), location)
	if err != nil {
		return nil, fmt.Errorf("start_date must use YYYY-MM-DD")
	}
	endDay, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), location)
	if err != nil {
		return nil, fmt.Errorf("end_date must use YYYY-MM-DD")
	}
	if endDay.Before(start) {
		return nil, fmt.Errorf("end_date must not be before start_date")
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if endDay.After(today) {
		return nil, fmt.Errorf("end_date must not be in the future")
	}

	requestedEnd := endDay.AddDate(0, 0, 1)
	safeEnd := now.Add(-approvalQueryClockSkew)
	if requestedEnd.Before(safeEnd) {
		safeEnd = requestedEnd
	}
	if !safeEnd.After(start) {
		return nil, fmt.Errorf("approval query range has not started yet")
	}

	windows := make([]approvalQueryWindow, 0, 1)
	for windowStart := start; windowStart.Before(safeEnd); {
		windowEnd := windowStart.Add(approvalQueryMaxWindow)
		if windowEnd.After(safeEnd) {
			windowEnd = safeEnd
		}
		windows = append(windows, approvalQueryWindow{Start: windowStart, End: windowEnd})
		windowStart = windowEnd
	}
	return windows, nil
}

// GetApprovalsForOrgContext queries one process code in safe 120-day windows.
// IDs are deduplicated across adjacent windows before details are fetched.
func GetApprovalsForOrgContext(ctx context.Context, orgID, processCode, startDate, endDate string) ([]ApprovalInstance, error) {
	result, err := GetApprovalsForOrgContextWithResult(ctx, orgID, processCode, startDate, endDate)
	if err != nil {
		return nil, err
	}
	if result.DetailFailCount > 0 {
		return nil, newSyncError(
			ErrorCodeResponseInvalid,
			"部分审批详情拉取失败",
			fmt.Errorf("approval detail failures: %d", result.DetailFailCount),
		)
	}
	return result.Instances, nil
}

// GetApprovalsForOrgContextWithResult returns successful details and a safe
// failure count. List-ID failures still fail the whole process because no
// complete process snapshot can be established.
func GetApprovalsForOrgContextWithResult(ctx context.Context, orgID, processCode, startDate, endDate string) (ApprovalFetchResult, error) {
	result := ApprovalFetchResult{}
	processCode = strings.TrimSpace(processCode)
	if processCode == "" {
		return result, fmt.Errorf("process_code is required")
	}
	windows, err := buildApprovalQueryWindows(startDate, endDate, time.Now())
	if err != nil {
		return result, err
	}
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return result, err
	}

	instanceIDs := make([]string, 0)
	seenIDs := make(map[string]struct{})
	for _, window := range windows {
		cursor := 0
		for {
			if err := ctx.Err(); err != nil {
				return result, err
			}
			body := map[string]interface{}{
				"process_code": processCode,
				"start_time":   window.Start.UnixMilli(),
				"end_time":     window.End.UnixMilli(),
				"size":         20,
				"cursor":       cursor,
			}
			resp, err := postJSONOAPIContext(ctx,
				fmt.Sprintf("https://oapi.dingtalk.com/topapi/processinstance/listids?access_token=%s", accessToken),
				body,
			)
			if err != nil {
				return result, err
			}

			errcode, _ := resp["errcode"].(float64)
			if errcode != 0 {
				errmsg, _ := resp["errmsg"].(string)
				return result, newSyncError(ErrorCodeResponseInvalid, "钉钉审批查询失败", fmt.Errorf("list approval ids failed: %s", sanitizeDingTalkDiagnostic(errmsg)))
			}

			result, ok := resp["result"].(map[string]interface{})
			if !ok {
				break
			}
			idList, _ := result["list"].([]interface{})
			instanceIDs = appendUniqueApprovalInstanceIDs(instanceIDs, seenIDs, idList)
			nextCursor, _ := result["next_cursor"].(float64)
			if nextCursor == 0 || len(idList) == 0 {
				break
			}
			cursor = int(nextCursor)
		}
	}

	allInstances := make([]ApprovalInstance, 0, len(instanceIDs))
	for _, instanceID := range instanceIDs {
		instance, err := getApprovalDetailContext(ctx, accessToken, instanceID)
		if err != nil {
			if ctx.Err() != nil {
				return result, ctx.Err()
			}
			result.DetailFailCount++
			logrus.Warnf("get approval detail failed process_instance_id=%s error_code=%s", instanceID, SyncErrorCode(err))
			continue
		}
		allInstances = append(allInstances, *instance)
	}
	result.Instances = allInstances
	logrus.Infof("dingtalk sync approvals complete: success=%d failed=%d", len(allInstances), result.DetailFailCount)
	return result, nil
}

func appendUniqueApprovalInstanceIDs(existing []string, seen map[string]struct{}, rawIDs []interface{}) []string {
	for _, rawID := range rawIDs {
		instanceID, ok := rawID.(string)
		instanceID = strings.TrimSpace(instanceID)
		if !ok || instanceID == "" {
			continue
		}
		if _, exists := seen[instanceID]; exists {
			continue
		}
		seen[instanceID] = struct{}{}
		existing = append(existing, instanceID)
	}
	return existing
}

// GetApprovalDetailForOrg 按组织凭证拉取单个审批实例详情。
func GetApprovalDetailForOrg(orgID, instanceID string) (*ApprovalInstance, error) {
	instanceID = strings.TrimSpace(instanceID)
	if instanceID == "" {
		return nil, fmt.Errorf("process_instance_id is required")
	}
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}
	return getApprovalDetail(accessToken, instanceID)
}

func getApprovalDetail(accessToken, instanceID string) (*ApprovalInstance, error) {
	return getApprovalDetailContext(context.Background(), accessToken, instanceID)
}

func getApprovalDetailContext(ctx context.Context, accessToken, instanceID string) (*ApprovalInstance, error) {
	body := map[string]interface{}{
		"process_instance_id": instanceID,
	}
	resp, err := postJSONOAPIContext(ctx,
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/processinstance/get?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, err
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉审批详情拉取失败", fmt.Errorf("get approval detail failed: %s", sanitizeDingTalkDiagnostic(errmsg)))
	}

	pi, ok := resp["process_instance"].(map[string]interface{})
	if !ok {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉审批详情格式异常", errors.New("approval detail response missing process_instance"))
	}

	instance := &ApprovalInstance{
		ProcessInstanceID: instanceID,
		Title:             getString(pi, "title"),
		Status:            getString(pi, "status"),
		Result:            getString(pi, "result"),
		CreateTime:        getString(pi, "create_time"),
		FinishTime:        getString(pi, "finish_time"),
		OriginatorUserID:  getString(pi, "originator_userid"),
	}

	if formValues, ok := pi["form_component_values"].([]interface{}); ok {
		for _, fv := range formValues {
			if m, ok := fv.(map[string]interface{}); ok {
				instance.FormValues = append(instance.FormValues, m)
			}
		}
	}

	return instance, nil
}

// ===================== HTTP 宸ュ叿 =====================

// postJSON 鍙戦€?POST 璇锋眰鍒版柊鐗?API锛坅pi.dingtalk.com锛?
func postJSON(endpoint string, body interface{}, headers map[string]string) (map[string]interface{}, error) {
	return postJSONContext(context.Background(), endpoint, body, headers)
}

func postJSONContext(ctx context.Context, endpoint string, body interface{}, headers map[string]string) (map[string]interface{}, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉请求数据格式异常", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉请求地址异常", fmt.Errorf("create request %s failed: %s", safeDingTalkEndpoint(endpoint), sanitizeDingTalkDiagnostic(err.Error())))
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := dingTalkHTTPClient.Do(req)
	if err != nil {
		return nil, dingTalkNetworkError("POST", endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, dingTalkNetworkError("POST", endpoint, err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		detail := fmt.Errorf("POST %s returned HTTP %d: %s", safeDingTalkEndpoint(endpoint), resp.StatusCode, sanitizeDingTalkDiagnostic(string(data)))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, newSyncError(ErrorCodePermissionDenied, "钉钉应用权限不足", detail)
		}
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉接口返回异常", detail)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回数据格式异常", fmt.Errorf("decode POST %s response failed", safeDingTalkEndpoint(endpoint)))
	}

	return result, nil
}

// postJSONOAPI 鍙戦€?POST 璇锋眰鍒版棫鐗?API锛坥api.dingtalk.com锛?
func postJSONOAPI(url string, body interface{}) (map[string]interface{}, error) {
	return postJSON(url, body, nil)
}

func postJSONOAPIContext(ctx context.Context, url string, body interface{}) (map[string]interface{}, error) {
	return postJSONContext(ctx, url, body, nil)
}

// getJSON 鍙戦€?GET 璇锋眰
func getJSON(endpoint string, headers map[string]string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", endpoint, nil)
	if err != nil {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉请求地址异常", fmt.Errorf("create request %s failed: %s", safeDingTalkEndpoint(endpoint), sanitizeDingTalkDiagnostic(err.Error())))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := dingTalkHTTPClient.Do(req)
	if err != nil {
		return nil, dingTalkNetworkError("GET", endpoint, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, dingTalkNetworkError("GET", endpoint, err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		detail := fmt.Errorf("GET %s returned HTTP %d: %s", safeDingTalkEndpoint(endpoint), resp.StatusCode, sanitizeDingTalkDiagnostic(string(data)))
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return nil, newSyncError(ErrorCodePermissionDenied, "钉钉应用权限不足", detail)
		}
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉接口返回异常", detail)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, newSyncError(ErrorCodeResponseInvalid, "钉钉返回数据格式异常", fmt.Errorf("decode GET %s response failed", safeDingTalkEndpoint(endpoint)))
	}

	return result, nil
}

func safeDingTalkEndpoint(endpoint string) string {
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return sanitizeDingTalkDiagnostic(endpoint)
	}
	query := parsed.Query()
	for key := range query {
		normalized := strings.ToLower(strings.ReplaceAll(key, "_", ""))
		if strings.Contains(normalized, "token") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") || strings.Contains(normalized, "authorization") || strings.Contains(normalized, "cookie") || strings.Contains(normalized, "appkey") {
			query.Set(key, "[REDACTED]")
		}
	}
	parsed.RawQuery = query.Encode()
	return sanitizeDingTalkDiagnostic(parsed.String())
}

func dingTalkNetworkError(method, endpoint string, err error) error {
	detailErr := err
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		detailErr = urlErr.Err
	}
	detail := fmt.Errorf("%s %s failed: %s", method, safeDingTalkEndpoint(endpoint), sanitizeDingTalkDiagnostic(detailErr.Error()))
	var netErr net.Error
	if errors.As(detailErr, &netErr) && netErr.Timeout() {
		detail = fmt.Errorf("%s %s timed out", method, safeDingTalkEndpoint(endpoint))
	}
	return newSyncError(ErrorCodeNetworkFailed, "连接钉钉服务失败", detail)
}

// ===================== 宸ュ叿鍑芥暟 =====================

func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

// ===================== 澶у皬鍛ㄦ帓鐝?API =====================

// GetAttendanceGroups 获取企业考勤组列表（带5分钟本地缓存）
func GetAttendanceGroups() ([]map[string]interface{}, error) {
	attGroupsCacheMu.Lock()
	if attGroupsCache.data != nil && time.Now().Before(attGroupsCache.expiry) {
		cached := attGroupsCache.data
		attGroupsCacheMu.Unlock()
		return cached, nil
	}
	attGroupsCacheMu.Unlock()

	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allGroups []map[string]interface{}
	offset := 0

	for {
		body := map[string]interface{}{
			"offset": offset,
			"size":   10,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/getsimplegroups?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, fmt.Errorf("鑾峰彇鑰冨嫟缁勫け璐? %w", err)
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, fmt.Errorf("鑾峰彇鑰冨嫟缁勫け璐? %s", errmsg)
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}

		groups, ok := result["groups"].([]interface{})
		if !ok || len(groups) == 0 {
			break
		}

		for _, g := range groups {
			if gm, ok := g.(map[string]interface{}); ok {
				allGroups = append(allGroups, gm)
			}
		}

		hasMore, _ := result["has_more"].(bool)
		if !hasMore {
			break
		}
		offset += 10
	}

	logrus.Infof("get attendance groups complete: %d", len(allGroups))
	attGroupsCacheMu.Lock()
	attGroupsCache = attendanceGroupCache{data: allGroups, expiry: time.Now().Add(5 * time.Minute)}
	attGroupsCacheMu.Unlock()
	return allGroups, nil
}

// GetAttendanceGroup 鏌ヨ鍗曚釜鑰冨嫟缁勮鎯?
func GetAttendanceGroupsForOrg(orgID string) ([]map[string]interface{}, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}

	var allGroups []map[string]interface{}
	offset := 0

	for {
		body := map[string]interface{}{
			"offset": offset,
			"size":   10,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/getsimplegroups?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, fmt.Errorf("get attendance groups failed: %w", err)
		}
		if errcode, _ := resp["errcode"].(float64); errcode != 0 {
			return nil, fmt.Errorf("get attendance groups failed: %s", dingTalkErrorMessage(resp, errcode))
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}
		groups, ok := result["groups"].([]interface{})
		if !ok || len(groups) == 0 {
			break
		}
		for _, g := range groups {
			if gm, ok := g.(map[string]interface{}); ok {
				allGroups = append(allGroups, gm)
			}
		}
		hasMore, _ := result["has_more"].(bool)
		if !hasMore {
			break
		}
		offset += 10
	}

	logrus.Infof("get attendance groups complete org=%s count=%d", database.NormalizeOrganizationID(orgID), len(allGroups))
	return allGroups, nil
}

func GetAttendanceGroup(opUserID string, groupID int64) (map[string]interface{}, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"op_user_id": opUserID,
		"group_id":   groupID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/group/query?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("query attendance group failed: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		if errmsg == "" {
			errmsg = fmt.Sprintf("unknown errcode %.0f", errcode)
		}
		return nil, fmt.Errorf("query attendance group failed: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("query attendance group failed: invalid result payload")
	}
	attGroupDetailMap.Store(groupID, attendanceGroupDetailCache{data: result, expiry: time.Now().Add(5 * time.Minute)})
	return result, nil
}

func GetAttendanceGroupForOrg(orgID, opUserID string, groupID int64) (map[string]interface{}, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}

	body := map[string]interface{}{
		"op_user_id": opUserID,
		"group_id":   groupID,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/group/query?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("query attendance group failed: %w", err)
	}
	if errcode, _ := resp["errcode"].(float64); errcode != 0 {
		return nil, fmt.Errorf("query attendance group failed: %s", dingTalkErrorMessage(resp, errcode))
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("query attendance group failed: invalid result payload")
	}
	return result, nil
}

func AttendanceGroupHasShift(group map[string]interface{}, shiftID int64) bool {
	if shiftID <= 0 || len(group) == 0 {
		return false
	}

	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) == shiftID {
					return true
				}
			}
		}
	}

	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) == shiftID {
				return true
			}
		}
	}

	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if classID, ok := item["class_id"].(float64); ok && int64(classID) == shiftID {
					return true
				}
			}
		}
	}

	return false
}

func FindAnyAttendanceGroupShiftID(group map[string]interface{}) int64 {
	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) > 0 {
					return int64(id)
				}
			}
		}
	}

	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) > 0 {
				return int64(id)
			}
		}
	}

	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				if classID, ok := item["class_id"].(float64); ok && int64(classID) > 0 {
					return int64(classID)
				}
			}
		}
	}

	return 0
}

func CollectAttendanceGroupShiftIDs(group map[string]interface{}) map[int64]struct{} {
	ids := make(map[int64]struct{})

	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) > 0 {
					ids[int64(id)] = struct{}{}
				}
			}
		}
	}

	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) > 0 {
				ids[int64(id)] = struct{}{}
			}
		}
	}

	return ids
}

// findRestShiftFromShiftIDs resolves the rest shift ID for rotation groups.
// It reads shift_ids from the group, calls GetShiftList, and finds the shift named "休息".
func findRestShiftFromShiftIDs(group map[string]interface{}) int64 {
	// Collect all shift IDs listed in the group
	groupShiftSet := CollectAttendanceGroupShiftIDs(group)
	if len(groupShiftSet) == 0 {
		return 0
	}
	groupShiftIDList := make([]string, 0, len(groupShiftSet))
	for gsid := range groupShiftSet {
		groupShiftIDList = append(groupShiftIDList, fmt.Sprintf("%d", gsid))
	}
	logrus.Infof("findRestShiftFromShiftIDs: group shift_ids=%v", groupShiftIDList)

	shifts, err := GetShiftList()
	if err != nil {
		logrus.Warnf("findRestShiftFromShiftIDs: GetShiftList failed: %v", err)
		return 0
	}

	var names []string
	for _, shift := range shifts {
		id, _ := shift["id"].(float64)
		if id <= 0 {
			continue
		}
		shiftID := int64(id)
		name, _ := shift["name"].(string)
		names = append(names, fmt.Sprintf("%d=%s", shiftID, name))
		if _, inGroup := groupShiftSet[shiftID]; !inGroup {
			continue
		}
		logrus.Infof("findRestShiftFromShiftIDs: group shift %d=%q all_fields=%v", shiftID, name, shift)
		// Match rest shift by name or by is_rest field
		if strings.Contains(name, "休") {
			logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d (%s) in group", shiftID, name)
			return shiftID
		}
		for _, key := range []string{"isRest", "is_rest", "isrest"} {
			switch val := shift[key].(type) {
			case string:
				if val == "Y" || val == "y" || val == "true" || val == "1" {
					logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d via %s field", shiftID, key)
					return shiftID
				}
			case bool:
				if val {
					logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d via %s field", shiftID, key)
					return shiftID
				}
			case float64:
				if val == 1 {
					logrus.Infof("findRestShiftFromShiftIDs: found rest shift %d via %s field", shiftID, key)
					return shiftID
				}
			}
		}
	}
	logrus.Warnf("findRestShiftFromShiftIDs: no rest shift found in group shift_ids; all shifts: %v", names)
	return 0
}

// FindRestClassID 从考勤组数据中找到休息班次 ID
// 钉钉排班制考勤组中，休息日必须与专属的休息班次 ID（不能用工作班次，也不能省略）
func FindRestClassID(group map[string]interface{}) int64 {
	// isRestClass checks a class map for rest indicators across different DingTalk API versions
	isRestClass := func(cls map[string]interface{}) bool {
		for _, key := range []string{"isrest", "isRestDay", "isRest", "is_rest"} {
			v, ok := cls[key]
			if !ok {
				continue
			}
			switch val := v.(type) {
			case string:
				if val == "Y" || val == "y" || val == "true" || val == "1" {
					return true
				}
			case bool:
				if val {
					return true
				}
			case float64:
				if val == 1 {
					return true
				}
			}
		}
		return false
	}

	getClassID := func(cls map[string]interface{}) int64 {
		for _, key := range []string{"id", "classId", "class_id", "classID"} {
			if id, ok := cls[key].(float64); ok && id > 0 {
				return int64(id)
			}
		}
		return 0
	}

	// Try both "classes" and "class_list" (field name varies by group type)
	for _, fieldName := range []string{"classes", "class_list"} {
		classes, ok := group[fieldName].([]interface{})
		if !ok {
			continue
		}
		for _, raw := range classes {
			cls, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			if isRestClass(cls) {
				if id := getClassID(cls); id > 0 {
					return id
				}
			}
		}
	}

	knownShiftIDs := CollectAttendanceGroupShiftIDs(group)
	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				className, _ := item["class_name"].(string)
				if strings.Contains(className, "休") {
					if classID, ok := item["class_id"].(float64); ok && int64(classID) > 0 {
						logrus.Infof("FindRestClassID: found rest class %d (%s) from cycle_schedules", int64(classID), className)
						return int64(classID)
					}
				}
			}
		}

		classCounts := make(map[int64]int)
		for _, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for _, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				classID, ok := item["class_id"].(float64)
				if !ok || int64(classID) <= 0 {
					continue
				}
				id := int64(classID)
				if _, known := knownShiftIDs[id]; known {
					continue
				}
				classCounts[id]++
			}
		}

		var selectedID int64
		var selectedCount int
		for id, count := range classCounts {
			if count > selectedCount {
				selectedID = id
				selectedCount = count
			}
		}
		if selectedID > 0 {
			logrus.Infof("FindRestClassID: inferred rest class %d from cycle_schedules", selectedID)
			return selectedID
		}
	}

	// Log full group keys to aid debugging when rest class is still not found
	keys := make([]string, 0, len(group))
	for k := range group {
		keys = append(keys, k)
	}
	logrus.Warnf("FindRestClassID: rest class not found; group top-level keys: %v", keys)
	return 0
}

// GetAttendanceGroupRestClassIDForOrg resolves rest shift class id for an attendance group under org.
func GetAttendanceGroupRestClassIDForOrg(orgID string, group map[string]interface{}) int64 {
	_ = orgID
	return FindRestClassID(group)
}

func GetAttendanceGroupRestClassID(group map[string]interface{}) int64 {
	restClassID := FindRestClassID(group)
	if restClassID == 0 {
		restClassID = findRestShiftFromShiftIDs(group)
	}
	if restClassID == 0 {
		logrus.Warnf("GetAttendanceGroupRestClassID: rest class still missing; debug=%s", summarizeAttendanceGroupRestDebug(group))
	}
	return restClassID
}

func resolveScheduleAsyncRestShiftID(opUserID string, groupID int64) int64 {
	return resolveScheduleAsyncRestShiftIDForOrg(database.DefaultOrganizationID, opUserID, groupID)
}

func resolveScheduleAsyncRestShiftIDForOrg(orgID, opUserID string, groupID int64) int64 {
	const dingTalkRestShiftSentinel int64 = 1

	group, err := GetAttendanceGroupForOrg(orgID, opUserID, groupID)
	if err != nil {
		logrus.Warnf("resolveScheduleAsyncRestShiftID: get group detail failed for %d: %v; fallback=%d", groupID, err, dingTalkRestShiftSentinel)
		return dingTalkRestShiftSentinel
	}

	restShiftID := GetAttendanceGroupRestClassID(group)
	if restShiftID > 0 {
		return restShiftID
	}

	logrus.Infof("resolveScheduleAsyncRestShiftID: group %d uses DingTalk rest sentinel %d", groupID, dingTalkRestShiftSentinel)
	return dingTalkRestShiftSentinel
}

func summarizeAttendanceGroupRestDebug(group map[string]interface{}) string {
	shiftRefs := make([]string, 0)
	if shiftIDs, ok := group["shift_ids"].(map[string]interface{}); ok {
		if numbers, ok := shiftIDs["number"].([]interface{}); ok {
			for _, raw := range numbers {
				if id, ok := raw.(float64); ok && int64(id) > 0 {
					shiftRefs = append(shiftRefs, fmt.Sprintf("%d", int64(id)))
				}
			}
		}
	}
	if shiftIDs, ok := group["shift_ids"].([]interface{}); ok {
		for _, raw := range shiftIDs {
			if id, ok := raw.(float64); ok && int64(id) > 0 {
				shiftRefs = append(shiftRefs, fmt.Sprintf("%d", int64(id)))
			}
		}
	}

	cycleItems := make([]string, 0)
	if cycles, ok := group["cycle_schedules"].([]interface{}); ok {
		for cycleIdx, cycleRaw := range cycles {
			cycle, ok := cycleRaw.(map[string]interface{})
			if !ok {
				continue
			}
			items, ok := cycle["item_list"].([]interface{})
			if !ok {
				continue
			}
			for itemIdx, itemRaw := range items {
				item, ok := itemRaw.(map[string]interface{})
				if !ok {
					continue
				}
				classID := int64(0)
				if v, ok := item["class_id"].(float64); ok {
					classID = int64(v)
				}
				className, _ := item["class_name"].(string)
				cycleItems = append(cycleItems, fmt.Sprintf("cycle[%d].item[%d]=%d:%s", cycleIdx, itemIdx, classID, className))
			}
		}
	}

	return fmt.Sprintf("group=%v(%v) shift_ids=%v cycle_items=%v",
		group["group_name"], group["group_id"], shiftRefs, cycleItems)
}

func GetShiftList() ([]map[string]interface{}, error) {
	cacheKey := shiftListCacheKey()
	shiftCacheMu.Lock()
	if shiftCache.key == cacheKey && shiftCache.data != nil && time.Now().Before(shiftCache.expiry) {
		cached := cloneShiftList(shiftCache.data)
		shiftCacheMu.Unlock()
		return cached, nil
	}
	shiftCacheMu.Unlock()

	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	var allShifts []map[string]interface{}
	cursor := 0

	for {
		body := map[string]interface{}{
			"op_user_id": "",
			"cursor":     cursor,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/shift/list?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, fmt.Errorf("鑾峰彇鐝鍒楄〃澶辫触: %w", err)
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			return nil, fmt.Errorf("鑾峰彇鐝鍒楄〃澶辫触: %s", errmsg)
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}

		shifts, ok := result["result"].([]interface{})
		if !ok || len(shifts) == 0 {
			break
		}

		for _, s := range shifts {
			if sm, ok := s.(map[string]interface{}); ok {
				allShifts = append(allShifts, sm)
			}
		}

		hasMore, _ := result["has_more"].(bool)
		if !hasMore {
			break
		}
		nextCursor, _ := result["cursor"].(float64)
		cursor = int(nextCursor)
	}

	logrus.Infof("get shifts complete: %d", len(allShifts))
	shiftCacheMu.Lock()
	shiftCache = shiftListCache{key: cacheKey, data: cloneShiftList(allShifts), expiry: time.Now().Add(10 * time.Minute)}
	shiftCacheMu.Unlock()
	return cloneShiftList(allShifts), nil
}

func GetShiftListForOrg(orgID string) ([]map[string]interface{}, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return nil, err
	}

	var allShifts []map[string]interface{}
	cursor := 0

	for {
		body := map[string]interface{}{
			"op_user_id": "",
			"cursor":     cursor,
		}
		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/shift/list?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			return nil, fmt.Errorf("get shifts failed: %w", err)
		}
		if errcode, _ := resp["errcode"].(float64); errcode != 0 {
			return nil, fmt.Errorf("get shifts failed: %s", dingTalkErrorMessage(resp, errcode))
		}

		result, ok := resp["result"].(map[string]interface{})
		if !ok {
			break
		}
		shifts, ok := result["result"].([]interface{})
		if !ok || len(shifts) == 0 {
			break
		}
		for _, s := range shifts {
			if sm, ok := s.(map[string]interface{}); ok {
				allShifts = append(allShifts, sm)
			}
		}
		hasMore, _ := result["has_more"].(bool)
		if !hasMore {
			break
		}
		nextCursor, _ := result["cursor"].(float64)
		cursor = int(nextCursor)
	}

	logrus.Infof("get shifts complete org=%s count=%d", database.NormalizeOrganizationID(orgID), len(allShifts))
	return cloneShiftList(allShifts), nil
}

func shiftListCacheKey() string {
	return corpID + "|" + appKey
}

func cloneShiftList(shifts []map[string]interface{}) []map[string]interface{} {
	cloned := make([]map[string]interface{}, 0, len(shifts))
	for _, shift := range shifts {
		item := make(map[string]interface{}, len(shift))
		for key, value := range shift {
			item[key] = value
		}
		cloned = append(cloned, item)
	}
	return cloned
}

// FindShiftByName 浠庣彮娆″垪琛ㄤ腑鎸夊悕绉版煡鎵?
func FindShiftByName(shifts []map[string]interface{}, name string) (int64, bool) {
	for _, shift := range shifts {
		if getString(shift, "name") == name {
			if id := int64(getFloat(shift, "id")); id > 0 {
				return id, true
			}
		}
	}
	return 0, false
}

// CreateShift 鍦ㄩ拤閽夊垱寤烘柊鐝
func CreateShift(opUserID string, shiftName string, checkInTime string, checkOutTime string) (int64, error) {
	return CreateShiftForOrg(database.DefaultOrganizationID, opUserID, shiftName, checkInTime, checkOutTime)
}

func CreateShiftForOrg(orgID, opUserID string, shiftName string, checkInTime string, checkOutTime string) (int64, error) {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return 0, err
	}

	checkInAt, err := formatShiftCheckTime(checkInTime, false)
	if err != nil {
		return 0, fmt.Errorf("娑撳﹦褰弮鍫曟？閺嶇厧绱￠柨娆掝嚖: %w", err)
	}
	checkOutAt, err := formatShiftCheckTime(checkOutTime, false)
	if err != nil {
		return 0, fmt.Errorf("娑撳褰弮鍫曟？閺嶇厧绱￠柨娆掝嚖: %w", err)
	}

	across := 0
	if !checkOutAt.After(checkInAt) {
		across = 1
		checkOutAt, err = formatShiftCheckTime(checkOutTime, true)
		if err != nil {
			return 0, fmt.Errorf("娑撳褰弮鍫曟？閺嶇厧绱￠柨娆掝嚖: %w", err)
		}
	}

	body := map[string]interface{}{
		"op_user_id": opUserID,
		"shift": map[string]interface{}{
			"name": shiftName,
			"sections": []map[string]interface{}{
				{
					"times": []map[string]interface{}{
						{
							"check_type": "OnDuty",
							"check_time": checkInAt.Format("2006-01-02 15:04:05"),
							"across":     0,
							"free_check": false,
						},
						{
							"check_type": "OffDuty",
							"check_time": checkOutAt.Format("2006-01-02 15:04:05"),
							"across":     across,
							"free_check": false,
						},
					},
				},
			},
		},
	}

	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/shift/add?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return 0, fmt.Errorf("鍒涘缓鐝澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return 0, fmt.Errorf("鍒涘缓鐝澶辫触: %s", errmsg)
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("鍒涘缓鐝鍝嶅簲鏍煎紡寮傚父")
	}

	shiftID := int64(getFloat(result, "id"))
	if shiftID == 0 {
		return 0, fmt.Errorf("鍒涘缓鐝鏈繑鍥炴湁鏁圛D")
	}

	logrus.Infof("鍒涘缓鐝鎴愬姛: name=%s, id=%d", shiftName, shiftID)
	return shiftID, nil
}

// GetUserScheduleList 鑾峰彇鐢ㄦ埛鏌愭鏃堕棿鐨勬帓鐝?
func formatShiftCheckTime(timeText string, nextDay bool) (time.Time, error) {
	timeText = strings.TrimSpace(timeText)
	layouts := []string{"15:04", "15:04:05"}
	baseDate := "2020-12-02"
	if nextDay {
		baseDate = "2020-12-03"
	}

	var lastErr error
	for _, layout := range layouts {
		t, err := time.ParseInLocation(layout, timeText, time.Local)
		if err != nil {
			lastErr = err
			continue
		}
		dateTime := fmt.Sprintf("%s %02d:%02d:%02d", baseDate, t.Hour(), t.Minute(), t.Second())
		return time.ParseInLocation("2006-01-02 15:04:05", dateTime, time.Local)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("unsupported time format")
	}
	return time.Time{}, lastErr
}

func GetUserScheduleList(userID string, workDateFrom, workDateTo string) ([]map[string]interface{}, error) {
	accessToken, err := GetAccessToken()
	if err != nil {
		return nil, err
	}

	// 瑙ｆ瀽鏃ユ湡鑼冨洿
	startDate, err := time.Parse("2006-01-02", workDateFrom)
	if err != nil {
		return nil, fmt.Errorf("寮€濮嬫棩鏈熸牸寮忛敊璇? %w", err)
	}

	endDate, err := time.Parse("2006-01-02", workDateTo)
	if err != nil {
		return nil, fmt.Errorf("缁撴潫鏃ユ湡鏍煎紡閿欒: %w", err)
	}

	var allSchedules []map[string]interface{}

	// 閬嶅巻姣忎竴澶╋紝璋冪敤API鏌ヨ
	for d := startDate; !d.After(endDate); d = d.AddDate(0, 0, 1) {
		dateStr := d.Format("2006-01-02")

		body := map[string]interface{}{
			"user_id":   userID,
			"work_date": dateStr,
		}

		resp, err := postJSONOAPI(
			fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/schedule/listbyday?access_token=%s", accessToken),
			body,
		)
		if err != nil {
			logrus.Warnf("鑾峰彇 %s 鎺掔彮澶辫触: %v", dateStr, err)
			continue
		}

		errcode, _ := resp["errcode"].(float64)
		if errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			logrus.Warnf("鑾峰彇 %s 鎺掔彮澶辫触: %s", dateStr, errmsg)
			continue
		}

		result, ok := resp["result"].([]interface{})
		if !ok {
			continue
		}

		for _, s := range result {
			if sm, ok := s.(map[string]interface{}); ok {
				allSchedules = append(allSchedules, sm)
			}
		}
	}

	return allSchedules, nil
}

// GetScheduleListBatchByDay 鎵归噺鑾峰彇澶氫釜鐢ㄦ埛鏌愬ぉ鐨勬帓鐝?// 浣跨敤 /topapi/attendance/schedule/listbyusers 鎺ュ彛锛屾敮鎸佷竴娆℃煡璇㈠涓敤鎴?
func GetScheduleListBatchByDay(userIDs []string, workDate string) ([]map[string]interface{}, error) {
	return GetScheduleListBatchByDayForOrg(database.DefaultOrganizationID, userIDs, workDate)
}

// GetScheduleListBatchByDayForOrg returns schedule rows for users on workDate using org credentials.
func GetScheduleListBatchByDayForOrg(orgID string, userIDs []string, workDate string) ([]map[string]interface{}, error) {
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return nil, err
	}
	return getScheduleListBatchByDayWithConfig(cfg, userIDs, workDate)
}

func getScheduleListBatchByDayWithConfig(cfg Config, userIDs []string, workDate string) ([]map[string]interface{}, error) {
	opUserID, err := ResolveAdminUserIDFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return nil, err
	}

	t, err := time.Parse("2006-01-02", workDate)
	if err != nil {
		return nil, fmt.Errorf("日期格式错误: %w", err)
	}
	dayMs := t.UnixMilli()
	body := map[string]interface{}{
		"op_user_id":     opUserID,
		"userids":        strings.Join(userIDs, ","),
		"from_date_time": dayMs,
		"to_date_time":   dayMs,
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/schedule/listbyusers?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return nil, fmt.Errorf("批量获取 %s 排班失败: %w", workDate, err)
	}
	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return nil, fmt.Errorf("批量获取 %s 排班失败: %s", workDate, errmsg)
	}
	result, ok := resp["result"].([]interface{})
	if !ok {
		return nil, nil
	}
	var schedules []map[string]interface{}
	for _, s := range result {
		if sm, ok := s.(map[string]interface{}); ok {
			schedules = append(schedules, sm)
		}
	}
	return schedules, nil
}

// GetHolidaysFromDingTalk 浠庨拤閽夎幏鍙栬妭鍋囨棩鏁版嵁
func GetHolidaysFromDingTalk(userID string, startDate, endDate string) (map[string]string, error) {
	schedules, err := GetUserScheduleList(userID, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("鑾峰彇鎺掔彮鏁版嵁澶辫触: %w", err)
	}

	holidays := make(map[string]string)

	for _, schedule := range schedules {
		// 鑾峰彇宸ヤ綔鏃ユ湡
		workDate, ok := schedule["work_date"].(string)
		if !ok {
			continue
		}

		// 检查是否为休息日
		isRest, ok := schedule["is_rest"].(string)
		if !ok {
			continue
		}

		if isRest == "Y" {
			// 从排班数据中提取节假日名称
			holidays[workDate] = "节假日"
		}
	}

	return holidays, nil
}

// FindScheduleGroupID 浠庤€冨嫟缁勫垪琛ㄤ腑鎵惧埌绗竴涓帓鐝埗鎴栬疆鐝埗鑰冨嫟缁?ID
func FindScheduleGroupID(groups []map[string]interface{}) (int64, error) {
	if len(groups) == 0 {
		return 0, fmt.Errorf("没有找到考勤组")
	}

	preferredGroupID := strings.TrimSpace(os.Getenv("DINGTALK_ATTENDANCE_GROUP_ID"))
	preferredGroupName := strings.TrimSpace(os.Getenv("DINGTALK_ATTENDANCE_GROUP_NAME"))

	if preferredGroupID != "" {
		expectedID, err := strconv.ParseInt(preferredGroupID, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("DINGTALK_ATTENDANCE_GROUP_ID 閺嶇厧绱￠柨娆掝嚖: %w", err)
		}
		for _, group := range groups {
			gid, ok := group["group_id"].(float64)
			if !ok || int64(gid) != expectedID {
				continue
			}
			groupType, _ := group["type"].(string)
			groupName, _ := group["group_name"].(string)
			if groupType != "SCHEDULE" && groupType != "TURN" {
				return 0, fmt.Errorf("指定考勤组 %s(%d) 类型为 %s，不支持排班操作", groupName, expectedID, groupType)
			}
			logrus.Infof("娴ｈ法鏁ら幐鍥х暰閼板啫瀚熺紒? %s, 缁鐎? %s, ID: %d", groupName, groupType, expectedID)
			return expectedID, nil
		}
		return 0, fmt.Errorf("閹稿洤鐣鹃懓鍐ㄥ珶缂? ID=%d 閺堫亜婀崣顖滄暏閼板啫瀚熺紒鍕灙鐞涖劋鑵戦幍鎯у煂", expectedID)
	}

	if preferredGroupName != "" {
		for _, group := range groups {
			groupType, _ := group["type"].(string)
			groupName, _ := group["group_name"].(string)
			if !strings.EqualFold(strings.TrimSpace(groupName), preferredGroupName) {
				continue
			}
			if groupType != "SCHEDULE" && groupType != "TURN" {
				return 0, fmt.Errorf("指定考勤组 %s 类型为 %s，不支持排班操作", groupName, groupType)
			}
			if gid, ok := group["group_id"].(float64); ok {
				logrus.Infof("娴ｈ法鏁ら幐鍥х暰閼板啫瀚熺紒? %s, 缁鐎? %s, ID: %v", groupName, groupType, gid)
				return int64(gid), nil
			}
		}
		return 0, fmt.Errorf("閹稿洤鐣鹃懓鍐ㄥ珶缂? %s 閺堫亜婀崣顖滄暏閼板啫瀚熺紒鍕灙鐞涖劋鑵戦幍鎯у煂", preferredGroupName)
	}

	type eligibleGroup struct {
		id   int64
		name string
		kind string
	}

	eligibleGroups := make([]eligibleGroup, 0)
	for _, group := range groups {
		groupType, ok := group["type"].(string)
		groupName, _ := group["group_name"].(string)
		logrus.Infof("鑰冨嫟缁? %s, 绫诲瀷: %s", groupName, groupType)
		if !ok || (groupType != "SCHEDULE" && groupType != "TURN") {
			continue
		}
		gid, ok := group["group_id"].(float64)
		if !ok {
			continue
		}
		eligibleGroups = append(eligibleGroups, eligibleGroup{
			id:   int64(gid),
			name: groupName,
			kind: groupType,
		})
	}

	if len(eligibleGroups) == 1 {
		group := eligibleGroups[0]
		logrus.Infof("浣跨敤鑰冨嫟缁? %s, 绫诲瀷: %s, ID: %d", group.name, group.kind, group.id)
		return group.id, nil
	}
	if len(eligibleGroups) > 1 {
		names := make([]string, 0, len(eligibleGroups))
		for _, group := range eligibleGroups {
			names = append(names, fmt.Sprintf("%s(%d)", group.name, group.id))
		}
		return 0, fmt.Errorf("found %d eligible attendance groups: %s; set DINGTALK_ATTENDANCE_GROUP_ID or DINGTALK_ATTENDANCE_GROUP_NAME before syncing", len(eligibleGroups), strings.Join(names, ", "))
	}

	for _, group := range groups {
		groupType, ok := group["type"].(string)
		if ok && groupType == "FIXED" {
			return 0, fmt.Errorf("考勤组为固定班制，无法排班，请创建排班制或轮班制考勤组")
		}
	}
	return 0, fmt.Errorf("没有找到可用考勤组，请确保已创建排班制或轮班制考勤组")
}

// SetAttendanceScheduleWithGroup 浣跨敤棰勮幏鍙栫殑鑰冨嫟缁?ID 璁剧疆鐢ㄦ埛鏌愬ぉ鐨勬帓鐝?
func SetAttendanceScheduleWithGroup(opUserID string, userID string, workDate string, shiftID int64, groupID int64) error {
	return SetAttendanceScheduleWithGroupForOrg(database.DefaultOrganizationID, opUserID, userID, workDate, shiftID, groupID)
}

func SetAttendanceScheduleWithGroupForOrg(orgID, opUserID string, userID string, workDate string, shiftID int64, groupID int64) error {
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return err
	}

	t, err := time.Parse("2006-01-02", workDate)
	if err != nil {
		return fmt.Errorf("鏃ユ湡鏍煎紡閿欒: %w", err)
	}

	scheduleItem := map[string]interface{}{
		"userid":    userID,
		"work_date": t.UnixMilli(),
		"is_rest":   shiftID == 0,
		"shift_id":  shiftID,
	}
	if shiftID == 0 {
		scheduleItem["shift_id"] = resolveScheduleAsyncRestShiftIDForOrg(orgID, opUserID, groupID)
	}

	body := map[string]interface{}{
		"op_user_id": opUserID,
		"group_id":   groupID,
		"schedules":  []map[string]interface{}{scheduleItem},
	}

	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/group/schedule/async?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return fmt.Errorf("璁剧疆鎺掔彮澶辫触: %w", err)
	}

	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return fmt.Errorf("璁剧疆鎺掔彮澶辫触: %s", errmsg)
	}

	logrus.Infof("璁剧疆鎺掔彮鎴愬姛: 鐢ㄦ埛=%s, 鏃ユ湡=%s, 鐝ID=%d", userID, workDate, shiftID)
	return nil
}

// ScheduleItem 鎺掔彮鏉＄洰
type ScheduleItem struct {
	UserID   string
	WorkDate string // "2006-01-02"
	ShiftID  int64  // 0 = 浼戞伅
}

// ValidationResult 校验结果
type ValidationResult struct {
	Valid   bool              `json:"valid"`
	Message string            `json:"message"`
	Errors  map[string]string `json:"errors"` // userID -> error message
}

// ValidateScheduleItems 校验排班项目
func ValidateScheduleItems(opUserID string, items []ScheduleItem, groupID int64) *ValidationResult {
	return ValidateScheduleItemsForOrg(database.DefaultOrganizationID, opUserID, items, groupID)
}

func ValidateScheduleItemsForOrg(orgID, opUserID string, items []ScheduleItem, groupID int64) *ValidationResult {
	result := &ValidationResult{
		Valid:   true,
		Message: "",
		Errors:  make(map[string]string),
	}

	// 1. 校验考勤组是否存在且有效
	group, err := GetAttendanceGroupForOrg(orgID, opUserID, groupID)
	if err != nil {
		result.Valid = false
		result.Message = "考勤组不存在或无效"
		return result
	}

	employeeIDs := collectAttendanceGroupUserIDs(group)
	shouldValidateMembers, skipReason := shouldValidateAttendanceGroupMembers(group, employeeIDs)
	if !shouldValidateMembers {
		logrus.Infof("skip attendance group member validation for group %d: %s", groupID, skipReason)
	}

	// 3. 提取考勤组中的班次
	groupShiftIDs := CollectAttendanceGroupShiftIDs(group)

	// 4. 校验每个项目
	now := time.Now()
	for _, item := range items {
		// 校验员工是否在考勤组中
		if shouldValidateMembers && !containsUserID(employeeIDs, item.UserID) {
			result.Errors[item.UserID] = "员工不在考勤组中"
			result.Valid = false
			continue
		}

		// 校验日期是否在允许范围内（不能是过去的日期）
		workDate, err := time.Parse("2006-01-02", item.WorkDate)
		if err != nil {
			result.Errors[item.UserID] = "日期格式错误"
			result.Valid = false
			continue
		}

		if workDate.Before(now.AddDate(0, 0, -1)) {
			result.Errors[item.UserID] = "不能修改过去的日期"
			result.Valid = false
			continue
		}

		// 校验班次是否在考勤组中
		if item.ShiftID > 0 && !containsShiftID(groupShiftIDs, item.ShiftID) {
			result.Errors[item.UserID] = "班次不在考勤组中"
			result.Valid = false
			continue
		}
	}

	if !result.Valid && result.Message == "" {
		result.Message = "部分项目校验失败"
	}

	return result
}

func collectAttendanceGroupUserIDs(group map[string]interface{}) map[string]struct{} {
	result := make(map[string]struct{})
	members, ok := group["userids"].(map[string]interface{})
	if !ok {
		return result
	}
	userIDs, ok := members["string"].([]interface{})
	if !ok {
		return result
	}
	for _, uid := range userIDs {
		userID, ok := uid.(string)
		if !ok || strings.TrimSpace(userID) == "" {
			continue
		}
		result[userID] = struct{}{}
	}
	return result
}

func shouldValidateAttendanceGroupMembers(group map[string]interface{}, memberIDs map[string]struct{}) (bool, string) {
	if len(memberIDs) > 0 {
		return true, ""
	}

	if memberCount := int64(getFloat(group, "member_count")); memberCount > 0 {
		return false, fmt.Sprintf("member_count=%d but no explicit userids field", memberCount)
	}

	if addressList, ok := group["address_list"].([]interface{}); ok && len(addressList) > 0 {
		return false, fmt.Sprintf("address_list has %d entries but no explicit userids field", len(addressList))
	}

	return true, ""
}

func containsUserID(userIDs map[string]struct{}, userID string) bool {
	_, ok := userIDs[userID]
	return ok
}

// containsShiftID 检查班次ID是否在集合中
func containsShiftID(shiftIDs map[int64]struct{}, shiftID int64) bool {
	_, ok := shiftIDs[shiftID]
	return ok
}

// BatchSetAttendanceSchedule 鎵归噺璁剧疆鎺掔彮锛屽皢澶氭潯鎺掔彮鎵撳寘鍒板崟娆?API 璇锋眰
func BatchSetAttendanceSchedule(opUserID string, items []ScheduleItem, groupID int64) (successCount int, failedItems []ScheduleItem, err error) {
	return BatchSetAttendanceScheduleForOrg(database.DefaultOrganizationID, opUserID, items, groupID)
}

func BatchSetAttendanceScheduleForOrg(orgID, opUserID string, items []ScheduleItem, groupID int64) (successCount int, failedItems []ScheduleItem, err error) {
	if len(items) == 0 {
		return 0, nil, nil
	}

	// 前置校验
	validationResult := ValidateScheduleItemsForOrg(orgID, opUserID, items, groupID)
	if !validationResult.Valid {
		// 收集失败的项目
		failedMap := make(map[string]bool)
		for userID := range validationResult.Errors {
			failedMap[userID] = true
		}

		for _, item := range items {
			if failedMap[item.UserID] {
				failedItems = append(failedItems, item)
			}
		}

		return 0, failedItems, fmt.Errorf("%s", validationResult.Message)
	}

	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return 0, items, err
	}

	restShiftID := resolveScheduleAsyncRestShiftIDForOrg(orgID, opUserID, groupID)

	// Split: work items (ShiftID>0) MUST be in a separate batch from rest items (ShiftID==0).
	// DingTalk rejects the entire batch if any schedule entry is missing shift_id.
	var workItems, restItems []ScheduleItem
	for _, item := range items {
		if item.ShiftID > 0 {
			workItems = append(workItems, item)
		} else {
			restItems = append(restItems, item)
		}
	}

	const batchSize = 200
	var batchErrors []string

	pushBatch := func(chunk []ScheduleItem, makeSchedule func(ScheduleItem, int64) map[string]interface{}) {
		schedules := make([]map[string]interface{}, 0, len(chunk))
		var parseFailItems []ScheduleItem
		for _, item := range chunk {
			t, parseErr := time.Parse("2006-01-02", item.WorkDate)
			if parseErr != nil {
				parseFailItems = append(parseFailItems, item)
				continue
			}
			schedules = append(schedules, makeSchedule(item, t.UnixMilli()))
		}
		if len(parseFailItems) > 0 {
			failedItems = append(failedItems, parseFailItems...)
		}
		if len(schedules) == 0 {
			return
		}
		body := map[string]interface{}{"op_user_id": opUserID, "group_id": groupID, "schedules": schedules}
		resp, postErr := postJSONOAPI(fmt.Sprintf("https://oapi.dingtalk.com/topapi/attendance/group/schedule/async?access_token=%s", accessToken), body)
		if postErr != nil {
			failedItems = append(failedItems, chunk...)
			batchErrors = append(batchErrors, "request failed: "+postErr.Error())
			return
		}
		if errcode, _ := resp["errcode"].(float64); errcode != 0 {
			errmsg, _ := resp["errmsg"].(string)
			failedItems = append(failedItems, chunk...)
			batchErrors = append(batchErrors, "api error: "+errmsg)
			return
		}
		successCount += len(schedules)
		logrus.Infof("batch set attendance schedule success: %d items", len(schedules))
	}

	// Push work items
	for i := 0; i < len(workItems); i += batchSize {
		end := i + batchSize
		if end > len(workItems) {
			end = len(workItems)
		}
		pushBatch(workItems[i:end], func(item ScheduleItem, ts int64) map[string]interface{} {
			return map[string]interface{}{
				"userid":    item.UserID,
				"work_date": ts,
				"is_rest":   false,
				"shift_id":  item.ShiftID,
			}
		})
	}

	// Push rest items with is_rest=true and shift_id=0 in a separate batch.
	// DingTalk batch API requires shift_id to be present (even 0) for all items.
	for i := 0; i < len(restItems); i += batchSize {
		end := i + batchSize
		if end > len(restItems) {
			end = len(restItems)
		}
		pushBatch(restItems[i:end], func(item ScheduleItem, ts int64) map[string]interface{} {
			return map[string]interface{}{
				"userid":    item.UserID,
				"work_date": ts,
				"is_rest":   true,
				"shift_id":  restShiftID,
			}
		})
	}

	if len(batchErrors) > 0 {
		return successCount, failedItems, fmt.Errorf("%s", strings.Join(batchErrors, "; "))
	}
	return successCount, failedItems, nil
}

// SetAttendanceSchedule 璁剧疆鐢ㄦ埛鏌愬ぉ鐨勬帓鐝紙鍚戝悗鍏煎锛屽唴閮ㄤ細鏌ヨ鑰冨嫟缁勶級
// shiftID > 0 琛ㄧず涓婄彮锛堜娇鐢ㄨ鐝锛夛紝shiftID == 0 琛ㄧず浼戞伅
func SetAttendanceSchedule(opUserID string, userID string, workDate string, shiftID int64) error {
	return SetAttendanceScheduleForOrg(database.DefaultOrganizationID, opUserID, userID, workDate, shiftID)
}

func SetAttendanceScheduleForOrg(orgID, opUserID string, userID string, workDate string, shiftID int64) error {
	groups, err := GetAttendanceGroupsForOrg(orgID)
	if err != nil {
		return fmt.Errorf("鑾峰彇鑰冨嫟缁勫け璐? %w", err)
	}
	groupID, err := FindScheduleGroupID(groups)
	if err != nil {
		return err
	}
	return SetAttendanceScheduleWithGroupForOrg(orgID, opUserID, userID, workDate, shiftID, groupID)
}

// SendCorpMessageToUser 发送企业内部消息通知到指定用户
func SendCorpMessageToUser(userID, title, content string) error {
	return sendCorpMessagePayloadToUser(userID, title, buildCorpMessagePayload(userID, title, content))
}

func SendCorpMessageToUserForOrg(orgID, userID, title, content string) error {
	return sendCorpMessagePayloadToUserForOrg(orgID, userID, title, buildCorpMessagePayload(userID, title, content))
}

func SendCorpActionCardToUser(userID, title, content, actionTitle, actionURL string) error {
	if strings.TrimSpace(actionURL) == "" {
		return SendCorpMessageToUser(userID, title, content)
	}
	return sendCorpMessagePayloadToUser(userID, title, buildCorpActionCardPayload(userID, title, content, actionTitle, actionURL))
}

func SendCorpActionCardToUserForOrg(orgID, userID, title, content, actionTitle, actionURL string) error {
	if strings.TrimSpace(actionURL) == "" {
		return SendCorpMessageToUserForOrg(orgID, userID, title, content)
	}
	return sendCorpMessagePayloadToUserForOrg(orgID, userID, title, buildCorpActionCardPayload(userID, title, content, actionTitle, actionURL))
}

func sendCorpMessagePayloadToUser(userID, title string, body map[string]interface{}) error {
	return sendCorpMessagePayloadToUserForOrg(database.DefaultOrganizationID, userID, title, body)
}

func sendCorpMessagePayloadToUserForOrg(orgID, userID, title string, body map[string]interface{}) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("orgID is empty")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return fmt.Errorf("userID is empty")
	}
	// Must use org-scoped notifiable check; never fall back to default-org lookup.
	if !IsNotifiableUserIDForOrg(orgID, userID) {
		return fmt.Errorf("%w: %s", ErrUserNotNotifiable, userID)
	}
	cfg, err := ConfigForOrgID(orgID)
	if err != nil {
		return err
	}
	agentID, err := requireDingTalkAgentID(cfg)
	if err != nil {
		return err
	}
	body["userid_list"] = userID
	body["agent_id"] = agentID
	accessToken, err := getAccessTokenWithConfig(cfg)
	if err != nil {
		return err
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/message/corpconversation/asyncsend?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return err
	}
	errcode, _ := resp["errcode"].(float64)
	if errcode != 0 {
		return fmt.Errorf("send message failed: %s", dingTalkErrorMessage(resp, errcode))
	}
	taskID := int64FromMap(resp, "task_id")
	requestID := strings.TrimSpace(getString(resp, "request_id"))
	if taskID > 0 {
		logrus.Infof("dingtalk message send task accepted for user %s: %s task_id=%d request_id=%s", userID, title, taskID, requestID)
	} else {
		logrus.Infof("dingtalk message send task accepted for user %s: %s request_id=%s", userID, title, requestID)
	}
	return nil
}

func buildCorpMessagePayload(userID, title, content string) map[string]interface{} {
	return buildCorpPayload(userID, map[string]interface{}{
		"msgtype": "text",
		"text": map[string]interface{}{
			"content": formatCorpMessageContent(title, content),
		},
	})
}

func buildCorpActionCardPayload(userID, title, content, actionTitle, actionURL string) map[string]interface{} {
	actionTitle = strings.TrimSpace(actionTitle)
	if actionTitle == "" {
		actionTitle = "查看详情"
	}
	return buildCorpPayload(userID, map[string]interface{}{
		"msgtype": "action_card",
		"action_card": map[string]interface{}{
			"title":        strings.TrimSpace(title),
			"markdown":     formatCorpMessageMarkdown(title, content),
			"single_title": actionTitle,
			"single_url":   strings.TrimSpace(actionURL),
		},
	})
}

func buildCorpImagePayload(userID, mediaID string) map[string]interface{} {
	return buildCorpPayload(userID, map[string]interface{}{
		"msgtype": "image",
		"image": map[string]interface{}{
			"media_id": strings.TrimSpace(mediaID),
		},
	})
}

// UploadImageMediaForOrg uploads a PNG/JPEG image to DingTalk media storage and returns media_id.
func UploadImageMediaForOrg(orgID, filename string, content []byte) (string, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return "", fmt.Errorf("orgID is empty")
	}
	if len(content) == 0 {
		return "", fmt.Errorf("image content is empty")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "schedule.png"
	}
	filename = filepath.Base(filename)
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return "", err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("media", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(content); err != nil {
		return "", err
	}
	if err := writer.Close(); err != nil {
		return "", err
	}
	url := fmt.Sprintf("https://oapi.dingtalk.com/media/upload?access_token=%s&type=image", accessToken)
	req, err := http.NewRequest("POST", url, &body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", fmt.Errorf("media upload response invalid: %w", err)
	}
	if errcode, _ := parsed["errcode"].(float64); errcode != 0 {
		errmsg, _ := parsed["errmsg"].(string)
		return "", fmt.Errorf("media upload failed: %s", errmsg)
	}
	mediaID, _ := parsed["media_id"].(string)
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return "", fmt.Errorf("media upload missing media_id")
	}
	return mediaID, nil
}

// SendCorpImageToUserForOrg sends an image corp message to one user.
func SendCorpImageToUserForOrg(orgID, userID, mediaID string) error {
	orgID = strings.TrimSpace(orgID)
	userID = strings.TrimSpace(userID)
	mediaID = strings.TrimSpace(mediaID)
	if orgID == "" {
		return fmt.Errorf("orgID is empty")
	}
	if userID == "" {
		return fmt.Errorf("userID is empty")
	}
	if mediaID == "" {
		return fmt.Errorf("media_id is empty")
	}
	if !IsNotifiableUserIDForOrg(orgID, userID) {
		return fmt.Errorf("%w: %s", ErrUserNotNotifiable, userID)
	}
	accessToken, err := GetAccessTokenForOrg(orgID)
	if err != nil {
		return err
	}
	body := buildCorpImagePayload(userID, mediaID)
	if cfg, err := ConfigForOrgID(orgID); err == nil {
		agentID := cfg.normalized().AgentID
		if agentID != "" {
			if agentNum, convErr := strconv.ParseInt(agentID, 10, 64); convErr == nil {
				body["agent_id"] = agentNum
			} else {
				body["agent_id"] = agentID
			}
		}
	}
	resp, err := postJSONOAPI(
		fmt.Sprintf("https://oapi.dingtalk.com/topapi/message/corpconversation/asyncsend_v2?access_token=%s", accessToken),
		body,
	)
	if err != nil {
		return err
	}
	if errcode, _ := resp["errcode"].(float64); errcode != 0 {
		errmsg, _ := resp["errmsg"].(string)
		return fmt.Errorf("%s", errmsg)
	}
	return nil
}

func buildCorpPayload(userID string, msg map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"agent_id":    getDingTalkAgentID(),
		"userid_list": strings.TrimSpace(userID),
		"msg":         msg,
	}
}

func formatCorpMessageContent(title, content string) string {
	trimmedTitle := strings.TrimSpace(title)
	trimmedContent := strings.TrimSpace(content)

	switch {
	case trimmedTitle == "":
		return trimmedContent
	case trimmedContent == "":
		return trimmedTitle
	default:
		return trimmedTitle + "\n\n" + trimmedContent
	}
}

func formatCorpMessageMarkdown(title, content string) string {
	trimmedTitle := strings.TrimSpace(title)
	trimmedContent := strings.TrimSpace(content)
	switch {
	case trimmedTitle == "":
		return strings.ReplaceAll(trimmedContent, "\n", "\n\n")
	case trimmedContent == "":
		return "### " + trimmedTitle
	default:
		return "### " + trimmedTitle + "\n\n" + strings.ReplaceAll(trimmedContent, "\n", "\n\n")
	}
}

func IsNotifiableUserID(userID string) bool {
	return IsNotifiableUserIDForOrg(database.DefaultOrganizationID, userID)
}

// IsNotifiableUserIDForOrg checks whether a user in a given org can receive corp messages.
func IsNotifiableUserIDForOrg(orgID, userID string) bool {
	orgID = strings.TrimSpace(orgID)
	trimmed := strings.TrimSpace(userID)
	if orgID == "" || trimmed == "" || strings.EqualFold(trimmed, "admin") || strings.EqualFold(trimmed, "system") {
		return false
	}
	if database.DB == nil {
		return true
	}

	var user database.User
	err := database.DB.
		Select("user_id", "status").
		Where("org_id = ? AND user_id = ? AND deleted_at IS NULL", orgID, trimmed).
		First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logrus.Infof("dingtalk notifiable user org=%s user=%s not found locally, allow send attempt", orgID, trimmed)
		return true
	}
	if err != nil {
		logrus.Warnf("check dingtalk notifiable user failed org=%s user=%s: %v", orgID, trimmed, err)
		return false
	}

	return isNotifiableUserStatus(user.Status)
}

func isNotifiableUserStatus(status string) bool {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "", "active", "enabled", "normal", "onjob", "on_job", "在职", "试用", "正式":
		return true
	case "inactive", "exited", "disabled", "resigned", "terminated", "deleted", "removed", "leave", "left", "离职", "已离职", "禁用", "停用":
		return false
	default:
		return true
	}
}

func int64FromMap(values map[string]interface{}, key string) int64 {
	switch value := values[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return parsed
	default:
		return 0
	}
}
