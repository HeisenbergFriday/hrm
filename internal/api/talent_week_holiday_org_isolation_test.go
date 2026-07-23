package api

import (
	"net/http"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/repository"
	"peopleops/internal/requestmeta"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openTalentWeekHolidayIsolationDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:talent-week-holiday-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.TalentAnalysis{},
		&database.WeekScheduleRule{},
		&database.WeekScheduleOverride{},
		&database.StatutoryHoliday{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.RegisterOrganizationCallbacksForTest(db)
	return db
}

func withRequestOrgDB(c *gin.Context, db *gorm.DB, orgID string) {
	ctx := requestmeta.WithRequestInfo(c.Request.Context(), &requestmeta.RequestInfo{OrgID: orgID})
	ctx = requestmeta.WithTenant(ctx, orgID)
	c.Request = c.Request.WithContext(ctx)
	c.Set("requestDB", db.Session(&gorm.Session{NewDB: true}).WithContext(ctx))
}

func TestCreateTalentAnalysis_RejectsCrossOrgBody(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{"user_id":"same-user","user_name":"同人","department_id":"d1","department_name":"D","position":"P","analysis_date":"2026-07-01","org_id":"org-b"}`
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/talent-analyses", body, "org-a")
	withRequestOrgDB(c, db, "org-a")

	CreateTalentAnalysis(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&database.TalentAnalysis{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0 (cross-org body must not write)", count)
	}
}

func TestCreateTalentAnalysis_ForcesCurrentOrgAndAllowsTwinUserIDs(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)

	// org-a create without org_id in body
	bodyA := `{"user_id":"same-user","user_name":"A","department_id":"d1","department_name":"D","position":"P","analysis_date":"2026-07-01"}`
	cA, recA := newSecurityCtx(t, http.MethodPost, "/api/v1/talent-analyses", bodyA, "org-a")
	withRequestOrgDB(cA, db, "org-a")
	CreateTalentAnalysis(cA)
	if recA.Code != http.StatusOK {
		t.Fatalf("org-a status = %d body=%s", recA.Code, recA.Body.String())
	}

	// org-b same user_id allowed
	bodyB := `{"user_id":"same-user","user_name":"B","department_id":"d2","department_name":"D2","position":"P","analysis_date":"2026-07-01"}`
	cB, recB := newSecurityCtx(t, http.MethodPost, "/api/v1/talent-analyses", bodyB, "org-b")
	withRequestOrgDB(cB, db, "org-b")
	CreateTalentAnalysis(cB)
	if recB.Code != http.StatusOK {
		t.Fatalf("org-b status = %d body=%s", recB.Code, recB.Body.String())
	}

	var rows []database.TalentAnalysis
	if err := db.Order("org_id asc").Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(rows))
	}
	if rows[0].OrgID != "org-a" || rows[1].OrgID != "org-b" {
		t.Fatalf("orgs = %q/%q, want org-a/org-b", rows[0].OrgID, rows[1].OrgID)
	}
	if rows[0].UserID != "same-user" || rows[1].UserID != "same-user" {
		t.Fatalf("user ids not preserved: %#v", rows)
	}
}

func TestCreateTalentAnalysis_MissingOrgContextFailClosed(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{"user_id":"u1","user_name":"U","department_id":"d1","department_name":"D","position":"P","analysis_date":"2026-07-01"}`
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/talent-analyses", body, "")
	withRequestOrgDB(c, db, "")
	CreateTalentAnalysis(c)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateWeekScheduleRule_RejectsCrossOrgBody(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{"scope_type":"user","scope_id":"same-user","scope_name":"U","base_date":"2026-01-05","pattern":"big_first","org_id":"org-b"}`
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/week-schedule/rules", body, "org-a")
	withRequestOrgDB(c, db, "org-a")
	CreateWeekScheduleRule(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&database.WeekScheduleRule{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count = %d, want 0", count)
	}
}

func TestCreateWeekScheduleRule_AllowsSameScopeAcrossOrgs(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{"scope_type":"user","scope_id":"same-user","scope_name":"U","base_date":"2026-01-05","pattern":"big_first"}`
	for _, org := range []string{"org-a", "org-b"} {
		c, rec := newSecurityCtx(t, http.MethodPost, "/api/v1/week-schedule/rules", body, org)
		withRequestOrgDB(c, db, org)
		CreateWeekScheduleRule(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", org, rec.Code, rec.Body.String())
		}
	}
	var rows []database.WeekScheduleRule
	if err := db.Order("org_id asc").Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	if rows[0].OrgID != "org-a" || rows[1].OrgID != "org-b" {
		t.Fatalf("orgs=%q/%q", rows[0].OrgID, rows[1].OrgID)
	}
}

func TestSetWeekOverride_RejectsCrossOrgBody(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{"scope_type":"user","scope_id":"same-user","week_start_date":"2026-01-05","week_type":"big","org_id":"org-b"}`
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/week-schedule/overrides", body, "org-a")
	withRequestOrgDB(c, db, "org-a")
	SetWeekOverride(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestCreateHoliday_RejectsCrossOrgBody(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{"date":"2026-10-01","name":"国庆","type":"holiday","year":2026,"org_id":"org-b"}`
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/week-schedule/holidays", body, "org-a")
	withRequestOrgDB(c, db, "org-a")
	CreateHoliday(c)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	var count int64
	if err := db.Model(&database.StatutoryHoliday{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("count=%d want 0", count)
	}
}

func TestBatchCreateHolidays_RejectsAnyCrossOrgItem(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{
		"holidays":[
			{"date":"2026-10-01","name":"国庆","type":"holiday","year":2026},
			{"date":"2026-10-02","name":"国庆","type":"holiday","year":2026,"org_id":"org-b"}
		]
	}`
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/week-schedule/holidays/batch", body, "org-a")
	withRequestOrgDB(c, db, "org-a")
	BatchCreateHolidays(c)
	if recorder.Code == http.StatusOK {
		t.Fatalf("status = 200, want reject; body=%s", recorder.Body.String())
	}
	var count int64
	if err := db.Model(&database.StatutoryHoliday{}).Count(&count).Error; err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("batch partial write count=%d, want 0 (transactional fail-closed)", count)
	}
}

func TestBatchCreateHolidays_AllowsSameDateAcrossOrgs(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	body := `{"holidays":[{"date":"2026-10-01","name":"国庆","type":"holiday","year":2026}]}`
	for _, org := range []string{"org-a", "org-b"} {
		c, rec := newSecurityCtx(t, http.MethodPost, "/api/v1/week-schedule/holidays/batch", body, org)
		withRequestOrgDB(c, db, org)
		BatchCreateHolidays(c)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status=%d body=%s", org, rec.Code, rec.Body.String())
		}
	}
	var rows []database.StatutoryHoliday
	if err := db.Order("org_id asc").Find(&rows).Error; err != nil {
		t.Fatalf("find: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows=%d want 2", len(rows))
	}
	if rows[0].OrgID != "org-a" || rows[1].OrgID != "org-b" {
		t.Fatalf("orgs=%q/%q", rows[0].OrgID, rows[1].OrgID)
	}
}

func TestDeleteWeekOverride_CannotDeleteOtherOrg(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	row := database.WeekScheduleOverride{
		OrgID:         "org-b",
		ScopeType:     "user",
		ScopeID:       "same-user",
		WeekStartDate: "2026-01-05",
		WeekType:      "big",
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}

	c, rec := newSecurityCtx(t, http.MethodDelete, "/api/v1/week-schedule/overrides/1", "", "org-a")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	withRequestOrgDB(c, db, "org-a")
	DeleteWeekOverride(c)

	// 删除应失败或表现为不存在；不得删除 org-b 数据
	var still database.WeekScheduleOverride
	if err := db.First(&still, row.ID).Error; err != nil {
		t.Fatalf("org-b override should still exist: %v; status=%d body=%s", err, rec.Code, rec.Body.String())
	}
}

func TestTalentRepository_EmptyOrgConstructorRejectsCreate(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	repo := repository.NewTalentRepository(db)
	err := repo.Create(&database.TalentAnalysis{
		UserID: "u1", UserName: "U", DepartmentID: "d", DepartmentName: "D",
		Position: "P", AnalysisDate: "2026-07-01", OrgID: "org-a",
	})
	if err == nil {
		t.Fatal("expected empty-org create to fail")
	}
	if !strings.Contains(err.Error(), "org") && err != repository.ErrMissingOrgID {
		// either explicit ErrMissingOrgID or message mentioning org is acceptable
		t.Fatalf("err=%v", err)
	}
}

func TestWeekScheduleRepository_EmptyOrgConstructorRejectsCreate(t *testing.T) {
	db := openTalentWeekHolidayIsolationDB(t)
	repo := repository.NewWeekScheduleRepository(db)
	err := repo.CreateRule(&database.WeekScheduleRule{
		ScopeType: "user", ScopeID: "u1", BaseDate: "2026-01-05", Pattern: "big_first", OrgID: "org-a",
	})
	if err == nil {
		t.Fatal("expected empty-org CreateRule to fail")
	}
}
