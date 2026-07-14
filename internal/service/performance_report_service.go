package service

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"math"
	"peopleops/internal/database"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"
)

const (
	PerformanceReportTypeAll      = "all"
	PerformanceReportTypeProgress = "progress"
	PerformanceReportTypeContent  = "content"
	PerformanceReportTypeResult   = "result"
)

type PerformanceReportService struct {
	db *gorm.DB
}

func NewPerformanceReportService(db *gorm.DB) *PerformanceReportService {
	return &PerformanceReportService{db: db}
}

type PerformanceReportFilter struct {
	CompanyID       string
	DepartmentID    string
	Status          string
	Level           string
	EmployeeKeyword string
	Scope           *OrgDataScope
	Access          PerformanceReportAccess
}

type PerformanceReportAccess struct {
	IdentityValues map[string]struct{}
	Privileged     bool
}

type PerformanceReport struct {
	Activity  database.PerformanceActivity `json:"activity"`
	IsNewFlow bool                         `json:"is_new_flow"`
	Progress  PerformanceProgressReport    `json:"progress"`
	Content   PerformanceContentReport     `json:"content"`
	Result    PerformanceResultReport      `json:"result"`
}

type PerformanceProgressReport struct {
	Summary            PerformanceProgressSummary `json:"summary"`
	Rows               []PerformanceProgressRow   `json:"rows"`
	StatusDistribution []PerformanceChartItem     `json:"status_distribution"`
}

type PerformanceProgressSummary struct {
	TotalParticipants       int     `json:"total_participants"`
	TargetSubmittedCount    int     `json:"target_submitted_count"`
	SelfSubmittedCount      int     `json:"self_submitted_count"`
	ManagerSubmittedCount   int     `json:"manager_submitted_count"`
	DepartmentReviewedCount int     `json:"department_reviewed_count"`
	HRConfirmedCount        int     `json:"hr_confirmed_count"`
	LockedCount             int     `json:"locked_count"`
	CompletionRate          float64 `json:"completion_rate"`
}

type PerformanceProgressRow struct {
	ParticipantID      uint    `json:"participant_id"`
	EmployeeID         string  `json:"employee_id"`
	EmployeeName       string  `json:"employee_name"`
	DepartmentID       string  `json:"department_id"`
	DepartmentName     string  `json:"department_name"`
	Position           string  `json:"position"`
	ManagerID          string  `json:"manager_id"`
	ManagerName        string  `json:"manager_name"`
	Status             string  `json:"status"`
	TargetSubmitted    bool    `json:"target_submitted"`
	SelfSubmitted      bool    `json:"self_submitted"`
	ManagerSubmitted   bool    `json:"manager_submitted"`
	DepartmentReviewed bool    `json:"department_reviewed"`
	HRConfirmed        bool    `json:"hr_confirmed"`
	Locked             bool    `json:"locked"`
	ProgressRate       float64 `json:"progress_rate"`
	ResultHidden       bool    `json:"result_hidden"`
}

type PerformanceContentReport struct {
	Summary PerformanceContentSummary `json:"summary"`
	Rows    []PerformanceContentRow   `json:"rows"`
	Phases  []PerformanceChartItem    `json:"phases"`
}

type PerformanceContentSummary struct {
	ReviewItemCount        int     `json:"review_item_count"`
	PlanItemCount          int     `json:"plan_item_count"`
	ParticipantsWithReview int     `json:"participants_with_review"`
	ParticipantsWithPlan   int     `json:"participants_with_plan"`
	AverageCompletionRate  float64 `json:"average_completion_rate"`
}

type PerformanceContentRow struct {
	ID               uint    `json:"id"`
	ParticipantID    uint    `json:"participant_id"`
	EmployeeID       string  `json:"employee_id"`
	EmployeeName     string  `json:"employee_name"`
	DepartmentID     string  `json:"department_id"`
	DepartmentName   string  `json:"department_name"`
	Position         string  `json:"position"`
	ManagerName      string  `json:"manager_name"`
	GoalPhase        string  `json:"goal_phase"`
	GoalPhaseLabel   string  `json:"goal_phase_label"`
	SectionType      string  `json:"section_type"`
	GoalType         string  `json:"goal_type"`
	ItemName         string  `json:"item_name"`
	ItemDefinition   string  `json:"item_definition"`
	Weight           float64 `json:"weight"`
	TargetValue      string  `json:"target_value"`
	ChallengeValue   string  `json:"challenge_value"`
	MetricUnit       string  `json:"metric_unit"`
	CompletionRate   float64 `json:"completion_rate"`
	ActualResult     string  `json:"actual_result"`
	SelfScore        float64 `json:"self_score"`
	ManagerScore     float64 `json:"manager_score"`
	BonusScore       float64 `json:"bonus_score"`
	AttachmentsCount int     `json:"attachments_count"`
	ApprovalStatus   string  `json:"approval_status"`
}

type PerformanceResultReport struct {
	Summary                PerformanceResultReportSummary `json:"summary"`
	Rows                   []PerformanceResultReportRow   `json:"rows"`
	LevelDistribution      []PerformanceChartItem         `json:"level_distribution"`
	DepartmentDistribution []PerformanceChartItem         `json:"department_distribution"`
}

type PerformanceResultReportSummary struct {
	TotalParticipants int     `json:"total_participants"`
	LockedCount       int     `json:"locked_count"`
	HiddenCount       int     `json:"hidden_count"`
	AverageScore      float64 `json:"average_score"`
}

type PerformanceResultReportRow struct {
	ParticipantID          uint     `json:"participant_id"`
	EmployeeID             string   `json:"employee_id"`
	EmployeeName           string   `json:"employee_name"`
	DepartmentID           string   `json:"department_id"`
	DepartmentName         string   `json:"department_name"`
	Position               string   `json:"position"`
	ManagerName            string   `json:"manager_name"`
	Status                 string   `json:"status"`
	SelfScore              float64  `json:"self_score"`
	ManagerScore           float64  `json:"manager_score"`
	TotalSelfScore         float64  `json:"total_self_score"`
	TotalManagerScore      float64  `json:"total_manager_score"`
	BonusScore             float64  `json:"bonus_score"`
	PenaltyScore           float64  `json:"penalty_score"`
	AdjustedScore          float64  `json:"adjusted_score"`
	SuggestedLevel         string   `json:"suggested_level"`
	FinalLevel             string   `json:"final_level"`
	EffectiveFinalLevel    string   `json:"effective_final_level"`
	AdjustReason           string   `json:"adjust_reason"`
	DepartmentFinalScore   *float64 `json:"department_final_score"`
	DepartmentFinalLevel   string   `json:"department_final_level"`
	DepartmentAdjustReason string   `json:"department_adjust_reason"`
	HRConfirmed            bool     `json:"hr_confirmed"`
	Locked                 bool     `json:"locked"`
	ResultHidden           bool     `json:"result_hidden"`
	ResultVisible          bool     `json:"result_visible"`
}

type PerformanceChartItem struct {
	Name  string  `json:"name"`
	Value int     `json:"value"`
	Rate  float64 `json:"rate"`
}

func (s *PerformanceReportService) BuildReport(activityID string, filter PerformanceReportFilter) (*PerformanceReport, error) {
	var activity database.PerformanceActivity
	if err := s.db.Where("id = ? AND deleted_at IS NULL", activityID).First(&activity).Error; err != nil {
		return nil, err
	}

	participants, err := s.listReportParticipants(activityID, filter)
	if err != nil {
		return nil, err
	}

	report := &PerformanceReport{
		Activity:  activity,
		IsNewFlow: strings.EqualFold(strings.TrimSpace(activity.FlowType), "new"),
	}
	report.Progress = buildProgressReport(activity, participants)
	report.Content, err = s.buildContentReport(activity, participants)
	if err != nil {
		return nil, err
	}
	report.Result = buildResultReport(activity, participants, filter.Access)
	return report, nil
}

func (s *PerformanceReportService) listReportParticipants(activityID string, filter PerformanceReportFilter) ([]database.PerformanceParticipant, error) {
	var participants []database.PerformanceParticipant
	query := s.db.Model(&database.PerformanceParticipant{}).
		Where("activity_id = ? AND deleted_at IS NULL", activityID).
		Where("status NOT IN ?", []string{"inactive", "removed_from_scope"})

	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.EmployeeKeyword != "" {
		like := "%" + strings.TrimSpace(filter.EmployeeKeyword) + "%"
		query = query.Where("employee_name LIKE ? OR employee_id LIKE ?", like, like)
	}

	departmentIDs, err := s.resolveReportDepartmentIDs(filter)
	if err != nil {
		return nil, err
	}
	if len(departmentIDs) > 0 {
		query = query.Where("department_id IN ?", departmentIDs)
	}
	if filter.Scope != nil && filter.Scope.IsSelf() {
		userIDs := uniqueReportStrings(filter.Scope.UserIDs)
		if len(userIDs) == 0 {
			return []database.PerformanceParticipant{}, nil
		}
		query = query.Where("employee_id IN ?", userIDs)
	}

	if err := query.Order("department_id ASC, employee_name ASC, id ASC").Find(&participants).Error; err != nil {
		return nil, err
	}
	if strings.TrimSpace(filter.Level) == "" {
		return participants, nil
	}
	level := strings.TrimSpace(filter.Level)
	filtered := make([]database.PerformanceParticipant, 0, len(participants))
	for _, participant := range participants {
		if effectiveParticipantLevel(participant) == level {
			filtered = append(filtered, participant)
		}
	}
	return filtered, nil
}

func (s *PerformanceReportService) resolveReportDepartmentIDs(filter PerformanceReportFilter) ([]string, error) {
	var selected []string
	hasDepartmentSelector := strings.TrimSpace(filter.CompanyID) != "" || strings.TrimSpace(filter.DepartmentID) != ""
	if strings.TrimSpace(filter.CompanyID) != "" {
		ids, err := s.departmentTreeIDs(filter.CompanyID)
		if err != nil {
			return nil, err
		}
		selected = ids
	}
	if strings.TrimSpace(filter.DepartmentID) != "" {
		ids, err := s.departmentTreeIDs(filter.DepartmentID)
		if err != nil {
			return nil, err
		}
		if len(selected) > 0 {
			selected = intersectReportStrings(selected, ids)
		} else {
			selected = ids
		}
	}
	selected = uniqueReportStrings(selected)
	if hasDepartmentSelector && len(selected) == 0 {
		return []string{"__no_matching_department__"}, nil
	}

	if filter.Scope == nil || filter.Scope.IsAll() || filter.Scope.IsSelf() {
		return selected, nil
	}
	scopeIDs := uniqueReportStrings(filter.Scope.DepartmentIDs)
	if len(selected) == 0 {
		return scopeIDs, nil
	}
	intersected := intersectReportStrings(selected, scopeIDs)
	if len(intersected) == 0 {
		return []string{"__no_matching_department__"}, nil
	}
	return intersected, nil
}

func (s *PerformanceReportService) departmentTreeIDs(departmentID string) ([]string, error) {
	departmentID = strings.TrimSpace(departmentID)
	if departmentID == "" {
		return nil, nil
	}
	var ids []string
	orgID := orgIDFromDB(s.db)
	query := `
		WITH RECURSIVE dept_tree AS (
			SELECT department_id, parent_id
			FROM departments
			WHERE org_id = ? AND department_id = ?
			UNION ALL
			SELECT d.department_id, d.parent_id
			FROM departments d
			INNER JOIN dept_tree dt ON d.org_id = ? AND d.parent_id = dt.department_id
		)
		SELECT department_id FROM dept_tree
	`
	if err := s.db.Raw(query, orgID, departmentID, orgID).Scan(&ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return []string{departmentID}, nil
	}
	return uniqueReportStrings(ids), nil
}

func buildProgressReport(activity database.PerformanceActivity, participants []database.PerformanceParticipant) PerformanceProgressReport {
	rows := make([]PerformanceProgressRow, 0, len(participants))
	statusCounts := make(map[string]int)
	summary := PerformanceProgressSummary{TotalParticipants: len(participants)}

	for _, participant := range participants {
		targetSubmitted := participantCompletedStage(participant.Status, "target_setting")
		selfSubmitted := participantCompletedStage(participant.Status, "self_evaluation") || participant.SelfSummary != "" || participant.SelfScore > 0
		managerSubmitted := participantCompletedStage(participant.Status, "manager_evaluation") || participant.ManagerScore > 0 || participant.FinalLevel != ""
		departmentReviewed := participant.DepartmentAdjusted || participantCompletedStage(participant.Status, "department_evaluation")
		hrConfirmed := participant.HRConfirmedAt != nil || participantCompletedStage(participant.Status, "hr_confirmation")
		locked := participant.IsLocked || participant.Status == "locked"
		progressRate := progressRateForParticipant(activity, participant, targetSubmitted, selfSubmitted, managerSubmitted, departmentReviewed, hrConfirmed, locked)

		if targetSubmitted {
			summary.TargetSubmittedCount++
		}
		if selfSubmitted {
			summary.SelfSubmittedCount++
		}
		if managerSubmitted {
			summary.ManagerSubmittedCount++
		}
		if departmentReviewed {
			summary.DepartmentReviewedCount++
		}
		if hrConfirmed {
			summary.HRConfirmedCount++
		}
		if locked {
			summary.LockedCount++
		}
		statusCounts[participant.Status]++
		rows = append(rows, PerformanceProgressRow{
			ParticipantID:      participant.ID,
			EmployeeID:         participant.EmployeeID,
			EmployeeName:       participant.EmployeeName,
			DepartmentID:       participant.DepartmentID,
			DepartmentName:     participant.DepartmentName,
			Position:           participant.Position,
			ManagerID:          reportStringValue(participant.ManagerID),
			ManagerName:        reportStringValue(participant.ManagerName),
			Status:             participant.Status,
			TargetSubmitted:    targetSubmitted,
			SelfSubmitted:      selfSubmitted,
			ManagerSubmitted:   managerSubmitted,
			DepartmentReviewed: departmentReviewed,
			HRConfirmed:        hrConfirmed,
			Locked:             locked,
			ProgressRate:       progressRate,
			ResultHidden:       participant.ResultHidden,
		})
	}
	if summary.TotalParticipants > 0 {
		summary.CompletionRate = roundPercent(float64(summary.LockedCount) / float64(summary.TotalParticipants) * 100)
	}
	return PerformanceProgressReport{
		Summary:            summary,
		Rows:               rows,
		StatusDistribution: buildChartItems(statusCounts, summary.TotalParticipants),
	}
}

func (s *PerformanceReportService) buildContentReport(activity database.PerformanceActivity, participants []database.PerformanceParticipant) (PerformanceContentReport, error) {
	if len(participants) == 0 {
		return PerformanceContentReport{}, nil
	}
	participantIDs := make([]uint, 0, len(participants))
	participantByID := make(map[uint]database.PerformanceParticipant, len(participants))
	for _, participant := range participants {
		participantIDs = append(participantIDs, participant.ID)
		participantByID[participant.ID] = participant
	}

	var records []database.PerformanceGoalRecord
	query := s.db.Where("activity_id = ? AND participant_id IN ? AND deleted_at IS NULL", strconv.FormatUint(uint64(activity.ID), 10), participantIDs)
	if !strings.EqualFold(strings.TrimSpace(activity.FlowType), "new") {
		query = query.Where("(goal_phase = ? OR goal_phase = '' OR goal_phase IS NULL)", "review")
	}
	if err := query.Order("participant_id ASC, goal_phase ASC, section_type ASC, sort_order ASC, created_at ASC").Find(&records).Error; err != nil {
		return PerformanceContentReport{}, err
	}

	rows := make([]PerformanceContentRow, 0, len(records))
	phaseCounts := map[string]int{}
	reviewParticipants := map[uint]struct{}{}
	planParticipants := map[uint]struct{}{}
	var completionTotal float64
	var completionCount int
	for _, record := range records {
		participant, ok := participantByID[record.ParticipantID]
		if !ok {
			continue
		}
		phase := strings.TrimSpace(record.GoalPhase)
		if phase == "" {
			phase = "review"
		}
		if phase == "plan" {
			planParticipants[record.ParticipantID] = struct{}{}
		} else {
			reviewParticipants[record.ParticipantID] = struct{}{}
		}
		phaseCounts[goalPhaseLabel(phase)]++
		if phase != "plan" {
			completionTotal += record.CompletionRate
			completionCount++
		}
		rows = append(rows, PerformanceContentRow{
			ID:               record.ID,
			ParticipantID:    record.ParticipantID,
			EmployeeID:       participant.EmployeeID,
			EmployeeName:     participant.EmployeeName,
			DepartmentID:     participant.DepartmentID,
			DepartmentName:   participant.DepartmentName,
			Position:         participant.Position,
			ManagerName:      reportStringValue(participant.ManagerName),
			GoalPhase:        phase,
			GoalPhaseLabel:   goalPhaseLabel(phase),
			SectionType:      record.SectionType,
			GoalType:         record.GoalType,
			ItemName:         record.ItemName,
			ItemDefinition:   record.ItemDefinition,
			Weight:           record.Weight,
			TargetValue:      record.TargetValue,
			ChallengeValue:   record.ChallengeValue,
			MetricUnit:       record.MetricUnit,
			CompletionRate:   record.CompletionRate,
			ActualResult:     record.ActualResult,
			SelfScore:        record.SelfScore,
			ManagerScore:     record.ManagerScore,
			BonusScore:       record.BonusScore,
			AttachmentsCount: len(record.Attachments),
			ApprovalStatus:   record.ApprovalStatus,
		})
	}

	summary := PerformanceContentSummary{
		ReviewItemCount:        phaseCounts[goalPhaseLabel("review")],
		PlanItemCount:          phaseCounts[goalPhaseLabel("plan")],
		ParticipantsWithReview: len(reviewParticipants),
		ParticipantsWithPlan:   len(planParticipants),
	}
	if completionCount > 0 {
		summary.AverageCompletionRate = roundPercent(completionTotal / float64(completionCount))
	}
	return PerformanceContentReport{
		Summary: summary,
		Rows:    rows,
		Phases:  buildChartItems(phaseCounts, len(rows)),
	}, nil
}

func buildResultReport(activity database.PerformanceActivity, participants []database.PerformanceParticipant, access PerformanceReportAccess) PerformanceResultReport {
	rows := make([]PerformanceResultReportRow, 0, len(participants))
	levelCounts := make(map[string]int)
	departmentCounts := make(map[string]int)
	summary := PerformanceResultReportSummary{TotalParticipants: len(participants)}
	var scoreTotal float64
	var scoreCount int

	for _, participant := range participants {
		redact := shouldRedactReportParticipant(participant, access)
		row := buildResultRow(participant, redact)
		rows = append(rows, row)
		if participant.IsLocked || participant.Status == "locked" {
			summary.LockedCount++
		}
		if participant.ResultHidden {
			summary.HiddenCount++
		}
		if row.EffectiveFinalLevel != "" {
			levelCounts[row.EffectiveFinalLevel]++
		}
		departmentName := strings.TrimSpace(participant.DepartmentName)
		if departmentName == "" {
			departmentName = participant.DepartmentID
		}
		if departmentName != "" {
			departmentCounts[departmentName]++
		}
		score := effectiveParticipantScore(participant)
		if !redact && score > 0 {
			scoreTotal += score
			scoreCount++
		}
	}
	if scoreCount > 0 {
		summary.AverageScore = roundReportScore(scoreTotal / float64(scoreCount))
	}
	return PerformanceResultReport{
		Summary:                summary,
		Rows:                   rows,
		LevelDistribution:      buildOrderedLevelChartItems(levelCounts, summary.TotalParticipants),
		DepartmentDistribution: buildChartItems(departmentCounts, summary.TotalParticipants),
	}
}

func buildResultRow(participant database.PerformanceParticipant, redact bool) PerformanceResultReportRow {
	row := PerformanceResultReportRow{
		ParticipantID:  participant.ID,
		EmployeeID:     participant.EmployeeID,
		EmployeeName:   participant.EmployeeName,
		DepartmentID:   participant.DepartmentID,
		DepartmentName: participant.DepartmentName,
		Position:       participant.Position,
		ManagerName:    reportStringValue(participant.ManagerName),
		Status:         participant.Status,
		ResultHidden:   participant.ResultHidden,
		ResultVisible:  !redact,
		HRConfirmed:    participant.HRConfirmedAt != nil || participantCompletedStage(participant.Status, "hr_confirmation"),
		Locked:         participant.IsLocked || participant.Status == "locked",
	}
	if redact {
		return row
	}
	row.SelfScore = participant.SelfScore
	row.ManagerScore = participant.ManagerScore
	row.TotalSelfScore = participant.TotalSelfScore
	row.TotalManagerScore = participant.TotalManagerScore
	row.BonusScore = participant.BonusScore
	row.PenaltyScore = participant.PenaltyScore
	row.AdjustedScore = participant.AdjustedScore
	row.SuggestedLevel = participant.SuggestedLevel
	row.FinalLevel = participant.FinalLevel
	row.EffectiveFinalLevel = effectiveParticipantLevel(participant)
	row.AdjustReason = participant.AdjustReason
	row.DepartmentFinalScore = participant.DepartmentFinalScore
	row.DepartmentFinalLevel = participant.DepartmentFinalLevel
	row.DepartmentAdjustReason = participant.DepartmentAdjustReason
	return row
}

func (s *PerformanceReportService) BuildReportXLSX(report *PerformanceReport, reportType string) ([]byte, error) {
	if report == nil {
		return nil, fmt.Errorf("report is nil")
	}
	reportType = normalizePerformanceReportType(reportType)
	sheets := performanceReportSheets(report, reportType)
	return buildSimpleXLSX(sheets)
}

func normalizePerformanceReportType(reportType string) string {
	switch strings.ToLower(strings.TrimSpace(reportType)) {
	case PerformanceReportTypeProgress:
		return PerformanceReportTypeProgress
	case PerformanceReportTypeContent:
		return PerformanceReportTypeContent
	case PerformanceReportTypeResult:
		return PerformanceReportTypeResult
	default:
		return PerformanceReportTypeAll
	}
}

type performanceExportSheet struct {
	Name string
	Rows [][]string
}

func performanceReportSheets(report *PerformanceReport, reportType string) []performanceExportSheet {
	sheets := make([]performanceExportSheet, 0, 4)
	if reportType == PerformanceReportTypeAll || reportType == PerformanceReportTypeProgress {
		sheets = append(sheets, progressExportSheet(report.Progress.Rows))
	}
	if reportType == PerformanceReportTypeAll || reportType == PerformanceReportTypeContent {
		sheets = append(sheets, contentExportSheets(report)...)
	}
	if reportType == PerformanceReportTypeAll || reportType == PerformanceReportTypeResult {
		sheets = append(sheets, resultExportSheet(report.Result.Rows))
	}
	return sheets
}

func progressExportSheet(rows []PerformanceProgressRow) performanceExportSheet {
	data := [][]string{{"员工工号", "员工姓名", "部门", "岗位", "考核上级", "当前状态", "目标提交", "自评提交", "上级评分", "部门/中心评估", "HR确认", "锁定", "进度"}}
	for _, row := range rows {
		data = append(data, []string{
			row.EmployeeID, row.EmployeeName, row.DepartmentName, row.Position, row.ManagerName, row.Status,
			yesNo(row.TargetSubmitted), yesNo(row.SelfSubmitted), yesNo(row.ManagerSubmitted), yesNo(row.DepartmentReviewed), yesNo(row.HRConfirmed), yesNo(row.Locked),
			fmt.Sprintf("%.2f%%", row.ProgressRate),
		})
	}
	return performanceExportSheet{Name: "考核进度", Rows: data}
}

func contentExportSheets(report *PerformanceReport) []performanceExportSheet {
	if report.IsNewFlow {
		reviewRows := contentExportRows(report.Content.Rows, "review")
		planRows := contentExportRows(report.Content.Rows, "plan")
		return []performanceExportSheet{
			{Name: "上季度完成情况", Rows: reviewRows},
			{Name: "下季度目标计划", Rows: planRows},
		}
	}
	return []performanceExportSheet{{Name: "考核内容", Rows: contentExportRows(report.Content.Rows, "")}}
}

func contentExportRows(rows []PerformanceContentRow, phase string) [][]string {
	data := [][]string{{"员工工号", "员工姓名", "部门", "岗位", "考核上级", "阶段", "目标类型", "类别", "目标名称", "说明", "权重", "目标值/计划", "挑战值", "单位", "完成率", "完成情况", "自评分", "主管评分", "附件数", "审批状态"}}
	for _, row := range rows {
		if phase != "" && row.GoalPhase != phase {
			continue
		}
		data = append(data, []string{
			row.EmployeeID, row.EmployeeName, row.DepartmentName, row.Position, row.ManagerName, row.GoalPhaseLabel, row.GoalType, row.SectionType, row.ItemName, row.ItemDefinition,
			formatFloat(row.Weight), row.TargetValue, row.ChallengeValue, row.MetricUnit, formatFloat(row.CompletionRate), row.ActualResult, formatFloat(row.SelfScore), formatFloat(row.ManagerScore), strconv.Itoa(row.AttachmentsCount), row.ApprovalStatus,
		})
	}
	return data
}

func resultExportSheet(rows []PerformanceResultReportRow) performanceExportSheet {
	data := [][]string{{"员工工号", "员工姓名", "部门", "岗位", "考核上级", "当前状态", "自评分", "主管评分", "自评总分", "主管总分", "加分", "扣分", "调整后分数", "建议等级", "最终等级", "部门最终分", "部门最终等级", "部门调整原因", "HR确认", "锁定", "结果屏蔽", "结果可见"}}
	for _, row := range rows {
		departmentScore := ""
		if row.DepartmentFinalScore != nil {
			departmentScore = formatFloat(*row.DepartmentFinalScore)
		}
		data = append(data, []string{
			row.EmployeeID, row.EmployeeName, row.DepartmentName, row.Position, row.ManagerName, row.Status,
			formatFloat(row.SelfScore), formatFloat(row.ManagerScore), formatFloat(row.TotalSelfScore), formatFloat(row.TotalManagerScore), formatFloat(row.BonusScore), formatFloat(row.PenaltyScore), formatFloat(row.AdjustedScore),
			row.SuggestedLevel, row.FinalLevel, departmentScore, row.DepartmentFinalLevel, row.DepartmentAdjustReason,
			yesNo(row.HRConfirmed), yesNo(row.Locked), yesNo(row.ResultHidden), yesNo(row.ResultVisible),
		})
	}
	return performanceExportSheet{Name: "考核结果", Rows: data}
}

func buildSimpleXLSX(sheets []performanceExportSheet) ([]byte, error) {
	if len(sheets) == 0 {
		sheets = []performanceExportSheet{{Name: "报表", Rows: [][]string{{"暂无数据"}}}}
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml":        xlsxContentTypes(len(sheets)),
		"_rels/.rels":                xlsxRootRels(),
		"xl/workbook.xml":            xlsxWorkbookXML(sheets),
		"xl/_rels/workbook.xml.rels": xlsxWorkbookRels(len(sheets)),
		"xl/styles.xml":              xlsxStyles(),
	}
	for name, content := range files {
		if err := writeZipFile(zw, name, content); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	for i, sheet := range sheets {
		if err := writeZipFile(zw, fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), xlsxWorksheetXML(sheet.Rows)); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func writeZipFile(zw *zip.Writer, name, content string) error {
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte(content))
	return err
}

func xlsxContentTypes(sheetCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">`)
	b.WriteString(`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>`)
	b.WriteString(`<Default Extension="xml" ContentType="application/xml"/>`)
	b.WriteString(`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	b.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	for i := 1; i <= sheetCount; i++ {
		fmt.Fprintf(&b, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i)
	}
	b.WriteString(`</Types>`)
	return b.String()
}

func xlsxRootRels() string {
	return `<?xml version="1.0" encoding="UTF-8"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`
}

func xlsxWorkbookXML(sheets []performanceExportSheet) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>`)
	for i, sheet := range sheets {
		fmt.Fprintf(&b, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEscape(limitSheetName(sheet.Name)), i+1, i+1)
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

func xlsxWorkbookRels(sheetCount int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= sheetCount; i++ {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i)
	}
	fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, sheetCount+1)
	b.WriteString(`</Relationships>`)
	return b.String()
}

func xlsxStyles() string {
	return `<?xml version="1.0" encoding="UTF-8"?><styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><fonts count="1"><font><sz val="11"/><name val="Calibri"/></font></fonts><fills count="1"><fill><patternFill patternType="none"/></fill></fills><borders count="1"><border/></borders><cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs><cellXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/></cellXfs></styleSheet>`
}

func xlsxWorksheetXML(rows [][]string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for rowIndex, row := range rows {
		fmt.Fprintf(&b, `<row r="%d">`, rowIndex+1)
		for colIndex, value := range row {
			ref := fmt.Sprintf("%s%d", xlsxColumnName(colIndex+1), rowIndex+1)
			fmt.Fprintf(&b, `<c r="%s" t="inlineStr"><is><t>%s</t></is></c>`, ref, xmlEscape(value))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

func xlsxColumnName(index int) string {
	name := ""
	for index > 0 {
		index--
		name = string(rune('A'+index%26)) + name
		index /= 26
	}
	return name
}

func xmlEscape(value string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(value))
	return buf.String()
}

func limitSheetName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "Sheet"
	}
	replacer := strings.NewReplacer(":", "：", "\\", "＼", "/", "／", "?", "？", "*", "＊", "[", "【", "]", "】")
	name = replacer.Replace(name)
	runes := []rune(name)
	if len(runes) > 31 {
		return string(runes[:31])
	}
	return name
}

func progressRateForParticipant(activity database.PerformanceActivity, participant database.PerformanceParticipant, targetSubmitted, selfSubmitted, managerSubmitted, departmentReviewed, hrConfirmed, locked bool) float64 {
	steps := []bool{targetSubmitted, selfSubmitted, managerSubmitted}
	if strings.EqualFold(strings.TrimSpace(activity.FlowType), "new") {
		steps = append(steps, departmentReviewed, hrConfirmed, locked)
	} else {
		employeeConfirmed := participant.EmployeeConfirmedAt != nil || participantCompletedStage(participant.Status, "employee_confirmation")
		managerConfirmed := participant.ManagerConfirmedAt != nil || participantCompletedStage(participant.Status, "manager_confirmation")
		steps = append(steps, employeeConfirmed, managerConfirmed, hrConfirmed, locked)
	}
	done := 0
	for _, step := range steps {
		if step {
			done++
		}
	}
	if len(steps) == 0 {
		return 0
	}
	return roundPercent(float64(done) / float64(len(steps)) * 100)
}

func shouldRedactReportParticipant(participant database.PerformanceParticipant, access PerformanceReportAccess) bool {
	if !participant.ResultHidden || access.Privileged {
		return false
	}
	_, ok := access.IdentityValues[strings.TrimSpace(participant.EmployeeID)]
	return ok
}

func effectiveParticipantLevel(participant database.PerformanceParticipant) string {
	if strings.TrimSpace(participant.DepartmentFinalLevel) != "" {
		return strings.TrimSpace(participant.DepartmentFinalLevel)
	}
	return strings.TrimSpace(participant.FinalLevel)
}

func effectiveParticipantScore(participant database.PerformanceParticipant) float64 {
	if participant.DepartmentFinalScore != nil {
		return *participant.DepartmentFinalScore
	}
	if participant.AdjustedScore > 0 {
		return participant.AdjustedScore
	}
	if participant.TotalManagerScore > 0 {
		return participant.TotalManagerScore
	}
	return participant.ManagerScore
}

func goalPhaseLabel(phase string) string {
	switch strings.TrimSpace(phase) {
	case "plan":
		return "下季度目标计划"
	default:
		return "上季度完成情况"
	}
}

func buildChartItems(counts map[string]int, total int) []PerformanceChartItem {
	items := make([]PerformanceChartItem, 0, len(counts))
	for name, value := range counts {
		items = append(items, PerformanceChartItem{Name: name, Value: value, Rate: chartRate(value, total)})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Value == items[j].Value {
			return items[i].Name < items[j].Name
		}
		return items[i].Value > items[j].Value
	})
	return items
}

func buildOrderedLevelChartItems(counts map[string]int, total int) []PerformanceChartItem {
	levels := []string{"S", "A", "B", "C", "D"}
	items := make([]PerformanceChartItem, 0, len(counts))
	seen := map[string]struct{}{}
	for _, level := range levels {
		value := counts[level]
		items = append(items, PerformanceChartItem{Name: level, Value: value, Rate: chartRate(value, total)})
		seen[level] = struct{}{}
	}
	for level, value := range counts {
		if _, ok := seen[level]; ok {
			continue
		}
		items = append(items, PerformanceChartItem{Name: level, Value: value, Rate: chartRate(value, total)})
	}
	return items
}

func chartRate(value, total int) float64 {
	if total <= 0 {
		return 0
	}
	return roundPercent(float64(value) / float64(total) * 100)
}

func roundPercent(value float64) float64 {
	return math.Round(value*100) / 100
}

func roundReportScore(value float64) float64 {
	return math.Round(value*100) / 100
}

func reportStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func uniqueReportStrings(values []string) []string {
	seen := map[string]struct{}{}
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
	sort.Strings(result)
	return result
}

func intersectReportStrings(left, right []string) []string {
	rightSet := make(map[string]struct{}, len(right))
	for _, value := range right {
		rightSet[value] = struct{}{}
	}
	result := make([]string, 0)
	for _, value := range left {
		if _, ok := rightSet[value]; ok {
			result = append(result, value)
		}
	}
	return uniqueReportStrings(result)
}

func yesNo(value bool) string {
	if value {
		return "是"
	}
	return "否"
}

func formatFloat(value float64) string {
	if value == 0 {
		return "0"
	}
	return strconv.FormatFloat(math.Round(value*100)/100, 'f', -1, 64)
}
