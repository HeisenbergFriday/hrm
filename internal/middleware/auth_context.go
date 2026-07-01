package middleware

import (
	"encoding/json"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"peopleops/internal/database"
	"peopleops/internal/service"
)

const authContextKey = "authContext"

type AuthContext struct {
	RawUserID        string
	UserID           string
	User             *database.User
	Roles            []database.Role
	PermissionSet    map[string]struct{}
	MenuKeySet       map[string]struct{}
	DataPermissions  []database.DataPermission
	DataScope        *service.OrgDataScope
	dataScopeLoaded  bool
	dataScopeLoadErr error
}

func GetAuthContext(c *gin.Context) (*AuthContext, error) {
	if existing, ok := c.Get(authContextKey); ok {
		if authCtx, ok := existing.(*AuthContext); ok {
			return authCtx, nil
		}
	}

	authCtx, err := loadAuthContext(c)
	if err != nil {
		return nil, err
	}
	c.Set(authContextKey, authCtx)
	return authCtx, nil
}

func HasAnyPermission(c *gin.Context, permissionCodes ...string) (bool, error) {
	authCtx, err := GetAuthContext(c)
	if err != nil {
		return false, err
	}
	if len(permissionCodes) == 0 {
		return false, nil
	}
	for _, code := range permissionCodes {
		if _, ok := authCtx.PermissionSet[code]; ok {
			return true, nil
		}
	}
	return false, nil
}

func HasMenuPermission(c *gin.Context, menuKey string) (bool, error) {
	authCtx, err := GetAuthContext(c)
	if err != nil {
		return false, err
	}
	needle := service.NormalizeMenuPermissionKey(menuKey)
	_, ok := authCtx.MenuKeySet[needle]
	return ok, nil
}

func UserDataScope(c *gin.Context) (*service.OrgDataScope, error) {
	authCtx, err := GetAuthContext(c)
	if err != nil {
		return nil, err
	}
	if authCtx.dataScopeLoaded {
		return authCtx.DataScope, authCtx.dataScopeLoadErr
	}
	authCtx.DataScope, authCtx.dataScopeLoadErr = buildDataScope(authCtx)
	authCtx.dataScopeLoaded = true
	return authCtx.DataScope, authCtx.dataScopeLoadErr
}

func loadAuthContext(c *gin.Context) (*AuthContext, error) {
	rawUserID := strings.TrimSpace(c.GetString("userID"))
	authCtx := &AuthContext{
		RawUserID:     rawUserID,
		UserID:        rawUserID,
		PermissionSet: make(map[string]struct{}),
		MenuKeySet:    make(map[string]struct{}),
	}
	if rawUserID == "" {
		return authCtx, nil
	}

	db := RequestDB(c)
	user, normalizedUserID, err := loadCurrentUser(db, rawUserID)
	if err != nil {
		return nil, err
	}
	authCtx.User = user
	authCtx.UserID = normalizedUserID

	roles, err := loadCurrentRoles(db, normalizedUserID)
	if err != nil {
		return nil, err
	}
	authCtx.Roles = roles
	roleIDs := authRoleIDs(roles)

	if len(roleIDs) > 0 {
		if err := loadCurrentPermissions(db, roleIDs, authCtx.PermissionSet); err != nil {
			return nil, err
		}
		if err := loadCurrentMenuKeys(db, roleIDs, authCtx.MenuKeySet); err != nil {
			return nil, err
		}
		dataPermissions, err := loadCurrentDataPermissions(db, roleIDs)
		if err != nil {
			return nil, err
		}
		authCtx.DataPermissions = dataPermissions
	}

	return authCtx, nil
}

func loadCurrentUser(db *gorm.DB, rawUserID string) (*database.User, string, error) {
	var user database.User
	err := db.Where("user_id = ? AND status = ? AND deleted_at IS NULL", rawUserID, "active").First(&user).Error
	if err == nil {
		return &user, user.UserID, nil
	}
	if err != nil && !isRecordNotFound(err) {
		return nil, rawUserID, err
	}
	if looksNumericID(rawUserID) {
		err = db.Where("id = ? AND status = ? AND deleted_at IS NULL", rawUserID, "active").First(&user).Error
		if err == nil {
			return &user, user.UserID, nil
		}
		if err != nil && !isRecordNotFound(err) {
			return nil, rawUserID, err
		}
	}
	return nil, rawUserID, nil
}

func loadCurrentRoles(db *gorm.DB, userID string) ([]database.Role, error) {
	var roles []database.Role
	err := db.
		Joins("JOIN user_roles ON user_roles.role_id = roles.id AND user_roles.deleted_at IS NULL").
		Where("user_roles.user_id = ? AND roles.deleted_at IS NULL", userID).
		Find(&roles).Error
	return roles, err
}

func loadCurrentPermissions(db *gorm.DB, roleIDs []uint, dest map[string]struct{}) error {
	var permissions []database.Permission
	if err := db.
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id AND role_permissions.deleted_at IS NULL").
		Where("role_permissions.role_id IN ? AND permissions.deleted_at IS NULL", roleIDs).
		Distinct().
		Find(&permissions).Error; err != nil {
		return err
	}
	for _, permission := range permissions {
		dest[permission.Code] = struct{}{}
	}
	return nil
}

func loadCurrentMenuKeys(db *gorm.DB, roleIDs []uint, dest map[string]struct{}) error {
	var menuPermissions []database.MenuPermission
	if err := db.Where("role_id IN ? AND deleted_at IS NULL", roleIDs).Find(&menuPermissions).Error; err != nil {
		return err
	}
	for _, record := range menuPermissions {
		keys, err := service.ParseMenuKeys(record.MenuKeys)
		if err != nil {
			return err
		}
		for _, key := range service.NormalizeMenuPermissionKeys(keys) {
			dest[key] = struct{}{}
		}
	}
	return nil
}

func loadCurrentDataPermissions(db *gorm.DB, roleIDs []uint) ([]database.DataPermission, error) {
	var dataPermissions []database.DataPermission
	err := db.Where("role_id IN ? AND deleted_at IS NULL", roleIDs).Find(&dataPermissions).Error
	return dataPermissions, err
}

func buildDataScope(authCtx *AuthContext) (*service.OrgDataScope, error) {
	if strings.EqualFold(authCtx.UserID, "admin") {
		return nil, nil
	}
	if len(authCtx.Roles) == 0 || len(authCtx.DataPermissions) == 0 {
		return selfDataScope(authCtx.UserID), nil
	}

	hasAll := false
	hasAnyConfig := false
	departmentIDs := make(map[string]struct{})
	for _, dp := range authCtx.DataPermissions {
		hasAnyConfig = true
		switch dp.Scope {
		case "all":
			hasAll = true
		case "department":
			var keys []string
			if err := json.Unmarshal([]byte(dp.DepartmentKeys), &keys); err != nil {
				continue
			}
			for _, key := range keys {
				if trimmed := strings.TrimSpace(key); trimmed != "" {
					departmentIDs[trimmed] = struct{}{}
				}
			}
		}
	}
	if hasAll {
		return nil, nil
	}
	if !hasAnyConfig {
		return selfDataScope(authCtx.UserID), nil
	}
	if len(departmentIDs) > 0 {
		depts := make([]string, 0, len(departmentIDs))
		for deptID := range departmentIDs {
			depts = append(depts, deptID)
		}
		return &service.OrgDataScope{Mode: "department", DepartmentIDs: depts, UserIDs: []string{}}, nil
	}
	return selfDataScope(authCtx.UserID), nil
}

func selfDataScope(userID string) *service.OrgDataScope {
	return &service.OrgDataScope{Mode: "self", DepartmentIDs: []string{}, UserIDs: []string{userID}}
}

func authRoleIDs(roles []database.Role) []uint {
	roleIDs := make([]uint, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, role.ID)
	}
	return roleIDs
}

func isRecordNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}

func looksNumericID(value string) bool {
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
