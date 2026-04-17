package repository

import (
	"peopleops/internal/database"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var testDB *gorm.DB

func setupTestDB() error {
	dsn := "root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	testDB = db
	return testDB.AutoMigrate(&database.User{})
}

func teardownTestDB() error {
	if testDB == nil {
		return nil
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func TestUserRepository_Create(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if err := setupTestDB(); err != nil {
		t.Skipf("Setup test DB failed: %v (is MySQL running?)", err)
	}
	defer teardownTestDB()

	repo := NewUserRepository(testDB)

	user := &database.User{
		UserID:       "test_user_id",
		Name:         "Test User",
		Email:        "test@example.com",
		Mobile:       "13800138000",
		DepartmentID: "1",
		Position:     "Test Position",
		Avatar:       "",
		Status:       "active",
		Extension:    map[string]interface{}{},
	}

	if err := repo.Create(user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	var createdUser database.User
	if err := testDB.Where("user_id = ?", user.UserID).First(&createdUser).Error; err != nil {
		t.Fatalf("Find created user failed: %v", err)
	}

	if createdUser.Name != user.Name {
		t.Errorf("Expected name %s, got %s", user.Name, createdUser.Name)
	}
}

func TestUserRepository_FindByUserID(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if err := setupTestDB(); err != nil {
		t.Skipf("Setup test DB failed: %v (is MySQL running?)", err)
	}
	defer teardownTestDB()

	repo := NewUserRepository(testDB)

	user := &database.User{
		UserID:       "test_user_id_find",
		Name:         "Test User Find",
		Email:        "test_find@example.com",
		Mobile:       "13800138001",
		DepartmentID: "1",
		Position:     "Test Position",
		Avatar:       "",
		Status:       "active",
		Extension:    map[string]interface{}{},
	}

	if err := repo.Create(user); err != nil {
		t.Fatalf("Create user failed: %v", err)
	}

	foundUser, err := repo.FindByUserID(user.UserID)
	if err != nil {
		t.Fatalf("Find user by userID failed: %v", err)
	}

	if foundUser.UserID != user.UserID {
		t.Errorf("Expected userID %s, got %s", user.UserID, foundUser.UserID)
	}
}

func TestUserRepository_FindAll(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	if err := setupTestDB(); err != nil {
		t.Skipf("Setup test DB failed: %v (is MySQL running?)", err)
	}
	defer teardownTestDB()

	repo := NewUserRepository(testDB)

	for i := 0; i < 5; i++ {
		user := &database.User{
			UserID:       "test_user_id_" + string(rune('a'+i)),
			Name:         "Test User " + string(rune('0'+i)),
			Email:        "test" + string(rune('0'+i)) + "@example.com",
			Mobile:       "1380013800" + string(rune('0'+i)),
			DepartmentID: "1",
			Position:     "Test Position",
			Avatar:       "",
			Status:       "active",
			Extension:    map[string]interface{}{},
		}
		if err := repo.Create(user); err != nil {
			t.Fatalf("Create user failed: %v", err)
		}
	}

	users, total, err := repo.FindAll(1, 5)
	if err != nil {
		t.Fatalf("Find all users failed: %v", err)
	}

	if len(users) > 5 {
		t.Errorf("Expected at most 5 users, got %d", len(users))
	}

	if total < 5 {
		t.Errorf("Expected at least 5 total users, got %d", total)
	}
}