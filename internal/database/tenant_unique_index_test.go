package database

import (
	"reflect"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func TestRoleNameUniqueIndexIsOrganizationScoped(t *testing.T) {
	assertUniqueIndexFields(t, &Role{}, "idx_roles_org_name", []string{"org_id", "name"})
	assertModelHasNoIndex(t, &Role{}, "uni_roles_name")
}

func TestPerformanceReminderUniqueIndexIsOrganizationScoped(t *testing.T) {
	assertUniqueIndexFields(t, &PerformanceReminderLog{}, performanceReminderOrgIndexName, []string{
		"org_id",
		"activity_id",
		"participant_id",
		"stage",
		"reminder_key",
		"reminder_date",
	})
	assertModelHasNoIndex(t, &PerformanceReminderLog{}, performanceReminderLegacyIndexName)
}

func TestIdempotencyRecordUniqueIndexIsOrganizationScoped(t *testing.T) {
	assertUniqueIndexFields(t, &IdempotencyRecord{}, "idx_idempotency_org_digest", []string{"org_id", "digest"})
	assertModelHasNoIndex(t, &IdempotencyRecord{}, "idx_idempotency_records_digest")
}

func TestAnnualLeaveConsumeRequestGateIsOrganizationScoped(t *testing.T) {
	assertUniqueIndexFields(t, &AnnualLeaveConsumeRequest{}, "idx_leave_request_org_ref", []string{"org_id", "request_ref"})
}

func TestSyncStatusUniqueIndexIsOrganizationScoped(t *testing.T) {
	assertUniqueIndexFields(t, &SyncStatus{}, "idx_org_sync_type", []string{"org_id", "type"})
	assertModelHasNoIndex(t, &SyncStatus{}, "idx_sync_statuses_org_type")
}

func assertUniqueIndexFields(t *testing.T, model interface{}, indexName string, want []string) {
	t.Helper()
	parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse %T schema: %v", model, err)
	}
	for _, index := range parsed.ParseIndexes() {
		if index.Name != indexName {
			continue
		}
		if index.Class != "UNIQUE" {
			t.Fatalf("index %s class = %q, want UNIQUE", indexName, index.Class)
		}
		got := make([]string, 0, len(index.Fields))
		for _, field := range index.Fields {
			got = append(got, field.DBName)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("index %s fields = %#v, want %#v", indexName, got, want)
		}
		return
	}
	t.Fatalf("index %s not found on %T", indexName, model)
}

func assertModelHasNoIndex(t *testing.T, model interface{}, indexName string) {
	t.Helper()
	parsed, err := schema.Parse(model, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse %T schema: %v", model, err)
	}
	for _, index := range parsed.ParseIndexes() {
		if index.Name == indexName {
			t.Fatalf("legacy index %s must not be declared on %T", indexName, model)
		}
	}
}
