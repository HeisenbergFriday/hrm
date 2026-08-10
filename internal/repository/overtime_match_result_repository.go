package repository

import (
	"peopleops/internal/database"
	"strings"
	"time"

	"gorm.io/gorm"
)

type OvertimeMatchResultRepository struct {
	db    *gorm.DB
	orgID string
}

func NewOvertimeMatchResultRepository(db *gorm.DB) *OvertimeMatchResultRepository {
	return &OvertimeMatchResultRepository{db: db}
}

func NewOvertimeMatchResultRepositoryWithOrgID(db *gorm.DB, orgID string) *OvertimeMatchResultRepository {
	return &OvertimeMatchResultRepository{db: db, orgID: orgID}
}

func (r *OvertimeMatchResultRepository) scoped() *gorm.DB {
	// Fail-closed: empty org must never return an unfiltered db session.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *OvertimeMatchResultRepository) FindByApprovalID(approvalID uint) (*database.OvertimeMatchResult, error) {
	var result database.OvertimeMatchResult
	err := r.scoped().Where("approval_id = ?", approvalID).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OvertimeMatchResultRepository) FindByID(id uint) (*database.OvertimeMatchResult, error) {
	var result database.OvertimeMatchResult
	err := r.scoped().First(&result, id).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OvertimeMatchResultRepository) FindByUserAndWorkDate(userID, workDate string) (*database.OvertimeMatchResult, error) {
	var result database.OvertimeMatchResult
	err := r.scoped().Where("user_id = ? AND work_date = ?", userID, workDate).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OvertimeMatchResultRepository) FindByUserDateRange(userID, startDate, endDate string) ([]database.OvertimeMatchResult, error) {
	var results []database.OvertimeMatchResult
	err := r.scoped().Where("user_id = ? AND work_date >= ? AND work_date <= ?", userID, startDate, endDate).
		Order("work_date asc").Find(&results).Error
	return results, err
}

func (r *OvertimeMatchResultRepository) FindByDateRange(startDate, endDate string) ([]database.OvertimeMatchResult, error) {
	var results []database.OvertimeMatchResult
	err := r.scoped().Where("work_date >= ? AND work_date <= ?", startDate, endDate).
		Find(&results).Error
	return results, err
}

func (r *OvertimeMatchResultRepository) Create(result *database.OvertimeMatchResult) error {
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(orgID, result.OrgID)
	if err != nil {
		return err
	}
	result.OrgID = merged
	return r.db.Create(result).Error
}

func (r *OvertimeMatchResultRepository) UpdateStatus(id uint, status, reason string) error {
	return r.scoped().Model(&database.OvertimeMatchResult{}).Where("id = ?", id).
		Updates(map[string]interface{}{"match_status": status, "match_reason": reason}).Error
}

func (r *OvertimeMatchResultRepository) UpdateSyncStatus(id uint, syncStatus, syncRequestID, syncError string) error {
	return r.scoped().Model(&database.OvertimeMatchResult{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"dingtalk_sync_status":     syncStatus,
			"dingtalk_sync_request_id": syncRequestID,
			"dingtalk_sync_error":      syncError,
		}).Error
}

func (r *OvertimeMatchResultRepository) UpdateLocalBalanceStatus(id uint, status string) error {
	return r.scoped().Model(&database.OvertimeMatchResult{}).Where("id = ?", id).
		Update("local_balance_status", status).Error
}

func (r *OvertimeMatchResultRepository) UpdateRollbackDingtalkStatus(id uint, status, safeErr string) error {
	return r.scoped().Model(&database.OvertimeMatchResult{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"rollback_dingtalk_sync_status": status,
			"rollback_dingtalk_sync_error":  safeErr,
		}).Error
}

// FindRetryableByUserDatePairs returns match results in retryable statuses for
// the given (user_id, work_date) pairs within a specific org. The caller must
// supply an org-bound repository so that cross-tenant queries are impossible.
func (r *OvertimeMatchResultRepository) FindRetryableByUserDatePairs(pairs []UserDatePair) ([]database.OvertimeMatchResult, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	// Group by user_id to build a compact WHERE clause.
	byUser := make(map[string][]string)
	for _, p := range pairs {
		uid := strings.TrimSpace(p.UserID)
		wd := strings.TrimSpace(p.WorkDate)
		if uid == "" || wd == "" {
			continue
		}
		byUser[uid] = append(byUser[uid], wd)
	}
	if len(byUser) == 0 {
		return nil, nil
	}

	retryableStatuses := []string{
		"no_clock_record",
		"insufficient_clock_record",
		"query_clock_failed",
		"unmatched",
		"invalid_clock_time",
	}

	// Build OR group: (user_id = ? AND work_date IN (?)) OR (user_id = ? AND work_date IN (?)) ...
	// GORM Or() at the same level creates: WHERE ... AND (... OR ...).
	var orExpr *gorm.DB
	for uid, dates := range byUser {
		cond := r.db.Where("user_id = ? AND work_date IN ?", uid, dates)
		if orExpr == nil {
			orExpr = cond
		} else {
			orExpr = orExpr.Or(cond)
		}
	}

	query := r.scoped().
		Where("match_status IN ?", retryableStatuses).
		Where(orExpr)

	var results []database.OvertimeMatchResult
	if err := query.Find(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

// FindRetryableInDateRange returns match results in retryable statuses within
// a date range for a specific org. Used by the scheduled compensation job.
func (r *OvertimeMatchResultRepository) FindRetryableInDateRange(startDate, endDate string, limit int) ([]database.OvertimeMatchResult, error) {
	retryableStatuses := []string{
		"no_clock_record",
		"insufficient_clock_record",
		"query_clock_failed",
		"unmatched",
		"invalid_clock_time",
	}
	query := r.scoped().
		Where("match_status IN ?", retryableStatuses).
		Where("work_date >= ? AND work_date <= ?", startDate, endDate).
		Order("updated_at asc").
		Order("id asc")
	if limit > 0 {
		query = query.Limit(limit)
	}
	var results []database.OvertimeMatchResult
	if err := query.Find(&results).Error; err != nil {
		return nil, err
	}
	return results, nil
}

// TouchRetryAttempts moves every selected retryable row behind records that
// have not been attempted recently, including malformed rows skipped by callers.
func (r *OvertimeMatchResultRepository) TouchRetryAttempts(ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return r.scoped().Model(&database.OvertimeMatchResult{}).
		Where("id IN ?", ids).
		UpdateColumn("updated_at", time.Now()).Error
}

type UserDatePair struct {
	UserID   string
	WorkDate string
}

func (r *OvertimeMatchResultRepository) UpdateCalculation(id uint, result *database.OvertimeMatchResult) error {
	return r.scoped().Model(&database.OvertimeMatchResult{}).Where("id = ?", id).Updates(map[string]interface{}{
		"approval_id":                result.ApprovalID,
		"approval_process_id":        result.ApprovalProcessID,
		"approval_status":            result.ApprovalStatus,
		"work_date":                  result.WorkDate,
		"approval_start_time":        result.ApprovalStartTime,
		"approval_end_time":          result.ApprovalEndTime,
		"approval_duration_minutes":  result.ApprovalDurationMinutes,
		"overtime_start_time":        result.OvertimeStartTime,
		"overtime_end_time":          result.OvertimeEndTime,
		"overtime_duration_minutes":  result.OvertimeDurationMinutes,
		"actual_first_clock_time":    result.ActualFirstClockTime,
		"actual_last_clock_time":     result.ActualLastClockTime,
		"actual_clock_span_minutes":  result.ActualClockSpanMinutes,
		"break_deduct_minutes":       result.BreakDeductMinutes,
		"effective_overtime_minutes": result.EffectiveOvertimeMinutes,
		"match_status":               result.MatchStatus,
		"match_reason":               result.MatchReason,
		"local_balance_status":       result.LocalBalanceStatus,
		"dingtalk_sync_status":       result.DingtalkSyncStatus,
		"dingtalk_sync_request_id":   "",
		"dingtalk_sync_error":        "",
		"calc_version":               result.CalcVersion,
	}).Error
}
