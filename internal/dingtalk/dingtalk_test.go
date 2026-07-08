package dingtalk

import (
	"testing"

	"peopleops/internal/database"
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

func TestIsNotifiableUserIDRejectsSystemAccountsWithoutDB(t *testing.T) {
	originalDB := database.DB
	database.DB = nil
	t.Cleanup(func() {
		database.DB = originalDB
	})

	for _, userID := range []string{"", "admin", "Admin", "system", "SYSTEM"} {
		if IsNotifiableUserID(userID) {
			t.Fatalf("IsNotifiableUserID(%q) = true, want false", userID)
		}
	}
	if !IsNotifiableUserID("employee-001") {
		t.Fatalf("IsNotifiableUserID(%q) = false, want true without DB", "employee-001")
	}
}
