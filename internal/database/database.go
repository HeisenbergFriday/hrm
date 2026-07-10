package database

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func generateRandomPassword(length int) string {
	b := make([]byte, length/2)
	if _, err := rand.Read(b); err != nil {
		// fallback: 极端情况用固定字符串
		panic(fmt.Sprintf("crypto random unavailable: %v", err))
	}
	return hex.EncodeToString(b)
}

var DB *gorm.DB

func gormConfig() *gorm.Config {
	baseLogger := logger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), logger.Config{
		LogLevel:                  logger.Info,
		IgnoreRecordNotFoundError: true,
	})
	return &gorm.Config{
		Logger:                                   newRequestLogger(baseLogger),
		DisableForeignKeyConstraintWhenMigrating: true,
	}
}

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
	db, err := gorm.Open(mysql.Open(dsn), gormConfig())
	if err != nil {
		log.Printf("连接数据库失败: %v", err)
		// 尝试创建数据库
		if err := createDatabase(dsn); err != nil {
			log.Printf("创建数据库失败: %v", err)
			return err
		}
		// 重新连接
		log.Println("重新连接数据库...")
		db, err = gorm.Open(mysql.Open(dsn), gormConfig())
		if err != nil {
			log.Printf("重新连接数据库失败: %v", err)
			return err
		}
	}

	DB = db
	log.Println("数据库连接成功")

	// 先独立补列，与 migrate() 成败无关，防止 main.go 吞错误后列仍缺失
	migrateAnnualLeaveGrantColumns()
	migrateUserManagerColumns()

	// 自动迁移表结构
	log.Println("开始迁移表结构...")
	if err := migrate(); err != nil {
		log.Printf("迁移表结构失败: %v", err)
		return err
	}
	if err := migrateMultitenantUniqueIndexes(); err != nil {
		log.Printf("迁移多租户唯一索引失败: %v", err)
		return err
	}
	log.Println("表结构迁移成功")

	// 种子数据
	log.Println("开始填充种子数据...")
	seed()
	log.Println("种子数据填充完成")

	// 增量迁移：为已有部署补充新权限码
	migratePermissions()
	migratePerformanceIndicatorRolePresets()
	migrateMenuPermissions()
	migrateAttendanceToolboxMenuPermissions()
	migrateLiedeOrganizationAdminRoles()

	// 绩效表已随主库 migrate() 一并迁移，无需独立数据源
	log.Println("绩效模块使用主库")

	return nil
}

// GetPerformanceDB 获取绩效模块的数据源（统一使用主库）
func GetPerformanceDB() *gorm.DB {
	return DB
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
		&OvertimeSyncHistory{},
		&OvertimeSupplementaryRequest{},
		&CompensatoryLeaveLedger{},
	); err != nil {
		return err
	}

	// AnnualLeaveConsumeLog 单独迁移，失败只打日志不阻断
	if err := DB.AutoMigrate(&AnnualLeaveConsumeLog{}); err != nil {
		log.Printf("[migrate] AnnualLeaveConsumeLog 迁移失败（忽略）: %v", err)
	}
	if err := migrateAnnualLeaveConsumeLogSchema(); err != nil {
		return err
	}
	if err := migrateOvertimeMatchSchema(); err != nil {
		return err
	}
	if err := migrateAnnualLeaveGrantIndexes(); err != nil {
		return err
	}

	// WeekScheduleRule 建唯一索引前先去重，避免历史重复数据导致迁移失败
	deduplicateWeekScheduleRules()
	if err := migrateUserRolesSingleRole(); err != nil {
		return err
	}

	if err := DB.AutoMigrate(
		&Organization{},
		&OrganizationUser{},
		&User{},
		&Department{},
		&DepartmentChangeLog{},
		&Attendance{},
		&Approval{},
		&ApprovalTemplate{},
		&Role{},
		&Permission{},
		&RolePermission{},
		&UserRole{},
		&MenuPermission{},
		&DataPermission{},
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
		&PerformanceTemplate{},
		&PerformanceTemplateSection{},
		&PerformanceTemplateItem{},
		&PerformanceLevelRule{},
		&PerformanceLevelRuleItem{},
		&PerformanceActivity{},
		&PerformanceDistributionRule{},
		&PerformanceDistributionException{},
		&PerformanceReminderLog{},
		&PerformanceParticipant{},
		&PerformanceReview{},
		&PerformanceReviewVersion{},
		&PerformanceRelationshipChangeLog{},
		&PerformanceGoalRecord{},
		&PerformanceGoalApprovalLog{},
		&PerformanceCompanyFinance{},
		&PerformanceIndicatorLibrary{},
		&PerformanceIndicatorItem{},
	); err != nil {
		return err
	}

	if err := migrateUserMobileUniqueIndex(); err != nil {
		return err
	}
	if err := migrateRoleNameUniqueIndex(); err != nil {
		return err
	}
	if err := migrateShiftCatalogSchema(); err != nil {
		return err
	}
	if err := migratePerformanceReviewVersionSchema(); err != nil {
		return err
	}
	if err := migratePerformanceParticipantAssessmentManagerSchema(); err != nil {
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

func migratePerformanceParticipantAssessmentManagerSchema() error {
	if !DB.Migrator().HasTable(&PerformanceParticipant{}) {
		return nil
	}

	// Historical manager_id / manager_name are already the activity assessment manager.
	// Only backfill new metadata columns; never derive manager_id from current users here.
	if err := DB.Model(&PerformanceParticipant{}).
		Where("(manager_source IS NULL OR manager_source = '') AND deleted_at IS NULL").
		Update("manager_source", "SYSTEM").Error; err != nil {
		return err
	}
	if err := DB.Model(&PerformanceParticipant{}).
		Where("manager_overridden IS NULL AND deleted_at IS NULL").
		Update("manager_overridden", false).Error; err != nil {
		return err
	}
	if err := DB.Model(&PerformanceParticipant{}).
		Where("(manager_config_status IS NULL OR manager_config_status = '') AND deleted_at IS NULL").
		Update("manager_config_status", gorm.Expr("CASE WHEN manager_id IS NULL OR manager_id = '' THEN 'PENDING' ELSE 'CONFIGURED' END")).Error; err != nil {
		return err
	}
	return nil
}

func migratePerformanceReviewVersionSchema() error {
	if !DB.Migrator().HasTable(&PerformanceReviewVersion{}) {
		return nil
	}

	var isNullable string
	if err := DB.Raw(`
		SELECT IS_NULLABLE
		FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'performance_review_versions'
		  AND COLUMN_NAME = 'confirmed_at'
	`).Scan(&isNullable).Error; err != nil {
		return err
	}

	if strings.EqualFold(strings.TrimSpace(isNullable), "NO") {
		if err := DB.Exec("ALTER TABLE performance_review_versions MODIFY COLUMN confirmed_at datetime(3) NULL").Error; err != nil {
			return err
		}
	}

	return nil
}

func migrateUserManagerColumns() {
	if !DB.Migrator().HasTable(&User{}) {
		return
	}
	type col struct {
		name string
		ddl  string
	}
	cols := []col{
		{"manager_user_id", "ALTER TABLE users ADD COLUMN manager_user_id varchar(64)"},
		{"manager_name", "ALTER TABLE users ADD COLUMN manager_name varchar(128)"},
	}
	for _, c := range cols {
		var count int64
		DB.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='users' AND COLUMN_NAME=?", c.name).Scan(&count)
		if count == 0 {
			if err := DB.Exec(c.ddl).Error; err != nil {
				log.Printf("[migrate] 添加列 %s 失败: %v", c.name, err)
			} else {
				log.Printf("[migrate] 成功添加列 users.%s", c.name)
			}
		}
	}
	// 添加索引
	var idxCount int64
	DB.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='users' AND INDEX_NAME='idx_users_manager_user_id'").Scan(&idxCount)
	if idxCount == 0 {
		if err := DB.Exec("CREATE INDEX idx_users_manager_user_id ON users(manager_user_id)").Error; err != nil {
			log.Printf("[migrate] 添加索引 idx_users_manager_user_id 失败: %v", err)
		} else {
			log.Printf("[migrate] 成功添加索引 idx_users_manager_user_id")
		}
	}
}

func migrateMultitenantUniqueIndexes() error {
	if DB == nil {
		return nil
	}

	if DB.Migrator().HasTable(&EmployeeProfile{}) {
		if err := DB.Exec(`
			UPDATE employee_profiles ep
			JOIN (
				SELECT user_id, MIN(org_id) AS org_id
				FROM users
				WHERE deleted_at IS NULL
				GROUP BY user_id
				HAVING COUNT(*) = 1
			) u ON u.user_id = ep.user_id
			SET ep.org_id = u.org_id
			WHERE ep.org_id IS NULL OR ep.org_id = '' OR ep.org_id = 'default'
		`).Error; err != nil {
			return err
		}
	}

	// 考勤表补齐 org_id：钉钉 UserID 在不同企业可能重号，唯一索引和过滤都必须带上 org_id。
	// 回填规则：对同一 user_id 只在一个组织出现的考勤记录，直接采用该组织；否则保持 default 兜底。
	if DB.Migrator().HasTable(&Attendance{}) {
		if err := DB.Exec(`
			UPDATE attendances a
			JOIN (
				SELECT user_id, MIN(org_id) AS org_id
				FROM users
				WHERE deleted_at IS NULL
				GROUP BY user_id
				HAVING COUNT(*) = 1
			) u ON u.user_id = a.user_id
			SET a.org_id = u.org_id
			WHERE a.org_id IS NULL OR a.org_id = '' OR a.org_id = 'default'
		`).Error; err != nil {
			return err
		}
	}

	for _, idx := range []struct {
		table string
		name  string
	}{
		{"users", "uni_users_mobile"},
		{"users", "uni_users_email"},
		{"users", "idx_org_email"},
		{"employee_profiles", "uni_employee_profiles_user_id"},
		{"employee_profiles", "uni_employee_profiles_employee_id"},
		{"attendances", "idx_user_time_type"},
	} {
		if err := dropIndexIfExists(idx.table, idx.name); err != nil {
			return err
		}
	}

	if err := createIndexIfMissing("employee_profiles", "idx_employee_profiles_org_user", true, "org_id", "user_id"); err != nil {
		return err
	}
	if err := createIndexIfMissing("employee_profiles", "idx_employee_profiles_org_employee", true, "org_id", "employee_id"); err != nil {
		return err
	}
	if err := createIndexIfMissing("users", "idx_org_email", true, "org_id", "email"); err != nil {
		return err
	}
	if err := createIndexIfMissing("users", "idx_users_org_mobile", false, "org_id", "mobile"); err != nil {
		return err
	}
	if err := createIndexIfMissing("attendances", "idx_org_user_time_type", true, "org_id", "user_id", "check_time", "check_type"); err != nil {
		return err
	}

	return nil
}

func dropIndexIfExists(table, indexName string) error {
	var count int64
	if err := DB.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME=?", table, indexName).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return nil
	}
	return DB.Exec(fmt.Sprintf("ALTER TABLE `%s` DROP INDEX `%s`", table, indexName)).Error
}

func createIndexIfMissing(table, indexName string, unique bool, columns ...string) error {
	var count int64
	if err := DB.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME=?", table, indexName).Scan(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	quotedColumns := make([]string, 0, len(columns))
	for _, column := range columns {
		quotedColumns = append(quotedColumns, fmt.Sprintf("`%s`", column))
	}
	uniqueSQL := ""
	if unique {
		uniqueSQL = "UNIQUE "
	}
	return DB.Exec(fmt.Sprintf("CREATE %sINDEX `%s` ON `%s` (%s)", uniqueSQL, indexName, table, strings.Join(quotedColumns, ", "))).Error
}

func migrateAnnualLeaveConsumeLogSchema() error {
	if !DB.Migrator().HasTable(&AnnualLeaveConsumeLog{}) {
		return nil
	}
	if !DB.Migrator().HasColumn(&AnnualLeaveConsumeLog{}, "RequestRef") {
		if err := DB.Migrator().AddColumn(&AnnualLeaveConsumeLog{}, "RequestRef"); err != nil {
			return err
		}
	}
	if err := DB.Exec(`
		UPDATE annual_leave_consume_logs
		SET request_ref = CASE
			WHEN approval_ref IS NULL OR approval_ref = '' THEN CONCAT('legacy:', id)
			ELSE CONCAT('approval:', approval_ref)
		END
		WHERE request_ref IS NULL OR request_ref = ''
	`).Error; err != nil {
		return err
	}
	if oldIndex, err := findUniqueIndexByColumn("annual_leave_consume_logs", "approval_ref"); err != nil {
		return err
	} else if oldIndex != "" {
		if err := DB.Migrator().DropIndex(&AnnualLeaveConsumeLog{}, oldIndex); err != nil {
			return err
		}
	}
	if !DB.Migrator().HasIndex(&AnnualLeaveConsumeLog{}, "idx_leave_consume_approval_ref") {
		if err := DB.Exec("CREATE INDEX `idx_leave_consume_approval_ref` ON `annual_leave_consume_logs` (`approval_ref`)").Error; err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
			return err
		}
	}
	if !DB.Migrator().HasIndex(&AnnualLeaveConsumeLog{}, "idx_leave_consume_request_grant") {
		if err := DB.Exec("CREATE UNIQUE INDEX `idx_leave_consume_request_grant` ON `annual_leave_consume_logs` (`request_ref`, `grant_id`)").Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateAnnualLeaveGrantIndexes() error {
	if !DB.Migrator().HasTable(&AnnualLeaveGrant{}) || DB.Migrator().HasIndex(&AnnualLeaveGrant{}, "idx_leave_grant_user_year_q_type") {
		return nil
	}
	type duplicateGrant struct {
		UserID    string
		Year      int
		Quarter   int
		GrantType string
		Count     int64
	}
	var duplicates []duplicateGrant
	if err := DB.Raw(`
		SELECT user_id, year, quarter, grant_type, COUNT(*) AS count
		FROM annual_leave_grants
		GROUP BY user_id, year, quarter, grant_type
		HAVING COUNT(*) > 1
		LIMIT 10
	`).Scan(&duplicates).Error; err != nil {
		return err
	}
	if len(duplicates) > 0 {
		log.Printf("[migrate] 跳过创建 idx_leave_grant_user_year_q_type，发现重复年假发放记录: %+v", duplicates)
		return nil
	}
	return DB.Exec("CREATE UNIQUE INDEX `idx_leave_grant_user_year_q_type` ON `annual_leave_grants` (`user_id`, `year`, `quarter`, `grant_type`)").Error
}

func migrateUserMobileUniqueIndex() error {
	if !DB.Migrator().HasTable(&User{}) {
		return nil
	}

	var indexCount int64
	if err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'users'
		  AND INDEX_NAME = 'uni_users_mobile'
	`).Scan(&indexCount).Error; err != nil {
		return err
	}
	if indexCount > 0 {
		return nil
	}

	type duplicateMobile struct {
		Mobile string
		Count  int64
	}
	var duplicates []duplicateMobile
	if err := DB.Raw(`
		SELECT mobile, COUNT(*) AS count
		FROM users
		WHERE mobile IS NOT NULL
		GROUP BY mobile
		HAVING COUNT(*) > 1
		LIMIT 5
	`).Scan(&duplicates).Error; err != nil {
		return err
	}
	if len(duplicates) > 0 {
		log.Printf("[migrate] 跳过创建 uni_users_mobile，发现 %d 个重复手机号样本，请先清理历史 users.mobile 数据", len(duplicates))
		return nil
	}

	if err := DB.Exec("CREATE UNIQUE INDEX `uni_users_mobile` ON `users` (`mobile`)").Error; err != nil {
		lowerErr := strings.ToLower(err.Error())
		if strings.Contains(lowerErr, "duplicate entry") || strings.Contains(lowerErr, "duplicate key name") {
			log.Printf("[migrate] 跳过创建 uni_users_mobile: %v", err)
			return nil
		}
		return err
	}
	return nil
}

func migrateRoleNameUniqueIndex() error {
	if !DB.Migrator().HasTable(&Role{}) {
		return nil
	}

	var indexCount int64
	if err := DB.Raw(`
		SELECT COUNT(*)
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'roles'
		  AND INDEX_NAME = 'uni_roles_name'
	`).Scan(&indexCount).Error; err != nil {
		return err
	}
	if indexCount > 0 {
		return nil
	}

	type duplicateRoleName struct {
		Name  string
		Count int64
	}
	var duplicates []duplicateRoleName
	if err := DB.Raw(`
		SELECT name, COUNT(*) AS count
		FROM roles
		GROUP BY name
		HAVING COUNT(*) > 1
		LIMIT 5
	`).Scan(&duplicates).Error; err != nil {
		return err
	}
	if len(duplicates) > 0 {
		log.Printf("[migrate] 跳过创建 uni_roles_name，发现 %d 个重复角色名样本，请先清理历史 roles.name 数据", len(duplicates))
		return nil
	}

	if err := DB.Exec("CREATE UNIQUE INDEX `uni_roles_name` ON `roles` (`name`)").Error; err != nil {
		lowerErr := strings.ToLower(err.Error())
		if strings.Contains(lowerErr, "duplicate entry") || strings.Contains(lowerErr, "duplicate key name") {
			log.Printf("[migrate] 跳过创建 uni_roles_name: %v", err)
			return nil
		}
		return err
	}
	return nil
}

func migrateOvertimeMatchSchema() error {
	if !DB.Migrator().HasTable(&OvertimeMatchResult{}) || !DB.Migrator().HasTable(&CompensatoryLeaveLedger{}) {
		return nil
	}
	if !DB.Migrator().HasColumn(&OvertimeMatchResult{}, "MatchRef") {
		if err := DB.Migrator().AddColumn(&OvertimeMatchResult{}, "MatchRef"); err != nil {
			return err
		}
	}
	if !DB.Migrator().HasColumn(&CompensatoryLeaveLedger{}, "SourceMatchRef") {
		if err := DB.Migrator().AddColumn(&CompensatoryLeaveLedger{}, "SourceMatchRef"); err != nil {
			return err
		}
	}
	if err := DB.Exec(`
		UPDATE overtime_match_results
		SET match_ref = CONCAT('legacy:', id)
		WHERE match_ref IS NULL OR match_ref = ''
	`).Error; err != nil {
		return err
	}
	if err := DB.Exec(`
		UPDATE compensatory_leave_ledgers
		SET source_match_ref = CASE
			WHEN source_match_id > 0 THEN CONCAT('legacy:', source_match_id)
			ELSE ''
		END
		WHERE source_match_ref IS NULL OR source_match_ref = ''
	`).Error; err != nil {
		return err
	}
	if DB.Migrator().HasTable(&OvertimeSyncHistory{}) {
		if err := DB.Exec(`
			INSERT INTO overtime_sync_histories (
				user_id,
				work_date,
				approval_id,
				approval_process_id,
				effective_overtime_minutes,
				sync_request_id,
				sync_mode,
				synced_at,
				created_at,
				updated_at
			)
			SELECT
				user_id,
				work_date,
				approval_id,
				approval_process_id,
				effective_overtime_minutes,
				CASE
					WHEN dingtalk_sync_request_id IS NULL OR dingtalk_sync_request_id = '' THEN CONCAT('legacy-sync:', user_id, ':', work_date)
					ELSE dingtalk_sync_request_id
				END,
				'backfill',
				NOW(3),
				NOW(3),
				NOW(3)
			FROM overtime_match_results
			WHERE dingtalk_sync_status = 'success' AND effective_overtime_minutes > 0
			ON DUPLICATE KEY UPDATE
				approval_id = VALUES(approval_id),
				approval_process_id = VALUES(approval_process_id),
				effective_overtime_minutes = VALUES(effective_overtime_minutes),
				sync_request_id = CASE
					WHEN overtime_sync_histories.sync_request_id IS NULL OR overtime_sync_histories.sync_request_id = '' THEN VALUES(sync_request_id)
					ELSE overtime_sync_histories.sync_request_id
				END,
				synced_at = COALESCE(overtime_sync_histories.synced_at, VALUES(synced_at)),
				updated_at = NOW(3)
		`).Error; err != nil {
			return err
		}
	}
	return nil
}

func findUniqueIndexByColumn(tableName, columnName string) (string, error) {
	type indexRow struct {
		IndexName string
	}
	var rows []indexRow
	if err := DB.Raw(`
		SELECT DISTINCT INDEX_NAME
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = ?
		  AND COLUMN_NAME = ?
		  AND NON_UNIQUE = 0
	`, tableName, columnName).Scan(&rows).Error; err != nil {
		return "", err
	}
	for _, row := range rows {
		name := strings.TrimSpace(row.IndexName)
		if name != "" {
			return name, nil
		}
	}
	return "", nil
}

func migrateUserRolesSingleRole() error {
	if !DB.Migrator().HasTable(&UserRole{}) {
		return nil
	}

	if !DB.Migrator().HasColumn(&UserRole{}, "OrgID") {
		if err := DB.Exec("ALTER TABLE `user_roles` ADD COLUMN `org_id` varchar(64) NOT NULL DEFAULT 'default'").Error; err != nil {
			return err
		}
	}

	if err := DB.Exec(`
		UPDATE user_roles ur
		LEFT JOIN (
			SELECT user_id, MIN(org_id) AS org_id
			FROM users
			WHERE deleted_at IS NULL
			GROUP BY user_id
		) u ON u.user_id = ur.user_id
		SET ur.org_id = COALESCE(NULLIF(ur.org_id, ''), u.org_id, 'default')
	`).Error; err != nil {
		return err
	}

	if err := DB.Unscoped().Where("deleted_at IS NOT NULL").Delete(&UserRole{}).Error; err != nil {
		return err
	}

	if err := DB.Exec(`
		DELETE ur
		FROM user_roles ur
		JOIN (
			SELECT org_id, user_id, MAX(id) AS keep_id
			FROM user_roles
			WHERE deleted_at IS NULL
			GROUP BY org_id, user_id
			HAVING COUNT(*) > 1
		) dup ON dup.org_id = ur.org_id AND dup.user_id = ur.user_id
		WHERE ur.deleted_at IS NULL
		  AND ur.id <> dup.keep_id
	`).Error; err != nil {
		return err
	}

	type indexInfo struct {
		IndexName string
	}
	var oldIndexes []indexInfo
	if err := DB.Raw(`
		SELECT INDEX_NAME
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'user_roles'
		  AND INDEX_NAME = 'idx_user_roles_user_id'
		GROUP BY INDEX_NAME
	`).Scan(&oldIndexes).Error; err != nil {
		return err
	}
	if len(oldIndexes) > 0 {
		if err := DB.Exec("DROP INDEX `idx_user_roles_user_id` ON `user_roles`").Error; err != nil {
			return err
		}
	}

	var newIndexes []indexInfo
	if err := DB.Raw(`
		SELECT INDEX_NAME
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'user_roles'
		  AND INDEX_NAME = 'idx_user_roles_org_user'
		GROUP BY INDEX_NAME
	`).Scan(&newIndexes).Error; err != nil {
		return err
	}
	if len(newIndexes) == 0 {
		if err := DB.Exec("CREATE UNIQUE INDEX `idx_user_roles_org_user` ON `user_roles` (`org_id`, `user_id`)").Error; err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
				return nil
			}
			return err
		}
	}
	return nil
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

type organizationSeedConfig struct {
	OrgID       string `json:"org_id"`
	Name        string `json:"name"`
	CorpID      string `json:"corp_id"`
	AppKey      string `json:"app_key"`
	AppSecret   string `json:"app_secret"`
	AgentID     string `json:"agent_id"`
	AppHomeURL  string `json:"app_home_url"`
	RedirectURI string `json:"redirect_uri"`
	Status      string `json:"status"`
}

func seedOrganizationConfigsFromEnv() []organizationSeedConfig {
	configs := make([]organizationSeedConfig, 0)
	indexByOrgID := make(map[string]int)
	add := func(cfg organizationSeedConfig) {
		cfg.OrgID = strings.TrimSpace(cfg.OrgID)
		cfg.Name = strings.TrimSpace(cfg.Name)
		cfg.CorpID = strings.TrimSpace(cfg.CorpID)
		cfg.AppKey = strings.TrimSpace(cfg.AppKey)
		cfg.AppSecret = strings.TrimSpace(cfg.AppSecret)
		cfg.AgentID = strings.TrimSpace(cfg.AgentID)
		cfg.AppHomeURL = strings.TrimSpace(cfg.AppHomeURL)
		cfg.RedirectURI = strings.TrimSpace(cfg.RedirectURI)
		cfg.Status = strings.TrimSpace(cfg.Status)
		if cfg.OrgID == "" || cfg.CorpID == "" {
			return
		}
		if cfg.Name == "" {
			cfg.Name = cfg.OrgID
		}
		if cfg.Status == "" {
			cfg.Status = "active"
		}
		if idx, ok := indexByOrgID[cfg.OrgID]; ok {
			configs[idx] = cfg
			return
		}
		indexByOrgID[cfg.OrgID] = len(configs)
		configs = append(configs, cfg)
	}

	defaultOrgID := strings.TrimSpace(os.Getenv("DINGTALK_SHARED_OAUTH_ORG_ID"))
	if defaultOrgID == "" {
		defaultOrgID = strings.TrimSpace(os.Getenv("DEFAULT_ORG_ID"))
	}
	if defaultOrgID == "" {
		defaultOrgID = strings.TrimSpace(os.Getenv("DINGTALK_QR_DEFAULT_ORG_ID"))
	}
	if defaultOrgID == "" {
		defaultOrgID = "default"
	}
	add(organizationSeedConfig{
		OrgID:       defaultOrgID,
		Name:        strings.TrimSpace(os.Getenv("DINGTALK_ORG_NAME")),
		CorpID:      strings.TrimSpace(os.Getenv("DINGTALK_CORP_ID")),
		AppKey:      strings.TrimSpace(os.Getenv("DINGTALK_APP_KEY")),
		AppSecret:   strings.TrimSpace(os.Getenv("DINGTALK_APP_SECRET")),
		AgentID:     strings.TrimSpace(os.Getenv("DINGTALK_AGENT_ID")),
		AppHomeURL:  strings.TrimSpace(os.Getenv("DINGTALK_APP_HOME_URL")),
		RedirectURI: strings.TrimSpace(os.Getenv("DINGTALK_REDIRECT_URI")),
		Status:      "active",
	})

	raw := strings.Trim(strings.TrimSpace(os.Getenv("DINGTALK_ORGANIZATIONS")), "'")
	if raw != "" {
		var envConfigs []organizationSeedConfig
		if err := json.Unmarshal([]byte(raw), &envConfigs); err != nil {
			log.Printf("[seed] parse DINGTALK_ORGANIZATIONS failed: %v", err)
		} else {
			for _, cfg := range envConfigs {
				add(cfg)
			}
		}
	}

	return configs
}

func seedOrganizationsFromEnv() {
	for _, cfg := range seedOrganizationConfigsFromEnv() {
		var org Organization
		err := DB.Where("org_id = ?", cfg.OrgID).First(&org).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			org = Organization{
				OrgID:       cfg.OrgID,
				Name:        cfg.Name,
				CorpID:      cfg.CorpID,
				Status:      cfg.Status,
				AppKey:      cfg.AppKey,
				AppSecret:   cfg.AppSecret,
				AgentID:     cfg.AgentID,
				AppHomeURL:  cfg.AppHomeURL,
				RedirectURI: cfg.RedirectURI,
			}
			if err := DB.Create(&org).Error; err != nil {
				log.Printf("[seed] create organization %s failed: %v", cfg.OrgID, err)
			}
			continue
		}
		if err != nil {
			log.Printf("[seed] query organization %s failed: %v", cfg.OrgID, err)
			continue
		}

		updates := map[string]interface{}{
			"name":         cfg.Name,
			"corp_id":      cfg.CorpID,
			"status":       cfg.Status,
			"app_home_url": cfg.AppHomeURL,
			"redirect_uri": cfg.RedirectURI,
		}
		if strings.TrimSpace(org.AgentID) == "" && cfg.AgentID != "" {
			updates["agent_id"] = cfg.AgentID
		}
		if strings.TrimSpace(org.AppKey) == "" && cfg.AppKey != "" {
			updates["app_key"] = cfg.AppKey
		}
		if strings.TrimSpace(org.AppSecret) == "" && cfg.AppSecret != "" {
			updates["app_secret"] = cfg.AppSecret
		}
		if err := DB.Model(&org).Updates(updates).Error; err != nil {
			log.Printf("[seed] update organization %s failed: %v", cfg.OrgID, err)
		}
	}
}

func seed() {
	seedOrganizationsFromEnv()

	// 创建默认管理员（如果不存在）
	adminUserID := getEnvOrDefault("ADMIN_USER_ID", "admin")
	var count int64
	DB.Model(&User{}).Where("user_id = ?", adminUserID).Count(&count)
	if count == 0 {
		adminPassword := os.Getenv("ADMIN_PASSWORD")
		if adminPassword == "" {
			adminPassword = generateRandomPassword(32)
			log.Printf("[security] ADMIN_PASSWORD is not set; default admin was initialized with a random password that is not printed")
		}
		hash, err := HashPassword(adminPassword)
		if err != nil {
			log.Printf("生成密码哈希失败: %v", err)
			return
		}
		admin := User{
			UserID:       adminUserID,
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
			log.Printf("已创建默认管理员账号: %s", adminUserID)
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
			// 通用权限
			{Name: "用户管理", Code: "user_manage", Description: "用户管理权限"},
			{Name: "部门管理", Code: "department_manage", Description: "部门管理权限"},
			{Name: "考勤管理", Code: "attendance_manage", Description: "考勤管理权限"},
			{Name: "审批管理", Code: "approval_manage", Description: "审批管理权限"},
			{Name: "同步审批", Code: "approval:sync", Description: "同步审批模板/实例数据"},
			{Name: "创建审批模板", Code: "approval:create", Description: "创建审批模板"},
			{Name: "编辑审批模板", Code: "approval:update", Description: "编辑审批模板"},
			{Name: "删除审批模板", Code: "approval:delete", Description: "删除审批模板"},
			{Name: "权限管理", Code: "permission_manage", Description: "权限管理权限"},
			// 绩效模块权限
			{Name: "绩效活动管理", Code: "performance:activity:manage", Description: "创建/编辑/发布/启动/锁定/归档绩效活动"},
			{Name: "绩效自评提交", Code: "performance:self_eval:submit", Description: "提交绩效自评"},
			{Name: "绩效主管评分", Code: "performance:manager_eval:submit", Description: "主管绩效评分"},
			{Name: "绩效员工确认", Code: "performance:employee_confirm:submit", Description: "员工确认绩效结果"},
			{Name: "绩效主管确认", Code: "performance:manager_confirm:submit", Description: "主管确认绩效结果"},
			{Name: "绩效HR确认", Code: "performance:hr_confirm:submit", Description: "HR确认绩效结果"},
			{Name: "绩效等级调整", Code: "performance:level_adjust:manage", Description: "调整绩效最终等级"},
			{Name: "绩效分布规则", Code: "performance:distribution:manage", Description: "设置绩效分布规则"},
			{Name: "绩效指标库管理", Code: "performance:indicator:manage", Description: "指标库/指标项CRUD"},
			{Name: "绩效目标管理", Code: "performance:goal:manage", Description: "目标设定/审批/分配"},
			{Name: "绩效考核上级调整", Code: "performance:assessment_manager:update", Description: "调整单个绩效参与人的考核上级"},
			{Name: "绩效考核上级批量调整", Code: "performance:assessment_manager:batch_update", Description: "批量调整绩效参与人的考核上级"},
			{Name: "绩效结果查看", Code: "performance:result:view", Description: "查看绩效结果"},
			// 细粒度权限（用于菜单派生）
			{Name: "组织数据只读", Code: "org:read", Description: "查看组织架构、花名册等组织数据"},
			{Name: "审计日志只读", Code: "audit_log:read", Description: "查看操作审计日志"},
		}
		for _, perm := range permissions {
			DB.Create(&perm)
		}
		log.Println("已创建默认权限数据")
	}

	// 创建角色-权限关联（如果不存在）
	DB.Model(&RolePermission{}).Count(&count)
	if count == 0 {
		seedRolePermissions()
		log.Println("已创建默认角色-权限关联数据")
	}

	// 创建用户-角色关联（如果不存在）
	DB.Model(&UserRole{}).Count(&count)
	if count == 0 {
		seedUserRoles()
		log.Println("已创建默认用户-角色关联数据")
	}
}

func seedRolePermissions() {
	// 查询角色
	roleMap := make(map[string]uint)
	var roles []Role
	DB.Find(&roles)
	for _, r := range roles {
		roleMap[r.Name] = r.ID
	}

	// 查询权限
	permMap := make(map[string]uint)
	var permissions []Permission
	DB.Find(&permissions)
	for _, p := range permissions {
		permMap[p.Code] = p.ID
	}

	// 所有权限码
	allPermCodes := []string{
		"user_manage", "department_manage", "attendance_manage", "approval_manage", "permission_manage",
		"approval:sync", "approval:create", "approval:update", "approval:delete",
		"performance:activity:manage", "performance:self_eval:submit", "performance:manager_eval:submit",
		"performance:employee_confirm:submit", "performance:manager_confirm:submit", "performance:hr_confirm:submit",
		"performance:level_adjust:manage", "performance:distribution:manage", "performance:indicator:manage",
		"performance:goal:manage", "performance:assessment_manager:update", "performance:assessment_manager:batch_update",
		"performance:result:view",
		"org:read", "audit_log:read",
	}

	// 管理员 = 全部权限
	if adminID, ok := roleMap["管理员"]; ok {
		for _, code := range allPermCodes {
			if permID, ok := permMap[code]; ok {
				DB.Create(&RolePermission{RoleID: adminID, PermissionID: permID})
			}
		}
	}

	// 部门负责人权限
	managerCodes := []string{
		"performance:activity:manage", "performance:self_eval:submit", "performance:manager_eval:submit",
		"performance:manager_confirm:submit", "performance:level_adjust:manage", "performance:distribution:manage",
		"performance:indicator:manage", "performance:goal:manage", "performance:assessment_manager:update",
		"performance:assessment_manager:batch_update", "performance:result:view",
	}
	if managerID, ok := roleMap["部门负责人"]; ok {
		for _, code := range managerCodes {
			if permID, ok := permMap[code]; ok {
				DB.Create(&RolePermission{RoleID: managerID, PermissionID: permID})
			}
		}
	}

	// 普通员工权限
	employeeCodes := []string{
		"performance:self_eval:submit", "performance:employee_confirm:submit", "performance:result:view",
	}
	if employeeID, ok := roleMap["普通员工"]; ok {
		for _, code := range employeeCodes {
			if permID, ok := permMap[code]; ok {
				DB.Create(&RolePermission{RoleID: employeeID, PermissionID: permID})
			}
		}
	}
}

func seedUserRoles() {
	// 查询角色
	roleMap := make(map[string]uint)
	var roles []Role
	DB.Find(&roles)
	for _, r := range roles {
		roleMap[r.Name] = r.ID
	}

	// admin 分配管理员角色
	if adminID, ok := roleMap["管理员"]; ok {
		var admin User
		if err := DB.Where("user_id = ?", getEnvOrDefault("ADMIN_USER_ID", "admin")).First(&admin).Error; err == nil {
			DB.Create(&UserRole{OrgID: admin.OrgID, UserID: admin.UserID, RoleID: adminID})
		}
	}
}

var liedeAdminOrgIDs = []string{"default", "xiaotie", "muteng"}

func migrateLiedeOrganizationAdminRoles() {
	adminRole, err := ensureRolePreset("管理员", "系统管理员")
	if err != nil {
		log.Printf("[migrate] 确保管理员角色失败: %v", err)
		return
	}
	if err := ensureAdminRoleFullAccess(adminRole.ID); err != nil {
		log.Printf("[migrate] 确保管理员角色权限失败: %v", err)
		return
	}

	users, err := findLiedeAdminUsers()
	if err != nil {
		log.Printf("[migrate] 查找列德用户失败: %v", err)
		return
	}
	if len(users) == 0 {
		log.Printf("[migrate] 未找到姓名包含“列德”的本地用户，跳过三组织管理员授权")
		return
	}

	usersByOrg := make(map[string][]User, len(users))
	for _, user := range users {
		if strings.TrimSpace(user.OrgID) != "" {
			usersByOrg[user.OrgID] = append(usersByOrg[user.OrgID], user)
		}
	}

	for _, orgID := range liedeAdminOrgIDs {
		orgUsers := usersByOrg[orgID]
		if len(orgUsers) == 0 {
			log.Printf("[migrate] 未找到列德在组织内的本地用户，跳过授权: org_id=%s", orgID)
			continue
		}
		for _, user := range orgUsers {
			if strings.TrimSpace(user.UserID) == "" {
				continue
			}
			if err := EnsureOrganizationUser(orgID, user.UserID, "active"); err != nil {
				log.Printf("[migrate] 维护列德组织成员缓存失败: org_id=%s user_id=%s err=%v", orgID, user.UserID, err)
			}
			if err := ensureUserRoleInOrg(orgID, user.UserID, adminRole.ID); err != nil {
				log.Printf("[migrate] 授权列德为组织管理员失败: org_id=%s user_id=%s err=%v", orgID, user.UserID, err)
				continue
			}
			log.Printf("[migrate] 已确保列德为组织管理员: org_id=%s user_id=%s", orgID, user.UserID)
		}
	}
}

func findLiedeAdminUsers() ([]User, error) {
	users := make([]User, 0)
	seen := make(map[uint]struct{})
	appendUsers := func(candidates []User) {
		for _, user := range candidates {
			if user.ID == 0 {
				continue
			}
			if _, ok := seen[user.ID]; ok {
				continue
			}
			seen[user.ID] = struct{}{}
			users = append(users, user)
		}
	}

	var nameMatches []User
	if err := DB.Where("deleted_at IS NULL AND name LIKE ?", "%列德%").
		Order("FIELD(org_id, 'default', 'xiaotie', 'muteng') DESC, id ASC").
		Find(&nameMatches).Error; err != nil {
		return nil, err
	}
	appendUsers(nameMatches)

	if adminUserID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID")); adminUserID != "" {
		var envMatches []User
		if err := DB.Where("deleted_at IS NULL AND user_id = ?", adminUserID).
			Order("FIELD(org_id, 'default', 'xiaotie', 'muteng') DESC, id ASC").
			Find(&envMatches).Error; err != nil {
			return nil, err
		}
		appendUsers(envMatches)
	}

	return users, nil
}

func ensureAdminRoleFullAccess(roleID uint) error {
	var permissions []Permission
	if err := DB.Where("deleted_at IS NULL").Find(&permissions).Error; err != nil {
		return err
	}

	permMap := make(map[string]uint, len(permissions))
	menuKeys := []string{"menu:home"}
	for _, permission := range permissions {
		if strings.TrimSpace(permission.Code) == "" {
			continue
		}
		permMap[permission.Code] = permission.ID
		menuKeys = append(menuKeys, legacyMenuKeysByPermission[permission.Code]...)
	}
	for code := range permMap {
		if err := ensureRolePermission(roleID, code, permMap); err != nil {
			return err
		}
	}
	if err := ensureRoleMenuPermission(roleID, menuKeys); err != nil {
		return err
	}
	return ensureRoleDataPermission(roleID, "all", "[]")
}

func ensureUserRoleInOrg(orgID, userID string, roleID uint) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		orgID = "default"
	}
	userID = strings.TrimSpace(userID)
	if userID == "" || roleID == 0 {
		return nil
	}

	var existing UserRole
	err := DB.Unscoped().
		Where("org_id = ? AND user_id = ?", orgID, userID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return DB.Create(&UserRole{OrgID: orgID, UserID: userID, RoleID: roleID}).Error
	}
	if err != nil {
		return err
	}

	updates := map[string]interface{}{}
	if existing.RoleID != roleID {
		updates["role_id"] = roleID
	}
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Unscoped().Model(&existing).Updates(updates).Error
}

// migratePermissions 幂等迁移：为已有部署补充新权限码和角色关联
func migratePermissions() {
	// 1. 确保新权限码存在
	newPerms := []Permission{
		{Name: "组织数据只读", Code: "org:read", Description: "查看组织架构、花名册等组织数据"},
		{Name: "审计日志只读", Code: "audit_log:read", Description: "查看操作审计日志"},
		{Name: "同步审批", Code: "approval:sync", Description: "同步审批模板/实例数据"},
		{Name: "创建审批模板", Code: "approval:create", Description: "创建审批模板"},
		{Name: "编辑审批模板", Code: "approval:update", Description: "编辑审批模板"},
		{Name: "删除审批模板", Code: "approval:delete", Description: "删除审批模板"},
		{Name: "绩效考核上级调整", Code: "performance:assessment_manager:update", Description: "调整单个绩效参与人的考核上级"},
		{Name: "绩效考核上级批量调整", Code: "performance:assessment_manager:batch_update", Description: "批量调整绩效参与人的考核上级"},
	}
	for _, p := range newPerms {
		var existing Permission
		if err := DB.Where("code = ?", p.Code).First(&existing).Error; err != nil {
			DB.Create(&p)
			log.Printf("迁移：已创建权限码 %s", p.Code)
		}
	}

	// 2. 构建权限码→ID映射
	permMap := make(map[string]uint)
	var allPerms []Permission
	DB.Find(&allPerms)
	for _, p := range allPerms {
		permMap[p.Code] = p.ID
	}

	// 3. 给有 user_manage 的角色补 org:read
	grantCompatPermission("user_manage", "org:read", permMap)
	// 4. 给有 permission_manage 的角色补 audit_log:read 和 org:read
	grantCompatPermission("permission_manage", "audit_log:read", permMap)
	grantCompatPermission("permission_manage", "org:read", permMap)
	// 5. 给有 performance:activity:manage 的角色补 org:read（部门负责人需要看部门树）
	grantCompatPermission("performance:activity:manage", "org:read", permMap)
	// 6. 将旧 approval_manage 平滑迁移到细粒度审批操作权限
	grantCompatPermission("approval_manage", "approval:sync", permMap)
	grantCompatPermission("approval_manage", "approval:create", permMap)
	grantCompatPermission("approval_manage", "approval:update", permMap)
	grantCompatPermission("approval_manage", "approval:delete", permMap)
	// 7. 拥有绩效活动管理的角色默认可维护活动内考核上级。
	grantCompatPermission("performance:activity:manage", "performance:assessment_manager:update", permMap)
	grantCompatPermission("performance:activity:manage", "performance:assessment_manager:batch_update", permMap)
}

// grantCompatPermission 给已拥有 sourcePerm 的角色自动补充 targetPerm（幂等）
func grantCompatPermission(sourcePerm, targetPerm string, permMap map[string]uint) {
	targetID, ok := permMap[targetPerm]
	if !ok {
		return
	}
	var source Permission
	if err := DB.Where("code = ?", sourcePerm).First(&source).Error; err != nil {
		return
	}
	// 找到所有拥有 sourcePerm 的角色
	var sourceRoles []RolePermission
	DB.Where("permission_id = ?", source.ID).Find(&sourceRoles)
	for _, rp := range sourceRoles {
		// 检查是否已有 targetPerm
		var existing RolePermission
		if err := DB.Where("role_id = ? AND permission_id = ?", rp.RoleID, targetID).First(&existing).Error; err != nil {
			DB.Create(&RolePermission{RoleID: rp.RoleID, PermissionID: targetID})
			log.Printf("迁移：角色 %d 已补充权限 %s", rp.RoleID, targetPerm)
		}
	}
}

type indicatorRolePreset struct {
	Name           string
	Description    string
	DataScope      string
	DepartmentKeys string
}

var performanceIndicatorRolePresets = []indicatorRolePreset{
	{
		Name:           "绩效管理者-人事",
		Description:    "维护全公司绩效指标库，核查并配置各部门绩效指标",
		DataScope:      "all",
		DepartmentKeys: "[]",
	},
	{
		Name:           "人力负责人",
		Description:    "维护全公司绩效指标库，查看并配置各部门绩效指标",
		DataScope:      "all",
		DepartmentKeys: "[]",
	},
	{
		Name:           "HRBP",
		Description:    "维护负责组织范围内的绩效指标库",
		DataScope:      "department",
		DepartmentKeys: "[]",
	},
	{
		Name:           "部门负责人",
		Description:    "维护所在组织架构管理范围内的绩效指标库",
		DataScope:      "department",
		DepartmentKeys: "[]",
	},
	{
		Name:           "部门助理",
		Description:    "协助维护负责组织范围内的绩效指标库",
		DataScope:      "department",
		DepartmentKeys: "[]",
	},
	{
		Name:           "部门主管",
		Description:    "维护主管负责组织范围内的绩效指标库",
		DataScope:      "department",
		DepartmentKeys: "[]",
	},
	{
		Name:           "经理",
		Description:    "维护经理负责组织范围内的绩效指标库",
		DataScope:      "department",
		DepartmentKeys: "[]",
	},
}

func migratePerformanceIndicatorRolePresets() {
	permMap := make(map[string]uint)
	var permissions []Permission
	if err := DB.Find(&permissions).Error; err != nil {
		log.Printf("迁移绩效指标库角色：读取权限失败: %v", err)
		return
	}
	for _, permission := range permissions {
		permMap[permission.Code] = permission.ID
	}

	for _, preset := range performanceIndicatorRolePresets {
		role, err := ensureRolePreset(preset.Name, preset.Description)
		if err != nil {
			log.Printf("迁移绩效指标库角色：角色 %s 创建/恢复失败: %v", preset.Name, err)
			continue
		}
		for _, code := range []string{"performance:indicator:manage", "org:read"} {
			if err := ensureRolePermission(role.ID, code, permMap); err != nil {
				log.Printf("迁移绩效指标库角色：角色 %s 补充权限 %s 失败: %v", preset.Name, code, err)
			}
		}
		if err := ensureRoleMenuPermission(role.ID, []string{"menu:home", "menu:performance-indicator-library"}); err != nil {
			log.Printf("迁移绩效指标库角色：角色 %s 补充菜单权限失败: %v", preset.Name, err)
		}
		if err := ensureRoleDataPermission(role.ID, preset.DataScope, preset.DepartmentKeys); err != nil {
			log.Printf("迁移绩效指标库角色：角色 %s 补充数据权限失败: %v", preset.Name, err)
		}
	}
}

func ensureRolePreset(name, description string) (Role, error) {
	var role Role
	err := DB.Unscoped().Where("name = ?", name).First(&role).Error
	if err == gorm.ErrRecordNotFound {
		role = Role{Name: name, Description: description}
		return role, DB.Create(&role).Error
	}
	if err != nil {
		return role, err
	}
	updates := map[string]interface{}{}
	if role.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	if strings.TrimSpace(role.Description) == "" {
		updates["description"] = description
		role.Description = description
	}
	if len(updates) > 0 {
		if err := DB.Unscoped().Model(&role).Updates(updates).Error; err != nil {
			return role, err
		}
	}
	role.DeletedAt.Valid = false
	return role, nil
}

func ensureRolePermission(roleID uint, permissionCode string, permMap map[string]uint) error {
	permissionID, ok := permMap[permissionCode]
	if !ok {
		return fmt.Errorf("permission %s not found", permissionCode)
	}
	var existing RolePermission
	err := DB.Unscoped().
		Where("role_id = ? AND permission_id = ?", roleID, permissionID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return DB.Create(&RolePermission{RoleID: roleID, PermissionID: permissionID}).Error
	}
	if err != nil {
		return err
	}
	if existing.DeletedAt.Valid {
		return DB.Unscoped().Model(&existing).Update("deleted_at", nil).Error
	}
	return nil
}

func ensureRoleMenuPermission(roleID uint, menuKeys []string) error {
	var existing MenuPermission
	err := DB.Unscoped().
		Where("role_id = ?", roleID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		payload, err := json.Marshal(normalizeMenuPermissionKeys(menuKeys))
		if err != nil {
			return err
		}
		return DB.Create(&MenuPermission{RoleID: roleID, MenuKeys: string(payload)}).Error
	}
	if err != nil {
		return err
	}
	mergedKeys := normalizeMenuPermissionKeys(menuKeys)
	var existingKeys []string
	existingKeysParsed := false
	if err := json.Unmarshal([]byte(strings.TrimSpace(existing.MenuKeys)), &existingKeys); err == nil {
		existingKeysParsed = true
		mergedKeys = normalizeMenuPermissionKeys(append(existingKeys, menuKeys...))
	}
	normalizedExistingKeys := normalizeMenuPermissionKeys(existingKeys)
	updates := map[string]interface{}{}
	if !existingKeysParsed || !stringSlicesEqual(normalizedExistingKeys, mergedKeys) {
		payload, err := json.Marshal(mergedKeys)
		if err != nil {
			return err
		}
		updates["menu_keys"] = string(payload)
	}
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Unscoped().Model(&existing).Updates(updates).Error
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func ensureRoleDataPermission(roleID uint, scope string, departmentKeys string) error {
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = "department"
	}
	departmentKeys = strings.TrimSpace(departmentKeys)
	if departmentKeys == "" {
		departmentKeys = "[]"
	}
	var existing DataPermission
	err := DB.Unscoped().
		Where("role_id = ?", roleID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return DB.Create(&DataPermission{RoleID: roleID, Scope: scope, DepartmentKeys: departmentKeys}).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	if strings.TrimSpace(existing.Scope) == "" {
		updates["scope"] = scope
	}
	if strings.TrimSpace(existing.DepartmentKeys) == "" {
		updates["department_keys"] = departmentKeys
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Unscoped().Model(&existing).Updates(updates).Error
}

func normalizeMenuPermissionKeys(keys []string) []string {
	keySet := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		normalized := strings.TrimSpace(key)
		if normalized == "" {
			continue
		}
		if !strings.HasPrefix(normalized, "menu:") {
			normalized = "menu:" + normalized
		}
		keySet[normalized] = struct{}{}
	}
	result := make([]string, 0, len(keySet))
	for key := range keySet {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}

var legacyMenuKeysByPermission = map[string][]string{
	"org:read":                            {"menu:organization-dashboard", "menu:department-tree", "menu:employees"},
	"user_manage":                         {"menu:employee-profile", "menu:employee-flow", "menu:talent-analysis", "menu:sync-log"},
	"attendance_manage":                   {"menu:attendance", "menu:attendance-stats", "menu:attendance-export", "menu:week-schedule", "menu:employee-shift-config", "menu:sync-jobs", "menu:leave-overtime"},
	"approval_manage":                     {"menu:approval-templates", "menu:approval-instances", "menu:approval-stats"},
	"permission_manage":                   {"menu:permission", "menu:setting"},
	"audit_log:read":                      {"menu:audit-logs"},
	"performance:activity:manage":         {"menu:performance-overview"},
	"performance:self_eval:submit":        {"menu:performance-overview"},
	"performance:manager_eval:submit":     {"menu:performance-overview"},
	"performance:employee_confirm:submit": {"menu:performance-overview"},
	"performance:manager_confirm:submit":  {"menu:performance-overview"},
	"performance:hr_confirm:submit":       {"menu:performance-overview"},
	"performance:goal:manage":             {"menu:performance-overview"},
	"performance:result:view":             {"menu:performance-overview"},
	"performance:indicator:manage":        {"menu:performance-indicator-library"},
}

// migrateMenuPermissions 将旧版本“操作权限推导菜单”的结果固化到 menu_permissions。
// 之后运行时菜单可见性只读取 menu_permissions，不再依赖操作权限码。
func migrateMenuPermissions() {
	var roles []Role
	if err := DB.Find(&roles).Error; err != nil {
		log.Printf("迁移菜单权限：读取角色失败: %v", err)
		return
	}

	for _, role := range roles {
		var existing MenuPermission
		if err := DB.Where("role_id = ? AND deleted_at IS NULL", role.ID).First(&existing).Error; err == nil {
			continue
		}

		menuKeys := deriveLegacyMenuKeysForRole(role.ID)
		if len(menuKeys) == 0 {
			continue
		}
		payload, err := json.Marshal(menuKeys)
		if err != nil {
			log.Printf("迁移菜单权限：序列化角色 %d 菜单失败: %v", role.ID, err)
			continue
		}
		if err := DB.Create(&MenuPermission{RoleID: role.ID, MenuKeys: string(payload)}).Error; err != nil {
			log.Printf("迁移菜单权限：写入角色 %d 菜单失败: %v", role.ID, err)
		}
	}
}

func deriveLegacyMenuKeysForRole(roleID uint) []string {
	var permissions []Permission
	if err := DB.
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id AND role_permissions.deleted_at IS NULL").
		Where("role_permissions.role_id = ? AND permissions.deleted_at IS NULL", roleID).
		Find(&permissions).Error; err != nil {
		log.Printf("迁移菜单权限：读取角色 %d 权限失败: %v", roleID, err)
		return nil
	}

	keySet := map[string]struct{}{"menu:home": {}}
	for _, permission := range permissions {
		for _, key := range legacyMenuKeysByPermission[permission.Code] {
			keySet[key] = struct{}{}
		}
	}

	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func migrateAttendanceToolboxMenuPermissions() {
	var attendancePermission Permission
	if err := DB.Where("code = ? AND deleted_at IS NULL", "attendance_manage").First(&attendancePermission).Error; err != nil {
		return
	}

	var rolePermissions []RolePermission
	if err := DB.Where("permission_id = ? AND deleted_at IS NULL", attendancePermission.ID).Find(&rolePermissions).Error; err != nil {
		log.Printf("迁移考勤工具箱菜单权限：读取角色权限失败: %v", err)
		return
	}

	menuKeys := append([]string{"menu:home"}, legacyMenuKeysByPermission["attendance_manage"]...)
	for _, rolePermission := range rolePermissions {
		if err := ensureRoleMenuPermission(rolePermission.RoleID, menuKeys); err != nil {
			log.Printf("迁移考勤工具箱菜单权限：角色 %d 补充菜单失败: %v", rolePermission.RoleID, err)
		}
	}
}
