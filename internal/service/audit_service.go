package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type AuditService struct {
	auditRepo *repository.AuditRepository
}

func NewAuditService(db *gorm.DB) *AuditService {
	return &AuditService{
		auditRepo: repository.NewAuditRepository(db),
	}
}

func (s *AuditService) GetLogs(page, pageSize int, filters map[string]string) ([]database.OperationLog, int64, error) {
	return s.auditRepo.FindAll(page, pageSize, filters)
}

func (s *AuditService) CreateLog(log *database.OperationLog) error {
	return s.auditRepo.Create(log)
}
