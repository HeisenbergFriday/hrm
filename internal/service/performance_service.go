package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PerformanceService struct {
	db *gorm.DB

	actRepo      *repository.PerformanceActivityRepository
	ruleRepo     *repository.PerformanceDistributionRuleRepository
	participantR *repository.PerformanceParticipantRepository
	versionRepo  *repository.PerformanceReviewVersionRepository
	changeRepo   *repository.PerformanceRelationshipChangeLogRepository
	templateRepo *repository.PerformanceTemplateRepository
	goalRepo     *repository.PerformanceGoalRecordRepository
	approvalRepo *repository.PerformanceGoalApprovalRepository
}

func NewPerformanceService(db *gorm.DB) *PerformanceService {
	return &PerformanceService{
		db:           db,
		actRepo:      repository.NewPerformanceActivityRepository(db),
		ruleRepo:     repository.NewPerformanceDistributionRuleRepository(db),
		participantR: repository.NewPerformanceParticipantRepository(db),
		versionRepo:  repository.NewPerformanceReviewVersionRepository(db),
		changeRepo:   repository.NewPerformanceRelationshipChangeLogRepository(db),
		templateRepo: repository.NewPerformanceTemplateRepository(db),
		goalRepo:     repository.NewPerformanceGoalRecordRepository(db),
		approvalRepo: repository.NewPerformanceGoalApprovalRepository(db),
	}
}

func (s *PerformanceService) displayNameForUser(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var user database.User
	if err := s.db.Where("id = ?", value).First(&user).Error; err == nil && strings.TrimSpace(user.Name) != "" {
		return strings.TrimSpace(user.Name)
	}
	if err := s.db.Where("user_id = ?", value).First(&user).Error; err == nil && strings.TrimSpace(user.Name) != "" {
		return strings.TrimSpace(user.Name)
	}
	return value
}

type CreateActivityRequest struct {
	Name                   string
	CycleType              string
	StartDate              string
	EndDate                string
	TargetSetStartAt       string
	TargetSetEndAt         string
	SelfEvalStartAt        string
	SelfEvalEndAt          string
	ManagerEvalStartAt     string
	ManagerEvalEndAt       string
	ResultConfirmStartAt   string
	ResultConfirmEndAt     string
	EmployeeConfirmStartAt string
	EmployeeConfirmEndAt   string
	ManagerConfirmStartAt  string
	ManagerConfirmEndAt    string
	HRConfirmStartAt       string
	HRConfirmEndAt         string
	HRConfirmDeadline      string
	Status                 string
	TargetDepartmentIDs    []string
	TargetEmployeeIDs      []string
	IndicatorLibraryID     *uint
	Description            string
	EnableBonusScore       bool
}

func resolveManagerInfo(user database.User) (string, string) {
	managerUserID := strings.TrimSpace(user.ManagerUserID)
	managerName := strings.TrimSpace(user.ManagerName)

	if managerUserID == "" {
		managerUserID = firstNonEmptyString(
			user.Extension,
			"manager_user_id",
			"leader_user_id",
			"supervisor_user_id",
		)
	}
	if managerName == "" {
		managerName = firstNonEmptyString(
			user.Extension,
			"manager_name",
			"leader_name",
			"supervisor_name",
		)
	}

	return managerUserID, managerName
}

func firstNonEmptyString(extension map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		raw, ok := extension[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *PerformanceService) CreateActivity(req CreateActivityRequest, createdBy string) (*database.PerformanceActivity, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name 不能为空")
	}
	cycleType := strings.TrimSpace(req.CycleType)
	if cycleType == "" {
		return nil, errors.New("cycle_type 不能为空")
	}
	if err := s.validateActivityIndicatorLibraryCycle(req.IndicatorLibraryID, cycleType); err != nil {
		return nil, err
	}
	activity := &database.PerformanceActivity{
		Name:                   strings.TrimSpace(req.Name),
		CycleType:              cycleType,
		StartDate:              strings.TrimSpace(req.StartDate),
		EndDate:                strings.TrimSpace(req.EndDate),
		IndicatorLibraryID:     req.IndicatorLibraryID,
		TargetSetStartAt:       strings.TrimSpace(req.TargetSetStartAt),
		TargetSetEndAt:         strings.TrimSpace(req.TargetSetEndAt),
		SelfEvalStartAt:        strings.TrimSpace(req.SelfEvalStartAt),
		SelfEvalEndAt:          strings.TrimSpace(req.SelfEvalEndAt),
		ManagerEvalStartAt:     strings.TrimSpace(req.ManagerEvalStartAt),
		ManagerEvalEndAt:       strings.TrimSpace(req.ManagerEvalEndAt),
		ResultConfirmStartAt:   strings.TrimSpace(req.ResultConfirmStartAt),
		ResultConfirmEndAt:     strings.TrimSpace(req.ResultConfirmEndAt),
		EmployeeConfirmStartAt: strings.TrimSpace(req.EmployeeConfirmStartAt),
		EmployeeConfirmEndAt:   strings.TrimSpace(req.EmployeeConfirmEndAt),
		ManagerConfirmStartAt:  strings.TrimSpace(req.ManagerConfirmStartAt),
		ManagerConfirmEndAt:    strings.TrimSpace(req.ManagerConfirmEndAt),
		HRConfirmStartAt:       strings.TrimSpace(req.HRConfirmStartAt),
		HRConfirmEndAt:         strings.TrimSpace(req.HRConfirmEndAt),
		HRConfirmDeadline:      strings.TrimSpace(req.HRConfirmDeadline),
		Status:                 strings.TrimSpace(req.Status),
		TargetDepartmentIDs:    req.TargetDepartmentIDs,
		TargetEmployeeIDs:      req.TargetEmployeeIDs,
		Description:            req.Description,
		EnableBonusScore:       req.EnableBonusScore,
		CreatedBy:              createdBy,
	}

	if err := s.actRepo.Create(activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (s *PerformanceService) UpdateActivity(activityID string, req CreateActivityRequest, updatedBy string) (*database.PerformanceActivity, error) {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, err
	}
	cycleType := strings.TrimSpace(req.CycleType)
	if cycleType == "" {
		return nil, errors.New("cycle_type 不能为空")
	}
	if err := s.validateActivityIndicatorLibraryCycle(req.IndicatorLibraryID, cycleType); err != nil {
		return nil, err
	}
	activity.Name = strings.TrimSpace(req.Name)
	activity.CycleType = cycleType
	activity.StartDate = strings.TrimSpace(req.StartDate)
	activity.EndDate = strings.TrimSpace(req.EndDate)
	activity.IndicatorLibraryID = req.IndicatorLibraryID
	activity.TargetSetStartAt = strings.TrimSpace(req.TargetSetStartAt)
	activity.TargetSetEndAt = strings.TrimSpace(req.TargetSetEndAt)
	activity.SelfEvalStartAt = strings.TrimSpace(req.SelfEvalStartAt)
	activity.SelfEvalEndAt = strings.TrimSpace(req.SelfEvalEndAt)
	activity.ManagerEvalStartAt = strings.TrimSpace(req.ManagerEvalStartAt)
	activity.ManagerEvalEndAt = strings.TrimSpace(req.ManagerEvalEndAt)
	activity.ResultConfirmStartAt = strings.TrimSpace(req.ResultConfirmStartAt)
	activity.ResultConfirmEndAt = strings.TrimSpace(req.ResultConfirmEndAt)
	activity.EmployeeConfirmStartAt = strings.TrimSpace(req.EmployeeConfirmStartAt)
	activity.EmployeeConfirmEndAt = strings.TrimSpace(req.EmployeeConfirmEndAt)
	activity.ManagerConfirmStartAt = strings.TrimSpace(req.ManagerConfirmStartAt)
	activity.ManagerConfirmEndAt = strings.TrimSpace(req.ManagerConfirmEndAt)
	activity.HRConfirmStartAt = strings.TrimSpace(req.HRConfirmStartAt)
	activity.HRConfirmEndAt = strings.TrimSpace(req.HRConfirmEndAt)
	activity.HRConfirmDeadline = strings.TrimSpace(req.HRConfirmDeadline)
	activity.Status = strings.TrimSpace(req.Status)
	activity.TargetDepartmentIDs = req.TargetDepartmentIDs
	activity.TargetEmployeeIDs = req.TargetEmployeeIDs
	activity.Description = req.Description
	activity.EnableBonusScore = req.EnableBonusScore
	activity.UpdatedBy = updatedBy

	if err := s.actRepo.Update(activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (s *PerformanceService) GetActivity(activityID string) (*database.PerformanceActivity, error) {
	return s.actRepo.GetByID(activityID)
}

func (s *PerformanceService) PublishActivity(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return err
	}

	// 幂等：publish 旧接口兼容到 open-self-evaluation
	if activity.Status == "self_evaluation" {
		return nil
	}
	if activity.Status == "manager_evaluation" || activity.Status == "result_confirmed" || activity.Status == "archived" {
		return errors.New("状态冲突：无法从当前状态 publish 到自评阶段")
	}
	if activity.Status == "target_setting" {
		return s.OpenSelfEvaluation(activityID, userID)
	}
	if activity.Status != "draft" {
		return errors.New("状态冲突：无法从当前状态 publish 到自评阶段")
	}

	// 从 draft 直接开启自评时，也需确保目标设定阶段完成
	if err := s.ensureParticipantStageComplete(activityID, "target_setting"); err != nil {
		return err
	}
	return s.actRepo.UpdateStatus(activityID, "self_evaluation", userID)
}

func (s *PerformanceService) CloseActivity(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return err
	}

	// 幂等：close 旧接口兼容到 archive
	if activity.Status == "archived" {
		return nil
	}
	if activity.Status == "result_confirmed" || activity.Status == "locked" {
		return s.actRepo.UpdateStatus(activityID, "archived", userID)
	}
	if activity.Status == "draft" || activity.Status == "target_setting" || activity.Status == "self_evaluation" || activity.Status == "manager_evaluation" || activity.Status == "employee_confirmation" || activity.Status == "manager_confirmation" || activity.Status == "hr_confirmation" {
		return errors.New("状态冲突：无法从当前状态 close 到归档")
	}

	return errors.New("状态冲突：无法从当前状态 close 到归档")
}

func (s *PerformanceService) ListActivities(page, pageSize int, status, keyword, startDate, endDate string, scope *OrgDataScope) ([]database.PerformanceActivity, int64, error) {
	var departmentIDs []string
	if scope != nil && !scope.IsAll() {
		departmentIDs = scope.DepartmentIDs
	}
	return s.actRepo.FindAll(page, pageSize, status, keyword, startDate, endDate, departmentIDs)
}

func (s *PerformanceService) GetResultSummary(activityID string) (map[string]interface{}, error) {
	var participants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&participants).Error; err != nil {
		return nil, err
	}

	summary := map[string]interface{}{
		"total_participants":       0,
		"target_set_count":         0,
		"self_submitted_count":     0,
		"manager_submitted_count":  0,
		"employee_confirmed_count": 0,
		"manager_confirmed_count":  0,
		"hr_confirmed_count":       0,
		"locked_count":             0,
		"result_confirmed_count":   0,
		"level_distribution":       map[string]int{"S": 0, "A": 0, "B": 0, "C": 0, "D": 0},
	}

	for _, p := range participants {
		if isIgnoredPerformanceParticipantStatus(p.Status) {
			continue
		}
		summary["total_participants"] = summary["total_participants"].(int) + 1
		if participantCompletedStage(p.Status, "target_setting") {
			summary["target_set_count"] = summary["target_set_count"].(int) + 1
		}
		if participantCompletedStage(p.Status, "self_evaluation") || p.SelfSummary != "" || p.SelfScore > 0 {
			summary["self_submitted_count"] = summary["self_submitted_count"].(int) + 1
		}
		if participantCompletedStage(p.Status, "manager_evaluation") || p.ManagerScore > 0 || p.FinalLevel != "" {
			summary["manager_submitted_count"] = summary["manager_submitted_count"].(int) + 1
		}
		if participantCompletedStage(p.Status, "employee_confirmation") || p.EmployeeConfirmedAt != nil {
			summary["employee_confirmed_count"] = summary["employee_confirmed_count"].(int) + 1
		}
		if participantCompletedStage(p.Status, "manager_confirmation") || p.ManagerConfirmedAt != nil {
			summary["manager_confirmed_count"] = summary["manager_confirmed_count"].(int) + 1
		}
		if participantCompletedStage(p.Status, "hr_confirmation") || p.HRConfirmedAt != nil {
			summary["hr_confirmed_count"] = summary["hr_confirmed_count"].(int) + 1
		}
		if p.IsLocked || p.Status == "locked" {
			summary["locked_count"] = summary["locked_count"].(int) + 1
		}
		if p.Status == "result_confirmed" || p.Status == "locked" || p.Status == "hr_confirmed" {
			summary["result_confirmed_count"] = summary["result_confirmed_count"].(int) + 1
		}
		if p.FinalLevel != "" {
			dist := summary["level_distribution"].(map[string]int)
			dist[p.FinalLevel]++
		}
	}

	return summary, nil
}

type DistributionCheckResult struct {
	Passed         bool                 `json:"passed"`
	TotalCount     int                  `json:"total_count"`
	ExceededLevels []LevelExceeded      `json:"exceeded_levels"`
	Distribution   map[string]LevelStat `json:"distribution"`
	Warnings       []string             `json:"warnings"`
}

type LevelExceeded struct {
	Level    string `json:"level"`
	Expected int    `json:"expected"`
	Actual   int    `json:"actual"`
	Excess   int    `json:"excess"`
}

type LevelStat struct {
	ExpectedCount   int     `json:"expected_count"`
	ActualCount     int     `json:"actual_count"`
	ExpectedPercent float64 `json:"expected_percent"`
	ActualPercent   float64 `json:"actual_percent"`
	Progress        float64 `json:"progress"`
	Status          string  `json:"status"` // ok, warning, exceeded
}

type TeamQuotaLevel struct {
	Current int `json:"current"`
	Max     int `json:"max"`
	Percent int `json:"percent"`
}

type TeamQuotaStatus struct {
	ManagerID   string                    `json:"manager_id"`
	ManagerName string                    `json:"manager_name"`
	Total       int                       `json:"total"`
	Levels      map[string]TeamQuotaLevel `json:"levels"`
}

var ignoredPerformanceParticipantStatuses = map[string]struct{}{
	"inactive":           {},
	"removed_from_scope": {},
}

var participantStageStatuses = map[string]map[string]struct{}{
	"target_setting": {
		"target_set":         {},
		"self_submitted":     {},
		"manager_submitted":  {},
		"employee_confirmed": {},
		"manager_confirmed":  {},
		"hr_confirmed":       {},
		"locked":             {},
		"result_confirmed":   {},
	},
	"self_evaluation": {
		"self_submitted":     {},
		"manager_submitted":  {},
		"employee_confirmed": {},
		"manager_confirmed":  {},
		"hr_confirmed":       {},
		"locked":             {},
		"result_confirmed":   {},
	},
	"manager_evaluation": {
		"manager_submitted":  {},
		"employee_confirmed": {},
		"manager_confirmed":  {},
		"hr_confirmed":       {},
		"locked":             {},
		"result_confirmed":   {},
	},
	"employee_confirmation": {
		"employee_confirmed": {},
		"manager_confirmed":  {},
		"hr_confirmed":       {},
		"locked":             {},
		"result_confirmed":   {},
	},
	"manager_confirmation": {
		"manager_confirmed": {},
		"hr_confirmed":      {},
		"locked":            {},
		"result_confirmed":  {},
	},
	"hr_confirmation": {
		"hr_confirmed":     {},
		"locked":           {},
		"result_confirmed": {},
	},
}

func isIgnoredPerformanceParticipantStatus(status string) bool {
	_, ok := ignoredPerformanceParticipantStatuses[status]
	return ok
}

func participantCompletedStage(status, stage string) bool {
	statuses, ok := participantStageStatuses[stage]
	if !ok {
		return false
	}
	_, ok = statuses[status]
	return ok
}

func (s *PerformanceService) GetDistributionCheck(activityID string) (*DistributionCheckResult, error) {
	var participants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&participants).Error; err != nil {
		return nil, err
	}

	rules, err := s.ruleRepo.ListByActivity(activityID)
	if err != nil {
		return nil, err
	}

	// 默认配额比例 15/20/40/10/15 (S/A/B/C/D)
	defaultRules := map[string]int{"S": 15, "A": 20, "B": 40, "C": 10, "D": 15}
	ruleMap := make(map[string]int)
	for _, r := range rules {
		ruleMap[r.Level] = r.DistributionPercent
	}
	for level, pct := range defaultRules {
		if _, ok := ruleMap[level]; !ok {
			ruleMap[level] = pct
		}
	}

	// 统计已评分人员的等级分布
	levelCount := map[string]int{"S": 0, "A": 0, "B": 0, "C": 0, "D": 0}
	activeCount := 0
	for _, p := range participants {
		if isIgnoredPerformanceParticipantStatus(p.Status) {
			continue
		}
		activeCount++
		if p.FinalLevel != "" {
			levelCount[p.FinalLevel]++
		}
	}

	result := &DistributionCheckResult{
		Passed:         true,
		TotalCount:     activeCount,
		ExceededLevels: []LevelExceeded{},
		Distribution:   make(map[string]LevelStat),
		Warnings:       []string{},
	}

	allOk := true
	for _, level := range []string{"S", "A", "B", "C", "D"} {
		expectedPct := float64(ruleMap[level])
		expectedCount := quotaMaxCount(activeCount, ruleMap[level])
		actualCount := levelCount[level]
		actualPct := 0.0
		if activeCount > 0 {
			actualPct = float64(actualCount) / float64(activeCount) * 100.0
		}
		progress := 0.0
		if expectedCount > 0 {
			progress = float64(actualCount) / float64(expectedCount) * 100.0
		}
		status := "ok"
		if activeCount > 0 && actualCount > expectedCount {
			status = "exceeded"
			allOk = false
			result.ExceededLevels = append(result.ExceededLevels, LevelExceeded{
				Level:    level,
				Expected: expectedCount,
				Actual:   actualCount,
				Excess:   actualCount - expectedCount,
			})
		} else if progress >= 80 && progress < 100 {
			status = "warning"
		}

		result.Distribution[level] = LevelStat{
			ExpectedCount:   expectedCount,
			ActualCount:     actualCount,
			ExpectedPercent: expectedPct,
			ActualPercent:   actualPct,
			Progress:        progress,
			Status:          status,
		}
	}

	if !allOk {
		result.Passed = false
		result.Warnings = append(result.Warnings, "部分等级超出配额限制，请调整后再提交")
	}

	return result, nil
}

func (s *PerformanceService) SetDistributionRules(activityID string, req []struct {
	Level               string
	DistributionPercent float64
	Description         string
}, userID string) ([]database.PerformanceDistributionRule, error) {
	if len(req) == 0 {
		return nil, errors.New("rules 不能为空")
	}
	total := 0.0
	seen := make(map[string]struct{})
	for _, r := range req {
		level := strings.TrimSpace(r.Level)
		if level == "" {
			return nil, errors.New("level 不能为空")
		}
		if _, ok := seen[level]; ok {
			return nil, errors.New("同一 activity 下 level 不能重复")
		}
		seen[level] = struct{}{}
		total += r.DistributionPercent
	}
	if total < 99.99 || total > 100.01 {
		return nil, errors.New("distribution_percent 总和必须等于 100")
	}

	levels := make([]database.PerformanceDistributionRule, 0, len(req))
	for _, r := range req {
		levels = append(levels, database.PerformanceDistributionRule{
			ActivityID:          activityID,
			Level:               strings.TrimSpace(r.Level),
			DistributionPercent: int(r.DistributionPercent),
			Description:         r.Description,
			CreatedBy:           userID,
			UpdatedBy:           userID,
		})
	}

	// fix description mapping
	for i := range req {
		levels[i].Description = req[i].Description
	}

	if err := s.ruleRepo.ReplaceForActivity(activityID, levels); err != nil {
		return nil, err
	}
	return s.ruleRepo.ListByActivity(activityID)
}

func (s *PerformanceService) GetDistributionRules(activityID string) ([]database.PerformanceDistributionRule, error) {
	return s.ruleRepo.ListByActivity(activityID)
}

type RefreshResult struct {
	AddedCount    int `json:"added_count"`
	UpdatedCount  int `json:"updated_count"`
	InactiveCount int `json:"inactive_count"`
}

func (s *PerformanceService) RefreshParticipants(activityID, userID string) (*RefreshResult, error) {
	result := &RefreshResult{}

	// 1. 获取活动信息
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, errors.New("活动不存在")
	}

	// 2. 获取所有在职员工（从 User 表）
	var allUsers []database.User
	if err := s.db.Where("status = ? AND deleted_at IS NULL", "active").Find(&allUsers).Error; err != nil {
		return nil, err
	}
	users := make([]database.User, 0, len(allUsers))
	for _, user := range allUsers {
		if activityIncludesUser(activity, user) {
			users = append(users, user)
		}
	}

	// 3. 获取部门信息映射
	var departments []database.Department
	s.db.Where("deleted_at IS NULL").Find(&departments)
	deptMap := make(map[string]database.Department)
	for _, d := range departments {
		deptMap[d.DepartmentID] = d
	}

	// 4. 获取现有参与人（在事务内重新查询并加锁）
	var existingParticipants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&existingParticipants).Error; err != nil {
		return nil, err
	}

	// 5-6. 在事务内执行写操作，防止并发重复创建
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var txParticipants []database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&txParticipants).Error; err != nil {
			return err
		}
		existingMap := make(map[string]*database.PerformanceParticipant)
		for i := range txParticipants {
			existingMap[txParticipants[i].EmployeeID] = &txParticipants[i]
		}

		now := time.Now()
		for _, user := range users {
			dept, hasDept := deptMap[user.DepartmentID]
			deptName := ""
			if hasDept {
				deptName = dept.Name
			}

			existing, exists := existingMap[user.UserID]
			if exists {
				changed := false
				var changeLogs []database.PerformanceRelationshipChangeLog

				if existing.EmployeeStatus != user.Status || existing.Status == "removed_from_scope" || existing.Status == "inactive" {
					changeLogs = append(changeLogs, database.PerformanceRelationshipChangeLog{
						ActivityID:    activityID,
						ParticipantID: existing.ID,
						ChangeType:    "status_changed",
						FieldName:     "employee_status",
						OldValue:      existing.EmployeeStatus,
						NewValue:      user.Status,
						ChangedAt:     now,
						Source:        "refresh_participants",
						CreatedBy:     userID,
					})
					existing.EmployeeStatus = user.Status
					if existing.Status == "removed_from_scope" || existing.Status == "inactive" {
						existing.Status = "pending"
					}
					changed = true
				}

				if existing.DepartmentID != user.DepartmentID {
					changeLogs = append(changeLogs, database.PerformanceRelationshipChangeLog{
						ActivityID:    activityID,
						ParticipantID: existing.ID,
						ChangeType:    "department_changed",
						FieldName:     "department_id",
						OldValue:      existing.DepartmentID,
						NewValue:      user.DepartmentID,
						ChangedAt:     now,
						Source:        "refresh_participants",
						CreatedBy:     userID,
					})
					existing.DepartmentID = user.DepartmentID
					existing.DepartmentName = deptName
					changed = true
				}

				oldManagerID := ""
				if existing.ManagerID != nil {
					oldManagerID = *existing.ManagerID
				}
				newManagerID, managerName := resolveManagerInfo(user)
				if oldManagerID != newManagerID {
					changeLogs = append(changeLogs, database.PerformanceRelationshipChangeLog{
						ActivityID:    activityID,
						ParticipantID: existing.ID,
						ChangeType:    "manager_changed",
						FieldName:     "manager_id",
						OldValue:      oldManagerID,
						NewValue:      newManagerID,
						ChangedAt:     now,
						Source:        "refresh_participants",
						CreatedBy:     userID,
					})
					if newManagerID == "" {
						existing.ManagerID = nil
						existing.ManagerName = nil
					} else {
						existing.ManagerID = &newManagerID
						existing.ManagerName = &managerName
					}
					changed = true
				}

				if changed {
					existing.UpdatedBy = userID
					if err := tx.Save(existing).Error; err != nil {
						return err
					}
					for _, log := range changeLogs {
						if err := tx.Create(&log).Error; err != nil {
							return err
						}
					}
					result.UpdatedCount++
				}
			} else {
				participant := database.PerformanceParticipant{
					ActivityID:     activityID,
					EmployeeID:     user.UserID,
					EmployeeName:   user.Name,
					DepartmentID:   user.DepartmentID,
					DepartmentName: deptName,
					Position:       user.Position,
					EmployeeStatus: user.Status,
					Status:         "pending",
					CreatedBy:      userID,
					UpdatedBy:      userID,
				}
				managerUserID, managerName := resolveManagerInfo(user)
				if managerUserID != "" {
					participant.ManagerID = &managerUserID
					participant.ManagerName = &managerName
				}
				if err := tx.Create(&participant).Error; err != nil {
					return err
				}
				result.AddedCount++
			}
		}

		// 标记离职或不再属于活动范围的员工
		scopedUserIDs := make(map[string]bool)
		for _, u := range users {
			scopedUserIDs[u.UserID] = true
		}
		allActiveUserIDs := make(map[string]bool)
		for _, u := range allUsers {
			allActiveUserIDs[u.UserID] = true
		}
		for i := range txParticipants {
			p := &txParticipants[i]
			if !scopedUserIDs[p.EmployeeID] {
				newEmployeeStatus := p.EmployeeStatus
				changeType := "removed_from_scope"
				if !allActiveUserIDs[p.EmployeeID] {
					newEmployeeStatus = "inactive"
					changeType = "employee_inactive"
				}
				if p.Status == "removed_from_scope" && p.EmployeeStatus == newEmployeeStatus {
					continue
				}
				oldStatus := p.Status
				oldEmployeeStatus := p.EmployeeStatus
				p.EmployeeStatus = newEmployeeStatus
				p.Status = "removed_from_scope"
				p.UpdatedBy = userID
				tx.Save(p)

				if err := tx.Create(&database.PerformanceRelationshipChangeLog{
					ActivityID:    activityID,
					ParticipantID: p.ID,
					ChangeType:    changeType,
					FieldName:     "status",
					OldValue:      fmt.Sprintf("%s/%s", oldStatus, oldEmployeeStatus),
					NewValue:      fmt.Sprintf("%s/%s", p.Status, p.EmployeeStatus),
					ChangedAt:     now,
					Source:        "refresh_participants",
					CreatedBy:     userID,
				}).Error; err != nil {
					return err
				}
				result.InactiveCount++
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

func (s *PerformanceService) ListParticipants(activityID string, page, pageSize int, departmentID, managerID, status, employeeKeyword string, scope *OrgDataScope) ([]database.PerformanceParticipant, int64, error) {
	var visibleDepartmentIDs []string
	if scope != nil && !scope.IsAll() {
		visibleDepartmentIDs = scope.DepartmentIDs
	}
	return s.participantR.FindAll(activityID, page, pageSize, departmentID, managerID, status, employeeKeyword, visibleDepartmentIDs)
}

func (s *PerformanceService) validateActivityIndicatorLibraryCycle(indicatorLibraryID *uint, cycleType string) error {
	if indicatorLibraryID == nil {
		return nil
	}

	var library database.PerformanceIndicatorLibrary
	if err := s.db.Where("id = ? AND deleted_at IS NULL", *indicatorLibraryID).First(&library).Error; err != nil {
		return fmt.Errorf("指标库不存在: %w", err)
	}

	libraryCycle := strings.TrimSpace(library.DefaultCycle)
	if libraryCycle == "" {
		return fmt.Errorf("指标库 %s 未配置默认周期，不能关联到绩效活动", library.Name)
	}
	if libraryCycle != strings.TrimSpace(cycleType) {
		return fmt.Errorf("指标库周期与活动周期不一致：活动周期为 %s，指标库周期为 %s", cycleType, libraryCycle)
	}
	return nil
}

func activityIncludesUser(activity *database.PerformanceActivity, user database.User) bool {
	hasEmployeeScope := false
	for _, employeeID := range activity.TargetEmployeeIDs {
		employeeID = strings.TrimSpace(employeeID)
		if employeeID == "" {
			continue
		}
		hasEmployeeScope = true
		if employeeID == user.UserID {
			return true
		}
	}
	if hasEmployeeScope {
		return false
	}

	hasDepartmentScope := false
	for _, departmentID := range activity.TargetDepartmentIDs {
		departmentID = strings.TrimSpace(departmentID)
		if departmentID == "" {
			continue
		}
		hasDepartmentScope = true
		if departmentID == user.DepartmentID {
			return true
		}
	}
	return !hasDepartmentScope
}

func (s *PerformanceService) GetParticipant(participantID string) (*database.PerformanceParticipant, error) {
	return s.participantR.GetByID(participantID)
}

func (s *PerformanceService) HydrateParticipantTargetConfirmers(participant *database.PerformanceParticipant) {
	if participant == nil || participant.ID == 0 || strings.TrimSpace(participant.ActivityID) == "" {
		return
	}

	logs, err := s.approvalRepo.FindByParticipant(participant.ID, participant.ActivityID)
	if err != nil {
		return
	}
	for _, log := range logs {
		name := strings.TrimSpace(log.ApproverName)
		if name == "" {
			name = s.displayNameForUser(log.ApproverID)
		}
		if name == "" {
			name = s.displayNameForUser(log.CreatedBy)
		}

		switch log.Action {
		case "submit":
			if participant.EmployeeTargetConfirmedAt == nil && !log.CreatedAt.IsZero() {
				confirmedAt := log.CreatedAt
				participant.EmployeeTargetConfirmedAt = &confirmedAt
			}
			if strings.TrimSpace(participant.EmployeeTargetConfirmedBy) == "" {
				participant.EmployeeTargetConfirmedBy = name
			}
		case "approve":
			if participant.ManagerTargetConfirmedAt == nil && !log.CreatedAt.IsZero() {
				confirmedAt := log.CreatedAt
				participant.ManagerTargetConfirmedAt = &confirmedAt
			}
			if strings.TrimSpace(participant.ManagerTargetConfirmedBy) == "" {
				participant.ManagerTargetConfirmedBy = name
			}
		}
	}
}

func (s *PerformanceService) GetRealtimeDistributionCheck(activityID string) ([]TeamQuotaStatus, error) {
	participants, _, err := s.participantR.FindAll(activityID, 1, 5000, "", "", "", "", nil)
	if err != nil {
		return nil, err
	}

	rules, err := s.ruleRepo.ListByActivity(activityID)
	if err != nil {
		return nil, err
	}

	ruleMap := map[string]int{
		"S":  15,
		"A":  20,
		"B":  40,
		"C":  10,
		"D":  15,
		"CD": 25,
	}
	for _, rule := range rules {
		ruleMap[rule.Level] = rule.DistributionPercent
	}
	if _, ok := ruleMap["CD"]; !ok {
		ruleMap["CD"] = ruleMap["C"] + ruleMap["D"]
	}

	teamMap := make(map[string]*TeamQuotaStatus)
	order := make([]string, 0)
	for _, participant := range participants {
		if participant.EmployeeStatus == "inactive" || participant.Status == "removed_from_scope" {
			continue
		}

		managerID := ""
		managerName := ""
		if participant.ManagerID != nil {
			managerID = strings.TrimSpace(*participant.ManagerID)
		}
		if participant.ManagerName != nil {
			managerName = strings.TrimSpace(*participant.ManagerName)
		}

		key := managerID
		team, exists := teamMap[key]
		if !exists {
			team = &TeamQuotaStatus{
				ManagerID:   managerID,
				ManagerName: managerName,
				Levels: map[string]TeamQuotaLevel{
					"S":  {Percent: ruleMap["S"]},
					"A":  {Percent: ruleMap["A"]},
					"B":  {Percent: ruleMap["B"]},
					"CD": {Percent: ruleMap["CD"]},
				},
			}
			teamMap[key] = team
			order = append(order, key)
		}
		if team.ManagerName == "" && managerName != "" {
			team.ManagerName = managerName
		}
		team.Total++

		level := strings.TrimSpace(participant.FinalLevel)
		if level == "" {
			level = strings.TrimSpace(participant.SuggestedLevel)
		}
		switch level {
		case "S", "A", "B":
			stat := team.Levels[level]
			stat.Current++
			team.Levels[level] = stat
		case "C", "D":
			stat := team.Levels["CD"]
			stat.Current++
			team.Levels["CD"] = stat
		}
	}

	teams := make([]TeamQuotaStatus, 0, len(order))
	for _, key := range order {
		team := teamMap[key]
		for level, stat := range team.Levels {
			stat.Max = quotaMaxCount(team.Total, stat.Percent)
			team.Levels[level] = stat
		}
		teams = append(teams, *team)
	}

	sort.SliceStable(teams, func(i, j int) bool {
		if teams[i].ManagerName == teams[j].ManagerName {
			return teams[i].ManagerID < teams[j].ManagerID
		}
		if teams[i].ManagerName == "" {
			return false
		}
		if teams[j].ManagerName == "" {
			return true
		}
		return teams[i].ManagerName < teams[j].ManagerName
	})

	return teams, nil
}

func (s *PerformanceService) SubmitSelfEvaluation(participantID string, req struct {
	SelfScore       float64
	SelfLevel       string
	SelfSummary     string
	SelfAttachments []string
}, userID string) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.CreateSelfEvaluationVersion(participantID, req.SelfScore, req.SelfLevel, req.SelfSummary, req.SelfAttachments, userID)
}

func (s *PerformanceService) SubmitManagerEvaluation(participantID string, req struct {
	ManagerScore    float64
	SuggestedLevel  string
	ManagerComment  string
	EvaluationItems []struct {
		ItemKey   string
		ItemScore float64
		ItemValue string
	}
}, userID string) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.CreateManagerEvaluationVersion(participantID, req.ManagerScore, req.SuggestedLevel, req.ManagerComment, req.EvaluationItems, userID)
}

func (s *PerformanceService) BatchSubmitManagerEvaluations(activityID string, evaluations []struct {
	ParticipantID   uint
	ManagerScore    float64
	SuggestedLevel  string
	ManagerComment  string
	EvaluationItems []struct {
		ItemKey   string
		ItemScore float64
		ItemValue string
	}
}, userID string) ([]database.PerformanceReviewVersion, error) {
	return s.versionRepo.BatchCreateManagerEvaluationVersions(activityID, evaluations, userID)
}

func (s *PerformanceService) AdjustFinalLevel(participantID string, finalLevel, reason, userID string) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.AdjustFinalLevel(participantID, finalLevel, reason, userID)
}

// PerformanceLevelByScore 根据分数计算绩效等级
func PerformanceLevelByScore(score float64) string {
	if score >= 100 {
		return "S"
	}
	if score >= 90 {
		return "A"
	}
	if score >= 80 {
		return "B"
	}
	if score >= 60 {
		return "C"
	}
	return "D"
}

func (s *PerformanceService) ConfirmResult(participantID string, confirmComment, userID string) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.ConfirmResult(participantID, confirmComment, userID)
}

func (s *PerformanceService) GetParticipantVersions(participantID string) ([]database.PerformanceReviewVersion, error) {
	return s.versionRepo.ListByParticipant(participantID)
}

func (s *PerformanceService) GetParticipantRelationshipChangeLogs(participantID string) ([]database.PerformanceRelationshipChangeLog, error) {
	return s.changeRepo.ListByParticipant(participantID)
}

func (s *PerformanceService) GetActivityRelationshipChangeLogs(activityID string) ([]database.PerformanceRelationshipChangeLog, error) {
	return s.changeRepo.ListByActivity(activityID)
}

// StartActivity 启动绩效活动（draft -> target_setting）
func (s *PerformanceService) StartActivity(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "target_setting" {
		return nil
	}
	if activity.Status != "draft" {
		return errors.New("状态冲突：只有 draft 活动可以启动目标设定")
	}
	if _, err := s.RefreshParticipants(activityID, userID); err != nil {
		return err
	}
	total, err := s.countActiveParticipants(activityID)
	if err != nil {
		return err
	}
	if total == 0 {
		return errors.New("活动范围内没有可参与员工，无法启动")
	}
	return s.actRepo.UpdateStatus(activityID, "target_setting", userID)
}

// OpenSelfEvaluation 开启自评阶段（target_setting -> self_evaluation）
func (s *PerformanceService) OpenSelfEvaluation(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "self_evaluation" {
		return nil
	}
	if activity.Status != "target_setting" {
		return errors.New("状态冲突：只有目标设定阶段活动可以开启自评")
	}
	if err := s.ensureParticipantStageComplete(activityID, "target_setting"); err != nil {
		return err
	}
	return s.actRepo.UpdateStatus(activityID, "self_evaluation", userID)
}

// OpenManagerEvaluation 开启主管评分阶段（self_evaluation -> manager_evaluation）
func (s *PerformanceService) OpenManagerEvaluation(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status != "self_evaluation" {
		return errors.New("状态冲突：只有自评阶段活动可以开启主管评分")
	}
	if err := s.ensureParticipantStageComplete(activityID, "self_evaluation"); err != nil {
		return err
	}
	if err := s.actRepo.UpdateStatus(activityID, "manager_evaluation", userID); err != nil {
		return err
	}
	go func() {
		if err := s.SendManagerEvalReminders(activityID); err != nil {
			logrus.Warnf("send manager evaluation reminders after opening manager evaluation failed: %v", err)
		}
	}()
	return nil
}

// ConfirmResults 兼容旧接口：主管评分完成后进入员工确认阶段
func (s *PerformanceService) ConfirmResults(activityID, userID string) error {
	return s.OpenEmployeeConfirmation(activityID, userID)
}

// ArchiveActivity 归档活动（locked/result_confirmed -> archived）
func (s *PerformanceService) ArchiveActivity(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "archived" {
		return nil
	}
	if activity.Status != "locked" && activity.Status != "result_confirmed" {
		return errors.New("状态冲突：只有已锁定或旧版结果已确认的活动可以归档")
	}
	return s.actRepo.UpdateStatus(activityID, "archived", userID)
}

// OpenTargetSetting 开启目标设定阶段（draft -> target_setting）
func (s *PerformanceService) OpenTargetSetting(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "target_setting" {
		return nil
	}
	if activity.Status != "draft" {
		return errors.New("状态冲突：只有 draft 活动可以开启目标设定")
	}
	if _, err := s.RefreshParticipants(activityID, userID); err != nil {
		return err
	}
	total, err := s.countActiveParticipants(activityID)
	if err != nil {
		return err
	}
	if total == 0 {
		return errors.New("活动范围内没有可参与员工，无法开启目标设定")
	}
	return s.actRepo.UpdateStatus(activityID, "target_setting", userID)
}

// OpenEmployeeConfirmation 开启员工确认阶段（manager_evaluation -> employee_confirmation）
func (s *PerformanceService) OpenEmployeeConfirmation(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status != "manager_evaluation" {
		return errors.New("状态冲突：只有主管评分阶段可以开启员工确认")
	}
	if err := s.ensureParticipantStageComplete(activityID, "manager_evaluation"); err != nil {
		return err
	}
	check, err := s.GetDistributionCheck(activityID)
	if err != nil {
		return err
	}
	if !check.Passed {
		return errors.New("强制分布不合规，无法开启员工确认")
	}
	return s.actRepo.UpdateStatus(activityID, "employee_confirmation", userID)
}

// OpenManagerConfirmation 开启主管确认阶段（employee_confirmation -> manager_confirmation）
func (s *PerformanceService) OpenManagerConfirmation(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status != "employee_confirmation" {
		return errors.New("状态冲突：只有员工确认阶段可以开启主管确认")
	}
	if err := s.ensureParticipantStageComplete(activityID, "employee_confirmation"); err != nil {
		return err
	}
	return s.actRepo.UpdateStatus(activityID, "manager_confirmation", userID)
}

// OpenHRConfirmation 开启HR确认阶段（manager_confirmation -> hr_confirmation）
func (s *PerformanceService) OpenHRConfirmation(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status != "manager_confirmation" {
		return errors.New("状态冲突：只有主管确认阶段可以开启HR确认")
	}
	if err := s.ensureParticipantStageComplete(activityID, "manager_confirmation"); err != nil {
		return err
	}
	return s.actRepo.UpdateStatus(activityID, "hr_confirmation", userID)
}

// LockActivity 锁定活动（hr_confirmation -> locked）
func (s *PerformanceService) LockActivity(activityID, userID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "locked" {
		return nil
	}
	if activity.Status != "hr_confirmation" {
		return errors.New("状态冲突：只有HR确认阶段可以锁定活动")
	}
	if err := s.ensureParticipantStageComplete(activityID, "hr_confirmation"); err != nil {
		return err
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		var participants []database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", activityID, ignoredParticipantStatusList()).
			Find(&participants).Error; err != nil {
			return err
		}
		now := time.Now()
		for i := range participants {
			p := &participants[i]
			wasLocked := p.Status == "locked"
			p.Status = "locked"
			p.UpdatedBy = userID
			p.IsLocked = true
			if !wasLocked {
				p.LockedAt = &now
				p.LockedBy = userID
			}
			if err := tx.Save(p).Error; err != nil {
				return err
			}
		}
		return tx.Model(&database.PerformanceActivity{}).
			Where("id = ?", activityID).
			Updates(map[string]interface{}{"status": "locked", "updated_by": userID}).Error
	})
}

func (s *PerformanceService) countActiveParticipants(activityID string) (int64, error) {
	var count int64
	if err := s.db.Model(&database.PerformanceParticipant{}).
		Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", activityID, ignoredParticipantStatusList()).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *PerformanceService) ensureParticipantStageComplete(activityID, stage string) error {
	var participants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&participants).Error; err != nil {
		return err
	}

	activeCount := 0
	incompleteCount := 0
	for _, participant := range participants {
		if isIgnoredPerformanceParticipantStatus(participant.Status) {
			continue
		}
		activeCount++
		if !participantCompletedStage(participant.Status, stage) || !participantHasStageEvidence(participant, stage) {
			incompleteCount++
		}
	}

	if activeCount == 0 {
		return errors.New("活动没有可参与员工，无法推进阶段")
	}
	if incompleteCount > 0 {
		return fmt.Errorf("仍有 %d 名参与人未完成当前阶段，无法推进", incompleteCount)
	}
	return nil
}

func participantHasStageEvidence(participant database.PerformanceParticipant, stage string) bool {
	if participant.Status == "locked" || participant.Status == "result_confirmed" {
		return true
	}
	switch stage {
	case "employee_confirmation":
		return participant.Status != "employee_confirmed" || participant.EmployeeConfirmedAt != nil
	case "manager_confirmation":
		return participant.Status != "manager_confirmed" || participant.ManagerConfirmedAt != nil
	case "hr_confirmation":
		return participant.Status != "hr_confirmed" || participant.HRConfirmedAt != nil
	default:
		return true
	}
}

func ignoredParticipantStatusList() []string {
	statuses := make([]string, 0, len(ignoredPerformanceParticipantStatuses))
	for status := range ignoredPerformanceParticipantStatuses {
		statuses = append(statuses, status)
	}
	return statuses
}

func normalizeTimeOrEmpty(v string) string {
	t := strings.TrimSpace(v)
	if t == "" {
		return ""
	}
	if _, err := time.Parse(time.RFC3339, t); err == nil {
		return t
	}
	return t
}

func sortRulesByLevel(r []database.PerformanceDistributionRule) {
	sort.SliceStable(r, func(i, j int) bool { return r[i].Level < r[j].Level })
}

type PerformanceTemplateItemRequest struct {
	Name        string
	Description string
	MaxScore    float64
	Weight      float64
	SortOrder   int
}

type PerformanceTemplateSectionRequest struct {
	Name              string
	SectionType       string
	Weight            float64
	SortOrder         int
	IsScoreRequired   bool
	IsCommentRequired bool
	Items             []PerformanceTemplateItemRequest
}

type PerformanceTemplateRequest struct {
	Name        string
	Description string
	Status      string
	Sections    []PerformanceTemplateSectionRequest
}

func validateTemplateSections(sections []PerformanceTemplateSectionRequest) error {
	totalWeight := 0.0
	for _, sec := range sections {
		if strings.TrimSpace(sec.Name) == "" {
			return errors.New("section name 不能为空")
		}
		if len(sec.Items) == 0 {
			return errors.New("每个 section 至少需要一个评分项")
		}
		totalWeight += sec.Weight

		itemWeightSum := 0.0
		for _, item := range sec.Items {
			if strings.TrimSpace(item.Name) == "" {
				return errors.New("item name 不能为空")
			}
			if item.MaxScore <= 0 {
				return errors.New("item max_score 必须大于 0")
			}
			if item.Weight < 0 || item.Weight > 100 {
				return errors.New("item weight 必须在 0 到 100 之间")
			}
			itemWeightSum += item.Weight
		}
		if int(itemWeightSum) != 100 {
			return errors.New("同一 section 下 items weight 总和必须等于 100")
		}
	}
	if int(totalWeight) != 100 {
		return errors.New("sections weight 总和必须等于 100")
	}
	return nil
}

func buildTemplateParts(sections []PerformanceTemplateSectionRequest) ([]database.PerformanceTemplateSection, []database.PerformanceTemplateItem, []int) {
	outSections := make([]database.PerformanceTemplateSection, 0, len(sections))
	outItems := make([]database.PerformanceTemplateItem, 0)
	sectionItemCounts := make([]int, 0, len(sections))

	for _, sec := range sections {
		outSections = append(outSections, database.PerformanceTemplateSection{
			Name:              strings.TrimSpace(sec.Name),
			SectionType:       strings.TrimSpace(sec.SectionType),
			Weight:            sec.Weight,
			SortOrder:         sec.SortOrder,
			IsScoreRequired:   sec.IsScoreRequired,
			IsCommentRequired: sec.IsCommentRequired,
		})
		for _, item := range sec.Items {
			outItems = append(outItems, database.PerformanceTemplateItem{
				Name:        strings.TrimSpace(item.Name),
				Description: item.Description,
				MaxScore:    item.MaxScore,
				Weight:      item.Weight,
				SortOrder:   item.SortOrder,
			})
		}
		sectionItemCounts = append(sectionItemCounts, len(sec.Items))
	}

	return outSections, outItems, sectionItemCounts
}

// CreateTemplate 创建绩效模板（兼容旧模板 API）。
func (s *PerformanceService) CreateTemplate(req PerformanceTemplateRequest, userID string) (*database.PerformanceTemplate, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("模板名称不能为空")
	}
	if len(req.Sections) == 0 {
		return nil, errors.New("至少需要一个评分维度")
	}
	if err := validateTemplateSections(req.Sections); err != nil {
		return nil, err
	}

	template := &database.PerformanceTemplate{
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Status:      strings.TrimSpace(req.Status),
		CreatedBy:   userID,
		UpdatedBy:   userID,
	}
	if template.Status == "" {
		template.Status = "draft"
	}

	sections, items, sectionItemCounts := buildTemplateParts(req.Sections)
	if err := s.templateRepo.Create(template, sections, items, sectionItemCounts); err != nil {
		return nil, err
	}

	_ = NewAuditService(s.db).CreateLog(&database.OperationLog{
		UserID:    userID,
		UserName:  userID,
		Operation: "create_template",
		Resource:  "performance_template:" + template.Name,
		Details: map[string]interface{}{
			"template_id":   template.ID,
			"template_name": template.Name,
			"status":        template.Status,
		},
	})

	return template, nil
}

// GetTemplate 获取模板详情（兼容旧模板 API）。
func (s *PerformanceService) GetTemplate(templateID uint) (map[string]interface{}, error) {
	template, sections, items, err := s.templateRepo.GetByID(templateID)
	if err != nil {
		return nil, err
	}

	itemsBySectionID := make(map[uint][]database.PerformanceTemplateItem)
	for _, item := range items {
		itemsBySectionID[item.SectionID] = append(itemsBySectionID[item.SectionID], item)
	}

	sectionsWithItems := make([]map[string]interface{}, 0, len(sections))
	for _, section := range sections {
		sectionsWithItems = append(sectionsWithItems, map[string]interface{}{
			"id":                  section.ID,
			"name":                section.Name,
			"section_type":        section.SectionType,
			"weight":              section.Weight,
			"sort_order":          section.SortOrder,
			"is_score_required":   section.IsScoreRequired,
			"is_comment_required": section.IsCommentRequired,
			"items":               itemsBySectionID[section.ID],
		})
	}

	return map[string]interface{}{
		"template": template,
		"sections": sectionsWithItems,
	}, nil
}

// ListTemplates 获取模板列表（兼容旧模板 API）。
func (s *PerformanceService) ListTemplates(page, pageSize int, status string) ([]database.PerformanceTemplate, int64, error) {
	return s.templateRepo.FindAll(page, pageSize, status)
}

// UpdateTemplate 更新模板（兼容旧模板 API）。
func (s *PerformanceService) UpdateTemplate(templateID uint, req PerformanceTemplateRequest, userID string) (*database.PerformanceTemplate, error) {
	template, _, _, err := s.templateRepo.GetByID(templateID)
	if err != nil {
		return nil, errors.New("模板不存在")
	}
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("模板名称不能为空")
	}

	structuralChange := len(req.Sections) > 0
	if structuralChange {
		isReferenced, err := s.templateRepo.IsReferencedByActivity(templateID)
		if err != nil {
			return nil, err
		}
		if isReferenced {
			return nil, errors.New("模板已被活动引用，不允许修改结构")
		}
		if err := validateTemplateSections(req.Sections); err != nil {
			return nil, err
		}
	}

	template.Name = strings.TrimSpace(req.Name)
	template.Description = req.Description
	template.Status = strings.TrimSpace(req.Status)
	if template.Status == "" {
		template.Status = "draft"
	}
	template.UpdatedBy = userID

	var sections []database.PerformanceTemplateSection
	var items []database.PerformanceTemplateItem
	var sectionItemCounts []int
	if structuralChange {
		sections, items, sectionItemCounts = buildTemplateParts(req.Sections)
	}

	if err := s.templateRepo.Update(template, sections, items, structuralChange, sectionItemCounts); err != nil {
		return nil, err
	}

	operation := "update_template_metadata"
	if structuralChange {
		operation = "update_template_structure"
	}
	_ = NewAuditService(s.db).CreateLog(&database.OperationLog{
		UserID:    userID,
		UserName:  userID,
		Operation: operation,
		Resource:  "performance_template:" + template.Name,
		Details: map[string]interface{}{
			"template_id":       template.ID,
			"template_name":     template.Name,
			"structural_change": structuralChange,
		},
	})

	return template, nil
}

// BatchConfirmResults 批量确认员工绩效结果
func (s *PerformanceService) BatchConfirmResults(activityID string, participantIDs []uint, userID string) ([]map[string]interface{}, error) {
	results := make([]map[string]interface{}, 0, len(participantIDs))
	for _, pid := range participantIDs {
		p, err := s.participantR.GetByID(strconv.FormatUint(uint64(pid), 10))
		if err != nil {
			results = append(results, map[string]interface{}{"participant_id": pid, "success": false, "error": err.Error()})
			continue
		}
		if p.Status != "manager_submitted" {
			results = append(results, map[string]interface{}{"participant_id": pid, "success": false, "error": "状态不是 manager_submitted"})
			continue
		}
		if err := s.confirmResultByID(p.ID, userID); err != nil {
			results = append(results, map[string]interface{}{"participant_id": pid, "success": false, "error": err.Error()})
			continue
		}
		results = append(results, map[string]interface{}{"participant_id": pid, "success": true})
	}
	return results, nil
}

func (s *PerformanceService) confirmResultByID(participantID uint, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return err
		}
		now := time.Now()
		p.Status = "locked"
		p.ConfirmedAt = &now
		p.ConfirmedBy = userID
		p.IsLocked = true
		p.LockedAt = &now
		p.LockedBy = userID
		p.UpdatedBy = userID

		version := &database.PerformanceReviewVersion{
			ParticipantID:  p.ID,
			ActivityID:     p.ActivityID,
			ReviewType:     "confirm_result",
			FinalLevel:     p.FinalLevel,
			ConfirmComment: "",
			ConfirmedAt:    &now,
			CreatedBy:      userID,
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		return tx.Save(p).Error
	})
}

// ConfirmEmployeeResult 员工确认结果
func (s *PerformanceService) ConfirmEmployeeResult(participantID uint, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return errors.New("参与人不存在")
		}
		if p.Status == "employee_confirmed" || p.Status == "manager_confirmed" || p.Status == "hr_confirmed" || p.Status == "locked" {
			return nil
		}
		if p.IsLocked {
			return errors.New("结果已锁定，无法确认")
		}
		var activity database.PerformanceActivity
		if err := tx.Where("id = ? AND deleted_at IS NULL", p.ActivityID).First(&activity).Error; err != nil {
			return errors.New("绩效活动不存在")
		}
		if activity.Status != "employee_confirmation" {
			return errors.New("状态冲突：活动尚未进入员工确认阶段")
		}
		if p.Status != "manager_submitted" && p.Status != "result_confirmed" {
			return errors.New("状态冲突：只有主管评分完成后员工可以确认")
		}
		now := time.Now()
		p.EmployeeConfirmedAt = &now
		p.EmployeeConfirmedBy = userID
		p.Status = "employee_confirmed"
		p.UpdatedBy = userID
		return tx.Save(p).Error
	})
}

// ConfirmManagerResult 主管确认结果并立即锁定
func (s *PerformanceService) ConfirmManagerResult(participantID uint, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return errors.New("参与人不存在")
		}
		if p.Status == "manager_confirmed" || p.Status == "hr_confirmed" || p.Status == "locked" {
			return nil
		}
		if p.IsLocked {
			return errors.New("结果已锁定，无法确认")
		}
		var activity database.PerformanceActivity
		if err := tx.Where("id = ? AND deleted_at IS NULL", p.ActivityID).First(&activity).Error; err != nil {
			return errors.New("绩效活动不存在")
		}
		if activity.Status != "manager_confirmation" {
			return errors.New("状态冲突：活动尚未进入主管确认阶段")
		}
		if p.Status != "employee_confirmed" {
			return errors.New("状态冲突：只有员工确认后主管可以确认")
		}
		now := time.Now()
		p.ManagerConfirmedAt = &now
		p.ManagerConfirmedBy = userID
		p.Status = "manager_confirmed"
		p.UpdatedBy = userID

		// 主管确认后立即锁定结果
		p.IsLocked = true
		p.LockedAt = &now
		p.LockedBy = userID

		// 创建版本记录
		version := &database.PerformanceReviewVersion{
			ParticipantID:  p.ID,
			ActivityID:     p.ActivityID,
			ReviewType:     "confirm_manager",
			FinalLevel:     p.FinalLevel,
			ConfirmComment: "",
			ConfirmedAt:    &now,
			CreatedBy:      userID,
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		return tx.Save(p).Error
	})
}

// ConfirmHRResult HR确认结果
func (s *PerformanceService) ConfirmHRResult(participantID uint, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return errors.New("参与人不存在")
		}
		if p.Status == "hr_confirmed" || p.Status == "locked" {
			return nil
		}
		var activity database.PerformanceActivity
		if err := tx.Where("id = ? AND deleted_at IS NULL", p.ActivityID).First(&activity).Error; err != nil {
			return errors.New("绩效活动不存在")
		}
		if activity.Status != "hr_confirmation" {
			return errors.New("状态冲突：活动尚未进入 HR 确认阶段")
		}
		if p.Status != "manager_confirmed" {
			return errors.New("状态冲突：只有主管确认后HR可以确认")
		}

		// 完度校验：确保前置流程数据完整
		if p.FinalLevel == "" {
			return errors.New("数据不完整：最终等级未设定，无法 HR 确认")
		}
		if p.ManagerScore == 0 {
			var itemCount int64
			tx.Model(&database.PerformanceGoalRecord{}).
				Where("participant_id = ? AND deleted_at IS NULL AND manager_score > 0", p.ID).
				Count(&itemCount)
			if itemCount == 0 {
				return errors.New("数据不完整：主管评分缺失，无法 HR 确认")
			}
		}
		if p.ManagerConfirmedAt == nil {
			return errors.New("数据不完整：主管确认时间缺失，无法 HR 确认")
		}

		now := time.Now()
		p.HRConfirmedAt = &now
		p.HRConfirmedBy = userID
		p.Status = "hr_confirmed"
		p.UpdatedBy = userID

		return tx.Save(p).Error
	})
}

// SendSelfEvalReminders 发送自评提醒给未提交的参与者
func (s *PerformanceService) SendSelfEvalReminders(activityID string) error {
	var activity database.PerformanceActivity
	if err := s.db.Where("id = ? AND deleted_at IS NULL", activityID).First(&activity).Error; err != nil {
		return fmt.Errorf("活动不存在: %v", err)
	}
	var participants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).
		Find(&participants).Error; err != nil {
		return err
	}
	filtered := make([]database.PerformanceParticipant, 0, len(participants))
	for _, participant := range participants {
		if isIgnoredPerformanceParticipantStatus(participant.Status) {
			continue
		}
		if participant.Status == "self_submitted" || participant.Status == "manager_submitted" || participant.Status == "employee_confirmed" || participant.Status == "manager_confirmed" || participant.Status == "hr_confirmed" || participant.Status == "locked" {
			continue
		}
		filtered = append(filtered, participant)
	}
	participants = filtered

	notifiedUsers := make(map[string]struct{})
	var succeeded, failed int
	for _, p := range participants {
		employeeID := strings.TrimSpace(p.EmployeeID)
		if employeeID == "" {
			continue
		}
		if _, exists := notifiedUsers[employeeID]; exists {
			continue
		}
		title := "绩效自评提醒"
		content := fmt.Sprintf("您有一个绩效自评待完成，请尽快登录系统完成自评。绩效活动：%s", activity.Name)
		if err := dingtalk.SendCorpMessageToUser(employeeID, title, content); err != nil {
			logrus.Warnf("send self eval reminder to %s failed: %v", employeeID, err)
			failed++
		} else {
			notifiedUsers[employeeID] = struct{}{}
			succeeded++
		}
	}
	logrus.Infof("sent self eval reminders: succeeded=%d, failed=%d", succeeded, failed)
	return nil
}

// SendManagerEvalReminders 发送主管评分提醒
func (s *PerformanceService) SendManagerEvalReminders(activityID string) error {
	var participants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND deleted_at IS NULL AND status = ?", activityID, "self_submitted").
		Find(&participants).Error; err != nil {
		return err
	}

	managerCounts := make(map[string]int)
	for _, p := range participants {
		if p.ManagerID == nil {
			continue
		}
		managerID := strings.TrimSpace(*p.ManagerID)
		if managerID == "" {
			continue
		}
		managerCounts[managerID]++
	}

	var succeeded, failed int
	for managerID, count := range managerCounts {
		title := "绩效评分提醒"
		content := fmt.Sprintf("您有%d位员工的绩效待评分，请尽快完成。", count)
		if err := dingtalk.SendCorpMessageToUser(managerID, title, content); err != nil {
			logrus.Warnf("send manager eval reminder to %s failed: %v", managerID, err)
			failed++
		} else {
			succeeded++
		}
	}
	logrus.Infof("sent manager eval reminders: succeeded=%d, failed=%d", succeeded, failed)
	return nil
}

// TriggerPerformanceInterview 触发绩效面谈流程
func (s *PerformanceService) TriggerPerformanceInterview(participantID string, interviewType string) error {
	p, err := s.participantR.GetByID(participantID)
	if err != nil {
		return err
	}

	// interviewType: "required" (C/D级) 或 "optional" (A级以上)
	// 这里可以实现创建面谈任务的逻辑
	logrus.Infof("trigger performance interview for participant %s, type=%s, final_level=%s",
		participantID, interviewType, p.FinalLevel)

	// 发送钉钉通知给员工和主管
	if p.ManagerID != nil && *p.ManagerID != "" {
		var content string
		if interviewType == "required" {
			content = fmt.Sprintf("您的绩效等级为%s，需要与主管进行绩效面谈，请联系您的直属主管安排面谈时间。", p.FinalLevel)
		} else {
			content = fmt.Sprintf("恭喜您获得绩效等级%s，主管可以选择与您进行绩效面谈反馈。", p.FinalLevel)
		}
		if err := dingtalk.SendCorpMessageToUser(p.EmployeeID, "绩效面谈通知", content); err != nil {
			logrus.Warnf("send interview notification to employee %s failed: %v", p.EmployeeID, err)
		}
	}

	return nil
}

// ===================== 目标记录管理 =====================

// GoalRecordRequest 目标记录请求
type GoalRecordRequest struct {
	ID             uint     `json:"id"`
	SectionType    string   `json:"section_type" binding:"required"`
	ItemName       string   `json:"item_name" binding:"required"`
	ItemDefinition string   `json:"item_definition"`
	Weight         float64  `json:"weight"`
	RedLineValue   string   `json:"red_line_value"`
	TargetValue    string   `json:"target_value"`
	ChallengeValue string   `json:"challenge_value"`
	ScoringRule    string   `json:"scoring_rule"`
	ActualResult   string   `json:"actual_result"`
	SelfScore      float64  `json:"self_score"`
	ManagerScore   float64  `json:"manager_score"`
	Attachments    []string `json:"attachments"`
	SortOrder      int      `json:"sort_order"`
	IsFromSuperior bool     `json:"is_from_superior"`
}

// GetGoalRecords 获取目标记录列表
func (s *PerformanceService) GetGoalRecords(participantID uint) ([]database.PerformanceGoalRecord, error) {
	return s.goalRepo.FindByParticipant(participantID)
}

// GetGoalRecordsByActivity 获取活动的所有目标记录
func (s *PerformanceService) GetGoalRecordsByActivity(activityID string) ([]database.PerformanceGoalRecord, error) {
	return s.goalRepo.FindByActivity(activityID)
}

// BatchSaveGoalRecords 批量保存目标记录
func (s *PerformanceService) BatchSaveGoalRecords(participantID uint, records []GoalRecordRequest, userID string) ([]database.PerformanceGoalRecord, error) {
	// 获取参与人信息
	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return nil, fmt.Errorf("参与人不存在: %w", err)
	}

	// 检查活动状态是否允许目标设定
	activity, err := s.actRepo.GetByID(participant.ActivityID)
	if err != nil {
		return nil, fmt.Errorf("获取绩效活动失败: %w", err)
	}
	if activity.Status != "target_setting" {
		return nil, fmt.Errorf("当前活动状态不允许设定目标，活动状态为: %s", activity.Status)
	}

	// 权重校验
	if participant.IsLocked {
		return nil, fmt.Errorf("该参与人的绩效结果已锁定，无法修改目标")
	}

	quantitativeWeight := 0.0
	keyActionWeight := 0.0
	normalizedRecords := make([]GoalRecordRequest, 0, len(records))
	for _, r := range records {
		r.Weight = normalizeGoalWeight(r.Weight)
		if r.Weight < 0 || r.Weight > 1 {
			return nil, fmt.Errorf("指标权重必须在 0%% 到 100%% 之间")
		}
		switch r.SectionType {
		case "quantitative":
			quantitativeWeight += r.Weight
		case "key_action":
			keyActionWeight += r.Weight
		}
		normalizedRecords = append(normalizedRecords, r)
	}

	totalWeight := quantitativeWeight + keyActionWeight
	if totalWeight < 0.999 || totalWeight > 1.001 {
		totalWeight = totalWeight * 100
		return nil, fmt.Errorf("量化指标和关键行动权重合计必须等于 100%%，当前为 %.1f%%", totalWeight)
	}

	// 在事务内增量更新目标记录（行锁防并发）
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// 对 participant 加行锁，防止并发写入
		var lockedP database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", participantID).First(&lockedP).Error; err != nil {
			return fmt.Errorf("锁定参与人失败: %w", err)
		}

		// 查询现有记录
		var existing []database.PerformanceGoalRecord
		if err := tx.Where("participant_id = ? AND activity_id = ? AND section_type IN ? AND deleted_at IS NULL",
			participantID, participant.ActivityID, []string{"quantitative", "key_action"}).
			Find(&existing).Error; err != nil {
			return err
		}
		existingMap := make(map[uint]database.PerformanceGoalRecord, len(existing))
		for _, e := range existing {
			existingMap[e.ID] = e
		}

		// 收集前端提交的已有 ID，用于判断哪些需要软删除
		submittedIDs := make(map[uint]bool)
		now := time.Now()

		for i, r := range normalizedRecords {
			sortOrder := r.SortOrder
			if sortOrder == 0 {
				sortOrder = i + 1
			}

			if r.ID > 0 {
				submittedIDs[r.ID] = true
				// 更新已有记录
				attachJSON, _ := json.Marshal(r.Attachments)
				if err := tx.Model(&database.PerformanceGoalRecord{}).Where("id = ? AND deleted_at IS NULL", r.ID).
					Updates(map[string]interface{}{
						"section_type":    r.SectionType,
						"item_name":       r.ItemName,
						"item_definition": r.ItemDefinition,
						"weight":          r.Weight,
						"red_line_value":  r.RedLineValue,
						"target_value":    r.TargetValue,
						"challenge_value": r.ChallengeValue,
						"scoring_rule":    r.ScoringRule,
						"actual_result":   r.ActualResult,
						"attachments":     string(attachJSON),
						"self_score":      r.SelfScore,
						"manager_score":   r.ManagerScore,
						"is_from_superior": r.IsFromSuperior,
						"sort_order":      sortOrder,
						"updated_at":      now,
					}).Error; err != nil {
					return err
				}
			} else {
				// 新增记录
				record := database.PerformanceGoalRecord{
					ActivityID:      participant.ActivityID,
					ParticipantID:   participantID,
					SectionType:     r.SectionType,
					ItemName:        r.ItemName,
					ItemDefinition:  r.ItemDefinition,
					Weight:          r.Weight,
					RedLineValue:    r.RedLineValue,
					TargetValue:     r.TargetValue,
					ChallengeValue:  r.ChallengeValue,
					ScoringRule:     r.ScoringRule,
					ActualResult:    r.ActualResult,
					Attachments:     r.Attachments,
					SelfScore:       r.SelfScore,
					ManagerScore:    r.ManagerScore,
					IsFromSuperior:  r.IsFromSuperior,
					SortOrder:       sortOrder,
					ApprovalStatus:  "pending",
					VisibilityScope: "department_only",
					CreatedAt:       now,
					UpdatedAt:       now,
				}
				if err := tx.Create(&record).Error; err != nil {
					return err
				}
			}
		}

		// 软删除不在提交列表中的旧记录
		for id := range existingMap {
			if !submittedIDs[id] {
				if err := tx.Model(&database.PerformanceGoalRecord{}).Where("id = ?", id).
					Update("deleted_at", now).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return s.goalRepo.FindByParticipant(participantID)
}

// SubmitGoalApproval 提交/审批/驳回目标
func (s *PerformanceService) SubmitGoalApproval(participantID uint, action, comment, userID string) error {
	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return fmt.Errorf("参与人不存在: %w", err)
	}

	// 检查活动状态是否允许目标审批
	activity, err := s.actRepo.GetByID(participant.ActivityID)
	if err != nil {
		return fmt.Errorf("获取绩效活动失败: %w", err)
	}
	if activity.Status != "target_setting" {
		return fmt.Errorf("当前活动状态不允许进行目标审批，活动状态为: %s", activity.Status)
	}

	// 获取最新审批日志
	latestLog, _ := s.approvalRepo.GetLatestByParticipant(participantID, participant.ActivityID)

	if participant.IsLocked {
		return fmt.Errorf("该参与人的绩效结果已锁定，无法更新目标审批")
	}

	var targetStatus string
	var participantStatus string
	switch action {
	case "submit":
		// 员工提交目标
		if latestLog != nil && latestLog.Action == "submit" {
			return fmt.Errorf("目标已提交，请勿重复提交")
		}
		targetStatus = "pending"
		participantStatus = "target_pending_approval"
	case "approve":
		// 上级审批通过
		if latestLog == nil || latestLog.Action != "submit" {
			return fmt.Errorf("目标未提交，无法审批")
		}
		targetStatus = "approved"
		participantStatus = "target_set"
	case "reject":
		// 上级驳回
		if latestLog == nil || latestLog.Action != "submit" {
			return fmt.Errorf("目标未提交，无法驳回")
		}
		targetStatus = "rejected"
		participantStatus = "target_rejected"
	default:
		return fmt.Errorf("无效的操作: %s", action)
	}

	// 更新所有目标记录的审批状态
	now := time.Now()
	displayName := s.displayNameForUser(userID)

	if err := s.db.Model(&database.PerformanceGoalRecord{}).
		Where("participant_id = ? AND activity_id = ?", participantID, participant.ActivityID).
		Update("approval_status", targetStatus).Error; err != nil {
		return err
	}
	if participantStatus != "" {
		participantUpdates := map[string]interface{}{
			"status":     participantStatus,
			"updated_by": userID,
		}
		switch action {
		case "submit":
			participantUpdates["employee_target_confirmed_at"] = now
			participantUpdates["employee_target_confirmed_by"] = displayName
		case "approve":
			participantUpdates["manager_target_confirmed_at"] = now
			participantUpdates["manager_target_confirmed_by"] = displayName
		}
		if err := s.db.Model(&database.PerformanceParticipant{}).
			Where("id = ? AND deleted_at IS NULL", participantID).
			Updates(participantUpdates).Error; err != nil {
			return err
		}
	}

	// 创建审批日志
	approvalLog := &database.PerformanceGoalApprovalLog{
		ParticipantID: participantID,
		ActivityID:    participant.ActivityID,
		Action:        action,
		Comment:       comment,
		ApproverID:    userID,
		ApproverName:  displayName,
		Version:       1,
		CreatedBy:     userID,
	}
	if latestLog != nil {
		approvalLog.Version = latestLog.Version + 1
	}

	return s.approvalRepo.Create(approvalLog)
}

// GetManagerGoals 获取上级下发的目标
func (s *PerformanceService) GetManagerGoals(participantID uint) ([]database.PerformanceGoalRecord, error) {
	records, err := s.goalRepo.FindByParticipant(participantID)
	if err != nil {
		return nil, err
	}

	var managerGoals []database.PerformanceGoalRecord
	for _, r := range records {
		if r.IsFromSuperior {
			managerGoals = append(managerGoals, r)
		}
	}
	return managerGoals, nil
}

// GetGoalSuggestions 获取目标模板建议
func (s *PerformanceService) GetGoalSuggestions(participantID uint) ([]database.PerformanceGoalRecord, error) {
	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return nil, fmt.Errorf("参与人不存在: %w", err)
	}

	// 从同一部门的其他参与人的目标中获取建议
	activity, err := s.actRepo.GetByID(participant.ActivityID)
	if err != nil {
		return nil, err
	}

	libraryIDSet := make(map[uint]struct{})
	libraryIDs := make([]uint, 0)
	if activity.IndicatorLibraryID != nil {
		libraryIDSet[*activity.IndicatorLibraryID] = struct{}{}
		libraryIDs = append(libraryIDs, *activity.IndicatorLibraryID)
	}

	var libraries []database.PerformanceIndicatorLibrary
	if err := s.db.Where("department_id = ? AND status = ? AND deleted_at IS NULL", participant.DepartmentID, "active").
		Order("created_at DESC").
		Find(&libraries).Error; err != nil {
		return nil, err
	}
	for _, library := range libraries {
		if _, exists := libraryIDSet[library.ID]; exists {
			continue
		}
		libraryIDSet[library.ID] = struct{}{}
		libraryIDs = append(libraryIDs, library.ID)
	}
	if len(libraryIDs) == 0 {
		return []database.PerformanceGoalRecord{}, nil
	}

	var indicatorItems []database.PerformanceIndicatorItem
	if err := s.db.Where("library_id IN ? AND deleted_at IS NULL AND section_type IN ?", libraryIDs, []string{"quantitative", "key_action"}).
		Order("is_default DESC, sort_order ASC, created_at ASC").
		Limit(12).
		Find(&indicatorItems).Error; err != nil {
		return nil, err
	}

	suggestions := make([]database.PerformanceGoalRecord, 0, len(indicatorItems))
	for _, item := range indicatorItems {
		weight := item.DefaultWeight
		if weight <= 0 {
			weight = item.Weight
		}
		suggestions = append(suggestions, database.PerformanceGoalRecord{
			IndicatorItemID: &item.ID,
			SectionType:     item.SectionType,
			ItemName:        item.Name,
			ItemDefinition:  item.Description,
			Weight:          normalizeGoalWeight(weight),
			RedLineValue:    item.RedLineValue,
			TargetValue:     item.TargetValue,
			ChallengeValue:  item.ChallengeValue,
			ScoringRule:     item.ScoringRule,
		})
	}

	return suggestions, nil
}

// BatchAssignGoals 批量下发目标给下属
func (s *PerformanceService) BatchAssignGoals(activityID string, managerID string, targets []GoalRecordRequest, participantIDs []uint, userID string) error {
	for _, participantID := range participantIDs {
		// 获取参与人
		participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
		if err != nil {
			logrus.Warnf("participant %d not found, skip", participantID)
			continue
		}

		// 验证参与人的上级是否是当前用户
		if managerID != "" && managerID != "system" {
			if participant.ManagerID == nil || *participant.ManagerID != managerID {
				logrus.Warnf("participant %d's manager is not %s, skip", participantID, managerID)
				continue
			}
		}

		// 批量保存目标记录
		superiorTargets := make([]GoalRecordRequest, 0, len(targets))
		for _, target := range targets {
			target.IsFromSuperior = true
			superiorTargets = append(superiorTargets, target)
		}

		if _, err := s.BatchSaveGoalRecords(participantID, superiorTargets, userID); err != nil {
			logrus.Warnf("save goals for participant %d failed: %v", participantID, err)
			continue
		}
	}

	return nil
}

// SetBonusPenaltyScore 设置附加项分数
func (s *PerformanceService) SetBonusPenaltyScore(participantID uint, bonusScore, penaltyScore float64, userID string) error {
	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return fmt.Errorf("参与人不存在: %w", err)
	}

	// 检查是否已锁定
	if participant.IsLocked {
		return fmt.Errorf("该参与人的绩效结果已锁定，无法修改")
	}

	// 查询活动配置，判断是否启用附加分
	var activity database.PerformanceActivity
	if err := s.db.Where("id = ? AND deleted_at IS NULL", participant.ActivityID).First(&activity).Error; err != nil {
		return fmt.Errorf("绩效活动不存在: %w", err)
	}

	if !activity.EnableBonusScore {
		return fmt.Errorf("该绩效活动未启用附加分，无法设置")
	}

	// 在事务内更新
	return s.db.Transaction(func(tx *gorm.DB) error {
		var p database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return err
		}

		if activity.EnableBonusScore {
			p.BonusScore = bonusScore
			p.PenaltyScore = penaltyScore
		} else {
			p.BonusScore = 0
			p.PenaltyScore = 0
		}
		p.AdjustedScore = p.ManagerScore + p.BonusScore - p.PenaltyScore
		if p.AdjustedScore < 0 {
			p.AdjustedScore = 0
		}
		p.FinalLevel = PerformanceLevelByScore(p.AdjustedScore)
		p.UpdatedBy = userID

		return tx.Save(&p).Error
	})
}

type GoalSelfEvaluationItem struct {
	RecordID     uint    `json:"record_id"`
	ActualResult string  `json:"actual_result"`
	SelfScore    float64 `json:"self_score"`
}

type GoalManagerEvaluationItem struct {
	RecordID     uint    `json:"record_id"`
	ManagerScore float64 `json:"manager_score"`
}

func (s *PerformanceService) SubmitGoalSelfEvaluation(participantID uint, items []GoalSelfEvaluationItem, bonusItems []GoalSelfEvaluationItem, evaluationGood, evaluationImprovement, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var participant database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&participant).Error; err != nil {
			return fmt.Errorf("参与人不存在: %w", err)
		}
		if participant.IsLocked {
			return fmt.Errorf("该参与人的绩效结果已锁定，无法提交自评")
		}

		// 检查活动状态是否允许自评
		var activity database.PerformanceActivity
		if err := tx.Where("id = ? AND deleted_at IS NULL", participant.ActivityID).First(&activity).Error; err != nil {
			return fmt.Errorf("获取绩效活动失败: %w", err)
		}
		if activity.Status != "self_evaluation" {
			return fmt.Errorf("当前活动状态不允许提交自评，活动状态为: %s", activity.Status)
		}

		var records []database.PerformanceGoalRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("participant_id = ? AND activity_id = ? AND deleted_at IS NULL", participantID, participant.ActivityID).
			Find(&records).Error; err != nil {
			return err
		}

		recordMap := make(map[uint]*database.PerformanceGoalRecord)
		for i := range records {
			recordMap[records[i].ID] = &records[i]
		}

		for _, item := range items {
			record, exists := recordMap[item.RecordID]
			if !exists {
				return fmt.Errorf("目标记录 %d 不存在", item.RecordID)
			}
			record.ActualResult = item.ActualResult
			record.SelfScore = item.SelfScore
			record.UpdatedAt = time.Now()
			if err := tx.Save(record).Error; err != nil {
				return err
			}
		}
		for _, item := range bonusItems {
			record, exists := recordMap[item.RecordID]
			if !exists {
				return fmt.Errorf("附加项记录 %d 不存在", item.RecordID)
			}
			record.SelfScore = item.SelfScore
			record.UpdatedAt = time.Now()
			if err := tx.Save(record).Error; err != nil {
				return err
			}
		}

		totalSelfScore := 0.0
		for _, record := range recordMap {
			if record.SectionType == "bonus_penalty" {
				continue
			}
			totalSelfScore += record.SelfScore * record.Weight
		}
		totalSelfScore = roundScore(totalSelfScore)
		participant.SelfScore = totalSelfScore
		participant.TotalSelfScore = totalSelfScore
		participant.SelfSummary = strings.TrimSpace(strings.Join([]string{evaluationGood, evaluationImprovement}, "\n"))
		participant.SelfEvaluationGood = strings.TrimSpace(evaluationGood)
		participant.SelfEvaluationImprovement = strings.TrimSpace(evaluationImprovement)
		participant.Status = "self_submitted"
		participant.UpdatedBy = userID
		if err := tx.Save(&participant).Error; err != nil {
			return err
		}

		version := &database.PerformanceReviewVersion{
			ParticipantID: participant.ID,
			ActivityID:    participant.ActivityID,
			ReviewType:    "self",
			SelfScore:     totalSelfScore,
			SelfSummary:   participant.SelfSummary,
			CreatedBy:     userID,
			OperationMeta: map[string]interface{}{
				"evaluation_good":        participant.SelfEvaluationGood,
				"evaluation_improvement": participant.SelfEvaluationImprovement,
				"goal_item_count":        len(items),
				"bonus_item_count":       len(bonusItems),
			},
		}
		return tx.Create(version).Error
	})
}

func (s *PerformanceService) SubmitGoalManagerEvaluation(participantID uint, items []GoalManagerEvaluationItem, bonusItems []GoalManagerEvaluationItem, suggestedLevel, evaluationGood, evaluationImprovement, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var participant database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", participantID).First(&participant).Error; err != nil {
			return fmt.Errorf("参与人不存在: %w", err)
		}
		if participant.IsLocked {
			return fmt.Errorf("该参与人的绩效结果已锁定，无法提交上级评分")
		}

		var activity database.PerformanceActivity
		if err := tx.Where("id = ? AND deleted_at IS NULL", participant.ActivityID).First(&activity).Error; err != nil {
			return fmt.Errorf("绩效活动不存在: %w", err)
		}

		// 检查活动状态是否允许主管评分
		if activity.Status != "manager_evaluation" {
			return fmt.Errorf("当前活动状态不允许提交主管评分，活动状态为: %s", activity.Status)
		}

		var records []database.PerformanceGoalRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("participant_id = ? AND activity_id = ? AND deleted_at IS NULL", participantID, participant.ActivityID).
			Find(&records).Error; err != nil {
			return err
		}

		recordMap := make(map[uint]*database.PerformanceGoalRecord)
		for i := range records {
			recordMap[records[i].ID] = &records[i]
		}

		for _, item := range items {
			record, exists := recordMap[item.RecordID]
			if !exists {
				return fmt.Errorf("目标记录 %d 不存在", item.RecordID)
			}
			record.ManagerScore = item.ManagerScore
			record.UpdatedAt = time.Now()
			if err := tx.Save(record).Error; err != nil {
				return err
			}
		}

		bonusTotal := 0.0
		for _, item := range bonusItems {
			record, exists := recordMap[item.RecordID]
			if !exists {
				return fmt.Errorf("附加项记录 %d 不存在", item.RecordID)
			}
			record.ManagerScore = item.ManagerScore
			record.BonusScore = item.ManagerScore
			record.UpdatedAt = time.Now()
			if err := tx.Save(record).Error; err != nil {
				return err
			}
			bonusTotal += item.ManagerScore
		}

		totalManagerScore := 0.0
		for _, record := range recordMap {
			if record.SectionType == "bonus_penalty" {
				continue
			}
			totalManagerScore += record.ManagerScore * record.Weight
		}
		totalManagerScore = roundScore(totalManagerScore)

		if activity.EnableBonusScore {
			participant.BonusScore = roundScore(bonusTotal)
		}

		adjustedScore := totalManagerScore + participant.BonusScore - participant.PenaltyScore
		if adjustedScore < 0 {
			adjustedScore = 0
		}

		autoLevel := PerformanceLevelByScore(totalManagerScore)
		if activity.EnableBonusScore {
			autoLevel = PerformanceLevelByScore(adjustedScore)
		}
		if strings.TrimSpace(suggestedLevel) != "" {
			autoLevel = strings.TrimSpace(suggestedLevel)
		}

		participant.ManagerScore = totalManagerScore
		participant.TotalManagerScore = totalManagerScore
		participant.AdjustedScore = roundScore(adjustedScore)
		participant.SuggestedLevel = autoLevel
		if participant.FinalLevel == "" || participant.FinalLevel == participant.SuggestedLevel || participant.AdjustReason == "" {
			participant.FinalLevel = autoLevel
		}
		participant.ManagerComment = strings.TrimSpace(strings.Join([]string{evaluationGood, evaluationImprovement}, "\n"))
		participant.ManagerEvaluationGood = strings.TrimSpace(evaluationGood)
		participant.ManagerEvaluationImprovement = strings.TrimSpace(evaluationImprovement)
		participant.Status = "manager_submitted"
		participant.UpdatedBy = userID
		if err := tx.Save(&participant).Error; err != nil {
			return err
		}

		version := &database.PerformanceReviewVersion{
			ParticipantID:  participant.ID,
			ActivityID:     participant.ActivityID,
			ReviewType:     "manager",
			ManagerScore:   totalManagerScore,
			SuggestedLevel: participant.SuggestedLevel,
			ManagerComment: participant.ManagerComment,
			FinalLevel:     participant.FinalLevel,
			CreatedBy:      userID,
			OperationMeta: map[string]interface{}{
				"evaluation_good":        participant.ManagerEvaluationGood,
				"evaluation_improvement": participant.ManagerEvaluationImprovement,
				"goal_item_count":        len(items),
				"bonus_item_count":       len(bonusItems),
				"adjusted_score":         participant.AdjustedScore,
				"bonus_score":            participant.BonusScore,
			},
		}
		return tx.Create(version).Error
	})
}

func (s *PerformanceService) SetCompanyFinance(activityID, revenueSign, description, remark, userID string) (*database.PerformanceCompanyFinance, error) {
	var finance database.PerformanceCompanyFinance
	err := s.db.Where("activity_id = ?", activityID).First(&finance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		finance = database.PerformanceCompanyFinance{
			ActivityID:  activityID,
			RevenueSign: strings.TrimSpace(revenueSign),
			Description: description,
			SetBy:       userID,
			SetAt:       time.Now(),
			Remark:      remark,
			CreatedBy:   userID,
			UpdatedBy:   userID,
		}
		if finance.RevenueSign == "" {
			finance.RevenueSign = "equal"
		}
		if createErr := s.db.Create(&finance).Error; createErr != nil {
			return nil, createErr
		}
		return &finance, nil
	}
	if err != nil {
		return nil, err
	}

	finance.RevenueSign = strings.TrimSpace(revenueSign)
	if finance.RevenueSign == "" {
		finance.RevenueSign = "equal"
	}
	finance.Description = description
	finance.Remark = remark
	finance.SetBy = userID
	finance.SetAt = time.Now()
	finance.UpdatedBy = userID
	if err := s.db.Save(&finance).Error; err != nil {
		return nil, err
	}
	return &finance, nil
}

func (s *PerformanceService) GetCompanyFinance(activityID string) (*database.PerformanceCompanyFinance, error) {
	var finance database.PerformanceCompanyFinance
	if err := s.db.Where("activity_id = ?", activityID).First(&finance).Error; err != nil {
		return nil, err
	}
	return &finance, nil
}

func (s *PerformanceService) GetPendingHRConfirm(activityID string) ([]database.PerformanceParticipant, error) {
	var participants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND status = ? AND deleted_at IS NULL", activityID, "manager_confirmed").
		Order("department_name ASC, employee_name ASC").
		Find(&participants).Error; err != nil {
		return nil, err
	}
	return participants, nil
}

func (s *PerformanceService) SetHRConfirmDeadline(activityID, deadline, userID string) (*database.PerformanceActivity, error) {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, err
	}
	activity.HRConfirmDeadline = strings.TrimSpace(deadline)
	activity.UpdatedBy = userID
	if err := s.actRepo.Update(activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (s *PerformanceService) GetHRConfirmDeadlineStatus(activityID string) (map[string]interface{}, error) {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, err
	}
	pending, err := s.GetPendingHRConfirm(activityID)
	if err != nil {
		return nil, err
	}

	status := map[string]interface{}{
		"deadline":       activity.HRConfirmDeadline,
		"pending_count":  len(pending),
		"overdue":        false,
		"can_force_lock": false,
	}
	if activity.HRConfirmDeadline != "" {
		if deadlineTime, parseErr := time.Parse("2006-01-02", activity.HRConfirmDeadline); parseErr == nil {
			status["overdue"] = time.Now().After(deadlineTime.Add(24 * time.Hour))
		}
	}
	status["can_force_lock"] = activity.Status == "hr_confirmation" && status["overdue"].(bool) && len(pending) > 0
	return status, nil
}

func (s *PerformanceService) ForceLockOverdueHRConfirmation(activityID, userID string) (map[string]interface{}, error) {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, errors.New("活动不存在")
	}
	if activity.Status == "locked" {
		return map[string]interface{}{
			"force_locked_count":   0,
			"locked_count":         0,
			"already_locked_count": 0,
			"total_count":          0,
		}, nil
	}
	if activity.Status != "hr_confirmation" {
		return nil, errors.New("状态冲突：只有 HR 确认阶段可以执行逾期强制锁定")
	}

	deadline := strings.TrimSpace(activity.HRConfirmDeadline)
	if deadline == "" {
		return nil, errors.New("未设置 HR 确认截止日期，无法执行逾期强制锁定")
	}
	deadlineTime, parseErr := time.Parse("2006-01-02", deadline)
	if parseErr != nil {
		return nil, errors.New("HR 确认截止日期格式错误")
	}
	if !time.Now().After(deadlineTime.Add(24 * time.Hour)) {
		return nil, errors.New("HR 确认截止日期尚未逾期，无法执行强制锁定")
	}

	var result map[string]interface{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var participants []database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", activityID, ignoredParticipantStatusList()).
			Order("id ASC").
			Find(&participants).Error; err != nil {
			return err
		}
		if len(participants) == 0 {
			return errors.New("活动没有可参与员工，无法锁定")
		}

		incompleteCount := 0
		for _, participant := range participants {
			switch participant.Status {
			case "manager_confirmed", "hr_confirmed", "locked", "result_confirmed":
			default:
				incompleteCount++
			}
		}
		if incompleteCount > 0 {
			return fmt.Errorf("仍有 %d 名参与人未完成主管确认或 HR 确认，无法逾期强制锁定", incompleteCount)
		}

		now := time.Now()
		reason := fmt.Sprintf("HR 确认逾期强制锁定，截止日期：%s", deadline)
		forceLockedCount := 0
		lockedCount := 0
		alreadyLockedCount := 0
		for i := range participants {
			p := &participants[i]
			wasLocked := p.Status == "locked"
			if p.Status == "manager_confirmed" {
				p.ForceLocked = true
				p.ForceLockedReason = reason
				forceLockedCount++
			} else if p.Status == "locked" {
				alreadyLockedCount++
			}
			if !wasLocked {
				lockedCount++
			}
			p.Status = "locked"
			p.IsLocked = true
			if !wasLocked {
				p.LockedAt = &now
				p.LockedBy = userID
			}
			p.UpdatedBy = userID
			if err := tx.Save(p).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&database.PerformanceActivity{}).
			Where("id = ?", activityID).
			Updates(map[string]interface{}{"status": "locked", "updated_by": userID}).Error; err != nil {
			return err
		}
		result = map[string]interface{}{
			"force_locked_count":   forceLockedCount,
			"locked_count":         lockedCount,
			"already_locked_count": alreadyLockedCount,
			"total_count":          len(participants),
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *PerformanceService) SendHRConfirmReminders(activityID string) error {
	pending, err := s.GetPendingHRConfirm(activityID)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}

	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return err
	}
	recipient := strings.TrimSpace(activity.CreatedBy)
	if recipient == "" || recipient == "system" {
		return nil
	}

	title := "绩效 HR 确认提醒"
	content := fmt.Sprintf("活动：%s\n当前仍有 %d 名员工待 HR 确认，请及时处理。", activity.Name, len(pending))
	return dingtalk.SendCorpMessageToUser(recipient, title, content)
}

func normalizeGoalWeight(weight float64) float64 {
	if weight <= 0 {
		return 0
	}
	if weight > 1.0001 {
		return weight / 100
	}
	return weight
}

func quotaMaxCount(total int, percent int) int {
	if total <= 0 || percent <= 0 {
		return 0
	}
	return int(math.Ceil(float64(total) * float64(percent) / 100))
}

func roundScore(score float64) float64 {
	return math.Round(score*100) / 100
}
