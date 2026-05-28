package service

import (
	"encoding/json"
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

type PermissionService struct {
	roleRepo           *repository.RoleRepository
	permissionRepo     *repository.PermissionRepository
	userRoleRepo       *repository.UserRoleRepository
	rolePermissionRepo *repository.RolePermissionRepository
	menuPermRepo       *repository.MenuPermissionRepository
	dataPermRepo       *repository.DataPermissionRepository
	deptRepo           *repository.DepartmentRepository
	userRepo           *repository.UserRepository
}

func NewPermissionService(db *gorm.DB) *PermissionService {
	return &PermissionService{
		roleRepo:           repository.NewRoleRepository(db),
		permissionRepo:     repository.NewPermissionRepository(db),
		userRoleRepo:       repository.NewUserRoleRepository(db),
		rolePermissionRepo: repository.NewRolePermissionRepository(db),
		menuPermRepo:       repository.NewMenuPermissionRepository(db),
		dataPermRepo:       repository.NewDataPermissionRepository(db),
		deptRepo:           repository.NewDepartmentRepository(db),
		userRepo:           repository.NewUserRepository(db),
	}
}

func (s *PermissionService) GetRoles() ([]database.Role, int64, error) {
	return s.roleRepo.FindAll()
}

func (s *PermissionService) CreateRole(role *database.Role) error {
	return s.roleRepo.Create(role)
}

func (s *PermissionService) UpdateRole(role *database.Role) error {
	return s.roleRepo.Update(role)
}

func (s *PermissionService) GetPermissions() ([]database.Permission, int64, error) {
	return s.permissionRepo.FindAll()
}

func (s *PermissionService) normalizeUserID(userID string) string {
	normalized := strings.TrimSpace(userID)
	if normalized == "" {
		return normalized
	}
	// 先按 user_id 字段查询（钉钉 userId 等字符串标识，即使外观像数字）
	if user, err := s.userRepo.FindByUserID(normalized); err == nil && user.UserID != "" {
		return user.UserID
	}
	// 再按主键 id 查询（JWT 中可能直接传数字主键）
	if looksNumericID(normalized) {
		if user, err := s.userRepo.FindByID(normalized); err == nil && user.UserID != "" {
			return user.UserID
		}
	}
	return normalized
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

// GetUserPermissions 返回用户通过角色获得的所有权限码
func (s *PermissionService) GetUserPermissions(userID string) ([]string, error) {
	perms, err := s.rolePermissionRepo.FindByUserRole(s.normalizeUserID(userID))
	if err != nil {
		return nil, err
	}
	codes := make([]string, len(perms))
	for i, p := range perms {
		codes[i] = p.Code
	}
	return codes, nil
}

// HasPermission 检查用户是否具有指定权限码
func (s *PermissionService) HasPermission(userID string, permissionCode string) (bool, error) {
	perms, err := s.GetUserPermissions(userID)
	if err != nil {
		return false, err
	}
	for _, code := range perms {
		if code == permissionCode {
			return true, nil
		}
	}
	return false, nil
}

// HasAnyPermission 检查用户是否具有任一指定权限码
func (s *PermissionService) HasAnyPermission(userID string, codes ...string) (bool, error) {
	perms, err := s.GetUserPermissions(userID)
	if err != nil {
		return false, err
	}
	permSet := make(map[string]struct{}, len(perms))
	for _, code := range perms {
		permSet[code] = struct{}{}
	}
	for _, code := range codes {
		if _, ok := permSet[code]; ok {
			return true, nil
		}
	}
	return false, nil
}

// GetUserRoles 获取用户的角色列表
func (s *PermissionService) GetUserRoles(userID string) ([]database.Role, error) {
	return s.userRoleRepo.FindByUserID(s.normalizeUserID(userID))
}

// AssignUserRole 给用户分配角色
func (s *PermissionService) AssignUserRole(userID string, roleID uint) error {
	return s.userRoleRepo.Assign(s.normalizeUserID(userID), roleID)
}

// RemoveUserRole 移除用户角色
func (s *PermissionService) RemoveUserRole(userID string, roleID uint) error {
	return s.userRoleRepo.Remove(s.normalizeUserID(userID), roleID)
}

// GetRoleUsers 获取角色下的所有用户
func (s *PermissionService) GetRoleUsers(roleID uint) ([]database.User, error) {
	return s.userRoleRepo.FindByRoleID(roleID)
}

// HasUserRole 检查用户是否有某角色
func (s *PermissionService) HasUserRole(userID string, roleName string) (bool, error) {
	return s.userRoleRepo.HasRole(s.normalizeUserID(userID), roleName)
}

// GetUserPerformanceScope 根据 data_permissions 配置返回绩效数据可见范围
// 返回 nil 表示全量权限，返回非 nil 的 OrgDataScope 表示受限范围
func (s *PermissionService) GetUserPerformanceScope(userID string) (*OrgDataScope, error) {
	return s.ResolveUserScope(userID)
}

// GetMenuPermission 获取角色的菜单权限
func (s *PermissionService) GetMenuPermission(roleID uint) (string, error) {
	keys, err := s.GetRoleMenuKeys(roleID)
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(keys)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// SaveMenuPermission 保存角色的菜单权限
func (s *PermissionService) SaveMenuPermission(roleID uint, menuKeys string) error {
	keys, err := ParseMenuKeys(menuKeys)
	if err != nil {
		return err
	}
	return s.SaveMenuPermissionKeys(roleID, keys)
}

// SaveMenuPermissionKeys 保存角色的菜单权限。
func (s *PermissionService) SaveMenuPermissionKeys(roleID uint, menuKeys []string) error {
	payload, err := json.Marshal(NormalizeMenuPermissionKeys(menuKeys))
	if err != nil {
		return err
	}
	return s.menuPermRepo.Save(roleID, string(payload))
}

// GetDataPermission 获取角色的数据权限
func (s *PermissionService) GetDataPermission(roleID uint) (scope string, departmentKeys string, err error) {
	dp, err := s.dataPermRepo.FindByRoleID(roleID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "all", "[]", nil
		}
		return "", "", err
	}
	return dp.Scope, dp.DepartmentKeys, nil
}

// SaveDataPermission 保存角色的数据权限
func (s *PermissionService) SaveDataPermission(roleID uint, scope string, departmentKeys string) error {
	return s.dataPermRepo.Save(roleID, scope, departmentKeys)
}

// GetUserMenuKeys 根据用户角色从 menu_permissions 表聚合菜单权限。
func (s *PermissionService) GetUserMenuKeys(userID string) ([]string, error) {
	records, err := s.menuPermRepo.FindByUserRole(s.normalizeUserID(userID))
	if err != nil {
		return nil, err
	}

	keySet := make(map[string]struct{})
	for _, record := range records {
		keys, err := ParseMenuKeys(record.MenuKeys)
		if err != nil {
			return nil, err
		}
		for _, key := range NormalizeMenuPermissionKeys(keys) {
			keySet[key] = struct{}{}
		}
	}
	return sortedKeys(keySet), nil
}

// GetRoleMenuKeys 从 menu_permissions 表读取角色菜单权限。
func (s *PermissionService) GetRoleMenuKeys(roleID uint) ([]string, error) {
	mp, err := s.menuPermRepo.FindByRoleID(roleID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return []string{}, nil
		}
		return nil, err
	}
	keys, err := ParseMenuKeys(mp.MenuKeys)
	if err != nil {
		return nil, err
	}
	return NormalizeMenuPermissionKeys(keys), nil
}

// HasMenuPermission 检查用户是否具有指定菜单权限。
func (s *PermissionService) HasMenuPermission(userID string, menuKey string) (bool, error) {
	keys, err := s.GetUserMenuKeys(userID)
	if err != nil {
		return false, err
	}
	needle := NormalizeMenuPermissionKey(menuKey)
	for _, key := range keys {
		if key == needle {
			return true, nil
		}
	}
	return false, nil
}

// ResolveUserScope 根据 data_permissions 表统一解析用户的数据可见范围。
// 优先级：all > department > self。多个角色取最宽松的合并结果。
// 返回 nil 表示全量权限（admin 或 all scope）。
func (s *PermissionService) ResolveUserScope(userID string) (*OrgDataScope, error) {
	// admin 用户全量权限
	if userID == "admin" {
		return nil, nil
	}

	// JWT token 存的是数字主键 ID，需要转换为 user_id 字段
	// user_roles.user_id 和 users.user_id 存的是字符串标识
	stringUserID := userID
	// 先按 user_id 字段查询（钉钉 userId 等字符串标识，即使外观像数字）
	if user, err := s.userRepo.FindByUserID(userID); err == nil && user.UserID != "" {
		stringUserID = user.UserID
	} else if looksNumericID(userID) {
		// 再按主键 id 查询
		if user, err := s.userRepo.FindByID(userID); err == nil && user.UserID != "" {
			stringUserID = user.UserID
		}
	}
	logrus.WithFields(logrus.Fields{"numericID": userID, "stringUserID": stringUserID}).Debug("ResolveUserScope: ID转换")

	// 获取用户所有角色
	roles, err := s.userRoleRepo.FindByUserID(stringUserID)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		logrus.WithField("stringUserID", stringUserID).Debug("ResolveUserScope: 无角色，返回self")
		return &OrgDataScope{Mode: "self", DepartmentIDs: []string{}, UserIDs: []string{stringUserID}}, nil
	}

	// 遍历角色，聚合数据权限
	hasAll := false
	mergedDeptIDs := make(map[string]struct{})
	hasAnyConfig := false

	for _, role := range roles {
		dp, err := s.dataPermRepo.FindByRoleID(role.ID)
		if err != nil {
			logrus.WithField("roleID", role.ID).Debug("ResolveUserScope: 角色无数据权限配置")
			continue // 该角色未配置数据权限，跳过
		}
		hasAnyConfig = true
		logrus.WithFields(logrus.Fields{"roleID": role.ID, "roleName": role.Name, "scope": dp.Scope}).Debug("ResolveUserScope: 角色数据权限")

		switch dp.Scope {
		case "all":
			hasAll = true
		case "self":
			// 不改变合并结果，仅标记已配置
		case "department":
			var keys []string
			if err := json.Unmarshal([]byte(dp.DepartmentKeys), &keys); err == nil {
				for _, k := range keys {
					mergedDeptIDs[k] = struct{}{}
				}
			}
		}
	}

	// 没有任何角色配置了数据权限 → 仅看自己（最小权限）
	if !hasAnyConfig {
		logrus.WithField("stringUserID", stringUserID).Debug("ResolveUserScope: 无任何配置，返回self")
		return &OrgDataScope{Mode: "self", DepartmentIDs: []string{}, UserIDs: []string{stringUserID}}, nil
	}

	// all 最高优先级
	if hasAll {
		logrus.Debug("ResolveUserScope: 有all权限，返回nil")
		return nil, nil
	}

	// 有 department 配置
	if len(mergedDeptIDs) > 0 {
		deptIDs := make([]string, 0, len(mergedDeptIDs))
		for id := range mergedDeptIDs {
			deptIDs = append(deptIDs, id)
		}
		logrus.WithField("deptIDs", deptIDs).Debug("ResolveUserScope: 返回department")
		return &OrgDataScope{
			Mode:          "department",
			DepartmentIDs: deptIDs,
		}, nil
	}

	// 全部角色都是 self
	logrus.WithField("stringUserID", stringUserID).Debug("ResolveUserScope: 全部角色self，返回self")
	return &OrgDataScope{Mode: "self", DepartmentIDs: []string{}, UserIDs: []string{stringUserID}}, nil
}

const menuPermissionPrefix = "menu:"

// NormalizeMenuPermissionKey 将前端菜单 key 规范化为 menu:* 权限码。
func NormalizeMenuPermissionKey(key string) string {
	normalized := strings.TrimSpace(key)
	if normalized == "" {
		return normalized
	}
	if strings.HasPrefix(normalized, menuPermissionPrefix) {
		return normalized
	}
	return menuPermissionPrefix + normalized
}

// NormalizeMenuPermissionKeys 去重、规范化并排序菜单权限码。
func NormalizeMenuPermissionKeys(keys []string) []string {
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		normalized := NormalizeMenuPermissionKey(key)
		if normalized == "" {
			continue
		}
		keySet[normalized] = struct{}{}
	}
	return sortedKeys(keySet)
}

// ParseMenuKeys 解析 menu_permissions.menu_keys 中的 JSON 数组。
func ParseMenuKeys(menuKeys string) ([]string, error) {
	raw := strings.TrimSpace(menuKeys)
	if raw == "" {
		return []string{}, nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil, err
	}
	return keys, nil
}

func sortedKeys(keySet map[string]struct{}) []string {
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
