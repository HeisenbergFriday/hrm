package service

import (
	"testing"
	"peopleops/internal/database"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestDepartmentService_GetAllDepartments(t *testing.T) {
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

	err = db.AutoMigrate(&database.Department{})
	if err != nil {
		t.Fatalf("AutoMigrate failed: %v", err)
	}

	departmentService := NewDepartmentService(db)

	t.Run("GetAllDepartments", func(t *testing.T) {
		departments, err := departmentService.GetAllDepartments()
		assert.NoError(t, err)
		assert.NotNil(t, departments)
	})
}