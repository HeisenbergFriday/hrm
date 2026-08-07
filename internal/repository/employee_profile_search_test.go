package repository

import (
	"fmt"
	"testing"

	"peopleops/internal/database"
)

func seedEmployeeProfileSearchFixtures(t *testing.T) (*EmployeeRepository, *database.EmployeeProfile, *database.EmployeeProfile) {
	t.Helper()
	db := openEmployeeIsolationSQLite(t)
	users := []database.User{
		{OrgID: "search-org", UserID: "anonymous-user-a", DingTalkUserID: "anonymous-dt-a", Name: "匿名中文姓名甲", Email: "anonymous-a@example.invalid", Mobile: "13900000001", DepartmentID: "dept-a", Position: "质量工程师", Status: "active"},
		{OrgID: "search-org", UserID: "anonymous-user-b", DingTalkUserID: "anonymous-dt-b", Name: "匿名中文姓名乙", Email: "anonymous-b@example.invalid", Mobile: "13900000002", DepartmentID: "dept-b", Position: "产品专员", Status: "active"},
		{OrgID: "other-org", UserID: "anonymous-user-other", DingTalkUserID: "anonymous-dt-other", Name: "匿名中文姓名甲", Email: "anonymous-other@example.invalid", Mobile: "13900000003", DepartmentID: "dept-a", Position: "质量工程师", Status: "active"},
		{OrgID: "search-org", UserID: "deleted-user", DingTalkUserID: "anonymous-dt-deleted", Name: "软删除员工", Email: "deleted-user@example.invalid", Mobile: "13900000004", DepartmentID: "dept-a", Position: "测试岗位", Status: "active"},
		{OrgID: "search-org", UserID: "deleted-profile-user", DingTalkUserID: "anonymous-dt-deleted-profile", Name: "软删除档案", Email: "deleted-profile@example.invalid", Mobile: "13900000005", DepartmentID: "dept-a", Position: "测试岗位", Status: "active"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("seed search users: %v", err)
	}
	profiles := []database.EmployeeProfile{
		{OrgID: "search-org", UserID: users[0].UserID, EmployeeID: "ANON-EMP-001", ProfileStatus: "active"},
		{OrgID: "search-org", UserID: users[1].UserID, EmployeeID: "ANON-EMP-002", ProfileStatus: "active"},
		{OrgID: "other-org", UserID: users[2].UserID, EmployeeID: "ANON-EMP-OTHER", ProfileStatus: "active"},
		{OrgID: "search-org", UserID: users[3].UserID, EmployeeID: "ANON-EMP-DELETED-USER", ProfileStatus: "active"},
		{OrgID: "search-org", UserID: users[4].UserID, EmployeeID: "ANON-EMP-DELETED-PROFILE", ProfileStatus: "active"},
	}
	if err := db.Create(&profiles).Error; err != nil {
		t.Fatalf("seed search profiles: %v", err)
	}
	if err := db.Delete(&users[3]).Error; err != nil {
		t.Fatalf("soft delete user: %v", err)
	}
	if err := db.Delete(&profiles[4]).Error; err != nil {
		t.Fatalf("soft delete profile: %v", err)
	}
	return NewEmployeeRepositoryWithOrgID(db, "search-org"), &profiles[0], &profiles[1]
}

func TestEmployeeRepository_FindAllProfilesKeywordFields(t *testing.T) {
	repo, first, _ := seedEmployeeProfileSearchFixtures(t)
	tests := []struct {
		name    string
		keyword string
	}{
		{name: "full Chinese name", keyword: "匿名中文姓名甲"},
		{name: "partial Chinese name", keyword: "中文姓名甲"},
		{name: "employee number", keyword: "EMP-001"},
		{name: "user id", keyword: "user-a"},
		{name: "mobile", keyword: "00000001"},
		{name: "email", keyword: "anonymous-a@"},
		{name: "position", keyword: "质量工程"},
		{name: "trimmed keyword", keyword: "  中文姓名甲  "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			items, total, err := repo.FindAllProfiles(1, 20, map[string]string{"keyword": tt.keyword})
			if err != nil {
				t.Fatalf("search profiles: %v", err)
			}
			if total != 1 || len(items) != 1 || items[0].ID != first.ID {
				t.Fatalf("keyword %q returned total=%d items=%v", tt.keyword, total, profileIDs(items))
			}
		})
	}
}

func TestEmployeeRepository_FindAllProfilesKeywordIsolationAndSafety(t *testing.T) {
	repo, first, second := seedEmployeeProfileSearchFixtures(t)

	items, total, err := repo.FindAllProfiles(1, 1, map[string]string{"keyword": "匿名中文姓名"})
	if err != nil {
		t.Fatalf("search all matching profiles: %v", err)
	}
	if total != 2 || len(items) != 1 {
		t.Fatalf("joined pagination total=%d len=%d, want total 2 and one page item", total, len(items))
	}
	if items[0].OrgID != "search-org" {
		t.Fatalf("cross-org profile leaked: org=%q", items[0].OrgID)
	}

	items, total, err = repo.FindAllProfiles(1, 20, map[string]string{
		"keyword":        "匿名中文姓名",
		"department_ids": "dept-a",
	})
	if err != nil {
		t.Fatalf("department-scoped search: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != first.ID {
		t.Fatalf("department scope expanded by search: total=%d items=%v", total, profileIDs(items))
	}

	items, total, err = repo.FindAllProfiles(1, 20, map[string]string{
		"keyword": "匿名中文姓名",
		"user_id": second.UserID,
	})
	if err != nil {
		t.Fatalf("self-scoped search: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("self scope expanded by search: total=%d items=%v", total, profileIDs(items))
	}

	for _, keyword := range []string{"不存在的关键字", "' OR 1=1 --", "软删除员工", "软删除档案"} {
		items, total, err = repo.FindAllProfiles(1, 20, map[string]string{"keyword": keyword})
		if err != nil {
			t.Fatalf("safe no-result search %q: %v", keyword, err)
		}
		if total != 0 || len(items) != 0 {
			t.Fatalf("keyword %q unexpectedly returned total=%d items=%v", keyword, total, profileIDs(items))
		}
	}
}

func profileIDs(items []database.EmployeeProfile) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, fmt.Sprintf("%d", item.ID))
	}
	return ids
}
