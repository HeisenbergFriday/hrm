package repository

import (
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
)

func TestMergeApprovalExtensionAppliesPatchWithoutDroppingExistingFields(t *testing.T) {
	base := map[string]interface{}{
		"local_match_ref": "match-1",
		"result":          "RUNNING",
	}
	patch := map[string]interface{}{
		"result":       "agree",
		"process_code": "PROC-1",
	}

	merged := mergeApprovalExtension(base, patch)
	if merged["local_match_ref"] != "match-1" {
		t.Fatalf("local field was dropped: %#v", merged)
	}
	if merged["result"] != "agree" {
		t.Fatalf("patched result = %#v, want agree", merged["result"])
	}
	if merged["process_code"] != "PROC-1" {
		t.Fatalf("process_code = %#v, want PROC-1", merged["process_code"])
	}
	if base["result"] != "RUNNING" {
		t.Fatalf("base map was mutated: %#v", base)
	}
}

func TestApprovalUpsertLookupUsesOrgAndProcessID(t *testing.T) {
	matched := false
	now := time.Now()
	db := newGoalApprovalTestDB(t, stubGoalApprovalQueryResponse{
		match: func(query string, args []driver.NamedValue) bool {
			normalized := strings.ToLower(query)
			if !strings.Contains(normalized, "org_id = ?") || !strings.Contains(normalized, "process_id = ?") {
				return false
			}
			if len(args) < 2 || args[0].Value != "org-a" || args[1].Value != "process-1" {
				return false
			}
			matched = true
			return true
		},
		columns: []string{
			"id", "org_id", "process_id", "title", "applicant_id", "applicant_name",
			"status", "create_time", "finish_time", "content", "extension", "created_at", "updated_at", "deleted_at",
		},
		rows: [][]driver.Value{{
			uint64(1), "org-a", "process-1", "old title", "u1", "User One",
			"RUNNING", now, now, `{}`, `{"local_match_ref":"match-1"}`, now, now, nil,
		}},
	})
	repo := NewApprovalRepositoryWithOrgID(db, "org-a")

	err := repo.UpsertByOrgProcessID(&database.Approval{
		OrgID:     "org-a",
		ProcessID: "process-1",
		Status:    "COMPLETED",
		Extension: map[string]interface{}{"result": "agree"},
	})
	if err != nil {
		t.Fatalf("UpsertByOrgProcessID() error = %v", err)
	}
	if !matched {
		t.Fatal("approval lookup did not include both org_id and process_id")
	}
}

func TestApprovalUpsertRejectsCrossOrgRecord(t *testing.T) {
	db := newDryRunGORM(t)
	repo := NewApprovalRepositoryWithOrgID(db, "org-a")

	err := repo.UpsertByOrgProcessID(&database.Approval{OrgID: "org-b", ProcessID: "process-1"})
	if err != ErrOrgMismatch {
		t.Fatalf("err = %v, want ErrOrgMismatch", err)
	}
}
