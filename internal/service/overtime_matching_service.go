package service

import (
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"time"

	"gorm.io/gorm"
)

type OvertimeMatchingService struct {
	db          *gorm.DB
	matchRepo   *repository.OvertimeMatchResultRepository
	ledgerRepo  *repository.CompensatoryLeaveLedgerRepository
	ruleRepo    *repository.OvertimeRuleConfigRepository
}

func NewOvertimeMatchingService(db *gorm.DB) *OvertimeMatchingService {
	return &OvertimeMatchingService{
		db:         db,
		matchRepo:  repository.NewOvertimeMatchResultRepository(db),
		ledgerRepo: repository.NewCompensatoryLeaveLedgerRepository(db),
		ruleRepo:   repository.NewOvertimeRuleConfigRepository(db),
	}
}

type MatchResult struct {
	ID                uint      `json:"id"`
	UserID            string    `json:"user_id"`
	ApprovalID        uint      `json:"approval_id"`
	ApprovalProcessID string    `json:"approval_process_id"`
	ApprovalStatus    string    `json:"approval_status"`
	ApprovalStartTime time.Time `json:"approval_start_time"`
	ApprovalEndTime   time.Time `json:"approval_end_time"`
	MatchedMinutes    int       `json:"matched_minutes"`
	QualifiedMinutes  int       `json:"qualified_minutes"`
	MatchStatus       string    `json:"match_status"`
	MatchReason       string    `json:"match_reason"`
}

func (s *OvertimeMatchingService) MatchApprovedOvertime(startDate, endDate string) error {
	var approvals []database.Approval
	if err := s.db.Where("status = 'completed' AND DATE(create_time) >= ? AND DATE(create_time) <= ?", startDate, endDate).
		Find(&approvals).Error; err != nil {
		return err
	}
	for _, a := range approvals {
		if !s.isOvertimeApproval(&a) {
			continue
		}
		if err := s.MatchApproval(a.ID); err != nil {
			return fmt.Errorf("审批%d匹配失败: %w", a.ID, err)
		}
	}
	return nil
}

func (s *OvertimeMatchingService) MatchApproval(approvalID uint) error {
	var approval database.Approval
	if err := s.db.First(&approval, approvalID).Error; err != nil {
		return err
	}
	if approval.Status != "completed" {
		return nil
	}

	approvalStart, approvalEnd := s.extractApprovalTimeWindow(&approval)
	if approvalStart.IsZero() || approvalEnd.IsZero() {
		return s.saveMatchResult(&approval, approvalStart, approvalEnd, time.Time{}, time.Time{}, 0, 0, "unmatched", "审批时间窗口解析失败")
	}

	// 查找该员工在审批时间窗口内的考勤打卡
	var attendances []database.Attendance
	s.db.Where("user_id = ? AND check_time >= ? AND check_time <= ?",
		approval.ApplicantID, approvalStart, approvalEnd).
		Order("check_time asc").Find(&attendances)

	checkin, checkout := s.extractCheckinCheckout(attendances)
	if checkin.IsZero() || checkout.IsZero() {
		return s.saveMatchResult(&approval, approvalStart, approvalEnd, time.Time{}, time.Time{}, 0, 0, "unmatched", "无有效考勤记录")
	}

	// 取审批窗口与考勤窗口的交集
	matchStart := approvalStart
	if checkin.After(matchStart) {
		matchStart = checkin
	}
	matchEnd := approvalEnd
	if checkout.Before(matchEnd) {
		matchEnd = checkout
	}

	if matchEnd.Before(matchStart) {
		return s.saveMatchResult(&approval, approvalStart, approvalEnd, checkin, checkout, 0, 0, "unmatched", "考勤时间与审批时间无交集")
	}

	matchedMinutes := int(matchEnd.Sub(matchStart).Minutes())
	qualifiedMinutes := s.applyOvertimeRules(matchedMinutes)

	status := "matched"
	reason := fmt.Sprintf("匹配%d分钟，有效%d分钟", matchedMinutes, qualifiedMinutes)
	if matchedMinutes < int(approvalEnd.Sub(approvalStart).Minutes()) {
		status = "partial"
	}

	if err := s.saveMatchResult(&approval, approvalStart, approvalEnd, checkin, checkout, matchedMinutes, qualifiedMinutes, status, reason); err != nil {
		return err
	}

	// 生成调休台账
	if qualifiedMinutes > 0 {
		match, err := s.matchRepo.FindByApprovalID(approvalID)
		if err != nil {
			return err
		}
		compSvc := NewCompensatoryLeaveService(s.db)
		return compSvc.CreditFromOvertime(match.ID)
	}
	return nil
}

func (s *OvertimeMatchingService) RollbackApprovalMatch(approvalID uint) error {
	match, err := s.matchRepo.FindByApprovalID(approvalID)
	if err != nil {
		return err
	}
	if match.MatchStatus == "rolled_back" {
		return nil
	}
	compSvc := NewCompensatoryLeaveService(s.db)
	if err := compSvc.RollbackCredit(match.ID); err != nil {
		return err
	}
	return s.matchRepo.UpdateStatus(match.ID, "rolled_back", "审批撤销")
}

func (s *OvertimeMatchingService) GetMatchResults(userID, startDate, endDate string) ([]MatchResult, error) {
	rows, err := s.matchRepo.FindByUserDateRange(userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	var results []MatchResult
	for _, r := range rows {
		results = append(results, MatchResult{
			ID: r.ID, UserID: r.UserID, ApprovalID: r.ApprovalID,
			ApprovalProcessID: r.ApprovalProcessID, ApprovalStatus: r.ApprovalStatus,
			ApprovalStartTime: r.ApprovalStartTime, ApprovalEndTime: r.ApprovalEndTime,
			MatchedMinutes: r.MatchedMinutes, QualifiedMinutes: r.QualifiedMinutes,
			MatchStatus: r.MatchStatus, MatchReason: r.MatchReason,
		})
	}
	return results, nil
}

func (s *OvertimeMatchingService) isOvertimeApproval(a *database.Approval) bool {
	if ext, ok := a.Extension["process_code"].(string); ok {
		return ext == "PROC-OVERTIME" || ext == "overtime"
	}
	if cat, ok := a.Extension["category"].(string); ok {
		return cat == "overtime" || cat == "加班"
	}
	return false
}

func (s *OvertimeMatchingService) extractApprovalTimeWindow(a *database.Approval) (time.Time, time.Time) {
	startStr, _ := a.Content["start_time"].(string)
	endStr, _ := a.Content["end_time"].(string)
	layout := "2006-01-02 15:04:05"
	start, _ := time.ParseInLocation(layout, startStr, time.Local)
	end, _ := time.ParseInLocation(layout, endStr, time.Local)
	if start.IsZero() {
		start = a.CreateTime
	}
	if end.IsZero() {
		end = a.FinishTime
	}
	return start, end
}

func (s *OvertimeMatchingService) extractCheckinCheckout(records []database.Attendance) (time.Time, time.Time) {
	var checkin, checkout time.Time
	for _, r := range records {
		if r.CheckType == "上班" && (checkin.IsZero() || r.CheckTime.Before(checkin)) {
			checkin = r.CheckTime
		}
		if r.CheckType == "下班" && (checkout.IsZero() || r.CheckTime.After(checkout)) {
			checkout = r.CheckTime
		}
	}
	return checkin, checkout
}

func (s *OvertimeMatchingService) applyOvertimeRules(minutes int) int {
	minThreshold := 30
	cfg, err := s.ruleRepo.FindByKey("overtime.min_threshold_minutes")
	if err == nil {
		var v map[string]interface{}
		if parseJSON(cfg.RuleValueJSON, &v) == nil {
			if val, ok := v["minutes"].(float64); ok {
				minThreshold = int(val)
			}
		}
	}
	if minutes < minThreshold {
		return 0
	}
	return minutes
}

func (s *OvertimeMatchingService) saveMatchResult(a *database.Approval, approvalStart, approvalEnd, attStart, attEnd time.Time, matched, qualified int, status, reason string) error {
	result := &database.OvertimeMatchResult{
		UserID:              a.ApplicantID,
		ApprovalID:          a.ID,
		ApprovalProcessID:   a.ProcessID,
		ApprovalStatus:      a.Status,
		ApprovalStartTime:   approvalStart,
		ApprovalEndTime:     approvalEnd,
		AttendanceStartTime: attStart,
		AttendanceEndTime:   attEnd,
		MatchedMinutes:      matched,
		QualifiedMinutes:    qualified,
		MatchStatus:         status,
		MatchReason:         reason,
		CalcVersion:         "v1",
	}
	return s.matchRepo.Upsert(result)
}
