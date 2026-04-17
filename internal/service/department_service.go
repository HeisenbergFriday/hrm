package service

import (
	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/gorm"
)

type DepartmentService struct {
	departmentRepo *repository.DepartmentRepository
}

func NewDepartmentService(db *gorm.DB) *DepartmentService {
	return &DepartmentService{
		departmentRepo: repository.NewDepartmentRepository(db),
	}
}

func (s *DepartmentService) CreateDepartment(department *database.Department) error {
	return s.departmentRepo.Create(department)
}

func (s *DepartmentService) UpdateDepartment(department *database.Department) error {
	return s.departmentRepo.Update(department)
}

func (s *DepartmentService) DeleteDepartment(departmentID string) error {
	return s.departmentRepo.Delete(departmentID)
}

func (s *DepartmentService) GetDepartmentByDepartmentID(departmentID string) (*database.Department, error) {
	return s.departmentRepo.FindByDepartmentID(departmentID)
}

func (s *DepartmentService) GetDepartmentByID(id string) (*database.Department, error) {
	return s.departmentRepo.FindByID(id)
}

func (s *DepartmentService) GetAllDepartments() ([]database.Department, error) {
	return s.departmentRepo.FindAll()
}

func (s *DepartmentService) GetDepartmentsByParent(parentID string) ([]database.Department, error) {
	return s.departmentRepo.FindByParent(parentID)
}

func (s *DepartmentService) UpdateDepartmentExtension(departmentID string, extension map[string]interface{}) error {
	department, err := s.departmentRepo.FindByDepartmentID(departmentID)
	if err != nil {
		return err
	}

	department.Extension = extension
	return s.departmentRepo.Update(department)
}