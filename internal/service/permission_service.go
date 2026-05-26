package service

import (
	"encoding/json"
	"peopleops/internal/database"
	"peopleops/internal/repository"

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

// GetUserPermissions 返回用户通过角色获得的所有权限码
func (s *PermissionService) GetUserPermissions(userID string) ([]string, error) {
	perms, err := s.rolePermissionRepo.FindByUserRole(userID)
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
	return s.userRoleRepo.FindByUserID(userID)
}

// AssignUserRole 给用户分配角色
func (s *PermissionService) AssignUserRole(userID string, roleID uint) error {
	return s.userRoleRepo.Assign(userID, roleID)
}

// RemoveUserRole 移除用户角色
func (s *PermissionService) RemoveUserRole(userID string, roleID uint) error {
	return s.userRoleRepo.Remove(userID, roleID)
}

// GetRoleUsers 获取角色下的所有用户
func (s *PermissionService) GetRoleUsers(roleID uint) ([]database.User, error) {
	return s.userRoleRepo.FindByRoleID(roleID)
}

// HasUserRole 检查用户是否有某角色
func (s *PermissionService) HasUserRole(userID string, roleName string) (bool, error) {
	return s.userRoleRepo.HasRole(userID, roleName)
}

// GetUserPerformanceScope 根据用户角色返回绩效数据可见范围
// 返回 nil 表示全量权限，返回非 nil 的 OrgDataScope 表示受限范围
// 对于普通员工，返回 Mode="self" 的 scope，调用方需特殊处理
func (s *PermissionService) GetUserPerformanceScope(userID string) (*OrgDataScope, error) {
	// 1. admin 用户全量权限
	if userID == "admin" {
		return nil, nil
	}

	// 2. 管理员角色 → 全量权限
	isAdmin, err := s.HasUserRole(userID, "管理员")
	if err != nil {
		return nil, err
	}
	if isAdmin {
		return nil, nil
	}

	// 3. 部门负责人角色 → 递归查询部门范围
	isManager, err := s.HasUserRole(userID, "部门负责人")
	if err != nil {
		return nil, err
	}

	if isManager {
		// 获取用户所属部门
		user, err := s.userRepo.FindByUserID(userID)
		if err != nil {
			return nil, err
		}

		// 递归查询该部门及所有子部门 ID
		departmentIDs, err := s.deptRepo.FindAllChildDepartmentIDs(user.DepartmentID)
		if err != nil {
			return nil, err
		}

		return &OrgDataScope{
			Mode:          "department",
			DepartmentIDs: departmentIDs,
		}, nil
	}

	// 4. 普通员工 → 只能看自己（Mode="self"）
	return &OrgDataScope{
		Mode:          "self",
		DepartmentIDs: []string{},
	}, nil
}

// GetMenuPermission 获取角色的菜单权限
func (s *PermissionService) GetMenuPermission(roleID uint) (string, error) {
	mp, err := s.menuPermRepo.FindByRoleID(roleID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return "[]", nil
		}
		return "", err
	}
	return mp.MenuKeys, nil
}

// SaveMenuPermission 保存角色的菜单权限
func (s *PermissionService) SaveMenuPermission(roleID uint, menuKeys string) error {
	return s.menuPermRepo.Save(roleID, menuKeys)
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

// GetUserMenuKeys 聚合用户所有角色的菜单权限，返回去重后的 menu key 列表
func (s *PermissionService) GetUserMenuKeys(userID string) ([]string, error) {
	roles, err := s.userRoleRepo.FindByUserID(userID)
	if err != nil {
		return nil, err
	}
	keySet := make(map[string]struct{})
	for _, role := range roles {
		mp, err := s.menuPermRepo.FindByRoleID(role.ID)
		if err != nil {
			continue // 该角色未配置菜单权限，跳过
		}
		var keys []string
		if err := json.Unmarshal([]byte(mp.MenuKeys), &keys); err != nil {
			continue
		}
		for _, k := range keys {
			keySet[k] = struct{}{}
		}
	}
	result := make([]string, 0, len(keySet))
	for k := range keySet {
		result = append(result, k)
	}
	return result, nil
}
