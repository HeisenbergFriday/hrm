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

func (s *SyncService) GetSyncStatus(orgID, syncType string) (*database.SyncStatus, error) {
	return s.syncRepo.FindByOrgAndType(orgID, syncType)
}

func (s *SyncService) GetSyncStatusByRequestID(orgID, syncType, requestID string) (*database.SyncStatus, error) {
	return s.syncRepo.FindByOrgTypeAndRequestID(orgID, syncType, requestID)
}

func (s *SyncService) GetAllSyncStatus(orgID string) ([]database.SyncStatus, error) {
	return s.syncRepo.FindAllByOrg(orgID)
}

func (s *SyncService) UpdateSyncStatus(orgID, syncType, status, message string) error {
	return s.UpdateSyncStatusDetails(&database.SyncStatus{
		OrgID:        orgID,
		Type:         syncType,
		LastSyncTime: time.Now(),
		Status:       status,
		Message:      message,
	})
}

func (s *SyncService) UpdateSyncStatusDetails(status *database.SyncStatus) error {
	if status == nil {
		return gorm.ErrInvalidData
	}
	if status.LastSyncTime.IsZero() {
		status.LastSyncTime = time.Now()
	}
	return s.syncRepo.Upsert(status)
}
