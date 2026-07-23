package repository

import (
	"peopleops/internal/database"
	"time"

	"gorm.io/gorm"
)

type AuditRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewAuditRepository(db *gorm.DB) *AuditRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &AuditRepository{db: db, orgID: orgID, orgErr: err}
}

func NewAuditRepositoryWithOrgID(db *gorm.DB, orgID string) *AuditRepository {
	normalized, err := RequireOrgID(orgID)
	return &AuditRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *AuditRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *AuditRepository) Create(log *database.OperationLog) error {
	if log == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, log.OrgID)
	if err != nil {
		return err
	}
	log.OrgID = merged
	return r.db.Create(log).Error
}

func (r *AuditRepository) FindAll(page, pageSize int, filters map[string]string) ([]database.OperationLog, int64, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, 0, err
	}
	var logs []database.OperationLog
	var total int64

	query := r.db.Model(&database.OperationLog{}).Where("org_id = ?", orgID)

	if v, ok := filters["user_id"]; ok && v != "" {
		query = query.Where("user_id = ?", v)
	}
	if v, ok := filters["operation"]; ok && v != "" {
		query = query.Where("operation = ?", v)
	}
	if v, ok := filters["resource"]; ok && v != "" {
		query = query.Where("resource = ?", v)
	}
	if v, ok := filters["start_date"]; ok && v != "" {
		t, parseErr := time.Parse("2006-01-02", v)
		if parseErr == nil {
			query = query.Where("created_at >= ?", t)
		}
	}
	if v, ok := filters["end_date"]; ok && v != "" {
		t, parseErr := time.Parse("2006-01-02", v)
		if parseErr == nil {
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
