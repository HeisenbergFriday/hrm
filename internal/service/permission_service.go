package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type PermissionService struct {
	roleRepo           *repository.RoleRepository
	permissionRepo     *repository.PermissionRepository
	userRoleRepo       *repository.UserRoleRepository
	rolePermissionRepo *repository.RolePermissionRepository
}

func NewPermissionService(db *gorm.DB) *PermissionService {
	return &PermissionService{
		roleRepo:           repository.NewRoleRepository(db),
		permissionRepo:     repository.NewPermissionRepository(db),
		userRoleRepo:       repository.NewUserRoleRepository(db),
		rolePermissionRepo: repository.NewRolePermissionRepository(db),
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

// HasUserRole 检查用户是否有某角色
func (s *PermissionService) HasUserRole(userID string, roleName string) (bool, error) {
	return s.userRoleRepo.HasRole(userID, roleName)
}
