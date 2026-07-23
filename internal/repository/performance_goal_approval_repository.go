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
	orgID = strings.TrimSpace(orgID)
	return &PerformanceGoalApprovalRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceGoalApprovalRepository) scoped() *gorm.DB {
	// Fail-closed: empty org never returns unfiltered db.
	return r.db.Scopes(ScopeOrg(strings.TrimSpace(r.orgID), "org_id"))
}

func (r *PerformanceGoalApprovalRepository) Create(log *database.PerformanceGoalApprovalLog) error {
	orgID := strings.TrimSpace(r.orgID)
	if orgID == "" {
		return ErrMissingOrgID
	}
	merged, err := EnsureSameOrg(orgID, log.OrgID)
	if err != nil {
		return err
	}
	log.OrgID = merged
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
