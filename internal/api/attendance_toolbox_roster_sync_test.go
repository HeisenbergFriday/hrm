package api

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/dingtalk"
	"peopleops/internal/service"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGenerateOrgRosterFromDingTalkDepartmentSync(t *testing.T) {
	dsn := "file:api-roster-sync-" + strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&database.Department{},
		&database.DepartmentChangeLog{},
		&database.User{},
		&database.EmployeeProfile{},
	); err != nil {
		t.Fatalf("automigrate roster sync chain: %v", err)
	}
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })

	dingTalkDepartments := []dingtalk.DeptInfo{
		{DeptID: 1, ParentID: 0, Name: "企业根"},
		{DeptID: 2, ParentID: 1, Name: "运营管理中心"},
		{DeptID: 3, ParentID: 2, Name: "运营支撑部"},
		{DeptID: 4, ParentID: 3, Name: "智慧寄存运维组"},
	}
	testCases := []struct {
		orgID      string
		userID     string
		employeeID string
		name       string
	}{
		{orgID: database.DefaultOrganizationID, userID: "default-ops-user", employeeID: "DEF9999", name: "默认组织运维"},
		{orgID: "tenant-a", userID: "tenant-a-ops-user", employeeID: "MT9999", name: "测试运维"},
	}

	for index, testCase := range testCases {
		items := dingtalkDepartmentsToOrgSyncItems(testCase.orgID, dingTalkDepartments)
		if len(items) != len(dingTalkDepartments) {
			t.Fatalf("%s converted department count = %d", testCase.orgID, len(items))
		}
		wantRootParentID := database.ScopedExternalID(testCase.orgID, "0")
		if items[0].ParentID != wantRootParentID || items[0].DingTalkParentID != "0" {
			t.Fatalf("%s root conversion = ParentID %q DingTalkParentID %q, want %q/0", testCase.orgID, items[0].ParentID, items[0].DingTalkParentID, wantRootParentID)
		}

		result, err := service.NewOrgServiceWithOrgID(db, testCase.orgID).
			SyncDepartmentsWithChangeLog(testCase.orgID, items, "dingtalk_sync")
		if err != nil {
			t.Fatalf("%s sync departments: %v", testCase.orgID, err)
		}
		if result.Count != len(dingTalkDepartments) {
			t.Fatalf("%s synced department count = %d", testCase.orgID, result.Count)
		}

		var root database.Department
		if err := db.Where("org_id = ? AND department_id = ?", testCase.orgID, database.ScopedExternalID(testCase.orgID, "1")).First(&root).Error; err != nil {
			t.Fatalf("%s load synced root: %v", testCase.orgID, err)
		}
		if root.ParentID != wantRootParentID {
			t.Fatalf("%s persisted root parent = %q, want %q", testCase.orgID, root.ParentID, wantRootParentID)
		}

		user := database.User{
			OrgID: testCase.orgID, UserID: testCase.userID, DingTalkUserID: testCase.userID,
			Name: testCase.name, Email: "roster-sync-" + string(rune('a'+index)) + "@example.test",
			Mobile: "1380000000" + string(rune('1'+index)), DepartmentID: database.ScopedExternalID(testCase.orgID, "4"),
			Position: "运维工程师", Status: "active",
		}
		if err := db.Create(&user).Error; err != nil {
			t.Fatalf("%s seed user: %v", testCase.orgID, err)
		}
		profile := database.EmployeeProfile{OrgID: testCase.orgID, UserID: testCase.userID, EmployeeID: testCase.employeeID, EmploymentType: "正式"}
		if err := db.Create(&profile).Error; err != nil {
			t.Fatalf("%s seed profile: %v", testCase.orgID, err)
		}
	}

	for _, testCase := range testCases {
		generated, err := service.NewAttendanceToolboxService().GenerateOrgRosterExcel(context.Background(), testCase.orgID)
		if err != nil {
			t.Fatalf("%s GenerateOrgRosterExcel: %v", testCase.orgID, err)
		}
		rows := inspectSyncedRosterWorkbook(t, generated.Data)
		if len(rows) != 2 || len(rows[1]) != 12 {
			t.Fatalf("%s workbook dimensions: %#v", testCase.orgID, rows)
		}
		row := rows[1]
		if row[0] != testCase.employeeID || row[1] != testCase.name {
			t.Fatalf("%s roster identity leaked across organizations: %#v", testCase.orgID, row)
		}
		if row[3] != "运营管理中心" || row[4] != "运营支撑部" || row[5] != "智慧寄存运维组" {
			t.Fatalf("%s roster department path = %#v", testCase.orgID, row[3:6])
		}
	}
}

func inspectSyncedRosterWorkbook(t *testing.T, data []byte) [][]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "synced-roster.xlsx")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write synced roster: %v", err)
	}
	code := `import json,openpyxl,sys; w=openpyxl.load_workbook(sys.argv[1],data_only=True); s=w["在职花名册"]; print(json.dumps([[c.value for c in row] for row in s.iter_rows()])); w.close()`
	cmd := exec.Command(apiTestPython(t), "-c", code, path)
	cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("inspect synced roster: %v\n%s", err, output)
	}
	var rows [][]any
	if err := json.Unmarshal(bytes.TrimSpace(output), &rows); err != nil {
		t.Fatalf("decode synced roster: %v, output=%s", err, output)
	}
	return rows
}
