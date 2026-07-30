package service

import (
	"encoding/json"
	"errors"
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// ErrUserNotInOrg indicates the target user does not belong to the current organization.
var ErrUserNotInOrg = errors.New("permission: user not found in organization")

// ErrRoleNotInOrg indicates the target role does not belong to the current organization.
var ErrRoleNotInOrg = errors.New("permission: role not found in organization")

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
	// Strict: only use explicit request/tenant org; never invent "default".
	orgID, _ := database.RequireOrganizationIDFromDB(db)
	return NewPermissionServiceWithOrgID(db, orgID)
}

// NewPermissionServiceWithOrgID 多租户构造：deptRepo/userRepo 携带 orgID 过滤。
// 用于 handler 层将当前请求的组织隔离下推到权限相关查询，
// 避免 resolveManagedDepartmentScope 拉全库部门造成跨企业串权限。
// orgID 为空时保持空绑定（fail-closed on tenant methods），禁止静默 default。
func NewPermissionServiceWithOrgID(db *gorm.DB, orgID string) *PermissionService {
	orgID = strings.TrimSpace(orgID)
	if orgID != "" {
		orgID = database.NormalizeOrganizationID(orgID)
	}
	svc := &PermissionService{
		db:                 db,
		orgID:              orgID,
		roleRepo:           repository.NewRoleRepository(db),
		permissionRepo:     repository.NewPermissionRepository(db),
		userRoleRepo:       repository.NewUserRoleRepository(db),
		rolePermissionRepo: repository.NewRolePermissionRepository(db),
	}
	if orgID != "" {
		svc.menuPermRepo = repository.NewMenuPermissionRepositoryWithOrgID(db, orgID)
		svc.dataPermRepo = repository.NewDataPermissionRepositoryWithOrgID(db, orgID)
		svc.deptRepo = repository.NewDepartmentRepositoryWithOrgID(db, orgID)
		svc.userRepo = repository.NewUserRepositoryWithOrgID(db, orgID)
	} else {
		// Unbound repos fail closed on tenant reads; permission catalogs remain global.
		svc.menuPermRepo = repository.NewMenuPermissionRepository(db)
		svc.dataPermRepo = repository.NewDataPermissionRepository(db)
		svc.deptRepo = repository.NewDepartmentRepository(db)
		svc.userRepo = repository.NewUserRepository(db)
	}
	return svc
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
	{Name: "作息表群聊推送", Code: "week_schedule_group_push", Description: "绑定群聊并推送月作息表"},
	{Name: "考勤工具箱操作", Code: "attendance_toolbox_operate", Description: "考勤工具箱计算、下载与普通操作"},
	{Name: "考勤工具箱钉钉同步", Code: "attendance_toolbox_dingtalk_sync", Description: "考勤工具箱钉钉同步"},
	{Name: "考勤工具箱规则编辑", Code: "attendance_toolbox_rules_edit", Description: "考勤工具箱加班规则编辑与应用"},
	{Name: "审批管理", Code: "approval_manage", Description: "审批管理权限"},
	{Name: "同步审批", Code: "approval:sync", Description: "同步审批模板/实例数据"},
	{Name: "创建审批模板", Code: "approval:create", Description: "创建审批模板"},
	{Name: "编辑审批模板", Code: "approval:update", Description: "编辑审批模板"},
	{Name: "删除审批模板", Code: "approval:delete", Description: "删除审批模板"},
	{Name: "权限管理", Code: "permission_manage", Description: "权限管理权限"},
	{Name: "绩效活动管理", Code: "performance:activity:manage", Description: "创建/编辑/发布/启动/锁定/归档绩效活动"},
	{Name: "绩效活动导入", Code: "performance:activity:import", Description: "通过 Excel 分析并创建绩效模板、草稿活动和目标"},
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
	orgID := s.effectiveOrgID("")
	if orgID == "" {
		// Fail-closed: never list roles across all tenants via FindAll().
		return nil, 0, repository.ErrMissingOrgID
	}
	return s.roleRepo.FindAllByOrg(orgID)
}

func (s *PermissionService) CreateRole(role *database.Role) error {
	if role == nil {
		return gorm.ErrInvalidData
	}
	orgID := s.effectiveOrgID(role.OrgID)
	if orgID == "" {
		return repository.ErrMissingOrgID
	}
	if trimmed := strings.TrimSpace(role.OrgID); trimmed != "" && database.NormalizeOrganizationID(trimmed) != orgID {
		return ErrRoleNotInOrg
	}
	role.OrgID = orgID
	return s.roleRepo.Create(role)
}

func (s *PermissionService) UpdateRole(role *database.Role) error {
	if role == nil {
		return gorm.ErrInvalidData
	}
	orgID := s.effectiveOrgID("")
	if orgID == "" {
		return repository.ErrMissingOrgID
	}
	if _, err := s.requireRoleInOrg(orgID, role.ID); err != nil {
		return err
	}
	// Prevent callers from re-parenting a role into another organization.
	// UpdateInOrg uses the service-bound org as the sole filter authority.
	return s.roleRepo.UpdateInOrg(orgID, role)
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
	orgID := s.effectiveOrgID("")
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	if _, err := s.requireRoleInOrg(orgID, roleID); err != nil {
		return nil, err
	}
	return s.rolePermissionRepo.FindByRoleID(roleID)
}

func (s *PermissionService) SaveRolePermissions(roleID uint, permissionIDs []uint) error {
	if err := s.EnsureSystemPermissions(); err != nil {
		return err
	}
	orgID := s.effectiveOrgID("")
	if orgID == "" {
		return repository.ErrMissingOrgID
	}
	if _, err := s.requireRoleInOrg(orgID, roleID); err != nil {
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

// effectiveOrgID resolves the tenant for permission lookups.
// Explicit orgID wins; otherwise use the service-bound org (from request DB context).
// Empty context returns "" (fail-closed) — never invents "default".
func (s *PermissionService) effectiveOrgID(orgID string) string {
	if o := strings.TrimSpace(orgID); o != "" {
		return database.NormalizeOrganizationID(o)
	}
	if s != nil {
		if o := strings.TrimSpace(s.orgID); o != "" {
			return database.NormalizeOrganizationID(o)
		}
	}
	return ""
}

// normalizePermissionOrgID keeps package-level helpers used by tests/callers that
// only have a raw org string. Prefer (s *PermissionService).effectiveOrgID in service methods.
// Empty input stays empty (fail-closed); does not invent default.
func normalizePermissionOrgID(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return ""
	}
	return database.NormalizeOrganizationID(orgID)
}

func (s *PermissionService) normalizeUserID(userID string) string {
	return s.normalizeUserIDInOrg(s.effectiveOrgID(""), userID)
}

func (s *PermissionService) normalizeUserIDInOrg(orgID, userID string) string {
	normalized := strings.TrimSpace(userID)
	if normalized == "" {
		return normalized
	}
	orgID = s.effectiveOrgID(orgID)
	if orgID != "" {
		if user, err := s.userRepo.FindByOrgAndUserID(orgID, normalized); err == nil && user.UserID != "" {
			return user.UserID
		}
	}
	// 先按 user_id 字段查询（钉钉 userId 等字符串标识，即使外观像数字）
	// userRepo may already be org-scoped via NewPermissionServiceWithOrgID / CurrentOrganizationIDFromDB.
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
	return s.GetUserPermissionsInOrg(s.effectiveOrgID(""), userID)
}

func (s *PermissionService) GetUserPermissionsInOrg(orgID, userID string) ([]string, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	perms, err := s.rolePermissionRepo.FindByUserRole(orgID, s.normalizeUserIDInOrg(orgID, userID))
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
	return s.HasPermissionInOrg(s.effectiveOrgID(""), userID, permissionCode)
}

func (s *PermissionService) HasPermissionInOrg(orgID, userID string, permissionCode string) (bool, error) {
	perms, err := s.GetUserPermissionsInOrg(orgID, userID)
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
	return s.HasAnyPermissionInOrg(s.effectiveOrgID(""), userID, codes...)
}

func (s *PermissionService) HasAnyPermissionInOrg(orgID, userID string, codes ...string) (bool, error) {
	perms, err := s.GetUserPermissionsInOrg(orgID, userID)
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
	return s.GetUserRolesInOrg(s.effectiveOrgID(""), userID)
}

func (s *PermissionService) GetUserRolesInOrg(orgID, userID string) ([]database.Role, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	return s.userRoleRepo.FindByUserID(orgID, s.normalizeUserIDInOrg(orgID, userID))
}

// AssignUserRole 设置用户角色，会替换该用户原有角色。
func (s *PermissionService) AssignUserRole(userID string, roleID uint) error {
	return s.AssignUserRoleInOrg(s.effectiveOrgID(""), userID, roleID)
}

func (s *PermissionService) AssignUserRoleInOrg(orgID, userID string, roleID uint) error {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return repository.ErrMissingOrgID
	}
	normalized, err := s.requireUserInOrg(orgID, userID)
	if err != nil {
		return err
	}
	if _, err := s.requireRoleInOrg(orgID, roleID); err != nil {
		return err
	}
	return s.userRoleRepo.Assign(orgID, normalized, roleID)
}

// AssignDefaultEmployeeRoleIfUnassigned assigns the default employee role only when the user has no role.
func (s *PermissionService) AssignDefaultEmployeeRoleIfUnassigned(userID string) (bool, error) {
	return s.AssignDefaultEmployeeRoleIfUnassignedInOrg(s.effectiveOrgID(""), userID)
}

func (s *PermissionService) AssignDefaultEmployeeRoleIfUnassignedInOrg(orgID, userID string) (bool, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return false, repository.ErrMissingOrgID
	}
	normalized, err := s.requireUserInOrg(orgID, userID)
	if err != nil {
		if errors.Is(err, ErrUserNotInOrg) || errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	roles, err := s.userRoleRepo.FindByUserID(orgID, normalized)
	if err != nil {
		return false, err
	}
	if len(roles) > 0 {
		return false, nil
	}

	role, err := s.ensureDefaultEmployeeRoleInOrg(orgID)
	if err != nil {
		return false, err
	}
	if err := s.userRoleRepo.Assign(orgID, normalized, role.ID); err != nil {
		return false, err
	}
	return true, nil
}

func (s *PermissionService) ensureDefaultEmployeeRole() (*database.Role, error) {
	return s.ensureDefaultEmployeeRoleInOrg(s.effectiveOrgID(""))
}

// ensureDefaultEmployeeRoleInOrg finds or creates the default employee role for a single org.
// It never reuses another organization's role of the same name.
func (s *PermissionService) ensureDefaultEmployeeRoleInOrg(orgID string) (*database.Role, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	var role database.Role
	err := s.db.Unscoped().Where("org_id = ? AND name = ?", orgID, DefaultEmployeeRoleName).First(&role).Error
	if err == nil {
		if role.DeletedAt.Valid {
			if err := s.db.Unscoped().Model(&role).Update("deleted_at", nil).Error; err != nil {
				return nil, err
			}
			role.DeletedAt.Valid = false
		}
		// Hard guarantee: never return a role that drifted to another org.
		if strings.TrimSpace(role.OrgID) != orgID {
			return nil, ErrRoleNotInOrg
		}
		return &role, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	role = database.Role{
		OrgID:       orgID,
		Name:        DefaultEmployeeRoleName,
		Description: DefaultEmployeeRoleName,
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
	return s.RemoveUserRoleInOrg(s.effectiveOrgID(""), userID, roleID)
}

func (s *PermissionService) RemoveUserRoleInOrg(orgID, userID string, roleID uint) error {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return repository.ErrMissingOrgID
	}
	normalized, err := s.requireUserInOrg(orgID, userID)
	if err != nil {
		return err
	}
	// Role must exist in current org; removing with a cross-org role_id is a not-found.
	if _, err := s.requireRoleInOrg(orgID, roleID); err != nil {
		return err
	}
	return s.userRoleRepo.Remove(orgID, normalized, roleID)
}

// GetRoleUsers 获取角色下的所有用户
func (s *PermissionService) GetRoleUsers(roleID uint) ([]database.User, error) {
	return s.GetRoleUsersInOrg(s.effectiveOrgID(""), roleID)
}

func (s *PermissionService) GetRoleUsersInOrg(orgID string, roleID uint) ([]database.User, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	if _, err := s.requireRoleInOrg(orgID, roleID); err != nil {
		return nil, err
	}
	return s.userRoleRepo.FindByRoleID(orgID, roleID)
}

// HasUserRole 检查用户是否有某角色
func (s *PermissionService) HasUserRole(userID string, roleName string) (bool, error) {
	return s.HasUserRoleInOrg(s.effectiveOrgID(""), userID, roleName)
}

func (s *PermissionService) HasUserRoleInOrg(orgID, userID string, roleName string) (bool, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return false, repository.ErrMissingOrgID
	}
	return s.userRoleRepo.HasRole(orgID, s.normalizeUserIDInOrg(orgID, userID), roleName)
}

// GetUserPerformanceScope 根据 data_permissions 配置返回绩效数据可见范围
// 返回 nil 表示全量权限，返回非 nil 的 OrgDataScope 表示受限范围
func (s *PermissionService) GetUserPerformanceScope(userID string) (*OrgDataScope, error) {
	return s.GetUserPerformanceScopeInOrg(s.effectiveOrgID(""), userID)
}

func (s *PermissionService) GetUserPerformanceScopeInOrg(orgID, userID string) (*OrgDataScope, error) {
	return s.ResolveUserScopeInOrg(orgID, userID)
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
	return s.GetUserMenuKeysInOrg(s.effectiveOrgID(""), userID)
}

func (s *PermissionService) GetUserMenuKeysInOrg(orgID, userID string) ([]string, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	records, err := s.menuPermRepo.FindByUserRole(orgID, s.normalizeUserIDInOrg(orgID, userID))
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
	return s.HasMenuPermissionInOrg(s.effectiveOrgID(""), userID, menuKey)
}

func (s *PermissionService) HasMenuPermissionInOrg(orgID, userID string, menuKey string) (bool, error) {
	keys, err := s.GetUserMenuKeysInOrg(orgID, userID)
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
	return s.ResolveUserScopeInOrg(s.effectiveOrgID(""), userID)
}

func (s *PermissionService) ResolveUserScopeInOrg(orgID, userID string) (*OrgDataScope, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	// admin 用户全量权限
	if userID == "admin" {
		return nil, nil
	}

	stringUserID := s.normalizeUserIDInOrg(orgID, userID)
	logrus.WithFields(logrus.Fields{"orgID": orgID, "numericID": userID, "stringUserID": stringUserID}).Debug("ResolveUserScope: ID转换")

	// 获取用户所有角色
	roles, err := s.userRoleRepo.FindByUserID(orgID, stringUserID)
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
	return s.requireRoleInOrg(s.effectiveOrgID(""), roleID)
}

// requireRoleInOrg loads a role that must belong to orgID.
// Cross-org role IDs return ErrRoleNotInOrg (wrapped around gorm.ErrRecordNotFound).
func (s *PermissionService) requireRoleInOrg(orgID string, roleID uint) (*database.Role, error) {
	if roleID == 0 {
		return nil, ErrRoleNotInOrg
	}
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return nil, repository.ErrMissingOrgID
	}
	role, err := s.roleRepo.FindByIDAndOrg(roleID, orgID)
	if err != nil {
		if errors.Is(err, repository.ErrMissingOrgID) {
			return nil, err
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrRoleNotInOrg
		}
		return nil, err
	}
	return role, nil
}

func (s *PermissionService) requireUser(userID string) (string, error) {
	return s.requireUserInOrg(s.effectiveOrgID(""), userID)
}

// requireUserInOrg resolves and verifies the user belongs to orgID.
// Missing / cross-org users return ErrUserNotInOrg.
func (s *PermissionService) requireUserInOrg(orgID, userID string) (string, error) {
	orgID = s.effectiveOrgID(orgID)
	if orgID == "" {
		return "", repository.ErrMissingOrgID
	}
	normalized := strings.TrimSpace(userID)
	if normalized == "" {
		return "", ErrUserNotInOrg
	}

	// Prefer exact (org_id, user_id) lookup — never fall back across organizations.
	if user, err := s.userRepo.FindByOrgAndUserID(orgID, normalized); err == nil && user.UserID != "" {
		return user.UserID, nil
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", err
	}

	// Numeric primary-key path still constrained to org via scoped userRepo when org-bound.
	if looksNumericID(normalized) {
		if user, err := s.userRepo.FindByID(normalized); err == nil && user.UserID != "" {
			if strings.TrimSpace(user.OrgID) != "" && strings.TrimSpace(user.OrgID) != orgID {
				return "", ErrUserNotInOrg
			}
			// When userRepo is org-scoped FindByID already filters; double-check when present.
			if s.orgID != "" && strings.TrimSpace(user.OrgID) != "" && user.OrgID != s.orgID {
				return "", ErrUserNotInOrg
			}
			// Confirm membership by org+user_id for unscoped repos.
			if _, err := s.userRepo.FindByOrgAndUserID(orgID, user.UserID); err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return "", ErrUserNotInOrg
				}
				return "", err
			}
			return user.UserID, nil
		} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
	}
	return "", ErrUserNotInOrg
}
