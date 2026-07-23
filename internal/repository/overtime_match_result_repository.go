package repository

import (
	"peopleops/internal/database"
	"strings"

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
