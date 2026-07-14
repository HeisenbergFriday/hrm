package repository

import (
	"peopleops/internal/database"

	"gorm.io/gorm"
)

type DepartmentRepository struct {
	db    *gorm.DB
	orgID string
}

func NewDepartmentRepository(db *gorm.DB) *DepartmentRepository {
	return &DepartmentRepository{
		db: db,
	}
}

// NewDepartmentRepositoryWithOrgID 构造带 org 隔离的部门仓储；orgID 为空时行为等同旧构造（不加过滤）。
func NewDepartmentRepositoryWithOrgID(db *gorm.DB, orgID string) *DepartmentRepository {
	return &DepartmentRepository{
		db:    db,
		orgID: orgID,
	}
}

func (r *DepartmentRepository) scoped() *gorm.DB {
	tx := r.db
	if r.orgID != "" {
		tx = tx.Where("org_id = ?", r.orgID)
	}
	return tx
}

func (r *DepartmentRepository) Create(department *database.Department) error {
	if department == nil {
		return gorm.ErrInvalidData
	}
	if r.orgID != "" {
		merged, err := EnsureSameOrg(r.orgID, department.OrgID)
		if err != nil {
			return err
		}
		department.OrgID = merged
	}
	return r.db.Create(department).Error
}

func (r *DepartmentRepository) Update(department *database.Department) error {
	if department == nil {
		return gorm.ErrInvalidData
	}
	if r.orgID != "" {
		merged, err := EnsureSameOrg(r.orgID, department.OrgID)
		if err != nil {
			return err
		}
		department.OrgID = merged
	}
	return r.db.Save(department).Error
}

func (r *DepartmentRepository) Delete(departmentID string) error {
	tx := r.db.Scopes(ScopeOrg(r.orgID, "org_id"))
	return tx.Delete(&database.Department{}, "department_id = ?", departmentID).Error
}

func (r *DepartmentRepository) FindByDepartmentID(departmentID string) (*database.Department, error) {
	var department database.Department
	err := r.scoped().Where("department_id = ?", departmentID).First(&department).Error
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
	var departmentIDs []string
	orgID := database.CurrentOrganizationIDFromDB(r.db)

	// 使用递归 CTE 查询所有子部门；orgID 非空时在 anchor 与 recursive 子句同时限定，
	// 避免同一 department_id 在多组织中重复出现导致跨企业串联。
	if r.orgID != "" {
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
		if err := r.db.Raw(query, parentDepartmentID, r.orgID, r.orgID).Scan(&departmentIDs).Error; err != nil {
			return nil, err
		}
		return departmentIDs, nil
	}

	query := `
		WITH RECURSIVE dept_tree AS (
			SELECT department_id, parent_id
			FROM departments
			WHERE org_id = ? AND department_id = ?
			UNION ALL
			SELECT d.department_id, d.parent_id
			FROM departments d
			INNER JOIN dept_tree dt ON d.org_id = ? AND d.parent_id = dt.department_id
		)
		SELECT department_id FROM dept_tree
	`

	err := r.db.Raw(query, orgID, parentDepartmentID, orgID).Scan(&departmentIDs).Error
	if err != nil {
		return nil, err
	}
	return departmentIDs, nil
}

// FindAllChildDepartmentIDsByParentID 通过内部 ID 递归查询子部门
func (r *DepartmentRepository) FindAllChildDepartmentIDsByParentID(parentID string) ([]string, error) {
	var departmentIDs []string
	orgID := database.CurrentOrganizationIDFromDB(r.db)

	if r.orgID != "" {
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
		if err := r.db.Raw(query, parentID, r.orgID, r.orgID).Scan(&departmentIDs).Error; err != nil {
			return nil, err
		}
		return departmentIDs, nil
	}

	query := `
		WITH RECURSIVE dept_tree AS (
			SELECT department_id, parent_id
			FROM departments
			WHERE org_id = ? AND parent_id = ?
			UNION ALL
			SELECT d.department_id, d.parent_id
			FROM departments d
			INNER JOIN dept_tree dt ON d.org_id = ? AND d.parent_id = dt.department_id
		)
		SELECT department_id FROM dept_tree
	`

	err := r.db.Raw(query, orgID, parentID, orgID).Scan(&departmentIDs).Error
	if err != nil {
		return nil, err
	}
	return departmentIDs, nil
}
