package repository

import (
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

// Upsert 更新或创建同步状态
func (r *SyncRepository) Upsert(status *database.SyncStatus) error {
	return r.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "type"}},
		DoUpdates: clause.AssignmentColumns([]string{"last_sync_time", "status", "message"}),
	}).Create(status).Error
}

func (r *SyncRepository) FindByType(syncType string) (*database.SyncStatus, error) {
	var status database.SyncStatus
	err := r.db.Where("type = ?", syncType).First(&status).Error
	if err != nil {
		return nil, err
	}
	return &status, nil
}

func (r *SyncRepository) FindAll() ([]database.SyncStatus, error) {
	var statuses []database.SyncStatus
	err := r.db.Find(&statuses).Error
	return statuses, err
}
