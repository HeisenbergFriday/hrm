package service

import (
	"fmt"
	"peopleops/internal/database"
	"peopleops/internal/repository"
)

type PerformanceIndicatorService struct {
	libRepo  *repository.PerformanceIndicatorLibraryRepository
	itemRepo *repository.PerformanceIndicatorItemRepository
}

func NewPerformanceIndicatorService(
	libRepo *repository.PerformanceIndicatorLibraryRepository,
	itemRepo *repository.PerformanceIndicatorItemRepository,
) *PerformanceIndicatorService {
	return &PerformanceIndicatorService{
		libRepo:  libRepo,
		itemRepo: itemRepo,
	}
}

// ===================== 指标库管理 =====================

func (s *PerformanceIndicatorService) CreateLibrary(lib *database.PerformanceIndicatorLibrary) error {
	if lib.Name == "" {
		return fmt.Errorf("指标库名称不能为空")
	}
	if lib.DepartmentID == "" {
		return fmt.Errorf("部门 ID 不能为空")
	}
	return s.libRepo.Create(lib)
}

func (s *PerformanceIndicatorService) GetLibrary(id uint) (*database.PerformanceIndicatorLibrary, error) {
	return s.libRepo.GetByID(id)
}

func (s *PerformanceIndicatorService) UpdateLibrary(lib *database.PerformanceIndicatorLibrary) error {
	existing, err := s.libRepo.GetByID(lib.ID)
	if err != nil {
		return err
	}
	existing.Name = lib.Name
	existing.Description = lib.Description
	existing.DepartmentName = lib.DepartmentName
	existing.DefaultCycle = lib.DefaultCycle
	existing.UpdatedBy = lib.UpdatedBy
	return s.libRepo.Update(existing)
}

func (s *PerformanceIndicatorService) ListLibraries(page, pageSize int, departmentID, keyword, status string, scope *OrgDataScope) ([]database.PerformanceIndicatorLibrary, int64, error) {
	var visibleDepartmentIDs []string
	if scope != nil && !scope.IsAll() {
		visibleDepartmentIDs = scope.DepartmentIDs
	}
	return s.libRepo.FindAll(page, pageSize, departmentID, keyword, status, visibleDepartmentIDs)
}

func (s *PerformanceIndicatorService) GetLibrariesByDepartment(departmentID string) ([]database.PerformanceIndicatorLibrary, error) {
	return s.libRepo.FindByDepartment(departmentID)
}

func (s *PerformanceIndicatorService) ArchiveLibrary(id uint, updatedBy string) error {
	return s.libRepo.Archive(id, updatedBy)
}

func (s *PerformanceIndicatorService) InheritLibrary(parentID uint, targetDepartmentID, targetDepartmentName, name, description, createdBy string) (*database.PerformanceIndicatorLibrary, error) {
	parent, err := s.libRepo.GetByID(parentID)
	if err != nil {
		return nil, fmt.Errorf("父指标库不存在: %w", err)
	}

	libName := parent.Name
	if name != "" {
		libName = name
	}
	libDesc := parent.Description
	if description != "" {
		libDesc = description
	}

	newLib := &database.PerformanceIndicatorLibrary{
		DepartmentID:    targetDepartmentID,
		DepartmentName:  targetDepartmentName,
		ParentLibraryID: &parent.ID,
		Name:            libName,
		Description:     libDesc,
		DefaultCycle:    parent.DefaultCycle,
		Status:          "active",
		CreatedBy:       createdBy,
		UpdatedBy:       createdBy,
	}
	if err := s.libRepo.Create(newLib); err != nil {
		return nil, err
	}

	// 复制指标项
	items, err := s.itemRepo.FindByLibrary(parentID, "")
	if err != nil {
		return newLib, nil // 指标库创建成功，但复制指标项失败
	}
	if len(items) > 0 {
		newItems := make([]database.PerformanceIndicatorItem, len(items))
		for i, item := range items {
			newItems[i] = database.PerformanceIndicatorItem{
				LibraryID:         newLib.ID,
				ParentIndicatorID: &item.ID,
				SectionType:       item.SectionType,
				Name:              item.Name,
				Description:       item.Description,
				IndicatorType:     item.IndicatorType,
				Keywords:          item.Keywords,
				CalculationMethod: item.CalculationMethod,
				DataSource:        item.DataSource,
				Cycle:             item.Cycle,
				DefaultWeight:     item.DefaultWeight,
				RedLineValue:      item.RedLineValue,
				TargetValue:       item.TargetValue,
				ChallengeValue:    item.ChallengeValue,
				ScoringRule:       item.ScoringRule,
				Weight:            item.Weight,
				IsDefault:         item.IsDefault,
				IsInherited:       true,
				SortOrder:         item.SortOrder,
				CreatedBy:         createdBy,
				UpdatedBy:         createdBy,
			}
		}
		_ = s.itemRepo.BatchCreate(newItems)
	}

	return newLib, nil
}

// ===================== 指标项管理 =====================

func (s *PerformanceIndicatorService) CreateItem(item *database.PerformanceIndicatorItem) error {
	if item.LibraryID == 0 {
		return fmt.Errorf("指标库 ID 不能为空")
	}
	if item.Name == "" {
		return fmt.Errorf("指标项名称不能为空")
	}
	if item.SectionType == "" {
		return fmt.Errorf("指标类型不能为空")
	}

	// 验证指标库存在
	if _, err := s.libRepo.GetByID(item.LibraryID); err != nil {
		return fmt.Errorf("指标库不存在: %w", err)
	}

	return s.itemRepo.Create(item)
}

func (s *PerformanceIndicatorService) GetItem(id uint) (*database.PerformanceIndicatorItem, error) {
	return s.itemRepo.GetByID(id)
}

func (s *PerformanceIndicatorService) UpdateItem(item *database.PerformanceIndicatorItem) error {
	existing, err := s.itemRepo.GetByID(item.ID)
	if err != nil {
		return err
	}
	existing.Name = item.Name
	existing.Description = item.Description
	existing.IndicatorType = item.IndicatorType
	existing.Keywords = item.Keywords
	existing.CalculationMethod = item.CalculationMethod
	existing.DataSource = item.DataSource
	existing.Cycle = item.Cycle
	existing.DefaultWeight = item.DefaultWeight
	existing.Weight = item.Weight
	existing.RedLineValue = item.RedLineValue
	existing.TargetValue = item.TargetValue
	existing.ChallengeValue = item.ChallengeValue
	existing.ScoringRule = item.ScoringRule
	existing.IsDefault = item.IsDefault
	existing.SortOrder = item.SortOrder
	existing.UpdatedBy = item.UpdatedBy
	return s.itemRepo.Update(existing)
}

func (s *PerformanceIndicatorService) DeleteItem(id uint, deletedBy string) error {
	return s.itemRepo.Delete(id, deletedBy)
}

func (s *PerformanceIndicatorService) ListItemsByLibrary(libraryID uint, sectionType string) ([]database.PerformanceIndicatorItem, error) {
	return s.itemRepo.FindByLibrary(libraryID, sectionType)
}

func (s *PerformanceIndicatorService) SearchItems(libraryIDs []uint, keyword string, sectionType string) ([]database.PerformanceIndicatorItem, error) {
	return s.itemRepo.Search(libraryIDs, keyword, sectionType)
}
