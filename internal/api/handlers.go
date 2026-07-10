package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/middleware"
	"peopleops/internal/service"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// 钉钉登录 state 存储（防 CSRF）
var (
	dingtalkStates   = make(map[string]time.Time)
	dingtalkStatesMu sync.Mutex
	dingtalkStateTTL = 5 * time.Minute
)

func updateSyncStatus(syncService *service.SyncService, syncType, status, message string) {
	if syncService == nil {
		return
	}
	if err := syncService.UpdateSyncStatus(syncType, status, message); err != nil {
		log.Printf("[sync-status] update %s=%s failed: %v", syncType, status, err)
	}
}

func generateLoginState() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("crypto random unavailable: %v", err))
	}
	state := hex.EncodeToString(b)
	dingtalkStatesMu.Lock()
	dingtalkStates[state] = time.Now()
	dingtalkStatesMu.Unlock()
	return state
}

func validateLoginState(state string) bool {
	if state == "" {
		return false
	}
	dingtalkStatesMu.Lock()
	defer dingtalkStatesMu.Unlock()
	expiry, ok := dingtalkStates[state]
	if !ok {
		return false
	}
	delete(dingtalkStates, state)
	return time.Since(expiry) < dingtalkStateTTL
}

func cleanupOldStates() {
	dingtalkStatesMu.Lock()
	defer dingtalkStatesMu.Unlock()
	for state, expiry := range dingtalkStates {
		if time.Since(expiry) > dingtalkStateTTL {
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
	".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
	".wps", ".et", ".dps",
	".txt", ".csv", ".md",
	".zip", ".rar", ".7z",
}

var allowedUploadExtensionSet = func() map[string]struct{} {
	values := make(map[string]struct{}, len(allowedUploadExtensions))
	for _, ext := range allowedUploadExtensions {
		values[ext] = struct{}{}
	}
	return values
}()

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
	if user.ActualRegularDate != "" {
		profile.ActualRegularDate = user.ActualRegularDate
		profile.ProbationEndDate = user.ActualRegularDate
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

func loadUserByUserID(userID string) (*database.User, error) {
	return loadUserByUserIDInOrg("", userID)
}

// loadUserByUserIDInOrg 在指定组织范围内按 user_id 查询用户。orgID 为空时退化为跨组织查询，
// 仅供无 auth 上下文的老代码兜底使用；有 gin.Context 的调用点必须传 c.GetString("orgID")。
func loadUserByUserIDInOrg(orgID, userID string) (*database.User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, gorm.ErrRecordNotFound
	}

	var user database.User
	query := database.DB.Where("user_id = ? AND deleted_at IS NULL", userID)
	if orgID = strings.TrimSpace(orgID); orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
	if err := query.First(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func loadUserByAuthID(authUserID string) (*database.User, error) {
	return loadUserByAuthIDInOrg("", authUserID)
}

func loadUserByAuthIDInOrg(orgID, authUserID string) (*database.User, error) {
	authUserID = strings.TrimSpace(authUserID)
	if authUserID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	orgID = strings.TrimSpace(orgID)

	var user database.User
	query := database.DB.Where("user_id = ? AND deleted_at IS NULL", authUserID)
	if orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
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
	query = database.DB.Where("id = ? AND deleted_at IS NULL", authUserID)
	if orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
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
	user, err := loadUserByUserIDInOrg(c.GetString("orgID"), userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
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

func dingtalkDepartmentsToOrgSyncItems(depts []dingtalk.DeptInfo) []service.OrgDepartmentSyncItem {
	items := make([]service.OrgDepartmentSyncItem, 0, len(depts))
	for _, d := range depts {
		items = append(items, service.OrgDepartmentSyncItem{
			DepartmentID: fmt.Sprintf("%d", d.DeptID),
			Name:         d.Name,
			ParentID:     fmt.Sprintf("%d", d.ParentID),
			HeadUserIDs:  d.DeptManagerUserIDs,
			Extension:    d.Extension,
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

func newLocalUserFromDingTalk(u dingtalk.UserInfo, deptID, status string) *database.User {
	user := &database.User{
		UserID:        u.UserID,
		Name:          u.Name,
		Email:         u.Email,
		Mobile:        u.Mobile,
		DepartmentID:  deptID,
		Position:      u.Position,
		Avatar:        u.Avatar,
		Status:        status,
		ManagerUserID: strings.TrimSpace(u.ManagerUserID),
		ManagerName:   strings.TrimSpace(u.ManagerName),
	}
	applyDingTalkOrgDiagnostics(user, u)
	return user
}

func applyDingTalkOrgUser(existing *database.User, u dingtalk.UserInfo, deptID, status string, overwriteEmpty bool) {
	existing.Name = u.Name
	existing.Email = u.Email
	existing.Mobile = u.Mobile
	existing.DepartmentID = deptID
	if strings.TrimSpace(u.Position) != "" || overwriteEmpty {
		existing.Position = strings.TrimSpace(u.Position)
	}
	existing.Avatar = u.Avatar
	existing.Status = status
	if strings.TrimSpace(u.ManagerUserID) != "" || overwriteEmpty {
		existing.ManagerUserID = strings.TrimSpace(u.ManagerUserID)
		existing.ManagerName = strings.TrimSpace(u.ManagerName)
	}
	applyDingTalkOrgDiagnostics(existing, u)
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

func assignDefaultEmployeeRoleForSyncedUser(permissionService *service.PermissionService, orgID, userID, source string) (bool, error) {
	assigned, err := permissionService.AssignDefaultEmployeeRoleIfUnassignedInOrg(orgID, userID)
	if err != nil {
		log.Printf("[%s] 为新增用户 %s/%s 分配普通员工角色失败: %v", source, orgID, userID, err)
		return false, err
	}
	if assigned {
		log.Printf("[%s] 已为新增用户 %s/%s 分配普通员工角色", source, orgID, userID)
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
	if user, err := loadUserByAuthID(userID); err == nil {
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

	auditService := service.NewAuditService(database.DB)
	_ = auditService.CreateLog(&database.OperationLog{
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

func fallbackDingTalkOrgID() string {
	orgID := strings.TrimSpace(os.Getenv("DINGTALK_SHARED_OAUTH_ORG_ID"))
	if orgID == "" {
		orgID = "default"
	}
	return orgID
}

func organizationDingTalkAppConfig(org *database.Organization) dingtalk.AppConfig {
	if org == nil {
		return dingtalk.AppConfig{}
	}
	return dingtalk.AppConfig{
		OrgID:       org.OrgID,
		Name:        org.Name,
		CorpID:      org.CorpID,
		AppKey:      org.AppKey,
		AppSecret:   org.AppSecret,
		AgentID:     org.AgentID,
		AppHomeURL:  org.AppHomeURL,
		RedirectURI: org.RedirectURI,
		Status:      org.Status,
	}
}

func resolveDingTalkLoginConfig(orgID, source string) (*database.Organization, dingtalk.AppConfig, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		orgID = fallbackDingTalkOrgID()
	}
	org, err := database.GetOrganizationByOrgID(orgID)
	if err != nil {
		return nil, dingtalk.AppConfig{}, fmt.Errorf("组织不存在: %s", orgID)
	}
	cfg := organizationDingTalkAppConfig(org)
	if strings.TrimSpace(cfg.CorpID) == "" || strings.TrimSpace(cfg.AppKey) == "" || strings.TrimSpace(cfg.AppSecret) == "" {
		log.Printf("[%s] organization dingtalk config incomplete: org_id=%s corp_id=%t app_key=%t app_secret=%t", source, org.OrgID, strings.TrimSpace(cfg.CorpID) != "", strings.TrimSpace(cfg.AppKey) != "", strings.TrimSpace(cfg.AppSecret) != "")
		return nil, dingtalk.AppConfig{}, fmt.Errorf("组织 %s 未配置完整的钉钉应用", orgID)
	}
	return org, cfg, nil
}

// generateToken issues new tokens with the business user_id as the primary identity.
func generateToken(user *database.User) (string, time.Time, error) {
	if user == nil {
		return "", time.Time{}, errors.New("missing user")
	}
	userID := strings.TrimSpace(user.UserID)
	if userID == "" {
		userID = strconv.FormatUint(uint64(user.ID), 10)
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	claims := &middleware.Claims{
		UserID:   userID,
		UserDBID: strconv.FormatUint(uint64(user.ID), 10),
		UserName: user.Name,
		OrgID:    user.OrgID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
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

// buildUserMenuKeys 聚合用户的菜单权限 key 列表
func buildUserMenuKeys(orgID, userID string) []string {
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	keys, err := permService.GetUserMenuKeysInOrg(orgID, userID)
	if err != nil {
		return []string{}
	}
	return keys
}

func buildUserPermissions(orgID, userID string) []string {
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	permissions, err := permService.GetUserPermissionsInOrg(orgID, userID)
	if err != nil {
		return []string{}
	}
	return permissions
}

func buildAuthUserPayload(user *database.User) gin.H {
	return gin.H{
		"id":            user.ID,
		"user_id":       user.UserID,
		"name":          user.Name,
		"email":         user.Email,
		"mobile":        user.Mobile,
		"department_id": user.DepartmentID,
		"position":      user.Position,
		"avatar":        user.Avatar,
		"status":        user.Status,
		"org_id":        user.OrgID,
		"menu_keys":     buildUserMenuKeys(user.OrgID, user.UserID),
		"permissions":   buildUserPermissions(user.OrgID, user.UserID),
	}
}

// Login 登录
func Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
		OrgID    string `json:"org_id"` // 可选，指定组织
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "用户名和密码不能为空",
		})
		return
	}

	// 用 user_id 或 email 查找用户
	orgID := strings.TrimSpace(req.OrgID)
	userService := service.NewUserServiceWithOrgID(database.DB, orgID)

	var user *database.User
	var err error

	if orgID != "" {
		// 指定了组织，按组织查找
		user, err = userService.GetUserByOrgAndUserID(orgID, req.Username)
	} else {
		// 未指定组织，向后兼容（查找任意组织的用户）
		user, err = userService.GetUserByUserID(req.Username)
	}

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
	tokenString, expiresAt, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "生成令牌失败",
		})
		return
	}

	// 写入 LoginLog
	database.DB.Create(&database.LoginLog{
		UserID:      user.UserID,
		UserName:    user.Name,
		LoginType:   "local",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"token":      tokenString,
			"user":       buildAuthUserPayload(user),
			"expires_at": expiresAt,
		},
	})
}

// GetUsers 获取用户列表
func GetUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

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
		orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	userService := service.NewUserServiceWithOrgID(database.DB, c.GetString("orgID"))
	users, total, err := userService.GetUsers(page, pageSize)
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

	userService := service.NewUserServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	userService := service.NewUserServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	departmentService := service.NewDepartmentServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	id := c.Param("id")

	departmentService := service.NewDepartmentServiceWithOrgID(database.DB, c.GetString("orgID"))
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

// SyncUsers 同步用户
func SyncUsers(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)

	orgID := strings.TrimSpace(c.GetString("orgID"))
	if orgID == "" {
		orgID = fallbackDingTalkOrgID()
	}
	_, appConfig, cfgErr := resolveDingTalkLoginConfig(orgID, "SyncUsers")
	if cfgErr != nil {
		updateSyncStatus(syncService, "users", "failed", cfgErr.Error())
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "组织钉钉应用配置不完整: " + cfgErr.Error(),
		})
		return
	}

	depts, deptErr := dingtalk.SyncDepartmentsForConfig(appConfig)
	if deptErr != nil {
		updateSyncStatus(syncService, "users", "failed", deptErr.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步用户前获取部门失败: " + deptErr.Error(),
		})
		return
	}

	// 从当前组织自己的钉钉应用拉取用户，避免多组织同步落到默认组织。
	users, err := dingtalk.SyncUsersWithDeptsForConfig(appConfig, depts)
	if err != nil {
		updateSyncStatus(syncService, "users", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步用户失败: " + err.Error(),
		})
		return
	}

	// 写入数据库
	userService := service.NewUserServiceWithOrgID(database.DB, orgID)
	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, orgID)
	permissionService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	count := 0
	positionMissingCount := 0
	defaultRoleAssignedCount := 0
	overwriteEmpty := shouldOverwriteEmptyDingTalkOrgFields(c)
	for _, u := range users {
		deptID := ""
		if len(u.DeptIDList) > 0 {
			deptID = fmt.Sprintf("%d", u.DeptIDList[0])
		}
		status := "active"
		if !u.Active {
			status = "inactive"
		}

		existing, err := userService.GetUserByOrgAndUserID(orgID, u.UserID)
		if err != nil {
			// 新建
			newUser := newLocalUserFromDingTalk(u, deptID, status)
			newUser.OrgID = orgID
			if err := userService.CreateUser(newUser); err != nil {
				log.Printf("[SyncUsers] 创建用户 %s 失败: %v", u.UserID, err)
				continue
			}
			if assigned, err := assignDefaultEmployeeRoleForSyncedUser(permissionService, orgID, u.UserID, "SyncUsers"); err == nil && assigned {
				defaultRoleAssignedCount++
			}
		} else {
			// 更新
			applyDingTalkOrgUser(existing, u, deptID, status, overwriteEmpty)
			if err := userService.UpdateUser(existing); err != nil {
				log.Printf("[SyncUsers] 更新用户 %s 失败: %v", u.UserID, err)
				continue
			}
		}
		if strings.TrimSpace(u.Position) == "" {
			positionMissingCount++
		}

		profile, profileErr := employeeService.GetProfileByUserID(u.UserID)
		if profileErr != nil {
			profile := &database.EmployeeProfile{
				UserID:     u.UserID,
				EmployeeID: u.UserID,
			}
			applyDingTalkProfileFields(profile, u, status)
			if err := employeeService.CreateProfile(profile); err != nil {
				log.Printf("[SyncUsers] 创建员工档案 %s 失败: %v", u.UserID, err)
				continue
			}
		} else {
			applyDingTalkProfileFields(profile, u, status)
			if err := employeeService.UpdateProfile(profile); err != nil {
				log.Printf("[SyncUsers] 更新员工档案 %s 失败: %v", u.UserID, err)
				continue
			}
		}
		count++

		// 维护 OrganizationUser 表
		if err := database.EnsureOrganizationUser(orgID, u.UserID, status); err != nil {
			log.Printf("[SyncUsers] 维护组织用户关系 %s 失败: %v", u.UserID, err)
		}
	}

	updateSyncStatus(syncService, "users", "success", fmt.Sprintf("同步 %d 个用户", count))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"count": count, "position_missing_count": positionMissingCount, "overwrite_empty": overwriteEmpty, "default_role_assigned_count": defaultRoleAssignedCount},
	})
}

// SyncDepartments 同步部门
func SyncDepartments(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)

	orgID := strings.TrimSpace(c.GetString("orgID"))
	if orgID == "" {
		orgID = fallbackDingTalkOrgID()
	}
	_, appConfig, cfgErr := resolveDingTalkLoginConfig(orgID, "SyncDepartments")
	if cfgErr != nil {
		updateSyncStatus(syncService, "departments", "failed", cfgErr.Error())
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "组织钉钉应用配置不完整: " + cfgErr.Error(),
		})
		return
	}

	// 从钉钉拉取部门（使用当前组织的钉钉应用凭证）
	depts, err := dingtalk.SyncDepartmentsForConfig(appConfig)
	if err != nil {
		updateSyncStatus(syncService, "departments", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步部门失败: " + err.Error(),
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(database.DB, orgID)
	result, err := orgService.SyncDepartmentsWithChangeLog(orgID, dingtalkDepartmentsToOrgSyncItems(depts), "dingtalk_sync")
	if err != nil {
		updateSyncStatus(syncService, "departments", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步部门失败: " + err.Error(),
		})
		return
	}
	updateSyncStatus(syncService, "departments", "success", fmt.Sprintf("同步 %d 个部门", result.Count))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"count": result.Count, "change_log_count": result.ChangeLogCount},
	})
}

// GetDingTalkConfig 返回钉钉前端配置（corpId 等），供 JS-SDK 初始化
func GetDingTalkConfig(c *gin.Context) {
	corpID := dingtalk.GetCorpID()
	clientID := os.Getenv("DINGTALK_APP_KEY")
	appHomeURL := resolveDingTalkAppHomeURL(c)
	redirectURI := resolveDingTalkRedirectURI(c)

	// 如果前端指定了 org_id，优先返回该企业的 corp_id，
	// 否则免登会用默认企业的 corpId，导致 corp 与 org 不匹配。
	orgID := strings.TrimSpace(c.Query("org_id"))
	if orgID != "" {
		if org, err := database.GetOrganizationByOrgID(orgID); err == nil {
			if strings.TrimSpace(org.CorpID) != "" {
				corpID = org.CorpID
			}
			if strings.TrimSpace(org.AppKey) != "" {
				clientID = org.AppKey
			}
			log.Printf("[dingtalk/config] resolved org corp: org_id=%s corp_id=%s", org.OrgID, org.CorpID)
		} else {
			log.Printf("[dingtalk/config] organization not found, fallback to default: org_id=%s err=%v", orgID, err)
		}
	}

	missingConfig := []string{}
	if corpID == "" {
		missingConfig = append(missingConfig, "DINGTALK_CORP_ID")
	}

	// 获取可用的企业列表
	orgs := make([]gin.H, 0)
	if activeOrgs, err := database.ListActiveOrganizations(); err == nil && len(activeOrgs) > 0 {
		for _, org := range activeOrgs {
			orgs = append(orgs, organizationOptionFromModel(org))
		}
	} else {
		activeConfigs := dingtalk.ActiveAppConfigs()
		orgs = make([]gin.H, 0, len(activeConfigs))
		for _, cfg := range activeConfigs {
			orgs = append(orgs, gin.H{
				"org_id":   cfg.OrgID,
				"name":     cfg.Name,
				"corp_id":  cfg.CorpID,
				"agent_id": cfg.AgentID,
			})
		}
	}

	log.Printf("[dingtalk/config] host=%s app_home_url=%s redirect_uri=%s missing=%v organizations=%d", c.Request.Host, appHomeURL, redirectURI, missingConfig, len(orgs))

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"corp_id":       corpID,
			"client_id":     clientID,
			"redirect_uri":  redirectURI,
			"app_home_url":  appHomeURL,
			"missing":       missingConfig,
			"organizations": orgs,
		},
	})
}

// DingTalkQRLoginStart 閽夐拤鎵爜鐧诲綍寮€濮?
func DingTalkQRLoginStart(c *gin.Context) {
	state := generateLoginState()
	redirectURI := resolveDingTalkRedirectURI(c)

	// 获取用户选择的组织ID
	orgID := strings.TrimSpace(c.Query("org_id"))
	if orgID == "" {
		orgID = os.Getenv("DINGTALK_SHARED_OAUTH_ORG_ID")
		if orgID == "" {
			orgID = "default"
		}
	}
	log.Printf("[dingtalk/qr/start] host=%s forwarded_host=%s redirect_uri=%s org_id=%s ua=%s", c.Request.Host, c.GetHeader("X-Forwarded-Host"), redirectURI, orgID, c.GetHeader("User-Agent"))

	_, appConfig, err := resolveDingTalkLoginConfig(orgID, "dingtalk/qr/start")
	if err != nil {
		log.Printf("[dingtalk/qr/start] resolve organization config failed: org_id=%s err=%v", orgID, err)
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}

	// 在回调 URL 中附加 org_id 参数
	callbackURL := redirectURI
	if !strings.Contains(callbackURL, "?") {
		callbackURL += "?org_id=" + orgID
	} else {
		callbackURL += "&org_id=" + orgID
	}

	qrCodeURL, err := dingtalk.GetQRCodeWithConfig(state, callbackURL, appConfig)
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
			"qr_code_url":  qrCodeURL,
			"state":        state,
			"redirect_uri": callbackURL,
			"org_id":       orgID,
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
	log.Printf("[dingtalk/in-app] host=%s has_code=%t ua=%s", c.Request.Host, strings.TrimSpace(req.Code) != "", c.GetHeader("User-Agent"))

	orgID := strings.TrimSpace(req.OrgID)
	if orgID == "" {
		orgID = strings.TrimSpace(c.Query("org_id"))
	}
	org, appConfig, err := resolveDingTalkLoginConfig(orgID, "dingtalk/in-app")
	if err != nil {
		log.Printf("[dingtalk/in-app] resolve organization config failed: org_id=%s err=%v", orgID, err)
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}
	orgID = org.OrgID
	log.Printf("[dingtalk/in-app] organization found: org_id=%s corp_id=%s", org.OrgID, org.CorpID)

	userid, err := dingtalk.GetUserIDByInAppCodeForConfig(req.Code, appConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "dingtalk in-app login failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/in-app] resolved_userid=%s", userid)

	userDetail, err := dingtalk.GetUserDetailByUserIDForConfig(userid, appConfig)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get dingtalk user detail failed: " + err.Error(),
		})
		return
	}
	log.Printf("[dingtalk/in-app] user_detail=%v", userDetail)

	cacheDingTalkRosterMembership("dingtalk/in-app", orgID, userid)

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

	userService := service.NewUserServiceWithOrgID(database.DB, orgID)
	user, err := findLocalUserByDingTalkIdentity(userService, orgID, userid)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "query local user failed: " + err.Error(),
			})
			return
		}

		respondDingTalkUserNotSynced(c, "dingtalk_in_app", userid, name)
		return
	}

	user.Name = name
	user.Avatar = avatar
	user.Position = position
	user.DepartmentID = deptID
	user.Status = "active"
	if err := assignUserEmailSafely(userService, user, email); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user email failed: " + err.Error(),
		})
		return
	}
	if err := assignUserMobileSafely(userService, user, mobile); err != nil {
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

	tokenString, expiresAt, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "generate token failed",
		})
		return
	}

	database.DB.Create(&database.LoginLog{
		UserID:      user.UserID,
		UserName:    user.Name,
		LoginType:   "dingtalk_in_app",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"token":      tokenString,
			"user":       buildAuthUserPayload(user),
			"expires_at": expiresAt,
		},
	})
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
	if !validateLoginState(state) {
		log.Printf("[dingtalk/callback] invalid or expired state")
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效或过期的登录状态，请重新扫码",
		})
		return
	}

	// 清理过期 state
	cleanupOldStates()

	if code == "" {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "missing auth code",
		})
		return
	}

	orgID := strings.TrimSpace(c.Query("org_id"))
	org, appConfig, err := resolveDingTalkLoginConfig(orgID, "dingtalk/callback")
	if err != nil {
		log.Printf("[dingtalk/callback] resolve organization config failed: org_id=%s err=%v", orgID, err)
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: err.Error(),
		})
		return
	}
	orgID = org.OrgID
	log.Printf("[dingtalk/callback] organization found: org_id=%s corp_id=%s", org.OrgID, org.CorpID)

	userInfo, err := dingtalk.GetUserInfoByCodeForConfig(code, appConfig)
	if err != nil {
		log.Printf("[dingtalk/callback] GetUserInfoByCode failed: %v", err)
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "get dingtalk user info failed: " + err.Error(),
		})
		return
	}
	// 记录钉钉返回的完整用户信息（用于诊断）
	if userInfoJSON, err := json.Marshal(userInfo); err == nil {
		log.Printf("[dingtalk/callback] user_info_raw: %s", string(userInfoJSON))
	}
	associatedUserID := getStringByKeys(userInfo, "associated_user_id", "associatedUserId", "userid", "userId")
	unionID := getStringByKeys(userInfo, "unionId", "unionid", "union_id")
	openID := getStringByKeys(userInfo, "openId", "openid", "open_id")
	authorizedCorpID := getStringByKeys(userInfo, "corpId", "corpID", "corp_id", "corpid")
	if authorizedCorpID != "" && org.CorpID != "" && !strings.EqualFold(authorizedCorpID, org.CorpID) {
		log.Printf("[dingtalk/callback] reject login because authorized corp mismatches selected org: org_id=%s selected_corp_id=%s authorized_corp_id=%s", orgID, org.CorpID, authorizedCorpID)
		respondDingTalkUserNotInSelectedOrg(c, "dingtalk_qr", openID, "", nil)
		return
	}
	log.Printf("[dingtalk/callback] parsed: associated_user_id=%s, unionid=%s, openid=%s, authorized_corp_id=%s", associatedUserID, unionID, openID, authorizedCorpID)

	dtUserID := associatedUserID
	if dtUserID == "" && unionID != "" {
		resolvedUserID, resolveErr := dingtalk.GetUserIDByUnionIDForConfig(unionID, appConfig)
		if resolveErr == nil {
			dtUserID = resolvedUserID
			log.Printf("[dingtalk/callback] resolved userid from unionid: %s", dtUserID)
		} else {
			log.Printf("[dingtalk/callback] resolve unionid failed: union_id=%s err=%v", unionID, resolveErr)
		}
	}

	// 扫码登录必须能解析到所选企业下的 userid，否则不能仅凭手机号/邮箱兜底，
	// 避免“系统选小铁，钉钉页使用非小铁账号”跨组织登录。
	if dtUserID == "" {
		identityForLog := openID
		if identityForLog == "" {
			identityForLog = unionID
		}
		log.Printf("[dingtalk/callback] reject login because user is not resolved in selected org: org_id=%s identity=%s", orgID, identityForLog)
		respondDingTalkUserNotInSelectedOrg(c, "dingtalk_qr", identityForLog, "", nil)
		return
	}

	var name, email, mobile, avatar, position string
	deptID := "1"
	userDetail, detailErr := dingtalk.GetUserDetailByUserIDForConfig(dtUserID, appConfig)
	if detailErr != nil {
		log.Printf("[dingtalk/callback] reject login because user detail is unavailable in selected org: org_id=%s userid=%s err=%v", orgID, dtUserID, detailErr)
		respondDingTalkUserNotInSelectedOrg(c, "dingtalk_qr", dtUserID, "", nil)
		return
	}
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

	log.Printf("[dingtalk/callback] extracted info: name=%s, email=%s, mobile=%s", name, email, mobile)

	cacheDingTalkRosterMembership("dingtalk/callback", orgID, dtUserID)

	userService := service.NewUserServiceWithOrgID(database.DB, orgID)
	user, err := findLocalUserByDingTalkIdentity(userService, orgID, dtUserID, associatedUserID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("[dingtalk/callback] findLocalUserByDingTalkIdentity error: %v", err)
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "query local user failed: " + err.Error(),
			})
			return
		}

		respondDingTalkUserNotSynced(c, "dingtalk_qr", dtUserID, name)
		return
	} else {
		log.Printf("[dingtalk/callback] user found by userid: user_id=%s", user.UserID)
	}

	user.Name = name
	user.Avatar = avatar
	user.Position = position
	user.DepartmentID = deptID
	user.Status = "active"
	if err := assignUserEmailSafely(userService, user, email); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "update local user email failed: " + err.Error(),
		})
		return
	}
	if err := assignUserMobileSafely(userService, user, mobile); err != nil {
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

	tokenString, expiresAt, err := generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "generate token failed",
		})
		return
	}

	database.DB.Create(&database.LoginLog{
		UserID:      user.UserID,
		UserName:    user.Name,
		LoginType:   "dingtalk_qr",
		LoginStatus: "success",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
	})

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"token":      tokenString,
			"user":       buildAuthUserPayload(user),
			"expires_at": expiresAt,
		},
	})
}

func cacheDingTalkRosterMembership(source, orgID, userID string) {
	orgID = strings.TrimSpace(orgID)
	userID = strings.TrimSpace(userID)
	if orgID == "" || userID == "" {
		return
	}
	if err := database.EnsureOrganizationUser(orgID, userID, "active"); err != nil {
		log.Printf("[%s] cache dingtalk roster membership failed: org_id=%s user_id=%s err=%v", source, orgID, userID, err)
	}
}

func respondDingTalkUserNotInSelectedOrg(c *gin.Context, loginType, userID, userName string, availableOrganizations []gin.H) {
	database.DB.Create(&database.LoginLog{
		UserID:      userID,
		UserName:    userName,
		LoginType:   loginType,
		LoginStatus: "failed",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		ErrorMsg:    "user not in selected organization",
	})

	var data interface{}
	if len(availableOrganizations) > 0 {
		data = gin.H{"available_organizations": availableOrganizations}
	}

	c.JSON(http.StatusForbidden, Response{
		Code:    http.StatusForbidden,
		Data:    data,
		Message: "当前钉钉账号不属于所选组织，请返回后选择正确组织登录",
	})
}

func respondDingTalkUserNotSynced(c *gin.Context, loginType, userID, userName string) {
	database.DB.Create(&database.LoginLog{
		UserID:      userID,
		UserName:    userName,
		LoginType:   loginType,
		LoginStatus: "failed",
		IP:          c.ClientIP(),
		UserAgent:   c.GetHeader("User-Agent"),
		ErrorMsg:    "user not synced",
	})

	c.JSON(http.StatusForbidden, Response{
		Code: http.StatusForbidden,
		Data: gin.H{
			"reason":  "local_user_missing",
			"action":  "sync_org_data",
			"user_id": userID,
		},
		Message: "当前钉钉账号已匹配通讯录，但系统尚未同步本地用户，请联系管理员执行组织同步后再登录",
	})
}

func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.Split(forwardedProto, ",")[0]
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = strings.TrimSpace(c.Request.Host)
	}
	if host == "" {
		return dingtalk.GetAppHomeURL()
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func getStringByKeys(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := data[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func organizationOptionFromModel(org database.Organization) gin.H {
	return gin.H{
		"org_id":   org.OrgID,
		"name":     org.Name,
		"corp_id":  org.CorpID,
		"agent_id": org.AgentID,
	}
}

func dingTalkUserInSelectedOrganization(orgID string, candidates ...string) (bool, error) {
	for _, candidate := range dingtalkIdentityCandidates(orgID, candidates...) {
		ok, err := database.IsUserInOrganization(orgID, candidate)
		if err != nil {
			return false, err
		}
		if ok {
			return true, nil
		}
	}
	return false, nil
}

func availableOrganizationsForDingTalkUser(candidates ...string) ([]gin.H, error) {
	seenCandidates := make(map[string]bool)
	seenOrgs := make(map[string]bool)
	options := make([]gin.H, 0)
	for _, candidate := range dingtalkIdentityCandidates("", candidates...) {
		if seenCandidates[candidate] {
			continue
		}
		seenCandidates[candidate] = true
		orgs, err := database.ListActiveOrganizationsForUser(candidate)
		if err != nil {
			return nil, err
		}
		for _, org := range orgs {
			if seenOrgs[org.OrgID] {
				continue
			}
			seenOrgs[org.OrgID] = true
			options = append(options, organizationOptionFromModel(org))
		}
	}
	return options, nil
}

func findLocalUserByDingTalkIdentity(userService *service.UserService, orgID string, candidates ...string) (*database.User, error) {
	for _, candidate := range dingtalkIdentityCandidates(orgID, candidates...) {
		user, err := userService.GetUserByOrgAndUserID(orgID, candidate)
		if err == nil {
			return user, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	}

	return nil, gorm.ErrRecordNotFound
}

func dingtalkIdentityCandidates(orgID string, candidates ...string) []string {
	orgID = strings.TrimSpace(orgID)
	seen := make(map[string]bool, len(candidates)*2)
	result := make([]string, 0, len(candidates)*2)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}
	for _, candidate := range candidates {
		add(candidate)
	}
	return result
}

func assignUserEmailSafely(userService *service.UserService, user *database.User, email string) error {
	email = strings.TrimSpace(email)
	if email == "" {
		return nil
	}

	existing, err := userService.GetUserByOrgAndEmail(user.OrgID, email)
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip email update for org_id=%s user_id=%s because email=%s already belongs to user_id=%s", user.OrgID, user.UserID, email, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	existing, err = userService.GetUserByEmail(email)
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip email update for org_id=%s user_id=%s because email=%s already belongs to %s/%s", user.OrgID, user.UserID, email, existing.OrgID, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	user.Email = email
	return nil
}

func assignUserMobileSafely(userService *service.UserService, user *database.User, mobile string) error {
	mobile = strings.TrimSpace(mobile)
	if mobile == "" {
		return nil
	}

	existing, err := userService.GetUserByOrgAndMobile(user.OrgID, mobile)
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip mobile update for org_id=%s user_id=%s because mobile=%s already belongs to user_id=%s", user.OrgID, user.UserID, mobile, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	existing, err = userService.GetUserByMobile(mobile)
	if err == nil && existing.ID != user.ID {
		log.Printf("[dingtalk/login] skip mobile update for org_id=%s user_id=%s because mobile=%s already belongs to %s/%s", user.OrgID, user.UserID, mobile, existing.OrgID, existing.UserID)
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	user.Mobile = mobile
	return nil
}

func resolveDingTalkAppHomeURL(c *gin.Context) string {
	if configured := dingtalk.GetConfiguredAppHomeURL(); configured != "" {
		return configured
	}
	return requestBaseURL(c)
}

func resolveDingTalkRedirectURI(c *gin.Context) string {
	if configured := dingtalk.GetConfiguredRedirectURI(); configured != "" {
		return configured
	}
	return resolveDingTalkAppHomeURL(c) + "/callback"
}

// Logout 登出
func Logout(c *gin.Context) {
	// 记录登出日志
	userID, _ := c.Get("userID")
	userName, _ := c.Get("userName")
	if uid, ok := userID.(string); ok {
		uname, _ := userName.(string)
		if user, err := loadUserByAuthID(uid); err == nil {
			uid = user.UserID
			if strings.TrimSpace(uname) == "" {
				uname = user.Name
			}
		}
		database.DB.Create(&database.OperationLog{
			UserID:    uid,
			UserName:  uname,
			Operation: "登出",
			Resource:  "系统",
			IP:        c.ClientIP(),
		})
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

	user, err := loadUserByAuthIDInOrg(c.GetString("orgID"), userID)
	if err != nil {
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
	syncService := service.NewSyncService(database.DB)
	statuses, err := syncService.GetAllSyncStatus()
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
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	// ?all=true 跳过 scope 过滤，用于配置数据权限时展示全部部门
	if c.Query("all") == "true" {
		if !currentUserHasAnyPermission(c, "permission_manage", "user_manage") {
			respondOrgAccessDenied(c)
			return
		}
		orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	scope, err := resolveOrgScope(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取组织范围失败",
			Data:    gin.H{"error": err.Error()},
		})
		return
	}

	orgService := service.NewOrgServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	userQuery := database.DB.Where("id = ? AND deleted_at IS NULL", c.Param("id"))
	if orgID := strings.TrimSpace(c.GetString("orgID")); orgID != "" {
		userQuery = userQuery.Where("org_id = ?", orgID)
	}
	if err := userQuery.First(&user).Error; err != nil {
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
	departmentService := service.NewDepartmentServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	userService := service.NewUserServiceWithOrgID(database.DB, c.GetString("orgID"))

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

	userService := service.NewUserServiceWithOrgID(database.DB, c.GetString("orgID"))
	user, err := userService.GetUserByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "员工不存在",
		})
		return
	}

	// 一并返回员工档案（按 user_id 查），避免前端再发请求
	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
	profile, _ := employeeService.GetProfileByUserID(user.UserID)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"employee": user, "profile": profile},
	})
}

// SyncOrgData 同步组织数据
func SyncOrgData(c *gin.Context) {
	syncService := service.NewSyncService(database.DB)

	orgID := strings.TrimSpace(firstNonEmptyQuery(c, "org_id", "target_org_id"))
	if orgID == "" {
		orgID = strings.TrimSpace(c.GetString("orgID"))
	}
	if orgID == "" {
		orgID = fallbackDingTalkOrgID()
	}
	if orgID != strings.TrimSpace(c.GetString("orgID")) {
		userID := strings.TrimSpace(c.GetString("userID"))
		permissionService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
		allowed, err := permissionService.HasAnyPermissionInOrg(orgID, userID, "attendance_manage", "permission_manage")
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "检查目标组织同步权限失败: " + err.Error(),
			})
			return
		}
		if !allowed {
			c.JSON(http.StatusForbidden, Response{
				Code:    http.StatusForbidden,
				Message: "无目标组织同步权限",
			})
			return
		}
	}
	log.Printf("[SyncOrgData] 开始同步组织数据: org_id=%s", orgID)

	// 使用当前登录组织自己的钉钉应用凭证（多租户隔离）
	_, appConfig, cfgErr := resolveDingTalkLoginConfig(orgID, "SyncOrgData")
	if cfgErr != nil {
		log.Printf("[SyncOrgData] 组织钉钉配置不完整: org_id=%s err=%v", orgID, cfgErr)
		updateSyncStatus(syncService, "departments", "failed", cfgErr.Error())
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "组织钉钉应用配置不完整: " + cfgErr.Error(),
		})
		return
	}

	// 同步部门
	depts, deptErr := dingtalk.SyncDepartmentsForConfig(appConfig)
	deptCount := 0
	deptStatus := "success"
	deptErrMsg := ""
	if deptErr != nil {
		deptStatus = "failed"
		deptErrMsg = deptErr.Error()
		log.Printf("[SyncOrgData] 部门同步失败: %v", deptErr)
		updateSyncStatus(syncService, "departments", "failed", deptErrMsg)
	} else {
		orgService := service.NewOrgServiceWithOrgID(database.DB, orgID)
		deptResult, err := orgService.SyncDepartmentsWithChangeLog(orgID, dingtalkDepartmentsToOrgSyncItems(depts), "dingtalk_sync")
		if err != nil {
			deptStatus = "failed"
			deptErrMsg = err.Error()
			log.Printf("[SyncOrgData] 部门落库失败: %v", err)
		} else {
			deptCount = deptResult.Count
			updateSyncStatus(syncService, "departments", "success", fmt.Sprintf("同步 %d 个部门", deptCount))
		}
	}

	// 同步用户（复用已有部门列表，避免重复调用 SyncDepartments）
	var users []dingtalk.UserInfo
	var userErr error
	if deptErr == nil {
		users, userErr = dingtalk.SyncUsersWithDeptsForConfig(appConfig, depts)
	}
	userCount := 0
	positionMissingCount := 0
	userStatus := "success"
	userErrMsg := ""
	defaultRoleAssignedCount := 0
	overwriteEmpty := shouldOverwriteEmptyDingTalkOrgFields(c)
	if deptErr != nil {
		userStatus = "failed"
		userErrMsg = "部门同步失败，已跳过用户同步: " + deptErrMsg
		updateSyncStatus(syncService, "users", userStatus, userErrMsg)
	} else if userErr != nil {
		userStatus = "failed"
		userErrMsg = userErr.Error()
		log.Printf("[SyncOrgData] 用户同步失败: %v", userErr)
	} else {
		userService := service.NewUserServiceWithOrgID(database.DB, orgID)
		employeeService := service.NewEmployeeServiceWithOrgID(database.DB, orgID)
		permissionService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
		for _, u := range users {
			deptID := ""
			if len(u.DeptIDList) > 0 {
				deptID = fmt.Sprintf("%d", u.DeptIDList[0])
			}
			status := "active"
			if !u.Active {
				status = "inactive"
			}
			existing, err := userService.GetUserByOrgAndUserID(orgID, u.UserID)
			if err != nil {
				newUser := newLocalUserFromDingTalk(u, deptID, status)
				newUser.OrgID = orgID
				if err := userService.CreateUser(newUser); err != nil {
					userStatus = "failed"
					userErrMsg = err.Error()
					log.Printf("[SyncOrgData] 创建用户 %s 失败: %v", u.UserID, err)
					continue
				} else if assigned, err := assignDefaultEmployeeRoleForSyncedUser(permissionService, orgID, u.UserID, "SyncOrgData"); err != nil {
					userStatus = "failed"
					userErrMsg = err.Error()
				} else if assigned {
					defaultRoleAssignedCount++
				}
				// 同时创建员工档案
				profile := &database.EmployeeProfile{
					UserID:     u.UserID,
					EmployeeID: u.UserID,
				}
				applyDingTalkProfileFields(profile, u, status)
				if err := employeeService.CreateProfile(profile); err != nil {
					userStatus = "failed"
					userErrMsg = err.Error()
					log.Printf("[SyncOrgData] 创建员工档案 %s 失败: %v", u.UserID, err)
					continue
				}
			} else {
				applyDingTalkOrgUser(existing, u, deptID, status, overwriteEmpty)
				if err := userService.UpdateUser(existing); err != nil {
					userStatus = "failed"
					userErrMsg = err.Error()
					log.Printf("[SyncOrgData] 更新用户 %s 失败: %v", u.UserID, err)
					continue
				}
				// 检查是否存在员工档案
				profile, profileErr := employeeService.GetProfileByUserID(u.UserID)
				if profileErr != nil {
					// 创建员工档案
					profile := &database.EmployeeProfile{
						UserID:     u.UserID,
						EmployeeID: u.UserID,
					}
					applyDingTalkProfileFields(profile, u, status)
					if err := employeeService.CreateProfile(profile); err != nil {
						userStatus = "failed"
						userErrMsg = err.Error()
						log.Printf("[SyncOrgData] 创建员工档案 %s 失败: %v", u.UserID, err)
						continue
					}
				} else {
					// 更新员工档案：始终同步入职日期（若钉钉有值则覆盖）
					applyDingTalkProfileFields(profile, u, status)
					if err := employeeService.UpdateProfile(profile); err != nil {
						userStatus = "failed"
						userErrMsg = err.Error()
						log.Printf("[SyncOrgData] 更新员工档案 %s 失败: %v", u.UserID, err)
						continue
					}
				}
			}
			if strings.TrimSpace(u.Position) == "" {
				positionMissingCount++
			}
			if err := database.EnsureOrganizationUser(orgID, u.UserID, status); err != nil {
				userStatus = "failed"
				userErrMsg = err.Error()
				log.Printf("[SyncOrgData] 维护组织用户关系失败: org_id=%s user_id=%s err=%v", orgID, u.UserID, err)
				continue
			}
			userCount++
		}
		userSyncMessage := fmt.Sprintf("同步 %d 个用户", userCount)
		if userStatus == "failed" && userErrMsg != "" {
			userSyncMessage = fmt.Sprintf("同步 %d 个用户，部分失败: %s", userCount, userErrMsg)
		}
		updateSyncStatus(syncService, "users", userStatus, userSyncMessage)
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data: gin.H{
			"sync_status": gin.H{
				"departments": gin.H{"count": deptCount, "status": deptStatus, "error": deptErrMsg},
				"employees":   gin.H{"count": userCount, "status": userStatus, "error": userErrMsg, "position_missing_count": positionMissingCount, "overwrite_empty": overwriteEmpty, "default_role_assigned_count": defaultRoleAssignedCount},
				"sync_time":   time.Now(),
			},
		},
	})
}

// GetAttendanceRecords 获取考勤记录列表
func GetAttendanceRecords(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"org_id":        strings.TrimSpace(c.GetString("orgID")),
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

// GetAttendanceStats 获取考勤统计
func GetAttendanceStats(c *gin.Context) {
	filters := map[string]string{
		"org_id":        strings.TrimSpace(c.GetString("orgID")),
		"start_date":    c.Query("start_date"),
		"end_date":      c.Query("end_date"),
		"department_id": c.Query("department_id"),
	}
	if !currentUserHasAnyPermission(c, "attendance_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	attendanceService := service.NewAttendanceService(middleware.RequestDB(c))
	stats, err := attendanceService.GetStats(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取考勤统计失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    stats,
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

	orgID := strings.TrimSpace(c.GetString("orgID"))

	if req.Force {
		cst := time.FixedZone("CST", 8*3600)
		start, err1 := time.ParseInLocation("2006-01-02", req.StartDate, cst)
		end, err2 := time.ParseInLocation("2006-01-02", req.EndDate, cst)
		if err1 != nil || err2 != nil {
			c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "日期格式错误"})
			return
		}
		end = end.AddDate(0, 0, 1) // 包含 end 当天
		// Attendance 表本身没有 org_id 列，按当前组织的 user_id 集合限定删除范围，
		// 避免跨企业删掉其他组织的考勤。
		deleteQuery := database.DB.Where("check_time >= ? AND check_time < ?", start, end)
		if orgID != "" {
			deleteQuery = deleteQuery.Where("user_id IN (SELECT user_id FROM users WHERE org_id = ? AND deleted_at IS NULL)", orgID)
		}
		if err := deleteQuery.Delete(&database.Attendance{}).Error; err != nil {
			c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "清理旧记录失败: " + err.Error()})
			return
		}
	}

	syncService := service.NewSyncService(database.DB)

	// 获取当前组织下所有用户的钉钉 UserID
	var users []database.User
	usersQuery := database.DB.Select("user_id, name")
	if orgID != "" {
		usersQuery = usersQuery.Where("org_id = ?", orgID)
	}
	usersQuery.Find(&users)

	var userIDs []string
	userNameMap := make(map[string]string)
	for _, u := range users {
		if u.UserID != "" && u.UserID != "admin" {
			userIDs = append(userIDs, u.UserID)
			userNameMap[u.UserID] = u.Name
		}
	}

	if len(userIDs) == 0 {
		updateSyncStatus(syncService, "attendance", "success", "没有需要同步的用户")
		c.JSON(http.StatusOK, Response{
			Code:    http.StatusOK,
			Message: "success",
			Data: gin.H{
				"sync_status": gin.H{"count": 0, "status": "success", "sync_time": time.Now()},
			},
		})
		return
	}

	records, err := dingtalk.GetAttendance(userIDs, req.StartDate, req.EndDate)
	if err != nil {
		updateSyncStatus(syncService, "attendance", "failed", err.Error())
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
			OrgID:     orgID,
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

		if err := service.NewAttendanceService(database.DB).SaveRecord(record); err != nil {
			updateSyncStatus(syncService, "attendance", "failed", err.Error())
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "鍚屾鑰冨嫟澶辫触: " + err.Error(),
			})
			return
		}
		count++
	}

	updateSyncStatus(syncService, "attendance", "success", fmt.Sprintf("同步 %d 条考勤记录", count))

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
	if user, err := loadUserByAuthID(uid); err == nil {
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

	attendanceService := service.NewAttendanceService(database.DB)
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

	attendanceService := service.NewAttendanceService(database.DB)
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
	attendanceService := service.NewAttendanceService(database.DB)
	status, err := attendanceService.GetLastSyncTime()
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
	countQuery := database.DB.Model(&database.Attendance{})
	orgID := strings.TrimSpace(c.GetString("orgID"))
	if orgID != "" {
		countQuery = countQuery.Where("org_id = ?", orgID)
	}
	if !currentUserHasAnyPermission(c, "attendance_manage") {
		filters := map[string]string{}
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
		if userID := strings.TrimSpace(filters["user_id"]); userID != "" {
			countQuery = countQuery.Where("user_id = ?", userID)
		} else if departmentID := strings.TrimSpace(filters["department_id"]); departmentID != "" {
			if orgID != "" {
				countQuery = countQuery.Where("user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id = ? AND deleted_at IS NULL)", orgID, departmentID)
			} else {
				countQuery = countQuery.Where("user_id IN (SELECT user_id FROM users WHERE department_id = ? AND deleted_at IS NULL)", departmentID)
			}
		} else if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
			if orgID != "" {
				countQuery = countQuery.Where("user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id IN ? AND deleted_at IS NULL)", orgID, departmentIDs)
			} else {
				countQuery = countQuery.Where("user_id IN (SELECT user_id FROM users WHERE department_id IN ? AND deleted_at IS NULL)", departmentIDs)
			}
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
	approvalService := service.NewApprovalService(database.DB)
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
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"status":       c.Query("status"),
		"template_id":  c.Query("template_id"),
		"applicant_id": c.Query("applicant_id"),
		"start_date":   c.Query("start_date"),
		"end_date":     c.Query("end_date"),
	}

	approvalService := service.NewApprovalService(database.DB)
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
	id := c.Param("id")

	approvalService := service.NewApprovalService(database.DB)
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
	var req struct {
		StartDate   string `json:"start_date"`
		EndDate     string `json:"end_date"`
		ProcessCode string `json:"process_code"`
	}
	c.ShouldBindJSON(&req)

	if req.StartDate == "" {
		req.StartDate = time.Now().AddDate(0, -1, 0).Format("2006-01-02")
	}
	if req.EndDate == "" {
		req.EndDate = time.Now().Format("2006-01-02")
	}

	syncService := service.NewSyncService(database.DB)

	req.ProcessCode = strings.TrimSpace(req.ProcessCode)
	if req.ProcessCode == "" {
		updateSyncStatus(syncService, "approvals", "failed", "缺少 process_code，未执行审批同步")
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "请在请求中提供 process_code 参数",
		})
		return
	}

	instances, err := dingtalk.GetApprovals(req.ProcessCode, req.StartDate, req.EndDate)
	if err != nil {
		updateSyncStatus(syncService, "approvals", "failed", err.Error())
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "同步审批失败: " + err.Error(),
		})
		return
	}

	// 写入数据库
	count := 0
	for _, inst := range instances {
		createTime, _ := time.Parse("2006-01-02 15:04:05", inst.CreateTime)
		finishTime, _ := time.Parse("2006-01-02 15:04:05", inst.FinishTime)

		// 将 form_component_values 转为 content map
		content := make(map[string]interface{})
		for _, fv := range inst.FormValues {
			name, _ := fv["name"].(string)
			value, _ := fv["value"].(string)
			if name != "" {
				content[name] = value
			}
		}

		approval := &database.Approval{
			ProcessID:     inst.ProcessInstanceID,
			Title:         inst.Title,
			ApplicantID:   inst.OriginatorUserID,
			ApplicantName: inst.OriginatorUserID,
			Status:        inst.Status,
			CreateTime:    createTime,
			FinishTime:    finishTime,
			Content:       content,
			Extension: map[string]interface{}{
				"result":       inst.Result,
				"process_code": req.ProcessCode,
			},
		}

		// Upsert by process_id
		var existing database.Approval
		if err := database.DB.Where("process_id = ?", inst.ProcessInstanceID).First(&existing).Error; err != nil {
			database.DB.Create(approval)
		} else {
			existing.Status = inst.Status
			existing.FinishTime = finishTime
			existing.Content = content
			existing.Extension = map[string]interface{}{
				"result":       inst.Result,
				"process_code": req.ProcessCode,
			}
			database.DB.Save(&existing)
		}
		count++
	}

	updateSyncStatus(syncService, "approvals", "success", fmt.Sprintf("同步 %d 个审批实例", count))

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

// GetRoles 获取角色列表
func GetRoles(c *gin.Context) {
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
	if err := permService.UpdateRole(role); err != nil {
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
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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

func resolvePermissionTargetOrgID(c *gin.Context, explicitOrgID, userID string) string {
	if orgID := strings.TrimSpace(explicitOrgID); orgID != "" {
		return orgID
	}
	if orgID := strings.TrimSpace(c.Query("org_id")); orgID != "" {
		return orgID
	}
	if orgID := strings.TrimSpace(c.GetString("orgID")); orgID != "" {
		return orgID
	}
	if user, err := loadUserByAuthID(userID); err == nil && strings.TrimSpace(user.OrgID) != "" {
		return user.OrgID
	}
	return "default"
}

// GetUserRoles 获取指定用户的角色列表
func GetUserRoles(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, Response{Code: http.StatusBadRequest, Message: "user_id 不能为空"})
		return
	}
	orgID := resolvePermissionTargetOrgID(c, "", userID)
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
	orgID := resolvePermissionTargetOrgID(c, req.OrgID, req.UserID)
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	if err := permService.AssignUserRoleInOrg(orgID, req.UserID, req.RoleID); err != nil {
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
	orgID := resolvePermissionTargetOrgID(c, req.OrgID, req.UserID)
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	if err := permService.RemoveUserRoleInOrg(orgID, req.UserID, req.RoleID); err != nil {
		c.JSON(http.StatusInternalServerError, Response{Code: http.StatusInternalServerError, Message: "移除角色失败"})
		return
	}
	c.JSON(http.StatusOK, Response{Code: http.StatusOK, Message: "角色移除成功"})
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
	orgID := resolvePermissionTargetOrgID(c, "", "")
	permService := service.NewPermissionServiceWithOrgID(database.DB, orgID)
	users, err := permService.GetRoleUsersInOrg(orgID, roleID)
	if err != nil {
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
	orgID := resolvePermissionTargetOrgID(c, "", userID)
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
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	permService := service.NewPermissionServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	auditService := service.NewAuditService(database.DB)
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
	// 任务列表基于同步状态表动态生成
	syncService := service.NewSyncService(database.DB)

	jobs := []gin.H{
		{"id": "1", "name": "同步用户数据", "description": "从钉钉同步用户数据", "type": "sync_users", "status": "idle"},
		{"id": "2", "name": "同步部门数据", "description": "从钉钉同步部门数据", "type": "sync_departments", "status": "idle"},
		{"id": "3", "name": "同步考勤数据", "description": "从钉钉同步考勤数据", "type": "sync_attendance", "status": "idle"},
	}

	typeMap := map[string]string{"1": "users", "2": "departments", "3": "attendance"}
	for i, job := range jobs {
		syncType := typeMap[job["id"].(string)]
		if status, err := syncService.GetSyncStatus(syncType); err == nil {
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

	typeMap := map[string]string{"1": "users", "2": "departments", "3": "attendance"}
	syncType, ok := typeMap[id]
	if !ok {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "任务不存在",
		})
		return
	}

	syncService := service.NewSyncService(database.DB)
	updateSyncStatus(syncService, syncType, "success", "手动执行任务")

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
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))

	filters := map[string]string{
		"department_id": c.Query("department_id"),
		"status":        c.Query("status"),
	}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
	profile, err := employeeService.GetProfileByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "档案不存在",
		})
		return
	}
	if !currentUserHasAnyPermission(c, "user_manage") {
		scope, err := resolveOrgScope(c)
		if err != nil {
			c.JSON(http.StatusInternalServerError, Response{
				Code:    http.StatusInternalServerError,
				Message: "鑾峰彇缁勭粐鑼冨洿澶辫触",
				Data:    gin.H{"error": err.Error()},
			})
			return
		}
		user, err := loadUserByUserIDInOrg(c.GetString("orgID"), profile.UserID)
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
	var profile database.EmployeeProfile

	if err := c.ShouldBindJSON(&profile); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if profile.ProfileStatus == "" {
		profile.ProfileStatus = "active"
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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

// GetTransfers 获取调动记录列表
func GetEmployeeLifecycleLedger(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

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

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	filters := map[string]string{"status": c.Query("status")}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	var transfer database.EmployeeTransfer
	if err := c.ShouldBindJSON(&transfer); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if transfer.Status == "" {
		transfer.Status = "pending"
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	filters := map[string]string{"status": c.Query("status")}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	var resignation database.EmployeeResignation
	if err := c.ShouldBindJSON(&resignation); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if resignation.Status == "" {
		resignation.Status = "pending"
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	filters := map[string]string{"status": c.Query("status")}
	if !currentUserHasAnyPermission(c, "user_manage") {
		if _, ok := resolveScopeAndApplyFilters(c, filters); !ok {
			return
		}
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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
	var onboarding database.EmployeeOnboarding
	if err := c.ShouldBindJSON(&onboarding); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	if onboarding.Status == "" {
		onboarding.Status = "pending"
	}

	employeeService := service.NewEmployeeServiceWithOrgID(database.DB, c.GetString("orgID"))
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

// GetTalentAnalysisList 获取人才分析列表
func GetTalentAnalysisList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	departmentID := c.Query("department_id")

	talentService := service.NewTalentService(database.DB)
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
	id := c.Param("id")

	talentService := service.NewTalentService(database.DB)
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
	var analysis database.TalentAnalysis
	if err := c.ShouldBindJSON(&analysis); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	talentService := service.NewTalentService(database.DB)
	if err := talentService.Create(&analysis); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建分析记录失败",
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"analysis": analysis},
	})
}

// ===================== 大小周管理 =====================

// GetWeekScheduleRules 获取所有大小周规则
func GetWeekScheduleRules(c *gin.Context) {
	svc := service.NewWeekScheduleService(database.DB)
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
	var rule database.WeekScheduleRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.CreateRule(&rule); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建规则失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"rule": rule},
	})
}

// UpdateWeekScheduleRule 更新大小周规则
func UpdateWeekScheduleRule(c *gin.Context) {
	idStr := c.Param("id")
	svc := service.NewWeekScheduleService(database.DB)

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

	var input database.WeekScheduleRule
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
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
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
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
	usersQuery := database.DB.Where("user_id IN ?", input.UserIDs)
	if orgID := strings.TrimSpace(c.GetString("orgID")); orgID != "" {
		usersQuery = usersQuery.Where("org_id = ?", orgID)
	}
	if err := usersQuery.Find(&users).Error; err != nil {
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

	svc := service.NewWeekScheduleService(database.DB)
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

	shifts, err := dingtalk.GetShiftList()
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
	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "未配置 DINGTALK_ADMIN_USER_ID",
		})
		return
	}

	groups, err := dingtalk.GetAttendanceGroups()
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "获取考勤组失败: " + err.Error(),
		})
		return
	}

	shifts, _ := dingtalk.GetShiftList()
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

		detail, detailErr := dingtalk.GetAttendanceGroup(opUserID, int64(gid))
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
			restID := dingtalk.GetAttendanceGroupRestClassID(detail)
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

	opUserID := os.Getenv("DINGTALK_ADMIN_USER_ID")
	if opUserID == "" {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "未配置 DINGTALK_ADMIN_USER_ID",
		})
		return
	}

	shiftID, err := dingtalk.CreateShift(opUserID, input.Name, input.CheckInTime, input.CheckOutTime)
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
			if user, err := loadUserByUserIDInOrg(c.GetString("orgID"), userID); err == nil {
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

	svc := service.NewWeekScheduleService(middleware.RequestDB(c))
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
	var override database.WeekScheduleOverride
	if err := c.ShouldBindJSON(&override); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.SetOverride(&override); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "设置覆盖失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"override": override},
	})
}

// DeleteWeekOverride 取消手动覆盖
func DeleteWeekOverride(c *gin.Context) {
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
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

// SyncWeekToDingTalk 将大小周配置推送到钉钉
func SyncWeekToDingTalk(c *gin.Context) {
	var input struct {
		Weeks int `json:"weeks"`
	}
	c.ShouldBindJSON(&input)
	if input.Weeks <= 0 {
		input.Weeks = 4
	}

	svc := service.NewWeekScheduleService(database.DB)
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

// SyncWeekFromDingTalk 从钉钉拉取大小周配置
func SyncWeekFromDingTalk(c *gin.Context) {
	svc := service.NewWeekScheduleService(database.DB)
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

	svc := service.NewWeekScheduleService(database.DB)
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
	yearStr := c.DefaultQuery("year", fmt.Sprintf("%d", time.Now().Year()))
	var year int
	fmt.Sscanf(yearStr, "%d", &year)
	if year <= 0 {
		year = time.Now().Year()
	}

	svc := service.NewWeekScheduleService(database.DB)
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
	var holiday database.StatutoryHoliday
	if err := c.ShouldBindJSON(&holiday); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	if err := svc.CreateHoliday(&holiday); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建节假日失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"holiday": holiday},
	})
}

// BatchCreateHolidays 批量创建节假日
func BatchCreateHolidays(c *gin.Context) {
	var input struct {
		Holidays []database.StatutoryHoliday `json:"holidays"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "参数错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
	created, err := svc.BatchCreateHolidays(input.Holidays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "批量创建失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "success",
		Data:    gin.H{"created": created, "total": len(input.Holidays)},
	})
}

// SyncHolidaysFromJuhe 从聚合数据API同步节假日
func SyncHolidaysFromJuhe(c *gin.Context) {
	svc := service.NewWeekScheduleService(database.DB)
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
	idStr := c.Param("id")
	var id uint
	if _, err := fmt.Sscanf(idStr, "%d", &id); err != nil {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "ID 格式错误",
		})
		return
	}

	svc := service.NewWeekScheduleService(database.DB)
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

	svc := service.NewShiftConfigService(database.DB)
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

	svc := service.NewShiftConfigService(database.DB)
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

	svc := service.NewShiftConfigService(database.DB)
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
	svc := service.NewShiftConfigService(database.DB)
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

	svc := service.NewShiftConfigService(database.DB)
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

// UploadFile 文件上传
func UploadFile(c *gin.Context) {
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

	// 检查上传目录
	uploadDir := "uploads"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "创建上传目录失败",
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
	filename := fmt.Sprintf("%s%s", hex.EncodeToString(randBytes), ext)
	filePath := filepath.Join(uploadDir, filename)

	// 保存文件
	if err := c.SaveUploadedFile(file, filePath); err != nil {
		c.JSON(http.StatusInternalServerError, Response{
			Code:    http.StatusInternalServerError,
			Message: "保存文件失败",
		})
		return
	}

	// 返回文件URL
	fileURL := fmt.Sprintf("/api/v1/files/%s", filename)

	c.JSON(http.StatusOK, Response{
		Code:    http.StatusOK,
		Message: "上传成功",
		Data: gin.H{
			"url":  fileURL,
			"name": file.Filename,
			"size": file.Size,
		},
	})
}

// ServeFile 提供文件访问
func ServeFile(c *gin.Context) {
	filename := strings.TrimSpace(c.Param("filename"))
	if !isSafeUploadFilename(filename) {
		c.JSON(http.StatusBadRequest, Response{
			Code:    http.StatusBadRequest,
			Message: "无效的文件名",
		})
		return
	}

	filePath := filepath.Join("uploads", filename)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, Response{
			Code:    http.StatusNotFound,
			Message: "文件不存在",
		})
		return
	}

	ext := strings.ToLower(filepath.Ext(filename))
	disposition := "attachment"
	if ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif" || ext == ".webp" ||
		ext == ".pdf" || ext == ".txt" || ext == ".csv" || ext == ".md" {
		disposition = "inline"
	}
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Referrer-Policy", "no-referrer")
	c.Header("Content-Disposition", fmt.Sprintf(`%s; filename="%s"`, disposition, filename))
	c.File(filePath)
}
