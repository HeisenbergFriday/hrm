package database

import (
	"log"
	"os"
	"peopleops/internal/database"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func getDatabaseURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	if url := os.Getenv("DATABASE_URL"); url != "" {
		return url
	}
	return "root:password@tcp(localhost:3306)/peopleops_test?charset=utf8mb4&parseTime=True&loc=Local"
}

func InitTestDB() error {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "1" {
		log.Println("Skipping test DB initialization (SKIP_INTEGRATION_TESTS=1)")
		return nil
	}

	dsn := getDatabaseURL()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	if err := db.AutoMigrate(
		&database.User{},
		&database.Department{},
		&database.Attendance{},
		&database.Approval{},
		&database.Role{},
		&database.Permission{},
		&database.RolePermission{},
		&database.UserRole{},
		&database.OperationLog{},
		&database.SyncStatus{},
		&database.DingTalkBinding{},
		&database.UserSession{},
		&database.LoginLog{},
		&database.AttendanceExport{},
		&database.EmployeeProfile{},
		&database.EmployeeTransfer{},
		&database.EmployeeResignation{},
		&database.EmployeeOnboarding{},
		&database.TalentAnalysis{},
	); err != nil {
		return err
	}

	log.Println("测试数据库初始化成功")
	return nil
}

func ClearTestDB() error {
	if os.Getenv("SKIP_INTEGRATION_TESTS") == "1" {
		log.Println("Skipping test DB cleanup (SKIP_INTEGRATION_TESTS=1)")
		return nil
	}

	dsn := getDatabaseURL()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return err
	}

	tables := []string{
		"operation_logs",
		"user_roles",
		"role_permissions",
		"permissions",
		"roles",
		"approvals",
		"attendance",
		"users",
		"departments",
		"sync_status",
		"ding_talk_bindings",
		"user_sessions",
		"login_logs",
		"attendance_exports",
		"employee_profiles",
		"employee_transfers",
		"employee_resignations",
		"employee_onboardings",
		"talent_analyses",
	}

	for _, table := range tables {
		if err := db.Exec("TRUNCATE TABLE " + table + " CASCADE").Error; err != nil {
			log.Printf("Warning: Failed to truncate table %s: %v", table, err)
		}
	}

	log.Println("测试数据库清理成功")
	return nil
}