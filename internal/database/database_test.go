package database

import (
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupDatabaseTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	return db
}

func TestMigrateShiftCatalogSchemaBackfillsShiftKeyAndIndex(t *testing.T) {
	db := setupDatabaseTestDB(t)

	if err := db.Exec(`
		CREATE TABLE dingtalk_shift_catalogs (
			id integer primary key autoincrement,
			name text not null,
			shift_key text,
			shift_id integer not null,
			check_in text,
			check_out text,
			created_at datetime,
			updated_at datetime
		)
	`).Error; err != nil {
		t.Fatalf("create legacy shift catalog table: %v", err)
	}

	if err := db.Exec(`
		INSERT INTO dingtalk_shift_catalogs (name, shift_key, shift_id, check_in, check_out)
		VALUES (?, ?, ?, ?, ?)
	`, "17:30下班", "", 101, "09:00", "17:30").Error; err != nil {
		t.Fatalf("seed legacy shift catalog row: %v", err)
	}

	previousDB := DB
	DB = db
	defer func() {
		DB = previousDB
	}()

	if err := migrateShiftCatalogSchema(); err != nil {
		t.Fatalf("migrate shift catalog schema: %v", err)
	}

	var catalog DingTalkShiftCatalog
	if err := DB.First(&catalog).Error; err != nil {
		t.Fatalf("load migrated shift catalog: %v", err)
	}

	expectedKey := normalizeShiftCatalogKey("17:30下班", "09:00", "17:30")
	if catalog.ShiftKey != expectedKey {
		t.Fatalf("expected shift_key %q, got %q", expectedKey, catalog.ShiftKey)
	}

	if !DB.Migrator().HasIndex(&DingTalkShiftCatalog{}, "idx_dingtalk_shift_catalogs_shift_key") {
		t.Fatal("expected unique shift_key index to exist after migration")
	}
}
