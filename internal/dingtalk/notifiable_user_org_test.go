package dingtalk

import (
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openNotifiableUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:notifiable-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	// User + Organization: send paths may call ConfigForOrgID which queries organizations.
	if err := db.AutoMigrate(&database.User{}, &database.Organization{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestIsNotifiableUserIDForOrg_IsolatesSameUserIDAcrossOrgs(t *testing.T) {
	db := openNotifiableUserDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	// same user_id, different org status
	if err := db.Create(&database.User{
		OrgID: "org-a", UserID: "same-user", DingTalkUserID: "dt-a", Name: "A",
		Status: "active", Email: "a@a.test", Mobile: "m-a",
	}).Error; err != nil {
		t.Fatalf("seed org-a: %v", err)
	}
	if err := db.Create(&database.User{
		OrgID: "org-b", UserID: "same-user", DingTalkUserID: "dt-b", Name: "B",
		Status: "resigned", Email: "b@b.test", Mobile: "m-b",
	}).Error; err != nil {
		t.Fatalf("seed org-b: %v", err)
	}

	if !IsNotifiableUserIDForOrg("org-a", "same-user") {
		t.Fatal("org-a active user must be notifiable")
	}
	if IsNotifiableUserIDForOrg("org-b", "same-user") {
		t.Fatal("org-b resigned user must not be notifiable")
	}
}

func TestIsNotifiableUserIDForOrg_FailClosedOnEmpty(t *testing.T) {
	if IsNotifiableUserIDForOrg("", "u1") {
		t.Fatal("empty org must fail closed")
	}
	if IsNotifiableUserIDForOrg("org-a", "") {
		t.Fatal("empty user must fail closed")
	}
	if IsNotifiableUserIDForOrg("org-a", "admin") {
		t.Fatal("admin must not be notifiable")
	}
}

func TestSendCorpMessageUsesOrgScopedNotifiableCheck(t *testing.T) {
	db := openNotifiableUserDB(t)
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })

	if err := db.Create(&database.User{
		OrgID: "org-b", UserID: "same-user", DingTalkUserID: "dt-b", Name: "B",
		Status: "resigned", Email: "b@b.test", Mobile: "m-b",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Seed active twin in org-a — must not make org-b notifiable.
	if err := db.Create(&database.User{
		OrgID: "org-a", UserID: "same-user", DingTalkUserID: "dt-a", Name: "A",
		Status: "active", Email: "a@a.test", Mobile: "m-a",
	}).Error; err != nil {
		t.Fatalf("seed a: %v", err)
	}

	err := SendCorpActionCardToUserForOrg("org-b", "same-user", "t", "c", "go", "https://example/x")
	if err == nil || !IsUserNotNotifiableError(err) {
		t.Fatalf("org-b resigned should be not-notifiable, err=%v", err)
	}
}
