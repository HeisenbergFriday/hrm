package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type EmployeeRepository struct {
	db *gorm.DB
}

type EmployeeLifecycleLedgerItem struct {
	ID                          uint   `json:"id"`
	UserID                      string `json:"user_id"`
	UserName                    string `json:"user_name"`
	EmployeeID                  string `json:"employee_id"`
	DepartmentID                string `json:"department_id"`
	DepartmentName              string `json:"department_name"`
	Position                    string `json:"position"`
	UserStatus                  string `json:"user_status"`
	ProfileStatus               string `json:"profile_status"`
	EmploymentType              string `json:"employment_type"`
	EntryDate                   string `json:"entry_date"`
	PlannedRegularDate          string `json:"planned_regular_date"`
	ActualRegularDate           string `json:"actual_regular_date"`
	LatestTransferDate          string `json:"latest_transfer_date"`
	LatestTransferStatus        string `json:"latest_transfer_status"`
	LatestTransferOldDepartment string `json:"latest_transfer_old_department"`
	LatestTransferOldPosition   string `json:"latest_transfer_old_position"`
	LatestTransferNewDepartment string `json:"latest_transfer_new_department"`
	LatestTransferNewPosition   string `json:"latest_transfer_new_position"`
	LatestResignDate            string `json:"latest_resign_date"`
	LatestResignationStatus     string `json:"latest_resignation_status"`
	LatestLastWorkingDay        string `json:"latest_last_working_day"`
	LatestResignReason          string `json:"latest_resign_reason"`
	LatestOnboardingStatus      string `json:"latest_onboarding_status"`
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

func (r *EmployeeRepository) buildLifecycleLedgerQuery(filters map[string]string) *gorm.DB {
	query := r.db.Table("users").
		Joins("LEFT JOIN employee_profiles ON employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Joins("LEFT JOIN departments current_departments ON current_departments.department_id = users.department_id AND current_departments.deleted_at IS NULL").
		Joins(`LEFT JOIN employee_onboardings latest_onboarding ON latest_onboarding.id = (
			SELECT eo.id
			FROM employee_onboardings eo
			WHERE eo.deleted_at IS NULL
			  AND (eo.employee_id = users.user_id OR eo.employee_id = employee_profiles.employee_id)
			ORDER BY eo.entry_date DESC, eo.id DESC
			LIMIT 1
		)`).
		Joins(`LEFT JOIN employee_transfers latest_transfer ON latest_transfer.id = (
			SELECT et.id
			FROM employee_transfers et
			WHERE et.deleted_at IS NULL
			  AND et.user_id = users.user_id
			ORDER BY et.transfer_date DESC, et.id DESC
			LIMIT 1
		)`).
		Joins(`LEFT JOIN employee_resignations latest_resignation ON latest_resignation.id = (
			SELECT er.id
			FROM employee_resignations er
			WHERE er.deleted_at IS NULL
			  AND er.user_id = users.user_id
			ORDER BY er.resign_date DESC, er.id DESC
			LIMIT 1
		)`).
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin")

	if v, ok := filters["department_id"]; ok && v != "" {
		query = query.Where("users.department_id = ?", v)
	}
	if v, ok := filters["status"]; ok && v != "" {
		query = query.Where("users.status = ?", v)
	}
	if v, ok := filters["keyword"]; ok && v != "" {
		like := "%" + v + "%"
		query = query.Where(
			`(
				users.user_id LIKE ?
				OR users.name LIKE ?
				OR users.email LIKE ?
				OR users.mobile LIKE ?
				OR users.position LIKE ?
				OR employee_profiles.employee_id LIKE ?
			)`,
			like, like, like, like, like, like,
		)
	}

	return query
}

func (r *EmployeeRepository) FindLifecycleLedger(page, pageSize int, filters map[string]string) ([]EmployeeLifecycleLedgerItem, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	query := r.buildLifecycleLedgerQuery(filters)
	countQuery := query.Session(&gorm.Session{})
	dataQuery := query.Session(&gorm.Session{})

	var total int64
	if err := countQuery.Distinct("users.id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var items []EmployeeLifecycleLedgerItem
	offset := (page - 1) * pageSize
	if err := dataQuery.
		Select(`
			users.id,
			users.user_id,
			users.name AS user_name,
			COALESCE(employee_profiles.employee_id, '') AS employee_id,
			users.department_id,
			COALESCE(NULLIF(current_departments.name, ''), users.department_id, '') AS department_name,
			COALESCE(users.position, '') AS position,
			COALESCE(users.status, '') AS user_status,
			COALESCE(employee_profiles.profile_status, '') AS profile_status,
			COALESCE(NULLIF(employee_profiles.employment_type, ''), NULLIF(latest_onboarding.employment_type, ''), '') AS employment_type,
			COALESCE(NULLIF(employee_profiles.entry_date, ''), NULLIF(latest_onboarding.entry_date, ''), '') AS entry_date,
			COALESCE(employee_profiles.planned_regular_date, '') AS planned_regular_date,
			COALESCE(employee_profiles.actual_regular_date, '') AS actual_regular_date,
			COALESCE(latest_transfer.transfer_date, '') AS latest_transfer_date,
			COALESCE(latest_transfer.status, '') AS latest_transfer_status,
			COALESCE(latest_transfer.old_department_name, '') AS latest_transfer_old_department,
			COALESCE(latest_transfer.old_position, '') AS latest_transfer_old_position,
			COALESCE(latest_transfer.new_department_name, '') AS latest_transfer_new_department,
			COALESCE(latest_transfer.new_position, '') AS latest_transfer_new_position,
			COALESCE(latest_resignation.resign_date, '') AS latest_resign_date,
			COALESCE(latest_resignation.status, '') AS latest_resignation_status,
			COALESCE(latest_resignation.last_working_day, '') AS latest_last_working_day,
			COALESCE(latest_resignation.resign_reason, '') AS latest_resign_reason,
			COALESCE(latest_onboarding.status, '') AS latest_onboarding_status
		`).
		Order("users.status ASC").
		Order("CASE WHEN COALESCE(NULLIF(employee_profiles.entry_date, ''), NULLIF(latest_onboarding.entry_date, '')) = '' THEN 1 ELSE 0 END ASC").
		Order("COALESCE(NULLIF(employee_profiles.entry_date, ''), NULLIF(latest_onboarding.entry_date, '')) DESC").
		Order("users.created_at DESC").
		Order("users.id DESC").
		Offset(offset).
		Limit(pageSize).
		Scan(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
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
