package repository

import (
	"errors"
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openUploadedFileDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:uploaded-file-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.UploadedFile{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestNewUploadedFileRepositoryWithOrgID_RequiresOrg(t *testing.T) {
	db := openUploadedFileDB(t)
	if _, err := NewUploadedFileRepositoryWithOrgID(db, ""); !errors.Is(err, ErrMissingOrgID) {
		t.Fatalf("err = %v, want ErrMissingOrgID", err)
	}
	if _, err := NewUploadedFileRepositoryWithOrgID(nil, "muteng"); err == nil {
		t.Fatal("expected error for nil db")
	}
}

func TestUploadedFileRepository_OrgIsolation(t *testing.T) {
	orgs := []string{"default", "xiaotie", "muteng"}
	for _, org := range orgs {
		t.Run(org, func(t *testing.T) {
			db := openUploadedFileDB(t)
			repo, err := NewUploadedFileRepositoryWithOrgID(db, org)
			if err != nil {
				t.Fatalf("repo: %v", err)
			}

			// cross-org write rejected
			if err := repo.Create(&database.UploadedFile{
				OrgID:          otherOrg(org),
				UploaderUserID: "u1",
				StoredName:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa.txt",
				OriginalName:   "a.txt",
				Size:           1,
			}); !errors.Is(err, ErrOrgMismatch) {
				t.Fatalf("cross-org create err = %v, want ErrOrgMismatch", err)
			}

			// same-org create inherits org when empty
			meta := &database.UploadedFile{
				UploaderUserID: "uploader-" + org,
				StoredName:     "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb.txt",
				OriginalName:   "b.txt",
				ContentType:    "text/plain",
				Size:           12,
			}
			if err := repo.Create(meta); err != nil {
				t.Fatalf("create: %v", err)
			}
			if meta.ID == 0 {
				t.Fatal("expected assigned id")
			}
			if meta.OrgID != org {
				t.Fatalf("OrgID = %q, want %q", meta.OrgID, org)
			}

			// find by id within org
			got, err := repo.FindByID(meta.ID)
			if err != nil {
				t.Fatalf("FindByID: %v", err)
			}
			if got.StoredName != meta.StoredName || got.OrgID != org {
				t.Fatalf("unexpected meta: %+v", got)
			}

			// other org cannot see the row
			foreign := otherOrg(org)
			foreignRepo, err := NewUploadedFileRepositoryWithOrgID(db, foreign)
			if err != nil {
				t.Fatalf("foreign repo: %v", err)
			}
			if _, err := foreignRepo.FindByID(meta.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("foreign FindByID err = %v, want ErrRecordNotFound", err)
			}

			// FindByStoredName scoped
			byName, err := repo.FindByStoredName(meta.StoredName)
			if err != nil {
				t.Fatalf("FindByStoredName: %v", err)
			}
			if byName.ID != meta.ID {
				t.Fatalf("id = %d, want %d", byName.ID, meta.ID)
			}
			if _, err := foreignRepo.FindByStoredName(meta.StoredName); !errors.Is(err, gorm.ErrRecordNotFound) {
				t.Fatalf("foreign FindByStoredName err = %v, want ErrRecordNotFound", err)
			}
		})
	}
}
