//go:build integration && mysql_drill

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/middleware"

	"github.com/gin-gonic/gin"
	gomysql "github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const performanceTenantMySQLDrillDSNEnv = "PEOPLEOPS_MYSQL_DRILL_DSN"

var performanceTenantMySQLDrillModels = []interface{}{
	&database.Organization{},
	&database.User{},
	&database.Department{},
	&database.Role{},
	&database.Permission{},
	&database.RolePermission{},
	&database.UserRole{},
	&database.MenuPermission{},
	&database.DataPermission{},
	&database.UserSession{},
	&database.PerformanceTemplate{},
	&database.PerformanceTemplateSection{},
	&database.PerformanceTemplateItem{},
	&database.PerformanceLevelRule{},
	&database.PerformanceLevelRuleItem{},
	&database.PerformanceActivity{},
	&database.PerformanceDistributionRule{},
	&database.PerformanceDistributionException{},
	&database.PerformanceReminderLog{},
	&database.PerformanceInterviewRecord{},
	&database.PerformanceAppealRecord{},
	&database.PerformanceParticipant{},
	&database.PerformanceReview{},
	&database.PerformanceReviewVersion{},
	&database.PerformanceRelationshipChangeLog{},
	&database.PerformanceGoalRecord{},
	&database.PerformanceGoalApprovalLog{},
	&database.PerformanceImportBatch{},
	&database.PerformanceCompanyFinance{},
	&database.PerformanceIndicatorLibrary{},
	&database.PerformanceIndicatorItem{},
}

type performanceTenantMySQLIdentity struct {
	OrgID     string
	UserID    string
	SessionID string
	Token     string
}

type performanceTenantMySQLResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func TestPerformanceTenantRealMySQLAPIIsolation(t *testing.T) {
	db := openAuthorizedPerformanceTenantMySQLDrill(t)
	resetPerformanceTenantMySQLDrill(t, db)
	t.Cleanup(func() { resetPerformanceTenantMySQLDrill(t, db) })

	if err := db.AutoMigrate(performanceTenantMySQLDrillModels...); err != nil {
		t.Fatalf("migrate production models in approved drill database: %v", err)
	}
	database.RegisterOrganizationCallbacksForTest(db)

	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })

	jwtSecret := fmt.Sprintf("mysql-drill-%d-%d-only-in-memory", time.Now().UnixNano(), os.Getpid())
	t.Setenv("JWT_SECRET", jwtSecret)
	t.Setenv("AUTH_SESSION_VERSION", "mysql-drill-v1")

	permissions := []string{
		"performance:activity:manage",
		"performance:result:view",
		"performance:indicator:manage",
		"performance:goal:manage",
		"performance:distribution:manage",
	}
	orgA := seedPerformanceTenantMySQLIdentity(t, db, "mysql-drill-org-a", permissions)
	orgB := seedPerformanceTenantMySQLIdentity(t, db, "mysql-drill-org-b", permissions)

	gin.SetMode(gin.TestMode)
	router := SetupRouter()

	activityA := createPerformanceTenantMySQLActivity(t, router, orgA, "org-a-isolated-activity")
	activityB := createPerformanceTenantMySQLActivity(t, router, orgB, "org-b-isolated-activity")
	assertPerformanceTenantMySQLRowOrg(t, db, &database.PerformanceActivity{}, activityA, orgA.OrgID)
	assertPerformanceTenantMySQLRowOrg(t, db, &database.PerformanceActivity{}, activityB, orgB.OrgID)

	assertPerformanceTenantMySQLActivityReads(t, router, orgA, activityA, activityB)
	assertPerformanceTenantMySQLActivityReads(t, router, orgB, activityB, activityA)
	assertPerformanceTenantMySQLActivityUpdateIsolation(t, router, db, orgA, activityA, activityB)
	assertPerformanceTenantMySQLBodyAndQuerySpoofsCannotSwitchOrg(t, router, db, orgA, orgB, activityA)

	participantA := seedPerformanceTenantMySQLParticipant(t, db, orgA, activityA, "employee-a", "A")
	participantB := seedPerformanceTenantMySQLParticipant(t, db, orgB, activityB, "employee-b", "B")
	assertPerformanceTenantMySQLStatisticsAndExport(t, router, orgA, activityA, activityB)
	assertPerformanceTenantMySQLGoalUpsertIsolation(t, router, db, orgA, participantA, participantB)
	assertPerformanceTenantMySQLFinanceUpsertIsolation(t, router, db, orgA, activityA, activityB)

	templateA := createPerformanceTenantMySQLTemplate(t, router, orgA, "org-a-template")
	templateB := createPerformanceTenantMySQLTemplate(t, router, orgB, "org-b-template")
	assertPerformanceTenantMySQLTemplateIsolation(t, router, orgA, templateA, templateB)
	assertPerformanceTenantMySQLIndicatorCRUDIsolation(t, router, db, orgA, orgB, templateA, templateB)

	missingOrgToken := signPerformanceTenantMySQLToken(t, performanceTenantMySQLIdentity{
		UserID:    orgA.UserID,
		SessionID: orgA.SessionID,
	})
	missingOrg := performPerformanceTenantMySQLRequest(t, router, missingOrgToken, http.MethodGet, "/api/v1/performance/activities", nil)
	if missingOrg.Code != http.StatusUnauthorized || !strings.Contains(missingOrg.Body.String(), "token_missing_org_id") {
		t.Fatalf("missing org claim must fail closed: status=%d body=%s", missingOrg.Code, missingOrg.Body.String())
	}
}

func openAuthorizedPerformanceTenantMySQLDrill(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(performanceTenantMySQLDrillDSNEnv))
	if dsn == "" {
		t.Skip(performanceTenantMySQLDrillDSNEnv + " is not set")
	}
	cfg, err := gomysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse drill DSN (credentials redacted): %v", err)
	}
	host, port, splitErr := net.SplitHostPort(cfg.Addr)
	if splitErr != nil || cfg.Net != "tcp" || host != "127.0.0.1" || port != "13306" || cfg.DBName != "peopleops_org_drill" {
		t.Fatal("refusing unsafe drill target: require tcp 127.0.0.1:13306 database peopleops_org_drill")
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open approved drill database (credentials redacted): %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get approved drill SQL connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping approved drill database: %v", err)
	}
	return db
}

func resetPerformanceTenantMySQLDrill(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db == nil {
		return
	}
	if err := db.Exec("SET FOREIGN_KEY_CHECKS = 0").Error; err != nil {
		t.Fatalf("disable foreign key checks for drill reset: %v", err)
	}
	defer func() {
		if err := db.Exec("SET FOREIGN_KEY_CHECKS = 1").Error; err != nil {
			t.Fatalf("restore foreign key checks after drill reset: %v", err)
		}
	}()
	for index := len(performanceTenantMySQLDrillModels) - 1; index >= 0; index-- {
		if err := db.Migrator().DropTable(performanceTenantMySQLDrillModels[index]); err != nil {
			t.Fatalf("drop drill table for %T: %v", performanceTenantMySQLDrillModels[index], err)
		}
	}
}

func seedPerformanceTenantMySQLIdentity(t *testing.T, db *gorm.DB, orgID string, permissionCodes []string) performanceTenantMySQLIdentity {
	t.Helper()
	identity := performanceTenantMySQLIdentity{
		OrgID:     orgID,
		UserID:    "admin-" + orgID,
		SessionID: "session-" + orgID,
	}
	org := database.Organization{OrgID: orgID, Name: orgID, CorpID: "corp-" + orgID, Status: "active"}
	user := database.User{OrgID: orgID, UserID: identity.UserID, Name: identity.UserID, DepartmentID: "dept-" + orgID, Status: "active"}
	dept := database.Department{OrgID: orgID, DepartmentID: "dept-" + orgID, Name: "dept-" + orgID, ParentID: "0"}
	role := database.Role{OrgID: orgID, Name: "performance-admin-" + orgID}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("seed organization %s: %v", orgID, err)
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user for %s: %v", orgID, err)
	}
	if err := db.Create(&dept).Error; err != nil {
		t.Fatalf("seed department for %s: %v", orgID, err)
	}
	if err := db.Create(&role).Error; err != nil {
		t.Fatalf("seed role for %s: %v", orgID, err)
	}
	if err := db.Create(&database.UserRole{OrgID: orgID, UserID: identity.UserID, RoleID: role.ID}).Error; err != nil {
		t.Fatalf("seed user role for %s: %v", orgID, err)
	}
	if err := db.Create(&database.DataPermission{OrgID: orgID, RoleID: role.ID, Scope: "all", DepartmentKeys: "[]"}).Error; err != nil {
		t.Fatalf("seed data scope for %s: %v", orgID, err)
	}
	if err := db.Create(&database.MenuPermission{OrgID: orgID, RoleID: role.ID, MenuKeys: `["menu:performance-overview","menu:performance-reports","menu:performance-indicator-library"]`}).Error; err != nil {
		t.Fatalf("seed menu permissions for %s: %v", orgID, err)
	}
	for _, code := range permissionCodes {
		var permission database.Permission
		if err := db.Where("code = ?", code).FirstOrCreate(&permission, database.Permission{Name: code, Code: code}).Error; err != nil {
			t.Fatalf("seed permission %s: %v", code, err)
		}
		if err := db.Create(&database.RolePermission{RoleID: role.ID, PermissionID: permission.ID}).Error; err != nil {
			t.Fatalf("bind permission %s for %s: %v", code, orgID, err)
		}
	}
	if err := db.Create(&database.UserSession{
		OrgID:     orgID,
		UserID:    identity.UserID,
		SessionID: identity.SessionID,
		Token:     "not-a-real-token",
		ExpiresAt: time.Now().Add(time.Hour),
	}).Error; err != nil {
		t.Fatalf("seed session for %s: %v", orgID, err)
	}
	identity.Token = signPerformanceTenantMySQLToken(t, identity)
	return identity
}

func signPerformanceTenantMySQLToken(t *testing.T, identity performanceTenantMySQLIdentity) string {
	t.Helper()
	secret, err := middleware.JWTSecret()
	if err != nil {
		t.Fatalf("load in-memory JWT secret: %v", err)
	}
	claims := middleware.Claims{
		OrgID:          identity.OrgID,
		UserID:         identity.UserID,
		UserName:       identity.UserID,
		SessionID:      identity.SessionID,
		SessionVersion: middleware.SessionVersion(),
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(secret)
	if err != nil {
		t.Fatalf("sign drill token: %v", err)
	}
	return token
}

func createPerformanceTenantMySQLActivity(t *testing.T, router http.Handler, identity performanceTenantMySQLIdentity, name string) uint {
	t.Helper()
	resp := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPost, "/api/v1/performance/activities", map[string]interface{}{
		"name": name, "cycle_type": "quarterly", "start_date": "2026-01-01", "end_date": "2026-03-31", "status": "draft",
	})
	var data struct {
		Activity database.PerformanceActivity `json:"activity"`
	}
	decodePerformanceTenantMySQLData(t, resp, &data)
	if data.Activity.ID == 0 || data.Activity.OrgID != identity.OrgID {
		t.Fatalf("activity create was not tenant-bound: %+v", data.Activity)
	}
	return data.Activity.ID
}

func assertPerformanceTenantMySQLActivityReads(t *testing.T, router http.Handler, identity performanceTenantMySQLIdentity, ownID, foreignID uint) {
	t.Helper()
	list := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodGet, "/api/v1/performance/activities?page=1&page_size=1&keyword=isolated-activity", nil)
	var listData struct {
		Items []database.PerformanceActivity `json:"items"`
		Total int64                          `json:"total"`
	}
	decodePerformanceTenantMySQLData(t, list, &listData)
	if listData.Total != 1 || len(listData.Items) != 1 || listData.Items[0].ID != ownID {
		t.Fatalf("tenant list/search/pagination leaked or omitted rows: total=%d items=%+v", listData.Total, listData.Items)
	}

	detail := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/activities/%d", ownID), nil)
	var detailData struct {
		Activity database.PerformanceActivity `json:"activity"`
	}
	decodePerformanceTenantMySQLData(t, detail, &detailData)
	if detailData.Activity.ID != ownID || detailData.Activity.OrgID != identity.OrgID {
		t.Fatalf("own activity detail mismatch: %+v", detailData.Activity)
	}

	foreign := performPerformanceTenantMySQLRequest(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/activities/%d", foreignID), nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-org activity detail must be hidden: status=%d body=%s", foreign.Code, foreign.Body.String())
	}
}

func assertPerformanceTenantMySQLActivityUpdateIsolation(t *testing.T, router http.Handler, db *gorm.DB, identity performanceTenantMySQLIdentity, ownID, foreignID uint) {
	t.Helper()
	update := map[string]interface{}{
		"name": "org-a-updated", "cycle_type": "quarterly", "start_date": "2026-01-01", "end_date": "2026-03-31",
	}
	requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPut, fmt.Sprintf("/api/v1/performance/activities/%d", ownID), update)
	var own database.PerformanceActivity
	if err := db.Where("id = ?", ownID).First(&own).Error; err != nil || own.Name != "org-a-updated" || own.OrgID != identity.OrgID {
		t.Fatalf("own activity update mismatch: activity=%+v err=%v", own, err)
	}

	var before database.PerformanceActivity
	if err := db.Where("id = ?", foreignID).First(&before).Error; err != nil {
		t.Fatalf("load foreign activity before cross-ID update: %v", err)
	}
	foreign := performPerformanceTenantMySQLRequest(t, router, identity.Token, http.MethodPut, fmt.Sprintf("/api/v1/performance/activities/%d", foreignID), update)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-org activity update must be hidden: status=%d body=%s", foreign.Code, foreign.Body.String())
	}
	var after database.PerformanceActivity
	if err := db.Where("id = ?", foreignID).First(&after).Error; err != nil || after.Name != before.Name || after.OrgID != before.OrgID || !after.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("cross-ID update changed foreign SQL row: before=%+v after=%+v err=%v", before, after, err)
	}
}

func assertPerformanceTenantMySQLBodyAndQuerySpoofsCannotSwitchOrg(t *testing.T, router http.Handler, db *gorm.DB, orgA, orgB performanceTenantMySQLIdentity, ownID uint) {
	t.Helper()
	resp := requirePerformanceTenantMySQLJSON(t, router, orgA.Token, http.MethodPost, "/api/v1/performance/activities?org_id="+orgB.OrgID+"&target_org_id="+orgB.OrgID, map[string]interface{}{
		"name": "spoof-attempt", "cycle_type": "quarterly", "start_date": "2026-04-01", "end_date": "2026-06-30", "status": "draft",
		"org_id": orgB.OrgID, "target_org_id": orgB.OrgID,
	})
	var data struct {
		Activity database.PerformanceActivity `json:"activity"`
	}
	decodePerformanceTenantMySQLData(t, resp, &data)
	if data.Activity.OrgID != orgA.OrgID {
		t.Fatalf("body/query spoof switched tenant: %+v", data.Activity)
	}
	assertPerformanceTenantMySQLRowOrg(t, db, &database.PerformanceActivity{}, data.Activity.ID, orgA.OrgID)

	var foreignCount int64
	if err := db.Model(&database.PerformanceActivity{}).Where("id IN ? AND org_id = ?", []uint{ownID, data.Activity.ID}, orgB.OrgID).Count(&foreignCount).Error; err != nil || foreignCount != 0 {
		t.Fatalf("spoof attempt wrote rows into foreign org: count=%d err=%v", foreignCount, err)
	}
}

func seedPerformanceTenantMySQLParticipant(t *testing.T, db *gorm.DB, identity performanceTenantMySQLIdentity, activityID uint, employeeID, level string) uint {
	t.Helper()
	participant := database.PerformanceParticipant{
		OrgID: identity.OrgID, ActivityID: strconv.FormatUint(uint64(activityID), 10), EmployeeID: employeeID,
		EmployeeName: employeeID, DepartmentID: "dept-" + identity.OrgID, DepartmentName: identity.OrgID,
		EmployeeStatus: "active", Status: "manager_submitted", FinalLevel: level, TotalManagerScore: 88,
	}
	if err := db.Create(&participant).Error; err != nil {
		t.Fatalf("seed participant for %s: %v", identity.OrgID, err)
	}
	return participant.ID
}

func assertPerformanceTenantMySQLStatisticsAndExport(t *testing.T, router http.Handler, identity performanceTenantMySQLIdentity, ownID, foreignID uint) {
	t.Helper()
	summary := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/activities/%d/result-summary", ownID), nil)
	if len(summary.Data) == 0 || string(summary.Data) == "null" {
		t.Fatal("result summary returned no data")
	}
	report := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/activities/%d/report?employee_keyword=employee", ownID), nil)
	var reportData struct {
		Activity database.PerformanceActivity `json:"activity"`
		Progress struct {
			Summary struct {
				TotalParticipants int `json:"total_participants"`
			} `json:"summary"`
		} `json:"progress"`
	}
	decodePerformanceTenantMySQLData(t, report, &reportData)
	if reportData.Activity.ID != ownID || reportData.Progress.Summary.TotalParticipants != 1 {
		t.Fatalf("tenant report statistics mismatch: %+v", reportData)
	}

	export := performPerformanceTenantMySQLRequest(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/activities/%d/report/export?report_type=all", ownID), nil)
	if export.Code != http.StatusOK || !strings.Contains(export.Header().Get("Content-Type"), "spreadsheetml.sheet") || export.Body.Len() == 0 {
		t.Fatalf("tenant export failed: status=%d content-type=%q bytes=%d", export.Code, export.Header().Get("Content-Type"), export.Body.Len())
	}
	foreign := performPerformanceTenantMySQLRequest(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/activities/%d/report/export", foreignID), nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-org export must be hidden: status=%d body=%s", foreign.Code, foreign.Body.String())
	}
}

func assertPerformanceTenantMySQLGoalUpsertIsolation(t *testing.T, router http.Handler, db *gorm.DB, identity performanceTenantMySQLIdentity, ownParticipantID, foreignParticipantID uint) {
	t.Helper()
	var ownParticipant database.PerformanceParticipant
	if err := db.Where("id = ?", ownParticipantID).First(&ownParticipant).Error; err != nil {
		t.Fatalf("load own participant for goal upsert: %v", err)
	}
	if err := db.Model(&database.PerformanceActivity{}).Where("id = ?", ownParticipant.ActivityID).Update("status", "target_setting").Error; err != nil {
		t.Fatalf("prepare own activity status for goal upsert: %v", err)
	}
	if err := db.Model(&database.PerformanceParticipant{}).Where("id = ?", ownParticipantID).Update("status", "pending").Error; err != nil {
		t.Fatalf("prepare own participant status for goal upsert: %v", err)
	}
	var foreignParticipant database.PerformanceParticipant
	if err := db.Where("id = ?", foreignParticipantID).First(&foreignParticipant).Error; err != nil {
		t.Fatalf("load foreign participant for goal upsert: %v", err)
	}
	if err := db.Model(&database.PerformanceActivity{}).Where("id = ?", foreignParticipant.ActivityID).Update("status", "target_setting").Error; err != nil {
		t.Fatalf("prepare foreign activity status for goal upsert: %v", err)
	}
	payload := map[string]interface{}{"records": []map[string]interface{}{{
		"section_type": "quantitative", "item_name": "tenant goal", "weight": 100, "target_value": "first",
	}}}
	first := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPost, fmt.Sprintf("/api/v1/performance/goal-records/%d", ownParticipantID), payload)
	var firstData struct {
		Items []database.PerformanceGoalRecord `json:"items"`
	}
	decodePerformanceTenantMySQLData(t, first, &firstData)
	if len(firstData.Items) != 1 || firstData.Items[0].ID == 0 || firstData.Items[0].OrgID != identity.OrgID {
		t.Fatalf("goal create mismatch: %+v", firstData.Items)
	}

	payload = map[string]interface{}{"records": []map[string]interface{}{{
		"id": firstData.Items[0].ID, "section_type": "quantitative", "item_name": "tenant goal", "weight": 100, "target_value": "updated",
	}}}
	requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPost, fmt.Sprintf("/api/v1/performance/goal-records/%d", ownParticipantID), payload)
	var goals []database.PerformanceGoalRecord
	if err := db.Where("participant_id = ?", ownParticipantID).Find(&goals).Error; err != nil || len(goals) != 1 || goals[0].TargetValue != "updated" || goals[0].OrgID != identity.OrgID {
		t.Fatalf("goal upsert mismatch: goals=%+v err=%v", goals, err)
	}

	var foreignBefore int64
	if err := db.Model(&database.PerformanceGoalRecord{}).Where("participant_id = ?", foreignParticipantID).Count(&foreignBefore).Error; err != nil {
		t.Fatal(err)
	}
	foreign := performPerformanceTenantMySQLRequest(t, router, identity.Token, http.MethodPost, fmt.Sprintf("/api/v1/performance/goal-records/%d", foreignParticipantID), payload)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-org goal upsert must be hidden: status=%d body=%s", foreign.Code, foreign.Body.String())
	}
	var foreignAfter int64
	if err := db.Model(&database.PerformanceGoalRecord{}).Where("participant_id = ?", foreignParticipantID).Count(&foreignAfter).Error; err != nil || foreignAfter != foreignBefore {
		t.Fatalf("cross-ID goal upsert changed foreign rows: before=%d after=%d err=%v", foreignBefore, foreignAfter, err)
	}
}

func assertPerformanceTenantMySQLFinanceUpsertIsolation(t *testing.T, router http.Handler, db *gorm.DB, identity performanceTenantMySQLIdentity, ownActivityID, foreignActivityID uint) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/performance/activities/%d/finance", ownActivityID)
	requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPut, path, map[string]interface{}{"revenue_sign": "equal", "remark": "first"})
	requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPut, path, map[string]interface{}{"revenue_sign": "revenue_gt_expense", "remark": "updated"})
	var finances []database.PerformanceCompanyFinance
	if err := db.Where("activity_id = ?", strconv.FormatUint(uint64(ownActivityID), 10)).Find(&finances).Error; err != nil || len(finances) != 1 || finances[0].Remark != "updated" || finances[0].OrgID != identity.OrgID {
		t.Fatalf("finance create/update mismatch: finances=%+v err=%v", finances, err)
	}

	foreign := performPerformanceTenantMySQLRequest(t, router, identity.Token, http.MethodPut, fmt.Sprintf("/api/v1/performance/activities/%d/finance", foreignActivityID), map[string]interface{}{"revenue_sign": "equal"})
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-org finance write must be hidden: status=%d body=%s", foreign.Code, foreign.Body.String())
	}
	var foreignCount int64
	if err := db.Model(&database.PerformanceCompanyFinance{}).Where("activity_id = ?", strconv.FormatUint(uint64(foreignActivityID), 10)).Count(&foreignCount).Error; err != nil || foreignCount != 0 {
		t.Fatalf("cross-ID finance write changed foreign rows: count=%d err=%v", foreignCount, err)
	}
}

func createPerformanceTenantMySQLTemplate(t *testing.T, router http.Handler, identity performanceTenantMySQLIdentity, name string) uint {
	t.Helper()
	resp := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPost, "/api/v1/performance/templates", map[string]interface{}{
		"name": name, "status": "active", "flow_type": "old",
		"sections": []map[string]interface{}{{
			"name": "score", "section_type": "score", "weight": 100,
			"items": []map[string]interface{}{{"name": "delivery", "max_score": 100, "weight": 100}},
		}},
	})
	var data struct {
		Template database.PerformanceTemplate `json:"template"`
	}
	decodePerformanceTenantMySQLData(t, resp, &data)
	if data.Template.ID == 0 || data.Template.OrgID != identity.OrgID {
		t.Fatalf("template create was not tenant-bound: %+v", data.Template)
	}
	return data.Template.ID
}

func assertPerformanceTenantMySQLTemplateIsolation(t *testing.T, router http.Handler, identity performanceTenantMySQLIdentity, ownID, foreignID uint) {
	t.Helper()
	// System templates may be seeded per organization; isolation requires every visible
	// row belongs to the auth org and foreign IDs stay hidden — not a hard total==1.
	list := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodGet, "/api/v1/performance/templates?page=1&page_size=50&status=active", nil)
	var listData struct {
		Items []database.PerformanceTemplate `json:"items"`
		Total int64                          `json:"total"`
	}
	decodePerformanceTenantMySQLData(t, list, &listData)
	if listData.Total < 1 || len(listData.Items) == 0 {
		t.Fatalf("template list empty for tenant: total=%d items=%+v", listData.Total, listData.Items)
	}
	foundOwn := false
	for _, item := range listData.Items {
		if item.OrgID != identity.OrgID {
			t.Fatalf("template list leaked foreign org row: item=%+v wantOrg=%s", item, identity.OrgID)
		}
		if item.ID == ownID {
			foundOwn = true
		}
		if item.ID == foreignID {
			t.Fatalf("template list included foreign template id=%d", foreignID)
		}
	}
	if !foundOwn {
		t.Fatalf("own template id=%d missing from tenant list: %+v", ownID, listData.Items)
	}
	own := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/templates/%d", ownID), nil)
	var ownData struct {
		Template database.PerformanceTemplate `json:"template"`
	}
	decodePerformanceTenantMySQLData(t, own, &ownData)
	if ownData.Template.ID != ownID || ownData.Template.OrgID != identity.OrgID {
		t.Fatalf("own template detail mismatch: %+v", ownData.Template)
	}
	foreign := performPerformanceTenantMySQLRequest(t, router, identity.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/templates/%d", foreignID), nil)
	if foreign.Code != http.StatusNotFound {
		t.Fatalf("cross-org template detail must be hidden: status=%d body=%s", foreign.Code, foreign.Body.String())
	}
}

func assertPerformanceTenantMySQLIndicatorCRUDIsolation(t *testing.T, router http.Handler, db *gorm.DB, orgA, orgB performanceTenantMySQLIdentity, templateA, templateB uint) {
	t.Helper()
	libraryA, itemA := createPerformanceTenantMySQLLibrary(t, router, orgA, templateA, "org-a-library", "org-a-searchable-indicator")
	libraryB, itemB := createPerformanceTenantMySQLLibrary(t, router, orgB, templateB, "org-b-library", "org-b-searchable-indicator")
	assertPerformanceTenantMySQLRowOrg(t, db, &database.PerformanceIndicatorLibrary{}, libraryA, orgA.OrgID)
	assertPerformanceTenantMySQLRowOrg(t, db, &database.PerformanceIndicatorItem{}, itemA, orgA.OrgID)

	list := requirePerformanceTenantMySQLJSON(t, router, orgA.Token, http.MethodGet, "/api/v1/performance/indicator-libraries?page=1&page_size=1&keyword=library", nil)
	var listData struct {
		Items []database.PerformanceIndicatorLibrary `json:"items"`
		Total int64                                  `json:"total"`
	}
	decodePerformanceTenantMySQLData(t, list, &listData)
	if listData.Total != 1 || len(listData.Items) != 1 || listData.Items[0].ID != libraryA {
		t.Fatalf("indicator library search/pagination leaked rows: total=%d items=%+v", listData.Total, listData.Items)
	}

	search := requirePerformanceTenantMySQLJSON(t, router, orgA.Token, http.MethodGet, "/api/v1/performance/indicator-items/search?keyword=searchable-indicator", nil)
	var searchData struct {
		Items []database.PerformanceIndicatorItem `json:"items"`
	}
	decodePerformanceTenantMySQLData(t, search, &searchData)
	if len(searchData.Items) != 1 || searchData.Items[0].ID != itemA {
		t.Fatalf("indicator item search leaked rows: %+v", searchData.Items)
	}

	requirePerformanceTenantMySQLJSON(t, router, orgA.Token, http.MethodPut, fmt.Sprintf("/api/v1/performance/indicator-items/%d", itemA), map[string]interface{}{"name": "org-a-updated-indicator", "weight": 50})
	var updated database.PerformanceIndicatorItem
	if err := db.Where("id = ?", itemA).First(&updated).Error; err != nil || updated.Name != "org-a-updated-indicator" || updated.OrgID != orgA.OrgID {
		t.Fatalf("indicator update mismatch: item=%+v err=%v", updated, err)
	}

	foreignUpdate := performPerformanceTenantMySQLRequest(t, router, orgA.Token, http.MethodPut, fmt.Sprintf("/api/v1/performance/indicator-items/%d", itemB), map[string]interface{}{"name": "stolen"})
	if foreignUpdate.Code != http.StatusNotFound {
		t.Fatalf("cross-org indicator update must be hidden: status=%d body=%s", foreignUpdate.Code, foreignUpdate.Body.String())
	}
	var foreignBefore database.PerformanceIndicatorItem
	if err := db.Where("id = ?", itemB).First(&foreignBefore).Error; err != nil || foreignBefore.Name != "org-b-searchable-indicator" {
		t.Fatalf("cross-ID indicator update changed SQL row: item=%+v err=%v", foreignBefore, err)
	}

	foreignDelete := performPerformanceTenantMySQLRequest(t, router, orgA.Token, http.MethodDelete, fmt.Sprintf("/api/v1/performance/indicator-items/%d", itemB), nil)
	if foreignDelete.Code != http.StatusNotFound {
		t.Fatalf("cross-org indicator delete must be hidden: status=%d body=%s", foreignDelete.Code, foreignDelete.Body.String())
	}
	var foreignCount int64
	if err := db.Unscoped().Model(&database.PerformanceIndicatorItem{}).Where("id = ? AND org_id = ? AND deleted_at IS NULL", itemB, orgB.OrgID).Count(&foreignCount).Error; err != nil || foreignCount != 1 {
		t.Fatalf("cross-ID delete changed foreign SQL row: count=%d err=%v", foreignCount, err)
	}

	requirePerformanceTenantMySQLJSON(t, router, orgA.Token, http.MethodDelete, fmt.Sprintf("/api/v1/performance/indicator-items/%d", itemA), nil)
	var deleted database.PerformanceIndicatorItem
	if err := db.Unscoped().Where("id = ?", itemA).First(&deleted).Error; err != nil || deleted.DeletedAt == nil || deleted.OrgID != orgA.OrgID {
		t.Fatalf("own indicator delete mismatch: item=%+v err=%v", deleted, err)
	}

	foreignLibrary := performPerformanceTenantMySQLRequest(t, router, orgA.Token, http.MethodGet, fmt.Sprintf("/api/v1/performance/indicator-libraries/%d", libraryB), nil)
	if foreignLibrary.Code != http.StatusNotFound {
		t.Fatalf("cross-org indicator library detail must be hidden: status=%d body=%s", foreignLibrary.Code, foreignLibrary.Body.String())
	}
}

func createPerformanceTenantMySQLLibrary(t *testing.T, router http.Handler, identity performanceTenantMySQLIdentity, templateID uint, name, itemName string) (uint, uint) {
	t.Helper()
	resp := requirePerformanceTenantMySQLJSON(t, router, identity.Token, http.MethodPost, "/api/v1/performance/indicator-libraries", map[string]interface{}{
		"department_id": "dept-" + identity.OrgID, "department_name": identity.OrgID, "template_id": templateID, "name": name,
		"items": []map[string]interface{}{{"section_type": "quantitative", "name": itemName, "weight": 100}},
	})
	var data struct {
		Library database.PerformanceIndicatorLibrary `json:"library"`
		Items   []database.PerformanceIndicatorItem  `json:"items"`
	}
	decodePerformanceTenantMySQLData(t, resp, &data)
	if data.Library.ID == 0 || data.Library.OrgID != identity.OrgID || len(data.Items) != 1 || data.Items[0].OrgID != identity.OrgID {
		t.Fatalf("indicator library create was not tenant-bound: library=%+v items=%+v", data.Library, data.Items)
	}
	return data.Library.ID, data.Items[0].ID
}

func performPerformanceTenantMySQLRequest(t *testing.T, router http.Handler, token, method, path string, body interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		encoded, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("encode request body: %v", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req := httptest.NewRequest(method, path, reader)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func requirePerformanceTenantMySQLJSON(t *testing.T, router http.Handler, token, method, path string, body interface{}) performanceTenantMySQLResponse {
	t.Helper()
	rec := performPerformanceTenantMySQLRequest(t, router, token, method, path, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("%s %s: status=%d body=%s", method, path, rec.Code, rec.Body.String())
	}
	var response performanceTenantMySQLResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode %s %s response: %v", method, path, err)
	}
	if response.Code != http.StatusOK {
		t.Fatalf("%s %s: business code=%d message=%q", method, path, response.Code, response.Message)
	}
	return response
}

func decodePerformanceTenantMySQLData(t *testing.T, response performanceTenantMySQLResponse, destination interface{}) {
	t.Helper()
	if err := json.Unmarshal(response.Data, destination); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
}

func assertPerformanceTenantMySQLRowOrg(t *testing.T, db *gorm.DB, model interface{}, id uint, wantOrgID string) {
	t.Helper()
	var orgID string
	if err := db.Model(model).Select("org_id").Where("id = ?", id).Scan(&orgID).Error; err != nil {
		t.Fatalf("read org_id for %T id=%d: %v", model, id, err)
	}
	if orgID != wantOrgID {
		t.Fatalf("unexpected org_id for %T id=%d: got=%q want=%q", model, id, orgID, wantOrgID)
	}
}
