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
