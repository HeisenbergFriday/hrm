package repository

import (
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
)

func TestAttendanceSelectColumnsWhitelist(t *testing.T) {
	joined := strings.Join(AttendanceSelectColumns, ",")
	if !strings.Contains(joined, "user_attendance_info_json") {
		t.Fatal("must select user_attendance_info_json")
	}
	if strings.Contains(joined, "user_attendance_info,") || strings.HasSuffix(joined, "user_attendance_info") {
		t.Fatal("must not select invalid user_attendance_info without _json")
	}
	// ensure no SELECT *
	for _, c := range AttendanceSelectColumns {
		if c == "*" {
			t.Fatal("SELECT * forbidden")
		}
	}
}

func TestBuildAttendancePageTieKeyPrefersRecordID(t *testing.T) {
	var nt sql.NullTime
	k := BuildAttendancePageTieKey("u1", "2026-07-01", "OnDuty", nt, "USER", "", "", "rec-9")
	if k != "r:rec-9" {
		t.Fatalf("got %s", k)
	}
}

func TestBuildAttendancePageTieKeyStableWhenRecordEmpty(t *testing.T) {
	ts := time.Date(2026, 7, 1, 9, 0, 0, 0, time.UTC)
	nt := sql.NullTime{Valid: true, Time: ts}
	k1 := BuildAttendancePageTieKey("u1", "2026-07-01", "OnDuty", nt, "USER", "p1", "", "")
	k2 := BuildAttendancePageTieKey("u1", "2026-07-01", "OnDuty", nt, "USER", "p1", "", "")
	if k1 == "" || !strings.HasPrefix(k1, "h:") || k1 != k2 {
		t.Fatalf("unstable key: %s / %s", k1, k2)
	}
	k3 := BuildAttendancePageTieKey("u2", "2026-07-01", "OnDuty", nt, "USER", "p1", "", "")
	if k1 == k3 {
		t.Fatal("different user must differ")
	}
}

func TestBuildApproveItemKeyStableWithEmptyBegin(t *testing.T) {
	a := BuildApproveItemKey("proc1", "请假", "", "", "", "", "1", "day")
	b := BuildApproveItemKey("proc1", "请假", "", "", "", "", "1", "day")
	if a == "" || a != b {
		t.Fatal("item key unstable")
	}
	c := BuildApproveItemKey("proc1", "加班", "", "", "", "", "1", "day")
	if a == c {
		t.Fatal("different tag should differ")
	}
}

func TestExternalSourceTableConstants(t *testing.T) {
	if ExternalSourceAttendanceTable != "dwd_dingtalk_user_attendance_info_di" {
		t.Fatal(ExternalSourceAttendanceTable)
	}
	if ExternalSourceDepartmentTable != "dwd_dingtalk_user_deparment_relation_di" {
		t.Fatal(ExternalSourceDepartmentTable)
	}
}

func TestNewExternalAttendanceLocalRepositoryNormalizesOrg(t *testing.T) {
	repo := NewExternalAttendanceLocalRepository(nil, "Xiaotie")
	if repo.orgID != database.NormalizeOrganizationID("Xiaotie") {
		t.Fatalf("org normalize failed: %s", repo.orgID)
	}
}

func TestAttendancePageTieKeySQLUsesSHA2(t *testing.T) {
	if !strings.Contains(attendancePageTieKeySQL, "SHA2") {
		t.Fatal("must use SHA2 in SQL")
	}
	if !strings.Contains(attendancePageTieKeySQL, "CONCAT_WS") {
		t.Fatal("must use CONCAT_WS")
	}
	if !strings.Contains(attendancePageTieKeySQL, "record_id") {
		t.Fatal("prefer record_id")
	}
}

func TestListAttendanceSQLHasNoOverFetchCommentContract(t *testing.T) {
	// Structural: white-list must not include *
	for _, c := range AttendanceSelectColumns {
		if strings.Contains(c, "*") {
			t.Fatal("no star")
		}
	}
}

func TestIsExternalSyncDuplicateKey(t *testing.T) {
	if !isExternalSyncDuplicateKey(errors.New("Error 1062: Duplicate entry")) {
		t.Fatal("1062")
	}
	if !isExternalSyncDuplicateKey(errors.New("UNIQUE constraint failed")) {
		t.Fatal("unique")
	}
	if isExternalSyncDuplicateKey(errors.New("connection refused")) {
		t.Fatal("non-dup")
	}
}

func TestExternalSyncLockScopeConstant(t *testing.T) {
	if ExternalSyncLockScope != "external-attendance" {
		t.Fatal(ExternalSyncLockScope)
	}
}

// TestSimulatedLargeSameTimestampPaging verifies cursor (time, page_tie_key) pagination
// over a synthetic batch larger than 2000 rows with empty record_id majority — the scenario
// that broke Go over-fetch. This is an offline model of Doris SQL-side page_tie_key.
func TestSimulatedLargeSameTimestampPaging(t *testing.T) {
	const (
		total    = 2500
		pageSize = 200
	)
	fixed := time.Date(2026, 5, 26, 16, 2, 3, 0, time.UTC)

	type row struct {
		t   time.Time
		key string
	}
	all := make([]row, 0, total)
	for i := 0; i < total; i++ {
		// Majority empty record_id (matches production); uniqueness comes from plan_id/user.
		recordID := ""
		if i%10 == 0 {
			recordID = "rec-" + itoa(i)
		}
		userID := "u" + itoa(i)
		planID := "plan-" + itoa(i)
		uct := sql.NullTime{Valid: true, Time: fixed.Add(time.Duration(i) * time.Millisecond)}
		key := BuildAttendancePageTieKey(userID, "2026-05-26", "OnDuty", uct, "USER", planID, "", recordID)
		if key == "" {
			t.Fatal("empty key")
		}
		all = append(all, row{t: fixed, key: key})
	}
	// Sort by (time, key) as Doris ORDER BY db_update_time, page_tie_key.
	sort.Slice(all, func(i, j int) bool {
		if all[i].t.Equal(all[j].t) {
			return all[i].key < all[j].key
		}
		return all[i].t.Before(all[j].t)
	})

	// Keyset pagination: WHERE time > ? OR (time = ? AND key > ?)
	cursorT := time.Time{}
	cursorKey := ""
	seen := make(map[string]struct{}, total)
	pages := 0
	for {
		var page []row
		for _, r := range all {
			if r.t.After(cursorT) || (r.t.Equal(cursorT) && r.key > cursorKey) {
				page = append(page, r)
				if len(page) >= pageSize {
					break
				}
			}
		}
		if len(page) == 0 {
			break
		}
		pages++
		last := page[len(page)-1]
		if last.t.Before(cursorT) || (last.t.Equal(cursorT) && last.key <= cursorKey) {
			t.Fatal("cursor did not advance")
		}
		for _, r := range page {
			id := r.t.Format(time.RFC3339Nano) + "|" + r.key
			if _, ok := seen[id]; ok {
				t.Fatalf("duplicate page row key=%s", r.key)
			}
			seen[id] = struct{}{}
		}
		cursorT = last.t
		cursorKey = last.key
		if len(page) < pageSize {
			break
		}
		if pages > total/pageSize+5 {
			t.Fatal("pagination loop runaway")
		}
	}
	if len(seen) != total {
		t.Fatalf("paged unique=%d want %d pages=%d", len(seen), total, pages)
	}
	if pages < total/pageSize {
		t.Fatalf("too few pages: %d", pages)
	}
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
