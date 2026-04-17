package service

import (
	"testing"
	"peopleops/internal/database"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestUserService_GetUsers(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	db, err := gorm.Open(mysql.Open("root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{})
	if err != nil {
		t.Skipf("Cannot connect to test DB: %v (is MySQL running?)", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	err = db.AutoMigrate(&database.User{})
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	userService := NewUserService(db)

	t.Run("GetUsers", func(t *testing.T) {
		users, total, err := userService.GetUsers(1, 10)
		assert.NoError(t, err)
		assert.NotNil(t, users)
		assert.GreaterOrEqual(t, total, int64(0))
	})
}

func TestUserService_GetUser(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}
	db, err := gorm.Open(mysql.Open("root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"), &gorm.Config{})
	if err != nil {
		t.Skipf("Cannot connect to test DB: %v (is MySQL running?)", err)
	}
	defer func() {
		sqlDB, _ := db.DB()
		sqlDB.Close()
	}()

	err = db.AutoMigrate(&database.User{})
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	userService := NewUserService(db)

	t.Run("GetUser", func(t *testing.T) {
		testUser := &database.User{
			UserID:       "test_user_id",
			Name:         "测试用户",
			Email:        "test@example.com",
			Mobile:       "13800138000",
			DepartmentID: "1",
			Position:     "工程师",
			Status:       "active",
		}
		db.Create(testUser)
		defer db.Delete(testUser)

		user, err := userService.GetUserByID("1")
		assert.NoError(t, err)
		assert.NotNil(t, user)
	})
}