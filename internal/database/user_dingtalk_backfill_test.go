package database

import "testing"

func TestBackfillUserDingTalkIDRejectsUnknownSourceColumn(t *testing.T) {
	if err := backfillUserDingTalkID("unsafe_column"); err == nil {
		t.Fatal("expected unsupported source column to be rejected")
	}
}
