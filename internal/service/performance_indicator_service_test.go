package service

import (
	stdsql "database/sql"
	"database/sql/driver"
	"fmt"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// stubCountMatcher 匹配包含 count(*) 的查询（用于 FindAll 的 Count 步骤）
func stubCountMatcher(table string) func(string, []driver.NamedValue) bool {
	table = strings.ToLower(table)
	return func(query string, _ []driver.NamedValue) bool {
		lower := strings.ToLower(query)
		return strings.Contains(lower, "count(*)") && strings.Contains(lower, table)
	}
}

// stubSelectMatcher 匹配不包含 count(*) 的普通 SELECT 查询
func stubSelectMatcher(table string) func(string, []driver.NamedValue) bool {
	table = strings.ToLower(table)
	return func(query string, _ []driver.NamedValue) bool {
		lower := strings.ToLower(query)
		return strings.Contains(lower, table) && !strings.Contains(lower, "count(*)")
	}
}

func newStubIndicatorService(t *testing.T, queries ...stubQueryResponse) *PerformanceIndicatorService {
	t.Helper()
	stubPerformanceDriverOnce.Do(func() {
		stdsql.Register(stubPerformanceDriverName, stubPerformanceDriver{})
	})

	dsn := fmt.Sprintf("indicator-%s-%d", t.Name(), time.Now().UnixNano())
	stubPerformanceDBs.Store(dsn, &stubPerformanceDB{queries: queries})
	t.Cleanup(func() {
		stubPerformanceDBs.Delete(dsn)
	})

	sqlDB, err := stdsql.Open(stubPerformanceDriverName, dsn)
	if err != nil {
		t.Fatalf("open stub sql db: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	db, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open stub gorm db: %v", err)
	}

	libRepo := repository.NewPerformanceIndicatorLibraryRepository(db)
	itemRepo := repository.NewPerformanceIndicatorItemRepository(db)
	return NewPerformanceIndicatorService(libRepo, itemRepo)
}

// ===================== 指标库测试 =====================

func TestCreateLibrary_Validation(t *testing.T) {
	svc := NewPerformanceIndicatorService(nil, nil)

	if err := svc.CreateLibrary(&database.PerformanceIndicatorLibrary{DepartmentID: "dept-1"}); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := svc.CreateLibrary(&database.PerformanceIndicatorLibrary{Name: "lib"}); err == nil {
		t.Fatal("empty department_id should fail")
	}
}

func TestCreateLibrary_Success(t *testing.T) {
	svc := newStubIndicatorService(t)

	lib := &database.PerformanceIndicatorLibrary{
		Name:         "test lib",
		DepartmentID: "dept-1",
	}
	if err := svc.CreateLibrary(lib); err != nil {
		t.Fatalf("CreateLibrary() error = %v", err)
	}
}

func TestGetLibrary(t *testing.T) {
	stubLib := database.PerformanceIndicatorLibrary{
		ID:           1,
		Name:         "lib-1",
		DepartmentID: "dept-1",
		Status:       "active",
	}

	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"},
		rows:    [][]driver.Value{{stubLib.ID, stubLib.DepartmentID, "", stubLib.Name, "", stubLib.Status, "", time.Now(), time.Now(), "", ""}},
	})

	got, err := svc.GetLibrary(1)
	if err != nil {
		t.Fatalf("GetLibrary() error = %v", err)
	}
	if got.Name != "lib-1" {
		t.Fatalf("GetLibrary() name = %q, want lib-1", got.Name)
	}
}

func TestUpdateLibrary_NotFound(t *testing.T) {
	svc := newStubIndicatorService(t) // empty db → GetByID returns error

	err := svc.UpdateLibrary(&database.PerformanceIndicatorLibrary{ID: 999, Name: "new"})
	if err == nil {
		t.Fatal("UpdateLibrary on non-existent should fail")
	}
}

func TestUpdateLibrary_Success(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_libraries"),
		columns: []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"},
		rows:    [][]driver.Value{{1, "dept-1", "Old Dept", "old name", "old desc", "active", "quarterly", now, now, "u1", "u1"}},
	})

	err := svc.UpdateLibrary(&database.PerformanceIndicatorLibrary{
		ID:             1,
		Name:           "new name",
		Description:    "new desc",
		DepartmentName: "New Dept",
		DefaultCycle:   "monthly",
		UpdatedBy:      "u2",
	})
	if err != nil {
		t.Fatalf("UpdateLibrary() error = %v", err)
	}
}

func TestListLibraries_WithScope(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t,
		// count query
		stubQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(1)}},
		},
		// select query
		stubQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"},
			rows:    [][]driver.Value{{1, "dept-1", "Dept", "lib-1", "", "active", "", now, now, "", ""}},
		},
	)

	// scope restricted to dept-1
	scope := &OrgDataScope{DepartmentIDs: []string{"dept-1"}}
	libs, total, err := svc.ListLibraries(1, 10, "", "", "", scope)
	if err != nil {
		t.Fatalf("ListLibraries() error = %v", err)
	}
	if total != 1 {
		t.Fatalf("ListLibraries() total = %d, want 1", total)
	}
	if len(libs) != 1 || libs[0].Name != "lib-1" {
		t.Fatalf("ListLibraries() = %#v", libs)
	}
}

func TestListLibraries_ScopeAll(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t,
		stubQueryResponse{
			match:   stubCountMatcher("performance_indicator_libraries"),
			columns: []string{"count(*)"},
			rows:    [][]driver.Value{{int64(2)}},
		},
		stubQueryResponse{
			match:   stubSelectMatcher("performance_indicator_libraries"),
			columns: []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"},
			rows: [][]driver.Value{
				{1, "dept-1", "Dept", "lib-1", "", "active", "", now, now, "", ""},
				{2, "dept-2", "Dept2", "lib-2", "", "active", "", now, now, "", ""},
			},
		},
	)

	// nil scope means all
	libs, total, err := svc.ListLibraries(1, 10, "", "", "", nil)
	if err != nil {
		t.Fatalf("ListLibraries() error = %v", err)
	}
	if total != 2 {
		t.Fatalf("ListLibraries() total = %d, want 2", total)
	}
	if len(libs) != 2 {
		t.Fatalf("ListLibraries() len = %d, want 2", len(libs))
	}
}

func TestArchiveLibrary(t *testing.T) {
	svc := newStubIndicatorService(t)
	if err := svc.ArchiveLibrary(1, "u1"); err != nil {
		t.Fatalf("ArchiveLibrary() error = %v", err)
	}
}

func TestInheritLibrary_ParentNotFound(t *testing.T) {
	svc := newStubIndicatorService(t) // empty → GetByID returns error

	_, err := svc.InheritLibrary(999, "dept-2", "Dept2", "", "", "u1")
	if err == nil {
		t.Fatal("InheritLibrary with non-existent parent should fail")
	}
}

func TestInheritLibrary_Success(t *testing.T) {
	now := time.Now()
	parentLib := database.PerformanceIndicatorLibrary{
		ID:             1,
		Name:           "parent",
		Description:    "parent desc",
		DefaultCycle:   "quarterly",
		DepartmentID:   "dept-1",
		DepartmentName: "Dept1",
	}
	parentItem := database.PerformanceIndicatorItem{
		ID:            10,
		LibraryID:     1,
		SectionType:   "quantitative",
		Name:          "KPI",
		IndicatorType: "number",
		DefaultWeight: 0.6,
		Weight:        0.6,
		IsDefault:     true,
		SortOrder:     1,
	}

	svc := newStubIndicatorService(t,
		// 1) GetByID for parent lib
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_libraries"),
			columns: []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"},
			rows:    [][]driver.Value{{parentLib.ID, parentLib.DepartmentID, parentLib.DepartmentName, parentLib.Name, parentLib.Description, "active", parentLib.DefaultCycle, now, now, "", ""}},
		},
		// 2) FindByLibrary for parent items
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_items"),
			columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
			rows:    [][]driver.Value{{parentItem.ID, parentItem.LibraryID, parentItem.SectionType, parentItem.Name, "", parentItem.IndicatorType, "", "", "", parentItem.DefaultWeight, "", "", "", "", parentItem.Weight, parentItem.IsDefault, parentItem.IsInherited, parentItem.SortOrder, now, now, "", ""}},
		},
	)

	newLib, err := svc.InheritLibrary(1, "dept-2", "Dept2", "child lib", "child desc", "u1")
	if err != nil {
		t.Fatalf("InheritLibrary() error = %v", err)
	}
	if newLib.Name != "child lib" {
		t.Fatalf("InheritLibrary() name = %q, want child lib", newLib.Name)
	}
	if newLib.Description != "child desc" {
		t.Fatalf("InheritLibrary() desc = %q, want child desc", newLib.Description)
	}
	if newLib.DepartmentID != "dept-2" {
		t.Fatalf("InheritLibrary() dept = %q, want dept-2", newLib.DepartmentID)
	}
	if newLib.ParentLibraryID == nil || *newLib.ParentLibraryID != 1 {
		t.Fatal("InheritLibrary() parent_library_id not set correctly")
	}
	if newLib.DefaultCycle != "quarterly" {
		t.Fatalf("InheritLibrary() cycle = %q, want quarterly", newLib.DefaultCycle)
	}
}

func TestInheritLibrary_EmptyNameUsesParent(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t,
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_libraries"),
			columns: []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"},
			rows:    [][]driver.Value{{1, "dept-1", "Dept1", "parent lib", "parent desc", "active", "monthly", now, now, "", ""}},
		},
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_items"),
			columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
			rows:    [][]driver.Value{},
		},
	)

	newLib, err := svc.InheritLibrary(1, "dept-2", "Dept2", "", "", "u1")
	if err != nil {
		t.Fatalf("InheritLibrary() error = %v", err)
	}
	if newLib.Name != "parent lib" {
		t.Fatalf("empty name should use parent name, got %q", newLib.Name)
	}
	if newLib.Description != "parent desc" {
		t.Fatalf("empty description should use parent desc, got %q", newLib.Description)
	}
}

// ===================== 指标项测试 =====================

func TestCreateItem_Validation(t *testing.T) {
	svc := NewPerformanceIndicatorService(nil, nil)

	if err := svc.CreateItem(&database.PerformanceIndicatorItem{Name: "x", SectionType: "y"}); err == nil {
		t.Fatal("empty library_id should fail")
	}
	if err := svc.CreateItem(&database.PerformanceIndicatorItem{LibraryID: 1, SectionType: "y"}); err == nil {
		t.Fatal("empty name should fail")
	}
	if err := svc.CreateItem(&database.PerformanceIndicatorItem{LibraryID: 1, Name: "x"}); err == nil {
		t.Fatal("empty section_type should fail")
	}
}

func TestCreateItem_LibraryNotFound(t *testing.T) {
	svc := newStubIndicatorService(t) // empty → GetByID returns error

	err := svc.CreateItem(&database.PerformanceIndicatorItem{
		LibraryID:   999,
		Name:        "kpi",
		SectionType: "quantitative",
	})
	if err == nil {
		t.Fatal("CreateItem with non-existent library should fail")
	}
	if !strings.Contains(err.Error(), "指标库不存在") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestCreateItem_Success(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t,
		// lib exists check
		stubQueryResponse{
			match:   stubTableMatcher("performance_indicator_libraries"),
			columns: []string{"id", "department_id", "department_name", "name", "description", "status", "default_cycle", "created_at", "updated_at", "created_by", "updated_by"},
			rows:    [][]driver.Value{{1, "dept-1", "Dept", "lib", "", "active", "", now, now, "", ""}},
		},
	)

	err := svc.CreateItem(&database.PerformanceIndicatorItem{
		LibraryID:   1,
		Name:        "kpi",
		SectionType: "quantitative",
	})
	if err != nil {
		t.Fatalf("CreateItem() error = %v", err)
	}
}

func TestGetItem(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
		rows:    [][]driver.Value{{5, 1, "quantitative", "KPI", "", "number", "", "", "", 0.6, "", "", "", "", 0.6, true, false, 1, now, now, "", ""}},
	})

	got, err := svc.GetItem(5)
	if err != nil {
		t.Fatalf("GetItem() error = %v", err)
	}
	if got.Name != "KPI" {
		t.Fatalf("GetItem() name = %q, want KPI", got.Name)
	}
}

func TestUpdateItem_NotFound(t *testing.T) {
	svc := newStubIndicatorService(t)

	err := svc.UpdateItem(&database.PerformanceIndicatorItem{ID: 999, Name: "new"})
	if err == nil {
		t.Fatal("UpdateItem on non-existent should fail")
	}
}

func TestUpdateItem_Success(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
		rows:    [][]driver.Value{{1, 1, "quantitative", "old", "", "number", "", "", "", 0.5, "", "", "", "", 0.5, false, false, 1, now, now, "", ""}},
	})

	err := svc.UpdateItem(&database.PerformanceIndicatorItem{
		ID:            1,
		Name:          "new name",
		Description:   "new desc",
		IndicatorType: "percent",
		Weight:        0.8,
		SortOrder:     2,
		UpdatedBy:     "u1",
	})
	if err != nil {
		t.Fatalf("UpdateItem() error = %v", err)
	}
}

func TestListItemsByLibrary(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
		rows: [][]driver.Value{
			{1, 1, "quantitative", "KPI1", "", "number", "", "", "", 0.5, "", "", "", "", 0.5, true, false, 1, now, now, "", ""},
			{2, 1, "quantitative", "KPI2", "", "number", "", "", "", 0.5, "", "", "", "", 0.5, false, false, 2, now, now, "", ""},
		},
	})

	items, err := svc.ListItemsByLibrary(1, "")
	if err != nil {
		t.Fatalf("ListItemsByLibrary() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("ListItemsByLibrary() len = %d, want 2", len(items))
	}
}

func TestListItemsByLibrary_FilterBySectionType(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
		rows: [][]driver.Value{
			{3, 1, "key_action", "Action1", "", "text", "", "", "", 0, "", "", "", "", 0, false, false, 1, now, now, "", ""},
		},
	})

	items, err := svc.ListItemsByLibrary(1, "key_action")
	if err != nil {
		t.Fatalf("ListItemsByLibrary() error = %v", err)
	}
	if len(items) != 1 || items[0].SectionType != "key_action" {
		t.Fatalf("ListItemsByLibrary() = %#v", items)
	}
}

func TestSearchItems(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
		rows: [][]driver.Value{
			{1, 1, "quantitative", "客户满意度", "", "number", "", "", "", 0.6, "", "", "", "", 0.6, true, false, 1, now, now, "", ""},
		},
	})

	items, err := svc.SearchItems([]uint{1}, "客户", "")
	if err != nil {
		t.Fatalf("SearchItems() error = %v", err)
	}
	if len(items) != 1 || !strings.Contains(items[0].Name, "客户") {
		t.Fatalf("SearchItems() = %#v", items)
	}
}

func TestSearchItems_EmptyLibIDs(t *testing.T) {
	now := time.Now()
	svc := newStubIndicatorService(t, stubQueryResponse{
		match:   stubTableMatcher("performance_indicator_items"),
		columns: []string{"id", "library_id", "section_type", "name", "description", "indicator_type", "calculation_method", "data_source", "cycle", "default_weight", "red_line_value", "target_value", "challenge_value", "scoring_rule", "weight", "is_default", "is_inherited", "sort_order", "created_at", "updated_at", "created_by", "updated_by"},
		rows: [][]driver.Value{
			{1, 1, "quantitative", "KPI1", "", "number", "", "", "", 0.5, "", "", "", "", 0.5, true, false, 1, now, now, "", ""},
			{2, 2, "key_action", "Action1", "", "text", "", "", "", 0, "", "", "", "", 0, false, false, 1, now, now, "", ""},
		},
	})

	items, err := svc.SearchItems(nil, "", "")
	if err != nil {
		t.Fatalf("SearchItems() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("SearchItems() len = %d, want 2", len(items))
	}
}
