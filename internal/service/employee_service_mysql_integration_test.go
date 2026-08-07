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

func TestEmployeeServiceGetProfilesMySQLSearch(t *testing.T) {
	dsn := os.Getenv("TEST_MYSQL_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_MYSQL_DATABASE_URL is not configured")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.EmployeeProfile{}); err != nil {
		t.Fatalf("migrate employee profile search fixtures: %v", err)
	}

	orgID := fmt.Sprintf("employee-profile-search-%d", time.Now().UnixNano())
	defer func() {
		db.Unscoped().Where("org_id = ?", orgID).Delete(&database.EmployeeProfile{})
		db.Unscoped().Where("org_id = ?", orgID).Delete(&database.User{})
	}()
	user := database.User{
		OrgID:          orgID,
		UserID:         "anonymous-mysql-user",
		DingTalkUserID: "anonymous-mysql-dt",
		Name:           "匿名集成中文姓名",
		Mobile:         "13900000008",
		Email:          "anonymous-mysql@example.invalid",
		Position:       "集成测试工程师",
		Status:         "active",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed MySQL user: %v", err)
	}
	profile := database.EmployeeProfile{OrgID: orgID, UserID: user.UserID, EmployeeID: "ANON-MYSQL-001", ProfileStatus: "active"}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("seed MySQL profile: %v", err)
	}

	svc := NewEmployeeServiceWithOrgID(db, orgID)
	for _, keyword := range []string{
		"匿名集成中文姓名",
		"中文姓名",
		"  中文姓名  ",
		"ANON-MYSQL",
		"mysql-user",
		"00000008",
		"anonymous-mysql@",
		"测试工程",
	} {
		items, total, err := svc.GetProfiles(1, 20, map[string]string{"keyword": keyword})
		if err != nil {
			t.Fatalf("search employee profiles with %q: %v", keyword, err)
		}
		if total != 1 || len(items) != 1 || items[0].ID != profile.ID {
			t.Fatalf("keyword %q returned total=%d items=%d", keyword, total, len(items))
		}
	}
}
