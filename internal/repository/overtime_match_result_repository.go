package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OvertimeMatchResultRepository struct {
	db *gorm.DB
}

func NewOvertimeMatchResultRepository(db *gorm.DB) *OvertimeMatchResultRepository {
	return &OvertimeMatchResultRepository{db: db}
}

func (r *OvertimeMatchResultRepository) FindByApprovalID(approvalID uint) (*database.OvertimeMatchResult, error) {
	var result database.OvertimeMatchResult
	err := r.db.Where("approval_id = ?", approvalID).First(&result).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *OvertimeMatchResultRepository) FindByUserDateRange(userID, startDate, endDate string) ([]database.OvertimeMatchResult, error) {
	var results []database.OvertimeMatchResult
	err := r.db.Where("user_id = ? AND DATE(approval_start_time) >= ? AND DATE(approval_start_time) <= ?", userID, startDate, endDate).
		Order("approval_start_time asc").Find(&results).Error
	return results, err
}

func (r *OvertimeMatchResultRepository) FindByDateRange(startDate, endDate string) ([]database.OvertimeMatchResult, error) {
	var results []database.OvertimeMatchResult
	err := r.db.Where("DATE(approval_start_time) >= ? AND DATE(approval_start_time) <= ?", startDate, endDate).
		Find(&results).Error
	return results, err
}

func (r *OvertimeMatchResultRepository) Upsert(result *database.OvertimeMatchResult) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "approval_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"approval_status", "attendance_start_time", "attendance_end_time", "matched_minutes", "qualified_minutes", "match_status", "match_reason", "calc_version", "updated_at"}),
	}).Create(result).Error
}

func (r *OvertimeMatchResultRepository) UpdateStatus(id uint, status, reason string) error {
	return r.db.Model(&database.OvertimeMatchResult{}).Where("id = ?", id).
		Updates(map[string]interface{}{"match_status": status, "match_reason": reason}).Error
}
