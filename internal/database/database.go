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
	migrateUserManagerColumns()

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

	if err := DB.AutoMigrate(
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

	if err := migrateShiftCatalogSchema(); err != nil {
		return err
	}
	if err := migratePerformanceReviewVersionSchema(); err != nil {
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
			// 通用权限
			{Name: "用户管理", Code: "user_manage", Description: "用户管理权限"},
			{Name: "部门管理", Code: "department_manage", Description: "部门管理权限"},
			{Name: "考勤管理", Code: "attendance_manage", Description: "考勤管理权限"},
			{Name: "审批管理", Code: "approval_manage", Description: "审批管理权限"},
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
			{Name: "绩效结果查看", Code: "performance:result:view", Description: "查看绩效结果"},
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
		"performance:activity:manage", "performance:self_eval:submit", "performance:manager_eval:submit",
		"performance:employee_confirm:submit", "performance:manager_confirm:submit", "performance:hr_confirm:submit",
		"performance:level_adjust:manage", "performance:distribution:manage", "performance:indicator:manage",
		"performance:goal:manage", "performance:result:view",
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
		"performance:indicator:manage", "performance:goal:manage", "performance:result:view",
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
		if err := DB.Where("user_id = ?", "admin").First(&admin).Error; err == nil {
			DB.Create(&UserRole{UserID: admin.UserID, RoleID: adminID})
		}
	}
}
