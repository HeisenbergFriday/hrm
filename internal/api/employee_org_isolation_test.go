package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"peopleops/internal/database"
	"peopleops/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openEmployeeProfileHandlerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:api-employee-profile-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open employee profile handler db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.EmployeeProfile{}); err != nil {
		t.Fatalf("migrate employee profile handler db: %v", err)
	}
	original := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = original })
	return db
}

func seedEmployeeProfileHandlerUser(t *testing.T, db *gorm.DB, orgID, userID, name, departmentID string) database.EmployeeProfile {
	t.Helper()
	user := database.User{
		OrgID:          orgID,
		UserID:         userID,
		DingTalkUserID: "dt-" + userID,
		Name:           name,
		Email:          userID + "@example.invalid",
		Mobile:         "mobile-" + userID,
		DepartmentID:   departmentID,
		Position:       "匿名测试岗位",
		Status:         "active",
	}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("seed handler user: %v", err)
	}
	profile := database.EmployeeProfile{OrgID: orgID, UserID: userID, EmployeeID: "EMP-" + userID, ProfileStatus: "active"}
	if err := db.Create(&profile).Error; err != nil {
		t.Fatalf("seed handler profile: %v", err)
	}
	return profile
}

func employeeProfileResponseItems(t *testing.T, body []byte) []map[string]any {
	t.Helper()
	var response struct {
		Data struct {
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("decode employee profile response: %v; body=%s", err, string(body))
	}
	return response.Data.Items
}

// TestGetEmployeeProfiles_MissingOrgContextFailClosed 验证普通请求缺组织时不得返回员工数据。
func TestGetEmployeeProfiles_MissingOrgContextFailClosed(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/employee-profiles?department_id=same-dept", "", "")
	GetEmployeeProfiles(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "组织") {
		t.Fatalf("body should mention missing org, got %s", recorder.Body.String())
	}
}

func TestGetEmployeeProfile_MissingOrgContextFailClosed(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/employee-profiles/1", "", "")
	c.Params = gin.Params{{Key: "id", Value: "1"}}
	GetEmployeeProfile(c)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetEmployeeProfiles_RejectsCrossOrgQueryParam(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodGet, "/api/v1/employee-profiles?org_id=other-org", "", "org-a")
	GetEmployeeProfiles(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "不允许通过参数切换到其它组织") {
		t.Fatalf("unexpected body: %s", recorder.Body.String())
	}
}

func TestCreateEmployeeProfile_RejectsCrossOrgBody(t *testing.T) {
	c, recorder := newSecurityCtx(t, http.MethodPost, "/api/v1/employee-profiles",
		`{"user_id":"u1","employee_id":"e1","org_id":"other-org"}`, "org-a")
	CreateEmployeeProfile(c)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestGetEmployeeProfiles_ReceivesAndTrimsKeyword(t *testing.T) {
	db := openEmployeeProfileHandlerDB(t)
	profile := seedEmployeeProfileHandlerUser(t, db, "org-a", "anonymous-handler-user", "匿名接口中文姓名", "dept-a")
	seedEmployeeProfileHandlerUser(t, db, "org-a", "anonymous-other-user", "匿名其他员工", "dept-a")

	target := "/api/v1/employee/profiles?page=1&page_size=20&keyword=" + url.QueryEscape("  接口中文  ")
	c, recorder := newSecurityCtx(t, http.MethodGet, target, "", "org-a")
	middleware.SetAuthContextForTest(c, &middleware.AuthContext{
		OrgID:         "org-a",
		UserID:        "tester",
		PermissionSet: map[string]struct{}{"user_manage": {}},
		MenuKeySet:    map[string]struct{}{},
	})
	GetEmployeeProfiles(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	items := employeeProfileResponseItems(t, recorder.Body.Bytes())
	if len(items) != 1 || uint(items[0]["id"].(float64)) != profile.ID {
		t.Fatalf("trimmed handler keyword returned %#v", items)
	}
}

func TestGetEmployeeProfiles_KeywordKeepsDepartmentDataScope(t *testing.T) {
	db := openEmployeeProfileHandlerDB(t)
	allowed := seedEmployeeProfileHandlerUser(t, db, "org-a", "anonymous-scope-a", "匿名范围姓名", "dept-a")
	seedEmployeeProfileHandlerUser(t, db, "org-a", "anonymous-scope-b", "匿名范围姓名", "dept-b")

	target := "/api/v1/employee/profiles?keyword=" + url.QueryEscape("匿名范围姓名")
	c, recorder := newSecurityCtx(t, http.MethodGet, target, "", "org-a")
	middleware.SetAuthContextForTest(c, &middleware.AuthContext{
		OrgID:         "org-a",
		UserID:        "tester",
		PermissionSet: map[string]struct{}{},
		MenuKeySet:    map[string]struct{}{},
		Roles:         []database.Role{{ID: 1, OrgID: "org-a"}},
		DataPermissions: []database.DataPermission{{
			OrgID:          "org-a",
			RoleID:         1,
			Scope:          "department",
			DepartmentKeys: `["dept-a"]`,
		}},
	})
	GetEmployeeProfiles(c)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	items := employeeProfileResponseItems(t, recorder.Body.Bytes())
	if len(items) != 1 || uint(items[0]["id"].(float64)) != allowed.ID {
		t.Fatalf("keyword expanded department data scope: %#v", items)
	}
}
