package repository

import (
	"errors"
	"strings"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

type RoleRepository struct {
	db *gorm.DB
}

func NewRoleRepository(db *gorm.DB) *RoleRepository {
	return &RoleRepository{db: db}
}

// normalizeOrgID trims orgID only. Empty stays empty so callers fail closed
// instead of silently querying the default tenant.
func normalizeOrgID(orgID string) string {
	return strings.TrimSpace(orgID)
}

func (r *RoleRepository) Create(role *database.Role) error {
	if role == nil {
		return gorm.ErrInvalidData
	}
	// Roles are organization-scoped tenant data; empty org_id is never allowed.
	if normalizeOrgID(role.OrgID) == "" {
		return ErrMissingOrgID
	}
	return r.db.Create(role).Error
}

func (r *RoleRepository) Update(role *database.Role) error {
	if role == nil {
		return gorm.ErrInvalidData
	}
	// Fail-closed: never update by primary key alone. Org must be explicit on the
	// entity (PermissionService stamps service-bound org before calling Update).
	// Entity OrgID is a filter, not a free-form re-parent target — only name/description change.
	orgID := normalizeOrgID(role.OrgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	if role.ID == 0 {
		return gorm.ErrInvalidData
	}
	res := r.db.Model(&database.Role{}).
		Where("id = ? AND org_id = ? AND deleted_at IS NULL", role.ID, orgID).
		Updates(map[string]interface{}{
			"name":        role.Name,
			"description": role.Description,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdateInOrg updates a role only when it belongs to orgID. Prefer this over Update
// when the tenant is known independently of the entity payload.
func (r *RoleRepository) UpdateInOrg(orgID string, role *database.Role) error {
	if role == nil {
		return gorm.ErrInvalidData
	}
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	// Ignore any forged role.OrgID from the client; bound org is authoritative.
	role.OrgID = orgID
	return r.Update(role)
}

// FindByID is intentionally fail-closed. Roles are tenant data; unscoped ID
// lookups are forbidden. Callers must use FindByIDAndOrg with an explicit org.
func (r *RoleRepository) FindByID(roleID uint) (*database.Role, error) {
	return nil, ErrMissingOrgID
}

// FindByIDAndOrg loads a role only when it belongs to the given organization.
// Cross-org role IDs resolve to gorm.ErrRecordNotFound.
func (r *RoleRepository) FindByIDAndOrg(roleID uint, orgID string) (*database.Role, error) {
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	var role database.Role
	if err := r.db.Where("id = ? AND org_id = ? AND deleted_at IS NULL", roleID, orgID).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// FindAll is intentionally fail-closed. Roles are tenant data; unscoped listing
// is forbidden. Callers must use FindAllByOrg with an explicit organization.
func (r *RoleRepository) FindAll() ([]database.Role, int64, error) {
	return nil, 0, ErrMissingOrgID
}

// FindAllByOrg lists roles for a single organization.
func (r *RoleRepository) FindAllByOrg(orgID string) ([]database.Role, int64, error) {
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return nil, 0, ErrMissingOrgID
	}
	var roles []database.Role
	var total int64

	if err := r.db.Model(&database.Role{}).Where("org_id = ?", orgID).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.Where("org_id = ?", orgID).Find(&roles).Error; err != nil {
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

func (r *UserRoleRepository) FindByUserID(orgID, userID string) ([]database.Role, error) {
	var roles []database.Role
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	// Require roles.org_id == user_roles.org_id so a poisoned cross-org role_id
	// cannot surface another organization's role definition.
	err := r.db.
		Joins("JOIN user_roles ON user_roles.role_id = roles.id AND user_roles.deleted_at IS NULL AND user_roles.org_id = roles.org_id").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND roles.org_id = ? AND roles.deleted_at IS NULL", orgID, userID, orgID).
		Find(&roles).Error
	return roles, err
}

func (r *UserRoleRepository) Assign(orgID, userID string, roleID uint) error {
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	if strings.TrimSpace(userID) == "" || roleID == 0 {
		return gorm.ErrRecordNotFound
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Validate role belongs to the same org BEFORE replacing any existing assignment.
		// Cross-org or missing role_id must fail closed and leave user_roles untouched.
		var role database.Role
		if err := tx.Where("id = ? AND org_id = ? AND deleted_at IS NULL", roleID, orgID).First(&role).Error; err != nil {
			return err
		}
		if err := tx.Unscoped().Where("org_id = ? AND user_id = ?", orgID, userID).Delete(&database.UserRole{}).Error; err != nil {
			return err
		}
		return tx.Create(&database.UserRole{OrgID: orgID, UserID: userID, RoleID: roleID}).Error
	})
}

func (r *UserRoleRepository) Remove(orgID, userID string, roleID uint) error {
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	return r.db.Unscoped().Where("org_id = ? AND user_id = ? AND role_id = ?", orgID, userID, roleID).
		Delete(&database.UserRole{}).Error
}

func (r *UserRoleRepository) HasRole(orgID, userID string, roleName string) (bool, error) {
	var count int64
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return false, ErrMissingOrgID
	}
	err := r.db.
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.deleted_at IS NULL AND roles.org_id = user_roles.org_id").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND roles.name = ? AND roles.org_id = ? AND user_roles.deleted_at IS NULL", orgID, userID, roleName, orgID).
		Model(&database.UserRole{}).Count(&count).Error
	return count > 0, err
}

func (r *UserRoleRepository) FindByRoleID(orgID string, roleID uint) ([]database.User, error) {
	var users []database.User
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	// Join roles so a cross-org role_id cannot list users under the current org.
	err := r.db.
		Joins("JOIN user_roles ON user_roles.user_id = users.user_id AND user_roles.org_id = users.org_id AND user_roles.deleted_at IS NULL").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.org_id = user_roles.org_id AND roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.role_id = ? AND roles.org_id = ? AND users.deleted_at IS NULL", orgID, roleID, orgID).
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

func (r *RolePermissionRepository) FindByUserRole(orgID, userID string) ([]database.Permission, error) {
	var permissions []database.Permission
	orgID = normalizeOrgID(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	// permissions / role_permissions remain global catalogs; only the user→role binding is org-scoped.
	// Still require roles.org_id == user_roles.org_id so a poisoned cross-org role_id cannot grant privileges.
	err := r.db.
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id AND role_permissions.deleted_at IS NULL").
		Joins("JOIN user_roles ON user_roles.role_id = role_permissions.role_id AND user_roles.deleted_at IS NULL").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.org_id = user_roles.org_id AND roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND roles.org_id = ? AND permissions.deleted_at IS NULL", orgID, userID, orgID).
		Distinct().
		Find(&permissions).Error
	return permissions, err
}

// MenuPermissionRepository 角色菜单权限
type MenuPermissionRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewMenuPermissionRepository(db *gorm.DB) *MenuPermissionRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &MenuPermissionRepository{db: db, orgID: orgID, orgErr: err}
}

func NewMenuPermissionRepositoryWithOrgID(db *gorm.DB, orgID string) *MenuPermissionRepository {
	normalized, err := RequireOrgID(orgID)
	return &MenuPermissionRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *MenuPermissionRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", mapMissingOrgErr(r.orgErr)
	}
	return RequireOrgID(r.orgID)
}

func (r *MenuPermissionRepository) FindByRoleID(roleID uint) (*database.MenuPermission, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var mp database.MenuPermission
	err = r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&mp).Error
	if err != nil {
		return nil, err
	}
	return &mp, nil
}

func (r *MenuPermissionRepository) FindByUserRole(orgID, userID string) ([]database.MenuPermission, error) {
	bound, err := r.requireOrgID()
	if err != nil {
		// Allow explicit org argument when repo is unbound (caller-supplied tenant).
		if orgID = normalizeOrgID(orgID); orgID == "" {
			return nil, err
		}
		bound = orgID
	} else if orgID = normalizeOrgID(orgID); orgID != "" && orgID != bound {
		return nil, ErrOrgMismatch
	}
	var menuPermissions []database.MenuPermission
	err = r.db.
		Joins("JOIN user_roles ON user_roles.role_id = menu_permissions.role_id AND user_roles.deleted_at IS NULL AND user_roles.org_id = menu_permissions.org_id").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.org_id = user_roles.org_id AND roles.deleted_at IS NULL").
		Where("user_roles.org_id = ? AND user_roles.user_id = ? AND menu_permissions.org_id = ? AND menu_permissions.deleted_at IS NULL", bound, userID, bound).
		Find(&menuPermissions).Error
	return menuPermissions, err
}

func (r *MenuPermissionRepository) Save(roleID uint, menuKeys string) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	var existing database.MenuPermission
	err = r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&existing).Error
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
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewDataPermissionRepository(db *gorm.DB) *DataPermissionRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &DataPermissionRepository{db: db, orgID: orgID, orgErr: err}
}

func NewDataPermissionRepositoryWithOrgID(db *gorm.DB, orgID string) *DataPermissionRepository {
	normalized, err := RequireOrgID(orgID)
	return &DataPermissionRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *DataPermissionRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", mapMissingOrgErr(r.orgErr)
	}
	return RequireOrgID(r.orgID)
}

func (r *DataPermissionRepository) FindByRoleID(roleID uint) (*database.DataPermission, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var dp database.DataPermission
	err = r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&dp).Error
	if err != nil {
		return nil, err
	}
	return &dp, nil
}

func (r *DataPermissionRepository) Save(roleID uint, scope string, departmentKeys string) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	var existing database.DataPermission
	err = r.db.Where("org_id = ? AND role_id = ? AND deleted_at IS NULL", orgID, roleID).First(&existing).Error
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

func mapMissingOrgErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, ErrMissingOrgID) {
		return err
	}
	if strings.Contains(err.Error(), "missing organization") {
		return ErrMissingOrgID
	}
	return err
}
