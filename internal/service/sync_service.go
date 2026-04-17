package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"
	"time"

	"gorm.io/gorm"
)

type SyncService struct {
	syncRepo *repository.SyncRepository
}

func NewSyncService(db *gorm.DB) *SyncService {
	return &SyncService{
		syncRepo: repository.NewSyncRepository(db),
	}
}

func (s *SyncService) GetSyncStatus(syncType string) (*database.SyncStatus, error) {
	return s.syncRepo.FindByType(syncType)
}

func (s *SyncService) GetAllSyncStatus() ([]database.SyncStatus, error) {
	return s.syncRepo.FindAll()
}

func (s *SyncService) UpdateSyncStatus(syncType, status, message string) error {
	return s.syncRepo.Upsert(&database.SyncStatus{
		Type:         syncType,
		LastSyncTime: time.Now(),
		Status:       status,
		Message:      message,
	})
}
