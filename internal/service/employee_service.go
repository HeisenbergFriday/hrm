package service

import (
	"strings"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type EmployeeService struct {
	employeeRepo *repository.EmployeeRepository
	orgID        string
}

// NewEmployeeService 仅保留给明确的全局迁移/审计场景。
// 普通 HTTP 请求必须使用 NewEmployeeServiceWithOrgID；无 org 时仓储读/写 fail-closed。
func NewEmployeeService(db *gorm.DB) *EmployeeService {
	return &EmployeeService{
		employeeRepo: repository.NewEmployeeRepository(db),
	}
}

// NewEmployeeServiceWithOrgID 构造带 org 隔离的员工服务。
// orgID 为空时仓储层 fail-closed，不会返回任何员工数据。
func NewEmployeeServiceWithOrgID(db *gorm.DB, orgID string) *EmployeeService {
	return &EmployeeService{
		employeeRepo: repository.NewEmployeeRepositoryWithOrgID(db, orgID),
		orgID:        orgID,
	}
}

// Profile

func (s *EmployeeService) GetProfiles(page, pageSize int, filters map[string]string) ([]database.EmployeeProfile, int64, error) {
	normalizedFilters := make(map[string]string, len(filters))
	for key, value := range filters {
		normalizedFilters[key] = value
	}
	normalizedFilters["keyword"] = strings.TrimSpace(normalizedFilters["keyword"])
	return s.employeeRepo.FindAllProfiles(page, pageSize, normalizedFilters)
}

func (s *EmployeeService) GetLifecycleLedger(page, pageSize int, filters map[string]string) ([]repository.EmployeeLifecycleLedgerItem, int64, error) {
	return s.employeeRepo.FindLifecycleLedger(page, pageSize, filters)
}

func (s *EmployeeService) GetProfileByID(id string) (*database.EmployeeProfile, error) {
	return s.employeeRepo.FindProfileByID(id)
}

func (s *EmployeeService) GetProfileByUserID(userID string) (*database.EmployeeProfile, error) {
	return s.employeeRepo.FindProfileByUserID(userID)
}

func (s *EmployeeService) CreateProfile(profile *database.EmployeeProfile) error {
	return s.employeeRepo.CreateProfile(profile)
}

func (s *EmployeeService) UpdateProfile(profile *database.EmployeeProfile) error {
	return s.employeeRepo.UpdateProfile(profile)
}

// Transfer

func (s *EmployeeService) GetTransfers(page, pageSize int, filters map[string]string) ([]database.EmployeeTransfer, int64, error) {
	return s.employeeRepo.FindAllTransfers(page, pageSize, filters)
}

func (s *EmployeeService) CreateTransfer(transfer *database.EmployeeTransfer) error {
	return s.employeeRepo.CreateTransfer(transfer)
}

// Resignation

func (s *EmployeeService) GetResignations(page, pageSize int, filters map[string]string) ([]database.EmployeeResignation, int64, error) {
	return s.employeeRepo.FindAllResignations(page, pageSize, filters)
}

func (s *EmployeeService) CreateResignation(resignation *database.EmployeeResignation) error {
	return s.employeeRepo.CreateResignation(resignation)
}

// Onboarding

func (s *EmployeeService) GetOnboardings(page, pageSize int, filters map[string]string) ([]database.EmployeeOnboarding, int64, error) {
	return s.employeeRepo.FindAllOnboardings(page, pageSize, filters)
}

func (s *EmployeeService) CreateOnboarding(onboarding *database.EmployeeOnboarding) error {
	return s.employeeRepo.CreateOnboarding(onboarding)
}
