package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type DepartmentRepository struct {
	db *gorm.DB
}

func NewDepartmentRepository(db *gorm.DB) *DepartmentRepository {
	return &DepartmentRepository{
		db: db,
	}
}

func (r *DepartmentRepository) Create(department *database.Department) error {
	return r.db.Create(department).Error
}

func (r *DepartmentRepository) Update(department *database.Department) error {
	return r.db.Save(department).Error
}

func (r *DepartmentRepository) Delete(departmentID string) error {
	return r.db.Delete(&database.Department{}, "department_id = ?", departmentID).Error
}

func (r *DepartmentRepository) FindByDepartmentID(departmentID string) (*database.Department, error) {
	var department database.Department
	err := r.db.Where("department_id = ?", departmentID).First(&department).Error
	if err != nil {
		return nil, err
	}
	return &department, nil
}

func (r *DepartmentRepository) FindByID(id string) (*database.Department, error) {
	var department database.Department
	err := r.db.First(&department, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &department, nil
}

func (r *DepartmentRepository) FindAll() ([]database.Department, error) {
	var departments []database.Department
	err := r.db.Find(&departments).Error
	if err != nil {
		return nil, err
	}
	return departments, nil
}

func (r *DepartmentRepository) FindByParent(parentID string) ([]database.Department, error) {
	var departments []database.Department
	err := r.db.Where("parent_id = ?", parentID).Find(&departments).Error
	if err != nil {
		return nil, err
	}
	return departments, nil
}