package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type TalentService struct {
	talentRepo *repository.TalentRepository
}

func NewTalentService(db *gorm.DB) *TalentService {
	return &TalentService{
		talentRepo: repository.NewTalentRepository(db),
	}
}

// NewTalentServiceWithOrgID 构造带 org 隔离的人才分析服务。
func NewTalentServiceWithOrgID(db *gorm.DB, orgID string) *TalentService {
	return &TalentService{
		talentRepo: repository.NewTalentRepositoryWithOrgID(db, orgID),
	}
}

func (s *TalentService) GetList(page, pageSize int, departmentID string) ([]database.TalentAnalysis, int64, error) {
	return s.talentRepo.FindAll(page, pageSize, departmentID)
}

func (s *TalentService) GetByID(id string) (*database.TalentAnalysis, error) {
	return s.talentRepo.FindByID(id)
}

func (s *TalentService) Create(analysis *database.TalentAnalysis) error {
	return s.talentRepo.Create(analysis)
}
