package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/requestmeta"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestPerformanceNoticeOrgIDFailClosed(t *testing.T) {
	if _, err := performanceNoticeOrgID("", "  ", ""); !errors.Is(err, ErrPerformanceNoticeMissingOrg) {
		t.Fatalf("empty candidates err = %v, want missing org", err)
	}
	got, err := performanceNoticeOrgID("", " org-b ", "org-a")
	if err != nil || got != "org-b" {
		t.Fatalf("performanceNoticeOrgID() = %q, %v", got, err)
	}
}

func TestSendPerformanceActionCardRequiresOrgID(t *testing.T) {
	original := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = original })

	called := false
	sendPerformanceActionCardToUser = func(orgID, userID, title, content, actionTitle, actionURL string) error {
		called = true
		return nil
	}

	if err := sendPerformanceActionCard("", "user-1", "t", "c", "a", "http://x"); !errors.Is(err, ErrPerformanceNoticeMissingOrg) {
		t.Fatalf("empty org err = %v, want missing org", err)
	}
	if called {
		t.Fatal("sender must not be called without orgID")
	}

	if err := sendPerformanceActionCard("org-a", "user-1", "t", "c", "a", "http://x"); err != nil {
		t.Fatalf("send with org err = %v", err)
	}
	if !called {
		t.Fatal("sender should be called with orgID")
	}
}

func TestPerformanceNoticeURLsAreOrgAware(t *testing.T) {
	// BuildAppURLForOrg for non-default without configured org home must fail closed.
	if got := PerformanceSelfEvalURL("org-b", "act-1", 9); got != "" {
		t.Fatalf("non-default without org home should be empty, got %q", got)
	}
	if got := PerformanceOverviewURL("", "act-1"); got != "" {
		t.Fatalf("empty org should fail closed, got %q", got)
	}

	// Default org may use process env fallbacks.
	t.Setenv("DINGTALK_APP_HOME_URL", "https://default.example/app")
	want := "https://default.example/app/performance-self-eval/act-1/9"
	if got := PerformanceSelfEvalURL(database.DefaultOrganizationID, "act-1", 9); got != want {
		t.Fatalf("default org URL = %q, want %q", got, want)
	}
}

func TestDualOrgActionCardUsesRespectiveOrgID(t *testing.T) {
	original := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = original })

	type call struct {
		orgID, userID, actionURL string
	}
	var calls []call
	sendPerformanceActionCardToUser = func(orgID, userID, title, content, actionTitle, actionURL string) error {
		calls = append(calls, call{orgID: orgID, userID: userID, actionURL: actionURL})
		return nil
	}

	// Simulate same ding-user_id across two tenants.
	userID := "same-user"
	for _, orgID := range []string{"org-a", "org-b"} {
		if err := sendPerformanceActionCard(orgID, userID, "绩效自评提醒", "content", "去完成自评", "https://"+orgID+".example/self"); err != nil {
			t.Fatalf("send org %s: %v", orgID, err)
		}
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want 2", calls)
	}
	if calls[0].orgID != "org-a" || calls[1].orgID != "org-b" {
		t.Fatalf("org IDs = %q/%q, want org-a/org-b", calls[0].orgID, calls[1].orgID)
	}
	if calls[0].userID != userID || calls[1].userID != userID {
		t.Fatalf("user IDs = %#v", calls)
	}
	if strings.Contains(calls[1].actionURL, "org-a") {
		t.Fatalf("org-b URL leaked org-a: %q", calls[1].actionURL)
	}
}

func TestNotifyInterviewAndAppealCarryOrgID(t *testing.T) {
	t.Setenv("DINGTALK_APP_HOME_URL", "https://peopleops.example/app")
	original := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = original })

	var orgs []string
	var lastURL string
	sendPerformanceActionCardToUser = func(orgID, userID, title, content, actionTitle, actionURL string) error {
		orgs = append(orgs, orgID)
		lastURL = actionURL
		return nil
	}

	svc := &PerformanceFollowupService{}
	svc.notifyInterviewChanged(&database.PerformanceInterviewRecord{
		OrgID:         "org-a",
		ActivityID:    "activity-1",
		ParticipantID: 1,
		EmployeeID:    "employee-1",
		Status:        PerformanceInterviewStatusPending,
	})
	if len(orgs) == 0 || orgs[0] != "org-a" {
		t.Fatalf("interview org = %#v, want org-a", orgs)
	}

	orgs = nil
	svc.notifyAppealStatusChanged(&database.PerformanceAppealRecord{
		OrgID:         "org-b",
		ActivityID:    "activity-2",
		ParticipantID: 2,
		EmployeeID:    "employee-2",
		Status:        PerformanceAppealStatusResolved,
	})
	if len(orgs) != 1 || orgs[0] != "org-b" {
		t.Fatalf("appeal org = %#v, want org-b", orgs)
	}
	// Non-default without configured AppHomeURL must not invent the global/default home.
	if strings.Contains(lastURL, "peopleops.example") {
		t.Fatalf("org-b notice URL reused default home: %q", lastURL)
	}

	// Missing org fails closed (no send).
	orgs = nil
	svc.notifyAppealStatusChanged(&database.PerformanceAppealRecord{
		ActivityID:    "activity-3",
		ParticipantID: 3,
		EmployeeID:    "employee-3",
		Status:        PerformanceAppealStatusResolved,
	})
	if len(orgs) != 0 {
		t.Fatalf("missing org still sent: %#v", orgs)
	}
}

func TestBuildAppURLForOrgFailClosed(t *testing.T) {
	if got := dingtalk.BuildAppURLForOrg("", "/x"); got != "" {
		t.Fatalf("empty org = %q", got)
	}
	if got := dingtalk.BuildAppURLForOrg("non-default-org", "/x"); got != "" {
		t.Fatalf("unconfigured non-default must not fall back to global, got %q", got)
	}
	t.Setenv("DINGTALK_APP_HOME_URL", "https://default.example")
	want := "https://default.example/x"
	if got := dingtalk.BuildAppURLForOrg(database.DefaultOrganizationID, "/x"); got != want {
		t.Fatalf("default org = %q, want %q", got, want)
	}
}

func TestSendPerformanceActionCardPropagatesNotifiableAndPartialFailures(t *testing.T) {
	original := sendPerformanceActionCardToUser
	t.Cleanup(func() { sendPerformanceActionCardToUser = original })

	sendPerformanceActionCardToUser = func(orgID, userID, title, content, actionTitle, actionURL string) error {
		if userID == "admin" {
			return dingtalk.ErrUserNotNotifiable
		}
		if userID == "bad" {
			return errors.New("dingtalk failed")
		}
		return nil
	}

	if err := sendPerformanceActionCard("org-a", "ok", "t", "c", "a", "u"); err != nil {
		t.Fatalf("success path: %v", err)
	}
	if err := sendPerformanceActionCard("org-a", "admin", "t", "c", "a", "u"); !dingtalk.IsUserNotNotifiableError(err) {
		t.Fatalf("notifiable err = %v", err)
	}
	if err := sendPerformanceActionCard("org-a", "bad", "t", "c", "a", "u"); err == nil || !strings.Contains(err.Error(), "dingtalk failed") {
		t.Fatalf("failure err = %v", err)
	}
}

func openPerformanceNoticeSQLite(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:perf-notice-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.Role{},
		&database.UserRole{},
		&database.Permission{},
		&database.RolePermission{},
		&database.DataPermission{},
		&database.Department{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func withOrgContextDB(db *gorm.DB, orgID string) *gorm.DB {
	info := &requestmeta.RequestInfo{OrgID: orgID}
	ctx := requestmeta.WithRequestInfo(context.Background(), info)
	ctx = requestmeta.WithTenant(ctx, orgID)
	return db.WithContext(ctx)
}

func TestFindPerformanceAppealManageRecipientsOrgIsolation(t *testing.T) {
	db := openPerformanceNoticeSQLite(t)
	// Shared permission catalog (global).
	permAppeal := database.Permission{Name: "appeal manage", Code: "performance:appeal:manage"}
	if err := db.Create(&permAppeal).Error; err != nil {
		t.Fatalf("seed perm appeal: %v", err)
	}

	// Two tenants, same human user_id string.
	for _, org := range []string{"org-a", "org-b"} {
		u := database.User{
			OrgID:          org,
			UserID:         "same-hr",
			DingTalkUserID: "dt-" + org + "-same-hr",
			Name:           "HR " + org,
			Status:         "active",
			Email:          "same-hr@" + org + ".test",
			Mobile:         "m-" + org + "-same-hr",
			DepartmentID:   "dept-" + org,
		}
		if err := db.Create(&u).Error; err != nil {
			t.Fatalf("seed user %s: %v", org, err)
		}
	}

	// Only org-a role has appeal manage; attach role to same-hr in org-a.
	roleA := database.Role{OrgID: "org-a", Name: "hr-a", Description: "org-a hr"}
	if err := db.Create(&roleA).Error; err != nil {
		t.Fatalf("seed role a: %v", err)
	}
	if err := db.Create(&database.RolePermission{RoleID: roleA.ID, PermissionID: permAppeal.ID}).Error; err != nil {
		t.Fatalf("seed role_perm a: %v", err)
	}
	if err := db.Create(&database.UserRole{OrgID: "org-a", UserID: "same-hr", RoleID: roleA.ID}).Error; err != nil {
		t.Fatalf("seed user_role a: %v", err)
	}
	// Data permission all so scope allows any department.
	if err := db.Create(&database.DataPermission{OrgID: "org-a", RoleID: roleA.ID, Scope: "all"}).Error; err != nil {
		t.Fatalf("seed data perm a: %v", err)
	}

	// org-b has a role WITHOUT appeal permission, and a poisoned user_roles row that
	// points at org-a's role_id with org-b org_id (must not leak via join).
	roleB := database.Role{OrgID: "org-b", Name: "hr-b", Description: "org-b hr no appeal"}
	if err := db.Create(&roleB).Error; err != nil {
		t.Fatalf("seed role b: %v", err)
	}
	// Poisoned assignment: user_roles.org_id=org-b but role_id belongs to org-a.
	if err := db.Create(&database.UserRole{OrgID: "org-b", UserID: "same-hr", RoleID: roleA.ID}).Error; err != nil {
		t.Fatalf("seed poisoned user_role: %v", err)
	}

	// Missing org fails closed (no tenant context needed).
	svcMissing := &PerformanceFollowupService{db: db}
	if _, err := svcMissing.findPerformanceAppealManageRecipients(database.PerformanceAppealRecord{
		EmployeeID:   "employee-1",
		DepartmentID: "dept-x",
	}); !errors.Is(err, ErrPerformanceNoticeMissingOrg) {
		t.Fatalf("missing org err = %v", err)
	}

	// Attach tenant context so DataPermissionRepository.FindByRoleID reads the same org
	// (mirrors request-scoped DB used by handlers).
	svcA := &PerformanceFollowupService{db: withOrgContextDB(db, "org-a")}
	gotA, err := svcA.findPerformanceAppealManageRecipients(database.PerformanceAppealRecord{
		OrgID:        "org-a",
		EmployeeID:   "employee-1",
		DepartmentID: "dept-a",
	})
	if err != nil {
		t.Fatalf("org-a recipients: %v", err)
	}
	if len(gotA) != 1 || gotA[0].UserID != "same-hr" {
		t.Fatalf("org-a recipients = %#v, want same-hr", gotA)
	}

	svcB := &PerformanceFollowupService{db: withOrgContextDB(db, "org-b")}
	gotB, err := svcB.findPerformanceAppealManageRecipients(database.PerformanceAppealRecord{
		OrgID:        "org-b",
		EmployeeID:   "employee-2",
		DepartmentID: "dept-b",
	})
	if err != nil {
		t.Fatalf("org-b recipients: %v", err)
	}
	if len(gotB) != 0 {
		t.Fatalf("org-b must not borrow org-a user_roles/permissions, got %#v", gotB)
	}
}
