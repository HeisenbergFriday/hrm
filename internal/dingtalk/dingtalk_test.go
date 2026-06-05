package dingtalk

import (
	"testing"

	"peopleops/internal/database"
)

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
