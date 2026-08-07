package service

import (
	"fmt"
	"log"
	"os"
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"strings"
	"time"

	"gorm.io/gorm"
)

type LeaveJobScheduler struct {
	db *gorm.DB
}

func NewLeaveJobScheduler(db *gorm.DB) *LeaveJobScheduler {
	return &LeaveJobScheduler{db: db}
}

// Start 启动所有年假/调休定时任务
func (s *LeaveJobScheduler) Start() {
	go s.runQuarterlyGrantJob()
	go s.runOvertimeMatchJob()
	go s.runDingTalkRetryJob()
	go s.runLeaveApprovalConsumeJob()
	log.Println("[LeaveJobs] 定时任务已启动")
}

// runQuarterlyGrantJob 每季度第一天凌晨1点执行年假发放
func (s *LeaveJobScheduler) runQuarterlyGrantJob() {
	for {
		next := s.nextQuarterStart()
		log.Printf("[LeaveJobs] 季度年假发放将在 %s 执行", next.Format("2006-01-02 15:04:05"))
		time.Sleep(time.Until(next))
		s.runEligibilityRecalc()
		s.runQuarterGrant()
	}
}

// runOvertimeMatchJob 每天凌晨2点执行加班匹配
func (s *LeaveJobScheduler) runOvertimeMatchJob() {
	for {
		next := s.nextDailyAt(2, 0)
		time.Sleep(time.Until(next))
		s.runOvertimeMatch()
	}
}

func (s *LeaveJobScheduler) runEligibilityRecalc() {
	log.Println("[LeaveJobs] 开始资格重算...")
	year := time.Now().Year()
	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		log.Printf("[LeaveJobs] 读取组织失败: %v", err)
		return
	}
	totalUsers := 0
	for _, orgID := range orgs {
		n, err := s.runEligibilityRecalcForOrg(orgID, year)
		if err != nil {
			log.Printf("[LeaveJobs] org=%s 资格重算失败: %v", orgID, err)
			continue
		}
		totalUsers += n
	}
	log.Printf("[LeaveJobs] 资格重算完成，组织数=%d，共处理 %d 人", len(orgs), totalUsers)
}

func (s *LeaveJobScheduler) runEligibilityRecalcForOrg(orgID string, year int) (int, error) {
	var users []database.User
	if err := s.db.Where("org_id = ?", orgID).Find(&users).Error; err != nil {
		return 0, err
	}
	if len(users) == 0 {
		return 0, nil
	}
	svc := NewAnnualLeaveServiceWithOrgID(s.db, orgID)
	chunk := 50
	for i := 0; i < len(users); i += chunk {
		end := i + chunk
		if end > len(users) {
			end = len(users)
		}
		var ids []string
		for _, u := range users[i:end] {
			ids = append(ids, u.UserID)
		}
		if err := svc.RecalculateEligibilityBatch(year, ids); err != nil {
			return 0, err
		}
	}
	return len(users), nil
}

func (s *LeaveJobScheduler) runQuarterGrant() {
	now := time.Now()
	year := now.Year()
	quarter := (int(now.Month())-1)/3 + 1
	log.Printf("[LeaveJobs] 开始 %d年Q%d 年假发放...", year, quarter)
	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		log.Printf("[LeaveJobs] 读取组织失败: %v", err)
		return
	}
	for _, orgID := range orgs {
		svc := NewAnnualLeaveGrantServiceWithOrgID(s.db, orgID)
		if err := svc.GrantQuarter(year, quarter); err != nil {
			log.Printf("[LeaveJobs] org=%s 季度发放失败: %v", orgID, err)
			continue
		}
		log.Printf("[LeaveJobs] org=%s %d年Q%d 年假发放完成", orgID, year, quarter)
	}
}

func (s *LeaveJobScheduler) runOvertimeMatch() {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	log.Printf("[LeaveJobs] 开始加班匹配，日期: %s", yesterday)
	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		log.Printf("[LeaveJobs] 读取组织失败: %v", err)
		return
	}
	for _, orgID := range orgs {
		svc := NewOvertimeMatchingServiceWithOrgID(s.db, orgID)
		if err := svc.MatchApprovedOvertime(yesterday, yesterday); err != nil {
			log.Printf("[LeaveJobs] org=%s 加班匹配失败: %v", orgID, err)
			continue
		}
		log.Printf("[LeaveJobs] org=%s 加班匹配完成", orgID)
	}
}

// RunManualEligibilityRecalc 手动触发资格重算（供API调用，逐组织执行）
func (s *LeaveJobScheduler) RunManualEligibilityRecalc(year int) error {
	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		return err
	}
	var firstErr error
	for _, orgID := range orgs {
		if _, err := s.runEligibilityRecalcForOrg(orgID, year); err != nil {
			log.Printf("[LeaveJobs] org=%s 手动资格重算失败: %v", orgID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// RunManualEligibilityRecalcForOrg 仅对指定组织做资格重算。
func (s *LeaveJobScheduler) RunManualEligibilityRecalcForOrg(orgID string, year int) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("org_id required")
	}
	_, err := s.runEligibilityRecalcForOrg(orgID, year)
	return err
}

// RunManualOvertimeMatch 手动触发加班匹配（供API调用，逐组织执行）
func (s *LeaveJobScheduler) RunManualOvertimeMatch(startDate, endDate string) error {
	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		return err
	}
	var firstErr error
	for _, orgID := range orgs {
		svc := NewOvertimeMatchingServiceWithOrgID(s.db, orgID)
		if err := svc.MatchApprovedOvertime(startDate, endDate); err != nil {
			log.Printf("[LeaveJobs] org=%s 手动加班匹配失败: %v", orgID, err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// RunManualOvertimeMatchForOrg 仅对指定组织做加班匹配。
func (s *LeaveJobScheduler) RunManualOvertimeMatchForOrg(orgID, startDate, endDate string) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("org_id required")
	}
	return NewOvertimeMatchingServiceWithOrgID(s.db, orgID).MatchApprovedOvertime(startDate, endDate)
}

// SeedDefaultRules 为所有活跃组织初始化默认规则（若不存在）。
// 这是明确的系统初始化/冷启动路径：组织表不可用或尚无活跃组织时，
// 仅允许为 default 组织写入种子规则，不得用于普通业务同步/消费。
func (s *LeaveJobScheduler) SeedDefaultRules() error {
	orgs, err := s.listActiveOrgIDsForSeed()
	if err != nil {
		return err
	}
	for _, orgID := range orgs {
		if err := s.seedDefaultRulesForOrg(orgID); err != nil {
			return err
		}
	}
	log.Printf("[LeaveJobs] 默认规则初始化完成，组织数=%d", len(orgs))
	return nil
}

func (s *LeaveJobScheduler) seedDefaultRulesForOrg(orgID string) error {
	ruleRepo := repository.NewOvertimeRuleConfigRepositoryWithOrgID(s.db, orgID)
	leaveRuleRepo := repository.NewLeaveRuleConfigRepositoryWithOrgID(s.db, orgID)

	overtimeRules := []database.OvertimeRuleConfig{
		{
			OrgID:         orgID,
			RuleKey:       "overtime.min_threshold_minutes",
			RuleName:      "加班最低时长（分钟）",
			RuleValueJSON: `{"minutes": 30}`,
			Status:        "active",
		},
		{
			OrgID:         orgID,
			RuleKey:       "overtime.allow_approve_clock_record",
			RuleName:      "是否允许补卡审批作为有效打卡",
			RuleValueJSON: `{"enabled": false}`,
			Status:        "active",
		},
		{
			OrgID:         orgID,
			RuleKey:       "overtime.rest_day_break_enabled",
			RuleName:      "休息日加班休息扣除开关",
			RuleValueJSON: `{"enabled": true}`,
			Status:        "active",
		},
		{
			OrgID:         orgID,
			RuleKey:       "overtime.rest_day_break_threshold_minutes",
			RuleName:      "休息日加班休息扣除阈值",
			RuleValueJSON: `{"minutes": 360}`,
			Status:        "active",
		},
		{
			OrgID:         orgID,
			RuleKey:       "overtime.rest_day_break_minutes",
			RuleName:      "休息日加班休息扣除分钟",
			RuleValueJSON: `{"minutes": 30}`,
			Status:        "active",
		},
		{
			OrgID:         orgID,
			RuleKey:       "overtime.process_code",
			RuleName:      "钉钉加班审批流程代码",
			RuleValueJSON: `{"code": "overtime"}`,
			Status:        "active",
		},
		{
			OrgID:         orgID,
			RuleKey:       "overtime.max_compensatory_minutes",
			RuleName:      "单次加班调休上限（分钟）",
			RuleValueJSON: `{"minutes": 480}`,
			Status:        "active",
		},
	}
	for i := range overtimeRules {
		if err := ruleRepo.Upsert(&overtimeRules[i]); err != nil {
			return fmt.Errorf("org=%s 初始化加班规则 %s 失败: %w", orgID, overtimeRules[i].RuleKey, err)
		}
	}

	leaveRules := []database.LeaveRuleConfig{
		{
			OrgID:         orgID,
			RuleType:      "eligibility",
			RuleKey:       "eligibility.retroactive_confirmation",
			RuleName:      "转正追溯年假资格",
			RuleValueJSON: `{"enabled": true}`,
			Status:        "active",
		},
		{
			OrgID:         orgID,
			RuleType:      "grant",
			RuleKey:       "grant.working_years_to_days",
			RuleName:      "工龄对应年假天数",
			RuleValueJSON: `[{"min_years":0,"days":5},{"min_years":1,"days":10},{"min_years":10,"days":15}]`,
			Status:        "active",
		},
	}
	for i := range leaveRules {
		if err := leaveRuleRepo.Upsert(&leaveRules[i]); err != nil {
			return fmt.Errorf("org=%s 初始化年假规则 %s 失败: %w", orgID, leaveRules[i].RuleKey, err)
		}
	}
	return nil
}

func (s *LeaveJobScheduler) nextQuarterStart() time.Time {
	now := time.Now()
	month := now.Month()
	quarterStartMonth := ((int(month)-1)/3+1)*3 - 2
	next := time.Date(now.Year(), time.Month(quarterStartMonth+3), 1, 1, 0, 0, 0, time.Local)
	if next.Before(now) {
		next = next.AddDate(0, 3, 0)
	}
	return next
}

func (s *LeaveJobScheduler) nextDailyAt(hour, minute int) time.Time {
	now := time.Now()
	next := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.Local)
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}

// runDingTalkRetryJob 每天凌晨3点重试同步失败的发放记录
func (s *LeaveJobScheduler) runDingTalkRetryJob() {
	for {
		time.Sleep(time.Until(s.nextDailyAt(3, 0)))
		log.Println("[LeaveJobs] 开始重试DingTalk同步失败记录...")
		orgs, err := s.listActiveOrgIDs()
		if err != nil {
			log.Printf("[LeaveJobs] 读取组织失败: %v", err)
			continue
		}
		totalRetried, totalSuccess := 0, 0
		for _, orgID := range orgs {
			grantRepo := repository.NewAnnualLeaveGrantRepositoryWithOrgID(s.db, orgID)
			failed, err := grantRepo.FindFailedSyncGrants()
			if err != nil {
				log.Printf("[LeaveJobs] org=%s 查询失败记录出错: %v", orgID, err)
				continue
			}
			if len(failed) == 0 {
				continue
			}
			svc := NewAnnualLeaveGrantServiceWithOrgID(s.db, orgID)
			for _, g := range failed {
				grant := g
				result := &GrantOperationResult{}
				svc.syncGrantToDingTalk(&grant, result)
				totalRetried++
				if result.DingTalkSyncedCount > 0 {
					totalSuccess++
				}
			}
		}
		log.Printf("[LeaveJobs] DingTalk重试完成，共%d条，成功%d条", totalRetried, totalSuccess)
	}
}

// runLeaveApprovalConsumeJob 每天凌晨2:30扫描已审批的年假申请并自动消费
func (s *LeaveJobScheduler) runLeaveApprovalConsumeJob() {
	for {
		time.Sleep(time.Until(s.nextDailyAt(2, 30)))
		s.runLeaveApprovalConsume()
	}
}

func (s *LeaveJobScheduler) runLeaveApprovalConsume() {
	keyword := strings.TrimSpace(os.Getenv("ANNUAL_LEAVE_APPROVAL_KEYWORD"))
	if keyword == "" {
		keyword = "年假"
	}

	yesterday := time.Now().AddDate(0, 0, -1)
	orgs, err := s.listActiveOrgIDs()
	if err != nil {
		log.Printf("[LeaveJobs] 读取组织失败: %v", err)
		return
	}
	for _, orgID := range orgs {
		var approvals []database.Approval
		// 同时兼容历史小写 completed 与钉钉详情同步的大写 COMPLETED。
		err := s.db.Where("org_id = ? AND LOWER(status) = ? AND finish_time >= ? AND title LIKE ?",
			orgID, "completed", yesterday.Format("2006-01-02"),
			"%"+keyword+"%",
		).Find(&approvals).Error
		if err != nil {
			log.Printf("[LeaveJobs] org=%s 查询年假审批失败: %v", orgID, err)
			continue
		}
		if len(approvals) == 0 {
			continue
		}
		s.consumeAnnualLeaveApprovalsForOrg(orgID, approvals)
	}
}

func (s *LeaveJobScheduler) consumeAnnualLeaveApprovalsForOrg(orgID string, approvals []database.Approval) {
	svc := NewAnnualLeaveGrantServiceWithOrgID(s.db, orgID)
	for _, approval := range approvals {
		if !isAnnualLeaveApprovalConsumable(approval.Status, approvalResultFromExtension(approval.Extension)) {
			log.Printf("[LeaveJobs] org=%s 审批 %s 状态/结果不可消费，跳过 (status=%s)", orgID, approval.ProcessID, approval.Status)
			continue
		}
		days := parseApprovalLeaveDays(approval.Content)
		if days <= 0 {
			log.Printf("[LeaveJobs] org=%s 审批 %s 无法解析天数，跳过（请手动录入）", orgID, approval.ProcessID)
			continue
		}
		ref := "approval:" + approval.ProcessID
		remark := approval.Title + "（自动同步）"
		if err := svc.ConsumeAnnualLeave(approval.ApplicantID, days, ref, remark); err != nil {
			log.Printf("[LeaveJobs] org=%s 年假消费失败 %s: %v", orgID, approval.ProcessID, err)
		} else {
			log.Printf("[LeaveJobs] org=%s 年假消费成功 %s %.2f天", orgID, approval.ApplicantID, days)
		}
	}
}

// listActiveOrgIDs 返回活跃组织 ID，供普通业务后台任务逐组织执行。
// 不在此隐式发明 "default"：组织表查询失败、全局 DB 未初始化或无活跃组织时返回空切片，
// 调用方应跳过本轮（或记录日志），避免把业务写到错误租户。
func (s *LeaveJobScheduler) listActiveOrgIDs() ([]string, error) {
	if database.DB == nil {
		log.Printf("[LeaveJobs] 全局 DB 未初始化，跳过本轮业务任务")
		return []string{}, nil
	}
	orgs, err := database.ListActiveOrganizations()
	if err != nil {
		// 业务任务：组织表不可用时 fail-open-to-empty，禁止静默落 default。
		log.Printf("[LeaveJobs] 列举活跃组织失败，跳过本轮业务任务: %v", err)
		return []string{}, nil
	}
	ids := make([]string, 0, len(orgs))
	for _, org := range orgs {
		id := strings.TrimSpace(org.OrgID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// listActiveOrgIDsForSeed 仅供 SeedDefaultRules 冷启动种子使用。
// 组织表不可用或尚无活跃组织时，允许回退 default 初始化默认规则。
func (s *LeaveJobScheduler) listActiveOrgIDsForSeed() ([]string, error) {
	if database.DB == nil {
		// 冷启动且全局 DB 尚未接线：若 scheduler 自带 db，仍可初始化 default。
		if s != nil && s.db != nil {
			return []string{database.DefaultOrganizationID}, nil
		}
		return nil, fmt.Errorf("database not initialized for seed")
	}
	orgs, err := database.ListActiveOrganizations()
	if err != nil {
		// 冷启动：organizations 表可能尚未创建；初始化 default 规则即可。
		if s != nil && s.db != nil {
			return []string{database.DefaultOrganizationID}, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(orgs))
	for _, org := range orgs {
		id := strings.TrimSpace(org.OrgID)
		if id == "" {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		// 明确的系统初始化：尚无活跃组织时为 default 写入种子。
		return []string{database.DefaultOrganizationID}, nil
	}
	return ids, nil
}

// isAnnualLeaveApprovalConsumable 判断审批是否可作为年假自动消费候选。
// 规则：
// 1) status 大小写不敏感，仅 completed/COMPLETED 可消费；
// 2) result 为 agree/approved/pass/success/同意/通过 时可消费；
// 3) result 为 refuse/rejected/拒绝 等必须跳过；
// 4) result 缺失时兼容历史数据：仅凭 completed 状态放行（旧 SyncApproval 可能未写 result）。
func isAnnualLeaveApprovalConsumable(status, result string) bool {
	if !strings.EqualFold(strings.TrimSpace(status), "completed") {
		return false
	}
	normalized := strings.ToLower(strings.TrimSpace(result))
	// 兼容历史：缺少 result 时不阻断 completed 审批消费。
	if normalized == "" {
		return true
	}
	switch normalized {
	case "agree", "approved", "pass", "success", "同意", "通过":
		return true
	case "refuse", "reject", "rejected", "deny", "denied", "拒绝", "不通过":
		return false
	default:
		// 未知结果保守跳过，避免错误扣减。
		return false
	}
}

func approvalResultFromExtension(ext map[string]interface{}) string {
	if ext == nil {
		return ""
	}
	if v, ok := ext["result"]; ok {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t)
		}
	}
	return ""
}

// parseApprovalLeaveDays 从审批 Content 中解析请假天数，尝试多个常见字段名
func parseApprovalLeaveDays(content map[string]interface{}) float64 {
	if content == nil {
		return 0
	}
	candidates := []string{"leave_days", "leaveDays", "days", "duration", "假期天数", "天数"}
	for _, key := range candidates {
		if v, ok := content[key]; ok {
			switch val := v.(type) {
			case float64:
				return val
			case string:
				var f float64
				if _, err := fmt.Sscanf(val, "%f", &f); err == nil {
					return f
				}
			}
		}
	}
	return 0
}
