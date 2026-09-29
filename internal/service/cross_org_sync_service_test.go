package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"peopleops/internal/database"
	"peopleops/internal/repository"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openCrossOrgSyncDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:cross-org-sync-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.OrganizationSyncLink{}, &database.OrganizationSyncRun{},
		&database.OrganizationEmployeeMirror{}, &database.OrganizationBusinessMirror{},
		&database.User{}, &database.EmployeeProfile{}, &database.Department{},
		&database.Attendance{}, &database.Approval{}, &database.AnnualLeaveGrant{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestCrossOrganizationSyncMirrorsEmployeesAndBusinessRowsIdempotently(t *testing.T) {
	db := openCrossOrgSyncDB(t)
	now := time.Now().UTC().Truncate(time.Second)
	if err := db.Create(&database.Department{OrgID: "wenyu", DepartmentID: "dept-1", Name: "内容部"}).Error; err != nil {
		t.Fatalf("department: %v", err)
	}
	if err := db.Create(&database.User{OrgID: "wenyu", UserID: "user-1", DingTalkUserID: "ding-1", Name: "员工甲", Email: "a@example.com", DepartmentID: "dept-1", Status: "active", UpdatedAt: now}).Error; err != nil {
		t.Fatalf("user: %v", err)
	}
	if err := db.Create(&database.EmployeeProfile{OrgID: "wenyu", UserID: "user-1", EmployeeID: "EMP-001", EmploymentType: "正式", EntryDate: "2025-01-01", UpdatedAt: now.Add(time.Minute)}).Error; err != nil {
		t.Fatalf("profile: %v", err)
	}
	if err := db.Create(&database.Attendance{OrgID: "wenyu", UserID: "user-1", UserName: "员工甲", CheckTime: now, CheckType: "上班"}).Error; err != nil {
		t.Fatalf("attendance: %v", err)
	}
	if err := db.Create(&database.Approval{OrgID: "wenyu", ProcessID: "process-1", Title: "请假", ApplicantID: "user-1", ApplicantName: "员工甲", Status: "COMPLETED", CreateTime: now}).Error; err != nil {
		t.Fatalf("approval: %v", err)
	}
	if err := db.Create(&database.AnnualLeaveGrant{OrgID: "wenyu", UserID: "user-1", Year: 2026, Quarter: 1, GrantType: "normal"}).Error; err != nil {
		t.Fatalf("annual leave grant: %v", err)
	}

	svc := NewCrossOrganizationSyncService(db)
	if err := svc.CreateOrUpdateLink(context.Background(), "wenyu", "muteng", []string{"attendance", "approval", "annual_leave_grant"}); err != nil {
		t.Fatalf("link: %v", err)
	}
	run, err := svc.Sync(context.Background(), "wenyu", "muteng", "admin")
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if run.Status != "success" || run.EmployeeCount != 1 || run.BusinessCount != 3 || run.FailureCount != 0 {
		t.Fatalf("unexpected run: %#v", run)
	}

	secondRun, err := svc.Sync(context.Background(), "wenyu", "muteng", "admin")
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if secondRun.Status != "success" {
		t.Fatalf("second run status: %#v", secondRun)
	}
	var employeeCount, businessCount int64
	if err := db.Model(&database.OrganizationEmployeeMirror{}).Where("target_org_id = ?", "muteng").Count(&employeeCount).Error; err != nil {
		t.Fatalf("employee count: %v", err)
	}
	if err := db.Model(&database.OrganizationBusinessMirror{}).Where("target_org_id = ?", "muteng").Count(&businessCount).Error; err != nil {
		t.Fatalf("business count: %v", err)
	}
	if employeeCount != 1 || businessCount != 3 {
		t.Fatalf("duplicate mirrors: employees=%d business=%d", employeeCount, businessCount)
	}
	if err := db.Create(&database.User{OrgID: "muteng", UserID: "local-1", Name: "本地员工", Status: "active", UpdatedAt: now}).Error; err != nil {
		t.Fatalf("local user: %v", err)
	}
	allEmployees, total, err := svc.ListPeopleDataCenterEmployees(context.Background(), "muteng", "", "", "", 1, 20)
	if err != nil || total != 2 || len(allEmployees) != 2 {
		t.Fatalf("all center employees: total=%d rows=%#v err=%v", total, allEmployees, err)
	}
	localEmployees, localTotal, err := svc.ListPeopleDataCenterEmployees(context.Background(), "muteng", "", "", "local", 1, 20)
	if err != nil || localTotal != 1 || len(localEmployees) != 1 || localEmployees[0].IsMirror {
		t.Fatalf("local center employees: total=%d rows=%#v err=%v", localTotal, localEmployees, err)
	}
	mirrorEmployees, mirrorTotal, err := svc.ListPeopleDataCenterEmployees(context.Background(), "muteng", "", "", "mirror", 1, 20)
	if err != nil || mirrorTotal != 1 || len(mirrorEmployees) != 1 || !mirrorEmployees[0].IsMirror {
		t.Fatalf("mirror center employees: total=%d rows=%#v err=%v", mirrorTotal, mirrorEmployees, err)
	}

	employees, err := svc.ListEmployeeMirrors(context.Background(), "muteng", 50)
	if err != nil || len(employees) != 1 || employees[0].EmployeeID != "EMP-001" || employees[0].DepartmentName != "内容部" {
		t.Fatalf("employee mirrors: err=%v rows=%#v", err, employees)
	}
	business, err := svc.ListBusinessMirrors(context.Background(), "muteng", "attendance", 50)
	if err != nil || len(business) != 1 || business[0].SourceUserID != "user-1" {
		t.Fatalf("business mirrors: err=%v rows=%#v", err, business)
	}
}

func TestCrossOrganizationSyncRejectsSameOrgAndRunningLink(t *testing.T) {
	db := openCrossOrgSyncDB(t)
	svc := NewCrossOrganizationSyncService(db)
	if err := svc.CreateOrUpdateLink(context.Background(), "wenyu", "wenyu", nil); err == nil {
		t.Fatal("same organization link should fail")
	}
	repo := repository.NewCrossOrganizationSyncRepository(db)
	if _, err := repo.StartRun(context.Background(), "wenyu", "muteng", "request-1", "admin"); err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := repo.StartRun(context.Background(), "wenyu", "muteng", "request-2", "admin"); !errors.Is(err, repository.ErrCrossOrgSyncRunning) {
		t.Fatalf("second running guard err=%v", err)
	}
}

func TestCrossOrganizationSyncTargetIsolation(t *testing.T) {
	db := openCrossOrgSyncDB(t)
	repo := repository.NewCrossOrganizationSyncRepository(db)
	now := time.Now()
	for _, target := range []string{"muteng", "other"} {
		if err := repo.UpsertEmployeeMirror(context.Background(), &database.OrganizationEmployeeMirror{
			TargetOrgID: target, SourceOrgID: "wenyu", SourceUserID: target + "-user", Name: target,
			Status: "active", LastSyncedAt: now,
		}); err != nil {
			t.Fatalf("upsert %s: %v", target, err)
		}
	}
	rows, err := repo.ListEmployeeMirrors(context.Background(), "muteng", 50)
	if err != nil || len(rows) != 1 || rows[0].TargetOrgID != "muteng" {
		t.Fatalf("target isolation: err=%v rows=%#v", err, rows)
	}
}

func TestCrossOrganizationSyncPrepareInboundUsesSavedTargetLink(t *testing.T) {
	db := openCrossOrgSyncDB(t)
	svc := NewCrossOrganizationSyncService(db)
	if err := svc.CreateOrUpdateLink(context.Background(), "wenyu", "muteng", nil); err != nil {
		t.Fatalf("link: %v", err)
	}
	run, err := svc.PrepareInbound(context.Background(), "muteng", "wenyu", "admin")
	if err != nil {
		t.Fatalf("prepare inbound: %v", err)
	}
	if run.SourceOrgID != "wenyu" || run.TargetOrgID != "muteng" || run.Status != "running" {
		t.Fatalf("unexpected inbound run: %#v", run)
	}
	if _, err := svc.PrepareInbound(context.Background(), "muteng", "other", "admin"); !errors.Is(err, ErrCrossOrgSyncSourceRequired) {
		t.Fatalf("unknown source should fail closed, got %v", err)
	}
}

func TestCrossOrganizationSyncDefaultBusinessScopesAreSixCoreTypes(t *testing.T) {
	got := normalizeScopes(nil)
	want := map[string]bool{"attendance": true, "approval": true, "annual_leave_grant": true, "overtime_match": true, "performance_activity": true, "performance_participant": true}
	if len(got) != len(want) {
		t.Fatalf("default scopes=%v", got)
	}
	for _, scope := range got {
		if !want[scope] {
			t.Fatalf("unexpected default scope %q", scope)
		}
	}
}
