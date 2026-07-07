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

func (r *RoleRepository) Update(role *database.Role) error {
	return r.db.Model(role).Updates(map[string]interface{}{
		"name":        role.Name,
		"description": role.Description,
	}).Error
}

func (r *RoleRepository) FindByID(roleID uint) (*database.Role, error) {
	var role database.Role
	if err := r.db.Where("id = ? AND deleted_at IS NULL", roleID).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
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

// UserRoleRepository 用户-角色关联
type UserRoleRepository struct {
	db *gorm.DB
}

func NewUserRoleRepository(db *gorm.DB) *UserRoleRepository {
	return &UserRoleRepository{db: db}
}

func (r *UserRoleRepository) FindByUserID(userID string) ([]database.Role, error) {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var roles []database.Role
	err := r.db.
		Joins("JOIN user_roles ON user_roles.role_id = roles.id AND user_roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND roles.org_id = ? AND roles.deleted_at IS NULL", orgID, userID, orgID).
		Find(&roles).Error
	return roles, err
}

func (r *UserRoleRepository) Assign(userID string, roleID uint) error {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Unscoped().Where("org_id = ? AND user_id = ?", orgID, userID).Delete(&database.UserRole{}).Error; err != nil {
			return err
		}
		return tx.Create(&database.UserRole{OrgID: orgID, UserID: userID, RoleID: roleID}).Error
	})
}

func (r *UserRoleRepository) Remove(userID string, roleID uint) error {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	return r.db.Unscoped().Where("org_id = ? AND user_id = ? AND role_id = ?", orgID, userID, roleID).
		Delete(&database.UserRole{}).Error
}

func (r *UserRoleRepository) HasRole(userID string, roleName string) (bool, error) {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var count int64
	err := r.db.
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND roles.org_id = ? AND roles.name = ? AND user_roles.deleted_at IS NULL", orgID, userID, orgID, roleName).
		Model(&database.UserRole{}).Count(&count).Error
	return count > 0, err
}

func (r *UserRoleRepository) FindByRoleID(roleID uint) ([]database.User, error) {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var users []database.User
	err := r.db.
		Joins("JOIN user_roles ON user_roles.user_id = users.user_id AND user_roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.role_id = ? AND users.org_id = ? AND users.deleted_at IS NULL", orgID, roleID, orgID).
		Find(&users).Error
	return users, err
}

// RolePermissionRepository 角色-权限关联
type RolePermissionRepository struct {
	db *gorm.DB
}

func NewRolePermissionRepository(db *gorm.DB) *RolePermissionRepository {
	return &RolePermissionRepository{db: db}
}

func (r *RolePermissionRepository) FindByRoleID(roleID uint) ([]database.Permission, error) {
	var permissions []database.Permission
	err := r.db.
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id AND role_permissions.deleted_at IS NULL").
		Where("role_permissions.role_id = ? AND permissions.deleted_at IS NULL", roleID).
		Find(&permissions).Error
	return permissions, err
}

func (r *RolePermissionRepository) Assign(roleID uint, permissionID uint) error {
	rp := database.RolePermission{RoleID: roleID, PermissionID: permissionID}
	return r.db.Where(database.RolePermission{RoleID: roleID, PermissionID: permissionID}).
		FirstOrCreate(&rp).Error
}

func (r *RolePermissionRepository) FindByUserRole(userID string) ([]database.Permission, error) {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var permissions []database.Permission
	err := r.db.
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id AND role_permissions.deleted_at IS NULL").
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id AND user_roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND permissions.deleted_at IS NULL", orgID, userID).
		Distinct().
		Find(&permissions).Error
	return permissions, err
}

// MenuPermissionRepository 角色菜单权限
type MenuPermissionRepository struct {
	db *gorm.DB
}

func NewMenuPermissionRepository(db *gorm.DB) *MenuPermissionRepository {
	return &MenuPermissionRepository{db: db}
}

func (r *MenuPermissionRepository) FindByRoleID(roleID uint) (*database.MenuPermission, error) {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var mp database.MenuPermission
	err := r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&mp).Error
	if err != nil {
		return nil, err
	}
	return &mp, nil
}

func (r *MenuPermissionRepository) FindByUserRole(userID string) ([]database.MenuPermission, error) {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var menuPermissions []database.MenuPermission
	err := r.db.
		Joins("JOIN user_roles ON user_roles.role_id = menu_permissions.role_id AND user_roles.deleted_at IS NULL").
		Where("menu_permissions.org_id = ? AND user_roles.org_id = ? AND user_roles.user_id = ? AND menu_permissions.deleted_at IS NULL", orgID, orgID, userID).
		Find(&menuPermissions).Error
	return menuPermissions, err
}

func (r *MenuPermissionRepository) Save(roleID uint, menuKeys string) error {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var existing database.MenuPermission
	err := r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		mp := database.MenuPermission{OrgID: orgID, RoleID: roleID, MenuKeys: menuKeys}
		return r.db.Create(&mp).Error
	}
	if err != nil {
		return err
	}
	return r.db.Model(&existing).Update("menu_keys", menuKeys).Error
}

// DataPermissionRepository 角色数据权限
type DataPermissionRepository struct {
	db *gorm.DB
}

func NewDataPermissionRepository(db *gorm.DB) *DataPermissionRepository {
	return &DataPermissionRepository{db: db}
}

func (r *DataPermissionRepository) FindByRoleID(roleID uint) (*database.DataPermission, error) {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var dp database.DataPermission
	err := r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&dp).Error
	if err != nil {
		return nil, err
	}
	return &dp, nil
}

func (r *DataPermissionRepository) Save(roleID uint, scope string, departmentKeys string) error {
	orgID := database.CurrentOrganizationIDFromDB(r.db)
	var existing database.DataPermission
	err := r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		dp := database.DataPermission{OrgID: orgID, RoleID: roleID, Scope: scope, DepartmentKeys: departmentKeys}
		return r.db.Create(&dp).Error
	}
	if err != nil {
		return err
	}
	return r.db.Model(&existing).Updates(map[string]interface{}{
		"scope":           scope,
		"department_keys": departmentKeys,
	}).Error
}
