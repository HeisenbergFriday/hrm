package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type ApprovalRepository struct {
	db *gorm.DB
}

func NewApprovalRepository(db *gorm.DB) *ApprovalRepository {
	return &ApprovalRepository{db: db}
}

func (r *ApprovalRepository) Create(approval *database.Approval) error {
	return r.db.Create(approval).Error
}

func (r *ApprovalRepository) FindByID(id string) (*database.Approval, error) {
	var approval database.Approval
	err := r.db.First(&approval, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &approval, nil
}

func (r *ApprovalRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.Approval, int64, error) {
	var approvals []database.Approval
	var total int64

	query := r.db.Model(&database.Approval{})

	if v, ok := filters["status"]; ok && v != "" {
		query = query.Where("status = ?", v)
	}
	if v, ok := filters["template_id"]; ok && v != "" {
		query = query.Where("extension->>'$.template_id' = ? OR extension LIKE ?", v, "%"+v+"%")
	}
	if v, ok := filters["applicant_id"]; ok && v != "" {
		query = query.Where("applicant_id = ?", v)
	}
	if v, ok := filters["start_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			query = query.Where("create_time >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			query = query.Where("create_time < ?", t.AddDate(0, 0, 1))
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("create_time DESC").Offset(offset).Limit(pageSize).Find(&approvals).Error; err != nil {
		return nil, 0, err
	}

	return approvals, total, nil
}

// ApprovalTemplate Repository

type ApprovalTemplateRepository struct {
	db *gorm.DB
}

func NewApprovalTemplateRepository(db *gorm.DB) *ApprovalTemplateRepository {
	return &ApprovalTemplateRepository{db: db}
}

func (r *ApprovalTemplateRepository) Create(template *database.ApprovalTemplate) error {
	return r.db.Create(template).Error
}

func (r *ApprovalTemplateRepository) FindAll() ([]database.ApprovalTemplate, int64, error) {
	var templates []database.ApprovalTemplate
	var total int64

	if err := r.db.Model(&database.ApprovalTemplate{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.Find(&templates).Error; err != nil {
		return nil, 0, err
	}

	return templates, total, nil
}
