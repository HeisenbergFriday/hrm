package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type PerformanceGoalApprovalRepository struct {
	db *gorm.DB
}

func NewPerformanceGoalApprovalRepository(db *gorm.DB) *PerformanceGoalApprovalRepository {
	return &PerformanceGoalApprovalRepository{db: db}
}

func (r *PerformanceGoalApprovalRepository) Create(log *database.PerformanceGoalApprovalLog) error {
	return r.db.Create(log).Error
}

func (r *PerformanceGoalApprovalRepository) FindByParticipant(participantID uint, activityID string) ([]database.PerformanceGoalApprovalLog, error) {
	var logs []database.PerformanceGoalApprovalLog
	if err := r.db.Where("participant_id = ? AND activity_id = ?", participantID, activityID).
		Order("created_at DESC").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *PerformanceGoalApprovalRepository) FindByGoalRecord(goalRecordID uint) ([]database.PerformanceGoalApprovalLog, error) {
	var logs []database.PerformanceGoalApprovalLog
	if err := r.db.Where("goal_record_id = ?", goalRecordID).
		Order("created_at DESC").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *PerformanceGoalApprovalRepository) GetLatestByParticipant(participantID uint, activityID string) (*database.PerformanceGoalApprovalLog, error) {
	var log database.PerformanceGoalApprovalLog
	if err := r.db.Where("participant_id = ? AND activity_id = ?", participantID, activityID).
		Order("created_at DESC").
		First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}
