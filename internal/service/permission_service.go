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
	db                 *gorm.DB
	orgID              string
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
		db:                 db,
		orgID:              database.CurrentOrganizationIDFromDB(db),
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

type SystemPermissionDefinition struct {
	Name        string
	Code        string
	Description string
}

var systemPermissionDefinitions = []SystemPermissionDefinition{
	{Name: "用户管理", Code: "user_manage", Description: "用户管理权限"},
	{Name: "部门管理", Code: "department_manage", Description: "部门管理权限"},
	{Name: "考勤管理", Code: "attendance_manage", Description: "考勤管理权限"},
	{Name: "审批管理", Code: "approval_manage", Description: "审批管理权限"},
	{Name: "同步审批", Code: "approval:sync", Description: "同步审批模板/实例数据"},
	{Name: "创建审批模板", Code: "approval:create", Description: "创建审批模板"},
	{Name: "编辑审批模板", Code: "approval:update", Description: "编辑审批模板"},
	{Name: "删除审批模板", Code: "approval:delete", Description: "删除审批模板"},
	{Name: "权限管理", Code: "permission_manage", Description: "权限管理权限"},
	{Name: "绩效活动管理", Code: "performance:activity:manage", Description: "创建/编辑/发布/启动/锁定/归档绩效活动"},
	{Name: "绩效自评提交", Code: "performance:self_eval:submit", Description: "提交绩效自评"},
	{Name: "绩效主管评分", Code: "performance:manager_eval:submit", Description: "主管绩效评分"},
	{Name: "绩效员工确认", Code: "performance:employee_confirm:submit", Description: "员工确认绩效结果"},
	{Name: "绩效主管确认", Code: "performance:manager_confirm:submit", Description: "主管确认绩效结果"},
	{Name: "绩效HR确认", Code: "performance:hr_confirm:submit", Description: "旧流程HR确认绩效结果"},
	{Name: "绩效部门/中心评估", Code: "performance:department_eval:submit", Description: "部门/中心负责人确认或调整绩效结果"},
	{Name: "绩效HR审核", Code: "performance:hr_review:submit", Description: "HR审核沐腾科技流程绩效结果"},
	{Name: "绩效结果公布", Code: "performance:result_publish:manage", Description: "公布沐腾科技流程绩效结果"},
	{Name: "绩效结果屏蔽管理", Code: "performance:result_visibility:manage", Description: "设置或解除绩效结果屏蔽"},
	{Name: "绩效屏蔽结果查看", Code: "performance:hidden_result:view", Description: "查看已屏蔽的绩效结果"},
	{Name: "绩效面谈管理", Code: "performance:interview:manage", Description: "安排、记录和完成绩效面谈"},
	{Name: "绩效申诉处理", Code: "performance:appeal:manage", Description: "处理沐腾科技流程绩效申诉"},
	{Name: "绩效等级调整", Code: "performance:level_adjust:manage", Description: "调整绩效最终等级"},
	{Name: "绩效分布规则", Code: "performance:distribution:manage", Description: "设置绩效分布规则"},
	{Name: "绩效指标库管理", Code: "performance:indicator:manage", Description: "指标库/指标项 CRUD"},
	{Name: "绩效目标管理", Code: "performance:goal:manage", Description: "目标设定/审批/分配"},
	{Name: "绩效考核上级调整", Code: "performance:assessment_manager:update", Description: "调整单个绩效参与人的考核上级"},
	{Name: "绩效考核上级批量调整", Code: "performance:assessment_manager:batch_update", Description: "批量调整绩效参与人的考核上级"},
	{Name: "绩效结果查看", Code: "performance:result:view", Description: "查看绩效结果"},
	{Name: "组织数据只读", Code: "org:read", Description: "查看组织架构、花名册等组织数据"},
	{Name: "审计日志只读", Code: "audit_log:read", Description: "查看操作审计日志"},
}

const DefaultEmployeeRoleName = "普通员工"

var defaultEmployeeRolePermissionCodes = []string{
	"performance:self_eval:submit",
	"performance:employee_confirm:submit",
	"performance:result:view",
}

var defaultEmployeeRoleMenuKeys = []string{
	"menu:home",
	"menu:performance-overview",
}

func (s *PermissionService) EnsureSystemPermissions() error {
	for _, def := range systemPermissionDefinitions {
		var perm database.Permission
		err := s.db.Unscoped().Where("code = ?", def.Code).First(&perm).Error
		if err == gorm.ErrRecordNotFound {
			perm = database.Permission{Name: def.Name, Code: def.Code, Description: def.Description}
			if err := s.db.Create(&perm).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		updates := map[string]interface{}{
			"name":        def.Name,
			"description": def.Description,
			"deleted_at":  nil,
		}
		if err := s.db.Unscoped().Model(&perm).Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *PermissionService) GetRoles() ([]database.Role, int64, error) {
	return s.roleRepo.FindAll()
}

func (s *PermissionService) CreateRole(role *database.Role) error {
	if role != nil && strings.TrimSpace(role.OrgID) == "" {
		role.OrgID = s.orgID
	}
	return s.roleRepo.Create(role)
}

func (s *PermissionService) UpdateRole(role *database.Role) error {
	if _, err := s.requireRole(role.ID); err != nil {
		return err
	}
	return s.roleRepo.Update(role)
}

func (s *PermissionService) GetPermissions() ([]database.Permission, int64, error) {
	if err := s.EnsureSystemPermissions(); err != nil {
		return nil, 0, err
	}
	return s.permissionRepo.FindAll()
}

func (s *PermissionService) GetRolePermissions(roleID uint) ([]database.Permission, error) {
	if err := s.EnsureSystemPermissions(); err != nil {
		return nil, err
	}
	if _, err := s.requireRole(roleID); err != nil {
		return nil, err
	}
	return s.rolePermissionRepo.FindByRoleID(roleID)
}

func (s *PermissionService) SaveRolePermissions(roleID uint, permissionIDs []uint) error {
	if err := s.EnsureSystemPermissions(); err != nil {
		return err
	}
	if _, err := s.requireRole(roleID); err != nil {
		return err
	}
	seen := make(map[uint]struct{}, len(permissionIDs))
	ids := make([]uint, 0, len(permissionIDs))
	for _, id := range permissionIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		query := tx.Where("role_id = ?", roleID)
		if len(ids) > 0 {
			query = query.Where("permission_id NOT IN ?", ids)
		}
		if err := query.Delete(&database.RolePermission{}).Error; err != nil {
			return err
		}

		for _, permissionID := range ids {
			var count int64
			if err := tx.Model(&database.Permission{}).Where("id = ? AND deleted_at IS NULL", permissionID).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				continue
			}

			var existing database.RolePermission
			err := tx.Unscoped().
				Where("role_id = ? AND permission_id = ?", roleID, permissionID).
				Order("deleted_at IS NULL DESC, id ASC").
				First(&existing).Error
			if err == gorm.ErrRecordNotFound {
				if err := tx.Create(&database.RolePermission{RoleID: roleID, PermissionID: permissionID}).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			if err := tx.Unscoped().Model(&existing).Update("deleted_at", nil).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

// GetUserRoles 获取用户当前角色。数据库约束保证同一用户最多只有一个角色。
func (s *PermissionService) GetUserRoles(userID string) ([]database.Role, error) {
	return s.userRoleRepo.FindByUserID(s.normalizeUserID(userID))
}

// AssignUserRole 设置用户角色，会替换该用户原有角色。
func (s *PermissionService) AssignUserRole(userID string, roleID uint) error {
	normalizedUserID, err := s.requireUser(userID)
	if err != nil {
		return err
	}
	if _, err := s.requireRole(roleID); err != nil {
		return err
	}
	return s.userRoleRepo.Assign(normalizedUserID, roleID)
}

// AssignDefaultEmployeeRoleIfUnassigned assigns the default employee role only when the user has no role.
func (s *PermissionService) AssignDefaultEmployeeRoleIfUnassigned(userID string) (bool, error) {
	normalized := s.normalizeUserID(userID)
	if normalized == "" {
		return false, nil
	}

	roles, err := s.userRoleRepo.FindByUserID(normalized)
	if err != nil {
		return false, err
	}
	if len(roles) > 0 {
		return false, nil
	}

	role, err := s.ensureDefaultEmployeeRole()
	if err != nil {
		return false, err
	}
	if err := s.userRoleRepo.Assign(normalized, role.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PermissionService) ensureDefaultEmployeeRole() (*database.Role, error) {
	var role database.Role
	err := s.db.Unscoped().Where("name = ?", DefaultEmployeeRoleName).First(&role).Error
	if err == nil {
		if role.DeletedAt.Valid {
			if err := s.db.Unscoped().Model(&role).Update("deleted_at", nil).Error; err != nil {
				return nil, err
			}
			role.DeletedAt.Valid = false
		}
		return &role, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	role = database.Role{Name: DefaultEmployeeRoleName, Description: DefaultEmployeeRoleName}
	if strings.TrimSpace(role.OrgID) == "" {
		role.OrgID = s.orgID
	}
	if err := s.db.Create(&role).Error; err != nil {
		return nil, err
	}
	if err := s.grantDefaultEmployeeRolePermissions(role.ID); err != nil {
		return nil, err
	}
	if err := s.grantDefaultEmployeeRoleMenuPermissions(role.ID); err != nil {
		return nil, err
	}
	return &role, nil
}

func (s *PermissionService) grantDefaultEmployeeRolePermissions(roleID uint) error {
	if err := s.EnsureSystemPermissions(); err != nil {
		return err
	}

	var permissions []database.Permission
	if err := s.db.Where("code IN ? AND deleted_at IS NULL", defaultEmployeeRolePermissionCodes).Find(&permissions).Error; err != nil {
		return err
	}
	for _, permission := range permissions {
		var existing database.RolePermission
		err := s.db.Unscoped().
			Where("role_id = ? AND permission_id = ?", roleID, permission.ID).
			First(&existing).Error
		if err == gorm.ErrRecordNotFound {
			if err := s.db.Create(&database.RolePermission{RoleID: roleID, PermissionID: permission.ID}).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if existing.DeletedAt.Valid {
			if err := s.db.Unscoped().Model(&existing).Update("deleted_at", nil).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *PermissionService) grantDefaultEmployeeRoleMenuPermissions(roleID uint) error {
	payload, err := json.Marshal(NormalizeMenuPermissionKeys(defaultEmployeeRoleMenuKeys))
	if err != nil {
		return err
	}
	return s.menuPermRepo.Save(roleID, string(payload))
}

// RemoveUserRole removes a role from a user.
func (s *PermissionService) RemoveUserRole(userID string, roleID uint) error {
	normalizedUserID, err := s.requireUser(userID)
	if err != nil {
		return err
	}
	if _, err := s.requireRole(roleID); err != nil {
		return err
	}
	return s.userRoleRepo.Remove(normalizedUserID, roleID)
}

// GetRoleUsers 获取角色下的所有用户
func (s *PermissionService) GetRoleUsers(roleID uint) ([]database.User, error) {
	if _, err := s.requireRole(roleID); err != nil {
		return nil, err
	}
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
	if _, err := s.requireRole(roleID); err != nil {
		return "", err
	}
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
	if _, err := s.requireRole(roleID); err != nil {
		return err
	}
	payload, err := json.Marshal(NormalizeMenuPermissionKeys(menuKeys))
	if err != nil {
		return err
	}
	return s.menuPermRepo.Save(roleID, string(payload))
}

// GetDataPermission 获取角色的数据权限
func (s *PermissionService) GetDataPermission(roleID uint) (scope string, departmentKeys string, err error) {
	if _, err := s.requireRole(roleID); err != nil {
		return "", "", err
	}
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
	if _, err := s.requireRole(roleID); err != nil {
		return err
	}
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
	if _, err := s.requireRole(roleID); err != nil {
		return nil, err
	}
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
// 数据库约束保证同一用户最多只有一个角色，保留 all > department > self 作为兼容兜底。
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
	mergedDeptRoots := make(map[string]struct{})
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
					if strings.TrimSpace(k) != "" {
						mergedDeptRoots[strings.TrimSpace(k)] = struct{}{}
					}
				}
			}
		}
	}

	// all 最高优先级
	if hasAll {
		logrus.Debug("ResolveUserScope: 有all权限，返回nil")
		return nil, nil
	}

	deptIDs, rootDeptIDs, err := s.resolveManagedDepartmentScope(stringUserID, mergedDeptRoots)
	if err != nil {
		return nil, err
	}
	if len(deptIDs) > 0 {
		logrus.WithField("deptIDs", deptIDs).Debug("ResolveUserScope: 返回department")
		return &OrgDataScope{
			Mode:              "department",
			DepartmentIDs:     deptIDs,
			RootDepartmentIDs: rootDeptIDs,
		}, nil
	}

	// 没有任何角色配置了数据权限 → 仅看自己（最小权限）
	if !hasAnyConfig {
		logrus.WithField("stringUserID", stringUserID).Debug("ResolveUserScope: 无任何配置，返回self")
		return &OrgDataScope{Mode: "self", DepartmentIDs: []string{}, UserIDs: []string{stringUserID}}, nil
	}

	// 全部角色都是 self
	logrus.WithField("stringUserID", stringUserID).Debug("ResolveUserScope: 全部角色self，返回self")
	return &OrgDataScope{Mode: "self", DepartmentIDs: []string{}, UserIDs: []string{stringUserID}}, nil
}

var managedDepartmentUserIDKeys = []string{
	"department_head_user_ids",
	"department_head_user_id",
	"dingtalk_department_head_user_ids",
	"head_user_ids",
	"head_user_id",
	"department_manager_user_ids",
	"department_manager_user_id",
	"manager_user_ids",
	"manager_user_id",
	"department_hrbp_user_ids",
	"department_hrbp_user_id",
	"hrbp_user_ids",
	"hrbp_user_id",
	"department_assistant_user_ids",
	"department_assistant_user_id",
	"assistant_user_ids",
	"assistant_user_id",
}

func (s *PermissionService) resolveManagedDepartmentScope(userID string, rootSet map[string]struct{}) ([]string, []string, error) {
	departments, err := s.deptRepo.FindAll()
	if err != nil {
		return nil, nil, err
	}

	departmentMap := make(map[string]database.Department, len(departments))
	childMap := make(map[string][]string, len(departments))
	for _, department := range departments {
		departmentID := strings.TrimSpace(department.DepartmentID)
		if departmentID == "" {
			continue
		}
		department.DepartmentID = departmentID
		departmentMap[departmentID] = department
		childMap[department.ParentID] = append(childMap[department.ParentID], departmentID)
		if departmentScopeContainsUser(department.Extension, userID) {
			rootSet[departmentID] = struct{}{}
		}
	}

	rootIDs := make([]string, 0, len(rootSet))
	for departmentID := range rootSet {
		if _, ok := departmentMap[departmentID]; ok {
			rootIDs = append(rootIDs, departmentID)
		}
	}
	rootIDs = uniqueStrings(rootIDs)
	sort.Strings(rootIDs)

	departmentIDs := make([]string, 0, len(rootIDs))
	for _, rootID := range rootIDs {
		departmentIDs = append(departmentIDs, collectDescendantIDs(rootID, childMap)...)
	}
	departmentIDs = uniqueStrings(departmentIDs)
	sort.Strings(departmentIDs)
	return departmentIDs, rootIDs, nil
}

func departmentScopeContainsUser(extension map[string]interface{}, userID string) bool {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return false
	}
	for _, candidate := range departmentScopeUserIDs(extension) {
		if candidate == userID {
			return true
		}
	}
	return false
}

func departmentScopeUserIDs(extension map[string]interface{}) []string {
	if len(extension) == 0 {
		return []string{}
	}
	ids := make([]string, 0)
	for _, key := range managedDepartmentUserIDKeys {
		value, ok := extension[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []string:
			ids = append(ids, typed...)
		case []interface{}:
			for _, item := range typed {
				if text, ok := item.(string); ok {
					ids = append(ids, text)
				}
			}
		case string:
			ids = append(ids, typed)
		}
	}
	return uniqueStrings(ids)
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

func (s *PermissionService) requireRole(roleID uint) (*database.Role, error) {
	if roleID == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return s.roleRepo.FindByID(roleID)
}

func (s *PermissionService) requireUser(userID string) (string, error) {
	normalized := s.normalizeUserID(userID)
	if normalized == "" {
		return "", gorm.ErrRecordNotFound
	}
	if _, err := s.userRepo.FindByUserID(normalized); err == nil {
		return normalized, nil
	} else if err != gorm.ErrRecordNotFound {
		return "", err
	}
	return "", gorm.ErrRecordNotFound
}
