package service

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/repository"
	"strings"
	"time"

	"gorm.io/gorm"
)

type AnnualLeaveGrantService struct {
	db              *gorm.DB
	grantRepo       *repository.AnnualLeaveGrantRepository
	eligibilityRepo *repository.AnnualLeaveEligibilityRepository
	ruleRepo        *repository.LeaveRuleConfigRepository
	nowFn           func() time.Time
}

func NewAnnualLeaveGrantService(db *gorm.DB) *AnnualLeaveGrantService {
	return &AnnualLeaveGrantService{
		db:              db,
		grantRepo:       repository.NewAnnualLeaveGrantRepository(db),
		eligibilityRepo: repository.NewAnnualLeaveEligibilityRepository(db),
		ruleRepo:        repository.NewLeaveRuleConfigRepository(db),
		nowFn:           time.Now,
	}
}

type GrantRecord struct {
	ID              uint    `json:"id"`
	UserID          string  `json:"user_id"`
	Year            int     `json:"year"`
	Quarter         int     `json:"quarter"`
	WorkingYears    float64 `json:"working_years"`
	BaseDays        float64 `json:"base_days"`
	GrantedDays     float64 `json:"granted_days"`
	RetroactiveDays float64 `json:"retroactive_days"`
	UsedDays        float64 `json:"used_days"`
	RemainingDays   float64 `json:"remaining_days"`
	GrantType       string  `json:"grant_type"`
	Remark          string  `json:"remark"`
	DingTalkStatus  string  `json:"dingtalk_sync_status"`
	DingTalkError   string  `json:"dingtalk_sync_error"`
}

type GrantOperationResult struct {
	CreatedCount        int      `json:"created_count"`
	SkippedCount        int      `json:"skipped_count"`
	DingTalkSyncedCount int      `json:"dingtalk_synced_count"`
	DingTalkFailedCount int      `json:"dingtalk_failed_count"`
	TotalDays           float64  `json:"total_days"`
	Errors              []string `json:"errors,omitempty"`
}

func (s *AnnualLeaveGrantService) GrantQuarter(year, quarter int) error {
	_, err := s.GrantQuarterWithResult(year, quarter)
	return err
}

func (s *AnnualLeaveGrantService) GrantQuarterWithResult(year, quarter int) (*GrantOperationResult, error) {
	result := &GrantOperationResult{}
	eligibilities, err := s.eligibilityRepo.FindEligibleByYear(year)
	if err != nil {
		return result, err
	}
	for _, e := range eligibilities {
		if e.Quarter != quarter {
			continue
		}
		userResult, err := s.GrantForUserWithResult(e.UserID, year, quarter)
		mergeGrantOperationResult(result, userResult)
		if err != nil {
			return result, fmt.Errorf("用户%s发放失败: %w", e.UserID, err)
		}
	}
	return result, nil
}

func (s *AnnualLeaveGrantService) GrantForUser(userID string, year, quarter int) error {
	_, err := s.GrantForUserWithResult(userID, year, quarter)
	return err
}

func (s *AnnualLeaveGrantService) GrantForUserWithResult(userID string, year, quarter int) (*GrantOperationResult, error) {
	result := &GrantOperationResult{}
	elig, err := s.eligibilityRepo.FindByUserYearQuarter(userID, year, quarter)
	if err != nil || !elig.IsEligible {
		result.SkippedCount++
		return result, nil
	}

	existing, err := s.grantRepo.FindByUserYearQuarterType(userID, year, quarter, "normal")
	if err == nil && existing != nil {
		result.SkippedCount++
		if existing.DingTalkSyncStatus != "success" {
			s.syncGrantToDingTalk(existing, result)
			if existing.DingTalkSyncStatus == "failed" {
				return result, fmt.Errorf("%s", existing.DingTalkSyncError)
			}
		}
		return result, nil
	}

	workingYears := s.calcWorkingYears(elig.EntryDate, year)
	baseDays := s.mapWorkingYearsToDays(workingYears)
	quarterlyDays := baseDays / 4.0

	grant := &database.AnnualLeaveGrant{
		UserID:              userID,
		Year:                year,
		Quarter:             quarter,
		WorkingYears:        workingYears,
		BaseDays:            baseDays,
		GrantedDays:         quarterlyDays,
		RemainingDays:       quarterlyDays,
		GrantType:           "normal",
		SourceEligibilityID: elig.ID,
		Remark:              fmt.Sprintf("Q%d正常发放，工龄%.1f年", quarter, workingYears),
	}
	if err := s.grantRepo.Create(grant); err != nil {
		return result, err
	}
	result.CreatedCount++
	s.syncGrantToDingTalk(grant, result)
	if grant.DingTalkSyncStatus == "failed" {
		return result, fmt.Errorf("%s", grant.DingTalkSyncError)
	}
	return result, nil
}

func (s *AnnualLeaveGrantService) RegrantForEligibilityChange(userID string, year int) error {
	_, err := s.RegrantForEligibilityChangeWithResult(userID, year)
	return err
}

func (s *AnnualLeaveGrantService) RegrantForEligibilityChangeWithResult(userID string, year int) (*GrantOperationResult, error) {
	result := &GrantOperationResult{}
	if err := NewAnnualLeaveService(s.db).RecalculateEligibility(userID, year); err != nil {
		return result, err
	}

	eligibilities, err := s.eligibilityRepo.FindByUserYear(userID, year)
	if err != nil {
		return result, err
	}
	for _, e := range eligibilities {
		if !e.IsEligible || e.RetroactiveSourceQuarter == 0 {
			result.SkippedCount++
			continue
		}

		existing, err := s.grantRepo.FindByUserYearQuarterType(userID, year, e.Quarter, "retroactive")
		if err == nil && existing != nil {
			result.SkippedCount++
			if existing.DingTalkSyncStatus != "success" {
				s.syncGrantToDingTalk(existing, result)
				if existing.DingTalkSyncStatus == "failed" {
					return result, fmt.Errorf("%s", existing.DingTalkSyncError)
				}
			}
			continue
		}

		workingYears := s.calcWorkingYears(e.EntryDate, year)
		baseDays := s.mapWorkingYearsToDays(workingYears)
		retroDays := baseDays / 4.0
		grant := &database.AnnualLeaveGrant{
			UserID:              userID,
			Year:                year,
			Quarter:             e.Quarter,
			WorkingYears:        workingYears,
			BaseDays:            baseDays,
			RetroactiveDays:     retroDays,
			GrantedDays:         0,
			RemainingDays:       retroDays,
			GrantType:           "retroactive",
			SourceEligibilityID: e.ID,
			Remark:              fmt.Sprintf("Q%d追溯发放（Q%d转正）", e.Quarter, e.RetroactiveSourceQuarter),
		}
		if err := s.grantRepo.Create(grant); err != nil {
			return result, err
		}
		result.CreatedCount++
		s.syncGrantToDingTalk(grant, result)
		if grant.DingTalkSyncStatus == "failed" {
			return result, fmt.Errorf("%s", grant.DingTalkSyncError)
		}
	}
	return result, nil
}

func (s *AnnualLeaveGrantService) GetGrantLedger(userID string, year int) ([]GrantRecord, error) {
	rows, err := s.grantRepo.FindByUserYear(userID, year)
	if err != nil {
		return nil, err
	}

	currentWorkingYears, hasCurrentWorkingYears := s.lookupCurrentWorkingYears(userID, year)

	records := make([]GrantRecord, 0, len(rows))
	for _, r := range rows {
		workingYears := r.WorkingYears
		if hasCurrentWorkingYears {
			workingYears = currentWorkingYears
		}

		remark := r.Remark
		if r.GrantType == "normal" && hasCurrentWorkingYears {
			remark = fmt.Sprintf("Q%d正常发放，工龄%.1f年", r.Quarter, workingYears)
		}

		records = append(records, GrantRecord{
			ID:              r.ID,
			UserID:          r.UserID,
			Year:            r.Year,
			Quarter:         r.Quarter,
			WorkingYears:    workingYears,
			BaseDays:        r.BaseDays,
			GrantedDays:     r.GrantedDays + r.RetroactiveDays,
			RetroactiveDays: r.RetroactiveDays,
			UsedDays:        r.UsedDays,
			RemainingDays:   r.RemainingDays,
			GrantType:       r.GrantType,
			Remark:          remark,
			DingTalkStatus:  r.DingTalkSyncStatus,
			DingTalkError:   r.DingTalkSyncError,
		})
	}
	return records, nil
}

func (s *AnnualLeaveGrantService) syncGrantToDingTalk(grant *database.AnnualLeaveGrant, result *GrantOperationResult) {
	days := grant.GrantedDays + grant.RetroactiveDays
	result.TotalDays += days

	setStatus := func(status, errMsg string, syncedAt *time.Time) {
		grant.DingTalkSyncStatus = status
		grant.DingTalkSyncError = errMsg
		if syncedAt != nil {
			grant.DingTalkSyncedAt = syncedAt
		}
		_ = s.grantRepo.UpdateSyncStatus(grant.ID, status, errMsg, syncedAt)
	}

	if days <= 0 {
		setStatus("skipped", "", nil)
		return
	}
	if !leaveDingTalkSyncEnabled() {
		setStatus("skipped", "DINGTALK_LEAVE_SYNC_ENABLED=false", nil)
		return
	}
	if strings.EqualFold(strings.TrimSpace(grant.DingTalkSyncStatus), "success") {
		result.DingTalkSyncedCount++
		return
	}

	reason := fmt.Sprintf("%d年Q%d%s %.2f天", grant.Year, grant.Quarter, grantTypeLabel(grant.GrantType), days)
	if grant.Remark != "" {
		reason = grant.Remark
	}
	if err := dingtalk.UpdateAnnualLeaveQuota(grant.UserID, grant.Year, days, reason); err != nil {
		log.Printf("[leave-sync] 同步失败 grantID=%d userID=%s year=%d days=%.2f err=%v", grant.ID, grant.UserID, grant.Year, days, err)
		result.DingTalkFailedCount++
		result.Errors = append(result.Errors, fmt.Sprintf("%s Q%d: %s", grant.UserID, grant.Quarter, err.Error()))
		setStatus("failed", err.Error(), nil)
		return
	}

	now := time.Now()
	result.DingTalkSyncedCount++
	setStatus("success", "", &now)
}

func leaveDingTalkSyncEnabled() bool {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv("DINGTALK_LEAVE_SYNC_ENABLED")))
	return raw != "false" && raw != "0" && raw != "no"
}

func mergeGrantOperationResult(dst, src *GrantOperationResult) {
	if dst == nil || src == nil {
		return
	}
	dst.CreatedCount += src.CreatedCount
	dst.SkippedCount += src.SkippedCount
	dst.DingTalkSyncedCount += src.DingTalkSyncedCount
	dst.DingTalkFailedCount += src.DingTalkFailedCount
	dst.TotalDays += src.TotalDays
	dst.Errors = append(dst.Errors, src.Errors...)
}

func grantTypeLabel(grantType string) string {
	switch grantType {
	case "retroactive":
		return "追溯补发"
	case "adjustment":
		return "调整"
	default:
		return "正常发放"
	}
}

func (s *AnnualLeaveGrantService) lookupCurrentWorkingYears(userID string, year int) (float64, bool) {
	var profile database.EmployeeProfile
	if err := s.db.Select("entry_date").Where("user_id = ?", userID).First(&profile).Error; err != nil {
		return 0, false
	}
	if profile.EntryDate == "" {
		return 0, false
	}
	return s.calcWorkingYears(profile.EntryDate, year), true
}

func (s *AnnualLeaveGrantService) calcWorkingYears(entryDateStr string, refYear int) float64 {
	if entryDateStr == "" {
		return 0
	}
	entryDate, err := time.Parse("2006-01-02", entryDateStr)
	if err != nil {
		return 0
	}

	refDate := s.nowFn()
	if refDate.IsZero() {
		refDate = time.Now()
	}

	cfg, err := s.ruleRepo.FindByKey("grant.working_years_ref_date")
	if err == nil {
		var v map[string]interface{}
		if json.Unmarshal([]byte(cfg.RuleValueJSON), &v) == nil {
			if ds, ok := v["ref_date"].(string); ok {
				if t, parseErr := time.Parse("2006-01-02", ds); parseErr == nil {
					refDate = t
				}
			}
		}
	}

	diff := refDate.Sub(entryDate)
	years := diff.Hours() / 24 / 365.0
	if years < 0 {
		return 0
	}
	return years
}

func (s *AnnualLeaveGrantService) mapWorkingYearsToDays(workingYears float64) float64 {
	cfg, err := s.ruleRepo.FindByKey("grant.working_years_to_days")
	if err == nil {
		var mapping []struct {
			MinYears float64 `json:"min_years"`
			Days     float64 `json:"days"`
		}
		if json.Unmarshal([]byte(cfg.RuleValueJSON), &mapping) == nil {
			days := 5.0
			for _, m := range mapping {
				if workingYears >= m.MinYears {
					days = m.Days
				}
			}
			return days
		}
	}

	switch {
	case workingYears < 1:
		return 5
	case workingYears < 10:
		return 10
	default:
		return 15
	}
}

// SyncAllGrantsToDingTalk 将所有未成功同步的发放记录补同步到钉钉。
// 仅同步 skipped/pending/failed 状态；已 success 的跳过。
// 警告：topapi/attendance/vacation/quota/update 是增量接口，重复调用会叠加余额。
// 本方法通过 dingtalk_sync_status 防止对已成功同步的记录重复调用。
func (s *AnnualLeaveGrantService) SyncAllGrantsToDingTalk() (*GrantOperationResult, error) {
	result := &GrantOperationResult{}
	grants, err := s.grantRepo.FindUnsyncedGrants()
	if err != nil {
		return result, err
	}
	for i := range grants {
		s.syncGrantToDingTalk(&grants[i], result)
	}
	return result, nil
}
// approvalRef 作为唯一键防止同一条审批重复扣减；传空时跳过去重检查（手动录入场景）。
func (s *AnnualLeaveGrantService) ConsumeAnnualLeave(userID string, days float64, approvalRef, remark string) error {
	if days <= 0 {
		return fmt.Errorf("消费天数必须大于0")
	}

	if approvalRef != "" {
		var exists int64
		s.db.Model(&database.AnnualLeaveConsumeLog{}).
			Where("approval_ref = ?", approvalRef).Count(&exists)
		if exists > 0 {
			return nil
		}
	}

	grants, err := s.grantRepo.FindGrantsWithRemaining(userID)
	if err != nil {
		return err
	}

	remaining := days
	for _, g := range grants {
		if remaining <= 0 {
			break
		}
		if g.RemainingDays <= 0 {
			continue
		}

		deduct := remaining
		if deduct > g.RemainingDays {
			deduct = g.RemainingDays
		}

		newUsed := g.UsedDays + deduct
		newRemaining := g.RemainingDays - deduct

		if err := s.grantRepo.UpdateConsumed(g.ID, newUsed, newRemaining); err != nil {
			return fmt.Errorf("更新发放记录 %d 失败: %w", g.ID, err)
		}

		logEntry := &database.AnnualLeaveConsumeLog{
			UserID:      userID,
			GrantID:     g.ID,
			ApprovalRef: approvalRef,
			Days:        deduct,
			Remark:      remark,
		}
		if err := s.db.Create(logEntry).Error; err != nil {
			return fmt.Errorf("写入消费记录失败: %w", err)
		}

		remaining -= deduct
	}

	if remaining > 0 {
		return fmt.Errorf("年假余额不足，还差 %.2f 天", remaining)
	}
	return nil
}

// GetConsumeLog 查询用户的年假消费记录
func (s *AnnualLeaveGrantService) GetConsumeLog(userID string) ([]database.AnnualLeaveConsumeLog, error) {
	var logs []database.AnnualLeaveConsumeLog
	err := s.db.Where("user_id = ?", userID).Order("created_at desc").Find(&logs).Error
	return logs, err
}
