package main

import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 需要添加 org_id 字段的表
var tablesToMigrate = []string{
	"users",
	"departments",
	"department_change_logs",
	"attendances",
	"approvals",
	"approval_templates",
	"roles",
	"permissions",
	"role_permissions",
	"user_roles",
	"menu_permissions",
	"data_permissions",
	"operation_logs",
	"sync_statuses",
	"dingtalk_bindings",
	"user_sessions",
	"login_logs",
	"attendance_exports",
	"employee_profiles",
	"employee_transfers",
	"employee_resignations",
	"employee_onboardings",
	"talent_analyses",
	"employee_shift_configs",
	"dingtalk_shift_catalogs",
	"week_schedule_rules",
	"week_schedule_overrides",
	"week_schedule_sync_logs",
	"statutory_holidays",
	"leave_rule_configs",
	"annual_leave_eligibilities",
	"annual_leave_grants",
	"overtime_rule_configs",
	"overtime_match_results",
	"overtime_sync_histories",
	"overtime_supplementary_requests",
	"compensatory_leave_ledgers",
	"annual_leave_consume_logs",
	// 绩效相关表
	"performance_templates",
	"performance_template_sections",
	"performance_template_items",
	"performance_activity_manager_assignments",
	"performance_activities",
	"performance_level_rules",
	"performance_level_rule_items",
	"performance_distribution_rules",
	"performance_distribution_exceptions",
	"performance_reminder_logs",
	"performance_participants",
	"performance_reviews",
	"performance_review_versions",
	"performance_relationship_change_logs",
	"performance_goal_records",
	"performance_goal_approval_logs",
	"performance_company_finances",
	"performance_indicator_libraries",
	"performance_indicator_items",
}

func main() {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		log.Fatal("DATABASE_DSN environment variable is required")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

	log.Println("Starting multi-tenant migration...")

	// 1. 创建 organizations 表
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS organizations (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			org_id VARCHAR(64) NOT NULL UNIQUE,
			name VARCHAR(128) NOT NULL,
			corp_id VARCHAR(128) NOT NULL UNIQUE,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			app_key VARCHAR(128),
			app_secret VARCHAR(256),
			agent_id VARCHAR(64),
			app_home_url VARCHAR(256),
			redirect_uri VARCHAR(256),
			extension JSON,
			created_at DATETIME(3),
			updated_at DATETIME(3),
			deleted_at DATETIME(3),
			INDEX idx_organizations_deleted_at (deleted_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`).Error; err != nil {
		log.Fatalf("Failed to create organizations table: %v", err)
	}
	log.Println("✓ Created organizations table")

	// 2. 创建 organization_users 表
	if err := db.Exec(`
		CREATE TABLE IF NOT EXISTS organization_users (
			id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
			org_id VARCHAR(64) NOT NULL,
			user_id VARCHAR(64) NOT NULL,
			status VARCHAR(32) NOT NULL DEFAULT 'active',
			created_at DATETIME(3),
			updated_at DATETIME(3),
			deleted_at DATETIME(3),
			UNIQUE INDEX idx_org_user (org_id, user_id),
			INDEX idx_organization_users_deleted_at (deleted_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
	`).Error; err != nil {
		log.Fatalf("Failed to create organization_users table: %v", err)
	}
	log.Println("✓ Created organization_users table")

	// 3. 为所有核心表添加 org_id 字段
	for _, table := range tablesToMigrate {
		// 检查字段是否已存在
		var count int64
		if err := db.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'org_id'", table).Scan(&count).Error; err != nil {
			log.Printf("Warning: Failed to check org_id column for table %s: %v", table, err)
			continue
		}

		if count > 0 {
			log.Printf("  - Table %s already has org_id column, skipping", table)
			continue
		}

		// 添加 org_id 字段（先允许NULL）
		sql := fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `org_id` VARCHAR(64) NULL AFTER `id`", table)
		if err := db.Exec(sql).Error; err != nil {
			log.Printf("Warning: Failed to add org_id to %s: %v", table, err)
			continue
		}
		log.Printf("  ✓ Added org_id column to %s", table)
	}

	// 4. 插入默认组织（从环境变量读取当前配置）
	defaultOrgID := os.Getenv("DEFAULT_ORG_ID")
	if defaultOrgID == "" {
		defaultOrgID = "default"
	}
	defaultCorpID := os.Getenv("DINGTALK_CORP_ID")
	if defaultCorpID == "" {
		log.Println("Warning: DINGTALK_CORP_ID not set, using placeholder")
		defaultCorpID = "default_corp_id"
	}

	var orgCount int64
	db.Raw("SELECT COUNT(*) FROM organizations WHERE org_id = ?", defaultOrgID).Scan(&orgCount)
	if orgCount == 0 {
		if err := db.Exec(`
			INSERT INTO organizations (org_id, name, corp_id, status, app_key, app_secret, agent_id, created_at, updated_at)
			VALUES (?, ?, ?, 'active', ?, ?, ?, NOW(), NOW())
		`, defaultOrgID, "默认组织", defaultCorpID,
			os.Getenv("DINGTALK_APP_KEY"),
			os.Getenv("DINGTALK_APP_SECRET"),
			os.Getenv("DINGTALK_AGENT_ID")).Error; err != nil {
			log.Fatalf("Failed to insert default organization: %v", err)
		}
		log.Printf("✓ Inserted default organization: %s", defaultOrgID)
	} else {
		log.Printf("  - Default organization %s already exists", defaultOrgID)
	}

	// 5. 将现有数据的 org_id 设置为默认组织
	log.Println("Setting org_id for existing data...")
	for _, table := range tablesToMigrate {
		sql := fmt.Sprintf("UPDATE `%s` SET `org_id` = ? WHERE `org_id` IS NULL", table)
		result := db.Exec(sql, defaultOrgID)
		if result.Error != nil {
			log.Printf("Warning: Failed to update org_id for %s: %v", table, result.Error)
			continue
		}
		if result.RowsAffected > 0 {
			log.Printf("  ✓ Updated %d rows in %s", result.RowsAffected, table)
		}
	}

	// 6. 将 org_id 字段改为 NOT NULL
	log.Println("Making org_id NOT NULL...")
	for _, table := range tablesToMigrate {
		sql := fmt.Sprintf("ALTER TABLE `%s` MODIFY COLUMN `org_id` VARCHAR(64) NOT NULL", table)
		if err := db.Exec(sql).Error; err != nil {
			log.Printf("Warning: Failed to make org_id NOT NULL for %s: %v", table, err)
			continue
		}
		log.Printf("  ✓ Made org_id NOT NULL in %s", table)
	}

	// 7. 为 org_id 添加索引
	log.Println("Adding org_id indexes...")
	for _, table := range tablesToMigrate {
		indexName := fmt.Sprintf("idx_%s_org_id", table)
		sql := fmt.Sprintf("CREATE INDEX `%s` ON `%s` (`org_id`)", indexName, table)
		if err := db.Exec(sql).Error; err != nil {
			// 索引可能已存在，忽略错误
			log.Printf("  - Index on %s might already exist: %v", table, err)
			continue
		}
		log.Printf("  ✓ Added index to %s", table)
	}

	// 8. 更新 users 表的唯一索引（org_id + user_id）
	log.Println("Updating unique constraints...")
	// 删除旧的 user_id unique 索引
	db.Exec("ALTER TABLE `users` DROP INDEX IF EXISTS `user_id`")
	// 添加新的复合唯一索引
	if err := db.Exec("CREATE UNIQUE INDEX `idx_org_user_id` ON `users` (`org_id`, `user_id`)").Error; err != nil {
		log.Printf("Warning: Failed to create unique index on users: %v", err)
	} else {
		log.Println("  ✓ Updated users unique constraint")
	}

	// 删除旧的 email unique 索引，添加新的复合索引
	db.Exec("ALTER TABLE `users` DROP INDEX IF EXISTS `email`")
	if err := db.Exec("CREATE UNIQUE INDEX `idx_org_email` ON `users` (`org_id`, `email`)").Error; err != nil {
		log.Printf("Warning: Failed to create unique index on users email: %v", err)
	} else {
		log.Println("  ✓ Updated users email unique constraint")
	}

	// 9. 更新 departments 表的唯一索引
	db.Exec("ALTER TABLE `departments` DROP INDEX IF EXISTS `department_id`")
	if err := db.Exec("CREATE UNIQUE INDEX `idx_org_dept_id` ON `departments` (`org_id`, `department_id`)").Error; err != nil {
		log.Printf("Warning: Failed to create unique index on departments: %v", err)
	} else {
		log.Println("  ✓ Updated departments unique constraint")
	}

	log.Println("✅ Multi-tenant migration completed successfully!")
	log.Println("")
	log.Println("Next steps:")
	log.Println("1. Restart the application")
	log.Println("2. Insert additional organizations into the organizations table")
	log.Println("3. Update DINGTALK_ORGANIZATIONS environment variable")
}
