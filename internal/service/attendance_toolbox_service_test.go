package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peopleops/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAttendanceToolboxDefaultsPreserveChinese(t *testing.T) {
	defaults, err := NewAttendanceToolboxService().Defaults(context.Background())
	if err != nil {
		t.Fatalf("read attendance toolbox defaults: %v", err)
	}
	specialNames := strings.Join(defaults["leave_special_names"], "、")
	if !strings.Contains(specialNames, "梁伯林") || strings.ContainsRune(specialNames, '\uFFFD') {
		t.Fatalf("unexpected Chinese defaults: %q", specialNames)
	}
}

func TestGenerateOrgRosterExcelRequiresOrg(t *testing.T) {
	_, err := NewAttendanceToolboxService().GenerateOrgRosterExcel(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "org_id") {
		t.Fatalf("expected org_id error, got %v", err)
	}
}

func TestGenerateOrgRosterExcelRequiresEngineDir(t *testing.T) {
	_, err := (&AttendanceToolboxService{}).GenerateOrgRosterExcel(context.Background(), "org-test")
	if !errors.Is(err, ErrRosterEngineDir) {
		t.Fatalf("expected ErrRosterEngineDir, got %v", err)
	}
}

func TestGenerateOrgRosterExcelNoValidEmployees(t *testing.T) {
	withRosterTestDB(t)
	_, err := NewAttendanceToolboxService().GenerateOrgRosterExcel(context.Background(), "org-empty")
	if !errors.Is(err, ErrRosterNoEmployees) {
		t.Fatalf("expected ErrRosterNoEmployees, got %v", err)
	}
}

func TestBuildRosterEmployeesUsesEmployeeProfileIDAndRealDepartmentPath(t *testing.T) {
	users := []database.User{
		{OrgID: "org-a", UserID: "user-1", DingTalkUserID: "ding-1", Name: "测试运维", DepartmentID: "ops", Position: "运维工程师", Status: "active"},
	}
	profiles := map[string]database.EmployeeProfile{
		"user-1": {OrgID: "org-a", UserID: "user-1", EmployeeID: "MT9999", EmploymentType: "正式", EntryDate: "2026-01-02"},
	}
	paths := map[string][]string{
		"ops": {"企业根", "运营管理中心", "运营支撑部", "智慧寄存运维组"},
	}

	employees, missingEmpNo, missingDeptPath := buildRosterEmployees(users, profiles, paths)
	if missingEmpNo != 0 || missingDeptPath != 0 || len(employees) != 1 {
		t.Fatalf("unexpected build result: employees=%#v missingEmpNo=%d missingDeptPath=%d", employees, missingEmpNo, missingDeptPath)
	}
	got := employees[0]
	if got.EmpNo != "MT9999" || got.EmpNo == users[0].UserID || got.EmpNo == users[0].DingTalkUserID {
		t.Fatalf("business employee ID was not sourced authoritatively: %#v", got)
	}
	if got.Dept1 != "运营管理中心" || got.Dept2 != "运营支撑部" || got.Dept3 != "智慧寄存运维组" {
		t.Fatalf("unexpected department path: %#v", got)
	}
}

func TestBuildRosterEmployeesMissingEmployeeIDNeverFallsBack(t *testing.T) {
	users := []database.User{
		{OrgID: "org-a", UserID: "user-as-id", DingTalkUserID: "ding-as-id", Name: "无工号员工", DepartmentID: "dept", Status: "active"},
	}
	employees, missingEmpNo, missingDeptPath := buildRosterEmployees(
		users,
		nil,
		map[string][]string{"dept": {"总部"}},
	)
	if len(employees) != 1 || employees[0].EmpNo != "" || missingEmpNo != 1 || missingDeptPath != 0 {
		t.Fatalf("missing EmployeeID must fail without fallback: employees=%#v missingEmpNo=%d missingDeptPath=%d", employees, missingEmpNo, missingDeptPath)
	}
}

func TestBuildRosterEmployeesMissingDepartmentPathIsCounted(t *testing.T) {
	users := []database.User{{OrgID: "org-a", UserID: "u1", Name: "无部门员工", DepartmentID: "missing", Status: "active"}}
	profiles := map[string]database.EmployeeProfile{"u1": {OrgID: "org-a", UserID: "u1", EmployeeID: "MT1000"}}
	employees, missingEmpNo, missingDeptPath := buildRosterEmployees(users, profiles, nil)
	if len(employees) != 1 || missingEmpNo != 0 || missingDeptPath != 1 {
		t.Fatalf("unresolved department must be counted: employees=%#v missingEmpNo=%d missingDeptPath=%d", employees, missingEmpNo, missingDeptPath)
	}
}

func TestBuildRosterEmployeesAllowsDuplicateNamesBecauseEmployeeIDIsPrimary(t *testing.T) {
	users := []database.User{
		{OrgID: "org-a", UserID: "u1", Name: "同名员工", DepartmentID: "dept", Status: "active"},
		{OrgID: "org-a", UserID: "u2", Name: "同名员工", DepartmentID: "dept", Status: "active"},
	}
	profiles := map[string]database.EmployeeProfile{
		"u1": {OrgID: "org-a", UserID: "u1", EmployeeID: "MT1001"},
		"u2": {OrgID: "org-a", UserID: "u2", EmployeeID: "MT1002"},
	}
	employees, missingEmpNo, missingDeptPath := buildRosterEmployees(users, profiles, map[string][]string{"dept": {"总部"}})
	if len(employees) != 2 || missingEmpNo != 0 || missingDeptPath != 0 || employees[0].EmpNo == employees[1].EmpNo {
		t.Fatalf("duplicate names must remain separated by EmployeeID: %#v", employees)
	}
}

func TestGenerateOrgRosterExcelMissingEmployeeIDFailsClosed(t *testing.T) {
	db := withRosterTestDB(t)
	seedRosterDepartment(t, db, "org-missing-id", "dept", "总部", "0")
	seedRosterUser(t, db, database.User{
		OrgID: "org-missing-id", UserID: "user-no-id", DingTalkUserID: "ding-no-id", Name: "无工号员工",
		Email: "no-id@example.test", Mobile: "13800000001", DepartmentID: "dept", Status: "active",
	})

	_, err := NewAttendanceToolboxService().GenerateOrgRosterExcel(context.Background(), "org-missing-id")
	if !errors.Is(err, ErrRosterMissingEmpNo) || !strings.Contains(err.Error(), "1 名") {
		t.Fatalf("expected explicit missing EmployeeID error, got %v", err)
	}
}

func TestGenerateOrgRosterExcelMissingDepartmentPathFailsClosed(t *testing.T) {
	db := withRosterTestDB(t)
	seedRosterUser(t, db, database.User{
		OrgID: "org-missing-dept", UserID: "u1", DingTalkUserID: "d1", Name: "无部门员工",
		Email: "missing-dept@example.test", Mobile: "13800000002", DepartmentID: "unknown", Status: "active",
	})
	if err := db.Create(&database.EmployeeProfile{OrgID: "org-missing-dept", UserID: "u1", EmployeeID: "MT1000"}).Error; err != nil {
		t.Fatalf("seed profile: %v", err)
	}

	_, err := NewAttendanceToolboxService().GenerateOrgRosterExcel(context.Background(), "org-missing-dept")
	if !errors.Is(err, ErrRosterMissingDeptPath) || !strings.Contains(err.Error(), "1 名") {
		t.Fatalf("expected explicit missing department path error, got %v", err)
	}
}

func TestLoadRosterEmployeesForOrgIsolatesUsersProfilesAndDepartments(t *testing.T) {
	db := withRosterTestDB(t)
	for _, orgID := range []string{"org-a", "org-b"} {
		seedRosterDepartment(t, db, orgID, "shared-dept", "部门-"+orgID, "0")
	}
	seedRosterUser(t, db, database.User{
		OrgID: "org-a", UserID: "shared-user", DingTalkUserID: "ding-a", Name: "员工A", Email: "a@example.test",
		Mobile: "13800000003", DepartmentID: "shared-dept", Status: "active",
	})
	seedRosterUser(t, db, database.User{
		OrgID: "org-b", UserID: "shared-user", DingTalkUserID: "ding-b", Name: "员工B", Email: "b@example.test",
		Mobile: "13800000004", DepartmentID: "shared-dept", Status: "active",
	})
	profiles := []database.EmployeeProfile{
		{OrgID: "org-a", UserID: "shared-user", EmployeeID: "EA001"},
		{OrgID: "org-b", UserID: "shared-user", EmployeeID: "EB001"},
	}
	if err := db.Create(&profiles).Error; err != nil {
		t.Fatalf("seed profiles: %v", err)
	}

	employees, missingEmpNo, missingDeptPath, err := (&AttendanceToolboxService{}).loadRosterEmployeesForOrg("org-a")
	if err != nil || len(employees) != 1 || missingEmpNo != 0 || missingDeptPath != 0 {
		t.Fatalf("unexpected org-a result: employees=%#v missingEmpNo=%d missingDeptPath=%d err=%v", employees, missingEmpNo, missingDeptPath, err)
	}
	if employees[0].Name != "员工A" || employees[0].EmpNo != "EA001" || employees[0].Dept1 != "部门-org-a" {
		t.Fatalf("cross-org data leaked into roster: %#v", employees[0])
	}
}

func TestBuildDepartmentPathMapRejectsDanglingAndCyclicPaths(t *testing.T) {
	db := withRosterTestDB(t)
	departments := []database.Department{
		{OrgID: "org-a", DepartmentID: "valid", DingTalkDepartmentID: "dt-valid", Name: "总部", ParentID: "0"},
		{OrgID: "org-a", DepartmentID: "dangling", DingTalkDepartmentID: "dt-dangling", Name: "孤儿部门", ParentID: "missing"},
		{OrgID: "org-a", DepartmentID: "cycle-a", DingTalkDepartmentID: "dt-cycle-a", Name: "循环A", ParentID: "cycle-b"},
		{OrgID: "org-a", DepartmentID: "cycle-b", DingTalkDepartmentID: "dt-cycle-b", Name: "循环B", ParentID: "cycle-a"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}
	paths, err := (&AttendanceToolboxService{}).buildDepartmentPathMap("org-a")
	if err != nil {
		t.Fatalf("build paths: %v", err)
	}
	if len(paths) != 1 || paths["valid"][0] != "总部" {
		t.Fatalf("invalid paths must be excluded: %#v", paths)
	}
}

func TestGenerateOrgRosterExcelProducesRealRichWorkbookFromCurrentOrg(t *testing.T) {
	db := withRosterTestDB(t)
	departments := []database.Department{
		{OrgID: "org-a", DepartmentID: "root", DingTalkDepartmentID: "dt-root-a", Name: "企业根", ParentID: "0"},
		{OrgID: "org-a", DepartmentID: "center", DingTalkDepartmentID: "dt-center-a", Name: "运营管理中心", ParentID: "root"},
		{OrgID: "org-a", DepartmentID: "support", DingTalkDepartmentID: "dt-support-a", Name: "运营支撑部", ParentID: "center"},
		{OrgID: "org-a", DepartmentID: "ops", DingTalkDepartmentID: "dt-ops-a", Name: "智慧寄存运维组", ParentID: "support"},
		{OrgID: "org-b", DepartmentID: "ops", DingTalkDepartmentID: "dt-ops-b", Name: "其他组织部门", ParentID: "0"},
	}
	if err := db.Create(&departments).Error; err != nil {
		t.Fatalf("seed departments: %v", err)
	}
	seedRosterUser(t, db, database.User{
		OrgID: "org-a", UserID: "user-ops", DingTalkUserID: "ding-ops", Name: "测试运维",
		Email: "ops@a.test", Mobile: "13800000005", DepartmentID: "ops", Position: "运维工程师", Status: "active",
	})
	seedRosterUser(t, db, database.User{
		OrgID: "org-b", UserID: "user-other", DingTalkUserID: "ding-other", Name: "其他员工",
		Email: "other@b.test", Mobile: "13800000006", DepartmentID: "ops", Status: "active",
	})
	profiles := []database.EmployeeProfile{
		{OrgID: "org-a", UserID: "user-ops", EmployeeID: "MT9999", EmploymentType: "正式", EntryDate: "2026-01-02", ActualRegularDate: "2026-04-02"},
		{OrgID: "org-b", UserID: "user-other", EmployeeID: "OTHER001"},
	}
	if err := db.Create(&profiles).Error; err != nil {
		t.Fatalf("seed profiles: %v", err)
	}

	result, err := NewAttendanceToolboxService().GenerateOrgRosterExcel(context.Background(), "org-a")
	if err != nil {
		t.Fatalf("GenerateOrgRosterExcel: %v", err)
	}
	rows := inspectRosterWorkbook(t, result.Data)
	if len(rows) != 2 || len(rows[0]) != 12 {
		t.Fatalf("unexpected workbook dimensions: %#v", rows)
	}
	wantHeaders := []any{"工号", "姓名", "合同主体", "一级部门", "二级部门", "三级部门", "岗位", "员工类型", "人员分类", "入职日期", "离职日期", "转正日期"}
	for index, want := range wantHeaders {
		if rows[0][index] != want {
			t.Fatalf("header[%d]=%v, want %v", index, rows[0][index], want)
		}
	}
	row := rows[1]
	if row[0] != "MT9999" || row[1] != "测试运维" || row[3] != "运营管理中心" || row[4] != "运营支撑部" || row[5] != "智慧寄存运维组" {
		t.Fatalf("unexpected generated roster row: %#v", row)
	}
	if row[0] == "user-ops" || row[0] == "ding-ops" || row[0] == "OTHER001" {
		t.Fatalf("generated business employee ID was forged or leaked: %#v", row)
	}
}

func inspectRosterWorkbook(t *testing.T, data []byte) [][]any {
	t.Helper()
	path := filepath.Join(t.TempDir(), "roster.xlsx")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write roster: %v", err)
	}
	code := `import json,openpyxl,sys; w=openpyxl.load_workbook(sys.argv[1],data_only=True); s=w["在职花名册"]; print(json.dumps([[c.value for c in row] for row in s.iter_rows()])); w.close()`
	output, err := exec.Command(findPython(t), "-c", code, path).CombinedOutput()
	if err != nil {
		t.Fatalf("inspect roster: %v\n%s", err, output)
	}
	var rows [][]any
	if err := json.Unmarshal(output, &rows); err != nil {
		t.Fatalf("decode roster rows: %v, output=%s", err, output)
	}
	return rows
}

const capturingRosterRunner = `
import json
import os
import sys

def arg(name):
    index = sys.argv.index(name)
    return sys.argv[index + 1]

workdir = arg("--workdir")
config = json.loads(arg("--config-json"))
output_dir = os.path.join(workdir, "outputs")
os.makedirs(output_dir, exist_ok=True)
output_path = os.path.join(output_dir, "captured.json")
with open(output_path, "w", encoding="utf-8") as handle:
    json.dump(config, handle, ensure_ascii=False)
print(json.dumps({"ok": True, "outputs": [{"path": output_path, "file_name": "captured.xlsx", "kind": "export", "row_count": len(config.get("employees", []))}]}))
`

func newCapturingRosterService(t *testing.T) *AttendanceToolboxService {
	t.Helper()
	engineDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(engineDir, "runner.py"), []byte(capturingRosterRunner), 0o600); err != nil {
		t.Fatalf("write runner: %v", err)
	}
	return &AttendanceToolboxService{engineDir: engineDir, pythonBin: findPython(t), timeout: 10 * time.Second}
}

func findPython(t *testing.T) string {
	t.Helper()
	for _, candidate := range []string{"python", "python3"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path
		}
	}
	t.Skip("python not found on PATH")
	return ""
}

func seedRosterDepartment(t *testing.T, db *gorm.DB, orgID, departmentID, name, parentID string) {
	t.Helper()
	if err := db.Create(&database.Department{OrgID: orgID, DepartmentID: departmentID, Name: name, ParentID: parentID}).Error; err != nil {
		t.Fatalf("seed department: %v", err)
	}
}

func seedRosterUser(t *testing.T, db *gorm.DB, user database.User) {
	t.Helper()
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
}

func withRosterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := openRosterTestDB(t)
	originalDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = originalDB })
	return db
}

func openRosterTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:roster-test-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Department{}, &database.EmployeeProfile{}, &database.Organization{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return db
}
