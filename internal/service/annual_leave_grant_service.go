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
	"gorm.io/gorm/clause"
)

type AnnualLeaveGrantService struct {
	db              *gorm.DB
	orgID           string
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

func NewAnnualLeaveGrantServiceWithOrgID(db *gorm.DB, orgID string) *AnnualLeaveGrantService {
	return &AnnualLeaveGrantService{
		db:              db,
		orgID:           orgID,
		grantRepo:       repository.NewAnnualLeaveGrantRepositoryWithOrgID(db, orgID),
		eligibilityRepo: repository.NewAnnualLeaveEligibilityRepositoryWithOrgID(db, orgID),
		ruleRepo:        repository.NewLeaveRuleConfigRepositoryWithOrgID(db, orgID),
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

type AnnualLeaveMutationResult struct {
	Changed     bool
	Status      string
	OperationNo int
}

type annualLeaveBusinessSegment struct {
	StartDate string
	EndDate   string
	Year      int
	Quarter   int
	Days      float64
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
	created, err := s.grantRepo.CreateIfAbsent(grant)
	if err != nil {
		return result, err
	}
	if !created {
		return s.handleExistingGrantResult(result, userID, year, quarter, "normal")
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
	if err := NewAnnualLeaveServiceWithOrgID(s.db, s.orgID).RecalculateEligibility(userID, year); err != nil {
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
		created, err := s.grantRepo.CreateIfAbsent(grant)
		if err != nil {
			return result, err
		}
		if !created {
			userResult, err := s.handleExistingGrantResult(&GrantOperationResult{}, userID, year, e.Quarter, "retroactive")
			mergeGrantOperationResult(result, userResult)
			if err != nil {
				return result, err
			}
			continue
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
	orgID, orgErr := resolveServiceOrgID(s.orgID, s.db)
	if orgErr != nil {
		log.Printf("[leave-sync] missing org context grantID=%d userID=%s err=%v", grant.ID, grant.UserID, orgErr)
		result.DingTalkFailedCount++
		result.Errors = append(result.Errors, fmt.Sprintf("%s Q%d: %s", grant.UserID, grant.Quarter, orgErr.Error()))
		setStatus("failed", orgErr.Error(), nil)
		return
	}
	if err := dingtalk.UpdateAnnualLeaveQuotaForOrg(orgID, grant.UserID, grant.Year, days, reason); err != nil {
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
	query := s.db.Select("entry_date").Where("user_id = ?", userID)
	if orgID := strings.TrimSpace(s.orgID); orgID != "" {
		query = query.Where("org_id = ?", orgID)
	}
	if err := query.First(&profile).Error; err != nil {
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

// 审批消费通过请求门闩防重；手动录入使用每次调用生成的 request_ref。
// 所有余额、门闩和台账读写必须绑定租户 orgID。
func (s *AnnualLeaveGrantService) ConsumeAnnualLeave(userID string, days float64, approvalRef, remark string) error {
	_, err := s.consumeAnnualLeave(userID, days, approvalRef, remark, time.Time{}, time.Time{})
	return err
}

func (s *AnnualLeaveGrantService) ConsumeAnnualLeaveForPeriod(userID string, days float64, approvalRef, remark string, start, end time.Time) (AnnualLeaveMutationResult, error) {
	if start.IsZero() || end.IsZero() {
		return AnnualLeaveMutationResult{}, fmt.Errorf("annual leave business date required")
	}
	return s.consumeAnnualLeave(userID, days, approvalRef, remark, start, end)
}

func (s *AnnualLeaveGrantService) consumeAnnualLeave(userID string, days float64, approvalRef, remark string, start, end time.Time) (AnnualLeaveMutationResult, error) {
	if days <= 0 {
		return AnnualLeaveMutationResult{}, fmt.Errorf("消费天数必须大于0")
	}
	orgID := strings.TrimSpace(s.orgID)
	if orgID == "" {
		return AnnualLeaveMutationResult{}, fmt.Errorf("org_id required for annual leave consumption")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return AnnualLeaveMutationResult{}, fmt.Errorf("user_id required for annual leave consumption")
	}

	approvalRef = strings.TrimSpace(approvalRef)
	requestRef := buildConsumeRequestRef(userID, approvalRef)
	segments, err := splitAnnualLeaveBusinessSegments(days, start, end)
	if err != nil {
		return AnnualLeaveMutationResult{}, err
	}
	result := AnnualLeaveMutationResult{}
	err = withAnnualLeaveTransactionRetry(s.db, func(tx *gorm.DB) error {
		gate := database.AnnualLeaveConsumeRequest{
			OrgID:             orgID,
			RequestRef:        requestRef,
			ApprovalRef:       approvalRef,
			UserID:            userID,
			Status:            "applying",
			OperationNo:       1,
			Days:              days,
			BusinessStartDate: formatAnnualLeaveBusinessDate(start),
			BusinessEndDate:   formatAnnualLeaveBusinessDate(end),
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&gate).Error; err != nil {
			return err
		}
		operationNo := 1
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("org_id = ? AND request_ref = ?", orgID, requestRef).
			First(&gate).Error; err != nil {
			return err
		}
		switch gate.Status {
		case "applying":
			// This state is only visible to the transaction that inserted it.
		case "applied":
			if gate.UserID != userID || gate.ApprovalRef != approvalRef {
				return fmt.Errorf("annual leave request identity mismatch")
			}
			startDate := formatAnnualLeaveBusinessDate(start)
			endDate := formatAnnualLeaveBusinessDate(end)
			if gate.BusinessStartDate != "" && (gate.BusinessStartDate != startDate || gate.BusinessEndDate != endDate || absFloat(gate.Days-days) > 1e-9) {
				return fmt.Errorf("annual leave request payload changed after application")
			}
			result = AnnualLeaveMutationResult{Changed: false, Status: gate.Status, OperationNo: gate.OperationNo}
			return nil
		case "reversed":
			if gate.UserID != userID || gate.ApprovalRef != approvalRef {
				return fmt.Errorf("annual leave request identity mismatch")
			}
			operationNo = gate.OperationNo + 1
		default:
			return fmt.Errorf("unsupported annual leave request status %q", gate.Status)
		}

		for _, segment := range segments {
			if err := consumeAnnualLeaveSegment(tx, orgID, userID, approvalRef, requestRef, remark, operationNo, segment); err != nil {
				return err
			}
		}
		if err := tx.Model(&database.AnnualLeaveConsumeRequest{}).
			Where("org_id = ? AND request_ref = ?", orgID, requestRef).
			Updates(map[string]interface{}{
				"status":              "applied",
				"operation_no":        operationNo,
				"days":                days,
				"business_start_date": formatAnnualLeaveBusinessDate(start),
				"business_end_date":   formatAnnualLeaveBusinessDate(end),
			}).Error; err != nil {
			return err
		}
		result = AnnualLeaveMutationResult{Changed: true, Status: "applied", OperationNo: operationNo}
		return nil
	})
	return result, err
}

func (s *AnnualLeaveGrantService) RollbackAnnualLeave(approvalRef, remark string) (AnnualLeaveMutationResult, error) {
	orgID := strings.TrimSpace(s.orgID)
	approvalRef = strings.TrimSpace(approvalRef)
	if orgID == "" {
		return AnnualLeaveMutationResult{}, fmt.Errorf("org_id required for annual leave rollback")
	}
	if approvalRef == "" {
		return AnnualLeaveMutationResult{}, fmt.Errorf("approval_ref required for annual leave rollback")
	}
	requestRef := buildConsumeRequestRef("", approvalRef)
	result := AnnualLeaveMutationResult{Status: "reversed"}
	err := withAnnualLeaveTransactionRetry(s.db, func(tx *gorm.DB) error {
		var gate database.AnnualLeaveConsumeRequest
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("org_id = ? AND request_ref = ?", orgID, requestRef).
			First(&gate).Error; err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		result.OperationNo = gate.OperationNo
		if gate.Status == "reversed" {
			return nil
		}
		if gate.Status != "applied" {
			return fmt.Errorf("unsupported annual leave request status %q", gate.Status)
		}
		var logs []database.AnnualLeaveConsumeLog
		if err := tx.Where("org_id = ? AND approval_ref = ? AND operation_no = ? AND entry_type = ?", orgID, gate.ApprovalRef, gate.OperationNo, "consume").
			Order("id asc").Find(&logs).Error; err != nil {
			return err
		}
		if len(logs) == 0 {
			return fmt.Errorf("annual leave consume logs missing for rollback")
		}
		for _, entry := range logs {
			var grant database.AnnualLeaveGrant
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
				Where("id = ? AND org_id = ?", entry.GrantID, orgID).First(&grant).Error; err != nil {
				return fmt.Errorf("load annual leave grant %d for rollback: %w", entry.GrantID, err)
			}
			if grant.UsedDays+1e-9 < entry.Days {
				return fmt.Errorf("annual leave grant %d rollback exceeds used days", grant.ID)
			}
			if err := tx.Model(&database.AnnualLeaveGrant{}).Where("id = ? AND org_id = ?", grant.ID, orgID).
				Updates(map[string]interface{}{
					"used_days":      grant.UsedDays - entry.Days,
					"remaining_days": grant.RemainingDays + entry.Days,
				}).Error; err != nil {
				return err
			}
			reversal := database.AnnualLeaveConsumeLog{
				OrgID:             orgID,
				UserID:            entry.UserID,
				GrantID:           entry.GrantID,
				ApprovalRef:       gate.ApprovalRef,
				RequestRef:        fmt.Sprintf("%s:reverse:%d", requestRef, gate.OperationNo),
				OperationNo:       gate.OperationNo,
				EntryType:         "reversal",
				BusinessStartDate: entry.BusinessStartDate,
				BusinessEndDate:   entry.BusinessEndDate,
				ReversalOfID:      entry.ID,
				Days:              entry.Days,
				Remark:            remark,
			}
			if err := tx.Create(&reversal).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&database.AnnualLeaveConsumeRequest{}).
			Where("id = ? AND org_id = ?", gate.ID, orgID).Update("status", "reversed").Error; err != nil {
			return err
		}
		result.Changed = true
		return nil
	})
	return result, err
}

func consumeAnnualLeaveSegment(tx *gorm.DB, orgID, userID, approvalRef, requestRef, remark string, operationNo int, segment annualLeaveBusinessSegment) error {
	var grants []database.AnnualLeaveGrant
	query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("org_id = ? AND user_id = ? AND remaining_days > 0", orgID, userID)
	if segment.Year > 0 {
		query = query.Where("year = ? AND quarter <= ?", segment.Year, segment.Quarter)
	}
	if err := query.Order("year asc, quarter asc, id asc").Find(&grants).Error; err != nil {
		return err
	}
	totalRemaining := 0.0
	for _, grant := range grants {
		totalRemaining += grant.RemainingDays
	}
	if totalRemaining+1e-9 < segment.Days {
		return fmt.Errorf("%d年年假余额不足，还差 %.2f 天", segment.Year, segment.Days-totalRemaining)
	}
	remaining := segment.Days
	for _, grant := range grants {
		if remaining <= 1e-9 {
			break
		}
		deduct := remaining
		if deduct > grant.RemainingDays {
			deduct = grant.RemainingDays
		}
		if err := tx.Model(&database.AnnualLeaveGrant{}).Where("id = ? AND org_id = ?", grant.ID, orgID).
			Updates(map[string]interface{}{
				"used_days":      grant.UsedDays + deduct,
				"remaining_days": grant.RemainingDays - deduct,
			}).Error; err != nil {
			return fmt.Errorf("更新发放记录 %d 失败: %w", grant.ID, err)
		}
		childRequestRef := requestRef
		if operationNo > 1 {
			childRequestRef = fmt.Sprintf("%s:op:%d", requestRef, operationNo)
		}
		entry := database.AnnualLeaveConsumeLog{
			OrgID:             orgID,
			UserID:            userID,
			GrantID:           grant.ID,
			ApprovalRef:       approvalRef,
			RequestRef:        childRequestRef,
			OperationNo:       operationNo,
			EntryType:         "consume",
			BusinessStartDate: segment.StartDate,
			BusinessEndDate:   segment.EndDate,
			Days:              deduct,
			Remark:            remark,
		}
		if err := tx.Create(&entry).Error; err != nil {
			return fmt.Errorf("写入消费记录失败: %w", err)
		}
		remaining -= deduct
	}
	return nil
}

func splitAnnualLeaveBusinessSegments(days float64, start, end time.Time) ([]annualLeaveBusinessSegment, error) {
	if start.IsZero() && end.IsZero() {
		return []annualLeaveBusinessSegment{{Quarter: 4, Days: days}}, nil
	}
	location := dingtalk.ApprovalBusinessLocation()
	start = start.In(location)
	end = end.In(location)
	startDay := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, location)
	endDay := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, location)
	if endDay.Before(startDay) {
		return nil, fmt.Errorf("annual leave business end date precedes start date")
	}
	totalCalendarDays := int(endDay.Sub(startDay).Hours()/24) + 1
	if totalCalendarDays <= 0 {
		return nil, fmt.Errorf("annual leave business date range invalid")
	}
	segments := make([]annualLeaveBusinessSegment, 0, endDay.Year()-startDay.Year()+1)
	allocated := 0.0
	for year := startDay.Year(); year <= endDay.Year(); year++ {
		segmentStart := startDay
		if year != startDay.Year() {
			segmentStart = time.Date(year, 1, 1, 0, 0, 0, 0, location)
		}
		segmentEnd := endDay
		if year != endDay.Year() {
			segmentEnd = time.Date(year, 12, 31, 0, 0, 0, 0, location)
		}
		calendarDays := int(segmentEnd.Sub(segmentStart).Hours()/24) + 1
		segmentDays := days * float64(calendarDays) / float64(totalCalendarDays)
		if year == endDay.Year() {
			segmentDays = days - allocated
		}
		allocated += segmentDays
		segments = append(segments, annualLeaveBusinessSegment{
			StartDate: segmentStart.Format("2006-01-02"),
			EndDate:   segmentEnd.Format("2006-01-02"),
			Year:      year,
			Quarter:   (int(segmentEnd.Month())-1)/3 + 1,
			Days:      segmentDays,
		})
	}
	return segments, nil
}

func formatAnnualLeaveBusinessDate(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.In(dingtalk.ApprovalBusinessLocation()).Format("2006-01-02")
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func withAnnualLeaveTransactionRetry(db *gorm.DB, fn func(*gorm.DB) error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = db.Transaction(fn)
		if err == nil || !isRetryableDatabaseTransactionError(err) {
			return err
		}
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return err
}

func isRetryableDatabaseTransactionError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "deadlock found") ||
		strings.Contains(message, "lock wait timeout")
}

// GetConsumeLog 查询用户的年假消费记录
func (s *AnnualLeaveGrantService) GetConsumeLog(userID string) ([]database.AnnualLeaveConsumeLog, error) {
	orgID := strings.TrimSpace(s.orgID)
	if orgID == "" {
		return nil, fmt.Errorf("org_id required for annual leave consume log")
	}
	var logs []database.AnnualLeaveConsumeLog
	err := s.db.Where("org_id = ? AND user_id = ?", orgID, userID).
		Order("created_at desc").Find(&logs).Error
	return logs, err
}

func (s *AnnualLeaveGrantService) handleExistingGrantResult(result *GrantOperationResult, userID string, year, quarter int, grantType string) (*GrantOperationResult, error) {
	if result == nil {
		result = &GrantOperationResult{}
	}
	existing, err := s.grantRepo.FindByUserYearQuarterType(userID, year, quarter, grantType)
	if err != nil || existing == nil {
		return result, err
	}
	result.SkippedCount++
	if existing.DingTalkSyncStatus != "success" {
		s.syncGrantToDingTalk(existing, result)
		if existing.DingTalkSyncStatus == "failed" {
			return result, fmt.Errorf("%s", existing.DingTalkSyncError)
		}
	}
	return result, nil
}

func buildConsumeRequestRef(userID, approvalRef string) string {
	if approvalRef != "" {
		if strings.HasPrefix(approvalRef, "approval:") {
			return approvalRef
		}
		return "approval:" + approvalRef
	}
	return fmt.Sprintf("manual:%s:%d", userID, time.Now().UnixNano())
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	lowerErr := strings.ToLower(err.Error())
	return strings.Contains(lowerErr, "duplicate entry") ||
		strings.Contains(lowerErr, "duplicated key") ||
		strings.Contains(lowerErr, "unique constraint failed")
}
