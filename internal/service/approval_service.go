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

// NewApprovalService binds org from the DB request/tenant context (fail-closed when missing).
func NewApprovalService(db *gorm.DB) *ApprovalService {
	return &ApprovalService{
		approvalRepo: repository.NewApprovalRepository(db),
		templateRepo: repository.NewApprovalTemplateRepository(db),
	}
}

func NewApprovalServiceWithOrgID(db *gorm.DB, orgID string) *ApprovalService {
	return &ApprovalService{
		approvalRepo: repository.NewApprovalRepositoryWithOrgID(db, orgID),
		templateRepo: repository.NewApprovalTemplateRepositoryWithOrgID(db, orgID),
	}
}

func (s *ApprovalService) GetTemplates() ([]database.ApprovalTemplate, int64, error) {
	return s.templateRepo.FindAll()
}

func (s *ApprovalService) GetInstances(page, pageSize int, filters map[string]string) ([]database.Approval, int64, error) {
	return s.approvalRepo.FindAll(page, pageSize, filters)
}

// GetInstancesFilteredByTitleKeywords 用于 category 分类筛选：按审批标题关键字命中或排除进行过滤。
// include=true 时命中任一关键字入选（具体分类）；include=false 时排除所有关键字（"other" 分类）。
func (s *ApprovalService) GetInstancesFilteredByTitleKeywords(page, pageSize int, keywords []string, include bool, filters map[string]string) ([]database.Approval, int64, error) {
	return s.approvalRepo.FindAllByTitleKeywords(page, pageSize, keywords, include, filters)
}

func (s *ApprovalService) GetByID(id string) (*database.Approval, error) {
	return s.approvalRepo.FindByID(id)
}
