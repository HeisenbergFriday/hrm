package database

import (
	"strings"
	"testing"
	"time"
)

func TestBuildExternalApproveItemKeyDiffersByTag(t *testing.T) {
	a := BuildExternalApproveItemKey("proc", "leave", "", "", "", "", "1", "day")
	b := BuildExternalApproveItemKey("proc", "overtime", "", "", "", "", "1", "day")
	if a == b {
		t.Fatal("tags must differ")
	}
}

func TestRedactToken(t *testing.T) {
	if redactToken("abcdefghij", 4) != "abcd…" {
		t.Fatalf("got %s", redactToken("abcdefghij", 4))
	}
	if redactToken("", 4) != "?" {
		t.Fatal("empty")
	}
}

func TestApproveItemKeyIncludesBeginEnd(t *testing.T) {
	begin := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	end := time.Date(2026, 5, 1, 18, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	a := BuildExternalApproveItemKey("p", "leave", "", "", begin, end, "1", "day")
	b := BuildExternalApproveItemKey("p", "leave", "", "", "", "", "1", "day")
	if a == b {
		t.Fatal("begin/end should change key")
	}
}

// TestLegacyApproveLinkUpgradeKeyContract documents the backfill rule used by
// migrateExternalAttendanceApproveLinksSchema for old rows without item_key.
// Full DB upgrade is exercised in migrate(); this guards the key algorithm.
func TestLegacyApproveLinkUpgradeKeyContract(t *testing.T) {
	// Old row shape: same source_row_key, different tag/duration, nullable begin_time.
	// New unique is (org, source_row_key, item_key) — empty begin must still be stable.
	k1 := BuildExternalApproveItemKey("proc-1", "请假", "1", "", "", "", "8", "hour")
	k2 := BuildExternalApproveItemKey("proc-1", "请假", "1", "", "", "", "8", "hour")
	k3 := BuildExternalApproveItemKey("proc-1", "加班", "1", "", "", "", "8", "hour")
	if k1 == "" || k1 != k2 {
		t.Fatal("legacy empty begin must produce stable non-empty item_key")
	}
	if k1 == k3 {
		t.Fatal("different tag_name on same source row must not collide")
	}
	// Conflict audit sample redaction must not empty-out org labels.
	if redactOrgID("xiaotie") != "xiaotie" {
		t.Fatal("org_id should remain for ops")
	}
	if !strings.HasSuffix(redactToken("abcdef0123456789", 8), "…") {
		t.Fatal("token redaction")
	}
}
