package service

import (
	"context"
	"strings"
	"testing"
)

func TestAttendanceToolboxDefaultsPreserveChinese(t *testing.T) {
	defaults, err := NewAttendanceToolboxService().Defaults(context.Background())
	if err != nil {
		t.Fatalf("read attendance toolbox defaults: %v", err)
	}

	specialNames := strings.Join(defaults["leave_special_names"], "、")
	if !strings.Contains(specialNames, "梁伯林") {
		t.Fatalf("expected Chinese default name to be preserved, got %q", specialNames)
	}
	if strings.ContainsRune(specialNames, '\uFFFD') {
		t.Fatalf("default names contain replacement characters: %q", specialNames)
	}
}
