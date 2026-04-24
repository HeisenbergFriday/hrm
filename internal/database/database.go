package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func Init() error {
	dsn := os.Getenv("DATABASE_URL")

	// 打印DSN（隐藏密码）
	if dsn != "" {
		// 隐藏密码部分
		hiddenDSN := dsn
		if strings.Contains(dsn, "@") {
			parts := strings.Split(dsn, "@")
			if len(parts) == 2 {
				userPass := parts[0]
				if strings.Contains(userPass, ":") {
					user := strings.Split(userPass, ":")[0]
					hiddenDSN = user + ":***@" + parts[1]
				}
			}
		}
		log.Printf("数据库连接字符串: %s", hiddenDSN)
	} else {
		log.Println("警告: DATABASE_URL 环境变量未设置")
	}

	// 尝试连接数据库
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Info),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		log.Printf("连接数据库失败: %v", err)
		// 尝试创建数据库
		if err := createDatabase(dsn); err != nil {
			log.Printf("创建数据库失败: %v", err)
			return err
		}
		// 重新连接
		log.Println("重新连接数据库...")
		db, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger:                                   logger.Default.LogMode(logger.Info),
			DisableForeignKeyConstraintWhenMigrating: true,
		})
		if err != nil {
			log.Printf("重新连接数据库失败: %v", err)
			return err
		}
	}

	DB = db
	log.Println("数据库连接成功")

	// 先独立补列，与 migrate() 成败无关，防止 main.go 吞错误后列仍缺失
	migrateAnnualLeaveGrantColumns()

	// 自动迁移表结构
	log.Println("开始迁移表结构...")
	if err := migrate(); err != nil {
		log.Printf("迁移表结构失败: %v", err)
		return err
	}
	log.Println("表结构迁移成功")

	// 种子数据
	log.Println("开始填充种子数据...")
	seed()
	log.Println("种子数据填充完成")

	return nil
}

// createDatabase 创建数据库
func createDatabase(dsn string) error {
	// 解析DSN获取数据库名称
	parts := strings.Split(dsn, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid DSN format")
	}
	dbName := strings.Split(parts[1], "?")[0]

	// 创建不带数据库名称的DSN
	baseDSN := strings.Split(dsn, "/")[0] + "/"

	// 连接到MySQL服务器
	db, err := sql.Open("mysql", baseDSN)
	if err != nil {
		return err
	}
	defer db.Close()

	// 创建数据库
	_, err = db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", dbName))
	return err
}

func migrate() error {
	DB.Exec("SET FOREIGN_KEY_CHECKS = 0")
	defer DB.Exec("SET FOREIGN_KEY_CHECKS = 1")

	// statutory_holidays 手动建表，避免 GORM AutoMigrate 的索引 DROP FOREIGN KEY 问题
	if err := DB.Exec("CREATE TABLE IF NOT EXISTS `statutory_holidays` (`id` bigint unsigned AUTO_INCREMENT PRIMARY KEY, `date` varchar(32) NOT NULL, `name` varchar(128) NOT NULL, `type` varchar(32) NOT NULL, `year` int NOT NULL, `created_at` datetime(3), `updated_at` datetime(3), UNIQUE INDEX `uni_statutory_holidays_date` (`date`))").Error; err != nil {
		return err
	}
	if err := DB.Exec("CREATE TABLE IF NOT EXISTS `employee_shift_configs` (`id` bigint unsigned AUTO_INCREMENT PRIMARY KEY, `created_at` datetime(3), `updated_at` datetime(3), `deleted_at` datetime(3), `user_id` varchar(64) NOT NULL, `user_name` varchar(128), `shift_id` bigint NOT NULL, `end_time` varchar(16), `note` varchar(256), UNIQUE INDEX `idx_employee_shift_configs_user_id` (`user_id`), INDEX `idx_employee_shift_configs_deleted_at` (`deleted_at`))").Error; err != nil {
		return err
	}
	if err := DB.Exec("CREATE TABLE IF NOT EXISTS `dingtalk_shift_catalogs` (`id` bigint unsigned AUTO_INCREMENT PRIMARY KEY, `name` varchar(128) NOT NULL, `shift_key` varchar(256) NOT NULL, `shift_id` bigint NOT NULL, `check_in` varchar(16), `check_out` varchar(16), `created_at` datetime(3), `updated_at` datetime(3), UNIQUE INDEX `idx_dingtalk_shift_catalogs_shift_key` (`shift_key`), INDEX `idx_dingtalk_shift_catalogs_name` (`name`))").Error; err != nil {
		return err
	}

	// 建新表（年假/调休）优先，不依赖其他表
	if err := DB.AutoMigrate(
		&LeaveRuleConfig{},
		&AnnualLeaveEligibility{},
		&AnnualLeaveGrant{},
		&OvertimeRuleConfig{},
		&OvertimeMatchResult{},
		&CompensatoryLeaveLedger{},
	); err != nil {
		return err
	}

	// AnnualLeaveConsumeLog 单独迁移，失败只打日志不阻断
	if err := DB.AutoMigrate(&AnnualLeaveConsumeLog{}); err != nil {
		log.Printf("[migrate] AnnualLeaveConsumeLog 迁移失败（忽略）: %v", err)
	}

	// WeekScheduleRule 建唯一索引前先去重，避免历史重复数据导致迁移失败
	deduplicateWeekScheduleRules()

	if err := DB.AutoMigrate(
		&User{},
		&Department{},
		&Attendance{},
		&Approval{},
		&ApprovalTemplate{},
		&Role{},
		&Permission{},
		&RolePermission{},
		&UserRole{},
		&OperationLog{},
		&SyncStatus{},
		&DingTalkBinding{},
		&UserSession{},
		&LoginLog{},
		&AttendanceExport{},
		&EmployeeProfile{},
		&EmployeeTransfer{},
		&EmployeeResignation{},
		&EmployeeOnboarding{},
		&TalentAnalysis{},
		&EmployeeShiftConfig{},
		&DingTalkShiftCatalog{},
		&WeekScheduleRule{},
		&WeekScheduleOverride{},
		&WeekScheduleSyncLog{},
	); err != nil {
		return err
	}

	if err := migrateShiftCatalogSchema(); err != nil {
		return err
	}

	return cleanupDeletedWeekScheduleRules()
}

func migrateAnnualLeaveGrantColumns() {
	if !DB.Migrator().HasTable(&AnnualLeaveGrant{}) {
		return
	}
	type col struct {
		name string
		ddl  string
	}
	cols := []col{
		{"dingtalk_sync_status", "ALTER TABLE annual_leave_grants ADD COLUMN dingtalk_sync_status varchar(32) DEFAULT 'pending'"},
		{"dingtalk_sync_error", "ALTER TABLE annual_leave_grants ADD COLUMN dingtalk_sync_error text"},
		{"dingtalk_synced_at", "ALTER TABLE annual_leave_grants ADD COLUMN dingtalk_synced_at datetime(3)"},
	}
	for _, c := range cols {
		var count int64
		DB.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='annual_leave_grants' AND COLUMN_NAME=?", c.name).Scan(&count)
		if count == 0 {
			if err := DB.Exec(c.ddl).Error; err != nil {
				log.Printf("[migrate] 添加列 %s 失败: %v", c.name, err)
			}
		}
	}
}

func migrateShiftCatalogSchema() error {
	if !DB.Migrator().HasColumn(&DingTalkShiftCatalog{}, "ShiftKey") {
		if err := DB.Migrator().AddColumn(&DingTalkShiftCatalog{}, "ShiftKey"); err != nil {
			return err
		}
	}

	var catalogs []DingTalkShiftCatalog
	if err := DB.Find(&catalogs).Error; err != nil {
		return err
	}
	for _, catalog := range catalogs {
		shiftKey := normalizeShiftCatalogKey(catalog.Name, catalog.CheckIn, catalog.CheckOut)
		if shiftKey == "" || catalog.ShiftKey == shiftKey {
			continue
		}
		if err := DB.Model(&DingTalkShiftCatalog{}).
			Where("id = ?", catalog.ID).
			Update("shift_key", shiftKey).Error; err != nil {
			return err
		}
	}

	if DB.Migrator().HasIndex(&DingTalkShiftCatalog{}, "idx_dingtalk_shift_catalogs_name") {
		if err := DB.Migrator().DropIndex(&DingTalkShiftCatalog{}, "idx_dingtalk_shift_catalogs_name"); err != nil {
			return err
		}
	}
	if err := DB.Exec("CREATE INDEX `idx_dingtalk_shift_catalogs_name` ON `dingtalk_shift_catalogs` (`name`)").Error; err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
		return err
	}
	if !DB.Migrator().HasIndex(&DingTalkShiftCatalog{}, "idx_dingtalk_shift_catalogs_shift_key") {
		if err := DB.Exec("CREATE UNIQUE INDEX `idx_dingtalk_shift_catalogs_shift_key` ON `dingtalk_shift_catalogs` (`shift_key`)").Error; err != nil {
			return err
		}
	}

	return nil
}

func normalizeShiftCatalogKey(name, checkIn, checkOut string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + strings.TrimSpace(checkIn) + "|" + strings.TrimSpace(checkOut)
}

func cleanupDeletedWeekScheduleRules() error {
	return DB.Unscoped().
		Where("deleted_at IS NOT NULL").
		Delete(&WeekScheduleRule{}).Error
}

// deduplicateWeekScheduleRules 在建唯一索引前去除 (scope_type, scope_id) 重复行，保留 id 最大的一条
func deduplicateWeekScheduleRules() {
	// 检查表是否存在
	if !DB.Migrator().HasTable(&WeekScheduleRule{}) {
		return
	}
	// 检查唯一索引是否已存在（已有索引则不需要去重）
	if DB.Migrator().HasIndex(&WeekScheduleRule{}, "idx_scope") {
		return
	}
	if err := DB.Exec(`
		DELETE w1 FROM week_schedule_rules w1
		INNER JOIN week_schedule_rules w2
		ON w1.scope_type = w2.scope_type AND w1.scope_id = w2.scope_id AND w1.id < w2.id
	`).Error; err != nil {
		log.Printf("[migrate] 去重 week_schedule_rules 失败（忽略）: %v", err)
	}
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// CheckPassword 校验密码
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

func seed() {
	// 创建默认管理员（如果不存在）
	var count int64
	DB.Model(&User{}).Where("user_id = ?", "admin").Count(&count)
	if count == 0 {
		hash, err := HashPassword("admin123")
		if err != nil {
			log.Printf("生成密码哈希失败: %v", err)
			return
		}
		admin := User{
			UserID:       "admin",
			Name:         "管理员",
			Email:        "admin@peopleops.local",
			Mobile:       "10000000000",
			Password:     hash,
			DepartmentID: "1",
			Position:     "系统管理员",
			Status:       "active",
		}
		if err := DB.Create(&admin).Error; err != nil {
			log.Printf("创建默认管理员失败: %v", err)
		} else {
			log.Println("已创建默认管理员账号: admin / admin123")
		}
	}

	// 创建默认部门（如果不存在）
	DB.Model(&Department{}).Count(&count)
	if count == 0 {
		departments := []Department{
			{DepartmentID: "1", Name: "总公司", ParentID: "0", Order: 1},
			{DepartmentID: "2", Name: "技术部", ParentID: "1", Order: 1},
			{DepartmentID: "3", Name: "前端组", ParentID: "2", Order: 1},
			{DepartmentID: "4", Name: "后端组", ParentID: "2", Order: 2},
			{DepartmentID: "5", Name: "市场部", ParentID: "1", Order: 2},
		}
		for _, dept := range departments {
			if err := DB.Create(&dept).Error; err != nil {
				log.Printf("创建默认部门失败: %v", err)
			}
		}
		log.Println("已创建默认部门数据")
	}

	// 创建默认角色（如果不存在）
	DB.Model(&Role{}).Count(&count)
	if count == 0 {
		roles := []Role{
			{Name: "管理员", Description: "系统管理员"},
			{Name: "部门负责人", Description: "部门负责人"},
			{Name: "普通员工", Description: "普通员工"},
		}
		for _, role := range roles {
			DB.Create(&role)
		}
		log.Println("已创建默认角色数据")
	}

	// 创建默认权限（如果不存在）
	DB.Model(&Permission{}).Count(&count)
	if count == 0 {
		permissions := []Permission{
			{Name: "用户管理", Code: "user_manage", Description: "用户管理权限"},
			{Name: "部门管理", Code: "department_manage", Description: "部门管理权限"},
			{Name: "考勤管理", Code: "attendance_manage", Description: "考勤管理权限"},
			{Name: "审批管理", Code: "approval_manage", Description: "审批管理权限"},
			{Name: "权限管理", Code: "permission_manage", Description: "权限管理权限"},
		}
		for _, perm := range permissions {
			DB.Create(&perm)
		}
		log.Println("已创建默认权限数据")
	}
}
