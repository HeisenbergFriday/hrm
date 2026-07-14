package repository

import (
	"strings"

	"peopleops/internal/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SyncRepository struct {
	db *gorm.DB
}

func NewSyncRepository(db *gorm.DB) *SyncRepository {
	return &SyncRepository{db: db}
}

func requireSyncOrgID(orgID string) string {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return "default"
	}
	return orgID
}

func (r *SyncRepository) DB() *gorm.DB {
	return r.db
}

// Upsert 更新或创建同步状态
func (r *SyncRepository) Upsert(status *database.SyncStatus) error {
	status.OrgID = requireSyncOrgID(status.OrgID)
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "org_id"}, {Name: "type"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_sync_time", "status", "message", "updated_at"}),
	}).Create(status).Error
}

func (r *SyncRepository) FindByOrgAndType(orgID, syncType string) (*database.SyncStatus, error) {
	var status database.SyncStatus
	err := r.db.Where("org_id = ? AND type = ?", requireSyncOrgID(orgID), syncType).First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (r *SyncRepository) FindAllByOrg(orgID string) ([]database.SyncStatus, error) {
	var statuses []database.SyncStatus
	err := r.db.Where("org_id = ?", requireSyncOrgID(orgID)).Find(&statuses).Error
	return statuses, err
}
