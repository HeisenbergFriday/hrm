package repository

import (
	"peopleops/internal/database"
	"peopleops/internal/requestmeta"
	"strings"
	"time"

	"gorm.io/gorm"
)

type PerformanceIndicatorLibraryRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceIndicatorLibraryRepository(db *gorm.DB) *PerformanceIndicatorLibraryRepository {
	return &PerformanceIndicatorLibraryRepository{db: db, orgID: tenantOrgIDFromDB(db)}
}

func NewPerformanceIndicatorLibraryRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceIndicatorLibraryRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceIndicatorLibraryRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func tenantOrgIDFromDB(db *gorm.DB) string {
	if db == nil || db.Statement == nil {
		return ""
	}
	orgID, err := requestmeta.TenantID(db.Statement.Context)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(orgID)
}

func (r *PerformanceIndicatorLibraryRepository) boundOrgID() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.orgID)
}

func (r *PerformanceIndicatorLibraryRepository) requireBoundOrg() (string, error) {
	orgID := r.boundOrgID()
	if orgID == "" {
		return "", ErrMissingOrgID
	}
	return orgID, nil
}

// scoped never returns the raw db without an org filter. Empty org uses 1=0.
func (r *PerformanceIndicatorLibraryRepository) scoped() *gorm.DB {
	return r.db.Scopes(ScopeOrg(r.boundOrgID(), "org_id"))
}

func (r *PerformanceIndicatorLibraryRepository) Create(lib *database.PerformanceIndicatorLibrary) error {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, lib.OrgID)
	if err != nil {
		return err
	}
	lib.OrgID = merged
	return r.scoped().Create(lib).Error
}

func (r *PerformanceIndicatorLibraryRepository) GetByID(id uint) (*database.PerformanceIndicatorLibrary, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var lib database.PerformanceIndicatorLibrary
	if err := r.scoped().Where("id = ? AND deleted_at IS NULL", id).First(&lib).Error; err != nil {
		return nil, err
	}
	return &lib, nil
}

func (r *PerformanceIndicatorLibraryRepository) Update(lib *database.PerformanceIndicatorLibrary) error {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, lib.OrgID)
	if err != nil {
		return err
	}
	lib.OrgID = merged
	return r.scoped().Save(lib).Error
}

func (r *PerformanceIndicatorLibraryRepository) Delete(id uint, deletedBy string) error {
	if _, err := r.requireBoundOrg(); err != nil {
		return err
	}
	now := time.Now()
	return r.scoped().Model(&database.PerformanceIndicatorLibrary{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at": now,
		"updated_by": deletedBy,
	}).Error
}

func (r *PerformanceIndicatorLibraryRepository) FindAll(page, pageSize int, departmentID, keyword, status string, visibleDepartmentIDs []string, templateIDs ...*uint) ([]database.PerformanceIndicatorLibrary, int64, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, 0, err
	}
	var items []database.PerformanceIndicatorLibrary
	var total int64

	query := r.scoped().Model(&database.PerformanceIndicatorLibrary{}).Where("deleted_at IS NULL")
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
	if len(templateIDs) > 0 && templateIDs[0] != nil && *templateIDs[0] > 0 {
		query = query.Where("template_id = ?", *templateIDs[0])
	}
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

func (r *PerformanceIndicatorLibraryRepository) FindByDepartment(departmentID string, templateIDs ...*uint) ([]database.PerformanceIndicatorLibrary, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var items []database.PerformanceIndicatorLibrary
	query := r.scoped().Where("department_id = ? AND deleted_at IS NULL AND status = ?", departmentID, "active")
	if len(templateIDs) > 0 && templateIDs[0] != nil && *templateIDs[0] > 0 {
		query = query.Where("template_id = ?", *templateIDs[0])
	}
	if err := query.Order("created_at DESC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PerformanceIndicatorLibraryRepository) Archive(id uint, updatedBy string) error {
	if _, err := r.requireBoundOrg(); err != nil {
		return err
	}
	return r.scoped().Model(&database.PerformanceIndicatorLibrary{}).Where("id = ?", id).Updates(map[string]interface{}{
		"status":     "archived",
		"updated_by": updatedBy,
	}).Error
}

type PerformanceIndicatorItemRepository struct {
	db    *gorm.DB
	orgID string
}

func NewPerformanceIndicatorItemRepository(db *gorm.DB) *PerformanceIndicatorItemRepository {
	return &PerformanceIndicatorItemRepository{db: db, orgID: tenantOrgIDFromDB(db)}
}

func NewPerformanceIndicatorItemRepositoryWithOrgID(db *gorm.DB, orgID string) *PerformanceIndicatorItemRepository {
	orgID = strings.TrimSpace(orgID)
	return &PerformanceIndicatorItemRepository{db: performanceRepositoryDB(db, orgID), orgID: orgID}
}

func (r *PerformanceIndicatorItemRepository) boundOrgID() string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.orgID)
}

func (r *PerformanceIndicatorItemRepository) requireBoundOrg() (string, error) {
	orgID := r.boundOrgID()
	if orgID == "" {
		return "", ErrMissingOrgID
	}
	return orgID, nil
}

func (r *PerformanceIndicatorItemRepository) scoped() *gorm.DB {
	return r.db.Scopes(ScopeOrg(r.boundOrgID(), "org_id"))
}

func (r *PerformanceIndicatorItemRepository) Create(item *database.PerformanceIndicatorItem) error {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, item.OrgID)
	if err != nil {
		return err
	}
	item.OrgID = merged
	return r.scoped().Create(item).Error
}

func (r *PerformanceIndicatorItemRepository) GetByID(id uint) (*database.PerformanceIndicatorItem, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var item database.PerformanceIndicatorItem
	if err := r.scoped().Where("id = ? AND deleted_at IS NULL", id).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (r *PerformanceIndicatorItemRepository) Update(item *database.PerformanceIndicatorItem) error {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	merged, err := EnsureSameOrg(orgID, item.OrgID)
	if err != nil {
		return err
	}
	item.OrgID = merged
	return r.scoped().Save(item).Error
}

func (r *PerformanceIndicatorItemRepository) Delete(id uint, deletedBy string) error {
	if _, err := r.requireBoundOrg(); err != nil {
		return err
	}
	now := time.Now()
	return r.scoped().Model(&database.PerformanceIndicatorItem{}).Where("id = ?", id).Updates(map[string]interface{}{
		"deleted_at": now,
		"updated_by": deletedBy,
	}).Error
}

func (r *PerformanceIndicatorItemRepository) FindByLibrary(libraryID uint, sectionType string) ([]database.PerformanceIndicatorItem, error) {
	if _, err := r.requireBoundOrg(); err != nil {
		return nil, err
	}
	var items []database.PerformanceIndicatorItem
	query := r.scoped().Where("library_id = ? AND deleted_at IS NULL", libraryID)
	if sectionType != "" {
		query = query.Where("section_type = ?", sectionType)
	}
	if err := query.Order("sort_order ASC, created_at ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PerformanceIndicatorItemRepository) Search(libraryIDs []uint, keyword string, sectionType string, visibleDepartmentIDs []string) ([]database.PerformanceIndicatorItem, error) {
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return nil, err
	}
	var items []database.PerformanceIndicatorItem
	query := r.db.Model(&database.PerformanceIndicatorItem{}).
		Where("performance_indicator_items.deleted_at IS NULL").
		Where("performance_indicator_items.org_id = ?", orgID)
	if len(visibleDepartmentIDs) > 0 {
		query = query.Joins("JOIN performance_indicator_libraries ON performance_indicator_libraries.id = performance_indicator_items.library_id AND performance_indicator_libraries.deleted_at IS NULL").
			Where("performance_indicator_libraries.department_id IN ?", visibleDepartmentIDs).
			Where("performance_indicator_libraries.org_id = ?", orgID)
	}
	if len(libraryIDs) > 0 {
		query = query.Where("performance_indicator_items.library_id IN ?", libraryIDs)
	}
	if sectionType != "" {
		query = query.Where("(performance_indicator_items.section_type = ? OR performance_indicator_items.indicator_type = ?)", sectionType, sectionType)
	}
	if keyword != "" {
		like := "%" + strings.TrimSpace(keyword) + "%"
		query = query.Where("performance_indicator_items.name LIKE ? OR performance_indicator_items.description LIKE ?", like, like)
	}
	if err := query.Order("performance_indicator_items.library_id ASC, performance_indicator_items.sort_order ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func (r *PerformanceIndicatorItemRepository) BatchCreate(items []database.PerformanceIndicatorItem) error {
	if len(items) == 0 {
		return nil
	}
	orgID, err := r.requireBoundOrg()
	if err != nil {
		return err
	}
	for i := range items {
		merged, err := EnsureSameOrg(orgID, items[i].OrgID)
		if err != nil {
			return err
		}
		items[i].OrgID = merged
	}
	return r.scoped().Create(&items).Error
}

func (r *PerformanceIndicatorItemRepository) DeleteByLibrary(libraryID uint, deletedBy string) error {
	if _, err := r.requireBoundOrg(); err != nil {
		return err
	}
	now := time.Now()
	return r.scoped().Model(&database.PerformanceIndicatorItem{}).Where("library_id = ?", libraryID).Updates(map[string]interface{}{
		"deleted_at": now,
		"updated_by": deletedBy,
	}).Error
}
