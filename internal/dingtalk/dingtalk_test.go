package dingtalk

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBuildCorpMessagePayloadUsesAsyncSendSchema(t *testing.T) {
	t.Setenv("DINGTALK_AGENT_ID", "42")

	payload := buildCorpMessagePayload("ding-user-1", "Review Reminder", "Please finish self review")

	if got, ok := payload["agent_id"].(int64); !ok || got != 42 {
		t.Fatalf("expected agent_id 42, got %#v", payload["agent_id"])
	}
	if got, ok := payload["userid_list"].(string); !ok || got != "ding-user-1" {
		t.Fatalf("expected userid_list ding-user-1, got %#v", payload["userid_list"])
	}

	msg, ok := payload["msg"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected msg payload, got %#v", payload["msg"])
	}
	if got, ok := msg["msgtype"].(string); !ok || got != "text" {
		t.Fatalf("expected msg.msgtype text, got %#v", msg["msgtype"])
	}

	text, ok := msg["text"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected msg.text payload, got %#v", msg["text"])
	}
	if got, ok := text["content"].(string); !ok || got != "Review Reminder\n\nPlease finish self review" {
		t.Fatalf("unexpected text content: %#v", text["content"])
	}
}

func TestShouldValidateAttendanceGroupMembersUsesExplicitUserIDs(t *testing.T) {
	group := map[string]interface{}{
		"userids": map[string]interface{}{
			"string": []interface{}{"u1", "u2"},
		},
	}

	memberIDs := collectAttendanceGroupUserIDs(group)
	shouldValidate, reason := shouldValidateAttendanceGroupMembers(group, memberIDs)
	if !shouldValidate {
		t.Fatalf("expected explicit userids to enable member validation, got skip reason %q", reason)
	}
	if len(memberIDs) != 2 {
		t.Fatalf("expected 2 member ids, got %d", len(memberIDs))
	}
}

func TestShouldValidateAttendanceGroupMembersSkipsWhenOnlyAddressListAvailable(t *testing.T) {
	group := map[string]interface{}{
		"member_count": float64(2),
		"address_list": []interface{}{"智恒产业园"},
	}

	memberIDs := collectAttendanceGroupUserIDs(group)
	shouldValidate, reason := shouldValidateAttendanceGroupMembers(group, memberIDs)
	if shouldValidate {
		t.Fatal("expected member validation to be skipped when DingTalk omits explicit userids")
	}
	if !strings.Contains(reason, "member_count=2") {
		t.Fatalf("expected skip reason to mention member_count, got %q", reason)
	}
}

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
