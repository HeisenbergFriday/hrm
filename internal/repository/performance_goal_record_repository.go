package repository

import (
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PerformanceGoalRecordRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceGoalRecordRepository(db *gorm.DB) *PerformanceGoalRecordRepository {
	return &PerformanceGoalRecordRepository{db: db}
}

func NewPerformanceGoalRecordRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceGoalRecordRepository {
	return &PerformanceGoalRecordRepository{db: db, orgID: strings.TrimSpace(orgID)}
}

func (r *PerformanceGoalRecordRepository) scoped() *gorm.DB {
	tx := r.db
	if strings.TrimSpace(r.orgID) != "" {
		tx = tx.Where("org_id = ?", strings.TrimSpace(r.orgID))
	}
	return tx
}

func (r *PerformanceGoalRecordRepository) GetByID(id uint) (*database.PerformanceGoalRecord, error) {
	var record database.PerformanceGoalRecord
	if err := r.scoped().Where("id = ? AND deleted_at IS NULL", id).First(&record).Error; err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *PerformanceGoalRecordRepository) FindByParticipant(participantID uint) ([]database.PerformanceGoalRecord, error) {
	var records []database.PerformanceGoalRecord
	if err := r.scoped().Where("participant_id = ? AND deleted_at IS NULL", participantID).
		Order("section_type ASC, sort_order ASC, created_at ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PerformanceGoalRecordRepository) FindByActivity(activityID string) ([]database.PerformanceGoalRecord, error) {
	var records []database.PerformanceGoalRecord
	if err := r.scoped().Where("activity_id = ? AND deleted_at IS NULL", activityID).
		Order("participant_id ASC, section_type ASC, sort_order ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PerformanceGoalRecordRepository) FindByActivityAndParticipant(activityID string, participantID uint) ([]database.PerformanceGoalRecord, error) {
	var records []database.PerformanceGoalRecord
	if err := r.scoped().Where("activity_id = ? AND participant_id = ? AND deleted_at IS NULL", activityID, participantID).
		Order("section_type ASC, sort_order ASC").
		Find(&records).Error; err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PerformanceGoalRecordRepository) BatchUpsert(records []database.PerformanceGoalRecord) error {
	if len(records) == 0 {
		return nil
	}
	for i := range records {
		if strings.TrimSpace(r.orgID) != "" {
			records[i].OrgID = strings.TrimSpace(r.orgID)
		}
	}
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"section_type", "goal_phase", "goal_type", "fixed_key", "is_fixed", "item_name", "item_definition", "weight", "red_line_value", "target_value", "challenge_value", "metric_unit", "completion_rate", "scoring_rule", "actual_result", "attachments", "self_score", "manager_score", "bonus_score", "is_from_superior", "approval_status", "visibility_scope", "sort_order", "updated_at"}),
	}).Create(&records).Error
}

func (r *PerformanceGoalRecordRepository) UpdateSingle(record *database.PerformanceGoalRecord) error {
	if strings.TrimSpace(r.orgID) != "" && strings.TrimSpace(record.OrgID) == "" {
		record.OrgID = strings.TrimSpace(r.orgID)
	}
	if strings.TrimSpace(r.orgID) != "" && strings.TrimSpace(record.OrgID) != strings.TrimSpace(r.orgID) {
		return ErrOrgMismatch
	}
	return r.scoped().Save(record).Error
}

func (r *PerformanceGoalRecordRepository) DeleteByParticipantAndActivity(participantID uint, activityID string) error {
	return r.scoped().Model(&database.PerformanceGoalRecord{}).
		Where("participant_id = ? AND activity_id = ?", participantID, activityID).
		Updates(map[string]interface{}{
			"deleted_at": gorm.Expr("NOW()"),
		}).Error
}

func (r *PerformanceGoalRecordRepository) SoftDelete(id uint) error {
	return r.scoped().Model(&database.PerformanceGoalRecord{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"deleted_at": gorm.Expr("NOW()"),
		}).Error
}
