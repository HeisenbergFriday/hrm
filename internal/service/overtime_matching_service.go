package service

import (
	"encoding/json"
	"fmt"
	"os"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OvertimeMatchingService struct {
	db         *gorm.DB
	matchRepo  *repository.OvertimeMatchResultRepository
	ledgerRepo *repository.CompensatoryLeaveLedgerRepository
	ruleRepo   *repository.OvertimeRuleConfigRepository
	suppRepo   *repository.SupplementaryRequestRepository
	rematch    *overtimeRematchSession
}

func NewOvertimeMatchingService(db *gorm.DB) *OvertimeMatchingService {
	return &OvertimeMatchingService{
		db:         db,
		matchRepo:  repository.NewOvertimeMatchResultRepository(db),
		ledgerRepo: repository.NewCompensatoryLeaveLedgerRepository(db),
		ruleRepo:   repository.NewOvertimeRuleConfigRepository(db),
		suppRepo:   repository.NewSupplementaryRequestRepository(db),
	}
}

type MatchResult struct {
	ID                       uint       `json:"id"`
	UserID                   string     `json:"user_id"`
	UserName                 string     `json:"user_name"`
	WorkDate                 string     `json:"work_date"`
	ApprovalID               uint       `json:"approval_id"`
	ApprovalProcessID        string     `json:"approval_process_id"`
	ApprovalStatus           string     `json:"approval_status"`
	ApprovalStartTime        time.Time  `json:"approval_start_time"`
	ApprovalEndTime          time.Time  `json:"approval_end_time"`
	ApprovalDurationMinutes  int        `json:"approval_duration_minutes"`
	OvertimeStartTime        time.Time  `json:"overtime_start_time"`
	OvertimeEndTime          time.Time  `json:"overtime_end_time"`
	OvertimeDurationMinutes  int        `json:"overtime_duration_minutes"`
	ActualFirstClockTime     *time.Time `json:"actual_first_clock_time"`
	ActualLastClockTime      *time.Time `json:"actual_last_clock_time"`
	ActualClockSpanMinutes   int        `json:"actual_clock_span_minutes"`
	BreakDeductMinutes       int        `json:"break_deduct_minutes"`
	EffectiveOvertimeMinutes int        `json:"effective_overtime_minutes"`
	MatchedMinutes           int        `json:"matched_minutes"`
	QualifiedMinutes         int        `json:"qualified_minutes"`
	MatchStatus              string     `json:"match_status"`
	MatchReason              string     `json:"match_reason"`
	LocalBalanceStatus       string     `json:"local_balance_status"`
	DingtalkSyncStatus       string     `json:"dingtalk_sync_status"`
	DingtalkSyncRequestID    string     `json:"dingtalk_sync_request_id"`
	DingtalkSyncError        string     `json:"dingtalk_sync_error"`
}

type overtimeRematchSnapshot struct {
	UserID                   string
	WorkDate                 string
	EffectiveOvertimeMinutes int
	DingtalkSyncStatus       string
	DingtalkSyncRequestID    string
}

type overtimeRematchSession struct {
	snapshots  map[string]overtimeRematchSnapshot
	syncScopes map[string]overtimeSyncScope
}

func (s *OvertimeMatchingService) MatchApprovedOvertime(startDate, endDate string) error {
	return s.MatchApprovedOvertimeForUser("", startDate, endDate)
}

func (s *OvertimeMatchingService) MatchApprovedOvertimeForUser(userID, startDate, endDate string) error {
	rangeStart, err := time.ParseInLocation("2006-01-02", startDate, time.Local)
	if err != nil {
		return fmt.Errorf("开始日期格式错误: %w", err)
	}
	rangeEnd, err := time.ParseInLocation("2006-01-02", endDate, time.Local)
	if err != nil {
		return fmt.Errorf("结束日期格式错误: %w", err)
	}
	if rangeEnd.Before(rangeStart) {
		return fmt.Errorf("结束日期不能早于开始日期")
	}

	var approvals []database.Approval
	query := s.db.Where("status IN ?", []string{"completed", "COMPLETED"})
	if trimmedUserID := strings.TrimSpace(userID); trimmedUserID != "" {
		query = query.Where("applicant_id = ?", trimmedUserID)
	}
	if err := query.Find(&approvals).Error; err != nil {
		return err
	}

	// 过滤出符合条件的加班审批
	var filteredApprovals []database.Approval
	for _, a := range approvals {
		if !s.isApprovedOvertimeApproval(&a) {
			continue
		}
		approvalStart, _ := s.extractApprovalTimeWindowQuiet(&a)
		if approvalStart.IsZero() {
			continue
		}
		workDate := time.Date(approvalStart.Year(), approvalStart.Month(), approvalStart.Day(), 0, 0, 0, 0, time.Local)
		if workDate.Before(rangeStart) || workDate.After(rangeEnd) {
			continue
		}
		filteredApprovals = append(filteredApprovals, a)
	}

	// 打印日志
	fmt.Printf("[OvertimeMatch] 本次查询到的审批数量: %d\n", len(filteredApprovals))

	for _, a := range filteredApprovals {
		if err := s.MatchApproval(a.ID); err != nil {
			return fmt.Errorf("审批%d匹配失败: %w", a.ID, err)
		}
	}
	return nil
}

func (s *OvertimeMatchingService) MatchApproval(approvalID uint) error {
	return s.MatchApprovalWithForce(approvalID, false)
}

func (s *OvertimeMatchingService) MatchApprovalWithForce(approvalID uint, force bool) error {
	var approval database.Approval
	if err := s.db.First(&approval, approvalID).Error; err != nil {
		return err
	}
	if !s.isApprovedOvertimeApproval(&approval) {
		return nil
	}

	approvalStart, approvalEnd := s.extractApprovalTimeWindow(&approval)
	if approvalStart.IsZero() || approvalEnd.IsZero() {
		return s.saveMatchResult(&approval, approvalStart, approvalEnd, nil, nil, 0, 0, 0, "unmatched", "审批时间窗口解析失败")
	}

	// 获取审批日期（取开始时间的日期部分）
	approvalDate := approvalStart.Format("2006-01-02")
	// 计算审批时长
	approvalDurationMinutes := int(approvalEnd.Sub(approvalStart).Minutes())

	// 检查是否已经存在该员工当天的匹配记录（幂等控制，含软删除记录避免唯一索引冲突）
	var existingMatch database.OvertimeMatchResult
	err := s.db.Unscoped().Where("user_id = ? AND work_date = ?", approval.ApplicantID, approvalDate).First(&existingMatch).Error
	if err == nil {
		if existingMatch.DeletedAt.Valid {
			// 软删除记录仍占用唯一索引，需物理删除
			if delErr := s.db.Unscoped().Delete(&existingMatch).Error; delErr != nil {
				return delErr
			}
		} else if !force {
			// 已存在有效匹配记录，直接返回
			fmt.Printf("[OvertimeMatch] 跳过已存在的匹配记录: user_id=%s, work_date=%s\n", approval.ApplicantID, approvalDate)
			return s.ensureExistingMatchSettled(&existingMatch)
		} else {
			// 强制重新匹配，物理删除旧记录（软删除同样会保留索引冲突）
			fmt.Printf("[OvertimeMatch] 强制重新匹配，删除旧记录: user_id=%s, work_date=%s\n", approval.ApplicantID, approvalDate)
			if delErr := s.db.Unscoped().Delete(&existingMatch).Error; delErr != nil {
				return delErr
			}
		}
	} else if err != gorm.ErrRecordNotFound {
		return err
	}

	// 查找该员工的考勤打卡记录（窗口：工作日 00:00 到次日 06:00，覆盖夜班跨日场景）
	startOfDay, _ := time.ParseInLocation("2006-01-02", approvalDate, time.Local)
	endOfWindow := startOfDay.Add(30 * time.Hour) // 次日 06:00:00

	var attendances []database.Attendance
	if err := s.db.Where("user_id = ? AND check_time >= ? AND check_time <= ?",
		approval.ApplicantID, startOfDay, endOfWindow).
		Order("check_time asc").Find(&attendances).Error; err != nil {
		return s.saveMatchResult(&approval, approvalStart, approvalEnd, nil, nil, 0, 0, 0, "query_clock_failed", "查询本地打卡记录失败，未生成调休："+err.Error())
	}

	// 过滤有效打卡记录
	validAttendances := s.filterValidAttendances(attendances)

	// 打印日志
	fmt.Printf("[OvertimeMatch] 处理审批: user_id=%s, work_date=%s, approval_duration=%d分钟\n", approval.ApplicantID, approvalDate, approvalDurationMinutes)
	fmt.Printf("[OvertimeMatch] 查询到的打卡记录数量: %d\n", len(attendances))
	fmt.Printf("[OvertimeMatch] 过滤后的有效打卡数量: %d\n", len(validAttendances))

	// 过滤出加班时间窗口内的打卡记录（允许前后2小时缓冲，覆盖提前到岗/略有延迟离开的场景）
	overtimeWindowAttendances := s.filterAttendancesInOvertimeWindow(validAttendances, approvalStart, approvalEnd)
	fmt.Printf("[OvertimeMatch] 加班时间窗口(%s~%s)内有效打卡数量: %d\n",
		approvalStart.Format("15:04"), approvalEnd.Format("15:04"), len(overtimeWindowAttendances))

	if len(overtimeWindowAttendances) == 0 {
		var msg string
		if len(validAttendances) == 0 {
			msg = fmt.Sprintf("审批已通过；当天共%d条打卡记录（全部无效）；加班窗口[%s~%s]无有效打卡，本次加班视为无效，未生成调休",
				len(attendances), approvalStart.Format("15:04"), approvalEnd.Format("15:04"))
		} else {
			msg = fmt.Sprintf("审批已通过；加班窗口[%s~%s]内无有效打卡（窗口外有%d条有效打卡）；本次加班视为无效，未生成调休",
				approvalStart.Format("15:04"), approvalEnd.Format("15:04"), len(validAttendances))
		}
		if err := s.saveMatchResult(&approval, approvalStart, approvalEnd, nil, nil, 0, 0, 0, "no_clock_record", msg); err != nil {
			return err
		}
		_ = s.createSupplementaryRequestIfNotExists(approval.ApplicantID, approvalDate, approval.ID)
		return nil
	}

	if len(overtimeWindowAttendances) < 2 {
		clockInfo := ""
		if len(overtimeWindowAttendances) == 1 {
			clockInfo = fmt.Sprintf("（仅1条：%s）", overtimeWindowAttendances[0].CheckTime.Format("15:04"))
		}
		if err := s.saveMatchResult(&approval, approvalStart, approvalEnd, nil, nil, 0, 0, 0, "insufficient_clock_record",
			fmt.Sprintf("审批已通过；加班窗口[%s~%s]内有效打卡仅%d次%s（不足2次）；无法计算打卡时长，本次加班视为无效，未生成调休",
					approvalStart.Format("15:04"), approvalEnd.Format("15:04"), len(overtimeWindowAttendances), clockInfo)); err != nil {
				return err
			}
			_ = s.createSupplementaryRequestIfNotExists(approval.ApplicantID, approvalDate, approval.ID)
			return nil
			}

	// 取最早和最晚的打卡时间
	checkin := overtimeWindowAttendances[0].CheckTime
	checkout := overtimeWindowAttendances[0].CheckTime
	for _, att := range overtimeWindowAttendances {
		if att.CheckTime.Before(checkin) {
			checkin = att.CheckTime
		}
		if att.CheckTime.After(checkout) {
			checkout = att.CheckTime
		}
	}

	// 计算实际打卡跨度
	actualDuration := checkout.Sub(checkin)
	actualClockSpanMinutes := int(actualDuration.Minutes())
	if actualClockSpanMinutes <= 0 {
		return s.saveMatchResult(&approval, approvalStart, approvalEnd, &checkin, &checkout, actualClockSpanMinutes, 0, 0, "invalid_clock_time", "打卡时间异常，本次加班视为无效加班。")
	}

	// 计算休息扣除时间
	breakDeductMinutes := s.calculateBreakDeduction(actualClockSpanMinutes)

	// 计算有效调休时长
	rawEffectiveMinutes := actualClockSpanMinutes - breakDeductMinutes
	if rawEffectiveMinutes < 0 {
		rawEffectiveMinutes = 0
	}
	// 应用加班规则（不足最低阈值时补足到阈值）
	effectiveOvertimeMinutes := s.applyOvertimeRules(rawEffectiveMinutes)

	// 打印日志
	fmt.Printf("[OvertimeMatch] 最早有效打卡时间: %s\n", checkin.Format("2006-01-02 15:04:05"))
	fmt.Printf("[OvertimeMatch] 最晚有效打卡时间: %s\n", checkout.Format("2006-01-02 15:04:05"))
	fmt.Printf("[OvertimeMatch] 打卡跨度分钟: %d\n", actualClockSpanMinutes)
	fmt.Printf("[OvertimeMatch] 休息扣除分钟: %d\n", breakDeductMinutes)
	fmt.Printf("[OvertimeMatch] 最终调休分钟: %d\n", effectiveOvertimeMinutes)

	status := "matched"
	reason := fmt.Sprintf("加班窗口[%s~%s]；打卡 %s~%s；实际打卡%d分钟，扣除休息%d分钟，有效调休%d分钟",
		approvalStart.Format("15:04"), approvalEnd.Format("15:04"),
		checkin.Format("15:04"), checkout.Format("15:04"),
		actualClockSpanMinutes, breakDeductMinutes, effectiveOvertimeMinutes)
	if rawEffectiveMinutes > 0 && effectiveOvertimeMinutes > rawEffectiveMinutes {
		reason = fmt.Sprintf("加班窗口[%s~%s]；打卡 %s~%s；实际打卡%d分钟，扣除休息%d分钟，实际%d分钟不足%d分钟，补足为%d分钟",
			approvalStart.Format("15:04"), approvalEnd.Format("15:04"),
			checkin.Format("15:04"), checkout.Format("15:04"),
			actualClockSpanMinutes, breakDeductMinutes, rawEffectiveMinutes, effectiveOvertimeMinutes, effectiveOvertimeMinutes)
	}
	if rawEffectiveMinutes > effectiveOvertimeMinutes && effectiveOvertimeMinutes > 0 {
		reason = fmt.Sprintf("加班窗口[%s~%s]；打卡 %s~%s；实际打卡%d分钟，扣除休息%d分钟，有效%d分钟超出封顶上限，已截断为%d分钟",
			approvalStart.Format("15:04"), approvalEnd.Format("15:04"),
			checkin.Format("15:04"), checkout.Format("15:04"),
			actualClockSpanMinutes, breakDeductMinutes, rawEffectiveMinutes, effectiveOvertimeMinutes)
	}
	if effectiveOvertimeMinutes == 0 {
		status = "zero_overtime"
		reason = fmt.Sprintf("加班窗口[%s~%s]；打卡 %s~%s；实际打卡%d分钟，扣除休息%d分钟，扣除后为0，未生成调休",
			approvalStart.Format("15:04"), approvalEnd.Format("15:04"),
			checkin.Format("15:04"), checkout.Format("15:04"),
			actualClockSpanMinutes, breakDeductMinutes)
	}

	if err := s.saveMatchResult(&approval, approvalStart, approvalEnd, &checkin, &checkout, actualClockSpanMinutes, breakDeductMinutes, effectiveOvertimeMinutes, status, reason); err != nil {
		return err
	}
	match, err := s.matchRepo.FindByUserAndWorkDate(approval.ApplicantID, approvalDate)
	if err != nil {
		return err
	}

	// 生成调休台账
	if effectiveOvertimeMinutes > 0 {
		compSvc := NewCompensatoryLeaveService(s.db)
		if err := compSvc.CreditFromOvertime(match.ID); err != nil {
			_ = s.matchRepo.UpdateLocalBalanceStatus(match.ID, "failed")
			_ = s.matchRepo.UpdateStatus(match.ID, "local_balance_failed", "本系统调休余额增加失败："+err.Error())
			fmt.Printf("[OvertimeMatch] 本系统调休余额增加失败: %s\n", err.Error())
			return nil
		}
		_ = s.matchRepo.UpdateLocalBalanceStatus(match.ID, "success")
		match.LocalBalanceStatus = "success"
		fmt.Printf("[OvertimeMatch] 本系统调休余额增加成功\n")
	}
	if handled, err := s.applyHistoricalSyncPolicy(match); err != nil {
		return err
	} else if handled {
		return nil
	}
	if handled, err := s.applyRematchSyncPolicy(match); err != nil {
		return err
	} else if handled {
		return nil
	}
	if effectiveOvertimeMinutes > 0 {
		_ = s.syncOvertimeToDingTalk(match)
	}
	return nil
}

func (s *OvertimeMatchingService) ensureExistingMatchSettled(match *database.OvertimeMatchResult) error {
	if match.EffectiveOvertimeMinutes <= 0 {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(match.LocalBalanceStatus), "success") {
		compSvc := NewCompensatoryLeaveService(s.db)
		if err := compSvc.CreditFromOvertime(match.ID); err != nil {
			_ = s.matchRepo.UpdateLocalBalanceStatus(match.ID, "failed")
			_ = s.matchRepo.UpdateStatus(match.ID, "local_balance_failed", "本系统调休余额增加失败："+err.Error())
			return nil
		}
		_ = s.matchRepo.UpdateLocalBalanceStatus(match.ID, "success")
		match.LocalBalanceStatus = "success"
	}
	if handled, err := s.applyHistoricalSyncPolicy(match); err != nil {
		return err
	} else if handled {
		return nil
	}
	if !strings.EqualFold(strings.TrimSpace(match.DingtalkSyncStatus), "success") {
		return s.syncOvertimeToDingTalk(match)
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

func (s *OvertimeMatchingService) syncOvertimeToDingTalk(match *database.OvertimeMatchResult) error {
	if !overtimeDingTalkSyncEnabled() {
		return s.matchRepo.UpdateSyncStatus(match.ID, "skipped", "", "DingTalk leave sync disabled")
	}
	if strings.EqualFold(strings.TrimSpace(match.DingtalkSyncStatus), "success") {
		return nil
	}

	requestID := fmt.Sprintf("overtime:%s:%s:%d", match.UserID, match.WorkDate, match.ID)
	reason := fmt.Sprintf("休息日加班调休 %s %d分钟", match.WorkDate, match.EffectiveOvertimeMinutes)
	if err := dingtalk.UpdateCompensatoryLeaveQuota(match.UserID, match.EffectiveOvertimeMinutes, match.WorkDate, reason); err != nil {
		_ = s.matchRepo.UpdateSyncStatus(match.ID, "failed", requestID, err.Error())
		_ = s.matchRepo.UpdateStatus(match.ID, "dingtalk_sync_failed", "钉钉调休余额同步失败："+err.Error())
		return nil
	}

	if err := s.matchRepo.UpdateSyncStatus(match.ID, "success", requestID, ""); err != nil {
		return err
	}
	if err := s.matchRepo.UpdateStatus(match.ID, "synced", "本系统调休已增加，钉钉调休余额已同步成功。"); err != nil {
		return err
	}
	return s.upsertOvertimeSyncHistory(match, requestID, "incremental")
}

func overtimeDingTalkSyncEnabled() bool {
	raw := strings.TrimSpace(os.Getenv("DINGTALK_COMP_TIME_SYNC_ENABLED"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("DINGTALK_LEAVE_SYNC_ENABLED"))
	}
	raw = strings.ToLower(raw)
	return raw != "false" && raw != "0" && raw != "no"
}

func (s *OvertimeMatchingService) GetMatchResults(userID, startDate, endDate string) ([]MatchResult, error) {
	rows, err := s.matchRepo.FindByUserDateRange(userID, startDate, endDate)
	if err != nil {
		return nil, err
	}
	var results []MatchResult
	for _, r := range rows {
		results = append(results, MatchResult{
			ID:                       r.ID,
			UserID:                   r.UserID,
			UserName:                 r.UserName,
			WorkDate:                 r.WorkDate,
			ApprovalID:               r.ApprovalID,
			ApprovalProcessID:        r.ApprovalProcessID,
			ApprovalStatus:           r.ApprovalStatus,
			ApprovalStartTime:        r.ApprovalStartTime,
			ApprovalEndTime:          r.ApprovalEndTime,
			ApprovalDurationMinutes:  r.ApprovalDurationMinutes,
			OvertimeStartTime:        r.OvertimeStartTime,
			OvertimeEndTime:          r.OvertimeEndTime,
			OvertimeDurationMinutes:  r.OvertimeDurationMinutes,
			ActualFirstClockTime:     r.ActualFirstClockTime,
			ActualLastClockTime:      r.ActualLastClockTime,
			ActualClockSpanMinutes:   r.ActualClockSpanMinutes,
			BreakDeductMinutes:       r.BreakDeductMinutes,
			EffectiveOvertimeMinutes: r.EffectiveOvertimeMinutes,
			MatchedMinutes:           r.ActualClockSpanMinutes,
			QualifiedMinutes:         r.EffectiveOvertimeMinutes,
			MatchStatus:              r.MatchStatus,
			MatchReason:              r.MatchReason,
			LocalBalanceStatus:       r.LocalBalanceStatus,
			DingtalkSyncStatus:       r.DingtalkSyncStatus,
			DingtalkSyncRequestID:    r.DingtalkSyncRequestID,
			DingtalkSyncError:        r.DingtalkSyncError,
		})
	}
	return results, nil
}

func (s *OvertimeMatchingService) isApprovedOvertimeApproval(a *database.Approval) bool {
	return s.isOvertimeApproval(a) && isCompletedApprovalStatus(a.Status) && isApprovedApprovalResult(a)
}

func isCompletedApprovalStatus(status string) bool {
	return strings.EqualFold(strings.TrimSpace(status), "completed")
}

func isApprovedApprovalResult(a *database.Approval) bool {
	raw, ok := a.Extension["result"].(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return true
	}
	result := strings.ToLower(strings.TrimSpace(raw))
	return result == "agree" || result == "approved" || result == "pass" || result == "success" || result == "同意" || result == "通过"
}

func (s *OvertimeMatchingService) getConfiguredProcessCode() string {
	cfg, err := s.ruleRepo.FindByKey("overtime.process_code")
	if err != nil {
		return ""
	}
	var v map[string]interface{}
	if parseJSON(cfg.RuleValueJSON, &v) != nil {
		return ""
	}
	code, _ := v["code"].(string)
	return strings.TrimSpace(code)
}

func (s *OvertimeMatchingService) isOvertimeApproval(a *database.Approval) bool {
	if ext, ok := a.Extension["process_code"].(string); ok {
		ext = strings.TrimSpace(ext)
		if strings.EqualFold(ext, "PROC-OVERTIME") || strings.EqualFold(ext, "overtime") || strings.Contains(ext, "加班") {
			return true
		}
		if configured := s.getConfiguredProcessCode(); configured != "" && strings.EqualFold(ext, configured) {
			return true
		}
	}
	if cat, ok := a.Extension["category"].(string); ok {
		cat = strings.TrimSpace(cat)
		if strings.EqualFold(cat, "overtime") || strings.Contains(cat, "加班") {
			return true
		}
	}
	// 严格检查标题，只有明确包含"加班"的审批才被识别为加班审批
	title := strings.TrimSpace(a.Title)
	return strings.Contains(title, "加班")
}

func (s *OvertimeMatchingService) extractApprovalTimeWindow(a *database.Approval) (time.Time, time.Time) {
	return s.extractApprovalTimeWindowWithLogging(a, true)
}

func (s *OvertimeMatchingService) extractApprovalTimeWindowQuiet(a *database.Approval) (time.Time, time.Time) {
	return s.extractApprovalTimeWindowWithLogging(a, false)
}

func (s *OvertimeMatchingService) extractApprovalTimeWindowWithLogging(a *database.Approval, shouldLog bool) (time.Time, time.Time) {
	var startStr, endStr string

	// 第一优先：从顶层 Content 中按常见 key 直接取值
	startStr = findApprovalContentValue(a.Content, []string{"start_time", "overtime_start_time", "加班开始时间", "开始时间", "开始", "start", "开始时刻", "开始时间点", "startTime", "from", "_from"})
	endStr = findApprovalContentValue(a.Content, []string{"end_time", "overtime_end_time", "加班结束时间", "结束时间", "结束", "end", "结束时刻", "结束时间点", "finishTime", "to", "_to"})

	// 第二优先：从 "加班" 字段（钉钉 JSON 组件数组）中按 bizAlias 提取
	if startStr == "" || endStr == "" {
		if overtimeJSON, ok := a.Content["加班"].(string); ok && overtimeJSON != "" {
			if startStr == "" {
				startStr = extractDingTalkComponentValue(overtimeJSON, "startTime")
			}
			if endStr == "" {
				endStr = extractDingTalkComponentValue(overtimeJSON, "finishTime")
			}
			// 第三优先：从 NumberField.extValue 中提取 _from / _to
			if startStr == "" {
				startStr = extractDingTalkExtValue(overtimeJSON, "_from")
			}
			if endStr == "" {
				endStr = extractDingTalkExtValue(overtimeJSON, "_to")
			}
			// 第四优先：正则兜底（兼容旧格式）
			if startStr == "" {
				startStr = extractTimeFromOvertimeData(overtimeJSON, "startTime")
			}
			if endStr == "" {
				endStr = extractTimeFromOvertimeData(overtimeJSON, "finishTime")
			}
			if startStr == "" {
				startStr = extractTimeFromOvertimeData(overtimeJSON, "_from")
			}
			if endStr == "" {
				endStr = extractTimeFromOvertimeData(overtimeJSON, "_to")
			}
		}
	}

	if shouldLog {
		fmt.Printf("[OvertimeMatch] 审批ID: %d, 标题: %s\n", a.ID, a.Title)
		fmt.Printf("[OvertimeMatch] 提取的开始时间: %s, 结束时间: %s\n", startStr, endStr)
	}

	start := parseApprovalTime(startStr)
	end := parseApprovalTime(endStr)

	if shouldLog {
		fmt.Printf("[OvertimeMatch] 解析后的开始时间: %v, 结束时间: %v\n", start, end)
	}

	return start, end
}

// extractDingTalkComponentValue 从钉钉表单组件 JSON 数组中按 bizAlias 提取 value
// 钉钉表单结构：[{"props":{"bizAlias":"startTime"},"value":"2026-04-16 09:00"}, ...]
func extractDingTalkComponentValue(overtimeJSON string, bizAlias string) string {
	var components []map[string]interface{}
	if err := json.Unmarshal([]byte(overtimeJSON), &components); err != nil {
		return ""
	}
	for _, comp := range components {
		if v := findComponentByBizAlias(comp, bizAlias); v != "" {
			return v
		}
	}
	return ""
}

func findComponentByBizAlias(comp map[string]interface{}, bizAlias string) string {
	if props, ok := comp["props"].(map[string]interface{}); ok {
		alias, _ := props["bizAlias"].(string)
		if strings.EqualFold(strings.TrimSpace(alias), bizAlias) {
			value, _ := comp["value"].(string)
			return strings.TrimSpace(value)
		}
	}
	// 递归检查 children
	if children, ok := comp["children"].([]interface{}); ok {
		for _, child := range children {
			if childMap, ok := child.(map[string]interface{}); ok {
				if v := findComponentByBizAlias(childMap, bizAlias); v != "" {
					return v
				}
			}
		}
	}
	return ""
}

// extractDingTalkExtValue 从 NumberField.extValue 中提取时间字段（_from / _to）
// extValue 本身是一个 JSON 字符串，包含 {"_from":"2026-04-16 09:00","_to":"2026-04-16 18:00",...}
func extractDingTalkExtValue(overtimeJSON string, key string) string {
	var components []map[string]interface{}
	if err := json.Unmarshal([]byte(overtimeJSON), &components); err != nil {
		return ""
	}
	for _, comp := range components {
		extValueStr, _ := comp["extValue"].(string)
		if extValueStr == "" {
			continue
		}
		var extValue map[string]interface{}
		if err := json.Unmarshal([]byte(extValueStr), &extValue); err != nil {
			continue
		}
		if v, ok := extValue[key].(string); ok && v != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// extractTimeFromOvertimeData 从加班数据中提取时间，兼容字符串值和数字时间戳
func extractTimeFromOvertimeData(data string, key string) string {
	// 先尝试匹配字符串值："key":"value"
	strPattern := fmt.Sprintf(`"%s":"([^"]+)"`, key)
	if matches := regexp.MustCompile(strPattern).FindStringSubmatch(data); len(matches) > 1 {
		return matches[1]
	}
	// 再尝试匹配数字值（Unix 毫秒或秒时间戳）："key":1234567890
	numPattern := fmt.Sprintf(`"%s":(\d{10,13})`, key)
	if matches := regexp.MustCompile(numPattern).FindStringSubmatch(data); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

func findApprovalContentValue(content map[string]interface{}, keys []string) string {
	for _, key := range keys {
		if value := stringifyContentValue(content[key]); value != "" {
			return value
		}
	}
	for key, raw := range content {
		value := stringifyContentValue(raw)
		if value == "" {
			continue
		}
		for _, expected := range keys {
			if strings.Contains(key, expected) {
				return value
			}
		}
	}
	return ""
}

func stringifyContentValue(raw interface{}) string {
	switch v := raw.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	case float64:
		if v > 0 {
			return strconv.FormatInt(int64(v), 10)
		}
	case int64:
		if v > 0 {
			return strconv.FormatInt(v, 10)
		}
	case int:
		if v > 0 {
			return strconv.Itoa(v)
		}
	default:
		return ""
	}
	return ""
}

func parseApprovalTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006/01/02 15:04:05",
		"2006/01/02 15:04",
		time.RFC3339,
	}
	for _, layout := range layouts {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t
		}
	}
	if ts, err := strconv.ParseInt(value, 10, 64); err == nil && ts > 0 {
		if ts > 1_000_000_000_000 {
			return time.UnixMilli(ts).In(time.Local)
		}
		return time.Unix(ts, 0).In(time.Local)
	}
	return time.Time{}
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
	if minutes <= 0 {
		return 0
	}
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
		return minThreshold
	}

	// 封顶上限（单次加班最多折算 N 分钟调休）
	maxCap := 480
	cfgCap, err := s.ruleRepo.FindByKey("overtime.max_compensatory_minutes")
	if err == nil {
		var v map[string]interface{}
		if parseJSON(cfgCap.RuleValueJSON, &v) == nil {
			if val, ok := v["minutes"].(float64); ok {
				maxCap = int(val)
			}
		}
	}
	if minutes > maxCap {
		return maxCap
	}
	return minutes
}

func (s *OvertimeMatchingService) filterAttendancesInOvertimeWindow(
	attendances []database.Attendance,
	approvalStart, approvalEnd time.Time,
) []database.Attendance {
	windowStart := approvalStart.Add(-2 * time.Hour)
	windowEnd := approvalEnd.Add(2 * time.Hour)
	var filtered []database.Attendance
	for _, att := range attendances {
		if !att.CheckTime.Before(windowStart) && !att.CheckTime.After(windowEnd) {
			filtered = append(filtered, att)
		}
	}
	return filtered
}

func (s *OvertimeMatchingService) filterValidAttendances(attendances []database.Attendance) []database.Attendance {
	// 读取配置：是否允许补卡审批作为有效打卡
	allowApproveClockRecord := false
	cfg, err := s.ruleRepo.FindByKey("overtime.allow_approve_clock_record")
	if err == nil {
		var v map[string]interface{}
		if parseJSON(cfg.RuleValueJSON, &v) == nil {
			if val, ok := v["enabled"].(bool); ok {
				allowApproveClockRecord = val
			}
		}
	}
	return filterAttendanceRecordsForCalculation(attendances, allowApproveClockRecord)
}

func (s *OvertimeMatchingService) calculateBreakDeduction(actualClockSpanMinutes int) int {
	// 读取配置
	restDayBreakEnabled := true
	restDayBreakThresholdMinutes := 360 // 6小时
	restDayBreakMinutes := 30           // 0.5小时

	// 休息扣除启用配置
	cfg, err := s.ruleRepo.FindByKey("overtime.rest_day_break_enabled")
	if err == nil {
		var v map[string]interface{}
		if parseJSON(cfg.RuleValueJSON, &v) == nil {
			if val, ok := v["enabled"].(bool); ok {
				restDayBreakEnabled = val
			}
		}
	}

	// 休息扣除阈值配置
	cfg, err = s.ruleRepo.FindByKey("overtime.rest_day_break_threshold_minutes")
	if err == nil {
		var v map[string]interface{}
		if parseJSON(cfg.RuleValueJSON, &v) == nil {
			if val, ok := v["minutes"].(float64); ok {
				restDayBreakThresholdMinutes = int(val)
			}
		}
	}

	// 休息扣除分钟配置
	cfg, err = s.ruleRepo.FindByKey("overtime.rest_day_break_minutes")
	if err == nil {
		var v map[string]interface{}
		if parseJSON(cfg.RuleValueJSON, &v) == nil {
			if val, ok := v["minutes"].(float64); ok {
				restDayBreakMinutes = int(val)
			}
		}
	}

	// 如果启用了休息扣除，并且实际打卡跨度达到或超过阈值，则扣除休息时间
	if restDayBreakEnabled && actualClockSpanMinutes >= restDayBreakThresholdMinutes {
		return restDayBreakMinutes
	}

	return 0
}

func (s *OvertimeMatchingService) saveMatchResult(a *database.Approval, approvalStart, approvalEnd time.Time, firstClock, lastClock *time.Time, actualClockSpanMinutes, breakDeductMinutes, effectiveOvertimeMinutes int, status, reason string) error {
	// approvalStart/approvalEnd 是表单填写的准备加班时段（overtime window）
	overtimeDurationMinutes := int(approvalEnd.Sub(approvalStart).Minutes())
	if overtimeDurationMinutes < 0 {
		overtimeDurationMinutes = 0
	}
	// 审批流耗时 = FinishTime - CreateTime
	approvalDurationMinutes := int(a.FinishTime.Sub(a.CreateTime).Minutes())
	if approvalDurationMinutes < 0 {
		approvalDurationMinutes = 0
	}
	localBalanceStatus := "pending"
	dingtalkSyncStatus := "pending"
	if effectiveOvertimeMinutes <= 0 {
		localBalanceStatus = "skipped"
		dingtalkSyncStatus = "skipped"
	}

	// 优先使用 ApplicantName；若为空或与 ID 相同，从 User 表回填
	userName := a.ApplicantName
	if userName == "" || userName == a.ApplicantID {
		var user database.User
		if s.db.Where("user_id = ?", a.ApplicantID).First(&user).Error == nil && user.Name != "" {
			userName = user.Name
		}
	}

	result := &database.OvertimeMatchResult{
		UserID:                   a.ApplicantID,
		UserName:                 userName,
		WorkDate:                 approvalStart.Format("2006-01-02"),
		MatchRef:                 newOvertimeMatchRef(a.ID, approvalStart),
		ApprovalID:               a.ID,
		ApprovalProcessID:        a.ProcessID,
		ApprovalStatus:           a.Status,
		ApprovalStartTime:        a.CreateTime, // 发起申请时间
		ApprovalEndTime:          a.FinishTime, // 审批流通过时间
		ApprovalDurationMinutes:  approvalDurationMinutes,
		OvertimeStartTime:        approvalStart, // 准备加班开始时间（表单填写）
		OvertimeEndTime:          approvalEnd,   // 准备加班结束时间（表单填写）
		OvertimeDurationMinutes:  overtimeDurationMinutes,
		ActualFirstClockTime:     firstClock,
		ActualLastClockTime:      lastClock,
		ActualClockSpanMinutes:   actualClockSpanMinutes,
		BreakDeductMinutes:       breakDeductMinutes,
		EffectiveOvertimeMinutes: effectiveOvertimeMinutes,
		MatchStatus:              status,
		MatchReason:              reason,
		LocalBalanceStatus:       localBalanceStatus,
		DingtalkSyncStatus:       dingtalkSyncStatus,
		CalcVersion:              "v2",
	}
	return s.matchRepo.Create(result)
}

func newOvertimeMatchRef(approvalID uint, approvalStart time.Time) string {
	return fmt.Sprintf("overtime:%d:%s:%d", approvalID, approvalStart.Format("20060102"), time.Now().UnixNano())
}

// ClearAndRematch 清空指定日期范围内的匹配记录并重新匹配（用于修正历史错误数据）
func (s *OvertimeMatchingService) ClearAndRematch(userID, startDate, endDate string) error {
	matches, err := s.listMatchesForRange(userID, startDate, endDate)
	if err != nil {
		return err
	}
	s.beginRematchSession(matches)
	defer s.finishRematchSession()

	_, _, err = s.rollbackAndDeleteMatches(userID, startDate, endDate)
	if err != nil {
		return fmt.Errorf("清空匹配记录失败: %w", err)
	}
	if err := s.MatchApprovedOvertimeForUser(userID, startDate, endDate); err != nil {
		return err
	}
	return nil
}

// DeleteMatchRecords 仅删除指定日期范围的匹配记录（不重新匹配）
func (s *OvertimeMatchingService) DeleteMatchRecords(userID, startDate, endDate string) (int64, error) {
	_, deletedCount, err := s.rollbackAndDeleteMatches(userID, startDate, endDate)
	if err != nil {
		return 0, err
	}
	return deletedCount, nil
}

type overtimeSyncScope struct {
	UserID string
	Year   int
}

func (s *OvertimeMatchingService) rollbackAndDeleteMatches(userID, startDate, endDate string) ([]overtimeSyncScope, int64, error) {
	matches, err := s.listMatchesForRange(userID, startDate, endDate)
	if err != nil {
		return nil, 0, err
	}
	scopes := collectOvertimeSyncScopes(matches)
	var deletedCount int64
	err = s.db.Transaction(func(tx *gorm.DB) error {
		compSvc := NewCompensatoryLeaveService(tx)
		for _, match := range matches {
			if shouldRollbackOvertimeMatch(match) {
				if err := compSvc.RollbackCredit(match.ID); err != nil {
					return err
				}
			}
		}

		query := tx.Where("work_date >= ? AND work_date <= ?", startDate, endDate)
		if userID != "" {
			query = query.Where("user_id = ?", userID)
		}
		result := query.Unscoped().Delete(&database.OvertimeMatchResult{})
		deletedCount = result.RowsAffected
		return result.Error
	})
	if err != nil {
		return nil, 0, err
	}
	return scopes, deletedCount, nil
}

func (s *OvertimeMatchingService) listMatchesForRange(userID, startDate, endDate string) ([]database.OvertimeMatchResult, error) {
	query := s.db.Where("work_date >= ? AND work_date <= ?", startDate, endDate)
	if userID != "" {
		query = query.Where("user_id = ?", userID)
	}
	var matches []database.OvertimeMatchResult
	if err := query.Find(&matches).Error; err != nil {
		return nil, err
	}
	return matches, nil
}

func collectOvertimeSyncScopes(matches []database.OvertimeMatchResult) []overtimeSyncScope {
	scopeMap := make(map[string]overtimeSyncScope)
	for _, match := range matches {
		year, ok := workDateYear(match.WorkDate)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%s:%d", match.UserID, year)
		scopeMap[key] = overtimeSyncScope{UserID: match.UserID, Year: year}
	}
	scopes := make([]overtimeSyncScope, 0, len(scopeMap))
	for _, scope := range scopeMap {
		scopes = append(scopes, scope)
	}
	return scopes
}

func buildOvertimeRematchSnapshots(matches []database.OvertimeMatchResult) map[string]overtimeRematchSnapshot {
	snapshots := make(map[string]overtimeRematchSnapshot, len(matches))
	for _, match := range matches {
		snapshots[overtimeRematchKey(match.UserID, match.WorkDate)] = overtimeRematchSnapshot{
			UserID:                   match.UserID,
			WorkDate:                 match.WorkDate,
			EffectiveOvertimeMinutes: match.EffectiveOvertimeMinutes,
			DingtalkSyncStatus:       strings.TrimSpace(match.DingtalkSyncStatus),
			DingtalkSyncRequestID:    strings.TrimSpace(match.DingtalkSyncRequestID),
		}
	}
	return snapshots
}

func overtimeRematchKey(userID, workDate string) string {
	return userID + ":" + workDate
}

func (s *OvertimeMatchingService) beginRematchSession(matches []database.OvertimeMatchResult) {
	s.rematch = &overtimeRematchSession{
		snapshots:  buildOvertimeRematchSnapshots(matches),
		syncScopes: make(map[string]overtimeSyncScope),
	}
}

func (s *OvertimeMatchingService) finishRematchSession() {
	s.rematch = nil
}

func (s *OvertimeMatchingService) rematchScopes() []overtimeSyncScope {
	if s.rematch == nil {
		return nil
	}
	scopes := make([]overtimeSyncScope, 0, len(s.rematch.syncScopes))
	for _, scope := range s.rematch.syncScopes {
		scopes = append(scopes, scope)
	}
	return scopes
}

func (s *OvertimeMatchingService) addRematchSyncScope(userID, workDate string) {
	if s.rematch == nil {
		return
	}
	year, ok := workDateYear(workDate)
	if !ok {
		return
	}
	key := fmt.Sprintf("%s:%d", userID, year)
	s.rematch.syncScopes[key] = overtimeSyncScope{UserID: userID, Year: year}
}

func (s *OvertimeMatchingService) applyRematchSyncPolicy(match *database.OvertimeMatchResult) (bool, error) {
	if s.rematch == nil {
		return false, nil
	}

	snapshot, ok := s.rematch.snapshots[overtimeRematchKey(match.UserID, match.WorkDate)]
	if ok &&
		strings.EqualFold(snapshot.DingtalkSyncStatus, "success") &&
		match.EffectiveOvertimeMinutes > 0 &&
		strings.EqualFold(strings.TrimSpace(match.LocalBalanceStatus), "success") {
		requestID := snapshot.DingtalkSyncRequestID
		if requestID == "" {
			requestID = fmt.Sprintf("preserved:%s:%s", match.UserID, match.WorkDate)
		}
		if err := s.markMatchSyncedWithoutPush(match, requestID, "沿用历史同步结果，不再重复发放"); err != nil {
			return true, err
		}
		if err := s.upsertOvertimeSyncHistory(match, requestID, "preserved"); err != nil {
			return true, err
		}
		return true, nil
	}

	return false, nil
}

func (s *OvertimeMatchingService) applyHistoricalSyncPolicy(match *database.OvertimeMatchResult) (bool, error) {
	if match == nil || match.EffectiveOvertimeMinutes <= 0 {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(match.LocalBalanceStatus), "success") {
		return false, nil
	}

	history, err := s.findOvertimeSyncHistory(match.UserID, match.WorkDate)
	if err == gorm.ErrRecordNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	requestID := strings.TrimSpace(history.SyncRequestID)
	if requestID == "" {
		requestID = fmt.Sprintf("preserved:%s:%s", match.UserID, match.WorkDate)
	}
	if err := s.markMatchSyncedWithoutPush(match, requestID, "已同步过的加班不再重复发放"); err != nil {
		return true, err
	}
	return true, nil
}

func (s *OvertimeMatchingService) markMatchSyncedWithoutPush(match *database.OvertimeMatchResult, requestID, reason string) error {
	if err := s.matchRepo.UpdateSyncStatus(match.ID, "success", requestID, ""); err != nil {
		return err
	}
	return s.matchRepo.UpdateStatus(match.ID, "synced", reason)
}

func (s *OvertimeMatchingService) findOvertimeSyncHistory(userID, workDate string) (*database.OvertimeSyncHistory, error) {
	var history database.OvertimeSyncHistory
	if err := s.db.Where("user_id = ? AND work_date = ?", userID, workDate).First(&history).Error; err != nil {
		return nil, err
	}
	return &history, nil
}

func (s *OvertimeMatchingService) upsertOvertimeSyncHistory(match *database.OvertimeMatchResult, requestID, syncMode string) error {
	if match == nil || match.EffectiveOvertimeMinutes <= 0 {
		return nil
	}
	now := time.Now()
	history := database.OvertimeSyncHistory{
		UserID:                   match.UserID,
		WorkDate:                 match.WorkDate,
		ApprovalID:               match.ApprovalID,
		ApprovalProcessID:        match.ApprovalProcessID,
		EffectiveOvertimeMinutes: match.EffectiveOvertimeMinutes,
		SyncRequestID:            requestID,
		SyncMode:                 syncMode,
		SyncedAt:                 &now,
	}
	return s.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "user_id"},
			{Name: "work_date"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"approval_id",
			"approval_process_id",
			"effective_overtime_minutes",
			"sync_request_id",
			"sync_mode",
			"synced_at",
			"updated_at",
		}),
	}).Create(&history).Error
}

func shouldRollbackOvertimeMatch(match database.OvertimeMatchResult) bool {
	return match.EffectiveOvertimeMinutes > 0 && !strings.EqualFold(strings.TrimSpace(match.MatchStatus), "rolled_back")
}

func workDateYear(workDate string) (int, bool) {
	parsed, err := time.ParseInLocation("2006-01-02", workDate, time.Local)
	if err != nil {
		return 0, false
	}
	return parsed.Year(), true
}

func (s *OvertimeMatchingService) syncAbsoluteOvertimeBalances(scopes []overtimeSyncScope) error {
	if !overtimeDingTalkSyncEnabled() || len(scopes) == 0 {
		return nil
	}

	compSvc := NewCompensatoryLeaveService(s.db)
	for _, scope := range scopes {
		totalMinutes, err := compSvc.GetOvertimeBalanceByYear(scope.UserID, scope.Year)
		if err != nil {
			return err
		}

		reason := fmt.Sprintf("加班匹配重算回写 %d", scope.Year)
		if err := dingtalk.SetCompensatoryLeaveQuota(scope.UserID, scope.Year, totalMinutes, reason); err != nil {
			_ = s.markOvertimeYearSyncFailed(scope, err)
			return fmt.Errorf("user %s year %d: %w", scope.UserID, scope.Year, err)
		}
		if err := s.markOvertimeYearSynced(scope); err != nil {
			return err
		}
	}
	return nil
}

func (s *OvertimeMatchingService) markOvertimeYearSynced(scope overtimeSyncScope) error {
	startDate := fmt.Sprintf("%04d-01-01", scope.Year)
	endDate := fmt.Sprintf("%04d-12-31", scope.Year)
	requestID := fmt.Sprintf("absolute:%s:%d:%d", scope.UserID, scope.Year, time.Now().UnixNano())
	return s.db.Model(&database.OvertimeMatchResult{}).
		Where("user_id = ? AND work_date >= ? AND work_date <= ? AND effective_overtime_minutes > 0 AND local_balance_status = ?", scope.UserID, startDate, endDate, "success").
		Updates(map[string]interface{}{
			"dingtalk_sync_status":     "success",
			"dingtalk_sync_request_id": requestID,
			"dingtalk_sync_error":      "",
			"match_status":             "synced",
		}).Error
}

func (s *OvertimeMatchingService) markOvertimeYearSyncFailed(scope overtimeSyncScope, syncErr error) error {
	startDate := fmt.Sprintf("%04d-01-01", scope.Year)
	endDate := fmt.Sprintf("%04d-12-31", scope.Year)
	return s.db.Model(&database.OvertimeMatchResult{}).
		Where("user_id = ? AND work_date >= ? AND work_date <= ? AND effective_overtime_minutes > 0 AND local_balance_status = ?", scope.UserID, startDate, endDate, "success").
		Updates(map[string]interface{}{
			"dingtalk_sync_status": "failed",
			"dingtalk_sync_error":  syncErr.Error(),
			"match_status":         "dingtalk_sync_failed",
		}).Error
}

// createSupplementaryRequestIfNotExists 为无打卡记录的加班匹配创建补卡申请记录
func (s *OvertimeMatchingService) createSupplementaryRequestIfNotExists(userID, workDate string, approvalID uint) error {
	match, err := s.matchRepo.FindByUserAndWorkDate(userID, workDate)
	if err != nil {
		return err
	}
	existing, err := s.suppRepo.FindPendingByMatchResultID(match.ID)
	if err == nil && existing != nil {
		return nil
	}
	req := &database.OvertimeSupplementaryRequest{
		MatchResultID: match.ID,
		UserID:        userID,
		WorkDate:      workDate,
		ApprovalID:    approvalID,
		Status:        "pending",
	}
	return s.suppRepo.Create(req)
}

// ApproveSupplementaryRequest 审批通过补卡申请，使用补卡时间重新匹配加班
func (s *OvertimeMatchingService) ApproveSupplementaryRequest(requestID uint, clockIn, clockOut time.Time, approvedBy string) error {
	suppReq, err := s.suppRepo.FindByID(requestID)
	if err != nil {
		return fmt.Errorf("补卡申请不存在: %w", err)
	}
	if suppReq.Status != "pending" {
		return fmt.Errorf("补卡申请状态为%s，无法审批", suppReq.Status)
	}

	// 更新补卡申请状态为已通过
	if err := s.suppRepo.Approve(requestID, approvedBy, clockIn, clockOut); err != nil {
		return fmt.Errorf("更新补卡申请状态失败: %w", err)
	}

	// 查找关联的匹配记录
	var match database.OvertimeMatchResult
	if err := s.db.First(&match, suppReq.MatchResultID).Error; err != nil {
		return fmt.Errorf("匹配记录不存在: %w", err)
	}

	// 获取加班审批信息
	var approval database.Approval
	if err := s.db.First(&approval, match.ApprovalID).Error; err != nil {
		return fmt.Errorf("审批记录不存在: %w", err)
	}

	approvalStart, approvalEnd := s.extractApprovalTimeWindow(&approval)
	if approvalStart.IsZero() || approvalEnd.IsZero() {
		return fmt.Errorf("审批时间窗口解析失败")
	}

	// 使用补卡时间计算有效调休
	actualDuration := clockOut.Sub(clockIn)
	actualClockSpanMinutes := int(actualDuration.Minutes())
	if actualClockSpanMinutes <= 0 {
		return fmt.Errorf("补卡时间异常：结束时间需晚于开始时间")
	}

	breakDeductMinutes := s.calculateBreakDeduction(actualClockSpanMinutes)
	rawEffectiveMinutes := actualClockSpanMinutes - breakDeductMinutes
	if rawEffectiveMinutes < 0 {
		rawEffectiveMinutes = 0
	}
	effectiveOvertimeMinutes := s.applyOvertimeRules(rawEffectiveMinutes)

	// 删除旧的匹配记录（物理删除以避免唯一索引冲突）
	if err := s.db.Unscoped().Delete(&match).Error; err != nil {
		return fmt.Errorf("删除旧匹配记录失败: %w", err)
	}

	// 回滚旧的调休台账
	if match.EffectiveOvertimeMinutes > 0 && !strings.EqualFold(strings.TrimSpace(match.MatchStatus), "rolled_back") {
		compSvc := NewCompensatoryLeaveService(s.db)
		_ = compSvc.RollbackCredit(match.ID)
	}

	// 保存新的匹配结果
	status := "matched"
	reason := fmt.Sprintf("补卡审批通过；补卡 %s~%s；实际打卡%d分钟，扣除休息%d分钟，有效调休%d分钟",
		clockIn.Format("15:04"), clockOut.Format("15:04"),
		actualClockSpanMinutes, breakDeductMinutes, effectiveOvertimeMinutes)

	if effectiveOvertimeMinutes == 0 {
		status = "zero_overtime"
		reason = fmt.Sprintf("补卡审批通过；补卡 %s~%s；实际打卡%d分钟，扣除休息%d分钟，扣除后为0，未生成调休",
			clockIn.Format("15:04"), clockOut.Format("15:04"),
			actualClockSpanMinutes, breakDeductMinutes)
	}

	if err := s.saveMatchResult(&approval, approvalStart, approvalEnd, &clockIn, &clockOut, actualClockSpanMinutes, breakDeductMinutes, effectiveOvertimeMinutes, status, reason); err != nil {
		return fmt.Errorf("保存匹配结果失败: %w", err)
	}

	// 生成调休台账
	if effectiveOvertimeMinutes > 0 {
		newMatch, err := s.matchRepo.FindByUserAndWorkDate(match.UserID, match.WorkDate)
		if err != nil {
			return fmt.Errorf("查找新匹配记录失败: %w", err)
		}
		compSvc := NewCompensatoryLeaveService(s.db)
		if err := compSvc.CreditFromOvertime(newMatch.ID); err != nil {
			_ = s.matchRepo.UpdateLocalBalanceStatus(newMatch.ID, "failed")
			_ = s.matchRepo.UpdateStatus(newMatch.ID, "local_balance_failed", "本系统调休余额增加失败："+err.Error())
			return nil
		}
		_ = s.matchRepo.UpdateLocalBalanceStatus(newMatch.ID, "success")
		_ = s.syncOvertimeToDingTalk(newMatch)
	}

	return nil
}

// RejectSupplementaryRequest 拒绝补卡申请
func (s *OvertimeMatchingService) RejectSupplementaryRequest(requestID uint, rejectedReason string) error {
	suppReq, err := s.suppRepo.FindByID(requestID)
	if err != nil {
		return fmt.Errorf("补卡申请不存在: %w", err)
	}
	if suppReq.Status != "pending" {
		return fmt.Errorf("补卡申请状态为%s，无法操作", suppReq.Status)
	}
	return s.suppRepo.Reject(requestID, rejectedReason)
}

// GetSupplementaryRequests 获取补卡申请列表
func (s *OvertimeMatchingService) GetSupplementaryRequests(userID, startDate, endDate string) ([]database.OvertimeSupplementaryRequest, error) {
	return s.suppRepo.FindByUserID(userID, startDate, endDate)
}
