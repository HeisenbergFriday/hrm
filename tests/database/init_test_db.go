package database

import (
	"log"
	"os"
	"peopleops/internal/database"
	"strings"

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

func testModels() []interface{} {
	return []interface{}{
		&database.LeaveRuleConfig{},
		&database.AnnualLeaveEligibility{},
		&database.AnnualLeaveGrant{},
		&database.OvertimeRuleConfig{},
		&database.OvertimeMatchResult{},
		&database.OvertimeSyncHistory{},
		&database.OvertimeSupplementaryRequest{},
		&database.CompensatoryLeaveLedger{},
		&database.AnnualLeaveConsumeLog{},
		&database.User{},
		&database.Department{},
		&database.DepartmentChangeLog{},
		&database.Attendance{},
		&database.Approval{},
		&database.ApprovalTemplate{},
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
		&database.EmployeeShiftConfig{},
		&database.DingTalkShiftCatalog{},
		&database.WeekScheduleRule{},
		&database.WeekScheduleOverride{},
		&database.WeekScheduleSyncLog{},
		&database.PerformanceTemplate{},
		&database.PerformanceTemplateSection{},
		&database.PerformanceTemplateItem{},
		&database.PerformanceLevelRule{},
		&database.PerformanceLevelRuleItem{},
		&database.PerformanceActivity{},
		&database.PerformanceDistributionRule{},
		&database.PerformanceDistributionException{},
		&database.PerformanceParticipant{},
		&database.PerformanceReview{},
		&database.PerformanceReviewVersion{},
		&database.PerformanceRelationshipChangeLog{},
		&database.PerformanceGoalRecord{},
		&database.PerformanceGoalApprovalLog{},
		&database.PerformanceCompanyFinance{},
		&database.PerformanceIndicatorLibrary{},
		&database.PerformanceIndicatorItem{},
	}
}

func quoteIdentifier(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
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

	if err := db.Exec("CREATE TABLE IF NOT EXISTS `statutory_holidays` (`id` bigint unsigned AUTO_INCREMENT PRIMARY KEY, `date` varchar(32) NOT NULL, `name` varchar(128) NOT NULL, `type` varchar(32) NOT NULL, `year` int NOT NULL, `created_at` datetime(3), `updated_at` datetime(3), UNIQUE INDEX `uni_statutory_holidays_date` (`date`))").Error; err != nil {
		return err
	}

	if err := db.AutoMigrate(testModels()...); err != nil {
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

	var tables []string
	if err := db.Raw("SHOW TABLES").Scan(&tables).Error; err != nil {
		return err
	}

	if err := db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
		return err
	}
	defer db.Exec("SET FOREIGN_KEY_CHECKS = 1")

	for _, table := range tables {
		if err := db.Exec("TRUNCATE TABLE " + quoteIdentifier(table)).Error; err != nil {
			log.Printf("Warning: Failed to truncate table %s: %v", table, err)
		}
	}

	log.Println("测试数据库清理成功")
	return nil
}
