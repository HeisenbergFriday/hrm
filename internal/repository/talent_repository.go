package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type TalentRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewTalentRepository(db *gorm.DB) *TalentRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &TalentRepository{db: db, orgID: orgID, orgErr: err}
}

// NewTalentRepositoryWithOrgID constructs a tenant-bound talent repository.
func NewTalentRepositoryWithOrgID(db *gorm.DB, orgID string) *TalentRepository {
	normalized, err := RequireOrgID(orgID)
	return &TalentRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *TalentRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *TalentRepository) Create(analysis *database.TalentAnalysis) error {
	if analysis == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, analysis.OrgID)
	if err != nil {
		return err
	}
	analysis.OrgID = merged
	return r.db.Create(analysis).Error
}

func (r *TalentRepository) FindByID(id string) (*database.TalentAnalysis, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var analysis database.TalentAnalysis
	err = r.db.Model(&database.TalentAnalysis{}).
		Where("talent_analyses.org_id = ? AND talent_analyses.id = ?", orgID, id).
		First(&analysis).Error
	if err != nil {
		return nil, err
	}
	return &analysis, nil
}

func (r *TalentRepository) FindAll(page, pageSize int, departmentID string) ([]database.TalentAnalysis, int64, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, 0, err
	}
	var analyses []database.TalentAnalysis
	var total int64

	query := r.db.Model(&database.TalentAnalysis{}).Where("talent_analyses.org_id = ?", orgID)
	if departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&analyses).Error; err != nil {
		return nil, 0, err
	}
	return analyses, total, nil
}
