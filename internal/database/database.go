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
	"peopleops/internal/requestmeta"
	"reflect"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	registerOrganizationCallbacks(DB)
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
		log.Printf("legacy multitenant compatibility migration failed: %v", err)
		return err
	}
	// Legacy compatibility code can still touch indexes; verify the final state again.
	if err := MigrateOrgCompositeUniqueIndexes(DB); err != nil {
		log.Printf("final organization composite unique index verification failed: %v", err)
		return err
	}
	log.Println("table schema migration completed")

	// 种子数据
	log.Println("开始填充种子数据...")
	seed()
	log.Println("种子数据填充完成")

	// 增量迁移：为已有部署补充新权限码
	migratePermissions()
	migratePerformanceIndicatorRolePresets()
	migrateMenuPermissions()
	migratePerformanceReportMenuPermissions()
	migratePerformanceFollowupMenuPermissions()
	migrateAttendanceToolboxMenuPermissions()
	migrateLiedeOrganizationAdminRoles()
	if err := remapCrossOrgUserRoleBindings(); err != nil {
		log.Printf("[migrate] remap cross-org user_roles failed: %v", err)
	}

	// 绩效表已随主库 migrate() 一并迁移，无需独立数据源
	log.Println("绩效模块使用主库")

	return nil
}

// GetPerformanceDB 获取绩效模块的数据源（统一使用主库）
func registerOrganizationCallbacks(db *gorm.DB) {
	if db == nil {
		return
	}
	if err := db.Callback().Create().Before("gorm:create").Register("peopleops:set_org_id", setCreateOrganizationID); err != nil {
		log.Printf("[org-scope] register create callback failed: %v", err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("peopleops:query_org_scope", applyOrganizationScope); err != nil {
		log.Printf("[org-scope] register query callback failed: %v", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("peopleops:update_org_scope", applyOrganizationScope); err != nil {
		log.Printf("[org-scope] register update callback failed: %v", err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("peopleops:delete_org_scope", applyOrganizationScope); err != nil {
		log.Printf("[org-scope] register delete callback failed: %v", err)
	}
}

func statementOrganizationID(db *gorm.DB) string {
	if db == nil || db.Statement == nil {
		return ""
	}
	info := requestmeta.FromContext(db.Statement.Context)
	if info == nil || strings.TrimSpace(info.OrgID) == "" {
		return ""
	}
	return NormalizeOrganizationID(info.OrgID)
}

func CurrentOrganizationIDFromDB(db *gorm.DB) string {
	if db != nil && db.Statement != nil {
		if info := requestmeta.FromContext(db.Statement.Context); info != nil {
			return NormalizeOrganizationID(info.OrgID)
		}
	}
	return DefaultOrganizationID
}

// RequireOrganizationIDFromDB returns the tenant org from the DB session and fails closed
// when no explicit organization is present. Unlike CurrentOrganizationIDFromDB it never
// invents "default" for an empty context.
func RequireOrganizationIDFromDB(db *gorm.DB) (string, error) {
	if db == nil || db.Statement == nil {
		return "", fmt.Errorf("missing organization context")
	}
	if info := requestmeta.FromContext(db.Statement.Context); info != nil {
		if orgID := strings.TrimSpace(info.OrgID); orgID != "" {
			return NormalizeOrganizationID(orgID), nil
		}
		return "", fmt.Errorf("missing organization context")
	}
	if tenantID, err := requestmeta.TenantID(db.Statement.Context); err == nil {
		if orgID := strings.TrimSpace(tenantID); orgID != "" {
			return NormalizeOrganizationID(orgID), nil
		}
	}
	return "", fmt.Errorf("missing organization context")
}

func statementHasOrganizationField(db *gorm.DB) bool {
	return db != nil &&
		db.Statement != nil &&
		db.Statement.Schema != nil &&
		db.Statement.Schema.LookUpField("OrgID") != nil
}

func statementOrganizationScopeColumn(db *gorm.DB) (clause.Column, bool) {
	if statementHasOrganizationField(db) {
		return clause.Column{Table: clause.CurrentTable, Name: "org_id"}, true
	}
	if db == nil || db.Statement == nil {
		return clause.Column{}, false
	}
	if isOrganizationScopedTable(db.Statement.Table) {
		return clause.Column{Table: clause.CurrentTable, Name: "org_id"}, true
	}
	return clause.Column{}, false
}

func applyOrganizationScope(db *gorm.DB) {
	orgID := statementOrganizationID(db)
	column, ok := statementOrganizationScopeColumn(db)
	if orgID == "" || !ok {
		return
	}
	db.Statement.AddClause(clause.Where{Exprs: []clause.Expression{
		clause.Eq{
			Column: column,
			Value:  orgID,
		},
	}})
}

func setCreateOrganizationID(db *gorm.DB) {
	orgID := statementOrganizationID(db)
	if orgID == "" || !statementHasOrganizationField(db) {
		return
	}
	field := db.Statement.Schema.LookUpField("OrgID")
	if field == nil {
		return
	}
	value := db.Statement.ReflectValue
	setOrgID := func(v reflect.Value) {
		for v.IsValid() && v.Kind() == reflect.Pointer {
			if v.IsNil() {
				return
			}
			v = v.Elem()
		}
		if !v.IsValid() || v.Kind() != reflect.Struct {
			return
		}
		current, zero := field.ValueOf(db.Statement.Context, v)
		if !zero && strings.TrimSpace(fmt.Sprint(current)) != "" {
			return
		}
		_ = field.Set(db.Statement.Context, v, orgID)
	}

	for value.IsValid() && value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		value = value.Elem()
	}
	switch value.Kind() {
	case reflect.Struct:
		setOrgID(value)
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			setOrgID(value.Index(i))
		}
	}
}

type envOrganization struct {
	OrgID               string            `json:"org_id"`
	Name                string            `json:"name"`
	CorpID              string            `json:"corp_id"`
	DingTalkAppKey      string            `json:"dingtalk_app_key"`
	DingTalkSecret      string            `json:"dingtalk_secret"`
	DingTalkAgentID     string            `json:"dingtalk_agent_id"`
	DingTalkAdminUserID string            `json:"dingtalk_admin_user_id"`
	AdminUserID         string            `json:"admin_user_id"`
	AppKey              string            `json:"app_key"`
	AppSecret           string            `json:"app_secret"`
	AgentID             string            `json:"agent_id"`
	AppHomeURL          string            `json:"app_home_url"`
	RedirectURI         string            `json:"redirect_uri"`
	Status              string            `json:"status"`
	ProcessCodes        map[string]string `json:"process_codes"`
}

func migrateOrganizationData() error {
	if err := seedConfiguredOrganizations(); err != nil {
		return err
	}
	for _, table := range organizationScopedTables() {
		if err := ensureOrganizationColumn(table); err != nil {
			return err
		}
		if err := backfillOrganizationColumn(table); err != nil {
			return err
		}
	}
	if DB.Migrator().HasTable(&User{}) {
		if err := ensureUserDingTalkColumn(); err != nil {
			return err
		}
		// Fail-closed same-org collision audit before set-based backfill.
		if err := backfillUserDingTalkID("user_id"); err != nil {
			return err
		}
	}
	if DB.Migrator().HasTable(&Department{}) {
		if err := ensureDepartmentDingTalkColumn(); err != nil {
			return err
		}
		if err := DB.Exec("UPDATE `departments` SET `dingtalk_department_id` = `department_id` WHERE (`dingtalk_department_id` IS NULL OR `dingtalk_department_id` = '') AND `department_id` <> ''").Error; err != nil {
			return err
		}
	}
	return nil
}

func migrateSyncStatusOrganizationScope() error {
	if !DB.Migrator().HasTable(&SyncStatus{}) {
		return nil
	}
	if err := dropUniqueIndexesForColumn("sync_statuses", "type", "idx_sync_statuses_org_type"); err != nil {
		return err
	}
	return ensureCompositeUniqueIndex("sync_statuses", "idx_sync_statuses_org_type", "org_id", "type")
}

func migrateRolePermissionOrganizationScope() error {
	if !DB.Migrator().HasTable(&Role{}) {
		return nil
	}

	// Role/user-role organization ownership is now governed by the unified
	// organization composite-index migration. Do not infer or rewrite
	// user_roles.org_id from users.user_id here: the same user_id may legally
	// exist in multiple organizations, and rewriting after the unique index is
	// installed can turn valid cross-org rows into an unreviewed collision.
	return cloneGlobalRoleConfigsToOrganizations()
}

func cloneGlobalRoleConfigsToOrganizations() error {
	orgIDs, err := activeOrganizationIDs()
	if err != nil || len(orgIDs) <= 1 {
		return err
	}

	var sourceRoles []Role
	if err := DB.Where("org_id = ? AND deleted_at IS NULL", DefaultOrganizationID).Order("id ASC").Find(&sourceRoles).Error; err != nil {
		return err
	}
	if len(sourceRoles) == 0 {
		return nil
	}

	sourceRoleIDs := make([]uint, 0, len(sourceRoles))
	for _, role := range sourceRoles {
		sourceRoleIDs = append(sourceRoleIDs, role.ID)
	}

	rolePermissionsByRoleID := make(map[uint][]uint, len(sourceRoleIDs))
	if len(sourceRoleIDs) > 0 {
		var mappings []RolePermission
		if err := DB.Where("role_id IN ? AND deleted_at IS NULL", sourceRoleIDs).Find(&mappings).Error; err != nil {
			return err
		}
		for _, mapping := range mappings {
			rolePermissionsByRoleID[mapping.RoleID] = append(rolePermissionsByRoleID[mapping.RoleID], mapping.PermissionID)
		}
	}

	menuPermissionsByRoleID := make(map[uint]MenuPermission, len(sourceRoleIDs))
	var menuPermissions []MenuPermission
	if err := DB.Where("org_id = ? AND role_id IN ? AND deleted_at IS NULL", DefaultOrganizationID, sourceRoleIDs).Find(&menuPermissions).Error; err != nil {
		return err
	}
	for _, item := range menuPermissions {
		menuPermissionsByRoleID[item.RoleID] = item
	}

	dataPermissionsByRoleID := make(map[uint]DataPermission, len(sourceRoleIDs))
	var dataPermissions []DataPermission
	if err := DB.Where("org_id = ? AND role_id IN ? AND deleted_at IS NULL", DefaultOrganizationID, sourceRoleIDs).Find(&dataPermissions).Error; err != nil {
		return err
	}
	for _, item := range dataPermissions {
		dataPermissionsByRoleID[item.RoleID] = item
	}

	for _, orgID := range orgIDs {
		if orgID == DefaultOrganizationID {
			continue
		}
		for _, sourceRole := range sourceRoles {
			targetRole, err := ensureOrganizationRoleClone(orgID, sourceRole)
			if err != nil {
				return err
			}
			for _, permissionID := range rolePermissionsByRoleID[sourceRole.ID] {
				if err := ensureRolePermissionBinding(targetRole.ID, permissionID); err != nil {
					return err
				}
			}
			if item, ok := menuPermissionsByRoleID[sourceRole.ID]; ok {
				if err := ensureOrganizationMenuPermission(orgID, targetRole.ID, item.MenuKeys); err != nil {
					return err
				}
			}
			if item, ok := dataPermissionsByRoleID[sourceRole.ID]; ok {
				if err := ensureOrganizationDataPermission(orgID, targetRole.ID, item.Scope, item.DepartmentKeys); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func remapUserRolesToOrganizationRoles() error {
	type userRoleBinding struct {
		UserRoleID uint
		RoleID     uint
		UserOrgID  string
		RoleName   string
	}

	var bindings []userRoleBinding
	if err := DB.Table("user_roles").
		Select("user_roles.id AS user_role_id, user_roles.role_id AS role_id, users.org_id AS user_org_id, roles.name AS role_name").
		Joins("JOIN users ON users.user_id = user_roles.user_id AND users.deleted_at IS NULL").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.deleted_at IS NULL").
		Where("user_roles.deleted_at IS NULL").
		Scan(&bindings).Error; err != nil {
		return err
	}

	for _, binding := range bindings {
		targetOrgID := NormalizeOrganizationID(binding.UserOrgID)
		var targetRole Role
		if err := DB.Where("org_id = ? AND name = ? AND deleted_at IS NULL", targetOrgID, binding.RoleName).First(&targetRole).Error; err != nil {
			return err
		}

		updates := map[string]interface{}{"org_id": targetOrgID}
		if targetRole.ID != binding.RoleID {
			updates["role_id"] = targetRole.ID
		}
		if err := DB.Model(&UserRole{}).Where("id = ?", binding.UserRoleID).Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}

func ensureOrganizationRoleClone(orgID string, source Role) (*Role, error) {
	orgID = NormalizeOrganizationID(orgID)
	var existing Role
	err := DB.Unscoped().Where("org_id = ? AND name = ?", orgID, source.Name).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		role := &Role{
			OrgID:       orgID,
			Name:        source.Name,
			Description: source.Description,
		}
		return role, DB.Create(role).Error
	}
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	if strings.TrimSpace(existing.Description) == "" && strings.TrimSpace(source.Description) != "" {
		updates["description"] = source.Description
	}
	if len(updates) > 0 {
		if err := DB.Unscoped().Model(&existing).Updates(updates).Error; err != nil {
			return nil, err
		}
	}
	return &existing, nil
}

func ensureRolePermissionBinding(roleID, permissionID uint) error {
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

func ensureOrganizationMenuPermission(orgID string, roleID uint, menuKeys string) error {
	orgID = NormalizeOrganizationID(orgID)
	var existing MenuPermission
	err := DB.Unscoped().
		Where("org_id = ? AND role_id = ?", orgID, roleID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return DB.Create(&MenuPermission{OrgID: orgID, RoleID: roleID, MenuKeys: menuKeys}).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	if strings.TrimSpace(existing.MenuKeys) == "" && strings.TrimSpace(menuKeys) != "" {
		updates["menu_keys"] = menuKeys
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Unscoped().Model(&existing).Updates(updates).Error
}

func ensureOrganizationDataPermission(orgID string, roleID uint, scope, departmentKeys string) error {
	orgID = NormalizeOrganizationID(orgID)
	var existing DataPermission
	err := DB.Unscoped().
		Where("org_id = ? AND role_id = ?", orgID, roleID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return DB.Create(&DataPermission{
			OrgID:          orgID,
			RoleID:         roleID,
			Scope:          scope,
			DepartmentKeys: departmentKeys,
		}).Error
	}
	if err != nil {
		return err
	}
	updates := map[string]interface{}{}
	if existing.DeletedAt.Valid {
		updates["deleted_at"] = nil
	}
	if strings.TrimSpace(existing.Scope) == "" && strings.TrimSpace(scope) != "" {
		updates["scope"] = scope
	}
	if strings.TrimSpace(existing.DepartmentKeys) == "" && strings.TrimSpace(departmentKeys) != "" {
		updates["department_keys"] = departmentKeys
	}
	if len(updates) == 0 {
		return nil
	}
	return DB.Unscoped().Model(&existing).Updates(updates).Error
}

func activeOrganizationIDs() ([]string, error) {
	if !DB.Migrator().HasTable(&Organization{}) {
		return []string{DefaultOrganizationID}, nil
	}
	var orgs []Organization
	if err := DB.Where("status = ? AND deleted_at IS NULL", "active").Order("org_id ASC").Find(&orgs).Error; err != nil {
		return nil, err
	}
	values := make([]string, 0, len(orgs)+1)
	values = append(values, DefaultOrganizationID)
	for _, org := range orgs {
		values = append(values, NormalizeOrganizationID(org.OrgID))
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, nil
}

func dropUniqueIndexesForColumn(table, column, keepIndex string) error {
	exists, err := tableExists(table)
	if err != nil || !exists {
		return err
	}

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
	`, table, column).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		indexName := strings.TrimSpace(row.IndexName)
		if indexName == "" || indexName == keepIndex || indexName == "PRIMARY" {
			continue
		}
		if err := DB.Exec(fmt.Sprintf("DROP INDEX %s ON %s", quoteIdentifier(indexName), quoteIdentifier(table))).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureCompositeUniqueIndex(table, index string, columns ...string) error {
	exists, err := tableExists(table)
	if err != nil || !exists {
		return err
	}
	hasIndex, err := indexExists(table, index)
	if err != nil {
		return err
	}
	if hasIndex {
		return nil
	}

	parts := make([]string, 0, len(columns))
	for _, column := range columns {
		parts = append(parts, quoteIdentifier(column))
	}
	return DB.Exec(fmt.Sprintf(
		"CREATE UNIQUE INDEX %s ON %s (%s)",
		quoteIdentifier(index),
		quoteIdentifier(table),
		strings.Join(parts, ", "),
	)).Error
}

func ensureUserDingTalkColumn() error {
	hasColumn, err := columnExists("users", "ding_talk_user_id")
	if err != nil {
		return err
	}
	if !hasColumn {
		if err := DB.Exec("ALTER TABLE `users` ADD COLUMN `ding_talk_user_id` varchar(64)").Error; err != nil {
			return err
		}
	}
	legacyColumn, err := columnExists("users", "dingtalk_user_id")
	if err != nil {
		return err
	}
	if legacyColumn {
		if err := DB.Exec("UPDATE `users` SET `ding_talk_user_id` = `dingtalk_user_id` WHERE (`ding_talk_user_id` IS NULL OR `ding_talk_user_id` = '') AND `dingtalk_user_id` <> ''").Error; err != nil {
			return err
		}
	}
	hasIndex, err := indexExists("users", "idx_users_org_dingtalk_user")
	if err != nil {
		return err
	}
	if !hasIndex {
		err := DB.Exec("CREATE UNIQUE INDEX `idx_users_org_dingtalk_user` ON `users` (`org_id`, `ding_talk_user_id`)").Error
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
			return err
		}
	}
	return nil
}

func ensureDepartmentDingTalkColumn() error {
	hasColumn, err := columnExists("departments", "dingtalk_department_id")
	if err != nil {
		return err
	}
	if !hasColumn {
		if err := DB.Exec("ALTER TABLE `departments` ADD COLUMN `dingtalk_department_id` varchar(64)").Error; err != nil {
			return err
		}
	}
	hasIndex, err := indexExists("departments", "idx_departments_org_dingtalk_department")
	if err != nil {
		return err
	}
	if !hasIndex {
		err := DB.Exec("CREATE UNIQUE INDEX `idx_departments_org_dingtalk_department` ON `departments` (`org_id`, `dingtalk_department_id`)").Error
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
			return err
		}
	}
	return nil
}

func seedConfiguredOrganizations() error {
	orgs := []Organization{defaultOrganizationFromEnv()}
	raw := strings.TrimSpace(os.Getenv("DINGTALK_ORGANIZATIONS"))
	if raw != "" {
		var configured []envOrganization
		if err := json.Unmarshal([]byte(raw), &configured); err != nil {
			return fmt.Errorf("invalid DINGTALK_ORGANIZATIONS: %w", err)
		}
		for _, cfg := range configured {
			orgs = append(orgs, organizationFromEnvConfig(cfg))
		}
	}

	for _, org := range orgs {
		org.OrgID = NormalizeOrganizationID(org.OrgID)
		org.Name = strings.TrimSpace(org.Name)
		if org.Name == "" {
			org.Name = org.OrgID
		}
		if strings.TrimSpace(org.Status) == "" {
			org.Status = "active"
		}
		var existing Organization
		err := DB.Where("org_id = ?", org.OrgID).First(&existing).Error
		if err == nil {
			updates := map[string]interface{}{
				"name":   org.Name,
				"status": org.Status,
			}
			if org.CorpID != "" {
				updates["corp_id"] = org.CorpID
			}
			if org.DingTalkAppKey != "" {
				updates["ding_talk_app_key"] = org.DingTalkAppKey
			}
			if org.DingTalkSecret != "" {
				updates["ding_talk_secret"] = org.DingTalkSecret
			}
			if org.DingTalkAgentID != "" {
				updates["ding_talk_agent_id"] = org.DingTalkAgentID
			}
			if org.AppHomeURL != "" {
				updates["app_home_url"] = org.AppHomeURL
			}
			if org.RedirectURI != "" {
				updates["redirect_uri"] = org.RedirectURI
			}
			if err := DB.Model(&Organization{}).Where("org_id = ?", org.OrgID).Updates(updates).Error; err != nil {
				return err
			}
			continue
		}
		if err != gorm.ErrRecordNotFound {
			return err
		}
		if err := DB.Create(&org).Error; err != nil {
			return err
		}
	}
	return nil
}

func defaultOrganizationFromEnv() Organization {
	return Organization{
		OrgID:           DefaultOrganizationID,
		Name:            getEnvOrDefault("PEOPLEOPS_DEFAULT_ORG_NAME", "Default Organization"),
		CorpID:          os.Getenv("DINGTALK_CORP_ID"),
		DingTalkAppKey:  os.Getenv("DINGTALK_APP_KEY"),
		DingTalkSecret:  os.Getenv("DINGTALK_APP_SECRET"),
		DingTalkAgentID: os.Getenv("DINGTALK_AGENT_ID"),
		AppHomeURL:      os.Getenv("DINGTALK_APP_HOME_URL"),
		RedirectURI:     os.Getenv("DINGTALK_REDIRECT_URI"),
		Status:          "active",
	}
}

func organizationFromEnvConfig(cfg envOrganization) Organization {
	return Organization{
		OrgID:           cfg.OrgID,
		Name:            cfg.Name,
		CorpID:          cfg.CorpID,
		DingTalkAppKey:  firstNonEmpty(cfg.DingTalkAppKey, cfg.AppKey),
		DingTalkSecret:  firstNonEmpty(cfg.DingTalkSecret, cfg.AppSecret),
		DingTalkAgentID: firstNonEmpty(cfg.DingTalkAgentID, cfg.AgentID),
		AppHomeURL:      cfg.AppHomeURL,
		RedirectURI:     cfg.RedirectURI,
		Status:          cfg.Status,
		Extension: organizationExtensionWithDingTalkProcessCodes(
			nil,
			cfg.ProcessCodes,
		),
	}
}

const dingTalkProcessCodesExtensionKey = "dingtalk_process_codes"

func organizationExtensionWithDingTalkProcessCodes(extension map[string]interface{}, processCodes map[string]string) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range extension {
		out[k] = v
	}
	if len(processCodes) == 0 {
		return out
	}
	codes := map[string]string{}
	for k, v := range processCodes {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		codes[key] = val
	}
	if len(codes) == 0 {
		return out
	}
	out[dingTalkProcessCodesExtensionKey] = codes
	return out
}

func organizationDingTalkProcessCodes(extension map[string]interface{}) map[string]string {
	if extension == nil {
		return map[string]string{}
	}
	raw, ok := extension[dingTalkProcessCodesExtensionKey]
	if !ok || raw == nil {
		return map[string]string{}
	}
	switch typed := raw.(type) {
	case map[string]string:
		out := make(map[string]string, len(typed))
		for k, v := range typed {
			k = strings.TrimSpace(k)
			v = strings.TrimSpace(v)
			if k != "" && v != "" {
				out[k] = v
			}
		}
		return out
	case map[string]interface{}:
		out := make(map[string]string, len(typed))
		for k, v := range typed {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if s, ok := v.(string); ok {
				s = strings.TrimSpace(s)
				if s != "" {
					out[k] = s
				}
			}
		}
		return out
	default:
		return map[string]string{}
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func organizationScopedTables() []string {
	// Authoritative list is organizationScopedTableNameSet. Do not rely on package
	// global DB / GORM Statement.Parse (nil DB panics under unit tests).
	tables := make([]string, 0, len(organizationScopedTableNameSet))
	for table := range organizationScopedTableNameSet {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}

// organizationScopedModelTables is retained for callers that still want model-driven
// discovery when a live DB is available; production scoping uses the static set.
func organizationScopedModelTables() []string {
	models := []interface{}{
		&User{},
		&Department{},
		&DepartmentChangeLog{},
		&Attendance{},
		&Approval{},
		&ApprovalTemplate{},
		&Role{},
		&UserRole{},
		&MenuPermission{},
		&DataPermission{},
		&OperationLog{},
		&SyncStatus{},
		&IdempotencyRecord{},
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
		&StatutoryHoliday{},
		&LeaveRuleConfig{},
		&AnnualLeaveEligibility{},
		&AnnualLeaveGrant{},
		&OvertimeRuleConfig{},
		&OvertimeMatchResult{},
		&OvertimeSyncHistory{},
		&OvertimeSupplementaryRequest{},
		&CompensatoryLeaveLedger{},
		&AnnualLeaveConsumeLog{},
		&PerformanceTemplate{},
		&PerformanceTemplateSection{},
		&PerformanceTemplateItem{},
		&PerformanceLevelRule{},
		&PerformanceLevelRuleItem{},
		&PerformanceActivity{},
		&PerformanceDistributionRule{},
		&PerformanceDistributionException{},
		&PerformanceReminderLog{},
		&PerformanceInterviewRecord{},
		&PerformanceAppealRecord{},
		&PerformanceParticipant{},
		&PerformanceReview{},
		&PerformanceReviewVersion{},
		&PerformanceRelationshipChangeLog{},
		&PerformanceGoalRecord{},
		&PerformanceGoalApprovalLog{},
		&PerformanceCompanyFinance{},
		&PerformanceIndicatorLibrary{},
		&PerformanceIndicatorItem{},
	}
	tables := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		if DB == nil {
			break
		}
		stmt := &gorm.Statement{DB: DB}
		if err := stmt.Parse(model); err != nil || stmt.Schema == nil {
			continue
		}
		table := strings.TrimSpace(stmt.Schema.Table)
		if table == "" {
			continue
		}
		if _, ok := seen[table]; ok {
			continue
		}
		seen[table] = struct{}{}
		tables = append(tables, table)
	}
	return tables
}

var organizationScopedTableNameSet = map[string]struct{}{
	"users":                                {},
	"departments":                          {},
	"department_change_logs":               {},
	"attendances":                          {},
	"approvals":                            {},
	"approval_templates":                   {},
	"roles":                                {},
	"user_roles":                           {},
	"menu_permissions":                     {},
	"data_permissions":                     {},
	"operation_logs":                       {},
	"sync_statuses":                        {},
	"idempotency_records":                  {},
	"ding_talk_bindings":                   {},
	"user_sessions":                        {},
	"login_logs":                           {},
	"attendance_exports":                   {},
	"employee_profiles":                    {},
	"employee_transfers":                   {},
	"employee_resignations":                {},
	"employee_onboardings":                 {},
	"talent_analyses":                      {},
	"employee_shift_configs":               {},
	"dingtalk_shift_catalogs":              {},
	"week_schedule_rules":                  {},
	"week_schedule_overrides":              {},
	"week_schedule_sync_logs":              {},
	"statutory_holidays":                   {},
	"leave_rule_configs":                   {},
	"annual_leave_eligibilities":           {},
	"annual_leave_grants":                  {},
	"overtime_rule_configs":                {},
	"overtime_match_results":               {},
	"overtime_sync_histories":              {},
	"overtime_supplementary_requests":      {},
	"compensatory_leave_ledgers":           {},
	"annual_leave_consume_logs":            {},
	"performance_templates":                {},
	"performance_template_sections":        {},
	"performance_template_items":           {},
	"performance_level_rules":              {},
	"performance_level_rule_items":         {},
	"performance_activities":               {},
	"performance_distribution_rules":       {},
	"performance_distribution_exceptions":  {},
	"performance_reminder_logs":            {},
	"performance_interview_records":        {},
	"performance_appeal_records":           {},
	"performance_participants":             {},
	"performance_reviews":                  {},
	"performance_review_versions":          {},
	"performance_relationship_change_logs": {},
	"performance_goal_records":             {},
	"performance_goal_approval_logs":       {},
	"performance_import_batches":           {},
	"performance_company_finances":         {},
	"performance_indicator_libraries":      {},
	"performance_indicator_items":          {},
	"uploaded_files":                       {},
	"organization_users":                   {},
	"ding_talk_event_logs":                 {},
	"external_attendance_raw":              {},
	"external_attendance_approve_links":    {},
	"external_user_department_raw":         {},
	"user_department_relations":            {},
	"external_sync_cursors":                {},
	"external_sync_jobs":                   {},
	"external_sync_locks":                  {},
}

func isOrganizationScopedTable(table string) bool {
	table = normalizeStatementTableName(table)
	if table == "" {
		return false
	}
	_, ok := organizationScopedTableNameSet[table]
	return ok
}

func normalizeStatementTableName(table string) string {
	table = strings.TrimSpace(table)
	if table == "" {
		return ""
	}
	table = strings.Trim(table, "`")
	fields := strings.Fields(table)
	if len(fields) > 0 {
		table = fields[0]
	}
	table = strings.Trim(table, "`")
	if idx := strings.LastIndex(table, "."); idx >= 0 {
		table = table[idx+1:]
	}
	return strings.Trim(table, "`")
}

func ensureOrganizationColumn(table string) error {
	exists, err := tableExists(table)
	if err != nil || !exists {
		return err
	}
	hasColumn, err := columnExists(table, "org_id")
	if err != nil {
		return err
	}
	if !hasColumn {
		if err := DB.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN `org_id` varchar(64) NOT NULL DEFAULT 'default'", quoteIdentifier(table))).Error; err != nil {
			return err
		}
	}
	indexName := organizationIndexName(table)
	hasIndex, err := indexExists(table, indexName)
	if err != nil {
		return err
	}
	if !hasIndex {
		err := DB.Exec(fmt.Sprintf("CREATE INDEX %s ON %s (`org_id`)", quoteIdentifier(indexName), quoteIdentifier(table))).Error
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate key name") {
			return err
		}
	}
	return nil
}

func backfillOrganizationColumn(table string) error {
	exists, err := tableExists(table)
	if err != nil || !exists {
		return err
	}
	hasColumn, err := columnExists(table, "org_id")
	if err != nil || !hasColumn {
		return err
	}
	return DB.Exec(fmt.Sprintf("UPDATE %s SET `org_id` = ? WHERE `org_id` IS NULL OR `org_id` = ''", quoteIdentifier(table)), DefaultOrganizationID).Error
}

func tableExists(table string) (bool, error) {
	var count int64
	err := DB.Raw("SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", table).Scan(&count).Error
	return count > 0, err
}

func columnExists(table, column string) (bool, error) {
	var count int64
	err := DB.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", table, column).Scan(&count).Error
	return count > 0, err
}

func indexExists(table, index string) (bool, error) {
	var count int64
	err := DB.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?", table, index).Scan(&count).Error
	return count > 0, err
}

func organizationIndexName(table string) string {
	name := "idx_" + table + "_org_id"
	if len(name) <= 60 {
		return name
	}
	return name[:60]
}

func quoteIdentifier(identifier string) string {
	return "`" + strings.ReplaceAll(identifier, "`", "``") + "`"
}

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

	// statutory_holidays 手动建表，避免 GORM AutoMigrate 的索引 DROP FOREIGN KEY 问题。
	// Phase 3B 只做 nullable org_id expand；唯一索引收口留到 contract 阶段。
	if err := DB.Exec("CREATE TABLE IF NOT EXISTS `statutory_holidays` (`id` bigint unsigned AUTO_INCREMENT PRIMARY KEY, `org_id` varchar(64) NULL, `date` varchar(32) NOT NULL, `name` varchar(128) NOT NULL, `type` varchar(32) NOT NULL, `year` int NOT NULL, `created_at` datetime(3), `updated_at` datetime(3), UNIQUE INDEX `uni_statutory_holidays_date` (`date`), INDEX `idx_statutory_holidays_org_id` (`org_id`))").Error; err != nil {
		return err
	}
	if err := DB.Exec("CREATE TABLE IF NOT EXISTS `employee_shift_configs` (`id` bigint unsigned AUTO_INCREMENT PRIMARY KEY, `created_at` datetime(3), `updated_at` datetime(3), `deleted_at` datetime(3), `org_id` varchar(64) NULL, `user_id` varchar(64) NOT NULL, `user_name` varchar(128), `shift_id` bigint NOT NULL, `end_time` varchar(16), `note` varchar(256), UNIQUE INDEX `idx_employee_shift_configs_user_id` (`user_id`), INDEX `idx_employee_shift_configs_deleted_at` (`deleted_at`), INDEX `idx_employee_shift_configs_org_id` (`org_id`))").Error; err != nil {
		return err
	}
	if err := DB.Exec("CREATE TABLE IF NOT EXISTS `dingtalk_shift_catalogs` (`id` bigint unsigned AUTO_INCREMENT PRIMARY KEY, `org_id` varchar(64) NULL, `name` varchar(128) NOT NULL, `shift_key` varchar(256) NOT NULL, `shift_id` bigint NOT NULL, `check_in` varchar(16), `check_out` varchar(16), `created_at` datetime(3), `updated_at` datetime(3), UNIQUE INDEX `idx_dingtalk_shift_catalogs_shift_key` (`shift_key`), INDEX `idx_dingtalk_shift_catalogs_name` (`name`), INDEX `idx_dingtalk_shift_catalogs_org_id` (`org_id`))").Error; err != nil {
		return err
	}
	if err := ensureNullableOrgIDColumn("statutory_holidays"); err != nil {
		return err
	}
	if err := ensureNullableOrgIDColumn("employee_shift_configs"); err != nil {
		return err
	}
	if err := ensureNullableOrgIDColumn("dingtalk_shift_catalogs"); err != nil {
		return err
	}

	// 建新表（年假/调休）优先，不依赖其他表
	// Upgrade every existing tenant-scoped unique key before AutoMigrate.
	// This audits same-org conflicts and removes legacy global unique indexes first.
	if err := PrepareOrgCompositeUniqueIndexes(DB); err != nil {
		return fmt.Errorf("prepare organization composite unique indexes: %w", err)
	}

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
	if err := DB.AutoMigrate(&AnnualLeaveConsumeLog{}); err != nil {
		log.Printf("[migrate] AnnualLeaveConsumeLog 迁移失败（忽略）: %v", err)
	}
	if err := migrateAnnualLeaveConsumeLogSchema(); err != nil {
		return err
	}
	if err := migrateOvertimeMatchSchema(); err != nil {
		return err
	}
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
		&DingTalkEventLog{},
		&Role{},
		&Permission{},
		&RolePermission{},
		&UserRole{},
		&MenuPermission{},
		&DataPermission{},
		&OperationLog{},
		&IdempotencyRecord{},
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
		&PerformanceInterviewRecord{},
		&PerformanceAppealRecord{},
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

	if err := migrateOrganizationData(); err != nil {
		return err
	}
	if err := migrateRolePermissionOrganizationScope(); err != nil {
		return err
	}
	if err := migrateSyncStatusOrganizationScope(); err != nil {
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
	if err := MigratePerformanceParticipantOrgIDsFromActivity(DB); err != nil {
		return err
	}
	if err := migrateMutengParticipantPipeline(); err != nil {
		return err
	}

	// Verify the complete tenant unique-index contract after AutoMigrate and legacy field migrations.
	if err := MigrateOrgCompositeUniqueIndexes(DB); err != nil {
		return fmt.Errorf("verify organization composite unique indexes: %w", err)
	}

	return nil
}

func ensureNullableOrgIDColumn(table string) error {
	var count int64
	if err := DB.Raw("SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND COLUMN_NAME='org_id'", table).Scan(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		if err := DB.Exec(fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `org_id` varchar(64) NULL AFTER `id`", table)).Error; err != nil {
			return err
		}
	}
	indexName := fmt.Sprintf("idx_%s_org_id", table)
	var indexCount int64
	if err := DB.Raw("SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME=? AND INDEX_NAME=?", table, indexName).Scan(&indexCount).Error; err != nil {
		return err
	}
	if indexCount == 0 {
		if err := DB.Exec(fmt.Sprintf("CREATE INDEX `%s` ON `%s` (`org_id`)", indexName, table)).Error; err != nil {
			return err
		}
	}
	return nil
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

// migrateMutengParticipantPipeline converts in-progress Muteng activities from
// the historical aggregate stage flow to participant-level progression. It is
// intentionally idempotent and leaves locked/archived activities untouched.
func migrateMutengParticipantPipeline() error {
	if !DB.Migrator().HasTable(&PerformanceActivity{}) || !DB.Migrator().HasTable(&PerformanceParticipant{}) {
		return nil
	}

	activeStatuses := mutengParticipantPipelineActiveActivityStatuses()
	var activities []PerformanceActivity
	if err := DB.Where("flow_type = ? AND status IN ? AND deleted_at IS NULL", "new", activeStatuses).
		Find(&activities).Error; err != nil {
		return err
	}

	for i := range activities {
		activity := activities[i]
		if err := DB.Transaction(func(tx *gorm.DB) error {
			var participants []PerformanceParticipant
			if err := tx.Where("activity_id = ? AND deleted_at IS NULL", activity.ID).Find(&participants).Error; err != nil {
				return err
			}

			now := time.Now()
			activeCount := 0
			completedCount := 0
			for j := range participants {
				participant := &participants[j]
				result := buildMutengParticipantPipelineUpdates(mutengParticipantPipelineInput{
					Status:              participant.Status,
					ResultHidden:        participant.ResultHidden,
					ResultHiddenReason:  participant.ResultHiddenReason,
					HRConfirmedAt:       participant.HRConfirmedAt,
					HRConfirmedBy:       participant.HRConfirmedBy,
					EmployeeConfirmedAt: participant.EmployeeConfirmedAt,
					EmployeeConfirmedBy: participant.EmployeeConfirmedBy,
					ConfirmedAt:         participant.ConfirmedAt,
					ConfirmedBy:         participant.ConfirmedBy,
				}, now)
				if result.CountedActive {
					activeCount++
				}
				if result.CountedCompleted {
					completedCount++
				}
				if len(result.Updates) == 0 {
					continue
				}
				if err := tx.Model(&PerformanceParticipant{}).Where("id = ?", participant.ID).Updates(result.Updates).Error; err != nil {
					return err
				}
			}

			nextStatus := migrateMutengActivityAggregateStatus(activity.ActivityKind, activity.Status, activeCount, completedCount)
			if nextStatus != activity.Status {
				return tx.Model(&PerformanceActivity{}).Where("id = ?", activity.ID).Updates(map[string]interface{}{
					"status":     nextStatus,
					"updated_by": "system:muteng-participant-pipeline",
				}).Error
			}
			return nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func mutengParticipantPipelineActiveActivityStatuses() []string {
	return []string{
		"target_setting", "target_approval", "self_evaluation", "manager_evaluation",
		"department_evaluation", "hr_review", "employee_confirmation",
		"manager_confirmation", "hr_confirmation", "result_publish",
	}
}

// mutengParticipantPipelineInput is the minimal participant snapshot used by the
// pure migration mapper. Keeping it pure lets unit tests cover the historical
// status remaps without a live database.
type mutengParticipantPipelineInput struct {
	Status              string
	ResultHidden        bool
	ResultHiddenReason  string
	HRConfirmedAt       *time.Time
	HRConfirmedBy       string
	EmployeeConfirmedAt *time.Time
	EmployeeConfirmedBy string
	ConfirmedAt         *time.Time
	ConfirmedBy         string
}

type mutengParticipantPipelineResult struct {
	Status           string
	Updates          map[string]interface{}
	CountedActive    bool
	CountedCompleted bool
}

// buildMutengParticipantPipelineUpdates maps one participant into the new
// Muteng independent-pipeline shape. Semantics must stay identical to the
// original migration:
//   - inactive / removed_from_scope are ignored for aggregate completion
//   - employee_confirmed becomes hr_confirmed (already published historically)
//   - manager_recheck becomes self_submitted and unlocks
//   - only system:unpublished is auto-cleared on published participants
//   - manual hide reasons are preserved
//   - unfinished participants without a hide flag receive system:unpublished
func buildMutengParticipantPipelineUpdates(participant mutengParticipantPipelineInput, now time.Time) mutengParticipantPipelineResult {
	status := strings.TrimSpace(participant.Status)
	if status == "inactive" || status == "removed_from_scope" {
		return mutengParticipantPipelineResult{}
	}

	result := mutengParticipantPipelineResult{
		Status:        status,
		Updates:       map[string]interface{}{},
		CountedActive: true,
	}

	if status == "employee_confirmed" {
		// Historical Muteng employee confirmation happened only after
		// HR confirmation, so it is equivalent to a published result.
		status = "hr_confirmed"
		result.Status = status
		result.Updates["status"] = status
		if participant.HRConfirmedAt == nil {
			publishedAt := participant.EmployeeConfirmedAt
			if publishedAt == nil {
				publishedAt = participant.ConfirmedAt
			}
			if publishedAt == nil {
				publishedAt = &now
			}
			result.Updates["hr_confirmed_at"] = publishedAt
		}
		if strings.TrimSpace(participant.HRConfirmedBy) == "" {
			result.Updates["hr_confirmed_by"] = firstNonEmptyString(participant.EmployeeConfirmedBy, participant.ConfirmedBy, "system")
		}
	}
	if status == "manager_recheck" {
		status = "self_submitted"
		result.Status = status
		result.Updates["status"] = status
		result.Updates["is_locked"] = false
		result.Updates["locked_at"] = nil
		result.Updates["locked_by"] = ""
	}
	if status == "hr_confirmed" {
		result.CountedCompleted = true
		if participant.ResultHidden && strings.TrimSpace(participant.ResultHiddenReason) == "system:unpublished" {
			result.Updates["result_hidden"] = false
			result.Updates["result_hidden_reason"] = ""
			result.Updates["result_hidden_at"] = nil
			result.Updates["result_hidden_by"] = ""
		}
		if participant.ConfirmedAt == nil {
			publishedAt := participant.HRConfirmedAt
			if value, ok := result.Updates["hr_confirmed_at"].(*time.Time); ok {
				publishedAt = value
			}
			if publishedAt == nil {
				publishedAt = &now
			}
			result.Updates["confirmed_at"] = publishedAt
			// Keep original firstNonEmptyString argument order for confirmed_by.
			result.Updates["confirmed_by"] = firstNonEmptyString(participant.HRConfirmedBy, participant.EmployeeConfirmedBy, participant.ConfirmedBy, "system")
		}
	} else if status == "locked" || status == "result_confirmed" {
		result.CountedCompleted = true
	} else if !participant.ResultHidden {
		result.Updates["result_hidden"] = true
		result.Updates["result_hidden_reason"] = "system:unpublished"
	}

	if len(result.Updates) > 0 {
		result.Updates["updated_by"] = "system:muteng-participant-pipeline"
	}
	return result
}

// migrateMutengActivityAggregateStatus decides the display/lifecycle activity
// status after participant remaps. It never blocks individual progress.
func migrateMutengActivityAggregateStatus(activityKind, currentStatus string, activeCount, completedCount int) string {
	nextStatus := strings.TrimSpace(currentStatus)
	if strings.TrimSpace(activityKind) == "goal_setting" {
		if nextStatus == "target_approval" {
			return "target_setting"
		}
		return nextStatus
	}
	if activeCount > 0 && completedCount == activeCount {
		return "result_publish"
	}
	if nextStatus != "target_setting" {
		return "self_evaluation"
	}
	return nextStatus
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
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
	// Organization-scoped unique indexes and their legacy data handling are
	// centralized in PrepareOrgCompositeUniqueIndexes/MigrateOrgCompositeUniqueIndexes.
	// Keep this compatibility hook side-effect free so old migrations cannot
	// rewrite org_id or delete business rows after the composite indexes exist.
	return nil
}

// migrateApprovalsOrgProcessUniqueIndex 将 approvals.process_id 单列唯一索引
// 收口为 (org_id, process_id) 复合唯一，避免跨企业 process_id 冲突。
func migrateApprovalsOrgProcessUniqueIndex() error {
	if DB == nil || !DB.Migrator().HasTable(&Approval{}) {
		return nil
	}

	// 历史空 org_id 先归一到 default，避免建复合唯一索引失败。
	if err := DB.Exec(`
		UPDATE approvals
		SET org_id = 'default'
		WHERE org_id IS NULL OR org_id = ''
	`).Error; err != nil {
		return err
	}

	// 探测并删除 process_id 单列唯一索引（GORM/历史命名可能不同）。
	type indexRow struct {
		IndexName string
	}
	var rows []indexRow
	if err := DB.Raw(`
		SELECT INDEX_NAME AS index_name
		FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE()
		  AND TABLE_NAME = 'approvals'
		  AND NON_UNIQUE = 0
		  AND INDEX_NAME <> 'PRIMARY'
		GROUP BY INDEX_NAME
		HAVING COUNT(*) = 1
		   AND MAX(COLUMN_NAME) = 'process_id'
	`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := dropIndexIfExists("approvals", row.IndexName); err != nil {
			return err
		}
	}
	// 兼容常见固定命名。
	for _, name := range []string{"uni_approvals_process_id", "process_id", "idx_approvals_process_id"} {
		if err := dropIndexIfExists("approvals", name); err != nil {
			return err
		}
	}

	return createIndexIfMissing("approvals", "idx_approvals_org_process", true, "org_id", "process_id")
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
	// Schema expand + request_ref backfill only; unique-index replacement is phase-4.
	return MigrateAnnualLeaveConsumeLogSchema(DB)
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

	// Only normalize missing legacy values. Organization ownership for an
	// existing row must not be inferred from user_id or deduplicated here.
	return DB.Exec(`
		UPDATE user_roles
		SET org_id = 'default'
		WHERE org_id IS NULL OR org_id = ''
	`).Error
}

func migrateShiftCatalogSchema() error {
	// Set-based schema expand + empty shift_key backfill; unique indexes are phase-4.
	return MigrateShiftCatalogSchema(DB)
}

func normalizeShiftCatalogKey(name, checkIn, checkOut string) string {
	return strings.ToLower(strings.TrimSpace(name)) + "|" + strings.TrimSpace(checkIn) + "|" + strings.TrimSpace(checkOut)
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
				OrgID:           cfg.OrgID,
				Name:            cfg.Name,
				CorpID:          cfg.CorpID,
				Status:          cfg.Status,
				DingTalkAppKey:  cfg.AppKey,
				DingTalkSecret:  cfg.AppSecret,
				DingTalkAgentID: cfg.AgentID,
				AppHomeURL:      cfg.AppHomeURL,
				RedirectURI:     cfg.RedirectURI,
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
		if strings.TrimSpace(org.DingTalkAgentID) == "" && cfg.AgentID != "" {
			updates["ding_talk_agent_id"] = cfg.AgentID
		}
		if strings.TrimSpace(org.DingTalkAppKey) == "" && cfg.AppKey != "" {
			updates["ding_talk_app_key"] = cfg.AppKey
		}
		if strings.TrimSpace(org.DingTalkSecret) == "" && cfg.AppSecret != "" {
			updates["ding_talk_secret"] = cfg.AppSecret
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
			{Name: "绩效HR确认", Code: "performance:hr_confirm:submit", Description: "旧流程HR确认绩效结果"},
			{Name: "绩效部门/中心评估", Code: "performance:department_eval:submit", Description: "部门/中心负责人确认或调整绩效结果"},
			{Name: "绩效HR审核", Code: "performance:hr_review:submit", Description: "HR审核沐腾科技流程绩效结果"},
			{Name: "绩效结果公布", Code: "performance:result_publish:manage", Description: "公布沐腾科技流程绩效结果"},
			{Name: "绩效结果屏蔽管理", Code: "performance:result_visibility:manage", Description: "设置或解除绩效结果屏蔽"},
			{Name: "绩效屏蔽结果查看", Code: "performance:hidden_result:view", Description: "查看已屏蔽的绩效结果"},
			{Name: "绩效面谈管理", Code: "performance:interview:manage", Description: "安排、记录和完成绩效面谈"},
			{Name: "绩效申诉处理", Code: "performance:appeal:manage", Description: "处理沐腾科技流程绩效申诉"},
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
		"performance:department_eval:submit", "performance:hr_review:submit", "performance:result_publish:manage",
		"performance:result_visibility:manage", "performance:hidden_result:view",
		"performance:interview:manage", "performance:appeal:manage", "performance:level_adjust:manage", "performance:distribution:manage", "performance:indicator:manage",
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
		"performance:result_visibility:manage", "performance:interview:manage", "performance:indicator:manage", "performance:goal:manage", "performance:assessment_manager:update",
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
			DB.Create(&UserRole{OrgID: NormalizeOrganizationID(admin.OrgID), UserID: admin.UserID, RoleID: adminID})
		}
	}
}

var liedeAdminOrgIDs = []string{"default", "xiaotie", "muteng"}

func migrateLiedeOrganizationAdminRoles() {
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
		orgID := strings.TrimSpace(user.OrgID)
		if orgID == "" {
			continue
		}
		usersByOrg[orgID] = append(usersByOrg[orgID], user)
	}

	for _, orgID := range liedeAdminOrgIDs {
		// Critical: admin role must belong to the same org as the user_roles row.
		// A cross-org role_id is filtered out by permission JOINs (roles.org_id = user_roles.org_id).
		adminRole, err := ensureRolePresetInOrg(orgID, "管理员", "系统管理员")
		if err != nil {
			log.Printf("[migrate] 确保组织管理员角色失败: org_id=%s err=%v", orgID, err)
			continue
		}
		if err := ensureAdminRoleFullAccessInOrg(orgID, adminRole.ID); err != nil {
			log.Printf("[migrate] 确保组织管理员角色权限失败: org_id=%s role_id=%d err=%v", orgID, adminRole.ID, err)
			continue
		}

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
			log.Printf("[migrate] 已确保列德为组织管理员: org_id=%s user_id=%s role_id=%d", orgID, user.UserID, adminRole.ID)
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
	// Avoid MySQL-only FIELD(); order is only for stable iteration and tests on SQLite.
	if err := DB.Where("deleted_at IS NULL AND name LIKE ?", "%列德%").
		Order("id ASC").
		Find(&nameMatches).Error; err != nil {
		return nil, err
	}
	appendUsers(nameMatches)

	if adminUserID := strings.TrimSpace(os.Getenv("DINGTALK_ADMIN_USER_ID")); adminUserID != "" {
		var envMatches []User
		if err := DB.Where("deleted_at IS NULL AND user_id = ?", adminUserID).
			Order("id ASC").
			Find(&envMatches).Error; err != nil {
			return nil, err
		}
		appendUsers(envMatches)
	}

	return users, nil
}

// remapCrossOrgUserRoleBindings rewrites user_roles whose role_id points at another
// organization's role. Permission resolution requires roles.org_id = user_roles.org_id.
func remapCrossOrgUserRoleBindings() error {
	if DB == nil {
		return nil
	}
	type poisonedBinding struct {
		UserRoleID uint   `gorm:"column:user_role_id"`
		OrgID      string `gorm:"column:org_id"`
		UserID     string `gorm:"column:user_id"`
		RoleID     uint   `gorm:"column:role_id"`
		RoleName   string `gorm:"column:role_name"`
	}
	var rows []poisonedBinding
	if err := DB.Table("user_roles").
		Select("user_roles.id AS user_role_id, user_roles.org_id AS org_id, user_roles.user_id AS user_id, user_roles.role_id AS role_id, roles.name AS role_name").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.deleted_at IS NULL").
		Where("user_roles.deleted_at IS NULL AND user_roles.org_id <> roles.org_id").
		Scan(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		orgID := strings.TrimSpace(row.OrgID)
		roleName := strings.TrimSpace(row.RoleName)
		if orgID == "" || roleName == "" || row.UserRoleID == 0 {
			continue
		}
		target, err := ensureRolePresetInOrg(orgID, roleName, roleName)
		if err != nil {
			return fmt.Errorf("ensure target role org=%s name=%s: %w", orgID, roleName, err)
		}
		if roleName == "管理员" {
			if err := ensureAdminRoleFullAccessInOrg(orgID, target.ID); err != nil {
				log.Printf("[migrate] ensure admin full access after remap failed: org_id=%s role_id=%d err=%v", orgID, target.ID, err)
			}
		}
		if target.ID == row.RoleID {
			continue
		}
		if err := DB.Model(&UserRole{}).Where("id = ?", row.UserRoleID).Update("role_id", target.ID).Error; err != nil {
			return fmt.Errorf("remap user_role id=%d: %w", row.UserRoleID, err)
		}
		log.Printf("[migrate] remapped cross-org user_role: org_id=%s user_id=%s role=%s %d->%d",
			orgID, row.UserID, roleName, row.RoleID, target.ID)
	}
	return nil
}

func ensureAdminRoleFullAccess(roleID uint) error {
	// Backward-compatible entry: treat role as belonging to default org only when
	// the caller does not know the org. Prefer ensureAdminRoleFullAccessInOrg.
	return ensureAdminRoleFullAccessInOrg(DefaultOrganizationID, roleID)
}

func ensureAdminRoleFullAccessInOrg(orgID string, roleID uint) error {
	orgID = NormalizeOrganizationID(strings.TrimSpace(orgID))
	if roleID == 0 {
		return fmt.Errorf("ensureAdminRoleFullAccessInOrg: role_id required")
	}
	var role Role
	if err := DB.Where("id = ? AND org_id = ? AND deleted_at IS NULL", roleID, orgID).First(&role).Error; err != nil {
		return fmt.Errorf("ensureAdminRoleFullAccessInOrg: role %d not in org %s: %w", roleID, orgID, err)
	}

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
	if err := ensureRoleMenuPermissionInOrg(orgID, roleID, menuKeys); err != nil {
		return err
	}
	return ensureRoleDataPermissionInOrg(orgID, roleID, "all", "[]")
}

func ensureUserRoleInOrg(orgID, userID string, roleID uint) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("ensureUserRoleInOrg: org_id required")
	}
	orgID = NormalizeOrganizationID(orgID)
	userID = strings.TrimSpace(userID)
	if userID == "" || roleID == 0 {
		return fmt.Errorf("ensureUserRoleInOrg: user_id and role_id required")
	}

	// Fail closed: never bind a role that belongs to a different organization.
	var role Role
	if err := DB.Where("id = ? AND org_id = ? AND deleted_at IS NULL", roleID, orgID).First(&role).Error; err != nil {
		return fmt.Errorf("ensureUserRoleInOrg: role %d not in org %s: %w", roleID, orgID, err)
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
		{Name: "绩效部门/中心评估", Code: "performance:department_eval:submit", Description: "部门/中心负责人确认或调整绩效结果"},
		{Name: "绩效HR审核", Code: "performance:hr_review:submit", Description: "HR审核沐腾科技流程绩效结果"},
		{Name: "绩效结果公布", Code: "performance:result_publish:manage", Description: "公布沐腾科技流程绩效结果"},
		{Name: "绩效结果屏蔽管理", Code: "performance:result_visibility:manage", Description: "设置或解除绩效结果屏蔽"},
		{Name: "绩效屏蔽结果查看", Code: "performance:hidden_result:view", Description: "查看已屏蔽的绩效结果"},
		{Name: "绩效面谈管理", Code: "performance:interview:manage", Description: "安排、记录和完成绩效面谈"},
		{Name: "绩效申诉处理", Code: "performance:appeal:manage", Description: "处理沐腾科技流程绩效申诉"},
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
	grantCompatPermission("permission_manage", "performance:hidden_result:view", permMap)
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
	// 8. 沐腾科技流程拆分节点权限后，保留既有绩效管理员和旧HR角色入口。
	grantCompatPermission("performance:activity:manage", "performance:department_eval:submit", permMap)
	grantCompatPermission("performance:activity:manage", "performance:hr_review:submit", permMap)
	grantCompatPermission("performance:activity:manage", "performance:result_publish:manage", permMap)
	grantCompatPermission("performance:activity:manage", "performance:result_visibility:manage", permMap)
	grantCompatPermission("performance:activity:manage", "performance:interview:manage", permMap)
	grantCompatPermission("performance:activity:manage", "performance:appeal:manage", permMap)
	grantCompatPermission("performance:hr_confirm:submit", "performance:hr_review:submit", permMap)
	grantCompatPermission("performance:hr_confirm:submit", "performance:result_publish:manage", permMap)
	grantCompatPermission("performance:result_publish:manage", "performance:result_visibility:manage", permMap)
	grantCompatPermission("performance:hr_confirm:submit", "performance:appeal:manage", permMap)
	grantCompatPermission("performance:level_adjust:manage", "performance:department_eval:submit", permMap)
	grantCompatPermission("performance:department_eval:submit", "performance:interview:manage", permMap)
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
		role, err := ensureRolePresetInOrg(DefaultOrganizationID, preset.Name, preset.Description)
		if err != nil {
			log.Printf("迁移绩效指标库角色：角色 %s 创建/恢复失败: %v", preset.Name, err)
			continue
		}
		for _, code := range []string{"performance:indicator:manage", "org:read"} {
			if err := ensureRolePermission(role.ID, code, permMap); err != nil {
				log.Printf("迁移绩效指标库角色：角色 %s 补充权限 %s 失败: %v", preset.Name, code, err)
			}
		}
		if err := ensureRoleMenuPermissionInOrg(role.OrgID, role.ID, []string{"menu:home", "menu:performance-indicator-library"}); err != nil {
			log.Printf("迁移绩效指标库角色：角色 %s 补充菜单权限失败: %v", preset.Name, err)
		}
		if err := ensureRoleDataPermissionInOrg(role.OrgID, role.ID, preset.DataScope, preset.DepartmentKeys); err != nil {
			log.Printf("迁移绩效指标库角色：角色 %s 补充数据权限失败: %v", preset.Name, err)
		}
	}
}

func ensureRolePreset(name, description string) (Role, error) {
	// Legacy entry point: only create/find in the default organization.
	// Multi-tenant callers must use ensureRolePresetInOrg.
	return ensureRolePresetInOrg(DefaultOrganizationID, name, description)
}

// ensureRolePresetInOrg finds or creates a role by (org_id, name).
// It never reuses another organization's role of the same name.
func ensureRolePresetInOrg(orgID, name, description string) (Role, error) {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return Role{}, fmt.Errorf("ensureRolePresetInOrg: org_id required")
	}
	orgID = NormalizeOrganizationID(orgID)
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, fmt.Errorf("ensureRolePresetInOrg: name required")
	}
	description = strings.TrimSpace(description)
	if description == "" {
		description = name
	}

	var role Role
	err := DB.Unscoped().Where("org_id = ? AND name = ?", orgID, name).First(&role).Error
	if err == gorm.ErrRecordNotFound {
		role = Role{OrgID: orgID, Name: name, Description: description}
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
	// Hard guarantee: never return a role that drifted to another org.
	if strings.TrimSpace(role.OrgID) != orgID {
		return Role{}, fmt.Errorf("ensureRolePresetInOrg: role %d org mismatch want=%s got=%s", role.ID, orgID, role.OrgID)
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

// ensureRoleMenuPermission is a legacy wrapper that assumes default-org menu rows.
// Prefer ensureRoleMenuPermissionInOrg.
func ensureRoleMenuPermission(roleID uint, menuKeys []string) error {
	return ensureRoleMenuPermissionInOrg(DefaultOrganizationID, roleID, menuKeys)
}

func ensureRoleMenuPermissionInOrg(orgID string, roleID uint, menuKeys []string) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("ensureRoleMenuPermissionInOrg: org_id required")
	}
	orgID = NormalizeOrganizationID(orgID)
	if roleID == 0 {
		return fmt.Errorf("ensureRoleMenuPermissionInOrg: role_id required")
	}

	var existing MenuPermission
	err := DB.Unscoped().
		Where("org_id = ? AND role_id = ?", orgID, roleID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		payload, err := json.Marshal(normalizeMenuPermissionKeys(menuKeys))
		if err != nil {
			return err
		}
		return DB.Create(&MenuPermission{OrgID: orgID, RoleID: roleID, MenuKeys: string(payload)}).Error
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
	if strings.TrimSpace(existing.OrgID) != orgID {
		updates["org_id"] = orgID
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

func stringSliceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ensureRoleDataPermission is a legacy wrapper that assumes default-org data rows.
// Prefer ensureRoleDataPermissionInOrg.
func ensureRoleDataPermission(roleID uint, scope string, departmentKeys string) error {
	return ensureRoleDataPermissionInOrg(DefaultOrganizationID, roleID, scope, departmentKeys)
}

func ensureRoleDataPermissionInOrg(orgID string, roleID uint, scope string, departmentKeys string) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return fmt.Errorf("ensureRoleDataPermissionInOrg: org_id required")
	}
	orgID = NormalizeOrganizationID(orgID)
	if roleID == 0 {
		return fmt.Errorf("ensureRoleDataPermissionInOrg: role_id required")
	}
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
		Where("org_id = ? AND role_id = ?", orgID, roleID).
		Order("deleted_at IS NULL DESC, id ASC").
		First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return DB.Create(&DataPermission{OrgID: orgID, RoleID: roleID, Scope: scope, DepartmentKeys: departmentKeys}).Error
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
	if strings.TrimSpace(existing.OrgID) != orgID {
		updates["org_id"] = orgID
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
	"org:read":                             {"menu:organization-dashboard", "menu:department-tree", "menu:employees"},
	"user_manage":                          {"menu:employee-profile", "menu:employee-flow", "menu:talent-analysis", "menu:sync-log"},
	"attendance_manage":                    {"menu:attendance", "menu:attendance-stats", "menu:attendance-export", "menu:attendance-toolbox", "menu:week-schedule", "menu:employee-shift-config", "menu:sync-jobs", "menu:leave-overtime"},
	"approval_manage":                      {"menu:approval-templates", "menu:approval-instances", "menu:approval-stats"},
	"permission_manage":                    {"menu:permission", "menu:setting"},
	"audit_log:read":                       {"menu:audit-logs"},
	"performance:activity:manage":          {"menu:performance-overview", "menu:performance-reports", "menu:performance-interviews", "menu:performance-appeals"},
	"performance:self_eval:submit":         {"menu:performance-overview", "menu:performance-reports"},
	"performance:manager_eval:submit":      {"menu:performance-overview", "menu:performance-reports"},
	"performance:employee_confirm:submit":  {"menu:performance-overview", "menu:performance-reports"},
	"performance:manager_confirm:submit":   {"menu:performance-overview", "menu:performance-reports"},
	"performance:hr_confirm:submit":        {"menu:performance-overview", "menu:performance-reports"},
	"performance:department_eval:submit":   {"menu:performance-overview", "menu:performance-reports"},
	"performance:hr_review:submit":         {"menu:performance-overview", "menu:performance-reports"},
	"performance:result_publish:manage":    {"menu:performance-overview", "menu:performance-reports", "menu:performance-interviews", "menu:performance-appeals"},
	"performance:result_visibility:manage": {"menu:performance-overview", "menu:performance-reports"},
	"performance:hidden_result:view":       {"menu:performance-overview", "menu:performance-reports"},
	"performance:interview:manage":         {"menu:performance-interviews"},
	"performance:appeal:manage":            {"menu:performance-overview", "menu:performance-reports", "menu:performance-appeals"},
	"performance:goal:manage":              {"menu:performance-overview", "menu:performance-reports"},
	"performance:result:view":              {"menu:performance-overview", "menu:performance-reports", "menu:performance-interviews", "menu:performance-appeals"},
	"performance:indicator:manage":         {"menu:performance-indicator-library"},
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
		if err := DB.Create(&MenuPermission{OrgID: NormalizeOrganizationID(role.OrgID), RoleID: role.ID, MenuKeys: string(payload)}).Error; err != nil {
			log.Printf("迁移菜单权限：写入角色 %d 菜单失败: %v", role.ID, err)
		}
	}
}

func migratePerformanceReportMenuPermissions() {
	var records []MenuPermission
	if err := DB.Where("deleted_at IS NULL").Find(&records).Error; err != nil {
		log.Printf("迁移绩效报表菜单权限：读取菜单权限失败: %v", err)
		return
	}
	for _, record := range records {
		var keys []string
		if err := json.Unmarshal([]byte(strings.TrimSpace(record.MenuKeys)), &keys); err != nil {
			log.Printf("迁移绩效报表菜单权限：解析角色 %d 菜单失败: %v", record.RoleID, err)
			continue
		}
		normalized := normalizeMenuPermissionKeys(keys)
		if !stringSliceContains(normalized, "menu:performance-overview") ||
			stringSliceContains(normalized, "menu:performance-reports") {
			continue
		}
		if err := ensureRoleMenuPermissionInOrg(record.OrgID, record.RoleID, []string{"menu:performance-reports"}); err != nil {
			log.Printf("迁移绩效报表菜单权限：角色 %d 补充菜单失败: %v", record.RoleID, err)
		}
	}
}

func migratePerformanceFollowupMenuPermissions() {
	var records []MenuPermission
	if err := DB.Where("deleted_at IS NULL").Find(&records).Error; err != nil {
		log.Printf("迁移绩效后续模块菜单权限：读取菜单权限失败: %v", err)
		return
	}
	for _, record := range records {
		var keys []string
		if err := json.Unmarshal([]byte(strings.TrimSpace(record.MenuKeys)), &keys); err != nil {
			log.Printf("迁移绩效后续模块菜单权限：解析角色 %d 菜单失败: %v", record.RoleID, err)
			continue
		}
		normalized := normalizeMenuPermissionKeys(keys)
		if !stringSliceContains(normalized, "menu:performance-overview") {
			continue
		}
		if stringSliceContains(normalized, "menu:performance-interviews") &&
			stringSliceContains(normalized, "menu:performance-appeals") {
			continue
		}
		if err := ensureRoleMenuPermissionInOrg(record.OrgID, record.RoleID, []string{"menu:performance-interviews", "menu:performance-appeals"}); err != nil {
			log.Printf("迁移绩效后续模块菜单权限：角色 %d 补充菜单失败: %v", record.RoleID, err)
		}
	}
}

func migrateAttendanceToolboxMenuPermissions() {
	var records []MenuPermission
	if err := DB.Where("deleted_at IS NULL").Find(&records).Error; err != nil {
		log.Printf("迁移考勤工具箱菜单权限：读取菜单权限失败: %v", err)
		return
	}
	for _, record := range records {
		var keys []string
		if err := json.Unmarshal([]byte(strings.TrimSpace(record.MenuKeys)), &keys); err != nil {
			log.Printf("迁移考勤工具箱菜单权限：解析角色 %d 菜单失败: %v", record.RoleID, err)
			continue
		}
		normalized := normalizeMenuPermissionKeys(keys)
		if stringSliceContains(normalized, "menu:attendance-toolbox") {
			continue
		}
		// 为有任何考勤相关菜单的角色添加工具箱权限
		// 包括：考勤、考勤统计、导出、排班、下班时间、年假调休等
		attendanceRelatedMenus := []string{
			"menu:attendance", "menu:attendance-stats", "menu:attendance-export",
			"menu:week-schedule", "menu:employee-shift-config", "menu:leave-overtime",
		}
		hasAttendanceMenu := false
		for _, menu := range attendanceRelatedMenus {
			if stringSliceContains(normalized, menu) {
				hasAttendanceMenu = true
				break
			}
		}
		if !hasAttendanceMenu {
			continue
		}
		if err := ensureRoleMenuPermissionInOrg(record.OrgID, record.RoleID, []string{"menu:attendance-toolbox"}); err != nil {
			log.Printf("迁移考勤工具箱菜单权限：角色 %d 补充菜单失败: %v", record.RoleID, err)
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
