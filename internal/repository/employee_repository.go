package repository

import (
	"strings"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

type EmployeeRepository struct {
	db    *gorm.DB
	orgID string
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

// NewEmployeeRepository 仅保留给明确的全局迁移/审计场景。
// 普通业务读/写在 orgID 为空时 fail-closed（返回 ErrMissingOrgID / 空结果），
// 禁止再把该构造器用于 HTTP 请求路径。
func NewEmployeeRepository(db *gorm.DB) *EmployeeRepository {
	return &EmployeeRepository{db: db}
}

// NewEmployeeRepositoryWithOrgID 多租户构造：所有查询自动追加 org 过滤。
// profiles/transfers/resignations/onboardings 均带 org_id，直接按表内 org_id 过滤。
// orgID 为空时行为等同未绑定组织：读路径 fail-closed。
func NewEmployeeRepositoryWithOrgID(db *gorm.DB, orgID string) *EmployeeRepository {
	return &EmployeeRepository{db: db, orgID: strings.TrimSpace(orgID)}
}

// requireBoundOrg 普通请求必须绑定组织；空 org 禁止返回任何员工数据。
func (r *EmployeeRepository) requireBoundOrg() (string, error) {
	if r == nil {
		return "", ErrMissingOrgID
	}
	return RequireOrgID(r.orgID)
}

// applyProfileOrgFilter 按 employee_profiles.org_id 过滤；未绑定组织时 fail-closed（1=0）。
func (r *EmployeeRepository) applyProfileOrgFilter(query *gorm.DB) *gorm.DB {
	if r.orgID == "" {
		return query.Where("1 = 0")
	}
	return query.Where("employee_profiles.org_id = ?", r.orgID)
}

// applyUsersOrgFilter 用于以 users 为主表的查询；未绑定组织时 fail-closed。
func (r *EmployeeRepository) applyUsersOrgFilter(query *gorm.DB) *gorm.DB {
	if r.orgID == "" {
		return query.Where("1 = 0")
	}
	return query.Where("users.org_id = ?", r.orgID)
}

// applyTransferOrgFilter 直接按 employee_transfers.org_id 过滤。
func (r *EmployeeRepository) applyTransferOrgFilter(query *gorm.DB) *gorm.DB {
	if r.orgID == "" {
		return query.Where("1 = 0")
	}
	return query.Where("employee_transfers.org_id = ?", r.orgID)
}

// applyResignationOrgFilter 直接按 employee_resignations.org_id 过滤。
func (r *EmployeeRepository) applyResignationOrgFilter(query *gorm.DB) *gorm.DB {
	if r.orgID == "" {
		return query.Where("1 = 0")
	}
	return query.Where("employee_resignations.org_id = ?", r.orgID)
}

// applyOnboardingOrgFilter 直接按 employee_onboardings.org_id 过滤。
func (r *EmployeeRepository) applyOnboardingOrgFilter(query *gorm.DB) *gorm.DB {
	if r.orgID == "" {
		return query.Where("1 = 0")
	}
	return query.Where("employee_onboardings.org_id = ?", r.orgID)
}

// usersDepartmentSubquery 生成 users 子查询，必须同时约束 org_id 与 deleted_at。
// GORM 租户回调不会注入原生 SQL 子查询，因此这里必须显式带 org。
func (r *EmployeeRepository) usersDepartmentSubquery(singleDepartmentID string, departmentIDs []string) (string, []interface{}, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return "", nil, err
	}
	if singleDepartmentID != "" {
		return "user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id = ? AND deleted_at IS NULL)",
			[]interface{}{orgID, singleDepartmentID}, nil
	}
	if len(departmentIDs) > 0 {
		return "user_id IN (SELECT user_id FROM users WHERE org_id = ? AND department_id IN ? AND deleted_at IS NULL)",
			[]interface{}{orgID, departmentIDs}, nil
	}
	return "", nil, nil
}

// EmployeeProfile

func (r *EmployeeRepository) CreateProfile(profile *database.EmployeeProfile) error {
	if profile == nil {
		return gorm.ErrInvalidData
	}
	// 写路径必须绑定组织：未绑定或跨组织写入一律拒绝。
	merged, err := EnsureSameOrg(r.orgID, profile.OrgID)
	if err != nil {
		return err
	}
	profile.OrgID = merged
	return r.db.Create(profile).Error
}

func (r *EmployeeRepository) UpdateProfile(profile *database.EmployeeProfile) error {
	if profile == nil {
		return gorm.ErrInvalidData
	}
	merged, err := EnsureSameOrg(r.orgID, profile.OrgID)
	if err != nil {
		return err
	}
	profile.OrgID = merged
	return r.db.Save(profile).Error
}

func (r *EmployeeRepository) FindProfileByID(id string) (*database.EmployeeProfile, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var profile database.EmployeeProfile
	query := r.db.Model(&database.EmployeeProfile{}).Where("id = ?", id)
	query = r.applyProfileOrgFilter(query)
	err := query.First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *EmployeeRepository) FindProfileByUserID(userID string) (*database.EmployeeProfile, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var profile database.EmployeeProfile
	query := r.db.Model(&database.EmployeeProfile{}).Where("user_id = ?", userID)
	query = r.applyProfileOrgFilter(query)
	err := query.First(&profile).Error
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func (r *EmployeeRepository) FindAllProfiles(page, pageSize int, filters map[string]string) ([]database.EmployeeProfile, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var profiles []database.EmployeeProfile
	var total int64

	query := r.db.Model(&database.EmployeeProfile{})
	query = r.applyProfileOrgFilter(query)

	if v, ok := filters["department_id"]; ok && v != "" {
		clause, args, err := r.usersDepartmentSubquery(v, nil)
		if err != nil {
			return nil, 0, err
		}
		if clause != "" {
			query = query.Where(clause, args...)
		}
	}
	if departmentIDs := csvFilterValues(filters["department_ids"]); len(departmentIDs) > 0 {
		clause, args, err := r.usersDepartmentSubquery("", departmentIDs)
		if err != nil {
			return nil, 0, err
		}
		if clause != "" {
			query = query.Where(clause, args...)
		}
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
	// 绑定组织优先使用仓储显式 org；不得回退 CurrentOrganizationIDFromDB 的 default。
	orgID := strings.TrimSpace(r.orgID)
	query := r.db.Table("users").
		Joins("LEFT JOIN employee_profiles ON employee_profiles.org_id = users.org_id AND employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Joins("LEFT JOIN departments current_departments ON current_departments.org_id = users.org_id AND current_departments.department_id = users.department_id AND current_departments.deleted_at IS NULL").
		Joins(`LEFT JOIN employee_onboardings latest_onboarding ON latest_onboarding.id = (
			SELECT eo.id
			FROM employee_onboardings eo
			WHERE eo.deleted_at IS NULL
			  AND eo.org_id = users.org_id
			  AND (eo.employee_id = users.user_id OR eo.employee_id = employee_profiles.employee_id)
			ORDER BY eo.entry_date DESC, eo.id DESC
			LIMIT 1
		)`).
		Joins(`LEFT JOIN employee_transfers latest_transfer ON latest_transfer.id = (
			SELECT et.id
			FROM employee_transfers et
			WHERE et.deleted_at IS NULL
			  AND et.org_id = users.org_id
			  AND et.user_id = users.user_id
			ORDER BY et.transfer_date DESC, et.id DESC
			LIMIT 1
		)`).
		Joins(`LEFT JOIN employee_resignations latest_resignation ON latest_resignation.id = (
			SELECT er.id
			FROM employee_resignations er
			WHERE er.deleted_at IS NULL
			  AND er.org_id = users.org_id
			  AND er.user_id = users.user_id
			ORDER BY er.resign_date DESC, er.id DESC
			LIMIT 1
		)`).
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin")

	if orgID != "" {
		// 主表与 JOIN 关联均以仓储 org 收口，避免仅靠 users.org_id 联接跨组织同 ID 行。
		query = query.Where("users.org_id = ?", orgID)
	}
	query = r.applyUsersOrgFilter(query)

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
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	query := r.db.Table("employee_onboardings").
		Where("employee_onboardings.deleted_at IS NULL").
		Where("employee_onboardings.status IN (?)", []string{"pending", "processing", "completed"}).
		Where(`NOT EXISTS (
			SELECT 1 FROM employee_profiles ep
			WHERE ep.employee_id = employee_onboardings.employee_id
			  AND ep.org_id = employee_onboardings.org_id
			  AND ep.deleted_at IS NULL
		)`)

	// applyOnboardingOrgFilter 强制 org_id；未绑定时 1=0。
	query = r.applyOnboardingOrgFilter(query)

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
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
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
	if transfer == nil {
		return gorm.ErrInvalidData
	}
	merged, err := EnsureSameOrg(r.orgID, transfer.OrgID)
	if err != nil {
		return err
	}
	transfer.OrgID = merged
	return r.db.Create(transfer).Error
}

func (r *EmployeeRepository) FindAllTransfers(page, pageSize int, filters map[string]string) ([]database.EmployeeTransfer, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var transfers []database.EmployeeTransfer
	var total int64

	query := r.db.Model(&database.EmployeeTransfer{})
	query = r.applyTransferOrgFilter(query)
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
	if resignation == nil {
		return gorm.ErrInvalidData
	}
	merged, err := EnsureSameOrg(r.orgID, resignation.OrgID)
	if err != nil {
		return err
	}
	resignation.OrgID = merged
	return r.db.Create(resignation).Error
}

func (r *EmployeeRepository) FindAllResignations(page, pageSize int, filters map[string]string) ([]database.EmployeeResignation, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var resignations []database.EmployeeResignation
	var total int64

	query := r.db.Model(&database.EmployeeResignation{})
	query = r.applyResignationOrgFilter(query)
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
	if onboarding == nil {
		return gorm.ErrInvalidData
	}
	merged, err := EnsureSameOrg(r.orgID, onboarding.OrgID)
	if err != nil {
		return err
	}
	onboarding.OrgID = merged
	return r.db.Create(onboarding).Error
}

func (r *EmployeeRepository) FindAllOnboardings(page, pageSize int, filters map[string]string) ([]database.EmployeeOnboarding, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var onboardings []database.EmployeeOnboarding
	var total int64

	query := r.db.Model(&database.EmployeeOnboarding{})
	query = r.applyOnboardingOrgFilter(query)
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
