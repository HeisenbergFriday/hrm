package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type EmployeeRepository struct {
	db *gorm.DB
}

func NewEmployeeRepository(db *gorm.DB) *EmployeeRepository {
	return &EmployeeRepository{db: db}
}

// EmployeeProfile

func (r *EmployeeRepository) CreateProfile(profile *database.EmployeeProfile) error {
	return r.db.Create(profile).Error
}

func (r *EmployeeRepository) UpdateProfile(profile *database.EmployeeProfile) error {
	return r.db.Save(profile).Error
}

func (r *EmployeeRepository) FindProfileByID(id string) (*database.EmployeeProfile, error) {
	var profile database.EmployeeProfile
	err := r.db.First(&profile, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *EmployeeRepository) FindProfileByUserID(userID string) (*database.EmployeeProfile, error) {
	var profile database.EmployeeProfile
	err := r.db.Where("user_id = ?", userID).First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *EmployeeRepository) FindAllProfiles(page, pageSize int, filters map[string]string) ([]database.EmployeeProfile, int64, error) {
	var profiles []database.EmployeeProfile
	var total int64

	query := r.db.Model(&database.EmployeeProfile{})

	if v, ok := filters["department_id"]; ok && v != "" {
		query = query.Where("user_id IN (SELECT user_id FROM users WHERE department_id = ? AND deleted_at IS NULL)", v)
	}
	if v, ok := filters["status"]; ok && v != "" {
		query = query.Where("profile_status = ?", v)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&profiles).Error; err != nil {
		return nil, 0, err
	}

	return profiles, total, nil
}

// EmployeeTransfer

func (r *EmployeeRepository) CreateTransfer(transfer *database.EmployeeTransfer) error {
	return r.db.Create(transfer).Error
}

func (r *EmployeeRepository) FindAllTransfers(page, pageSize int, status string) ([]database.EmployeeTransfer, int64, error) {
	var transfers []database.EmployeeTransfer
	var total int64

	query := r.db.Model(&database.EmployeeTransfer{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&transfers).Error; err != nil {
		return nil, 0, err
	}

	return transfers, total, nil
}

// EmployeeResignation

func (r *EmployeeRepository) CreateResignation(resignation *database.EmployeeResignation) error {
	return r.db.Create(resignation).Error
}

func (r *EmployeeRepository) FindAllResignations(page, pageSize int, status string) ([]database.EmployeeResignation, int64, error) {
	var resignations []database.EmployeeResignation
	var total int64

	query := r.db.Model(&database.EmployeeResignation{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&resignations).Error; err != nil {
		return nil, 0, err
	}

	return resignations, total, nil
}

// EmployeeOnboarding

func (r *EmployeeRepository) CreateOnboarding(onboarding *database.EmployeeOnboarding) error {
	return r.db.Create(onboarding).Error
}

func (r *EmployeeRepository) FindAllOnboardings(page, pageSize int, status string) ([]database.EmployeeOnboarding, int64, error) {
	var onboardings []database.EmployeeOnboarding
	var total int64

	query := r.db.Model(&database.EmployeeOnboarding{})
	if status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&onboardings).Error; err != nil {
		return nil, 0, err
	}

	return onboardings, total, nil
}
