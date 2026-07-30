package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type DepartmentRepository struct {
	db     *gorm.DB
	orgID  string
	orgErr error
}

func NewDepartmentRepository(db *gorm.DB) *DepartmentRepository {
	orgID, err := database.RequireOrganizationIDFromDB(db)
	return &DepartmentRepository{db: db, orgID: orgID, orgErr: err}
}

// NewDepartmentRepositoryWithOrgID 构造带 org 隔离的部门仓储；orgID 为空时绑定失败（fail-closed）。
func NewDepartmentRepositoryWithOrgID(db *gorm.DB, orgID string) *DepartmentRepository {
	normalized, err := RequireOrgID(orgID)
	return &DepartmentRepository{db: db, orgID: normalized, orgErr: err}
}

func (r *DepartmentRepository) requireOrgID() (string, error) {
	if r == nil || r.db == nil {
		return "", ErrMissingOrgID
	}
	if r.orgErr != nil {
		return "", r.orgErr
	}
	return RequireOrgID(r.orgID)
}

func (r *DepartmentRepository) scoped() *gorm.DB {
	orgID, err := r.requireOrgID()
	if err != nil {
		return r.db.Where("1 = 0")
	}
	return r.db.Where("org_id = ?", orgID)
}

func (r *DepartmentRepository) Create(department *database.Department) error {
	if department == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, department.OrgID)
	if err != nil {
		return err
	}
	department.OrgID = merged
	return r.db.Create(department).Error
}

func (r *DepartmentRepository) Update(department *database.Department) error {
	if department == nil {
		return gorm.ErrInvalidData
	}
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, department.OrgID)
	if err != nil {
		return err
	}
	department.OrgID = merged
	return r.db.Save(department).Error
}

func (r *DepartmentRepository) Delete(departmentID string) error {
	orgID, err := r.requireOrgID()
	if err != nil {
		return err
	}
	return rowsAffectedOrNotFound(r.db.Where("org_id = ? AND department_id = ?", orgID, departmentID).
		Delete(&database.Department{}))
}

func (r *DepartmentRepository) FindByDepartmentID(departmentID string) (*database.Department, error) {
	var department database.Department
	err := r.scoped().Where("department_id = ?", departmentID).First(&department).Error
	if err != nil {
		return nil, err
	}
	return &department, nil
}

func (r *DepartmentRepository) FindByDingTalkDepartmentID(dingTalkDepartmentID string) (*database.Department, error) {
	var department database.Department
	err := r.scoped().Where("dingtalk_department_id = ?", dingTalkDepartmentID).First(&department).Error
	if err != nil {
		return nil, err
	}
	return &department, nil
}

func (r *DepartmentRepository) FindByID(id string) (*database.Department, error) {
	var department database.Department
	err := r.scoped().First(&department, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &department, nil
}

func (r *DepartmentRepository) FindAll() ([]database.Department, error) {
	var departments []database.Department
	err := r.scoped().Find(&departments).Error
	if err != nil {
		return nil, err
	}
	return departments, nil
}

func (r *DepartmentRepository) FindByParent(parentID string) ([]database.Department, error) {
	var departments []database.Department
	err := r.scoped().Where("parent_id = ?", parentID).Find(&departments).Error
	if err != nil {
		return nil, err
	}
	return departments, nil
}

// FindAllChildDepartmentIDs 递归查询指定部门及其所有子部门的 ID 列表
func (r *DepartmentRepository) FindAllChildDepartmentIDs(parentDepartmentID string) ([]string, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var departmentIDs []string

	// 使用递归 CTE 查询所有子部门；anchor 与 recursive 子句同时限定 org_id，
	// 避免同一 department_id 在多组织中重复出现导致跨企业串联。
	query := `
		WITH RECURSIVE dept_tree AS (
			SELECT department_id, parent_id
			FROM departments
			WHERE department_id = ? AND org_id = ?
			UNION ALL
			SELECT d.department_id, d.parent_id
			FROM departments d
			INNER JOIN dept_tree dt ON d.parent_id = dt.department_id
			WHERE d.org_id = ?
		)
		SELECT department_id FROM dept_tree
	`
	if err := r.db.Raw(query, parentDepartmentID, orgID, orgID).Scan(&departmentIDs).Error; err != nil {
		return nil, err
	}
	return departmentIDs, nil
}

// FindAllChildDepartmentIDsByParentID 通过内部 ID 递归查询子部门
func (r *DepartmentRepository) FindAllChildDepartmentIDsByParentID(parentID string) ([]string, error) {
	orgID, err := r.requireOrgID()
	if err != nil {
		return nil, err
	}
	var departmentIDs []string

	query := `
		WITH RECURSIVE dept_tree AS (
			SELECT department_id, parent_id
			FROM departments
			WHERE parent_id = ? AND org_id = ?
			UNION ALL
			SELECT d.department_id, d.parent_id
			FROM departments d
			INNER JOIN dept_tree dt ON d.parent_id = dt.department_id
			WHERE d.org_id = ?
		)
		SELECT department_id FROM dept_tree
	`
	if err := r.db.Raw(query, parentID, orgID, orgID).Scan(&departmentIDs).Error; err != nil {
		return nil, err
	}
	return departmentIDs, nil
}
