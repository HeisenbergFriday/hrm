package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"

	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

const (
	PerformanceInterviewStatusPending   = "pending"
	PerformanceInterviewStatusScheduled = "scheduled"
	PerformanceInterviewStatusCompleted = "completed"
	PerformanceInterviewStatusCancelled = "cancelled"

	PerformanceInterviewTypeRequired = "required"
	PerformanceInterviewTypeOptional = "optional"

	PerformanceAppealStatusSubmitted  = "submitted"
	PerformanceAppealStatusProcessing = "processing"
	PerformanceAppealStatusResolved   = "resolved"
	PerformanceAppealStatusRejected   = "rejected"
	PerformanceAppealStatusWithdrawn  = "withdrawn"
)

type PerformanceFollowupListFilter struct {
	Page            int
	PageSize        int
	ActivityID      string
	Status          string
	EmployeeKeyword string
	Scope           *OrgDataScope
	IdentityValues  map[string]struct{}
	CanManage       bool
}

type PerformanceInterviewPayload struct {
	ParticipantID   uint
	InterviewType   string
	Status          string
	InterviewerID   string
	InterviewerName string
	ScheduledAt     *time.Time
	Location        string
	Summary         string
	Result          string
	CancelReason    string
	OperatorID      string
	SuppressNotice  bool
}

type PerformanceAppealPayload struct {
	ParticipantID  uint
	AppealReason   string
	DesiredResult  string
	Status         string
	HandlerID      string
	HandlerName    string
	HandleComment  string
	WithdrawReason string
	OperatorID     string
}

type PerformanceFollowupSummary struct {
	Total      int64          `json:"total"`
	StatusMap  map[string]int `json:"status_map"`
	Pending    int            `json:"pending"`
	Processing int            `json:"processing"`
	Completed  int            `json:"completed"`
	Closed     int            `json:"closed"`
}

type PerformanceFollowupService struct {
	db *gorm.DB
}

func NewPerformanceFollowupService(db *gorm.DB) *PerformanceFollowupService {
	return &PerformanceFollowupService{db: db}
}

// requireOrgID fails closed when the request DB session has no tenant context.
// Follow-up tables are org-scoped; never list/mutate without an explicit org.
func (s *PerformanceFollowupService) requireOrgID() (string, error) {
	if s == nil || s.db == nil {
		return "", fmt.Errorf("missing organization context")
	}
	return requireOrgIDFromDB(s.db)
}

func (s *PerformanceFollowupService) ListInterviews(filter PerformanceFollowupListFilter) ([]database.PerformanceInterviewRecord, int64, PerformanceFollowupSummary, error) {
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}
	filter = normalizePerformanceFollowupListFilter(filter)
	query := s.db.Model(&database.PerformanceInterviewRecord{}).Where("org_id = ? AND deleted_at IS NULL", orgID)
	query = applyPerformanceFollowupFilters(query, filter)

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}

	summary, err := buildPerformanceFollowupSummary(query.Session(&gorm.Session{}), "performance_interview_records")
	if err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}

	var items []database.PerformanceInterviewRecord
	if err := query.Session(&gorm.Session{}).
		Order("FIELD(status, 'pending', 'scheduled', 'completed', 'cancelled'), COALESCE(scheduled_at, created_at) DESC, id DESC").
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}
	return items, total, summary, nil
}

func (s *PerformanceFollowupService) ArrangeInterview(payload PerformanceInterviewPayload) (*database.PerformanceInterviewRecord, error) {
	if payload.ParticipantID == 0 {
		return nil, errors.New("请选择绩效参与人")
	}
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	participant, activity, err := s.loadParticipantAndActivity(payload.ParticipantID)
	if err != nil {
		return nil, err
	}
	if !performanceFollowupAllowed(participant, activity) {
		return nil, errors.New("绩效结果公布后才能安排面谈")
	}

	record := database.PerformanceInterviewRecord{}
	err = s.db.Where("org_id = ? AND participant_id = ? AND deleted_at IS NULL", orgID, payload.ParticipantID).First(&record).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	isNew := errors.Is(err, gorm.ErrRecordNotFound)
	if isNew {
		record = database.PerformanceInterviewRecord{}
		fillInterviewSnapshot(&record, participant, activity)
		record.CreatedBy = strings.TrimSpace(payload.OperatorID)
	}

	applyInterviewPayload(&record, payload, isNew)
	record.UpdatedBy = strings.TrimSpace(payload.OperatorID)
	if isNew {
		if err := s.db.Create(&record).Error; err != nil {
			return nil, err
		}
		if !payload.SuppressNotice {
			s.notifyInterviewChanged(&record)
		}
		return &record, nil
	}
	if err := s.db.Save(&record).Error; err != nil {
		return nil, err
	}
	if !payload.SuppressNotice {
		s.notifyInterviewChanged(&record)
	}
	return &record, nil
}

func (s *PerformanceFollowupService) UpdateInterview(id uint, payload PerformanceInterviewPayload) (*database.PerformanceInterviewRecord, error) {
	if id == 0 {
		return nil, errors.New("无效的面谈记录 ID")
	}
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	var record database.PerformanceInterviewRecord
	if err := s.db.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, id).First(&record).Error; err != nil {
		return nil, err
	}
	applyInterviewPayload(&record, payload, false)
	record.UpdatedBy = strings.TrimSpace(payload.OperatorID)
	if err := s.db.Save(&record).Error; err != nil {
		return nil, err
	}
	if !payload.SuppressNotice {
		s.notifyInterviewChanged(&record)
	}
	return &record, nil
}

func (s *PerformanceFollowupService) GetInterview(id uint) (*database.PerformanceInterviewRecord, error) {
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	var record database.PerformanceInterviewRecord
	if err := s.db.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, id).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (s *PerformanceFollowupService) ListAppeals(filter PerformanceFollowupListFilter) ([]database.PerformanceAppealRecord, int64, PerformanceFollowupSummary, error) {
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}
	filter = normalizePerformanceFollowupListFilter(filter)
	query := s.db.Model(&database.PerformanceAppealRecord{}).Where("org_id = ? AND deleted_at IS NULL", orgID)
	query = applyPerformanceFollowupFilters(query, filter)

	var total int64
	if err := query.Session(&gorm.Session{}).Count(&total).Error; err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}

	summary, err := buildPerformanceFollowupSummary(query.Session(&gorm.Session{}), "performance_appeal_records")
	if err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}

	var items []database.PerformanceAppealRecord
	if err := query.Session(&gorm.Session{}).
		Order("FIELD(status, 'submitted', 'processing', 'resolved', 'rejected', 'withdrawn'), created_at DESC, id DESC").
		Offset((filter.Page - 1) * filter.PageSize).
		Limit(filter.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, PerformanceFollowupSummary{}, err
	}
	return items, total, summary, nil
}

func (s *PerformanceFollowupService) SubmitAppeal(payload PerformanceAppealPayload) (*database.PerformanceAppealRecord, error) {
	if payload.ParticipantID == 0 {
		return nil, errors.New("请选择绩效参与人")
	}
	if strings.TrimSpace(payload.AppealReason) == "" {
		return nil, errors.New("请填写申诉原因")
	}
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	participant, activity, err := s.loadParticipantAndActivity(payload.ParticipantID)
	if err != nil {
		return nil, err
	}
	if !performanceFollowupAllowed(participant, activity) {
		return nil, errors.New("绩效结果公布后才能提交申诉")
	}

	var activeCount int64
	if err := s.db.Model(&database.PerformanceAppealRecord{}).
		Where("org_id = ? AND participant_id = ? AND status IN ? AND deleted_at IS NULL", orgID, payload.ParticipantID, []string{PerformanceAppealStatusSubmitted, PerformanceAppealStatusProcessing}).
		Count(&activeCount).Error; err != nil {
		return nil, err
	}
	if activeCount > 0 {
		return nil, errors.New("该参与人已有处理中申诉")
	}

	record := database.PerformanceAppealRecord{}
	fillAppealSnapshot(&record, participant, activity)
	record.Status = PerformanceAppealStatusSubmitted
	record.AppealReason = strings.TrimSpace(payload.AppealReason)
	record.DesiredResult = strings.TrimSpace(payload.DesiredResult)
	record.CreatedBy = strings.TrimSpace(payload.OperatorID)
	record.UpdatedBy = strings.TrimSpace(payload.OperatorID)
	if err := s.db.Create(&record).Error; err != nil {
		return nil, err
	}
	s.notifyAppealSubmitted(&record)
	return &record, nil
}

func (s *PerformanceFollowupService) UpdateAppeal(id uint, payload PerformanceAppealPayload) (*database.PerformanceAppealRecord, error) {
	if id == 0 {
		return nil, errors.New("无效的申诉记录 ID")
	}
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	var record database.PerformanceAppealRecord
	if err := s.db.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, id).First(&record).Error; err != nil {
		return nil, err
	}
	status := normalizePerformanceAppealStatus(payload.Status)
	if status == "" || status == PerformanceAppealStatusSubmitted || status == PerformanceAppealStatusWithdrawn {
		return nil, errors.New("无效的申诉处理状态")
	}
	if record.Status == PerformanceAppealStatusWithdrawn {
		return nil, errors.New("已撤回的申诉不能继续处理")
	}
	if record.Status == PerformanceAppealStatusResolved || record.Status == PerformanceAppealStatusRejected {
		return nil, errors.New("已完结的申诉不能继续处理")
	}

	now := time.Now()
	record.Status = status
	record.HandlerID = strings.TrimSpace(payload.HandlerID)
	record.HandlerName = strings.TrimSpace(payload.HandlerName)
	record.HandleComment = strings.TrimSpace(payload.HandleComment)
	if status == PerformanceAppealStatusResolved || status == PerformanceAppealStatusRejected {
		record.HandledAt = &now
	}
	record.UpdatedBy = strings.TrimSpace(payload.OperatorID)
	if err := s.db.Save(&record).Error; err != nil {
		return nil, err
	}
	s.notifyAppealStatusChanged(&record)
	return &record, nil
}

func (s *PerformanceFollowupService) WithdrawAppeal(id uint, reason, operatorID string) (*database.PerformanceAppealRecord, error) {
	if id == 0 {
		return nil, errors.New("无效的申诉记录 ID")
	}
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	var record database.PerformanceAppealRecord
	if err := s.db.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, id).First(&record).Error; err != nil {
		return nil, err
	}
	if record.Status != PerformanceAppealStatusSubmitted && record.Status != PerformanceAppealStatusProcessing {
		return nil, errors.New("当前申诉状态不能撤回")
	}
	record.Status = PerformanceAppealStatusWithdrawn
	record.WithdrawReason = strings.TrimSpace(reason)
	record.UpdatedBy = strings.TrimSpace(operatorID)
	if err := s.db.Save(&record).Error; err != nil {
		return nil, err
	}
	s.notifyAppealStatusChanged(&record)
	return &record, nil
}

func (s *PerformanceFollowupService) GetAppeal(id uint) (*database.PerformanceAppealRecord, error) {
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, err
	}
	var record database.PerformanceAppealRecord
	if err := s.db.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, id).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func PerformanceInterviewsURL(orgID, activityID string) string {
	return performanceFollowupModuleURL(orgID, "/performance-interviews", activityID)
}

func PerformanceAppealsURL(orgID, activityID string) string {
	return performanceFollowupModuleURL(orgID, "/performance-appeals", activityID)
}

func performanceFollowupModuleURL(orgID, path, activityID string) string {
	activityID = strings.TrimSpace(activityID)
	if activityID == "" {
		return buildPerformanceAppURL(orgID, path)
	}
	return buildPerformanceAppURL(orgID, fmt.Sprintf("%s?activity_id=%s", path, url.QueryEscape(activityID)))
}

func (s *PerformanceFollowupService) notifyInterviewChanged(record *database.PerformanceInterviewRecord) {
	if record == nil {
		return
	}
	orgID, err := performanceNoticeOrgID(record.OrgID)
	if err != nil {
		logrus.Warnf("skip interview notice: %v", err)
		return
	}
	employeeID := strings.TrimSpace(record.EmployeeID)
	if employeeID != "" {
		title, content := performanceInterviewEmployeeNotice(*record)
		sendPerformanceFollowupActionCard(orgID, employeeID, title, content, "查看绩效结果", PerformanceResultURL(orgID, record.ActivityID, record.ParticipantID), "interview employee notice")
	}

	interviewerID := strings.TrimSpace(record.InterviewerID)
	if interviewerID == "" || interviewerID == employeeID {
		return
	}
	if record.Status == PerformanceInterviewStatusCompleted || record.Status == PerformanceInterviewStatusCancelled {
		return
	}
	title, content := performanceInterviewInterviewerNotice(*record)
	sendPerformanceFollowupActionCard(orgID, interviewerID, title, content, "查看面谈记录", PerformanceInterviewsURL(orgID, record.ActivityID), "interview owner notice")
}

func (s *PerformanceFollowupService) notifyAppealSubmitted(record *database.PerformanceAppealRecord) {
	if record == nil {
		return
	}
	recipients, err := s.findPerformanceAppealManageRecipients(*record)
	if err != nil {
		logrus.Warnf("find performance appeal notice recipients failed: %v", err)
		return
	}
	if len(recipients) == 0 {
		logrus.Warnf("skip performance appeal notice for participant %d: no appeal manager recipients", record.ParticipantID)
		return
	}

	title := "绩效申诉待处理"
	content := strings.Join([]string{
		fmt.Sprintf("活动：%s", nonEmptyPerformanceFollowupText(record.ActivityName, record.ActivityID)),
		fmt.Sprintf("员工：%s", nonEmptyPerformanceFollowupText(record.EmployeeName, record.EmployeeID)),
		fmt.Sprintf("部门：%s", nonEmptyPerformanceFollowupText(record.DepartmentName, "-")),
		fmt.Sprintf("当前等级：%s", nonEmptyPerformanceFollowupText(record.FinalLevel, "-")),
		fmt.Sprintf("申诉原因：%s", record.AppealReason),
		"请及时受理并处理该绩效申诉。",
	}, "\n")
	orgID, err := performanceNoticeOrgID(record.OrgID)
	if err != nil {
		logrus.Warnf("skip performance appeal notice: %v", err)
		return
	}
	for _, recipient := range recipients {
		sendPerformanceFollowupActionCard(orgID, recipient.UserID, title, content, "去处理申诉", PerformanceAppealsURL(orgID, record.ActivityID), "appeal manager notice")
	}
}

func (s *PerformanceFollowupService) notifyAppealStatusChanged(record *database.PerformanceAppealRecord) {
	if record == nil {
		return
	}
	employeeID := strings.TrimSpace(record.EmployeeID)
	if employeeID == "" {
		return
	}
	orgID, err := performanceNoticeOrgID(record.OrgID)
	if err != nil {
		logrus.Warnf("skip appeal status notice for %s: %v", employeeID, err)
		return
	}
	title := "绩效申诉处理通知"
	lines := []string{
		fmt.Sprintf("活动：%s", nonEmptyPerformanceFollowupText(record.ActivityName, record.ActivityID)),
		fmt.Sprintf("申诉状态：%s", performanceAppealStatusText(record.Status)),
	}
	if handler := nonEmptyPerformanceFollowupText(record.HandlerName, record.HandlerID); handler != "" && handler != "-" {
		lines = append(lines, fmt.Sprintf("处理人：%s", handler))
	}
	if comment := strings.TrimSpace(record.HandleComment); comment != "" {
		lines = append(lines, fmt.Sprintf("处理意见：%s", comment))
	}
	if reason := strings.TrimSpace(record.WithdrawReason); reason != "" {
		lines = append(lines, fmt.Sprintf("撤回原因：%s", reason))
	}
	sendPerformanceFollowupActionCard(orgID, employeeID, title, strings.Join(lines, "\n"), "查看绩效结果", PerformanceResultURL(orgID, record.ActivityID, record.ParticipantID), "appeal employee notice")
}

func (s *PerformanceFollowupService) findPerformanceAppealManageRecipients(record database.PerformanceAppealRecord) ([]ReminderRecipient, error) {
	var recipients []ReminderRecipient
	orgID := strings.TrimSpace(record.OrgID)
	if orgID == "" {
		return nil, ErrPerformanceNoticeMissingOrg
	}
	// permissions / role_permissions remain global; isolate via users/user_roles/roles.org_id.
	if err := s.db.Table("users").
		Select("DISTINCT users.user_id AS user_id, users.name AS name").
		Joins("JOIN user_roles ON user_roles.user_id = users.user_id AND user_roles.org_id = users.org_id AND user_roles.deleted_at IS NULL").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.org_id = users.org_id AND roles.deleted_at IS NULL").
		Joins("JOIN role_permissions ON role_permissions.role_id = user_roles.role_id AND role_permissions.deleted_at IS NULL").
		Joins("JOIN permissions ON permissions.id = role_permissions.permission_id AND permissions.deleted_at IS NULL").
		Where("permissions.code IN ?", []string{"performance:appeal:manage", "performance:activity:manage"}).
		Where("users.org_id = ?", orgID).
		Where("users.deleted_at IS NULL").
		Where("users.status = ?", "active").
		Find(&recipients).Error; err != nil {
		return nil, err
	}
	recipients = normalizeReminderRecipients(recipients)
	if len(recipients) == 0 {
		return recipients, nil
	}

	// Scope checks must stay in the appeal record's org; never borrow another tenant's roles.
	permissionService := NewPermissionServiceWithOrgID(s.db, orgID)
	filtered := make([]ReminderRecipient, 0, len(recipients))
	for _, recipient := range recipients {
		if strings.TrimSpace(recipient.UserID) == strings.TrimSpace(record.EmployeeID) {
			continue
		}
		scope, err := permissionService.ResolveUserScopeInOrg(orgID, recipient.UserID)
		if err != nil {
			logrus.Warnf("resolve appeal recipient scope failed for user %s: %v", recipient.UserID, err)
			continue
		}
		if performanceFollowupScopeAllowsRecord(scope, record.EmployeeID, record.DepartmentID) {
			filtered = append(filtered, recipient)
		}
	}
	return filtered, nil
}

func performanceFollowupScopeAllowsRecord(scope *OrgDataScope, employeeID, departmentID string) bool {
	if scope == nil || scope.IsAll() {
		return true
	}
	if scope.IsSelf() {
		for _, userID := range scope.UserIDs {
			if strings.TrimSpace(userID) == strings.TrimSpace(employeeID) {
				return true
			}
		}
		return false
	}
	return scope.AllowsDepartment(strings.TrimSpace(departmentID))
}

func sendPerformanceFollowupActionCard(orgID, userID, title, content, actionTitle, actionURL, logContext string) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return
	}
	if err := sendPerformanceActionCard(orgID, userID, title, content, actionTitle, actionURL); err != nil {
		if dingtalk.IsUserNotNotifiableError(err) {
			logrus.Infof("skip %s to non-notifiable user %s: %v", logContext, userID, err)
			return
		}
		logrus.Warnf("send %s to %s failed: %v", logContext, userID, err)
	}
}

func performanceInterviewEmployeeNotice(record database.PerformanceInterviewRecord) (string, string) {
	statusText := performanceInterviewStatusText(record.Status)
	title := "绩效面谈通知"
	switch record.Status {
	case PerformanceInterviewStatusCancelled:
		title = "绩效面谈取消通知"
	case PerformanceInterviewStatusCompleted:
		title = "绩效面谈完成通知"
	}
	lines := []string{
		fmt.Sprintf("活动：%s", nonEmptyPerformanceFollowupText(record.ActivityName, record.ActivityID)),
		fmt.Sprintf("面谈状态：%s", statusText),
	}
	if interviewer := nonEmptyPerformanceFollowupText(record.InterviewerName, record.InterviewerID); interviewer != "" && interviewer != "-" {
		lines = append(lines, fmt.Sprintf("面谈人：%s", interviewer))
	}
	if record.ScheduledAt != nil {
		lines = append(lines, fmt.Sprintf("面谈时间：%s", record.ScheduledAt.Format("2006-01-02 15:04")))
	}
	if location := strings.TrimSpace(record.Location); location != "" {
		lines = append(lines, fmt.Sprintf("面谈地点：%s", location))
	}
	if reason := strings.TrimSpace(record.CancelReason); record.Status == PerformanceInterviewStatusCancelled && reason != "" {
		lines = append(lines, fmt.Sprintf("取消原因：%s", reason))
	}
	return title, strings.Join(lines, "\n")
}

func performanceInterviewInterviewerNotice(record database.PerformanceInterviewRecord) (string, string) {
	lines := []string{
		fmt.Sprintf("活动：%s", nonEmptyPerformanceFollowupText(record.ActivityName, record.ActivityID)),
		fmt.Sprintf("员工：%s", nonEmptyPerformanceFollowupText(record.EmployeeName, record.EmployeeID)),
		fmt.Sprintf("当前等级：%s", nonEmptyPerformanceFollowupText(record.FinalLevel, "-")),
		fmt.Sprintf("面谈状态：%s", performanceInterviewStatusText(record.Status)),
	}
	if record.ScheduledAt != nil {
		lines = append(lines, fmt.Sprintf("面谈时间：%s", record.ScheduledAt.Format("2006-01-02 15:04")))
	}
	if location := strings.TrimSpace(record.Location); location != "" {
		lines = append(lines, fmt.Sprintf("面谈地点：%s", location))
	}
	lines = append(lines, "请按安排完成绩效面谈记录。")
	return "绩效面谈任务提醒", strings.Join(lines, "\n")
}

func performanceInterviewStatusText(status string) string {
	switch strings.TrimSpace(status) {
	case PerformanceInterviewStatusPending:
		return "待安排"
	case PerformanceInterviewStatusScheduled:
		return "待面谈"
	case PerformanceInterviewStatusCompleted:
		return "已完成"
	case PerformanceInterviewStatusCancelled:
		return "已取消"
	default:
		return "待安排"
	}
}

func performanceAppealStatusText(status string) string {
	switch strings.TrimSpace(status) {
	case PerformanceAppealStatusSubmitted:
		return "已提交"
	case PerformanceAppealStatusProcessing:
		return "处理中"
	case PerformanceAppealStatusResolved:
		return "已完成"
	case PerformanceAppealStatusRejected:
		return "已驳回"
	case PerformanceAppealStatusWithdrawn:
		return "已撤回"
	default:
		return "已提交"
	}
}

func nonEmptyPerformanceFollowupText(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *PerformanceFollowupService) loadParticipantAndActivity(participantID uint) (*database.PerformanceParticipant, *database.PerformanceActivity, error) {
	orgID, err := s.requireOrgID()
	if err != nil {
		return nil, nil, err
	}
	var participant database.PerformanceParticipant
	if err := s.db.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, participantID).First(&participant).Error; err != nil {
		return nil, nil, errors.New("绩效参与人不存在")
	}
	if participant.Status == "inactive" || participant.Status == "removed_from_scope" {
		return nil, nil, errors.New("该参与人不在当前绩效范围内")
	}
	var activity database.PerformanceActivity
	if err := s.db.Where("org_id = ? AND id = ? AND deleted_at IS NULL", orgID, participant.ActivityID).First(&activity).Error; err != nil {
		return nil, nil, errors.New("绩效活动不存在")
	}
	return &participant, &activity, nil
}

func normalizePerformanceFollowupListFilter(filter PerformanceFollowupListFilter) PerformanceFollowupListFilter {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.PageSize > 200 {
		filter.PageSize = 200
	}
	filter.ActivityID = strings.TrimSpace(filter.ActivityID)
	filter.Status = strings.TrimSpace(filter.Status)
	filter.EmployeeKeyword = strings.TrimSpace(filter.EmployeeKeyword)
	return filter
}

func applyPerformanceFollowupFilters(query *gorm.DB, filter PerformanceFollowupListFilter) *gorm.DB {
	if filter.ActivityID != "" {
		query = query.Where("activity_id = ?", filter.ActivityID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.EmployeeKeyword != "" {
		like := "%" + filter.EmployeeKeyword + "%"
		query = query.Where("employee_name LIKE ? OR employee_id LIKE ?", like, like)
	}
	if !filter.CanManage {
		userIDs := performanceFollowupIdentityKeys(filter.IdentityValues)
		if len(userIDs) == 0 {
			return query.Where("1 = 0")
		}
		return query.Where("employee_id IN ?", userIDs)
	}
	if filter.Scope != nil && filter.Scope.IsSelf() {
		userIDs := uniquePerformanceFollowupStrings(filter.Scope.UserIDs)
		if len(userIDs) == 0 {
			return query.Where("1 = 0")
		}
		return query.Where("employee_id IN ?", userIDs)
	}
	if filter.Scope != nil && !filter.Scope.IsAll() {
		departmentIDs := uniquePerformanceFollowupStrings(filter.Scope.DepartmentIDs)
		if len(departmentIDs) == 0 {
			return query.Where("1 = 0")
		}
		return query.Where("department_id IN ?", departmentIDs)
	}
	return query
}

func buildPerformanceFollowupSummary(query *gorm.DB, table string) (PerformanceFollowupSummary, error) {
	type statusCount struct {
		Status string
		Count  int
	}
	var rows []statusCount
	if err := query.Session(&gorm.Session{}).Select("status, COUNT(*) AS count").Group("status").Scan(&rows).Error; err != nil {
		return PerformanceFollowupSummary{}, err
	}
	summary := PerformanceFollowupSummary{StatusMap: map[string]int{}}
	for _, row := range rows {
		summary.Total += int64(row.Count)
		summary.StatusMap[row.Status] = row.Count
		switch table {
		case "performance_interview_records":
			switch row.Status {
			case PerformanceInterviewStatusPending:
				summary.Pending += row.Count
			case PerformanceInterviewStatusScheduled:
				summary.Processing += row.Count
			case PerformanceInterviewStatusCompleted:
				summary.Completed += row.Count
			case PerformanceInterviewStatusCancelled:
				summary.Closed += row.Count
			}
		default:
			switch row.Status {
			case PerformanceAppealStatusSubmitted:
				summary.Pending += row.Count
			case PerformanceAppealStatusProcessing:
				summary.Processing += row.Count
			case PerformanceAppealStatusResolved:
				summary.Completed += row.Count
			case PerformanceAppealStatusRejected, PerformanceAppealStatusWithdrawn:
				summary.Closed += row.Count
			}
		}
	}
	return summary, nil
}

func fillInterviewSnapshot(record *database.PerformanceInterviewRecord, participant *database.PerformanceParticipant, activity *database.PerformanceActivity) {
	orgID := strings.TrimSpace(participant.OrgID)
	if orgID == "" {
		orgID = strings.TrimSpace(activity.OrgID)
	}
	record.OrgID = orgID
	record.ActivityID = strings.TrimSpace(participant.ActivityID)
	record.ActivityName = strings.TrimSpace(activity.Name)
	record.ParticipantID = participant.ID
	record.EmployeeID = strings.TrimSpace(participant.EmployeeID)
	record.EmployeeName = strings.TrimSpace(participant.EmployeeName)
	record.DepartmentID = strings.TrimSpace(participant.DepartmentID)
	record.DepartmentName = strings.TrimSpace(participant.DepartmentName)
	record.Position = strings.TrimSpace(participant.Position)
	record.FinalLevel = strings.TrimSpace(effectiveParticipantLevel(*participant))
}

func fillAppealSnapshot(record *database.PerformanceAppealRecord, participant *database.PerformanceParticipant, activity *database.PerformanceActivity) {
	orgID := strings.TrimSpace(participant.OrgID)
	if orgID == "" {
		orgID = strings.TrimSpace(activity.OrgID)
	}
	record.OrgID = orgID
	record.ActivityID = strings.TrimSpace(participant.ActivityID)
	record.ActivityName = strings.TrimSpace(activity.Name)
	record.ParticipantID = participant.ID
	record.EmployeeID = strings.TrimSpace(participant.EmployeeID)
	record.EmployeeName = strings.TrimSpace(participant.EmployeeName)
	record.DepartmentID = strings.TrimSpace(participant.DepartmentID)
	record.DepartmentName = strings.TrimSpace(participant.DepartmentName)
	record.Position = strings.TrimSpace(participant.Position)
	record.FinalLevel = strings.TrimSpace(effectiveParticipantLevel(*participant))
}

func applyInterviewPayload(record *database.PerformanceInterviewRecord, payload PerformanceInterviewPayload, isNew bool) {
	if interviewType := normalizePerformanceInterviewType(payload.InterviewType); interviewType != "" {
		record.InterviewType = interviewType
	} else if isNew || strings.TrimSpace(record.InterviewType) == "" {
		record.InterviewType = PerformanceInterviewTypeRequired
	}

	status := normalizePerformanceInterviewStatus(payload.Status)
	if status == "" {
		if payload.ScheduledAt != nil {
			status = PerformanceInterviewStatusScheduled
		} else if isNew || strings.TrimSpace(record.Status) == "" {
			status = PerformanceInterviewStatusPending
		}
	}
	if status != "" {
		record.Status = status
	}
	record.InterviewerID = strings.TrimSpace(payload.InterviewerID)
	record.InterviewerName = strings.TrimSpace(payload.InterviewerName)
	record.ScheduledAt = payload.ScheduledAt
	record.Location = strings.TrimSpace(payload.Location)
	record.Summary = strings.TrimSpace(payload.Summary)
	record.Result = strings.TrimSpace(payload.Result)
	record.CancelReason = strings.TrimSpace(payload.CancelReason)
	if record.Status == PerformanceInterviewStatusCompleted && record.CompletedAt == nil {
		now := time.Now()
		record.CompletedAt = &now
	}
	if record.Status != PerformanceInterviewStatusCompleted {
		record.CompletedAt = nil
	}
}

func performanceFollowupAllowed(participant *database.PerformanceParticipant, activity *database.PerformanceActivity) bool {
	if participant == nil || activity == nil {
		return false
	}
	if isNewPerformanceFlow(activity) && mutengPublishedParticipantStatus(participant.Status) {
		return true
	}
	switch strings.TrimSpace(activity.Status) {
	case "result_publish", "result_confirmed", "locked", "archived", "interview", "appeal":
		return true
	default:
		return false
	}
}

func normalizePerformanceInterviewType(value string) string {
	switch strings.TrimSpace(value) {
	case PerformanceInterviewTypeRequired, PerformanceInterviewTypeOptional:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizePerformanceInterviewStatus(value string) string {
	switch strings.TrimSpace(value) {
	case PerformanceInterviewStatusPending, PerformanceInterviewStatusScheduled, PerformanceInterviewStatusCompleted, PerformanceInterviewStatusCancelled:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func normalizePerformanceAppealStatus(value string) string {
	switch strings.TrimSpace(value) {
	case PerformanceAppealStatusSubmitted, PerformanceAppealStatusProcessing, PerformanceAppealStatusResolved, PerformanceAppealStatusRejected, PerformanceAppealStatusWithdrawn:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func performanceFollowupIdentityKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			keys = append(keys, value)
		}
	}
	return uniquePerformanceFollowupStrings(keys)
}

func uniquePerformanceFollowupStrings(values []string) []string {
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
	return result
}
