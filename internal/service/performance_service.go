package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"peopleops/internal/requestmeta"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PerformanceService struct {
	db    *gorm.DB
	orgID string

	actRepo      *repository.PerformanceActivityRepository
	ruleRepo     *repository.PerformanceDistributionRuleRepository
	participantR *repository.PerformanceParticipantRepository
	versionRepo  *repository.PerformanceReviewVersionRepository
	changeRepo   *repository.PerformanceRelationshipChangeLogRepository
	templateRepo *repository.PerformanceTemplateRepository
	goalRepo     *repository.PerformanceGoalRecordRepository
	approvalRepo *repository.PerformanceGoalApprovalRepository
}

const (
	performanceReminderStageSelfEval       = "self_evaluation"
	performanceReminderStageManagerRecheck = "manager_recheck"
	performanceReminderChannelDing         = "dingtalk"
	performanceReminderKeyManagerRecheck   = "self_edit_after_manager_confirm"
	managerRecheckNotificationWindow       = time.Hour
)

var sendPerformanceActionCardToUser = dingtalk.SendCorpActionCardToUser

type SelfEvalReminderSendResult struct {
	Candidates  int
	Sent        int
	Skipped     int
	AlreadySent int
	Failed      int
}

type AutoSelfEvalReminderResult struct {
	ActivitiesScanned int
	ActivitiesMatched int
	Candidates        int
	Sent              int
	Skipped           int
	AlreadySent       int
	Failed            int
}

type SelfEvalAutoReminderRunOptions struct {
	IncludeCurrentDay bool
	OrgID             string
}

type selfEvalReminderSendOptions struct {
	Automatic    bool
	ReminderKey  string
	ReminderDate string
	Now          time.Time
}

func NewPerformanceService(db *gorm.DB) *PerformanceService {
	orgID := ""
	if db != nil {
		if tenantOrgID, err := requestmeta.TenantID(db.Statement.Context); err == nil {
			orgID = tenantOrgID
		}
	}
	return &PerformanceService{
		db:           db,
		orgID:        orgID,
		actRepo:      repository.NewPerformanceActivityRepositoryWithOrgID(db, orgID),
		ruleRepo:     repository.NewPerformanceDistributionRuleRepositoryWithOrgID(db, orgID),
		participantR: repository.NewPerformanceParticipantRepositoryWithOrgID(db, orgID),
		versionRepo:  repository.NewPerformanceReviewVersionRepositoryWithOrgID(db, orgID),
		changeRepo:   repository.NewPerformanceRelationshipChangeLogRepositoryWithOrgID(db, orgID),
		templateRepo: repository.NewPerformanceTemplateRepositoryWithOrgID(db, orgID),
		goalRepo:     repository.NewPerformanceGoalRecordRepositoryWithOrgID(db, orgID),
		approvalRepo: repository.NewPerformanceGoalApprovalRepositoryWithOrgID(db, orgID),
	}
}

func (s *PerformanceService) tenantOrgID() string {
	return strings.TrimSpace(s.orgID)
}

func (s *PerformanceService) scopedDB() *gorm.DB {
	return s.scopeOrg(s.db, "org_id")
}

func (s *PerformanceService) scopeOrg(tx *gorm.DB, column string) *gorm.DB {
	if tx == nil {
		return tx
	}
	orgID := s.tenantOrgID()
	if orgID == "" {
		return tx
	}
	column = strings.TrimSpace(column)
	if column == "" {
		column = "org_id"
	}
	return tx.Where(column+" = ?", orgID)
}

func (s *PerformanceService) displayNameForUser(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	var user database.User
	if err := s.scopedDB().Where("id = ?", value).First(&user).Error; err == nil && strings.TrimSpace(user.Name) != "" {
		return strings.TrimSpace(user.Name)
	}
	if err := s.scopedDB().Where("user_id = ?", value).First(&user).Error; err == nil && strings.TrimSpace(user.Name) != "" {
		return strings.TrimSpace(user.Name)
	}
	return value
}

func PerformanceSelfEvalURL(activityID string, participantID uint) string {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" || participantID == 0 {
		return ""
	}
	return dingtalk.BuildAppURL(fmt.Sprintf("/performance-self-eval/%s/%d", url.PathEscape(activityID), participantID))
}

func PerformanceManagerEvalURL(activityID string, participantID uint) string {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" || participantID == 0 {
		return ""
	}
	return dingtalk.BuildAppURL(fmt.Sprintf("/performance-manager-eval/%s/%d", url.PathEscape(activityID), participantID))
}

func PerformanceGoalSettingURL(activityID string, participantID uint) string {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" || participantID == 0 {
		return ""
	}
	return dingtalk.BuildAppURL(fmt.Sprintf("/performance-goal-setting/%s/%d", url.PathEscape(activityID), participantID))
}

func PerformanceResultURL(activityID string, participantID uint) string {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" || participantID == 0 {
		return ""
	}
	return dingtalk.BuildAppURL(fmt.Sprintf("/performance-result/%s/%d", url.PathEscape(activityID), participantID))
}

func PerformanceOverviewURL(activityID string) string {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return dingtalk.BuildAppURL("/performance-overview")
	}
	return dingtalk.BuildAppURL(fmt.Sprintf("/performance-overview?activity_id=%s", url.QueryEscape(activityID)))
}

type CreateActivityRequest struct {
	Name                           string
	CycleType                      string
	StartDate                      string
	EndDate                        string
	TemplateID                     *uint
	FlowType                       string
	OrganizationID                 string
	ApplicableOrgScope             []string
	TargetSetStartAt               string
	TargetSetEndAt                 string
	SelfEvalStartAt                string
	SelfEvalEndAt                  string
	ManagerEvalStartAt             string
	ManagerEvalEndAt               string
	ResultConfirmStartAt           string
	ResultConfirmEndAt             string
	EmployeeConfirmStartAt         string
	EmployeeConfirmEndAt           string
	ManagerConfirmStartAt          string
	ManagerConfirmEndAt            string
	HRConfirmStartAt               string
	HRConfirmEndAt                 string
	HRConfirmDeadline              string
	Status                         string
	TargetDepartmentIDs            []string
	TargetEmployeeIDs              []string
	ManagerAssignments             []database.PerformanceActivityManagerAssignment
	IndicatorLibraryID             *uint
	Description                    string
	DefaultAssessmentManagerSource string
	SnapshotAsOfDate               string
	SnapshotSource                 string
	TargetPlanActivityID           *uint
	PreviousReviewActivityID       *uint
	PublishMode                    string
	PublishAt                      string
	ReminderConfig                 map[string]interface{}
	EnableBonusScore               bool
	StrictTimeMode                 bool
}

const (
	ManagerSourceDirectManager  = "DIRECT_MANAGER"
	ManagerSourceDepartmentHead = "DEPARTMENT_HEAD"
	ManagerSourceCenterHead     = "CENTER_HEAD"
	ManagerSourceManual         = "MANUAL"
	ManagerSourceImport         = "IMPORT"
	ManagerSourceEmpty          = "EMPTY"
	ManagerSourceSystem         = "SYSTEM"

	ManagerConfigConfigured = "CONFIGURED"
	ManagerConfigPending    = "PENDING"
	ManagerConfigInvalid    = "INVALID"
)

var validPerformanceManagerSources = map[string]struct{}{
	ManagerSourceDirectManager:  {},
	ManagerSourceDepartmentHead: {},
	ManagerSourceCenterHead:     {},
	ManagerSourceManual:         {},
	ManagerSourceImport:         {},
	ManagerSourceEmpty:          {},
	ManagerSourceSystem:         {},
}

const (
	PerformanceFlowOld = "old"
	PerformanceFlowNew = "new"

	PerformanceTemplateCodeOld = "legacy_performance"
	PerformanceTemplateCodeNew = "new_performance"

	PerformanceGoalPhaseReview = "review"
	PerformanceGoalPhasePlan   = "plan"
)

type PerformanceLevelRuleConfig struct {
	Rules []PerformanceLevelRuleConfigItem `json:"rules"`
}

type PerformanceLevelRuleConfigItem struct {
	Level        string   `json:"level"`
	MinScore     *float64 `json:"min_score,omitempty"`
	MaxScore     *float64 `json:"max_score,omitempty"`
	MinInclusive bool     `json:"min_inclusive"`
	MaxInclusive bool     `json:"max_inclusive"`
	SortOrder    int      `json:"sort_order"`
}

func floatPtr(value float64) *float64 {
	return &value
}

func normalizePerformanceFlowType(flowType string) string {
	switch strings.ToLower(strings.TrimSpace(flowType)) {
	case PerformanceFlowNew:
		return PerformanceFlowNew
	default:
		return PerformanceFlowOld
	}
}

func performanceFlowTypeLabel(flowType string) string {
	if normalizePerformanceFlowType(flowType) == PerformanceFlowNew {
		return "新流程"
	}
	return "旧流程"
}

func builtInPerformanceTemplateFlowType(code string) (string, bool) {
	switch strings.TrimSpace(code) {
	case PerformanceTemplateCodeNew:
		return PerformanceFlowNew, true
	case PerformanceTemplateCodeOld:
		return PerformanceFlowOld, true
	default:
		return "", false
	}
}

func performanceTemplateFlowType(template *database.PerformanceTemplate, fallback string) string {
	if template == nil {
		return normalizePerformanceFlowType(fallback)
	}
	if flowType, ok := builtInPerformanceTemplateFlowType(template.Code); ok {
		return flowType
	}
	if strings.TrimSpace(template.FlowType) != "" {
		return normalizePerformanceFlowType(template.FlowType)
	}
	return normalizePerformanceFlowType(fallback)
}

func isNewPerformanceFlow(activity *database.PerformanceActivity) bool {
	return activity != nil && normalizePerformanceFlowType(activity.FlowType) == PerformanceFlowNew
}

func normalizePerformanceGoalPhase(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case PerformanceGoalPhasePlan:
		return PerformanceGoalPhasePlan
	default:
		return PerformanceGoalPhaseReview
	}
}

func targetSettingGoalPhaseForActivity(activity *database.PerformanceActivity) string {
	if isNewPerformanceFlow(activity) {
		return PerformanceGoalPhasePlan
	}
	return PerformanceGoalPhaseReview
}

func isReviewGoalRecordForActivity(activity *database.PerformanceActivity, record database.PerformanceGoalRecord) bool {
	if record.SectionType == "bonus_penalty" {
		return false
	}
	if !isNewPerformanceFlow(activity) {
		return true
	}
	return normalizePerformanceGoalPhase(record.GoalPhase) == PerformanceGoalPhaseReview
}

func isScorableGoalRecordForActivity(activity *database.PerformanceActivity, record database.PerformanceGoalRecord) bool {
	if record.SectionType == "bonus_penalty" {
		return true
	}
	return isReviewGoalRecordForActivity(activity, record)
}

func defaultPerformanceLevelRuleConfig(flowType string) map[string]interface{} {
	if normalizePerformanceFlowType(flowType) == PerformanceFlowNew {
		return map[string]interface{}{
			"rules": []map[string]interface{}{
				{"level": "S", "min_score": 10, "min_inclusive": false, "sort_order": 1},
				{"level": "A", "min_score": 9, "max_score": 10, "min_inclusive": true, "max_inclusive": true, "sort_order": 2},
				{"level": "B", "min_score": 7.5, "max_score": 9, "min_inclusive": true, "max_inclusive": false, "sort_order": 3},
				{"level": "C", "min_score": 6, "max_score": 7.5, "min_inclusive": true, "max_inclusive": false, "sort_order": 4},
				{"level": "D", "max_score": 6, "max_inclusive": false, "sort_order": 5},
			},
		}
	}
	return map[string]interface{}{
		"rules": []map[string]interface{}{
			{"level": "S", "min_score": 100, "min_inclusive": true, "sort_order": 1},
			{"level": "A", "min_score": 90, "max_score": 100, "min_inclusive": true, "max_inclusive": false, "sort_order": 2},
			{"level": "B", "min_score": 80, "max_score": 90, "min_inclusive": true, "max_inclusive": false, "sort_order": 3},
			{"level": "C", "min_score": 60, "max_score": 80, "min_inclusive": true, "max_inclusive": false, "sort_order": 4},
			{"level": "D", "max_score": 60, "max_inclusive": false, "sort_order": 5},
		},
	}
}

func defaultPerformanceDistributionConfig(flowType string) map[string]interface{} {
	percentages := map[string]interface{}{"S": 15, "A": 20, "B": 40, "C": 10, "D": 15}
	if normalizePerformanceFlowType(flowType) == PerformanceFlowNew {
		percentages = map[string]interface{}{"S": 5, "A": 15, "B": 60, "C": 15, "D": 5}
	}
	return map[string]interface{}{
		"percentages":        percentages,
		"distribution_scope": "department",
		"rounding_strategy":  "ceil",
		"enforcement_stage":  "manager_submit_warning",
		"allow_exception":    true,
	}
}

func defaultPerformanceWorkflowConfig(flowType string) map[string]interface{} {
	if normalizePerformanceFlowType(flowType) == PerformanceFlowNew {
		return map[string]interface{}{
			"nodes": []string{
				"target_setting",
				"target_approval",
				"self_evaluation",
				"manager_evaluation",
				"department_evaluation",
				"hr_review",
				"result_publish",
				"interview",
				"employee_interview_confirm",
				"appeal",
				"hr_appeal_process",
				"manager_recheck",
				"rescore",
				"archive",
			},
			"self_evaluation_required": false,
			"interview_required":       true,
			"allow_return_between":     []string{"manager_evaluation", "department_evaluation", "hr_review"},
		}
	}
	return map[string]interface{}{
		"nodes": []string{"target_setting", "self_evaluation", "manager_evaluation", "employee_confirmation", "manager_confirmation", "hr_confirmation", "archive"},
	}
}

func defaultPerformanceFormConfig(flowType string) map[string]interface{} {
	if normalizePerformanceFlowType(flowType) == PerformanceFlowNew {
		return map[string]interface{}{
			"score_scale":                 map[string]interface{}{"min": 0, "max": 10, "allow_decimal": true},
			"review_weight_total_policy":  "allow_over_100",
			"target_plan_total_weight":    1,
			"target_plan_variable_weight": 0.7,
			"fixed_items": []map[string]interface{}{
				{"fixed_key": "manager_arrangement", "name": "上级安排事项完成情况", "weight": 0.15, "required": true, "locked": true, "description": "上级安排的所有事项需在规定时间内完成，工作结果得到领导认可"},
				{"fixed_key": "values_discipline", "name": "价值观及工作纪律", "weight": 0.15, "required": true, "locked": true, "description": "拥抱公司价值观，不得违反公司管理制度、规范等"},
			},
			"variable_goal_types": []string{"okr", "kpi"},
		}
	}
	return map[string]interface{}{
		"score_scale": map[string]interface{}{"min": 0, "max": 100, "allow_decimal": true},
	}
}

func defaultPerformancePermissionConfig(flowType string) map[string]interface{} {
	return map[string]interface{}{
		"roles": []string{"employee", "manager", "department_head", "hr", "admin"},
	}
}

func defaultPerformancePublishConfig(flowType string) map[string]interface{} {
	return map[string]interface{}{
		"publish_mode": "manual",
	}
}

func defaultPerformanceConfig(flowType string) (map[string]interface{}, map[string]interface{}, map[string]interface{}, map[string]interface{}, map[string]interface{}, map[string]interface{}) {
	normalized := normalizePerformanceFlowType(flowType)
	return defaultPerformanceWorkflowConfig(normalized),
		defaultPerformanceFormConfig(normalized),
		defaultPerformanceLevelRuleConfig(normalized),
		defaultPerformanceDistributionConfig(normalized),
		defaultPerformancePermissionConfig(normalized),
		defaultPerformancePublishConfig(normalized)
}

func cloneJSONMap(value map[string]interface{}) map[string]interface{} {
	if len(value) == 0 {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return value
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		return value
	}
	return out
}

func mergeDefaultConfig(value, fallback map[string]interface{}) map[string]interface{} {
	if len(value) > 0 {
		return cloneJSONMap(value)
	}
	return cloneJSONMap(fallback)
}

func normalizeAssessmentManagerSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		source = ManagerSourceManual
	}
	switch source {
	case "直属主管", "直接主管":
		source = ManagerSourceDirectManager
	case "部门负责人", "部门主管":
		source = ManagerSourceDepartmentHead
	case "中心负责人", "中心主管":
		source = ManagerSourceCenterHead
	case "手动指定", "手工指定":
		source = ManagerSourceManual
	case "导入指定", "导入":
		source = ManagerSourceImport
	case "暂不设置", "待配置", "不设置", "空":
		source = ManagerSourceEmpty
	case "系统兼容", "系统":
		source = ManagerSourceSystem
	default:
		source = strings.ToUpper(source)
	}
	if _, ok := validPerformanceManagerSources[source]; !ok {
		return "", fmt.Errorf("无效的考核上级来源: %s", source)
	}
	return source, nil
}

func normalizeDefaultAssessmentManagerSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return ManagerSourceDirectManager, nil
	}
	normalized, err := normalizeAssessmentManagerSource(source)
	if err != nil {
		return "", err
	}
	switch normalized {
	case ManagerSourceDirectManager, ManagerSourceDepartmentHead, ManagerSourceCenterHead, ManagerSourceEmpty:
		return normalized, nil
	default:
		return "", fmt.Errorf("%s 不能作为活动默认考核上级规则", assessmentManagerSourceLabel(normalized))
	}
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

func stringPtrOrNil(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func addTrimmedStrings(values map[string]struct{}, rawValues ...string) {
	for _, value := range rawValues {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		values[value] = struct{}{}
	}
}

func stringSetContains(values map[string]struct{}, value string) bool {
	_, ok := values[strings.TrimSpace(value)]
	return ok
}

func (s *PerformanceService) participantSelfUserIDs(participant *database.PerformanceParticipant) map[string]struct{} {
	values := map[string]struct{}{}
	if participant == nil {
		return values
	}
	employeeID := strings.TrimSpace(participant.EmployeeID)
	addTrimmedStrings(values, employeeID)
	if employeeID == "" {
		return values
	}

	var profiles []database.EmployeeProfile
	if err := s.scopedDB().Where("(user_id = ? OR employee_id = ?) AND deleted_at IS NULL", employeeID, employeeID).Find(&profiles).Error; err != nil {
		return values
	}
	for _, profile := range profiles {
		if strings.TrimSpace(profile.UserID) == employeeID || strings.TrimSpace(profile.EmployeeID) == employeeID {
			addTrimmedStrings(values, profile.UserID, profile.EmployeeID)
		}
	}
	return values
}

// checkTimeWindow 校验当前时间是否在指定阶段的时间窗口内（严格模式下）
func checkTimeWindow(activity *database.PerformanceActivity, stage string) error {
	if !activity.StrictTimeMode {
		return nil
	}
	now := time.Now().Format("2006-01-02")
	var startAt, endAt string
	switch stage {
	case "self_evaluation":
		startAt, endAt = activity.SelfEvalStartAt, activity.SelfEvalEndAt
	case "manager_evaluation":
		startAt, endAt = activity.ManagerEvalStartAt, activity.ManagerEvalEndAt
	case "result_confirm":
		startAt, endAt = activity.ResultConfirmStartAt, activity.ResultConfirmEndAt
	case "target_setting":
		startAt, endAt = activity.TargetSetStartAt, activity.TargetSetEndAt
	}
	if startAt != "" && now < startAt {
		return fmt.Errorf("该阶段尚未开始，开始时间为 %s", startAt)
	}
	if endAt != "" && now > endAt {
		return fmt.Errorf("该阶段已截止，截止时间为 %s", endAt)
	}
	return nil
}

func SelfEvalDeadlineReminderText(endAt string) string {
	return selfEvalDeadlineReminderText(endAt, time.Now())
}

func selfEvalDeadlineReminderText(endAt string, now time.Time) string {
	endAt = strings.TrimSpace(endAt)
	if endAt == "" {
		return "请关注绩效活动配置的自评截止时间。"
	}
	deadline, err := time.ParseInLocation("2006-01-02", endAt, time.Local)
	if err != nil {
		return fmt.Sprintf("自评截止时间：%s。", endAt)
	}
	deadlineEnd := time.Date(deadline.Year(), deadline.Month(), deadline.Day(), 23, 59, 59, 0, deadline.Location())
	if now.IsZero() {
		now = time.Now()
	}
	if now.After(deadlineEnd) {
		return fmt.Sprintf("自评截止时间：%s，当前已逾期。", endAt)
	}
	if now.Format("2006-01-02") == deadline.Format("2006-01-02") {
		return fmt.Sprintf("自评截止时间：%s，今天截止。", endAt)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	deadlineDay := time.Date(deadline.Year(), deadline.Month(), deadline.Day(), 0, 0, 0, 0, deadline.Location())
	days := int(math.Ceil(deadlineDay.Sub(today).Hours() / 24))
	if days < 1 {
		days = 1
	}
	return fmt.Sprintf("自评截止时间：%s，距离截止还有 %d 天。", endAt, days)
}

func defaultSelfEvalReminderOffsets() []int {
	return []int{3, 1, 0}
}

func parsePerformanceDate(value string, loc *time.Location) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if loc == nil {
		loc = time.Local
	}
	layouts := []string{
		"2006-01-02",
		"2006-01-02 15:04:05",
		time.RFC3339,
		"2006-01-02T15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, loc); err == nil {
			return t.In(loc), true
		}
	}
	if len(value) >= 10 {
		if t, err := time.ParseInLocation("2006-01-02", value[:10], loc); err == nil {
			return t.In(loc), true
		}
	}
	return time.Time{}, false
}

func daysUntilDate(deadline time.Time, now time.Time, loc *time.Location) int {
	if loc == nil {
		loc = time.Local
	}
	if now.IsZero() {
		now = time.Now()
	}
	now = now.In(loc)
	deadline = deadline.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	deadlineDay := time.Date(deadline.Year(), deadline.Month(), deadline.Day(), 0, 0, 0, 0, loc)
	return int(deadlineDay.Sub(today).Hours() / 24)
}

func selfEvalAutoReminderEnabled(activity *database.PerformanceActivity) bool {
	if activity == nil || activity.ReminderConfig == nil {
		return true
	}
	for _, key := range []string{"self_eval_auto_reminder_enabled", "auto_self_eval_reminder_enabled"} {
		if raw, ok := activity.ReminderConfig[key]; ok {
			switch value := raw.(type) {
			case bool:
				return value
			case string:
				normalized := strings.ToLower(strings.TrimSpace(value))
				return normalized != "false" && normalized != "0" && normalized != "off"
			}
		}
	}
	return true
}

func normalizeReminderOffsets(values []int) []int {
	seen := make(map[int]struct{}, len(values))
	offsets := make([]int, 0, len(values))
	for _, value := range values {
		if value < 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		offsets = append(offsets, value)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(offsets)))
	return offsets
}

func parseReminderOffsetValue(raw interface{}) []int {
	switch value := raw.(type) {
	case []int:
		return value
	case []float64:
		offsets := make([]int, 0, len(value))
		for _, item := range value {
			offsets = append(offsets, int(item))
		}
		return offsets
	case []interface{}:
		offsets := make([]int, 0, len(value))
		for _, item := range value {
			offsets = append(offsets, parseReminderOffsetValue(item)...)
		}
		return offsets
	case float64:
		return []int{int(value)}
	case int:
		return []int{value}
	case string:
		parts := strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '，' || r == ';' || r == '；' || r == ' '
		})
		offsets := make([]int, 0, len(parts))
		for _, part := range parts {
			if parsed, err := strconv.Atoi(strings.TrimSpace(part)); err == nil {
				offsets = append(offsets, parsed)
			}
		}
		return offsets
	default:
		return nil
	}
}

func selfEvalReminderOffsets(activity *database.PerformanceActivity) []int {
	if activity != nil && activity.ReminderConfig != nil {
		for _, key := range []string{"self_eval_reminder_days", "self_eval_days_before_deadline", "self_eval_reminder_offsets"} {
			if raw, ok := activity.ReminderConfig[key]; ok {
				if offsets := normalizeReminderOffsets(parseReminderOffsetValue(raw)); len(offsets) > 0 {
					return offsets
				}
			}
		}
	}
	return defaultSelfEvalReminderOffsets()
}

func selfEvalReminderKey(daysUntilDeadline int) string {
	if daysUntilDeadline == 0 {
		return "self_eval_due_today"
	}
	return fmt.Sprintf("self_eval_due_in_%dd", daysUntilDeadline)
}

func dueSelfEvalReminderRound(activity *database.PerformanceActivity, now time.Time) (string, int, bool) {
	if activity == nil || !selfEvalAutoReminderEnabled(activity) {
		return "", 0, false
	}
	loc := time.Local
	deadline, ok := parsePerformanceDate(activity.SelfEvalEndAt, loc)
	if !ok {
		return "", 0, false
	}
	days := daysUntilDate(deadline, now, loc)
	if days < 0 {
		return "", days, false
	}
	for _, offset := range selfEvalReminderOffsets(activity) {
		if days == offset {
			return selfEvalReminderKey(days), days, true
		}
	}
	return "", days, false
}

func normalizePublishMode(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "manual"
	}
	switch value {
	case "manual", "auto":
		return value
	default:
		return "manual"
	}
}

func (s *PerformanceService) loadTemplateForActivity(templateID *uint, flowType string) (*database.PerformanceTemplate, string, error) {
	normalizedFlowType := normalizePerformanceFlowType(flowType)
	if templateID != nil && *templateID > 0 {
		template, _, _, err := s.templateRepo.GetByID(*templateID)
		if err != nil {
			return nil, "", errors.New("绩效流程模板不存在")
		}
		templateFlowType := performanceTemplateFlowType(template, normalizedFlowType)
		if strings.TrimSpace(flowType) != "" && templateFlowType != normalizedFlowType {
			return nil, "", fmt.Errorf("流程模板与流程类型不一致，请清空模板或选择%s模板", performanceFlowTypeLabel(normalizedFlowType))
		}
		return template, templateFlowType, nil
	}
	if normalizedFlowType == PerformanceFlowNew {
		template, err := s.ensureBuiltInPerformanceTemplate(PerformanceTemplateCodeNew)
		if err != nil {
			return nil, "", err
		}
		return template, PerformanceFlowNew, nil
	}
	return nil, PerformanceFlowOld, nil
}

func applyTemplateSnapshotToActivity(activity *database.PerformanceActivity, template *database.PerformanceTemplate, flowType string) {
	normalizedFlowType := normalizePerformanceFlowType(flowType)
	workflowConfig, formConfig, levelRuleConfig, distributionConfig, permissionConfig, publishConfig := defaultPerformanceConfig(normalizedFlowType)
	if template != nil {
		activity.TemplateID = &template.ID
		normalizedFlowType = performanceTemplateFlowType(template, flowType)
		workflowConfig, formConfig, levelRuleConfig, distributionConfig, permissionConfig, publishConfig = defaultPerformanceConfig(normalizedFlowType)
		activity.ApplicableOrgScope = append([]string(nil), template.OrganizationScope...)
		if strings.TrimSpace(activity.OrganizationID) == "" {
			activity.OrganizationID = strings.TrimSpace(template.OrganizationID)
		}
		activity.WorkflowConfig = mergeDefaultConfig(template.WorkflowConfig, workflowConfig)
		activity.FormConfig = mergeDefaultConfig(template.FormConfig, formConfig)
		activity.LevelRuleConfig = mergeDefaultConfig(template.LevelRuleConfig, levelRuleConfig)
		activity.DistributionConfig = mergeDefaultConfig(template.DistributionConfig, distributionConfig)
		activity.PermissionConfig = mergeDefaultConfig(template.PermissionConfig, permissionConfig)
		activity.PublishConfig = mergeDefaultConfig(template.PublishConfig, publishConfig)
	} else {
		activity.WorkflowConfig = mergeDefaultConfig(activity.WorkflowConfig, workflowConfig)
		activity.FormConfig = mergeDefaultConfig(activity.FormConfig, formConfig)
		activity.LevelRuleConfig = mergeDefaultConfig(activity.LevelRuleConfig, levelRuleConfig)
		activity.DistributionConfig = mergeDefaultConfig(activity.DistributionConfig, distributionConfig)
		activity.PermissionConfig = mergeDefaultConfig(activity.PermissionConfig, permissionConfig)
		activity.PublishConfig = mergeDefaultConfig(activity.PublishConfig, publishConfig)
	}
	activity.FlowType = normalizedFlowType
	if strings.TrimSpace(activity.PublishMode) == "" {
		activity.PublishMode = "manual"
	}
	if strings.TrimSpace(activity.SnapshotSource) == "" {
		activity.SnapshotSource = "current_user"
	}
}

func (s *PerformanceService) CreateActivity(req CreateActivityRequest, createdBy string) (*database.PerformanceActivity, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errors.New("name 不能为空")
	}
	cycleType := strings.TrimSpace(req.CycleType)
	if cycleType == "" {
		return nil, errors.New("cycle_type 不能为空")
	}
	defaultManagerSource, err := normalizeDefaultAssessmentManagerSource(req.DefaultAssessmentManagerSource)
	if err != nil {
		return nil, err
	}
	managerAssignments, err := s.normalizeActivityManagerAssignments(req.ManagerAssignments)
	if err != nil {
		return nil, err
	}
	template, flowType, err := s.loadTemplateForActivity(req.TemplateID, req.FlowType)
	if err != nil {
		return nil, err
	}
	if err := s.validateActivityIndicatorLibrary(req.IndicatorLibraryID, cycleType, performanceTemplateIDForValidation(template, req.TemplateID)); err != nil {
		return nil, err
	}
	activity := &database.PerformanceActivity{
		Name:                           strings.TrimSpace(req.Name),
		CycleType:                      cycleType,
		StartDate:                      strings.TrimSpace(req.StartDate),
		EndDate:                        strings.TrimSpace(req.EndDate),
		IndicatorLibraryID:             req.IndicatorLibraryID,
		TemplateID:                     req.TemplateID,
		FlowType:                       flowType,
		OrganizationID:                 strings.TrimSpace(req.OrganizationID),
		TargetSetStartAt:               strings.TrimSpace(req.TargetSetStartAt),
		TargetSetEndAt:                 strings.TrimSpace(req.TargetSetEndAt),
		SelfEvalStartAt:                strings.TrimSpace(req.SelfEvalStartAt),
		SelfEvalEndAt:                  strings.TrimSpace(req.SelfEvalEndAt),
		ManagerEvalStartAt:             strings.TrimSpace(req.ManagerEvalStartAt),
		ManagerEvalEndAt:               strings.TrimSpace(req.ManagerEvalEndAt),
		ResultConfirmStartAt:           strings.TrimSpace(req.ResultConfirmStartAt),
		ResultConfirmEndAt:             strings.TrimSpace(req.ResultConfirmEndAt),
		EmployeeConfirmStartAt:         strings.TrimSpace(req.EmployeeConfirmStartAt),
		EmployeeConfirmEndAt:           strings.TrimSpace(req.EmployeeConfirmEndAt),
		ManagerConfirmStartAt:          strings.TrimSpace(req.ManagerConfirmStartAt),
		ManagerConfirmEndAt:            strings.TrimSpace(req.ManagerConfirmEndAt),
		HRConfirmStartAt:               strings.TrimSpace(req.HRConfirmStartAt),
		HRConfirmEndAt:                 strings.TrimSpace(req.HRConfirmEndAt),
		HRConfirmDeadline:              strings.TrimSpace(req.HRConfirmDeadline),
		Status:                         strings.TrimSpace(req.Status),
		TargetDepartmentIDs:            req.TargetDepartmentIDs,
		TargetEmployeeIDs:              req.TargetEmployeeIDs,
		ApplicableOrgScope:             req.ApplicableOrgScope,
		ManagerAssignments:             managerAssignments,
		DefaultAssessmentManagerSource: defaultManagerSource,
		SnapshotAsOfDate:               strings.TrimSpace(req.SnapshotAsOfDate),
		SnapshotSource:                 strings.TrimSpace(req.SnapshotSource),
		TargetPlanActivityID:           req.TargetPlanActivityID,
		PreviousReviewActivityID:       req.PreviousReviewActivityID,
		PublishMode:                    normalizePublishMode(req.PublishMode),
		PublishAt:                      strings.TrimSpace(req.PublishAt),
		ReminderConfig:                 cloneJSONMap(req.ReminderConfig),
		Description:                    req.Description,
		EnableBonusScore:               req.EnableBonusScore,
		StrictTimeMode:                 req.StrictTimeMode,
		CreatedBy:                      createdBy,
	}
	applyTemplateSnapshotToActivity(activity, template, flowType)

	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(activity).Error; err != nil {
			return err
		}
		return seedDefaultDistributionRules(tx, activity, createdBy)
	}); err != nil {
		return nil, err
	}
	if strings.TrimSpace(activity.Status) == "draft" {
		activityID := strconv.FormatUint(uint64(activity.ID), 10)
		if _, err := s.RefreshParticipants(activityID, createdBy); err != nil {
			return nil, err
		}
	}
	return activity, nil
}

func (s *PerformanceService) UpdateActivity(activityID string, req CreateActivityRequest, updatedBy string) (*database.PerformanceActivity, error) {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, err
	}
	oldStatus := strings.TrimSpace(activity.Status)
	managerAssignments, err := s.normalizeActivityManagerAssignments(req.ManagerAssignments)
	if err != nil {
		return nil, err
	}
	scopeChanged := !sameStringSet(activity.TargetDepartmentIDs, req.TargetDepartmentIDs) ||
		!sameStringSet(activity.TargetEmployeeIDs, req.TargetEmployeeIDs)
	managerAssignmentsChanged := !sameActivityManagerAssignments(activity.ManagerAssignments, managerAssignments)
	templateChanged := (activity.TemplateID == nil && req.TemplateID != nil) ||
		(activity.TemplateID != nil && req.TemplateID == nil) ||
		(activity.TemplateID != nil && req.TemplateID != nil && *activity.TemplateID != *req.TemplateID) ||
		(strings.TrimSpace(req.FlowType) != "" && normalizePerformanceFlowType(activity.FlowType) != normalizePerformanceFlowType(req.FlowType))
	if oldStatus != "draft" && scopeChanged {
		return nil, errors.New("目标设定开启后不能调整参与范围")
	}
	if oldStatus != "draft" && templateChanged {
		return nil, errors.New("目标设定开启后不能切换绩效流程模板")
	}
	cycleType := strings.TrimSpace(req.CycleType)
	if cycleType == "" {
		return nil, errors.New("cycle_type 不能为空")
	}
	defaultManagerSource, err := normalizeDefaultAssessmentManagerSource(req.DefaultAssessmentManagerSource)
	if err != nil {
		return nil, err
	}
	shouldApplyTemplate := req.TemplateID != nil || strings.TrimSpace(req.FlowType) != ""
	var template *database.PerformanceTemplate
	flowType := activity.FlowType
	if shouldApplyTemplate {
		template, flowType, err = s.loadTemplateForActivity(req.TemplateID, req.FlowType)
		if err != nil {
			return nil, err
		}
	}
	validationTemplateID := activity.TemplateID
	if shouldApplyTemplate {
		validationTemplateID = performanceTemplateIDForValidation(template, req.TemplateID)
	}
	if err := s.validateActivityIndicatorLibrary(req.IndicatorLibraryID, cycleType, validationTemplateID); err != nil {
		return nil, err
	}
	activity.Name = strings.TrimSpace(req.Name)
	activity.CycleType = cycleType
	activity.StartDate = strings.TrimSpace(req.StartDate)
	activity.EndDate = strings.TrimSpace(req.EndDate)
	activity.IndicatorLibraryID = req.IndicatorLibraryID
	if shouldApplyTemplate {
		activity.TemplateID = req.TemplateID
		activity.FlowType = flowType
	}
	activity.OrganizationID = strings.TrimSpace(req.OrganizationID)
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
	activity.TargetDepartmentIDs = req.TargetDepartmentIDs
	activity.TargetEmployeeIDs = req.TargetEmployeeIDs
	activity.ApplicableOrgScope = req.ApplicableOrgScope
	activity.ManagerAssignments = managerAssignments
	activity.DefaultAssessmentManagerSource = defaultManagerSource
	activity.SnapshotAsOfDate = strings.TrimSpace(req.SnapshotAsOfDate)
	activity.SnapshotSource = strings.TrimSpace(req.SnapshotSource)
	activity.TargetPlanActivityID = req.TargetPlanActivityID
	activity.PreviousReviewActivityID = req.PreviousReviewActivityID
	activity.PublishMode = normalizePublishMode(req.PublishMode)
	activity.PublishAt = strings.TrimSpace(req.PublishAt)
	activity.ReminderConfig = cloneJSONMap(req.ReminderConfig)
	activity.Description = req.Description
	activity.EnableBonusScore = req.EnableBonusScore
	activity.StrictTimeMode = req.StrictTimeMode
	activity.UpdatedBy = updatedBy
	if shouldApplyTemplate {
		applyTemplateSnapshotToActivity(activity, template, flowType)
	}

	if err := s.actRepo.Update(activity); err != nil {
		return nil, err
	}
	if oldStatus == "draft" && strings.TrimSpace(activity.Status) == "draft" && (scopeChanged || managerAssignmentsChanged) {
		if _, err := s.RefreshParticipants(activityID, updatedBy); err != nil {
			return nil, err
		}
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
	if err := s.syncPreviousPlanRecordsForNewFlowActivity(activity, userID); err != nil {
		return err
	}
	if err := s.ensureNewFlowReviewRecordsReady(activity); err != nil {
		return err
	}
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
	// 普通员工（self 模式）只能看到自己参与的活动
	if scope != nil && scope.IsSelf() {
		return s.actRepo.FindAllByUserID(page, pageSize, status, keyword, startDate, endDate, scope.UserIDs)
	}

	var departmentIDs []string
	if scope != nil && !scope.IsAll() {
		departmentIDs = scope.DepartmentIDs
	}
	return s.actRepo.FindAll(page, pageSize, status, keyword, startDate, endDate, departmentIDs)
}

func (s *PerformanceService) GetResultSummary(activityID string) (map[string]interface{}, error) {
	var participants []database.PerformanceParticipant
	if err := s.scopedDB().Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&participants).Error; err != nil {
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
		"manager_recheck":    {},
		"manager_confirmed":  {},
		"hr_confirmed":       {},
		"locked":             {},
		"result_confirmed":   {},
	},
	"self_evaluation": {
		"self_submitted":     {},
		"manager_submitted":  {},
		"employee_confirmed": {},
		"manager_recheck":    {},
		"manager_confirmed":  {},
		"hr_confirmed":       {},
		"locked":             {},
		"result_confirmed":   {},
	},
	"manager_evaluation": {
		"manager_submitted":  {},
		"employee_confirmed": {},
		"manager_recheck":    {},
		"manager_confirmed":  {},
		"hr_confirmed":       {},
		"locked":             {},
		"result_confirmed":   {},
	},
	"employee_confirmation": {
		"employee_confirmed": {},
		"manager_recheck":    {},
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

var participantStageDisplayNames = map[string]string{
	"target_setting":        "目标设定/审批",
	"self_evaluation":       "自评",
	"manager_evaluation":    "主管评分",
	"employee_confirmation": "员工确认",
	"manager_confirmation":  "主管确认",
	"hr_confirmation":       "HR确认",
}

var participantStageAdvanceActions = map[string]string{
	"target_setting":        "开启自评",
	"self_evaluation":       "开启主管评分",
	"manager_evaluation":    "开启员工确认",
	"employee_confirmation": "开启主管确认",
	"manager_confirmation":  "开启HR确认",
	"hr_confirmation":       "锁定活动",
}

var participantStatusDisplayNames = map[string]string{
	"pending":                 "待处理",
	"target_pending_approval": "目标待审批",
	"target_rejected":         "目标已驳回",
	"target_set":              "目标设定",
	"self_submitted":          "已自评",
	"manager_submitted":       "已评分",
	"employee_confirmed":      "员工已确认",
	"manager_recheck":         "待领导复核",
	"manager_confirmed":       "主管已确认",
	"hr_confirmed":            "HR已确认",
	"locked":                  "已锁定",
	"result_confirmed":        "结果已确认",
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

func toFloat64(value interface{}) float64 {
	switch v := value.(type) {
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	case float32:
		return float64(v)
	case json.Number:
		f, _ := v.Float64()
		return f
	default:
		f, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64)
		return f
	}
}

func performanceDistributionPercentages(activity *database.PerformanceActivity) map[string]int {
	flowType := PerformanceFlowOld
	var config map[string]interface{}
	if activity != nil {
		flowType = activity.FlowType
		config = activity.DistributionConfig
	}
	defaultConfig := defaultPerformanceDistributionConfig(flowType)
	defaults := map[string]int{"S": 15, "A": 20, "B": 40, "C": 10, "D": 15}
	if rawPercentages, ok := defaultConfig["percentages"].(map[string]interface{}); ok {
		for level, value := range rawPercentages {
			defaults[level] = int(math.Round(toFloat64(value)))
		}
	}
	if len(config) == 0 {
		return defaults
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return defaults
	}
	var decoded struct {
		Percentages map[string]float64 `json:"percentages"`
	}
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Percentages) == 0 {
		return defaults
	}
	for level, pct := range decoded.Percentages {
		defaults[level] = int(math.Round(pct))
	}
	return defaults
}

func seedDefaultDistributionRules(tx *gorm.DB, activity *database.PerformanceActivity, userID string) error {
	if !isNewPerformanceFlow(activity) || activity.ID == 0 {
		return nil
	}
	activityID := strconv.FormatUint(uint64(activity.ID), 10)
	percentages := performanceDistributionPercentages(activity)
	rules := make([]database.PerformanceDistributionRule, 0, 5)
	for _, level := range []string{"S", "A", "B", "C", "D"} {
		rules = append(rules, database.PerformanceDistributionRule{
			OrgID:               strings.TrimSpace(activity.OrgID),
			ActivityID:          activityID,
			Level:               level,
			DistributionPercent: percentages[level],
			Description:         "流程模板默认分布",
			CreatedBy:           userID,
			UpdatedBy:           userID,
		})
	}
	deleteQuery := tx.Where("activity_id = ?", activityID)
	if strings.TrimSpace(activity.OrgID) != "" {
		deleteQuery = deleteQuery.Where("org_id = ?", strings.TrimSpace(activity.OrgID))
	}
	if err := deleteQuery.Delete(&database.PerformanceDistributionRule{}).Error; err != nil {
		return err
	}
	return tx.Create(&rules).Error
}

func (s *PerformanceService) GetDistributionCheck(activityID string) (*DistributionCheckResult, error) {
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		activity = nil
	}

	var participants []database.PerformanceParticipant
	if err := s.scopedDB().Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&participants).Error; err != nil {
		return nil, err
	}

	rules, err := s.ruleRepo.ListByActivity(activityID)
	if err != nil {
		return nil, err
	}

	defaultRules := performanceDistributionPercentages(activity)
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

func sameStringSet(left, right []string) bool {
	leftValues := uniqueStrings(left)
	rightValues := uniqueStrings(right)
	if len(leftValues) != len(rightValues) {
		return false
	}
	sort.Strings(leftValues)
	sort.Strings(rightValues)
	for i := range leftValues {
		if leftValues[i] != rightValues[i] {
			return false
		}
	}
	return true
}

type activityManagerAssignmentComparable struct {
	EmployeeID                  string
	AssessmentManagerUserID     string
	AssessmentManagerEmployeeID string
	AssessmentManagerName       string
	AssessmentManagerSource     string
	ManagerOverrideReason       string
}

func normalizeActivityManagerAssignmentSource(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return ManagerSourceImport, nil
	}
	normalized, err := normalizeAssessmentManagerSource(source)
	if err != nil {
		return "", err
	}
	if normalized == ManagerSourceEmpty || normalized == ManagerSourceSystem {
		return ManagerSourceImport, nil
	}
	return normalized, nil
}

func comparableActivityManagerAssignments(assignments []database.PerformanceActivityManagerAssignment) map[string]activityManagerAssignmentComparable {
	values := make(map[string]activityManagerAssignmentComparable, len(assignments))
	for _, assignment := range assignments {
		userID := strings.TrimSpace(assignment.UserID)
		if userID == "" {
			continue
		}
		values[userID] = activityManagerAssignmentComparable{
			EmployeeID:                  strings.TrimSpace(assignment.EmployeeID),
			AssessmentManagerUserID:     strings.TrimSpace(assignment.AssessmentManagerUserID),
			AssessmentManagerEmployeeID: strings.TrimSpace(assignment.AssessmentManagerEmployeeID),
			AssessmentManagerName:       strings.TrimSpace(assignment.AssessmentManagerName),
			AssessmentManagerSource:     strings.ToUpper(strings.TrimSpace(assignment.AssessmentManagerSource)),
			ManagerOverrideReason:       strings.TrimSpace(assignment.ManagerOverrideReason),
		}
	}
	return values
}

func sameActivityManagerAssignments(left, right []database.PerformanceActivityManagerAssignment) bool {
	leftValues := comparableActivityManagerAssignments(left)
	rightValues := comparableActivityManagerAssignments(right)
	if len(leftValues) != len(rightValues) {
		return false
	}
	for userID, leftValue := range leftValues {
		if rightValue, ok := rightValues[userID]; !ok || rightValue != leftValue {
			return false
		}
	}
	return true
}

func (s *PerformanceService) normalizeActivityManagerAssignments(assignments []database.PerformanceActivityManagerAssignment) ([]database.PerformanceActivityManagerAssignment, error) {
	if len(assignments) == 0 {
		return nil, nil
	}

	orderedUserIDs := make([]string, 0, len(assignments))
	normalizedByUserID := make(map[string]database.PerformanceActivityManagerAssignment, len(assignments))
	managerIDs := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		userID := strings.TrimSpace(assignment.UserID)
		managerUserID := strings.TrimSpace(assignment.AssessmentManagerUserID)
		if userID == "" || managerUserID == "" {
			continue
		}
		if userID == managerUserID || strings.TrimSpace(assignment.EmployeeID) == managerUserID {
			return nil, fmt.Errorf("考核上级不能设置为员工本人: %s", userID)
		}
		source, err := normalizeActivityManagerAssignmentSource(assignment.AssessmentManagerSource)
		if err != nil {
			return nil, err
		}
		if _, exists := normalizedByUserID[userID]; !exists {
			orderedUserIDs = append(orderedUserIDs, userID)
		}
		normalizedByUserID[userID] = database.PerformanceActivityManagerAssignment{
			UserID:                      userID,
			EmployeeID:                  strings.TrimSpace(assignment.EmployeeID),
			AssessmentManagerUserID:     managerUserID,
			AssessmentManagerEmployeeID: strings.TrimSpace(assignment.AssessmentManagerEmployeeID),
			AssessmentManagerName:       strings.TrimSpace(assignment.AssessmentManagerName),
			AssessmentManagerSource:     source,
			ManagerOverrideReason:       strings.TrimSpace(assignment.ManagerOverrideReason),
		}
		managerIDs = append(managerIDs, managerUserID)
	}
	if len(normalizedByUserID) == 0 {
		return nil, nil
	}

	var managers []database.User
	if err := s.scopedDB().Where("user_id IN ? AND status = ? AND deleted_at IS NULL", uniqueStrings(managerIDs), "active").Find(&managers).Error; err != nil {
		return nil, err
	}
	managerByID := make(map[string]database.User, len(managers))
	for _, manager := range managers {
		managerByID[manager.UserID] = manager
	}

	result := make([]database.PerformanceActivityManagerAssignment, 0, len(orderedUserIDs))
	for _, userID := range orderedUserIDs {
		assignment := normalizedByUserID[userID]
		manager, ok := managerByID[assignment.AssessmentManagerUserID]
		if !ok {
			return nil, fmt.Errorf("考核上级不存在或不是在职状态: %s", assignment.AssessmentManagerUserID)
		}
		if assignment.AssessmentManagerName == "" {
			assignment.AssessmentManagerName = strings.TrimSpace(manager.Name)
		}
		result = append(result, assignment)
	}
	return result, nil
}

func activityManagerAssignmentsByUser(assignments []database.PerformanceActivityManagerAssignment) map[string]database.PerformanceActivityManagerAssignment {
	assignmentByUserID := make(map[string]database.PerformanceActivityManagerAssignment, len(assignments))
	for _, assignment := range assignments {
		userID := strings.TrimSpace(assignment.UserID)
		managerUserID := strings.TrimSpace(assignment.AssessmentManagerUserID)
		if userID == "" || managerUserID == "" {
			continue
		}
		assignment.UserID = userID
		assignment.AssessmentManagerUserID = managerUserID
		assignmentByUserID[userID] = assignment
	}
	return assignmentByUserID
}

func shouldApplyActivityManagerAssignment(participant *database.PerformanceParticipant) bool {
	if participant == nil {
		return false
	}
	if !participant.ManagerOverridden {
		return true
	}
	source := strings.TrimSpace(participant.ManagerSource)
	return source == "" || source == ManagerSourceEmpty || source == ManagerSourceImport
}

func applyActivityManagerAssignment(participant *database.PerformanceParticipant, assignment database.PerformanceActivityManagerAssignment, activeUsers map[string]database.User, operatorID string, now time.Time) (bool, *database.PerformanceRelationshipChangeLog) {
	if participant == nil {
		return false, nil
	}
	managerUserID := strings.TrimSpace(assignment.AssessmentManagerUserID)
	if managerUserID == "" ||
		managerUserID == strings.TrimSpace(participant.EmployeeID) ||
		managerUserID == strings.TrimSpace(assignment.UserID) ||
		managerUserID == strings.TrimSpace(assignment.EmployeeID) {
		return false, nil
	}
	source, err := normalizeActivityManagerAssignmentSource(assignment.AssessmentManagerSource)
	if err != nil {
		source = ManagerSourceImport
	}
	managerName := strings.TrimSpace(assignment.AssessmentManagerName)
	configStatus := ManagerConfigInvalid
	if manager, ok := activeUsers[managerUserID]; ok {
		configStatus = ManagerConfigConfigured
		if managerName == "" {
			managerName = strings.TrimSpace(manager.Name)
		}
	}
	reason := strings.TrimSpace(assignment.ManagerOverrideReason)
	oldManagerID := ptrStringValue(participant.ManagerID)
	oldManagerName := ptrStringValue(participant.ManagerName)
	oldManagerSource := strings.TrimSpace(participant.ManagerSource)
	if oldManagerSource == "" {
		oldManagerSource = ManagerSourceSystem
	}
	if oldManagerID == managerUserID &&
		oldManagerName == managerName &&
		oldManagerSource == source &&
		participant.ManagerOverridden &&
		strings.TrimSpace(participant.ManagerOverrideReason) == reason &&
		strings.TrimSpace(participant.ManagerConfigStatus) == configStatus {
		return false, nil
	}

	participant.ManagerID = stringPtrOrNil(managerUserID)
	participant.ManagerName = stringPtrOrNil(managerName)
	participant.ManagerSource = source
	participant.ManagerOverridden = true
	participant.ManagerOverrideReason = reason
	participant.ManagerConfigStatus = configStatus
	participant.UpdatedBy = operatorID

	log := &database.PerformanceRelationshipChangeLog{
		ActivityID:       participant.ActivityID,
		ParticipantID:    participant.ID,
		UserID:           participant.EmployeeID,
		ChangeType:       "assessment_manager_changed",
		FieldName:        "manager_id",
		OldValue:         formatManagerValue(oldManagerID, oldManagerName),
		NewValue:         formatManagerValue(managerUserID, managerName),
		ChangedAt:        now,
		Source:           strings.ToLower(source),
		CreatedBy:        operatorID,
		OldManagerID:     oldManagerID,
		OldManagerName:   oldManagerName,
		NewManagerID:     managerUserID,
		NewManagerName:   managerName,
		OldManagerSource: oldManagerSource,
		NewManagerSource: source,
		Reason:           reason,
		OperatorID:       operatorID,
	}
	return true, log
}

type RefreshResult struct {
	AddedCount    int `json:"added_count"`
	UpdatedCount  int `json:"updated_count"`
	InactiveCount int `json:"inactive_count"`
}

func (s *PerformanceService) RefreshParticipants(activityID, userID string) (*RefreshResult, error) {
	// 1. 获取活动信息
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, errors.New("活动不存在")
	}
	if strings.TrimSpace(activity.Status) != "draft" {
		return nil, errors.New("目标设定开启后不能增减参与人")
	}

	ctx, err := s.buildParticipantRefreshContext(s.db, activity, userID)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		return s.applyParticipantRefresh(tx, ctx)
	})
	if err != nil {
		return nil, err
	}

	return ctx.result, nil
}

type participantRefreshContext struct {
	result                    *RefreshResult
	activity                  *database.PerformanceActivity
	activityID                string
	userID                    string
	allUsers                  []database.User
	activeUserByID            map[string]database.User
	users                     []database.User
	managerAssignmentByUserID map[string]database.PerformanceActivityManagerAssignment
	deptMap                   map[string]database.Department
}

func (s *PerformanceService) buildParticipantRefreshContext(db *gorm.DB, activity *database.PerformanceActivity, userID string) (*participantRefreshContext, error) {
	if activity == nil || activity.ID == 0 {
		return nil, errors.New("活动不存在")
	}

	// 2. 获取所有在职员工（从 User 表）
	var allUsers []database.User
	if err := db.Where("status = ? AND deleted_at IS NULL", "active").Find(&allUsers).Error; err != nil {
		return nil, err
	}
	activeUserByID := make(map[string]database.User, len(allUsers))
	users := make([]database.User, 0, len(allUsers))
	for _, user := range allUsers {
		activeUserByID[user.UserID] = user
		if activityIncludesUser(activity, user) {
			users = append(users, user)
		}
	}
	managerAssignmentByUserID := activityManagerAssignmentsByUser(activity.ManagerAssignments)

	// 3. 获取部门信息映射
	var departments []database.Department
	if err := db.Where("deleted_at IS NULL").Find(&departments).Error; err != nil {
		return nil, err
	}
	deptMap := make(map[string]database.Department)
	for _, d := range departments {
		deptMap[d.DepartmentID] = d
	}

	return &participantRefreshContext{
		result:                    &RefreshResult{},
		activity:                  activity,
		activityID:                strconv.FormatUint(uint64(activity.ID), 10),
		userID:                    userID,
		allUsers:                  allUsers,
		activeUserByID:            activeUserByID,
		users:                     users,
		managerAssignmentByUserID: managerAssignmentByUserID,
		deptMap:                   deptMap,
	}, nil
}

func (s *PerformanceService) applyParticipantRefresh(tx *gorm.DB, ctx *participantRefreshContext) error {
	if ctx == nil {
		return errors.New("活动不存在")
	}

	var txParticipants []database.PerformanceParticipant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("activity_id = ? AND deleted_at IS NULL", ctx.activityID).Find(&txParticipants).Error; err != nil {
		return err
	}
	existingMap := make(map[string]*database.PerformanceParticipant)
	for i := range txParticipants {
		existingMap[txParticipants[i].EmployeeID] = &txParticipants[i]
	}

	now := time.Now()
	for _, user := range ctx.users {
		dept, hasDept := ctx.deptMap[user.DepartmentID]
		deptName := ""
		if hasDept {
			deptName = dept.Name
		}

		existing, exists := existingMap[user.UserID]
		if exists {
			changed, changeLogs := refreshPerformanceParticipantProfile(existing, user, deptName, ctx.activityID, ctx.userID, now)
			if assignment, ok := ctx.managerAssignmentByUserID[user.UserID]; ok && shouldApplyActivityManagerAssignment(existing) {
				managerChanged, managerLog := applyActivityManagerAssignment(existing, assignment, ctx.activeUserByID, ctx.userID, now)
				if managerChanged {
					changed = true
					if managerLog != nil {
						changeLogs = append(changeLogs, *managerLog)
					}
				}
			}
			if changed {
				if err := tx.Save(existing).Error; err != nil {
					return err
				}
				for _, log := range changeLogs {
					if err := tx.Create(&log).Error; err != nil {
						return err
					}
				}
				ctx.result.UpdatedCount++
			}
		} else {
			participant := s.newPerformanceParticipantForActivity(ctx.activity, ctx.userID, user, deptName)
			managerChanged, managerLog := false, (*database.PerformanceRelationshipChangeLog)(nil)
			if assignment, ok := ctx.managerAssignmentByUserID[user.UserID]; ok {
				managerChanged, managerLog = applyActivityManagerAssignment(&participant, assignment, ctx.activeUserByID, ctx.userID, now)
			}
			if err := tx.Create(&participant).Error; err != nil {
				return err
			}
			if managerChanged && managerLog != nil {
				managerLog.ParticipantID = participant.ID
				if err := tx.Create(managerLog).Error; err != nil {
					return err
				}
			}
			ctx.result.AddedCount++
		}
	}

	// 标记离职或不再属于活动范围的员工
	scopedUserIDs := make(map[string]bool)
	for _, u := range ctx.users {
		scopedUserIDs[u.UserID] = true
	}
	allActiveUserIDs := make(map[string]bool)
	for _, u := range ctx.allUsers {
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
			p.UpdatedBy = ctx.userID
			tx.Save(p)

			if err := tx.Create(&database.PerformanceRelationshipChangeLog{
				ActivityID:    ctx.activityID,
				ParticipantID: p.ID,
				ChangeType:    changeType,
				FieldName:     "status",
				OldValue:      fmt.Sprintf("%s/%s", oldStatus, oldEmployeeStatus),
				NewValue:      fmt.Sprintf("%s/%s", p.Status, p.EmployeeStatus),
				ChangedAt:     now,
				Source:        "refresh_participants",
				CreatedBy:     ctx.userID,
			}).Error; err != nil {
				return err
			}
			ctx.result.InactiveCount++
		}
	}

	return nil
}

func newPerformanceParticipantFromUser(activityID, operatorID string, user database.User, deptName string) database.PerformanceParticipant {
	participant := database.PerformanceParticipant{
		ActivityID:          activityID,
		EmployeeID:          user.UserID,
		EmployeeName:        user.Name,
		DepartmentID:        user.DepartmentID,
		DepartmentName:      deptName,
		Position:            user.Position,
		EmployeeStatus:      user.Status,
		Status:              "pending",
		CreatedBy:           operatorID,
		UpdatedBy:           operatorID,
		ManagerSource:       ManagerSourceEmpty,
		ManagerConfigStatus: ManagerConfigPending,
	}
	managerUserID, managerName := resolveManagerInfo(user)
	participant.DirectManagerIDSnapshot = stringPtrOrNil(managerUserID)
	participant.DirectManagerNameSnapshot = stringPtrOrNil(managerName)
	participant.ManagerOverridden = false
	if managerUserID != "" {
		participant.ManagerID = &managerUserID
		participant.ManagerName = &managerName
		participant.ManagerSource = ManagerSourceDirectManager
		participant.ManagerConfigStatus = ManagerConfigConfigured
	}
	return participant
}

func (s *PerformanceService) newPerformanceParticipantForActivity(activity *database.PerformanceActivity, operatorID string, user database.User, deptName string) database.PerformanceParticipant {
	activityID := ""
	defaultSource := ManagerSourceDirectManager
	if activity != nil {
		activityID = strconv.FormatUint(uint64(activity.ID), 10)
		if normalized, err := normalizeDefaultAssessmentManagerSource(activity.DefaultAssessmentManagerSource); err == nil {
			defaultSource = normalized
		}
	}
	participant := newPerformanceParticipantFromUser(activityID, operatorID, user, deptName)
	if activity != nil {
		participant.SnapshotSource = strings.TrimSpace(activity.SnapshotSource)
		participant.SnapshotAsOfDate = strings.TrimSpace(activity.SnapshotAsOfDate)
	}
	if strings.TrimSpace(participant.SnapshotSource) == "" {
		participant.SnapshotSource = "current_user"
	}
	participant.ManagerID = nil
	participant.ManagerName = nil
	participant.ManagerSource = ManagerSourceEmpty
	participant.ManagerConfigStatus = ManagerConfigPending

	managerUserID, managerName, source := s.resolveDefaultAssessmentManagerForParticipant(user, participant.DepartmentID, defaultSource)
	if strings.TrimSpace(managerUserID) != "" {
		participant.ManagerID = stringPtrOrNil(managerUserID)
		participant.ManagerName = stringPtrOrNil(managerName)
		participant.ManagerSource = source
		participant.ManagerConfigStatus = ManagerConfigConfigured
	}
	return participant
}

func (s *PerformanceService) resolveDefaultAssessmentManagerForParticipant(user database.User, departmentID, source string) (string, string, string) {
	selfUserID := strings.TrimSpace(user.UserID)
	switch source {
	case ManagerSourceDirectManager:
		managerUserID, managerName := resolveManagerInfo(user)
		if strings.TrimSpace(managerUserID) == "" || strings.TrimSpace(managerUserID) == selfUserID {
			return "", "", ManagerSourceEmpty
		}
		return strings.TrimSpace(managerUserID), strings.TrimSpace(managerName), ManagerSourceDirectManager
	case ManagerSourceDepartmentHead, ManagerSourceCenterHead:
		for _, candidate := range s.departmentManagerCandidateIDs(departmentID) {
			candidateUserID := strings.TrimSpace(candidate.userID)
			normalized, err := normalizeAssessmentManagerSource(candidate.source)
			if err != nil || normalized != source || candidateUserID == "" || candidateUserID == selfUserID {
				continue
			}
			managerName := s.displayNameForUser(candidateUserID)
			return candidateUserID, strings.TrimSpace(managerName), source
		}
	}
	return "", "", ManagerSourceEmpty
}

func refreshPerformanceParticipantProfile(existing *database.PerformanceParticipant, user database.User, deptName, activityID, operatorID string, now time.Time) (bool, []database.PerformanceRelationshipChangeLog) {
	if existing == nil {
		return false, nil
	}
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
			CreatedBy:     operatorID,
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
			CreatedBy:     operatorID,
		})
		existing.DepartmentID = user.DepartmentID
		existing.DepartmentName = deptName
		changed = true
	}

	if existing.EmployeeName != user.Name {
		existing.EmployeeName = user.Name
		changed = true
	}
	if existing.Position != user.Position {
		existing.Position = user.Position
		changed = true
	}
	if changed {
		existing.UpdatedBy = operatorID
	}
	return changed, changeLogs
}

func (s *PerformanceService) ListParticipants(activityID string, page, pageSize int, departmentID, managerID, status, employeeKeyword string, scope *OrgDataScope) ([]database.PerformanceParticipant, int64, error) {
	var visibleDepartmentIDs []string
	var visibleUserIDs []string
	if scope != nil && !scope.IsAll() {
		if scope.IsSelf() {
			visibleUserIDs = scope.UserIDs
		} else {
			visibleDepartmentIDs = scope.DepartmentIDs
		}
	}
	items, total, err := s.participantR.FindAll(activityID, page, pageSize, departmentID, managerID, status, employeeKeyword, visibleDepartmentIDs, visibleUserIDs)
	if err != nil {
		return nil, 0, err
	}
	s.hydrateManagerConfigStatus(items)
	return items, total, nil
}

func (s *PerformanceService) hydrateManagerConfigStatus(items []database.PerformanceParticipant) {
	managerIDs := make([]string, 0, len(items))
	for _, item := range items {
		if managerID := ptrStringValue(item.ManagerID); managerID != "" {
			managerIDs = append(managerIDs, managerID)
		}
	}
	managerIDs = uniqueStrings(managerIDs)
	activeManagers := make(map[string]struct{}, len(managerIDs))
	if len(managerIDs) > 0 {
		var users []database.User
		if err := s.scopedDB().Where("user_id IN ? AND status = ? AND deleted_at IS NULL", managerIDs, "active").Find(&users).Error; err == nil {
			for _, user := range users {
				activeManagers[user.UserID] = struct{}{}
			}
		}
	}
	for i := range items {
		managerID := ptrStringValue(items[i].ManagerID)
		if managerID == "" {
			items[i].ManagerConfigStatus = ManagerConfigPending
			items[i].ManagerSource = ManagerSourceEmpty
		} else if managerID == strings.TrimSpace(items[i].EmployeeID) {
			if _, ok := activeManagers[managerID]; ok && s.participantUsesSelfFinalAssessment(&items[i]) {
				items[i].ManagerConfigStatus = ManagerConfigConfigured
			} else {
				items[i].ManagerConfigStatus = ManagerConfigInvalid
			}
		} else {
			if _, ok := activeManagers[managerID]; ok {
				items[i].ManagerConfigStatus = ManagerConfigConfigured
			} else {
				items[i].ManagerConfigStatus = ManagerConfigInvalid
			}
		}
	}
}

func performanceTemplateIDForValidation(template *database.PerformanceTemplate, fallback *uint) *uint {
	if template == nil {
		return fallback
	}
	templateID := template.ID
	return &templateID
}

func (s *PerformanceService) validateActivityIndicatorLibraryCycle(indicatorLibraryID *uint, cycleType string) error {
	return s.validateActivityIndicatorLibrary(indicatorLibraryID, cycleType, nil)
}

func (s *PerformanceService) validateActivityIndicatorLibrary(indicatorLibraryID *uint, cycleType string, templateID *uint) error {
	if indicatorLibraryID == nil {
		return nil
	}

	var library database.PerformanceIndicatorLibrary
	if err := s.scopedDB().Where("id = ? AND deleted_at IS NULL", *indicatorLibraryID).First(&library).Error; err != nil {
		return fmt.Errorf("指标库不存在: %w", err)
	}

	libraryCycle := strings.TrimSpace(library.DefaultCycle)
	if libraryCycle == "" {
		return fmt.Errorf("指标库 %s 未配置默认周期，不能关联到绩效活动", library.Name)
	}
	if libraryCycle != strings.TrimSpace(cycleType) {
		return fmt.Errorf("指标库周期与活动周期不一致：活动周期为 %s，指标库周期为 %s", cycleType, libraryCycle)
	}
	if templateID == nil || *templateID == 0 {
		if library.TemplateID != nil && *library.TemplateID > 0 {
			return fmt.Errorf("请先选择绩效流程模板，再关联指标库")
		}
		return nil
	}
	if library.TemplateID == nil || *library.TemplateID == 0 {
		return fmt.Errorf("指标库 %s 未绑定绩效流程模板，请先在指标库维护中选择流程模板", library.Name)
	}
	if *library.TemplateID != *templateID {
		return fmt.Errorf("指标库所属流程模板与活动流程模板不一致，请选择同一流程模板下的指标库")
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

type AssessmentManagerUpdateRequest struct {
	ParticipantID uint
	ManagerUserID string
	ManagerSource string
	Reason        string
}

type AssessmentManagerBatchResult struct {
	ParticipantID uint                             `json:"participant_id"`
	Success       bool                             `json:"success"`
	Error         string                           `json:"error,omitempty"`
	Participant   *database.PerformanceParticipant `json:"participant,omitempty"`
}

type AssessmentManagerCandidate struct {
	UserID               string `json:"user_id"`
	Name                 string `json:"name"`
	EmployeeNo           string `json:"employee_no"`
	DepartmentName       string `json:"department_name"`
	CandidateSource      string `json:"candidate_source"`
	CandidateSourceLabel string `json:"candidate_source_label"`
	IsSelfFinalCandidate bool   `json:"is_self_final_candidate,omitempty"`
}

type AssessmentManagerCandidateSourceGroup struct {
	Source      string                       `json:"source"`
	SourceLabel string                       `json:"source_label"`
	Items       []AssessmentManagerCandidate `json:"items"`
	Reason      string                       `json:"reason,omitempty"`
}

func (s *PerformanceService) UpdateParticipantAssessmentManager(participantID uint, managerUserID, managerSource, reason, operatorID string) (*database.PerformanceParticipant, error) {
	if participantID == 0 {
		return nil, errors.New("参与人不能为空")
	}
	managerUserID = strings.TrimSpace(managerUserID)
	if managerUserID == "" {
		return nil, errors.New("考核上级不能为空")
	}
	normalizedSource, err := normalizeAssessmentManagerSource(managerSource)
	if err != nil {
		return nil, err
	}
	if normalizedSource == ManagerSourceImport || normalizedSource == ManagerSourceSystem || normalizedSource == ManagerSourceEmpty {
		return nil, fmt.Errorf("%s 不能在调整入口手动选择", assessmentManagerSourceLabel(normalizedSource))
	}
	operatorID = strings.TrimSpace(operatorID)
	if operatorID == "" {
		operatorID = "system"
	}

	var updated database.PerformanceParticipant
	err = s.db.Transaction(func(tx *gorm.DB) error {
		manager, err := s.findActiveUserByUserIDWithDB(tx, managerUserID)
		if err != nil {
			return err
		}

		var participant database.PerformanceParticipant
		if err := s.scopeOrg(tx.Clauses(clause.Locking{Strength: "UPDATE"}), "org_id").Where("id = ? AND deleted_at IS NULL", participantID).First(&participant).Error; err != nil {
			return errors.New("参与人不存在")
		}
		if stringSetContains(s.participantSelfUserIDs(&participant), manager.UserID) {
			if normalizedSource != ManagerSourceManual {
				return errors.New("本人自评即终评需使用手动指定来源")
			}
			if !s.participantCanUseSelfFinalAssessment(&participant) {
				return errors.New("只有最高级或无可用组织上级人员可设置本人为自评即终评")
			}
		}
		if participant.IsLocked || participant.Status == "locked" || participant.Status == "hr_confirmed" {
			return errors.New("绩效结果已锁定，无法调整考核上级")
		}

		if !s.assessmentManagerSourceAllowsUser(&participant, manager.UserID, normalizedSource) {
			return fmt.Errorf("考核上级与来源 %s 不匹配", assessmentManagerSourceLabel(normalizedSource))
		}

		oldManagerID := ptrStringValue(participant.ManagerID)
		oldManagerName := ptrStringValue(participant.ManagerName)
		oldManagerSource := strings.TrimSpace(participant.ManagerSource)
		if oldManagerSource == "" {
			oldManagerSource = ManagerSourceSystem
		}

		newManagerName := strings.TrimSpace(manager.Name)
		participant.ManagerID = stringPtrOrNil(manager.UserID)
		participant.ManagerName = stringPtrOrNil(newManagerName)
		participant.ManagerSource = normalizedSource
		participant.ManagerOverridden = true
		participant.ManagerOverrideReason = strings.TrimSpace(reason)
		participant.ManagerConfigStatus = ManagerConfigConfigured
		participant.UpdatedBy = operatorID

		if err := tx.Save(&participant).Error; err != nil {
			return err
		}
		if participant.Status == "self_submitted" && s.participantUsesSelfFinalAssessmentWithDB(tx, &participant) {
			var activity database.PerformanceActivity
			if err := s.scopeOrg(tx, "org_id").Where("id = ? AND deleted_at IS NULL", participant.ActivityID).First(&activity).Error; err != nil {
				return err
			}
			if err := s.applySelfFinalAssessmentWithDB(tx, &participant, &activity, operatorID); err != nil {
				return err
			}
		}
		log := database.PerformanceRelationshipChangeLog{
			ActivityID:       participant.ActivityID,
			ParticipantID:    participant.ID,
			UserID:           participant.EmployeeID,
			ChangeType:       "assessment_manager_changed",
			FieldName:        "manager_id",
			OldValue:         formatManagerValue(oldManagerID, oldManagerName),
			NewValue:         formatManagerValue(manager.UserID, newManagerName),
			ChangedAt:        time.Now(),
			Source:           strings.ToLower(normalizedSource),
			CreatedBy:        operatorID,
			OldManagerID:     oldManagerID,
			OldManagerName:   oldManagerName,
			NewManagerID:     manager.UserID,
			NewManagerName:   newManagerName,
			OldManagerSource: oldManagerSource,
			NewManagerSource: normalizedSource,
			Reason:           strings.TrimSpace(reason),
			OperatorID:       operatorID,
			OperatorName:     s.displayNameForUser(operatorID),
		}
		if log.OperatorName == "" {
			log.OperatorName = operatorID
		}
		if err := tx.Create(&log).Error; err != nil {
			return err
		}
		updated = participant
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (s *PerformanceService) BatchUpdateActivityAssessmentManagers(activityID string, items []AssessmentManagerUpdateRequest, operatorID string) ([]AssessmentManagerBatchResult, error) {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("活动不能为空")
	}
	if len(items) == 0 {
		return nil, errors.New("请至少选择一个参与人")
	}
	if _, err := s.actRepo.GetByID(activityID); err != nil {
		return nil, errors.New("活动不存在")
	}

	results := make([]AssessmentManagerBatchResult, 0, len(items))
	for _, item := range items {
		result := AssessmentManagerBatchResult{ParticipantID: item.ParticipantID}
		if item.ParticipantID == 0 {
			result.Error = "参与人不能为空"
			results = append(results, result)
			continue
		}

		participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(item.ParticipantID), 10))
		if err != nil {
			result.Error = "参与人不存在"
			results = append(results, result)
			continue
		}
		if participant.ActivityID != activityID {
			result.Error = "参与人不属于当前活动"
			results = append(results, result)
			continue
		}

		updated, err := s.UpdateParticipantAssessmentManager(item.ParticipantID, item.ManagerUserID, item.ManagerSource, item.Reason, operatorID)
		if err != nil {
			result.Error = err.Error()
		} else {
			result.Success = true
			result.Participant = updated
		}
		results = append(results, result)
	}
	return results, nil
}

func (s *PerformanceService) assessmentManagerSourceAllowsUser(participant *database.PerformanceParticipant, managerUserID, source string) bool {
	if participant == nil {
		return false
	}
	managerUserID = strings.TrimSpace(managerUserID)
	if managerUserID == "" {
		return false
	}
	source, err := normalizeAssessmentManagerSource(source)
	if err != nil {
		return false
	}

	if ptrStringValue(participant.ManagerID) == managerUserID {
		if currentSource, err := normalizeAssessmentManagerSource(participant.ManagerSource); err == nil && currentSource == source {
			return true
		}
	}

	switch source {
	case ManagerSourceManual, ManagerSourceImport, ManagerSourceSystem:
		return true
	case ManagerSourceDirectManager:
		if ptrStringValue(participant.DirectManagerIDSnapshot) == managerUserID {
			return true
		}
		var employee database.User
		if err := s.scopedDB().Where("user_id = ? AND deleted_at IS NULL", participant.EmployeeID).First(&employee).Error; err == nil {
			directManagerUserID, _ := resolveManagerInfo(employee)
			return strings.TrimSpace(directManagerUserID) == managerUserID
		}
		return false
	case ManagerSourceDepartmentHead, ManagerSourceCenterHead:
		for _, scoped := range s.departmentManagerCandidateIDs(participant.DepartmentID) {
			scopedSource, err := normalizeAssessmentManagerSource(scoped.source)
			if err == nil && scopedSource == source && strings.TrimSpace(scoped.userID) == managerUserID {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (s *PerformanceService) activeUserIDExistsWithDB(db *gorm.DB, userID string) bool {
	userID = strings.TrimSpace(userID)
	if db == nil || userID == "" {
		return false
	}
	var user database.User
	if err := db.Select("user_id").Where("user_id = ? AND status = ? AND deleted_at IS NULL", userID, "active").First(&user).Error; err != nil {
		return false
	}
	return strings.TrimSpace(user.UserID) == userID
}

func (s *PerformanceService) participantCanUseSelfFinalAssessment(participant *database.PerformanceParticipant) bool {
	return s.participantCanUseSelfFinalAssessmentWithDB(s.db, participant)
}

func (s *PerformanceService) participantCanUseSelfFinalAssessmentWithDB(db *gorm.DB, participant *database.PerformanceParticipant) bool {
	if participant == nil {
		return false
	}
	selfUserIDs := s.participantSelfUserIDs(participant)
	if len(selfUserIDs) == 0 {
		return false
	}

	orgManagerIDs := make(map[string]struct{})
	addOrgManagerID := func(userID string) {
		userID = strings.TrimSpace(userID)
		if userID == "" || stringSetContains(selfUserIDs, userID) {
			return
		}
		orgManagerIDs[userID] = struct{}{}
	}

	addOrgManagerID(ptrStringValue(participant.DirectManagerIDSnapshot))
	var employee database.User
	if db != nil {
		if err := db.Where("user_id = ? AND deleted_at IS NULL", participant.EmployeeID).First(&employee).Error; err == nil {
			managerUserID, _ := resolveManagerInfo(employee)
			addOrgManagerID(managerUserID)
		}
	}
	for _, scoped := range s.departmentManagerCandidateIDs(participant.DepartmentID) {
		addOrgManagerID(scoped.userID)
	}

	for userID := range orgManagerIDs {
		if s.activeUserIDExistsWithDB(db, userID) {
			return false
		}
	}
	return true
}

func (s *PerformanceService) participantUsesSelfFinalAssessment(participant *database.PerformanceParticipant) bool {
	return s.participantUsesSelfFinalAssessmentWithDB(s.db, participant)
}

func (s *PerformanceService) participantUsesSelfFinalAssessmentWithDB(db *gorm.DB, participant *database.PerformanceParticipant) bool {
	if participant == nil {
		return false
	}
	if strings.TrimSpace(participant.ManagerSource) != ManagerSourceManual {
		return false
	}
	managerID := ptrStringValue(participant.ManagerID)
	if managerID == "" || !stringSetContains(s.participantSelfUserIDs(participant), managerID) {
		return false
	}
	return s.participantCanUseSelfFinalAssessmentWithDB(db, participant)
}

func (s *PerformanceService) applySelfFinalAssessmentWithDB(tx *gorm.DB, participant *database.PerformanceParticipant, activity *database.PerformanceActivity, operatorID string) error {
	if tx == nil || participant == nil || activity == nil {
		return nil
	}
	if participant.Status != "self_submitted" || !s.participantUsesSelfFinalAssessmentWithDB(tx, participant) {
		return nil
	}

	var records []database.PerformanceGoalRecord
	if err := tx.Where("participant_id = ? AND activity_id = ? AND deleted_at IS NULL", participant.ID, participant.ActivityID).Find(&records).Error; err != nil {
		return err
	}

	totalManagerScore := 0.0
	bonusTotal := 0.0
	scorableCount := 0
	for i := range records {
		if !isScorableGoalRecordForActivity(activity, records[i]) {
			continue
		}
		scorableCount++
		records[i].ManagerScore = records[i].SelfScore
		if records[i].SectionType == "bonus_penalty" {
			records[i].BonusScore = records[i].SelfScore
			bonusTotal += records[i].SelfScore
		} else if isReviewGoalRecordForActivity(activity, records[i]) {
			totalManagerScore += records[i].ManagerScore * records[i].Weight
		}
		if err := tx.Save(&records[i]).Error; err != nil {
			return err
		}
	}
	if scorableCount == 0 {
		totalManagerScore = participant.TotalSelfScore
		if totalManagerScore == 0 {
			totalManagerScore = participant.SelfScore
		}
	}
	totalManagerScore = roundScore(totalManagerScore)

	if activity.EnableBonusScore {
		participant.BonusScore = roundScore(bonusTotal)
	}
	adjustedScore := totalManagerScore + participant.BonusScore - participant.PenaltyScore
	if adjustedScore < 0 {
		adjustedScore = 0
	}

	autoLevel := PerformanceLevelByActivity(totalManagerScore, activity)
	if activity.EnableBonusScore {
		autoLevel = PerformanceLevelByActivity(adjustedScore, activity)
	}

	participant.ManagerScore = totalManagerScore
	participant.TotalManagerScore = totalManagerScore
	participant.AdjustedScore = roundScore(adjustedScore)
	participant.SuggestedLevel = autoLevel
	if participant.FinalLevel == "" || participant.FinalLevel == participant.SuggestedLevel || participant.AdjustReason == "" {
		participant.FinalLevel = autoLevel
	}
	participant.ManagerComment = participant.SelfSummary
	participant.ManagerEvaluationGood = participant.SelfEvaluationGood
	participant.ManagerEvaluationImprovement = participant.SelfEvaluationImprovement
	participant.Status = "manager_submitted"
	participant.UpdatedBy = operatorID
	if err := tx.Save(participant).Error; err != nil {
		return err
	}

	version := &database.PerformanceReviewVersion{
		OrgID:          strings.TrimSpace(participant.OrgID),
		ParticipantID:  participant.ID,
		ActivityID:     participant.ActivityID,
		ReviewType:     "manager",
		ManagerScore:   totalManagerScore,
		SuggestedLevel: participant.SuggestedLevel,
		ManagerComment: participant.ManagerComment,
		FinalLevel:     participant.FinalLevel,
		CreatedBy:      operatorID,
		OperationMeta: map[string]interface{}{
			"self_final":     true,
			"source":         "self_evaluation",
			"adjusted_score": participant.AdjustedScore,
			"bonus_score":    participant.BonusScore,
		},
	}
	return tx.Create(version).Error
}

func (s *PerformanceService) syncSelfFinalAssessmentsForActivity(activityID, operatorID string) error {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return errors.New("活动不能为空")
	}
	operatorID = strings.TrimSpace(operatorID)
	if operatorID == "" {
		operatorID = "system"
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		var activity database.PerformanceActivity
		if err := s.scopeOrg(tx, "org_id").Where("id = ? AND deleted_at IS NULL", activityID).First(&activity).Error; err != nil {
			return err
		}

		var participants []database.PerformanceParticipant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("activity_id = ? AND status = ? AND deleted_at IS NULL", activityID, "self_submitted").
			Find(&participants).Error; err != nil {
			return err
		}
		for i := range participants {
			if err := s.applySelfFinalAssessmentWithDB(tx, &participants[i], &activity, operatorID); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *PerformanceService) participantCompletedStageForFlow(participant database.PerformanceParticipant, stage string) bool {
	if participantCompletedStage(participant.Status, stage) {
		return true
	}
	return stage == "manager_evaluation" &&
		participant.Status == "self_submitted" &&
		s.participantUsesSelfFinalAssessment(&participant)
}

func (s *PerformanceService) participantCanUseSelfFinalAssessmentByID(participantID uint) bool {
	if participantID == 0 {
		return false
	}
	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return false
	}
	return s.participantCanUseSelfFinalAssessment(participant)
}

func (s *PerformanceService) ListAssessmentManagerCandidates(activityID string, participantID uint, sourceFilter string, keyword string, limit int) ([]AssessmentManagerCandidate, error) {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return nil, errors.New("活动不能为空")
	}
	if _, err := s.actRepo.GetByID(activityID); err != nil {
		return nil, errors.New("活动不存在")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	normalizedSourceFilter := ""
	if strings.TrimSpace(sourceFilter) != "" {
		normalized, err := normalizeAssessmentManagerSource(sourceFilter)
		if err != nil {
			return nil, err
		}
		normalizedSourceFilter = normalized
	}

	type pendingCandidate struct {
		userID               string
		source               string
		isSelfFinalCandidate bool
	}
	excludedUserIDs := map[string]struct{}{}
	candidateByUserID := make(map[string]string)
	orderedCandidates := make([]pendingCandidate, 0)
	addCandidate := func(userID, source string) {
		userID = strings.TrimSpace(userID)
		if userID == "" {
			return
		}
		if stringSetContains(excludedUserIDs, userID) {
			return
		}
		source, err := normalizeAssessmentManagerSource(source)
		if err != nil {
			source = ManagerSourceManual
		}
		if normalizedSourceFilter != "" && source != normalizedSourceFilter {
			return
		}
		if _, exists := candidateByUserID[userID]; exists {
			return
		}
		candidateByUserID[userID] = source
		orderedCandidates = append(orderedCandidates, pendingCandidate{userID: userID, source: source})
	}
	addSelfFinalCandidate := func(userID string) {
		userID = strings.TrimSpace(userID)
		if userID == "" || (normalizedSourceFilter != "" && normalizedSourceFilter != ManagerSourceManual) {
			return
		}
		if _, exists := candidateByUserID[userID]; exists {
			return
		}
		candidateByUserID[userID] = ManagerSourceManual
		orderedCandidates = append(orderedCandidates, pendingCandidate{
			userID:               userID,
			source:               ManagerSourceManual,
			isSelfFinalCandidate: true,
		})
	}

	var participant *database.PerformanceParticipant
	selfFinalAllowed := false
	if participantID > 0 {
		loaded, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
		if err != nil {
			return nil, errors.New("参与人不存在")
		}
		if loaded.ActivityID != activityID {
			return nil, errors.New("参与人不属于当前活动")
		}
		participant = loaded
		excludedUserIDs = s.participantSelfUserIDs(participant)
		selfFinalAllowed = s.participantCanUseSelfFinalAssessment(participant)
		addCandidate(ptrStringValue(participant.ManagerID), participant.ManagerSource)
		addCandidate(ptrStringValue(participant.DirectManagerIDSnapshot), ManagerSourceDirectManager)

		var employee database.User
		if err := s.scopedDB().Where("user_id = ? AND deleted_at IS NULL", participant.EmployeeID).First(&employee).Error; err == nil {
			managerUserID, _ := resolveManagerInfo(employee)
			addCandidate(managerUserID, ManagerSourceDirectManager)
		}
		for _, scoped := range s.departmentManagerCandidateIDs(participant.DepartmentID) {
			addCandidate(scoped.userID, scoped.source)
		}
	}

	keyword = strings.TrimSpace(keyword)
	if normalizedSourceFilter == "" || normalizedSourceFilter == ManagerSourceManual {
		shouldSearchManual := keyword != "" || participant == nil || normalizedSourceFilter == ManagerSourceManual
		if shouldSearchManual {
			manualUsers, err := s.searchActiveUsersForAssessmentManager(keyword, limit)
			if err != nil {
				return nil, err
			}
			for _, user := range manualUsers {
				if stringSetContains(excludedUserIDs, user.UserID) {
					if selfFinalAllowed {
						addSelfFinalCandidate(user.UserID)
					}
					continue
				}
				addCandidate(user.UserID, ManagerSourceManual)
			}
		}
	}

	if len(orderedCandidates) == 0 {
		return []AssessmentManagerCandidate{}, nil
	}
	candidateUserIDs := make([]string, 0, len(orderedCandidates))
	for _, candidate := range orderedCandidates {
		candidateUserIDs = append(candidateUserIDs, candidate.userID)
	}

	var users []database.User
	if err := s.scopedDB().Where("user_id IN ? AND status = ? AND deleted_at IS NULL", candidateUserIDs, "active").Find(&users).Error; err != nil {
		return nil, err
	}
	userByID := make(map[string]database.User)
	departmentIDs := make(map[string]struct{})
	for _, user := range users {
		userByID[user.UserID] = user
		if strings.TrimSpace(user.DepartmentID) != "" {
			departmentIDs[user.DepartmentID] = struct{}{}
		}
	}

	var profiles []database.EmployeeProfile
	if err := s.scopedDB().Where("user_id IN ? AND deleted_at IS NULL", candidateUserIDs).Find(&profiles).Error; err != nil {
		return nil, err
	}
	employeeNoByUserID := make(map[string]string)
	for _, profile := range profiles {
		employeeNoByUserID[profile.UserID] = strings.TrimSpace(profile.EmployeeID)
	}

	departmentNames := make(map[string]string)
	if len(departmentIDs) > 0 {
		ids := make([]string, 0, len(departmentIDs))
		for id := range departmentIDs {
			ids = append(ids, id)
		}
		var departments []database.Department
		if err := s.scopedDB().Where("department_id IN ? AND deleted_at IS NULL", ids).Find(&departments).Error; err != nil {
			return nil, err
		}
		for _, department := range departments {
			departmentNames[department.DepartmentID] = department.Name
		}
	}

	candidates := make([]AssessmentManagerCandidate, 0, len(orderedCandidates))
	for _, candidate := range orderedCandidates {
		user, ok := userByID[candidate.userID]
		if !ok {
			continue
		}
		candidates = append(candidates, AssessmentManagerCandidate{
			UserID:               user.UserID,
			Name:                 user.Name,
			EmployeeNo:           employeeNoByUserID[user.UserID],
			DepartmentName:       departmentNames[user.DepartmentID],
			CandidateSource:      candidate.source,
			CandidateSourceLabel: assessmentManagerSourceLabel(candidate.source),
			IsSelfFinalCandidate: candidate.isSelfFinalCandidate,
		})
		if len(candidates) >= limit {
			break
		}
	}
	return candidates, nil
}

func (s *PerformanceService) ListAssessmentManagerCandidateSourceGroups(activityID string, participantID uint, keyword string, limit int) ([]AssessmentManagerCandidateSourceGroup, error) {
	sources := []string{
		ManagerSourceDirectManager,
		ManagerSourceDepartmentHead,
		ManagerSourceCenterHead,
		ManagerSourceManual,
	}
	groups := make([]AssessmentManagerCandidateSourceGroup, 0, len(sources))
	for _, source := range sources {
		items, err := s.ListAssessmentManagerCandidates(activityID, participantID, source, keyword, limit)
		if err != nil {
			return nil, err
		}
		group := AssessmentManagerCandidateSourceGroup{
			Source:      source,
			SourceLabel: assessmentManagerSourceLabel(source),
			Items:       items,
		}
		if len(items) == 0 {
			group.Reason = s.assessmentManagerCandidateMissingReason(activityID, participantID, source, keyword)
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func (s *PerformanceService) assessmentManagerCandidateOnlySelf(participantID uint, source string) bool {
	if participantID == 0 {
		return false
	}
	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return false
	}
	selfUserIDs := s.participantSelfUserIDs(participant)
	if len(selfUserIDs) == 0 {
		return false
	}
	normalizedSource, err := normalizeAssessmentManagerSource(source)
	if err != nil {
		return false
	}
	switch normalizedSource {
	case ManagerSourceDirectManager:
		if stringSetContains(selfUserIDs, ptrStringValue(participant.DirectManagerIDSnapshot)) {
			return true
		}
		var employee database.User
		if err := s.scopedDB().Where("user_id = ? AND deleted_at IS NULL", participant.EmployeeID).First(&employee).Error; err == nil {
			managerUserID, _ := resolveManagerInfo(employee)
			return stringSetContains(selfUserIDs, managerUserID)
		}
	case ManagerSourceDepartmentHead, ManagerSourceCenterHead:
		foundSelf := false
		foundOther := false
		for _, candidate := range s.departmentManagerCandidateIDs(participant.DepartmentID) {
			candidateSource, err := normalizeAssessmentManagerSource(candidate.source)
			if err != nil || candidateSource != normalizedSource {
				continue
			}
			candidateUserID := strings.TrimSpace(candidate.userID)
			if candidateUserID == "" {
				continue
			}
			if stringSetContains(selfUserIDs, candidateUserID) {
				foundSelf = true
			} else {
				foundOther = true
			}
		}
		return foundSelf && !foundOther
	}
	return false
}

func (s *PerformanceService) assessmentManagerCandidateMissingReason(activityID string, participantID uint, source string, keyword string) string {
	switch source {
	case ManagerSourceDirectManager:
		if participantID == 0 {
			return "未指定参与人，无法读取直属主管。"
		}
		participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
		if err != nil {
			return "参与人不存在。"
		}
		if ptrStringValue(participant.DirectManagerIDSnapshot) == "" {
			return "该参与人在活动创建时没有直属主管快照，且当前员工档案未配置直属主管。"
		}
		if s.assessmentManagerCandidateOnlySelf(participantID, source) {
			if s.participantCanUseSelfFinalAssessmentByID(participantID) {
				return "直属主管候选人为员工本人。最高级或无可用组织上级人员可切换为手动指定并选择本人，按自评即终评处理。"
			}
			return "直属主管配置为员工本人，普通员工不能把本人设置为考核上级。请手动指定其他在职考核人。"
		}
		return "直属主管不存在、已离职或不在可选范围。"
	case ManagerSourceDepartmentHead:
		if s.assessmentManagerCandidateOnlySelf(participantID, source) {
			if s.participantCanUseSelfFinalAssessmentByID(participantID) {
				return "部门负责人候选人为员工本人。最高级或无可用组织上级人员可切换为手动指定并选择本人，按自评即终评处理。"
			}
			return "部门负责人候选人为员工本人，普通员工不能把本人设置为考核上级。请手动指定其他在职考核人。"
		}
		return "部门负责人未配置。请在部门扩展配置或钉钉部门负责人同步结果中维护 department_head_user_id。"
	case ManagerSourceCenterHead:
		if s.assessmentManagerCandidateOnlySelf(participantID, source) {
			if s.participantCanUseSelfFinalAssessmentByID(participantID) {
				return "中心负责人候选人为员工本人。最高级或无可用组织上级人员可切换为手动指定并选择本人，按自评即终评处理。"
			}
			return "中心负责人候选人为员工本人，普通员工不能把本人设置为考核上级。请手动指定其他在职考核人。"
		}
		return "中心负责人未配置。当前项目未硬编码中心负责人来源，可通过 departments.extension.center_head_user_id 等扩展字段维护。"
	case ManagerSourceManual:
		if strings.TrimSpace(keyword) == "" {
			return "请输入姓名、工号或手机号搜索员工。"
		}
		if s.manualAssessmentManagerOnlyMatchesParticipantSelf(participantID, keyword, 20) {
			if s.participantCanUseSelfFinalAssessmentByID(participantID) {
				return "搜索结果仅匹配调整对象本人。最高级或无可用组织上级人员可选择本人，按自评即终评处理。"
			}
			return "搜索结果仅匹配调整对象本人。只有最高级或无可用组织上级人员可选择本人自评即终评；普通员工请指定其他在职考核人。"
		}
		return "没有匹配的在职员工。"
	default:
		return "该来源没有可用候选人。"
	}
}

func (s *PerformanceService) manualAssessmentManagerOnlyMatchesParticipantSelf(participantID uint, keyword string, limit int) bool {
	keyword = strings.TrimSpace(keyword)
	if participantID == 0 || keyword == "" {
		return false
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}

	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return false
	}
	selfUserIDs := s.participantSelfUserIDs(participant)
	if len(selfUserIDs) == 0 {
		return false
	}
	users, err := s.searchActiveUsersForAssessmentManager(keyword, limit)
	if err != nil || len(users) == 0 {
		return false
	}

	matchedSelf := false
	for _, user := range users {
		if stringSetContains(selfUserIDs, user.UserID) {
			matchedSelf = true
			continue
		}
		return false
	}
	return matchedSelf
}

type assessmentManagerScopedCandidate struct {
	userID string
	source string
}

func (s *PerformanceService) departmentManagerCandidateIDs(departmentID string) []assessmentManagerScopedCandidate {
	departmentID = strings.TrimSpace(departmentID)
	if departmentID == "" {
		return nil
	}
	candidates := make([]assessmentManagerScopedCandidate, 0)
	visited := make(map[string]struct{})
	for depth := 0; depth < 12 && departmentID != ""; depth++ {
		if _, seen := visited[departmentID]; seen {
			break
		}
		visited[departmentID] = struct{}{}

		var department database.Department
		if err := s.scopedDB().Where("department_id = ? AND deleted_at IS NULL", departmentID).First(&department).Error; err != nil {
			break
		}
		if userID := firstNonEmptyString(department.Extension,
			"assessment_manager_user_id",
			"performance_manager_user_id",
			"department_head_user_id",
			"dept_head_user_id",
			"head_user_id",
			"leader_user_id",
			"owner_user_id",
		); userID != "" {
			candidates = append(candidates, assessmentManagerScopedCandidate{userID: userID, source: ManagerSourceDepartmentHead})
		}
		if userID := firstNonEmptyString(department.Extension,
			"center_head_user_id",
			"center_leader_user_id",
			"center_manager_user_id",
		); userID != "" {
			candidates = append(candidates, assessmentManagerScopedCandidate{userID: userID, source: ManagerSourceCenterHead})
		}
		departmentID = strings.TrimSpace(department.ParentID)
	}
	return candidates
}

func (s *PerformanceService) searchActiveUsersForAssessmentManager(keyword string, limit int) ([]database.User, error) {
	query := s.scopedDB().Where("status = ? AND deleted_at IS NULL", "active")
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		like := "%" + keyword + "%"
		var profileUserIDs []string
		if err := s.db.Model(&database.EmployeeProfile{}).Where("employee_id LIKE ? AND deleted_at IS NULL", like).Pluck("user_id", &profileUserIDs).Error; err != nil {
			return nil, err
		}
		if len(profileUserIDs) > 0 {
			query = query.Where("(name LIKE ? OR user_id LIKE ? OR mobile LIKE ? OR user_id IN ?)", like, like, like, profileUserIDs)
		} else {
			query = query.Where("(name LIKE ? OR user_id LIKE ? OR mobile LIKE ?)", like, like, like)
		}
	}
	var users []database.User
	if err := query.Order("name ASC").Limit(limit).Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

func (s *PerformanceService) findActiveUserByUserIDWithDB(db *gorm.DB, userID string) (*database.User, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, errors.New("考核上级不能为空")
	}
	var user database.User
	if err := db.Where("user_id = ? AND deleted_at IS NULL", userID).First(&user).Error; err == nil {
		if strings.TrimSpace(user.Status) != "active" {
			return nil, errors.New("考核上级不存在或不是在职状态")
		}
		return &user, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if _, err := strconv.ParseUint(userID, 10, 64); err == nil {
		if err := db.Where("id = ? AND status = ? AND deleted_at IS NULL", userID, "active").First(&user).Error; err == nil {
			return &user, nil
		}
	}
	return nil, errors.New("考核上级不存在或不是在职状态")
}

func assessmentManagerSourceLabel(source string) string {
	switch strings.ToUpper(strings.TrimSpace(source)) {
	case ManagerSourceDirectManager:
		return "直属主管"
	case ManagerSourceDepartmentHead:
		return "部门负责人"
	case ManagerSourceCenterHead:
		return "中心负责人"
	case ManagerSourceImport:
		return "导入指定"
	case ManagerSourceSystem:
		return "系统兼容"
	case ManagerSourceEmpty:
		return "暂未配置"
	default:
		return "手动指定"
	}
}

func ptrStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func formatManagerValue(userID, name string) string {
	userID = strings.TrimSpace(userID)
	name = strings.TrimSpace(name)
	if userID == "" {
		return ""
	}
	if name == "" {
		return userID
	}
	return fmt.Sprintf("%s/%s", userID, name)
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
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
		activity = nil
	}

	participants, _, err := s.participantR.FindAll(activityID, 1, 5000, "", "", "", "", nil, nil)
	if err != nil {
		return nil, err
	}

	rules, err := s.ruleRepo.ListByActivity(activityID)
	if err != nil {
		return nil, err
	}

	ruleMap := performanceDistributionPercentages(activity)
	for _, rule := range rules {
		ruleMap[rule.Level] = rule.DistributionPercent
	}
	ruleMap["CD"] = ruleMap["C"] + ruleMap["D"]

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
	if _, _, err := s.validateLegacyEvaluationSubmission(participantID, "self_evaluation"); err != nil {
		return nil, err
	}
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
	if _, _, err := s.validateLegacyEvaluationSubmission(participantID, "manager_evaluation"); err != nil {
		return nil, err
	}
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
	activity, err := s.actRepo.GetByID(activityID)
	if err != nil {
		return nil, fmt.Errorf("绩效活动不存在: %w", err)
	}
	if strings.TrimSpace(activity.Status) != "manager_evaluation" {
		return nil, fmt.Errorf("当前活动状态不允许提交主管评分，活动状态为: %s", activity.Status)
	}
	if err := checkTimeWindow(activity, "manager_evaluation"); err != nil {
		return nil, err
	}
	for _, evaluation := range evaluations {
		participantID := strconv.FormatUint(uint64(evaluation.ParticipantID), 10)
		participant, err := s.participantR.GetByID(participantID)
		if err != nil {
			return nil, fmt.Errorf("参与人不存在: %w", err)
		}
		if participant.ActivityID != activityID {
			return nil, fmt.Errorf("参与人 %d 不属于当前活动", evaluation.ParticipantID)
		}
		if participant.IsLocked {
			return nil, fmt.Errorf("参与人 %d 的绩效结果已锁定，无法提交主管评分", evaluation.ParticipantID)
		}
	}
	return s.versionRepo.BatchCreateManagerEvaluationVersions(activityID, evaluations, userID)
}

func (s *PerformanceService) validateLegacyEvaluationSubmission(participantID, stage string) (*database.PerformanceParticipant, *database.PerformanceActivity, error) {
	participant, err := s.participantR.GetByID(participantID)
	if err != nil {
		return nil, nil, fmt.Errorf("参与人不存在: %w", err)
	}
	if participant.IsLocked {
		return nil, nil, fmt.Errorf("该参与人的绩效结果已锁定，无法提交评价")
	}
	activity, err := s.actRepo.GetByID(participant.ActivityID)
	if err != nil {
		return nil, nil, fmt.Errorf("绩效活动不存在: %w", err)
	}
	if strings.TrimSpace(activity.Status) != stage {
		return nil, nil, fmt.Errorf("当前活动状态不允许提交评价，活动状态为: %s", activity.Status)
	}
	if err := checkTimeWindow(activity, stage); err != nil {
		return nil, nil, err
	}
	return participant, activity, nil
}

func (s *PerformanceService) AdjustFinalLevel(participantID string, finalLevel, reason, userID string) (*database.PerformanceReviewVersion, error) {
	return s.versionRepo.AdjustFinalLevel(participantID, finalLevel, reason, userID)
}

func decodePerformanceLevelRuleConfig(config map[string]interface{}, flowType string) PerformanceLevelRuleConfig {
	rawConfig := config
	if len(rawConfig) == 0 {
		rawConfig = defaultPerformanceLevelRuleConfig(flowType)
	}
	raw, err := json.Marshal(rawConfig)
	if err != nil {
		return decodePerformanceLevelRuleConfig(defaultPerformanceLevelRuleConfig(flowType), PerformanceFlowOld)
	}
	var decoded PerformanceLevelRuleConfig
	if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Rules) == 0 {
		if normalizePerformanceFlowType(flowType) == PerformanceFlowOld && len(config) == 0 {
			return PerformanceLevelRuleConfig{Rules: []PerformanceLevelRuleConfigItem{
				{Level: "S", MinScore: floatPtr(100), MinInclusive: true, SortOrder: 1},
				{Level: "A", MinScore: floatPtr(90), MaxScore: floatPtr(100), MinInclusive: true, MaxInclusive: false, SortOrder: 2},
				{Level: "B", MinScore: floatPtr(80), MaxScore: floatPtr(90), MinInclusive: true, MaxInclusive: false, SortOrder: 3},
				{Level: "C", MinScore: floatPtr(60), MaxScore: floatPtr(80), MinInclusive: true, MaxInclusive: false, SortOrder: 4},
				{Level: "D", MaxScore: floatPtr(60), MaxInclusive: false, SortOrder: 5},
			}}
		}
		return decodePerformanceLevelRuleConfig(defaultPerformanceLevelRuleConfig(flowType), PerformanceFlowOld)
	}
	sort.SliceStable(decoded.Rules, func(i, j int) bool {
		return decoded.Rules[i].SortOrder < decoded.Rules[j].SortOrder
	})
	return decoded
}

func scoreMatchesLevelRule(score float64, rule PerformanceLevelRuleConfigItem) bool {
	if rule.MinScore != nil {
		if rule.MinInclusive {
			if score < *rule.MinScore {
				return false
			}
		} else if score <= *rule.MinScore {
			return false
		}
	}
	if rule.MaxScore != nil {
		if rule.MaxInclusive {
			if score > *rule.MaxScore {
				return false
			}
		} else if score >= *rule.MaxScore {
			return false
		}
	}
	return true
}

func PerformanceLevelByRuleConfig(score float64, config map[string]interface{}, flowType string) string {
	decoded := decodePerformanceLevelRuleConfig(config, flowType)
	for _, rule := range decoded.Rules {
		if strings.TrimSpace(rule.Level) == "" {
			continue
		}
		if scoreMatchesLevelRule(score, rule) {
			return strings.TrimSpace(rule.Level)
		}
	}
	return "D"
}

func PerformanceLevelByActivity(score float64, activity *database.PerformanceActivity) string {
	if activity == nil {
		return PerformanceLevelByScore(score)
	}
	return PerformanceLevelByRuleConfig(score, activity.LevelRuleConfig, activity.FlowType)
}

// PerformanceLevelByScore 根据旧流程 100 分制计算绩效等级。新流程请使用 PerformanceLevelByActivity。
func PerformanceLevelByScore(score float64) string {
	return PerformanceLevelByRuleConfig(score, defaultPerformanceLevelRuleConfig(PerformanceFlowOld), PerformanceFlowOld)
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

func (s *PerformanceService) findPreviousPlanActivity(activity *database.PerformanceActivity) (*database.PerformanceActivity, error) {
	if activity == nil || activity.ID == 0 || !isNewPerformanceFlow(activity) {
		return nil, nil
	}
	if activity.PreviousReviewActivityID != nil && *activity.PreviousReviewActivityID > 0 {
		previous, err := s.actRepo.GetByID(strconv.FormatUint(uint64(*activity.PreviousReviewActivityID), 10))
		if err != nil {
			return nil, errors.New("上一期绩效活动不存在")
		}
		return previous, nil
	}

	var previous database.PerformanceActivity
	query := s.scopedDB().Where(
		"deleted_at IS NULL AND id <> ? AND flow_type = ? AND cycle_type = ?",
		activity.ID,
		PerformanceFlowNew,
		activity.CycleType,
	)
	if strings.TrimSpace(activity.StartDate) != "" {
		query = query.Where("end_date < ?", strings.TrimSpace(activity.StartDate))
	}
	if strings.TrimSpace(activity.OrganizationID) != "" {
		query = query.Where("organization_id = ?", strings.TrimSpace(activity.OrganizationID))
	}
	if err := query.Order("end_date DESC, id DESC").First(&previous).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &previous, nil
}

func clonePlanRecordAsReview(source database.PerformanceGoalRecord, currentActivityID string, currentParticipantID uint, now time.Time) database.PerformanceGoalRecord {
	return database.PerformanceGoalRecord{
		ActivityID:      currentActivityID,
		ParticipantID:   currentParticipantID,
		IndicatorItemID: source.IndicatorItemID,
		SectionType:     source.SectionType,
		GoalPhase:       PerformanceGoalPhaseReview,
		GoalType:        source.GoalType,
		FixedKey:        source.FixedKey,
		IsFixed:         source.IsFixed,
		ItemName:        source.ItemName,
		ItemDefinition:  source.ItemDefinition,
		Weight:          source.Weight,
		RedLineValue:    source.RedLineValue,
		TargetValue:     source.TargetValue,
		ChallengeValue:  source.ChallengeValue,
		MetricUnit:      source.MetricUnit,
		CompletionRate:  source.CompletionRate,
		ScoringRule:     source.ScoringRule,
		Attachments:     append([]string(nil), source.Attachments...),
		IsFromSuperior:  source.IsFromSuperior,
		ApprovalStatus:  "approved",
		VisibilityScope: source.VisibilityScope,
		SortOrder:       source.SortOrder,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (s *PerformanceService) syncPreviousPlanRecordsForNewFlowActivity(activity *database.PerformanceActivity, userID string) error {
	if activity == nil || !isNewPerformanceFlow(activity) || activity.ID == 0 {
		return nil
	}
	previous, err := s.findPreviousPlanActivity(activity)
	if err != nil || previous == nil || previous.ID == 0 || previous.ID == activity.ID {
		return err
	}

	currentActivityID := strconv.FormatUint(uint64(activity.ID), 10)
	previousActivityID := strconv.FormatUint(uint64(previous.ID), 10)
	orgID := strings.TrimSpace(activity.OrgID)
	now := time.Now()

	return s.db.Transaction(func(tx *gorm.DB) error {
		var participants []database.PerformanceParticipant
		participantQuery := tx.Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", currentActivityID, ignoredParticipantStatusList())
		if orgID != "" {
			participantQuery = participantQuery.Where("org_id = ?", orgID)
		}
		if err := participantQuery.Find(&participants).Error; err != nil {
			return err
		}
		if len(participants) == 0 {
			return nil
		}

		employeeIDs := make([]string, 0, len(participants))
		for _, participant := range participants {
			if employeeID := strings.TrimSpace(participant.EmployeeID); employeeID != "" {
				employeeIDs = append(employeeIDs, employeeID)
			}
		}
		if len(employeeIDs) == 0 {
			return nil
		}

		var previousParticipants []database.PerformanceParticipant
		previousParticipantQuery := tx.Where("activity_id = ? AND employee_id IN ? AND deleted_at IS NULL", previousActivityID, employeeIDs)
		if orgID != "" {
			previousParticipantQuery = previousParticipantQuery.Where("org_id = ?", orgID)
		}
		if err := previousParticipantQuery.Find(&previousParticipants).Error; err != nil {
			return err
		}
		if len(previousParticipants) == 0 {
			return nil
		}

		previousByEmployeeID := make(map[string]database.PerformanceParticipant, len(previousParticipants))
		previousParticipantIDs := make([]uint, 0, len(previousParticipants))
		for _, participant := range previousParticipants {
			previousByEmployeeID[strings.TrimSpace(participant.EmployeeID)] = participant
			previousParticipantIDs = append(previousParticipantIDs, participant.ID)
		}

		var sourceRecords []database.PerformanceGoalRecord
		sourceRecordQuery := tx.Where(
			"activity_id = ? AND participant_id IN ? AND goal_phase = ? AND section_type IN ? AND deleted_at IS NULL",
			previousActivityID,
			previousParticipantIDs,
			PerformanceGoalPhasePlan,
			[]string{"quantitative", "key_action"},
		)
		if orgID != "" {
			sourceRecordQuery = sourceRecordQuery.Where("org_id = ?", orgID)
		}
		if err := sourceRecordQuery.Order("participant_id ASC, sort_order ASC, id ASC").Find(&sourceRecords).Error; err != nil {
			return err
		}
		if len(sourceRecords) == 0 {
			return nil
		}

		sourceRecordsByParticipantID := make(map[uint][]database.PerformanceGoalRecord)
		for _, record := range sourceRecords {
			sourceRecordsByParticipantID[record.ParticipantID] = append(sourceRecordsByParticipantID[record.ParticipantID], record)
		}

		if activity.PreviousReviewActivityID == nil || *activity.PreviousReviewActivityID == 0 {
			previousID := previous.ID
			if err := s.scopeOrg(tx.Model(&database.PerformanceActivity{}), "org_id").
				Where("id = ?", activity.ID).
				Updates(map[string]interface{}{"previous_review_activity_id": previousID, "updated_by": userID}).Error; err != nil {
				return err
			}
			activity.PreviousReviewActivityID = &previousID
		}

		for _, participant := range participants {
			previousParticipant, ok := previousByEmployeeID[strings.TrimSpace(participant.EmployeeID)]
			if !ok {
				continue
			}
			records := sourceRecordsByParticipantID[previousParticipant.ID]
			if len(records) == 0 {
				continue
			}

			var existingCount int64
			if err := s.scopeOrg(tx.Model(&database.PerformanceGoalRecord{}), "org_id").
				Where("activity_id = ? AND participant_id = ? AND goal_phase = ? AND section_type IN ? AND deleted_at IS NULL",
					currentActivityID,
					participant.ID,
					PerformanceGoalPhaseReview,
					[]string{"quantitative", "key_action"},
				).Count(&existingCount).Error; err != nil {
				return err
			}
			if existingCount > 0 {
				continue
			}

			newRecords := make([]database.PerformanceGoalRecord, 0, len(records))
			for _, source := range records {
				record := clonePlanRecordAsReview(source, currentActivityID, participant.ID, now)
				record.OrgID = orgID
				newRecords = append(newRecords, record)
			}
			if err := tx.Create(&newRecords).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *PerformanceService) ensureNewFlowReviewRecordsReady(activity *database.PerformanceActivity) error {
	if activity == nil || !isNewPerformanceFlow(activity) || activity.ID == 0 {
		return nil
	}

	activityID := strconv.FormatUint(uint64(activity.ID), 10)
	var participants []database.PerformanceParticipant
	if err := s.scopedDB().Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", activityID, ignoredParticipantStatusList()).
		Find(&participants).Error; err != nil {
		return err
	}
	if len(participants) == 0 {
		return nil
	}

	participantIDs := make([]uint, 0, len(participants))
	for _, participant := range participants {
		participantIDs = append(participantIDs, participant.ID)
	}

	var rows []struct {
		ParticipantID uint `gorm:"column:participant_id"`
	}
	if err := s.scopedDB().Model(&database.PerformanceGoalRecord{}).
		Select("participant_id").
		Where("activity_id = ? AND participant_id IN ? AND goal_phase = ? AND section_type IN ? AND deleted_at IS NULL",
			activityID,
			participantIDs,
			PerformanceGoalPhaseReview,
			[]string{"quantitative", "key_action"},
		).
		Group("participant_id").
		Find(&rows).Error; err != nil {
		return err
	}

	readyParticipantIDs := make(map[uint]struct{}, len(rows))
	for _, row := range rows {
		readyParticipantIDs[row.ParticipantID] = struct{}{}
	}
	if len(readyParticipantIDs) < len(participants) {
		return fmt.Errorf("新流程缺少上一季度绩效考核指标，仍有 %d/%d 名参与人没有可自评指标，无法开启自评。请先确认上一期活动已有下季度目标计划，或补录/导入本期上一季度考核指标",
			len(participants)-len(readyParticipantIDs),
			len(participants),
		)
	}
	return nil
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
	if err := s.syncPreviousPlanRecordsForNewFlowActivity(activity, userID); err != nil {
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
	if err := s.syncPreviousPlanRecordsForNewFlowActivity(activity, userID); err != nil {
		return err
	}
	if err := s.ensureNewFlowReviewRecordsReady(activity); err != nil {
		return err
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
	if err := s.syncPreviousPlanRecordsForNewFlowActivity(activity, userID); err != nil {
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
	if err := s.syncSelfFinalAssessmentsForActivity(activityID, userID); err != nil {
		return err
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
		participantQuery := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", activityID, ignoredParticipantStatusList()).
			Order("id ASC")
		if strings.TrimSpace(activity.OrgID) != "" {
			participantQuery = participantQuery.Where("org_id = ?", strings.TrimSpace(activity.OrgID))
		}
		if err := participantQuery.Find(&participants).Error; err != nil {
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
		return s.scopeOrg(tx.Model(&database.PerformanceActivity{}), "org_id").
			Where("id = ?", activityID).
			Updates(map[string]interface{}{"status": "locked", "updated_by": userID}).Error
	})
}

func (s *PerformanceService) countActiveParticipants(activityID string) (int64, error) {
	var count int64
	if err := s.scopedDB().Model(&database.PerformanceParticipant{}).
		Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", activityID, ignoredParticipantStatusList()).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *PerformanceService) ensureParticipantStageComplete(activityID, stage string) error {
	var participants []database.PerformanceParticipant
	if err := s.scopedDB().Where("activity_id = ? AND deleted_at IS NULL", activityID).Find(&participants).Error; err != nil {
		return err
	}

	activeCount := 0
	incompleteCount := 0
	blockers := make([]string, 0, 3)
	blockerReasons := make([]string, 0, 3)
	for _, participant := range participants {
		if isIgnoredPerformanceParticipantStatus(participant.Status) {
			continue
		}
		activeCount++
		if !s.participantCompletedStageForFlow(participant, stage) || !participantHasStageEvidence(participant, stage) {
			incompleteCount++
			if len(blockers) < 3 {
				reason := participantStageIncompleteReason(participant, stage)
				blockers = append(blockers, formatParticipantStageBlocker(participant, reason))
				blockerReasons = append(blockerReasons, reason)
			}
		}
	}

	if activeCount == 0 {
		return errors.New("活动没有可参与员工，无法推进阶段")
	}
	if incompleteCount > 0 {
		return fmt.Errorf(
			"无法%s：还有 %d 名参与人未完成%s（%s）。%s",
			participantStageAdvanceAction(stage),
			incompleteCount,
			participantStageDisplayName(stage),
			strings.Join(blockers, "；"),
			participantStageAdvanceSuggestion(stage, blockerReasons),
		)
	}
	return nil
}

func participantStageDisplayName(stage string) string {
	if name, ok := participantStageDisplayNames[stage]; ok {
		return name
	}
	return "当前阶段"
}

func participantStageAdvanceAction(stage string) string {
	if action, ok := participantStageAdvanceActions[stage]; ok {
		return action
	}
	return "推进阶段"
}

func participantStageAdvanceSuggestion(stage string, reasons []string) string {
	switch stage {
	case "target_setting":
		if hasParticipantBlockerReason(reasons, "目标已提交") || hasParticipantBlockerReason(reasons, "目标待审批") {
			return "请先在参与人列表处理目标审批，通过后再开启自评。"
		}
		if hasParticipantBlockerReason(reasons, "考核上级") {
			return "请先配置考核上级，并完成目标提交/审批。"
		}
		return "请先完成目标提交/审批，通过后再开启自评。"
	case "self_evaluation":
		return "请提醒员工完成自评后再推进。"
	case "manager_evaluation":
		return "请提醒主管完成评分，且确认强制分布合规后再推进。"
	case "employee_confirmation":
		return "请提醒员工确认绩效结果后再推进。"
	case "manager_confirmation":
		return "请提醒主管确认绩效结果后再推进。"
	case "hr_confirmation":
		return "请完成HR确认后再锁定活动。"
	default:
		return "请处理未完成参与人后再推进。"
	}
}

func hasParticipantBlockerReason(reasons []string, keyword string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, keyword) {
			return true
		}
	}
	return false
}

func formatParticipantStageBlocker(participant database.PerformanceParticipant, reason string) string {
	return fmt.Sprintf(
		"%s：%s",
		participantDisplayName(participant),
		reason,
	)
}

func participantDisplayName(participant database.PerformanceParticipant) string {
	if name := strings.TrimSpace(participant.EmployeeName); name != "" {
		return name
	}
	if employeeID := strings.TrimSpace(participant.EmployeeID); employeeID != "" {
		return employeeID
	}
	if participant.ID > 0 {
		return fmt.Sprintf("#%d", participant.ID)
	}
	return "未命名参与人"
}

func participantStageIncompleteReason(participant database.PerformanceParticipant, stage string) string {
	switch stage {
	case "target_setting":
		switch participant.Status {
		case "target_pending_approval":
			return "目标已提交，待审批通过或驳回"
		case "target_rejected":
			return "目标被驳回后未重新通过"
		}
		if issue := participantAssessmentManagerIssue(participant); issue != "" {
			return issue
		}
		if participant.Status == "" || participant.Status == "pending" {
			return "目标未提交"
		}
	case "self_evaluation":
		switch participant.Status {
		case "", "pending":
			return "目标尚未完成"
		case "target_pending_approval":
			return "目标待审批，无法自评"
		case "target_rejected":
			return "目标被驳回后未重新通过"
		case "target_set":
			return "员工未提交自评"
		}
	case "manager_evaluation":
		if issue := participantAssessmentManagerIssue(participant); issue != "" {
			return issue
		}
		switch participant.Status {
		case "", "pending":
			return "目标尚未完成"
		case "target_set":
			return "员工未提交自评"
		case "self_submitted":
			return "主管未完成评分"
		}
	case "employee_confirmation":
		switch participant.Status {
		case "manager_submitted":
			return "员工未确认结果"
		case "self_submitted":
			return "主管未完成评分"
		}
	case "manager_confirmation":
		switch participant.Status {
		case "manager_recheck":
			return "员工已修改自评，主管待复核"
		case "employee_confirmed":
			return "主管未确认结果"
		case "manager_submitted":
			return "员工未确认结果"
		}
	case "hr_confirmation":
		switch participant.Status {
		case "manager_recheck":
			return "员工已修改自评，主管待复核"
		case "manager_confirmed":
			return "HR未确认结果"
		case "employee_confirmed":
			return "主管未确认结果"
		}
	}
	return fmt.Sprintf("当前状态为%s", participantStatusDisplayName(participant.Status))
}

func participantAssessmentManagerIssue(participant database.PerformanceParticipant) string {
	managerID := ptrStringValue(participant.ManagerID)
	configStatus := strings.ToUpper(strings.TrimSpace(participant.ManagerConfigStatus))
	switch {
	case configStatus == ManagerConfigPending || (configStatus == "" && managerID == ""):
		return "考核上级未配置"
	case configStatus == ManagerConfigInvalid:
		return "考核上级无效或已离职"
	default:
		return ""
	}
}

func participantStatusDisplayName(status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return "空状态"
	}
	if name, ok := participantStatusDisplayNames[status]; ok {
		return "「" + name + "」"
	}
	return "「" + status + "」"
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
	Name               string
	Code               string
	Description        string
	FlowType           string
	OrganizationID     string
	OrganizationScope  []string
	Status             string
	CycleTypes         []string
	WorkflowConfig     map[string]interface{}
	FormConfig         map[string]interface{}
	LevelRuleConfig    map[string]interface{}
	DistributionConfig map[string]interface{}
	PermissionConfig   map[string]interface{}
	PublishConfig      map[string]interface{}
	Sections           []PerformanceTemplateSectionRequest
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

	flowType := normalizePerformanceFlowType(req.FlowType)
	workflowConfig, formConfig, levelRuleConfig, distributionConfig, permissionConfig, publishConfig := defaultPerformanceConfig(flowType)
	template := &database.PerformanceTemplate{
		Name:               strings.TrimSpace(req.Name),
		Code:               strings.TrimSpace(req.Code),
		Description:        req.Description,
		FlowType:           flowType,
		OrganizationID:     strings.TrimSpace(req.OrganizationID),
		OrganizationScope:  req.OrganizationScope,
		Status:             strings.TrimSpace(req.Status),
		CycleTypes:         append([]string(nil), req.CycleTypes...),
		WorkflowConfig:     mergeDefaultConfig(req.WorkflowConfig, workflowConfig),
		FormConfig:         mergeDefaultConfig(req.FormConfig, formConfig),
		LevelRuleConfig:    mergeDefaultConfig(req.LevelRuleConfig, levelRuleConfig),
		DistributionConfig: mergeDefaultConfig(req.DistributionConfig, distributionConfig),
		PermissionConfig:   mergeDefaultConfig(req.PermissionConfig, permissionConfig),
		PublishConfig:      mergeDefaultConfig(req.PublishConfig, publishConfig),
		CreatedBy:          userID,
		UpdatedBy:          userID,
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

func builtInPerformanceTemplate(code string) (*database.PerformanceTemplate, []database.PerformanceTemplateSection, []database.PerformanceTemplateItem, []int, error) {
	switch code {
	case PerformanceTemplateCodeNew:
		workflowConfig, formConfig, levelRuleConfig, distributionConfig, permissionConfig, publishConfig := defaultPerformanceConfig(PerformanceFlowNew)
		template := &database.PerformanceTemplate{
			Name:               "沐腾科技流程模版",
			Code:               PerformanceTemplateCodeNew,
			Description:        "0-10 分制，S5/A15/B60/C15/D5，包含上季度完成情况和下季度目标计划。",
			FlowType:           PerformanceFlowNew,
			Status:             "active",
			CycleTypes:         []string{"monthly", "quarterly", "semiannual", "annual", "probation"},
			WorkflowConfig:     workflowConfig,
			FormConfig:         formConfig,
			LevelRuleConfig:    levelRuleConfig,
			DistributionConfig: distributionConfig,
			PermissionConfig:   permissionConfig,
			PublishConfig:      publishConfig,
			CreatedBy:          "system",
			UpdatedBy:          "system",
		}
		sections := []database.PerformanceTemplateSection{{
			Name:              "员工绩效考核表和目标制定表",
			SectionType:       "new_performance_form",
			Weight:            100,
			SortOrder:         1,
			IsScoreRequired:   true,
			IsCommentRequired: false,
		}}
		items := []database.PerformanceTemplateItem{
			{Name: "上级安排事项完成情况", Description: "固定项，不可删除，不可改权重", MaxScore: 10, Weight: 15, SortOrder: 1},
			{Name: "价值观及工作纪律", Description: "固定项，不可删除，不可改权重", MaxScore: 10, Weight: 15, SortOrder: 2},
			{Name: "OKR/KPI 自定义目标", Description: "员工可增减，OKR/KPI 二选一", MaxScore: 10, Weight: 70, SortOrder: 3},
		}
		return template, sections, items, []int{3}, nil
	case PerformanceTemplateCodeOld:
		workflowConfig, formConfig, levelRuleConfig, distributionConfig, permissionConfig, publishConfig := defaultPerformanceConfig(PerformanceFlowOld)
		template := &database.PerformanceTemplate{
			Name:               "小铁文娱流程模版",
			Code:               PerformanceTemplateCodeOld,
			Description:        "兼容历史 100 分制绩效流程。",
			FlowType:           PerformanceFlowOld,
			Status:             "active",
			CycleTypes:         []string{"monthly", "quarterly", "annual"},
			WorkflowConfig:     workflowConfig,
			FormConfig:         formConfig,
			LevelRuleConfig:    levelRuleConfig,
			DistributionConfig: distributionConfig,
			PermissionConfig:   permissionConfig,
			PublishConfig:      publishConfig,
			CreatedBy:          "system",
			UpdatedBy:          "system",
		}
		sections := []database.PerformanceTemplateSection{{
			Name:              "综合绩效",
			SectionType:       "score",
			Weight:            100,
			SortOrder:         1,
			IsScoreRequired:   true,
			IsCommentRequired: false,
		}}
		items := []database.PerformanceTemplateItem{{
			Name:        "综合评分",
			Description: "旧流程综合评分项",
			MaxScore:    100,
			Weight:      100,
			SortOrder:   1,
		}}
		return template, sections, items, []int{1}, nil
	default:
		return nil, nil, nil, nil, fmt.Errorf("未知内置绩效模板: %s", code)
	}
}

func (s *PerformanceService) ensureBuiltInPerformanceTemplate(code string) (*database.PerformanceTemplate, error) {
	var existing database.PerformanceTemplate
	err := s.scopedDB().Where("code = ? AND deleted_at IS NULL", code).First(&existing).Error
	if err == nil {
		return s.repairBuiltInPerformanceTemplate(&existing, code)
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	template, sections, items, counts, err := builtInPerformanceTemplate(code)
	if err != nil {
		return nil, err
	}
	if err := s.templateRepo.Create(template, sections, items, counts); err != nil {
		return nil, err
	}
	return template, nil
}

func (s *PerformanceService) repairBuiltInPerformanceTemplate(existing *database.PerformanceTemplate, code string) (*database.PerformanceTemplate, error) {
	if existing == nil {
		return nil, nil
	}
	canonical, _, _, _, err := builtInPerformanceTemplate(code)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	flowTypeChanged := normalizePerformanceFlowType(existing.FlowType) != canonical.FlowType
	if flowTypeChanged {
		existing.FlowType = canonical.FlowType
		updates["flow_type"] = canonical.FlowType
	}
	if len(existing.CycleTypes) == 0 {
		existing.CycleTypes = append([]string(nil), canonical.CycleTypes...)
		updates["cycle_types"] = existing.CycleTypes
	}
	if flowTypeChanged || len(existing.WorkflowConfig) == 0 {
		existing.WorkflowConfig = cloneJSONMap(canonical.WorkflowConfig)
		updates["workflow_config"] = existing.WorkflowConfig
	}
	if flowTypeChanged || len(existing.FormConfig) == 0 {
		existing.FormConfig = cloneJSONMap(canonical.FormConfig)
		updates["form_config"] = existing.FormConfig
	}
	if flowTypeChanged || len(existing.LevelRuleConfig) == 0 {
		existing.LevelRuleConfig = cloneJSONMap(canonical.LevelRuleConfig)
		updates["level_rule_config"] = existing.LevelRuleConfig
	}
	if flowTypeChanged || len(existing.DistributionConfig) == 0 {
		existing.DistributionConfig = cloneJSONMap(canonical.DistributionConfig)
		updates["distribution_config"] = existing.DistributionConfig
	}
	if flowTypeChanged || len(existing.PermissionConfig) == 0 {
		existing.PermissionConfig = cloneJSONMap(canonical.PermissionConfig)
		updates["permission_config"] = existing.PermissionConfig
	}
	if flowTypeChanged || len(existing.PublishConfig) == 0 {
		existing.PublishConfig = cloneJSONMap(canonical.PublishConfig)
		updates["publish_config"] = existing.PublishConfig
	}

	if len(updates) > 0 {
		existing.UpdatedBy = "system"
		if err := s.db.Save(existing).Error; err != nil {
			return nil, err
		}
	}
	return existing, nil
}

func (s *PerformanceService) ensureBuiltInPerformanceTemplates() {
	for _, code := range []string{PerformanceTemplateCodeOld, PerformanceTemplateCodeNew} {
		if _, err := s.ensureBuiltInPerformanceTemplate(code); err != nil {
			logrus.Warnf("ensure built-in performance template %s failed: %v", code, err)
		}
	}
}

// ListTemplates 获取模板列表（兼容旧模板 API）。
func (s *PerformanceService) ListTemplates(page, pageSize int, status string) ([]database.PerformanceTemplate, int64, error) {
	s.ensureBuiltInPerformanceTemplates()
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
	template.Code = strings.TrimSpace(req.Code)
	template.Description = req.Description
	if strings.TrimSpace(req.FlowType) != "" {
		template.FlowType = normalizePerformanceFlowType(req.FlowType)
	}
	template.OrganizationID = strings.TrimSpace(req.OrganizationID)
	template.OrganizationScope = req.OrganizationScope
	template.Status = strings.TrimSpace(req.Status)
	if template.Status == "" {
		template.Status = "draft"
	}
	if len(req.CycleTypes) > 0 {
		template.CycleTypes = append([]string(nil), req.CycleTypes...)
	}
	workflowConfig, formConfig, levelRuleConfig, distributionConfig, permissionConfig, publishConfig := defaultPerformanceConfig(template.FlowType)
	template.WorkflowConfig = mergeDefaultConfig(req.WorkflowConfig, workflowConfig)
	template.FormConfig = mergeDefaultConfig(req.FormConfig, formConfig)
	template.LevelRuleConfig = mergeDefaultConfig(req.LevelRuleConfig, levelRuleConfig)
	template.DistributionConfig = mergeDefaultConfig(req.DistributionConfig, distributionConfig)
	template.PermissionConfig = mergeDefaultConfig(req.PermissionConfig, permissionConfig)
	template.PublishConfig = mergeDefaultConfig(req.PublishConfig, publishConfig)
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
		if err := s.scopeOrg(tx.Clauses(clause.Locking{Strength: "UPDATE"}), "org_id").Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
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
			OrgID:          strings.TrimSpace(p.OrgID),
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
		if err := s.scopeOrg(tx.Clauses(clause.Locking{Strength: "UPDATE"}), "org_id").Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return errors.New("参与人不存在")
		}
		if p.Status == "employee_confirmed" || p.Status == "manager_confirmed" || p.Status == "hr_confirmed" || p.Status == "locked" {
			return nil
		}
		if p.IsLocked {
			return errors.New("结果已锁定，无法确认")
		}
		var activity database.PerformanceActivity
		if err := s.scopeOrg(tx, "org_id").Where("id = ? AND deleted_at IS NULL", p.ActivityID).First(&activity).Error; err != nil {
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
		if err := s.scopeOrg(tx.Clauses(clause.Locking{Strength: "UPDATE"}), "org_id").Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return errors.New("参与人不存在")
		}
		if p.Status == "manager_confirmed" || p.Status == "hr_confirmed" || p.Status == "locked" {
			return nil
		}
		if p.IsLocked {
			return errors.New("结果已锁定，无法确认")
		}
		var activity database.PerformanceActivity
		if err := s.scopeOrg(tx, "org_id").Where("id = ? AND deleted_at IS NULL", p.ActivityID).First(&activity).Error; err != nil {
			return errors.New("绩效活动不存在")
		}
		isManagerRecheck := strings.TrimSpace(p.Status) == "manager_recheck"
		validActivityStatus := activity.Status == "manager_confirmation" || (isManagerRecheck && activity.Status == "hr_confirmation")
		if !validActivityStatus {
			return errors.New("状态冲突：活动尚未进入主管确认阶段")
		}
		if p.Status != "employee_confirmed" && !isManagerRecheck {
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
		reviewType := "confirm_manager"
		if isManagerRecheck {
			reviewType = "confirm_manager_recheck"
		}
		version := &database.PerformanceReviewVersion{
			OrgID:          strings.TrimSpace(p.OrgID),
			ParticipantID:  p.ID,
			ActivityID:     p.ActivityID,
			ReviewType:     reviewType,
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
		if err := s.scopeOrg(tx.Clauses(clause.Locking{Strength: "UPDATE"}), "org_id").Where("id = ? AND deleted_at IS NULL", participantID).First(&p).Error; err != nil {
			return errors.New("参与人不存在")
		}
		if p.Status == "hr_confirmed" || p.Status == "locked" {
			return nil
		}
		var activity database.PerformanceActivity
		if err := s.scopeOrg(tx, "org_id").Where("id = ? AND deleted_at IS NULL", p.ActivityID).First(&activity).Error; err != nil {
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
			s.scopeOrg(tx.Model(&database.PerformanceGoalRecord{}), "org_id").
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
	if err := s.scopedDB().Where("id = ? AND deleted_at IS NULL", activityID).First(&activity).Error; err != nil {
		return fmt.Errorf("活动不存在: %v", err)
	}
	result, err := s.sendSelfEvalRemindersForActivity(&activity, selfEvalReminderSendOptions{})
	if err != nil {
		return err
	}
	logrus.Infof("sent self eval reminders: succeeded=%d skipped=%d already_sent=%d failed=%d", result.Sent, result.Skipped, result.AlreadySent, result.Failed)
	if result.Sent == 0 && result.Skipped == 0 && result.Failed == 0 && result.AlreadySent == 0 {
		return fmt.Errorf("没有需要发送自评提醒的参与人")
	}
	if result.Failed > 0 {
		return fmt.Errorf("自评提醒发送失败：成功 %d 人，跳过 %d 人，失败 %d 人，请查看后端日志", result.Sent, result.Skipped, result.Failed)
	}
	return nil
}

func (s *PerformanceService) SendDueSelfEvalAutoReminders(now time.Time) (*AutoSelfEvalReminderResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	result := &AutoSelfEvalReminderResult{}
	var activities []database.PerformanceActivity
	if err := s.db.
		Where("status = ? AND deleted_at IS NULL", performanceReminderStageSelfEval).
		Find(&activities).Error; err != nil {
		return result, err
	}
	result.ActivitiesScanned = len(activities)
	reminderDate := now.In(time.Local).Format("2006-01-02")
	for i := range activities {
		activity := &activities[i]
		reminderKey, _, ok := dueSelfEvalReminderRound(activity, now)
		if !ok {
			continue
		}
		result.ActivitiesMatched++
		sendResult, err := s.sendSelfEvalRemindersForActivity(activity, selfEvalReminderSendOptions{
			Automatic:    true,
			ReminderKey:  reminderKey,
			ReminderDate: reminderDate,
			Now:          now,
		})
		if err != nil {
			logrus.Warnf("auto self eval reminder failed for activity %d: %v", activity.ID, err)
			result.Failed++
			continue
		}
		result.Candidates += sendResult.Candidates
		result.Sent += sendResult.Sent
		result.Skipped += sendResult.Skipped
		result.AlreadySent += sendResult.AlreadySent
		result.Failed += sendResult.Failed
	}
	logrus.Infof(
		"auto self eval reminders finished: scanned=%d matched=%d candidates=%d sent=%d skipped=%d already_sent=%d failed=%d",
		result.ActivitiesScanned,
		result.ActivitiesMatched,
		result.Candidates,
		result.Sent,
		result.Skipped,
		result.AlreadySent,
		result.Failed,
	)
	return result, nil
}

func (s *PerformanceService) SendDueSelfEvalAutoReminderForActivity(activityID string, now time.Time, opts SelfEvalAutoReminderRunOptions) (*AutoSelfEvalReminderResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	result := &AutoSelfEvalReminderResult{}
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return result, fmt.Errorf("activity id is required")
	}

	var activity database.PerformanceActivity
	query := s.scopedDB().Where("id = ? AND deleted_at IS NULL", activityID)
	if orgID := strings.TrimSpace(opts.OrgID); orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
	if err := query.First(&activity).Error; err != nil {
		return result, err
	}
	result.ActivitiesScanned = 1
	if activity.Status != performanceReminderStageSelfEval {
		return result, nil
	}
	if opts.IncludeCurrentDay {
		includeCurrentSelfEvalReminderOffset(&activity, now)
	}

	reminderKey, _, ok := dueSelfEvalReminderRound(&activity, now)
	if !ok {
		return result, nil
	}
	result.ActivitiesMatched = 1
	sendResult, err := s.sendSelfEvalRemindersForActivity(&activity, selfEvalReminderSendOptions{
		Automatic:    true,
		ReminderKey:  reminderKey,
		ReminderDate: now.In(time.Local).Format("2006-01-02"),
		Now:          now,
	})
	if err != nil {
		result.Failed++
		return result, err
	}
	result.Candidates = sendResult.Candidates
	result.Sent = sendResult.Sent
	result.Skipped = sendResult.Skipped
	result.AlreadySent = sendResult.AlreadySent
	result.Failed = sendResult.Failed
	return result, nil
}

func includeCurrentSelfEvalReminderOffset(activity *database.PerformanceActivity, now time.Time) {
	if activity == nil {
		return
	}
	loc := time.Local
	deadline, ok := parsePerformanceDate(activity.SelfEvalEndAt, loc)
	if !ok {
		return
	}
	days := daysUntilDate(deadline, now, loc)
	if days < 0 {
		return
	}
	offsets := append(selfEvalReminderOffsets(activity), days)
	offsets = normalizeReminderOffsets(offsets)
	if activity.ReminderConfig == nil {
		activity.ReminderConfig = map[string]interface{}{}
	}
	activity.ReminderConfig["self_eval_reminder_offsets"] = offsets
}

func (s *PerformanceService) sendSelfEvalRemindersForActivity(activity *database.PerformanceActivity, opts selfEvalReminderSendOptions) (*SelfEvalReminderSendResult, error) {
	result := &SelfEvalReminderSendResult{}
	if activity == nil {
		return result, fmt.Errorf("活动不存在")
	}
	activityID := strconv.FormatUint(uint64(activity.ID), 10)
	if activityID == "0" {
		activityID = strings.TrimSpace(fmt.Sprint(activity.ID))
	}
	var participants []database.PerformanceParticipant
	query := s.scopedDB().Where("activity_id = ? AND deleted_at IS NULL AND status NOT IN ?", activityID, selfEvalReminderExcludedStatuses())
	activityOrgID := strings.TrimSpace(activity.OrgID)
	if activityOrgID != "" {
		query = query.Where("org_id = ?", activityOrgID)
	}
	if err := query.Find(&participants).Error; err != nil {
		return result, err
	}
	filtered := make([]database.PerformanceParticipant, 0, len(participants))
	for _, participant := range participants {
		if activityOrgID != "" && strings.TrimSpace(participant.OrgID) != activityOrgID {
			continue
		}
		if isIgnoredPerformanceParticipantStatus(participant.Status) {
			continue
		}
		if participant.Status == "self_submitted" || participant.Status == "manager_submitted" || participant.Status == "employee_confirmed" || participant.Status == "manager_recheck" || participant.Status == "manager_confirmed" || participant.Status == "hr_confirmed" || participant.Status == "locked" {
			continue
		}
		filtered = append(filtered, participant)
	}
	participants = filtered

	handledUsers := make(map[string]struct{})
	for _, p := range participants {
		employeeID := strings.TrimSpace(p.EmployeeID)
		if employeeID == "" {
			continue
		}
		if _, exists := handledUsers[employeeID]; exists {
			continue
		}
		result.Candidates++
		if opts.Automatic {
			alreadySent, err := s.hasPerformanceReminderLogForOrg(strings.TrimSpace(activity.OrgID), activityID, p.ID, performanceReminderStageSelfEval, opts.ReminderKey, opts.ReminderDate)
			if err != nil {
				return result, err
			}
			if alreadySent {
				result.AlreadySent++
				handledUsers[employeeID] = struct{}{}
				continue
			}
		}
		title := "绩效自评提醒"
		deadlineText := SelfEvalDeadlineReminderText(activity.SelfEvalEndAt)
		if !opts.Now.IsZero() {
			deadlineText = selfEvalDeadlineReminderText(activity.SelfEvalEndAt, opts.Now)
		}
		content := fmt.Sprintf(
			"您有一个绩效自评待完成，请尽快登录系统完成自评。\n绩效活动：%s\n%s\n提交完成后将不再收到后续自评提醒。",
			activity.Name,
			deadlineText,
		)
		if err := sendPerformanceActionCardToUser(employeeID, title, content, "去完成自评", PerformanceSelfEvalURL(activityID, p.ID)); err != nil {
			if dingtalk.IsUserNotNotifiableError(err) {
				logrus.Infof("skip self eval reminder to non-notifiable user %s: %v", employeeID, err)
				handledUsers[employeeID] = struct{}{}
				result.Skipped++
				if opts.Automatic {
					_ = s.createPerformanceReminderLog(activityID, p, opts, "skipped", err.Error())
				}
				continue
			}
			logrus.Warnf("send self eval reminder to %s failed: %v", employeeID, err)
			result.Failed++
		} else {
			handledUsers[employeeID] = struct{}{}
			result.Sent++
			if opts.Automatic {
				if err := s.createPerformanceReminderLog(activityID, p, opts, "sent", ""); err != nil {
					return result, err
				}
			}
		}
	}
	return result, nil
}

func selfEvalReminderExcludedStatuses() []string {
	rawStatuses := []string{
		"self_submitted",
		"manager_submitted",
		"employee_confirmed",
		"manager_recheck",
		"manager_confirmed",
		"hr_confirmed",
		"locked",
		"result_confirmed",
	}
	rawStatuses = append(rawStatuses, ignoredParticipantStatusList()...)
	seen := make(map[string]struct{}, len(rawStatuses))
	statuses := make([]string, 0, len(rawStatuses))
	for _, status := range rawStatuses {
		status = strings.TrimSpace(status)
		if status == "" {
			continue
		}
		if _, ok := seen[status]; ok {
			continue
		}
		seen[status] = struct{}{}
		statuses = append(statuses, status)
	}
	return statuses
}

func (s *PerformanceService) hasPerformanceReminderLog(activityID string, participantID uint, stage, reminderKey, reminderDate string) (bool, error) {
	return s.hasPerformanceReminderLogForOrg("", activityID, participantID, stage, reminderKey, reminderDate)
}

func (s *PerformanceService) hasPerformanceReminderLogForOrg(orgID, activityID string, participantID uint, stage, reminderKey, reminderDate string) (bool, error) {
	if strings.TrimSpace(reminderKey) == "" || strings.TrimSpace(reminderDate) == "" {
		return false, nil
	}
	var count int64
	query := s.scopedDB().Model(&database.PerformanceReminderLog{}).
		Where(
			"activity_id = ? AND participant_id = ? AND stage = ? AND reminder_key = ? AND reminder_date = ?",
			activityID,
			participantID,
			stage,
			reminderKey,
			reminderDate,
		)
	if orgID = strings.TrimSpace(orgID); orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *PerformanceService) hasRecentPerformanceReminderLog(activityID string, participantID uint, stage, reminderKey string, since time.Time) (bool, error) {
	if strings.TrimSpace(reminderKey) == "" {
		return false, nil
	}
	var count int64
	if err := s.scopedDB().Model(&database.PerformanceReminderLog{}).
		Where(
			"activity_id = ? AND participant_id = ? AND stage = ? AND reminder_key = ? AND created_at >= ?",
			activityID,
			participantID,
			stage,
			reminderKey,
			since,
		).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *PerformanceService) createPerformanceReminderLog(activityID string, participant database.PerformanceParticipant, opts selfEvalReminderSendOptions, status string, errorMessage string) error {
	if strings.TrimSpace(opts.ReminderKey) == "" || strings.TrimSpace(opts.ReminderDate) == "" {
		return nil
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	return s.createPerformanceReminderLogWithStage(activityID, participant, performanceReminderStageSelfEval, opts.ReminderKey, opts.ReminderDate, status, errorMessage, now)
}

func (s *PerformanceService) createPerformanceReminderLogWithStage(activityID string, participant database.PerformanceParticipant, stage, reminderKey, reminderDate, status, errorMessage string, now time.Time) error {
	if strings.TrimSpace(reminderKey) == "" || strings.TrimSpace(reminderDate) == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	log := database.PerformanceReminderLog{
		OrgID:         strings.TrimSpace(participant.OrgID),
		ActivityID:    activityID,
		ParticipantID: participant.ID,
		EmployeeID:    strings.TrimSpace(participant.EmployeeID),
		Stage:         strings.TrimSpace(stage),
		ReminderKey:   strings.TrimSpace(reminderKey),
		ReminderDate:  strings.TrimSpace(reminderDate),
		Channel:       performanceReminderChannelDing,
		Status:        strings.TrimSpace(status),
		ErrorMessage:  strings.TrimSpace(errorMessage),
		SentAt:        &now,
	}
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&log).Error
}

func (s *PerformanceService) notifyManagerSelfEvaluationRecheck(activity database.PerformanceActivity, participant database.PerformanceParticipant) error {
	activityID := strings.TrimSpace(participant.ActivityID)
	managerID := ptrStringValue(participant.ManagerID)
	if activityID == "" || managerID == "" {
		return nil
	}
	now := time.Now()
	alreadySent, err := s.hasRecentPerformanceReminderLog(
		activityID,
		participant.ID,
		performanceReminderStageManagerRecheck,
		performanceReminderKeyManagerRecheck,
		now.Add(-managerRecheckNotificationWindow),
	)
	if err != nil {
		return err
	}
	if alreadySent {
		return nil
	}

	title := "绩效自评修改待复核"
	content := fmt.Sprintf(
		"员工 %s 修改了绩效自评，请查看最新完成情况并复核。\n绩效活动：%s\n1小时内多次修改仅提醒一次。",
		participant.EmployeeName,
		activity.Name,
	)
	status := "sent"
	errorMessage := ""
	if err := sendPerformanceActionCardToUser(managerID, title, content, "去复核", PerformanceManagerEvalURL(activityID, participant.ID)); err != nil {
		errorMessage = err.Error()
		if dingtalk.IsUserNotNotifiableError(err) {
			status = "skipped"
			logrus.Infof("skip manager recheck notice to non-notifiable user %s: %v", managerID, err)
		} else {
			status = "failed"
			logrus.Warnf("send manager recheck notice to %s failed: %v", managerID, err)
		}
	}
	return s.createPerformanceReminderLogWithStage(
		activityID,
		participant,
		performanceReminderStageManagerRecheck,
		performanceReminderKeyManagerRecheck,
		now.Format("2006-01-02 15:04:05"),
		status,
		errorMessage,
		now,
	)
}

// SendManagerEvalReminders 发送主管评分提醒
func (s *PerformanceService) SendManagerEvalReminders(activityID string) error {
	var participants []database.PerformanceParticipant
	if err := s.scopedDB().Where("activity_id = ? AND deleted_at IS NULL AND status = ?", activityID, "self_submitted").
		Find(&participants).Error; err != nil {
		return err
	}

	managerCounts := make(map[string]int)
	managerFirstParticipant := make(map[string]database.PerformanceParticipant)
	for _, p := range participants {
		if p.ManagerID == nil {
			continue
		}
		managerID := strings.TrimSpace(*p.ManagerID)
		if managerID == "" {
			continue
		}
		managerCounts[managerID]++
		if _, exists := managerFirstParticipant[managerID]; !exists {
			managerFirstParticipant[managerID] = p
		}
	}

	var succeeded, skipped, failed int
	for managerID, count := range managerCounts {
		title := "绩效评分提醒"
		content := fmt.Sprintf("您有%d位员工的绩效待评分，请尽快完成。", count)
		firstParticipant := managerFirstParticipant[managerID]
		if err := dingtalk.SendCorpActionCardToUser(managerID, title, content, "去完成评分", PerformanceManagerEvalURL(activityID, firstParticipant.ID)); err != nil {
			if dingtalk.IsUserNotNotifiableError(err) {
				logrus.Infof("skip manager eval reminder to non-notifiable user %s: %v", managerID, err)
				skipped++
				continue
			}
			logrus.Warnf("send manager eval reminder to %s failed: %v", managerID, err)
			failed++
		} else {
			succeeded++
		}
	}
	logrus.Infof("sent manager eval reminders: succeeded=%d, skipped=%d, failed=%d", succeeded, skipped, failed)
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
		if err := dingtalk.SendCorpActionCardToUser(p.EmployeeID, "绩效面谈通知", content, "查看绩效结果", PerformanceResultURL(p.ActivityID, p.ID)); err != nil {
			if dingtalk.IsUserNotNotifiableError(err) {
				logrus.Infof("skip interview notification to non-notifiable user %s: %v", p.EmployeeID, err)
			} else {
				logrus.Warnf("send interview notification to employee %s failed: %v", p.EmployeeID, err)
			}
		}
	}

	return nil
}

// ===================== 目标记录管理 =====================

// GoalRecordRequest 目标记录请求
type GoalRecordRequest struct {
	ID             uint     `json:"id"`
	SectionType    string   `json:"section_type" binding:"required"`
	GoalPhase      string   `json:"goal_phase"`
	GoalType       string   `json:"goal_type"`
	FixedKey       string   `json:"fixed_key"`
	IsFixed        bool     `json:"is_fixed"`
	ItemName       string   `json:"item_name" binding:"required"`
	ItemDefinition string   `json:"item_definition"`
	Weight         float64  `json:"weight"`
	RedLineValue   string   `json:"red_line_value"`
	TargetValue    string   `json:"target_value"`
	ChallengeValue string   `json:"challenge_value"`
	MetricUnit     string   `json:"metric_unit"`
	CompletionRate float64  `json:"completion_rate"`
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
	s.ensureNewFlowReviewRecordsForParticipant(participantID)
	return s.goalRepo.FindByParticipant(participantID)
}

func (s *PerformanceService) GetLatestGoalRejection(participantID uint, activityID string) (*database.PerformanceGoalApprovalLog, error) {
	latestLog, err := s.approvalRepo.GetLatestByParticipant(participantID, activityID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if latestLog == nil || strings.TrimSpace(latestLog.Action) != "reject" {
		return nil, nil
	}
	return latestLog, nil
}

func (s *PerformanceService) ensureNewFlowReviewRecordsForParticipant(participantID uint) {
	participant, err := s.participantR.GetByID(strconv.FormatUint(uint64(participantID), 10))
	if err != nil {
		return
	}
	activity, err := s.actRepo.GetByID(participant.ActivityID)
	if err != nil || !isNewPerformanceFlow(activity) {
		return
	}
	var reviewCount int64
	if err := s.scopedDB().Model(&database.PerformanceGoalRecord{}).
		Where("activity_id = ? AND participant_id = ? AND goal_phase = ? AND section_type IN ? AND deleted_at IS NULL",
			participant.ActivityID,
			participantID,
			PerformanceGoalPhaseReview,
			[]string{"quantitative", "key_action"},
		).
		Count(&reviewCount).Error; err != nil || reviewCount > 0 {
		return
	}
	if err := s.syncPreviousPlanRecordsForNewFlowActivity(activity, "system"); err != nil {
		logrus.Warnf("sync previous plan records for participant %d failed: %v", participantID, err)
	}
}

// GetGoalRecordsByActivity 获取活动的所有目标记录
func (s *PerformanceService) GetGoalRecordsByActivity(activityID string) ([]database.PerformanceGoalRecord, error) {
	return s.goalRepo.FindByActivity(activityID)
}

func targetSettingApproved(participantStatus string, records []database.PerformanceGoalRecord) bool {
	switch participantStatus {
	case "target_set", "self_submitted", "manager_submitted", "result_confirmed", "employee_confirmed", "manager_confirmed", "hr_confirmed", "locked":
		return true
	}
	for _, record := range records {
		if record.ApprovalStatus == "approved" {
			return true
		}
	}
	return false
}

var newPerformanceFixedGoalItems = []GoalRecordRequest{
	{
		SectionType:    "key_action",
		GoalPhase:      "plan",
		GoalType:       "fixed",
		FixedKey:       "manager_arrangement",
		IsFixed:        true,
		ItemName:       "上级安排事项完成情况",
		ItemDefinition: "上级安排的所有事项需在规定时间内完成，工作结果得到领导认可",
		Weight:         0.15,
	},
	{
		SectionType:    "key_action",
		GoalPhase:      "plan",
		GoalType:       "fixed",
		FixedKey:       "values_discipline",
		IsFixed:        true,
		ItemName:       "价值观及工作纪律",
		ItemDefinition: "拥抱公司价值观，不得违反公司管理制度、规范等",
		Weight:         0.15,
	},
}

func normalizeNewPerformanceGoalRecords(records []GoalRecordRequest, goalPhase string) []GoalRecordRequest {
	goalPhase = normalizePerformanceGoalPhase(goalPhase)
	normalized := make([]GoalRecordRequest, 0, len(records)+len(newPerformanceFixedGoalItems))
	fixedByKey := make(map[string]GoalRecordRequest)
	for _, r := range records {
		key := strings.TrimSpace(r.FixedKey)
		if key != "" {
			fixedByKey[key] = r
		}
	}
	for _, fixed := range newPerformanceFixedGoalItems {
		item := fixed
		item.GoalPhase = goalPhase
		if existing, ok := fixedByKey[fixed.FixedKey]; ok {
			item.ID = existing.ID
			item.ActualResult = existing.ActualResult
			item.SelfScore = existing.SelfScore
			item.ManagerScore = existing.ManagerScore
			item.Attachments = existing.Attachments
			item.SortOrder = existing.SortOrder
		}
		normalized = append(normalized, item)
	}
	for _, r := range records {
		if strings.TrimSpace(r.FixedKey) != "" {
			continue
		}
		normalized = append(normalized, r)
	}
	return normalized
}

type goalRecordSaveOptions struct {
	goalPhase        string
	reviewSupplement bool
}

// BatchSaveGoalRecords 批量保存目标记录
func (s *PerformanceService) BatchSaveGoalRecords(participantID uint, records []GoalRecordRequest, userID string) ([]database.PerformanceGoalRecord, error) {
	return s.batchSaveGoalRecords(participantID, records, userID, goalRecordSaveOptions{})
}

func (s *PerformanceService) BatchSaveReviewGoalRecords(participantID uint, records []GoalRecordRequest, userID string) ([]database.PerformanceGoalRecord, error) {
	return s.batchSaveGoalRecords(participantID, records, userID, goalRecordSaveOptions{
		goalPhase:        PerformanceGoalPhaseReview,
		reviewSupplement: true,
	})
}

func (s *PerformanceService) batchSaveGoalRecords(participantID uint, records []GoalRecordRequest, userID string, options goalRecordSaveOptions) ([]database.PerformanceGoalRecord, error) {
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
	targetGoalPhase := targetSettingGoalPhaseForActivity(activity)
	if strings.TrimSpace(options.goalPhase) != "" {
		targetGoalPhase = normalizePerformanceGoalPhase(options.goalPhase)
	}
	isReviewSupplement := options.reviewSupplement || (isNewPerformanceFlow(activity) && targetGoalPhase == PerformanceGoalPhaseReview)

	if isReviewSupplement {
		if !isNewPerformanceFlow(activity) {
			return nil, fmt.Errorf("只有新流程活动可以补录上一季度考核指标")
		}
		if activity.Status != "target_setting" && activity.Status != "self_evaluation" {
			return nil, fmt.Errorf("当前活动状态不允许补录上一季度考核指标，活动状态为: %s", activity.Status)
		}
	} else if activity.Status != "target_setting" {
		return nil, fmt.Errorf("当前活动状态不允许设定目标，活动状态为: %s", activity.Status)
	}

	// 权重校验
	if participant.IsLocked {
		return nil, fmt.Errorf("该参与人的绩效结果已锁定，无法修改目标")
	}
	if isReviewSupplement && !canSupplementReviewGoalRecords(participant.Status) {
		return nil, fmt.Errorf("该参与人已进入自评或后续阶段，无法补录上一季度考核指标")
	}
	if !isReviewSupplement && targetSettingApproved(participant.Status, nil) {
		return nil, fmt.Errorf("目标设定已审批通过，无法修改")
	}
	if isNewPerformanceFlow(activity) {
		records = normalizeNewPerformanceGoalRecords(records, targetGoalPhase)
	}

	quantitativeWeight := 0.0
	keyActionWeight := 0.0
	normalizedRecords := make([]GoalRecordRequest, 0, len(records))
	for _, r := range records {
		if strings.TrimSpace(r.GoalPhase) == "" {
			r.GoalPhase = targetGoalPhase
		}
		r.GoalPhase = normalizePerformanceGoalPhase(r.GoalPhase)
		if isNewPerformanceFlow(activity) {
			r.GoalPhase = targetGoalPhase
		}
		if strings.TrimSpace(r.GoalType) == "" {
			switch r.SectionType {
			case "quantitative":
				r.GoalType = "kpi"
			case "key_action":
				r.GoalType = "okr"
			}
		}
		if r.IsFixed || strings.TrimSpace(r.FixedKey) != "" {
			r.GoalType = "fixed"
			r.IsFixed = true
		}
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
		if lockedP.IsLocked {
			return fmt.Errorf("该参与人的绩效结果已锁定，无法修改目标")
		}
		if isReviewSupplement && !canSupplementReviewGoalRecords(lockedP.Status) {
			return fmt.Errorf("该参与人已进入自评或后续阶段，无法补录上一季度考核指标")
		}

		// 查询现有记录
		var existing []database.PerformanceGoalRecord
		if err := tx.Where("participant_id = ? AND activity_id = ? AND goal_phase = ? AND section_type IN ? AND deleted_at IS NULL",
			participantID, participant.ActivityID, targetGoalPhase, []string{"quantitative", "key_action"}).
			Find(&existing).Error; err != nil {
			return err
		}
		if !isReviewSupplement && targetSettingApproved(lockedP.Status, existing) {
			return fmt.Errorf("目标设定已审批通过，无法修改")
		}
		existingMap := make(map[uint]database.PerformanceGoalRecord, len(existing))
		existingFixedIDByKey := make(map[string]uint)
		for _, e := range existing {
			existingMap[e.ID] = e
			if strings.TrimSpace(e.FixedKey) != "" {
				existingFixedIDByKey[strings.TrimSpace(e.FixedKey)] = e.ID
			}
		}

		// 收集前端提交的已有 ID，用于判断哪些需要软删除
		submittedIDs := make(map[uint]bool)
		now := time.Now()

		for i, r := range normalizedRecords {
			if r.ID == 0 && strings.TrimSpace(r.FixedKey) != "" {
				if id, ok := existingFixedIDByKey[strings.TrimSpace(r.FixedKey)]; ok {
					r.ID = id
				}
			}
			if r.ID > 0 {
				if _, ok := existingMap[r.ID]; !ok {
					return fmt.Errorf("目标记录 %d 不属于当前编辑阶段", r.ID)
				}
			}
			sortOrder := r.SortOrder
			if sortOrder == 0 {
				sortOrder = i + 1
			}

			if r.ID > 0 {
				submittedIDs[r.ID] = true
				// 更新已有记录
				attachJSON, _ := json.Marshal(r.Attachments)
				updates := map[string]interface{}{
					"section_type":     r.SectionType,
					"goal_phase":       strings.TrimSpace(r.GoalPhase),
					"goal_type":        strings.TrimSpace(r.GoalType),
					"fixed_key":        strings.TrimSpace(r.FixedKey),
					"is_fixed":         r.IsFixed,
					"item_name":        r.ItemName,
					"item_definition":  r.ItemDefinition,
					"weight":           r.Weight,
					"red_line_value":   r.RedLineValue,
					"target_value":     r.TargetValue,
					"challenge_value":  r.ChallengeValue,
					"metric_unit":      r.MetricUnit,
					"completion_rate":  r.CompletionRate,
					"scoring_rule":     r.ScoringRule,
					"actual_result":    r.ActualResult,
					"attachments":      string(attachJSON),
					"self_score":       r.SelfScore,
					"manager_score":    r.ManagerScore,
					"is_from_superior": r.IsFromSuperior,
					"sort_order":       sortOrder,
					"updated_at":       now,
				}
				if isReviewSupplement {
					updates["approval_status"] = "approved"
				}
				if err := s.scopeOrg(tx.Model(&database.PerformanceGoalRecord{}), "org_id").Where("id = ? AND deleted_at IS NULL", r.ID).
					Updates(updates).Error; err != nil {
					return err
				}
			} else {
				// 新增记录
				approvalStatus := "pending"
				if isReviewSupplement {
					approvalStatus = "approved"
				}
				record := database.PerformanceGoalRecord{
					ActivityID:      participant.ActivityID,
					ParticipantID:   participantID,
					SectionType:     r.SectionType,
					GoalPhase:       strings.TrimSpace(r.GoalPhase),
					GoalType:        strings.TrimSpace(r.GoalType),
					FixedKey:        strings.TrimSpace(r.FixedKey),
					IsFixed:         r.IsFixed,
					ItemName:        r.ItemName,
					ItemDefinition:  r.ItemDefinition,
					Weight:          r.Weight,
					RedLineValue:    r.RedLineValue,
					TargetValue:     r.TargetValue,
					ChallengeValue:  r.ChallengeValue,
					MetricUnit:      r.MetricUnit,
					CompletionRate:  r.CompletionRate,
					ScoringRule:     r.ScoringRule,
					ActualResult:    r.ActualResult,
					Attachments:     r.Attachments,
					SelfScore:       r.SelfScore,
					ManagerScore:    r.ManagerScore,
					IsFromSuperior:  r.IsFromSuperior,
					SortOrder:       sortOrder,
					ApprovalStatus:  approvalStatus,
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
				if err := s.scopeOrg(tx.Model(&database.PerformanceGoalRecord{}), "org_id").Where("id = ?", id).
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

func canSupplementReviewGoalRecords(participantStatus string) bool {
	switch strings.TrimSpace(participantStatus) {
	case "", "pending", "target_pending_approval", "target_rejected", "target_set":
		return true
	default:
		return false
	}
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
	targetGoalPhase := targetSettingGoalPhaseForActivity(activity)

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

	if err := s.scopedDB().Model(&database.PerformanceGoalRecord{}).
		Where("participant_id = ? AND activity_id = ? AND goal_phase = ?", participantID, participant.ActivityID, targetGoalPhase).
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
		if err := s.scopedDB().Model(&database.PerformanceParticipant{}).
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

	if activity.IndicatorLibraryID == nil || *activity.IndicatorLibraryID == 0 {
		return []database.PerformanceGoalRecord{}, nil
	}
	libraryID := *activity.IndicatorLibraryID

	var indicatorItems []database.PerformanceIndicatorItem
	if err := s.scopedDB().Where("library_id = ? AND deleted_at IS NULL AND section_type IN ?", libraryID, []string{"quantitative", "key_action"}).
		Order("is_default DESC, sort_order ASC, created_at ASC").
		Limit(12).
		Find(&indicatorItems).Error; err != nil {
		return nil, err
	}

	suggestions := make([]database.PerformanceGoalRecord, 0, len(indicatorItems))
	for _, item := range indicatorItems {
		if item.LibraryID != libraryID {
			continue
		}
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
	if err := s.scopedDB().Where("id = ? AND deleted_at IS NULL", participant.ActivityID).First(&activity).Error; err != nil {
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
		p.FinalLevel = PerformanceLevelByActivity(p.AdjustedScore, &activity)
		p.UpdatedBy = userID

		return tx.Save(&p).Error
	})
}

type GoalSelfEvaluationItem struct {
	RecordID     uint     `json:"record_id"`
	ActualResult string   `json:"actual_result"`
	Attachments  []string `json:"attachments"`
	SelfScore    float64  `json:"self_score"`
}

type GoalManagerEvaluationItem struct {
	RecordID     uint    `json:"record_id"`
	ManagerScore float64 `json:"manager_score"`
}

func validateGoalScoreForActivity(activity *database.PerformanceActivity, score float64, fieldName string) error {
	if !isNewPerformanceFlow(activity) {
		return nil
	}
	if score < 0 || score > 10 {
		return fmt.Errorf("%s必须在 0 到 10 之间", fieldName)
	}
	return nil
}

func canEditSelfEvaluationInActivity(activityStatus string) bool {
	switch strings.TrimSpace(activityStatus) {
	case "self_evaluation", "manager_evaluation", "employee_confirmation", "manager_confirmation", "hr_confirmation", "result_confirmed":
		return true
	default:
		return false
	}
}

func isHRFinalizedParticipant(participant database.PerformanceParticipant) bool {
	status := strings.TrimSpace(participant.Status)
	return participant.HRConfirmedAt != nil || status == "hr_confirmed" || status == "locked"
}

func isSelfEditAfterManagerConfirm(participant database.PerformanceParticipant) bool {
	status := strings.TrimSpace(participant.Status)
	return participant.ManagerConfirmedAt != nil || status == "manager_confirmed" || status == "manager_recheck"
}

func managerRecheckOperationMeta(previous database.PerformanceParticipant, itemCount, bonusItemCount int) map[string]interface{} {
	meta := map[string]interface{}{
		"previous_status":                 previous.Status,
		"previous_self_score":             previous.SelfScore,
		"previous_total_self_score":       previous.TotalSelfScore,
		"previous_evaluation_good":        previous.SelfEvaluationGood,
		"previous_evaluation_improvement": previous.SelfEvaluationImprovement,
		"edit_after_manager_confirm":      isSelfEditAfterManagerConfirm(previous),
		"goal_item_count":                 itemCount,
		"bonus_item_count":                bonusItemCount,
	}
	if previous.ManagerConfirmedAt != nil {
		meta["previous_manager_confirmed_at"] = previous.ManagerConfirmedAt
	}
	if strings.TrimSpace(previous.ManagerConfirmedBy) != "" {
		meta["previous_manager_confirmed_by"] = previous.ManagerConfirmedBy
	}
	return meta
}

func (s *PerformanceService) SubmitGoalSelfEvaluation(participantID uint, items []GoalSelfEvaluationItem, bonusItems []GoalSelfEvaluationItem, evaluationGood, evaluationImprovement, userID string) error {
	var notifyActivity database.PerformanceActivity
	var notifyParticipant database.PerformanceParticipant
	shouldNotifyManager := false
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var participant database.PerformanceParticipant
		if err := s.scopeOrg(tx.Clauses(clause.Locking{Strength: "UPDATE"}), "org_id").Where("id = ? AND deleted_at IS NULL", participantID).First(&participant).Error; err != nil {
			return fmt.Errorf("参与人不存在: %w", err)
		}
		previousParticipant := participant

		// 检查活动状态是否允许自评
		var activity database.PerformanceActivity
		if err := s.scopeOrg(tx, "org_id").Where("id = ? AND deleted_at IS NULL", participant.ActivityID).First(&activity).Error; err != nil {
			return fmt.Errorf("获取绩效活动失败: %w", err)
		}
		if isHRFinalizedParticipant(participant) {
			return fmt.Errorf("HR已确认或结果已锁定，无法提交自评")
		}
		if participant.IsLocked && !isSelfEditAfterManagerConfirm(participant) {
			return fmt.Errorf("该参与人的绩效结果已锁定，无法提交自评")
		}
		if !canEditSelfEvaluationInActivity(activity.Status) {
			return fmt.Errorf("当前活动状态不允许提交自评，活动状态为: %s", activity.Status)
		}
		if activity.Status == "self_evaluation" {
			if err := checkTimeWindow(&activity, "self_evaluation"); err != nil {
				return err
			}
		}

		var records []database.PerformanceGoalRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("participant_id = ? AND activity_id = ? AND deleted_at IS NULL", participantID, participant.ActivityID).
			Find(&records).Error; err != nil {
			return err
		}

		recordMap := make(map[uint]*database.PerformanceGoalRecord)
		reviewRecordCount := 0
		for i := range records {
			if isReviewGoalRecordForActivity(&activity, records[i]) {
				reviewRecordCount++
			}
			if isScorableGoalRecordForActivity(&activity, records[i]) {
				recordMap[records[i].ID] = &records[i]
			}
		}
		if isNewPerformanceFlow(&activity) && reviewRecordCount == 0 {
			return errors.New("新流程缺少上一季度绩效考核指标，无法提交自评。请联系HR先补录/导入本期上一季度考核指标，或从上一期活动的下季度目标计划承接后再自评")
		}

		for _, item := range items {
			record, exists := recordMap[item.RecordID]
			if !exists {
				return fmt.Errorf("目标记录 %d 不存在", item.RecordID)
			}
			if err := validateGoalScoreForActivity(&activity, item.SelfScore, "自评分"); err != nil {
				return err
			}
			record.ActualResult = item.ActualResult
			record.Attachments = item.Attachments
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
			if err := validateGoalScoreForActivity(&activity, item.SelfScore, "附加项自评分"); err != nil {
				return err
			}
			record.SelfScore = item.SelfScore
			record.UpdatedAt = time.Now()
			if err := tx.Save(record).Error; err != nil {
				return err
			}
		}

		totalSelfScore := 0.0
		for _, record := range recordMap {
			if !isReviewGoalRecordForActivity(&activity, *record) {
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
		shouldNotifyManager = isSelfEditAfterManagerConfirm(previousParticipant)
		if shouldNotifyManager {
			participant.Status = "manager_recheck"
			participant.IsLocked = false
			participant.LockedAt = nil
			participant.LockedBy = ""
			participant.ManagerConfirmedAt = nil
			participant.ManagerConfirmedBy = ""
		} else {
			participant.Status = "self_submitted"
		}
		participant.UpdatedBy = userID
		if err := tx.Save(&participant).Error; err != nil {
			return err
		}
		if err := s.applySelfFinalAssessmentWithDB(tx, &participant, &activity, userID); err != nil {
			return err
		}

		operationMeta := managerRecheckOperationMeta(previousParticipant, len(items), len(bonusItems))
		operationMeta["evaluation_good"] = participant.SelfEvaluationGood
		operationMeta["evaluation_improvement"] = participant.SelfEvaluationImprovement
		version := &database.PerformanceReviewVersion{
			OrgID:         strings.TrimSpace(participant.OrgID),
			ParticipantID: participant.ID,
			ActivityID:    participant.ActivityID,
			ReviewType:    "self",
			SelfScore:     totalSelfScore,
			SelfSummary:   participant.SelfSummary,
			CreatedBy:     userID,
			OperationMeta: operationMeta,
		}
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		notifyActivity = activity
		notifyParticipant = participant
		return nil
	})
	if err != nil {
		return err
	}
	if shouldNotifyManager {
		go func() {
			if err := s.notifyManagerSelfEvaluationRecheck(notifyActivity, notifyParticipant); err != nil {
				logrus.Warnf("notify manager self evaluation recheck failed: %v", err)
			}
		}()
	}
	return nil
}

func (s *PerformanceService) SubmitGoalManagerEvaluation(participantID uint, items []GoalManagerEvaluationItem, bonusItems []GoalManagerEvaluationItem, suggestedLevel, evaluationGood, evaluationImprovement, userID string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		var participant database.PerformanceParticipant
		if err := s.scopeOrg(tx.Clauses(clause.Locking{Strength: "UPDATE"}), "org_id").Where("id = ? AND deleted_at IS NULL", participantID).First(&participant).Error; err != nil {
			return fmt.Errorf("参与人不存在: %w", err)
		}

		var activity database.PerformanceActivity
		if err := s.scopeOrg(tx, "org_id").Where("id = ? AND deleted_at IS NULL", participant.ActivityID).First(&activity).Error; err != nil {
			return fmt.Errorf("绩效活动不存在: %w", err)
		}

		isManagerRecheckSubmission := strings.TrimSpace(participant.Status) == "manager_recheck"
		if isHRFinalizedParticipant(participant) {
			return fmt.Errorf("HR已确认或结果已锁定，无法提交上级评分")
		}
		if participant.IsLocked && !isManagerRecheckSubmission {
			return fmt.Errorf("该参与人的绩效结果已锁定，无法提交上级评分")
		}
		if isManagerRecheckSubmission {
			if activity.Status != "manager_confirmation" && activity.Status != "hr_confirmation" {
				return fmt.Errorf("当前活动状态不允许主管复核，活动状态为: %s", activity.Status)
			}
		} else {
			// 检查活动状态是否允许主管评分
			if activity.Status != "manager_evaluation" {
				return fmt.Errorf("当前活动状态不允许提交主管评分，活动状态为: %s", activity.Status)
			}
			if err := checkTimeWindow(&activity, "manager_evaluation"); err != nil {
				return err
			}
		}

		var records []database.PerformanceGoalRecord
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("participant_id = ? AND activity_id = ? AND deleted_at IS NULL", participantID, participant.ActivityID).
			Find(&records).Error; err != nil {
			return err
		}

		recordMap := make(map[uint]*database.PerformanceGoalRecord)
		for i := range records {
			if isScorableGoalRecordForActivity(&activity, records[i]) {
				recordMap[records[i].ID] = &records[i]
			}
		}

		for _, item := range items {
			record, exists := recordMap[item.RecordID]
			if !exists {
				return fmt.Errorf("目标记录 %d 不存在", item.RecordID)
			}
			if err := validateGoalScoreForActivity(&activity, item.ManagerScore, "上级评分"); err != nil {
				return err
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
			if err := validateGoalScoreForActivity(&activity, item.ManagerScore, "附加项评分"); err != nil {
				return err
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
			if !isReviewGoalRecordForActivity(&activity, *record) {
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

		autoLevel := PerformanceLevelByActivity(totalManagerScore, &activity)
		if activity.EnableBonusScore {
			autoLevel = PerformanceLevelByActivity(adjustedScore, &activity)
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
		if isManagerRecheckSubmission {
			now := time.Now()
			participant.Status = "manager_confirmed"
			participant.ManagerConfirmedAt = &now
			participant.ManagerConfirmedBy = userID
			participant.IsLocked = true
			participant.LockedAt = &now
			participant.LockedBy = userID
		} else {
			participant.Status = "manager_submitted"
		}
		participant.UpdatedBy = userID
		if err := tx.Save(&participant).Error; err != nil {
			return err
		}

		version := &database.PerformanceReviewVersion{
			OrgID:          strings.TrimSpace(participant.OrgID),
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
		if isManagerRecheckSubmission {
			version.ReviewType = "manager_recheck"
			version.ConfirmedAt = participant.ManagerConfirmedAt
			if meta, ok := version.OperationMeta.(map[string]interface{}); ok {
				meta["rechecked_after_self_edit"] = true
			}
		}
		return tx.Create(version).Error
	})
}

func (s *PerformanceService) SetCompanyFinance(activityID, revenueSign, description, remark, userID string) (*database.PerformanceCompanyFinance, error) {
	var finance database.PerformanceCompanyFinance
	err := s.scopedDB().Where("activity_id = ?", activityID).First(&finance).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		finance = database.PerformanceCompanyFinance{
			OrgID:       s.tenantOrgID(),
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
		if createErr := s.scopedDB().Create(&finance).Error; createErr != nil {
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
	if err := s.scopedDB().Save(&finance).Error; err != nil {
		return nil, err
	}
	return &finance, nil
}

func (s *PerformanceService) GetCompanyFinance(activityID string) (*database.PerformanceCompanyFinance, error) {
	var finance database.PerformanceCompanyFinance
	if err := s.scopedDB().Where("activity_id = ?", activityID).First(&finance).Error; err != nil {
		return nil, err
	}
	return &finance, nil
}

func (s *PerformanceService) GetPendingHRConfirm(activityID string) ([]database.PerformanceParticipant, error) {
	var participants []database.PerformanceParticipant
	if err := s.scopedDB().Where("activity_id = ? AND status = ? AND deleted_at IS NULL", activityID, "manager_confirmed").
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
		if err := s.scopeOrg(tx.Model(&database.PerformanceActivity{}), "org_id").
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

	recipients, err := s.hrConfirmReminderRecipients(activity)
	if err != nil {
		return err
	}
	if len(recipients) == 0 {
		return nil
	}

	title := "绩效 HR 确认提醒"
	content := fmt.Sprintf("活动：%s\n当前仍有 %d 名员工待 HR 确认，请及时处理。", activity.Name, len(pending))
	var failed []string
	for _, recipient := range recipients {
		if err := dingtalk.SendCorpActionCardToUser(recipient, title, content, "去处理确认", PerformanceOverviewURL(activityID)); err != nil {
			if dingtalk.IsUserNotNotifiableError(err) {
				logrus.Infof("skip HR confirm reminder to non-notifiable user %s: %v", recipient, err)
				continue
			}
			logrus.Warnf("send HR confirm reminder to %s failed: %v", recipient, err)
			failed = append(failed, recipient)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("HR确认提醒部分发送失败：%s", strings.Join(failed, ","))
	}
	return nil
}

func (s *PerformanceService) hrConfirmReminderRecipients(activity *database.PerformanceActivity) ([]string, error) {
	if activity == nil {
		return nil, nil
	}
	orgID := strings.TrimSpace(activity.OrgID)
	if orgID == "" {
		orgID = s.tenantOrgID()
	}
	recipients := make([]string, 0, 4)
	seen := make(map[string]struct{})
	add := func(userID string) {
		userID = strings.TrimSpace(userID)
		if userID == "" || userID == "system" || !dingtalk.IsNotifiableUserID(userID) {
			return
		}
		if _, ok := seen[userID]; ok {
			return
		}
		seen[userID] = struct{}{}
		recipients = append(recipients, userID)
	}
	if orgID == "" {
		add(activity.CreatedBy)
		return recipients, nil
	}

	permissionCodes := []string{"performance:hr_confirm:submit", "performance:activity:manage"}
	var users []database.User
	query := s.db.Model(&database.User{}).
		Select("DISTINCT users.*").
		Joins("JOIN user_roles ON user_roles.user_id = users.user_id AND user_roles.deleted_at IS NULL").
		Joins("JOIN role_permissions ON role_permissions.role_id = user_roles.role_id AND role_permissions.deleted_at IS NULL").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id AND permissions.deleted_at IS NULL").
		Where("permissions.code IN ? AND users.deleted_at IS NULL", permissionCodes)
	if orgID != "" {
		query = query.Where("user_roles.org_id = ? AND users.org_id = ?", orgID, orgID)
	}
	if err := query.Find(&users).Error; err != nil {
		return nil, err
	}

	for _, user := range users {
		add(user.UserID)
	}
	if len(recipients) == 0 {
		add(activity.CreatedBy)
	}
	sort.Strings(recipients)
	return recipients, nil
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
