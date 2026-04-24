package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{
		db: db,
	}
}

func (r *UserRepository) Create(user *database.User) error {
	return r.db.Create(user).Error
}

func (r *UserRepository) Update(user *database.User) error {
	return r.db.Save(user).Error
}

func (r *UserRepository) Delete(userID string) error {
	return r.db.Delete(&database.User{}, "user_id = ?", userID).Error
}

func (r *UserRepository) FindByUserID(userID string) (*database.User, error) {
	var user database.User
	err := r.db.Where("user_id = ?", userID).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (r *UserRepository) FindByID(id string) (*database.User, error) {
	var user database.User
	err := r.db.First(&user, "id = ?", id).Error
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
	err := r.db.Model(&database.User{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 查询数据
	err = r.db.Offset(offset).Limit(pageSize).Find(&users).Error
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
		Joins("JOIN employee_profiles ON employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin")

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
	err := r.db.Model(&database.User{}).Where("department_id = ?", departmentID).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	// 查询数据
	err = r.db.Where("department_id = ?", departmentID).Offset(offset).Limit(pageSize).Find(&users).Error
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
		Joins("JOIN employee_profiles ON employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin").
		Where("users.department_id = ?", departmentID)

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	if err := query.Select("users.*").Order("users.created_at DESC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}
