package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type ApprovalService struct {
	approvalRepo *repository.ApprovalRepository
	templateRepo *repository.ApprovalTemplateRepository
}

func NewApprovalService(db *gorm.DB) *ApprovalService {
	return &ApprovalService{
		approvalRepo: repository.NewApprovalRepository(db),
		templateRepo: repository.NewApprovalTemplateRepository(db),
	}
}

func (s *ApprovalService) GetTemplates() ([]database.ApprovalTemplate, int64, error) {
	return s.templateRepo.FindAll()
}

func (s *ApprovalService) GetInstances(page, pageSize int, filters map[string]string) ([]database.Approval, int64, error) {
	return s.approvalRepo.FindAll(page, pageSize, filters)
}

func (s *ApprovalService) GetByID(id string) (*database.Approval, error) {
	return s.approvalRepo.FindByID(id)
}
