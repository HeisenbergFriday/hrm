package repository

import (
	"errors"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openIndicatorIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:ind-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&database.PerformanceIndicatorLibrary{}, &database.PerformanceIndicatorItem{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func seedIndicatorTwinOrgs(t *testing.T, db *gorm.DB) (libA, libB database.PerformanceIndicatorLibrary) {
	t.Helper()
	libA = database.PerformanceIndicatorLibrary{
		OrgID: "muteng", Name: "A lib", DepartmentID: "d1", Status: "active",
	}
	libB = database.PerformanceIndicatorLibrary{
		OrgID: "xiaotie", Name: "B lib", DepartmentID: "d1", Status: "active",
	}
	if err := db.Create(&libA).Error; err != nil {
		t.Fatalf("seed A: %v", err)
	}
	if err := db.Create(&libB).Error; err != nil {
		t.Fatalf("seed B: %v", err)
	}
	itemA := database.PerformanceIndicatorItem{OrgID: "muteng", LibraryID: libA.ID, Name: "A item", SectionType: "kpi"}
	itemB := database.PerformanceIndicatorItem{OrgID: "xiaotie", LibraryID: libB.ID, Name: "B item", SectionType: "kpi"}
	if err := db.Create(&itemA).Error; err != nil {
		t.Fatalf("seed itemA: %v", err)
	}
	if err := db.Create(&itemB).Error; err != nil {
		t.Fatalf("seed itemB: %v", err)
	}
	return libA, libB
}

func TestPerformanceIndicatorRepository_EmptyOrgFailClosed(t *testing.T) {
	db := openIndicatorIsolationDB(t)
	libA, _ := seedIndicatorTwinOrgs(t, db)

	libRepo := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "")
	itemRepo := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "")

	t.Run("library read", func(t *testing.T) {
		if _, err := libRepo.GetByID(libA.ID); !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("GetByID err=%v, want ErrMissingOrgID", err)
		}
		items, total, err := libRepo.FindAll(1, 10, "", "", "", nil)
		if !errors.Is(err, ErrMissingOrgID) || items != nil || total != 0 {
			t.Fatalf("FindAll err=%v items=%v total=%d", err, items, total)
		}
	})

	t.Run("library write", func(t *testing.T) {
		if err := libRepo.Create(&database.PerformanceIndicatorLibrary{Name: "x", DepartmentID: "d"}); !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("Create err=%v", err)
		}
		if err := libRepo.Update(&database.PerformanceIndicatorLibrary{ID: libA.ID, Name: "x"}); !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("Update err=%v", err)
		}
		if err := libRepo.Delete(libA.ID, "u"); !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("Delete err=%v", err)
		}
		if err := libRepo.Archive(libA.ID, "u"); !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("Archive err=%v", err)
		}
	})

	t.Run("item batch", func(t *testing.T) {
		if err := itemRepo.BatchCreate([]database.PerformanceIndicatorItem{{Name: "x", LibraryID: libA.ID}}); !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("BatchCreate err=%v", err)
		}
		if _, err := itemRepo.Search(nil, "", "", nil); !errors.Is(err, ErrMissingOrgID) {
			t.Fatalf("Search err=%v", err)
		}
	})
}

func TestPerformanceIndicatorRepository_CrossOrgIsolation(t *testing.T) {
	db := openIndicatorIsolationDB(t)
	libA, libB := seedIndicatorTwinOrgs(t, db)

	repoA := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "muteng")
	repoB := NewPerformanceIndicatorLibraryRepositoryWithOrgID(db, "xiaotie")
	itemA := NewPerformanceIndicatorItemRepositoryWithOrgID(db, "muteng")

	got, err := repoA.GetByID(libA.ID)
	if err != nil || got.OrgID != "muteng" {
		t.Fatalf("A read own: got=%#v err=%v", got, err)
	}
	if _, err := repoA.GetByID(libB.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("A must not read B library, err=%v", err)
	}
	if err := repoA.Update(&database.PerformanceIndicatorLibrary{ID: libB.ID, OrgID: "xiaotie", Name: "hack"}); !errors.Is(err, ErrOrgMismatch) && !errors.Is(err, gorm.ErrRecordNotFound) {
		// EnsureSameOrg rejects foreign org_id; if entity org emptied, scoped Save still cannot hit B row.
		t.Fatalf("A update B: err=%v", err)
	}
	if err := repoA.Delete(libB.ID, "u"); err != nil {
		t.Fatalf("delete foreign should no-op without error: %v", err)
	}
	var still database.PerformanceIndicatorLibrary
	if err := db.First(&still, libB.ID).Error; err != nil {
		t.Fatalf("B library should still exist: %v", err)
	}
	if still.DeletedAt != nil {
		t.Fatalf("B library must not be soft-deleted by A")
	}

	items, err := itemA.FindByLibrary(libB.ID, "")
	if err != nil {
		t.Fatalf("FindByLibrary: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("A must not see B items via library id, got %#v", items)
	}

	// Create injects bound org and rejects foreign.
	lib := &database.PerformanceIndicatorLibrary{Name: "new", DepartmentID: "d2", Status: "active"}
	if err := repoA.Create(lib); err != nil {
		t.Fatalf("create: %v", err)
	}
	if lib.OrgID != "muteng" {
		t.Fatalf("create inject org = %q", lib.OrgID)
	}
	if err := repoA.Create(&database.PerformanceIndicatorLibrary{OrgID: "xiaotie", Name: "cross", DepartmentID: "d2"}); !errors.Is(err, ErrOrgMismatch) {
		t.Fatalf("cross create err=%v", err)
	}
	_ = repoB
}
