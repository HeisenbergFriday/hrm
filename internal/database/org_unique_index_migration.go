package database

import (
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

// OrgCompositeUniqueSpec describes one multi-tenant composite UNIQUE index that
// must include org_id as the leading column. Phase 4 unifies phases 1-3 under a
// single injectable, idempotent, fail-closed migration path.
//
// Safety rules:
//   - Never auto-delete / auto-merge / auto-pick survivor business rows.
//   - Conflict audit is always GROUP BY org_id (+ business columns).
//   - Same business key across different orgs is legal and must not fail.
//   - Empty org_id is normalized before audit; nullable business-key NULLs follow MySQL UNIQUE semantics.
//   - Re-running is safe when the target unique index already matches.
type OrgCompositeUniqueSpec struct {
	Table         string
	NewIndex      string
	Columns       []string // must start with org_id
	SoftDelete    bool
	OldIndexes    []string // known legacy unique names to drop after audit
	OldSingleCols []string // drop any single-column UNIQUE on these columns
	// EmptyNullableCols lists optional business-key columns that represent "no value"
	// as empty string in application code. Before audit/DDL the migrator:
	//  1) ensures the column is nullable (schema expand only, never drops data);
	//  2) rewrites TRIM(col)='' to NULL so MySQL UNIQUE multi-NULL semantics apply.
	// This is value normalization, not delete/merge of business rows.
	// Non-empty values are never rewritten. NULL rows are then excluded from conflict
	// grouping to match the target index semantics.
	EmptyNullableCols []string
	// AllowDefaultOrgBackfill: when true, NULL/'' org_id is set to 'default' so the
	// composite unique contract can be applied. This is the only allowed default-org
	// write path for unique-index migration; historical multi-tenant discover/apply
	// tooling must still refuse silent default backfill of unresolved rows.
	AllowDefaultOrgBackfill bool
	// SkipIfMissingTable: when true (default), missing table is not an error.
	SkipIfMissingTable bool
	// SampleIDColumn: primary-ish id column for conflict samples (default "id").
	SampleIDColumn string
}

// AllOrgCompositeUniqueSpecs returns the full phase-4 index migration matrix
// (phase-1 core + phase-2 leave/schedule + phase-3 lifecycle/binding + approvals).
func AllOrgCompositeUniqueSpecs() []OrgCompositeUniqueSpec {
	specs := make([]OrgCompositeUniqueSpec, 0, 40)
	specs = append(specs, phase1CoreOrgCompositeUniqueSpecs()...)
	specs = append(specs, leaveScheduleSpecsAsOrgSpecs()...)
	specs = append(specs, lifecycleSpecsAsOrgSpecs()...)
	specs = append(specs, phase4SupplementalOrgCompositeUniqueSpecs()...)
	return specs
}

func phase1CoreOrgCompositeUniqueSpecs() []OrgCompositeUniqueSpec {
	return []OrgCompositeUniqueSpec{
		{
			Table:                   "organization_users",
			NewIndex:                "idx_org_user",
			Columns:                 []string{"org_id", "user_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_organization_users_user_id", "idx_organization_users_user_id", "user_id"},
			OldSingleCols:           []string{"user_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			// Core tenant identity: (org_id, user_id). Legacy global user_id unique must drop.
			Table:                   "users",
			NewIndex:                "idx_org_user_id",
			Columns:                 []string{"org_id", "user_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_users_user_id", "user_id", "idx_users_user_id"},
			OldSingleCols:           []string{"user_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "users",
			NewIndex:                "idx_org_email",
			Columns:                 []string{"org_id", "email"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_users_email", "email"},
			OldSingleCols:           []string{"email"},
			EmptyNullableCols:       []string{"email"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "departments",
			NewIndex:                "idx_org_dept_id",
			Columns:                 []string{"org_id", "department_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_departments_department_id", "department_id", "idx_departments_department_id"},
			OldSingleCols:           []string{"department_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "employee_profiles",
			NewIndex:                "idx_employee_profiles_org_user",
			Columns:                 []string{"org_id", "user_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_employee_profiles_user_id", "user_id"},
			OldSingleCols:           []string{"user_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "employee_profiles",
			NewIndex:                "idx_employee_profiles_org_employee",
			Columns:                 []string{"org_id", "employee_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_employee_profiles_employee_id", "employee_id"},
			OldSingleCols:           []string{"employee_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "attendances",
			NewIndex:                "idx_org_user_time_type",
			Columns:                 []string{"org_id", "user_id", "check_time", "check_type"},
			SoftDelete:              true,
			OldIndexes:              []string{"idx_user_time_type", "uni_attendances_user_time_type"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			// Model tag + phase-4 matrix use idx_org_sync_type.
			// Older migrateSyncStatusOrganizationScope used idx_sync_statuses_org_type.
			Table:                   "sync_statuses",
			NewIndex:                "idx_org_sync_type",
			Columns:                 []string{"org_id", "type"},
			SoftDelete:              false,
			OldIndexes:              []string{"uni_sync_statuses_type", "idx_sync_statuses_type", "idx_sync_statuses_org_type", "type"},
			OldSingleCols:           []string{"type"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "approvals",
			NewIndex:                "idx_approvals_org_process",
			Columns:                 []string{"org_id", "process_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_approvals_process_id", "process_id", "idx_approvals_process_id"},
			OldSingleCols:           []string{"process_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "approval_templates",
			NewIndex:                "idx_approval_templates_org_template",
			Columns:                 []string{"org_id", "template_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_approval_templates_template_id", "template_id", "idx_approval_templates_template_id"},
			OldSingleCols:           []string{"template_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "dingtalk_event_logs",
			NewIndex:                "idx_dingtalk_event_org_event",
			Columns:                 []string{"org_id", "event_id"},
			SoftDelete:              false,
			OldIndexes:              []string{"uni_dingtalk_event_logs_event_id", "event_id"},
			OldSingleCols:           []string{"event_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "roles",
			NewIndex:                "idx_roles_org_name",
			Columns:                 []string{"org_id", "name"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_roles_name", "name"},
			OldSingleCols:           []string{"name"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "user_roles",
			NewIndex:                "idx_user_roles_org_user",
			Columns:                 []string{"org_id", "user_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_user_roles_user_id", "user_id"},
			OldSingleCols:           []string{"user_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "menu_permissions",
			NewIndex:                "idx_menu_permissions_org_role",
			Columns:                 []string{"org_id", "role_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_menu_permissions_role_id", "role_id"},
			OldSingleCols:           []string{"role_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
		{
			Table:                   "data_permissions",
			NewIndex:                "idx_data_permissions_org_role",
			Columns:                 []string{"org_id", "role_id"},
			SoftDelete:              true,
			OldIndexes:              []string{"uni_data_permissions_role_id", "role_id"},
			OldSingleCols:           []string{"role_id"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
	}
}

func leaveScheduleSpecsAsOrgSpecs() []OrgCompositeUniqueSpec {
	out := make([]OrgCompositeUniqueSpec, 0, len(leaveScheduleBusinessUniqueSpecs()))
	for _, s := range leaveScheduleBusinessUniqueSpecs() {
		oldSingles := []string{}
		if s.OldSingleCol != "" {
			oldSingles = []string{s.OldSingleCol}
		}
		out = append(out, OrgCompositeUniqueSpec{
			Table:                   s.Table,
			NewIndex:                s.NewIndex,
			Columns:                 append([]string(nil), s.Columns...),
			SoftDelete:              s.SoftDelete,
			OldIndexes:              append([]string(nil), s.OldIndexes...),
			OldSingleCols:           oldSingles,
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		})
	}
	return out
}

func lifecycleSpecsAsOrgSpecs() []OrgCompositeUniqueSpec {
	out := make([]OrgCompositeUniqueSpec, 0, len(lifecycleBindingBusinessUniqueSpecs()))
	for _, s := range lifecycleBindingBusinessUniqueSpecs() {
		oldSingles := []string{}
		if s.OldSingleCol != "" {
			oldSingles = []string{s.OldSingleCol}
		}
		out = append(out, OrgCompositeUniqueSpec{
			Table:                   s.Table,
			NewIndex:                s.NewIndex,
			Columns:                 append([]string(nil), s.Columns...),
			SoftDelete:              s.SoftDelete,
			OldIndexes:              append([]string(nil), s.OldIndexes...),
			OldSingleCols:           oldSingles,
			EmptyNullableCols:       append([]string(nil), s.EmptyNullableCols...),
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		})
	}
	return out
}

// phase4SupplementalOrgCompositeUniqueSpecs covers org-scoped unique contracts
// that were not part of the phase 1-3 migration lists.
func phase4SupplementalOrgCompositeUniqueSpecs() []OrgCompositeUniqueSpec {
	return []OrgCompositeUniqueSpec{
		{
			Table: "users", NewIndex: "idx_users_org_dingtalk_user",
			Columns:       []string{"org_id", "ding_talk_user_id"},
			OldIndexes:    []string{"uni_users_ding_talk_user_id", "idx_users_ding_talk_user_id"},
			OldSingleCols: []string{"ding_talk_user_id"}, EmptyNullableCols: []string{"ding_talk_user_id"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "departments", NewIndex: "idx_departments_org_dingtalk_department",
			Columns:       []string{"org_id", "dingtalk_department_id"},
			OldIndexes:    []string{"uni_departments_dingtalk_department_id", "idx_departments_dingtalk_department_id"},
			OldSingleCols: []string{"dingtalk_department_id"}, EmptyNullableCols: []string{"dingtalk_department_id"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "performance_reminder_logs", NewIndex: "idx_perf_reminder_org_round",
			Columns:                 []string{"org_id", "activity_id", "participant_id", "stage", "reminder_key", "reminder_date"},
			OldIndexes:              []string{"idx_perf_reminder_round"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "external_attendance_raw", NewIndex: "uk_ext_att_org_row",
			Columns:       []string{"org_id", "source_row_key"},
			OldIndexes:    []string{"uk_ext_att_row", "uni_external_attendance_raw_source_row_key"},
			OldSingleCols: []string{"source_row_key"}, AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "external_attendance_approve_links", NewIndex: "uk_ext_appr_item",
			Columns:                 []string{"org_id", "source_row_key", "item_key"},
			OldIndexes:              []string{"uk_ext_appr_link", "uk_ext_appr_item_legacy"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "external_user_department_raw", NewIndex: "uk_ext_dept_org_row",
			Columns:    []string{"org_id", "source_row_key"},
			OldIndexes: []string{"uk_ext_dept_row"}, OldSingleCols: []string{"source_row_key"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "user_department_relations", NewIndex: "uk_user_dept_rel",
			Columns:                 []string{"org_id", "user_id", "department_id"},
			OldIndexes:              []string{"uk_user_dept_rel_legacy"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "external_sync_cursors", NewIndex: "uk_ext_sync_cursor",
			Columns:    []string{"org_id", "source_table"},
			OldIndexes: []string{"uk_ext_sync_cursor_source"}, OldSingleCols: []string{"source_table"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table: "external_sync_locks", NewIndex: "uk_ext_sync_lock",
			Columns:    []string{"org_id", "scope_key"},
			OldIndexes: []string{"uk_ext_sync_lock_scope"}, OldSingleCols: []string{"scope_key"},
			AllowDefaultOrgBackfill: true, SkipIfMissingTable: true,
		},
		{
			Table:                   "performance_import_batches",
			NewIndex:                "uk_performance_import_batch_org_key",
			Columns:                 []string{"org_id", "batch_key"},
			SoftDelete:              true,
			OldIndexes:              []string{"uk_performance_import_batch_key", "uni_performance_import_batches_batch_key", "batch_key"},
			OldSingleCols:           []string{"batch_key"},
			AllowDefaultOrgBackfill: true,
			SkipIfMissingTable:      true,
		},
	}
}

type uniqueIndexDefinition struct {
	Name    string
	Columns []string
	Unique  bool
}

type orgUniqueMigrationOptions struct {
	addMissingOrgColumn bool
	skipMissingColumns  bool
}

// PrepareOrgCompositeUniqueIndexes runs before AutoMigrate. Existing tables are
// upgraded and audited first, so GORM cannot create a new composite unique before
// conflict details are available. Missing tables are left to AutoMigrate; missing
// business columns fail closed instead of allowing an unaudited index creation.
func PrepareOrgCompositeUniqueIndexes(db *gorm.DB) error {
	return migrateOrgCompositeUniqueIndexes(db, orgUniqueMigrationOptions{
		addMissingOrgColumn: true,
		skipMissingColumns:  false,
	})
}

// MigrateOrgCompositeUniqueIndexes is the injectable post-AutoMigrate contract
// verifier. It is safe to execute repeatedly.
func MigrateOrgCompositeUniqueIndexes(db *gorm.DB) error {
	return migrateOrgCompositeUniqueIndexes(db, orgUniqueMigrationOptions{})
}

func migrateOrgCompositeUniqueIndexes(db *gorm.DB, opts orgUniqueMigrationOptions) error {
	if db == nil {
		return nil
	}
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		if err := migrateOneOrgCompositeUniqueWithOptions(db, spec, opts); err != nil {
			return err
		}
	}
	return nil
}

func migrateOneOrgCompositeUnique(db *gorm.DB, spec OrgCompositeUniqueSpec) error {
	return migrateOneOrgCompositeUniqueWithOptions(db, spec, orgUniqueMigrationOptions{})
}

// migrateOneOrgCompositeUniqueWithOptions performs, in order: schema checks,
// org_id backfill, same-org conflict audit, atomic legacy-index replacement, and
// exact post-DDL verification. It never deletes, merges, or chooses business rows.
func migrateOneOrgCompositeUniqueWithOptions(db *gorm.DB, spec OrgCompositeUniqueSpec, opts orgUniqueMigrationOptions) error {
	if err := validateOrgCompositeUniqueSpec(spec); err != nil {
		return err
	}
	exists, err := tableExistsDB(db, spec.Table)
	if err != nil {
		return fmt.Errorf("check table %s: %w", spec.Table, err)
	}
	if !exists {
		if spec.SkipIfMissingTable {
			return nil
		}
		return fmt.Errorf("table %s does not exist", spec.Table)
	}

	orgExists, err := columnExistsDB(db, spec.Table, "org_id")
	if err != nil {
		return err
	}
	if !orgExists && opts.addMissingOrgColumn {
		if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN `org_id` varchar(64) NULL", quoteIdentifier(spec.Table))).Error; err != nil {
			return fmt.Errorf("%s add org_id column before unique migration: %w", spec.Table, err)
		}
		orgExists = true
	}
	if !orgExists {
		return fmt.Errorf("%s missing required column org_id for unique index %s", spec.Table, spec.NewIndex)
	}
	for _, col := range spec.Columns[1:] {
		ok, checkErr := columnExistsDB(db, spec.Table, col)
		if checkErr != nil {
			return checkErr
		}
		if !ok {
			if opts.skipMissingColumns {
				return nil
			}
			return fmt.Errorf("%s missing required business column %s for unique index %s; refusing unaudited AutoMigrate", spec.Table, col, spec.NewIndex)
		}
	}

	if spec.AllowDefaultOrgBackfill {
		if err := db.Exec(fmt.Sprintf(
			"UPDATE %s SET `org_id` = 'default' WHERE `org_id` IS NULL OR TRIM(`org_id`) = ''",
			quoteIdentifier(spec.Table),
		)).Error; err != nil {
			return fmt.Errorf("%s backfill empty org_id: %w", spec.Table, err)
		}
	}

	// Optional business keys only: empty string → NULL (after ensuring nullable).
	// Non-empty business values and all non-listed columns are never rewritten.
	if err := normalizeEmptyNullableBusinessKeys(db, spec); err != nil {
		return err
	}

	// Same-org non-NULL business-key collisions stop migration with samples.
	// Never auto-delete / auto-merge / auto-pick survivors.
	if err := AuditOrgCompositeUniqueConflicts(db, spec); err != nil {
		return err
	}

	definitions, err := listIndexesDB(db, spec.Table)
	if err != nil {
		return fmt.Errorf("list indexes for %s: %w", spec.Table, err)
	}
	ready := indexDefinitionMatches(definitions, spec.NewIndex, spec.Columns, true)
	drops := legacyIndexesForSpec(definitions, spec, ready)
	if ready && len(drops) == 0 {
		return verifyUniqueIndexDB(db, spec)
	}

	rollbackSQL := buildOrgUniqueRollbackSQL(spec.Table, spec.NewIndex, drops, !ready)
	alterSQL := buildOrgUniqueAlterSQL(spec, drops, !ready)
	if alterSQL != "" {
		if err := db.Exec(alterSQL).Error; err != nil {
			return fmt.Errorf("replace unique index %s on table %s atomically: %w; rollback SQL (only if DDL partially applied): %s",
				spec.NewIndex, spec.Table, err, rollbackSQL)
		}
	}
	if err := verifyUniqueIndexDB(db, spec); err != nil {
		return fmt.Errorf("%w; rollback SQL: %s", err, rollbackSQL)
	}
	return nil
}

func validateOrgCompositeUniqueSpec(spec OrgCompositeUniqueSpec) error {
	if strings.TrimSpace(spec.Table) == "" || strings.TrimSpace(spec.NewIndex) == "" {
		return fmt.Errorf("invalid org unique spec: empty table/index")
	}
	if len(spec.Columns) < 2 || !strings.EqualFold(spec.Columns[0], "org_id") {
		return fmt.Errorf("spec %s/%s must be composite starting with org_id, got %v", spec.Table, spec.NewIndex, spec.Columns)
	}
	return nil
}

func listIndexesDB(db *gorm.DB, table string) ([]uniqueIndexDefinition, error) {
	type row struct {
		IndexName  string
		ColumnName string
		NonUnique  int64
		Seq        int64
	}
	var rows []row
	if err := db.Raw(`
		SELECT INDEX_NAME AS index_name, COLUMN_NAME AS column_name,
		       NON_UNIQUE AS non_unique, SEQ_IN_INDEX AS seq
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
		ORDER BY INDEX_NAME, SEQ_IN_INDEX
	`, table).Scan(&rows).Error; err != nil {
		return nil, err
	}
	byName := make(map[string]*uniqueIndexDefinition)
	order := make([]string, 0)
	for _, r := range rows {
		name := strings.TrimSpace(r.IndexName)
		if name == "" {
			continue
		}
		def := byName[name]
		if def == nil {
			def = &uniqueIndexDefinition{Name: name, Unique: r.NonUnique == 0}
			byName[name] = def
			order = append(order, name)
		}
		def.Columns = append(def.Columns, strings.TrimSpace(r.ColumnName))
	}
	sort.Strings(order)
	out := make([]uniqueIndexDefinition, 0, len(order))
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out, nil
}

func indexDefinitionMatches(defs []uniqueIndexDefinition, name string, columns []string, unique bool) bool {
	for _, def := range defs {
		if strings.EqualFold(def.Name, name) && def.Unique == unique && equalColumns(def.Columns, columns) {
			return true
		}
	}
	return false
}

func equalColumns(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(strings.TrimSpace(a[i]), strings.TrimSpace(b[i])) {
			return false
		}
	}
	return true
}

func legacyIndexesForSpec(defs []uniqueIndexDefinition, spec OrgCompositeUniqueSpec, targetReady bool) []uniqueIndexDefinition {
	known := make(map[string]struct{}, len(spec.OldIndexes))
	for _, name := range spec.OldIndexes {
		known[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}
	singles := make(map[string]struct{}, len(spec.OldSingleCols))
	for _, col := range spec.OldSingleCols {
		singles[strings.ToLower(strings.TrimSpace(col))] = struct{}{}
	}
	businessColumns := spec.Columns[1:]
	seen := make(map[string]struct{})
	out := make([]uniqueIndexDefinition, 0)
	for _, def := range defs {
		lowerName := strings.ToLower(def.Name)
		if strings.EqualFold(def.Name, "PRIMARY") {
			continue
		}
		shouldDrop := false
		if strings.EqualFold(def.Name, spec.NewIndex) {
			shouldDrop = !targetReady
		} else if def.Unique {
			_, namedLegacy := known[lowerName]
			_, singleLegacy := singles[strings.ToLower(firstColumn(def.Columns))]
			shouldDrop = namedLegacy || (len(def.Columns) == 1 && singleLegacy) || equalColumns(def.Columns, businessColumns)
		}
		if shouldDrop {
			if _, ok := seen[lowerName]; !ok {
				seen[lowerName] = struct{}{}
				out = append(out, def)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func firstColumn(columns []string) string {
	if len(columns) == 0 {
		return ""
	}
	return strings.TrimSpace(columns[0])
}

func buildOrgUniqueAlterSQL(spec OrgCompositeUniqueSpec, drops []uniqueIndexDefinition, addTarget bool) string {
	clauses := make([]string, 0, len(drops)+1)
	for _, def := range drops {
		clauses = append(clauses, "DROP INDEX "+quoteIdentifier(def.Name))
	}
	if addTarget {
		clauses = append(clauses, "ADD UNIQUE INDEX "+quoteIdentifier(spec.NewIndex)+" ("+quotedColumns(spec.Columns)+")")
	}
	if len(clauses) == 0 {
		return ""
	}
	return "ALTER TABLE " + quoteIdentifier(spec.Table) + " " + strings.Join(clauses, ", ")
}

func buildOrgUniqueRollbackSQL(table, target string, dropped []uniqueIndexDefinition, targetAdded bool) string {
	clauses := make([]string, 0, len(dropped)+1)
	if targetAdded {
		clauses = append(clauses, "DROP INDEX "+quoteIdentifier(target))
	}
	for _, def := range dropped {
		kind := "INDEX "
		if def.Unique {
			kind = "UNIQUE INDEX "
		}
		clauses = append(clauses, "ADD "+kind+quoteIdentifier(def.Name)+" ("+quotedColumns(def.Columns)+")")
	}
	if len(clauses) == 0 {
		return "-- no index rollback required"
	}
	return "ALTER TABLE " + quoteIdentifier(table) + " " + strings.Join(clauses, ", ") + ";"
}

func quotedColumns(columns []string) string {
	parts := make([]string, 0, len(columns))
	for _, col := range columns {
		parts = append(parts, quoteIdentifier(col))
	}
	return strings.Join(parts, ", ")
}

// AuditOrgCompositeUniqueConflicts audits exactly the rows covered by the
// target MySQL index. Soft-deleted rows are included unless deleted_at itself is
// part of the index. NULL business-key columns are excluded because MySQL UNIQUE
// permits multiple NULL values. Empty strings remain business values.
func AuditOrgCompositeUniqueConflicts(db *gorm.DB, spec OrgCompositeUniqueSpec) error {
	if db == nil {
		return nil
	}
	if err := validateOrgCompositeUniqueSpec(spec); err != nil {
		return err
	}
	idCol := spec.SampleIDColumn
	if idCol == "" {
		idCol = "id"
	}
	whereParts := nullableBusinessWhere(spec)
	where := ""
	if len(whereParts) > 0 {
		where = " WHERE " + strings.Join(whereParts, " AND ")
	}
	groupBy := quotedColumns(spec.Columns)
	hasID, err := columnExistsDB(db, spec.Table, idCol)
	if err != nil {
		return err
	}
	selectTail := "COUNT(*) AS cnt"
	if hasID {
		selectTail += ", SUBSTRING_INDEX(GROUP_CONCAT(" + quoteIdentifier(idCol) + " ORDER BY " + quoteIdentifier(idCol) + " SEPARATOR ','), ',', 5) AS sample_ids"
	}
	query := fmt.Sprintf("SELECT %s, %s FROM %s%s GROUP BY %s HAVING COUNT(*) > 1 LIMIT 20",
		groupBy, selectTail, quoteIdentifier(spec.Table), where, groupBy)
	rows, err := db.Raw(query).Rows()
	if err != nil {
		return fmt.Errorf("%s conflict audit query: %w", spec.Table, err)
	}
	defer func() { _ = rows.Close() }()

	conflicts := make([]string, 0, 20)
	for rows.Next() {
		n := len(spec.Columns) + 1
		if hasID {
			n++
		}
		values := make([]interface{}, n)
		ptrs := make([]interface{}, n)
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return fmt.Errorf("%s conflict audit scan: %w", spec.Table, err)
		}
		business := make([]string, 0, len(spec.Columns)-1)
		for i, col := range spec.Columns[1:] {
			business = append(business, fmt.Sprintf("%s=%s", col, stringifySQLValue(values[i+1])))
		}
		entry := fmt.Sprintf("table=%s org_id=%s business_key={%s} duplicate_count=%s",
			spec.Table, stringifySQLValue(values[0]), strings.Join(business, ","), stringifySQLValue(values[len(spec.Columns)]))
		if hasID {
			entry += " sample_ids=" + stringifySQLValue(values[len(spec.Columns)+1])
		}
		conflicts = append(conflicts, entry)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf("cannot create unique index %s: same-organization duplicates found; no rows were deleted or merged; %s; resolve manually and rerun",
		spec.NewIndex, strings.Join(conflicts, " | "))
}

func nullableBusinessWhere(spec OrgCompositeUniqueSpec) []string {
	out := make([]string, 0, len(spec.EmptyNullableCols))
	for _, col := range spec.EmptyNullableCols {
		out = append(out, quoteIdentifier(col)+" IS NOT NULL")
	}
	return out
}

// ReadonlyOrgUniqueConflictAuditSQL returns only SELECT statements. The
// conflict grouping uses the post-backfill org_id value, so blank/default pairs
// are detected before production migration.
func ReadonlyOrgUniqueConflictAuditSQL(spec OrgCompositeUniqueSpec) string {
	idCol := spec.SampleIDColumn
	if idCol == "" {
		idCol = "id"
	}
	normalizedOrg := "COALESCE(NULLIF(TRIM(`org_id`), ''), 'default')"
	business := make([]string, 0, len(spec.Columns)-1)
	for _, col := range spec.Columns[1:] {
		business = append(business, quoteIdentifier(col))
	}
	groupExpressions := append([]string{normalizedOrg}, business...)
	where := ""
	if parts := nullableBusinessWhere(spec); len(parts) > 0 {
		where = " WHERE " + strings.Join(parts, " AND ")
	}
	return fmt.Sprintf(`-- table=%s target_index=%s
SELECT TABLE_NAME, COLUMN_NAME
FROM information_schema.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '%s'
  AND COLUMN_NAME IN (%s)
ORDER BY ORDINAL_POSITION;
SELECT INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME
FROM information_schema.STATISTICS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '%s'
ORDER BY INDEX_NAME, SEQ_IN_INDEX;
SELECT COUNT(*) AS empty_org_count,
       SUBSTRING_INDEX(GROUP_CONCAT(%s ORDER BY %s SEPARATOR ','), ',', 5) AS sample_ids
FROM %s
WHERE org_id IS NULL OR TRIM(org_id) = '';
SELECT %s AS migrated_org_id, %s, COUNT(*) AS duplicate_count,
       SUBSTRING_INDEX(GROUP_CONCAT(%s ORDER BY %s SEPARATOR ','), ',', 5) AS sample_ids
FROM %s%s
GROUP BY %s
HAVING COUNT(*) > 1
LIMIT 50;`,
		spec.Table, spec.NewIndex, spec.Table, quotedSQLStringList(spec.Columns), spec.Table,
		quoteIdentifier(idCol), quoteIdentifier(idCol), quoteIdentifier(spec.Table),
		normalizedOrg, strings.Join(business, ", "), quoteIdentifier(idCol), quoteIdentifier(idCol),
		quoteIdentifier(spec.Table), where, strings.Join(groupExpressions, ", "))
}

func quotedSQLStringList(values []string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, "'"+strings.ReplaceAll(value, "'", "''")+"'")
	}
	return strings.Join(parts, ", ")
}

func ReadonlyAllOrgUniqueConflictAuditSQL() string {
	var b strings.Builder
	b.WriteString("-- PeopleOps phase-4 org composite unique READ-ONLY audit\n")
	b.WriteString("-- Run with a read-only account. Same key in different organizations is legal.\n")
	b.WriteString("-- Blank org_id is grouped as default to model the migration result.\n")
	b.WriteString("-- Do not delete or merge rows automatically.\n\n")
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		b.WriteString(ReadonlyOrgUniqueConflictAuditSQL(spec))
		b.WriteString("\n\n")
	}
	return b.String()
}

func tableExistsDB(db *gorm.DB, table string) (bool, error) {
	var count int64
	err := db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", table).Scan(&count).Error
	return count > 0, err
}

func columnExistsDB(db *gorm.DB, table, column string) (bool, error) {
	var count int64
	err := db.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", table, column).Scan(&count).Error
	return count > 0, err
}

func uniqueIndexMatchesDB(db *gorm.DB, table, index string, wantCols []string) (bool, error) {
	defs, err := listIndexesDB(db, table)
	if err != nil {
		return false, err
	}
	return indexDefinitionMatches(defs, index, wantCols, true), nil
}

func verifyUniqueIndexDB(db *gorm.DB, spec OrgCompositeUniqueSpec) error {
	ok, err := uniqueIndexMatchesDB(db, spec.Table, spec.NewIndex, spec.Columns)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("post-migrate verify failed: unique index %s on %s with columns %v is absent, non-unique, or in the wrong order", spec.NewIndex, spec.Table, spec.Columns)
	}
	return nil
}

func normalizeEmptyNullableBusinessKeys(db *gorm.DB, spec OrgCompositeUniqueSpec) error {
	if db == nil || len(spec.EmptyNullableCols) == 0 {
		return nil
	}
	for _, col := range spec.EmptyNullableCols {
		col = strings.TrimSpace(col)
		if col == "" {
			continue
		}
		exists, err := columnExistsDB(db, spec.Table, col)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("%s missing EmptyNullableCols column %s for unique index %s", spec.Table, col, spec.NewIndex)
		}
		if err := ensureColumnNullableDB(db, spec.Table, col); err != nil {
			return err
		}
		sqlText := fmt.Sprintf(
			"UPDATE %s SET %s = NULL WHERE %s IS NOT NULL AND TRIM(%s) = ''",
			quoteIdentifier(spec.Table), quoteIdentifier(col), quoteIdentifier(col), quoteIdentifier(col),
		)
		if err := db.Exec(sqlText).Error; err != nil {
			return fmt.Errorf("%s normalize empty %s to NULL: %w", spec.Table, col, err)
		}
	}
	return nil
}

func ensureColumnNullableDB(db *gorm.DB, table, column string) error {
	type colMeta struct {
		IsNullable string
		ColumnType string
	}
	var meta colMeta
	if err := db.Raw(`
		SELECT IS_NULLABLE AS is_nullable, COLUMN_TYPE AS column_type
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?
	`, table, column).Scan(&meta).Error; err != nil {
		return fmt.Errorf("%s.%s nullability lookup: %w", table, column, err)
	}
	if strings.EqualFold(strings.TrimSpace(meta.IsNullable), "YES") {
		return nil
	}
	colType := strings.TrimSpace(meta.ColumnType)
	if colType == "" {
		return fmt.Errorf("%s.%s missing COLUMN_TYPE for nullability expand", table, column)
	}
	// Schema expand only: keep original type, allow NULL. Never drop or rewrite non-empty values.
	sqlText := fmt.Sprintf("ALTER TABLE %s MODIFY COLUMN %s %s NULL",
		quoteIdentifier(table), quoteIdentifier(column), colType)
	if err := db.Exec(sqlText).Error; err != nil {
		return fmt.Errorf("%s.%s ensure nullable: %w", table, column, err)
	}
	return nil
}

func stringifySQLValue(value interface{}) string {
	if value == nil {
		return "<nil>"
	}
	switch v := value.(type) {
	case []byte:
		return string(v)
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}

// OrgCompositeUniqueIndexMatrixMarkdown returns a human-readable migration matrix
// for ops review (table, target index, columns, known legacy indexes).
func OrgCompositeUniqueIndexMatrixMarkdown() string {
	var b strings.Builder
	b.WriteString("| Table | New unique index | Columns | Old indexes / single cols | Empty->NULL cols |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, spec := range AllOrgCompositeUniqueSpecs() {
		old := append([]string(nil), spec.OldIndexes...)
		for _, col := range spec.OldSingleCols {
			old = append(old, "single:"+col)
		}
		if len(old) == 0 {
			old = []string{"-"}
		}
		empty := "-"
		if len(spec.EmptyNullableCols) > 0 {
			empty = strings.Join(spec.EmptyNullableCols, ", ")
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			spec.Table, spec.NewIndex, strings.Join(spec.Columns, ", "),
			strings.Join(old, ", "), empty)
	}
	return b.String()
}
