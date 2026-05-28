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
	// 阶段 3B 新增字段：候选入职人员支持
	IsCandidate             bool   `json:"is_candidate"`              // 是否候选入职人员（未建档）
	OnboardingID            string `json:"onboarding_id"`             // 入职记录ID
	OnboardingStatusDisplay string `json:"onboarding_status_display"` // 状态展示文本
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
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		query = query.Where("user_id IN (SELECT user_id FROM users WHERE department_id IN ? AND deleted_at IS NULL)", departmentIDs)
	}
	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("user_id = ?", v)
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
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		query = query.Where("users.department_id IN ?", departmentIDs)
	}
	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("users.user_id = ?", v)
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
	// 阶段 3B：调用新方法支持候选入职人员合并
	return r.FindLifecycleLedgerWithCandidates(page, pageSize, filters)
}

// FindCandidateOnboardings 查询候选入职人员（未建档的 onboarding 记录）
// 阶段 3B：候选入职人员合并进台账
// 当前历史口径限制：
// 1. employee_onboardings.employee_id 是员工工号，不是 users.user_id
// 2. onboarding 与 users 之间没有明确的 user_id 外键
// 3. 判断是否已建档只能通过 employee_profiles.employee_id 匹配工号
// 4. 后续建议：增加 employee_onboardings.user_id 字段建立明确关联
func (r *EmployeeRepository) FindCandidateOnboardings(filters map[string]string) ([]EmployeeLifecycleLedgerItem, int64, error) {
	query := r.db.Table("employee_onboardings").
		Where("employee_onboardings.deleted_at IS NULL").
		Where("employee_onboardings.status IN (?)", []string{"pending", "processing", "completed"}).
		Where(`NOT EXISTS (
			SELECT 1 FROM employee_profiles ep
			WHERE ep.employee_id = employee_onboardings.employee_id
			  AND ep.deleted_at IS NULL
		)`)

	// 应用筛选条件
	if v, ok := filters["department_id"]; ok && v != "" {
		query = query.Where("employee_onboardings.department_id = ?", v)
	}
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		query = query.Where("employee_onboardings.department_id IN ?", departmentIDs)
	}
	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("1 = 0")
	}
	if v, ok := filters["keyword"]; ok && v != "" {
		like := "%" + v + "%"
		query = query.Where(
			`(
				employee_onboardings.employee_id LIKE ?
				OR employee_onboardings.name LIKE ?
				OR employee_onboardings.email LIKE ?
				OR employee_onboardings.mobile LIKE ?
				OR employee_onboardings.onboarding_id LIKE ?
			)`,
			like, like, like, like, like,
		)
	}

	// 计数
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// 查询数据
	var items []EmployeeLifecycleLedgerItem
	if err := query.
		Select(`
			employee_onboardings.id,
			'' AS user_id,
			employee_onboardings.name AS user_name,
			employee_onboardings.employee_id,
			employee_onboardings.department_id,
			employee_onboardings.department_name,
			employee_onboardings.position,
			'candidate' AS user_status,
			'' AS profile_status,
			employee_onboardings.employment_type,
			employee_onboardings.entry_date,
			employee_onboardings.probation_end_date AS planned_regular_date,
			'' AS actual_regular_date,
			'' AS latest_transfer_date,
			'' AS latest_transfer_status,
			'' AS latest_transfer_old_department,
			'' AS latest_transfer_old_position,
			'' AS latest_transfer_new_department,
			'' AS latest_transfer_new_position,
			'' AS latest_resign_date,
			'' AS latest_resignation_status,
			'' AS latest_last_working_day,
			'' AS latest_resign_reason,
			employee_onboardings.status AS latest_onboarding_status,
			true AS is_candidate,
			employee_onboardings.onboarding_id,
			CASE
				WHEN employee_onboardings.status = 'pending' THEN '候选入职'
				WHEN employee_onboardings.status = 'processing' THEN '入职处理中'
				WHEN employee_onboardings.status = 'completed' THEN '入职已完成/待建档'
				ELSE employee_onboardings.status
			END AS onboarding_status_display
		`).
		Order("employee_onboardings.entry_date DESC").
		Order("employee_onboardings.created_at DESC").
		Scan(&items).Error; err != nil {
		return nil, 0, err
	}

	return items, total, nil
}

// FindLifecycleLedgerWithCandidates 查询台账（包含候选入职人员）
// 阶段 3B：合并候选入职人员和已入职员工
func (r *EmployeeRepository) FindLifecycleLedgerWithCandidates(page, pageSize int, filters map[string]string) ([]EmployeeLifecycleLedgerItem, int64, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}

	// 1. 查询候选入职人员
	var candidateItems []EmployeeLifecycleLedgerItem
	var candidateTotal int64

	// 如果筛选了 status=active 或 inactive，不查询候选人员
	statusFilter := filters["status"]
	if statusFilter == "" || statusFilter == "candidate" {
		var err error
		candidateItems, candidateTotal, err = r.FindCandidateOnboardings(filters)
		if err != nil {
			return nil, 0, err
		}
	}

	// 2. 查询已入职员工
	var existingItems []EmployeeLifecycleLedgerItem
	var existingTotal int64

	if statusFilter != "candidate" {
		query := r.buildLifecycleLedgerQuery(filters)
		countQuery := query.Session(&gorm.Session{})
		dataQuery := query.Session(&gorm.Session{})

		if err := countQuery.Distinct("users.id").Count(&existingTotal).Error; err != nil {
			return nil, 0, err
		}

		// 计算分页偏移
		offset := (page - 1) * pageSize
		existingOffset := 0
		limit := pageSize

		// 如果 offset 落在候选区间内，先跳过候选人员
		if offset < int(candidateTotal) {
			// 当前页包含候选人员
			limit = pageSize
		} else {
			// 当前页只包含已入职员工
			existingOffset = offset - int(candidateTotal)
		}

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
				COALESCE(latest_onboarding.status, '') AS latest_onboarding_status,
				false AS is_candidate,
				COALESCE(latest_onboarding.onboarding_id, '') AS onboarding_id,
				'' AS onboarding_status_display
			`).
			Order("users.status ASC").
			Order("CASE WHEN COALESCE(NULLIF(employee_profiles.entry_date, ''), NULLIF(latest_onboarding.entry_date, '')) = '' THEN 1 ELSE 0 END ASC").
			Order("COALESCE(NULLIF(employee_profiles.entry_date, ''), NULLIF(latest_onboarding.entry_date, '')) DESC").
			Order("users.created_at DESC").
			Order("users.id DESC").
			Offset(existingOffset).
			Limit(limit).
			Scan(&existingItems).Error; err != nil {
			return nil, 0, err
		}
	}

	// 3. 合并结果
	offset := (page - 1) * pageSize
	var result []EmployeeLifecycleLedgerItem

	if offset < int(candidateTotal) {
		// 当前页包含候选人员
		candidateStart := offset
		candidateEnd := candidateStart + pageSize
		if candidateEnd > int(candidateTotal) {
			candidateEnd = int(candidateTotal)
		}

		// 添加候选人员
		result = append(result, candidateItems[candidateStart:candidateEnd]...)

		// 如果还有空间，添加已入职员工
		remaining := pageSize - len(result)
		if remaining > 0 && len(existingItems) > 0 {
			if remaining > len(existingItems) {
				remaining = len(existingItems)
			}
			result = append(result, existingItems[:remaining]...)
		}
	} else {
		// 当前页只包含已入职员工
		result = existingItems
	}

	totalCount := candidateTotal + existingTotal
	return result, totalCount, nil
}

// EmployeeTransfer

func (r *EmployeeRepository) CreateTransfer(transfer *database.EmployeeTransfer) error {
	return r.db.Create(transfer).Error
}

func (r *EmployeeRepository) FindAllTransfers(page, pageSize int, filters map[string]string) ([]database.EmployeeTransfer, int64, error) {
	var transfers []database.EmployeeTransfer
	var total int64

	query := r.db.Model(&database.EmployeeTransfer{})
	if status := filters["status"]; status != "" {
		query = query.Where("status = ?", status)
	}
	if userID := filters["user_id"]; userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if departmentID := filters["department_id"]; departmentID != "" {
		query = query.Where("(old_department_id = ? OR new_department_id = ?)", departmentID, departmentID)
	}
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		query = query.Where("(old_department_id IN ? OR new_department_id IN ?)", departmentIDs, departmentIDs)
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

func (r *EmployeeRepository) FindAllResignations(page, pageSize int, filters map[string]string) ([]database.EmployeeResignation, int64, error) {
	var resignations []database.EmployeeResignation
	var total int64

	query := r.db.Model(&database.EmployeeResignation{})
	if status := filters["status"]; status != "" {
		query = query.Where("status = ?", status)
	}
	if userID := filters["user_id"]; userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	if departmentID := filters["department_id"]; departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		query = query.Where("department_id IN ?", departmentIDs)
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

func (r *EmployeeRepository) FindAllOnboardings(page, pageSize int, filters map[string]string) ([]database.EmployeeOnboarding, int64, error) {
	var onboardings []database.EmployeeOnboarding
	var total int64

	query := r.db.Model(&database.EmployeeOnboarding{})
	if status := filters["status"]; status != "" {
		query = query.Where("status = ?", status)
	}
	if userID := filters["user_id"]; userID != "" {
		query = query.Where("1 = 0")
	}
	if departmentID := filters["department_id"]; departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		query = query.Where("department_id IN ?", departmentIDs)
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
