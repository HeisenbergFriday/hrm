package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type AuditRepository struct {
	db *gorm.DB
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) Create(log *database.OperationLog) error {
	return r.db.Create(log).Error
}

func (r *AuditRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.OperationLog, int64, error) {
	var logs []database.OperationLog
	var total int64

	query := r.db.Model(&database.OperationLog{})

	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("user_id = ?", v)
	}
	if v, ok := filters["start_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, err := time.Parse("2006-01-02", v)
		if err == nil {
			query = query.Where("created_at < ?", t.AddDate(0, 0, 1))
		}
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		return nil, 0, err
	}

	return logs, total, nil
}
