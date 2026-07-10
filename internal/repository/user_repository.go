package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type UserRepository struct {
	db    *gorm.DB
	orgID string
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

// NewUserRepositoryWithOrgID 构造带 org 隔离的用户仓储；orgID 为空时行为等同旧构造（不加过滤）。
func NewUserRepositoryWithOrgID(db *gorm.DB, orgID string) *UserRepository {
	return &UserRepository{
		db:    db,
		orgID: orgID,
	}
}

// scoped 返回一个已应用 orgID 过滤（如果非空）的查询构造器，基于 users 表。
func (r *UserRepository) scoped() *gorm.DB {
	tx := r.db
	if r.orgID != "" {
		tx = tx.Where("org_id = ?", r.orgID)
	}
	return tx
}

// scopedTable 返回一个已应用 users.org_id 过滤的查询构造器，供 join 场景使用。
func (r *UserRepository) scopedTable() *gorm.DB {
	tx := r.db.Model(&database.User{})
	if r.orgID != "" {
		tx = tx.Where("users.org_id = ?", r.orgID)
	}
	return tx
}

func (r *UserRepository) Create(user *database.User) error {
	return r.db.Create(user).Error
}

func (r *UserRepository) Update(user *database.User) error {
	return r.db.Save(user).Error
}

func (r *UserRepository) Delete(userID string) error {
	tx := r.db
	if r.orgID != "" {
		tx = tx.Where("org_id = ?", r.orgID)
	}
	return tx.Delete(&database.User{}, "user_id = ?", userID).Error
}

func (r *UserRepository) FindByUserID(userID string) (*database.User, error) {
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
	var user database.User
	err := r.scoped().Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByOrgAndEmail 根据组织ID和邮箱查找用户（多租户）
func (r *UserRepository) FindByOrgAndEmail(orgID, email string) (*database.User, error) {
	var user database.User
	err := r.db.Where("org_id = ? AND email = ?", orgID, email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByOrgAndMobile 根据组织ID和手机号查找用户（多租户）
func (r *UserRepository) FindByOrgAndMobile(orgID, mobile string) (*database.User, error) {
	var user database.User
	err := r.db.Where("org_id = ? AND mobile = ?", orgID, mobile).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByMobile(mobile string) (*database.User, error) {
	var user database.User
	err := r.scoped().Where("mobile = ?", mobile).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByID(id string) (*database.User, error) {
	var user database.User
	err := r.scoped().First(&user, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindAll(page, pageSize int) ([]database.User, int64, error) {
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

func (r *UserRepository) FindSyncedEmployees(page, pageSize int) ([]database.User, int64, error) {
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize
	query := r.db.Model(&database.User{}).
		Joins("JOIN employee_profiles ON employee_profiles.org_id = users.org_id AND employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin")
	if r.orgID != "" {
		query = query.Where("users.org_id = ?", r.orgID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Select("users.*").Order("users.created_at DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (r *UserRepository) FindByDepartment(departmentID string, page, pageSize int) ([]database.User, int64, error) {
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
	var users []database.User
	var total int64

	offset := (page - 1) * pageSize
	query := r.db.Model(&database.User{}).
		Joins("JOIN employee_profiles ON employee_profiles.org_id = users.org_id AND employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin").
		Where("users.department_id = ?", departmentID)
	if r.orgID != "" {
		query = query.Where("users.org_id = ?", r.orgID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Select("users.*").Order("users.created_at DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
