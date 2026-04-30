package service

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"peopleops/internal/database"

	"gorm.io/gorm"
)

var ErrOrgAccessDenied = errors.New("org access denied")

const scopeEmptyDepartmentMarker = "__scope_empty__"

type OrgDataScope struct {
	Mode              string   `json:"mode"`
	DepartmentIDs     []string `json:"department_ids"`
	DepartmentNames   []string `json:"department_names"`
	RootDepartmentIDs []string `json:"root_department_ids"`

	all             bool
	departmentIDSet map[string]struct{}
}

func (s *OrgDataScope) init() {
	s.departmentIDSet = make(map[string]struct{}, len(s.DepartmentIDs))
	for _, departmentID := range s.DepartmentIDs {
		departmentID = strings.TrimSpace(departmentID)
		if departmentID == "" {
			continue
		}
		s.departmentIDSet[departmentID] = struct{}{}
	}
}

func (s *OrgDataScope) IsAll() bool {
	return s == nil || s.all || strings.EqualFold(s.Mode, "all")
}

func (s *OrgDataScope) AllowsDepartment(departmentID string) bool {
	if s == nil || s.IsAll() {
		return true
	}
	_, ok := s.departmentIDSet[departmentID]
	return ok
}

func (s *OrgDataScope) clone() *OrgDataScope {
	if s == nil {
		scope := &OrgDataScope{Mode: "all", all: true}
		scope.init()
		return scope
	}

	cloned := &OrgDataScope{
		Mode:              s.Mode,
		DepartmentIDs:     append([]string(nil), s.DepartmentIDs...),
		DepartmentNames:   append([]string(nil), s.DepartmentNames...),
		RootDepartmentIDs: append([]string(nil), s.RootDepartmentIDs...),
		all:               s.all,
	}
	cloned.init()
	return cloned
}

type OrgOverviewSummary struct {
	TotalEmployees              int `json:"total_employees"`
	ActiveEmployees             int `json:"active_employees"`
	InactiveEmployees           int `json:"inactive_employees"`
	DepartmentCount             int `json:"department_count"`
	ProbationEmployeeCount      int `json:"probation_employee_count"`
	ProbationDueCount           int `json:"probation_due_count"`
	PlannedRegularizationCount  int `json:"planned_regularization_count"`
	ContractExpiringCount       int `json:"contract_expiring_count"`
	PendingOnboardingCount      int `json:"pending_onboarding_count"`
	PendingTransferCount        int `json:"pending_transfer_count"`
	PendingResignationCount     int `json:"pending_resignation_count"`
	ConsecutiveResignationCount int `json:"consecutive_resignation_count"`
	OverspanManagerCount        int `json:"overspan_manager_count"`
}

type OrgWarningItem struct {
	Type           string `json:"type"`
	Level          string `json:"level"`
	Title          string `json:"title"`
	Description    string `json:"description"`
	UserID         string `json:"user_id,omitempty"`
	UserName       string `json:"user_name,omitempty"`
	DepartmentID   string `json:"department_id,omitempty"`
	DepartmentName string `json:"department_name,omitempty"`
	DueDate        string `json:"due_date,omitempty"`
	DaysLeft       int    `json:"days_left,omitempty"`
}

type OrgTrendPoint struct {
	Month            string `json:"month"`
	OnboardingCount  int    `json:"onboarding_count"`
	TransferCount    int    `json:"transfer_count"`
	ResignationCount int    `json:"resignation_count"`
}

type OrgDepartmentStat struct {
	DepartmentID   string `json:"department_id"`
	DepartmentName string `json:"department_name"`
	ParentID       string `json:"parent_id"`
	Headcount      int    `json:"headcount"`
	ActiveCount    int    `json:"active_count"`
	InactiveCount  int    `json:"inactive_count"`
}

type OrgDistributionItem struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type OrgOverview struct {
	Scope                    *OrgDataScope         `json:"scope"`
	Summary                  OrgOverviewSummary    `json:"summary"`
	Warnings                 []OrgWarningItem      `json:"warnings"`
	Trends                   []OrgTrendPoint       `json:"trends"`
	DepartmentStats          []OrgDepartmentStat   `json:"department_stats"`
	EmployeeTypeDistribution []OrgDistributionItem `json:"employee_type_distribution"`
	JobLevelDistribution     []OrgDistributionItem `json:"job_level_distribution"`
	JobFamilyDistribution    []OrgDistributionItem `json:"job_family_distribution"`
}

type OrgDepartmentSyncItem struct {
	DepartmentID string
	Name         string
	ParentID     string
	Order        int
}

type OrgDepartmentSyncResult struct {
	Count          int `json:"count"`
	ChangeLogCount int `json:"change_log_count"`
}

type OrgDepartmentTreeNode struct {
	ID                string                   `json:"id"`
	Name              string                   `json:"name"`
	ParentID          string                   `json:"parent_id"`
	Headcount         int                      `json:"headcount"`
	ActiveCount       int                      `json:"active_count"`
	InactiveCount     int                      `json:"inactive_count"`
	DirectHeadcount   int                      `json:"direct_headcount"`
	DirectActiveCount int                      `json:"direct_active_count"`
	Children          []*OrgDepartmentTreeNode `json:"children"`
}

type OrgEmployeeFilters struct {
	DepartmentID string
	Search       string
	Status       string
}

type EmployeeDepartmentPath struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type EmployeeDepartmentInfo struct {
	ID   string                   `json:"id"`
	Name string                   `json:"name"`
	Path []EmployeeDepartmentPath `json:"path"`
}

type EmployeeMemberRef struct {
	ID             string `json:"id"`
	UserID         string `json:"user_id"`
	Name           string `json:"name"`
	DepartmentID   string `json:"department_id"`
	DepartmentName string `json:"department_name"`
	Position       string `json:"position"`
}

type EmployeeOrgRelation struct {
	Manager             *EmployeeMemberRef  `json:"manager,omitempty"`
	DirectReports       []EmployeeMemberRef `json:"direct_reports"`
	SameDepartmentCount int                 `json:"same_department_count"`
}

type EmployeeTimelineEndpoint struct {
	DepartmentID   string `json:"department_id,omitempty"`
	DepartmentName string `json:"department_name,omitempty"`
	Position       string `json:"position,omitempty"`
}

type EmployeeTimelineEvent struct {
	Type         string                    `json:"type"`
	Title        string                    `json:"title"`
	Description  string                    `json:"description"`
	Date         string                    `json:"date"`
	Status       string                    `json:"status,omitempty"`
	OperatorName string                    `json:"operator_name,omitempty"`
	From         *EmployeeTimelineEndpoint `json:"from,omitempty"`
	To           *EmployeeTimelineEndpoint `json:"to,omitempty"`
	Reason       string                    `json:"reason,omitempty"`
}

type EmployeeAggregate struct {
	Employee    *database.User            `json:"employee"`
	Profile     *database.EmployeeProfile `json:"profile"`
	Scope       *OrgDataScope             `json:"scope"`
	Department  EmployeeDepartmentInfo    `json:"department"`
	OrgRelation EmployeeOrgRelation       `json:"org_relation"`
	Timeline    []EmployeeTimelineEvent   `json:"timeline"`
	Warnings    []OrgWarningItem          `json:"warnings"`
}

type orgEmployeeSnapshot struct {
	ID                 uint   `gorm:"column:id"`
	UserID             string `gorm:"column:user_id"`
	Name               string `gorm:"column:name"`
	Email              string `gorm:"column:email"`
	Mobile             string `gorm:"column:mobile"`
	DepartmentID       string `gorm:"column:department_id"`
	Position           string `gorm:"column:position"`
	Status             string `gorm:"column:status"`
	Avatar             string `gorm:"column:avatar"`
	EntryDate          string `gorm:"column:entry_date"`
	PlannedRegularDate string `gorm:"column:planned_regular_date"`
	ActualRegularDate  string `gorm:"column:actual_regular_date"`
	ProbationEndDate   string `gorm:"column:probation_end_date"`
	ContractEndDate    string `gorm:"column:contract_end_date"`
	EmploymentType     string `gorm:"column:employment_type"`
	JobLevel           string `gorm:"column:job_level"`
	JobFamily          string `gorm:"column:job_family"`
	ProfileStatus      string `gorm:"column:profile_status"`
}

type OrgService struct {
	db    *gorm.DB
	nowFn func() time.Time
}

func NewOrgService(db *gorm.DB) *OrgService {
	return &OrgService{
		db:    db,
		nowFn: func() time.Time { return time.Now() },
	}
}

func (s *OrgService) ResolveScopeForUser(currentUserID string) (*OrgDataScope, error) {
	if strings.TrimSpace(currentUserID) == "" {
		scope := &OrgDataScope{Mode: "all", all: true}
		scope.init()
		return scope, nil
	}

	var currentUser database.User
	if err := s.db.Where("id = ? AND deleted_at IS NULL", currentUserID).First(&currentUser).Error; err != nil {
		return nil, err
	}

	if strings.EqualFold(currentUser.UserID, "admin") {
		scope := &OrgDataScope{Mode: "all", all: true}
		scope.init()
		return scope, nil
	}

	mode := strings.ToLower(strings.TrimSpace(firstStringValue(
		currentUser.Extension,
		"org_data_scope",
		"data_scope",
		"department_scope",
	)))
	if mode == "all" {
		scope := &OrgDataScope{Mode: "all", all: true}
		scope.init()
		return scope, nil
	}

	_, departmentMap, childMap, err := s.loadDepartmentGraph()
	if err != nil {
		return nil, err
	}

	rootDepartmentIDs := firstStringSliceValue(
		currentUser.Extension,
		"org_data_department_ids",
		"department_ids",
		"data_scope_department_ids",
	)
	if len(rootDepartmentIDs) == 0 && strings.TrimSpace(currentUser.DepartmentID) != "" {
		rootDepartmentIDs = []string{currentUser.DepartmentID}
	}
	if len(rootDepartmentIDs) == 0 {
		scope := &OrgDataScope{Mode: "all", all: true}
		scope.init()
		return scope, nil
	}

	visibleDepartmentIDs := make([]string, 0)
	for _, rootDepartmentID := range rootDepartmentIDs {
		rootDepartmentID = strings.TrimSpace(rootDepartmentID)
		if rootDepartmentID == "" {
			continue
		}
		if _, ok := departmentMap[rootDepartmentID]; !ok {
			continue
		}
		visibleDepartmentIDs = append(visibleDepartmentIDs, collectDescendantIDs(rootDepartmentID, childMap)...)
	}
	visibleDepartmentIDs = uniqueStrings(visibleDepartmentIDs)

	scope := &OrgDataScope{
		Mode:              "department",
		DepartmentIDs:     visibleDepartmentIDs,
		DepartmentNames:   departmentNamesByIDs(rootDepartmentIDs, departmentMap),
		RootDepartmentIDs: uniqueStrings(rootDepartmentIDs),
	}
	if len(visibleDepartmentIDs) == 0 {
		scope.Mode = "none"
	}
	scope.init()
	return scope, nil
}

func (s *OrgService) GetVisibleDepartments(scope *OrgDataScope) ([]database.Department, error) {
	departments, _, _, err := s.loadDepartmentGraph()
	if err != nil {
		return nil, err
	}
	if scope == nil || scope.IsAll() {
		return departments, nil
	}

	visible := make([]database.Department, 0, len(scope.DepartmentIDs))
	for _, department := range departments {
		if scope.AllowsDepartment(department.DepartmentID) {
			visible = append(visible, department)
		}
	}
	return visible, nil
}

func (s *OrgService) SyncDepartmentsWithChangeLog(items []OrgDepartmentSyncItem, source string) (OrgDepartmentSyncResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = "dingtalk_sync"
	}

	result := OrgDepartmentSyncResult{}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, item := range items {
			item.DepartmentID = strings.TrimSpace(item.DepartmentID)
			item.Name = strings.TrimSpace(item.Name)
			item.ParentID = strings.TrimSpace(item.ParentID)
			if item.DepartmentID == "" || item.Name == "" {
				continue
			}

			result.Count++
			var existing database.Department
			err := tx.Where("department_id = ?", item.DepartmentID).First(&existing).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				department := database.Department{
					DepartmentID: item.DepartmentID,
					Name:         item.Name,
					ParentID:     item.ParentID,
					Order:        item.Order,
				}
				if err := tx.Create(&department).Error; err != nil {
					return err
				}
				if err := createDepartmentChangeLog(tx, item.DepartmentID, item.Name, "created", "department", "", item.Name, source, s.nowFn()); err != nil {
					return err
				}
				result.ChangeLogCount++
				continue
			}
			if err != nil {
				return err
			}

			logs := make([]database.DepartmentChangeLog, 0, 3)
			if existing.Name != item.Name {
				logs = append(logs, newDepartmentChangeLog(item.DepartmentID, item.Name, "updated", "name", existing.Name, item.Name, source, s.nowFn()))
				existing.Name = item.Name
			}
			if existing.ParentID != item.ParentID {
				logs = append(logs, newDepartmentChangeLog(item.DepartmentID, item.Name, "updated", "parent_id", existing.ParentID, item.ParentID, source, s.nowFn()))
				existing.ParentID = item.ParentID
			}
			if existing.Order != item.Order {
				logs = append(logs, newDepartmentChangeLog(item.DepartmentID, item.Name, "updated", "order", strconv.Itoa(existing.Order), strconv.Itoa(item.Order), source, s.nowFn()))
				existing.Order = item.Order
			}
			if len(logs) == 0 {
				continue
			}
			if err := tx.Save(&existing).Error; err != nil {
				return err
			}
			if err := tx.Create(&logs).Error; err != nil {
				return err
			}
			result.ChangeLogCount += len(logs)
		}
		return nil
	})
	return result, err
}

func (s *OrgService) GetDepartmentHistory(scope *OrgDataScope, departmentID string, limit int) ([]database.DepartmentChangeLog, error) {
	departmentID = strings.TrimSpace(departmentID)
	if departmentID == "" {
		return []database.DepartmentChangeLog{}, nil
	}

	if scope != nil && !scope.IsAll() {
		_, departmentMap, _, err := s.loadDepartmentGraph()
		if err != nil {
			return nil, err
		}
		if _, ok := departmentMap[departmentID]; ok && !scope.AllowsDepartment(departmentID) {
			return nil, ErrOrgAccessDenied
		}
	}

	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	var logs []database.DepartmentChangeLog
	if err := s.db.Where("department_id = ?", departmentID).
		Order("changed_at DESC, id DESC").
		Limit(limit).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (s *OrgService) ListEmployees(scope *OrgDataScope, page, pageSize int, filters OrgEmployeeFilters) ([]database.User, int64, error) {
	departmentIDs, _, err := s.resolveDepartmentFilter(scope, filters.DepartmentID)
	if err != nil {
		return nil, 0, err
	}

	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}

	query := s.baseEmployeeQuery(departmentIDs)
	if status := strings.TrimSpace(filters.Status); status != "" {
		query = query.Where("users.status = ?", status)
	}
	if search := strings.TrimSpace(filters.Search); search != "" {
		like := "%" + search + "%"
		query = query.Where(
			"(users.user_id LIKE ? OR users.name LIKE ? OR users.email LIKE ? OR users.mobile LIKE ? OR users.position LIKE ?)",
			like, like, like, like, like,
		)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var users []database.User
	offset := (page - 1) * pageSize
	if err := query.Select("users.*").
		Order("users.created_at DESC").
		Offset(offset).
		Limit(pageSize).
		Find(&users).Error; err != nil {
		return nil, 0, err
	}

	return users, total, nil
}

func (s *OrgService) GetOverview(scope *OrgDataScope, departmentID string) (*OrgOverview, error) {
	filteredDepartmentIDs, departmentMap, err := s.resolveDepartmentFilter(scope, departmentID)
	if err != nil {
		return nil, err
	}

	snapshots, err := s.listEmployeeSnapshots(filteredDepartmentIDs)
	if err != nil {
		return nil, err
	}

	scopeCopy := scope.clone()
	if departmentID != "" {
		scopeCopy.Mode = "department"
		scopeCopy.DepartmentIDs = append([]string(nil), filteredDepartmentIDs...)
		scopeCopy.DepartmentNames = departmentNamesByIDs([]string{departmentID}, departmentMap)
		scopeCopy.RootDepartmentIDs = []string{departmentID}
		scopeCopy.all = false
		scopeCopy.init()
	}

	summary, warnings := s.buildOverviewSummary(snapshots, departmentMap)
	pendingWarnings, pendingCounts, err := s.collectPendingFlowWarnings(filteredDepartmentIDs, departmentMap)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, pendingWarnings...)

	consecWarnings, consecCount, err := s.collectConsecutiveResignationWarnings(filteredDepartmentIDs, departmentMap)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, consecWarnings...)

	overspanWarnings, overspanCount, err := s.collectOverspanManagerWarnings(filteredDepartmentIDs, departmentMap)
	if err != nil {
		return nil, err
	}
	warnings = append(warnings, overspanWarnings...)

	sortWarnings(warnings)
	if len(warnings) > 10 {
		warnings = warnings[:10]
	}

	summary.PendingOnboardingCount = pendingCounts["onboarding"]
	summary.PendingTransferCount = pendingCounts["transfer"]
	summary.PendingResignationCount = pendingCounts["resignation"]
	summary.ConsecutiveResignationCount = consecCount
	summary.OverspanManagerCount = overspanCount
	summary.DepartmentCount = countUniqueDepartments(snapshots)

	trends, err := s.buildTrendPoints(filteredDepartmentIDs, snapshots)
	if err != nil {
		return nil, err
	}

	return &OrgOverview{
		Scope:                    scopeCopy,
		Summary:                  summary,
		Warnings:                 warnings,
		Trends:                   trends,
		DepartmentStats:          buildDepartmentStats(snapshots, departmentMap),
		EmployeeTypeDistribution: buildDistributionItems(snapshots, func(snapshot orgEmployeeSnapshot) string { return snapshot.EmploymentType }),
		JobLevelDistribution:     buildDistributionItems(snapshots, func(snapshot orgEmployeeSnapshot) string { return snapshot.JobLevel }),
		JobFamilyDistribution:    buildDistributionItems(snapshots, func(snapshot orgEmployeeSnapshot) string { return snapshot.JobFamily }),
	}, nil
}

func (s *OrgService) GetDepartmentTree(scope *OrgDataScope) ([]*OrgDepartmentTreeNode, error) {
	departments, _, _, err := s.loadDepartmentGraph()
	if err != nil {
		return nil, err
	}

	if scope != nil && !scope.IsAll() {
		filtered := make([]database.Department, 0, len(scope.DepartmentIDs))
		for _, department := range departments {
			if scope.AllowsDepartment(department.DepartmentID) {
				filtered = append(filtered, department)
			}
		}
		departments = filtered
	}

	snapshots, err := s.listEmployeeSnapshots(scopeDepartmentIDs(scope))
	if err != nil {
		return nil, err
	}

	directCounts := make(map[string]*OrgDepartmentTreeNode)
	for _, snapshot := range snapshots {
		count := directCounts[snapshot.DepartmentID]
		if count == nil {
			count = &OrgDepartmentTreeNode{}
			directCounts[snapshot.DepartmentID] = count
		}
		count.Headcount++
		count.DirectHeadcount++
		if strings.EqualFold(snapshot.Status, "active") {
			count.ActiveCount++
			count.DirectActiveCount++
		} else {
			count.InactiveCount++
		}
	}

	nodeMap := make(map[string]*OrgDepartmentTreeNode, len(departments))
	roots := make([]*OrgDepartmentTreeNode, 0)
	for _, department := range departments {
		count := directCounts[department.DepartmentID]
		if count == nil {
			count = &OrgDepartmentTreeNode{}
		}
		nodeMap[department.DepartmentID] = &OrgDepartmentTreeNode{
			ID:                department.DepartmentID,
			Name:              department.Name,
			ParentID:          department.ParentID,
			Headcount:         count.Headcount,
			ActiveCount:       count.ActiveCount,
			InactiveCount:     count.InactiveCount,
			DirectHeadcount:   count.DirectHeadcount,
			DirectActiveCount: count.DirectActiveCount,
			Children:          []*OrgDepartmentTreeNode{},
		}
	}

	for _, department := range departments {
		node := nodeMap[department.DepartmentID]
		parent, ok := nodeMap[department.ParentID]
		if ok {
			parent.Children = append(parent.Children, node)
			continue
		}
		roots = append(roots, node)
	}

	sortDepartmentTree(roots)
	for _, root := range roots {
		rollupTreeCounts(root)
	}
	return roots, nil
}

func (s *OrgService) GetEmployeeAggregate(scope *OrgDataScope, id string) (*EmployeeAggregate, error) {
	var user database.User
	if err := s.db.Where("id = ? AND deleted_at IS NULL", id).First(&user).Error; err != nil {
		return nil, err
	}
	if scope != nil && !scope.IsAll() && !scope.AllowsDepartment(user.DepartmentID) {
		return nil, ErrOrgAccessDenied
	}

	var profile database.EmployeeProfile
	profileErr := s.db.Where("user_id = ? AND deleted_at IS NULL", user.UserID).First(&profile).Error
	if profileErr != nil && !errors.Is(profileErr, gorm.ErrRecordNotFound) {
		return nil, profileErr
	}

	var profilePtr *database.EmployeeProfile
	if profileErr == nil {
		profileCopy := profile
		profilePtr = &profileCopy
	}

	_, departmentMap, _, err := s.loadDepartmentGraph()
	if err != nil {
		return nil, err
	}

	orgRelation, err := s.buildOrgRelation(scope, &user, departmentMap)
	if err != nil {
		return nil, err
	}
	timeline, err := s.buildEmployeeTimeline(&user, profilePtr)
	if err != nil {
		return nil, err
	}

	return &EmployeeAggregate{
		Employee: &user,
		Profile:  profilePtr,
		Scope:    scope.clone(),
		Department: EmployeeDepartmentInfo{
			ID:   user.DepartmentID,
			Name: departmentMap[user.DepartmentID].Name,
			Path: buildDepartmentPath(user.DepartmentID, scope, departmentMap),
		},
		OrgRelation: orgRelation,
		Timeline:    timeline,
		Warnings:    s.buildEmployeeWarnings(snapshotFromEmployee(&user, profilePtr), departmentMap),
	}, nil
}

func (s *OrgService) baseEmployeeQuery(departmentIDs []string) *gorm.DB {
	query := s.db.Model(&database.User{}).
		Joins("JOIN employee_profiles ON employee_profiles.user_id = users.user_id AND employee_profiles.deleted_at IS NULL").
		Where("users.deleted_at IS NULL").
		Where("users.user_id <> ?", "admin")
	if len(departmentIDs) > 0 {
		query = query.Where("users.department_id IN ?", departmentIDs)
	}
	return query
}

func (s *OrgService) listEmployeeSnapshots(departmentIDs []string) ([]orgEmployeeSnapshot, error) {
	var snapshots []orgEmployeeSnapshot
	err := s.baseEmployeeQuery(departmentIDs).
		Select(`
			users.id,
			users.user_id,
			users.name,
			users.email,
			users.mobile,
			users.department_id,
			users.position,
			users.status,
			users.avatar,
			employee_profiles.entry_date,
			employee_profiles.planned_regular_date,
			employee_profiles.actual_regular_date,
			employee_profiles.probation_end_date,
			employee_profiles.contract_end_date,
			employee_profiles.employment_type,
			employee_profiles.job_level,
			employee_profiles.job_family,
			employee_profiles.profile_status
		`).
		Order("users.created_at DESC").
		Find(&snapshots).Error
	return snapshots, err
}

func (s *OrgService) resolveDepartmentFilter(scope *OrgDataScope, requestedDepartmentID string) ([]string, map[string]database.Department, error) {
	_, departmentMap, childMap, err := s.loadDepartmentGraph()
	if err != nil {
		return nil, nil, err
	}

	requestedDepartmentID = strings.TrimSpace(requestedDepartmentID)
	if requestedDepartmentID == "" {
		if scope == nil || scope.IsAll() {
			return nil, departmentMap, nil
		}
		if len(scope.DepartmentIDs) == 0 {
			return []string{scopeEmptyDepartmentMarker}, departmentMap, nil
		}
		return append([]string(nil), scope.DepartmentIDs...), departmentMap, nil
	}

	if _, ok := departmentMap[requestedDepartmentID]; !ok {
		return []string{scopeEmptyDepartmentMarker}, departmentMap, nil
	}

	subtree := collectDescendantIDs(requestedDepartmentID, childMap)
	if scope == nil || scope.IsAll() {
		return subtree, departmentMap, nil
	}

	filtered := make([]string, 0, len(subtree))
	for _, departmentID := range subtree {
		if scope.AllowsDepartment(departmentID) {
			filtered = append(filtered, departmentID)
		}
	}
	if len(filtered) == 0 {
		return nil, nil, ErrOrgAccessDenied
	}
	return filtered, departmentMap, nil
}

func (s *OrgService) loadDepartmentGraph() ([]database.Department, map[string]database.Department, map[string][]string, error) {
	var departments []database.Department
	if err := s.db.Where("deleted_at IS NULL").
		Order("parent_id ASC, `order` ASC, name ASC").
		Find(&departments).Error; err != nil {
		return nil, nil, nil, err
	}

	departmentMap := make(map[string]database.Department, len(departments))
	childMap := make(map[string][]string, len(departments))
	for _, department := range departments {
		departmentMap[department.DepartmentID] = department
		childMap[department.ParentID] = append(childMap[department.ParentID], department.DepartmentID)
	}
	for parentID := range childMap {
		children := childMap[parentID]
		sort.SliceStable(children, func(i, j int) bool {
			left := departmentMap[children[i]]
			right := departmentMap[children[j]]
			if left.Order == right.Order {
				return left.Name < right.Name
			}
			return left.Order < right.Order
		})
		childMap[parentID] = children
	}

	return departments, departmentMap, childMap, nil
}

func (s *OrgService) buildOverviewSummary(snapshots []orgEmployeeSnapshot, departmentMap map[string]database.Department) (OrgOverviewSummary, []OrgWarningItem) {
	summary := OrgOverviewSummary{
		TotalEmployees: len(snapshots),
	}
	warnings := make([]OrgWarningItem, 0)
	now := beginningOfDay(s.nowFn())
	windowEnd := now.AddDate(0, 0, 30)

	for _, snapshot := range snapshots {
		if strings.EqualFold(snapshot.Status, "active") {
			summary.ActiveEmployees++
			if isProbationEmployee(snapshot) {
				summary.ProbationEmployeeCount++
			}
		} else {
			summary.InactiveEmployees++
		}

		for _, warning := range s.buildEmployeeWarnings(snapshot, departmentMap) {
			if dueDate, ok := parseDateValue(warning.DueDate); ok {
				if !dueDate.Before(now) && !dueDate.After(windowEnd) {
					switch warning.Type {
					case "probation_due":
						summary.ProbationDueCount++
						summary.PlannedRegularizationCount++
					case "contract_expiring":
						summary.ContractExpiringCount++
					}
				}
			}
			warnings = append(warnings, warning)
		}
	}

	return summary, warnings
}

func (s *OrgService) buildEmployeeWarnings(snapshot orgEmployeeSnapshot, departmentMap map[string]database.Department) []OrgWarningItem {
	departmentName := departmentMap[snapshot.DepartmentID].Name
	now := beginningOfDay(s.nowFn())
	windowEnd := now.AddDate(0, 0, 30)
	warnings := make([]OrgWarningItem, 0, 2)

	if strings.EqualFold(snapshot.Status, "active") {
		regularDate := firstNonEmpty(snapshot.PlannedRegularDate, snapshot.ProbationEndDate)
		if strings.TrimSpace(snapshot.ActualRegularDate) == "" {
			if dueDate, ok := parseDateValue(regularDate); ok && !dueDate.Before(now) && !dueDate.After(windowEnd) {
				warnings = append(warnings, OrgWarningItem{
					Type:           "probation_due",
					Level:          "warning",
					Title:          "转正跟进提醒",
					Description:    "试用期即将到期，请确认转正安排。",
					UserID:         snapshot.UserID,
					UserName:       snapshot.Name,
					DepartmentID:   snapshot.DepartmentID,
					DepartmentName: departmentName,
					DueDate:        dueDate.Format("2006-01-02"),
					DaysLeft:       int(dueDate.Sub(now).Hours() / 24),
				})
			}
		}

		if dueDate, ok := parseDateValue(snapshot.ContractEndDate); ok && !dueDate.Before(now) && !dueDate.After(windowEnd) {
			warnings = append(warnings, OrgWarningItem{
				Type:           "contract_expiring",
				Level:          "warning",
				Title:          "合同到期提醒",
				Description:    "合同将在近期到期，请提前处理续签或离职安排。",
				UserID:         snapshot.UserID,
				UserName:       snapshot.Name,
				DepartmentID:   snapshot.DepartmentID,
				DepartmentName: departmentName,
				DueDate:        dueDate.Format("2006-01-02"),
				DaysLeft:       int(dueDate.Sub(now).Hours() / 24),
			})
		}
	}

	return warnings
}

func (s *OrgService) collectPendingFlowWarnings(departmentIDs []string, departmentMap map[string]database.Department) ([]OrgWarningItem, map[string]int, error) {
	counts := map[string]int{
		"onboarding":  0,
		"transfer":    0,
		"resignation": 0,
	}
	warnings := make([]OrgWarningItem, 0)

	var onboardings []database.EmployeeOnboarding
	onboardingQuery := s.db.Where("deleted_at IS NULL").Where("status IN ?", []string{"pending", "processing"})
	if len(departmentIDs) > 0 {
		onboardingQuery = onboardingQuery.Where("department_id IN ?", departmentIDs)
	}
	if err := onboardingQuery.Order("entry_date ASC, created_at ASC").Find(&onboardings).Error; err != nil {
		return nil, nil, err
	}
	counts["onboarding"] = len(onboardings)
	for _, onboarding := range onboardings {
		warnings = append(warnings, OrgWarningItem{
			Type:           "pending_onboarding",
			Level:          "info",
			Title:          "待完成入职办理",
			Description:    "有员工入职流程仍在处理中。",
			UserName:       onboarding.Name,
			DepartmentID:   onboarding.DepartmentID,
			DepartmentName: onboarding.DepartmentName,
			DueDate:        onboarding.EntryDate,
		})
	}

	var transfers []database.EmployeeTransfer
	transferQuery := s.db.Where("deleted_at IS NULL").Where("status = ?", "pending")
	if len(departmentIDs) > 0 {
		transferQuery = transferQuery.Where("(old_department_id IN ? OR new_department_id IN ?)", departmentIDs, departmentIDs)
	}
	if err := transferQuery.Order("transfer_date ASC, created_at ASC").Find(&transfers).Error; err != nil {
		return nil, nil, err
	}
	counts["transfer"] = len(transfers)
	for _, transfer := range transfers {
		warnings = append(warnings, OrgWarningItem{
			Type:           "pending_transfer",
			Level:          "info",
			Title:          "待处理调岗",
			Description:    strings.TrimSpace(firstNonEmpty(transfer.OldDepartmentName, transfer.OldDepartmentID) + " -> " + firstNonEmpty(transfer.NewDepartmentName, transfer.NewDepartmentID)),
			UserID:         transfer.UserID,
			UserName:       transfer.UserName,
			DepartmentID:   transfer.NewDepartmentID,
			DepartmentName: firstNonEmpty(transfer.NewDepartmentName, transfer.OldDepartmentName),
			DueDate:        transfer.TransferDate,
		})
	}

	var resignations []database.EmployeeResignation
	resignationQuery := s.db.Where("deleted_at IS NULL").Where("status = ?", "pending")
	if len(departmentIDs) > 0 {
		resignationQuery = resignationQuery.Where("department_id IN ?", departmentIDs)
	}
	if err := resignationQuery.Order("resign_date ASC, created_at ASC").Find(&resignations).Error; err != nil {
		return nil, nil, err
	}
	counts["resignation"] = len(resignations)
	for _, resignation := range resignations {
		warnings = append(warnings, OrgWarningItem{
			Type:           "pending_resignation",
			Level:          "warning",
			Title:          "待处理离职",
			Description:    "有员工离职流程待跟进。",
			UserID:         resignation.UserID,
			UserName:       resignation.UserName,
			DepartmentID:   resignation.DepartmentID,
			DepartmentName: firstNonEmpty(resignation.DepartmentName, departmentMap[resignation.DepartmentID].Name),
			DueDate:        resignation.LastWorkingDay,
		})
	}

	return warnings, counts, nil
}

// collectConsecutiveResignationWarnings 检测 30 天内同部门离职 >= 3 人的情况
func (s *OrgService) collectConsecutiveResignationWarnings(departmentIDs []string, departmentMap map[string]database.Department) ([]OrgWarningItem, int, error) {
	since := s.nowFn().AddDate(0, 0, -30)
	var resignations []database.EmployeeResignation
	q := s.db.Where("deleted_at IS NULL").Where("status IN ?", []string{"pending", "approved", "completed"}).
		Where("resign_date >= ?", since.Format("2006-01-02"))
	if len(departmentIDs) > 0 {
		q = q.Where("department_id IN ?", departmentIDs)
	}
	if err := q.Find(&resignations).Error; err != nil {
		return nil, 0, err
	}

	deptCounts := make(map[string]int)
	for _, r := range resignations {
		deptCounts[r.DepartmentID]++
	}

	warnings := make([]OrgWarningItem, 0)
	affectedDepts := 0
	for deptID, count := range deptCounts {
		if count < 3 {
			continue
		}
		affectedDepts++
		warnings = append(warnings, OrgWarningItem{
			Type:           "consecutive_resignation",
			Level:          "error",
			Title:          "连续离职预警",
			Description:    "近30天内该部门已有" + strconv.Itoa(count) + "人离职，请关注团队稳定性。",
			DepartmentID:   deptID,
			DepartmentName: firstNonEmpty(departmentMap[deptID].Name, deptID),
		})
	}
	return warnings, affectedDepts, nil
}

// collectOverspanManagerWarnings 检测直接下属 >= 10 人的管理者
func (s *OrgService) collectOverspanManagerWarnings(departmentIDs []string, departmentMap map[string]database.Department) ([]OrgWarningItem, int, error) {
	var users []database.User
	q := s.db.Where("deleted_at IS NULL").Where("status = ?", "active")
	if len(departmentIDs) > 0 {
		q = q.Where("department_id IN ?", departmentIDs)
	}
	if err := q.Select("user_id, name, department_id, extension").Find(&users).Error; err != nil {
		return nil, 0, err
	}

	managerReportCount := make(map[string]int)
	userMap := make(map[string]*database.User, len(users))
	for i := range users {
		userMap[users[i].UserID] = &users[i]
	}
	for _, u := range users {
		managerID := firstStringValue(u.Extension, "manager_user_id", "leader_user_id", "supervisor_user_id")
		if managerID == "" {
			continue
		}
		managerReportCount[managerID]++
	}

	const overspanThreshold = 10
	warnings := make([]OrgWarningItem, 0)
	affectedManagers := 0
	for managerID, count := range managerReportCount {
		if count < overspanThreshold {
			continue
		}
		affectedManagers++
		manager := userMap[managerID]
		name, deptID := managerID, ""
		if manager != nil {
			name = manager.Name
			deptID = manager.DepartmentID
		}
		warnings = append(warnings, OrgWarningItem{
			Type:           "overspan_manager",
			Level:          "warning",
			Title:          "管理幅度过宽",
			Description:    name + " 当前直接下属 " + strconv.Itoa(count) + " 人，超出建议管理幅度（10人）。",
			UserID:         managerID,
			UserName:       name,
			DepartmentID:   deptID,
			DepartmentName: firstNonEmpty(departmentMap[deptID].Name, deptID),
		})
	}
	return warnings, affectedManagers, nil
}

func (s *OrgService) buildTrendPoints(departmentIDs []string, snapshots []orgEmployeeSnapshot) ([]OrgTrendPoint, error) {
	now := beginningOfDay(s.nowFn())
	months := make([]string, 0, 6)
	points := make(map[string]*OrgTrendPoint, 6)
	for offset := 5; offset >= 0; offset-- {
		monthTime := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -offset, 0)
		monthKey := monthTime.Format("2006-01")
		months = append(months, monthKey)
		points[monthKey] = &OrgTrendPoint{Month: monthKey}
	}

	for _, snapshot := range snapshots {
		if monthKey := normalizeMonth(snapshot.EntryDate); monthKey != "" && points[monthKey] != nil {
			points[monthKey].OnboardingCount++
		}
	}

	var transfers []database.EmployeeTransfer
	transferQuery := s.db.Where("deleted_at IS NULL")
	if len(departmentIDs) > 0 {
		transferQuery = transferQuery.Where("(old_department_id IN ? OR new_department_id IN ?)", departmentIDs, departmentIDs)
	}
	if err := transferQuery.Find(&transfers).Error; err != nil {
		return nil, err
	}
	for _, transfer := range transfers {
		if monthKey := normalizeMonth(transfer.TransferDate); monthKey != "" && points[monthKey] != nil {
			points[monthKey].TransferCount++
		}
	}

	var resignations []database.EmployeeResignation
	resignationQuery := s.db.Where("deleted_at IS NULL")
	if len(departmentIDs) > 0 {
		resignationQuery = resignationQuery.Where("department_id IN ?", departmentIDs)
	}
	if err := resignationQuery.Find(&resignations).Error; err != nil {
		return nil, err
	}
	for _, resignation := range resignations {
		if monthKey := normalizeMonth(resignation.ResignDate); monthKey != "" && points[monthKey] != nil {
			points[monthKey].ResignationCount++
		}
	}

	result := make([]OrgTrendPoint, 0, len(months))
	for _, monthKey := range months {
		result = append(result, *points[monthKey])
	}
	return result, nil
}

func buildDepartmentStats(snapshots []orgEmployeeSnapshot, departmentMap map[string]database.Department) []OrgDepartmentStat {
	statsMap := make(map[string]*OrgDepartmentStat)
	for _, snapshot := range snapshots {
		stat := statsMap[snapshot.DepartmentID]
		if stat == nil {
			department := departmentMap[snapshot.DepartmentID]
			stat = &OrgDepartmentStat{
				DepartmentID:   snapshot.DepartmentID,
				DepartmentName: department.Name,
				ParentID:       department.ParentID,
			}
			statsMap[snapshot.DepartmentID] = stat
		}
		stat.Headcount++
		if strings.EqualFold(snapshot.Status, "active") {
			stat.ActiveCount++
		} else {
			stat.InactiveCount++
		}
	}

	stats := make([]OrgDepartmentStat, 0, len(statsMap))
	for _, stat := range statsMap {
		stats = append(stats, *stat)
	}
	sort.SliceStable(stats, func(i, j int) bool {
		if stats[i].Headcount == stats[j].Headcount {
			return stats[i].DepartmentName < stats[j].DepartmentName
		}
		return stats[i].Headcount > stats[j].Headcount
	})
	return stats
}

func buildDistributionItems(snapshots []orgEmployeeSnapshot, selector func(orgEmployeeSnapshot) string) []OrgDistributionItem {
	counts := make(map[string]int)
	for _, snapshot := range snapshots {
		if !strings.EqualFold(snapshot.Status, "active") {
			continue
		}
		label := strings.TrimSpace(selector(snapshot))
		if label == "" {
			label = "未填写"
		}
		counts[label]++
	}

	items := make([]OrgDistributionItem, 0, len(counts))
	for label, count := range counts {
		items = append(items, OrgDistributionItem{
			Key:   label,
			Label: label,
			Count: count,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Count == items[j].Count {
			return items[i].Label < items[j].Label
		}
		return items[i].Count > items[j].Count
	})
	return items
}

func (s *OrgService) buildOrgRelation(scope *OrgDataScope, user *database.User, departmentMap map[string]database.Department) (EmployeeOrgRelation, error) {
	relation := EmployeeOrgRelation{
		DirectReports: []EmployeeMemberRef{},
	}

	managerUserID := firstStringValue(user.Extension, "manager_user_id", "leader_user_id", "supervisor_user_id")
	if managerUserID != "" {
		var manager database.User
		if err := s.db.Where("user_id = ? AND deleted_at IS NULL", managerUserID).First(&manager).Error; err == nil {
			if scope == nil || scope.IsAll() || scope.AllowsDepartment(manager.DepartmentID) {
				relation.Manager = &EmployeeMemberRef{
					ID:             uintToString(manager.ID),
					UserID:         manager.UserID,
					Name:           manager.Name,
					DepartmentID:   manager.DepartmentID,
					DepartmentName: departmentMap[manager.DepartmentID].Name,
					Position:       manager.Position,
				}
			}
		}
	}

	var sameDepartmentCount int64
	if err := s.baseEmployeeQuery(scopeDepartmentIDs(scope)).
		Where("users.department_id = ?", user.DepartmentID).
		Where("users.user_id <> ?", user.UserID).
		Count(&sameDepartmentCount).Error; err != nil {
		return relation, err
	}
	relation.SameDepartmentCount = int(sameDepartmentCount)

	var users []database.User
	if err := s.baseEmployeeQuery(scopeDepartmentIDs(scope)).
		Select("users.*").
		Find(&users).Error; err != nil {
		return relation, err
	}
	for _, candidate := range users {
		if candidate.UserID == user.UserID {
			continue
		}
		if firstStringValue(candidate.Extension, "manager_user_id", "leader_user_id", "supervisor_user_id") != user.UserID {
			continue
		}
		relation.DirectReports = append(relation.DirectReports, EmployeeMemberRef{
			ID:             uintToString(candidate.ID),
			UserID:         candidate.UserID,
			Name:           candidate.Name,
			DepartmentID:   candidate.DepartmentID,
			DepartmentName: departmentMap[candidate.DepartmentID].Name,
			Position:       candidate.Position,
		})
	}
	sort.SliceStable(relation.DirectReports, func(i, j int) bool {
		return relation.DirectReports[i].Name < relation.DirectReports[j].Name
	})

	return relation, nil
}

func (s *OrgService) buildEmployeeTimeline(user *database.User, profile *database.EmployeeProfile) ([]EmployeeTimelineEvent, error) {
	type timelineItem struct {
		Event  EmployeeTimelineEvent
		SortAt time.Time
	}

	items := make([]timelineItem, 0)
	addDateEvent := func(eventType, title, description, date, status string) {
		if parsed, ok := parseDateValue(date); ok {
			items = append(items, timelineItem{
				Event: EmployeeTimelineEvent{
					Type:        eventType,
					Title:       title,
					Description: description,
					Date:        parsed.Format("2006-01-02"),
					Status:      status,
				},
				SortAt: parsed,
			})
		}
	}

	if profile != nil {
		addDateEvent("entry", "入职", "员工进入组织。", profile.EntryDate, "")
		addDateEvent("regularization_plan", "计划转正", "试用期转正计划时间。", profile.PlannedRegularDate, "planned")
		addDateEvent("regularization_done", "实际转正", "员工已完成转正。", profile.ActualRegularDate, "completed")
		addDateEvent("contract_end", "合同到期", "当前合同结束时间。", profile.ContractEndDate, "")
	}

	var onboardings []database.EmployeeOnboarding
	onboardingQuery := s.db.Where("deleted_at IS NULL")
	if profile != nil && strings.TrimSpace(profile.EmployeeID) != "" {
		onboardingQuery = onboardingQuery.Where("employee_id = ? OR employee_id = ?", profile.EmployeeID, user.UserID)
	} else {
		onboardingQuery = onboardingQuery.Where("employee_id = ?", user.UserID)
	}
	if err := onboardingQuery.Find(&onboardings).Error; err != nil {
		return nil, err
	}
	for _, onboarding := range onboardings {
		addDateEvent("onboarding", "入职办理", "入职流程留痕。", onboarding.EntryDate, onboarding.Status)
	}

	var transfers []database.EmployeeTransfer
	if err := s.db.Where("deleted_at IS NULL").Where("user_id = ?", user.UserID).Find(&transfers).Error; err != nil {
		return nil, err
	}
	for _, transfer := range transfers {
		parsed, ok := parseDateValue(transfer.TransferDate)
		if !ok {
			continue
		}
		description := strings.TrimSpace(transfer.Reason)
		if description == "" {
			description = "员工发生部门或岗位调整。"
		}
		items = append(items, timelineItem{
			Event: EmployeeTimelineEvent{
				Type:        "transfer",
				Title:       "调岗",
				Description: description,
				Date:        parsed.Format("2006-01-02"),
				Status:      transfer.Status,
				From: &EmployeeTimelineEndpoint{
					DepartmentID:   transfer.OldDepartmentID,
					DepartmentName: firstNonEmpty(transfer.OldDepartmentName, transfer.OldDepartmentID),
					Position:       transfer.OldPosition,
				},
				To: &EmployeeTimelineEndpoint{
					DepartmentID:   transfer.NewDepartmentID,
					DepartmentName: firstNonEmpty(transfer.NewDepartmentName, transfer.NewDepartmentID),
					Position:       transfer.NewPosition,
				},
				Reason: strings.TrimSpace(transfer.Reason),
			},
			SortAt: parsed,
		})
	}

	var resignations []database.EmployeeResignation
	if err := s.db.Where("deleted_at IS NULL").Where("user_id = ?", user.UserID).Find(&resignations).Error; err != nil {
		return nil, err
	}
	for _, resignation := range resignations {
		addDateEvent("resignation", "离职", "离职流程留痕。", firstNonEmpty(resignation.ResignDate, resignation.LastWorkingDay), resignation.Status)
	}

	var logs []database.OperationLog
	if err := s.db.Where("resource = ?", "employee_profile:"+user.UserID).
		Order("created_at DESC").
		Limit(20).
		Find(&logs).Error; err != nil {
		return nil, err
	}
	for _, logItem := range logs {
		items = append(items, timelineItem{
			Event: EmployeeTimelineEvent{
				Type:         "audit",
				Title:        mapAuditTitle(logItem.Operation),
				Description:  "关键变更已记录到审计日志。",
				Date:         logItem.CreatedAt.Format("2006-01-02"),
				OperatorName: logItem.UserName,
			},
			SortAt: logItem.CreatedAt,
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].SortAt.After(items[j].SortAt)
	})

	result := make([]EmployeeTimelineEvent, 0, len(items))
	for _, item := range items {
		result = append(result, item.Event)
	}
	return result, nil
}

func buildDepartmentPath(departmentID string, scope *OrgDataScope, departmentMap map[string]database.Department) []EmployeeDepartmentPath {
	path := make([]EmployeeDepartmentPath, 0)
	currentID := strings.TrimSpace(departmentID)
	for currentID != "" {
		department, ok := departmentMap[currentID]
		if !ok {
			break
		}
		path = append(path, EmployeeDepartmentPath{ID: department.DepartmentID, Name: department.Name})
		currentID = department.ParentID
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	if scope == nil || scope.IsAll() {
		return path
	}

	firstVisible := 0
	for firstVisible < len(path) && !scope.AllowsDepartment(path[firstVisible].ID) {
		firstVisible++
	}
	if firstVisible >= len(path) {
		return []EmployeeDepartmentPath{}
	}
	return path[firstVisible:]
}

func rollupTreeCounts(node *OrgDepartmentTreeNode) {
	for _, child := range node.Children {
		rollupTreeCounts(child)
		node.Headcount += child.Headcount
		node.ActiveCount += child.ActiveCount
		node.InactiveCount += child.InactiveCount
	}
}

func sortDepartmentTree(nodes []*OrgDepartmentTreeNode) {
	sort.SliceStable(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
	for _, node := range nodes {
		sortDepartmentTree(node.Children)
	}
}

func newDepartmentChangeLog(departmentID, departmentName, changeType, fieldName, oldValue, newValue, source string, changedAt time.Time) database.DepartmentChangeLog {
	return database.DepartmentChangeLog{
		DepartmentID:   departmentID,
		DepartmentName: departmentName,
		ChangeType:     changeType,
		FieldName:      fieldName,
		OldValue:       oldValue,
		NewValue:       newValue,
		Source:         source,
		ChangedAt:      changedAt,
	}
}

func createDepartmentChangeLog(tx *gorm.DB, departmentID, departmentName, changeType, fieldName, oldValue, newValue, source string, changedAt time.Time) error {
	logItem := newDepartmentChangeLog(departmentID, departmentName, changeType, fieldName, oldValue, newValue, source, changedAt)
	return tx.Create(&logItem).Error
}

func countUniqueDepartments(snapshots []orgEmployeeSnapshot) int {
	seen := make(map[string]struct{}, len(snapshots))
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.DepartmentID) == "" {
			continue
		}
		seen[snapshot.DepartmentID] = struct{}{}
	}
	return len(seen)
}

func scopeDepartmentIDs(scope *OrgDataScope) []string {
	if scope == nil || scope.IsAll() {
		return nil
	}
	if len(scope.DepartmentIDs) == 0 {
		return []string{scopeEmptyDepartmentMarker}
	}
	return append([]string(nil), scope.DepartmentIDs...)
}

func collectDescendantIDs(rootDepartmentID string, childMap map[string][]string) []string {
	queue := []string{rootDepartmentID}
	result := make([]string, 0)
	seen := make(map[string]struct{})

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := seen[current]; ok {
			continue
		}
		seen[current] = struct{}{}
		result = append(result, current)
		queue = append(queue, childMap[current]...)
	}

	return result
}

func departmentNamesByIDs(departmentIDs []string, departmentMap map[string]database.Department) []string {
	names := make([]string, 0, len(departmentIDs))
	for _, departmentID := range uniqueStrings(departmentIDs) {
		if department, ok := departmentMap[departmentID]; ok {
			names = append(names, department.Name)
		}
	}
	return names
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func firstStringValue(source map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := source[key]
		if !ok {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) != "" {
			return strings.TrimSpace(text)
		}
	}
	return ""
}

func firstStringSliceValue(source map[string]interface{}, keys ...string) []string {
	for _, key := range keys {
		value, ok := source[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []string:
			return uniqueStrings(typed)
		case []interface{}:
			result := make([]string, 0, len(typed))
			for _, item := range typed {
				if text, ok := item.(string); ok {
					result = append(result, text)
				}
			}
			return uniqueStrings(result)
		case string:
			if strings.TrimSpace(typed) != "" {
				return []string{strings.TrimSpace(typed)}
			}
		}
	}
	return nil
}

func parseDateValue(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func normalizeMonth(value string) string {
	if parsed, ok := parseDateValue(value); ok {
		return parsed.Format("2006-01")
	}
	return ""
}

func beginningOfDay(value time.Time) time.Time {
	return time.Date(value.Year(), value.Month(), value.Day(), 0, 0, 0, 0, value.Location())
}

func sortWarnings(warnings []OrgWarningItem) {
	sort.SliceStable(warnings, func(i, j int) bool {
		leftDate, leftOK := parseDateValue(warnings[i].DueDate)
		rightDate, rightOK := parseDateValue(warnings[j].DueDate)
		if leftOK && rightOK {
			return leftDate.Before(rightDate)
		}
		if leftOK {
			return true
		}
		if rightOK {
			return false
		}
		return warnings[i].Title < warnings[j].Title
	})
}

func isProbationEmployee(snapshot orgEmployeeSnapshot) bool {
	if strings.TrimSpace(snapshot.ActualRegularDate) != "" {
		return false
	}
	regularDate := firstNonEmpty(snapshot.PlannedRegularDate, snapshot.ProbationEndDate)
	if regularDate == "" {
		return false
	}
	_, ok := parseDateValue(regularDate)
	return ok
}

func snapshotFromEmployee(user *database.User, profile *database.EmployeeProfile) orgEmployeeSnapshot {
	snapshot := orgEmployeeSnapshot{
		ID:           user.ID,
		UserID:       user.UserID,
		Name:         user.Name,
		Email:        user.Email,
		Mobile:       user.Mobile,
		DepartmentID: user.DepartmentID,
		Position:     user.Position,
		Status:       user.Status,
		Avatar:       user.Avatar,
	}
	if profile != nil {
		snapshot.EntryDate = profile.EntryDate
		snapshot.PlannedRegularDate = profile.PlannedRegularDate
		snapshot.ActualRegularDate = profile.ActualRegularDate
		snapshot.ProbationEndDate = profile.ProbationEndDate
		snapshot.ContractEndDate = profile.ContractEndDate
		snapshot.ProfileStatus = profile.ProfileStatus
	}
	return snapshot
}

func mapAuditTitle(operation string) string {
	switch operation {
	case "employee.profile.created":
		return "创建员工档案"
	case "employee.profile.updated":
		return "更新员工档案"
	default:
		return "组织数据变更"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uintToString(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
