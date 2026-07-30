package service

import (
	"testing"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openOrgDepartmentSyncTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:org-department-sync-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.Department{}, &database.DepartmentChangeLog{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}

func TestSyncDepartmentsWithChangeLogMatchesHistoricalRowsByDingTalkID(t *testing.T) {
	db := openOrgDepartmentSyncTestDB(t)
	orgID := "history-org"
	seed := []database.Department{
		{OrgID: orgID, DepartmentID: "1", DingTalkDepartmentID: "1", Name: "旧根部门"},
		{OrgID: orgID, DepartmentID: "2", DingTalkDepartmentID: "2", Name: "旧研发部", ParentID: "1"},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}

	result, err := NewOrgServiceWithOrgID(db, orgID).SyncDepartmentsWithChangeLog(orgID, []OrgDepartmentSyncItem{
		{
			DepartmentID:         "history-org::dept::1",
			DingTalkDepartmentID: "1",
			DingTalkParentID:     "0",
			Name:                 "根部门",
		},
		{
			DepartmentID:         "history-org::dept::2",
			DingTalkDepartmentID: "2",
			DingTalkParentID:     "1",
			Name:                 "研发部",
		},
	}, "dingtalk_sync")
	if err != nil {
		t.Fatalf("sync departments: %v", err)
	}
	if result.Count != 2 {
		t.Fatalf("result count = %d, want 2", result.Count)
	}

	var departments []database.Department
	if err := db.Where("org_id = ?", orgID).Order("department_id").Find(&departments).Error; err != nil {
		t.Fatalf("query departments: %v", err)
	}
	if len(departments) != 2 {
		t.Fatalf("department count = %d, want 2: %#v", len(departments), departments)
	}
	if departments[0].DepartmentID != "1" || departments[0].Name != "根部门" {
		t.Fatalf("historical root id was not preserved: %#v", departments[0])
	}
	if departments[1].DepartmentID != "2" || departments[1].ParentID != "1" || departments[1].Name != "研发部" {
		t.Fatalf("historical child identity or parent was not preserved: %#v", departments[1])
	}
}

func TestSyncDepartmentsWithChangeLogRollsBackAllWritesOnPersistFailure(t *testing.T) {
	db := openOrgDepartmentSyncTestDB(t)
	orgID := "rollback-org"
	if err := db.Create(&database.Department{
		OrgID:                orgID,
		DepartmentID:         "1",
		DingTalkDepartmentID: "1",
		Name:                 "原根部门",
	}).Error; err != nil {
		t.Fatalf("seed department: %v", err)
	}
	if err := db.Exec(`CREATE TRIGGER fail_second_department_insert
		BEFORE INSERT ON departments
		WHEN NEW.department_id = 'rollback-org::dept::2'
		BEGIN
			SELECT RAISE(ABORT, 'forced department insert failure');
		END`).Error; err != nil {
		t.Fatalf("create failure trigger: %v", err)
	}

	_, err := NewOrgServiceWithOrgID(db, orgID).SyncDepartmentsWithChangeLog(orgID, []OrgDepartmentSyncItem{
		{
			DepartmentID:         "rollback-org::dept::1",
			DingTalkDepartmentID: "1",
			DingTalkParentID:     "0",
			Name:                 "不应提交的新名称",
		},
		{
			DepartmentID:         "rollback-org::dept::2",
			DingTalkDepartmentID: "2",
			DingTalkParentID:     "1",
			Name:                 "触发失败的子部门",
		},
	}, "dingtalk_sync")
	if err == nil {
		t.Fatal("sync unexpectedly succeeded")
	}

	var stored database.Department
	if err := db.Where("org_id = ? AND department_id = ?", orgID, "1").First(&stored).Error; err != nil {
		t.Fatalf("query root department: %v", err)
	}
	if stored.Name != "原根部门" {
		t.Fatalf("root update was not rolled back: %#v", stored)
	}
	var departmentCount int64
	if err := db.Model(&database.Department{}).Where("org_id = ?", orgID).Count(&departmentCount).Error; err != nil {
		t.Fatalf("count departments: %v", err)
	}
	if departmentCount != 1 {
		t.Fatalf("department count = %d, want 1 after rollback", departmentCount)
	}
	var logCount int64
	if err := db.Model(&database.DepartmentChangeLog{}).Where("org_id = ?", orgID).Count(&logCount).Error; err != nil {
		t.Fatalf("count change logs: %v", err)
	}
	if logCount != 0 {
		t.Fatalf("change log count = %d, want 0 after rollback", logCount)
	}
}
