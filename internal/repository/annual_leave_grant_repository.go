package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type AnnualLeaveGrantRepository struct {
	db *gorm.DB
}

func NewAnnualLeaveGrantRepository(db *gorm.DB) *AnnualLeaveGrantRepository {
	return &AnnualLeaveGrantRepository{db: db}
}

func (r *AnnualLeaveGrantRepository) FindByUserYear(userID string, year int) ([]database.AnnualLeaveGrant, error) {
	var results []database.AnnualLeaveGrant
	err := r.db.Where("user_id = ? AND year = ?", userID, year).Order("quarter asc, created_at asc").Find(&results).Error
	return results, err
}

func (r *AnnualLeaveGrantRepository) FindByUserYearQuarterType(userID string, year, quarter int, grantType string) (*database.AnnualLeaveGrant, error) {
	var result database.AnnualLeaveGrant
	err := r.db.Where("user_id = ? AND year = ? AND quarter = ? AND grant_type = ?", userID, year, quarter, grantType).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *AnnualLeaveGrantRepository) FindGrantsWithRemaining(userID string) ([]database.AnnualLeaveGrant, error) {
	var results []database.AnnualLeaveGrant
	err := r.db.Where("user_id = ? AND remaining_days > 0", userID).
		Order("year asc, quarter asc").Find(&results).Error
	return results, err
}

func (r *AnnualLeaveGrantRepository) FindFailedSyncGrants() ([]database.AnnualLeaveGrant, error) {
	var results []database.AnnualLeaveGrant
	err := r.db.Where("dingtalk_sync_status = 'failed'").Find(&results).Error
	return results, err
}

// FindUnsyncedGrants 查找所有未成功同步的发放记录（skipped/pending/failed）
func (r *AnnualLeaveGrantRepository) FindUnsyncedGrants() ([]database.AnnualLeaveGrant, error) {
	var results []database.AnnualLeaveGrant
	err := r.db.Where("dingtalk_sync_status != 'success'").
		Order("year asc, quarter asc, created_at asc").Find(&results).Error
	return results, err
}

func (r *AnnualLeaveGrantRepository) Create(grant *database.AnnualLeaveGrant) error {
	return r.db.Create(grant).Error
}

func (r *AnnualLeaveGrantRepository) Save(grant *database.AnnualLeaveGrant) error {
	return r.db.Save(grant).Error
}

func (r *AnnualLeaveGrantRepository) UpdateSyncStatus(id uint, status, errMsg string, syncedAt *time.Time) error {
	updates := map[string]interface{}{
		"dingtalk_sync_status": status,
		"dingtalk_sync_error":  errMsg,
	}
	if syncedAt != nil {
		updates["dingtalk_synced_at"] = syncedAt
	}
	return r.db.Model(&database.AnnualLeaveGrant{}).Where("id = ?", id).Updates(updates).Error
}

func (r *AnnualLeaveGrantRepository) UpdateConsumed(id uint, usedDays, remainingDays float64) error {
	return r.db.Model(&database.AnnualLeaveGrant{}).Where("id = ?", id).Updates(map[string]interface{}{
		"used_days":      usedDays,
		"remaining_days": remainingDays,
	}).Error
}

func (r *AnnualLeaveGrantRepository) SumGrantedDaysByUserYear(userID string, year int) (float64, error) {
	var total float64
	err := r.db.Model(&database.AnnualLeaveGrant{}).
		Where("user_id = ? AND year = ?", userID, year).
		Select("COALESCE(SUM(granted_days + retroactive_days), 0)").
		Scan(&total).Error
	return total, err
}
