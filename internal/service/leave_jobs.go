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
	var users []database.User
	if err := s.db.Find(&users).Error; err != nil {
		log.Printf("[LeaveJobs] 读取用户失败: %v", err)
		return
	}
	svc := NewAnnualLeaveService(s.db)
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
			log.Printf("[LeaveJobs] 批量资格重算失败: %v", err)
		}
	}
	log.Printf("[LeaveJobs] 资格重算完成，共处理 %d 人", len(users))
}

func (s *LeaveJobScheduler) runQuarterGrant() {
	now := time.Now()
	year := now.Year()
	quarter := (int(now.Month())-1)/3 + 1
	log.Printf("[LeaveJobs] 开始 %d年Q%d 年假发放...", year, quarter)
	svc := NewAnnualLeaveGrantService(s.db)
	if err := svc.GrantQuarter(year, quarter); err != nil {
		log.Printf("[LeaveJobs] 季度发放失败: %v", err)
		return
	}
	log.Printf("[LeaveJobs] %d年Q%d 年假发放完成", year, quarter)
}

func (s *LeaveJobScheduler) runOvertimeMatch() {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	log.Printf("[LeaveJobs] 开始加班匹配，日期: %s", yesterday)
	svc := NewOvertimeMatchingService(s.db)
	if err := svc.MatchApprovedOvertime(yesterday, yesterday); err != nil {
		log.Printf("[LeaveJobs] 加班匹配失败: %v", err)
		return
	}
	log.Printf("[LeaveJobs] 加班匹配完成")
}

// RunManualEligibilityRecalc 手动触发资格重算（供API调用）
func (s *LeaveJobScheduler) RunManualEligibilityRecalc(year int) error {
	var users []database.User
	if err := s.db.Find(&users).Error; err != nil {
		return err
	}
	svc := NewAnnualLeaveService(s.db)
	var ids []string
	for _, u := range users {
		ids = append(ids, u.UserID)
	}
	return svc.RecalculateEligibilityBatch(year, ids)
}

// RunManualOvertimeMatch 手动触发加班匹配（供API调用）
func (s *LeaveJobScheduler) RunManualOvertimeMatch(startDate, endDate string) error {
	svc := NewOvertimeMatchingService(s.db)
	return svc.MatchApprovedOvertime(startDate, endDate)
}

// VerifyNoOvertimeRuleConfigs 初始化默认规则（若不存在）
func (s *LeaveJobScheduler) SeedDefaultRules() {
	ruleRepo := repository.NewOvertimeRuleConfigRepository(s.db)
	leaveRuleRepo := repository.NewLeaveRuleConfigRepository(s.db)

	_ = ruleRepo.Upsert(&database.OvertimeRuleConfig{
		RuleKey:       "overtime.min_threshold_minutes",
		RuleName:      "加班最低时长（分钟）",
		RuleValueJSON: `{"minutes": 30}`,
		Status:        "active",
	})

	_ = leaveRuleRepo.Upsert(&database.LeaveRuleConfig{
		RuleType:      "eligibility",
		RuleKey:       "eligibility.retroactive_confirmation",
		RuleName:      "转正追溯年假资格",
		RuleValueJSON: `{"enabled": true}`,
		Status:        "active",
	})

	_ = leaveRuleRepo.Upsert(&database.LeaveRuleConfig{
		RuleType:      "grant",
		RuleKey:       "grant.working_years_to_days",
		RuleName:      "工龄对应年假天数",
		RuleValueJSON: `[{"min_years":0,"days":5},{"min_years":1,"days":10},{"min_years":10,"days":15}]`,
		Status:        "active",
	})

	log.Println("[LeaveJobs] 默认规则初始化完成")
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
		grantRepo := repository.NewAnnualLeaveGrantRepository(s.db)
		failed, err := grantRepo.FindFailedSyncGrants()
		if err != nil {
			log.Printf("[LeaveJobs] 查询失败记录出错: %v", err)
			continue
		}
		if len(failed) == 0 {
			log.Println("[LeaveJobs] 无失败同步记录，跳过")
			continue
		}
		svc := NewAnnualLeaveGrantService(s.db)
		retried, success := 0, 0
		for _, g := range failed {
			grant := g
			result := &GrantOperationResult{}
			svc.syncGrantToDingTalk(&grant, result)
			retried++
			if result.DingTalkSyncedCount > 0 {
				success++
			}
		}
		log.Printf("[LeaveJobs] DingTalk重试完成，共%d条，成功%d条", retried, success)
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
	var approvals []database.Approval
	err := s.db.Where("status = ? AND finish_time >= ? AND title LIKE ?",
		"completed", yesterday.Format("2006-01-02"),
		"%"+keyword+"%",
	).Find(&approvals).Error
	if err != nil {
		log.Printf("[LeaveJobs] 查询年假审批失败: %v", err)
		return
	}

	if len(approvals) == 0 {
		return
	}

	svc := NewAnnualLeaveGrantService(s.db)
	for _, approval := range approvals {
		days := parseApprovalLeaveDays(approval.Content)
		if days <= 0 {
			log.Printf("[LeaveJobs] 审批 %s 无法解析天数，跳过（请手动录入）", approval.ProcessID)
			continue
		}
		ref := "approval:" + approval.ProcessID
		remark := approval.Title + "（自动同步）"
		if err := svc.ConsumeAnnualLeave(approval.ApplicantID, days, ref, remark); err != nil {
			log.Printf("[LeaveJobs] 年假消费失败 %s: %v", approval.ProcessID, err)
		} else {
			log.Printf("[LeaveJobs] 年假消费成功 %s %.2f天", approval.ApplicantID, days)
		}
	}
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
