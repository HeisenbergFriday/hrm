package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type EmployeeService struct {
	employeeRepo *repository.EmployeeRepository
}

func NewEmployeeService(db *gorm.DB) *EmployeeService {
	return &EmployeeService{
		employeeRepo: repository.NewEmployeeRepository(db),
	}
}

// Profile

func (s *EmployeeService) GetProfiles(page, pageSize int, filters map[string]string) ([]database.EmployeeProfile, int64, error) {
	return s.employeeRepo.FindAllProfiles(page, pageSize, filters)
}

func (s *EmployeeService) GetProfileByID(id string) (*database.EmployeeProfile, error) {
	return s.employeeRepo.FindProfileByID(id)
}

func (s *EmployeeService) CreateProfile(profile *database.EmployeeProfile) error {
	return s.employeeRepo.CreateProfile(profile)
}

func (s *EmployeeService) UpdateProfile(profile *database.EmployeeProfile) error {
	return s.employeeRepo.UpdateProfile(profile)
}

// Transfer

func (s *EmployeeService) GetTransfers(page, pageSize int, status string) ([]database.EmployeeTransfer, int64, error) {
	return s.employeeRepo.FindAllTransfers(page, pageSize, status)
}

func (s *EmployeeService) CreateTransfer(transfer *database.EmployeeTransfer) error {
	return s.employeeRepo.CreateTransfer(transfer)
}

// Resignation

func (s *EmployeeService) GetResignations(page, pageSize int, status string) ([]database.EmployeeResignation, int64, error) {
	return s.employeeRepo.FindAllResignations(page, pageSize, status)
}

func (s *EmployeeService) CreateResignation(resignation *database.EmployeeResignation) error {
	return s.employeeRepo.CreateResignation(resignation)
}

// Onboarding

func (s *EmployeeService) GetOnboardings(page, pageSize int, status string) ([]database.EmployeeOnboarding, int64, error) {
	return s.employeeRepo.FindAllOnboardings(page, pageSize, status)
}

func (s *EmployeeService) CreateOnboarding(onboarding *database.EmployeeOnboarding) error {
	return s.employeeRepo.CreateOnboarding(onboarding)
}
