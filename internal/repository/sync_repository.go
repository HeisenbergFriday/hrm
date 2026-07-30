package repository

import (
	"strings"

	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SyncRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewSyncRepository(db *gorm.DB) *SyncRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &SyncRepository{db: db, orgID: orgID, orgErr: err}
}

func NewSyncRepositoryWithOrgID(db *gorm.DB, orgID string) *SyncRepository {
	normalized, err := RequireOrgID(orgID)
	return &SyncRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *SyncRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *SyncRepository) DB() *gorm.DB {
	return r.db
}

// Upsert 更新或创建同步状态
func (r *SyncRepository) Upsert(status *database.SyncStatus) error {
	if status == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, status.OrgID)
	if err != nil {
		return err
	}
	status.OrgID = merged
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "type"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_sync_time", "status", "message", "request_id", "duration_ms", "error_code", "success_count", "fail_count", "details", "updated_at"}),
	}).Create(status).Error
}

func (r *SyncRepository) FindByOrgAndType(orgID, syncType string) (*database.SyncStatus, error) {
	bound, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	// Prefer bound org; if caller passes an explicit org it must match.
	if orgID = normalizeOrgID(orgID); orgID != "" && orgID != bound {
		return nil, ErrOrgMismatch
	}
	var status database.SyncStatus
	err = r.db.Where("org_id = ? AND type = ?", bound, syncType).First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (r *SyncRepository) FindByOrgTypeAndRequestID(orgID, syncType, requestID string) (*database.SyncStatus, error) {
	bound, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	if orgID = normalizeOrgID(orgID); orgID != "" && orgID != bound {
		return nil, ErrOrgMismatch
	}
	requestID = strings.TrimSpace(requestID)
	if requestID == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var status database.SyncStatus
	err = r.db.Where("org_id = ? AND type = ? AND request_id = ?", bound, syncType, requestID).First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (r *SyncRepository) FindAllByOrg(orgID string) ([]database.SyncStatus, error) {
	bound, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	if orgID = normalizeOrgID(orgID); orgID != "" && orgID != bound {
		return nil, ErrOrgMismatch
	}
	var statuses []database.SyncStatus
	err = r.db.Where("org_id = ?", bound).Find(&statuses).Error
	return statuses, err
}
