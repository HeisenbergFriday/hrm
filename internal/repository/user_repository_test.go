package repository

import (
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupUserRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("create test db: %v", err)
	}

	if err := db.AutoMigrate(&database.User{}, &database.EmployeeProfile{}); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}

	return db
}

func createTestUser(t *testing.T, db *gorm.DB, userID, name, departmentID string) {
	t.Helper()

	user := &database.User{
		UserID:       userID,
		Name:         name,
		Email:        userID + "@example.com",
		Mobile:       "1380000" + userID,
		DepartmentID: departmentID,
		Status:       "active",
	}

	if err := db.Create(user).Error; err != nil {
		t.Fatalf("create user %s: %v", userID, err)
	}
}

func createTestProfile(t *testing.T, db *gorm.DB, userID string) {
	t.Helper()

	profile := &database.EmployeeProfile{
		UserID:        userID,
		EmployeeID:    userID,
		ProfileStatus: "active",
	}

	if err := db.Create(profile).Error; err != nil {
		t.Fatalf("create profile %s: %v", userID, err)
	}
}

func TestFindSyncedEmployeesExcludesLocalUsers(t *testing.T) {
	db := setupUserRepositoryTestDB(t)
	repo := NewUserRepository(db)

	createTestUser(t, db, "admin", "管理员", "local")
	createTestUser(t, db, "local-only", "本地用户", "local")
	createTestUser(t, db, "ding-1", "钉钉员工1", "dept-a")
	createTestUser(t, db, "ding-2", "钉钉员工2", "dept-b")

	createTestProfile(t, db, "ding-1")
	createTestProfile(t, db, "ding-2")

	users, total, err := repo.FindSyncedEmployees(1, 10)
	if err != nil {
		t.Fatalf("find synced employees: %v", err)
	}

	if total != 2 {
		t.Fatalf("expected total 2 synced employees, got %d", total)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(users))
	}

	got := map[string]bool{}
	for _, user := range users {
		got[user.UserID] = true
	}

	if !got["ding-1"] || !got["ding-2"] {
		t.Fatalf("expected ding-1 and ding-2 in result, got %#v", got)
	}

	if got["admin"] || got["local-only"] {
		t.Fatalf("local users should be excluded, got %#v", got)
	}
}

func TestFindSyncedEmployeesByDepartment(t *testing.T) {
	db := setupUserRepositoryTestDB(t)
	repo := NewUserRepository(db)

	createTestUser(t, db, "ding-a-1", "部门A员工1", "dept-a")
	createTestUser(t, db, "ding-a-2", "部门A员工2", "dept-a")
	createTestUser(t, db, "ding-b-1", "部门B员工1", "dept-b")
	createTestUser(t, db, "local-a", "本地A", "dept-a")

	createTestProfile(t, db, "ding-a-1")
	createTestProfile(t, db, "ding-a-2")
	createTestProfile(t, db, "ding-b-1")

	users, total, err := repo.FindSyncedEmployeesByDepartment("dept-a", 1, 10)
	if err != nil {
		t.Fatalf("find synced employees by department: %v", err)
	}

	if total != 2 {
		t.Fatalf("expected total 2 department employees, got %d", total)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(users))
	}

	for _, user := range users {
		if user.DepartmentID != "dept-a" {
			t.Fatalf("unexpected department %s for user %s", user.DepartmentID, user.UserID)
		}
		if user.UserID == "local-a" {
			t.Fatalf("local user should not be returned")
		}
	}
}
