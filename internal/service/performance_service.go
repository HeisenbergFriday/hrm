package service

import (
	"errors"
	"fmt"
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
	}
}

type CreateActivityRequest struct {
	Name                 string
	CycleType            string
	StartDate            string
	EndDate              string
	SelfEvalStartAt      string
	SelfEvalEndAt        string
	ManagerEvalStartAt   string
	ManagerEvalEndAt     string
	ResultConfirmStartAt string
	ResultConfirmEndAt   string
	Status               string
	Description          string
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

func (s *PerformanceService) CreateActivity(req struct {
	Name                 string
	CycleType            string
	StartDate            string
	EndDate              string
	SelfEvalStartAt      string
	SelfEvalEndAt        string
	ManagerEvalStartAt   string
	ManagerEvalEndAt     string
	ResultConfirmStartAt string
	ResultConfirmEndAt   string
	Status               string
	TemplateID           *uint
	Description          string
}) (*database.PerformanceActivity, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name 不能为空")
	}
	if strings.TrimSpace(req.CycleType) == "" {
		return nil, errors.New("cycle_type 不能为空")
	}
	activity := &database.PerformanceActivity{
		Name:                 strings.TrimSpace(req.Name),
		CycleType:            strings.TrimSpace(req.CycleType),
		StartDate:            strings.TrimSpace(req.StartDate),
		EndDate:              strings.TrimSpace(req.EndDate),
		SelfEvalStartAt:      strings.TrimSpace(req.SelfEvalStartAt),
		SelfEvalEndAt:        strings.TrimSpace(req.SelfEvalEndAt),
		ManagerEvalStartAt:   strings.TrimSpace(req.ManagerEvalStartAt),
		ManagerEvalEndAt:     strings.TrimSpace(req.ManagerEvalEndAt),
		ResultConfirmStartAt: strings.TrimSpace(req.ResultConfirmStartAt),
		ResultConfirmEndAt:   strings.TrimSpace(req.ResultConfirmEndAt),
		Status:               strings.TrimSpace(req.Status),
		TemplateID:           req.TemplateID,
		Description:          req.Description,
		CreatedBy:            "system",
	}

	if err := s.actRepo.Create(activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (s *PerformanceService) UpdateActivity(activityID string, req struct {
	Name                 string
	CycleType            string
	StartDate            string
	EndDate              string
	SelfEvalStartAt      string
	SelfEvalEndAt        string
	ManagerEvalStartAt   string
	ManagerEvalEndAt     string
	ResultConfirmStartAt string
	ResultConfirmEndAt   string
	Status               string
	TemplateID           *uint
	Description          string
}) (*database.PerformanceActivity, error) {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, err
	}
	activity.Name = strings.TrimSpace(req.Name)
	activity.CycleType = strings.TrimSpace(req.CycleType)
	activity.StartDate = strings.TrimSpace(req.StartDate)
	activity.EndDate = strings.TrimSpace(req.EndDate)
	activity.SelfEvalStartAt = strings.TrimSpace(req.SelfEvalStartAt)
	activity.SelfEvalEndAt = strings.TrimSpace(req.SelfEvalEndAt)
	activity.ManagerEvalStartAt = strings.TrimSpace(req.ManagerEvalStartAt)
	activity.ManagerEvalEndAt = strings.TrimSpace(req.ManagerEvalEndAt)
	activity.ResultConfirmStartAt = strings.TrimSpace(req.ResultConfirmStartAt)
	activity.ResultConfirmEndAt = strings.TrimSpace(req.ResultConfirmEndAt)
	activity.Status = strings.TrimSpace(req.Status)
	activity.TemplateID = req.TemplateID
	activity.Description = req.Description
	activity.UpdatedBy = "system"

	if err := s.actRepo.Update(activity); err != nil {
		return nil, err
	}
	return activity, nil
}

func (s *PerformanceService) GetActivity(activityID string) (*database.PerformanceActivity, error) {
	return s.actRepo.GetByID(activityID)
}

func (s *PerformanceService) PublishActivity(activityID string) error {
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
	if activity.Status != "draft" {
		return errors.New("状态冲突：无法从当前状态 publish 到自评阶段")
	}

	return s.actRepo.UpdateStatus(activityID, "self_evaluation", "system")
}

func (s *PerformanceService) CloseActivity(activityID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return err
	}

	// 幂等：close 旧接口兼容到 archive
	if activity.Status == "archived" {
		return nil
	}
	if activity.Status == "result_confirmed" {
		return s.actRepo.UpdateStatus(activityID, "archived", "system")
	}
	if activity.Status == "draft" || activity.Status == "self_evaluation" || activity.Status == "manager_evaluation" {
		return errors.New("状态冲突：无法从当前状态 close 到归档")
	}

	return errors.New("状态冲突：无法从当前状态 close 到归档")
}

func (s *PerformanceService) ListActivities(page, pageSize int, status, keyword, startDate, endDate string) ([]database.PerformanceActivity, int64, error) {
	return s.actRepo.FindAll(page, pageSize, status, keyword, startDate, endDate)
}

func (s *PerformanceService) GetResultSummary(activityID string) (map[string]interface{}, error) {
	var participants []database.PerformanceParticipant
	if err := s.db.Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&participants).Error; err != nil {
		return nil, err
	}

	summary := map[string]interface{}{
		"total_participants":      len(participants),
		"self_submitted_count":    0,
		"manager_submitted_count": 0,
		"result_confirmed_count":   0,
		"level_distribution":      map[string]int{"S": 0, "A": 0, "B": 0, "C": 0, "D": 0},
	}

	for _, p := range participants {
		if p.SelfSummary != "" || p.SelfScore > 0 {
			summary["self_submitted_count"] = summary["self_submitted_count"].(int) + 1
		}
		if p.ManagerScore > 0 || p.FinalLevel != "" {
			summary["manager_submitted_count"] = summary["manager_submitted_count"].(int) + 1
		}
		if p.Status == "result_confirmed" {
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
	Passed        bool            `json:"passed"`
	TotalCount    int             `json:"total_count"`
	ExceededLevels []LevelExceeded `json:"exceeded_levels"`
	Distribution  map[string]LevelStat `json:"distribution"`
	Warnings      []string        `json:"warnings"`
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
	ratedCount := 0
	for _, p := range participants {
		if p.FinalLevel != "" {
			ratedCount++
			levelCount[p.FinalLevel]++
		}
	}

	result := &DistributionCheckResult{
		Passed:        true,
		TotalCount:    len(participants),
		ExceededLevels: []LevelExceeded{},
		Distribution:  make(map[string]LevelStat),
		Warnings:      []string{},
	}

	allOk := true
	for _, level := range []string{"S", "A", "B", "C", "D"} {
		expectedPct := float64(ruleMap[level])
		expectedCount := 0
		if ratedCount > 0 {
			expectedCount = int(float64(ratedCount) * expectedPct / 100.0)
		}
		actualCount := levelCount[level]
		actualPct := 0.0
		if ratedCount > 0 {
			actualPct = float64(actualCount) / float64(ratedCount) * 100.0
		}
		progress := 0.0
		if expectedCount > 0 {
			progress = float64(actualCount) / float64(expectedCount) * 100.0
		}
		status := "ok"
		if ratedCount > 0 && actualCount > expectedCount {
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
}) ([]database.PerformanceDistributionRule, error) {
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
			Description:         req[0].Description,
			CreatedBy:           "system",
			UpdatedBy:           "system",
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

func (s *PerformanceService) RefreshParticipants(activityID string) (*RefreshResult, error) {
	result := &RefreshResult{}

	// 1. 获取活动信息
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, errors.New("活动不存在")
	}
	_ = activity

	// 2. 获取所有在职员工（从 User 表）
	var users []database.User
	if err := s.db.Where("status = ? AND deleted_at IS NULL", "active").Find(&users).Error; err != nil {
		return nil, err
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
						CreatedBy:     "system",
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
						CreatedBy:     "system",
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
					existing.UpdatedBy = "system"
					if err := tx.Save(existing).Error; err != nil {
						return err
					}
					for _, log := range changeLogs {
						tx.Create(&log)
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
					CreatedBy:      "system",
					UpdatedBy:      "system",
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

		// 标记已离职员工
		activeUserIDs := make(map[string]bool)
		for _, u := range users {
			activeUserIDs[u.UserID] = true
		}
		for i := range txParticipants {
			p := &txParticipants[i]
			if !activeUserIDs[p.EmployeeID] && p.EmployeeStatus == "active" {
				p.EmployeeStatus = "inactive"
				p.Status = "removed_from_scope"
				p.UpdatedBy = "system"
				tx.Save(p)

				tx.Create(&database.PerformanceRelationshipChangeLog{
					ActivityID:    activityID,
					ParticipantID: p.ID,
					ChangeType:    "status_changed",
					FieldName:     "employee_status",
					OldValue:      "active",
					NewValue:      "inactive",
					ChangedAt:     now,
					Source:        "refresh_participants",
					CreatedBy:     "system",
				})
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

func (s *PerformanceService) ListParticipants(activityID string, page, pageSize int, departmentID, managerID, status, employeeKeyword string) ([]database.PerformanceParticipant, int64, error) {
	return s.participantR.FindAll(activityID, page, pageSize, departmentID, managerID, status, employeeKeyword)
}

func (s *PerformanceService) GetParticipant(participantID string) (*database.PerformanceParticipant, error) {
	return s.participantR.GetByID(participantID)
}

func (s *PerformanceService) SubmitSelfEvaluation(participantID string, req struct {
	SelfScore       float64
	SelfLevel       string
	SelfSummary     string
	SelfAttachments []string
}) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.CreateSelfEvaluationVersion(participantID, req.SelfScore, req.SelfLevel, req.SelfSummary, req.SelfAttachments, "system")
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
}) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.CreateManagerEvaluationVersion(participantID, req.ManagerScore, req.SuggestedLevel, req.ManagerComment, req.EvaluationItems, "system")
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
}) ([]database.PerformanceReviewVersion, error) {
	return s.versionRepo.BatchCreateManagerEvaluationVersions(activityID, evaluations, "system")
}

func (s *PerformanceService) AdjustFinalLevel(participantID string, finalLevel, reason string) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.AdjustFinalLevel(participantID, finalLevel, reason, "system")
}

func (s *PerformanceService) ConfirmResult(participantID string, confirmComment string) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.ConfirmResult(participantID, confirmComment, "system")
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

// StartActivity 启动绩效活动（draft -> self_evaluation）
func (s *PerformanceService) StartActivity(activityID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "self_evaluation" {
		return nil
	}
	if activity.Status != "draft" {
		return errors.New("状态冲突：只有 draft 活动可以 start 为自评阶段")
	}
	return s.actRepo.UpdateStatus(activityID, "self_evaluation", "system")
}

// OpenSelfEvaluation 开启自评阶段（draft -> self_evaluation）
func (s *PerformanceService) OpenSelfEvaluation(activityID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "self_evaluation" {
		return nil
	}
	if activity.Status != "draft" {
		return errors.New("状态冲突：只有 draft 活动可以开启自评")
	}
	return s.actRepo.UpdateStatus(activityID, "self_evaluation", "system")
}

// OpenManagerEvaluation 开启主管评分阶段（self_evaluation -> manager_evaluation）
func (s *PerformanceService) OpenManagerEvaluation(activityID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status != "self_evaluation" {
		return errors.New("状态冲突：只有自评阶段活动可以开启主管评分")
	}
	return s.actRepo.UpdateStatus(activityID, "manager_evaluation", "system")
}

// ConfirmResults 确认结果（manager_evaluation -> result_confirmed）
func (s *PerformanceService) ConfirmResults(activityID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status != "manager_evaluation" {
		return errors.New("状态冲突：只有主管评分阶段活动可以确认结果")
	}
	return s.actRepo.UpdateStatus(activityID, "result_confirmed", "system")
}

// ArchiveActivity 归档活动（result_confirmed -> archived）
func (s *PerformanceService) ArchiveActivity(activityID string) error {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return errors.New("活动不存在")
	}
	if activity.Status == "archived" {
		return nil
	}
	if activity.Status != "result_confirmed" {
		return errors.New("状态冲突：只有结果已确认的活动可以归档")
	}
	return s.actRepo.UpdateStatus(activityID, "archived", "system")
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

// CreateTemplate 创建绩效模板
func (s *PerformanceService) CreateTemplate(req struct {
	Name        string
	Description string
	Status      string
	Sections    []struct {
		Name              string
		SectionType       string
		Weight            float64
		SortOrder         int
		IsScoreRequired   bool
		IsCommentRequired bool
		Items             []struct {
			Name        string
			Description string
			MaxScore    float64
			Weight      float64
			SortOrder   int
		}
	}
}, userID string) (*database.PerformanceTemplate, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("模板名称不能为空")
	}
	if len(req.Sections) == 0 {
		return nil, errors.New("至少需要一个评分维度")
	}

	totalWeight := 0.0
	for _, sec := range req.Sections {
		if strings.TrimSpace(sec.Name) == "" {
			return nil, errors.New("section name 不能为空")
		}
		if len(sec.Items) == 0 {
			return nil, errors.New("每个 section 至少需要一个评分项")
		}
		totalWeight += sec.Weight

		itemWeightSum := 0.0
		for _, item := range sec.Items {
			if strings.TrimSpace(item.Name) == "" {
				return nil, errors.New("item name 不能为空")
			}
			if item.MaxScore <= 0 {
				return nil, errors.New("item max_score 必须大于 0")
			}
			if item.Weight < 0 || item.Weight > 100 {
				return nil, errors.New("item weight 必须在 0 到 100 之间")
			}
			itemWeightSum += item.Weight
		}
		if int(itemWeightSum) != 100 {
			return nil, errors.New("同一 section 下 items weight 总和必须等于 100")
		}
	}
	if int(totalWeight) != 100 {
		return nil, errors.New("sections weight 总和必须等于 100")
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

	var sections []database.PerformanceTemplateSection
	var items []database.PerformanceTemplateItem
	var sectionItemCounts []int
	for _, sec := range req.Sections {
		section := database.PerformanceTemplateSection{
			Name:              strings.TrimSpace(sec.Name),
			SectionType:       strings.TrimSpace(sec.SectionType),
			Weight:            sec.Weight,
			SortOrder:         sec.SortOrder,
			IsScoreRequired:   sec.IsScoreRequired,
			IsCommentRequired: sec.IsCommentRequired,
		}
		sections = append(sections, section)
		for _, it := range sec.Items {
			item := database.PerformanceTemplateItem{
				Name:        strings.TrimSpace(it.Name),
				Description: it.Description,
				MaxScore:    it.MaxScore,
				Weight:      it.Weight,
				SortOrder:   it.SortOrder,
			}
			items = append(items, item)
		}
		sectionItemCounts = append(sectionItemCounts, len(sec.Items))
	}

	if err := s.templateRepo.Create(template, sections, items, sectionItemCounts); err != nil {
		return nil, err
	}

	auditSvc := NewAuditService(s.db)
	auditSvc.CreateLog(&database.OperationLog{
		UserID:    userID,
		UserName:  userID,
		Operation: "create_template",
		Resource:  "performance_template:" + template.Name,
		IP:        "",
		Details: map[string]interface{}{
			"template_id":   template.ID,
			"template_name": template.Name,
			"status":        template.Status,
		},
	})

	return template, nil
}

// GetTemplate 获取模板详情
func (s *PerformanceService) GetTemplate(templateID uint) (map[string]interface{}, error) {
	template, sections, items, err := s.templateRepo.GetByID(templateID)
	if err != nil {
		return nil, err
	}

	itemsBySectionID := make(map[uint][]database.PerformanceTemplateItem)
	for _, item := range items {
		itemsBySectionID[item.SectionID] = append(itemsBySectionID[item.SectionID], item)
	}

	var sectionsWithItems []map[string]interface{}
	for _, sec := range sections {
		secMap := map[string]interface{}{
			"id":                  sec.ID,
			"name":                sec.Name,
			"section_type":        sec.SectionType,
			"weight":              sec.Weight,
			"sort_order":          sec.SortOrder,
			"is_score_required":   sec.IsScoreRequired,
			"is_comment_required": sec.IsCommentRequired,
			"items":               itemsBySectionID[sec.ID],
		}
		sectionsWithItems = append(sectionsWithItems, secMap)
	}

	return map[string]interface{}{
		"template": template,
		"sections": sectionsWithItems,
	}, nil
}

// ListTemplates 获取模板列表
func (s *PerformanceService) ListTemplates(page, pageSize int, status string) ([]database.PerformanceTemplate, int64, error) {
	return s.templateRepo.FindAll(page, pageSize, status)
}

// UpdateTemplate 更新模板
func (s *PerformanceService) UpdateTemplate(templateID uint, req struct {
	Name        string
	Description string
	Status      string
	Sections    []struct {
		Name              string
		SectionType       string
		Weight            float64
		SortOrder         int
		IsScoreRequired   bool
		IsCommentRequired bool
		Items             []struct {
			Name        string
			Description string
			MaxScore    float64
			Weight      float64
			SortOrder   int
		}
	}
}, userID string) (*database.PerformanceTemplate, error) {
	template, _, _, err := s.templateRepo.GetByID(templateID)
	if err != nil {
		return nil, errors.New("模板不存在")
	}

	isReferenced, err := s.templateRepo.IsReferencedByActivity(templateID)
	if err != nil {
		return nil, err
	}

	structuralChange := len(req.Sections) > 0
	if isReferenced && structuralChange {
		return nil, errors.New("模板已被活动引用，不允许修改结构")
	}

	template.Name = strings.TrimSpace(req.Name)
	template.Description = req.Description
	template.Status = strings.TrimSpace(req.Status)
	template.UpdatedBy = userID

	var sections []database.PerformanceTemplateSection
	var items []database.PerformanceTemplateItem
	var sectionItemCounts []int

	if structuralChange {
		if strings.TrimSpace(req.Name) == "" {
			return nil, errors.New("模板名称不能为空")
		}
		if len(req.Sections) == 0 {
			return nil, errors.New("至少需要一个评分维度")
		}

		totalWeight := 0.0
		for _, sec := range req.Sections {
			if strings.TrimSpace(sec.Name) == "" {
				return nil, errors.New("section name 不能为空")
			}
			if len(sec.Items) == 0 {
				return nil, errors.New("每个 section 至少需要一个评分项")
			}
			totalWeight += sec.Weight

			itemWeightSum := 0.0
			for _, item := range sec.Items {
				if strings.TrimSpace(item.Name) == "" {
					return nil, errors.New("item name 不能为空")
				}
				if item.MaxScore <= 0 {
					return nil, errors.New("item max_score 必须大于 0")
				}
				if item.Weight < 0 || item.Weight > 100 {
					return nil, errors.New("item weight 必须在 0 到 100 之间")
				}
				itemWeightSum += item.Weight
			}
			if int(itemWeightSum) != 100 {
				return nil, errors.New("同一 section 下 items weight 总和必须等于 100")
			}
		}
		if int(totalWeight) != 100 {
			return nil, errors.New("sections weight 总和必须等于 100")
		}

		for _, sec := range req.Sections {
			section := database.PerformanceTemplateSection{
				Name:              strings.TrimSpace(sec.Name),
				SectionType:       strings.TrimSpace(sec.SectionType),
				Weight:            sec.Weight,
				SortOrder:         sec.SortOrder,
				IsScoreRequired:   sec.IsScoreRequired,
				IsCommentRequired: sec.IsCommentRequired,
			}
			sections = append(sections, section)
			for _, it := range sec.Items {
				item := database.PerformanceTemplateItem{
					Name:        strings.TrimSpace(it.Name),
					Description: it.Description,
					MaxScore:    it.MaxScore,
					Weight:      it.Weight,
					SortOrder:   it.SortOrder,
				}
				items = append(items, item)
			}
			sectionItemCounts = append(sectionItemCounts, len(sec.Items))
		}
	}

	if err := s.templateRepo.Update(template, sections, items, structuralChange, sectionItemCounts); err != nil {
		return nil, err
	}

	auditSvc := NewAuditService(s.db)
	operation := "update_template_metadata"
	if structuralChange {
		operation = "update_template_structure"
	}
	auditSvc.CreateLog(&database.OperationLog{
		UserID:    userID,
		UserName:  userID,
		Operation: operation,
		Resource:  "performance_template:" + template.Name,
		IP:        "",
		Details: map[string]interface{}{
			"template_id":       template.ID,
			"template_name":     template.Name,
			"structural_change": structuralChange,
		},
	})

	return template, nil
}

// BatchConfirmResults 批量确认员工绩效结果
func (s *PerformanceService) BatchConfirmResults(activityID string, participantIDs []uint) ([]map[string]interface{}, error) {
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
		if err := s.confirmResultByID(p.ID, "system"); err != nil {
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
		p.Status = "result_confirmed"
		p.ConfirmedAt = &now
		p.ConfirmedBy = userID
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

// SendSelfEvalReminders 发送自评提醒给未提交的参与者
func (s *PerformanceService) SendSelfEvalReminders(activityID string) error {
	participants, _, err := s.participantR.FindAll(activityID, 1, 1000, "", "", "pending", "")
	if err != nil {
		return err
	}

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
		content := fmt.Sprintf("您有一个绩效自评待完成，请尽快登录系统完成自评。员工：%s", p.EmployeeName)
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
	participants, _, err := s.participantR.FindAll(activityID, 1, 1000, "", "", "self_submitted", "")
	if err != nil {
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
