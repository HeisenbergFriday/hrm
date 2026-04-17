package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type PermissionService struct {
	roleRepo       *repository.RoleRepository
	permissionRepo *repository.PermissionRepository
}

func NewPermissionService(db *gorm.DB) *PermissionService {
	return &PermissionService{
		roleRepo:       repository.NewRoleRepository(db),
		permissionRepo: repository.NewPermissionRepository(db),
	}
}

func (s *PermissionService) GetRoles() ([]database.Role, int64, error) {
	return s.roleRepo.FindAll()
}

func (s *PermissionService) CreateRole(role *database.Role) error {
	return s.roleRepo.Create(role)
}

func (s *PermissionService) GetPermissions() ([]database.Permission, int64, error) {
	return s.permissionRepo.FindAll()
}
