package service

import (
	"fmt"
	"os"
	"testing"
	"time"

	"peopleops/internal/database"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func TestOrgServiceListEmployeesMySQLSearch(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DATABASE_URL is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	if db.Name() != "mysql" {
		t.Fatalf("dialector = %q, want mysql", db.Name())
	}
	if err := db.AutoMigrate(&database.Department{}, &database.User{}, &database.EmployeeProfile{}); err != nil {
		t.Fatalf("migrate MySQL search fixtures: %v", err)
	}

	orgID := fmt.Sprintf("mysql-search-%d", time.Now().UnixNano())
	defer func() {
		db.Unscoped().Where("org_id = ?", orgID).Delete(&database.EmployeeProfile{})
		db.Unscoped().Where("org_id = ?", orgID).Delete(&database.User{})
	}()

	users := []database.User{
		{OrgID: orgID, UserID: "search-user-a", DingTalkUserID: "search-dingtalk-a", Name: "集成中文姓名甲", Mobile: "13900000001", Email: "search-a@example.invalid", Position: "集成测试工程师", Status: "active"},
		{OrgID: orgID, UserID: "search-user-b", DingTalkUserID: "search-dingtalk-b", Name: "集成中文姓名乙", Mobile: "13900000002", Email: "search-b@example.invalid", Position: "质量验证专员", Status: "active"},
	}
	if err := db.Create(&users).Error; err != nil {
		t.Fatalf("seed MySQL users: %v", err)
	}
	profiles := []database.EmployeeProfile{
		{OrgID: orgID, UserID: users[0].UserID, EmployeeID: "EMP-MYSQL-001", ProfileStatus: "active"},
		{OrgID: orgID, UserID: users[1].UserID, EmployeeID: "EMP-MYSQL-002", ProfileStatus: "active"},
	}
	if err := db.Create(&profiles).Error; err != nil {
		t.Fatalf("seed MySQL profiles: %v", err)
	}

	svc := NewOrgServiceWithOrgID(db, orgID)
	tests := []struct {
		name   string
		search string
		wantID string
	}{
		{name: "full Chinese name", search: "集成中文姓名甲", wantID: users[0].UserID},
		{name: "partial Chinese name", search: "中文姓名甲", wantID: users[0].UserID},
		{name: "trimmed keyword", search: "  中文姓名甲  ", wantID: users[0].UserID},
		{name: "employee number", search: "EMP-MYSQL-001", wantID: users[0].UserID},
		{name: "mobile", search: "00000001", wantID: users[0].UserID},
		{name: "email", search: "search-a@", wantID: users[0].UserID},
		{name: "position", search: "测试工程", wantID: users[0].UserID},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, total, err := svc.ListEmployees(nil, 1, 20, OrgEmployeeFilters{Search: tt.search})
			if err != nil {
				t.Fatalf("search employees: %v", err)
			}
			if total != 1 || len(got) != 1 || got[0].UserID != tt.wantID {
				t.Fatalf("search %q returned total=%d items=%d", tt.search, total, len(got))
			}
		})
	}
}
