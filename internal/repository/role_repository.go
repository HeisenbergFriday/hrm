package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

func (r *RoleRepository) Create(role *database.Role) error {
	return r.db.Create(role).Error
}

func (r *RoleRepository) FindAll() ([]database.Role, int64, error) {
	var roles []database.Role
	var total int64

	if err := r.db.Model(&database.Role{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.Find(&roles).Error; err != nil {
		return nil, 0, err
	}

	return roles, total, nil
}

type PermissionRepository struct {
	db *gorm.DB
}

func NewPermissionRepository(db *gorm.DB) *PermissionRepository {
	return &PermissionRepository{db: db}
}

func (r *PermissionRepository) FindAll() ([]database.Permission, int64, error) {
	var permissions []database.Permission
	var total int64

	if err := r.db.Model(&database.Permission{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.Find(&permissions).Error; err != nil {
		return nil, 0, err
	}

	return permissions, total, nil
}
