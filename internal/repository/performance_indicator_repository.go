package repository

import (
	"peopleops/internal/database"
	"strings"

	"gorm.io/gorm"
)

type PerformanceIndicatorLibraryRepository struct{ db *gorm.DB }

func NewPerformanceIndicatorLibraryRepository(db *gorm.DB) *PerformanceIndicatorLibraryRepository {
	return &PerformanceIndicatorLibraryRepository{db: db}
}

func (r *PerformanceIndicatorLibraryRepository) Create(lib *database.PerformanceIndicatorLibrary) error {
	return r.db.Create(lib).Error
}

func (r *PerformanceIndicatorLibraryRepository) GetByID(id uint) (*database.PerformanceIndicatorLibrary, error) {
	var lib database.PerformanceIndicatorLibrary
	if err := r.db.Where("id = ? AND deleted_at IS NULL", id).First(&lib).Error; err != nil {
		return nil, err
	}
	return &lib, nil
}

func (r *PerformanceIndicatorLibraryRepository) Update(lib *database.PerformanceIndicatorLibrary) error {
	return r.db.Save(lib).Error
}

func (r *PerformanceIndicatorLibraryRepository) Delete(id uint, deletedBy string) error {
	return r.db.Model(&database.PerformanceIndicatorLibrary{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at": gorm.Expr("NOW()"),
		"updated_by": deletedBy,
	}).Error
}

func (r *PerformanceIndicatorLibraryRepository) FindAll(page, pageSize int, departmentID, keyword, status string, visibleDepartmentIDs []string) ([]database.PerformanceIndicatorLibrary, int64, error) {
	var items []database.PerformanceIndicatorLibrary
	var total int64

	query := r.db.Model(&database.PerformanceIndicatorLibrary{})
	query = query.Where("deleted_at IS NULL")
	if departmentID != "" {
		query = query.Where("department_id = ?", departmentID)
	}
	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if status != "" {
		query = query.Where("status = ?", status)
	}
	// 部门隔离：只显示可见部门的指标库
	if len(visibleDepartmentIDs) > 0 {
		query = query.Where("department_id IN ?", visibleDepartmentIDs)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 10
	}
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

func (r *PerformanceIndicatorLibraryRepository) FindByDepartment(departmentID string) ([]database.PerformanceIndicatorLibrary, error) {
	var items []database.PerformanceIndicatorLibrary
	if err := r.db.Where("department_id = ? AND deleted_at IS NULL AND status = ?", departmentID, "active").
		Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PerformanceIndicatorLibraryRepository) Archive(id uint, updatedBy string) error {
	return r.db.Model(&database.PerformanceIndicatorLibrary{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     "archived",
		"updated_by": updatedBy,
	}).Error
}

// PerformanceIndicatorItemRepository 指标项 Repository
type PerformanceIndicatorItemRepository struct{ db *gorm.DB }

func NewPerformanceIndicatorItemRepository(db *gorm.DB) *PerformanceIndicatorItemRepository {
	return &PerformanceIndicatorItemRepository{db: db}
}

func (r *PerformanceIndicatorItemRepository) Create(item *database.PerformanceIndicatorItem) error {
	return r.db.Create(item).Error
}

func (r *PerformanceIndicatorItemRepository) GetByID(id uint) (*database.PerformanceIndicatorItem, error) {
	var item database.PerformanceIndicatorItem
	if err := r.db.Where("id = ? AND deleted_at IS NULL", id).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PerformanceIndicatorItemRepository) Update(item *database.PerformanceIndicatorItem) error {
	return r.db.Save(item).Error
}

func (r *PerformanceIndicatorItemRepository) Delete(id uint, deletedBy string) error {
	return r.db.Model(&database.PerformanceIndicatorItem{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at": gorm.Expr("NOW()"),
		"updated_by": deletedBy,
	}).Error
}

func (r *PerformanceIndicatorItemRepository) FindByLibrary(libraryID uint, sectionType string) ([]database.PerformanceIndicatorItem, error) {
	var items []database.PerformanceIndicatorItem
	query := r.db.Where("library_id = ? AND deleted_at IS NULL", libraryID)
	if sectionType != "" {
		query = query.Where("section_type = ?", sectionType)
	}
	if err := query.Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PerformanceIndicatorItemRepository) Search(libraryIDs []uint, keyword string, sectionType string) ([]database.PerformanceIndicatorItem, error) {
	var items []database.PerformanceIndicatorItem
	query := r.db.Where("deleted_at IS NULL")
	if len(libraryIDs) > 0 {
		query = query.Where("library_id IN ?", libraryIDs)
	}
	if sectionType != "" {
		query = query.Where("(section_type = ? OR indicator_type = ?)", sectionType, sectionType)
	}
	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("name LIKE ? OR description LIKE ?", like, like)
	}
	if err := query.Order("library_id ASC, sort_order ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PerformanceIndicatorItemRepository) BatchCreate(items []database.PerformanceIndicatorItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Create(&items).Error
}

func (r *PerformanceIndicatorItemRepository) DeleteByLibrary(libraryID uint, deletedBy string) error {
	return r.db.Model(&database.PerformanceIndicatorItem{}).Where("library_id = ?", libraryID).Updates(map[string]interface{}{
		"deleted_at": gorm.Expr("NOW()"),
		"updated_by": deletedBy,
	}).Error
}
