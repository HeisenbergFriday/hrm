package database

import (
	"reflect"
	"testing"
)

func TestNormalizeMenuPermissionKeysDropsRemovedPages(t *testing.T) {
	got := normalizeMenuPermissionKeys([]string{
		"home",
		"menu:approval-stats",
		"attendance-stats",
		"menu:approval-instances",
		"menu:approval-instances",
	})
	want := []string{"menu:approval-instances", "menu:home"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeMenuPermissionKeys() = %#v, want %#v", got, want)
	}
}
