package service

import (
	"sync"
	"testing"
)

func TestShiftIDCacheUsesStableShiftKey(t *testing.T) {
	shiftIDCache = sync.Map{}

	firstKey := normalize("17:30下班", "09:00", "17:30")
	secondKey := normalize("17:30下班", "10:00", "17:30")

	cacheShiftID(firstKey, 101)

	if id, ok := getCachedShiftID(firstKey); !ok || id != 101 {
		t.Fatalf("expected first shift key to resolve cached id 101, got id=%d ok=%v", id, ok)
	}

	if id, ok := getCachedShiftID(secondKey); ok {
		t.Fatalf("expected different time range to miss cache, got id=%d", id)
	}
}
