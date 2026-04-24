package dingtalk

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExtractHRMFieldValueNormalizesDateValues(t *testing.T) {
	field := map[string]interface{}{
		"field_value_list": []interface{}{
			map[string]interface{}{
				"value": "2026-04-24 00:00:00",
			},
		},
	}

	if got := extractHRMFieldValue(field); got != "2026-04-24" {
		t.Fatalf("expected normalized date, got %s", got)
	}
}

func TestNormalizeDingTalkDateFromMilliseconds(t *testing.T) {
	ts := time.Date(2026, 4, 24, 12, 0, 0, 0, time.Local).UnixMilli()
	if got := normalizeDingTalkDate(strconv.FormatInt(ts, 10)); got != "2026-04-24" {
		t.Fatalf("expected 2026-04-24, got %s", got)
	}
}

func TestFindScheduleGroupIDReturnsSingleEligibleGroup(t *testing.T) {
	groups := []map[string]interface{}{
		{"group_id": float64(10), "group_name": "Fixed", "type": "FIXED"},
		{"group_id": float64(22), "group_name": "Main Schedule", "type": "SCHEDULE"},
	}

	groupID, err := FindScheduleGroupID(groups)
	if err != nil {
		t.Fatalf("expected single eligible group to be selected, got error: %v", err)
	}
	if groupID != 22 {
		t.Fatalf("expected group id 22, got %d", groupID)
	}
}

func TestFindScheduleGroupIDRejectsAmbiguousAutoSelection(t *testing.T) {
	t.Setenv("DINGTALK_ATTENDANCE_GROUP_ID", "")
	t.Setenv("DINGTALK_ATTENDANCE_GROUP_NAME", "")

	groups := []map[string]interface{}{
		{"group_id": float64(22), "group_name": "Schedule A", "type": "SCHEDULE"},
		{"group_id": float64(23), "group_name": "Schedule B", "type": "TURN"},
	}

	_, err := FindScheduleGroupID(groups)
	if err == nil {
		t.Fatal("expected ambiguous eligible groups to require explicit configuration")
	}
	if !strings.Contains(err.Error(), "DINGTALK_ATTENDANCE_GROUP_ID") {
		t.Fatalf("expected explicit configuration hint, got %v", err)
	}
}

func TestFindScheduleGroupIDHonorsConfiguredGroupID(t *testing.T) {
	t.Setenv("DINGTALK_ATTENDANCE_GROUP_ID", "23")
	t.Setenv("DINGTALK_ATTENDANCE_GROUP_NAME", "")

	groups := []map[string]interface{}{
		{"group_id": float64(22), "group_name": "Schedule A", "type": "SCHEDULE"},
		{"group_id": float64(23), "group_name": "Schedule B", "type": "TURN"},
	}

	groupID, err := FindScheduleGroupID(groups)
	if err != nil {
		t.Fatalf("expected configured group id to win, got error: %v", err)
	}
	if groupID != 23 {
		t.Fatalf("expected group id 23, got %d", groupID)
	}
}
