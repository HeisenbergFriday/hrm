// migrate_multitenant is the staged multi-tenant migration CLI.
//
// Safety rules:
//   - discover/report/infer/verify/contract without confirm are read-only.
//   - apply only writes entries explicitly marked ready in a reviewed manifest.
//   - no command ever falls back to writing a default org_id.
package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"peopleops/internal/tenant/registry"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const usage = `migrate_multitenant - staged multi-tenant migration tool

Usage:
  migrate_multitenant <subcommand> [flags]

Subcommands:
  discover   Read-only schema scan, written as JSON.
  report     Same as discover, optionally writes JSON to -out.
  infer      Read-only org_id attribution manifest for rows whose org_id is empty.
  apply      Writes reviewed ready entries from an infer manifest; requires --confirm-apply.
  verify     Checks tenant tables for missing org_id, empty org_id, and parent org mismatches.
  contract   Plans/enforces final NOT NULL and index contract; writes only with --confirm-contract.

Environment:
  DATABASE_DSN   MySQL DSN used when --dsn is omitted.
`

const (
	entryReady      = "ready"
	entryUnresolved = "unresolved"
	entrySkipped    = "skipped"
)

var identifierRE = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch os.Args[1] {
	case "-h", "--help", "help":
		fmt.Fprint(os.Stdout, usage)
		return
	case "discover":
		err = runDiscover(os.Args[2:], os.Stdout)
	case "report":
		err = runReport(os.Args[2:], os.Stdout)
	case "infer":
		err = runInfer(os.Args[2:], os.Stdout)
	case "apply":
		err = runApply(os.Args[2:], os.Stdout)
	case "verify":
		err = runVerify(os.Args[2:], os.Stdout)
	case "contract":
		err = runContract(os.Args[2:], os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s failed: %v\n", os.Args[1], err)
		os.Exit(1)
	}
}

// ============================== discover/report ==============================

type TableFinding struct {
	Name          string   `json:"name"`
	Kind          string   `json:"kind"`
	Priority      int      `json:"priority,omitempty"`
	Exists        bool     `json:"exists"`
	HasOrgIDCol   bool     `json:"has_org_id_column"`
	OrgIDNullable bool     `json:"org_id_nullable"`
	RowCount      int64    `json:"row_count"`
	NullOrgRows   int64    `json:"null_org_rows"`
	OrgIDIndexes  []string `json:"org_id_indexes,omitempty"`
	Notes         string   `json:"notes,omitempty"`
	ParentTable   string   `json:"parent_table,omitempty"`
}

type DiscoverReport struct {
	GeneratedAt time.Time      `json:"generated_at"`
	Database    string         `json:"database"`
	Tables      []TableFinding `json:"tables"`
	Summary     ReportSummary  `json:"summary"`
}

type ReportSummary struct {
	TotalTenantTables       int   `json:"total_tenant_tables"`
	TenantTablesWithOrgCol  int   `json:"tenant_tables_with_org_col"`
	TenantTablesMissingCol  int   `json:"tenant_tables_missing_col"`
	TotalNullOrgRows        int64 `json:"total_null_org_rows"`
	MembershipTables        int   `json:"membership_tables"`
	PlatformTables          int   `json:"platform_tables"`
	MissingTablesInDatabase int   `json:"missing_tables_in_database"`
}

func runDiscover(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_DSN"), "MySQL DSN (default $DATABASE_DSN)")
	pretty := fs.Bool("pretty", true, "pretty-print JSON output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	db, err := openFromDSN(*dsn)
	if err != nil {
		return err
	}
	defer closeGormDB(db)
	report, err := Discover(db)
	if err != nil {
		return err
	}
	return writeJSON(out, report, *pretty)
}

func runReport(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("report", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_DSN"), "MySQL DSN (default $DATABASE_DSN)")
	outPath := fs.String("out", "", "write report to file (default stdout)")
	pretty := fs.Bool("pretty", true, "pretty-print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	db, err := openFromDSN(*dsn)
	if err != nil {
		return err
	}
	defer closeGormDB(db)
	report, err := Discover(db)
	if err != nil {
		return err
	}
	return writeJSONToPathOrWriter(stdout, *outPath, report, *pretty)
}

func Discover(db *gorm.DB) (DiscoverReport, error) {
	report := DiscoverReport{GeneratedAt: time.Now().UTC()}
	if name, err := currentDatabaseName(db); err == nil {
		report.Database = name
	}

	all := registry.All()
	report.Tables = make([]TableFinding, 0, len(all))
	summary := ReportSummary{}

	for _, tbl := range all {
		finding := TableFinding{
			Name:        tbl.Name,
			Kind:        kindLabel(tbl.Kind),
			Priority:    int(tbl.Priority),
			ParentTable: tbl.ParentTable,
			Notes:       tbl.Notes,
		}
		switch tbl.Kind {
		case registry.KindTenant:
			summary.TotalTenantTables++
		case registry.KindMembership:
			summary.MembershipTables++
		case registry.KindPlatform:
			summary.PlatformTables++
		}

		exists, err := tableExists(db, tbl.Name)
		if err != nil {
			return DiscoverReport{}, fmt.Errorf("check %s existence: %w", tbl.Name, err)
		}
		finding.Exists = exists
		if !exists {
			if tbl.Kind == registry.KindTenant {
				summary.MissingTablesInDatabase++
				summary.TenantTablesMissingCol++
			}
			report.Tables = append(report.Tables, finding)
			continue
		}

		if tbl.Kind != registry.KindPlatform {
			col, nullable, colErr := describeOrgIDColumn(db, tbl.Name)
			if colErr != nil {
				return DiscoverReport{}, fmt.Errorf("inspect %s.org_id: %w", tbl.Name, colErr)
			}
			finding.HasOrgIDCol = col
			finding.OrgIDNullable = nullable
			indexes, idxErr := orgIDIndexes(db, tbl.Name)
			if idxErr != nil {
				return DiscoverReport{}, fmt.Errorf("read %s indexes: %w", tbl.Name, idxErr)
			}
			finding.OrgIDIndexes = indexes
			rowCount, rowErr := tableRowCount(db, tbl.Name)
			if rowErr != nil {
				return DiscoverReport{}, fmt.Errorf("count %s rows: %w", tbl.Name, rowErr)
			}
			finding.RowCount = rowCount
			if col {
				nullRows, nullErr := tableNullOrgIDRows(db, tbl.Name)
				if nullErr != nil {
					return DiscoverReport{}, fmt.Errorf("count %s null org_id: %w", tbl.Name, nullErr)
				}
				finding.NullOrgRows = nullRows
				summary.TotalNullOrgRows += nullRows
			}
		}
		if tbl.Kind == registry.KindTenant {
			if finding.HasOrgIDCol {
				summary.TenantTablesWithOrgCol++
			} else {
				summary.TenantTablesMissingCol++
			}
		}
		report.Tables = append(report.Tables, finding)
	}

	sort.SliceStable(report.Tables, func(i, j int) bool {
		return report.Tables[i].Name < report.Tables[j].Name
	})
	report.Summary = summary
	return report, nil
}

// ============================== infer ==============================

type InferenceManifest struct {
	GeneratedAt time.Time        `json:"generated_at"`
	Database    string           `json:"database"`
	Entries     []BackfillEntry  `json:"entries"`
	Summary     InferenceSummary `json:"summary"`
}

type InferenceSummary struct {
	TotalRows       int `json:"total_rows"`
	ReadyRows       int `json:"ready_rows"`
	UnresolvedRows  int `json:"unresolved_rows"`
	SkippedRows     int `json:"skipped_rows"`
	TruncatedTables int `json:"truncated_tables"`
}

type BackfillEntry struct {
	Table      string   `json:"table"`
	PKColumn   string   `json:"pk_column"`
	PKValue    string   `json:"pk_value"`
	OrgID      string   `json:"org_id,omitempty"`
	Status     string   `json:"status"`
	Reason     string   `json:"reason"`
	Candidates []string `json:"candidates,omitempty"`
}

type parentRule struct {
	Table        string
	LocalColumn  string
	ParentTable  string
	ParentColumn string
}

var parentRules = []parentRule{
	{"department_change_logs", "department_id", "departments", "department_id"},
	{"user_roles", "user_id", "users", "user_id"},
	{"employee_profiles", "user_id", "users", "user_id"},
	{"dingtalk_bindings", "user_id", "users", "user_id"},
	{"user_sessions", "user_id", "users", "user_id"},
	{"login_logs", "user_id", "users", "user_id"},
	{"operation_logs", "user_id", "users", "user_id"},
	{"attendances", "user_id", "users", "user_id"},
	{"attendance_exports", "user_id", "users", "user_id"},
	{"approvals", "applicant_id", "users", "user_id"},
	{"employee_transfers", "user_id", "users", "user_id"},
	{"employee_resignations", "user_id", "users", "user_id"},
	{"employee_onboardings", "department_id", "departments", "department_id"},
	{"talent_analyses", "user_id", "users", "user_id"},
	{"employee_shift_configs", "user_id", "users", "user_id"},
	{"week_schedule_overrides", "rule_id", "week_schedule_rules", "id"},
	{"annual_leave_eligibilities", "user_id", "users", "user_id"},
	{"annual_leave_grants", "user_id", "users", "user_id"},
	{"annual_leave_grants", "source_eligibility_id", "annual_leave_eligibilities", "id"},
	{"annual_leave_consume_logs", "user_id", "users", "user_id"},
	{"annual_leave_consume_logs", "grant_id", "annual_leave_grants", "id"},
	{"overtime_match_results", "user_id", "users", "user_id"},
	{"overtime_match_results", "approval_id", "approvals", "id"},
	{"overtime_sync_histories", "user_id", "users", "user_id"},
	{"overtime_sync_histories", "approval_id", "approvals", "id"},
	{"overtime_supplementary_requests", "match_result_id", "overtime_match_results", "id"},
	{"overtime_supplementary_requests", "user_id", "users", "user_id"},
	{"compensatory_leave_ledgers", "user_id", "users", "user_id"},
	{"compensatory_leave_ledgers", "source_match_id", "overtime_match_results", "id"},
	{"performance_template_sections", "template_id", "performance_templates", "id"},
	{"performance_template_items", "section_id", "performance_template_sections", "id"},
	{"performance_level_rule_items", "rule_id", "performance_level_rules", "id"},
	{"performance_distribution_rules", "activity_id", "performance_activities", "id"},
	{"performance_distribution_exceptions", "activity_id", "performance_activities", "id"},
	{"performance_reminder_logs", "activity_id", "performance_activities", "id"},
	{"performance_participants", "activity_id", "performance_activities", "id"},
	{"performance_participants", "employee_id", "users", "user_id"},
	{"performance_reviews", "participant_id", "performance_participants", "id"},
	{"performance_review_versions", "participant_id", "performance_participants", "id"},
	{"performance_relationship_change_logs", "participant_id", "performance_participants", "id"},
	{"performance_goal_records", "participant_id", "performance_participants", "id"},
	{"performance_goal_approval_logs", "goal_record_id", "performance_goal_records", "id"},
	{"performance_company_finances", "activity_id", "performance_activities", "id"},
	{"performance_indicator_libraries", "department_id", "departments", "department_id"},
	{"performance_indicator_items", "library_id", "performance_indicator_libraries", "id"},
}

var anchorColumns = map[string]string{
	"user_id":               "users.user_id",
	"applicant_id":          "users.user_id",
	"employee_id":           "users.user_id",
	"operator_id":           "users.user_id",
	"reviewer_id":           "users.user_id",
	"created_by":            "users.user_id",
	"updated_by":            "users.user_id",
	"set_by":                "users.user_id",
	"approved_by":           "users.user_id",
	"approver_id":           "users.user_id",
	"department_id":         "departments.department_id",
	"current_dept_id":       "departments.department_id",
	"current_department_id": "departments.department_id",
	"new_dept_id":           "departments.department_id",
	"new_department_id":     "departments.department_id",
	"target_department_id":  "departments.department_id",
}

func runInfer(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("infer", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_DSN"), "MySQL DSN (default $DATABASE_DSN)")
	outPath := fs.String("out", "", "write manifest to file (default stdout)")
	limit := fs.Int("limit", 10000, "maximum empty-org rows to inspect per table")
	pretty := fs.Bool("pretty", true, "pretty-print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	db, err := openFromDSN(*dsn)
	if err != nil {
		return err
	}
	defer closeGormDB(db)
	manifest, err := Infer(db, *limit)
	if err != nil {
		return err
	}
	return writeJSONToPathOrWriter(stdout, *outPath, manifest, *pretty)
}

func Infer(db *gorm.DB, limit int) (InferenceManifest, error) {
	if limit <= 0 {
		limit = 10000
	}
	manifest := InferenceManifest{GeneratedAt: time.Now().UTC()}
	if name, err := currentDatabaseName(db); err == nil {
		manifest.Database = name
	}

	for _, tbl := range registry.All() {
		if tbl.Kind == registry.KindPlatform {
			continue
		}
		exists, err := tableExists(db, tbl.Name)
		if err != nil {
			return InferenceManifest{}, err
		}
		if !exists {
			continue
		}
		hasOrg, _, err := describeOrgIDColumn(db, tbl.Name)
		if err != nil {
			return InferenceManifest{}, err
		}
		if !hasOrg {
			continue
		}
		cols, err := tableColumns(db, tbl.Name)
		if err != nil {
			return InferenceManifest{}, err
		}
		if !cols["id"] {
			manifest.Entries = append(manifest.Entries, BackfillEntry{
				Table:  tbl.Name,
				Status: entrySkipped,
				Reason: "missing id primary key column",
			})
			continue
		}
		rows, truncated, err := emptyOrgRows(db, tbl.Name, cols, limit)
		if err != nil {
			return InferenceManifest{}, err
		}
		if truncated {
			manifest.Summary.TruncatedTables++
		}
		for _, row := range rows {
			entry := inferRow(db, tbl.Name, row, cols)
			manifest.Entries = append(manifest.Entries, entry)
		}
	}
	sort.SliceStable(manifest.Entries, func(i, j int) bool {
		if manifest.Entries[i].Table == manifest.Entries[j].Table {
			return manifest.Entries[i].PKValue < manifest.Entries[j].PKValue
		}
		return manifest.Entries[i].Table < manifest.Entries[j].Table
	})
	for _, e := range manifest.Entries {
		manifest.Summary.TotalRows++
		switch e.Status {
		case entryReady:
			manifest.Summary.ReadyRows++
		case entrySkipped:
			manifest.Summary.SkippedRows++
		default:
			manifest.Summary.UnresolvedRows++
		}
	}
	return manifest, nil
}

func inferRow(db *gorm.DB, table string, row map[string]string, cols map[string]bool) BackfillEntry {
	entry := BackfillEntry{
		Table:    table,
		PKColumn: "id",
		PKValue:  row["id"],
		Status:   entryUnresolved,
	}
	candidates := map[string]string{}

	for col, anchor := range anchorColumns {
		if !cols[col] || strings.TrimSpace(row[col]) == "" {
			continue
		}
		parts := strings.Split(anchor, ".")
		for _, orgID := range lookupDistinctOrgIDs(db, parts[0], parts[1], row[col]) {
			candidates[orgID] = fmt.Sprintf("%s -> %s", col, anchor)
		}
	}
	if cols["user_id"] && strings.TrimSpace(row["user_id"]) != "" {
		for _, orgID := range lookupDistinctOrgIDs(db, "organization_users", "user_id", row["user_id"]) {
			candidates[orgID] = "user_id -> organization_users.user_id"
		}
	}
	for _, rule := range rulesForTable(table) {
		if !cols[rule.LocalColumn] || strings.TrimSpace(row[rule.LocalColumn]) == "" {
			continue
		}
		for _, orgID := range lookupDistinctOrgIDs(db, rule.ParentTable, rule.ParentColumn, row[rule.LocalColumn]) {
			candidates[orgID] = fmt.Sprintf("%s -> %s.%s", rule.LocalColumn, rule.ParentTable, rule.ParentColumn)
		}
	}

	orgs := make([]string, 0, len(candidates))
	for orgID := range candidates {
		orgs = append(orgs, orgID)
	}
	sort.Strings(orgs)
	entry.Candidates = orgs
	switch len(orgs) {
	case 0:
		entry.Reason = "no unique anchor found; manual review required"
	case 1:
		entry.Status = entryReady
		entry.OrgID = orgs[0]
		entry.Reason = candidates[orgs[0]]
	default:
		entry.Reason = "multiple candidate orgs; manual review required"
	}
	return entry
}

// ============================== apply ==============================

type ApplyReport struct {
	GeneratedAt time.Time `json:"generated_at"`
	Database    string    `json:"database"`
	DryRun      bool      `json:"dry_run"`
	Matched     int       `json:"matched"`
	Updated     int64     `json:"updated"`
	Skipped     int       `json:"skipped"`
}

func runApply(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("apply", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_DSN"), "MySQL DSN (default $DATABASE_DSN)")
	manifestPath := fs.String("manifest", "", "reviewed inference manifest path")
	confirm := fs.Bool("confirm-apply", false, "actually write org_id values")
	pretty := fs.Bool("pretty", true, "pretty-print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifestPath == "" {
		return errors.New("missing --manifest")
	}
	if !*confirm {
		return errors.New("apply requires --confirm-apply; review manifest before writing")
	}
	manifest, err := readManifest(*manifestPath)
	if err != nil {
		return err
	}
	db, err := openFromDSN(*dsn)
	if err != nil {
		return err
	}
	defer closeGormDB(db)
	report, err := ApplyManifest(db, manifest, false)
	if err != nil {
		return err
	}
	return writeJSON(stdout, report, *pretty)
}

func ApplyManifest(db *gorm.DB, manifest InferenceManifest, dryRun bool) (ApplyReport, error) {
	report := ApplyReport{GeneratedAt: time.Now().UTC(), DryRun: dryRun}
	if name, err := currentDatabaseName(db); err == nil {
		report.Database = name
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		for _, entry := range manifest.Entries {
			if entry.Status != entryReady || strings.TrimSpace(entry.OrgID) == "" {
				report.Skipped++
				continue
			}
			if err := validateIdentifier(entry.Table); err != nil {
				return err
			}
			pk := strings.TrimSpace(entry.PKColumn)
			if pk == "" {
				pk = "id"
			}
			if err := validateIdentifier(pk); err != nil {
				return err
			}
			report.Matched++
			if dryRun {
				continue
			}
			sqlText := fmt.Sprintf(
				"UPDATE `%s` SET org_id = ? WHERE `%s` = ? AND (org_id IS NULL OR org_id = '')",
				entry.Table, pk,
			)
			res := tx.Exec(sqlText, entry.OrgID, entry.PKValue)
			if res.Error != nil {
				return res.Error
			}
			report.Updated += res.RowsAffected
		}
		return nil
	})
	return report, err
}

// ============================== verify ==============================

type VerificationReport struct {
	GeneratedAt time.Time           `json:"generated_at"`
	Database    string              `json:"database"`
	Passed      bool                `json:"passed"`
	Issues      []VerificationIssue `json:"issues,omitempty"`
	Summary     ReportSummary       `json:"summary"`
}

type VerificationIssue struct {
	Severity string `json:"severity"`
	Table    string `json:"table"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Count    int64  `json:"count,omitempty"`
}

func runVerify(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_DSN"), "MySQL DSN (default $DATABASE_DSN)")
	pretty := fs.Bool("pretty", true, "pretty-print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	db, err := openFromDSN(*dsn)
	if err != nil {
		return err
	}
	defer closeGormDB(db)
	report, err := Verify(db)
	if err != nil {
		return err
	}
	if err := writeJSON(stdout, report, *pretty); err != nil {
		return err
	}
	if !report.Passed {
		return errors.New("verification failed")
	}
	return nil
}

func Verify(db *gorm.DB) (VerificationReport, error) {
	discover, err := Discover(db)
	if err != nil {
		return VerificationReport{}, err
	}
	report := VerificationReport{
		GeneratedAt: time.Now().UTC(),
		Database:    discover.Database,
		Passed:      true,
		Summary:     discover.Summary,
	}
	for _, f := range discover.Tables {
		tbl, _ := registry.Find(f.Name)
		if tbl.Kind == registry.KindPlatform {
			if f.HasOrgIDCol {
				report.addIssue("warn", f.Name, "platform_has_org_id", "platform table unexpectedly has org_id", 0)
			}
			continue
		}
		if !f.Exists {
			report.addIssue("error", f.Name, "missing_table", "tenant table is missing in database", 0)
			continue
		}
		if !f.HasOrgIDCol {
			report.addIssue("error", f.Name, "missing_org_id", "tenant table lacks org_id column", 0)
			continue
		}
		if f.NullOrgRows > 0 {
			report.addIssue("error", f.Name, "empty_org_id", "tenant table contains NULL or empty org_id rows", f.NullOrgRows)
		}
	}
	for _, issue := range parentMismatchIssues(db) {
		report.Issues = append(report.Issues, issue)
	}
	for _, issue := range report.Issues {
		if issue.Severity == "error" {
			report.Passed = false
			break
		}
	}
	return report, nil
}

func (r *VerificationReport) addIssue(severity, table, code, message string, count int64) {
	r.Issues = append(r.Issues, VerificationIssue{
		Severity: severity,
		Table:    table,
		Code:     code,
		Message:  message,
		Count:    count,
	})
}

func parentMismatchIssues(db *gorm.DB) []VerificationIssue {
	var issues []VerificationIssue
	for _, rule := range parentRules {
		if !tableAndColumnsExist(db, rule.Table, "org_id", rule.LocalColumn) ||
			!tableAndColumnsExist(db, rule.ParentTable, "org_id", rule.ParentColumn) {
			continue
		}
		query := fmt.Sprintf(
			"SELECT COUNT(*) FROM `%s` c JOIN `%s` p ON c.`%s` = p.`%s` WHERE c.org_id IS NOT NULL AND c.org_id <> '' AND p.org_id IS NOT NULL AND p.org_id <> '' AND c.org_id <> p.org_id",
			rule.Table, rule.ParentTable, rule.LocalColumn, rule.ParentColumn,
		)
		var count int64
		if err := db.Raw(query).Scan(&count).Error; err != nil || count == 0 {
			continue
		}
		issues = append(issues, VerificationIssue{
			Severity: "error",
			Table:    rule.Table,
			Code:     "parent_org_mismatch",
			Message:  fmt.Sprintf("%s.%s disagrees with %s.%s org_id", rule.Table, rule.LocalColumn, rule.ParentTable, rule.ParentColumn),
			Count:    count,
		})
	}
	return issues
}

// ============================== contract ==============================

type ContractReport struct {
	GeneratedAt time.Time `json:"generated_at"`
	Database    string    `json:"database"`
	DryRun      bool      `json:"dry_run"`
	Statements  []string  `json:"statements"`
	Executed    int       `json:"executed"`
}

type uniqueIndexSpec struct {
	Table   string
	Name    string
	Columns []string
}

var uniqueIndexSpecs = []uniqueIndexSpec{
	{"users", "idx_org_user_id", []string{"org_id", "user_id"}},
	{"departments", "idx_org_dept_id", []string{"org_id", "department_id"}},
	{"employee_profiles", "idx_employee_profiles_org_user", []string{"org_id", "user_id"}},
	{"employee_profiles", "idx_employee_profiles_org_employee", []string{"org_id", "employee_id"}},
	{"attendances", "idx_org_user_time_type", []string{"org_id", "user_id", "check_time", "check_type"}},
	{"organization_users", "idx_org_user", []string{"org_id", "user_id"}},
}

func runContract(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("contract", flag.ContinueOnError)
	dsn := fs.String("dsn", os.Getenv("DATABASE_DSN"), "MySQL DSN (default $DATABASE_DSN)")
	confirm := fs.Bool("confirm-contract", false, "execute ALTER/CREATE INDEX statements")
	pretty := fs.Bool("pretty", true, "pretty-print JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	db, err := openFromDSN(*dsn)
	if err != nil {
		return err
	}
	defer closeGormDB(db)
	report, err := Contract(db, !*confirm)
	if err != nil {
		return err
	}
	return writeJSON(stdout, report, *pretty)
}

func Contract(db *gorm.DB, dryRun bool) (ContractReport, error) {
	verify, err := Verify(db)
	if err != nil {
		return ContractReport{}, err
	}
	if !verify.Passed {
		return ContractReport{}, errors.New("verification must pass before contract")
	}
	report := ContractReport{GeneratedAt: time.Now().UTC(), DryRun: dryRun, Database: verify.Database}
	statements, err := contractStatements(db)
	if err != nil {
		return ContractReport{}, err
	}
	report.Statements = statements
	if dryRun {
		return report, nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		for _, stmt := range statements {
			if err := tx.Exec(stmt).Error; err != nil {
				return err
			}
			report.Executed++
		}
		return nil
	})
	return report, err
}

func contractStatements(db *gorm.DB) ([]string, error) {
	var statements []string
	for _, tbl := range registry.All() {
		if tbl.Kind == registry.KindPlatform {
			continue
		}
		exists, err := tableExists(db, tbl.Name)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		hasOrg, nullable, err := describeOrgIDColumn(db, tbl.Name)
		if err != nil {
			return nil, err
		}
		if !hasOrg {
			continue
		}
		if nullable {
			statements = append(statements, fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN `org_id` VARCHAR(64) NOT NULL", tbl.Name))
		}
		indexes, err := orgIDIndexes(db, tbl.Name)
		if err != nil {
			return nil, err
		}
		if len(indexes) == 0 {
			statements = append(statements, fmt.Sprintf("CREATE INDEX `idx_%s_org_id` ON `%s` (`org_id`)", tbl.Name, tbl.Name))
		}
	}
	for _, spec := range uniqueIndexSpecs {
		if !tableAndColumnsExist(db, spec.Table, spec.Columns...) {
			continue
		}
		exists, err := indexExists(db, spec.Table, spec.Name)
		if err != nil {
			return nil, err
		}
		if !exists {
			statements = append(statements, fmt.Sprintf("CREATE UNIQUE INDEX `%s` ON `%s` (%s)", spec.Name, spec.Table, quotedColumnList(spec.Columns)))
		}
	}
	sort.Strings(statements)
	return statements, nil
}

// ============================== SQL helpers ==============================

func openFromDSN(dsn string) (*gorm.DB, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, errors.New("missing DATABASE_DSN (or --dsn)")
	}
	return openTargetDB(dsn)
}

func openTargetDB(dsn string) (*gorm.DB, error) {
	return gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
}

func closeGormDB(db *gorm.DB) {
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func currentDatabaseName(db *gorm.DB) (string, error) {
	var name string
	err := db.Raw("SELECT DATABASE()").Scan(&name).Error
	return name, err
}

func tableExists(db *gorm.DB, table string) (bool, error) {
	if err := validateIdentifier(table); err != nil {
		return false, err
	}
	var count int64
	err := db.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", table).Scan(&count).Error
	return count > 0, err
}

func describeOrgIDColumn(db *gorm.DB, table string) (hasCol bool, nullable bool, err error) {
	if err := validateIdentifier(table); err != nil {
		return false, false, err
	}
	var row struct {
		IsNullable string `gorm:"column:IS_NULLABLE"`
	}
	tx := db.Raw(
		"SELECT IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'org_id'",
		table,
	).Scan(&row)
	if tx.Error != nil {
		return false, false, tx.Error
	}
	if tx.RowsAffected == 0 {
		return false, false, nil
	}
	return true, row.IsNullable == "YES", nil
}

func tableColumns(db *gorm.DB, table string) (map[string]bool, error) {
	if err := validateIdentifier(table); err != nil {
		return nil, err
	}
	var rows []struct {
		ColumnName string `gorm:"column:COLUMN_NAME"`
	}
	if err := db.Raw(
		"SELECT COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?",
		table,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make(map[string]bool, len(rows))
	for _, row := range rows {
		out[row.ColumnName] = true
	}
	return out, nil
}

func orgIDIndexes(db *gorm.DB, table string) ([]string, error) {
	if err := validateIdentifier(table); err != nil {
		return nil, err
	}
	var rows []struct {
		IndexName string `gorm:"column:INDEX_NAME"`
	}
	if err := db.Raw(
		"SELECT DISTINCT INDEX_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'org_id'",
		table,
	).Scan(&rows).Error; err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, r.IndexName)
	}
	sort.Strings(names)
	return names, nil
}

func tableRowCount(db *gorm.DB, table string) (int64, error) {
	if err := validateIdentifier(table); err != nil {
		return 0, err
	}
	var count int64
	err := db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s`", table)).Scan(&count).Error
	return count, err
}

func tableNullOrgIDRows(db *gorm.DB, table string) (int64, error) {
	if err := validateIdentifier(table); err != nil {
		return 0, err
	}
	var count int64
	err := db.Raw(fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE org_id IS NULL OR org_id = ''", table)).Scan(&count).Error
	return count, err
}

func emptyOrgRows(db *gorm.DB, table string, available map[string]bool, limit int) ([]map[string]string, bool, error) {
	cols := []string{"id"}
	for col := range anchorColumns {
		if available[col] {
			cols = append(cols, col)
		}
	}
	for _, rule := range rulesForTable(table) {
		if available[rule.LocalColumn] && !contains(cols, rule.LocalColumn) {
			cols = append(cols, rule.LocalColumn)
		}
	}
	sort.Strings(cols[1:])
	selects := make([]string, 0, len(cols))
	for _, col := range cols {
		if err := validateIdentifier(col); err != nil {
			return nil, false, err
		}
		selects = append(selects, fmt.Sprintf("`%s`", col))
	}
	query := fmt.Sprintf("SELECT %s FROM `%s` WHERE org_id IS NULL OR org_id = '' ORDER BY id LIMIT %d", strings.Join(selects, ", "), table, limit+1)
	rows, err := db.Raw(query).Rows()
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	got, err := scanStringMaps(rows)
	if err != nil {
		return nil, false, err
	}
	truncated := len(got) > limit
	if truncated {
		got = got[:limit]
	}
	return got, truncated, nil
}

func lookupDistinctOrgIDs(db *gorm.DB, table, column, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || !tableAndColumnsExist(db, table, "org_id", column) {
		return nil
	}
	query := fmt.Sprintf("SELECT DISTINCT org_id FROM `%s` WHERE `%s` = ? AND org_id IS NOT NULL AND org_id <> ''", table, column)
	var rows []struct {
		OrgID string `gorm:"column:org_id"`
	}
	if err := db.Raw(query, value).Scan(&rows).Error; err != nil {
		return nil
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if orgID := strings.TrimSpace(row.OrgID); orgID != "" {
			out = append(out, orgID)
		}
	}
	sort.Strings(out)
	return out
}

func indexExists(db *gorm.DB, table, index string) (bool, error) {
	var count int64
	err := db.Raw(
		"SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?",
		table, index,
	).Scan(&count).Error
	return count > 0, err
}

func tableAndColumnsExist(db *gorm.DB, table string, columns ...string) bool {
	exists, err := tableExists(db, table)
	if err != nil || !exists {
		return false
	}
	available, err := tableColumns(db, table)
	if err != nil {
		return false
	}
	for _, col := range columns {
		if !available[col] {
			return false
		}
	}
	return true
}

func scanStringMaps(rows *sql.Rows) ([]map[string]string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]string
	for rows.Next() {
		values := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range values {
			ptrs[i] = &values[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := make(map[string]string, len(cols))
		for i, col := range cols {
			row[col] = valueToString(values[i])
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

func valueToString(v interface{}) string {
	switch typed := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(typed)
	case string:
		return typed
	case time.Time:
		return typed.Format(time.RFC3339Nano)
	default:
		return fmt.Sprint(typed)
	}
}

func rulesForTable(table string) []parentRule {
	var out []parentRule
	for _, rule := range parentRules {
		if rule.Table == table {
			out = append(out, rule)
		}
	}
	return out
}

// ============================== generic helpers ==============================

func kindLabel(k registry.Kind) string {
	switch k {
	case registry.KindTenant:
		return "tenant"
	case registry.KindMembership:
		return "membership"
	case registry.KindPlatform:
		return "platform"
	default:
		return "unknown"
	}
}

func readManifest(path string) (InferenceManifest, error) {
	var manifest InferenceManifest
	data, err := os.ReadFile(path)
	if err != nil {
		return manifest, err
	}
	err = json.Unmarshal(data, &manifest)
	return manifest, err
}

func writeJSONToPathOrWriter(stdout io.Writer, outPath string, value interface{}, pretty bool) error {
	var target io.Writer = stdout
	if outPath != "" {
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		defer f.Close()
		target = f
	}
	return writeJSON(target, value, pretty)
}

func writeJSON(w io.Writer, v interface{}, pretty bool) error {
	enc := json.NewEncoder(w)
	if pretty {
		enc.SetIndent("", "  ")
	}
	return enc.Encode(v)
}

func validateIdentifier(value string) error {
	if !identifierRE.MatchString(value) {
		return fmt.Errorf("unsafe SQL identifier %q", value)
	}
	return nil
}

func quotedColumnList(cols []string) string {
	out := make([]string, 0, len(cols))
	for _, col := range cols {
		out = append(out, fmt.Sprintf("`%s`", col))
	}
	return strings.Join(out, ", ")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
