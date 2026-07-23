package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ShiftConfigRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewShiftConfigRepository(db *gorm.DB) *ShiftConfigRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &ShiftConfigRepository{db: db, orgID: orgID, orgErr: err}
}

func NewShiftConfigRepositoryWithOrgID(db *gorm.DB, orgID string) *ShiftConfigRepository {
	normalized, err := RequireOrgID(orgID)
	return &ShiftConfigRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *ShiftConfigRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *ShiftConfigRepository) FindAll() ([]database.EmployeeShiftConfig, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var configs []database.EmployeeShiftConfig
	err = r.db.Where("org_id = ?", orgID).Find(&configs).Error
	return configs, err
}

func (r *ShiftConfigRepository) FindByUserID(userID string) (*database.EmployeeShiftConfig, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var config database.EmployeeShiftConfig
	if err := r.db.Where("org_id = ? AND user_id = ?", orgID, userID).First(&config).Error; err != nil {
		return nil, err
	}
	return &config, nil
}

// Upsert 创建或更新（按 org_id + user_id 唯一键）
func (r *ShiftConfigRepository) Upsert(config *database.EmployeeShiftConfig) error {
	if config == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, config.OrgID)
	if err != nil {
		return err
	}
	config.OrgID = merged
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"user_name", "shift_id", "end_time", "note", "updated_at"}),
	}).Create(config).Error
}

func (r *ShiftConfigRepository) DeleteByUserID(userID string) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	return r.db.Where("org_id = ? AND user_id = ?", orgID, userID).Delete(&database.EmployeeShiftConfig{}).Error
}
