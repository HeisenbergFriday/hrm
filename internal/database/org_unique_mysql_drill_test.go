//go:build integration && mysql_drill

package database

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"testing"

	gomysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const mysqlDrillDSNEnv = "PEOPLEOPS_MYSQL_DRILL_DSN"

func TestOrgCompositeUniqueMigrationRealMySQLDrill(t *testing.T) {
	dsn := os.Getenv(mysqlDrillDSNEnv)
	if strings.TrimSpace(dsn) == "" {
		t.Skip(mysqlDrillDSNEnv + " is not set")
	}
	cfg, err := gomysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse drill DSN (credentials redacted): %v", err)
	}
	host, port, err := net.SplitHostPort(cfg.Addr)
	if err != nil || cfg.Net != "tcp" || host != "127.0.0.1" || port != "13306" || cfg.DBName != "peopleops_org_drill" {
		t.Fatalf("refusing unsafe drill target: require tcp 127.0.0.1:13306 database peopleops_org_drill")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open approved drill database (credentials redacted): %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get drill SQL connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping approved drill database: %v", err)
	}

	specs := AllOrgCompositeUniqueSpecs()
	if len(specs) == 0 {
		t.Fatal("production org unique manifest is empty")
	}
	tables := mysqlDrillTableColumns(specs)
	if len(tables) == 0 {
		t.Fatal("production org unique manifest has no tables")
	}

	mysqlDrillResetTables(t, db, tables)
	t.Cleanup(func() { mysqlDrillResetTables(t, db, tables) })
	mysqlDrillCreateLegacyTables(t, db, tables)

	covered := make(map[string]bool, len(specs))
	for _, spec := range specs {
		covered[spec.Table+"/"+spec.NewIndex] = true
	}
	if len(covered) != len(specs) {
		t.Fatalf("manifest contains duplicate table/index entries: specs=%d unique=%d", len(specs), len(covered))
	}
	t.Logf("fixture coverage: %d/%d production specs across %d minimal tables; skipped=0", len(covered), len(specs), len(tables))

	// Install one representative production-declared legacy UNIQUE. Its rows remain
	// globally distinct until the production-generated rollback is exercised later.
	rollbackSpec := mysqlDrillFindSpec(t, specs, "roles", "idx_roles_org_name")
	legacyDef := uniqueIndexDefinition{Name: "uni_roles_name", Columns: []string{"name"}, Unique: true}
	if err := db.Exec("ALTER TABLE `roles` ADD UNIQUE INDEX `uni_roles_name` (`name`)").Error; err != nil {
		t.Fatalf("create representative legacy index: %v", err)
	}

	// The full read-only production audit must be executable against every generated
	// fixture table. Executing each statement separately avoids requiring multiStatements.
	mysqlDrillExecuteReadonlyAudit(t, sqlDB, ReadonlyAllOrgUniqueConflictAuditSQL())

	conflictSpec := mysqlDrillFindSpec(t, specs, "users", "idx_org_user_id")
	mysqlDrillInsert(t, db, "users", map[string]interface{}{"org_id": "org-conflict", "user_id": "same-user"})
	mysqlDrillInsert(t, db, "users", map[string]interface{}{"org_id": "org-conflict", "user_id": "same-user"})
	beforeConflictRows := mysqlDrillRowCount(t, db, "users")
	err = MigrateOrgCompositeUniqueIndexes(db)
	if err == nil || !strings.Contains(err.Error(), "same-organization duplicates found") {
		t.Fatalf("intentional same-org conflict did not fail closed: %v", err)
	}
	if got := mysqlDrillRowCount(t, db, "users"); got != beforeConflictRows {
		t.Fatalf("conflict audit auto-remediated rows: before=%d after=%d", beforeConflictRows, got)
	}
	if ok, checkErr := uniqueIndexMatchesDB(db, conflictSpec.Table, conflictSpec.NewIndex, conflictSpec.Columns); checkErr != nil || ok {
		t.Fatalf("conflict must block target index creation: present=%v err=%v", ok, checkErr)
	}
	if err := db.Exec("DELETE FROM `users`").Error; err != nil {
		t.Fatalf("remove intentional fixture conflict: %v", err)
	}

	// Same business key across organizations is legal. Blank/NULL org IDs are
	// intentionally included and must become default. Empty nullable keys must become NULL.
	mysqlDrillInsert(t, db, "users", map[string]interface{}{"org_id": "org-a", "user_id": "cross-org", "email": ""})
	mysqlDrillInsert(t, db, "users", map[string]interface{}{"org_id": "org-b", "user_id": "cross-org", "email": nil})
	mysqlDrillInsert(t, db, "users", map[string]interface{}{"org_id": "", "user_id": "blank-org", "email": "   "})
	mysqlDrillInsert(t, db, "users", map[string]interface{}{"org_id": nil, "user_id": "null-org", "email": nil})
	mysqlDrillInsert(t, db, "roles", map[string]interface{}{"org_id": "", "name": "legacy-blank-org"})
	mysqlDrillInsert(t, db, "roles", map[string]interface{}{"org_id": nil, "name": "legacy-null-org"})

	before := mysqlDrillMetadata(t, db, tables)
	if err := MigrateOrgCompositeUniqueIndexes(db); err != nil {
		t.Fatalf("first production migration: %v", err)
	}
	afterFirst := mysqlDrillMetadata(t, db, tables)
	mysqlDrillRequireAllTargets(t, db, specs)
	if before == afterFirst {
		t.Fatal("metadata did not change on first migration")
	}

	var defaultUsers, nullEmails int64
	if err := db.Raw("SELECT COUNT(*) FROM `users` WHERE `org_id` = 'default'").Scan(&defaultUsers).Error; err != nil {
		t.Fatal(err)
	}
	if defaultUsers != 2 {
		t.Fatalf("blank/NULL org backfill mismatch: got=%d want=2", defaultUsers)
	}
	if err := db.Raw("SELECT COUNT(*) FROM `users` WHERE `email` IS NULL").Scan(&nullEmails).Error; err != nil {
		t.Fatal(err)
	}
	if nullEmails != 4 {
		t.Fatalf("blank/NULL nullable-key normalization mismatch: got=%d want=4", nullEmails)
	}

	if err := MigrateOrgCompositeUniqueIndexes(db); err != nil {
		t.Fatalf("second idempotency migration: %v", err)
	}
	afterSecond := mysqlDrillMetadata(t, db, tables)
	if afterSecond != afterFirst {
		t.Fatalf("second migration changed metadata\nfirst:\n%s\nsecond:\n%s", afterFirst, afterSecond)
	}

	// Real MySQL must enforce the new contract with native error 1062, while the
	// same key remains legal in another organization.
	mysqlDrillInsert(t, db, "roles", map[string]interface{}{"org_id": "org-unique", "name": "per-org-key"})
	duplicateErr := db.Exec("INSERT INTO `roles` (`org_id`,`name`) VALUES (?,?)", "org-unique", "per-org-key").Error
	var mysqlErr *gomysql.MySQLError
	if !errors.As(duplicateErr, &mysqlErr) || mysqlErr.Number != 1062 {
		t.Fatalf("same-org duplicate did not return MySQL 1062: %T", duplicateErr)
	}
	mysqlDrillInsert(t, db, "roles", map[string]interface{}{"org_id": "org-other", "name": "per-org-key"})
	if err := db.Exec("DELETE FROM `roles` WHERE `name` = ?", "per-org-key").Error; err != nil {
		t.Fatalf("remove cross-org enforcement fixture before global legacy rollback: %v", err)
	}

	// Exercise production-generated rollback SQL, verify the legacy definition is
	// restored, then remigrate using the production migration function.
	rollbackSQL := buildOrgUniqueRollbackSQL(rollbackSpec.Table, rollbackSpec.NewIndex, []uniqueIndexDefinition{legacyDef}, true)
	if err := db.Exec(rollbackSQL).Error; err != nil {
		t.Fatalf("execute production-generated rollback SQL: %v", err)
	}
	defs, err := listIndexesDB(db, rollbackSpec.Table)
	if err != nil {
		t.Fatal(err)
	}
	if !indexDefinitionMatches(defs, legacyDef.Name, legacyDef.Columns, true) {
		t.Fatalf("rollback did not restore representative legacy index: %+v", defs)
	}
	if indexDefinitionMatches(defs, rollbackSpec.NewIndex, rollbackSpec.Columns, true) {
		t.Fatal("rollback left target index installed")
	}
	if err := MigrateOrgCompositeUniqueIndexes(db); err != nil {
		t.Fatalf("remigration after rollback: %v", err)
	}
	mysqlDrillRequireAllTargets(t, db, specs)
	t.Logf("drill complete: audit structure, conflict blocking, cross-org keys, blank/NULL normalization, %d target indexes, idempotency, 1062, rollback, and remigration verified", len(specs))
}

func mysqlDrillTableColumns(specs []OrgCompositeUniqueSpec) map[string]map[string]struct{} {
	tables := make(map[string]map[string]struct{})
	for _, spec := range specs {
		cols := tables[spec.Table]
		if cols == nil {
			cols = map[string]struct{}{"id": {}}
			tables[spec.Table] = cols
		}
		for _, col := range spec.Columns {
			cols[col] = struct{}{}
		}
		for _, col := range spec.EmptyNullableCols {
			cols[col] = struct{}{}
		}
		for _, col := range spec.OldSingleCols {
			cols[col] = struct{}{}
		}
	}
	return tables
}

func mysqlDrillResetTables(t *testing.T, db *gorm.DB, tables map[string]map[string]struct{}) {
	t.Helper()
	names := make([]string, 0, len(tables))
	for table := range tables {
		names = append(names, table)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	if err := db.Exec("SET FOREIGN_KEY_CHECKS=0").Error; err != nil {
		t.Fatalf("disable foreign key checks: %v", err)
	}
	for _, table := range names {
		if err := db.Exec("DROP TABLE IF EXISTS " + quoteIdentifier(table)).Error; err != nil {
			t.Fatalf("drop drill table %s: %v", table, err)
		}
	}
	if err := db.Exec("SET FOREIGN_KEY_CHECKS=1").Error; err != nil {
		t.Fatalf("enable foreign key checks: %v", err)
	}
}

func mysqlDrillCreateLegacyTables(t *testing.T, db *gorm.DB, tables map[string]map[string]struct{}) {
	t.Helper()
	names := make([]string, 0, len(tables))
	for table := range tables {
		names = append(names, table)
	}
	sort.Strings(names)
	for _, table := range names {
		columns := make([]string, 0, len(tables[table]))
		for col := range tables[table] {
			if col != "id" {
				columns = append(columns, col)
			}
		}
		sort.Strings(columns)
		defs := []string{"`id` BIGINT UNSIGNED NOT NULL AUTO_INCREMENT"}
		for _, col := range columns {
			defs = append(defs, quoteIdentifier(col)+" VARCHAR(64) NULL")
		}
		defs = append(defs, "PRIMARY KEY (`id`)")
		query := "CREATE TABLE " + quoteIdentifier(table) + " (" + strings.Join(defs, ", ") + ") ENGINE=InnoDB DEFAULT CHARSET=utf8mb4"
		if err := db.Exec(query).Error; err != nil {
			t.Fatalf("create minimal legacy table %s: %v", table, err)
		}
	}
}

func mysqlDrillExecuteReadonlyAudit(t *testing.T, db *sql.DB, audit string) {
	t.Helper()
	for _, raw := range strings.Split(audit, ";") {
		lines := strings.Split(raw, "\n")
		kept := lines[:0]
		for _, line := range lines {
			if !strings.HasPrefix(strings.TrimSpace(line), "--") && strings.TrimSpace(line) != "" {
				kept = append(kept, line)
			}
		}
		statement := strings.TrimSpace(strings.Join(kept, "\n"))
		if statement == "" {
			continue
		}
		if !strings.HasPrefix(strings.ToUpper(statement), "SELECT") {
			t.Fatalf("production read-only audit contains non-SELECT statement")
		}
		rows, err := db.Query(statement)
		if err != nil {
			t.Fatalf("execute production read-only audit statement: %v", err)
		}
		columns, err := rows.Columns()
		if err != nil {
			_ = rows.Close()
			t.Fatal(err)
		}
		values := make([]interface{}, len(columns))
		pointers := make([]interface{}, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		for rows.Next() {
			if err := rows.Scan(pointers...); err != nil {
				_ = rows.Close()
				t.Fatalf("scan production read-only audit result: %v", err)
			}
		}
		if err := rows.Err(); err != nil {
			_ = rows.Close()
			t.Fatalf("iterate production read-only audit result: %v", err)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close production read-only audit result: %v", err)
		}
	}
}

func mysqlDrillInsert(t *testing.T, db *gorm.DB, table string, values map[string]interface{}) {
	t.Helper()
	columns := make([]string, 0, len(values))
	for col := range values {
		columns = append(columns, col)
	}
	sort.Strings(columns)
	quoted := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	args := make([]interface{}, len(columns))
	for i, col := range columns {
		quoted[i] = quoteIdentifier(col)
		placeholders[i] = "?"
		args[i] = values[col]
	}
	query := "INSERT INTO " + quoteIdentifier(table) + " (" + strings.Join(quoted, ",") + ") VALUES (" + strings.Join(placeholders, ",") + ")"
	if err := db.Exec(query, args...).Error; err != nil {
		t.Fatalf("insert drill row into %s: %v", table, err)
	}
}

func mysqlDrillRowCount(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()
	var count int64
	if err := db.Raw("SELECT COUNT(*) FROM " + quoteIdentifier(table)).Scan(&count).Error; err != nil {
		t.Fatalf("count drill rows in %s: %v", table, err)
	}
	return count
}

func mysqlDrillMetadata(t *testing.T, db *gorm.DB, tables map[string]map[string]struct{}) string {
	t.Helper()
	names := make([]string, 0, len(tables))
	for table := range tables {
		names = append(names, table)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, table := range names {
		defs, err := listIndexesDB(db, table)
		if err != nil {
			t.Fatalf("read metadata for %s: %v", table, err)
		}
		for _, def := range defs {
			fmt.Fprintf(&b, "%s|%s|%t|%s\n", table, def.Name, def.Unique, strings.Join(def.Columns, ","))
		}
	}
	return b.String()
}

func mysqlDrillRequireAllTargets(t *testing.T, db *gorm.DB, specs []OrgCompositeUniqueSpec) {
	t.Helper()
	for _, spec := range specs {
		if err := verifyUniqueIndexDB(db, spec); err != nil {
			t.Fatalf("production spec not covered after migration (%s/%s): %v", spec.Table, spec.NewIndex, err)
		}
	}
}

func mysqlDrillFindSpec(t *testing.T, specs []OrgCompositeUniqueSpec, table, index string) OrgCompositeUniqueSpec {
	t.Helper()
	for _, spec := range specs {
		if spec.Table == table && spec.NewIndex == index {
			return spec
		}
	}
	t.Fatalf("required production spec missing: %s/%s", table, index)
	return OrgCompositeUniqueSpec{}
}
