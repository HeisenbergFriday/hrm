package database

import (
	"errors"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestSwitchableOrganizationsRequireConfiguredUserAndDingTalkMembership(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:org-switch-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&Organization{}, &OrganizationUser{}, &User{}, &DingTalkBinding{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	withTempDB(t, db)

	orgs := []Organization{
		{OrgID: "org-a", Name: "沐腾", CorpID: "corp-a", Status: "active"},
		{OrgID: "org-b", Name: "文娱", CorpID: "corp-b", Status: "active"},
		{OrgID: "org-c", Name: "仅配置", CorpID: "corp-c", Status: "active"},
		{OrgID: "org-d", Name: "仅钉钉成员", CorpID: "corp-d", Status: "active"},
	}
	if err := db.Create(&orgs).Error; err != nil {
		t.Fatalf("create orgs: %v", err)
	}
	current := User{OrgID: "org-a", UserID: "org-a:u1", DingTalkUserID: "dt-u1", Name: "测试用户", Status: "active"}
	target := User{OrgID: "org-b", UserID: "org-b:u1", DingTalkUserID: "dt-u1-other-enterprise", Name: "测试用户", Status: "active"}
	configuredOnly := User{OrgID: "org-c", UserID: "org-c:u1", DingTalkUserID: "dt-u1-configured-only", Name: "测试用户", Status: "active"}
	if err := db.Create(&[]User{current, target, configuredOnly}).Error; err != nil {
		t.Fatalf("create users: %v", err)
	}
	if err := db.Create(&[]OrganizationUser{
		{OrgID: "org-a", UserID: "dt-u1", Status: "active"},
		{OrgID: "org-b", UserID: "dt-u1-other-enterprise", Status: "active"},
		{OrgID: "org-d", UserID: "dt-u1-membership-only", Status: "active"},
	}).Error; err != nil {
		t.Fatalf("create memberships: %v", err)
	}
	if err := db.Create(&[]DingTalkBinding{
		{OrgID: "org-a", UserID: current.UserID, DingTalkUserID: current.DingTalkUserID, UnionID: "union-u1"},
		{OrgID: "org-b", UserID: target.UserID, DingTalkUserID: target.DingTalkUserID, UnionID: "union-u1"},
		{OrgID: "org-c", UserID: configuredOnly.UserID, DingTalkUserID: configuredOnly.DingTalkUserID, UnionID: "union-u1"},
	}).Error; err != nil {
		t.Fatalf("create dingtalk bindings: %v", err)
	}

	got, err := ListSwitchableOrganizationsForUser(&current)
	if err != nil {
		t.Fatalf("ListSwitchableOrganizationsForUser: %v", err)
	}
	if len(got) != 2 || got[0].OrgID != "org-a" || got[1].OrgID != "org-b" {
		t.Fatalf("switchable orgs = %#v, want org-a and org-b", got)
	}
	resolved, err := FindSwitchableUser(&current, "org-b")
	if err != nil || resolved == nil || resolved.OrgID != "org-b" {
		t.Fatalf("FindSwitchableUser(org-b) = %#v, %v", resolved, err)
	}
	if _, err := FindSwitchableUser(&current, "org-c"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("FindSwitchableUser(org-c) error = %v, want record not found", err)
	}
}
