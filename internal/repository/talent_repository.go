package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type TalentRepository struct {
	db *gorm.DB
}

func NewTalentRepository(db *gorm.DB) *TalentRepository {
	return &TalentRepository{db: db}
}

func (r *TalentRepository) Create(analysis *database.TalentAnalysis) error {
	return r.db.Create(analysis).Error
}

func (r *TalentRepository) FindByID(id string) (*database.TalentAnalysis, error) {
	var analysis database.TalentAnalysis
	err := r.db.First(&analysis, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &analysis, nil
}

func (r *TalentRepository) FindAll(page, pageSize int, departmentID string) ([]database.TalentAnalysis, int64, error) {
	var analyses []database.TalentAnalysis
	var total int64

	query := r.db.Model(&database.TalentAnalysis{})
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
