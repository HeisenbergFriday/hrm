package repository

import (
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
)

type PerformanceGoalApprovalRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceGoalApprovalRepository(db *gorm.DB) *PerformanceGoalApprovalRepository {
	return &PerformanceGoalApprovalRepository{db: db}
}

func NewPerformanceGoalApprovalRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceGoalApprovalRepository {
	return &PerformanceGoalApprovalRepository{db: db, orgID: strings.TrimSpace(orgID)}
}

func (r *PerformanceGoalApprovalRepository) scoped() *gorm.DB {
	tx := r.db
	if strings.TrimSpace(r.orgID) != "" {
		tx = tx.Where("org_id = ?", strings.TrimSpace(r.orgID))
	}
	return tx
}

func (r *PerformanceGoalApprovalRepository) Create(log *database.PerformanceGoalApprovalLog) error {
	if strings.TrimSpace(r.orgID) != "" && strings.TrimSpace(log.OrgID) == "" {
		log.OrgID = strings.TrimSpace(r.orgID)
	}
	if strings.TrimSpace(r.orgID) != "" && strings.TrimSpace(log.OrgID) != strings.TrimSpace(r.orgID) {
		return ErrOrgMismatch
	}
	return r.scoped().Create(log).Error
}

func (r *PerformanceGoalApprovalRepository) FindByParticipant(participantID uint, activityID string) ([]database.PerformanceGoalApprovalLog, error) {
	var logs []database.PerformanceGoalApprovalLog
	if err := r.scoped().Where("participant_id = ? AND activity_id = ?", participantID, activityID).
		Order("created_at DESC").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *PerformanceGoalApprovalRepository) FindByGoalRecord(goalRecordID uint) ([]database.PerformanceGoalApprovalLog, error) {
	var logs []database.PerformanceGoalApprovalLog
	if err := r.scoped().Where("goal_record_id = ?", goalRecordID).
		Order("created_at DESC").
		Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *PerformanceGoalApprovalRepository) GetLatestByParticipant(participantID uint, activityID string) (*database.PerformanceGoalApprovalLog, error) {
	var log database.PerformanceGoalApprovalLog
	if err := r.scoped().Where("participant_id = ? AND activity_id = ?", participantID, activityID).
		Order("created_at DESC").
		First(&log).Error; err != nil {
		return nil, err
	}
	return &log, nil
}
