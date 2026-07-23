package repository

import (
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
)

type UserRepository struct {
	db    *gorm.DB
	orgID string
}

// NewUserRepository constructs a user repository bound to the org context on db
// when present. Without an explicit tenant context the repository stays unbound
// and all read/write methods fail closed (1=0 / ErrMissingOrgID).
func NewUserRepository(db *gorm.DB) *UserRepository {
	orgID := ""
	if db != nil {
		if resolved, err := database.RequireOrganizationIDFromDB(db); err == nil {
			orgID = strings.TrimSpace(resolved)
		}
	}
	return &UserRepository{
		db:    db,
		orgID: orgID,
	}
}

// NewUserRepositoryWithOrgID 构造带 org 隔离的用户仓储；orgID 为空时 fail-closed。
func NewUserRepositoryWithOrgID(db *gorm.DB, orgID string) *UserRepository {
	return &UserRepository{
		db:    db,
		orgID: strings.TrimSpace(orgID),
	}
}

func (r *UserRepository) boundOrgID() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.orgID)
}

func (r *UserRepository) requireBoundOrg() (string, error) {
	orgID := r.boundOrgID()
	if orgID == "" {
		return "", ErrMissingOrgID
	}
	return orgID, nil
}

// scoped 返回已应用 org 过滤的查询；空 org 时 fail-closed（1=0），禁止全表扫描。
func (r *UserRepository) scoped() *gorm.DB {
	return r.db.Scopes(ScopeOrg(r.boundOrgID(), "org_id"))
}

// scopedTable 返回已应用 users.org_id 过滤的查询构造器，供 join 场景使用。
func (r *UserRepository) scopedTable() *gorm.DB {
	return r.db.Model(&database.User{}).Scopes(ScopeOrg(r.boundOrgID(), "users.org_id"))
}

func (r *UserRepository) Create(user *database.User) error {
	if user == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	// 严格 tenant 仓储：写入时强制 org 一致。允许调用方留空以继承仓储 org，
	// 但不允许传入其它组织的 org_id，从而防止越权写入。
	merged, err := EnsureSameOrg(orgID, user.OrgID)
	if err != nil {
		return err
	}
	user.OrgID = merged
	return r.db.Create(user).Error
}

func (r *UserRepository) Update(user *database.User) error {
	if user == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, user.OrgID)
	if err != nil {
		return err
	}
	user.OrgID = merged
	return r.scoped().Where("id = ?", user.ID).Save(user).Error
}

func (r *UserRepository) Delete(userID string) error {
	if _, err := r.requireBoundOrg(); err != nil {
		return err
	}
	return r.scoped().Delete(&database.User{}, "user_id = ?", userID).Error
}

func (r *UserRepository) FindByUserID(userID string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	tx := r.scoped().Where("user_id = ?", userID).Limit(1).Find(&user)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

// FindByOrgAndUserID 根据组织ID和用户ID查找用户（多租户）
func (r *UserRepository) FindByOrgAndUserID(orgID, userID string) (*database.User, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	if bound := r.boundOrgID(); bound != "" && bound != orgID {
		return nil, ErrOrgMismatch
	}
	var user database.User
	tx := r.db.Where("org_id = ? AND user_id = ?", orgID, userID).Limit(1).Find(&user)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &user, nil
}

func (r *UserRepository) FindByEmail(email string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	err := r.scoped().Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByOrgAndEmail 根据组织ID和邮箱查找用户（多租户）
func (r *UserRepository) FindByOrgAndEmail(orgID, email string) (*database.User, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	if bound := r.boundOrgID(); bound != "" && bound != orgID {
		return nil, ErrOrgMismatch
	}
	var user database.User
	err := r.db.Where("org_id = ? AND email = ?", orgID, email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByOrgAndMobile 根据组织ID和手机号查找用户（多租户）
func (r *UserRepository) FindByOrgAndMobile(orgID, mobile string) (*database.User, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil, ErrMissingOrgID
	}
	if bound := r.boundOrgID(); bound != "" && bound != orgID {
		return nil, ErrOrgMismatch
	}
	var user database.User
	err := r.db.Where("org_id = ? AND mobile = ?", orgID, mobile).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByMobile(mobile string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	err := r.scoped().Where("mobile = ?", mobile).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByID(id string) (*database.User, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var user database.User
	err := r.scoped().First(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindAll(page, pageSize int) ([]database.User, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize

	// 计算总数
	err := r.scopedTable().Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 查询数据
	err = r.scoped().Offset(offset).Limit(pageSize).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) FindAllFiltered(page, pageSize int, search string) ([]database.User, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	// 作息表推送等选人场景需要一次拉取更多在职员工；仍保留上限防止误用超大分页。
	if pageSize > 1000 {
		pageSize = 1000
	}

	query := r.scopedTable()
	if search = strings.TrimSpace(search); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"users.name LIKE ? OR users.user_id LIKE ? OR users.mobile LIKE ? OR users.email LIKE ?",
			like, like, like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []database.User
	err := query.Order("users.name ASC, users.id ASC").
		Offset((page - 1) * pageSize).
		Limit(pageSize).
		Find(&users).Error
	return users, total, err
}

func (r *UserRepository) FindSyncedEmployees(page, pageSize int) ([]database.User, int64, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize
	query := r.db.Model(&database.User{}).
		Joins("JOIN employee_profiles ON employee_profiles.org_id = users.org_id AND employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin").
		Where("users.org_id = ?", orgID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Select("users.*").Order("users.created_at DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) FindByDepartment(departmentID string, page, pageSize int) ([]database.User, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize

	// 计算总数
	err := r.scopedTable().Where("department_id = ?", departmentID).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 查询数据
	err = r.scoped().Where("department_id = ?", departmentID).Offset(offset).Limit(pageSize).Find(&users).Error
	if err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) FindSyncedEmployeesByDepartment(departmentID string, page, pageSize int) ([]database.User, int64, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return nil, 0, err
	}
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize
	query := r.db.Model(&database.User{}).
		Joins("JOIN employee_profiles ON employee_profiles.org_id = users.org_id AND employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin").
		Where("users.department_id = ?", departmentID).
		Where("users.org_id = ?", orgID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Select("users.*").Order("users.created_at DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
